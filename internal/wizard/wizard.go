package wizard

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Config holds the configuration values collected by the wizard.
type Config struct {
	APIKey         string
	Provider       string
	Model          string
	PermissionMode string
	SafeMode       bool
}

// ProviderModels maps each provider name to a list of recommended models.
var ProviderModels = map[string][]string{
	"openai":    {"gpt-4o", "gpt-4o-mini", "o3"},
	"anthropic": {"claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-haiku-3-5-20241022"},
	"gemini":    {"gemini-2.5-pro", "gemini-2.5-flash"},
	"deepseek":  {"deepseek-chat", "deepseek-reasoner"},
	"qwen":      {"qwen-max", "qwen-plus"},
	"glm":       {"glm-4-plus"},
	"ollama":    {"llama3", "codellama", "mistral"},
	"cohere":    {"command-r-plus"},
	"mistral":   {"mistral-large-latest"},
}

// Providers returns the sorted list of all provider names.
func Providers() []string {
	return []string{
		"openai", "anthropic", "gemini", "ollama",
		"deepseek", "qwen", "glm", "cohere", "mistral",
	}
}

// PermissionModes returns the available permission modes with descriptions.
func PermissionModes() []string {
	return []string{"suggest", "auto-edit", "full-auto"}
}

// RunWizard runs an interactive terminal wizard that collects configuration
// from the user and returns a populated Config.
func RunWizard() (*Config, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("╔══════════════════════════════════════╗")
	fmt.Println("║     CodecastCLI Configuration Wizard ║")
	fmt.Println("╚══════════════════════════════════════╝")
	fmt.Println()

	// Step 1: API Key
	apiKey, err := ReadHiddenInput("API Key: ")
	if err != nil {
		return nil, fmt.Errorf("reading API key: %w", err)
	}
	fmt.Println()

	// Step 2: Provider
	provider, err := SelectOption("Select your provider:", Providers())
	if err != nil {
		return nil, fmt.Errorf("selecting provider: %w", err)
	}
	fmt.Println()

	// Step 3: Model
	models := ProviderModels[provider]
	model, err := SelectOption("Select a model:", models)
	if err != nil {
		return nil, fmt.Errorf("selecting model: %w", err)
	}
	fmt.Println()

	// Step 4: Permission Mode
	fmt.Println("Permission modes:")
	fmt.Println("  suggest   - Ask before every action (safest)")
	fmt.Println("  auto-edit - Automatically apply file edits, ask before running commands")
	fmt.Println("  full-auto - Automatically apply all changes (use with caution)")
	fmt.Println()
	mode, err := SelectOption("Select permission mode:", PermissionModes())
	if err != nil {
		return nil, fmt.Errorf("selecting permission mode: %w", err)
	}
	fmt.Println()

	// Step 5: Safe Mode
	safeMode, err := askYesNo(reader, "Enable Safe Mode? (restricts dangerous operations) [y/N]: ")
	if err != nil {
		return nil, fmt.Errorf("reading safe mode: %w", err)
	}
	fmt.Println()

	// Step 6: Summary & Confirmation
	cfg := &Config{
		APIKey:         apiKey,
		Provider:       provider,
		Model:          model,
		PermissionMode: mode,
		SafeMode:       safeMode,
	}

	printSummary(cfg)

	confirmed, err := askYesNo(reader, "Confirm configuration? [Y/n]: ")
	if err != nil {
		return nil, fmt.Errorf("reading confirmation: %w", err)
	}
	if !confirmed {
		fmt.Println("Configuration cancelled.")
		return nil, fmt.Errorf("configuration cancelled by user")
	}

	fmt.Println("Configuration saved!")
	return cfg, nil
}

// SelectOption shows a numbered list of options and reads the user's selection.
func SelectOption(prompt string, options []string) (string, error) {
	fmt.Println(prompt)
	for i, opt := range options {
		fmt.Printf("  %d. %s\n", i+1, opt)
	}
	fmt.Print("Enter number: ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading selection: %w", err)
	}
	line = strings.TrimSpace(line)

	idx := 0
	_, err = fmt.Sscanf(line, "%d", &idx)
	if err != nil || idx < 1 || idx > len(options) {
		return "", fmt.Errorf("invalid selection: %q", line)
	}
	return options[idx-1], nil
}

// askYesNo prompts a yes/no question and returns the boolean result.
// Default is false for prompts ending in [y/N] and true for [Y/n].
func askYesNo(reader *bufio.Reader, prompt string) (bool, error) {
	fmt.Print(prompt)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("reading input: %w", err)
	}
	line = strings.TrimSpace(strings.ToLower(line))

	defaultYes := strings.Contains(prompt, "[Y/n]")
	if line == "" {
		return defaultYes, nil
	}
	return line == "y" || line == "yes", nil
}

// printSummary displays the collected configuration.
func printSummary(cfg *Config) {
	fmt.Println("── Configuration Summary ──")
	fmt.Printf("  Provider:        %s\n", cfg.Provider)
	fmt.Printf("  Model:           %s\n", cfg.Model)
	fmt.Printf("  Permission Mode: %s\n", cfg.PermissionMode)
	fmt.Printf("  Safe Mode:       %t\n", cfg.SafeMode)
	fmt.Printf("  API Key:         %s\n", maskKey(cfg.APIKey))
	fmt.Println("───────────────────────────")
}

// maskKey returns a masked version of the API key for display.
func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}
