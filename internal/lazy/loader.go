package lazy

import (
	"fmt"
	"runtime"
	"sync"
)

// Value is a generic lazy value holder that defers construction until first access.
type Value[T any] struct {
	newFn  func() (T, error)
	value  T
	err    error
	loaded bool
	mu     sync.RWMutex
}

// NewValue creates a new lazy value that will call fn on first Get.
func NewValue[T any](fn func() (T, error)) *Value[T] {
	return &Value[T]{
		newFn: fn,
	}
}

// Get returns the value, calling the constructor on first access.
// Subsequent calls return the cached result.
func (v *Value[T]) Get() (T, error) {
	v.mu.RLock()
	if v.loaded {
		val, err := v.value, v.err
		v.mu.RUnlock()
		return val, err
	}
	v.mu.RUnlock()

	v.mu.Lock()
	defer v.mu.Unlock()

	// Double-check after acquiring write lock.
	if v.loaded {
		return v.value, v.err
	}

	v.value, v.err = v.newFn()
	v.loaded = true
	return v.value, v.err
}

// MustGet returns the value or panics if the constructor returns an error.
//
// 注意：此函数保留 panic 行为，因为：
//   1. 名字 MustGet 是显式 opt-in——调用方已选择"必须成功"语义
//   2. 用于 init-time 必须成功的资源（如 Provider）
//   3. 默认推荐使用 Get() 返回错误，让调用方处理
//
// 改进（v0.2.0）：panic 时携带调用方函数名（通过 runtime.Caller），
// 便于 debug 时定位失败源头。同时保留原 error 以兼容 recover() 断言。
func (v *Value[T]) MustGet() T {
	val, err := v.Get()
	if err != nil {
		pc, _, _, _ := runtime.Caller(1)
		funcName := runtime.FuncForPC(pc).Name()
		wrapped := fmt.Errorf("lazy.MustGet failed in %s: %w", funcName, err)
		panic(wrapped)
	}
	return val
}

// IsLoaded returns whether the value has been loaded.
func (v *Value[T]) IsLoaded() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.loaded
}

// Reset clears the cached value, forcing a reload on the next Get call.
func (v *Value[T]) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	var zero T
	v.value = zero
	v.err = nil
	v.loaded = false
}

// Group manages a collection of named lazy values for batch initialization.
type Group struct {
	values map[string]interface{}
	mu     sync.Mutex
}

// NewGroup creates a new lazy value group.
func NewGroup() *Group {
	return &Group{
		values: make(map[string]interface{}),
	}
}

// Register adds a named lazy value to the group.
func (g *Group) Register(name string, fn func() (interface{}, error)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.values[name] = NewValue[interface{}](fn)
}

// Load loads a specific named value from the group.
func (g *Group) Load(name string) (interface{}, error) {
	g.mu.Lock()
	v, ok := g.values[name]
	g.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("lazy: value %q not registered", name)
	}

	return v.(*Value[interface{}]).Get()
}

// LoadAll loads all values in the group in parallel using goroutines.
func (g *Group) LoadAll() error {
	g.mu.Lock()
	names := make([]string, 0, len(g.values))
	vals := make([]*Value[interface{}], 0, len(g.values))
	for name, v := range g.values {
		names = append(names, name)
		vals = append(vals, v.(*Value[interface{}]))
	}
	g.mu.Unlock()

	var wg sync.WaitGroup
	errCh := make(chan error, len(vals))

	for i := range vals {
		wg.Add(1)
		go func(v *Value[interface{}]) {
			defer wg.Done()
			if _, err := v.Get(); err != nil {
				errCh <- err
			}
		}(vals[i])
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// LoadRequired loads only the specified named values in parallel.
func (g *Group) LoadRequired(names ...string) error {
	g.mu.Lock()
	vals := make([]*Value[interface{}], 0, len(names))
	for _, name := range names {
		v, ok := g.values[name]
		if !ok {
			g.mu.Unlock()
			return fmt.Errorf("lazy: value %q not registered", name)
		}
		vals = append(vals, v.(*Value[interface{}]))
	}
	g.mu.Unlock()

	var wg sync.WaitGroup
	errCh := make(chan error, len(vals))

	for i := range vals {
		wg.Add(1)
		go func(v *Value[interface{}]) {
			defer wg.Done()
			if _, err := v.Get(); err != nil {
				errCh <- err
			}
		}(vals[i])
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}
