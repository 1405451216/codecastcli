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
	"read_file":    CategoryReadonly,
	"list_files":   CategoryReadonly,
	"grep_search":  CategoryReadonly,
	"glob_search":  CategoryReadonly,
	"web_request":  CategoryReadonly,
	"web_fetch":    CategoryReadonly,
	"lsp":          CategoryReadonly,
	// 编辑工具
	"write_file": CategoryEdit,
	"edit_file":  CategoryEdit,
	"multi_edit": CategoryEdit,
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

// HITLManagerWrapper 封装 HITL 通道管理，提供线程安全的请求/响应交互。
// 交互层（interactive.go）通过 PendingRequest 读取挂起的中断请求，
// 通过 SendResponse 发回人类响应。
type HITLManagerWrapper struct {
	responseCh chan *HumanResponse
	pending    *InterruptRequest
	mu         sync.RWMutex
}

// PendingRequest 返回当前挂起的中断请求（nil 表示无挂起请求）
func (w *HITLManagerWrapper) PendingRequest() *InterruptRequest {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.pending
}

// SendResponse 发送人类响应，解除 HITL 阻塞
func (w *HITLManagerWrapper) SendResponse(resp *HumanResponse) {
	w.responseCh <- resp
}

// Manager 权限管理器
type Manager struct {
	mode        ApprovalMode
	autoAllow   map[string]bool // 工具级白名单（mode 默认 + 用户运行时 always-allow）
	denyList    map[string]bool // 工具级黑名单
	userAllowed map[string]bool // 用户运行时通过 AddAutoAllow 显式加入的条目；
	// SetMode 时不会被 mode 默认集合覆盖清除
	mu          sync.RWMutex
	// HITL 集成：管理器持有 HITLConfig 和响应通道，
	// 供 OnInterrupt 回调写入请求、交互层读取并回复。
	hitlConfig    HITLConfig
	hitlMgr       *HITLManagerWrapper
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
//   - 重建 HITL 配置以反映新模式
func (m *Manager) SetMode(mode ApprovalMode) {
	m.mu.Lock()
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
	m.mu.Unlock()

	// 重建 HITL 配置（在锁外执行，BuildHITLConfig 内部会加锁）
	m.rebuildHITLConfig()
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

// BuildHITLConfig 生成 HITL 配置，同时初始化 Manager 内部的 HITLManagerWrapper。
//
// 工作原理：
//   - ModeSuggest: 所有工具需确认（InterruptPoint.ToolName="" 匹配所有工具），
//     AutoApproveTools 为空
//   - ModeAutoEdit: 所有工具设为中断点，但只读/编辑类工具加入 AutoApproveTools
//     跳过确认，仅危险工具和 MCP 工具需确认
//   - ModeFullAuto: 无中断点，所有工具自动放行
//
// OnInterrupt 回调将请求存入 HITLManagerWrapper.pending，
// 交互层通过 m.HitlManager().PendingRequest() 读取并渲染确认提示，
// 用户响应后通过 m.HitlManager().SendResponse() 发回。
func (m *Manager) BuildHITLConfig() HITLConfig {
	var interruptPoints []InterruptPoint

	switch m.mode {
	case ModeSuggest:
		// 所有工具需确认
		interruptPoints = append(interruptPoints, InterruptPoint{
			Type:     InterruptToolConfirm,
			ToolName: "", // 空字符串 = 所有工具
		})
	case ModeAutoEdit:
		// 所有工具设为中断点，通过 AutoApproveTools 过滤安全的工具
		interruptPoints = append(interruptPoints, InterruptPoint{
			Type:     InterruptToolConfirm,
			ToolName: "",
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

	responseCh := make(chan *HumanResponse, 8)

	wrapper := &HITLManagerWrapper{
		responseCh: responseCh,
	}

	// R5-C6 修复：写入 hitlMgr 需要写锁保护
	m.mu.Lock()
	m.hitlMgr = wrapper
	m.mu.Unlock()

	onInterrupt := func(req *InterruptRequest) {
		wrapper.mu.Lock()
		wrapper.pending = req
		wrapper.mu.Unlock()
	}

	cfg := HITLConfig{
		InterruptPoints:  interruptPoints,
		HumanInputChan:   responseCh,
		OnInterrupt:      onInterrupt,
		AutoApproveTools: autoApproveTools,
	}

	// R5-C6 修复：写入 hitlConfig 需要写锁保护
	m.mu.Lock()
	m.hitlConfig = cfg
	m.mu.Unlock()
	return cfg
}

// HitlManager 返回 HITL 管理器包装，供交互层读取挂起请求和发送响应。
// 必须在 BuildHITLConfig() 之后调用，否则返回 nil。
func (m *Manager) HitlManager() *HITLManagerWrapper {
	return m.hitlMgr
}

// HitlConfig 返回当前 HITL 配置
func (m *Manager) HitlConfig() HITLConfig {
	return m.hitlConfig
}

// ShouldInterrupt 判断工具是否需要 HITL 中断确认。
// 语义与 AP 框架的 HITLManager.ShouldInterrupt 一致：
//   - 工具在 AutoApproveTools 中 → 不中断
//   - 工具匹配 InterruptPoint（空 ToolName 匹配所有）→ 需中断
//   - 无匹配的 InterruptPoint → 不中断
func (m *Manager) ShouldInterrupt(toolName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 被禁止的工具不需要中断（直接拒绝）
	if m.denyList[toolName] {
		return false
	}

	// 自动放行的工具不需要中断
	if m.autoAllow[toolName] {
		return false
	}

	for _, ip := range m.hitlConfig.InterruptPoints {
		if ip.Type != InterruptToolConfirm {
			continue
		}
		if ip.ToolName == "" || ip.ToolName == toolName {
			return true
		}
	}
	return false
}

// rebuildHITLConfig 内部方法：重建 HITL 配置，保留已有的 responseCh 和 wrapper。
// SetMode 调用此方法而非 BuildHITLConfig，避免创建新通道导致响应丢失。
func (m *Manager) rebuildHITLConfig() {
	var interruptPoints []InterruptPoint

	switch m.mode {
	case ModeSuggest:
		interruptPoints = append(interruptPoints, InterruptPoint{
			Type:     InterruptToolConfirm,
			ToolName: "",
		})
	case ModeAutoEdit:
		interruptPoints = append(interruptPoints, InterruptPoint{
			Type:     InterruptToolConfirm,
			ToolName: "",
		})
	case ModeFullAuto:
	}

	var autoApproveTools []string
	m.mu.RLock()
	for tool := range m.autoAllow {
		autoApproveTools = append(autoApproveTools, tool)
	}
	m.mu.RUnlock()

	// 复用已有的 wrapper 和 responseCh
	if m.hitlMgr == nil {
		// 首次构建，走 BuildHITLConfig
		m.BuildHITLConfig()
		return
	}

	onInterrupt := func(req *InterruptRequest) {
		m.hitlMgr.mu.Lock()
		m.hitlMgr.pending = req
		m.hitlMgr.mu.Unlock()
	}

	// R5-C6 修复：写入 hitlConfig 需要写锁保护
	m.mu.Lock()
	m.hitlConfig = HITLConfig{
		InterruptPoints:  interruptPoints,
		HumanInputChan:   m.hitlMgr.responseCh,
		OnInterrupt:      onInterrupt,
		AutoApproveTools: autoApproveTools,
	}
	m.mu.Unlock()
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
	// MAINT-22 修复：未知工具默认归为危险类，保守处理
	return CategoryDanger
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
