package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileEntry 文件条目
type FileEntry struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Ext      string    `json:"ext"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"mod_time"`
	Language string    `json:"language"`
	Imports  []string  `json:"imports,omitempty"`
	Exports  []string  `json:"exports,omitempty"`
	Tags     []Tag     `json:"tags,omitempty"`
	IsDir    bool      `json:"is_dir"`
}

// Dependency 依赖关系
type Dependency struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // "import", "require", "include"
}

// Index 索引
type Index struct {
	Files        map[string]*FileEntry `json:"files"`
	Dependencies []Dependency          `json:"dependencies"`
	Languages    map[string]int        `json:"languages"` // language -> count
	TotalFiles   int                   `json:"total_files"`
	TotalSize    int64                 `json:"total_size"`
	RootDir      string                `json:"root_dir"`
	IndexedAt    time.Time             `json:"indexed_at"`
}

// Indexer 代码库索引器
type Indexer struct {
	rootDir    string
	index      *Index
	mu         sync.RWMutex
	ignoreDirs []string
	ignoreExts map[string]bool
	watcher    *fsnotify.Watcher
	done       chan struct{}
}

// NewIndexer 创建索引器
func NewIndexer(rootDir string) *Indexer {
	return &Indexer{
		rootDir: rootDir,
		index: &Index{
			Files:     make(map[string]*FileEntry),
			Languages: make(map[string]int),
		},
		done: make(chan struct{}),
		ignoreDirs: []string{
			".git", ".svn", ".hg",
			"node_modules", "vendor", "__pycache__",
			".codecast", ".idea", ".vscode",
			"dist", "build", "out", "target",
			"bin", "obj", ".next", ".nuxt",
		},
		ignoreExts: map[string]bool{
			".exe": true, ".dll": true, ".so": true, ".dylib": true,
			".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
			".ico": true, ".svg": true, ".woff": true, ".woff2": true,
			".ttf": true, ".eot": true, ".mp3": true, ".mp4": true,
			".zip": true, ".tar": true, ".gz": true, ".rar": true,
			".7z": true, ".pdf": true, ".doc": true, ".docx": true,
			".pyc": true, ".pyo": true, ".class": true, ".o": true,
			".a": true, ".lib": true, ".pdb": true, ".ilk": true,
		},
	}
}

// Build 构建索引（同步），构建完成后保存缓存
func (idx *Indexer) Build() error {
	return idx.BuildWithCallback(nil)
}

// BuildWithCallback 构建索引，每个文件处理完后调用 cb(path)（F-07 配套）。
// cb 为 nil 时等价于 Build()。回调在锁外执行。
// 使用显式 Lock/Unlock 而非 defer，因为 callback 需要临时释放锁。
func (idx *Indexer) BuildWithCallback(cb func(path string)) error {
	idx.mu.Lock()

	// 重置索引
	idx.index = &Index{
		Files:     make(map[string]*FileEntry),
		Languages: make(map[string]int),
	}
	idx.index.RootDir = idx.rootDir

	err := filepath.Walk(idx.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过错误
		}

		relPath, _ := filepath.Rel(idx.rootDir, path)

		// 跳过忽略目录
		if info.IsDir() {
			if idx.shouldIgnoreDir(relPath) {
				return filepath.SkipDir
			}
			return nil
		}

		// 跳过忽略文件
		ext := strings.ToLower(filepath.Ext(path))
		if idx.ignoreExts[ext] {
			return nil
		}

		// 创建文件条目
		entry := &FileEntry{
			Path:     relPath,
			Name:     filepath.Base(path),
			Ext:      ext,
			Size:     info.Size(),
			ModTime:  info.ModTime(),
			Language: detectLanguage(ext),
			IsDir:    false,
		}

		// 提取代码标签（轻量，始终执行）
		if info.Size() < 500*1024 { // 500KB 以下
			data, err := os.ReadFile(path)
			if err == nil {
				entry.Tags = extractTags(path, data, entry.Language)
			}
		}

		// 提取依赖信息（较重，限制文件大小）
		if info.Size() < 100*1024 { // 100KB 以下
			idx.extractDependencies(path, entry)
		}

		idx.index.Files[relPath] = entry
		idx.index.TotalFiles++
		idx.index.TotalSize += info.Size()

		if entry.Language != "" {
			idx.index.Languages[entry.Language]++
		}

		// F-07 配套：把回调放到锁外执行 — 简化方案是直接调用，
		// 大量小文件场景下 callback 仍可能慢；如需更高吞吐可改 channel 模式。
		if cb != nil {
			idx.mu.Unlock()
			cb(relPath)
			idx.mu.Lock()
			if idx.index == nil {
				// 回调可能调用了 Rebuild；不要继续修改旧 index
				return filepath.SkipAll
			}
		}

		return nil
	})

	if err != nil {
		idx.mu.Unlock()
		return fmt.Errorf("索引构建失败: %w", err)
	}

	idx.index.IndexedAt = time.Now()
	idx.mu.Unlock()

	// 构建完成后保存缓存
	_ = idx.saveCache()

	return nil
}

// GetIndex 获取索引
func (idx *Indexer) GetIndex() *Index {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.index
}

// SearchFiles 搜索文件
func (idx *Indexer) SearchFiles(query string) []*FileEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	query = strings.ToLower(query)
	var results []*FileEntry

	for _, entry := range idx.index.Files {
		if strings.Contains(strings.ToLower(entry.Name), query) ||
			strings.Contains(strings.ToLower(entry.Path), query) {
			results = append(results, entry)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Path < results[j].Path
	})

	return results
}

// SearchByLanguage 按语言搜索文件
func (idx *Indexer) SearchByLanguage(language string) []*FileEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []*FileEntry
	for _, entry := range idx.index.Files {
		if entry.Language == language {
			results = append(results, entry)
		}
	}
	return results
}

// GetDependencies 获取文件的依赖
func (idx *Indexer) GetDependencies(filePath string) []Dependency {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var deps []Dependency
	for _, dep := range idx.index.Dependencies {
		if dep.From == filePath {
			deps = append(deps, dep)
		}
	}
	return deps
}

// GetDependents 获取依赖此文件的文件
func (idx *Indexer) GetDependents(filePath string) []Dependency {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var deps []Dependency
	for _, dep := range idx.index.Dependencies {
		if dep.To == filePath {
			deps = append(deps, dep)
		}
	}
	return deps
}

// GetFileTree 获取文件树（用于上下文注入）
func (idx *Indexer) GetFileTree() string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("项目: %s (%d 文件, %s)\n\n", idx.index.RootDir, idx.index.TotalFiles, FormatSize(idx.index.TotalSize)))

	// 语言统计
	sb.WriteString("语言分布:\n")
	for lang, count := range idx.index.Languages {
		sb.WriteString(fmt.Sprintf("  %s: %d 文件\n", lang, count))
	}
	sb.WriteString("\n")

	// 文件树
	sb.WriteString("文件结构:\n")
	dirs := make(map[string][]string)
	for path, entry := range idx.index.Files {
		dir := filepath.Dir(path)
		if dir == "." {
			dir = "(root)"
		}
		dirs[dir] = append(dirs[dir], entry.Name)
	}

	var sortedDirs []string
	for dir := range dirs {
		sortedDirs = append(sortedDirs, dir)
	}
	sort.Strings(sortedDirs)

	for _, dir := range sortedDirs {
		sb.WriteString(fmt.Sprintf("  %s/\n", dir))
		files := dirs[dir]
		sort.Strings(files)
		for _, f := range files {
			sb.WriteString(fmt.Sprintf("    - %s\n", f))
		}
	}

	return sb.String()
}

// GetContextForFile 获取文件的上下文信息
func (idx *Indexer) GetContextForFile(filePath string) string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, ok := idx.index.Files[filePath]
	if !ok {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("文件: %s\n", entry.Path))
	sb.WriteString(fmt.Sprintf("语言: %s\n", entry.Language))
	sb.WriteString(fmt.Sprintf("大小: %s\n", FormatSize(entry.Size)))

	if len(entry.Imports) > 0 {
		sb.WriteString("导入:\n")
		for _, imp := range entry.Imports {
			sb.WriteString(fmt.Sprintf("  - %s\n", imp))
		}
	}

	if len(entry.Exports) > 0 {
		sb.WriteString("导出:\n")
		for _, exp := range entry.Exports {
			sb.WriteString(fmt.Sprintf("  - %s\n", exp))
		}
	}

	// 依赖此文件的文件
	dependents := idx.GetDependents(filePath)
	if len(dependents) > 0 {
		sb.WriteString("被引用:\n")
		for _, dep := range dependents {
			sb.WriteString(fmt.Sprintf("  - %s\n", dep.From))
		}
	}

	return sb.String()
}

// shouldIgnoreDir 判断是否应忽略目录
func (idx *Indexer) shouldIgnoreDir(relPath string) bool {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	for _, part := range parts {
		for _, ignore := range idx.ignoreDirs {
			if part == ignore {
				return true
			}
		}
	}
	return false
}

// extractDependencies 提取文件依赖
func (idx *Indexer) extractDependencies(path string, entry *FileEntry) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)

	switch entry.Language {
	case "go":
		idx.extractGoDeps(content, entry)
	case "python":
		idx.extractPythonDeps(content, entry)
	case "javascript", "typescript":
		idx.extractJSDeps(content, entry)
	}
}

// extractGoDeps 提取 Go 依赖
func (idx *Indexer) extractGoDeps(content string, entry *FileEntry) {
	// 匹配 import 语句
	importRe := regexp.MustCompile(`import\s+(?:"([^"]+)"|[\(]([\s\S]*?)[\)])`)
	matches := importRe.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if m[1] != "" {
			entry.Imports = append(entry.Imports, m[1])
		} else if m[2] != "" {
			// 多行 import
			lines := strings.Split(m[2], "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				line = strings.TrimSuffix(line, `"`)
				line = strings.TrimPrefix(line, `"`)
				if line != "" && !strings.HasPrefix(line, "//") {
					entry.Imports = append(entry.Imports, strings.Trim(line, `"`))
				}
			}
		}
	}

	// 匹配导出函数/类型
	exportRe := regexp.MustCompile(`(?:func|type|var|const)\s+([A-Z]\w+)`)
	exportMatches := exportRe.FindAllStringSubmatch(content, -1)
	for _, m := range exportMatches {
		entry.Exports = append(entry.Exports, m[1])
	}
}

// extractPythonDeps 提取 Python 依赖
func (idx *Indexer) extractPythonDeps(content string, entry *FileEntry) {
	importRe := regexp.MustCompile(`(?:import|from)\s+([a-zA-Z_][\w.]*)`)
	matches := importRe.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		entry.Imports = append(entry.Imports, m[1])
	}

	// 匹配类和函数定义
	defRe := regexp.MustCompile(`(?:class|def)\s+(\w+)`)
	defMatches := defRe.FindAllStringSubmatch(content, -1)
	for _, m := range defMatches {
		if !strings.HasPrefix(m[1], "_") {
			entry.Exports = append(entry.Exports, m[1])
		}
	}
}

// extractJSDeps 提取 JS/TS 依赖
func (idx *Indexer) extractJSDeps(content string, entry *FileEntry) {
	// import 语句
	importRe := regexp.MustCompile(`(?:import|require)\s*\(?\s*['"]([^'"]+)['"]`)
	matches := importRe.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		entry.Imports = append(entry.Imports, m[1])
	}

	// export 语句
	exportRe := regexp.MustCompile(`export\s+(?:default\s+)?(?:function|class|const|let|var)\s+(\w+)`)
	exportMatches := exportRe.FindAllStringSubmatch(content, -1)
	for _, m := range exportMatches {
		entry.Exports = append(entry.Exports, m[1])
	}
}

// detectLanguage 检测编程语言
func detectLanguage(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts", ".tsx":
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
	case ".r", ".R":
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
	case ".dockerfile":
		return "dockerfile"
	case ".toml":
		return "toml"
	default:
		return ""
	}
}

// FormatSize 格式化文件大小
func FormatSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case size >= GB:
		return fmt.Sprintf("%.1f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.1f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.1f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}
