package subagent

import (
	"strings"
	"testing"
)

func TestParsePlanOutput_Simple(t *testing.T) {
	text := `[1] 读取 main.go
[2] 分析逻辑 (依赖: 1)
[3] 修改代码 (依赖: 2) (文件: main.go)`
	tasks, err := ParsePlanOutput(text)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("期望 3 个任务，得到 %d", len(tasks))
	}
	if tasks[0].ID != "1" || tasks[0].Description != "读取 main.go" {
		t.Errorf("任务1 不正确: %+v", tasks[0])
	}
	if len(tasks[1].DependsOn) != 1 || tasks[1].DependsOn[0] != "1" {
		t.Errorf("任务2 依赖不正确: %+v", tasks[1])
	}
	if len(tasks[2].ConflictFiles) != 1 || tasks[2].ConflictFiles[0] != "main.go" {
		t.Errorf("任务3 文件不正确: %+v", tasks[2])
	}
}

func TestParsePlanOutput_FieldsSwapped(t *testing.T) {
	// 文件字段在依赖之前
	text := `[1] 修改代码 (文件: a.go) (依赖: 2)
[2] 读取 a.go`
	tasks, err := ParsePlanOutput(text)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("期望 2 个任务，得到 %d", len(tasks))
	}
	if len(tasks[0].ConflictFiles) != 1 || tasks[0].ConflictFiles[0] != "a.go" {
		t.Errorf("任务1 文件不正确: %+v", tasks[0])
	}
	if len(tasks[0].DependsOn) != 1 || tasks[0].DependsOn[0] != "2" {
		t.Errorf("任务1 依赖不正确: %+v", tasks[0])
	}
}

func TestParsePlanOutput_NoFields(t *testing.T) {
	text := `[1] 简单任务
[2] 另一个任务`
	tasks, err := ParsePlanOutput(text)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("期望 2 个任务，得到 %d", len(tasks))
	}
	for _, tk := range tasks {
		if len(tk.DependsOn) != 0 || len(tk.ConflictFiles) != 0 {
			t.Errorf("无字段任务不应有依赖或文件: %+v", tk)
		}
	}
}

func TestParsePlanOutput_SkipsInvalidLines(t *testing.T) {
	text := `=== 计划 ===
[1] 任务一

# 注释行
无效行
[2] 任务二`
	tasks, err := ParsePlanOutput(text)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("期望 2 个任务（跳过无效行），得到 %d", len(tasks))
	}
}

func TestParsePlanOutput_Empty(t *testing.T) {
	_, err := ParsePlanOutput("")
	if err == nil {
		t.Error("空输入应返回错误")
	}
}

func TestParsePlanOutput_MultipleFiles(t *testing.T) {
	text := `[1] 修改多文件 (文件: a.go, b.go, c.go)`
	tasks, err := ParsePlanOutput(text)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(tasks[0].ConflictFiles) != 3 {
		t.Errorf("期望 3 个文件，得到 %d", len(tasks[0].ConflictFiles))
	}
}

func TestAutoResolveDependencies_NoConflict(t *testing.T) {
	// 两个任务操作不同文件，无显式依赖 → 保持无依赖（可并行）
	tasks := []PlanTask{
		{ID: "1", Description: "改 a.go", ConflictFiles: []string{"a.go"}},
		{ID: "2", Description: "改 b.go", ConflictFiles: []string{"b.go"}},
	}
	result := AutoResolveDependencies(tasks)
	if len(result[0].DependsOn) != 0 || len(result[1].DependsOn) != 0 {
		t.Errorf("无文件冲突不应加依赖: %+v", result)
	}
}

func TestAutoResolveDependencies_FileConflict(t *testing.T) {
	// 两个任务操作同一文件，无显式依赖 → 自动加串行依赖
	tasks := []PlanTask{
		{ID: "1", Description: "读 a.go", ConflictFiles: []string{"a.go"}},
		{ID: "2", Description: "改 a.go", ConflictFiles: []string{"a.go"}},
	}
	result := AutoResolveDependencies(tasks)
	// ID 小的应成为 ID 大的依赖
	if !hasDependency(&result[1], "1") {
		t.Errorf("文件冲突应自动加依赖: %+v", result)
	}
	if hasDependency(&result[0], "2") {
		t.Errorf("ID 小的不应依赖 ID 大的: %+v", result)
	}
}

func TestAutoResolveDependencies_PreservesExplicitDeps(t *testing.T) {
	// 已有显式依赖不应被破坏
	tasks := []PlanTask{
		{ID: "1", Description: "任务1", ConflictFiles: []string{"a.go"}},
		{ID: "2", Description: "任务2", DependsOn: []string{"1"}, ConflictFiles: []string{"a.go"}},
	}
	result := AutoResolveDependencies(tasks)
	// 已有 1→2 依赖，不应重复添加
	count := 0
	for _, d := range result[1].DependsOn {
		if d == "1" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("显式依赖不应被重复添加，count=%d", count)
	}
}

func TestAutoResolveDependencies_PathNormalization(t *testing.T) {
	// 不同写法的同一路径应被识别为冲突
	tasks := []PlanTask{
		{ID: "1", ConflictFiles: []string{"./src/a.go"}},
		{ID: "2", ConflictFiles: []string{"src/a.go"}},
	}
	result := AutoResolveDependencies(tasks)
	if !hasDependency(&result[1], "1") {
		t.Errorf("路径归一化后应识别冲突: %+v", result)
	}
}

func TestAutoResolveDependencies_WindowsPath(t *testing.T) {
	tasks := []PlanTask{
		{ID: "1", ConflictFiles: []string{"src\\a.go"}},
		{ID: "2", ConflictFiles: []string{"src/a.go"}},
	}
	result := AutoResolveDependencies(tasks)
	if !hasDependency(&result[1], "1") {
		t.Errorf("Windows 路径应归一化后识别冲突: %+v", result)
	}
}

func TestAutoResolveDependencies_NoFiles(t *testing.T) {
	// 无文件操作的任务不与其他任务冲突
	tasks := []PlanTask{
		{ID: "1", Description: "分析"},
		{ID: "2", Description: "总结"},
	}
	result := AutoResolveDependencies(tasks)
	if len(result[0].DependsOn) != 0 || len(result[1].DependsOn) != 0 {
		t.Errorf("无文件任务不应加依赖: %+v", result)
	}
}

func TestFilesConflict(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{"both empty", nil, nil, false},
		{"a empty", nil, []string{"a.go"}, false},
		{"b empty", []string{"a.go"}, nil, false},
		{"no overlap", []string{"a.go"}, []string{"b.go"}, false},
		{"overlap", []string{"a.go", "b.go"}, []string{"b.go"}, true},
		{"same single", []string{"a.go"}, []string{"a.go"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := filesConflict(c.a, c.b); got != c.want {
				t.Errorf("filesConflict(%v,%v) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"a.go", "a.go"},
		{"./a.go", "a.go"},
		{"  a.go  ", "a.go"},
		{"src\\a.go", "src/a.go"},
		{"./src/a.go", "src/a.go"},
	}
	for _, c := range cases {
		if got := normalizePath(c.in); got != c.want {
			t.Errorf("normalizePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAnalyzeParallelism_AllParallel(t *testing.T) {
	tasks := []PlanTask{
		{ID: "1"},
		{ID: "2"},
		{ID: "3"},
	}
	p, s, c := AnalyzeParallelism(tasks)
	if p != 3 {
		t.Errorf("可并行组期望 3，得到 %d", p)
	}
	if s != 1 {
		t.Errorf("串行链期望 1，得到 %d", s)
	}
	if c != 0 {
		t.Errorf("冲突文件期望 0，得到 %d", c)
	}
}

func TestAnalyzeParallelism_FullySerial(t *testing.T) {
	tasks := []PlanTask{
		{ID: "1"},
		{ID: "2", DependsOn: []string{"1"}},
		{ID: "3", DependsOn: []string{"2"}},
	}
	p, s, _ := AnalyzeParallelism(tasks)
	if p != 1 {
		t.Errorf("可并行组期望 1，得到 %d", p)
	}
	if s != 3 {
		t.Errorf("串行链期望 3，得到 %d", s)
	}
}

func TestLongestDepChain_Diamond(t *testing.T) {
	// 菱形依赖：1 → 2,3 → 4
	tasks := []PlanTask{
		{ID: "1"},
		{ID: "2", DependsOn: []string{"1"}},
		{ID: "3", DependsOn: []string{"1"}},
		{ID: "4", DependsOn: []string{"2", "3"}},
	}
	if l := longestDepChain(tasks); l != 3 {
		t.Errorf("菱形依赖最长链期望 3，得到 %d", l)
	}
}

func TestStripFields(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"任务描述", "任务描述"},
		{"任务描述 (依赖: 1)", "任务描述"},
		{"任务描述 (文件: a.go)", "任务描述"},
		{"任务描述 (依赖: 1) (文件: a.go)", "任务描述"},
		{"任务描述 (依赖: 1,2) (文件: a.go,b.go)", "任务描述"},
	}
	for _, c := range cases {
		got := strings.TrimSpace(stripFields(c.in))
		if got != c.want {
			t.Errorf("stripFields(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtractField(t *testing.T) {
	desc := "任务 (依赖: 1,2) (文件: a.go, b.go)"
	dep := extractField(desc, "依赖")
	if len(dep) != 2 || dep[0] != "1" || dep[1] != "2" {
		t.Errorf("依赖提取错误: %v", dep)
	}
	files := extractField(desc, "文件")
	if len(files) != 2 || files[0] != "a.go" || files[1] != "b.go" {
		t.Errorf("文件提取错误: %v", files)
	}
	// 不存在的字段
	none := extractField(desc, "不存在")
	if none != nil {
		t.Errorf("不存在的字段应返回 nil: %v", none)
	}
}
