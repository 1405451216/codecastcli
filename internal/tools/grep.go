package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	ap "agentprimordia/pkg"
)

// GrepSearchTool 在工作目录中搜索文件内容，支持正则表达式
type GrepSearchTool struct{}

// NewGrepSearchTool 创建 GrepSearchTool 实例
func NewGrepSearchTool() *GrepSearchTool {
	return &GrepSearchTool{}
}

// Name 返回工具名称
func (t *GrepSearchTool) Name() string {
	return "grep_search"
}

// Description 返回工具描述
func (t *GrepSearchTool) Description() string {
	return "在工作目录中搜索文件内容，支持正则表达式。"
}

// grepSearchParams 定义 grep_search 工具的参数
type grepSearchParams struct {
	Pattern         string `json:"pattern"`
	Path            string `json:"path"`
	FilePattern     string `json:"file_pattern"`
	MaxResults      int    `json:"max_results"`
	CaseInsensitive bool   `json:"case_insensitive"`
}

// Parameters 返回工具参数的 JSON Schema
func (t *GrepSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "搜索模式（正则表达式）"
			},
			"path": {
				"type": "string",
				"description": "搜索的根目录",
				"default": "."
			},
			"file_pattern": {
				"type": "string",
				"description": "文件名过滤（如 \"*.go\"）"
			},
			"max_results": {
				"type": "integer",
				"description": "最大结果数",
				"default": 50
			},
			"case_insensitive": {
				"type": "boolean",
				"description": "忽略大小写",
				"default": false
			}
		},
		"required": ["pattern"]
	}`)
}

// 需要跳过的目录
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".svn":         true,
	".hg":          true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".cache":       true,
}

// grepMatch 表示一个搜索匹配
type grepMatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
	Before  string `json:"before,omitempty"`
	After   string `json:"after,omitempty"`
}

// Execute 执行 grep_search 工具
func (t *GrepSearchTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
	var params grepSearchParams
	if err := json.Unmarshal(args, &params); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("参数解析失败: %v", err)), nil
	}

	if params.Pattern == "" {
		return ap.NewToolErrorResult("pattern 不能为空"), nil
	}

	if params.Path == "" {
		params.Path = "."
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 50
	}

	// 编译正则表达式
	flags := ""
	if params.CaseInsensitive {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + params.Pattern)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("正则表达式编译失败: %v", err)), nil
	}

	// 编译文件名过滤模式
	var fileRe *regexp.Regexp
	if params.FilePattern != "" {
		globPattern := globToRegex(params.FilePattern)
		fileRe, err = regexp.Compile(globPattern)
		if err != nil {
			return ap.NewToolErrorResult(fmt.Sprintf("文件名过滤模式无效: %v", err)), nil
		}
	}

	// 搜索文件
	var matches []grepMatch
	resultCount := 0

	err = filepath.WalkDir(params.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 跳过无法访问的路径
		}

		// 跳过指定目录
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// 检查文件名过滤
		if fileRe != nil && !fileRe.MatchString(d.Name()) {
			return nil
		}

		// 检查是否为二进制文件
		if isBinaryFile(path) {
			return nil
		}

		// 搜索文件内容
		fileMatches, err := searchInFile(path, re)
		if err != nil {
			return nil // 跳过无法读取的文件
		}

		for _, m := range fileMatches {
			if resultCount >= params.MaxResults {
				return fmt.Errorf("max results reached")
			}
			matches = append(matches, m)
			resultCount++
		}

		return nil
	})

	// 忽略 max results 错误
	if err != nil && err.Error() != "max results reached" {
		return ap.NewToolErrorResult(fmt.Sprintf("搜索失败: %v", err)), nil
	}

	if len(matches) == 0 {
		return ap.NewToolResult("未找到匹配的结果"), nil
	}

	// 格式化输出
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("找到 %d 处匹配:\n", len(matches)))
	for _, m := range matches {
		sb.WriteString(fmt.Sprintf("\n%s:%d: %s", m.File, m.Line, m.Content))
		if m.Before != "" {
			sb.WriteString(fmt.Sprintf("\n  前一行: %s", m.Before))
		}
		if m.After != "" {
			sb.WriteString(fmt.Sprintf("\n  后一行: %s", m.After))
		}
	}

	if resultCount >= params.MaxResults {
		sb.WriteString(fmt.Sprintf("\n\n（已达到最大结果数 %d，可能还有更多匹配）", params.MaxResults))
	}

	return ap.NewToolResult(sb.String()), nil
}

// searchInFile 在单个文件中搜索匹配
func searchInFile(path string, re *regexp.Regexp) ([]grepMatch, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var matches []grepMatch
	var lines []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	for i, line := range lines {
		if re.MatchString(line) {
			m := grepMatch{
				File:    path,
				Line:    i + 1,
				Content: strings.TrimSpace(line),
			}
			if i > 0 {
				m.Before = strings.TrimSpace(lines[i-1])
			}
			if i < len(lines)-1 {
				m.After = strings.TrimSpace(lines[i+1])
			}
			matches = append(matches, m)
		}
	}

	return matches, nil
}

// isBinaryFile 检查文件是否为二进制文件（读取前 8KB 检查 null 字节）
func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()

	buf := make([]byte, 8192)
	n, err := f.Read(buf)
	if err != nil {
		return true
	}

	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	return false
}

// globToRegex 将简单的 glob 模式转换为正则表达式
func globToRegex(pattern string) string {
	var re strings.Builder
	re.WriteString("^")
	for _, ch := range pattern {
		switch ch {
		case '*':
			re.WriteString(".*")
		case '?':
			re.WriteString(".")
		case '.':
			re.WriteString("\\.")
		default:
			re.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	re.WriteString("$")
	return re.String()
}
