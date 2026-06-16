package routing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- L1 特征提取测试 ---

func TestExtractFeatures_Basic(t *testing.T) {
	f := ExtractFeatures("解释这段代码")
	if f.Intent != IntentQuestion {
		t.Errorf("期望 IntentQuestion，得到 %s", f.Intent)
	}
	if f.FileRefCount != 0 {
		t.Errorf("期望 0 文件引用，得到 %d", f.FileRefCount)
	}
}

func TestExtractFeatures_FileRef(t *testing.T) {
	f := ExtractFeatures("修改 @main.go 和 @utils.go 里的逻辑")
	if f.FileRefCount != 2 {
		t.Errorf("期望 2 文件引用，得到 %d", f.FileRefCount)
	}
	if f.Intent != IntentEdit {
		t.Errorf("期望 IntentEdit，得到 %s", f.Intent)
	}
}

func TestExtractFeatures_CodeBlocks(t *testing.T) {
	input := "```go\nfmt.Println()\n```\n和 ```python\nprint()\n```"
	f := ExtractFeatures(input)
	if f.CodeBlockCount != 2 {
		t.Errorf("期望 2 代码块，得到 %d", f.CodeBlockCount)
	}
}

func TestExtractFeatures_IntentPriority(t *testing.T) {
	// "重构测试代码" 应归为 refactor 而非 test（优先级）
	f := ExtractFeatures("重构测试代码")
	if f.Intent != IntentRefactor {
		t.Errorf("期望 IntentRefactor（优先级高于 test），得到 %s", f.Intent)
	}
}

func TestExtractFeatures_MultiStep(t *testing.T) {
	f := ExtractFeatures("先读取文件，然后解析，最后输出")
	if !f.HasMultiStep {
		t.Error("期望 HasMultiStep=true")
	}
}

func TestExtractFeatures_ToolHint(t *testing.T) {
	f := ExtractFeatures("运行 npm run build")
	if !f.HasToolHint {
		t.Error("期望 HasToolHint=true")
	}
}

func TestExtractFeatures_Unicode(t *testing.T) {
	// 中文不应被截断
	f := ExtractFeatures("你好世界，这是一段中文输入用于测试长度计算")
	if f.InputLength == 0 {
		t.Error("期望非零长度")
	}
	// rune 计数应与字符数一致
	expected := len([]rune("你好世界，这是一段中文输入用于测试长度计算"))
	if f.InputLength != expected {
		t.Errorf("长度期望 %d，得到 %d", expected, f.InputLength)
	}
}

// --- L1 档位分类测试 ---

func TestClassifyTier_Simple(t *testing.T) {
	f := TaskFeatures{
		Intent:      IntentQuestion,
		InputLength: 50,
	}
	if tier := ClassifyTier(f); tier != TierSimple {
		t.Errorf("期望 TierSimple，得到 %s", tier)
	}
}

func TestClassifyTier_Complex_Refactor(t *testing.T) {
	f := TaskFeatures{
		Intent: IntentRefactor,
	}
	if tier := ClassifyTier(f); tier != TierComplex {
		t.Errorf("期望 TierComplex（refactor 意图），得到 %s", tier)
	}
}

func TestClassifyTier_Complex_ManyFiles(t *testing.T) {
	f := TaskFeatures{
		Intent:       IntentEdit,
		FileRefCount: 5,
	}
	if tier := ClassifyTier(f); tier != TierComplex {
		t.Errorf("期望 TierComplex（5 文件引用），得到 %s", tier)
	}
}

func TestClassifyTier_Medium(t *testing.T) {
	f := TaskFeatures{
		Intent:      IntentEdit,
		InputLength: 100,
		FileRefCount: 1,
	}
	if tier := ClassifyTier(f); tier != TierMedium {
		t.Errorf("期望 TierMedium，得到 %s", tier)
	}
}

func TestClassifyTier_Complex_MultiStep(t *testing.T) {
	f := TaskFeatures{
		Intent:       IntentUnknown,
		InputLength:  200,
		HasMultiStep: true,
	}
	if tier := ClassifyTier(f); tier != TierComplex {
		t.Errorf("期望 TierComplex（多步骤+长输入），得到 %s", tier)
	}
}

// --- L0 RouteWithFeatures 测试 ---

func TestModelRouter_RouteWithFeatures(t *testing.T) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)

	// Simple 特征 → SimpleModel
	f := TaskFeatures{Intent: IntentQuestion, InputLength: 30}
	if m := router.RouteWithFeatures(f); m != cfg.SimpleModel {
		t.Errorf("Simple 特征期望 %s，得到 %s", cfg.SimpleModel, m)
	}

	// Complex 特征 → ComplexModel
	f = TaskFeatures{Intent: IntentRefactor}
	if m := router.RouteWithFeatures(f); m != cfg.ComplexModel {
		t.Errorf("Complex 特征期望 %s，得到 %s", cfg.ComplexModel, m)
	}

	// Medium 特征 → MediumModel
	f = TaskFeatures{Intent: IntentEdit, InputLength: 100, FileRefCount: 1}
	if m := router.RouteWithFeatures(f); m != cfg.MediumModel {
		t.Errorf("Medium 特征期望 %s，得到 %s", cfg.MediumModel, m)
	}
}

// --- L2 学习型路由测试 ---

func TestLearningRouter_Disabled(t *testing.T) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)
	lcfg := DefaultLearningConfig() // Enabled=false
	lr := NewLearningRouter(router, lcfg)

	// 禁用时退化为 L0
	m := lr.Route("解释这段代码", 0)
	if m != cfg.SimpleModel {
		t.Errorf("禁用学习层期望退化为 L0 Simple，得到 %s", m)
	}
}

func TestLearningRouter_NoCandidates(t *testing.T) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)
	lcfg := DefaultLearningConfig()
	lcfg.Enabled = true
	// 不配置 CandidateModels → 退化 L0
	lr := NewLearningRouter(router, lcfg)

	m := lr.Route("重构架构", 0)
	if m != cfg.ComplexModel {
		t.Errorf("无候选模型期望退化 L0 Complex，得到 %s", m)
	}
}

func TestLearningRouter_SingleCandidate(t *testing.T) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)
	lcfg := DefaultLearningConfig()
	lcfg.Enabled = true
	lcfg.CandidateModels = map[Tier][]string{
		TierSimple: {"gpt-4o-mini"},
	}
	lr := NewLearningRouter(router, lcfg)

	m := lr.Route("解释这段代码", 0)
	if m != "gpt-4o-mini" {
		t.Errorf("单候选期望直接返回，得到 %s", m)
	}
}

func TestLearningRouter_ColdStart(t *testing.T) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)
	lcfg := DefaultLearningConfig()
	lcfg.Enabled = true
	lcfg.MinSamples = 3
	lcfg.CandidateModels = map[Tier][]string{
		TierSimple: {"gpt-4o-mini", "gemini-flash", "deepseek-chat"},
	}
	lr := NewLearningRouter(router, lcfg)

	// 冷启动期应轮转：连续调用应覆盖所有候选
	seen := map[string]bool{}
	for i := 0; i < 9; i++ { // 3 模型 * 3 MinSamples
		m := lr.Route("什么是闭包", 0)
		seen[m] = true
		// 模拟记录结果
		lr.RecordOutcome(m, TierSimple, 100, 0.001, true)
	}
	if len(seen) < 3 {
		t.Errorf("冷启动期期望覆盖所有 3 个候选，只见到 %d 个: %v", len(seen), seen)
	}
}

func TestLearningRouter_ExploitBestModel(t *testing.T) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)
	lcfg := DefaultLearningConfig()
	lcfg.Enabled = true
	lcfg.Epsilon = 0 // 纯利用，不探索
	lcfg.MinSamples = 2
	lcfg.CandidateModels = map[Tier][]string{
		TierSimple: {"cheap-model", "expensive-model"},
	}
	lr := NewLearningRouter(router, lcfg)

	// 给 cheap-model 更好的历史（低成本高成功）
	for i := 0; i < 5; i++ {
		lr.RecordOutcome("cheap-model", TierSimple, 100, 0.0001, true)
	}
	// 给 expensive-model 较差的历史
	for i := 0; i < 5; i++ {
		lr.RecordOutcome("expensive-model", TierSimple, 200, 0.01, false)
	}

	// 纯利用应选 cheap-model
	m := lr.Route("什么是闭包", 0)
	if m != "cheap-model" {
		t.Errorf("纯利用期望选 cheap-model，得到 %s", m)
	}
}

func TestLearningRouter_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)
	lcfg := DefaultLearningConfig()
	lcfg.Enabled = true
	lcfg.StatePath = statePath
	lcfg.CandidateModels = map[Tier][]string{
		TierSimple: {"model-a", "model-b"},
	}
	lr := NewLearningRouter(router, lcfg)

	// 记录一些数据
	lr.RecordOutcome("model-a", TierSimple, 100, 0.001, true)
	lr.RecordOutcome("model-b", TierSimple, 100, 0.002, true)

	// 保存
	if err := lr.Save(); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("状态文件未创建: %v", err)
	}

	// 新建路由器加载状态
	lr2 := NewLearningRouter(router, lcfg)
	if err := lr2.Load(); err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	stats := lr2.Stats()
	if len(stats) != 2 {
		t.Errorf("加载后期望 2 个模型统计，得到 %d", len(stats))
	}

	// 验证数据正确
	found := false
	for _, s := range stats {
		if s.Name == "model-a" && s.Samples == 1 && s.Successes == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("加载后 model-a 统计不正确: %+v", stats)
	}
}

func TestLearningRouter_RecordOutcome(t *testing.T) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)
	lcfg := DefaultLearningConfig()
	lcfg.Enabled = true
	lr := NewLearningRouter(router, lcfg)

	lr.RecordOutcome("test-model", TierMedium, 500, 0.05, true)
	lr.RecordOutcome("test-model", TierMedium, 300, 0.03, false)

	stats := lr.Stats()
	if len(stats) != 1 {
		t.Fatalf("期望 1 个模型统计，得到 %d", len(stats))
	}
	s := stats[0]
	if s.Samples != 2 {
		t.Errorf("Samples 期望 2，得到 %d", s.Samples)
	}
	if s.Successes != 1 {
		t.Errorf("Successes 期望 1，得到 %d", s.Successes)
	}
	if s.TotalTokens != 800 {
		t.Errorf("TotalTokens 期望 800，得到 %d", s.TotalTokens)
	}
}

func TestModelStat_Score(t *testing.T) {
	// 低成本高成功 → 高分
	good := ModelStat{Samples: 10, Successes: 10, TotalCostUSD: 0.01}
	// 高成本低成功 → 低分
	bad := ModelStat{Samples: 10, Successes: 2, TotalCostUSD: 1.0}
	if good.Score() <= bad.Score() {
		t.Errorf("good.Score() (%f) 应大于 bad.Score() (%f)", good.Score(), bad.Score())
	}

	// 无数据 → 0
	empty := ModelStat{}
	if empty.Score() != 0 {
		t.Errorf("空统计 Score 期望 0，得到 %f", empty.Score())
	}
}

func TestIsSignificantlyBetter(t *testing.T) {
	// A: 90/100 成功，B: 30/100 成功，应显著更好
	if !IsSignificantlyBetter(90, 100, 30, 100, 5) {
		t.Error("期望 A 显著优于 B")
	}
	// 样本不足 → 不显著
	if IsSignificantlyBetter(90, 2, 30, 2, 5) {
		t.Error("样本不足不应判为显著")
	}
	// 相近 → 不显著
	if IsSignificantlyBetter(50, 100, 49, 100, 5) {
		t.Error("相近成功率不应判为显著")
	}
}

func TestLoadLearningState_FileNotExist(t *testing.T) {
	state, err := LoadLearningState("/nonexistent/path/state.json")
	if err != nil {
		t.Fatalf("文件不存在应返回空状态而非错误: %v", err)
	}
	if state.Models == nil {
		t.Error("空状态的 Models 不应为 nil")
	}
}

func TestLearningRouter_RouteWithFeatures(t *testing.T) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)
	lcfg := DefaultLearningConfig()
	lcfg.Enabled = true
	lcfg.CandidateModels = map[Tier][]string{
		TierComplex: {"claude-opus", "gpt-4o"},
	}
	lr := NewLearningRouter(router, lcfg)

	f := TaskFeatures{Intent: IntentRefactor}
	m := lr.RouteWithFeatures(f)
	// 冷启动期应返回候选之一
	if m != "claude-opus" && m != "gpt-4o" {
		t.Errorf("期望候选之一，得到 %s", m)
	}
}

// 验证长输入不崩溃
func TestExtractFeatures_VeryLongInput(t *testing.T) {
	long := strings.Repeat("a", 10000)
	f := ExtractFeatures(long)
	if f.InputLength != 10000 {
		t.Errorf("长度期望 10000，得到 %d", f.InputLength)
	}
}
