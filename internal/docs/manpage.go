package docs

import (
	"fmt"
	"strings"
	"time"
)

// GenerateManPage returns a complete man page in groff format for codecast(1).
func GenerateManPage() string {
	var b strings.Builder

	now := time.Now()
	dateStr := now.Format("January 2, 2006")

	// Header
	b.WriteString(`.TH CODECAST 1 "` + dateStr + `" "codecast" "User Commands"` + "\n")

	// NAME
	b.WriteString(`.SH NAME
codecast \- AI-powered terminal agent
`)

	// SYNOPSIS
	b.WriteString(`.SH SYNOPSIS
.B codecast
.RI [ flags ] " [prompt]"
.br
.B codecast exec
.RI [ flags ] " <prompt>"
.br
.B codecast init
.br
.B codecast config
.I <subcommand>
.br
.PP
\fBNote:\fR \fIcodecast config\fR subcommands have been migrated to the \fB/config\fR slash command in interactive mode (since v0.2.0).
.br
.B codecast plugin
.I <subcommand>
.br
.B codecast pool
.I <subcommand>
.br
.B codecast sandbox
.I <subcommand>
.br
.B codecast mcp
.I <subcommand>
.br
.B codecast cost
.I <subcommand>
`)

	// DESCRIPTION
	b.WriteString(`.SH DESCRIPTION
Codecast CLI is an AI-driven terminal agent tool built on the AgentPrimordia framework.
It supports intelligent conversation, code generation, file operations, command execution,
and more.
.PP
It supports 13+ LLM providers: OpenAI, Anthropic, Gemini, Ollama, DeepSeek, Qwen, GLM, and others.
When invoked without a subcommand, codecast starts an interactive REPL session.
`)

	// OPTIONS
	b.WriteString(`.SH OPTIONS
`)

	options := []struct {
		name, desc string
	}{
		{"--model, -m", "AI model to use (default: gpt-4o)"},
		{"--provider, -p", "LLM provider: openai, anthropic, gemini, ollama, deepseek, qwen, glm (default: openai)"},
		{"--api-key, -k", "API key for the selected provider"},
		{"--base-url, -u", "Custom API base URL (optional)"},
		{"--permission-mode", "Permission approval mode: suggest, auto-edit, full-auto (default: suggest)"},
		{"--scope", "File access scope (can be specified multiple times; default: current directory)"},
		{"--safe", "Safe mode: disable Shell and Web tools"},
		{"--auto-compact", "Automatically compact context when approaching the limit"},
		{"--auto-compact-ratio", "Ratio at which auto-compact triggers, 0.0-1.0 (default: 0.8)"},
		{"--auto-checkpoint", "Automatically create a Git checkpoint before file modifications"},
		{"--auto-stash", "Use git stash instead of git commit for checkpoints (default: true)"},
		{"--daily-budget", "Daily budget limit in USD (0 = unlimited)"},
		{"--session-budget", "Per-session budget limit in USD (0 = unlimited)"},
		{"--continue, -c", "Continue the most recent session"},
		{"--resume, -r", "Resume a specific session by ID"},
		{"--log-level", "Log level: debug, info, warn, error (default: info)"},
		{"--log-format", "Log format: text, json (default: text)"},
	}

	for _, opt := range options {
		b.WriteString(fmt.Sprintf(".TP\n.B %s\n%s\n", opt.name, opt.desc))
	}

	// SUBCOMMANDS
	b.WriteString(`.SH SUBCOMMANDS
`)

	subcommands := []struct {
		name, desc string
	}{
		{"exec [flags] <prompt>", "Run a single prompt in headless (non-interactive) mode. Supports --format text|json|stream-json, --model, and --timeout flags."},
		{"init", "Initialize project configuration. Creates .codecast/ directory with rules.md template."},
		{"plugin <subcommand>", "Manage community plugins. Subcommands: list, install, unload, available."},
		{"pool <subcommand>", "Manage distributed Agent Pool. Subcommands: status, run."},
		{"sandbox <subcommand>", "Manage sandbox isolation. Subcommands: status, build."},
		{"mcp <subcommand>", "Manage MCP (Model Context Protocol) servers. Subcommands: list, add, remove, test, templates, categories, connect, disconnect."},
		{"cost <subcommand>", "Cost tracking and management. Subcommands: summary, daily, list, clear."},
		{"config", "Deprecated since v0.2.0 — use the /config slash command inside the interactive REPL. Running this command now prints a migration notice."},
	}

	for _, sc := range subcommands {
		b.WriteString(fmt.Sprintf(".TP\n.B %s\n%s\n", sc.name, sc.desc))
	}

	// INTERACTIVE COMMANDS
	b.WriteString(`.SH INTERACTIVE COMMANDS
The following slash commands are available inside the interactive REPL:
`)

	commands := []struct {
		name, desc string
	}{
		{"/help", "Display help information"},
		{"/quit", "Exit the interactive session"},
		{"/clear", "Clear the conversation context"},
		{"/compact", "Compact the conversation context"},
		{"/rules [show|reload]", "View or reload project rules"},
		{"/tools", "List available tools"},
		{"/models", "List available models"},
		{"/stats", "Show agent statistics"},
		{"/sessions", "List saved sessions"},
		{"/export [filename]", "Export the current session to a Markdown file"},
		{"/resume", "Resume a previous session"},
		{"/plan <task>", "Plan a task using Plan-Agent"},
		{"/delegate <task>", "Plan and execute a task using dual-agent collaboration"},
		{"/hooks", "View hook configuration"},
		{"/vision <image>", "Analyze an image file"},
		{"/screenshot", "Capture and analyze a screenshot"},
		{"/pool", "View Agent Pool status"},
		{"/plugins", "List loaded plugins"},
		{"/index", "View codebase index information"},
		{"/model [name]", "Switch or view the current model"},
		{"/sandbox", "Sandbox management"},
		{"/mcp", "MCP server management"},
		{"/mode [mode]", "Switch permission mode (suggest, auto-edit, full-auto)"},
		{"/undo [path]", "Undo the most recent file modification"},
		{"/budget", "View budget usage"},
		{"/config [list|get|set|wizard|providers|init]", "Manage configuration (replaces `codecast config` subcommands)"},
	}

	for _, cmd := range commands {
		b.WriteString(fmt.Sprintf(".TP\n.B %s\n%s\n", cmd.name, cmd.desc))
	}

	// ENVIRONMENT
	b.WriteString(`.SH ENVIRONMENT
`)

	envVars := []struct {
		name, desc string
	}{
		{"CODECAST_API_KEY", "Default API key for the LLM provider."},
		{"CODECAST_PROVIDER", "Default LLM provider (e.g., openai, anthropic)."},
		{"CODECAST_MODEL", "Default AI model to use."},
		{"CODECAST_BASE_URL", "Default API base URL."},
	}

	for _, ev := range envVars {
		b.WriteString(fmt.Sprintf(".TP\n.B %s\n%s\n", ev.name, ev.desc))
	}

	// FILES
	b.WriteString(`.SH FILES
`)

	files := []struct {
		path, desc string
	}{
		{"~/.codecast/config.yaml", "Global configuration file."},
		{".codecast/rules.md", "Project-level rules (committed to version control)."},
		{".codecast/auto_rules.md", "Auto-generated project rules."},
	}

	for _, f := range files {
		b.WriteString(fmt.Sprintf(".TP\n.B %s\n%s\n", f.path, f.desc))
	}

	// EXAMPLES
	b.WriteString(`.SH EXAMPLES
Start an interactive session:
.RS
.B codecast
.RE
.PP
Run a single prompt in headless mode:
.RS
.B codecast exec "explain the main.go file"
.RE
.PP
Use a specific model and provider:
.RS
.B codecast --provider anthropic --model claude-3-opus-20240229
.RE
.PP
Continue the most recent session:
.RS
.B codecast --continue
.RE
.PP
Resume a specific session:
.RS
.B codecast --resume abc123
.RE
.PP
Run in safe mode with a daily budget:
.RS
.B codecast --safe --daily-budget 5.0
.RE
.PP
Stream JSON output for CI/CD integration:
.RS
.B codecast exec --format stream-json "fix the bug in auth.go"
.RE
.PP
Initialize project configuration:
.RS
.B codecast init
.RE
.PP
Set the API key (in interactive mode):
.RS
.B /config set api_key sk-xxxx
.RE
.PP
View cost summary:
.RS
.B codecast cost summary
.RE
`)

	// SEE ALSO
	b.WriteString(`.SH "SEE ALSO"
.BR git (1),
.BR docker (1),
.BR curl (1)
`)

	return b.String()
}

// GenerateMarkdownHelp returns the man page content in Markdown format for --help.
func GenerateMarkdownHelp() string {
	var b strings.Builder

	b.WriteString("# codecast(1)\n\n")

	// NAME
	b.WriteString("## NAME\n\n")
	b.WriteString("codecast - AI-powered terminal agent\n\n")

	// SYNOPSIS
	b.WriteString("## SYNOPSIS\n\n")
	b.WriteString("`codecast [flags] [prompt]`\n\n")
	b.WriteString("`codecast exec [flags] <prompt>`\n\n")
	b.WriteString("`codecast init`\n\n")
	b.WriteString("`codecast plugin <subcommand>` *(deprecated, use `/plugin` in interactive mode)*\n\n")
	b.WriteString("`codecast pool <subcommand>` *(deprecated, use `/pool` in interactive mode)*\n\n")
	b.WriteString("`codecast sandbox <subcommand>` *(deprecated, use `/sandbox` in interactive mode)*\n\n")
	b.WriteString("`codecast mcp <subcommand>` *(deprecated, use `/mcp` in interactive mode)*\n\n")
	b.WriteString("`codecast cost <subcommand>` *(deprecated, use `/cost` in interactive mode)*\n\n")

	// DESCRIPTION
	b.WriteString("## DESCRIPTION\n\n")
	b.WriteString("Codecast CLI is an AI-driven terminal agent tool built on the AgentPrimordia framework. ")
	b.WriteString("It supports intelligent conversation, code generation, file operations, command execution, and more.\n\n")
	b.WriteString("It supports 13+ LLM providers: OpenAI, Anthropic, Gemini, Ollama, DeepSeek, Qwen, GLM, and others. ")
	b.WriteString("When invoked without a subcommand, codecast starts an interactive REPL session.\n\n")

	// OPTIONS
	b.WriteString("## OPTIONS\n\n")

	options := []struct {
		name, desc string
	}{
		{"--model, -m", "AI model to use (default: gpt-4o)"},
		{"--provider, -p", "LLM provider: openai, anthropic, gemini, ollama, deepseek, qwen, glm (default: openai)"},
		{"--api-key, -k", "API key for the selected provider"},
		{"--base-url, -u", "Custom API base URL (optional)"},
		{"--permission-mode", "Permission approval mode: suggest, auto-edit, full-auto (default: suggest)"},
		{"--scope", "File access scope (can be specified multiple times; default: current directory)"},
		{"--safe", "Safe mode: disable Shell and Web tools"},
		{"--auto-compact", "Automatically compact context when approaching the limit"},
		{"--auto-compact-ratio", "Ratio at which auto-compact triggers, 0.0-1.0 (default: 0.8)"},
		{"--auto-checkpoint", "Automatically create a Git checkpoint before file modifications"},
		{"--auto-stash", "Use git stash instead of git commit for checkpoints (default: true)"},
		{"--daily-budget", "Daily budget limit in USD (0 = unlimited)"},
		{"--session-budget", "Per-session budget limit in USD (0 = unlimited)"},
		{"--continue, -c", "Continue the most recent session"},
		{"--resume, -r", "Resume a specific session by ID"},
		{"--log-level", "Log level: debug, info, warn, error (default: info)"},
		{"--log-format", "Log format: text, json (default: text)"},
	}

	for _, opt := range options {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", opt.name, opt.desc))
	}
	b.WriteString("\n")

	// SUBCOMMANDS
	b.WriteString("## SUBCOMMANDS\n\n")

	subcommands := []struct {
		name, desc string
	}{
		{"exec [flags] <prompt>", "Run a single prompt in headless (non-interactive) mode. Supports --format text|json|stream-json, --model, and --timeout flags."},
		{"init", "Initialize project configuration. Creates .codecast/ directory with rules.md template."},
		{"plugin <subcommand>", "Manage community plugins. Subcommands: list, install, unload, available."},
		{"pool <subcommand>", "Manage distributed Agent Pool. Subcommands: status, run."},
		{"sandbox <subcommand>", "Manage sandbox isolation. Subcommands: status, build."},
		{"mcp <subcommand>", "Manage MCP (Model Context Protocol) servers. Subcommands: list, add, remove, test, templates, categories, connect, disconnect."},
		{"cost <subcommand>", "Cost tracking and management. Subcommands: summary, daily, list, clear."},
	}

	for _, sc := range subcommands {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", sc.name, sc.desc))
	}
	b.WriteString("\n")

	// INTERACTIVE COMMANDS
	b.WriteString("## INTERACTIVE COMMANDS\n\n")
	b.WriteString("The following slash commands are available inside the interactive REPL:\n\n")

	commands := []struct {
		name, desc string
	}{
		{"/help", "Display help information"},
		{"/quit", "Exit the interactive session"},
		{"/clear", "Clear the conversation context"},
		{"/compact", "Compact the conversation context"},
		{"/rules [show|reload]", "View or reload project rules"},
		{"/tools", "List available tools"},
		{"/models", "List available models"},
		{"/stats", "Show agent statistics"},
		{"/sessions", "List saved sessions"},
		{"/export [filename]", "Export the current session to a Markdown file"},
		{"/resume", "Resume a previous session"},
		{"/plan <task>", "Plan a task using Plan-Agent"},
		{"/delegate <task>", "Plan and execute a task using dual-agent collaboration"},
		{"/hooks", "View hook configuration"},
		{"/vision <image>", "Analyze an image file"},
		{"/screenshot", "Capture and analyze a screenshot"},
		{"/pool", "View Agent Pool status"},
		{"/plugins", "List loaded plugins"},
		{"/index", "View codebase index information"},
		{"/model [name]", "Switch or view the current model"},
		{"/sandbox", "Sandbox management"},
		{"/mcp", "MCP server management"},
		{"/mode [mode]", "Switch permission mode (suggest, auto-edit, full-auto)"},
		{"/undo [path]", "Undo the most recent file modification"},
		{"/budget", "View budget usage"},
		{"/config [list|get|set|wizard|providers|init]", "Manage configuration (replaces `codecast config` subcommands)"},
	}

	for _, cmd := range commands {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", cmd.name, cmd.desc))
	}
	b.WriteString("\n")

	// ENVIRONMENT
	b.WriteString("## ENVIRONMENT\n\n")

	envVars := []struct {
		name, desc string
	}{
		{"CODECAST_API_KEY", "Default API key for the LLM provider."},
		{"CODECAST_PROVIDER", "Default LLM provider (e.g., openai, anthropic)."},
		{"CODECAST_MODEL", "Default AI model to use."},
		{"CODECAST_BASE_URL", "Default API base URL."},
	}

	for _, ev := range envVars {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", ev.name, ev.desc))
	}
	b.WriteString("\n")

	// FILES
	b.WriteString("## FILES\n\n")

	files := []struct {
		path, desc string
	}{
		{"~/.codecast/config.yaml", "Global configuration file."},
		{".codecast/rules.md", "Project-level rules (committed to version control)."},
		{".codecast/auto_rules.md", "Auto-generated project rules."},
	}

	for _, f := range files {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", f.path, f.desc))
	}
	b.WriteString("\n")

	// EXAMPLES
	b.WriteString("## EXAMPLES\n\n")

	examples := []struct {
		desc, cmd string
	}{
		{"Start an interactive session:", "codecast"},
		{"Run a single prompt in headless mode:", `codecast exec "explain the main.go file"`},
		{"Use a specific model and provider:", "codecast --provider anthropic --model claude-3-opus-20240229"},
		{"Continue the most recent session:", "codecast --continue"},
		{"Resume a specific session:", "codecast --resume abc123"},
		{"Run in safe mode with a daily budget:", "codecast --safe --daily-budget 5.0"},
		{"Stream JSON output for CI/CD integration:", `codecast exec --format stream-json "fix the bug in auth.go"`},
		{"Initialize project configuration:", "codecast init"},
		{"Set the API key (in interactive mode):", "/config set api_key sk-xxxx"},
		{"View cost summary:", "codecast cost summary"},
	}

	for _, ex := range examples {
		b.WriteString(fmt.Sprintf("%s\n\n    %s\n\n", ex.desc, ex.cmd))
	}

	// SEE ALSO
	b.WriteString("## SEE ALSO\n\n")
	b.WriteString("[git](https://git-scm.com), [docker](https://docker.com), [curl](https://curl.se)\n")

	return b.String()
}
