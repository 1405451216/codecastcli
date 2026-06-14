package routing

import (
	"strings"
)

// RoutingConfig 智能模型路由配置
type RoutingConfig struct {
	// SimpleModel 用于简单任务（问答、解释）
	SimpleModel string `yaml:"simple_model"`
	// MediumModel 用于中等任务（单文件编辑）
	MediumModel string `yaml:"medium_model"`
	// ComplexModel 用于复杂任务（多文件重构、架构设计）
	ComplexModel string `yaml:"complex_model"`
	// Enabled 是否启用智能路由
	Enabled bool `yaml:"enabled"`
}

// DefaultRoutingConfig 返回合理的默认路由配置
func DefaultRoutingConfig() RoutingConfig {
	return RoutingConfig{
		SimpleModel:  "gpt-4o-mini",
		MediumModel:  "gpt-4o",
		ComplexModel: "claude-opus-4",
		Enabled:      true,
	}
}

// ModelRouter 智能模型路由器，根据输入复杂度自动选择合适的模型
type ModelRouter struct {
	cfg RoutingConfig
}

// NewModelRouter 创建模型路由器
func NewModelRouter(cfg RoutingConfig) *ModelRouter {
	return &ModelRouter{cfg: cfg}
}

// IsEnabled 返回路由器是否启用
func (r *ModelRouter) IsEnabled() bool {
	return r.cfg.Enabled
}

// SetEnabled 动态启用或禁用路由器
func (r *ModelRouter) SetEnabled(enabled bool) {
	r.cfg.Enabled = enabled
}

// Config 返回当前路由配置的副本
func (r *ModelRouter) Config() RoutingConfig {
	return r.cfg
}

// Route 根据输入内容和文件数量选择合适的模型
func (r *ModelRouter) Route(input string, fileCount int) string {
	if !r.cfg.Enabled {
		return r.cfg.MediumModel
	}

	score := r.complexityScore(input, fileCount)

	switch {
	case score >= 5:
		return r.cfg.ComplexModel
	case score >= 2:
		return r.cfg.MediumModel
	default:
		return r.cfg.SimpleModel
	}
}

// complexityKeywords 触发复杂度加分的中文和英文关键词
var complexityKeywords = []string{
	"重构", "架构", "迁移", "refactor", "design", "migrate", "rewrite",
}

// complexityScore 计算输入的复杂度分数
func (r *ModelRouter) complexityScore(input string, fileCount int) int {
	score := 0

	// 输入长度评分
	inputLen := len(input)
	if inputLen > 200 {
		score += 2
	}
	if inputLen > 500 {
		score += 1
	}

	// 关键词评分
	lower := strings.ToLower(input)
	for _, kw := range complexityKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			score += 2
		}
	}

	// 文件引用数量评分
	if fileCount > 3 {
		score += 2
	}
	if fileCount > 8 {
		score += 2
	}

	return score
}
