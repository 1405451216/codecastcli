package cmd

// interactive_handlers.go: 斜杠命令处理器（从 interactive.go 拆分）
//
// 包含所有 handleXxxCommand 函数及其辅助函数。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codecast/cli/internal/agent"
	"codecast/cli/internal/config"
	"codecast/cli/internal/hooks"
	"codecast/cli/internal/indexer"
	"codecast/cli/internal/model"
	"codecast/cli/internal/permission"
	"codecast/cli/internal/rules"
	"codecast/cli/internal/subagent"
	"codecast/cli/internal/tui"
	"codecast/cli/internal/vision"

	"github.com/fatih/color"
)

// humanizeBytes 将字节数转为人类可读格式
func humanizeBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}

// showRuleSource 显示规则文件来源信息
func showRuleSource(path, level, content string) {
	if content == "" {
		return
	}
	color.Green("  ✓ %s (%s, %s)", path, level, humanizeBytes(len(content)))
}

// handleModeCommand 处理 /mode 命令，切换权限模式
func handleModeCommand(args string, ag *agent.CodecastAgent) {
	mode := strings.TrimSpace(args)
	if mode == "" {
		permMgr := ag.PermMgr()
		if permMgr != nil {
			color.Cyan("当前权限模式: %s", permMgr.ModeName())
			fmt.Println("可选模式: suggest, auto-edit, full-auto")
		}
		return
	}
	permMgr := ag.PermMgr()
	if permMgr == nil {
		color.Red("权限管理器未初始化")
		return
	}
	newMode, err := permission.ParseApprovalMode(mode)
	if err != nil {
		color.Red("%v", err)
		return
	}
	permMgr.SetMode(newMode)
	color.Green("✓ 权限模式已切换为: %s", permMgr.ModeName())
}

// handleRulesCommand 处理 /rules 命令
func handleRulesCommand(args string, ag *agent.CodecastAgent) {
	switch strings.TrimSpace(args) {
	case "", "show":
		loader := rules.NewLoader(".")
		rs, err := loader.Load()
		if err != nil {
			color.Red("加载规则失败: %v", err)
			return
		}
		if rs.Merged == "" {
			color.Yellow("未找到项目规则")
			color.White("使用 /rules init 初始化项目配置")
			return
		}
		color.Cyan("已加载的规则:")
		homeDir, _ := os.UserHomeDir()
		showRuleSource(filepath.Join(homeDir, ".codecast", "rules.md"), "全局", rs.Global)
		showRuleSource(".codecast/rules.md", "项目", rs.Project)
		showRuleSource(".codecast/rules.local.md", "本地", rs.Local)
		for _, sm := range rs.SubModules {
			showRuleSource(filepath.Join(".codecast", "rules", sm.Filename), "子模块", sm.Content)
		}
		totalSize := len(rs.Global) + len(rs.Project) + len(rs.Local)
		for _, sm := range rs.SubModules {
			totalSize += len(sm.Content)
		}
		sources := 0
		if rs.Global != "" {
			sources++
		}
		if rs.Project != "" {
			sources++
		}
		if rs.Local != "" {
			sources++
		}
		sources += len(rs.SubModules)
		color.HiBlack("总大小: %s | 来源: %d 个文件", humanizeBytes(totalSize), sources)
		fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")
		fmt.Println(rs.Merged)
	case "init":
		if err := rules.InitProject("."); err != nil {
			if strings.Contains(err.Error(), "已存在") {
				color.Yellow("项目规则已存在: %v", err)
			} else {
				color.Red("初始化失败: %v", err)
			}
			return
		}
		color.Green("✓ 已创建 .codecast/rules.md 模板文件")
		color.White("  请根据项目需求编辑此文件。")
	case "reload":
		color.Yellow("规则将在下次对话时自动重新加载")
	default:
		color.Yellow("未知子命令: %s", args)
		color.White("可用: /rules [show|init|reload]")
	}
}

// handleCompactCommand 处理 /compact 命令（摘要式压缩）
func handleCompactCommand(args string, ag *agent.CodecastAgent) {
	if ag == nil {
		color.Red("Agent 未初始化")
		return
	}
	color.Cyan("正在摘要压缩上下文...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := ag.SummarizeContext(ctx); err != nil {
		color.Yellow("摘要失败，降级到清空: %v", err)
		ag.ClearContext()
		color.Yellow("✓ 上下文已清空（降级）")
		return
	}
	color.Green("✓ 上下文已摘要压缩")
}

// handlePlanCommand 处理 /plan 命令
func handlePlanCommand(args string, ag *agent.CodecastAgent) {
	if args == "" {
		color.Yellow("用法: /plan <任务描述>")
		return
	}
	color.Cyan("正在规划任务...")
	cfg := config.Load()
	orchestrator, err := subagent.NewOrchestrator(cfg, nil, nil)
	if err != nil {
		color.Red("创建编排器失败: %v", err)
		color.White("回退到普通模式规划...")
		ctx := context.Background()
		planPrompt := fmt.Sprintf("请分析以下任务并制定执行计划（仅规划，不执行）：\n\n%s", args)
		if err := ag.StreamProcess(ctx, planPrompt); err != nil {
			color.Red("规划失败: %v", err)
		}
		return
	}
	ctx := context.Background()
	plan, err := orchestrator.PlanOnly(ctx, args)
	if err != nil {
		color.Red("规划失败: %v", err)
		return
	}
	tui.PrintHeader("执行计划")
	fmt.Println(ag.GetRenderer().RenderMarkdown(plan))
}

// handleDelegateCommand 处理 /delegate 命令
func handleDelegateCommand(args string, ag *agent.CodecastAgent) {
	visualize := false
	task := args
	if strings.HasPrefix(args, "-v ") {
		visualize = true
		task = strings.TrimSpace(strings.TrimPrefix(args, "-v"))
	} else if strings.HasPrefix(args, "--visualize ") {
		visualize = true
		task = strings.TrimSpace(strings.TrimPrefix(args, "--visualize"))
	}
	if task == "" {
		color.Yellow("用法: /delegate [-v] <任务描述>")
		color.White("  -v, --visualize  显示 DAG 可视化")
		return
	}
	color.Cyan("正在使用 Plan+Execute 双 Agent 协作...")
	cfg := config.Load()
	orchestrator, err := subagent.NewOrchestrator(cfg, nil, nil)
	if err != nil {
		color.Red("创建编排器失败: %v", err)
		color.White("回退到普通模式执行...")
		ctx := context.Background()
		if err := ag.StreamProcess(ctx, task); err != nil {
			color.Red("执行失败: %v", err)
		}
		return
	}
	if visualize {
		orchestrator.SetVisualization(true)
	}
	spinner := tui.NewSpinner("规划中...")
	spinner.Start()
	var renderDone chan struct{}
	if visualize {
		renderDone = make(chan struct{})
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					dv := orchestrator.GetDAGView()
					if dv != nil {
						fmt.Print("\r\033[K")
						fmt.Println(dv.Render(60))
					}
				case <-renderDone:
					return
				}
			}
		}()
	}
	ctx := context.Background()
	result, err := orchestrator.PlanAndExecute(ctx, task)
	spinner.Stop()
	if renderDone != nil {
		close(renderDone)
	}
	if visualize {
		dv := orchestrator.GetDAGView()
		if dv != nil {
			fmt.Println(dv.Render(60))
		}
	}
	if err != nil {
		color.Red("执行失败: %v", err)
		return
	}
	if result.Plan != "" {
		tui.PrintHeader("规划结果")
		fmt.Println(ag.GetRenderer().RenderMarkdown(result.Plan))
	}
	if result.Execution != "" {
		tui.PrintHeader("执行结果")
		fmt.Println(ag.GetRenderer().RenderMarkdown(result.Execution))
	}
	tui.PrintDim(result.Summary())
}

// handleHooksCommand 处理 /hooks 命令
func handleHooksCommand(args string, ag *agent.CodecastAgent) {
	hooksDir := filepath.Join(".codecast", "hooks")
	hm := hooks.NewHookManager(hooksDir)
	hookList := hm.List()
	if len(hookList) == 0 {
		color.Yellow("未配置任何钩子")
		color.White("在 .codecast/hooks/hooks.yaml 中配置钩子")
		return
	}
	color.Cyan("已配置的钩子:")
	for _, h := range hookList {
		status := "禁用"
		if h.Enabled {
			status = "启用"
		}
		color.White("  [%s] %s - %s (%s)", status, h.Name, h.Point, h.Command)
	}
}

// handleVisionCommand 处理 /vision 命令
func handleVisionCommand(args string, ag *agent.CodecastAgent) {
	if args == "" {
		color.Yellow("用法: /vision <图片路径>")
		return
	}
	imagePath := strings.TrimSpace(args)
	if !vision.IsImageFile(imagePath) {
		color.Red("不支持的图片格式，支持: jpg, png, gif, webp, bmp")
		return
	}
	color.Cyan("正在分析图片: %s", imagePath)
	ctx := context.Background()
	analysisPrompt := fmt.Sprintf("请分析以下图片文件: %s\n描述图片内容，如果图片包含代码，请分析代码逻辑。", imagePath)
	if err := ag.StreamProcess(ctx, analysisPrompt); err != nil {
		color.Red("图片分析失败: %v", err)
	}
}

// handleScreenshotCommand 处理 /screenshot 命令
func handleScreenshotCommand(args string, ag *agent.CodecastAgent) {
	color.Cyan("正在截取屏幕截图...")
	capture := vision.NewScreenshotCapture()
	path, err := capture.Capture()
	if err != nil {
		color.Red("截图失败: %v", err)
		return
	}
	color.Green("截图已保存: %s", path)
	color.White("使用 /vision %s 分析截图", path)
}

// handlePoolCommand 处理 /pool 命令
func handlePoolCommand(args string, ag *agent.CodecastAgent) {
	color.Cyan("Agent Pool 状态:")
	color.White("  Pool 功能需要先初始化，使用 codecast pool 命令管理")
}

// handlePluginsCommand 处理 /plugins 命令
func handlePluginsCommand(args string, ag *agent.CodecastAgent) {
	color.Cyan("已加载的插件:")
	color.White("  使用 codecast plugin list 查看插件列表")
}

// handleIndexCommand 处理 /index 命令
func handleIndexCommand(args string, ag *agent.CodecastAgent) {
	idx := ag.GetIndexer()
	if idx == nil {
		color.Yellow("索引器未初始化")
		return
	}
	index := idx.GetIndex()
	color.Cyan("代码库索引:")
	color.White("  文件数: %d", index.TotalFiles)
	color.White("  总大小: %s", indexer.FormatSize(index.TotalSize))
	for lang, count := range index.Languages {
		color.White("  %s: %d 文件", lang, count)
	}
}

// handleModelCommand 处理 /model 命令
func handleModelCommand(args string, ag *agent.CodecastAgent) {
	if args == "" {
		switcher := ag.GetModelSwitcher()
		if switcher != nil {
			color.Cyan("当前模型: %s (%s)", switcher.CurrentModel(), switcher.CurrentProvider())
			color.White("可用模型:")
			for _, m := range model.ListModels("") {
				active := ""
				if m.ID == switcher.CurrentModel() {
					active = " ← 当前"
				}
				color.White("  %s (%s) - $%.4f/1k%s", m.ID, m.Provider, m.CostPer1kIn, active)
			}
		}
		return
	}
	if err := ag.SwitchModel(strings.TrimSpace(args)); err != nil {
		color.Red("%v", err)
		return
	}
	switcher := ag.GetModelSwitcher()
	color.Green("✓ 模型已切换为: %s (%s)", switcher.CurrentModel(), switcher.CurrentProvider())
}

// handleStatsFromRegistry 处理 stats 回调
func handleStatsFromRegistry(stats any) {
	uiPrintStatsFromInteractive(stats)
}

func printAgentStats(stats any) {
	switch s := stats.(type) {
	default:
		color.Yellow("📊 Agent 统计: (类型不可用 %T)", stats)
		_ = s
	}
}

func uiPrintStatsFromInteractive(stats any) {
	printAgentStats(stats)
}

func printResumeHint() {
	color.Cyan("💡 提示: 启动时使用 --resume <id> 或 --continue 恢复会话")
}
