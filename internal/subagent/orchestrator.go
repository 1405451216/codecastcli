package subagent

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/config"
	"codecast/cli/internal/provider"
	"codecast/cli/internal/tui"
)

// Orchestrator 编排 Plan-Agent + Execute-Agent 双 Agent 协作
type Orchestrator struct {
	config             *config.Config
	planAgent          *ap.CapabilityAgent
	execAgent          *ap.CapabilityAgent
	registry           *ap.ToolRegistry
	memory             ap.MemoryStore
	llmProvider        ap.Provider
	dagView            *tui.DAGView
	enableVisualization bool
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
	// ConflictFiles 该任务会读写的文件列表（P2 自动并行）。
	// 两个无显式依赖的任务若 ConflictFiles 有交集，会被自动加串行依赖。
	ConflictFiles []string `json:"conflict_files,omitempty"`
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
		config:             cfg,
		planAgent:          planAgent,
		execAgent:          execAgent,
		registry:           registry,
		memory:             memory,
		llmProvider:        llmProvider,
		dagView:            tui.NewDAGView("多 Agent 协作"),
		enableVisualization: false,
	}, nil
}

// PlanAndExecute 规划并执行任务（Plan → Execute 顺序 DAG）
func (o *Orchestrator) PlanAndExecute(ctx context.Context, task string) (*ExecutionResult, error) {
	// DAGView: add Plan Agent node and set to Running
	if o.enableVisualization {
		o.dagView.AddNode("plan", "Plan Agent")
		o.dagView.UpdateNode("plan", tui.StatusRunning, "规划中...")
		o.dagView.AddChildNode("plan", "execute", "Execute Agent")
	}

	// 构建 Plan → Execute 顺序 DAG
	dag, err := ap.NewDAGBuilder("plan-execute").
		DelegateNode("plan", o.planAgent).
		DelegateNode("execute", o.execAgent).
		Edge("plan", "execute").
		Build()
	if err != nil {
		if o.enableVisualization {
			o.dagView.UpdateNode("plan", tui.StatusFailed, "DAG 构建失败")
		}
		return nil, fmt.Errorf("构建 DAG 失败: %w", err)
	}

	result, err := dag.Run(ctx, task)
	if err != nil {
		if o.enableVisualization {
			o.dagView.UpdateNode("plan", tui.StatusFailed, "执行失败")
			o.dagView.UpdateNode("execute", tui.StatusFailed, "执行失败")
		}
		return nil, fmt.Errorf("DAG 执行失败: %w", err)
	}

	// DAGView: update node statuses based on results
	if o.enableVisualization {
		if nr, ok := result.NodeResults["plan"]; ok {
			if nr.Error != nil {
				o.dagView.UpdateNode("plan", tui.StatusFailed, nr.Error.Error())
			} else {
				summary := nr.Output
				if len(summary) > 60 {
					summary = summary[:57] + "..."
				}
				o.dagView.UpdateNode("plan", tui.StatusCompleted, summary)
			}
		}
		if nr, ok := result.NodeResults["execute"]; ok {
			if nr.Error != nil {
				o.dagView.UpdateNode("execute", tui.StatusFailed, nr.Error.Error())
			} else {
				o.dagView.UpdateNode("execute", tui.StatusRunning, "执行中...")
				o.dagView.UpdateNode("execute", tui.StatusCompleted, "执行完成")
			}
		}
	}

	return &ExecutionResult{
		DAGResult: result,
		Plan:      extractNodeOutput(result, "plan"),
		Execution: extractNodeOutput(result, "execute"),
	}, nil
}

// ParallelExecute 并行执行多个独立子任务
// 每个子任务使用独立的 Agent 实例和隔离的内存存储，避免上下文污染。
// 最后通过聚合节点汇总所有子任务结果。
func (o *Orchestrator) ParallelExecute(ctx context.Context, tasks []PlanTask) (*ExecutionResult, error) {
	builder := ap.NewDAGBuilder("parallel-execute")
	var cleanupPaths []string
	var cleanupStores []ap.MemoryStore // COR-09: 跟踪所有 SQLite 存储连接 // C-04: 跟踪临时文件路径

	// DAGView: add Plan Agent node
	if o.enableVisualization {
		o.dagView.AddNode("plan", "Plan Agent")
		o.dagView.UpdateNode("plan", tui.StatusRunning, "规划中...")
	}

	// 先规划
	builder.DelegateNode("plan", o.planAgent)

	// 为每个任务创建独立的执行 Agent（隔离内存，共享工具注册表）
	for _, task := range tasks {
		nodeID := fmt.Sprintf("exec_%s", task.ID)

		// DAGView: add child node for each exec agent
		if o.enableVisualization {
			o.dagView.AddChildNode("plan", nodeID, fmt.Sprintf("Exec %s", task.ID))
			o.dagView.UpdateNode(nodeID, tui.StatusPending, task.Description)
		}

		// 每个子任务创建独立的 Agent 实例，使用独立的内存存储
		isolatedMemory, dbPath, err := newIsolatedMemory(task.ID)
		if err != nil {
			return nil, fmt.Errorf("创建隔离内存失败 (task %s): %w", task.ID, err)
		}
		cleanupPaths = append(cleanupPaths, dbPath)
		cleanupStores = append(cleanupStores, isolatedMemory) // COR-09: 跟踪连接
		isolatedAgent := ap.NewAgent(
			fmt.Sprintf("ExecuteAgent_%s", task.ID),
			execSystemPrompt,
			o.llmProvider,
			ap.WithMaxTurns(15),
		).WithToolkit(o.registry).WithMemory(isolatedMemory)

		delegateNode := ap.NewAgentDelegateNode(nodeID, isolatedAgent)
		builder.NodeWithAgent(nodeID, delegateNode)

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

	// DAGView: add aggregate node
	if o.enableVisualization {
		o.dagView.AddChildNode("plan", "aggregate", "Aggregate Agent")
	}

	// 聚合节点：收集所有子任务结果并生成摘要
	aggregateMemory, dbPath, err := newIsolatedMemory("aggregate")
	if err != nil {
		return nil, fmt.Errorf("创建聚合内存失败: %w", err)
	}
	cleanupPaths = append(cleanupPaths, dbPath)
	cleanupStores = append(cleanupStores, aggregateMemory) // COR-09: 跟踪连接
	aggregateAgent := ap.NewAgent(
		"AggregateAgent",
		aggregateSystemPrompt,
		o.llmProvider,
		ap.WithMaxTurns(3),
	).WithToolkit(o.registry).WithMemory(aggregateMemory)

	aggregateNode := ap.NewAgentDelegateNode("aggregate", aggregateAgent)
	aggregateNode.WithInputMapper(ap.MapConcatAll())
	builder.NodeWithAgent("aggregate", aggregateNode)

	// 所有执行节点汇聚到聚合节点
	for _, task := range tasks {
		execNodeID := fmt.Sprintf("exec_%s", task.ID)
		builder.Edge(execNodeID, "aggregate")
	}

	dag, err := builder.Build()
	if err != nil {
		if o.enableVisualization {
			o.dagView.UpdateNode("plan", tui.StatusFailed, "DAG 构建失败")
		}
		return nil, fmt.Errorf("构建并行 DAG 失败: %w", err)
	}

	result, err := dag.Run(ctx, fmt.Sprintf("并行执行 %d 个子任务", len(tasks)))
	if err != nil {
		if o.enableVisualization {
			o.dagView.UpdateNode("plan", tui.StatusFailed, "执行失败")
		}
		return nil, fmt.Errorf("并行执行失败: %w", err)
	}

	// DAGView: update all node statuses based on results
	if o.enableVisualization {
		if nr, ok := result.NodeResults["plan"]; ok {
			if nr.Error != nil {
				o.dagView.UpdateNode("plan", tui.StatusFailed, nr.Error.Error())
			} else {
				o.dagView.UpdateNode("plan", tui.StatusCompleted, "规划完成")
			}
		}
		for _, task := range tasks {
			nodeID := fmt.Sprintf("exec_%s", task.ID)
			if nr, ok := result.NodeResults[nodeID]; ok {
				if nr.Error != nil {
					o.dagView.UpdateNode(nodeID, tui.StatusFailed, nr.Error.Error())
				} else {
					o.dagView.UpdateNode(nodeID, tui.StatusRunning, "执行中...")
					o.dagView.UpdateNode(nodeID, tui.StatusCompleted, "执行完成")
				}
			}
		}
		if nr, ok := result.NodeResults["aggregate"]; ok {
			if nr.Error != nil {
				o.dagView.UpdateNode("aggregate", tui.StatusFailed, nr.Error.Error())
			} else {
				o.dagView.UpdateNode("aggregate", tui.StatusCompleted, "汇总完成")
			}
		}
	}

	return &ExecutionResult{
		DAGResult:   result,
		Plan:        extractNodeOutput(result, "plan"),
		Aggregation: extractNodeOutput(result, "aggregate"),
		cleanupDirs:   cleanupPaths,
		cleanupStores: cleanupStores,
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
	DAGResult   *ap.DAGResult
	Plan        string
	Execution   string
	Aggregation string
	cleanupDirs  []string     // C-04: 需要清理的临时数据库文件路径
	cleanupStores []ap.MemoryStore // COR-09: 需要关闭的 SQLite 存储连接
}

// Cleanup 清理执行过程中产生的临时文件和连接（C-04 + COR-09 修复）
func (r *ExecutionResult) Cleanup() {
	// 先关闭数据库连接，再删除文件
	for _, store := range r.cleanupStores {
		if closer, ok := store.(interface{ Close() error }); ok {
			closer.Close()
		}
	}
	r.cleanupStores = nil
	for _, dir := range r.cleanupDirs {
		os.Remove(dir)
	}
	r.cleanupDirs = nil
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

// newIsolatedMemory 创建独立的 SQLite 内存存储实例。
// 使用唯一文件路径避免 cache=shared 模式下的跨实例共享问题，
// 确保每个子任务的内存完全隔离。
// C-04 修复：返回 dbPath 供调用方在完成后清理。
func newIsolatedMemory(id string) (ap.MemoryStore, string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return nil, "", fmt.Errorf("生成随机 ID 失败: %w", err)
	}
	tmpDir := os.TempDir()
	dbPath := filepath.Join(tmpDir, fmt.Sprintf("codecast_subagent_%s_%x.db", id, b))
	store, err := ap.NewSQLiteStore(dbPath)
	if err != nil {
		return nil, "", fmt.Errorf("创建隔离 SQLite 存储失败: %w", err)
	}
	return store, dbPath, nil
}

// GetDAGView 返回编排器的 DAGView 实例
func (o *Orchestrator) GetDAGView() *tui.DAGView {
	return o.dagView
}

// SetVisualization 启用或禁用 DAG 可视化
func (o *Orchestrator) SetVisualization(enabled bool) {
	o.enableVisualization = enabled
}
