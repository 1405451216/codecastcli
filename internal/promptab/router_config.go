package promptab

// router_config.go: 把 Router 规则从 YAML 加载的桥接层。
//
// YAML schema:
//
//	rules:
//	  - name: my-rule
//	    variant: default
//	    priority: 80
//	    description: "我的自定义规则"
//	    keywords: ["foo", "bar"]
//	complexity:
//	  long_task_chars: 250
//	  short_question_chars: 40
//	  has_tool_hint: ["重构", "refactor"]
//
// 文件查找顺序（先找到的优先，更具体 → 更通用 → 兜底）：
//   1. --prompt-routing <path> 指定的文件
//   2. .codecast/prompts/routing.yaml（项目级）
//   3. ~/.codecast/prompts/routing.yaml（用户级）
//   4. 编译时嵌入的默认规则

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// routingConfigFile 是 routing.yaml 的反序列化目标。
type routingConfigFile struct {
	Rules      []*RouteRule        `yaml:"rules"`
	Complexity *ComplexityConfig   `yaml:"complexity"`
}

// LoadRoutingFromFile 从单一 YAML 文件加载（追加模式，不清空已有）。
// 文件不存在 → 返回 nil（不报错，调用方可以安全调用）。
// YAML 解析错误 → 返回 error。
func (r *Router) LoadRoutingFromFile(path string) error {
	if r == nil || path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read routing file %q: %w", path, err)
	}
	return r.parseRoutingYAML(data)
}

// LoadRoutingFromDir 扫描目录里的 routing*.yaml（按文件名排序）。
func (r *Router) LoadRoutingFromDir(dir string) error {
	if r == nil || dir == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		// 只匹配 routing*.yaml
		if len(name) < 7 || name[:7] != "routing" {
			continue
		}
		path := filepath.Join(dir, name)
		if err := r.LoadRoutingFromFile(path); err != nil {
			return err
		}
	}
	return nil
}

// parseRoutingYAML 解析 YAML 字节流并应用到 router。
func (r *Router) parseRoutingYAML(data []byte) error {
	var cfg routingConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse routing yaml: %w", err)
	}
	if len(cfg.Rules) > 0 {
		// 给 YAML 规则分配 index（保留 YAML 顺序）
		for i, rule := range cfg.Rules {
			if rule == nil {
				continue
			}
			rule.Index = i
		}
		r.LoadRules(cfg.Rules)
	}
	if cfg.Complexity != nil {
		r.SetComplexityConfig(*cfg.Complexity)
	}
	return nil
}

// NewDefaultRouter 构造带默认规则的 router。
// 这是推荐用法：先嵌入默认 → 再叠加用户/项目级 YAML。
func NewDefaultRouter() *Router {
	r := NewRouter()
	r.LoadRules(DefaultRouteRules())
	return r
}
