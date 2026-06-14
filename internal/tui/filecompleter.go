package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"codecast/cli/internal/indexer"
)

// maxCompletionResults 限制补全结果数量
const maxCompletionResults = 20

// maxFileContentLen 文件内容最大长度
const maxFileContentLen = 4000

// fileRefPattern 匹配 @path/to/file 格式的文件引用
var fileRefPattern = regexp.MustCompile(`@([\w./\-]+\.[\w]+)`)

// FileCompleter @file 引用补全器
type FileCompleter struct {
	rootDir string
	indexer *indexer.Indexer
}

// NewFileCompleter 创建文件补全器
func NewFileCompleter(rootDir string) *FileCompleter {
	return &FileCompleter{
		rootDir: rootDir,
	}
}

// SetIndexer 设置索引器引用（可选，用于加速搜索）
func (fc *FileCompleter) SetIndexer(idx *indexer.Indexer) {
	fc.indexer = idx
}

// Complete 搜索匹配 prefix 的文件，返回相对路径列表（最多 20 条）
func (fc *FileCompleter) Complete(prefix string) []string {
	if prefix == "" {
		return nil
	}

	prefix = strings.ToLower(prefix)

	// 优先使用索引器
	if fc.indexer != nil {
		return fc.completeFromIndex(prefix)
	}

	// 回退到文件系统遍历
	return fc.completeFromFilesystem(prefix)
}

// completeFromIndex 使用索引器搜索
func (fc *FileCompleter) completeFromIndex(prefix string) []string {
	idx := fc.indexer.GetIndex()
	if idx == nil {
		return fc.completeFromFilesystem(prefix)
	}

	var results []string
	for _, entry := range idx.Files {
		if strings.Contains(strings.ToLower(entry.Path), prefix) {
			results = append(results, entry.Path)
		}
		if len(results) >= maxCompletionResults {
			break
		}
	}

	sort.Strings(results)
	if len(results) > maxCompletionResults {
		results = results[:maxCompletionResults]
	}
	return results
}

// completeFromFilesystem 从文件系统搜索
func (fc *FileCompleter) completeFromFilesystem(prefix string) []string {
	var results []string

	filepath.Walk(fc.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		relPath, relErr := filepath.Rel(fc.rootDir, path)
		if relErr != nil {
			return nil
		}

		// 跳过隐藏目录和常见忽略目录
		if shouldSkipPath(relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.Contains(strings.ToLower(relPath), prefix) {
			results = append(results, filepath.ToSlash(relPath))
		}

		if len(results) >= maxCompletionResults {
			return filepath.SkipDir
		}

		return nil
	})

	sort.Strings(results)
	return results
}

// ExpandFileReferences 展开输入中的 @path/to/file 引用为代码块
//
// 格式:
//
//	@path/to/file.go →
//	```path/to/file.go
//	// File: path/to/file.go
//	<file content>
//	```
func (fc *FileCompleter) ExpandFileReferences(input string) string {
	return fileRefPattern.ReplaceAllStringFunc(input, func(match string) string {
		// 提取路径（去掉 @ 前缀）
		refPath := match[1:]

		absPath := filepath.Join(fc.rootDir, filepath.FromSlash(refPath))
		data, err := os.ReadFile(absPath)
		if err != nil {
			// 文件无法读取，保留原始引用
			return match
		}

		content := string(data)
		if len(content) > maxFileContentLen {
			content = content[:maxFileContentLen] + "\n... (truncated)"
		}

		lang := detectLanguageFromExt(filepath.Ext(refPath))
		// 构建代码块：语言标记后跟路径，注释行标注文件路径，然后是内容
		codeBlock := fmt.Sprintf("```%s\n// File: %s\n%s\n```", lang, refPath, content)
		return codeBlock
	})
}

// detectLanguageFromExt 根据文件扩展名检测语言名称（用于语法高亮提示）
func detectLanguageFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescript"
	case ".jsx":
		return "javascript"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".kt":
		return "kotlin"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".scala":
		return "scala"
	case ".r":
		return "r"
	case ".sql":
		return "sql"
	case ".sh", ".bash":
		return "shell"
	case ".ps1":
		return "powershell"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".xml":
		return "xml"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".md":
		return "markdown"
	case ".toml":
		return "toml"
	case ".dockerfile":
		return "dockerfile"
	case ".lua":
		return "lua"
	case ".zig":
		return "zig"
	case ".ex", ".exs":
		return "elixir"
	case ".erl":
		return "erlang"
	case ".hs":
		return "haskell"
	case ".ml":
		return "ocaml"
	case ".vim":
		return "vim"
	default:
		return ""
	}
}

// shouldSkipPath 判断路径是否应跳过（隐藏目录、常见忽略目录）
func shouldSkipPath(relPath string) bool {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	ignoreDirs := map[string]bool{
		".git": true, ".svn": true, ".hg": true,
		"node_modules": true, "vendor": true, "__pycache__": true,
		".codecast": true, ".idea": true, ".vscode": true,
		"dist": true, "build": true, "out": true, "target": true,
		"bin": true, "obj": true, ".next": true, ".nuxt": true,
	}
	for _, part := range parts {
		if ignoreDirs[part] {
			return true
		}
		if strings.HasPrefix(part, ".") && len(part) > 1 {
			return true
		}
	}
	return false
}
