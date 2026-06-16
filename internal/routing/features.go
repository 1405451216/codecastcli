package routing

import (
	"regexp"
	"strings"
)

// IntentType 任务意图分类。
// 用于 L1 特征扩展：不同意图倾向不同档位模型。
type IntentType string

const (
	IntentQuestion    IntentType = "question"    // 问答/解释
	IntentEdit        IntentType = "edit"        // 单点修改
	IntentRefactor    IntentType = "refactor"    // 重构/重写
	IntentTest        IntentType = "test"        // 测试相关
	IntentDeploy      IntentType = "deploy"      // 部署/发布
	IntentSecurity    IntentType = "security"    // 安全相关
	IntentArchitecture IntentType = "architecture" // 架构/设计
	IntentUnknown     IntentType = "unknown"     // 未分类
)

// TaskFeatures 任务特征向量。
// 由 ExtractFeatures 从用户输入提取，供 L1 档位判断和 L2 学习使用。
type TaskFeatures struct {
	// InputLength 输入字符数（rune 计数，避免中文截断）
	InputLength int
	// FileRefCount @file 引用数量（@path 模式）
	FileRefCount int
	// CodeBlockCount 代码块数量（``` 围栏）
	CodeBlockCount int
	// Intent 意图分类
	Intent IntentType
	// KeywordScore 复杂度关键词命中数（重构/架构/迁移等）
	KeywordScore int
	// HasMultiStep 是否含多步骤信号（"先...再..."、"步骤"、"第一步"等）
	HasMultiStep bool
	// HasToolHint 是否含工具调用暗示（"运行"、"执行"、"build"、"run"）
	HasToolHint bool
}

// 意图关键词表（顺序：先匹配的优先）
// 每个意图一组关键词，命中任一即归类。
var intentKeywords = map[IntentType][]string{
	IntentArchitecture: {"架构", "设计", "architecture", "design", "体系结构", "模块划分"},
	IntentRefactor:     {"重构", "refactor", "重写", "rewrite", "重新组织", "改造"},
	IntentTest:         {"测试", "test", "单元测试", "unittest", "mock", "fixture", "覆盖率"},
	IntentDeploy:       {"部署", "deploy", "发布", "release", "上线", "CI/CD", "pipeline"},
	IntentSecurity:     {"安全", "漏洞", "security", "vulnerability", "注入", "injection", "XSS", "CSRF", "越权"},
	IntentEdit:         {"修改", "改为", "替换", "edit", "change", "modify", "添加", "删除", "fix", "修复"},
	IntentQuestion:     {"?", "？", "为什么", "什么是", "解释", "explain", "what", "why", "how", "区别", "原理"},
}

// @file 引用模式：@path 或 @./path 或 @/path
var fileRefRe = regexp.MustCompile(`@(?:\.?/)?[^\s@,;:]+`)

// 代码块围栏模式
var codeBlockRe = regexp.MustCompile("```")

// 多步骤信号关键词
var multiStepKeywords = []string{
	"先", "再", "然后", "接着", "最后", "步骤", "第一步", "第二步",
	"phase", "step", "first", "then", "finally", "after that",
}

// 工具调用暗示关键词
var toolHintKeywords = []string{
	"运行", "执行", "build", "run", "make", "npm", "go test", "pytest",
	"compile", "lint", "format",
}

// ExtractFeatures 从用户输入提取任务特征。
// 纯函数，无副作用，便于单测。
func ExtractFeatures(input string) TaskFeatures {
	f := TaskFeatures{
		InputLength: len([]rune(input)),
	}

	// @file 引用数
	f.FileRefCount = len(fileRefRe.FindAllString(input, -1))

	// 代码块数（成对出现，除以 2）
	f.CodeBlockCount = len(codeBlockRe.FindAllString(input, -1)) / 2

	// 意图分类（按优先级顺序匹配）
	f.Intent = classifyIntent(input)

	// 复杂度关键词命中数
	lower := strings.ToLower(input)
	for _, kw := range complexityKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			f.KeywordScore++
		}
	}

	// 多步骤信号
	for _, kw := range multiStepKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			f.HasMultiStep = true
			break
		}
	}

	// 工具调用暗示
	for _, kw := range toolHintKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			f.HasToolHint = true
			break
		}
	}

	return f
}

// classifyIntent 按优先级顺序匹配意图。
// 顺序：architecture > refactor > test > deploy > security > edit > question > unknown
// 高优先级意图先匹配，避免"重构测试代码"被归为 test。
func classifyIntent(input string) IntentType {
	lower := strings.ToLower(input)
	// 按优先级顺序检查
	priority := []IntentType{
		IntentArchitecture,
		IntentRefactor,
		IntentSecurity,
		IntentDeploy,
		IntentTest,
		IntentEdit,
		IntentQuestion,
	}
	for _, intent := range priority {
		for _, kw := range intentKeywords[intent] {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return intent
			}
		}
	}
	return IntentUnknown
}

// Tier 模型档位。
type Tier string

const (
	TierSimple  Tier = "simple"
	TierMedium  Tier = "medium"
	TierComplex Tier = "complex"
)

// ClassifyTier 基于特征向量判断档位（L1 规则）。
// 比 complexityScore 更结构化，考虑意图类型。
//
// 规则：
//   - Complex: refactor/architecture 意图 OR 多步骤 OR 关键词命中 >=2 OR 文件引用 >3
//   - Simple: question 意图 AND 短输入 AND 无文件引用 AND 无代码块
//   - Medium: 其他
func ClassifyTier(f TaskFeatures) Tier {
	// Complex 信号
	if f.Intent == IntentRefactor || f.Intent == IntentArchitecture {
		return TierComplex
	}
	if f.HasMultiStep && f.InputLength > 100 {
		return TierComplex
	}
	if f.KeywordScore >= 2 {
		return TierComplex
	}
	if f.FileRefCount > 3 {
		return TierComplex
	}
	if f.CodeBlockCount >= 2 {
		return TierComplex
	}

	// Simple 信号
	if f.Intent == IntentQuestion && f.InputLength < 80 && f.FileRefCount == 0 && f.CodeBlockCount == 0 {
		return TierSimple
	}

	return TierMedium
}

// TierToModel 档位映射到配置的模型名。
func (r *ModelRouter) TierToModel(t Tier) string {
	switch t {
	case TierSimple:
		return r.cfg.SimpleModel
	case TierComplex:
		return r.cfg.ComplexModel
	default:
		return r.cfg.MediumModel
	}
}

// InferTierFromModel 根据模型名反推档位。
// 优先用 router 配置匹配（精确），匹配不上按模型名启发式判断。
// 用于学习路由记录 outcome 时推断 tier。
func InferTierFromModel(model string, router *ModelRouter) Tier {
	if router != nil {
		cfg := router.Config()
		if model == cfg.SimpleModel {
			return TierSimple
		}
		if model == cfg.ComplexModel {
			return TierComplex
		}
		if model == cfg.MediumModel {
			return TierMedium
		}
	}
	// 启发式：mini/flash/haiku → simple；opus/o1 → complex；其他 → medium
	m := strings.ToLower(model)
	if strings.Contains(m, "mini") || strings.Contains(m, "flash") ||
		strings.Contains(m, "haiku") || strings.Contains(m, "nano") {
		return TierSimple
	}
	if strings.Contains(m, "opus") || strings.Contains(m, "o1") ||
		strings.Contains(m, "o3") || strings.Contains(m, "ultra") {
		return TierComplex
	}
	return TierMedium
}

// RouteWithFeatures 基于特征向量的路由（L1 入口）。
// 比 Route 更精确：用结构化特征替代原始 input+fileCount。
func (r *ModelRouter) RouteWithFeatures(f TaskFeatures) string {
	if !r.cfg.Enabled {
		return r.cfg.MediumModel
	}
	tier := ClassifyTier(f)
	return r.TierToModel(tier)
}
