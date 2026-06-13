// Package promptab 提供系统提示词的 A/B 测试与外部化能力。
//
// 设计目标：
//  1. 提示词可以"打包"为命名变体（variant），如 default / concise / safety-first
//  2. 变体可来自三处：
//     a. 编译时嵌入（fallback，保证永不失败）
//     b. 用户级 ~/.codecast/prompts/*.yaml（个人覆盖）
//     c. 项目级 .codecast/prompts/*.yaml（团队共享）
//  3. 运行时通过 flag / config / 环境变量选择 variant，缺省走 default
//  4. 选择事件可被埋点（OnSelect 回调）便于未来接 A/B 分析
//
// 典型用法：
//
//	reg := promptab.NewRegistry(promptab.EmbeddedVariants())
//	reg.LoadDir(filepath.Join(home, ".codecast", "prompts"))   // 覆盖
//	reg.LoadDir(".codecast/prompts")                            // 项目级
//	spec, _ := reg.Resolve("concise")
//	prompt := spec.Render(...)
package promptab

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Section 是提示词中的一个可独立切换段落。
// 不同 variant 可在不同 section 上有不同实现。
type Section struct {
	// Key 是 section 标识，如 "identity" / "tool_guide" / "anti_patterns" / "workflow" / "output_format"
	Key string
	// Body 是 Markdown 文本，支持 {{var}} 插值与 $ARGUMENTS。
	Body string
}

// Variant 是一个完整提示词变体，由多个 section 组成。
type Variant struct {
	// Name 唯一标识，如 "default" / "concise" / "safety-first"
	Name string `yaml:"name"`
	// Description 给人看的描述
	Description string `yaml:"description"`
	// Author 变体作者，便于溯源
	Author string `yaml:"author"`
	// Sections 按 Key 索引
	Sections map[string]Section `yaml:"-"`
	// RawSections 是 yaml.Unmarshal 的中间结果（map[string]string），
	// 由 Parse 转为 Sections。
	RawSections map[string]string `yaml:"sections"`
}

// Parse 把 Variant 的 RawSections 转为 Sections。
func (v *Variant) Parse() error {
	if v.RawSections == nil {
		v.Sections = map[string]Section{}
		return nil
	}
	v.Sections = make(map[string]Section, len(v.RawSections))
	for k, body := range v.RawSections {
		v.Sections[k] = Section{Key: k, Body: body}
	}
	return nil
}

// Get 取 section，找不到返回空字符串。
func (v *Variant) Get(key string) string {
	if v == nil || v.Sections == nil {
		return ""
	}
	if s, ok := v.Sections[key]; ok {
		return s.Body
	}
	return ""
}

// Registry 管理一组 variant，支持多目录加载与选择。
type Registry struct {
	mu       sync.RWMutex
	variants map[string]*Variant
	// loadOrder 记录加载顺序，便于 Resolve 时按"后加载覆盖先加载"处理
	loadOrder []string
	// onSelect 埋点回调，签名 (variantName string, source string)
	onSelect func(string, string)
}

// NewRegistry 构造空 registry。
func NewRegistry() *Registry {
	return &Registry{
		variants: make(map[string]*Variant),
	}
}

// SetOnSelect 设置选择埋点回调。可选。
func (r *Registry) SetOnSelect(fn func(variantName, source string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onSelect = fn
}

// Register 注册一个或多个 variant。同名后注册的覆盖先注册的。
// 静默跳过 nil 与空 Name 的项。
func (r *Registry) Register(vs ...*Variant) {
	for _, v := range vs {
		r.registerOne(v)
	}
}

func (r *Registry) registerOne(v *Variant) {
	if v == nil || v.Name == "" {
		return
	}
	if err := v.Parse(); err != nil {
		return // 静默跳过错误 variant（由 LoadDir 上报）
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.variants[v.Name]; !exists {
		r.loadOrder = append(r.loadOrder, v.Name)
	}
	r.variants[v.Name] = v
}

// LoadDir 从目录加载所有 .yaml/.yml 文件，每个文件一个 variant。
// 加载错误立即返回（不静默跳过），调用方决定如何处理。
func (r *Registry) LoadDir(dir string) error {
	if dir == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 目录不存在不算错（用户可能没自定义）
		}
		return fmt.Errorf("stat prompts dir %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("prompts path %q is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read prompts dir %q: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		v, err := LoadFile(path)
		if err != nil {
			return fmt.Errorf("load %s: %w", path, err)
		}
		r.Register(v)
	}
	return nil
}

// LoadFile 解析单个 YAML 文件。
func LoadFile(path string) (*Variant, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	v := &Variant{}
	if err := yaml.Unmarshal(data, v); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if v.Name == "" {
		return nil, fmt.Errorf("missing required field 'name'")
	}
	return v, nil
}

// Resolve 查找指定名称的 variant。返回 (variant, source)，
// source 为 "embedded" / "user" / "project"（粗略判断）。
// 找不到时返回错误（且不触发 onSelect 回调）。
func (r *Registry) Resolve(name string) (*Variant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.variants[name]
	if !ok {
		return nil, fmt.Errorf("variant %q not found; available: %v", name, r.Names())
	}
	if r.onSelect != nil {
		r.onSelect(name, "explicit")
	}
	return v, nil
}

// Names 返回所有已知 variant 名（按加载顺序）。
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.loadOrder))
	copy(out, r.loadOrder)
	sort.Strings(out) // 给人类看用稳定排序
	return out
}

// All 返回所有 variant（拷贝），便于列举。
func (r *Registry) All() []*Variant {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Variant, 0, len(r.loadOrder))
	for _, n := range r.loadOrder {
		if v, ok := r.variants[n]; ok {
			out = append(out, v)
		}
	}
	return out
}

// SelectStrategy 决定 ResolveWithStrategy 的选择逻辑。
type SelectStrategy int

const (
	// SelectFixed 指定名称（缺省 fallback 到 default）
	SelectFixed SelectStrategy = iota
	// SelectRoundRobin 按权重轮转
	SelectRoundRobin
	// SelectWeightedRandom 加权随机（不重复连续）
	SelectWeightedRandom
)

// Selector 是可序列化的选择配置，由 CLI / config 解析得到。
type Selector struct {
	Strategy SelectStrategy `yaml:"strategy"`
	// Fixed 仅在 SelectFixed 下生效
	Fixed string `yaml:"fixed,omitempty"`
	// Weights 仅在 SelectRoundRobin / SelectWeightedRandom 下生效
	// map[variant_name]weight，权重为正整数
	Weights map[string]int `yaml:"weights,omitempty"`
}

// ResolveWithStrategy 按策略选 variant。优先用 Selector.Fixed；缺省走 "default"。
// 选完后调用 onSelect 回调。
func (r *Registry) ResolveWithStrategy(sel Selector) (*Variant, string, error) {
	r.mu.RLock()
	available := append([]string(nil), r.loadOrder...)
	r.mu.RUnlock()
	sort.Strings(available)

	var chosen string
	switch sel.Strategy {
	case SelectFixed:
		chosen = sel.Fixed
		if chosen == "" {
			chosen = "default"
		}
	case SelectRoundRobin, SelectWeightedRandom:
		chosen = pickByWeight(sel.Weights, available, sel.Strategy == SelectWeightedRandom)
		if chosen == "" {
			chosen = "default"
		}
	default:
		chosen = "default"
	}

	v, err := r.Resolve(chosen)
	if err != nil {
		// fallback 到 default
		if defV, defErr := r.Resolve("default"); defErr == nil {
			if r.onSelect != nil {
				r.onSelect("default", "fallback")
			}
			return defV, "default", nil
		}
		return nil, "", err
	}
	return v, chosen, nil
}

// pickByWeight 是加权 / 轮转选择的辅助实现。
//
//   - weighted=true:  按权重概率选（轮转计数器+权重累积），用 stickyCounter 替代 math/rand
//     保证跨平台/可重现
//   - weighted=false: 严格轮转（按 available 顺序依次返回），不读 weights
//
// 用全局确定性时间无关的伪随机（基于上一次选择+轮转）保证行为可重现。
func pickByWeight(weights map[string]int, available []string, weighted bool) string {
	if len(available) == 0 {
		return ""
	}

	// 严格轮转：每次调用推进到下一个 candidate。
	// 与 weighted 共用 stickyCounter，行为同样可重现。
	if !weighted {
		idx := stickyCounter.Add(1) % uint64(len(available))
		return available[idx]
	}

	// 加权随机
	var total int
	for _, n := range available {
		if w, ok := weights[n]; ok && w > 0 {
			total += w
		}
	}
	if total <= 0 {
		return available[0]
	}
	idx := stickyCounter.Add(1) % uint64(total)
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
