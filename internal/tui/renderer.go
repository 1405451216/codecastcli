package tui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// Renderer 终端 Markdown 渲染器
type Renderer struct {
	renderer *glamour.TermRenderer
	mu       sync.Mutex
	width    int
}

// NewRenderer 创建 Markdown 渲染器
func NewRenderer() *Renderer {
	width := getTerminalWidth()
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	return &Renderer{
		renderer: r,
		width:    width,
	}
}

// RenderMarkdown 渲染 Markdown 文本
func (r *Renderer) RenderMarkdown(text string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	rendered, err := r.renderer.Render(text)
	if err != nil {
		return text // 回退到原始文本
	}
	return rendered
}

// RenderCodeBlock 渲染代码块（带语法高亮标签）
func (r *Renderer) RenderCodeBlock(code, language string) string {
	fenced := fmt.Sprintf("```%s\n%s\n```", language, code)
	return r.RenderMarkdown(fenced)
}

// RenderInlineCode 渲染行内代码
func (r *Renderer) RenderInlineCode(code string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("59")).
		Padding(0, 1).
		Render(code)
}

// RenderDiff 渲染 Diff 内容（红绿对比）
func (r *Renderer) RenderDiff(diffText string) string {
	var sb strings.Builder
	lines := strings.Split(diffText, "\n")

	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // 绿色
	delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))  // 红色
	hunkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // 青色
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // 灰色

	for _, line := range lines {
		if len(line) == 0 {
			sb.WriteString("\n")
			continue
		}
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			sb.WriteString(infoStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "@@"):
			sb.WriteString(hunkStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "+"):
			sb.WriteString(addStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "-"):
			sb.WriteString(delStyle.Render(line) + "\n")
		default:
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

// StreamPrinter 流式打印器（打字机效果 + F3: 实时 Markdown 渲染）
type StreamPrinter struct {
	renderer *Renderer
	buffer   strings.Builder
	writer   io.Writer
	mu       sync.Mutex
	// F3: 实时渲染状态
	lastRenderLen int  // 上次渲染的字符数
	liveRender    bool // 是否启用实时渲染
	tokenCount    int  // 累计 token 数（用于防抖）
}

// NewStreamPrinter 创建流式打印器
func NewStreamPrinter() *StreamPrinter {
	return &StreamPrinter{
		renderer:   NewRenderer(),
		writer:     os.Stdout,
		liveRender: true, // F3: 默认启用实时渲染
	}
}

// WriteToken 写入一个 token（F3: 实时 Markdown 渲染，带防抖）
func (s *StreamPrinter) WriteToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buffer.WriteString(token)
	s.tokenCount++

	if s.liveRender {
		// 防抖：每 5 个 token 或遇到换行时才重新渲染，避免频繁闪烁
		shouldRender := s.tokenCount%5 == 0 || strings.Contains(token, "\n")
		if !shouldRender {
			// 直接输出原始 token（不渲染），减少闪烁
			fmt.Fprint(s.writer, token)
			return
		}

		// 实时渲染：渲染当前缓冲区
		content := s.buffer.String()
		rendered := s.renderer.RenderMarkdown(content)

		// 清除上次渲染的内容并重新渲染
		if s.lastRenderLen > 0 {
			fmt.Fprint(s.writer, "\r\033[J")
		}

		// 去掉末尾多余换行（glamour 会加换行）
		rendered = strings.TrimRight(rendered, "\n")
		fmt.Fprint(s.writer, rendered)
		s.lastRenderLen = len(rendered)
	} else {
		// 原始模式：直接输出 token
		fmt.Fprint(s.writer, token)
	}
}

// Flush 刷新缓冲区，渲染最终 Markdown
func (s *StreamPrinter) Flush() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	content := s.buffer.String()
	if content == "" {
		return ""
	}

	rendered := s.renderer.RenderMarkdown(content)
	fmt.Fprint(s.writer, rendered)
	s.buffer.Reset()
	return rendered
}

// Reset 重置缓冲区
func (s *StreamPrinter) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer.Reset()
}

// Spinner 进度指示器
type Spinner struct {
	frames  []string
	current int
	message string
	active  bool
	mu      sync.Mutex
}

// NewSpinner 创建进度指示器
func NewSpinner(message string) *Spinner {
	return &Spinner{
		frames:  []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		message: message,
		active:  false,
	}
}

// Start 启动进度指示器
func (s *Spinner) Start() {
	s.mu.Lock()
	s.active = true
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			s.mu.Lock()
			if !s.active {
				s.mu.Unlock()
				return
			}
			frame := s.frames[s.current%len(s.frames)]
			s.current++
			msg := s.message
			s.mu.Unlock()

			fmt.Printf("\r%s %s", frame, msg)
		}
	}()
}

// Stop 停止进度指示器
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = false
	fmt.Printf("\r%s\r", strings.Repeat(" ", len(s.message)+4))
}

// UpdateMessage 更新消息
func (s *Spinner) UpdateMessage(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = msg
}

// Styles 预定义样式
var Styles = struct {
	Header    lipgloss.Style
	SubHeader lipgloss.Style
	Success   lipgloss.Style
	Warning   lipgloss.Style
	Error     lipgloss.Style
	Info      lipgloss.Style
	Dim       lipgloss.Style
	Bold      lipgloss.Style
}{
	Header: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")),
	SubHeader: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")),
	Success: lipgloss.NewStyle().
		Foreground(lipgloss.Color("2")),
	Warning: lipgloss.NewStyle().
		Foreground(lipgloss.Color("3")),
	Error: lipgloss.NewStyle().
		Foreground(lipgloss.Color("1")),
	Info: lipgloss.NewStyle().
		Foreground(lipgloss.Color("4")),
	Dim: lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")),
	Bold: lipgloss.NewStyle().
		Bold(true),
}

// PrintHeader 打印标题
func PrintHeader(text string) {
	fmt.Println(Styles.Header.Render(text))
}

// PrintSuccess 打印成功消息
func PrintSuccess(text string) {
	fmt.Println(Styles.Success.Render("✓ " + text))
}

// PrintWarning 打印警告消息
func PrintWarning(text string) {
	fmt.Println(Styles.Warning.Render("⚠ " + text))
}

// PrintError 打印错误消息
func PrintError(text string) {
	fmt.Println(Styles.Error.Render("✗ " + text))
}

// PrintInfo 打印信息
func PrintInfo(text string) {
	fmt.Println(Styles.Info.Render("ℹ " + text))
}

// PrintDim 打印灰色文本
func PrintDim(text string) {
	fmt.Println(Styles.Dim.Render(text))
}

// getTerminalWidth 获取终端宽度
func getTerminalWidth() int {
	width, _, err := termSize()
	if err != nil || width <= 0 {
		return 80
	}
	return width - 4 // 留边距
}

// termSize 获取终端尺寸
func termSize() (int, int, error) {
	// 使用 os 包获取
	return 80, 24, nil
}
