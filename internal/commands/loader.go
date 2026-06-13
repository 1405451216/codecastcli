// Package commands 加载并渲染 .codecast/commands/*.md 中的用户可扩展斜杠命令。
//
// 设计动机：
//   - 之前斜杠命令的提示词要么硬编码在 Go 里、要么用 4 行 markdown 凑合
//   - 想要参数化（audience / depth / format / focus 等可调维度）
//     又不想每次改都重新编译 Go
//   - 解决方案：把"命令元数据"放 YAML frontmatter，把"提示词模板"放 markdown body
//
// frontmatter schema：
//
//	---
//	name: review                   # 命令名（不含 "/"），必填
//	description: 代码审查          # 补全弹窗显示，必填
//	mode: build                    # ask | build，可选，默认 ask
//	audience: intermediate         # 目标读者（注入到模板），可选
//	depth: detailed                # 解释粒度，可选
//	format: structured-report      # 输出格式，可选
//	example: /review x.go          # 补全提示，可选
//	arguments:                     # 参数定义（用于补全 + 校验），可选
//	  - name: target
//	    required: true
//	    description: 文件路径
//	  - name: focus
//	    required: false
//	    description: security|performance|maintainability
//	template: |                    # 提示词模板，支持 $ARGUMENTS 简写和 {{var}} 插值
//	  ... 多行 markdown ...
//
// 模板语法：
//   - {{var}}              取 defaults 中的值（来自 frontmatter）
//   - {{var | "fallback"}} 带 fallback 的取值
//   - $ARGUMENTS           替换为用户输入的 args 字符串（trim）
//   - $ARG0..$ARG9         替换为按空白拆分的第 N 个参数
package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ArgumentSpec 描述一个命令参数的元数据，用于补全和校验。
type ArgumentSpec struct {
	Name        string `yaml:"name"`
	Required    bool   `yaml:"required"`
	Description string `yaml:"description"`
	// Default 在 Render 时作为 fallback；可空
	Default string `yaml:"default,omitempty"`
}

// CommandSpec 描述一个斜杠命令的完整定义。
type CommandSpec struct {
	// 来自 frontmatter
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Mode        string         `yaml:"mode"`
	Audience    string         `yaml:"audience"`
	Depth       string         `yaml:"depth"`
	Format      string         `yaml:"format"`
	Example     string         `yaml:"example"`
	Arguments   []ArgumentSpec `yaml:"arguments"`
	// Template 是 frontmatter 后面的 markdown body（去掉 frontmatter 后整段）
	Template string `yaml:"-"`

	// SourceFile 仅供调试与错误消息使用
	SourceFile string `yaml:"-"`
}

// LoadDir 扫描 dir 中所有 .md 文件，解析为 CommandSpec 列表。
// 不递归。文件解析失败时返回 error（不会静默跳过）。
func LoadDir(dir string) ([]*CommandSpec, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read commands dir %q: %w", dir, err)
	}
	var out []*CommandSpec
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		spec, err := LoadFile(path)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", path, err)
		}
		out = append(out, spec)
	}
	// 按 name 排序，便于稳定输出
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// LoadFile 解析单个 .md 文件。
// frontmatter 与 body 用 "---" 分割。
func LoadFile(path string) (*CommandSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	spec, err := Parse(string(data))
	if err != nil {
		return nil, err
	}
	spec.SourceFile = path
	if spec.Name == "" {
		return nil, fmt.Errorf("missing required field 'name' in frontmatter")
	}
	if spec.Description == "" {
		return nil, fmt.Errorf("missing required field 'description' in frontmatter")
	}
	if spec.Template == "" {
		return nil, fmt.Errorf("missing template body (empty markdown body)")
	}
	return spec, nil
}

// Parse 解析 markdown 文本（不含 frontmatter 包裹的 "---" 行外的语法约束）。
// 接受 ""、"\n---\n" 单独存在（视为无 frontmatter）以及标准的 "key: value\n---\nbody" 格式。
func Parse(markdown string) (*CommandSpec, error) {
	scanner := bufio.NewScanner(strings.NewReader(markdown))
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024) // 4MB 上限

	var fmLines []string
	var bodyLines []string
	state := "preamble" // preamble -> fm -> body
	for scanner.Scan() {
		line := scanner.Text()
		switch state {
		case "preamble":
			trimmed := strings.TrimSpace(line)
			if trimmed == "---" {
				state = "fm"
				continue
			}
			// 在第一个 --- 之前允许空行 / 注释；遇到非空非 --- 行就停止扫描
			if trimmed != "" {
				return nil, fmt.Errorf("unexpected content before frontmatter: %q", line)
			}
		case "fm":
			if strings.TrimSpace(line) == "---" {
				state = "body"
				continue
			}
			fmLines = append(fmLines, line)
		case "body":
			bodyLines = append(bodyLines, line)
		}
	}
	if state != "body" {
		return nil, fmt.Errorf("frontmatter not closed (expected '---' delimiter)")
	}

	// 解析 frontmatter
	yamlText := strings.Join(fmLines, "\n")
	spec := &CommandSpec{}
	if err := yaml.Unmarshal([]byte(yamlText), spec); err != nil {
		return nil, fmt.Errorf("parse frontmatter yaml: %w", err)
	}
	spec.Template = strings.TrimSpace(strings.Join(bodyLines, "\n"))
	return spec, nil
}

// RenderInputs 是 Render 的输入。
type RenderInputs struct {
	// Arguments 是用户键入的原始参数字符串（不含命令名），如 "src/foo.go focus=security"
	Arguments string
	// Defaults 覆盖 frontmatter 的默认值（来自命令行 flag / config）
	// 例如 {"audience": "senior", "format": "bullets"}
	Defaults map[string]string
}

// Render 把模板渲染为最终提示词。
// 模板语法支持：
//
//	{{var}}              从 spec 的字段 + Defaults 中查找
//	{{var | "fallback"}} 找不到时用 fallback
//	$ARGUMENTS           替换为 inputs.Arguments（trim）
//	$ARG0..$ARG9         按空白拆分的第 N 个 token（0-indexed）
//	$K=V                 若 Arguments 中含 "key=value"，则 {{key}} 也可取值
func (s *CommandSpec) Render(inputs RenderInputs) string {
	out := s.Template

	// 1. 处理 $ARGUMENTS 与 $ARG0..$ARG9
	args := strings.TrimSpace(inputs.Arguments)
	out = strings.ReplaceAll(out, "$ARGUMENTS", args)

	tokens := splitArgsRespectQuotes(args)
	for i, tok := range tokens {
		out = strings.ReplaceAll(out, fmt.Sprintf("$ARG%d", i), tok)
	}

	// 2. 处理 key=value 形式的参数注入
	kvMap := map[string]string{}
	for _, tok := range tokens {
		if idx := strings.IndexByte(tok, '='); idx > 0 {
			kvMap[tok[:idx]] = tok[idx+1:]
		}
	}

	// 3. 处理 {{var}} 与 {{var | "fallback"}}
	// 优先顺序：Defaults > kvMap > spec 字段
	getValue := func(name string) (string, bool) {
		if v, ok := inputs.Defaults[name]; ok && v != "" {
			return v, true
		}
		if v, ok := kvMap[name]; ok {
			return v, true
		}
		switch name {
		case "name":
			return s.Name, s.Name != ""
		case "description":
			return s.Description, s.Description != ""
		case "mode":
			return s.Mode, s.Mode != ""
		case "audience":
			return s.Audience, s.Audience != ""
		case "depth":
			return s.Depth, s.Depth != ""
		case "format":
			return s.Format, s.Format != ""
		case "example":
			return s.Example, s.Example != ""
		}
		return "", false
	}

	re := regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*(?:\|\s*"([^"]*)")?\s*\}\}`)
	out = re.ReplaceAllStringFunc(out, func(match string) string {
		groups := re.FindStringSubmatch(match)
		name, fallback := groups[1], groups[2]
		if v, ok := getValue(name); ok {
			return v
		}
		return fallback // 空字符串也返回（fallback 可能是 ""）
	})

	return out
}

// splitArgsRespectQuotes 按空白拆，但尊重双引号包裹的整体（用于含空格的路径）。
// 这是简化实现：不处理转义双引号；足够命令行参数场景。
func splitArgsRespectQuotes(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuote = !inQuote
		case (c == ' ' || c == '\t') && !inQuote:
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}
