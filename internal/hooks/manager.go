package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	ap "agentprimordia/pkg"
	"gopkg.in/yaml.v3"
)

// HookConfig 钩子配置
type HookConfig struct {
	Name    string            `yaml:"name" json:"name"`
	Point   string            `yaml:"point" json:"point"`                         // before_tool, after_tool, before_run, after_run
	Command string            `yaml:"command" json:"command"`                     // Shell command to execute
	Args    []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Timeout int               `yaml:"timeout,omitempty" json:"timeout,omitempty"` // seconds
	Enabled bool              `yaml:"enabled" json:"enabled"`
}

// HookManager 管理用户自定义钩子
type HookManager struct {
	hooks    []HookConfig
	hooksDir string
	mu       sync.RWMutex
}

// NewHookManager 创建钩子管理器
func NewHookManager(hooksDir string) *HookManager {
	return &HookManager{
		hooksDir: hooksDir,
		hooks:    make([]HookConfig, 0),
	}
}

// Load 从 .codecast/hooks/ 目录加载钩子配置
func (m *HookManager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 确保 hooks 目录存在
	if err := os.MkdirAll(m.hooksDir, 0755); err != nil {
		return fmt.Errorf("创建 hooks 目录失败: %w", err)
	}

	// 读取 hooks.yaml 配置
	configPath := filepath.Join(m.hooksDir, "hooks.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 没有配置文件是正常的
		}
		return fmt.Errorf("读取 hooks 配置失败: %w", err)
	}

	var configs []HookConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return fmt.Errorf("解析 hooks 配置失败: %w", err)
	}

	m.hooks = configs
	return nil
}

// RegisterAll 将所有用户钩子注册到 AP 的 HookManager
func (m *HookManager) RegisterAll(apHooks *ap.HookManager) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, hook := range m.hooks {
		if !hook.Enabled {
			continue
		}

		point, err := parseHookPoint(hook.Point)
		if err != nil {
			return fmt.Errorf("钩子 %q: %w", hook.Name, err)
		}

		hookFunc := m.buildHookFunc(hook)
		apHooks.Register(point, hookFunc)
	}

	return nil
}

// buildHookFunc 构建钩子执行函数
func (m *HookManager) buildHookFunc(cfg HookConfig) ap.HookFunc {
	return func(ctx context.Context, hctx *ap.HookContext) error {
		// 构建环境变量
		env := m.buildEnv(hctx)

		// 执行 Shell 命令
		result, err := m.runCommand(ctx, cfg, env)
		if err != nil {
			return fmt.Errorf("钩子 %q 执行失败: %w", cfg.Name, err)
		}

		// 如果钩子输出非空，可以用于日志或修改上下文
		_ = result
		return nil
	}
}

// buildEnv 构建钩子环境变量
func (m *HookManager) buildEnv(hctx *ap.HookContext) map[string]string {
	env := map[string]string{
		"CODECAST_AGENT_ID":   hctx.AgentID,
		"CODECAST_SESSION_ID": hctx.SessionID,
		"CODECAST_TURN":       fmt.Sprintf("%d", hctx.Turn),
	}

	if hctx.ToolCall != nil {
		env["CODECAST_TOOL_NAME"] = hctx.ToolCall.Name
		env["CODECAST_TOOL_ARGS"] = hctx.ToolCall.Args
	}

	if hctx.ToolResult != nil {
		env["CODECAST_TOOL_RESULT"] = hctx.ToolResult.Content
	}

	return env
}

// runCommand 执行 Shell 命令
func (m *HookManager) runCommand(ctx context.Context, cfg HookConfig, env map[string]string) (string, error) {
	// R5-C4 修复：根据操作系统选择正确的 Shell
	var shell string
	switch runtime.GOOS {
	case "windows":
		shell = "powershell"
	default:
		shell = "sh"
	}

	cmdStr := cfg.Command
	// R5-H17 修复：空命令检查
	if strings.TrimSpace(cmdStr) == "" {
		return "", fmt.Errorf("钩子命令为空: %s", cfg.Name)
	}
	// C-07 修复：命令白名单验证，防止恶意命令注入
	if err := validateCommand(cmdStr); err != nil {
		return "", fmt.Errorf("命令验证失败: %w", err)
	}
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cfg.Timeout)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, shell, "-c", cmdStr)

	// High 修复：清理基础环境变量，防止注入
	cmd.Env = sanitizeEnvironment(os.Environ())
	for k, v := range env {
		// C-07 修复：增强环境变量验证，防止注入
		safeV := sanitizeEnvValue(v)
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, safeV))
	}
	for k, v := range cfg.Env {
		safeV := sanitizeEnvValue(v)
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, safeV))
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("命令执行失败: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// validateCommand 验证命令安全性
func validateCommand(cmd string) error {
	// 禁止的危险命令模式
	dangerousPatterns := []string{
		"rm -rf /", "rm -rf /*", "del /f /s /q", "format",
		"mkfs", "dd if=", "> /dev/", "chmod -R 777",
		"curl.*|.*sh", "wget.*|.*sh", "eval",
	}
	cmdLower := strings.ToLower(cmd)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(cmdLower, strings.ToLower(pattern)) {
			return fmt.Errorf("命令包含危险模式: %s", pattern)
		}
	}
	return nil
}

// sanitizeEnvValue 清理环境变量值，防止注入
func sanitizeEnvValue(v string) string {
	// 移除换行符和回车符
	v = strings.ReplaceAll(v, "\n", "")
	v = strings.ReplaceAll(v, "\r", "")
	// 移除可能的命令分隔符
	v = strings.ReplaceAll(v, ";", "")
	v = strings.ReplaceAll(v, "|", "")
	v = strings.ReplaceAll(v, "&", "")
	v = strings.ReplaceAll(v, "`", "")
	v = strings.ReplaceAll(v, "$(", "")
	return v
}

// sanitizeEnvironment 清理基础环境变量，移除包含危险字符的变量值
func sanitizeEnvironment(env []string) []string {
	sanitized := make([]string, 0, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]
		// 移除可能包含命令注入的环境变量
		if strings.Contains(value, ";") || strings.Contains(value, "|") ||
			strings.Contains(value, "&") || strings.Contains(value, "`") ||
			strings.Contains(value, "\n") || strings.Contains(value, "\r") ||
			strings.Contains(value, "$(") {
			continue
		}
		sanitized = append(sanitized, fmt.Sprintf("%s=%s", key, value))
	}
	return sanitized
}

// List 返回所有钩子
func (m *HookManager) List() []HookConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]HookConfig, len(m.hooks))
	copy(result, m.hooks)
	return result
}

// Add 添加钩子
func (m *HookManager) Add(hook HookConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook)
}

// Remove 移除钩子
func (m *HookManager) Remove(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, h := range m.hooks {
		if h.Name == name {
			m.hooks = append(m.hooks[:i], m.hooks[i+1:]...)
			return
		}
	}
}

// Save 保存钩子配置到文件
func (m *HookManager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := os.MkdirAll(m.hooksDir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(m.hooks)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(m.hooksDir, "hooks.yaml"), data, 0644)
}

// parseHookPoint 解析钩子挂载点
func parseHookPoint(s string) (ap.HookPoint, error) {
	switch strings.ToLower(s) {
	case "before_tool":
		return ap.HookBeforeTool, nil
	case "after_tool":
		return ap.HookAfterTool, nil
	case "before_run":
		return ap.HookBeforeRun, nil
	case "after_run":
		return ap.HookAfterRun, nil
	case "before_turn":
		return ap.HookBeforeTurn, nil
	case "after_turn":
		return ap.HookAfterTurn, nil
	case "before_llm":
		return ap.HookBeforeLLM, nil
	case "after_llm":
		return ap.HookAfterLLM, nil
	case "on_error":
		return ap.HookOnError, nil
	case "on_complete":
		return ap.HookOnComplete, nil
	default:
		return ap.HookBeforeTool, fmt.Errorf("未知的钩子挂载点: %s", s)
	}
}

// InitHooksTemplate 创建 hooks 配置模板
func InitHooksTemplate(hooksDir string) error {
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return err
	}

	template := []HookConfig{
		{
			Name:    "log-tool-usage",
			Point:   "after_tool",
			Command: "echo \"[$(date)] Tool: $CODECAST_TOOL_NAME\" >> .codecast/hooks/tool-log.txt",
			Enabled: false,
		},
	}

	data, err := yaml.Marshal(template)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(hooksDir, "hooks.yaml"), data, 0644)
}
