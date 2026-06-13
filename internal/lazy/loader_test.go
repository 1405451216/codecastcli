package lazy

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

func TestValue_Get_Basic(t *testing.T) {
	callCount := int32(0)
	v := NewValue(func() (string, error) {
		atomic.AddInt32(&callCount, 1)
		return "hello", nil
	})

	val, err := v.Get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "hello" {
		t.Fatalf("expected 'hello', got %q", val)
	}
	if callCount != 1 {
		t.Fatalf("expected constructor called once, got %d", callCount)
	}

	// 第二次调用应使用缓存
	val2, err := v.Get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val2 != "hello" {
		t.Fatalf("expected 'hello' from cache, got %q", val2)
	}
	if callCount != 1 {
		t.Fatalf("constructor should not be called again, got %d calls", callCount)
	}
}

func TestValue_Get_Error(t *testing.T) {
	v := NewValue(func() (string, error) {
		return "", errors.New("oops")
	})
	_, err := v.Get()
	if err == nil || err.Error() != "oops" {
		t.Fatalf("expected 'oops' error, got %v", err)
	}
}

func TestMustGet_Panic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		// v0.2.0: panic 现在是 wrapped error, 验证包含原 error
		err, ok := r.(error)
		if !ok {
			t.Fatalf("expected error type, got %T", r)
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Fatalf("expected wrapped 'boom' error, got %v", err)
		}
		// 验证错误信息包含调用方函数名（用于 debug）
		if !strings.Contains(err.Error(), "TestMustGet_Panic") {
			t.Errorf("error should contain caller function name, got %q", err.Error())
		}
	}()

	v := NewValue(func() (string, error) {
		return "", errors.New("boom")
	})
	v.MustGet()
}

func TestValue_Reset(t *testing.T) {
	callCount := int32(0)
	v := NewValue(func() (string, error) {
		atomic.AddInt32(&callCount, 1)
		return "value", nil
	})

	_, _ = v.Get()
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}

	v.Reset()
	_, _ = v.Get()
	if callCount != 2 {
		t.Fatalf("expected 2 calls after reset, got %d", callCount)
	}
}

func TestValue_Concurrent(t *testing.T) {
	v := NewValue(func() (int, error) {
		return 42, nil
	})

	const goroutines = 100
	done := make(chan bool, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			val, err := v.Get()
			if err != nil || val != 42 {
				t.Errorf("got (%d, %v)", val, err)
			}
			done <- true
		}()
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}
}
