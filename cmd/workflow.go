package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/config"
	"codecast/cli/internal/provider"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// rootWorkflowCmd 是 `codecast workflow` 根命令。
//
// 在 v0.2.0 中，workflow 子命令已迁移到交互模式的 `/workflow` 斜杠命令。
var rootWorkflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "多 Agent 工作流编排（已迁移: 在交互模式中使用 /workflow）",
	Long: "⚠️  codecast workflow 子命令已在 v0.2.0 中迁移到交互模式。\n" +
		"\n" +
		"请在交互模式（运行 codecast 后）中直接使用 /workflow 斜杠命令：\n" +
		"\n" +
		"  /workflow pipeline <prompt>    - Pipeline 顺序执行\n" +
		"  /workflow parallel <prompt>    - Parallel 并行执行\n" +
		"  /workflow handoff <prompt>     - Handoff 动态交接",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("⚠️  `codecast workflow` 子命令已在 v0.2.0 中迁移到交互模式。")
		fmt.Println()
		fmt.Println("请在交互模式中使用 /workflow 斜杠命令：")
		fmt.Println("  /workflow pipeline <prompt>  - Pipeline 顺序执行")
		fmt.Println("  /workflow parallel <prompt>  - Parallel 并行执行")
		fmt.Println("  /workflow handoff <prompt>   - Handoff 动态交接")
		fmt.Println()
		fmt.Println("直接运行 `codecast` 进入交互模式，然后输入 /workflow 即可。")
	},
}

// ============== 可复用函数 ==============

// workflowRunPipeline Pipeline 顺序执行
func workflowRunPipeline(prompt, stepsStr string) error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}
	stepNames := strings.Split(stepsStr, ",")

	color.Yellow("🔄 Pipeline 工作流")
	color.White("输入: %s", prompt)
	fmt.Println()

	steps := make([]ap.PipelineStep, len(stepNames))
	for i, name := range stepNames {
		ag, err := createWorkflowAgent(cfg, name, getSystemPromptForRole(name))
		if err != nil {
			return fmt.Errorf("创建 Agent 失败: %w", err)
		}
		steps[i] = ap.PipelineStep{
			Name:  strings.TrimSpace(name),
			Agent: ag,
		}
	}

	pipeline := ap.NewPipeline(steps...)
	ctx := context.Background()

	result, err := pipeline.Run(ctx, prompt)
	if err != nil {
		return fmt.Errorf("Pipeline 执行失败: %w", err)
	}

	color.Green("\n✓ 执行完成")
	for i, step := range result.Steps {
		status := "✅"
		if step.Error != nil {
			status = "❌"
		}
		fmt.Printf("  [%d] %s %s (%v)\n", i+1, status, step.Name, step.Duration)
	}
	fmt.Printf("\n📝 最终输出:\n%s\n", result.Final)
	return nil
}

// workflowRunParallel Parallel 并行执行
func workflowRunParallel(prompt, agentsStr string) error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}
	agentNames := strings.Split(agentsStr, ",")

	color.Yellow("🚀 Parallel 并行执行")
	color.White("输入: %s", prompt)
	fmt.Println()

	agents := make([]ap.Agent, len(agentNames))
	for i, name := range agentNames {
		ag, err := createWorkflowAgent(cfg, name, getSystemPromptForRole(name))
		if err != nil {
			return fmt.Errorf("创建 Agent 失败: %w", err)
		}
		agents[i] = ag
	}

	ctx := context.Background()
	start := time.Now()
	result, err := ap.ParallelRun(ctx, agents, prompt)
	if err != nil {
		return fmt.Errorf("Parallel 执行失败: %w", err)
	}

	color.Green("\n✓ 执行完成 (总耗时: %v)", time.Since(start))
	for i, r := range result.Results {
		status := "✅"
		if r.Error != nil {
			status = "❌"
		}
		name := agentNames[i]
		if i < len(agentNames) {
			name = strings.TrimSpace(agentNames[i])
		}
		fmt.Printf("\n  [%d] %s %s (%v)\n", i+1, status, name, r.Duration)
		if r.Error != nil {
			fmt.Printf("  错误: %v\n", r.Error)
		} else {
			content := r.Output
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			fmt.Printf("  输出: %s\n", content)
		}
	}
	return nil
}

// workflowRunHandoff Handoff 动态交接
func workflowRunHandoff(prompt, agentsStr string) error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}
	agentNames := strings.Split(agentsStr, ",")

	color.Yellow("🎯 Handoff 动态交接")
	color.White("输入: %s", prompt)
	fmt.Println()

	agents := make([]ap.Agent, len(agentNames))
	for i, name := range agentNames {
		ag, err := createWorkflowAgent(cfg, name, getSystemPromptForRole(name))
		if err != nil {
			return fmt.Errorf("创建 Agent 失败: %w", err)
		}
		agents[i] = ag
	}

	handoff := ap.NewHandoff(ap.HandoffConfig{
		Agents: agents,
		Router: func(ctx context.Context, input string) int {
			inputLower := strings.ToLower(input)
			for i, name := range agentNames {
				if strings.Contains(inputLower, strings.ToLower(name)) {
					return i
				}
			}
			return 0
		},
		MaxHandoffs: 5,
	})

	ctx := context.Background()
	start := time.Now()
	result, err := handoff.Run(ctx, prompt)
	if err != nil {
		return fmt.Errorf("Handoff 执行失败: %w", err)
	}

	color.Green("\n✓ 执行完成 (耗时: %v)", time.Since(start))
	fmt.Printf("🎯 最终处理 Agent: %s\n", result.AgentName)
	fmt.Printf("🔄 交接次数: %d\n", result.Handoffs)
	fmt.Printf("\n📝 输出:\n%s\n", result.Output)
	return nil
}

// createWorkflowAgent 创建一个 Agent
func createWorkflowAgent(cfg *config.Config, name string, systemPrompt string) (ap.Agent, error) {
	llmProvider, err := provider.CreateProvider(cfg)
	if err != nil {
		return nil, err
	}

	agent, err := ap.NewAgent(name, systemPrompt, llmProvider,
		ap.WithMaxTurns(10),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 agent 失败: %w", err)
	}
	return agent, nil
}

// getSystemPromptForRole 根据角色返回系统提示词
func getSystemPromptForRole(role string) string {
	rolePrompts := map[string]string{
		"需求分析": "你是需求分析师，负责收集和分析用户需求，输出清晰的需求文档。",
		"代码开发": "你是资深开发工程师，负责编写高质量、可维护的代码。",
		"测试验证": "你是 QA 工程师，负责编写测试用例并验证功能正确性。",
		"安全审查": "你是安全专家，负责审查代码中的安全漏洞和风险。",
		"性能优化": "你是性能优化专家，负责分析和优化代码性能。",
		"代码风格": "你是代码审查员，负责检查代码风格和最佳实践。",
		"文档编写": "你是技术文档工程师，负责编写清晰的技术文档。",
	}

	if prompt, ok := rolePrompts[role]; ok {
		return prompt
	}
	return fmt.Sprintf("你是 %s 专家，请根据输入提供专业的分析和建议。", role)
}

func init() {
	// 仅保留 rootWorkflowCmd，不再注册子命令
	rootCmd.AddCommand(rootWorkflowCmd)
}
