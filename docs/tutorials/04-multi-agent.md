# 双 Agent 协作

## 概念

Codecast 支持将复杂任务拆分给多个子 Agent 并行执行：

- **Plan Agent** — 分析任务，生成执行计划（DAG）
- **Execute Agent** — 按计划逐步执行，调用工具

## /plan — 制定计划

```
> /plan 重构数据库层，把 ORM 从 gorm 换成 ent
```

输出示例：
```
## 执行计划

1. 分析现有 gorm 用法 → [read_file] src/db/*.go
2. 设计 ent schema    → [create_file] ent/schema/*.go
3. 迁移查询逻辑      → [edit_file] src/db/repository.go
4. 更新测试          → [edit_file] src/db/*_test.go
5. 运行验证          → [shell_exec] go test ./...

预估 Token: ~15,000 | 预估耗时: ~3 分钟
```

## /delegate — 委托执行（顺序）

```
> /delegate 给所有 API handler 添加 input validation
```

AI 会：
1. 分析任务范围（找到所有 handler）
2. 拆分为独立子任务
3. 子 Agent **顺序**执行（v0.4 行为）
4. 汇总结果

### DAG 可视化

执行过程中，TUI 会展示 DAG 执行图：

```
[分析 handler] ──→ [添加验证: auth] ──→ [汇总]
               ├──→ [添加验证: user] ──┘
               └──→ [添加验证: post] ──┘
```

## 工作流模式

### Pipeline（串行）

```
> /workflow pipeline "分析代码 → 写测试 → 运行测试"
```

### Parallel（并行）

```
> /workflow parallel "给 5 个模块分别写文档"
```

### Handoff（接力）

```
> /workflow handoff "先用 GPT-4 设计方案，再用 Claude 实现代码"
```

## 隔离子 Agent

每个子 Agent 运行在独立沙箱中：

- **文件系统隔离** — 子 Agent 只能访问分配给它的文件
- **上下文隔离** — 子 Agent 之间互不可见
- **资源限制** — 每个子 Agent 有独立的 Token 预算

## /subagent — 自动并行编排（v1.0+）

v1.0 引入自动并行编排，比 `/delegate` 更智能：

```
> /subagent 重构 internal/tools 下所有 edit 工具的错误处理
```

**工作流**：
1. Plan Agent 拆分为多个子任务
2. AutoParallel 检测每个子任务读写的文件集合
3. **无文件冲突的任务自动并行执行**
4. 有文件冲突的任务自动串行执行
5. 各子 Agent 在隔离沙箱运行

**输出示例**：
```
[plan] 拆分为 3 个子任务：
  ├─ T1: edit.go (无依赖)
  ├─ T2: multi_edit.go (无依赖)
  └─ T3: fuzzy.go (无依赖)
[exec] 并行执行 T1 T2 T3
[done] 3/3 成功
```

### 查看 DAG

```bash
/dag
```

### 强制串行

如果并行导致冲突，可以强制串行：

```bash
/subagent --serial "任务描述"
```

### /delegate vs /subagent 对比

| 命令 | 行为 | 版本 |
|------|------|------|
| `/delegate` | 顺序执行子任务 | v0.4 |
| `/subagent` | 自动检测冲突，无冲突并行 | v1.0+ |
| `/subagent --serial` | 强制串行 | v1.0+ |

## 下一步

- [自定义提示词](05-custom-prompts.md)
- [成本优化](06-cost-optimization.md)
- [v1.0 新功能教程](07-v1-features.md)
