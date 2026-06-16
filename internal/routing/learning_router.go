package routing

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// LearningRouter 学习型模型路由器（L2）。
//
// 设计：
//   - L1 规则（ClassifyTier）决定档位：simple / medium / complex
//   - 每个档位可有多个候选模型（CandidateModels 配置）
//   - L2 在同档位候选模型中用 epsilon-greedy + Wilson CI 收敛
//   - 冷启动期均匀轮转，保证每个模型有数据
//   - 状态持久化到 ~/.codecast/model_routing_state.json
//
// 与 ModelRouter 关系：
//   - ModelRouter 是纯规则路由（L0），保持不变
//   - LearningRouter 组合 ModelRouter，先调 L1 得档位，再调 L2 选模型
//   - 不配置候选模型时退化为 ModelRouter 行为
//
// 统计复用 ab 包的 WilsonInterval / IsSignificantlyBetter 思路，
// 但独立实现，避免 ab 包依赖 routing 包形成循环。
type LearningRouter struct {
	mu          sync.RWMutex
	cfg         LearningConfig
	router      *ModelRouter
	state       *LearningState
	rng         *rand.Rand
}

// LearningConfig 学习型路由配置
type LearningConfig struct {
	// Enabled 是否启用学习层（false 时退化为 ModelRouter）
	Enabled bool `yaml:"enabled"`
	// Epsilon 探索率（0-1）。0=纯利用，1=纯探索。推荐 0.15
	Epsilon float64 `yaml:"epsilon"`
	// MinSamples 冷启动期每模型最少采样数。推荐 3
	MinSamples int `yaml:"min_samples"`
	// StatePath 状态文件路径，默认 ~/.codecast/model_routing_state.json
	StatePath string `yaml:"state_path"`
	// CandidateModels 每个档位的候选模型列表。
	// 未配置的档位退化为 ModelRouter 的单一模型。
	CandidateModels map[Tier][]string `yaml:"candidate_models"`
}

// DefaultLearningConfig 默认学习配置。
// 默认不启用，需用户显式开启。
func DefaultLearningConfig() LearningConfig {
	return LearningConfig{
		Enabled:     false,
		Epsilon:     0.15,
		MinSamples:  3,
		StatePath:   defaultStatePath(),
		CandidateModels: map[Tier][]string{},
	}
}

// defaultStatePath 返回默认状态文件路径 ~/.codecast/model_routing_state.json
func defaultStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "model_routing_state.json"
	}
	return filepath.Join(home, ".codecast", "model_routing_state.json")
}

// ModelStat 单个模型的统计
type ModelStat struct {
	// Name 模型名
	Name string `json:"name"`
	// Tier 所属档位
	Tier Tier `json:"tier"`
	// Samples 被选中次数
	Samples int `json:"samples"`
	// Successes 成功次数（无错误 + 用户接受）
	Successes int `json:"successes"`
	// TotalCostUSD 累计成本
	TotalCostUSD float64 `json:"total_cost_usd"`
	// TotalTokens 累计 token
	TotalTokens int64 `json:"total_tokens"`
	// LastUpdated 最后更新时间
	LastUpdated time.Time `json:"last_updated"`
}

// AvgCost 平均每次成本
func (s *ModelStat) AvgCost() float64 {
	if s.Samples == 0 {
		return 0
	}
	return s.TotalCostUSD / float64(s.Samples)
}

// SuccessRate 成功率
func (s *ModelStat) SuccessRate() float64 {
	if s.Samples == 0 {
		return 0
	}
	return float64(s.Successes) / float64(s.Samples)
}

// Score 综合评分（越大越好）。
// 公式：(1/avgCost) * (0.5 + successRate*0.5)
// 成本越低、成功率越高，分越高。
// 无数据返回 0。
func (s *ModelStat) Score() float64 {
	if s.Samples == 0 {
		return 0
	}
	avgCost := s.AvgCost()
	if avgCost <= 0 {
		// 成本为 0（本地模型）→ 用成功率作分
		return s.SuccessRate()
	}
	baseScore := 1.0 / avgCost
	return baseScore * (0.5 + s.SuccessRate()*0.5)
}

// LearningState 持久化状态
type LearningState struct {
	// Models 每个模型的统计，key = model name
	Models map[string]*ModelStat `json:"models"`
	// TotalDecisions 总决策次数
	TotalDecisions int `json:"total_decisions"`
	// LastUpdated 最后更新时间
	LastUpdated time.Time `json:"last_updated"`
}

// NewLearningRouter 创建学习型路由器。
// router 是底层 L0 规则路由器，cfg 是学习配置。
func NewLearningRouter(router *ModelRouter, cfg LearningConfig) *LearningRouter {
	if cfg.Epsilon <= 0 {
		cfg.Epsilon = 0.15
	}
	if cfg.MinSamples <= 0 {
		cfg.MinSamples = 3
	}
	if cfg.StatePath == "" {
		cfg.StatePath = defaultStatePath()
	}
	return &LearningRouter{
		cfg:    cfg,
		router: router,
		state: &LearningState{
			Models: make(map[string]*ModelStat),
		},
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// LoadLearningState 从文件加载状态。
// 文件不存在不算错，返回空状态。
func LoadLearningState(path string) (*LearningState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &LearningState{Models: make(map[string]*ModelStat)}, nil
		}
		return nil, fmt.Errorf("read learning state: %w", err)
	}
	var s LearningState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse learning state: %w", err)
	}
	if s.Models == nil {
		s.Models = make(map[string]*ModelStat)
	}
	return &s, nil
}

// Load 加载持久化状态到路由器
func (lr *LearningRouter) Load() error {
	state, err := LoadLearningState(lr.cfg.StatePath)
	if err != nil {
		return err
	}
	lr.mu.Lock()
	lr.state = state
	lr.mu.Unlock()
	return nil
}

// Save 持久化状态
func (lr *LearningRouter) Save() error {
	lr.mu.RLock()
	defer lr.mu.RUnlock()
	if lr.cfg.StatePath == "" {
		return nil
	}
	lr.state.LastUpdated = time.Now()
	data, err := json.MarshalIndent(lr.state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(lr.cfg.StatePath), 0755); err != nil {
		return err
	}
	return os.WriteFile(lr.cfg.StatePath, data, 0644)
}

// Route 学习型路由主入口。
// 流程：
//  1. L1 规则得档位（ClassifyTier）
//  2. 该档位无候选模型 → 退化为 L0 单一模型
//  3. 该档位有候选模型 → L2 在候选中选最优
func (lr *LearningRouter) Route(input string, fileCount int) string {
	if !lr.cfg.Enabled {
		return lr.router.Route(input, fileCount)
	}

	features := ExtractFeatures(input)
	tier := ClassifyTier(features)

	candidates := lr.cfg.CandidateModels[tier]
	if len(candidates) == 0 {
		// 无候选 → 退化 L0
		return lr.router.TierToModel(tier)
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	return lr.selectModel(tier, candidates)
}

// RouteWithFeatures 基于特征向量的学习型路由
func (lr *LearningRouter) RouteWithFeatures(f TaskFeatures) string {
	if !lr.cfg.Enabled {
		return lr.router.RouteWithFeatures(f)
	}

	tier := ClassifyTier(f)
	candidates := lr.cfg.CandidateModels[tier]
	if len(candidates) == 0 {
		return lr.router.TierToModel(tier)
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	return lr.selectModel(tier, candidates)
}

// selectModel 在同档位候选模型中选最优。
// 算法：
//  1. 冷启动期（某模型 samples < MinSamples）→ 优先选该模型
//  2. epsilon 概率随机探索
//  3. 否则选评分最高的（利用）
func (lr *LearningRouter) selectModel(tier Tier, candidates []string) string {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	// 1. 冷启动：选采样数最少的候选
	var coldStart string
	minSamples := math.MaxInt
	for _, name := range candidates {
		stat, ok := lr.state.Models[name]
		samples := 0
		if ok {
			samples = stat.Samples
		}
		if samples < lr.cfg.MinSamples {
			if samples < minSamples {
				minSamples = samples
				coldStart = name
			}
		}
	}
	if coldStart != "" {
		return coldStart
	}

	// 2. 探索：epsilon 概率随机
	if lr.rng.Float64() < lr.cfg.Epsilon {
		return candidates[lr.rng.Intn(len(candidates))]
	}

	// 3. 利用：选评分最高的
	bestName := candidates[0]
	bestScore := -1.0
	for _, name := range candidates {
		stat, ok := lr.state.Models[name]
		if !ok {
			// 无数据 → 优先选（避免卡住）
			return name
		}
		score := stat.Score()
		if score > bestScore {
			bestScore = score
			bestName = name
		}
	}
	return bestName
}

// RecordOutcome 记录一次模型调用的结果。
// success: true = 用户接受 / 无错误；false = 拒绝 / 错误
func (lr *LearningRouter) RecordOutcome(model string, tier Tier, tokens int, costUSD float64, success bool) {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	stat, ok := lr.state.Models[model]
	if !ok {
		stat = &ModelStat{Name: model, Tier: tier}
		lr.state.Models[model] = stat
	}
	stat.Samples++
	stat.TotalTokens += int64(tokens)
	stat.TotalCostUSD += costUSD
	if success {
		stat.Successes++
	}
	stat.LastUpdated = time.Now()
	lr.state.TotalDecisions++
}

// Stats 返回所有模型统计的副本（按 Score 降序）
func (lr *LearningRouter) Stats() []ModelStat {
	lr.mu.RLock()
	defer lr.mu.RUnlock()
	out := make([]ModelStat, 0, len(lr.state.Models))
	for _, s := range lr.state.Models {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score() > out[j].Score()
	})
	return out
}

// IsSignificantlyBetter 判断 A 模型是否显著优于 B（Wilson 95% CI 单侧检验）。
// 复用 ab 包思路但独立实现，避免循环依赖。
func IsSignificantlyBetter(aSuccess, aTrials, bSuccess, bTrials, minSamples int) bool {
	if aTrials < minSamples || bTrials < minSamples {
		return false
	}
	aLo, _ := wilsonInterval(aSuccess, aTrials)
	_, bHi := wilsonInterval(bSuccess, bTrials)
	return aLo > bHi
}

// wilsonInterval Wilson 95% 置信区间
func wilsonInterval(successes, trials int) (float64, float64) {
	if trials == 0 {
		return 0, 0
	}
	const z95 = 1.959963984540054
	n := float64(trials)
	p := float64(successes) / n
	denom := 1 + z95*z95/n
	center := (p + z95*z95/(2*n)) / denom
	margin := (z95 / denom) * math.Sqrt(p*(1-p)/n+z95*z95/(4*n*n))
	lo := center - margin
	hi := center + margin
	if lo < 0 {
		lo = 0
	}
	if hi > 1 {
		hi = 1
	}
	return lo, hi
}

// Config 返回配置副本
func (lr *LearningRouter) Config() LearningConfig {
	lr.mu.RLock()
	defer lr.mu.RUnlock()
	return lr.cfg
}

// SetEnabled 动态启用/禁用学习层
func (lr *LearningRouter) SetEnabled(enabled bool) {
	lr.mu.Lock()
	defer lr.mu.Unlock()
	lr.cfg.Enabled = enabled
}

// Router 返回底层 L0 路由器
func (lr *LearningRouter) Router() *ModelRouter {
	return lr.router
}
