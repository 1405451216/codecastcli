// F-01 (IMPROVEMENT-PLAN Task 1.1) — multi_edit 工具：原子批量编辑多个文件。
//
// 设计要点：
//   - 4 步流程：预检 → 全量预检失败 → 原子写入 → 任一失败回滚
//   - 复用 edit.go 的 buildDiffSummary、tolerantNormalize、findClosestMatch
//   - 错误信息包含 edit 索引（方便 LLM 定位失败的 edit）
//   - 去重：相同 (file_path, old_string) 只读一次（但允许多个 edit 顺序应用到同一文件）
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/util"
)

// MultiEditTool 一次编辑多个文件，原子提交（全部成功或全部回滚）
type MultiEditTool struct{}

// NewMultiEditTool 创建 MultiEditTool 实例
func NewMultiEditTool() *MultiEditTool {
	return &MultiEditTool{}
}

// Name 返回工具名称
func (t *MultiEditTool) Name() string {
	return "multi_edit"
}

// Description 返回工具描述
func (t *MultiEditTool) Description() string {
	return "一次修改多个文件，原子提交（全部成功或全部回滚）。"
}

// multiEditParams 定义 multi_edit 工具的参数
type multiEditParams struct {
	Edits []editOperation `json:"edits"`
}

// editOperation 单个编辑操作的参数
type editOperation struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

// Parameters 返回工具参数的 JSON Schema
func (t *MultiEditTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"edits": {
				"type": "array",
				"description": "编辑操作列表，按顺序处理",
				"items": {
					"type": "object",
					"properties": {
						"file_path": {
							"type": "string",
							"description": "要编辑的文件路径"
						},
						"old_string": {
							"type": "string",
							"description": "要被替换的原始文本（必须唯一匹配，除非 replace_all=true）"
						},
						"new_string": {
							"type": "string",
							"description": "替换后的新文本"
						},
						"replace_all": {
							"type": "boolean",
							"description": "是否替换所有匹配项",
							"default": false
						}
					},
					"required": ["file_path", "old_string", "new_string"]
				}
			}
		},
		"required": ["edits"]
	}`)
}

// planEntry 是预检阶段的计划条目：
// 记录每个 edit 的最终 oldString（已通过容差匹配校正），
// 以及该文件最新内容、所有已应用 edit 的累计新内容。
type planEntry struct {
	edit         editOperation
	resolvedOld  string // 容差匹配后，在原文中的精确子串
	original     string // 该文件原始内容
	newContent   string // 该文件最终新内容（已应用本文件所有 edits）
	originalMode os.FileMode
	matchCount   int
}

// Execute 执行 multi_edit 工具
func (t *MultiEditTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
	var params multiEditParams
	if err := json.Unmarshal(args, &params); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("参数解析失败: %v", err)), nil
	}

	if len(params.Edits) == 0 {
		return ap.NewToolErrorResult("edits 数组不能为空"), nil
	}

	// Step 1+2：预检所有 edit，构建每文件的最新内容计划
	plan, err := t.preflight(params.Edits)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("预检失败: %s", err.Error())), nil
	}

	// Step 3+4：原子写入；任一 rename 失败时回滚已写入的文件
	if err := t.atomicWrite(plan); err != nil {
		return ap.NewToolErrorResult(err.Error()), nil
	}

	// 成功：生成汇总报告
	return ap.NewToolResult(buildMultiEditSummary(plan)), nil
}

// preflight 预检所有 edit，构造 plan。
// 预检失败时返回包含 edit 索引的明确错误。
// 顺序：保持用户传入的顺序；同文件多次 edit 顺序累积应用。
func (t *MultiEditTool) preflight(edits []editOperation) ([]planEntry, error) {
	// 按 file_path 顺序累积 edit 计划
	// key = 规范化后的绝对路径
	order := []string{}
	files := map[string]*planEntry{}

	for i, edit := range edits {
		if edit.FilePath == "" {
			return nil, fmt.Errorf("edit[%d] (file=%s): file_path 不能为空", i, edit.FilePath)
		}
		if edit.OldString == "" {
			return nil, fmt.Errorf("edit[%d] (file=%s): old_string 不能为空", i, edit.FilePath)
		}

		absPath, err := filepath.Abs(edit.FilePath)
		if err != nil {
			return nil, fmt.Errorf("edit[%d] (file=%s): 路径解析失败: %v", i, edit.FilePath, err)
		}

		entry, exists := files[absPath]
		if !exists {
			// 第一次遇到此文件：读取并初始化
			info, err := os.Stat(absPath)
			if err != nil {
				return nil, fmt.Errorf("edit[%d] (file=%s): 获取文件信息失败: %v", i, edit.FilePath, err)
			}
			content, err := os.ReadFile(absPath)
			if err != nil {
				return nil, fmt.Errorf("edit[%d] (file=%s): 读取文件失败: %v", i, edit.FilePath, err)
			}
			entry = &planEntry{
				edit:         edit,
				originalMode: info.Mode(),
				original:     string(content),
				newContent:   string(content),
			}
			files[absPath] = entry
			order = append(order, absPath)
		}

		// 校验 oldString 唯一性
		current := entry.newContent
		count := strings.Count(current, edit.OldString)
		resolvedOld := edit.OldString

		// 容差匹配（仅在严格匹配为 0 时启用）
		if count == 0 {
			tolerantCurrent := tolerantNormalize(current)
			tolerantOld := tolerantNormalize(edit.OldString)
			if tolerantCount := strings.Count(tolerantCurrent, tolerantOld); tolerantCount > 0 {
				resolvedOld = findClosestMatch(current, edit.OldString)
				if resolvedOld == "" {
					return nil, fmt.Errorf("edit[%d] (file=%s): 容差匹配失败: %q", i, edit.FilePath, util.Truncate(edit.OldString, 50))
				}
				count = tolerantCount
			}
		}

		if count == 0 {
			return nil, fmt.Errorf("edit[%d] (file=%s): 未找到匹配 %q", i, edit.FilePath, util.Truncate(edit.OldString, 50))
		}

		if count > 1 && !edit.ReplaceAll {
			return nil, fmt.Errorf(
				"edit[%d] (file=%s): 找到 %d 处匹配，请提供更多上下文或设置 replace_all 为 true",
				i, edit.FilePath, count,
			)
		}

		// 应用替换到该文件的累计 newContent
		if edit.ReplaceAll {
			entry.newContent = strings.ReplaceAll(entry.newContent, resolvedOld, edit.NewString)
		} else {
			entry.newContent = strings.Replace(entry.newContent, resolvedOld, edit.NewString, 1)
		}
		entry.matchCount = count
	}

	// 转为切片，按用户首次访问的顺序
	result := make([]planEntry, 0, len(order))
	for _, k := range order {
		result = append(result, *files[k])
	}
	return result, nil
}

// atomicWrite 原子写入所有 plan 中的文件。
// 任何 rename 失败时回滚已成功写入的文件。
func (t *MultiEditTool) atomicWrite(plan []planEntry) error {
	// 跟踪已成功 rename 的文件，以便失败时回滚
	// key = abs path, value = 原始内容（用于回滚）
	rolled := map[string][]byte{}

	// 记录所有产生的临时文件，失败时清理
	tmpFiles := []string{}

	// 标记是否整体成功
	committed := false
	defer func() {
		if committed {
			return
		}
		// 回滚：恢复已写入的文件
		for path, orig := range rolled {
			if err := os.WriteFile(path, orig, 0644); err != nil {
				// 回滚失败：记录但不掩盖原错误
				fmt.Fprintf(os.Stderr, "严重: 回滚文件 %s 失败: %v\n", path, err)
			}
		}
		// 清理所有临时文件
		for _, tmp := range tmpFiles {
			os.Remove(tmp)
		}
	}()

	// Step 1：写所有临时文件
	for _, p := range plan {
		// abs path —— planEntry 没有保存，但 edit.FilePath 在执行期间不变
		// 我们用 edit.FilePath 作为标识符
		tmpFile := p.edit.FilePath + ".tmp"
		tmpFiles = append(tmpFiles, tmpFile)
		if err := os.WriteFile(tmpFile, []byte(p.newContent), p.originalMode); err != nil {
			return fmt.Errorf("写入失败: %s: %v", p.edit.FilePath, err)
		}
	}

	// Step 2：依次 rename。每次成功则记入 rolled。
	// 注意：必须在 rename 成功前读取原始内容用于回滚。
	for _, p := range plan {
		tmpFile := p.edit.FilePath + ".tmp"
		// 读取当前原文（rename 前），用于回滚
		origBytes, readErr := os.ReadFile(p.edit.FilePath)
		if readErr != nil {
			return fmt.Errorf("写入失败: 读取原文用于回滚 %s: %v", p.edit.FilePath, readErr)
		}
		if err := os.Rename(tmpFile, p.edit.FilePath); err != nil {
			return fmt.Errorf("写入失败: 重命名 %s: %v", p.edit.FilePath, err)
		}
		rolled[p.edit.FilePath] = origBytes
	}

	committed = true
	return nil
}

// buildMultiEditSummary 生成 multi_edit 成功执行的汇总报告
func buildMultiEditSummary(plan []planEntry) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "multi_edit 成功：共修改 %d 个文件\n", len(plan))
	for i, p := range plan {
		fmt.Fprintf(&sb, "\n[%d] %s\n", i+1, p.edit.FilePath)
		// 行级 diff 摘要
		replaceInfo := "（替换了 1 处匹配）"
		if p.edit.ReplaceAll && p.matchCount > 1 {
			replaceInfo = fmt.Sprintf("（替换了全部 %d 处匹配）", p.matchCount)
		}
		fmt.Fprintf(&sb, "变更:%s\n", replaceInfo)
		diffSummary := buildDiffSummary(p.original, p.newContent, p.edit.FilePath, p.matchCount, p.edit.ReplaceAll)
		// buildDiffSummary 第一行 "文件 xxx 已更新" 已包含文件名，去掉它避免重复
		lines := strings.Split(diffSummary, "\n")
		if len(lines) > 1 {
			sb.WriteString(strings.Join(lines[1:], "\n"))
		} else {
			sb.WriteString(diffSummary)
		}
	}
	return sb.String()
}
