package logx

import (
	"log/slog"
	"sync"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input   string
		want    Level
		wantErr bool
	}{
		{"debug", LevelDebug, false},
		{"info", LevelInfo, false},
		{"warn", LevelWarn, false},
		{"warning", LevelWarn, false},
		{"error", LevelError, false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLevel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestInitAndSetLevel(t *testing.T) {
	// 重置全局状态以便测试
	globalLogger = nil
	globalOpts = new(sync.Once)

	Init(WithLevel(LevelDebug), WithFormat("json"), WithOutput("stderr"))

	if globalLogger == nil {
		t.Fatal("Init 后 globalLogger 不应为 nil")
	}

	if GetLevel() != "DEBUG" {
		t.Errorf("初始级别应为 DEBUG, 得到 %s", GetLevel())
	}

	SetLevel(LevelError)
	if GetLevel() != "ERROR" {
		t.Errorf("设置后级别应为 ERROR, 得到 %s", GetLevel())
	}
}

func TestLogger(t *testing.T) {
	// 确保已初始化
	if Logger() == nil {
		t.Fatal("Logger() 不应返回 nil")
	}
}

func TestParseLevel_Internal(t *testing.T) {
	tests := []struct {
		input Level
		want  slog.Level
	}{
		{LevelDebug, slog.LevelDebug},
		{LevelInfo, slog.LevelInfo},
		{LevelWarn, slog.LevelWarn},
		{LevelError, slog.LevelError},
		{"unknown", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := parseLevel(tt.input)
			if got != tt.want {
				t.Errorf("parseLevel(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
