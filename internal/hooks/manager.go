package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

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
	cmd := exec.CommandContext(ctx, "sh", "-c", cfg.Command)

	// 设置环境变量
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("命令执行失败: %w\n输出: %s", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
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
