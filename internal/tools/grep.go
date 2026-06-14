package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	ap "agentprimordia/pkg"
)

// 默认 ripgrep 二进制名（通过 exec.LookPath 查找）。
// 暴露为 var 便于测试覆盖为 echo、cat 等"假二进制"。
var defaultRipgrepBinary = "rg"

// GrepSearchTool 在工作目录中搜索文件内容，支持正则表达式。
//
// 执行路径：
//  1. 如果检测到 ripgrep（RipgrepPath），调用二进制并解析 --json 输出；
//  2. 否则回退到纯 Go 原生实现（grep_native.go）。
type GrepSearchTool struct {
	// RipgrepPath ripgrep 二进制路径；留空则使用 defaultRipgrepBinary。
	// 测试时可通过 NewGrepSearchToolWithRG 注入自定义路径。
	RipgrepPath string
}

// NewGrepSearchTool 创建默认 GrepSearchTool 实例。
// 默认自动通过 exec.LookPath("rg") 检测 ripgrep。
func NewGrepSearchTool() *GrepSearchTool {
	return &GrepSearchTool{RipgrepPath: defaultRipgrepBinary}
}

// NewGrepSearchToolWithRG 创建注入 ripgrep 路径的工具实例。
// 主要用于测试：传入 "" 强制走 native，传 "echo" 等可注入伪二进制。
func NewGrepSearchToolWithRG(rgPath string) *GrepSearchTool {
	return &GrepSearchTool{RipgrepPath: rgPath}
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

// Execute 执行 grep_search 工具。
// 优先调用 ripgrep（如果 RipgrepPath 指向有效二进制），失败时回退到 Go 原生实现。
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

	// 1. 检测 rg 是否可用
	if t.RipgrepPath != "" {
		rgPath, err := exec.LookPath(t.RipgrepPath)
		if err == nil {
			result, runErr := t.executeWithRipgrep(ctx, rgPath, params)
			if runErr == nil {
				return result, nil
			}
			// ripgrep 启动/解析失败 → 回退到 native（不要把整次调用搞砸）
			result, fallbackErr := t.executeNative(ctx, params)
			if fallbackErr != nil {
				return nil, fallbackErr
			}
			return result, nil
		}
	}

	// 2. 回退到 Go 原生实现
	return t.executeNative(ctx, params)
}

// executeWithRipgrep 通过 ripgrep 子进程执行搜索，解析 --json 输出。
// 在 --max-results 达到时主动 kill 进程，避免大仓库阻塞。
func (t *GrepSearchTool) executeWithRipgrep(ctx context.Context, rgPath string, params grepSearchParams) (*ap.ToolResult, error) {
	args := []string{
		"--json",               // JSON Lines 输出
		"--max-count", "5",     // 每文件最多 5 个匹配
		"--max-filesize", "1M", // 跳过超大文件
		"--smart-case",         // 智能大小写
		"--no-messages",        // 抑制无关 stderr
	}
	if params.FilePattern != "" {
		args = append(args, "--glob", params.FilePattern)
	}
	// case_insensitive=true 时显式覆盖 smart-case
	if params.CaseInsensitive {
		args = append(args, "-i")
	}
	// 模式 → 倒数第二个参数；路径 → 最后一个参数
	args = append(args, params.Pattern, params.Path)

	cmd := exec.CommandContext(ctx, rgPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	// 抑制 ripgrep 的 stderr 噪音（如权限警告），避免污染 ToolResult
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ripgrep start: %w", err)
	}

	matches, err := parseRipgrepJSON(stdout, params.MaxResults)
	if err != nil {
		// 主动 kill，避免子进程泄漏
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}

	// 等待 ripgrep 退出。退出码 1 通常表示"无匹配"，不算错。
	waitErr := cmd.Wait()
	exitCode := 0
	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}

	// ripgrep 退出码语义：
	//   0 = 有匹配
	//   1 = 无匹配
	//   2 = 实际错误
	// 这里我们只关心 2 以上的"真错误"，1 视作"无匹配"。
	if exitCode > 1 {
		return nil, fmt.Errorf("ripgrep 退出码 %d", exitCode)
	}

	if len(matches) == 0 {
		return ap.NewToolResult("未找到匹配的结果"), nil
	}

	output := formatGrepMatches(matches, len(matches), params.MaxResults)
	return ap.NewToolResult(output), nil
}

// parseRipgrepJSON 逐行解析 `rg --json` 的 JSON Lines 输出。
// 严格按 ripgrep 文档格式：每行一个 JSON，含 type 字段。
//
// 支持的 type：
//   - "begin"  → 记录当前文件路径
//   - "match"  → 创建一条 grepMatch 并加入结果
//   - "end"    → 忽略
//   - "summary"→ 忽略
//
// 达到 maxResults 时返回 (matches, errMaxResultsReached)，由调用方 kill 进程。
func parseRipgrepJSON(r io.Reader, maxResults int) ([]grepMatch, error) {
	var matches []grepMatch
	var currentFile string

	scanner := bufio.NewScanner(r)
	// 提升单行上限，应对长匹配行（默认 64KB 在大 minified 文件里不够）
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				LineNumber int `json:"line_number"`
			} `json:"data"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			// 忽略单行解析错误，继续读
			continue
		}

		switch msg.Type {
		case "begin":
			currentFile = msg.Data.Path.Text
		case "match":
			if currentFile == "" {
				// 防御性兜底：少数 ripgrep 版本对单行 stdin 流可能省略 begin
				currentFile = msg.Data.Path.Text
			}
			matches = append(matches, grepMatch{
				File:    currentFile,
				Line:    msg.Data.LineNumber,
				Content: strings.TrimRight(msg.Data.Lines.Text, "\n"),
			})
			if len(matches) >= maxResults {
				// 返回哨兵错误：调用方需要 kill 进程
				return matches, errMaxResultsReached
			}
		case "end", "summary":
			// 忽略
		}
	}

	if err := scanner.Err(); err != nil {
		return matches, fmt.Errorf("读取 ripgrep 输出失败: %w", err)
	}
	return matches, nil
}

// errMaxResultsReached 是 parseRipgrepJSON 达到 maxResults 时返回的哨兵错误。
var errMaxResultsReached = fmt.Errorf("max results reached")
