package semantic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SemanticIndex 语义索引器（P3 主入口）。
// 整合 Chunker + EmbeddingProvider + VectorStore + BM25Index，
// 提供代码库级的语义检索能力。
type SemanticIndex struct {
	mu          sync.RWMutex
	rootDir     string
	embedder    EmbeddingProvider
	chunker     *Chunker
	vectorStore *VectorStore
	bm25Index   *BM25Index
	retriever   *HybridRetriever

	// symbolExtractor 符号提取器（nil 则用固定行数切块）
	symbolExtractor SymbolExtractor

	// statePath 持久化路径（空则不持久化）
	statePath string

	// indexedFiles 已索引文件 → modtime，用于增量更新
	indexedFiles map[string]time.Time
}

// SemanticIndexConfig 索引配置
type SemanticIndexConfig struct {
	RootDir    string
	Embedder   EmbeddingProvider
	StatePath  string // 持久化路径，空则不持久化
	MaxChunkLines int
	// SymbolExtractor 可选符号提取器（nil 则用固定行数切块）
	SymbolExtractor SymbolExtractor
}

// NewSemanticIndex 创建语义索引器
func NewSemanticIndex(cfg SemanticIndexConfig) (*SemanticIndex, error) {
	if cfg.Embedder == nil {
		return nil, fmt.Errorf("Embedder 不能为空")
	}
	if cfg.RootDir == "" {
		return nil, fmt.Errorf("RootDir 不能为空")
	}

	dim := cfg.Embedder.Dim()
	vs := NewVectorStore(dim)
	bm25 := NewBM25Index()
	chunker := NewChunker()
	if cfg.MaxChunkLines > 0 {
		chunker.MaxChunkLines = cfg.MaxChunkLines
	}

	idx := &SemanticIndex{
		rootDir:         cfg.RootDir,
		embedder:        cfg.Embedder,
		chunker:         chunker,
		vectorStore:     vs,
		bm25Index:       bm25,
		statePath:       cfg.StatePath,
		symbolExtractor: cfg.SymbolExtractor,
		indexedFiles:    make(map[string]time.Time),
	}
	idx.retriever = NewHybridRetriever(cfg.Embedder, vs, bm25)
	return idx, nil
}

// IndexFile 索引单个文件（增量更新入口）。
// 1. 读取文件内容
// 2. 按符号切块
// 3. 批量生成 embedding
// 4. 加入向量存储和 BM25 索引
// 若文件已索引且未修改则跳过。
func (s *SemanticIndex) IndexFile(ctx context.Context, relPath string, symbols []SymbolInfo) error {
	absPath := filepath.Join(s.rootDir, relPath)
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	s.mu.RLock()
	lastMod, exists := s.indexedFiles[relPath]
	s.mu.RUnlock()
	if exists && !info.ModTime().After(lastMod) {
		return nil // 未修改，跳过
	}

	// 读取内容
	content, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// 检测语言
	language := detectLanguage(relPath)

	// 分块
	chunks := s.chunker.ChunkFile(relPath, language, string(content), symbols)
	if len(chunks) == 0 {
		return nil
	}

	// 先删除该文件的旧索引（增量更新）
	s.vectorStore.RemoveByFile(relPath)
	s.bm25Index.RemoveByFile(relPath)

	// 批量生成 embedding
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		// embedding 输入：符号名 + 签名 + 代码内容
		texts[i] = c.Symbol + "\n" + c.Signature + "\n" + c.Content
	}

	vectors, err := s.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed batch: %w", err)
	}

	// 加入向量存储
	entries := make([]VectorEntry, len(chunks))
	for i, c := range chunks {
		if i < len(vectors) && vectors[i] != nil {
			entries[i] = VectorEntry{Chunk: c, Vector: vectors[i]}
		}
	}
	if err := s.vectorStore.AddBatch(entries); err != nil {
		return fmt.Errorf("add to vector store: %w", err)
	}

	// 加入 BM25 索引
	for _, c := range chunks {
		s.bm25Index.Add(c)
	}

	s.mu.Lock()
	s.indexedFiles[relPath] = info.ModTime()
	s.mu.Unlock()

	return nil
}

// IndexDir 索引整个目录。
// 遍历目录，对每个支持的文件类型调用 IndexFile。
// 若配置了 SymbolExtractor，则按符号切块；否则用固定行数切块。
// cb 是进度回调（每个文件处理后调用）。
func (s *SemanticIndex) IndexDir(ctx context.Context, cb func(path string, err error)) error {
	return filepath.Walk(s.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if cb != nil {
				cb(path, err)
			}
			return nil
		}
		if info.IsDir() {
			if shouldSkipDir(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSupportedFile(path) {
			return nil
		}

		relPath, _ := filepath.Rel(s.rootDir, path)

		// 若有符号提取器，先提取符号再索引
		var symbols []SymbolInfo
		if s.symbolExtractor != nil {
			content, err := os.ReadFile(path)
			if err == nil {
				language := detectLanguage(relPath)
				symbols = s.symbolExtractor.Extract(relPath, content, language)
			}
			// 读取失败则 symbols 保持 nil，退化为固定行数切块
		}

		if err := s.IndexFile(ctx, relPath, symbols); err != nil {
			if cb != nil {
				cb(relPath, err)
			}
			return nil // 跳过错误文件
		}
		if cb != nil {
			cb(relPath, nil)
		}
		return nil
	})
}

// Retrieve 语义检索
func (s *SemanticIndex) Retrieve(ctx context.Context, query string) ([]SearchResult, error) {
	s.mu.RLock()
	retriever := s.retriever
	s.mu.RUnlock()
	if retriever == nil {
		return nil, fmt.Errorf("retriever not initialized")
	}
	return retriever.Retrieve(ctx, query)
}

// VectorStore 返回向量存储（用于持久化）
func (s *SemanticIndex) VectorStore() *VectorStore { return s.vectorStore }

// BM25Index 返回 BM25 索引
func (s *SemanticIndex) BM25Index() *BM25Index { return s.bm25Index }

// Stats 返回索引统计
func (s *SemanticIndex) Stats() IndexStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return IndexStats{
		IndexedFiles:  len(s.indexedFiles),
		VectorCount:   s.vectorStore.Size(),
		BM25DocCount:  s.bm25Index.Size(),
		EmbedderName:  s.embedder.Name(),
		VectorDim:     s.embedder.Dim(),
	}
}

// Clear 清空索引（向量存储 + BM25 + 文件记录）
func (s *SemanticIndex) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 重建空存储
	s.vectorStore = NewVectorStore(s.embedder.Dim())
	s.bm25Index = NewBM25Index()
	s.indexedFiles = make(map[string]time.Time)
	s.retriever = NewHybridRetriever(s.embedder, s.vectorStore, s.bm25Index)
}

// IndexStats 索引统计
type IndexStats struct {
	IndexedFiles int    `json:"indexed_files"`
	VectorCount  int    `json:"vector_count"`
	BM25DocCount int    `json:"bm25_doc_count"`
	EmbedderName string `json:"embedder_name"`
	VectorDim    int    `json:"vector_dim"`
}

// Save 持久化索引状态
func (s *SemanticIndex) Save() error {
	if s.statePath == "" {
		return nil
	}
	if err := s.vectorStore.Save(s.statePath); err != nil {
		return fmt.Errorf("save vector store: %w", err)
	}
	// BM25 索引不持久化（重建快，且 JSON 格式复杂）
	// indexedFiles 也不持久化（重启时按 modtime 增量更新）
	return nil
}

// Load 加载持久化状态
func (s *SemanticIndex) Load() error {
	if s.statePath == "" {
		return nil
	}
	if err := s.vectorStore.Load(s.statePath); err != nil {
		if os.IsNotExist(err) {
			return nil // 首次运行，无状态文件
		}
		return err
	}
	// 从向量存储重建 BM25 索引
	for _, entry := range s.vectorStore.All() {
		s.bm25Index.Add(entry.Chunk)
	}
	return nil
}

// detectLanguage 根据文件扩展名检测语言
func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".jsx", ".mjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".java":
		return "java"
	case ".kt":
		return "kotlin"
	case ".rs":
		return "rust"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".scala":
		return "scala"
	case ".sh", ".bash":
		return "shell"
	case ".md":
		return "markdown"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	default:
		return "unknown"
	}
}

// isSupportedFile 判断文件是否支持索引
func isSupportedFile(path string) bool {
	return detectLanguage(path) != "unknown"
}

// shouldSkipDir 判断目录是否应跳过
func shouldSkipDir(path string) bool {
	base := filepath.Base(path)
	skip := []string{
		".git", ".svn", ".hg", "node_modules", "vendor",
		"__pycache__", ".codecast", ".idea", ".vscode",
		"dist", "build", "out", "target", "bin", "obj",
		".next", ".nuxt",
	}
	for _, s := range skip {
		if base == s {
			return true
		}
	}
	return false
}
