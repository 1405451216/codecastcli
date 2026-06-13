package promptab

import "strings"

// RenderInputs 是 variant 渲染的输入参数。
// 与 internal/agent/prompt.go 的 buildSystemPrompt 共享变量命名，
// 但 variant 模板使用 {{var}} 语法而非 fmt.Sprintf。
type RenderInputs struct {
	OS           string
	CWD          string
	Mode         string // suggest / auto-edit / full-auto
	Budget       string // 字符串化的预算数字，如 "5.50" 或 "0"
	ModeAdvice   string // 权限模式的具体建议文案
	FileTree     string
	ProjectRules string
	ExtraVars    map[string]string // 扩展变量，key→value
}

// Render 把 variant 的所有 section 拼接为最终提示词。
// section 顺序固定（即使 yaml 中乱序）：
//
//	identity → environment → tool_guide → anti_patterns → permission_boundary
//	→ budget_awareness → workflow → output_format → codebase_context → project_rules
//
// 每个 section 的 body 内部仍支持 {{var}} 插值（运行时变量）。
func (v *Variant) Render(in RenderInputs) string {
	if v == nil {
		return ""
	}
	order := []string{
		"identity",
		"environment",
		"tool_guide",
		"anti_patterns",
		"permission_boundary",
		"budget_awareness",
		"workflow",
		"output_format",
		"codebase_context",
		"project_rules",
	}
	var sb strings.Builder
	for _, key := range order {
		s, ok := v.Sections[key]
		if !ok || strings.TrimSpace(s.Body) == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(s.Body)
		sb.WriteString("\n")
	}
	return interpolate(sb.String(), in)
}

// interpolate 替换 {{var}} 为 RenderInputs 字段值。
// 缺失变量保留为空（不报错，便于可选 section 留空）。
func interpolate(template string, in RenderInputs) string {
	// 构造变量表
	vars := map[string]string{
		"os":            in.OS,
		"cwd":           in.CWD,
		"mode":          in.Mode,
		"budget":        in.Budget,
		"mode_advice":   in.ModeAdvice,
		"file_tree":     in.FileTree,
		"project_rules": in.ProjectRules,
	}
	for k, v := range in.ExtraVars {
		vars[k] = v
	}
	// 单 pass 替换（不用 regex 因为 {{ }} 内不会有歧义）
	out := template
	for {
		start := strings.Index(out, "{{")
		if start < 0 {
			break
		}
		end := strings.Index(out[start:], "}}")
		if end < 0 {
			break
		}
		end += start
		key := strings.TrimSpace(out[start+2 : end])
		val, ok := vars[key]
		if !ok {
			val = ""
		}
		out = out[:start] + val + out[end+2:]
	}
	return out
}
