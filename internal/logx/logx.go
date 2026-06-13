package logx

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"codecast/cli/internal/config"
)

var (
	globalLogger *slog.Logger
	globalLevel  = new(slog.LevelVar)
	globalOpts   = &sync.Once{}
)

// Level 日志级别
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Init 初始化全局日志
func Init(opts ...Option) {
	globalOpts.Do(func() {
		cfg := &logConfig{
			level:     LevelInfo,
			format:    "text",
			output:    "stderr",
			addSource: false,
		}
		for _, o := range opts {
			o(cfg)
		}

		globalLevel.Set(parseLevel(cfg.level))

		var w io.Writer
		switch cfg.output {
		case "stdout":
			w = os.Stdout
		case "stderr":
			w = os.Stderr
		default:
			// 文件输出
			_ = os.MkdirAll(filepath.Dir(cfg.output), 0750)
			f, err := os.OpenFile(cfg.output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
			if err != nil {
				w = os.Stderr
			} else {
				w = f
			}
		}

		var handler slog.Handler
		if cfg.format == "json" {
			handler = slog.NewJSONHandler(w, &slog.HandlerOptions{
				Level:       globalLevel,
				AddSource:   cfg.addSource,
			})
		} else {
			handler = slog.NewTextHandler(w, &slog.HandlerOptions{
				Level:       globalLevel,
				AddSource:   cfg.addSource,
			})
		}

		globalLogger = slog.New(handler)
		slog.SetDefault(globalLogger)
	})
}

// Logger 返回全局 logger
func Logger() *slog.Logger {
	if globalLogger == nil {
		Init()
	}
	return globalLogger
}

// SetLevel 动态设置日志级别
func SetLevel(l Level) {
	globalLevel.Set(parseLevel(l))
}

// GetLevel 返回当前日志级别
func GetLevel() string {
	lvl := globalLevel.Level()
	switch lvl {
	case slog.LevelDebug:
		return "DEBUG"
	case slog.LevelInfo:
		return "INFO"
	case slog.LevelWarn:
		return "WARN"
	case slog.LevelError:
		return "ERROR"
	default:
		return lvl.String()
	}
}

// Debug 输出 debug 日志
func Debug(msg string, args ...any) {
	Logger().Debug(msg, args...)
}

// Info 输出 info 日志
func Info(msg string, args ...any) {
	Logger().Info(msg, args...)
}

// Warn 输出 warn 日志
func Warn(msg string, args ...any) {
	Logger().Warn(msg, args...)
}

// Error 输出 error 日志
func Error(msg string, args ...any) {
	Logger().Error(msg, args...)
}

func parseLevel(l Level) slog.Level {
	switch strings.ToLower(string(l)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type logConfig struct {
	level     Level
	format    string
	output    string
	addSource bool
}

// Option 日志配置选项
type Option func(*logConfig)

// WithLevel 设置日志级别
func WithLevel(l Level) Option {
	return func(c *logConfig) { c.level = l }
}

// WithFormat 设置输出格式 (text/json)
func WithFormat(f string) Option {
	return func(c *logConfig) { c.format = f }
}

// WithOutput 设置输出目标 (stdout/stderr/文件路径)
func WithOutput(o string) Option {
	return func(c *logConfig) { c.output = o }
}

// WithSource 是否添加源码位置
func WithSource(v bool) Option {
	return func(c *logConfig) { c.addSource = v }
}

// DefaultLogPath 返回默认日志文件路径
func DefaultLogPath() string {
	return filepath.Join(config.GetConfigDir(), "codecast.log")
}

// ParseLevel 从字符串解析日志级别
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	default:
		return "", fmt.Errorf("无效的日志级别: %s", s)
	}
}
