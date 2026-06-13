package cmd

import (
	"codecast/cli/internal/agent"

	"github.com/c-bata/go-prompt"
)

// CommandHandler 是斜杠命令的统一处理器签名。
//
// 参数:
//   - args: 去掉命令名后的剩余字符串（已 TrimSpace）
//   - ag: 当前活动的 CodecastAgent 实例（可能为 nil）
//
// 返回:
//   - bool: 是否消费了此命令（true 表示不再走 Agent 流程）
//
// 注意：现有 interactive.go 中的 handle*Command 函数签名是
// `func(args string, ag *agent.CodecastAgent)`（无返回），需要适配。
// 在 commands.go 中通过 wrapper 函数（以 handle* 命名）调用。
type CommandHandler = func(args string, ag *agent.CodecastAgent) (handled bool)

// CommandEntry 是注册表中的单个命令定义。
type CommandEntry struct {
	// Name 不含前导 "/"，例如 "config"
	Name string
	// Aliases 别名（不含前导 "/"）
	Aliases []string
	// Description 在补全弹窗中显示
	Description string
	// Handler 实际处理器
	Handler CommandHandler
}

// CommandRegistry 集中管理所有斜杠命令。
//
// 优点：
//   - 单一来源：补全、调度、帮助都从同一处读取
//   - 去重：避免之前 commandSuggestions 有重复条目（/sandbox 出现两次）
//   - 易于测试：可以 mock registry 验证调度
type CommandRegistry struct {
	entries map[string]*CommandEntry
	order   []string // 用于稳定的补全顺序
}

// NewCommandRegistry 创建空注册表
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		entries: make(map[string]*CommandEntry),
	}
}

// Register 注册一个斜杠命令
func (r *CommandRegistry) Register(e *CommandEntry) {
	if e == nil || e.Name == "" {
		return
	}
	r.entries[e.Name] = e
	if !contains(r.order, e.Name) {
		r.order = append(r.order, e.Name)
	}
	for _, alias := range e.Aliases {
		if alias == "" {
			continue
		}
		r.entries[alias] = e
	}
}

// Lookup 按命令名（不含 "/"）查找
func (r *CommandRegistry) Lookup(name string) (*CommandEntry, bool) {
	e, ok := r.entries[name]
	return e, ok
}

// All 按注册顺序返回所有独立命令（不展开别名）
func (r *CommandRegistry) All() []*CommandEntry {
	out := make([]*CommandEntry, 0, len(r.order))
	seen := make(map[*CommandEntry]bool)
	for _, name := range r.order {
		e := r.entries[name]
		if seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

// Suggestions 返回 go-prompt 补全建议列表
func (r *CommandRegistry) Suggestions() []prompt.Suggest {
	all := r.All()
	out := make([]prompt.Suggest, 0, len(all))
	for _, e := range all {
		out = append(out, prompt.Suggest{
			Text:        "/" + e.Name,
			Description: e.Description,
		})
	}
	return out
}

// Dispatch 解析 "input" 并分发给对应 handler。
// 返回 true 表示已消费（不进入 Agent 处理）；false 表示不是斜杠命令或 handler 让出控制。
//
// 调度规则：
//   - input 必须以 "/" 开头，否则返回 false
//   - 解析出 command + args（args 已 TrimSpace）
//   - 通过 registry.Lookup 查找 handler
//   - 调用 handler(args, ag)
func (r *CommandRegistry) Dispatch(input string, ag *agent.CodecastAgent) bool {
	if len(input) == 0 || input[0] != '/' {
		return false
	}
	// 切出命令名
	name := input[1:]
	rest := ""
	for i := 0; i < len(name); i++ {
		if name[i] == ' ' || name[i] == '\t' {
			rest = name[i+1:]
			name = name[:i]
			break
		}
	}
	entry, ok := r.Lookup(name)
	if !ok {
		return false
	}
	return entry.Handler(rest, ag)
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
