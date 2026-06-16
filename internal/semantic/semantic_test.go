package semantic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- Embedding 测试 ---

func TestMockEmbedding_Deterministic(t *testing.T) {
	m := NewMockEmbedding(64)
	ctx := context.Background()

	v1, err := m.Embed(ctx, "hello world")
	if err != nil {
		t.Fatalf("Embed 失败: %v", err)
	}
	v2, err := m.Embed(ctx, "hello world")
	if err != nil {
		t.Fatalf("Embed 失败: %v", err)
	}

	// 相同输入应产生相同向量
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Errorf("相同输入应产生相同向量，维度 %d 不一致", i)
			break
		}
	}
}

func TestMockEmbedbing_Normalized(t *testing.T) {
	m := NewMockEmbedding(32)
	v, _ := m.Embed(context.Background(), "test input")
	// 验证 L2 范数 ≈ 1
	var norm float32
	for _, x := range v {
		norm += x * x
	}
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("向量应归一化，范数²=%f", norm)
	}
}

func TestMockEmbedding_Dim(t *testing.T) {
	m := NewMockEmbedding(128)
	if m.Dim() != 128 {
		t.Errorf("Dim 期望 128，得到 %d", m.Dim())
	}
}

func TestMockEmbedding_Batch(t *testing.T) {
	m := NewMockEmbedding(64)
	texts := []string{"hello", "world", "foo bar"}
	vecs, err := m.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatch 失败: %v", err)
	}
	if len(vecs) != 3 {
		t.Errorf("期望 3 个向量，得到 %d", len(vecs))
	}
}

func TestMockEmbedding_Empty(t *testing.T) {
	m := NewMockEmbedding(32)
	v, err := m.Embed(context.Background(), "")
	if err != nil {
		t.Fatalf("Embed 空字符串失败: %v", err)
	}
	// 空字符串应返回零向量
	for i, x := range v {
		if x != 0 {
			t.Errorf("空字符串应返回零向量，维度 %d = %f", i, x)
			break
		}
	}
}

// --- Chunker 测试 ---

func TestChunker_WithSymbols(t *testing.T) {
	c := NewChunker()
	content := "package main\n\nfunc foo() {\n\treturn 1\n}\n\nfunc bar() {\n\treturn 2\n}\n"
	symbols := []SymbolInfo{
		{Name: "foo", Kind: "function", Line: 3, Signature: "func foo()"},
		{Name: "bar", Kind: "function", Line: 7, Signature: "func bar()"},
	}
	chunks := c.ChunkFile("main.go", "go", content, symbols)
	if len(chunks) != 2 {
		t.Fatalf("期望 2 个 chunk，得到 %d", len(chunks))
	}
	if chunks[0].Symbol != "foo" {
		t.Errorf("chunk[0] Symbol 期望 foo，得到 %s", chunks[0].Symbol)
	}
	if chunks[0].StartLine != 3 {
		t.Errorf("chunk[0] StartLine 期望 3，得到 %d", chunks[0].StartLine)
	}
	if chunks[1].Symbol != "bar" {
		t.Errorf("chunk[1] Symbol 期望 bar，得到 %s", chunks[1].Symbol)
	}
}

func TestChunker_NoSymbols(t *testing.T) {
	c := NewChunker()
	c.MaxChunkLines = 5
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\n"
	chunks := c.ChunkFile("test.txt", "text", content, nil)
	if len(chunks) < 2 {
		t.Errorf("无符号时应按行数切块，期望 >=2 个 chunk，得到 %d", len(chunks))
	}
}

func TestChunker_MaxLines(t *testing.T) {
	c := NewChunker()
	c.MaxChunkLines = 3
	// 一个大函数
	content := "func big() {\n" + repeatLine("\tstmt", 10) + "}\n"
	symbols := []SymbolInfo{
		{Name: "big", Kind: "function", Line: 1},
	}
	chunks := c.ChunkFile("big.go", "go", content, symbols)
	if len(chunks) != 1 {
		t.Fatalf("期望 1 个 chunk，得到 %d", len(chunks))
	}
	if chunks[0].EndLine-chunks[0].StartLine+1 > c.MaxChunkLines {
		t.Errorf("chunk 行数应 <= %d，得到 %d", c.MaxChunkLines, chunks[0].EndLine-chunks[0].StartLine+1)
	}
}

func TestExtractSymbolsFromTags(t *testing.T) {
	tags := []TagLike{
		SimpleTag{Name: "foo", Kind: "function", Line: 10, Signature: "func foo()"},
		SimpleTag{Name: "Bar", Kind: "struct", Line: 20},
	}
	syms := ExtractSymbolsFromTags(tags)
	if len(syms) != 2 {
		t.Fatalf("期望 2 个符号，得到 %d", len(syms))
	}
	if syms[0].Name != "foo" || syms[0].Line != 10 {
		t.Errorf("符号 0 不正确: %+v", syms[0])
	}
}

// --- VectorStore 测试 ---

func TestVectorStore_AddSearch(t *testing.T) {
	vs := NewVectorStore(4)
	chunk := Chunk{ID: "f:1-3", File: "f", Symbol: "foo", Content: "foo"}
	vs.Add(chunk, []float32{1, 0, 0, 0})

	results := vs.Search([]float32{1, 0, 0, 0}, 5)
	if len(results) != 1 {
		t.Fatalf("期望 1 个结果，得到 %d", len(results))
	}
	if results[0].Chunk.Symbol != "foo" {
		t.Errorf("期望 foo，得到 %s", results[0].Chunk.Symbol)
	}
	if results[0].Score < 0.99 {
		t.Errorf("完全匹配 Score 应接近 1，得到 %f", results[0].Score)
	}
}

func TestVectorStore_DimMismatch(t *testing.T) {
	vs := NewVectorStore(4)
	err := vs.Add(Chunk{ID: "x"}, []float32{1, 2, 3})
	if err == nil {
		t.Error("维度不匹配应返回错误")
	}
}

func TestVectorStore_Overwrite(t *testing.T) {
	vs := NewVectorStore(4)
	vs.Add(Chunk{ID: "x", Symbol: "old"}, []float32{1, 0, 0, 0})
	vs.Add(Chunk{ID: "x", Symbol: "new"}, []float32{0, 1, 0, 0})
	if vs.Size() != 1 {
		t.Errorf("覆盖后 Size 应为 1，得到 %d", vs.Size())
	}
	results := vs.Search([]float32{0, 1, 0, 0}, 1)
	if results[0].Chunk.Symbol != "new" {
		t.Errorf("期望 new，得到 %s", results[0].Chunk.Symbol)
	}
}

func TestVectorStore_RemoveByFile(t *testing.T) {
	vs := NewVectorStore(4)
	vs.Add(Chunk{ID: "a:1", File: "a.go"}, []float32{1, 0, 0, 0})
	vs.Add(Chunk{ID: "b:1", File: "b.go"}, []float32{0, 1, 0, 0})
	removed := vs.RemoveByFile("a.go")
	if removed != 1 {
		t.Errorf("期望删除 1 个，得到 %d", removed)
	}
	if vs.Size() != 1 {
		t.Errorf("删除后 Size 应为 1，得到 %d", vs.Size())
	}
}

func TestVectorStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "vectors.json")

	vs1 := NewVectorStore(4)
	vs1.Add(Chunk{ID: "x", Symbol: "foo"}, []float32{1, 0, 0, 0})
	vs1.Add(Chunk{ID: "y", Symbol: "bar"}, []float32{0, 1, 0, 0})

	if err := vs1.Save(path); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	vs2 := NewVectorStore(4)
	if err := vs2.Load(path); err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if vs2.Size() != 2 {
		t.Errorf("加载后 Size 期望 2，得到 %d", vs2.Size())
	}

	results := vs2.Search([]float32{1, 0, 0, 0}, 1)
	if results[0].Chunk.Symbol != "foo" {
		t.Errorf("加载后检索期望 foo，得到 %s", results[0].Chunk.Symbol)
	}
}

func TestCosineSimilarity(t *testing.T) {
	cases := []struct {
		a, b []float32
		want float64
	}{
		{[]float32{1, 0}, []float32{1, 0}, 1.0},
		{[]float32{1, 0}, []float32{0, 1}, 0.0},
		{[]float32{1, 0}, []float32{-1, 0}, -1.0},
		{[]float32{0, 0}, []float32{1, 0}, 0.0}, // 零向量
	}
	for _, c := range cases {
		got := cosineSimilarity(c.a, c.b)
		if abs(got-c.want) > 0.001 {
			t.Errorf("cosineSimilarity(%v,%v) = %f, want %f", c.a, c.b, got, c.want)
		}
	}
}

// --- BM25 测试 ---

func TestBM25_Search(t *testing.T) {
	idx := NewBM25Index()
	idx.Add(Chunk{ID: "1", Symbol: "getUserInfo", File: "user.go", Content: "func getUserInfo(id int) User"})
	idx.Add(Chunk{ID: "2", Symbol: "getOrderInfo", File: "order.go", Content: "func getOrderInfo(id int) Order"})

	results := idx.Search("user", 5)
	if len(results) == 0 {
		t.Fatal("期望非空结果")
	}
	if results[0].Chunk.Symbol != "getUserInfo" {
		t.Errorf("期望 getUserInfo 排第一，得到 %s", results[0].Chunk.Symbol)
	}
}

func TestBM25_RemoveByFile(t *testing.T) {
	idx := NewBM25Index()
	idx.Add(Chunk{ID: "1", File: "a.go", Symbol: "foo", Content: "foo"})
	idx.Add(Chunk{ID: "2", File: "b.go", Symbol: "bar", Content: "bar"})

	removed := idx.RemoveByFile("a.go")
	if removed != 1 {
		t.Errorf("期望删除 1 个，得到 %d", removed)
	}
	if idx.Size() != 1 {
		t.Errorf("删除后 Size 期望 1，得到 %d", idx.Size())
	}
	// 搜索应只返回 b.go 的结果
	results := idx.Search("bar", 5)
	if len(results) != 1 || results[0].Chunk.File != "b.go" {
		t.Errorf("删除后搜索不正确: %+v", results)
	}
}

func TestBM25_Empty(t *testing.T) {
	idx := NewBM25Index()
	results := idx.Search("anything", 5)
	if results != nil {
		t.Errorf("空索引搜索应返回 nil，得到 %+v", results)
	}
}

// --- HybridRetriever 测试 ---

func TestHybridRetriever_Mock(t *testing.T) {
	embedder := NewMockEmbedding(32)
	vs := NewVectorStore(32)
	bm25 := NewBM25Index()

	// 添加几个 chunk
	chunks := []struct {
		symbol  string
		file    string
		content string
	}{
		{"getUserInfo", "user.go", "func getUserInfo(id int) User"},
		{"getOrderInfo", "order.go", "func getOrderInfo(id int) Order"},
		{"createUser", "user.go", "func createUser(name string) User"},
	}

	ctx := context.Background()
	for _, c := range chunks {
		ch := Chunk{ID: c.file + ":" + c.symbol, File: c.file, Symbol: c.symbol, Content: c.content, Language: "go"}
		vec, _ := embedder.Embed(ctx, c.symbol+" "+c.content)
		vs.Add(ch, vec)
		bm25.Add(ch)
	}

	r := NewHybridRetriever(embedder, vs, bm25)
	r.TopK = 3

	results, err := r.Retrieve(ctx, "user")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("期望非空结果")
	}
	// 至少一个结果应包含 "user" 相关符号
	found := false
	for _, r := range results {
		if contains(r.Chunk.Symbol, "User") || contains(r.Chunk.File, "user") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("期望结果包含 user 相关 chunk: %+v", results)
	}
}

func TestNormalizeScores(t *testing.T) {
	results := []SearchResult{
		{Score: 0.5},
		{Score: 1.0},
		{Score: 0.0},
	}
	norm := normalizeScores(results, 0, 1)
	if norm[0].Score < 0 || norm[0].Score > 1 {
		t.Errorf("归一化后分数应在 [0,1]: %f", norm[0].Score)
	}
	if norm[1].Score != 1.0 {
		t.Errorf("最大值应归一化为 1.0: %f", norm[1].Score)
	}
}

// --- SemanticIndex 集成测试 ---

func TestSemanticIndex_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	// 创建测试文件
	files := map[string]string{
		"user.go":  "package main\n\nfunc getUserInfo(id int) User {\n\treturn User{}\n}\n",
		"order.go": "package main\n\nfunc getOrderInfo(id int) Order {\n\treturn Order{}\n}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("创建文件失败: %v", err)
		}
	}

	embedder := NewMockEmbedding(32)
	idx, err := NewSemanticIndex(SemanticIndexConfig{
		RootDir:  tmpDir,
		Embedder: embedder,
	})
	if err != nil {
		t.Fatalf("NewSemanticIndex 失败: %v", err)
	}

	ctx := context.Background()

	// 索引目录
	if err := idx.IndexDir(ctx, nil); err != nil {
		t.Fatalf("IndexDir 失败: %v", err)
	}

	stats := idx.Stats()
	if stats.IndexedFiles != 2 {
		t.Errorf("期望 2 个已索引文件，得到 %d", stats.IndexedFiles)
	}
	if stats.VectorCount == 0 {
		t.Error("期望非零向量数")
	}

	// 检索
	results, err := idx.Retrieve(ctx, "user info")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("期望非空检索结果")
	}
}

func TestSemanticIndex_IncrementalUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := "test.go"
	fullPath := filepath.Join(tmpDir, filePath)
	content := "package main\n\nfunc foo() {}\n"
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	embedder := NewMockEmbedding(32)
	idx, _ := NewSemanticIndex(SemanticIndexConfig{
		RootDir:  tmpDir,
		Embedder: embedder,
	})

	ctx := context.Background()

	// 第一次索引（1 个符号 foo）
	if err := idx.IndexFile(ctx, filePath, []SymbolInfo{
		{Name: "foo", Kind: "function", Line: 3},
	}); err != nil {
		t.Fatalf("首次索引失败: %v", err)
	}
	stats1 := idx.Stats()
	if stats1.VectorCount != 1 {
		t.Fatalf("首次索引期望 1 个向量，得到 %d", stats1.VectorCount)
	}

	// 不修改文件，再次索引应跳过（相同 symbols）
	if err := idx.IndexFile(ctx, filePath, []SymbolInfo{
		{Name: "foo", Kind: "function", Line: 3},
	}); err != nil {
		t.Fatalf("二次索引失败: %v", err)
	}
	stats2 := idx.Stats()
	if stats2.VectorCount != stats1.VectorCount {
		t.Errorf("未修改文件不应重新索引: %d vs %d", stats1.VectorCount, stats2.VectorCount)
	}

	// 修改文件：加一个函数
	newContent := "package main\n\nfunc foo() {}\nfunc bar() {}\n"
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		t.Fatalf("修改文件失败: %v", err)
	}
	// 确保 modtime 变化
	os.Chtimes(fullPath, futureTime(), futureTime())

	// 修改后索引（2 个符号 foo + bar）
	if err := idx.IndexFile(ctx, filePath, []SymbolInfo{
		{Name: "foo", Kind: "function", Line: 3},
		{Name: "bar", Kind: "function", Line: 4},
	}); err != nil {
		t.Fatalf("修改后索引失败: %v", err)
	}
	stats3 := idx.Stats()
	if stats3.VectorCount != 2 {
		t.Errorf("修改后期望 2 个向量，得到 %d", stats3.VectorCount)
	}
}

func TestSemanticIndex_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	embedder := NewMockEmbedding(16)
	idx1, _ := NewSemanticIndex(SemanticIndexConfig{
		RootDir:   tmpDir,
		Embedder:  embedder,
		StatePath: statePath,
	})

	// 添加数据
	ctx := context.Background()
	idx1.IndexFile(ctx, "test.go", nil)
	// 手动创建测试文件
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("func foo(){}"), 0644)
	idx1.IndexFile(ctx, "test.go", nil)

	if err := idx1.Save(); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	// 新索引器加载
	idx2, _ := NewSemanticIndex(SemanticIndexConfig{
		RootDir:   tmpDir,
		Embedder:  embedder,
		StatePath: statePath,
	})
	if err := idx2.Load(); err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if idx2.VectorStore().Size() == 0 {
		t.Error("加载后向量存储不应为空")
	}
}

func TestDetectLanguage(t *testing.T) {
	cases := map[string]string{
		"main.go":     "go",
		"app.py":      "python",
		"index.js":    "javascript",
		"app.ts":      "typescript",
		"Main.java":   "java",
		"lib.rs":      "rust",
		"unknown.xyz": "unknown",
	}
	for path, want := range cases {
		if got := detectLanguage(path); got != want {
			t.Errorf("detectLanguage(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestShouldSkipDir(t *testing.T) {
	skip := []string{".git", "node_modules", "vendor", "build", "dist"}
	for _, d := range skip {
		if !shouldSkipDir(d) {
			t.Errorf("shouldSkipDir(%q) 期望 true", d)
		}
	}
	keep := []string{"src", "lib", "cmd", "internal"}
	for _, d := range keep {
		if shouldSkipDir(d) {
			t.Errorf("shouldSkipDir(%q) 期望 false", d)
		}
	}
}

// --- SymbolExtractor 集成测试 ---

// recordingExtractor 记录被调用的次数和路径，用于验证 IndexDir 是否真的调用了 SymbolExtractor
type recordingExtractor struct {
	calls   int
	paths   []string
	symbols []SymbolInfo
}

func (r *recordingExtractor) Extract(path string, content []byte, language string) []SymbolInfo {
	r.calls++
	r.paths = append(r.paths, path)
	// 返回一个伪符号，让切块按符号边界
	return []SymbolInfo{
		{Name: "synthetic_" + path, Kind: "function", Line: 1},
	}
}

func TestSemanticIndex_IndexDirCallsSymbolExtractor(t *testing.T) {
	tmpDir := t.TempDir()
	files := map[string]string{
		"a.go": "package main\n\nfunc foo() {}\n",
		"b.go": "package main\n\nfunc bar() {}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("创建文件失败: %v", err)
		}
	}

	rec := &recordingExtractor{}
	embedder := NewMockEmbedding(16)
	idx, err := NewSemanticIndex(SemanticIndexConfig{
		RootDir:         tmpDir,
		Embedder:        embedder,
		SymbolExtractor: rec,
	})
	if err != nil {
		t.Fatalf("NewSemanticIndex 失败: %v", err)
	}

	if err := idx.IndexDir(context.Background(), nil); err != nil {
		t.Fatalf("IndexDir 失败: %v", err)
	}

	if rec.calls != 2 {
		t.Errorf("SymbolExtractor 应被调用 2 次，实际 %d 次", rec.calls)
	}
	// 验证每个文件都被提取
	expectedPaths := map[string]bool{"a.go": false, "b.go": false}
	for _, p := range rec.paths {
		if _, ok := expectedPaths[p]; ok {
			expectedPaths[p] = true
		}
	}
	for p, seen := range expectedPaths {
		if !seen {
			t.Errorf("SymbolExtractor 未被对 %s 调用", p)
		}
	}

	// 验证生成的 chunk 用了符号名
	stats := idx.Stats()
	if stats.VectorCount != 2 {
		t.Errorf("期望 2 个向量（每个文件 1 个符号），得到 %d", stats.VectorCount)
	}
}

func TestSemanticIndex_NoExtractorFallback(t *testing.T) {
	tmpDir := t.TempDir()
	// 一个长文件，无符号提取器时应按行数切块
	content := ""
	for i := 0; i < 30; i++ {
		content += fmt.Sprintf("line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "long.go"), []byte(content), 0644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	embedder := NewMockEmbedding(16)
	idx, _ := NewSemanticIndex(SemanticIndexConfig{
		RootDir:       tmpDir,
		Embedder:      embedder,
		MaxChunkLines: 10,
		// 不设置 SymbolExtractor
	})

	if err := idx.IndexDir(context.Background(), nil); err != nil {
		t.Fatalf("IndexDir 失败: %v", err)
	}

	stats := idx.Stats()
	// 30 行 / 10 行每块，至少应有多块（行数切块，非符号切块）
	if stats.VectorCount < 2 {
		t.Errorf("无提取器时应按行数切块，期望 >=2 个向量，得到 %d", stats.VectorCount)
	}
}

// --- 智谱/通义 Embedding Provider 构造测试 ---

func TestNewZhipuEmbedding_Defaults(t *testing.T) {
	e := NewZhipuEmbedding(DefaultZhipuEmbeddingConfig("test-key"))
	if e.Name() != "openai:embedding-3" {
		t.Errorf("Name 期望 openai:embedding-3，得到 %s", e.Name())
	}
	if e.Dim() != 2048 {
		t.Errorf("Dim 期望 2048，得到 %d", e.Dim())
	}
}

func TestNewZhipuEmbedding_CustomModel(t *testing.T) {
	cfg := ZhipuEmbeddingConfig{APIKey: "k", Model: "embedding-2", BaseURL: "https://custom.example.com"}
	e := NewZhipuEmbedding(cfg)
	if e.Dim() != 2048 {
		t.Errorf("Dim 期望 2048，得到 %d", e.Dim())
	}
}

func TestNewZhipuEmbedding_EmptyDefaults(t *testing.T) {
	e := NewZhipuEmbedding(ZhipuEmbeddingConfig{APIKey: "k"})
	if e.Dim() != 2048 {
		t.Errorf("空配置应填充默认值，Dim 期望 2048，得到 %d", e.Dim())
	}
}

func TestNewDashScopeEmbedding_Defaults(t *testing.T) {
	e := NewDashScopeEmbedding(DefaultDashScopeEmbeddingConfig("test-key"))
	if e.Name() != "openai:text-embedding-v3" {
		t.Errorf("Name 期望 openai:text-embedding-v3，得到 %s", e.Name())
	}
	if e.Dim() != 1024 {
		t.Errorf("Dim 期望 1024，得到 %d", e.Dim())
	}
}

func TestNewDashScopeEmbedding_CustomModel(t *testing.T) {
	cfg := DashScopeEmbeddingConfig{APIKey: "k", Model: "text-embedding-v2", BaseURL: "https://custom.example.com"}
	e := NewDashScopeEmbedding(cfg)
	if e.Dim() != 1024 {
		t.Errorf("Dim 期望 1024，得到 %d", e.Dim())
	}
}

func TestNewDashScopeEmbedding_EmptyDefaults(t *testing.T) {
	e := NewDashScopeEmbedding(DashScopeEmbeddingConfig{APIKey: "k"})
	if e.Dim() != 1024 {
		t.Errorf("空配置应填充默认值，Dim 期望 1024，得到 %d", e.Dim())
	}
}

// --- 辅助函数 ---

func repeatLine(line string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += line + "\n"
	}
	return result
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// futureTime 返回一个未来时间（用于测试增量更新时强制 modtime 变化）
func futureTime() time.Time {
	return time.Now().Add(1 * time.Hour)
}
