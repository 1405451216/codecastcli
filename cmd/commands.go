package cmd

// commands.go: 内置斜杠命令的注册（统一注册表）。
//
// 之前 30+ 个命令的注册分散在 interactive.go 的 init() /
// commandSuggestions / handleSpecialCommand 三处，存在 /sandbox
// 重复等问题。现在所有内置命令通过 RegisterBuiltinCommands(r)
// 集中注册到 registry，补全、调度都从同一处读取。

import (
	"codecast/cli/internal/agent"
	"codecast/cli/internal/ui"
)

// RegisterBuiltinCommands 注册所有内置斜杠命令到 r
func RegisterBuiltinCommands(r *CommandRegistry) {
	if r == nil {
		return
	}

	// 如果共享 schema 可加载，验证一致性（仅在调试时输出）
	if schema, err := LoadSharedCommandSchema("cmd/shared_commands.json"); err == nil {
		// 验证一致性：注册完成后调用，避免循环
		defer VerifyRegistryConsistency(r, schema)
	}

	// === 系统命令（help/quit/clear/compact） ===
	r.Register(&CommandEntry{
		Name: "help", Aliases: []string{"h"},
		Description: "显示帮助",
		Handler: func(string, *agent.CodecastAgent) bool {
			ui.PrintHelp()
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "quit", Aliases: []string{"q", "exit"},
		Description: "退出",
		Handler: func(string, *agent.CodecastAgent) bool {
			// 退出由 REPL 主循环处理（保留兼容性）
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "clear",
		Description: "清除上下文",
		Handler: func(_ string, ag *agent.CodecastAgent) bool {
			if ag != nil {
				ag.ClearContext()
			}
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "compact",
		Description: "压缩上下文",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleCompactCommand(args, ag)
			return true
		},
	})

	// === 信息查询类 ===
	r.Register(&CommandEntry{
		Name: "tools",
		Description: "查看工具",
		Handler: func(string, *agent.CodecastAgent) bool {
			ui.PrintTools()
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "models",
		Description: "查看模型",
		Handler: func(string, *agent.CodecastAgent) bool {
			ui.PrintModels()
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "rules",
		Description: "查看/重载项目规则",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleRulesCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "stats",
		Description: "查看统计",
		Handler: func(_ string, ag *agent.CodecastAgent) bool {
			if ag == nil {
				return true
			}
			stats := ag.GetStats()
			uiPrintStats(stats)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "hooks",
		Description: "查看钩子配置",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleHooksCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "index",
		Description: "查看代码库索引",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleIndexCommand(args, ag)
			return true
		},
	})

	// === 会话管理类 ===
	r.Register(&CommandEntry{
		Name: "sessions",
		Description: "查看会话列表",
		Handler: func(string, *agent.CodecastAgent) bool {
			listSessions()
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "export",
		Description: "导出会话 (使用 /export [filename])",
		Handler: func(args string, _ *agent.CodecastAgent) bool {
			if args == "" {
				exportCurrentSession()
			} else {
				exportCurrentSessionTo(args)
			}
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "resume",
		Description: "恢复会话",
		Handler: func(string, *agent.CodecastAgent) bool {
			// resume 在 init 阶段已经处理；这里仅做提示
			uiPrintResumeHint()
			return true
		},
	})

	// === 任务执行类 ===
	r.Register(&CommandEntry{
		Name: "plan",
		Description: "规划任务 (Plan-Agent)",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handlePlanCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "delegate",
		Description: "规划并执行任务 (双Agent协作)",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleDelegateCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "vision",
		Description: "分析图片文件 (使用 /vision <path>)",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleVisionCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "screenshot",
		Description: "截图并分析",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleScreenshotCommand(args, ag)
			return true
		},
	})

	// === 资源管理类 ===
	r.Register(&CommandEntry{
		Name: "pool",
		Description: "Agent Pool 管理",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handlePoolCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "plugins",
		Description: "列出已加载插件",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handlePluginsCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "model",
		Description: "切换模型",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleModelCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "sandbox",
		Description: "沙箱管理 (status/build)",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleSandboxCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "mcp",
		Description: "MCP 服务器管理",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleMCPInteractiveCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "mode",
		Description: "切换权限模式",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleModeCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "undo",
		Description: "撤销最近文件修改",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleUndoCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "budget",
		Description: "查看预算使用情况",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleBudgetCommand(args, ag)
			return true
		},
	})

	// === v0.2.0 新增命令（已迁移到 /<cmd>） ===
	r.Register(&CommandEntry{
		Name: "config",
		Description: "配置管理 (set/get/list/wizard)",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleConfigCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "cost",
		Description: "成本管理 (summary/daily/list/clear)",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleCostCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "session",
		Description: "会话管理 (list/show/delete/export)",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleSessionCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "plugin",
		Description: "插件管理 (list/install/unload)",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handlePluginCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "rag",
		Description: "知识库 (index/query/chat)",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleRagCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "workflow",
		Description: "工作流 (pipeline/parallel/handoff)",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleWorkflowCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name: "prompt",
		Aliases: []string{"p"},
		Description: "提示词变体管理 (list/use/show/current/reload)",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handlePromptCommand(args, ag)
			return true
		},
	})
	r.Register(&CommandEntry{
		Name:        "ab",
		Description: "A/B 自动收敛管理 (enable/disable/reset/suggest/apply/epsilon)",
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			handleAbCommand(args, ag)
			return true
		},
	})
}

// ============== 辅助函数 ==============

// uiPrintStats 由 commands.go 调用，避免在 commands.go 直接依赖 stats 类型
func uiPrintStats(stats any) {
	// 委托给 interactive.go 的实际实现
	printAgentStats(stats)
}

// uiPrintResumeHint 显示 resume 提示
func uiPrintResumeHint() {
	// 委托给 interactive.go 的实际实现
	printResumeHint()
}
