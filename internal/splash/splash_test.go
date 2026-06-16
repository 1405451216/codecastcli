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
	if s.tag != "AI-POWERED TERMINAL AGENT" {
		t.Errorf("tag = %q", s.tag)
	}
	if s.width < 40 {
		t.Errorf("width = %d, want >= 40", s.width)
	}
	if len(s.stars) == 0 {
		t.Error("stars should have at least one star")
	}
}

func TestSplashFinish(t *testing.T) {
	s := DefaultSplash()
	s.Finish()
}

func TestSplashRunFor(t *testing.T) {
	s := DefaultSplash()
	done := make(chan struct{})
	go func() {
		s.RunFor(5)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("RunFor didn't finish in 2 seconds")
	}
}

func TestSplashRender(t *testing.T) {
	s := DefaultSplash()
	s.render(0)
	s.render(1)
	s.render(5)
	s.render(10)
	s.render(20)
}

func TestPrintBanner(t *testing.T) {
	PrintBanner("v0.4.0")
}
