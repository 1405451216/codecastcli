package routing

import (
	"strings"
	"testing"
)

func TestDefaultRoutingConfig(t *testing.T) {
	cfg := DefaultRoutingConfig()

	if cfg.SimpleModel != "gpt-4o-mini" {
		t.Errorf("SimpleModel: got %q, want %q", cfg.SimpleModel, "gpt-4o-mini")
	}
	if cfg.MediumModel != "gpt-4o" {
		t.Errorf("MediumModel: got %q, want %q", cfg.MediumModel, "gpt-4o")
	}
	if cfg.ComplexModel != "claude-opus-4" {
		t.Errorf("ComplexModel: got %q, want %q", cfg.ComplexModel, "claude-opus-4")
	}
	if !cfg.Enabled {
		t.Errorf("Enabled: got false, want true")
	}
}

func TestRouteSimple(t *testing.T) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)

	model := router.Route("解释这段代码", 0)
	if model != cfg.SimpleModel {
		t.Errorf("Route(%q): got %q, want %q", "解释这段代码", model, cfg.SimpleModel)
	}
}

func TestRouteComplex(t *testing.T) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)

	// "重构" keyword gives +2, "架构" gives +2, total = 4 → still MediumModel (needs >= 5 for Complex)
	// Add enough length to push score to >= 5
	longInput := "重构整个认证模块，需要重新设计架构并迁移所有现有逻辑到新的微服务体系结构"
	model := router.Route(longInput, 0)
	if model != cfg.ComplexModel {
		t.Errorf("Route(complex input): got %q, want %q", model, cfg.ComplexModel)
	}

	// Verify that just "重构" alone scores only 2 → MediumModel
	model = router.Route("重构这个函数", 0)
	if model != cfg.MediumModel {
		t.Errorf("Route(single keyword): got %q, want %q", model, cfg.MediumModel)
	}
}

func TestRouteDisabled(t *testing.T) {
	cfg := DefaultRoutingConfig()
	cfg.Enabled = false
	router := NewModelRouter(cfg)

	// Even with a complex prompt, should return MediumModel when disabled
	model := router.Route("重构整个认证模块", 10)
	if model != cfg.MediumModel {
		t.Errorf("Route when disabled: got %q, want %q", model, cfg.MediumModel)
	}

	// Simple prompt also returns MediumModel when disabled
	model = router.Route("解释这段代码", 0)
	if model != cfg.MediumModel {
		t.Errorf("Route when disabled (simple): got %q, want %q", model, cfg.MediumModel)
	}
}

func TestComplexityScore(t *testing.T) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)

	tests := []struct {
		name      string
		input     string
		fileCount int
		wantMin   int
		wantMax   int
	}{
		{
			name:      "short input no keywords",
			input:     "hi",
			fileCount: 0,
			wantMin:   0,
			wantMax:   0,
		},
		{
			name:      "long input adds score",
			input:     strings.Repeat("x", 300),
			fileCount: 0,
			wantMin:   2,
			wantMax:   3,
		},
		{
			name:      "very long input adds more score",
			input:     strings.Repeat("x", 600),
			fileCount: 0,
			wantMin:   3,
			wantMax:   3,
		},
		{
			name:      "keyword adds score",
			input:     "请重构这个函数",
			fileCount: 0,
			wantMin:   2,
			wantMax:   2,
		},
		{
			name:      "multiple keywords add score",
			input:     "重构并迁移架构",
			fileCount: 0,
			wantMin:   6,
			wantMax:   6,
		},
		{
			name:      "many files adds score",
			input:     "fix bug",
			fileCount: 10,
			wantMin:   4,
			wantMax:   4,
		},
		{
			name:      "moderate files adds score",
			input:     "fix bug",
			fileCount: 5,
			wantMin:   2,
			wantMax:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := router.complexityScore(tt.input, tt.fileCount)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("complexityScore(%q, %d) = %d, want between %d and %d",
					tt.input, tt.fileCount, score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestRouteMedium(t *testing.T) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)

	// A medium-complexity prompt: no keywords, but a few files
	model := router.Route("修改这个函数的逻辑", 4)
	if model != cfg.MediumModel {
		t.Errorf("Route medium: got %q, want %q", model, cfg.MediumModel)
	}
}
