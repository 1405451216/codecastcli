package errors

// errors 包提供统一的错误码体系和用户友好错误类型。
//
// Phase 5.3: 所有面向用户的错误都应包含：
//   - 机器可读的错误码（用于脚本判断和日志分析）
//   - 人类可读的错误消息（含中文提示和修复建议）
//   - 可选的修复建议（帮助用户自助排查）
//
// 错误码命名规范：
//   - ERR_PROVIDER_xxx  — Provider/模型相关
//   - ERR_CONFIG_xxx    — 配置相关
//   - ERR_SESSION_xxx   — 会话/记忆相关
//   - ERR_TOOL_xxx      — 工具执行相关
//   - ERR_PERMISSION_xxx — 权限相关
//   - ERR_NETWORK_xxx   — 网络/连接相关
//   - ERR_BUDGET_xxx    — 预算/费用相关
//   - ERR_INTERNAL_xxx  — 内部错误

import (
	"errors"
	"fmt"
	"sync"
)

// ErrorCode 是机器可读的错误码
type ErrorCode string

// 错误码常量
const (
	// Provider 相关
	ErrProviderNotFound    ErrorCode = "ERR_PROVIDER_NOT_FOUND"
	ErrProviderUnavailable ErrorCode = "ERR_PROVIDER_UNAVAILABLE"
	ErrProviderAuth        ErrorCode = "ERR_PROVIDER_AUTH"
	ErrProviderRateLimit   ErrorCode = "ERR_PROVIDER_RATE_LIMIT"
	ErrProviderTimeout     ErrorCode = "ERR_PROVIDER_TIMEOUT"

	// 配置相关
	ErrConfigInvalid     ErrorCode = "ERR_CONFIG_INVALID"
	ErrConfigMissingKey  ErrorCode = "ERR_CONFIG_MISSING_KEY"
	ErrConfigParseFailed ErrorCode = "ERR_CONFIG_PARSE_FAILED"

	// 会话相关
	ErrSessionNotFound ErrorCode = "ERR_SESSION_NOT_FOUND"
	ErrSessionCorrupt  ErrorCode = "ERR_SESSION_CORRUPT"
	ErrMemoryInit      ErrorCode = "ERR_MEMORY_INIT"

	// 工具相关
	ErrToolNotFound   ErrorCode = "ERR_TOOL_NOT_FOUND"
	ErrToolExecFailed ErrorCode = "ERR_TOOL_EXEC_FAILED"
	ErrToolPermission ErrorCode = "ERR_TOOL_PERMISSION"
	ErrToolTimeout    ErrorCode = "ERR_TOOL_TIMEOUT"

	// 权限相关
	ErrPermissionDenied ErrorCode = "ERR_PERMISSION_DENIED"
	ErrSafeModeBlocked  ErrorCode = "ERR_SAFE_MODE_BLOCKED"

	// 网络相关
	ErrNetworkUnreachable ErrorCode = "ERR_NETWORK_UNREACHABLE"
	ErrNetworkDNS         ErrorCode = "ERR_NETWORK_DNS"

	// 预算相关
	ErrBudgetExceeded     ErrorCode = "ERR_BUDGET_EXCEEDED"
	ErrBudgetDailyLimit   ErrorCode = "ERR_BUDGET_DAILY_LIMIT"
	ErrBudgetSessionLimit ErrorCode = "ERR_BUDGET_SESSION_LIMIT"

	// 内部错误
	ErrInternalError ErrorCode = "ERR_INTERNAL_ERROR"
	ErrContextCancel ErrorCode = "ERR_CONTEXT_CANCEL"
)

// UserFacingError 是面向用户的错误类型，包含错误码、消息和修复建议。
//
// 使用方式：
//
//	return errors.NewUserFacingError(ErrProviderAuth,
//	    "API Key 验证失败",
//	    "请检查: 1) API Key 是否正确 2) Key 是否已过期 3) 账户余额是否充足")
type UserFacingError struct {
	Code       ErrorCode
	Message    string
	Hint       string
	WrappedErr error
}

// Error 实现 error 接口
func (e *UserFacingError) Error() string {
	if e.WrappedErr != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.WrappedErr)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Is/As 链路
func (e *UserFacingError) Unwrap() error {
	return e.WrappedErr
}

// UserHint 返回面向用户的修复建议
func (e *UserFacingError) UserHint() string {
	return e.Hint
}

// NewUserFacingError 创建一个新的用户友好错误
func NewUserFacingError(code ErrorCode, message, hint string) *UserFacingError {
	return &UserFacingError{
		Code:    code,
		Message: message,
		Hint:    hint,
	}
}

// WrapError 包装一个已有错误为用户友好错误
func WrapError(code ErrorCode, message, hint string, err error) *UserFacingError {
	return &UserFacingError{
		Code:       code,
		Message:    message,
		Hint:       hint,
		WrappedErr: err,
	}
}

// IsUserFacingError 检查错误是否为 UserFacingError 类型
func IsUserFacingError(err error) bool {
	var ufe *UserFacingError
	return errors.As(err, &ufe)
}

// AsUserFacingError 尝试将错误转换为 UserFacingError
func AsUserFacingError(err error) (*UserFacingError, bool) {
	var ufe *UserFacingError
	if errors.As(err, &ufe) {
		return ufe, true
	}
	return nil, false
}

// DegradationLevel 表示优雅降级的级别
type DegradationLevel int

const (
	// DegradationNone 无降级，全功能运行
	DegradationNone DegradationLevel = iota
	// DegradationMinor 次要功能降级（如索引失败但 Agent 仍可用）
	DegradationMinor
	// DegradationMajor 主要功能降级（如 TUI 回退到纯文本）
	DegradationMajor
	// DegradationCritical 核心功能降级（如 LLM 不可用，只保留工具执行）
	DegradationCritical
)

// DegradationStatus 描述当前降级状态
type DegradationStatus struct {
	Level    DegradationLevel
	Features []string // 被降级的功能列表
	Reason   string
}

// DegradationMatrix 跟踪各模块的降级状态
type DegradationMatrix struct {
	mu       sync.RWMutex
	statuses map[string]*DegradationStatus
}

// NewDegradationMatrix 创建降级矩阵
func NewDegradationMatrix() *DegradationMatrix {
	return &DegradationMatrix{
		statuses: make(map[string]*DegradationStatus),
	}
}

// ReportDegradation 报告某个模块的降级状态
func (m *DegradationMatrix) ReportDegradation(module string, status *DegradationStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses[module] = status
}

// GetOverallLevel 返回全局降级级别（取最高）
func (m *DegradationMatrix) GetOverallLevel() DegradationLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.overallLevelLocked()
}

// overallLevelLocked 无锁版本，调用方必须已持有读锁或写锁
func (m *DegradationMatrix) overallLevelLocked() DegradationLevel {
	maxLevel := DegradationNone
	for _, s := range m.statuses {
		if s.Level > maxLevel {
			maxLevel = s.Level
		}
	}
	return maxLevel
}

// IsDegraded 返回是否有功能降级
func (m *DegradationMatrix) IsDegraded() bool {
	return m.GetOverallLevel() > DegradationNone
}

// Summary 返回降级摘要信息
func (m *DegradationMatrix) Summary() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	level := m.overallLevelLocked()
	if level == DegradationNone {
		return "全功能运行中"
	}
	summary := fmt.Sprintf("降级级别: %d\n", level)
	for module, status := range m.statuses {
		summary += fmt.Sprintf("  [%s] %s (级别=%d)\n", module, status.Reason, status.Level)
	}
	return summary
}
