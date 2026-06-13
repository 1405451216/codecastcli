package pool

import (
	"context"
	"fmt"
	"time"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/config"
	"codecast/cli/internal/provider"
)

// Manager Agent Pool 管理器
type Manager struct {
	pool   *ap.Pool
	config *config.Config
}

// NewManager 创建 Pool 管理器
func NewManager(cfg *config.Config, maxConcurrency int) (*Manager, error) {
	if maxConcurrency <= 0 {
		maxConcurrency = 5
	}

	poolCfg := ap.PoolConfig{
		MaxConcurrency: maxConcurrency,
		Timeout:        5 * time.Minute,
		RetryPolicy: ap.RetryPolicy{
			MaxRetries:      2,
			Backoff:         5 * time.Second,
			RetryableErrors: []string{"timeout", "rate_limit", "connection_reset"},
		},
		DefaultAgent: ap.ReActAgentConfig{
			SystemPrompt: "你是一个专业的 AI 编程助手，负责并行执行子任务。",
			MaxTurns:     15,
			Temperature:  0.3,
		},
	}

	pool := ap.NewPool(poolCfg)

	// 设置 Agent 工厂
	llmProvider, err := provider.CreateProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Provider 失败: %w", err)
	}
	pool.SetModel(llmProvider)

	return &Manager{
		pool:   pool,
		config: cfg,
	}, nil
}

// SetToolkit 设置工具集
func (m *Manager) SetToolkit(registry *ap.ToolRegistry) {
	m.pool.SetToolkit(registry)
}

// SetAgentFactory 设置自定义 Agent 工厂
func (m *Manager) SetAgentFactory(factory ap.AgentFactory) {
	m.pool.SetAgentFactory(factory)
}

// DispatchTasks 分发多个并行任务
func (m *Manager) DispatchTasks(ctx context.Context, tasks []TaskDefinition) ([]*TaskExecutionResult, error) {
	taskConfigs := make([]ap.TaskConfig, len(tasks))
	for i, t := range tasks {
		taskConfigs[i] = ap.TaskConfig{
			ID:         t.ID,
			Title:      t.Title,
			Prompt:     t.Prompt,
			SessionID:  t.SessionID,
			FilesScope: t.FilesScope,
			MaxTurns:   t.MaxTurns,
			Metadata:   t.Metadata,
		}
	}

	results, err := m.pool.Dispatch(ctx, taskConfigs)
	if err != nil {
		return nil, fmt.Errorf("任务分发失败: %w", err)
	}

	// 转换结果
	execResults := make([]*TaskExecutionResult, len(results))
	for i, r := range results {
		execResults[i] = &TaskExecutionResult{
			TaskID:   r.TaskID,
			Status:   string(r.Status),
			Duration: r.Duration,
		}
		if r.Response != nil {
			execResults[i].Content = r.Response.Content
		}
		if r.Error != nil {
			execResults[i].Error = r.Error.Error()
		}
	}

	return execResults, nil
}

// CancelTask 取消任务
func (m *Manager) CancelTask(taskID string) error {
	return m.pool.Cancel(taskID)
}

// CancelAll 取消所有任务
func (m *Manager) CancelAll() {
	m.pool.CancelAll()
}

// GetStats 获取 Pool 统计信息
func (m *Manager) GetStats() ap.PoolStats {
	return m.pool.Stats()
}

// ListTasks 列出所有任务
func (m *Manager) ListTasks() []ap.TaskResult {
	return m.pool.ListTasks()
}

// EventChannel 返回事件通道
func (m *Manager) EventChannel() <-chan ap.PoolEvent {
	return m.pool.EventChannel()
}

// Close 关闭 Pool
func (m *Manager) Close() {
	m.pool.Close()
}

// TaskDefinition 任务定义
type TaskDefinition struct {
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	Prompt     string            `json:"prompt"`
	SessionID  string            `json:"session_id,omitempty"`
	FilesScope []string          `json:"files_scope,omitempty"`
	MaxTurns   int               `json:"max_turns,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// TaskExecutionResult 任务执行结果
type TaskExecutionResult struct {
	TaskID   string        `json:"task_id"`
	Content  string        `json:"content,omitempty"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
	Status   string        `json:"status"`
}
