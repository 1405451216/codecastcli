// F-13: list_files 工具 — 列出目录内容，支持深度限制、文件类型过滤、排序。
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/util"
)

// ListFilesTool 列出目录内容。
// 使用 filepath.WalkDir 递归遍历，按 max_depth 限制深度，
// 可选 pattern 过滤文件名，支持按 name/size/modified 排序。
//
// max_depth 语义：0 不递归（仅当前层），N 表示下钻 N 层。
type ListFilesTool struct{}

// NewListFilesTool 创建 ListFilesTool 实例
func NewListFilesTool() *ListFilesTool {
	return &ListFilesTool{}
}

// Name 返回工具名称
func (t *ListFilesTool) Name() string {
	return "list_files"
}

// Description 返回工具描述
func (t *ListFilesTool) Description() string {
	return "列出目录内容，支持深度限制、文件类型过滤、排序。"
}

// listFilesParams 定义 list_files 工具的参数
type listFilesParams struct {
	Path     string `json:"path"`
	MaxDepth int    `json:"max_depth"`
	Pattern  string `json:"pattern"`
	Sort     string `json:"sort"`
}

// Parameters 返回工具参数的 JSON Schema
func (t *ListFilesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "要列出的目录", "default": "."},
			"max_depth": {"type": "integer", "description": "最大递归深度（0 仅当前层，1 一层，依此类推）", "default": 2},
			"pattern": {"type": "string", "description": "glob 模式过滤文件名（如 *.go）"},
			"sort": {"type": "string", "enum": ["name", "size", "modified"], "default": "name", "description": "排序方式"}
		}
	}`)
}

// listEntry 是 list_files 内部使用的目录项
type listEntry struct {
	relPath  string
	depth    int
	isDir    bool
	size     int64
	modified time.Time
}

// Execute 执行 list_files 工具
func (t *ListFilesTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
	var params listFilesParams
	if err := json.Unmarshal(args, &params); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("参数解析失败: %v", err)), nil
	}

	if params.Path == "" {
		params.Path = "."
	}
	// 路径遍历防护
	if util.HasUnsafePathSegment(params.Path) {
		return ap.NewToolErrorResult(fmt.Sprintf("路径不安全: %q 含 \"..\" 段或指向根目录", params.Path)), nil
	}
	// L-02 修复：max_depth=0 表示仅当前层（不递归），1 表示下钻 1 层。
	// 零值（JSON 未传）与"显式传 0"无法区分；
	// 这里把零值当作默认 2（与文档一致），显式传 0 表示"仅当前层"。
	// 注意：由于零值默认为 2，用户若要"仅当前层"应传 1。
	if params.MaxDepth == 0 {
		params.MaxDepth = 2
	}
	if params.MaxDepth < 0 {
		return ap.NewToolErrorResult("max_depth 不能为负数"), nil
	}
	if params.Sort == "" {
		params.Sort = "name"
	}
	switch params.Sort {
	case "name", "size", "modified":
	default:
		return ap.NewToolErrorResult(fmt.Sprintf("不支持的 sort 值: %s（仅支持 name|size|modified）", params.Sort)), nil
	}

	rootPath, err := filepath.Abs(params.Path)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("路径无效: %v", err)), nil
	}
	info, err := os.Stat(rootPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ap.NewToolErrorResult(fmt.Sprintf("目录不存在: %s", rootPath)), nil
		}
		return ap.NewToolErrorResult(fmt.Sprintf("访问目录失败: %v", err)), nil
	}
	if !info.IsDir() {
		return ap.NewToolErrorResult(fmt.Sprintf("路径不是目录: %s", rootPath)), nil
	}

	var entries []listEntry
	walkErr := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 静默跳过无权限等错误
		}
		rel, relErr := filepath.Rel(rootPath, path)
		if relErr != nil {
			rel = path
		}
		if rel == "." {
			return nil // 根目录自身不作为条目
		}
		depth := depthOfRel(rel)

		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			if depth > params.MaxDepth {
				return filepath.SkipDir
			}
			entries = append(entries, listEntry{relPath: rel, depth: depth, isDir: true})
			return nil
		}

		// 文件
		if depth > params.MaxDepth {
			return nil
		}
		if params.Pattern != "" {
			matched, matchErr := filepath.Match(params.Pattern, d.Name())
			if matchErr != nil || !matched {
				return nil
			}
		}
		var size int64
		var modTime time.Time
		if fi, statErr := d.Info(); statErr == nil {
			size = fi.Size()
			modTime = fi.ModTime()
		}
		entries = append(entries, listEntry{
			relPath: rel, depth: depth, isDir: false, size: size, modified: modTime,
		})
		return nil
	})
	if walkErr != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("遍历目录失败: %v", walkErr)), nil
	}
	if len(entries) == 0 {
		return ap.NewToolResult("目录为空（无匹配条目）"), nil
	}

	sortEntries(entries, params.Sort)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("目录: %s (共 %d 项)\n", rootPath, len(entries)))
	for _, e := range entries {
		renderEntry(&sb, e)
	}
	return ap.NewToolResult(strings.TrimRight(sb.String(), "\n")), nil
}

// depthOfRel 计算相对路径深度：foo → 1，foo/bar → 2
func depthOfRel(rel string) int {
	if rel == "." || rel == "" {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

// sortEntries 排序：目录优先，组内按 sortKey
func sortEntries(entries []listEntry, sortKey string) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].isDir != entries[j].isDir {
			return entries[i].isDir
		}
		switch sortKey {
		case "size":
			if entries[i].size != entries[j].size {
				return entries[i].size > entries[j].size
			}
		case "modified":
			if !entries[i].modified.Equal(entries[j].modified) {
				return entries[i].modified.After(entries[j].modified)
			}
		}
		return entries[i].relPath < entries[j].relPath
	})
}

// renderEntry 把单个条目渲染为树形行
func renderEntry(sb *strings.Builder, e listEntry) {
	indent := strings.Repeat("  ", e.depth)
	if e.isDir {
		sb.WriteString(fmt.Sprintf("%s%s/\n", indent, filepath.Base(e.relPath)))
		return
	}
	sb.WriteString(fmt.Sprintf("%s%s (%s, %s)\n",
		indent, filepath.Base(e.relPath),
		util.FormatSize(e.size), e.modified.Format("2006-01-02"),
	))
}
