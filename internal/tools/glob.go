package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	ap "agentprimordia/pkg"
)

// GlobSearchTool 按文件名模式搜索文件
type GlobSearchTool struct{}

// NewGlobSearchTool 创建 GlobSearchTool 实例
func NewGlobSearchTool() *GlobSearchTool {
	return &GlobSearchTool{}
}

// Name 返回工具名称
func (t *GlobSearchTool) Name() string {
	return "glob_search"
}

// Description 返回工具描述
func (t *GlobSearchTool) Description() string {
	return "按文件名模式搜索文件。"
}

// globSearchParams 定义 glob_search 工具的参数
type globSearchParams struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

// Parameters 返回工具参数的 JSON Schema
func (t *GlobSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "glob 模式（如 \"**/*.go\", \"src/**/*.ts\"）"
			},
			"path": {
				"type": "string",
				"description": "搜索的根目录",
				"default": "."
			}
		},
		"required": ["pattern"]
	}`)
}

// Execute 执行 glob_search 工具
func (t *GlobSearchTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
	var params globSearchParams
	if err := json.Unmarshal(args, &params); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("参数解析失败: %v", err)), nil
	}

	if params.Pattern == "" {
		return ap.NewToolErrorResult("pattern 不能为空"), nil
	}

	if params.Path == "" {
		params.Path = "."
	}

	// 规范化路径
	rootPath, err := filepath.Abs(params.Path)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("路径无效: %v", err)), nil
	}

	// 检查根目录是否存在
	info, err := os.Stat(rootPath)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("目录不存在: %v", err)), nil
	}
	if !info.IsDir() {
		return ap.NewToolErrorResult(fmt.Sprintf("路径不是目录: %s", rootPath)), nil
	}

	// 解析 glob 模式，处理 ** 递归匹配
	var matches []string

	// 判断是否包含 ** 递归模式
	if strings.Contains(params.Pattern, "**") {
		matches, err = globRecursive(rootPath, params.Pattern)
	} else {
		// 使用标准 filepath.Glob
		fullPattern := filepath.Join(rootPath, params.Pattern)
		matches, err = filepath.Glob(fullPattern)
	}

	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("搜索失败: %v", err)), nil
	}

	// 过滤掉目录，只保留文件
	var files []string
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil {
			continue
		}
		if !fi.IsDir() {
			files = append(files, m)
		}
	}

	if len(files) == 0 {
		return ap.NewToolResult(fmt.Sprintf("未找到匹配 %q 的文件", params.Pattern)), nil
	}

	// 格式化输出
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("找到 %d 个文件:\n", len(files)))
	for _, f := range files {
		// 使用相对路径显示
		rel, err := filepath.Rel(rootPath, f)
		if err != nil {
			rel = f
		}
		sb.WriteString(fmt.Sprintf("  %s\n", rel))
	}

	return ap.NewToolResult(strings.TrimRight(sb.String(), "\n")), nil
}

// globRecursive 处理包含 ** 的递归 glob 模式
func globRecursive(root, pattern string) ([]string, error) {
	// 将模式按 ** 分割
	parts := strings.SplitN(pattern, "**", 2)
	if len(parts) != 2 {
		// 没有 **，使用标准 glob
		return filepath.Glob(filepath.Join(root, pattern))
	}

	suffix := parts[1]
	suffix = strings.TrimPrefix(suffix, string(filepath.Separator))

	var results []string

	// 加载 .gitignore 过滤（若不存在则静默忽略）
	gitFilter, _ := NewGitignoreFilter(root)

	// 遍历所有子目录
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// 跳过 .git 等目录
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			// .gitignore 感知：检查目录是否被忽略
			if gitFilter != nil {
				if rel, relErr := filepath.Rel(root, path); relErr == nil && gitFilter.ShouldSkipDir(rel) {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// .gitignore 感知：检查文件是否被忽略
		if gitFilter != nil {
			if rel, relErr := filepath.Rel(root, path); relErr == nil && gitFilter.ShouldSkip(rel) {
				return nil
			}
		}

		// 获取相对路径
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		// 检查后缀匹配
		// 修复 globRecursive bug：**/*.ext 的 suffix 是 "/*.ext"，
		// 用 filepath.Match("/*.go", "main.go") 会失败（* 不匹配 /）。
		// 解决方案：当 suffix 以 / 开头时，用 path.Match（POSIX 风格，* 不跨 /）
		// 匹配去掉前导 / 的 suffix；suffix 不含 / 时直接用 filepath.Base 匹配。
		if suffix != "" {
			matched := false
			if strings.HasPrefix(suffix, string(filepath.Separator)) || strings.HasPrefix(suffix, "/") {
				// suffix 是路径模式（**/*.go → "/*.go"），用完整相对路径匹配
				// 去掉前导分隔符得到 "*.go"，path.Match 在 POSIX 风格下 * 不跨 /
				cleanSuffix := strings.TrimLeft(suffix, "/\\")
				matched, _ = filepath.Match(cleanSuffix, filepath.Base(rel))
				// 也尝试匹配子目录中的同名文件（如 src/auth/main.go）
				if !matched {
					// 用 HasSuffix 兜底处理深层路径
					relNoSlash := strings.ReplaceAll(rel, string(filepath.Separator), "/")
					suffixNoSlash := strings.ReplaceAll(cleanSuffix, "*", "")
					if strings.HasSuffix(relNoSlash, suffixNoSlash) {
						matched = true
					}
				}
			} else {
				// suffix 是纯文件名模式（如 *.go），直接匹配 basename
				matched, _ = filepath.Match(suffix, filepath.Base(rel))
			}
			if !matched {
				return nil
			}
		}

		results = append(results, path)
		return nil
	})

	return results, err
}
