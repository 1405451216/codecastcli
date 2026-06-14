package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// StatusBar 状态栏组件
type StatusBar struct {
	Model      string
	TokenCount int
	Budget     float64
	Mode       string
}

// statusBarStyle 状态栏样式
var statusBarStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("57")).
	Foreground(lipgloss.Color("230")).
	Padding(0, 1)

// statusBarDimStyle 状态栏次要信息样式
var statusBarDimStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("57")).
	Foreground(lipgloss.Color("189")).
	Padding(0, 1)

// NewStatusBar 创建状态栏
func NewStatusBar() *StatusBar {
	return &StatusBar{
		Model:      "unknown",
		TokenCount: 0,
		Budget:     0.0,
		Mode:       "auto-edit",
	}
}

// UpdateModel 更新模型名称
func (s *StatusBar) UpdateModel(model string) {
	s.Model = model
}

// UpdateTokens 更新 token 计数
func (s *StatusBar) UpdateTokens(count int) {
	s.TokenCount = count
}

// UpdateBudget 更新预算
func (s *StatusBar) UpdateBudget(cost float64) {
	s.Budget = cost
}

// UpdateMode 更新模式
func (s *StatusBar) UpdateMode(mode string) {
	s.Mode = mode
}

// Render 渲染状态栏，适配指定宽度
func (s *StatusBar) Render(width int) string {
	content := fmt.Sprintf(" model=%s | tokens=%s | budget=$%.2f | mode=%s ",
		s.Model,
		formatTokenCount(s.TokenCount),
		s.Budget,
		s.Mode,
	)

	// 截断或填充到指定宽度
	if len(content) > width {
		content = content[:width]
	} else if len(content) < width {
		content += strings.Repeat(" ", width-len(content))
	}

	return statusBarStyle.Render(content)
}
