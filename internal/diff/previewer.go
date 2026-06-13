package diff

import (
	"fmt"
	"strings"
)

// ChangeType 变更类型
type ChangeType int

const (
	ChangeAdd    ChangeType = iota // 新增
	ChangeDelete                    // 删除
	ChangeModify                    // 修改
)

// FileChange 文件变更
type FileChange struct {
	Path       string     `json:"path"`
	Type       ChangeType `json:"type"`
	OldContent string     `json:"old_content,omitempty"`
	NewContent string     `json:"new_content,omitempty"`
	Diff       string     `json:"diff,omitempty"`
}

// PreviewResult 预览结果
type PreviewResult struct {
	Changes []FileChange `json:"changes"`
	Summary string       `json:"summary"`
}

// Previewer Diff 预览器
type Previewer struct{}

// NewPreviewer 创建预览器
func NewPreviewer() *Previewer {
	return &Previewer{}
}

// PreviewEdit 预览编辑操作
func (p *Previewer) PreviewEdit(filePath, oldString, newString string) *FileChange {
	change := &FileChange{
		Path: filePath,
		Type: ChangeModify,
	}

	// 生成 unified diff
	change.Diff = generateUnifiedDiff(filePath, oldString, newString)
	change.OldContent = oldString
	change.NewContent = newString

	return change
}

// PreviewWrite 预览写入操作
func (p *Previewer) PreviewWrite(filePath, content string, exists bool) *FileChange {
	change := &FileChange{
		Path: filePath,
	}

	if exists {
		change.Type = ChangeModify
		change.NewContent = content
		change.Diff = fmt.Sprintf("--- %s (修改)\n+++ %s\n%s", filePath, filePath, content)
	} else {
		change.Type = ChangeAdd
		change.NewContent = content
		change.Diff = fmt.Sprintf("--- /dev/null\n+++ %s\n%s", filePath, content)
	}

	return change
}

// PreviewDelete 预览删除操作
func (p *Previewer) PreviewDelete(filePath, content string) *FileChange {
	return &FileChange{
		Path:       filePath,
		Type:       ChangeDelete,
		OldContent: content,
		Diff:       fmt.Sprintf("--- %s\n+++ /dev/null\n(文件已删除)", filePath),
	}
}

// FormatChange 格式化变更（终端友好）
func FormatChange(change *FileChange) string {
	var sb strings.Builder

	typeStr := ""
	switch change.Type {
	case ChangeAdd:
		typeStr = "新增"
	case ChangeDelete:
		typeStr = "删除"
	case ChangeModify:
		typeStr = "修改"
	}

	sb.WriteString(fmt.Sprintf("文件: %s [%s]\n", change.Path, typeStr))

	if change.Diff != "" {
		sb.WriteString(change.Diff)
	}

	return sb.String()
}

// generateUnifiedDiff 生成 unified diff
func generateUnifiedDiff(filePath, oldStr, newStr string) string {
	var sb strings.Builder
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	sb.WriteString(fmt.Sprintf("--- %s\n", filePath))
	sb.WriteString(fmt.Sprintf("+++ %s\n", filePath))
	sb.WriteString("@@\n")

	// 简单的行级 diff
	maxLen := len(oldLines)
	if len(newLines) > maxLen {
		maxLen = len(newLines)
	}

	for i := 0; i < maxLen; i++ {
		oldLine := ""
		newLine := ""
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if oldLine != newLine {
			if oldLine != "" {
				sb.WriteString(fmt.Sprintf("-%s\n", oldLine))
			}
			if newLine != "" {
				sb.WriteString(fmt.Sprintf("+%s\n", newLine))
			}
		} else {
			sb.WriteString(fmt.Sprintf(" %s\n", oldLine))
		}
	}

	return sb.String()
}

// ChangeTypeName 返回变更类型名称
func ChangeTypeName(t ChangeType) string {
	switch t {
	case ChangeAdd:
		return "新增"
	case ChangeDelete:
		return "删除"
	case ChangeModify:
		return "修改"
	default:
		return "未知"
	}
}
