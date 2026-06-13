package ui

import (
	"fmt"

	"github.com/charmbracelet/glamour"
)

var glamourRenderer *glamour.TermRenderer

func init() {
	var err error
	glamourRenderer, err = glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(120),
	)
	if err != nil {
		// glamour 初始化失败时设为 nil，RenderMarkdown 会回退到纯文本
		glamourRenderer = nil
	}
}

// RenderMarkdown 使用 glamour 渲染 Markdown 内容
// 如果渲染失败，回退到纯文本输出
func RenderMarkdown(content string) string {
	if glamourRenderer == nil {
		return content
	}

	rendered, err := glamourRenderer.Render(content)
	if err != nil {
		// 渲染失败，回退到纯文本
		return content
	}
	return rendered
}

// PrintMarkdown 渲染并打印 Markdown 内容
func PrintMarkdown(content string) {
	rendered := RenderMarkdown(content)
	fmt.Print(rendered)
}
