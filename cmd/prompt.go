package cmd

// prompt.go: /prompt 斜杠命令 — 运行时管理 promptab 变体。
//
// 子命令：
//
//	/prompt                       显示当前 variant + 简要帮助
//	/prompt list                  列出所有可用变体
//	/prompt use <name>            切换到指定变体（写入配置）
//	/prompt show <name>           渲染并展示指定变体的完整 prompt
//	/prompt reload                重新扫描 ~/.codecast/prompts/ 与 .codecast/prompts/
//	/prompt current               展示当前选中的变体
//
// 设计动机：让用户在不退出 REPL 的情况下实验不同变体效果。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codecast/cli/internal/agent"
	"codecast/cli/internal/config"
	"codecast/cli/internal/promptab"

	"github.com/fatih/color"
)

// handlePromptCommand 处理 /prompt 斜杠命令
func handlePromptCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	if args == "" {
		printPromptHelp()
		return
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		printPromptHelp()
		return
	}
	sub := fields[0]
	rest := ""
	if len(fields) > 1 {
		rest = strings.TrimSpace(strings.TrimPrefix(args, sub))
	}

	switch sub {
	case "list", "ls":
		promptList()
	case "use", "set":
		if rest == "" {
			color.Yellow("用法: /prompt use <variant>")
			return
		}
		promptUse(rest)
	case "show", "preview":
		if rest == "" {
			color.Yellow("用法: /prompt show <variant>")
			return
		}
		promptShow(rest)
	case "current":
		promptCurrent()
	case "reload":
		promptReload()
	case "help", "-h", "--help":
		printPromptHelp()
	default:
		color.Yellow("未知子命令: %s", sub)
		printPromptHelp()
	}
}

func printPromptHelp() {
	color.Cyan("📜 /prompt — 提示词变体管理")
	fmt.Println()
	color.White("用法:")
	color.White("  /prompt                显示帮助")
	color.White("  /prompt list           列出所有可用变体")
	color.White("  /prompt use <name>     切换到指定变体（持久化到配置）")
	color.White("  /prompt show <name>    渲染并预览指定变体的完整 prompt")
	color.White("  /prompt current        显示当前选中的变体")
	color.White("  /prompt reload         重新扫描 prompts 目录")
	fmt.Println()
	color.White("示例:")
	color.White("  /prompt use concise")
	color.White("  /prompt show safety-first")
	fmt.Println()
}

// promptList 列出所有可用变体
func promptList() {
	res := agent.DefaultResolver()
	vs := res.Registry().All()
	if len(vs) == 0 {
		color.Yellow("无可用变体")
		return
	}
	cfg := config.Load()
	active := cfg.PromptVariant
	if active == "" {
		active = "default"
	}
	color.Yellow("可用变体（共 %d 个）：", len(vs))
	for _, v := range vs {
		marker := "  "
		if v.Name == active {
			marker = color.GreenString("★ ")
		}
		fmt.Printf("%s%-20s  %s\n", marker, v.Name, v.Description)
		if v.Author != "" {
			fmt.Printf("    作者: %s\n", v.Author)
		}
	}
}

// promptUse 切换到指定变体（持久化到 ~/.codecast/config.yaml）
func promptUse(name string) {
	res := agent.DefaultResolver()
	if _, err := res.Registry().Resolve(name); err != nil {
		color.Red("✗ 变体不存在: %s", name)
		color.Yellow("可用变体: %s", strings.Join(res.Registry().Names(), ", "))
		return
	}
	cfg := config.Load()
	cfg.PromptVariant = name
	if err := config.Save(cfg); err != nil {
		color.Red("✗ 保存失败: %v", err)
		return
	}
	// 同步更新 resolver selector（影响后续提示词渲染，无需重启）
	res.SetSelector(agent.SelectorConfig{
		Variant:  name,
		Strategy: cfg.PromptStrategy,
		Weights:  cfg.PromptWeights,
	}.ToSelector())
	color.Green("✓ 已切换到变体: %s", name)
	color.HiBlack("（后续消息将使用新提示词）")
}

// promptShow 渲染并展示指定变体的完整 prompt
func promptShow(name string) {
	res := agent.DefaultResolver()
	v, err := res.Registry().Resolve(name)
	if err != nil {
		color.Red("✗ 变体不存在: %s", name)
		return
	}
	// 构造与生产路径一致的 RenderInputs
	cfg := config.Load()
	in := promptab.RenderInputs{
		OS:           "linux", // 演示用，生产取自 runtime.GOOS
		CWD:          ".",
		Mode:         cfg.PermissionMode,
		Budget:       fmt.Sprintf("%.2f", cfg.SessionBudgetUSD),
		ModeAdvice:   agentModeAdviceForShow(cfg.PermissionMode),
		ProjectRules: "(示例项目规则：从 .codecast/rules.md 加载)",
		FileTree:     "(示例代码库结构：src/main.go, internal/, README.md, ...)",
	}
	rendered := v.Render(in)
	color.Cyan("━━━ 变体: %s ━━━", name)
	color.HiBlack("作者: %s | 描述: %s", v.Author, v.Description)
	fmt.Println()
	fmt.Println(rendered)
	fmt.Println()
	color.HiBlack("━━━ 渲染后字节数: %d ━━━", len(rendered))
}

// promptCurrent 显示当前选中的变体
func promptCurrent() {
	cfg := config.Load()
	active := cfg.PromptVariant
	if active == "" {
		active = "default"
	}
	res := agent.DefaultResolver()
	sel := res.Selector()
	v, err := res.Registry().Resolve(active)
	if err != nil {
		color.Red("✗ 当前变体 %q 不可用: %v", active, err)
		return
	}
	color.Cyan("当前变体: %s", active)
	color.White("  描述: %s", v.Description)
	color.White("  作者: %s", v.Author)
	color.White("  选择策略: %s", strategyName(sel.Strategy))
	if len(sel.Weights) > 0 {
		color.White("  权重: %v", sel.Weights)
	}
}

// promptReload 重新扫描 prompts 目录
func promptReload() {
	res := agent.DefaultResolver()
	// 用户级
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, ".codecast", "prompts")
		if err := res.LoadProjectDir(dir); err != nil {
			color.Yellow("⚠ 重新加载 %s 失败: %v", dir, err)
		} else {
			color.Green("✓ 已重新加载: %s", dir)
		}
	}
	// 项目级（cwd 解析）
	cwd, _ := os.Getwd()
	dir := filepath.Join(cwd, ".codecast", "prompts")
	if err := res.LoadProjectDir(dir); err != nil {
		color.Yellow("⚠ 重新加载 %s 失败: %v", dir, err)
	} else {
		color.Green("✓ 已重新加载: %s", dir)
	}
}

// agentModeAdviceForShow 给出与 PromptResolver 内部一致的权限模式建议。
// 重复实现以避免对 internal/agent 内部函数的依赖。
func agentModeAdviceForShow(mode string) string {
	switch mode {
	case "full-auto":
		return "所有工具调用无需确认，可直接执行写操作。但 scope 之外的文件仍然禁止。"
	case "auto-edit":
		return "文件编辑类操作自动通过；shell 写操作（rm/git push/curl POST）仍需用户确认。"
	default:
		return "所有写操作前必须通过 permission 提示获得用户确认。在 prompt 中清晰说明即将做什么。"
	}
}

// strategyName 策略枚举→人类可读字符串
func strategyName(s promptab.SelectStrategy) string {
	switch s {
	case promptab.SelectFixed:
		return "fixed"
	case promptab.SelectRoundRobin:
		return "round-robin"
	case promptab.SelectWeightedRandom:
		return "weighted"
	default:
		return "unknown"
	}
}
