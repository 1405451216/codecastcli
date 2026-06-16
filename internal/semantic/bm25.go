package semantic

import (
	"math"
	"sort"
	"strings"
	"sync"
)

// BM25Index BM25 关键词检索索引。
// 用于符号名/路径/代码文本的精确关键词匹配，
// 与向量检索互补：向量擅长语义相似，BM25 擅长精确符号匹配。
type BM25Index struct {
	mu sync.RWMutex
	// docs 文档列表，每个文档对应一个 Chunk
	docs []bm25Doc
	// termFreq term → 文档ID → 词频
	termFreq map[string]map[int]int
	// docLen 每个文档的长度（token 数）
	docLen []int
	// avgDocLen 平均文档长度
	avgDocLen float64
	// df term → 包含该 term 的文档数
	df map[string]int
	// k1, b BM25 参数
	k1, b float64
}

type bm25Doc struct {
	Chunk Chunk
	Terms []string
}

// NewBM25Index 创建 BM25 索引。
// k1 控制词频饱和（典型 1.2-2.0），b 控制文档长度归一化（典型 0.75）。
func NewBM25Index() *BM25Index {
	return &BM25Index{
		termFreq: make(map[string]map[int]int),
		df:       make(map[string]int),
		k1:       1.5,
		b:        0.75,
	}
}

// Add 添加一个 chunk 到 BM25 索引。
// 同 ID 的 chunk 会被覆盖（需先 Remove）。
func (idx *BM25Index) Add(chunk Chunk) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// 分词：符号名（驼峰拆分）+ 路径 + 内容
	terms := idx.tokenizeChunk(chunk)

	docID := len(idx.docs)
	idx.docs = append(idx.docs, bm25Doc{Chunk: chunk, Terms: terms})
	idx.docLen = append(idx.docLen, len(terms))

	// 统计词频和文档频率
	seen := make(map[string]bool)
	for _, t := range terms {
		if idx.termFreq[t] == nil {
			idx.termFreq[t] = make(map[int]int)
		}
		idx.termFreq[t][docID]++
		if !seen[t] {
			idx.df[t]++
			seen[t] = true
		}
	}

	// 更新平均文档长度
	total := 0
	for _, l := range idx.docLen {
		total += l
	}
	if len(idx.docLen) > 0 {
		idx.avgDocLen = float64(total) / float64(len(idx.docLen))
	}
}

// tokenizeChunk 把 chunk 分词。
// 包含：符号名（驼峰拆分）+ 文件路径 + 代码内容。
func (idx *BM25Index) tokenizeChunk(chunk Chunk) []string {
	var terms []string
	// 符号名驼峰拆分
	terms = append(terms, splitCamelCase(strings.ToLower(chunk.Symbol))...)
	// 路径分词
	terms = append(terms, tokenize(strings.ReplaceAll(chunk.File, "/", " "))...)
	// 内容分词
	terms = append(terms, tokenize(chunk.Content)...)
	return terms
}

// RemoveByFile 删除指定文件的所有文档。
// 注意：BM25 的删除需要重建索引（文档 ID 会变）。
// 这里采用标记删除 + 惰性重建策略。
func (idx *BM25Index) RemoveByFile(file string) int {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	kept := idx.docs[:0]
	removed := 0
	for _, d := range idx.docs {
		if d.Chunk.File == file {
			removed++
			continue
		}
		kept = append(kept, d)
	}
	if removed > 0 {
		// 重建索引（文档 ID 变了）
		idx.docs = kept
		idx.rebuild()
	}
	return removed
}

// rebuild 重建内部统计（文档 ID 变化后调用）
func (idx *BM25Index) rebuild() {
	idx.termFreq = make(map[string]map[int]int)
	idx.df = make(map[string]int)
	idx.docLen = idx.docLen[:0]

	for docID, d := range idx.docs {
		terms := d.Terms
		idx.docLen = append(idx.docLen, len(terms))
		seen := make(map[string]bool)
		for _, t := range terms {
			if idx.termFreq[t] == nil {
				idx.termFreq[t] = make(map[int]int)
			}
			idx.termFreq[t][docID]++
			if !seen[t] {
				idx.df[t]++
				seen[t] = true
			}
		}
	}

	total := 0
	for _, l := range idx.docLen {
		total += l
	}
	if len(idx.docLen) > 0 {
		idx.avgDocLen = float64(total) / float64(len(idx.docLen))
	} else {
		idx.avgDocLen = 0
	}
}

// Search BM25 检索。
// query 会被分词，返回与查询最相关的 topK 个 chunk。
func (idx *BM25Index) Search(query string, topK int) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(idx.docs) == 0 {
		return nil
	}
	if topK <= 0 {
		topK = 10
	}

	queryTerms := tokenize(query)
	// 加上驼峰拆分（提升符号查询效果）
	for _, qt := range splitCamelCase(query) {
		queryTerms = append(queryTerms, strings.ToLower(qt))
	}

	N := len(idx.docs)
	scores := make(map[int]float64, N)

	for _, term := range queryTerms {
	 postings, ok := idx.termFreq[term]
		if !ok {
			continue
		}
		df := idx.df[term]
		if df == 0 {
			continue
		}
		// IDF
		idf := math.Log(1 + (float64(N-df)+0.5)/(float64(df)+0.5))
		for docID, tf := range postings {
			k1 := idx.k1
			b := idx.b
			dl := float64(idx.docLen[docID])
			avgdl := idx.avgDocLen
			if avgdl == 0 {
				avgdl = 1
			}
			tfF := float64(tf)
			// BM25 公式
			num := tfF * (k1 + 1)
			denom := tfF + k1*(1-b+b*dl/avgdl)
			score := idf * num / denom
			scores[docID] += score
		}
	}

	results := make([]SearchResult, 0, len(scores))
	for docID, score := range scores {
		results = append(results, SearchResult{
			Chunk:  idx.docs[docID].Chunk,
			Score:  score,
			Source: "bm25",
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

// Size 返回索引文档数
func (idx *BM25Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.docs)
}
