package agent

// router_cache.go: 任务感知路由的轻量封装。
//
// 设计动机：
//   - promptab.Router 加载 YAML 规则较重（每次 Process 都重 load 不划算）
//   - 缓存 router 实例 + 在 RefreshConfig 时重载
//   - 提供 SelectForInput(input, hasTools) 一行调用

import (
	"sync"

	"codecast/cli/internal/promptab"
)

// RouterCache 缓存 promptab.Router，避免每轮 Process 重新加载。
// 线程安全：使用 sync.RWMutex。
type RouterCache struct {
	mu     sync.RWMutex
	router *promptab.Router
}

// NewRouterCache 构造缓存（首次 SelectForInput 时懒初始化 router）。
func NewRouterCache() *RouterCache {
	return &RouterCache{}
}

// Router 返回内部 router（懒初始化）。
func (c *RouterCache) Router() *promptab.Router {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.router == nil {
		c.router = promptab.NewDefaultRouter()
	}
	return c.router
}

// Reload 重新构造 router（用户在 /prompt reload 时调用）。
func (c *RouterCache) Reload() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.router = promptab.NewDefaultRouter()
}

// SelectForInput 根据 strategy 决策变体名。
//
// strategy 取值：
//   - "" / "fixed" / "weighted" / "round-robin"：与 cfg.PromptVariant / 现有权重策略一致
//   - "routed" / "task-aware"：用 router 做 L1/L2 决策
//
// 返回：选中的变体名 + 决策来源（l1:/l2:/weighted:/fallback）。
// available：当前所有可用变体名（用来校验 Rule.Variant 存在）。
func (c *RouterCache) SelectForInput(strategy, fixedVariant string, weights map[string]int, userInput string, hasTools bool, available []string) (string, string) {
	if strategy == "routed" || strategy == "task-aware" || strategy == "router" {
		rt := c.Router()
		dec := rt.Route(promptab.RouteInput{
			UserInput: userInput,
			HasTools:  hasTools,
			Available: available,
		})
		if dec.Variant != "" {
			return dec.Variant, dec.Source
		}
		// 路由未命中 → 回落：走 weighted / fixed
	}
	// 兜底：用 cfg.PromptVariant 或 default
	if fixedVariant != "" {
		return fixedVariant, "fixed"
	}
	if len(weights) > 0 {
		// 用 stickyCounter 加权（与 promptab.pickByWeight 行为一致）
		return pickByWeights(weights, available), "weighted"
	}
	return "default", "fallback"
}

// pickByWeights 是 promptab.pickByWeight 的复制（避免循环依赖）。
func pickByWeights(weights map[string]int, available []string) string {
	if len(available) == 0 {
		return "default"
	}
	var total int
	for _, n := range available {
		if w, ok := weights[n]; ok && w > 0 {
			total += w
		}
	}
	if total <= 0 {
		return available[0]
	}
	idx := promptab.StickyCounterNext() % uint64(total)
	var cum int
	for _, n := range available {
		w, ok := weights[n]
		if !ok || w <= 0 {
			continue
		}
		if idx < uint64(cum+w) {
			return n
		}
		cum += w
	}
	return available[0]
}
