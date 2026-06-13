// Package ab 实现变体权重的自动收敛。
//
// 背景：Codecast 允许用户在 ~/.codecast/config.yaml 配置变体权重做 A/B 测试。
// 但人工调整权重很慢，且无理论保证。本包实现"在线学习"：
// 根据 cost.Tracker 的历史数据，自动调整权重让"看起来高效"的变体被更频繁选中。
//
// 算法：epsilon-greedy + 冷启动期
//   1. 冷启动期（每变体被选 < MinSamples 次）：均匀轮转，保证每个变体有数据
//   2. 冷启动后：epsilon 概率随机探索，(1-epsilon) 概率选当前评分最高的
//   3. 评分函数 score(v) = 1 / avg_cost(v) * success_bonus
//      —— 越便宜越好；成功调用加权更多
//
// 状态持久化：写入 ~/.codecast/ab_state.json，启动时读回。
package ab

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Config 收敛器配置
type Config struct {
	// Epsilon 探索率（0-1）。0 = 纯利用；1 = 纯探索。推荐 0.1
	Epsilon float64 `yaml:"epsilon"`
	// MinSamples 冷启动期每变体最少采样数。推荐 5
	MinSamples int `yaml:"min_samples"`
	// MinDecisions 总决策数门槛，低于此数一律均匀分布。推荐 0（让 MinSamples 控制）
	MinDecisions int `yaml:"min_decisions"`
	// StatePath 状态文件路径，默认 ~/.codecast/ab_state.json
	StatePath string `yaml:"state_path"`
	// MinWeight 下限权重（防止某变体权重归零）
	MinWeight int `yaml:"min_weight"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Epsilon:      0.1,
		MinSamples:   5,
		MinDecisions: 0,
		MinWeight:    1,
	}
}

// VariantStat 单个变体的统计
type VariantStat struct {
	// Name 变体名
	Name string
	// Samples 被选中次数
	Samples int
	// TotalCostUSD 累计成本
	TotalCostUSD float64
	// TotalTokens 累计 token
	TotalTokens int64
	// Successes 成功调用次数（用户接受 / 无错误）
	Successes int
	// LastUpdated 最后更新时间
	LastUpdated time.Time
}

// AvgCost 平均每次成本（0 表示无数据）
func (s *VariantStat) AvgCost() float64 {
	if s.Samples == 0 {
		return 0
	}
	return s.TotalCostUSD / float64(s.Samples)
}

// Score 评分（0-1 范围，越大越好）
// 综合考虑：成本倒数 + 成功率
func (s *VariantStat) Score() float64 {
	if s.Samples == 0 {
		return 0
	}
	avgCost := s.AvgCost()
	if avgCost <= 0 {
		return 0
	}
	// 基础分：成本倒数（USD 越少分越高）
	// 用 1/cost 而非 -cost，避免大数值
	baseScore := 1.0 / avgCost
	// 成功率加权
	successRate := float64(s.Successes) / float64(s.Samples)
	// 公式：baseScore * (0.5 + successRate*0.5)
	// 当 successRate=1，得分 = baseScore * 1.0
	// 当 successRate=0，得分 = baseScore * 0.5（不归零，避免永远不被选）
	weighted := baseScore * (0.5 + successRate*0.5)
	return weighted
}

// State 持久化状态
type State struct {
	// Config 持久化配置（修改后可重启生效）
	Config *Config `json:"config"`
	// Variants 每个变体的统计
	Variants map[string]*VariantStat `json:"variants"`
	// TotalDecisions 总决策次数
	TotalDecisions int `json:"total_decisions"`
	// LastUpdated 最后更新时间
	LastUpdated time.Time `json:"last_updated"`
}

// Converger 收敛器
type Converger struct {
	mu     sync.RWMutex
	config *Config
	state  *State
}

// Config 返回配置（只读语义；修改不影响已记录的 state）
// 真正修改需直接 Save
func (c *Converger) Config() *Config { return c.config }

// NewConverger 创建收敛器（不加载状态）
func NewConverger(cfg *Config) *Converger {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Converger{
		config: cfg,
		state: &State{
			Config:        cfg,
			Variants:      make(map[string]*VariantStat),
			TotalDecisions: 0,
			LastUpdated:   time.Now(),
		},
	}
}

// Load 从文件加载状态
func Load(path string) (*Converger, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// 文件不存在不算错，返回空状态
			return NewConverger(DefaultConfig()), nil
		}
		return nil, fmt.Errorf("read ab state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse ab state: %w", err)
	}
	if s.Config == nil {
		s.Config = DefaultConfig()
	}
	if s.Variants == nil {
		s.Variants = make(map[string]*VariantStat)
	}
	return &Converger{
		config: s.Config,
		state:  &s,
	}, nil
}

// Save 持久化到文件
func (c *Converger) Save() error {
	if c.config.StatePath == "" {
		return nil // 未配置路径，跳过
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	c.state.LastUpdated = time.Now()
	data, err := json.MarshalIndent(c.state, "", "  ")
	if err != nil {
		return err
	}
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(c.config.StatePath), 0755); err != nil {
		return err
	}
	return os.WriteFile(c.config.StatePath, data, 0644)
}

// RecordOutcome 记录一次变体调用的结果。
// success: true = 用户接受 / 无错误；false = 拒绝 / 错误
func (c *Converger) RecordOutcome(variant string, tokens int, costUSD float64, success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stat, ok := c.state.Variants[variant]
	if !ok {
		stat = &VariantStat{Name: variant}
		c.state.Variants[variant] = stat
	}
	stat.Samples++
	stat.TotalTokens += int64(tokens)
	stat.TotalCostUSD += costUSD
	if success {
		stat.Successes++
	}
	stat.LastUpdated = time.Now()
	c.state.TotalDecisions++
}

// CandidateSelector 根据当前状态选出下一个变体。
// 流程：
//   1. 冷启动期（某变体 samples < MinSamples）→ 选该变体
//   2. 否则以 epsilon 概率随机选（探索）
//   3. 否则选评分最高的（利用）
func (c *Converger) CandidateSelector(available []string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(available) == 0 {
		return ""
	}

	// 冷启动：找采样数最少的变体
	var coldStartCandidate string
	minSamples := math.MaxInt
	for _, name := range available {
		stat, ok := c.state.Variants[name]
		samples := 0
		if ok {
			samples = stat.Samples
		}
		if samples < c.config.MinSamples {
			if samples < minSamples {
				minSamples = samples
				coldStartCandidate = name
			}
		}
	}
	if coldStartCandidate != "" {
		return coldStartCandidate
	}

	// 探索：epsilon 概率随机
	if randFloat() < c.config.Epsilon {
		return available[randIntn(len(available))]
	}

	// 利用：选评分最高的
	bestName := available[0]
	bestScore := -1.0
	for _, name := range available {
		stat, ok := c.state.Variants[name]
		if !ok {
			// 没数据 → 优先选（避免冷启动后还卡住）
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

// ComputeWeights 根据当前状态计算建议权重。
// 算法：score 归一化 → 映射到 1-10 范围 → 应用 MinWeight 下限。
// 返回：map[variant]weight 整数。
func (c *Converger) ComputeWeights(available []string) map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	weights := make(map[string]int, len(available))
	if len(available) == 0 {
		return weights
	}

	// 收集分数
	scores := make(map[string]float64, len(available))
	var maxScore float64
	for _, name := range available {
		stat, ok := c.state.Variants[name]
		var s float64
		if ok {
			s = stat.Score()
		}
		// 没数据时给一个中位分数（0.5）
		if s == 0 {
			s = 0.5
		}
		scores[name] = s
		if s > maxScore {
			maxScore = s
		}
	}

	// 归一化到 1-10 范围，应用 MinWeight
	for _, name := range available {
		var w int
		if maxScore > 0 {
			normalized := scores[name] / maxScore
			// 线性映射：0.5 分 → 5 权重；1.0 分 → 10 权重
			w = int(math.Round(normalized * 10))
		}
		if w < c.config.MinWeight {
			w = c.config.MinWeight
		}
		weights[name] = w
	}
	return weights
}

// Suggest 一次性"建议下一个变体 + 当前权重"快照。
// 供 /ab show 展示。
func (c *Converger) Suggest(available []string) Suggestion {
	return Suggestion{
		NextVariant: c.CandidateSelector(available),
		Weights:     c.ComputeWeights(available),
	}
}

// Suggestion 单次推荐结果
type Suggestion struct {
	NextVariant string         `json:"next_variant"`
	Weights     map[string]int `json:"weights"`
}

// Report 生成人类可读的状态报告
func (c *Converger) Report(available []string) string {
	return c.ReportWithLatency(available, nil)
}

// ReportWithLatency 在 Report 基础上叠加 p50/p95 时延列。
// latency 为 nil 时省略时延列。
func (c *Converger) ReportWithLatency(available []string, latency *LatencyTracker) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sug := c.Suggest(available)
	// 按 score 降序排
	type row struct {
		name      string
		samples   int
		avgCost   float64
		success   int
		score     float64
		weight    int
		winLo     float64
		winHi     float64
		p50, p95  float64
	}
	rows := make([]row, 0, len(available))
	for _, name := range available {
		stat, ok := c.state.Variants[name]
		var samples int
		var avgCost, score float64
		var success int
		if ok {
			samples = stat.Samples
			avgCost = stat.AvgCost()
			success = stat.Successes
			score = stat.Score()
		}
		lo, hi := WilsonInterval(success, samples)
		r := row{
			name: name, samples: samples, avgCost: avgCost,
			success: success, score: score, weight: sug.Weights[name],
			winLo: lo, winHi: hi,
		}
		if latency != nil {
			ls := latency.Stats(name)
			r.p50 = ls.P50
			r.p95 = ls.P95
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].score > rows[j].score })

	out := fmt.Sprintf("A/B 收敛状态（epsilon=%.2f, 总决策=%d）\n",
		c.config.Epsilon, c.state.TotalDecisions)
	out += fmt.Sprintf("推荐下一个: %s\n\n", sug.NextVariant)

	if latency != nil {
		out += "变体          采样   胜率 (95%CI)            平均成本     p50时延  p95时延  评分  权重\n"
		out += "----------------------------------------------------------------------------------\n"
	} else {
		out += "变体          采样   胜率 (95%CI)            平均成本     评分  权重\n"
		out += "------------------------------------------------------------------------\n"
	}

	for _, r := range rows {
		ci := "-"
		if r.samples > 0 {
			ci = fmt.Sprintf("[%.0f%%, %.0f%%]", r.winLo*100, r.winHi*100)
		}
		scoreStr := "-"
		if r.score > 0 {
			scoreStr = fmt.Sprintf("%.3f", r.score)
		}
		costStr := "-"
		if r.avgCost > 0 {
			costStr = fmt.Sprintf("$%.4f", r.avgCost)
		}
		successStr := "-"
		if r.samples > 0 {
			successStr = fmt.Sprintf("%2d/%-2d", r.success, r.samples)
		}
		if latency != nil {
			p50 := "-"
			p95 := "-"
			if r.p50 > 0 {
				p50 = fmt.Sprintf("%.0fms", r.p50)
			}
			if r.p95 > 0 {
				p95 = fmt.Sprintf("%.0fms", r.p95)
			}
			out += fmt.Sprintf("%-12s  %4d   %-7s %-18s   %-12s  %-7s  %-7s  %-7s  %d\n",
				r.name, r.samples, successStr, ci, costStr, p50, p95, scoreStr, r.weight)
		} else {
			out += fmt.Sprintf("%-12s  %4d   %-7s %-18s   %-12s  %-7s  %d\n",
				r.name, r.samples, successStr, ci, costStr, scoreStr, r.weight)
		}
	}

	// 显著性提示：找出 best 与每个其他变体两两比较
	if len(rows) >= 2 {
		best := rows[0]
		notes := []string{}
		for _, r := range rows[1:] {
			if IsSignificantlyBetter(best.success, best.samples, r.success, r.samples, c.config.MinSamples) {
				notes = append(notes, fmt.Sprintf("↑ %s 显著优于 %s（CI 不重叠）", best.name, r.name))
			} else if best.samples >= c.config.MinSamples && r.samples >= c.config.MinSamples {
				notes = append(notes, fmt.Sprintf("~ %s vs %s 差异不显著（需更多样本）", best.name, r.name))
			}
		}
		if len(notes) > 0 {
			out += "\n结论:\n"
			for _, n := range notes {
				out += "  " + n + "\n"
			}
		}
	}
	return out
}

// Reset 清空所有状态（慎用）
func (c *Converger) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.Variants = make(map[string]*VariantStat)
	c.state.TotalDecisions = 0
	c.state.LastUpdated = time.Now()
}
