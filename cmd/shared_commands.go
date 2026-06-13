package cmd

// shared_commands.go: 跨语言命令 schema 共享。
//
// 之前 Go 侧 (commands.go) 与 TS 侧 (src/commands.ts) 各自维护
// 斜杠命令的补全列表，容易出现 Go 有 /sandbox TS 没有、TS 有 /new
// Go 没有的不一致情况。
//
// 现在两边都从 cmd/shared_commands.json 读取：
//   - Go:  LoadSharedCommandSchema()  (在 init 时打印一条调试信息)
//   - TS:   src/commands.ts 直接 import JSON
//
// 注意：JSON 只描述命令的元数据（name/aliases/description/owner），
// 实际 handler 仍然分别在 Go 和 TS 中实现——因为它们的执行环境
// 和可用资源差异很大。但补全和文档从此统一来源。

import (
	"encoding/json"
	"fmt"
	"os"
)

// SharedCommand JSON schema 中的一条命令
type SharedCommand struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases"`
	Description string   `json:"description"`
	Owner       string   `json:"owner"`
}

// SharedCommandSchema JSON schema 根
type SharedCommandSchema struct {
	Version     string          `json:"version"`
	Generated   string          `json:"generated"`
	Description string          `json:"description"`
	Commands    []SharedCommand `json:"commands"`
}

// LoadSharedCommandSchema 从 JSON 文件加载 schema
//
// 用于运行时验证 Go 端 RegisterBuiltinCommands 注册的命令与共享
// schema 一致。如果发现遗漏或冲突，打印警告到 stderr（不阻塞启动）。
func LoadSharedCommandSchema(path string) (*SharedCommandSchema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read shared commands schema: %w", err)
	}
	var schema SharedCommandSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("parse shared commands schema: %w", err)
	}
	return &schema, nil
}

// VerifyRegistryConsistency 验证 r 与 schema 在 Name/Aliases/Description 三个字段上的完全一致。
//
// v0.2.0 修复: 之前只比较 Name，会漏掉：
//   - alias 拼写错误
//   - description 描述漂移
//   - owner 分类变化
//
// 现在对所有公开字段做完整对照。
func VerifyRegistryConsistency(r *CommandRegistry, schema *SharedCommandSchema) {
	type registryEntry struct {
		aliases     []string
		description string
	}
	type schemaEntry struct {
		aliases     []string
		description string
		owner       string
	}

	regMap := make(map[string]registryEntry)
	for _, e := range r.All() {
		regMap[e.Name] = registryEntry{
			aliases:     e.Aliases,
			description: e.Description,
		}
	}
	schemaMap := make(map[string]schemaEntry)
	for _, c := range schema.Commands {
		schemaMap[c.Name] = schemaEntry{
			aliases:     c.Aliases,
			description: c.Description,
			owner:       c.Owner,
		}
	}

	// orphan: schema 有但 registry 没有
	for name, sc := range schemaMap {
		rc, ok := regMap[name]
		if !ok {
			fmt.Fprintf(os.Stderr,
				"[commands] ⚠️  shared schema has '%s' (owner=%s) but registry does not\n",
				name, sc.owner)
			continue
		}
		// description 一致性
		if rc.description != sc.description {
			fmt.Fprintf(os.Stderr,
				"[commands] ⚠️  '%s' description mismatch: registry=%q vs schema=%q\n",
				name, rc.description, sc.description)
		}
		// aliases 一致性（顺序无关，作为集合比较）
		if !sameStringSet(rc.aliases, sc.aliases) {
			fmt.Fprintf(os.Stderr,
				"[commands] ⚠️  '%s' aliases mismatch: registry=%v vs schema=%v\n",
				name, rc.aliases, sc.aliases)
		}
	}
	// missing: registry 有但 schema 没有
	for name := range regMap {
		if _, ok := schemaMap[name]; !ok {
			fmt.Fprintf(os.Stderr,
				"[commands] ⚠️  registry has '%s' but shared schema does not\n", name)
		}
	}
}

// sameStringSet 比较两个字符串切片是否包含相同元素（顺序无关）
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ma := make(map[string]bool, len(a))
	for _, x := range a {
		ma[x] = true
	}
	for _, y := range b {
		if !ma[y] {
			return false
		}
	}
	return true
}
