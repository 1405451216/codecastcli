package splash

import (
	"testing"
	"time"
)

func TestDefaultSplash(t *testing.T) {
	s := DefaultSplash()
	if s == nil {
		t.Fatal("DefaultSplash returned nil")
	}
	if s.logo != "CODECAST" {
		t.Errorf("logo = %q, want %q", s.logo, "CODECAST")
	}
	if s.tagline != "AI-POWERED TERMINAL AGENT" {
		t.Errorf("tagline = %q", s.tagline)
	}
	if s.width < 40 {
		t.Errorf("width = %d, want >= 40", s.width)
	}
	if len(s.nodes) == 0 {
		t.Error("nodes should have at least one node")
	}
}

func TestSplashFinish(t *testing.T) {
	s := DefaultSplash()
	// Finish 应该不 panic
	s.Finish()
}

func TestSplashRunFor(t *testing.T) {
	s := DefaultSplash()
	// 跑 3 帧自动结束
	done := make(chan struct{})
	go func() {
		s.RunFor(3)
		close(done)
	}()
	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Error("RunFor didn't finish in 2 seconds")
	}
}

func TestSplashRender(t *testing.T) {
	s := DefaultSplash()
	// 渲染不应该 panic
	s.render(0)
	s.render(1)
	s.render(2)
}
