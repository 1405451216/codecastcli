package indexer

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// IndexVersion 缓存版本号，版本不匹配时触发全量重建
const IndexVersion = "1.0"

// CachedFile 缓存的文件条目
type CachedFile struct {
	Path      string     `json:"path"`
	ModTime   time.Time  `json:"mod_time"`
	Size      int64      `json:"size"`
	Hash      string     `json:"hash"`
	FileEntry *FileEntry `json:"file_entry"`
}

// CachedIndex 可序列化的索引缓存
type CachedIndex struct {
	Version   string                 `json:"version"`
	IndexedAt time.Time              `json:"indexed_at"`
	Files     map[string]*CachedFile `json:"files"`
}

// cachePath 返回缓存文件的路径
func (idx *Indexer) cachePath() string {
	return filepath.Join(idx.rootDir, ".codecast", "index.json")
}

// loadCache 从磁盘加载缓存
func (idx *Indexer) loadCache(path string) (*CachedIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取缓存失败: %w", err)
	}

	var cached CachedIndex
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("解析缓存失败: %w", err)
	}

	return &cached, nil
}

// saveCache 持久化索引缓存到磁盘
func (idx *Indexer) saveCache() error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	cacheDir := filepath.Join(idx.rootDir, ".codecast")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}

	cached := &CachedIndex{
		Version:   IndexVersion,
		IndexedAt: idx.index.IndexedAt,
		Files:     make(map[string]*CachedFile, len(idx.index.Files)),
	}

	for relPath, entry := range idx.index.Files {
		absPath := filepath.Join(idx.rootDir, relPath)
		hash, err := fileHash(absPath)
		if err != nil {
			// 文件可能已被删除，跳过
			continue
		}
		cached.Files[relPath] = &CachedFile{
			Path:      relPath,
			ModTime:   entry.ModTime,
			Size:      entry.Size,
			Hash:      hash,
			FileEntry: entry,
		}
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化缓存失败: %w", err)
	}

	if err := os.WriteFile(idx.cachePath(), data, 0644); err != nil {
		return fmt.Errorf("写入缓存失败: %w", err)
	}

	return nil
}

// BuildOrLoad 尝试加载缓存，若缓存有效则增量更新，否则全量构建
func (idx *Indexer) BuildOrLoad() error {
	cacheFile := idx.cachePath()

	cached, err := idx.loadCache(cacheFile)
	if err != nil || cached.Version != IndexVersion {
		// 缓存不存在或版本不匹配，全量构建
		if buildErr := idx.Build(); buildErr != nil {
			return buildErr
		}
		// 全量构建后保存缓存
		_ = idx.saveCache()
		// 启动后台增量更新
		idx.startIncrementalUpdate(nil)
		return nil
	}

	// 缓存命中：加载到内存索引
	idx.mu.Lock()
	idx.index = &Index{
		Files:     make(map[string]*FileEntry, len(cached.Files)),
		Languages: make(map[string]int),
		RootDir:   idx.rootDir,
		IndexedAt: cached.IndexedAt,
	}
	for relPath, cf := range cached.Files {
		if cf.FileEntry != nil {
			idx.index.Files[relPath] = cf.FileEntry
			idx.index.TotalFiles++
			idx.index.TotalSize += cf.FileEntry.Size
			if cf.FileEntry.Language != "" {
				idx.index.Languages[cf.FileEntry.Language]++
			}
		}
	}
	idx.mu.Unlock()

	// 启动后台增量更新
	idx.startIncrementalUpdate(cached)
	return nil
}

// startIncrementalUpdate 初始化 fsnotify watcher 并启动后台增量更新 goroutine
func (idx *Indexer) startIncrementalUpdate(cached *CachedIndex) {
	// R5-C3 修复：关闭旧 watcher，防止 goroutine 泄漏
	if idx.watcher != nil {
		_ = idx.watcher.Close()
	}
	// R5-C3 修复：重新创建 done channel（Stop() 后 channel 可能已关闭）
	idx.done = make(chan struct{})
	// 重置 stopOnce，确保后续 Stop() 调用能正常执行清理
	idx.stopOnce = sync.Once{}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	idx.watcher = watcher

	// 监听根目录及其子目录
	filepath.Walk(idx.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			relPath, _ := filepath.Rel(idx.rootDir, path)
			if idx.shouldIgnoreDir(relPath) {
				return filepath.SkipDir
			}
			_ = watcher.Add(path)
		}
		return nil
	})

	go idx.incrementalUpdate(cached)
}

// incrementalUpdate 使用 fsnotify 监听文件变更并增量更新索引
func (idx *Indexer) incrementalUpdate(cached *CachedIndex) {
	if cached == nil {
		cached = &CachedIndex{
			Version:   IndexVersion,
			IndexedAt: time.Now(),
			Files:     make(map[string]*CachedFile),
		}
	}

	// 先做一次全量比对，处理缓存期间发生的变更
	idx.syncFromDisk(cached)

	for {
		select {
		case <-idx.done:
			return
		case event, ok := <-idx.watcher.Events:
			if !ok {
				return
			}
			idx.handleFsEvent(event, cached)
		case _, ok := <-idx.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

// syncFromDisk 对比磁盘文件与缓存，增量更新变更的文件
func (idx *Indexer) syncFromDisk(cached *CachedIndex) {
	// 收集磁盘上当前存在的文件
	diskFiles := make(map[string]bool)

	filepath.Walk(idx.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(idx.rootDir, path)

		if info.IsDir() {
			if idx.shouldIgnoreDir(relPath) {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if idx.ignoreExts[ext] {
			return nil
		}

		diskFiles[relPath] = true

		cf, inCache := cached.Files[relPath]
		if !inCache {
			// 新文件，需要索引
			idx.reindexFile(relPath, info)
			return nil
		}

		// 检查 ModTime 和 Size 是否变化
		if info.ModTime().Equal(cf.ModTime) && info.Size() == cf.Size {
			// 快速路径：未变化
			return nil
		}

		// 检查内容哈希
		hash, err := fileHash(path)
		if err != nil || hash != cf.Hash {
			idx.reindexFile(relPath, info)
		}
		return nil
	})

	// 删除磁盘上不存在的缓存条目
	idx.mu.Lock()
	for relPath := range cached.Files {
		if !diskFiles[relPath] {
			delete(idx.index.Files, relPath)
			delete(cached.Files, relPath)
		}
	}
	idx.recalcStats()
	idx.mu.Unlock()

	_ = idx.saveCache()
}

// handleFsEvent 处理单个 fsnotify 事件
func (idx *Indexer) handleFsEvent(event fsnotify.Event, cached *CachedIndex) {
	absPath := event.Name
	relPath, err := filepath.Rel(idx.rootDir, absPath)
	if err != nil {
		return
	}

	// 跳过忽略的目录
	if idx.shouldIgnoreDir(relPath) {
		return
	}

	// 跳过忽略的扩展名
	ext := strings.ToLower(filepath.Ext(absPath))
	if idx.ignoreExts[ext] {
		return
	}

	switch {
	case event.Has(fsnotify.Create):
		info, err := os.Stat(absPath)
		if err != nil {
			return
		}
		if info.IsDir() {
			// 新目录，加入 watcher
			if idx.watcher != nil {
				_ = idx.watcher.Add(absPath)
			}
			return
		}
		idx.reindexFile(relPath, info)
		_ = idx.saveCache()

	case event.Has(fsnotify.Write):
		info, err := os.Stat(absPath)
		if err != nil {
			return
		}
		if info.IsDir() {
			return
		}
		idx.reindexFile(relPath, info)
		_ = idx.saveCache()

	case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
		idx.mu.Lock()
		if _, exists := idx.index.Files[relPath]; exists {
			entry := idx.index.Files[relPath]
			idx.index.TotalFiles--
			idx.index.TotalSize -= entry.Size
			if entry.Language != "" {
				idx.index.Languages[entry.Language]--
				if idx.index.Languages[entry.Language] <= 0 {
					delete(idx.index.Languages, entry.Language)
				}
			}
			delete(idx.index.Files, relPath)
			delete(cached.Files, relPath)
		}
		idx.mu.Unlock()
		_ = idx.saveCache()
	}
}

// reindexFile 重新索引单个文件
func (idx *Indexer) reindexFile(relPath string, info os.FileInfo) {
	absPath := filepath.Join(idx.rootDir, relPath)
	ext := strings.ToLower(filepath.Ext(absPath))

	entry := &FileEntry{
		Path:     relPath,
		Name:     filepath.Base(absPath),
		Ext:      ext,
		Size:     info.Size(),
		ModTime:  info.ModTime(),
		Language: detectLanguage(ext),
		IsDir:    false,
	}

	if info.Size() < 100*1024 {
		idx.extractDependencies(absPath, entry)
	}

	idx.mu.Lock()
	// 如果旧条目存在，先减去其统计
	if old, exists := idx.index.Files[relPath]; exists {
		idx.index.TotalSize -= old.Size
		if old.Language != "" {
			idx.index.Languages[old.Language]--
			if idx.index.Languages[old.Language] <= 0 {
				delete(idx.index.Languages, old.Language)
			}
		}
	} else {
		idx.index.TotalFiles++
	}

	idx.index.Files[relPath] = entry
	idx.index.TotalSize += entry.Size
	if entry.Language != "" {
		idx.index.Languages[entry.Language]++
	}
	idx.mu.Unlock()
}

// recalcStats 重新计算 TotalFiles 和 TotalSize（调用时须持有 idx.mu）
func (idx *Indexer) recalcStats() {
	idx.index.TotalFiles = 0
	idx.index.TotalSize = 0
	idx.index.Languages = make(map[string]int)

	for _, entry := range idx.index.Files {
		idx.index.TotalFiles++
		idx.index.TotalSize += entry.Size
		if entry.Language != "" {
			idx.index.Languages[entry.Language]++
		}
	}
}

// Stop 停止增量更新并清理 watcher
// R5-C2 修复：使用 sync.Once 防止重复关闭 channel 导致 panic
func (idx *Indexer) Stop() {
	idx.stopOnce.Do(func() {
		if idx.done != nil {
			close(idx.done)
		}
		if idx.watcher != nil {
			_ = idx.watcher.Close()
		}
	})
}

// fileHash 计算文件的 MD5 哈希
func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(data)
	return fmt.Sprintf("%x", hash), nil
}
