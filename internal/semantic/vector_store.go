package semantic

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"sync"
)

// VectorEntry 向量存储条目
type VectorEntry struct {
	Chunk   Chunk    `json:"chunk"`
	Vector  []float32 `json:"vector"`
}

// VectorStore 向量存储。
// 纯 Go 内存实现 + JSON 持久化，避免 CGO/sqlite-vec 依赖。
// 适合中小型代码库（< 10 万 chunk）；超大规模可换 HNSW 实现。
type VectorStore struct {
	mu      sync.RWMutex
	entries []VectorEntry
	dim     int
}

// NewVectorStore 创建向量存储
func NewVectorStore(dim int) *VectorStore {
	return &VectorStore{dim: dim}
}

// Add 添加一个向量条目。
// 若 chunk.ID 已存在则覆盖。
func (s *VectorStore) Add(chunk Chunk, vector []float32) error {
	if len(vector) != s.dim {
		return fmt.Errorf("向量维度不匹配: 期望 %d，得到 %d", s.dim, len(vector))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 查找是否已存在
	for i, e := range s.entries {
		if e.Chunk.ID == chunk.ID {
			s.entries[i] = VectorEntry{Chunk: chunk, Vector: vector}
			return nil
		}
	}
	s.entries = append(s.entries, VectorEntry{Chunk: chunk, Vector: vector})
	return nil
}

// AddBatch 批量添加
func (s *VectorStore) AddBatch(items []VectorEntry) error {
	for _, item := range items {
		if len(item.Vector) != s.dim {
			return fmt.Errorf("向量维度不匹配: 期望 %d，得到 %d", s.dim, len(item.Vector))
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 构建已存在 ID 集合
	existing := make(map[string]int, len(s.entries))
	for i, e := range s.entries {
		existing[e.Chunk.ID] = i
	}
	for _, item := range items {
		if idx, ok := existing[item.Chunk.ID]; ok {
			s.entries[idx] = item
		} else {
			s.entries = append(s.entries, item)
			existing[item.Chunk.ID] = len(s.entries) - 1
		}
	}
	return nil
}

// RemoveByFile 删除指定文件的所有向量
func (s *VectorStore) RemoveByFile(file string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.entries[:0]
	removed := 0
	for _, e := range s.entries {
		if e.Chunk.File == file {
			removed++
			continue
		}
		kept = append(kept, e)
	}
	s.entries = kept
	return removed
}

// Search 向量检索：返回与 query 向量最相似的 topK 个 chunk。
// 使用余弦相似度。
func (s *VectorStore) Search(query []float32, topK int) []SearchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(query) != s.dim {
		return nil
	}
	if topK <= 0 {
		topK = 10
	}
	if topK > len(s.entries) {
		topK = len(s.entries)
	}

	results := make([]SearchResult, 0, len(s.entries))
	for _, e := range s.entries {
		score := cosineSimilarity(query, e.Vector)
		results = append(results, SearchResult{
			Chunk:  e.Chunk,
			Score:  score,
			Source: "vector",
		})
	}

	// 部分排序：只取 topK
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

// SearchResult 检索结果
type SearchResult struct {
	Chunk  Chunk   `json:"chunk"`
	Score  float64 `json:"score"`
	Source string  `json:"source"` // "vector" | "bm25" | "hybrid"
}

// Size 返回存储的向量数量
func (s *VectorStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// Dim 返回向量维度
func (s *VectorStore) Dim() int { return s.dim }

// All 返回所有条目的副本（用于持久化）
func (s *VectorStore) All() []VectorEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]VectorEntry, len(s.entries))
	copy(out, s.entries)
	return out
}

// Save 持久化到 JSON 文件
func (s *VectorStore) Save(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := json.MarshalIndent(struct {
		Dim     int           `json:"dim"`
		Entries []VectorEntry `json:"entries"`
	}{Dim: s.dim, Entries: s.entries}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Load 从 JSON 文件加载
func (s *VectorStore) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var payload struct {
		Dim     int           `json:"dim"`
		Entries []VectorEntry `json:"entries"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	s.mu.Lock()
	s.dim = payload.Dim
	s.entries = payload.Entries
	if s.entries == nil {
		s.entries = []VectorEntry{}
	}
	s.mu.Unlock()
	return nil
}

// cosineSimilarity 计算两个向量的余弦相似度。
// 返回值范围 [-1, 1]，1 = 完全相同。
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		af := float64(a[i])
		bf := float64(b[i])
		dot += af * bf
		na += af * af
		nb += bf * bf
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
