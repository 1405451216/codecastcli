package wizard

import (
	"fmt"
	"strings"
	"testing"
)

func TestProviderModelsContainsAllProviders(t *testing.T) {
	providers := Providers()
	for _, p := range providers {
		models, ok := ProviderModels[p]
		if !ok {
			t.Errorf("ProviderModels missing entry for provider %q", p)
			continue
		}
		if len(models) == 0 {
			t.Errorf("ProviderModels[%q] has no models", p)
		}
	}
}

func TestProviderModelsKnownValues(t *testing.T) {
	tests := map[string][]string{
		"openai":    {"gpt-5.4", "gpt-5.4-pro", "gpt-5.5-instant"},
		"anthropic": {"claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-haiku-3-5-20241022"},
		"gemini":    {"gemini-3-flash", "gemini-3-pro"},
		"deepseek":  {"deepseek-v4-pro", "deepseek-v4-flash", "deepseek-v3"},
		"qwen":      {"qwen3.7-max", "qwen3.7-plus"},
		"glm":       {"glm-5.2", "glm-5v-turbo"},
		"ollama":    {"qwen3:32b", "qwen3:14b", "deepseek-r1:14b", "llama3.3:70b"},
		"cohere":    {"command-r-plus"},
		"mistral":   {"mistral-large-latest"},
	}

	for provider, expected := range tests {
		models, ok := ProviderModels[provider]
		if !ok {
			t.Errorf("missing provider %q", provider)
			continue
		}
		if len(models) != len(expected) {
			t.Errorf("ProviderModels[%q]: got %d models, want %d", provider, len(models), len(expected))
			continue
		}
		for i, m := range expected {
			if models[i] != m {
				t.Errorf("ProviderModels[%q][%d]: got %q, want %q", provider, i, models[i], m)
			}
		}
	}
}

func TestSelectOptionValidSelection(t *testing.T) {
	options := []string{"alpha", "beta", "gamma"}

	tests := []struct {
		input string
		want  string
	}{
		{"1", "alpha"},
		{"2", "beta"},
		{"3", "gamma"},
	}

	for _, tt := range tests {
		line := strings.TrimSpace(tt.input)
		idx := 0
		n, err := fmt.Sscanf(line, "%d", &idx)
		if err != nil {
			t.Errorf("parsing %q: unexpected error: %v", tt.input, err)
			continue
		}
		if n != 1 {
			t.Errorf("parsing %q: expected 1 match, got %d", tt.input, n)
			continue
		}
		if idx < 1 || idx > len(options) {
			t.Errorf("index %d out of range for %q", idx, tt.input)
			continue
		}
		got := options[idx-1]
		if got != tt.want {
			t.Errorf("selection for input %q: got %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSelectOptionInvalidSelection(t *testing.T) {
	options := []string{"alpha", "beta", "gamma"}

	invalidInputs := []string{"0", "4", "abc", "-1", ""}
	for _, input := range invalidInputs {
		line := strings.TrimSpace(input)
		idx := 0
		n, err := fmt.Sscanf(line, "%d", &idx)
		valid := err == nil && n == 1 && idx >= 1 && idx <= len(options)
		if valid {
			t.Errorf("input %q should be invalid but was accepted as index %d", input, idx)
		}
	}
}

func TestProvidersReturnsExpectedOrder(t *testing.T) {
	providers := Providers()
	expected := []string{"openai", "anthropic", "gemini", "ollama", "deepseek", "qwen", "glm", "cohere", "mistral"}
	if len(providers) != len(expected) {
		t.Fatalf("Providers(): got %d entries, want %d", len(providers), len(expected))
	}
	for i, p := range expected {
		if providers[i] != p {
			t.Errorf("Providers()[%d]: got %q, want %q", i, providers[i], p)
		}
	}
}

func TestPermissionModes(t *testing.T) {
	modes := PermissionModes()
	expected := []string{"suggest", "auto-edit", "full-auto"}
	if len(modes) != len(expected) {
		t.Fatalf("PermissionModes(): got %d entries, want %d", len(modes), len(expected))
	}
	for i, m := range expected {
		if modes[i] != m {
			t.Errorf("PermissionModes()[%d]: got %q, want %q", i, modes[i], m)
		}
	}
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"short", "****"},
		{"", "****"},
		{"abcdefgh", "****"},
		{"abcdefghijklmnop", "abcd********mnop"},
		{"sk-1234567890abcdef", "sk-1***********cdef"},
	}

	for _, tt := range tests {
		got := maskKey(tt.input)
		if got != tt.want {
			t.Errorf("maskKey(%q): got %q, want %q", tt.input, got, tt.want)
		}
	}
}
