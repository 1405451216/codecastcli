package cmd

// interactive_files.go: 文件引用与 @file 展开相关函数（从 interactive.go 拆分）
//
// 包含：文件引用正则、expandFileReferences、语言检测、handleFilesCommand。

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"codecast/cli/internal/agent"
	"codecast/cli/internal/util"

	"github.com/fatih/color"
)

// fileRefRe matches @path/to/file references in user input.
var fileRefRe = regexp.MustCompile(`@([^\s/@][^\s]*)`)

// expandFileReferences expands @path/to/file references in user input.
func expandFileReferences(input string) string {
	if fileCompleter != nil {
		return fileCompleter.ExpandFileReferences(input)
	}
	return fileRefRe.ReplaceAllStringFunc(input, func(match string) string {
		path := match[1:]
		// R5-C10 修复：验证路径安全性，防止路径遍历攻击
		absPath, err := filepath.Abs(filepath.Clean(path))
		if err != nil {
			return match
		}
		if util.HasUnsafePathSegment(absPath) {
			return match
		}
		// 检查是否为常规文件，拒绝符号链接
		info, err := os.Lstat(absPath)
		if err != nil {
			return match
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return match // 拒绝符号链接
		}
		if info.IsDir() {
			return match // 拒绝目录
		}
		content, err := os.ReadFile(absPath)
		if err != nil {
			return match
		}
		ext := filepath.Ext(absPath)
		lang := detectLanguageFromExt(ext)
		truncated := truncateContent(string(content), 4000)
		return fmt.Sprintf("\n```%s\n// File: %s\n%s\n```\n", lang, path, truncated)
	})
}

// detectLanguageFromExt maps file extensions to code fence language identifiers.
func detectLanguageFromExt(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".js":
		return "javascript"
	case ".jsx":
		return "jsx"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".md":
		return "markdown"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	case ".sql":
		return "sql"
	case ".sh":
		return "bash"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".h":
		return "c"
	case ".hpp":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".r":
		return "r"
	case ".lua":
		return "lua"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	case ".xml":
		return "xml"
	case ".proto":
		return "protobuf"
	case ".dockerfile":
		return "dockerfile"
	case ".makefile":
		return "makefile"
	default:
		return ""
	}
}

// truncateContent truncates content to maxChars bytes, appending a notice if truncated.
func truncateContent(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	return content[:maxChars] + "\n... (truncated, file too large)"
}

// handleFilesCommand 处理 /files 命令，列出匹配 glob 模式的文件
func handleFilesCommand(args string, ag *agent.CodecastAgent) {
	pattern := strings.TrimSpace(args)
	if pattern == "" {
		pattern = "*"
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		color.Red("无效的 glob 模式: %v", err)
		return
	}

	if len(matches) == 0 {
		color.Yellow("没有匹配 %s 的文件", pattern)
		color.White("提示: 使用 @<path> 在消息中引用文件内容")
		return
	}

	color.Cyan("匹配 %s 的文件 (%d):", pattern, len(matches))
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if info.IsDir() {
			color.White("  📁 %s/", m)
		} else {
			color.White("  📄 %s (%s)", m, humanizeBytes(int(info.Size())))
		}
	}
	color.HiBlack("提示: 使用 @<path> 在消息中引用文件内容")
}
