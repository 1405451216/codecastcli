package tools

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	ap "agentprimordia/pkg"
)

// executeNative 是 grep_search 的纯 Go 原生实现，用作 ripgrep 不可用时的回退。
// 逻辑与重构前保持一致：filepath.WalkDir + Go regexp + 跳过常见大目录。
func (t *GrepSearchTool) executeNative(ctx context.Context, params grepSearchParams) (*ap.ToolResult, error) {
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

	// 加载 .gitignore 过滤（若不存在则静默忽略；与 glob.go 保持一致）
	gitFilter, _ := NewGitignoreFilter(params.Path)

	// 搜索文件
	var matches []grepMatch
	resultCount := 0
	stopWalk := fmt.Errorf("max results reached")

	err = filepath.WalkDir(params.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 跳过无法访问的路径
		}

		// 计算相对路径（用于 .gitignore 匹配）
		rel, _ := filepath.Rel(params.Path, path)

		// 跳过指定目录
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			// .gitignore 感知
			if gitFilter != nil && rel != "" && gitFilter.ShouldSkipDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}

		// .gitignore 感知
		if gitFilter != nil && rel != "" && gitFilter.ShouldSkip(rel) {
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
				return stopWalk
			}
			matches = append(matches, m)
			resultCount++
		}

		return nil
	})

	// "max results reached" 是正常终止信号，不是错误
	if err != nil && err != stopWalk {
		return ap.NewToolErrorResult(fmt.Sprintf("搜索失败: %v", err)), nil
	}

	if len(matches) == 0 {
		return ap.NewToolResult("未找到匹配的结果"), nil
	}

	// 格式化输出
	output := formatGrepMatches(matches, resultCount, params.MaxResults)
	return ap.NewToolResult(output), nil
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

// formatGrepMatches 把匹配结果格式化为最终用户可见的字符串。
// ripgrep 和 native 两条路径共用，保证输出风格一致。
func formatGrepMatches(matches []grepMatch, resultCount, maxResults int) string {
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

	if resultCount >= maxResults {
		sb.WriteString(fmt.Sprintf("\n\n（已达到最大结果数 %d，可能还有更多匹配）", maxResults))
	}

	return sb.String()
}
