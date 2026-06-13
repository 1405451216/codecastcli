package docs

import (
	"strings"
	"testing"
)

func TestGenerateManPage(t *testing.T) {
	got := GenerateManPage()

	// Must not be empty
	if got == "" {
		t.Fatal("GenerateManPage() returned empty string")
	}

	// Must contain essential groff header
	if !strings.Contains(got, ".TH CODECAST 1") {
		t.Error("man page missing .TH header")
	}

	// Required sections
	sections := []string{
		".SH NAME",
		".SH SYNOPSIS",
		".SH DESCRIPTION",
		".SH OPTIONS",
		".SH SUBCOMMANDS",
		".SH INTERACTIVE COMMANDS",
		".SH ENVIRONMENT",
		".SH FILES",
		".SH EXAMPLES",
		".SH \"SEE ALSO\"",
	}
	for _, s := range sections {
		if !strings.Contains(got, s) {
			t.Errorf("man page missing section: %s", s)
		}
	}

	// NAME section content
	if !strings.Contains(got, "codecast \\- AI-powered terminal agent") {
		t.Error("man page missing NAME content")
	}

	// SYNOPSIS must reference key invocations
	synopsisItems := []string{
		"codecast exec",
		"codecast init",
		"codecast config",
		"codecast plugin",
		"codecast pool",
		"codecast sandbox",
		"codecast mcp",
		"codecast cost",
	}
	for _, item := range synopsisItems {
		if !strings.Contains(got, item) {
			t.Errorf("man page synopsis missing: %s", item)
		}
	}

	// All required flags from root.go
	flags := []string{
		"--model",
		"--provider",
		"--api-key",
		"--base-url",
		"--permission-mode",
		"--scope",
		"--safe",
		"--auto-compact",
		"--auto-compact-ratio",
		"--auto-checkpoint",
		"--auto-stash",
		"--daily-budget",
		"--session-budget",
		"--continue",
		"--resume",
		"--log-level",
		"--log-format",
	}
	for _, f := range flags {
		if !strings.Contains(got, f) {
			t.Errorf("man page missing flag: %s", f)
		}
	}

	// Interactive commands
	slashCommands := []string{
		"/help",
		"/quit",
		"/clear",
		"/compact",
		"/rules",
		"/tools",
		"/models",
		"/stats",
		"/sessions",
		"/export",
		"/resume",
		"/plan",
		"/delegate",
		"/hooks",
		"/vision",
		"/screenshot",
		"/pool",
		"/plugins",
		"/index",
		"/model",
		"/sandbox",
		"/mcp",
		"/mode",
		"/undo",
		"/budget",
		"/config",
	}
	for _, c := range slashCommands {
		if !strings.Contains(got, c) {
			t.Errorf("man page missing interactive command: %s", c)
		}
	}

	// Environment variables
	envVars := []string{
		"CODECAST_API_KEY",
		"CODECAST_PROVIDER",
		"CODECAST_MODEL",
		"CODECAST_BASE_URL",
	}
	for _, e := range envVars {
		if !strings.Contains(got, e) {
			t.Errorf("man page missing environment variable: %s", e)
		}
	}

	// Files
	filePaths := []string{
		"~/.codecast/config.yaml",
		".codecast/rules.md",
		".codecast/auto_rules.md",
	}
	for _, f := range filePaths {
		if !strings.Contains(got, f) {
			t.Errorf("man page missing file path: %s", f)
		}
	}

	// Examples section must contain some usage examples
	exampleSnippets := []string{
		"codecast exec",
		"codecast init",
		"/config set api_key",
		"codecast cost summary",
	}
	for _, ex := range exampleSnippets {
		if !strings.Contains(got, ex) {
			t.Errorf("man page examples missing: %s", ex)
		}
	}

	// SEE ALSO section
	if !strings.Contains(got, "git") {
		t.Error("man page SEE ALSO missing git reference")
	}
}

func TestGenerateManPageGroffFormat(t *testing.T) {
	got := GenerateManPage()

	// Should contain common groff macros
	groffMacros := []string{".TH", ".SH", ".TP", ".B", ".BR", ".PP", ".RS", ".RE"}
	for _, m := range groffMacros {
		if !strings.Contains(got, m) {
			t.Errorf("man page missing groff macro: %s", m)
		}
	}
}

func TestGenerateMarkdownHelp(t *testing.T) {
	got := GenerateMarkdownHelp()

	// Must not be empty
	if got == "" {
		t.Fatal("GenerateMarkdownHelp() returned empty string")
	}

	// Must contain Markdown headings
	headings := []string{
		"## NAME",
		"## SYNOPSIS",
		"## DESCRIPTION",
		"## OPTIONS",
		"## SUBCOMMANDS",
		"## INTERACTIVE COMMANDS",
		"## ENVIRONMENT",
		"## FILES",
		"## EXAMPLES",
		"## SEE ALSO",
	}
	for _, h := range headings {
		if !strings.Contains(got, h) {
			t.Errorf("markdown help missing heading: %s", h)
		}
	}

	// NAME section content
	if !strings.Contains(got, "codecast - AI-powered terminal agent") {
		t.Error("markdown help missing NAME content")
	}

	// Synopsis must reference key invocations
	synopsisItems := []string{
		"codecast exec",
		"codecast init",
		"codecast config",
		"codecast plugin",
		"codecast pool",
		"codecast sandbox",
		"codecast mcp",
		"codecast cost",
	}
	for _, item := range synopsisItems {
		if !strings.Contains(got, item) {
			t.Errorf("markdown help synopsis missing: %s", item)
		}
	}

	// All required flags
	flags := []string{
		"--model",
		"--provider",
		"--api-key",
		"--base-url",
		"--permission-mode",
		"--scope",
		"--safe",
		"--auto-compact",
		"--auto-compact-ratio",
		"--auto-checkpoint",
		"--auto-stash",
		"--daily-budget",
		"--session-budget",
		"--continue",
		"--resume",
		"--log-level",
		"--log-format",
	}
	for _, f := range flags {
		if !strings.Contains(got, f) {
			t.Errorf("markdown help missing flag: %s", f)
		}
	}

	// Interactive commands
	slashCommands := []string{
		"/help",
		"/quit",
		"/clear",
		"/compact",
		"/rules",
		"/tools",
		"/models",
		"/stats",
		"/sessions",
		"/export",
		"/resume",
		"/plan",
		"/delegate",
		"/hooks",
		"/vision",
		"/screenshot",
		"/pool",
		"/plugins",
		"/index",
		"/model",
		"/sandbox",
		"/mcp",
		"/mode",
		"/undo",
		"/budget",
		"/config",
	}
	for _, c := range slashCommands {
		if !strings.Contains(got, c) {
			t.Errorf("markdown help missing interactive command: %s", c)
		}
	}

	// Environment variables
	envVars := []string{
		"CODECAST_API_KEY",
		"CODECAST_PROVIDER",
		"CODECAST_MODEL",
		"CODECAST_BASE_URL",
	}
	for _, e := range envVars {
		if !strings.Contains(got, e) {
			t.Errorf("markdown help missing environment variable: %s", e)
		}
	}

	// Files
	filePaths := []string{
		"~/.codecast/config.yaml",
		".codecast/rules.md",
		".codecast/auto_rules.md",
	}
	for _, f := range filePaths {
		if !strings.Contains(got, f) {
			t.Errorf("markdown help missing file path: %s", f)
		}
	}

	// Examples
	exampleSnippets := []string{
		"codecast exec",
		"codecast init",
		"/config set api_key",
		"codecast cost summary",
	}
	for _, ex := range exampleSnippets {
		if !strings.Contains(got, ex) {
			t.Errorf("markdown help examples missing: %s", ex)
		}
	}

	// SEE ALSO
	if !strings.Contains(got, "git") {
		t.Error("markdown help SEE ALSO missing git reference")
	}
}

func TestGenerateMarkdownHelpFormat(t *testing.T) {
	got := GenerateMarkdownHelp()

	// Should use Markdown bold for option names
	if !strings.Contains(got, "**--model") {
		t.Error("markdown help should use **bold** for option names")
	}

	// Should use backtick code spans for synopsis
	if !strings.Contains(got, "`codecast") {
		t.Error("markdown help should use backtick code spans for synopsis")
	}

	// Should use list items for options
	if !strings.Contains(got, "- **") {
		t.Error("markdown help should use list items for options/subcommands")
	}
}

func TestGenerateManPageAndMarkdownConsistency(t *testing.T) {
	manPage := GenerateManPage()
	markdown := GenerateMarkdownHelp()

	// Both should mention the same core items
	coreItems := []string{
		"codecast exec",
		"codecast init",
		"codecast config",
		"CODECAST_API_KEY",
		"CODECAST_PROVIDER",
		"CODECAST_MODEL",
		"CODECAST_BASE_URL",
		"~/.codecast/config.yaml",
		".codecast/rules.md",
		".codecast/auto_rules.md",
		"AgentPrimordia",
	}
	for _, item := range coreItems {
		if !strings.Contains(manPage, item) {
			t.Errorf("man page missing core item: %s", item)
		}
		if !strings.Contains(markdown, item) {
			t.Errorf("markdown help missing core item: %s", item)
		}
	}
}
