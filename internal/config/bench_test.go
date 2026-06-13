package config

import (
	"os"
	"testing"
)

// BenchmarkConfigLoad benchmarks loading config.
func BenchmarkConfigLoad(b *testing.B) {
	tmpDir := b.TempDir()
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProfile)
	}()

	// Pre-create config file so Load has something to read.
	cfg := &Config{
		APIKey:   "bench-api-key-12345678",
		Model:    "gpt-4o",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
	}
	if err := Save(cfg); err != nil {
		b.Fatalf("Save() error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Load()
	}
}

// BenchmarkConfigSave benchmarks saving config.
func BenchmarkConfigSave(b *testing.B) {
	tmpDir := b.TempDir()
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProfile)
	}()

	cfg := &Config{
		APIKey:   "bench-api-key-12345678",
		Model:    "gpt-4o",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Save(cfg); err != nil {
			b.Fatalf("Save() error: %v", err)
		}
	}
}
