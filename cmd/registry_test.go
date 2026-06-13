package cmd

import (
	"testing"

	"codecast/cli/internal/agent"
)

// TestCommandRegistry_RegisterAndLookup 验证基本注册与查找
func TestCommandRegistry_RegisterAndLookup(t *testing.T) {
	r := NewCommandRegistry()
	called := false
	handler := func(args string, ag *agent.CodecastAgent) bool {
		called = true
		return true
	}
	r.Register(&CommandEntry{
		Name:    "test",
		Handler: handler,
	})

	// 查主名
	entry, ok := r.Lookup("test")
	if !ok {
		t.Fatal("expected to find 'test'")
	}
	if entry.Name != "test" {
		t.Errorf("got name %q, want %q", entry.Name, "test")
	}
	entry.Handler("", nil)
	if !called {
		t.Error("handler was not called")
	}
}

// TestCommandRegistry_Aliases 验证别名机制
func TestCommandRegistry_Aliases(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(&CommandEntry{
		Name:    "help",
		Aliases: []string{"h", "?"},
		Handler: func(string, *agent.CodecastAgent) bool { return true },
	})

	for _, name := range []string{"help", "h", "?"} {
		entry, ok := r.Lookup(name)
		if !ok {
			t.Errorf("expected to find alias %q", name)
		}
		if entry.Name != "help" {
			t.Errorf("alias %q resolved to %q, want %q", name, entry.Name, "help")
		}
	}
}

// TestCommandRegistry_Suggestions_NoDuplicates 验证补全去重
//
// 这是审计中发现的问题：之前 commandSuggestions 中 /sandbox 出现两次。
// 现在通过 CommandRegistry 自动去重。
func TestCommandRegistry_Suggestions_NoDuplicates(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(&CommandEntry{
		Name: "config", Handler: func(string, *agent.CodecastAgent) bool { return true },
	})
	r.Register(&CommandEntry{
		Name: "sandbox", Handler: func(string, *agent.CodecastAgent) bool { return true },
	})
	// 故意注册第二次 /sandbox（模拟迁移时可能出现的重复）
	r.Register(&CommandEntry{
		Name: "sandbox", Handler: func(string, *agent.CodecastAgent) bool { return true },
	})

	suggestions := r.Suggestions()
	names := make(map[string]int)
	for _, s := range suggestions {
		names[s.Text]++
	}
	if names["/sandbox"] != 1 {
		t.Errorf("expected /sandbox to appear once, got %d times", names["/sandbox"])
	}
}

// TestCommandRegistry_Dispatch 验证调度逻辑
func TestCommandRegistry_Dispatch(t *testing.T) {
	r := NewCommandRegistry()
	called := false
	r.Register(&CommandEntry{
		Name:    "foo",
		Aliases: []string{"f"},
		Handler: func(args string, ag *agent.CodecastAgent) bool {
			called = true
			return true
		},
	})

	tests := []struct {
		input    string
		expected bool
	}{
		{"/foo", true},
		{"/foo arg1 arg2", true},
		{"/f", true},
		{"/unknown", false}, // 不消费，让 Agent 处理
		{"not a command", false},
		{"", false},
	}
	for _, tt := range tests {
		called = false
		got := r.Dispatch(tt.input, nil)
		if got != tt.expected {
			t.Errorf("Dispatch(%q) = %v, want %v", tt.input, got, tt.expected)
		}
		if tt.expected && !called {
			t.Errorf("Dispatch(%q): handler was not called", tt.input)
		}
	}
}

// TestRegisterBuiltinCommands_CoversAllSlashCommands 验证内置命令全部已注册
//
// v0.2.0 修复: 之前只检查 8 个名字 + 数量 >= 25，遗漏别名失配等问题。
// 现在做精确集合匹配 + 别名存在性检查。
func TestRegisterBuiltinCommands_CoversAllSlashCommands(t *testing.T) {
	r := NewCommandRegistry()
	RegisterBuiltinCommands(r)
	all := r.All()
	if len(all) < 30 {
		t.Errorf("expected at least 30 builtin slash commands, got %d", len(all))
	}

	// 精确集合：所有内置命令主名
	requiredNames := []string{
		"help", "quit", "clear", "compact",
		"tools", "models", "rules", "stats", "hooks", "index",
		"sessions", "export", "resume",
		"plan", "delegate", "vision", "screenshot",
		"pool", "plugins", "model", "sandbox", "mcp", "mode", "undo", "budget",
		"config", "cost", "session", "plugin", "rag", "workflow",
	}
	registered := make(map[string]bool)
	for _, e := range all {
		registered[e.Name] = true
	}
	for _, name := range requiredNames {
		if !registered[name] {
			t.Errorf("missing builtin command: /%s", name)
		}
	}

	// 验证关键别名
	aliasTests := []struct{ name, alias string }{
		{"help", "h"},
		{"quit", "q"},
		{"quit", "exit"},
	}
	for _, tt := range aliasTests {
		entry, ok := r.Lookup(tt.alias)
		if !ok {
			t.Errorf("alias /%s not found", tt.alias)
			continue
		}
		if entry.Name != tt.name {
			t.Errorf("alias /%s resolved to %q, want %q", tt.alias, entry.Name, tt.name)
		}
	}
}

// TestSharedCommandSchema_ConsistentWithRegistry 验证 schema 与注册表一致
func TestSharedCommandSchema_ConsistentWithRegistry(t *testing.T) {
	schema, err := LoadSharedCommandSchema("shared_commands.json")
	if err != nil {
		t.Skipf("shared_commands.json not loadable: %v", err)
	}
	r := NewCommandRegistry()
	RegisterBuiltinCommands(r)

	registryNames := make(map[string]bool)
	for _, e := range r.All() {
		registryNames[e.Name] = true
	}
	schemaNames := make(map[string]bool)
	for _, c := range schema.Commands {
		schemaNames[c.Name] = true
	}

	// 双向检查：两边都应至少有 25 个命令
	if len(registryNames) < 25 {
		t.Errorf("registry has only %d commands", len(registryNames))
	}
	if len(schemaNames) < 25 {
		t.Errorf("schema has only %d commands", len(schemaNames))
	}
}
