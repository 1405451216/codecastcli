// Package semantic 提供代码库语义索引能力（P3）。
//
// 设计目标：
//   - 把代码库按 tree-sitter 符号切块，每块生成 embedding
//   - 支持向量检索（语义相似）+ BM25 检索（关键词匹配）的混合检索
//   - 增量更新：文件变更时只重算该文件的 embedding
//   - 持久化：JSON 格式，零外部依赖（避免 CGO/sqlite-vec）
//
// 模块组成：
//   - embedding.go:  EmbeddingProvider 接口 + OpenAI 实现
//   - chunker.go:    代码分块（复用 indexer.Tag）
//   - vector_store.go: 向量存储 + 余弦相似度检索
//   - bm25.go:       BM25 关键词检索
//   - retriever.go:  混合检索（向量 + BM25 加权融合）
//   - index.go:      主索引器，整合以上 + 增量更新
package semantic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// EmbeddingProvider 向量嵌入接口。
// 不同 provider（OpenAI / Ollama / 本地 ONNX）实现此接口。
type EmbeddingProvider interface {
	// Embed 把文本转为向量。
	// 单次调用，输入一段文本，返回固定维度的向量。
	Embed(ctx context.Context, text string) ([]float32, error)
	// EmbedBatch 批量嵌入（性能优化）。
	// 默认实现可逐个调用 Embed。
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	// Dim 返回向量维度。
	Dim() int
	// Name 返回 provider 名称（用于持久化元数据）。
	Name() string
}

// OpenAIEmbeddingConfig OpenAI embedding 配置
type OpenAIEmbeddingConfig struct {
	APIKey  string
	BaseURL string // 默认 https://api.openai.com/v1
	Model   string // 默认 text-embedding-3-small
}

// DefaultOpenAIEmbeddingConfig 默认配置
func DefaultOpenAIEmbeddingConfig(apiKey string) OpenAIEmbeddingConfig {
	return OpenAIEmbeddingConfig{
		APIKey:  apiKey,
		BaseURL: "https://api.openai.com/v1",
		Model:   "text-embedding-3-small",
	}
}

// openAIEmbeddingResponse OpenAI API 响应
type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// NewOpenAIEmbedding 创建 OpenAI embedding provider
func NewOpenAIEmbedding(cfg OpenAIEmbeddingConfig) *OpenAIEmbedding {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "text-embedding-3-small"
	}
	return &OpenAIEmbedding{cfg: cfg, dim: 1536} // text-embedding-3-small 默认 1536 维
}

// OpenAIEmbedding OpenAI embedding 实现
type OpenAIEmbedding struct {
	cfg OpenAIEmbeddingConfig
	dim int
}

func (e *OpenAIEmbedding) Name() string { return "openai:" + e.cfg.Model }

func (e *OpenAIEmbedding) Dim() int { return e.dim }

func (e *OpenAIEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	vecs, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	return vecs[0], nil
}

func (e *OpenAIEmbedding) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// 构造请求
	reqBody := struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}{
		Model: e.cfg.Model,
		Input: texts,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimSuffix(e.cfg.BaseURL, "/") + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embedding API error: %d %s", resp.StatusCode, string(respBody))
	}

	var result openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// 按 index 排序确保顺序正确
	out := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index >= 0 && d.Index < len(out) {
			out[d.Index] = d.Embedding
		}
	}
	// 更新实际维度
	if len(out) > 0 && len(out[0]) > 0 {
		e.dim = len(out[0])
	}
	return out, nil
}

// MockEmbedding 测试用 mock provider。
// 用简单的字符哈希生成确定性向量，不依赖网络。
type MockEmbedding struct {
	dim int
}

// NewMockEmbedding 创建 mock provider
func NewMockEmbedding(dim int) *MockEmbedding {
	return &MockEmbedding{dim: dim}
}

func (m *MockEmbedding) Name() string { return "mock" }
func (m *MockEmbedding) Dim() int     { return m.dim }

// Embed 用文本字符的简单哈希生成向量。
// 相同文本生成相同向量，相似文本（共享词）生成相近向量。
// 仅用于测试，不代表真实语义。
func (m *MockEmbedding) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, m.dim)
	if len(text) == 0 {
		return vec, nil
	}
	// 简单哈希：每个字符贡献到对应维度
	words := strings.Fields(strings.ToLower(text))
	for _, w := range words {
		h := uint32(0)
		for _, c := range w {
			h = h*31 + uint32(c)
		}
		idx := int(h % uint32(m.dim))
		vec[idx] += 1.0
	}
	// L2 归一化
	norm := float32(0)
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		norm = sqrt32(norm)
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec, nil
}

func (m *MockEmbedding) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, err := m.Embed(ctx, t)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// sqrt32 float32 平方根
func sqrt32(x float32) float32 {
	if x <= 0 {
		return 0
	}
	// 用 Newton 迭代，避免引入 math.Sqrt 的 float64 转换
	g := x
	for i := 0; i < 10; i++ {
		g = (g + x/g) * 0.5
	}
	return g
}

// --- 智谱（Zhipu / BigModel）embedding ---
//
// 智谱 API 是 OpenAI 兼容接口，但 URL 和默认模型不同。
// 文档: https://open.bigmodel.cn/dev/api/vector/embedding-3
// 默认模型 embedding-3，维度 2048。

// ZhipuEmbeddingConfig 智谱 embedding 配置
type ZhipuEmbeddingConfig struct {
	APIKey  string
	BaseURL string // 默认 https://open.bigmodel.cn/api/paas/v4
	Model   string // 默认 embedding-3
}

// DefaultZhipuEmbeddingConfig 智谱默认配置
func DefaultZhipuEmbeddingConfig(apiKey string) ZhipuEmbeddingConfig {
	return ZhipuEmbeddingConfig{
		APIKey:  apiKey,
		BaseURL: "https://open.bigmodel.cn/api/paas/v4",
		Model:   "embedding-3",
	}
}

// NewZhipuEmbedding 创建智谱 embedding provider。
// 复用 OpenAIEmbedding 实现（智谱接口 OpenAI 兼容）。
func NewZhipuEmbedding(cfg ZhipuEmbeddingConfig) *OpenAIEmbedding {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://open.bigmodel.cn/api/paas/v4"
	}
	if cfg.Model == "" {
		cfg.Model = "embedding-3"
	}
	// 智谱 embedding-3 默认 2048 维
	return &OpenAIEmbedding{
		cfg: OpenAIEmbeddingConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		},
		dim: 2048,
	}
}

// --- 通义（DashScope / 阿里云百炼）embedding ---
//
// 通义 DashScope 提供 OpenAI 兼容模式。
// 文档: https://help.aliyun.com/zh/model-studio/getting-started/models
// 默认模型 text-embedding-v3，维度 1024。

// DashScopeEmbeddingConfig 通义 embedding 配置
type DashScopeEmbeddingConfig struct {
	APIKey  string
	BaseURL string // 默认 https://dashscope.aliyuncs.com/compatible-mode/v1
	Model   string // 默认 text-embedding-v3
}

// DefaultDashScopeEmbeddingConfig 通义默认配置
func DefaultDashScopeEmbeddingConfig(apiKey string) DashScopeEmbeddingConfig {
	return DashScopeEmbeddingConfig{
		APIKey:  apiKey,
		BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		Model:   "text-embedding-v3",
	}
}

// NewDashScopeEmbedding 创建通义 embedding provider。
// 复用 OpenAIEmbedding 实现（DashScope 兼容模式 OpenAI 兼容）。
func NewDashScopeEmbedding(cfg DashScopeEmbeddingConfig) *OpenAIEmbedding {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "text-embedding-v3"
	}
	// 通义 text-embedding-v3 默认 1024 维
	return &OpenAIEmbedding{
		cfg: OpenAIEmbeddingConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		},
		dim: 1024,
	}
}
