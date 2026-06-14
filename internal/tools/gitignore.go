package tools

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/sabhiram/go-gitignore"
)

// GitignoreFilter wraps a .gitignore matcher and provides ShouldSkip / ShouldSkipDir
// helpers used by grep_search and glob_search to avoid traversing user-ignored files.
//
// 简单实现：只加载 rootDir 自身的 .gitignore（父目录与子目录的 .gitignore 暂不合并）。
// 这覆盖 90% 用例；复杂多级场景用户可手动指定 path 排除。
type GitignoreFilter struct {
	ignore *ignore.GitIgnore // nil 表示未加载（无 .gitignore 或加载失败）
	root   string            // 用于规范化的根目录
}

// NewGitignoreFilter 在 rootDir 下尝试加载 .gitignore。
// 任何错误（不存在、读失败、解析失败）都返回 (filter, nil)，
// filter.ShouldSkip 永远返回 false —— 不会因为 .gitignore 问题阻塞工具执行。
func NewGitignoreFilter(rootDir string) (*GitignoreFilter, error) {
	abs, err := filepath.Abs(rootDir)
	if err != nil {
		return &GitignoreFilter{root: rootDir}, nil
	}
	gitignorePath := filepath.Join(abs, ".gitignore")
	ig, err := ignore.CompileIgnoreFile(gitignorePath)
	if err != nil {
		// 不存在 / 读取失败 / 解析失败：记录日志但不阻塞
		if !os.IsNotExist(err) {
			log.Printf("gitignore: failed to load %s: %v (continuing without ignore)", gitignorePath, err)
		}
		return &GitignoreFilter{root: abs}, nil
	}
	return &GitignoreFilter{ignore: ig, root: abs}, nil
}

// normalize 去除前导 ./，并把反斜杠替换为正斜杠以匹配 gitignore 规则。
func (f *GitignoreFilter) normalize(relPath string) string {
	relPath = filepath.ToSlash(relPath)
	relPath = strings.TrimPrefix(relPath, "./")
	return relPath
}

// ShouldSkip 判断给定的文件相对路径是否应被 .gitignore 跳过。
// relPath 应为相对 f.root 的路径（使用正斜杠或 filepath.Separator 都可）。
func (f *GitignoreFilter) ShouldSkip(relPath string) bool {
	if f == nil || f.ignore == nil {
		return false
	}
	return f.ignore.MatchesPath(f.normalize(relPath))
}

// ShouldSkipDir 判断给定的目录相对路径是否应被 .gitignore 跳过。
// 当 .gitignore 中某条规则以 / 结尾（仅匹配目录）时，调用方应使用本方法。
func (f *GitignoreFilter) ShouldSkipDir(relDirPath string) bool {
	if f == nil || f.ignore == nil {
		return false
	}
	p := f.normalize(relDirPath)
	// 去掉末尾斜杠再加 "/"，因为 MatchesPath 内部用 "dir/" 形式匹配目录
	p = strings.TrimRight(p, "/") + "/"
	return f.ignore.MatchesPath(p)
}
