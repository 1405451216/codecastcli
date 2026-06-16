package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/util"
)

// ReadFileTool 读取文件内容，支持行号范围读取、大文件截断提示、输出带行号。
// 覆盖 AP 框架默认的 filesystem 工具中的 read action，提供更精细的 LLM 友好 API。
type ReadFileTool struct{}

// NewReadFileTool 创建 ReadFileTool 实例
func NewReadFileTool() *ReadFileTool {
	return &ReadFileTool{}
}

// Name 返回工具名称
func (t *ReadFileTool) Name() string {
	return "read_file"
}

// Description 返回工具描述
func (t *ReadFileTool) Description() string {
	return "读取文件内容，支持行号范围读取、大文件截断提示、输出带行号。"
}

// readFileParams 定义 read_file 工具的参数
// 注意：StartLine / EndLine 使用指针类型以区分"未提供"与"显式为 0"。
// 默认值：StartLine=0, EndLine=-1（读到末尾），通过 Unmarshal 后回填。
type readFileParams struct {
	FilePath  string `json:"file_path"`
	StartLine *int   `json:"start_line,omitempty"`
	EndLine   *int   `json:"end_line,omitempty"`
	Encoding  string `json:"encoding"`
}

// largeFileThreshold 触发大文件截断提示的阈值（行数）
const largeFileThreshold = 500

// Parameters 返回工具参数的 JSON Schema
func (t *ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "要读取的文件路径"
			},
			"start_line": {
				"type": "integer",
				"description": "起始行（0-based 包含），默认 0",
				"default": 0
			},
			"end_line": {
				"type": "integer",
				"description": "结束行（包含，-1 表示读到末尾），默认 -1",
				"default": -1
			},
			"encoding": {
				"type": "string",
				"description": "文件编码（v1 仅支持 utf-8 / utf8）",
				"default": "utf-8"
			}
		},
		"required": ["file_path"]
	}`)
}

// Execute 执行 read_file 工具
func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
	var params readFileParams
	if err := json.Unmarshal(args, &params); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("参数解析失败: %v", err)), nil
	}

	if params.FilePath == "" {
		return ap.NewToolErrorResult("file_path 不能为空"), nil
	}

	// S-04 修复：路径遍历防护
	if util.HasUnsafePathSegment(params.FilePath) {
		return ap.NewToolErrorResult(fmt.Sprintf("路径不安全: %q 含 \"..\" 段或指向根目录", params.FilePath)), nil
	}

	// R5-H14 修复：检查文件大小，防止大文件导致内存溢出
	const maxFileSize = 50 * 1024 * 1024 // 50MB
	info, err := os.Stat(params.FilePath)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("获取文件信息失败: %v", err)), nil
	}
	if info.Size() > maxFileSize {
		return ap.NewToolErrorResult(fmt.Sprintf("文件过大（%s），超过 %dMB 限制。请使用 start_line/end_line 分段读取", util.FormatSize(info.Size()), maxFileSize/(1024*1024))), nil
	}

	// 编码验证（v1 仅支持 utf-8 / utf8）
	enc := strings.ToLower(strings.TrimSpace(params.Encoding))
	if enc == "" {
		enc = "utf-8"
	}
	if enc != "utf-8" && enc != "utf8" {
		return ap.NewToolErrorResult(fmt.Sprintf("不支持的编码: %q（v1 仅支持 utf-8 / utf8）", params.Encoding)), nil
	}

	// 打开文件
	f, err := os.Open(params.FilePath)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("打开文件失败: %v", err)), nil
	}
	defer f.Close()

	// 二进制检测：读取前 8KB 检查 null 字节
	if isBinaryReader(f) {
		return ap.NewToolErrorResult("二进制文件不支持读取"), nil
	}
	// 回到文件开头
	if _, err := f.Seek(0, 0); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("读取文件失败: %v", err)), nil
	}

	// 按行扫描
	// M-01 修复：如果指定了行号范围，只收集目标行，避免大文件全量读入内存
	scanner := bufio.NewScanner(f)
	// 单行可能超过默认 64KB，扩展 buffer 上限到 10MB
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	needAll := params.StartLine == nil && params.EndLine == nil
	start := 0
	if params.StartLine != nil {
		start = *params.StartLine
	}
	end := -1
	if params.EndLine != nil {
		end = *params.EndLine
	}

	var lines []string
	totalLines := 0
	lineNum := 0
	for scanner.Scan() {
		totalLines++
		lineNum++
		if !needAll {
			// 只收集目标范围内的行
			if lineNum >= start+1 && (end == -1 || lineNum <= end+1) {
				lines = append(lines, scanner.Text())
			}
			// N-04 修复：指定行号范围时，读完目标范围后提前退出
			// totalLines 仅用于大文件截断提示，而指定行号范围时不会触发该提示
			if end != -1 && lineNum > end+1 {
				break
			}
		} else {
			lines = append(lines, scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("读取文件失败: %v", err)), nil
	}

	// 参数校验
	if start < 0 {
		return ap.NewToolErrorResult("start_line 不能为负数"), nil
	}
	if start >= totalLines {
		return ap.NewToolErrorResult(fmt.Sprintf("start_line 超出文件长度（文件共 %d 行）", totalLines)), nil
	}
	if end == -1 || end >= totalLines {
		end = totalLines - 1
	}
	if end < start {
		return ap.NewToolErrorResult("start_line 不能大于 end_line"), nil
	}

	// 构造带行号的输出（行号 1-based，与 grep_search 一致）
	var sb strings.Builder
	if needAll {
		for i, line := range lines {
			fmt.Fprintf(&sb, "%4d│ %s\n", i+1, line)
		}
	} else {
		// M-01: lines 只包含目标范围的行，行号从 start+1 开始
		for i, line := range lines {
			fmt.Fprintf(&sb, "%4d│ %s\n", start+1+i, line)
		}
	}

	// 大文件截断提示：仅在未指定 start_line/end_line 时触发
	truncated := params.StartLine == nil && params.EndLine == nil
	if truncated && totalLines > largeFileThreshold {
		fmt.Fprintf(&sb, "\n⚠ 文件有 %d 行，超过 %d 行。建议用 start_line 和 end_line 分段读取，例如 read_file(file_path='%s', start_line=0, end_line=200)。\n",
			totalLines, largeFileThreshold, params.FilePath)
	}

	return ap.NewToolResult(sb.String()), nil
}

// isBinaryReader 在已打开的 reader 上读取前 8KB 检查 null 字节。
// 与 grep.go 中的 isBinaryFile 类似但作用于已打开的文件。
func isBinaryReader(f *os.File) bool {
	buf := make([]byte, 8192)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		// 空文件不视为二进制
		return false
	}
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	return false
}
