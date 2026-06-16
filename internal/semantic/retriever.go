package semantic

import (
	"context"
	"sort"
)

// HybridRetriever 混合检索器。
// 融合向量检索（语义相似）和 BM25 检索（关键词匹配），
// 取两者加权融合后的 topK 结果。
type HybridRetriever struct {
	vectorStore *VectorStore
	bm25Index   *BM25Index
	embedder    EmbeddingProvider
	// VectorWeight 向量检索权重（0-1），默认 0.7
	VectorWeight float64
	// BM25Weight BM25 检索权重（0-1），默认 0.3
	BM25Weight float64
	// TopK 返回结果数
	TopK int
}

// NewHybridRetriever 创建混合检索器
func NewHybridRetriever(embedder EmbeddingProvider, vs *VectorStore, bm25 *BM25Index) *HybridRetriever {
	return &HybridRetriever{
		vectorStore:  vs,
		bm25Index:    bm25,
		embedder:     embedder,
		VectorWeight: 0.7,
		BM25Weight:   0.3,
		TopK:         10,
	}
}

// Retrieve 混合检索。
// 1. 用 embedder 把 query 转为向量
// 2. 并行执行向量检索和 BM25 检索
// 3. 归一化各自分数后加权融合
// 4. 返回融合后的 topK 结果
func (r *HybridRetriever) Retrieve(ctx context.Context, query string) ([]SearchResult, error) {
	if r.vectorStore == nil || r.bm25Index == nil {
		return nil, nil
	}

	// 1. 向量检索
	vecResults := make([]SearchResult, 0)
	var vecErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		if r.embedder != nil && r.vectorStore.Size() > 0 {
			queryVec, err := r.embedder.Embed(ctx, query)
			if err != nil {
				vecErr = err
				return
			}
			vecResults = r.vectorStore.Search(queryVec, r.TopK*2) // 多取一些用于融合
		}
	}()

	// 2. BM25 检索（同步，很快）
	bm25Results := r.bm25Index.Search(query, r.TopK*2)

	<-done // 等待向量检索完成
	if vecErr != nil {
		// 向量检索失败，降级为纯 BM25
		return normalizeAndFusion(nil, bm25Results, 0, 1, r.TopK), nil
	}

	// 3. 归一化 + 融合
	return normalizeAndFusion(vecResults, bm25Results, r.VectorWeight, r.BM25Weight, r.TopK), nil
}

// normalizeAndFusion 归一化分数并加权融合。
// 向量分数范围 [-1,1]，BM25 分数范围 [0, ∞)，需归一化到 [0,1]。
func normalizeAndFusion(vecResults, bm25Results []SearchResult, vecW, bm25W float64, topK int) []SearchResult {
	// 归一化向量分数 [-1,1] → [0,1]
	vecNorm := normalizeScores(vecResults, -1, 1)
	// 归一化 BM25 分数 [0,max] → [0,1]
	bm25Norm := normalizeScores(bm25Results, 0, 0) // max=0 表示用实际最大值

	// 融合：按 chunk.ID 聚合
	fusion := make(map[string]SearchResult)
	for _, r := range vecNorm {
		r.Source = "hybrid"
		r.Score = r.Score * vecW
		fusion[r.Chunk.ID] = r
	}
	for _, r := range bm25Norm {
		r.Source = "hybrid"
		r.Score = r.Score * bm25W
		if existing, ok := fusion[r.Chunk.ID]; ok {
			// 已有向量结果，累加分数
			existing.Score += r.Score
			fusion[r.Chunk.ID] = existing
		} else {
			fusion[r.Chunk.ID] = r
		}
	}

	// 转为切片并排序
	results := make([]SearchResult, 0, len(fusion))
	for _, r := range fusion {
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

// normalizeScores 把分数归一化到 [0,1]。
// min, max 指定理论范围；若 max<=min 则用实际最大值。
func normalizeScores(results []SearchResult, min, max float64) []SearchResult {
	if len(results) == 0 {
		return results
	}

	// 找实际范围
	actualMin := results[0].Score
	actualMax := results[0].Score
	for _, r := range results {
		if r.Score < actualMin {
			actualMin = r.Score
		}
		if r.Score > actualMax {
			actualMax = r.Score
		}
	}

	// 使用理论范围或实际范围
	lo := min
	hi := max
	if hi <= lo {
		lo = actualMin
		hi = actualMax
	}

	range_ := hi - lo
	if range_ == 0 {
		// 所有分数相同，归一化为 1
		for i := range results {
			results[i].Score = 1.0
		}
		return results
	}

	out := make([]SearchResult, len(results))
	for i, r := range results {
		out[i] = r
		out[i].Score = (r.Score - lo) / range_
		if out[i].Score < 0 {
			out[i].Score = 0
		}
		if out[i].Score > 1 {
			out[i].Score = 1
		}
	}
	return out
}

// VectorOnlyRetrieve 纯向量检索（跳过 BM25）。
// 用于 BM25 索引为空或纯语义查询场景。
func (r *HybridRetriever) VectorOnlyRetrieve(ctx context.Context, query string) ([]SearchResult, error) {
	if r.embedder == nil || r.vectorStore == nil || r.vectorStore.Size() == 0 {
		return nil, nil
	}
	queryVec, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	return r.vectorStore.Search(queryVec, r.TopK), nil
}

// BM25OnlyRetrieve 纯 BM25 检索（跳过向量）。
// 用于无 embedding provider 或精确符号查询场景。
func (r *HybridRetriever) BM25OnlyRetrieve(query string) []SearchResult {
	if r.bm25Index == nil {
		return nil
	}
	return r.bm25Index.Search(query, r.TopK)
}
