package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/util"
)

// EditFileTool 通过精确字符串替换编辑文件
type EditFileTool struct{}

// NewEditFileTool 创建 EditFileTool 实例
func NewEditFileTool() *EditFileTool {
	return &EditFileTool{}
}

// Name 返回工具名称
func (t *EditFileTool) Name() string {
	return "edit_file"
}

// Description 返回工具描述
func (t *EditFileTool) Description() string {
	return "通过精确字符串替换编辑文件。指定要被替换的旧文本和新文本，工具会在文件中查找并替换。"
}

// editFileParams 定义 edit_file 工具的参数
type editFileParams struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

// Parameters 返回工具参数的 JSON Schema
func (t *EditFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "要编辑的文件路径"
			},
			"old_string": {
				"type": "string",
				"description": "要被替换的原始文本（必须在文件中唯一匹配）"
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
	}`)
}

// Execute 执行 edit_file 工具
func (t *EditFileTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
	var params editFileParams
	if err := json.Unmarshal(args, &params); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("参数解析失败: %v", err)), nil
	}

	if params.FilePath == "" {
		return ap.NewToolErrorResult("file_path 不能为空"), nil
	}
	if params.OldString == "" {
		return ap.NewToolErrorResult("old_string 不能为空"), nil
	}

	// S-04 修复：路径遍历防护
	if util.HasUnsafePathSegment(params.FilePath) {
		return ap.NewToolErrorResult(fmt.Sprintf("路径不安全: %q 含 \"..\" 段或指向根目录", params.FilePath)), nil
	}

	// Critical 修复：使用 Lstat 检查符号链接，防止符号链接重定向攻击
	linfo, err := os.Lstat(params.FilePath)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("获取文件信息失败: %v", err)), nil
	}
	if linfo.Mode()&os.ModeSymlink != 0 {
		return ap.NewToolErrorResult(fmt.Sprintf("拒绝编辑符号链接: %q，请编辑其指向的实际文件", params.FilePath)), nil
	}

	// 获取文件信息（保留原始权限）
	info, err := os.Stat(params.FilePath)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("获取文件信息失败: %v", err)), nil
	}

	// 读取文件
	content, err := os.ReadFile(params.FilePath)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("读取文件失败: %v", err)), nil
	}

	original := string(content)

	// 统计匹配次数
	count := strings.Count(original, params.OldString)

	// F-08：自动空白容差（每行 trim 尾部空白）— 减少 LLM 缩进漂移导致的失败
	// 仅在严格匹配为 0 次时尝试，不影响明确匹配的语义
	tolerantContent := tolerantNormalize(original)
	tolerantOld := tolerantNormalize(params.OldString)
	tolerantCount := strings.Count(tolerantContent, tolerantOld)
	if count == 0 && tolerantCount > 0 {
		// 重新执行宽松匹配：用原 old 字符串在原文中寻找
		// 简化处理：把原 content 的每行做 trimRight，再做替换
		// 然后把新 content 也做 trimRight，再把 old/new 的对齐写回
		count = tolerantCount
		params.OldString = findClosestMatch(original, params.OldString)
		if params.OldString == "" {
			return ap.NewToolErrorResult(fmt.Sprintf("未找到匹配的文本: %q", util.Truncate(params.OldString, 50))), nil
		}
	}

	// Fuzzy 匹配层：严格匹配和空白容差都失败时，用 Levenshtein 行级相似度
	// 找最近候选。置信度 > FuzzyMatchThreshold 自动应用，否则返回错误并提示候选。
	// 这是 Edit L1 改进：减少 LLM 缩进漂移/小笔误导致的匹配失败。
	if count == 0 {
		fz := fuzzyMatchLines(original, params.OldString)
		if fz.Confidence >= FuzzyMatchThreshold {
			// 自动应用：用 fuzzy 匹配到的原文子串作为 old_string
			count = 1
			params.OldString = fz.Matched
		} else if fz.Confidence > 0 {
			// 有候选但置信度不足，提示用户
			return ap.NewToolErrorResult(fmt.Sprintf(
				"未找到精确匹配。最相似候选在第 %d 行，置信度 %.2f（阈值 %.2f）。\n"+
					"候选片段:\n%s\n"+
					"如需应用，请提供更精确的 old_string，或调整缩进/空白后重试。",
				fz.StartLine, fz.Confidence, FuzzyMatchThreshold,
				util.Truncate(fz.Matched, 200),
			)), nil
		}
	}

	if count == 0 {
		return ap.NewToolErrorResult(fmt.Sprintf("未找到匹配的文本: %q", util.Truncate(params.OldString, 50))), nil
	}

	if count > 1 && !params.ReplaceAll {
		return ap.NewToolErrorResult(fmt.Sprintf(
			"找到 %d 处匹配，但 replace_all 为 false。请提供更多上下文使 old_string 唯一匹配，或设置 replace_all 为 true",
			count,
		)), nil
	}

	// 执行替换
	var newContent string
	if params.ReplaceAll {
		newContent = strings.ReplaceAll(original, params.OldString, params.NewString)
	} else {
		newContent = strings.Replace(original, params.OldString, params.NewString, 1)
	}

	// 原子写入：先写临时文件，再重命名
	// F-08 提示：os.Rename 在 POSIX 下遵循"replace target"语义，
	// 如果 params.FilePath 是符号链接，rename 会替换符号链接本身
	// （而非其指向的目标）。对于 v0.1.0 暂接受此行为 — 真要安全需要
	// 用 O_NOFOLLOW 打开并 renameat2(RENAME_NOREPLACE)。
	tmpFile := params.FilePath + ".tmp"
	if err := os.WriteFile(tmpFile, []byte(newContent), info.Mode()); err != nil {
		os.Remove(tmpFile)
		return ap.NewToolErrorResult(fmt.Sprintf("写入临时文件失败: %v", err)), nil
	}
	if err := os.Rename(tmpFile, params.FilePath); err != nil {
		os.Remove(tmpFile)
		// C-02 修复：Windows 上 Rename 可能因文件锁定失败，回退到覆盖写入
		if writeErr := overwriteFile(params.FilePath, []byte(newContent), info.Mode()); writeErr != nil {
			return ap.NewToolErrorResult(fmt.Sprintf("重命名和覆盖均失败: rename=%v overwrite=%v", err, writeErr)), nil
		}
	}

	// 生成 diff 摘要
	summary := buildDiffSummary(original, newContent, params.FilePath, count, params.ReplaceAll)

	return ap.NewToolResult(summary), nil
}

// buildDiffSummary 生成变更摘要
func buildDiffSummary(original, newContent, filePath string, count int, replaceAll bool) string {
	originalLines := strings.Split(original, "\n")
	newLines := strings.Split(newContent, "\n")

	var changedLines []string
	minLen := len(originalLines)
	if len(newLines) < minLen {
		minLen = len(newLines)
	}

	for i := 0; i < minLen; i++ {
		if originalLines[i] != newLines[i] {
			lineNum := i + 1
			changedLines = append(changedLines, fmt.Sprintf("  行 %d: -%s", lineNum, util.Truncate(originalLines[i], 80)))
			changedLines = append(changedLines, fmt.Sprintf("  行 %d: +%s", lineNum, util.Truncate(newLines[i], 80)))
		}
	}

	// 处理新增行
	for i := minLen; i < len(newLines); i++ {
		lineNum := i + 1
		changedLines = append(changedLines, fmt.Sprintf("  行 %d: +%s", lineNum, util.Truncate(newLines[i], 80)))
	}

	// 处理删除行
	for i := minLen; i < len(originalLines); i++ {
		lineNum := i + 1
		changedLines = append(changedLines, fmt.Sprintf("  行 %d: -%s", lineNum, util.Truncate(originalLines[i], 80)))
	}

	replaceInfo := ""
	if replaceAll && count > 1 {
		replaceInfo = fmt.Sprintf("（替换了全部 %d 处匹配）", count)
	} else {
		replaceInfo = "（替换了 1 处匹配）"
	}

	result := fmt.Sprintf("文件 %s 已更新%s\n变更行:\n%s", filePath, replaceInfo, strings.Join(changedLines, "\n"))
	return result
}

// tolerantNormalize 对每行做 trim 尾部空白，用于 F-08 的空白容差匹配。
// 不改变行内空白字符的位置。
func tolerantNormalize(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

// findClosestMatch 在 original 中寻找与 old 空白等价的最长匹配行，
// 返回其在原文中的精确子串。找不到返回空字符串。
// 实现思路：把 old 切成行，遍历 original 的每行做 trimRight 比较；
// 找到首个连续多行匹配窗口就返回原文对应片段。
func findClosestMatch(original, old string) string {
	if old == "" {
		return ""
	}
	tolerantOld := tolerantNormalize(old)
	tolerantOriginal := tolerantNormalize(original)
	idx := strings.Index(tolerantOriginal, tolerantOld)
	if idx < 0 {
		return ""
	}
	// 计算 tolerantOriginal 中 idx 位置对应到 original 的位置：
	// 逐字符累计直到 tolerantOriginal 达到 idx
	tOrig, tTol := 0, 0
	for tOrig < len(original) && tTol < idx {
		if original[tOrig] == '\n' {
			// 跳到下一行
			tOrig++
			tTol++
		} else {
			tOrig++
		}
	}
	// 现在 tOrig 指向 original 中 tolerant idx 处；向后取等长
	endOrig := tOrig
	endTol := tTol
	for endTol < tTol+len(tolerantOld) && endOrig < len(original) {
		if original[endOrig] == '\n' {
			endOrig++
			endTol++
		} else {
			endOrig++
		}
	}
	return original[tOrig:endOrig]
}

// overwriteFile 直接覆盖写入文件内容（C-02 修复：Windows 上 os.Rename
// 可能因目标文件被锁定而失败时的回退方案）。
// 先截断文件再写入，确保原子性（最坏情况下文件被截断但写入失败，
// 但 multi_edit 的回滚机制会恢复原文）。
func overwriteFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	return nil
}


