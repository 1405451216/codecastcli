package subagent

import (
	"context"
	"fmt"
	"strings"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/config"
	"codecast/cli/internal/provider"
)

// Orchestrator 编排 Plan-Agent + Execute-Agent 双 Agent 协作
type Orchestrator struct {
	config    *config.Config
	planAgent *ap.CapabilityAgent
	execAgent *ap.CapabilityAgent
	registry  *ap.ToolRegistry
	memory    ap.MemoryStore
}

// PlanResult 规划结果
type PlanResult struct {
	Tasks []PlanTask `json:"tasks"`
}

// PlanTask 单个规划任务
type PlanTask struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Priority    int      `json:"priority"`
}

// NewOrchestrator 创建编排器
func NewOrchestrator(cfg *config.Config, registry *ap.ToolRegistry, memory ap.MemoryStore) (*Orchestrator, error) {
	llmProvider, err := provider.CreateProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Provider 失败: %w", err)
	}

	// Plan-Agent: 负责分析任务、制定计划
	planAgent := ap.NewAgent("PlanAgent", planSystemPrompt, llmProvider,
		ap.WithMaxTurns(5),
	).WithToolkit(registry).WithMemory(memory)

	// Execute-Agent: 负责执行具体任务
	execAgent := ap.NewAgent("ExecuteAgent", execSystemPrompt, llmProvider,
		ap.WithMaxTurns(15),
	).WithToolkit(registry).WithMemory(memory)

	return &Orchestrator{
		config:    cfg,
		planAgent: planAgent,
		execAgent: execAgent,
		registry:  registry,
		memory:    memory,
	}, nil
}

// PlanAndExecute 规划并执行任务（Plan → Execute 顺序 DAG）
func (o *Orchestrator) PlanAndExecute(ctx context.Context, task string) (*ExecutionResult, error) {
	// 构建 Plan → Execute 顺序 DAG
	dag, err := ap.NewDAGBuilder("plan-execute").
		DelegateNode("plan", o.planAgent).
		DelegateNode("execute", o.execAgent).
		Edge("plan", "execute").
		Build()
	if err != nil {
		return nil, fmt.Errorf("构建 DAG 失败: %w", err)
	}

	result, err := dag.Run(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("DAG 执行失败: %w", err)
	}

	return &ExecutionResult{
		DAGResult: result,
		Plan:      extractNodeOutput(result, "plan"),
		Execution: extractNodeOutput(result, "execute"),
	}, nil
}

// ParallelExecute 并行执行多个独立子任务
func (o *Orchestrator) ParallelExecute(ctx context.Context, tasks []PlanTask) (*ExecutionResult, error) {
	builder := ap.NewDAGBuilder("parallel-execute")

	// 先规划
	builder.DelegateNode("plan", o.planAgent)

	// 为每个任务创建执行节点
	for _, task := range tasks {
		nodeID := fmt.Sprintf("exec_%s", task.ID)
		_ = ap.NewAgentDelegateNode(nodeID, o.execAgent)
		builder.DelegateNode(nodeID, o.execAgent)

		// 设置依赖关系
		if len(task.DependsOn) == 0 {
			builder.Edge("plan", nodeID)
		} else {
			for _, dep := range task.DependsOn {
				depNodeID := fmt.Sprintf("exec_%s", dep)
				builder.Edge(depNodeID, nodeID)
			}
		}
	}

	dag, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("构建并行 DAG 失败: %w", err)
	}

	result, err := dag.Run(ctx, fmt.Sprintf("并行执行 %d 个子任务", len(tasks)))
	if err != nil {
		return nil, fmt.Errorf("并行执行失败: %w", err)
	}

	return &ExecutionResult{
		DAGResult: result,
		Plan:      extractNodeOutput(result, "plan"),
	}, nil
}

// PlanOnly 仅规划不执行
func (o *Orchestrator) PlanOnly(ctx context.Context, task string) (string, error) {
	resp, err := o.planAgent.Run(ctx, ap.UserMessage(task))
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ExecutionResult 执行结果
type ExecutionResult struct {
	DAGResult *ap.DAGResult
	Plan      string
	Execution string
}

// Summary 返回执行结果摘要
func (r *ExecutionResult) Summary() string {
	if r.DAGResult == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("DAG 执行完成，共 %d 个节点\n", len(r.DAGResult.Order)))
	for _, nodeID := range r.DAGResult.Order {
		if nr, ok := r.DAGResult.NodeResults[nodeID]; ok {
			status := "成功"
			if nr.Error != nil {
				status = fmt.Sprintf("失败: %v", nr.Error)
			}
			sb.WriteString(fmt.Sprintf("  [%s] %s\n", nodeID, status))
		}
	}
	return sb.String()
}

// extractNodeOutput 提取节点输出
func extractNodeOutput(result *ap.DAGResult, nodeID string) string {
	if result == nil || result.NodeResults == nil {
		return ""
	}
	if nr, ok := result.NodeResults[nodeID]; ok {
		return nr.Output
	}
	return ""
}

const planSystemPrompt = `你是一个专业的任务规划 Agent。你的职责是：
1. 分析用户的需求
2. 将复杂任务拆解为可执行的子任务
3. 确定子任务之间的依赖关系
4. 输出结构化的执行计划

输出格式：
- 每个子任务一行，格式：[ID] 描述 (依赖: dep1,dep2)
- 如果没有依赖，省略依赖部分
- 示例：
  [1] 读取 main.go 文件内容
  [2] 分析代码结构 (依赖: 1)
  [3] 编写单元测试 (依赖: 2)
`

const execSystemPrompt = `你是一个专业的任务执行 Agent。你的职责是：
1. 接收规划 Agent 的输出
2. 按顺序执行每个子任务
3. 使用可用工具完成具体操作
4. 汇报执行结果

执行原则：
- 严格按照计划顺序执行
- 每个步骤完成后简要汇报结果
- 遇到错误时尝试修复或跳过
- 最终输出完整的执行报告
`
