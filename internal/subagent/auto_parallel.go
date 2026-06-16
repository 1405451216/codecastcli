package subagent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// AutoParallelExecute 自动并行执行入口（P2）。
//
// 流程：
//  1. 调 Plan-Agent 生成计划文本
//  2. 解析为 []PlanTask（含 ConflictFiles）
//  3. 自动计算文件冲突：无显式依赖但 ConflictFiles 有交集的任务，自动加串行依赖
//  4. 调 ParallelExecute 执行
//
// 相比 ParallelExecute 的改进：用户无需手工指定 DependsOn，
// 编排器根据文件冲突自动决定并行度。
func (o *Orchestrator) AutoParallelExecute(ctx context.Context, task string) (*ExecutionResult, error) {
	// 1. 规划
	planText, err := o.PlanOnly(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("规划失败: %w", err)
	}

	// 2. 解析
	tasks, err := ParsePlanOutput(planText)
	if err != nil {
		return nil, fmt.Errorf("解析计划失败: %w", err)
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("计划为空")
	}

	// 3. 自动补充依赖（文件冲突检测）
	tasks = AutoResolveDependencies(tasks)

	// 4. 执行
	return o.ParallelExecute(ctx, tasks)
}

// ParsePlanOutput 解析 Plan-Agent 的文本输出为 []PlanTask。
//
// 支持格式：
//   [ID] 描述 (依赖: 1,2) (文件: path1,path2)
//   [ID] 描述 (文件: path1)
//   [ID] 描述 (依赖: 1)
//   [ID] 描述
//
// 字段顺序不固定（依赖和文件可互换）。括号内字段可选。
// 解析失败的行会被跳过（容错），不返回错误。
func ParsePlanOutput(text string) ([]PlanTask, error) {
	lines := strings.Split(text, "\n")
	var tasks []PlanTask

	// 行匹配模式：[ID] 描述 ...
	lineRe := regexp.MustCompile(`^\s*\[(\d+)\]\s*(.+?)\s*$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "===") {
			continue
		}

		m := lineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		id := m[1]
		rest := m[2]

		// 提取依赖和文件字段
		dependsOn := extractField(rest, "依赖")
		conflictFiles := extractField(rest, "文件")

		// 描述 = 去掉所有 (...) 字段后的剩余
		desc := stripFields(rest)

		task := PlanTask{
			ID:          id,
			Description: strings.TrimSpace(desc),
		}
		if len(dependsOn) > 0 {
			task.DependsOn = dependsOn
		}
		if len(conflictFiles) > 0 {
			task.ConflictFiles = conflictFiles
		}
		tasks = append(tasks, task)
	}

	if len(tasks) == 0 {
		return nil, fmt.Errorf("未解析到任何任务，原始输出:\n%s", text)
	}
	return tasks, nil
}

// extractField 从描述中提取指定字段的值。
// 字段格式：(字段名: v1,v2,v3) 或 (字段名: v1, v2)
// 返回逗号分隔的值列表（已 trim 空白）。
func extractField(desc, fieldName string) []string {
	// 匹配 (字段名: 值) 模式
	pattern := fmt.Sprintf(`\(\s*%s\s*:\s*([^)]+)\s*\)`, regexp.QuoteMeta(fieldName))
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(desc)
	if m == nil {
		return nil
	}
	parts := strings.Split(m[1], ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// stripFields 去掉描述中所有 (...) 字段，只保留任务描述本身。
func stripFields(desc string) string {
	// 去掉所有 (xxx) 段
	re := regexp.MustCompile(`\s*\([^)]*\)\s*`)
	return re.ReplaceAllString(desc, " ")
}

// AutoResolveDependencies 自动补充依赖关系（P2 核心）。
//
// 规则：
//  1. 保留任务已有的显式 DependsOn
//  2. 对无显式依赖的任务对，若 ConflictFiles 有交集，自动加串行依赖
//     （ID 较小的任务成为 ID 较大任务的依赖）
//  3. 不破坏已有依赖（不重复添加）
//
// 返回更新后的 tasks（原切片会被修改）。
func AutoResolveDependencies(tasks []PlanTask) []PlanTask {
	// 构建 ID → task 索引
	byID := make(map[string]int, len(tasks))
	for i, t := range tasks {
		byID[t.ID] = i
	}

	// 对每对无显式依赖的任务，检查文件冲突
	for i := 0; i < len(tasks); i++ {
		for j := i + 1; j < len(tasks); j++ {
			a := &tasks[i]
			b := &tasks[j]

			// 已有依赖关系（a 依赖 b 或 b 依赖 a）→ 跳过
			if hasDependency(a, b.ID) || hasDependency(b, a.ID) {
				continue
			}

			// 文件冲突检测
			if filesConflict(a.ConflictFiles, b.ConflictFiles) {
				// ID 较小的成为较大的依赖
				if a.ID < b.ID {
					addDependency(b, a.ID)
				} else {
					addDependency(a, b.ID)
				}
			}
		}
	}

	return tasks
}

// hasDependency 检查 task 是否已依赖 depID
func hasDependency(task *PlanTask, depID string) bool {
	for _, d := range task.DependsOn {
		if d == depID {
			return true
		}
	}
	return false
}

// addDependency 添加依赖（去重）
func addDependency(task *PlanTask, depID string) {
	if hasDependency(task, depID) {
		return
	}
	task.DependsOn = append(task.DependsOn, depID)
}

// filesConflict 检查两组文件是否有交集。
// 空列表视为无冲突（无文件操作的任务不与其他任务冲突）。
func filesConflict(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, f := range a {
		set[normalizePath(f)] = true
	}
	for _, f := range b {
		if set[normalizePath(f)] {
			return true
		}
	}
	return false
}

// normalizePath 路径归一化：去前后空白、去 ./ 前缀、统一斜杠。
// 避免同一路径不同写法导致冲突检测漏判。
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.ReplaceAll(p, "\\", "/")
	return p
}

// AnalyzeParallelism 分析计划的并行度（供调试/可视化用）。
// 返回：(可并行组数, 串行链长度, 冲突文件数)
func AnalyzeParallelism(tasks []PlanTask) (parallelGroups, serialChain, conflicts int) {
	// 无依赖的任务可并行
	parallel := 0
	for _, t := range tasks {
		if len(t.DependsOn) == 0 {
			parallel++
		}
	}
	// 最长依赖链
	serialChain = longestDepChain(tasks)
	// 有冲突文件的任务数
	for _, t := range tasks {
		if len(t.ConflictFiles) > 0 {
			conflicts++
		}
	}
	return parallel, serialChain, conflicts
}

// longestDepChain 计算最长依赖链长度（拓扑排序 + DP）。
func longestDepChain(tasks []PlanTask) int {
	byID := make(map[string]*PlanTask, len(tasks))
	for i := range tasks {
		byID[tasks[i].ID] = &tasks[i]
	}

	// 记忆化：每个任务到末端的最长链
	memo := make(map[string]int)
	var dfs func(id string) int
	dfs = func(id string) int {
		if v, ok := memo[id]; ok {
			return v
		}
		t, ok := byID[id]
		if !ok {
			return 0
		}
		max := 0
		for _, dep := range t.DependsOn {
			if l := dfs(dep); l > max {
				max = l
			}
		}
		memo[id] = max + 1
		return max + 1
	}

	longest := 0
	for _, t := range tasks {
		if l := dfs(t.ID); l > longest {
			longest = l
		}
	}
	return longest
}
