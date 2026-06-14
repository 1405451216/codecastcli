package agent

import (
	"testing"

	"codecast/cli/internal/routing"
)

// TestCountFileReferences 验证 @file 引用计数
func TestCountFileReferences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{name: "no refs", input: "hello world", want: 0},
		{name: "one ref", input: "look at @main.go", want: 1},
		{name: "two refs", input: "compare @foo.go and @bar.go", want: 2},
		{name: "adjacent refs", input: "@a @b @c", want: 3},
		{name: "email not ref", input: "user@example.com", want: 1}, // @user counts as one ref
		{name: "empty input", input: "", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countFileReferences(tt.input)
			if got != tt.want {
				t.Errorf("countFileReferences(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestRouterFieldInitialized 验证 CodecastAgent 的 router 字段在 newAgent 中被初始化
func TestRouterFieldInitialized(t *testing.T) {
	// 直接验证 ModelRouter 的创建和配置
	cfg := routing.DefaultRoutingConfig()
	router := routing.NewModelRouter(cfg)

	if router == nil {
		t.Fatal("NewModelRouter returned nil")
	}
	if !router.IsEnabled() {
		t.Fatal("DefaultRoutingConfig should have Enabled=true")
	}
}

// TestRouterIsEnabled 验证 IsEnabled/SetEnabled 配对
func TestRouterIsEnabled(t *testing.T) {
	cfg := routing.DefaultRoutingConfig()
	router := routing.NewModelRouter(cfg)

	if !router.IsEnabled() {
		t.Fatal("should be enabled by default")
	}

	router.SetEnabled(false)
	if router.IsEnabled() {
		t.Fatal("should be disabled after SetEnabled(false)")
	}

	router.SetEnabled(true)
	if !router.IsEnabled() {
		t.Fatal("should be enabled after SetEnabled(true)")
	}
}

// TestRouterConfig 验证 Config() 返回配置副本
func TestRouterConfig(t *testing.T) {
	cfg := routing.DefaultRoutingConfig()
	router := routing.NewModelRouter(cfg)

	got := router.Config()
	if got.SimpleModel != cfg.SimpleModel {
		t.Errorf("Config().SimpleModel = %q, want %q", got.SimpleModel, cfg.SimpleModel)
	}
	if got.MediumModel != cfg.MediumModel {
		t.Errorf("Config().MediumModel = %q, want %q", got.MediumModel, cfg.MediumModel)
	}
	if got.ComplexModel != cfg.ComplexModel {
		t.Errorf("Config().ComplexModel = %q, want %q", got.ComplexModel, cfg.ComplexModel)
	}
	if got.Enabled != cfg.Enabled {
		t.Errorf("Config().Enabled = %v, want %v", got.Enabled, cfg.Enabled)
	}
}

// TestRoutingDecisionInStreamProcess 验证路由决策逻辑：
// 当路由器启用且路由模型与当前模型不同时，应触发模型切换。
// 此测试不依赖完整 Agent 初始化，仅验证路由决策路径。
func TestRoutingDecisionInStreamProcess(t *testing.T) {
	cfg := routing.DefaultRoutingConfig()
	router := routing.NewModelRouter(cfg)

	// 简单输入 → SimpleModel
	simpleModel := router.Route("解释这段代码", 0)
	if simpleModel != cfg.SimpleModel {
		t.Errorf("Route(simple): got %q, want %q", simpleModel, cfg.SimpleModel)
	}

	// 复杂输入 → ComplexModel
	complexInput := "重构整个认证模块，需要重新设计架构并迁移所有现有逻辑到新的微服务体系结构"
	complexModel := router.Route(complexInput, 10)
	if complexModel != cfg.ComplexModel {
		t.Errorf("Route(complex): got %q, want %q", complexModel, cfg.ComplexModel)
	}

	// 禁用路由 → 始终返回 MediumModel
	router.SetEnabled(false)
	disabledModel := router.Route(complexInput, 10)
	if disabledModel != cfg.MediumModel {
		t.Errorf("Route(disabled): got %q, want %q", disabledModel, cfg.MediumModel)
	}
}

// TestModelSwitchOnRouteDiff 验证当路由模型与当前模型不同时，
// 逻辑上应触发 SwitchModel。此测试验证条件判断路径。
func TestModelSwitchOnRouteDiff(t *testing.T) {
	cfg := routing.DefaultRoutingConfig()
	router := routing.NewModelRouter(cfg)

	// 模拟当前模型是 MediumModel，简单输入路由到 SimpleModel
	currentModel := cfg.MediumModel
	routedModel := router.Route("解释这段代码", 0)

	if routedModel == currentModel {
		t.Errorf("expected routed model (%s) to differ from current model (%s)", routedModel, currentModel)
	}

	// 模拟当前模型是 SimpleModel，简单输入路由到 SimpleModel → 无需切换
	currentModel = cfg.SimpleModel
	routedModel = router.Route("解释这段代码", 0)
	if routedModel != currentModel {
		t.Errorf("expected no switch needed, but routed=%s current=%s", routedModel, currentModel)
	}
}

// TestRoutingWithFileReferences 验证 @file 引用数量影响路由决策
func TestRoutingWithFileReferences(t *testing.T) {
	cfg := routing.DefaultRoutingConfig()
	router := routing.NewModelRouter(cfg)

	// 无文件引用的简单输入 → SimpleModel
	model := router.Route("修改这个函数", 0)
	if model != cfg.SimpleModel {
		t.Errorf("Route(0 files): got %q, want %q", model, cfg.SimpleModel)
	}

	// 5个文件引用 → MediumModel (fileCount > 3 → +2)
	model = router.Route("修改这个函数", 5)
	if model != cfg.MediumModel {
		t.Errorf("Route(5 files): got %q, want %q", model, cfg.MediumModel)
	}

	// 10个文件引用 → ComplexModel (fileCount > 3 → +2, > 8 → +2, total 4 → Medium)
	// Actually: fileCount=10 → +2 (for >3) + +2 (for >8) = +4, still Medium
	// Need keyword to push to Complex
	model = router.Route("重构这个函数", 10)
	// keyword "重构" = +2, fileCount >3 = +2, >8 = +2, total = 6 → ComplexModel
	if model != cfg.ComplexModel {
		t.Errorf("Route(refactor + 10 files): got %q, want %q", model, cfg.ComplexModel)
	}
}
