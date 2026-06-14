package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ChatMessage 表示聊天历史中的一条消息
type ChatMessage struct {
	Role      string // "user", "assistant", "tool"
	Content   string
	IsTool    bool
	Collapsed bool
}

// StreamEvent 表示流式响应中的一个事件，由 AgentRunner 产生
type StreamEvent struct {
	Type    string // "token", "tool_call", "tool_result", "complete", "error"
	Content string
	Detail  string // 工具名称等附加信息
}

// AgentRunner 定义 Agent 执行接口，由 CodecastAgentAdapter 实现
// Bubble Tea Model 通过此接口与 Agent 交互，不直接依赖具体实现
type AgentRunner interface {
	// StreamRun 以流式方式执行用户输入，返回事件通道
	// 调用方负责消费通道直到关闭
	StreamRun(ctx context.Context, userInput string) (<-chan StreamEvent, error)
}

// Bubble Tea 内部消息类型
type (
	// streamTokenMsg 流式 token 到达
	streamTokenMsg struct{ content string }
	// streamToolCallMsg 工具调用事件
	streamToolCallMsg struct {
		toolName string
		detail   string
	}
	// streamToolResultMsg 工具结果事件
	streamToolResultMsg struct{ content string }
	// streamCompleteMsg 流式响应完成
	streamCompleteMsg struct{ err error }
	// spinnerTickMsg spinner 定时器
	spinnerTickMsg struct{}
)

// Model 实现 tea.Model，提供 Bubble Tea 驱动的 TUI
type Model struct {
	messages     []ChatMessage
	viewport     viewport.Model
	textInput    textinput.Model
	spinner      spinner.Model
	statusBar    *StatusBar
	isProcessing bool
	err          error
	width        int
	height       int
	renderer     *Renderer
	ready        bool
	quit         bool

	// Agent 集成
	agent  AgentRunner
	ctx    context.Context
	cancel context.CancelFunc
	mu     *sync.Mutex // 指针避免值拷贝（tea.Model 接口按值传递）
	// 当前正在累积的 assistant 消息内容
	streamingContent strings.Builder

	// program 引用，用于在 goroutine 中通过 p.Send() 发送消息
	program *tea.Program
}

// 消息样式
var (
	userMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("63")).
			Padding(0, 1).
			MarginLeft(4).
			Align(lipgloss.Right)

	assistantMsgStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Padding(0, 1).
				MarginRight(4)

	toolMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("11")).
			Padding(0, 1).
			MarginRight(4)

	toolCollapsedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("11")).
				Padding(0, 1).
				MarginRight(4)

	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("57")).
			BorderBottom(true).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))
)

// NewModel 创建 Bubble Tea TUI 模型
func NewModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	ti := textinput.New()
	ti.Placeholder = "输入消息...（Shift+Enter 换行）"
	ti.Focus()
	ti.CharLimit = 0
	ti.Width = 50

	sb := NewStatusBar()

	ctx, cancel := context.WithCancel(context.Background())

	return Model{
		messages:  []ChatMessage{},
		viewport:  viewport.New(80, 20),
		textInput: ti,
		spinner:   s,
		statusBar: sb,
		renderer:  NewRenderer(),
		ready:     false,
		quit:      false,
		ctx:       ctx,
		cancel:    cancel,
		mu:        &sync.Mutex{},
	}
}

// SetAgent 设置 Agent 执行器
func (m *Model) SetAgent(agent AgentRunner) {
	m.agent = agent
}

// SetProgram 设置 tea.Program 引用，用于在 goroutine 中发送消息
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

// Init 实现 tea.Model.Init
func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, spinner.Tick)
}

// Update 实现 tea.Model.Update
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 4
		m.textInput.Width = msg.Width - 4
		if !m.ready {
			m.ready = true
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.isProcessing {
				// 中断当前请求
				m.cancel()
				m.isProcessing = false
				// 重建 context 供下次使用
				m.ctx, m.cancel = context.WithCancel(context.Background())
				return m, nil
			}
			m.quit = true
			return m, tea.Quit

		case tea.KeyEsc:
			if !m.isProcessing {
				m.quit = true
				return m, tea.Quit
			}

		case tea.KeyCtrlL:
			m.messages = nil
			m.viewport.SetContent("")
			return m, nil

		case tea.KeyEnter:
			if msg.Alt || msg.Paste {
				m.textInput.SetValue(m.textInput.Value() + "\n")
				return m, nil
			}
			return m.handleSubmit()

		case tea.KeyUp:
			if !m.textInput.Focused() {
				m.viewport.LineUp(1)
				return m, nil
			}

		case tea.KeyDown:
			if !m.textInput.Focused() {
				m.viewport.LineDown(1)
				return m, nil
			}

		case tea.KeyPgUp:
			m.viewport.HalfViewUp()
			return m, nil

		case tea.KeyPgDown:
			m.viewport.HalfViewDown()
			return m, nil
		}

		if msg.Type == tea.KeyCtrlJ {
			m.textInput.SetValue(m.textInput.Value() + "\n")
			return m, nil
		}

	// 流式 token 到达
	case streamTokenMsg:
		m.streamingContent.WriteString(msg.content)
		m.updateStreamingViewport()
		return m, nil

	// 工具调用事件
	case streamToolCallMsg:
		// 先把当前累积的 assistant 内容固化
		m.finalizeStreamingContent()
		m.messages = append(m.messages, ChatMessage{
			Role:      "tool",
			Content:   fmt.Sprintf("⚙ %s: %s", msg.toolName, msg.detail),
			IsTool:    true,
			Collapsed: true,
		})
		m.refreshViewport()
		return m, nil

	// 工具结果事件
	case streamToolResultMsg:
		m.messages = append(m.messages, ChatMessage{
			Role:      "tool",
			Content:   fmt.Sprintf("✓ %s", msg.content),
			IsTool:    true,
			Collapsed: true,
		})
		m.refreshViewport()
		return m, nil

	// 流式响应完成
	case streamCompleteMsg:
		m.finalizeStreamingContent()
		m.isProcessing = false
		if msg.err != nil {
			m.err = msg.err
		}
		m.refreshViewport()
		return m, nil

	case spinner.TickMsg:
		if m.isProcessing {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	m.textInput, cmd = m.textInput.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// handleSubmit 处理用户提交消息
func (m Model) handleSubmit() (tea.Model, tea.Cmd) {
	value := m.textInput.Value()
	if strings.TrimSpace(value) == "" {
		return m, nil
	}

	// 添加用户消息
	m.messages = append(m.messages, ChatMessage{
		Role:    "user",
		Content: value,
	})

	m.textInput.SetValue("")
	m.isProcessing = true
	m.err = nil
	m.streamingContent.Reset()

	m.refreshViewport()

	// 如果有 Agent，启动流式处理
	if m.agent != nil {
		return m, tea.Batch(spinner.Tick, m.startAgentStream(value))
	}

	// 无 Agent 时仅显示 spinner
	return m, spinner.Tick
}

// startAgentStream 启动 Agent 流式处理，返回一个 tea.Cmd。
// 该 Cmd 在 goroutine 中运行，通过 p.Send() 将中间事件发送给 Bubble Tea Update，
// 最终返回 streamCompleteMsg 作为 Cmd 的返回值。
func (m Model) startAgentStream(userInput string) tea.Cmd {
	return func() tea.Msg {
		ch, err := m.agent.StreamRun(m.ctx, userInput)
		if err != nil {
			return streamCompleteMsg{err: err}
		}

		for evt := range ch {
			switch evt.Type {
			case "token":
				// 通过 p.Send() 实时推送 token，实现流式输出
				if m.program != nil {
					m.program.Send(streamTokenMsg{content: evt.Content})
				}
			case "tool_call":
				if m.program != nil {
					m.program.Send(streamToolCallMsg{
						toolName: evt.Content,
						detail:   evt.Detail,
					})
				}
			case "tool_result":
				if m.program != nil {
					m.program.Send(streamToolResultMsg{content: evt.Content})
				}
			case "error":
				return streamCompleteMsg{err: fmt.Errorf("%s", evt.Content)}
			case "complete":
				// 流结束，由 for-range 退出后返回 streamCompleteMsg
			}
		}

		return streamCompleteMsg{}
	}
}

// updateStreamingViewport 更新流式输出中的 viewport（实时追加 token）
func (m *Model) updateStreamingViewport() {
	content := m.streamingContent.String()
	if content == "" {
		return
	}

	// 找到最后一条 assistant 消息并更新其内容
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
		m.messages[len(m.messages)-1].Content = content
	} else {
		// 新建 assistant 消息
		m.messages = append(m.messages, ChatMessage{
			Role:    "assistant",
			Content: content,
		})
	}

	m.refreshViewport()
}

// finalizeStreamingContent 将流式累积的内容固化到最后一条 assistant 消息
func (m *Model) finalizeStreamingContent() {
	content := m.streamingContent.String()
	if content == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
		m.messages[len(m.messages)-1].Content = content
	} else {
		m.messages = append(m.messages, ChatMessage{
			Role:    "assistant",
			Content: content,
		})
	}

	m.streamingContent.Reset()
}

// refreshViewport 从 messages 重建 viewport 内容
func (m *Model) refreshViewport() {
	var sb strings.Builder

	for i, msg := range m.messages {
		switch {
		case msg.IsTool:
			sb.WriteString(m.renderToolMessage(msg))
		case msg.Role == "user":
			sb.WriteString(m.renderUserMessage(msg))
		case msg.Role == "assistant":
			sb.WriteString(m.renderAssistantMessage(msg))
		}
		if i < len(m.messages)-1 {
			sb.WriteString("\n")
		}
	}

	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

// renderUserMessage 渲染用户消息（右对齐蓝色）
func (m *Model) renderUserMessage(msg ChatMessage) string {
	content := msg.Content
	rendered := userMsgStyle.Render(content)
	return alignRight(rendered, m.width)
}

// renderAssistantMessage 渲染助手消息（Markdown）
func (m *Model) renderAssistantMessage(msg ChatMessage) string {
	rendered := m.renderer.RenderMarkdown(msg.Content)
	return assistantMsgStyle.Render(rendered)
}

// renderToolMessage 渲染工具调用消息（可折叠）
func (m *Model) renderToolMessage(msg ChatMessage) string {
	if msg.Collapsed {
		return toolCollapsedStyle.Render("▶ [tool] " + truncate(msg.Content, 60))
	}
	return toolMsgStyle.Render("▼ [tool]\n" + msg.Content)
}

// ToggleCollapse 切换工具消息的折叠状态
func (m *Model) ToggleCollapse(index int) {
	if index >= 0 && index < len(m.messages) && m.messages[index].IsTool {
		m.messages[index].Collapsed = !m.messages[index].Collapsed
		m.refreshViewport()
	}
}

// AddMessage 添加消息并刷新 viewport
func (m *Model) AddMessage(role, content string, isTool bool) {
	m.messages = append(m.messages, ChatMessage{
		Role:      role,
		Content:   content,
		IsTool:    isTool,
		Collapsed: isTool,
	})
	m.refreshViewport()
}

// SetProcessing 设置处理状态
func (m *Model) SetProcessing(processing bool) {
	m.isProcessing = processing
}

// SetError 设置错误信息
func (m *Model) SetError(err error) {
	m.err = err
}

// View 实现 tea.Model.View
func (m Model) View() string {
	if !m.ready {
		return "初始化中..."
	}

	statusBar := m.statusBar.Render(m.width)
	viewportView := m.viewport.View()
	inputView := inputStyle.Render(m.textInput.View())

	var processingLine string
	if m.isProcessing {
		processingLine = fmt.Sprintf("\n%s 思考中...", m.spinner.View())
	}

	var errorLine string
	if m.err != nil {
		errorLine = "\n" + errorStyle.Render("错误: "+m.err.Error())
	}

	helpLine := helpStyle.Render("Enter: 发送 | Shift+Enter/Ctrl+J: 换行 | Ctrl+C: 中断 | Ctrl+L: 清屏 | Esc: 退出")

	return statusBar + "\n" + viewportView + processingLine + errorLine + "\n" + inputView + "\n" + helpLine
}

// Run 创建并启动 Bubble Tea 程序
func Run() error {
	p := tea.NewProgram(
		NewModel(),
		tea.WithAltScreen(),
	)
	_, err := p.Run()
	return err
}

// RunWithAgent 创建带 Agent 的 Bubble Tea 程序
// agent 参数实现 AgentRunner 接口，用于流式执行用户输入
func RunWithAgent(runner AgentRunner) error {
	m := NewModel()
	m.SetAgent(runner)

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
	)
	m.SetProgram(p)

	_, err := p.Run()
	return err
}

// RunWithAgentAndConfig 创建带 Agent 和配置的 Bubble Tea 程序。
// 从 cfg 设置状态栏信息（模型名、权限模式等）。
// 注意：为避免循环依赖，CodecastAgentAdapter 在 agent 包中定义，
// 调用方应使用 agent.NewTUIAdapter(ag) 创建 AgentRunner。
func RunWithAgentAndConfig(runner AgentRunner, model, permissionMode string) error {
	m := NewModel()
	m.SetAgent(runner)

	// 设置状态栏信息
	m.statusBar.Model = model
	m.statusBar.Mode = permissionMode

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
	)
	m.SetProgram(p)

	_, err := p.Run()
	return err
}

// RunWithModel 创建并启动带自定义 Model 的 Bubble Tea 程序
func RunWithModel(m Model) error {
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
	)
	_, err := p.Run()
	return err
}

// alignRight 右对齐字符串
func alignRight(s string, width int) string {
	lines := strings.Split(s, "\n")
	var sb strings.Builder
	for i, line := range lines {
		visibleLen := lipgloss.Width(line)
		padding := width - visibleLen
		if padding > 0 {
			sb.WriteString(strings.Repeat(" ", padding))
		}
		sb.WriteString(line)
		if i < len(lines)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
