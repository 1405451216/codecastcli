# CodecastCLI 1.0 试用指南

本指南帮助你快速体验 1.0 的四大新能力：智能路由、子 Agent 并行、模糊编辑、语义索引。

## 前置准备

```bash
# 确认版本 >= 1.0.0
codecast --version

# 首次运行配置向导（已配置可跳过）
codecast
```

需要至少一个 LLM Provider（OpenAI / Anthropic / DeepSeek 等任一）。
语义索引可选配 Embedding Provider（智谱/通义/OpenAI），不配也能用（退化为 BM25 关键词检索）。

## 体验 1：智能模型路由（自动）

无需手动操作。系统会根据你的请求自动选择模型：

| 请求类型 | 路由目标 | 示例 |
|---------|---------|------|
| 简单问答 | 轻量模型 | "这个函数做什么" |
| 代码编辑 | 主力模型 | "把 foo 改成线程安全" |
| 多文件重构 | 主力模型 | "拆分这个 God struct" |
| 复杂调试 | 主力模型 | "为什么这个测试 flaky" |

**观察方式**：每轮回复末尾会显示 `[routed: model-name]`。

**手动覆盖**：`/model deepseek-chat` 或 `/model gpt-4o`。

**查看路由统计**：`/route stats`。

## 体验 2：子 Agent 并行编排

对多文件任务，系统会自动拆分为并行子任务：

```
> 重构 internal/tools 下所有 edit 工具，统一错误处理
```

观察输出中的 DAG 视图：

```
[plan] 拆分为 3 个子任务：
  ├─ T1: edit.go (无依赖)
  ├─ T2: multi_edit.go (无依赖)
  └─ T3: fuzzy.go (无依赖)
[exec] 并行执行 T1 T2 T3
[done] 3/3 成功
```

**手动触发**：`/subagent "任务描述"`。

**查看 DAG**：`/dag`。

## 体验 3：模糊编辑（Fuzzy Edit）

即使你给出的代码片段缩进/空白不完全匹配，也能定位修改：

```
> 把 getUserInfo 函数里的 return User{} 改成 return User{ID: id}
```

系统会：
1. 用 Levenshtein 距离做模糊匹配
2. 置信度 > 0.85 自动应用
3. 置信度 0.6~0.85 询问确认
4. 置信度 < 0.6 报错让你重述

**对比旧版**：旧版纯字符串替换，少一个空格就失败。

## 体验 4：语义索引

首次使用会自动建索引（约 1-3 秒/百文件）：

```
> /index build
[semantic] 索引中... 152 文件, 487 符号
[semantic] 完成: 487 向量, 487 BM25 文档
```

之后提问会自动检索相关代码：

```
> 用户认证逻辑在哪
[retrieve] top-3: auth/login.go:42, auth/middleware.go:15, auth/token.go:8
```

**查看索引状态**：`/index stats`。

**重建索引**：`/index rebuild`。

**配置 Embedding Provider**（可选，提升检索质量）：

```yaml
# ~/.codecast/config.yaml
semantic_index:
  enabled: true
  embedder: zhipu   # 或 dashscope / openai
  embedder_api_key: "your-key"
  embedder_model: "embedding-3"
```

## 体验 5：基准测试

跑内置基准看你的配置表现：

```
> /benchmark run
[benchmark] 运行 15 个任务（5 类型 × 3 难度）...
[benchmark] 成功率: 12/15 (80%)
[benchmark] 平均延迟: 3.2s
[benchmark] 总 token: 48213
[benchmark] 估算成本: $0.18
```

**只看任务列表**：`/benchmark list`。

## 体验 6：A/B 反馈闭环

每轮回复后可以反馈质量，系统会学习：

```
> /fb y    # 这轮回复有用
> /fb n    # 这轮回复有问题
> /fb show # 看当前状态
```

## 常见问题

**Q: 语义索引建索引很慢？**
A: 首次建索引需调用 Embedding API。不配 Embedder 则用本地 Mock，秒建但检索质量较低。建议配智谱/通义（国内快）。

**Q: 路由选的模型不对？**
A: 路由器需要样本学习。初期用 `/model` 手动指定，积累样本后自动路由会变准。`/route reset` 可清空学习数据重来。

**Q: 子 Agent 并行导致冲突？**
A: 系统会检测文件冲突，有冲突的任务自动串行。如仍出错，用 `/subagent --serial "任务"` 强制串行。

## 下一步

- 完整文档：`docs/tutorials/`
- 反馈试用体验：见 `docs/trial-feedback.md`
- 填完反馈后：`/fb` 或 GitHub Issue
