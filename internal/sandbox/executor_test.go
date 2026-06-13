package sandbox

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("DefaultConfig().Enabled 应为 false")
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("DefaultConfig().Timeout = %v, want %v", cfg.Timeout, 30*time.Second)
	}
	if cfg.MaxMemoryMB != 512 {
		t.Errorf("DefaultConfig().MaxMemoryMB = %d, want 512", cfg.MaxMemoryMB)
	}
	if cfg.AllowNetwork {
		t.Error("DefaultConfig().AllowNetwork 应为 false")
	}
}

func TestExecutor_ExecuteDirect(t *testing.T) {
	cfg := DefaultConfig()
	e := NewExecutor(cfg)

	ctx := context.Background()

	// 在 Windows 上执行简单命令
	result, err := e.Execute(ctx, "cmd", "/c", "echo hello")
	if err != nil {
		t.Fatalf("Execute() 失败: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}

	// cmd /c echo hello 输出带换行
	if result.Stdout == "" {
		t.Error("Stdout 不应为空")
	}
}
