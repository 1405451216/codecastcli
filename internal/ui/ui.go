package ui

import (
	"fmt"

	"github.com/fatih/color"
)

func PrintBanner() {
	banner := `   ______          __    ______          __           __
  / ____/___  ____/ /__  / ____/__  ____/ /  _______  / /_
 / /   / __ \/ __  / _ \/ /   / _ \/ __  /  / __ \/ _ \/ __/
/ /___/ /_/ / /_/ /  __/ /___/  __/ /_/ /  / /_/ /  __/ /_
\____/\____/\__,_/\___/\____/\___/\__,_/  / .___/\___/\__/
                                         /_/`
	color.HiCyan(banner)
	color.White("  AI Agent CLI - 基于 AgentPrimordia 框架")
}

func PrintHelp() {
	color.Yellow("📖 常用命令:")
	fmt.Println("  /help, /h       显示帮助")
	fmt.Println("  /quit, /q       退出程序")
	fmt.Println("  /clear          清除对话上下文")
	fmt.Println("  /tools          查看可用工具")
	fmt.Println("  /models         查看支持的模型")
	fmt.Println("  /stats          查看 Agent 统计")
	fmt.Println("  /sessions       查看会话列表")
	fmt.Println("  /export         导出当前会话为 Markdown")
	fmt.Println("  /export <文件>  导出到指定文件")
	fmt.Println()
	color.Yellow("💡 使用示例:")
	fmt.Println("  • 直接输入问题与 AI 对话")
	fmt.Println("  • 让 AI 帮你写代码、读文件、执行命令")
	fmt.Println("  • 使用 `codecast chat <消息>` 进行单轮对话")
	fmt.Println("  • 使用 `/config` 管理配置")
	fmt.Println("  • 使用 `codecast session` 管理会话历史")
	fmt.Println("  • 使用 `codecast cost` 查看成本统计")
}

func PrintTools() {
	color.Yellow("🔧 可用工具:")
	fmt.Println("  • FileSystem   - 文件读写、目录列表、文件搜索")
	fmt.Println("  • Shell        - 执行系统命令（带安全限制）")
	fmt.Println("  • Web          - HTTP 请求（GET/POST 等）")
	fmt.Println("  • Knowledge    - 知识库搜索（如配置了 RAG）")
}

func PrintModels() {
	color.Yellow("🤖 支持的模型 Provider:")
	fmt.Println("  • openai     - OpenAI GPT-4o / GPT-4o-mini")
	fmt.Println("  • anthropic  - Claude 3.5 Sonnet / Opus")
	fmt.Println("  • gemini     - Google Gemini Pro")
	fmt.Println("  • deepseek   - DeepSeek Chat / Coder")
	fmt.Println("  • qwen       - 通义千问")
	fmt.Println("  • glm        - 智谱 GLM")
	fmt.Println("  • ollama     - 本地 Ollama 模型")
	fmt.Println("  • ...以及更多")
}

func PrintAssistant(content string) {
	fmt.Println()
	PrintMarkdown(content)
	fmt.Println()
}

func PrintUser(content string) {
	color.Cyan("❯ %s", content)
}

func PrintError(err string) {
	color.Red("✗ %s", err)
}

func PrintSuccess(msg string) {
	color.Green("✓ %s", msg)
}
