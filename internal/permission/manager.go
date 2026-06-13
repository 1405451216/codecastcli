package permission

import (
	"fmt"
	"strings"
	"sync"
)

// ApprovalMode 审批模式
type ApprovalMode int

const (
	// ModeSuggest 建议模式 - 所有工具调用前弹出确认提示
	ModeSuggest ApprovalMode = iota
	// ModeAutoEdit 自动编辑 - 文件读写类工具自动放行，Shell 命令仍需确认
	ModeAutoEdit
	// ModeFullAuto 全自动 - 所有工具自动放行，无任何确认
	ModeFullAuto
)

// 工具分类
const (
	CategoryReadonly = "readonly" // 只读工具
	CategoryEdit     = "edit"     // 编辑工具
	CategoryDanger   = "danger"   // 危险工具
	CategoryMCP      = "mcp"      // MCP 工具
)

// 工具分类表
var toolCategories = map[string]string{
	// 只读工具
	"read_file":   CategoryReadonly,
	"list_dir":    CategoryReadonly,
	"grep_search": CategoryReadonly,
	"glob_search": CategoryReadonly,
	"web_request": CategoryReadonly,
	"web_fetch":   CategoryReadonly,
	// 编辑工具
	"write_file": CategoryEdit,
	"edit_file":  CategoryEdit,
	// 危险工具
	"shell_execute": CategoryDanger,
}

// InterruptReason 中断原因
type InterruptReason string

const (
	InterruptToolConfirm   InterruptReason = "tool_confirm"
	InterruptDecisionPoint InterruptReason = "decision_point"
	InterruptBudgetExceed  InterruptReason = "budget_exceed"
	InterruptCustom        InterruptReason = "custom"
)

// InterruptPoint 中断点配置
type InterruptPoint struct {
	Type     InterruptReason
	ToolName string
	Message  string
}

// InterruptRequest 中断请求
type InterruptRequest struct {
	Reason  InterruptReason `json:"reason"`
	Message string          `json:"message"`
	Data    map[string]any  `json:"data,omitempty"`
	Turn    int             `json:"turn"`
}

// HumanResponse 人类响应
type HumanResponse struct {
	Approved bool           `json:"approved"`
	Input    string         `json:"input,omitempty"`
	Modified map[string]any `json:"modified,omitempty"`
}

// HITLConfig 人机协作配置
type HITLConfig struct {
	InterruptPoints  []InterruptPoint
	HumanInputChan   chan *HumanResponse
	OnInterrupt      func(req *InterruptRequest)
	AutoApproveTools []string
}

// Manager 权限管理器
type Manager struct {
	mode        ApprovalMode
	autoAllow   map[string]bool // 工具级白名单（mode 默认 + 用户运行时 always-allow）
	denyList    map[string]bool // 工具级黑名单
	userAllowed map[string]bool // 用户运行时通过 AddAutoAllow 显式加入的条目；
	// SetMode 时不会被 mode 默认集合覆盖清除
	mu          sync.RWMutex
}

// NewManager 根据模式创建权限管理器
func NewManager(mode ApprovalMode) *Manager {
	m := &Manager{
		mode:        mode,
		autoAllow:   make(map[string]bool),
		denyList:    make(map[string]bool),
		userAllowed: make(map[string]bool),
	}

	switch mode {
	case ModeSuggest:
		// 建议模式：无自动放行
	case ModeAutoEdit:
		// 自动编辑模式：只读和编辑类自动放行
		for tool, cat := range toolCategories {
			if cat == CategoryReadonly || cat == CategoryEdit {
				m.autoAllow[tool] = true
			}
		}
	case ModeFullAuto:
		// 全自动模式：所有已知工具自动放行
		for tool := range toolCategories {
			m.autoAllow[tool] = true
		}
	}

	return m
}

// Mode 返回当前审批模式
func (m *Manager) Mode() ApprovalMode {
	return m.mode
}

// ModeName 返回审批模式名称
func (m *Manager) ModeName() string {
	switch m.mode {
	case ModeSuggest:
		return "suggest"
	case ModeAutoEdit:
		return "auto-edit"
	case ModeFullAuto:
		return "full-auto"
	default:
		return "unknown"
	}
}

// ShouldApprove 判断该工具是否需要用户确认
func (m *Manager) ShouldApprove(toolName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.denyList[toolName] {
		return false
	}

	if m.autoAllow[toolName] {
		return false
	}

	switch m.mode {
	case ModeSuggest:
		return true
	case ModeAutoEdit:
		cat := GetToolCategory(toolName)
		return cat == CategoryDanger || cat == CategoryMCP
	case ModeFullAuto:
		return false
	default:
		return true
	}
}

// IsDenied 判断工具是否被禁止
func (m *Manager) IsDenied(toolName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.denyList[toolName]
}

// SetMode 切换审批模式，**保留** denyList 与 userAllowed（F-02 修复）。
// v0.2.0 之前的实现是 *m = *NewManager(mode)，会把 SafeMode 注入的
// 黑名单以及用户已 always-allow 的白名单全部清空，违背最小惊讶原则。
//
// 行为约定：
//   - m.mode 切换到新值
//   - 重新计算 mode 默认白名单（auto-edit: 只读+编辑；full-auto: 全部）
//   - 用户通过 AddAutoAllow 加入的条目（userAllowed）始终保留
//   - denyList 始终保留；denyList 永远胜出（覆盖 mode 默认白名单）
func (m *Manager) SetMode(mode ApprovalMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mode = mode

	// 重置 autoAllow = mode 默认 + 用户白名单
	m.autoAllow = make(map[string]bool)
	switch mode {
	case ModeSuggest:
		// 无默认白名单
	case ModeAutoEdit:
		for tool, cat := range toolCategories {
			if cat == CategoryReadonly || cat == CategoryEdit {
				m.autoAllow[tool] = true
			}
		}
	case ModeFullAuto:
		for tool := range toolCategories {
			m.autoAllow[tool] = true
		}
	}
	// 恢复用户白名单
	for tool := range m.userAllowed {
		m.autoAllow[tool] = true
	}
	// denyList 永远胜出
	for tool := range m.denyList {
		delete(m.autoAllow, tool)
	}
}

// SetModeByName 是 SetMode 的字符串入口，解析失败返回错误。
func (m *Manager) SetModeByName(name string) error {
	mode, err := ParseApprovalMode(name)
	if err != nil {
		return err
	}
	m.SetMode(mode)
	return nil
}

// AddAutoAllow 动态添加白名单（always-allow 功能）
// 同时记录到 userAllowed，SetMode 时不会因 mode 默认集合变化而被覆盖
func (m *Manager) AddAutoAllow(toolName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoAllow[toolName] = true
	m.userAllowed[toolName] = true
}

// AddDeny 添加黑名单
func (m *Manager) AddDeny(toolName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.denyList[toolName] = true
}

// BuildHITLConfig 生成 HITL 配置。
// F-11 状态：F-03 修好后，路径已经清晰 ——
//  - 框架的 CapabilityAgent.WithHITL(cfg HITLConfig) 接受 *agent.HITLConfig
//    （注意是框架自己的类型，不是 codecastcli 的 permission.HITLConfig）
//  - 当前的 buildPermHook 已经覆盖了相同语义（confirm + 4 选项 UI）
//  - 完整的 HITL 集成需要：把本函数改成构造 *agent.HITLConfig
//    并加 ap.WithHITL(...) 注入；本轮没有改这个，因为当前 buildPermHook
//    行为更直接，不绕 HITLManager 通道。
// 保留本函数供未来扩展（多步 plan / decision point）。
func (m *Manager) BuildHITLConfig(onInterrupt func(req *InterruptRequest)) HITLConfig {
	var interruptPoints []InterruptPoint

	switch m.mode {
	case ModeSuggest:
		interruptPoints = append(interruptPoints, InterruptPoint{
			Type:     InterruptToolConfirm,
			ToolName: "", // 空字符串 = 所有工具
		})
	case ModeAutoEdit:
		interruptPoints = append(interruptPoints, InterruptPoint{
			Type:     InterruptToolConfirm,
			ToolName: "", // 所有工具，通过 AutoApproveTools 过滤
		})
	case ModeFullAuto:
		// 全自动模式：无中断点
	}

	var autoApproveTools []string
	m.mu.RLock()
	for tool := range m.autoAllow {
		autoApproveTools = append(autoApproveTools, tool)
	}
	m.mu.RUnlock()

	humanInputCh := make(chan *HumanResponse, 8)

	cfg := HITLConfig{
		InterruptPoints:  interruptPoints,
		HumanInputChan:   humanInputCh,
		OnInterrupt:      onInterrupt,
		AutoApproveTools: autoApproveTools,
	}

	return cfg
}

// SendHumanResponse 通过 HITLConfig 发送人类响应
func SendHumanResponse(cfg HITLConfig, resp *HumanResponse) {
	cfg.HumanInputChan <- resp
}

// GetToolCategory 获取工具的分类
func GetToolCategory(toolName string) string {
	if cat, ok := toolCategories[toolName]; ok {
		return cat
	}
	if strings.HasPrefix(toolName, "mcp_") {
		return CategoryMCP
	}
	return CategoryMCP
}

// ParseApprovalMode 从字符串解析审批模式
func ParseApprovalMode(s string) (ApprovalMode, error) {
	switch strings.ToLower(s) {
	case "suggest":
		return ModeSuggest, nil
	case "auto-edit", "autoedit", "auto_edit":
		return ModeAutoEdit, nil
	case "full-auto", "fullauto", "full_auto":
		return ModeFullAuto, nil
	default:
		return ModeSuggest, fmt.Errorf("未知的审批模式: %s (可选: suggest, auto-edit, full-auto)", s)
	}
}

// NewManagerFromString 从字符串创建权限管理器
func NewManagerFromString(mode string) (*Manager, error) {
	m, err := ParseApprovalMode(mode)
	if err != nil {
		return nil, err
	}
	return NewManager(m), nil
}
