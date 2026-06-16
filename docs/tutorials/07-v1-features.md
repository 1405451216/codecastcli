# v1.0 新功能教程

本教程涵盖 codecastcli v1.0 的五大新功能：智能路由、子 Agent 并行、模糊编辑、语义索引、基准测试。

## 1. 智能模型路由（L1 + L2）

### 启用学习型路由

编辑 `~/.codecast/config.yaml`：

```yaml
learning_routing:
  enabled: true
  epsilon: 0.1            # 10% 探索新选择
  min_samples: 10         # 最少 10 个样本才开始学习
```

### 工作原理

**L1 特征分类**（即时决策）：
- 简单问答 → 轻量模型（省钱）
- 代码编辑 / 重构 / 调试 → 主力模型
- 多文件任务 → 主力模型 + 子 Agent

**L2 学习型路由器**（自适应）：
- Wilson 置信区间评估每个模型的历史成功率
- epsilon-greedy：90% 用当前最优，10% 探索新选择
- 样本足够多时自动收敛到全局最优

### 命令

```bash
# 查看路由统计
/route stats

# 清空学习数据重来
/route reset

# 手动覆盖（不学习）
/model deepseek-chat
```

### 观察路由决策

每轮回复末尾会显示 `[routed: model-name]`，告诉你这轮用了哪个模型。

## 2. 子 Agent 自动并行编排

### 触发自动并行

```
> /subagent 重构 internal/tools 下所有 edit 工具的错误处理
```

系统会：
1. Plan Agent 拆分为多个子任务
2. AutoParallel 检测每个子任务读写的文件
3. 无冲突任务自动并行，有冲突自动串行
4. 各子 Agent 在隔离沙箱运行

### 查看 DAG

```bash
/dag
```

输出示例：
```
[plan] 拆分为 3 个子任务：
  ├─ T1: edit.go (无依赖)
  ├─ T2: multi_edit.go (无依赖)
  └─ T3: fuzzy.go (无依赖)
[exec] 并行执行 T1 T2 T3
[done] 3/3 成功
```

### 强制串行

如果并行导致冲突，可以强制串行：

```bash
/subagent --serial "任务描述"
```

### 与 /delegate 的区别

| 命令 | 行为 |
|------|------|
| `/delegate` | 顺序执行子任务（v0.4 行为） |
| `/subagent` | 自动检测冲突，无冲突并行（v1.0 新） |
| `/subagent --serial` | 强制串行 |

## 3. 模糊编辑（Fuzzy Edit）

### 体验模糊匹配

```
> 把 getUserInfo 函数里的 return User{} 改成 return User{ID: id}
```

即使你给的代码片段缩进/空白不完全匹配，系统也能定位。

### 置信度策略

| 置信度 | 行为 |
|--------|------|
| > 0.85 | 自动应用 |
| 0.6 ~ 0.85 | 询问用户确认 |
| < 0.6 | 报错，让用户重述 |

### 对比旧版

旧版 Edit 工具是纯字符串替换，少一个空格就失败。新版用 Levenshtein 距离做模糊匹配，自动找最近的位置。

## 4. 代码库语义索引

### 配置 Embedding Provider

编辑 `~/.codecast/config.yaml`：

```yaml
# 国内推荐：智谱
semantic_index:
  enabled: true
  embedding_provider: zhipu
  embedding_api_key: "your-zhipu-key"
  embedding_model: embedding-3

# 或通义（阿里云）
# semantic_index:
#   enabled: true
#   embedding_provider: dashscope
#   embedding_api_key: "your-dashscope-key"
#   embedding_model: text-embedding-v3

# 或 OpenAI（海外）
# semantic_index:
#   enabled: true
#   embedding_provider: openai
#   embedding_api_key: "your-openai-key"
#   embedding_model: text-embedding-3-small
```

### 建索引

```bash
/semantic index
```

输出：
```
[semantic] 索引中... 152 文件, 487 符号
[semantic] 完成: 487 向量, 487 BM25 文档
[semantic] 耗时: 2.3s
```

### 语义检索

```bash
/semantic query "用户认证逻辑在哪"
```

输出：
```
🔍 检索结果 (query: "用户认证逻辑在哪")
1. [0.8234] internal/auth/login.go:45-78 (hybrid)
   符号: handleLogin (function)
   签名: func handleLogin(w http.ResponseWriter, r *http.Request)
2. [0.7891] internal/auth/middleware.go:15-42 (hybrid)
   符号: AuthMiddleware (function)
3. [0.7234] internal/auth/token.go:8-30 (hybrid)
   符号: GenerateToken (function)
```

### 查看统计

```bash
/semantic stats
```

### 工作原理

```
代码文件
  → tree-sitter 提取符号（函数/类/方法）
  → 按符号边界切块
  → embedding 向量（语义相似）+ BM25 索引（关键词匹配）
  → 混合检索：向量 0.7 + BM25 0.3 加权融合
```

**特性**：
- 增量更新：文件 modtime 变化时只重算该文件
- JSON 持久化：零外部依赖（无 CGO/sqlite-vec）
- 检索延迟 < 50ms（纯本地余弦相似度）

## 5. 基准测试

### 运行基准测试

```bash
/benchmark run
```

输出：
```
[benchmark] 运行 15 个任务（5 类型 × 3 难度）...
[benchmark] 进度: 15/15
[benchmark]
[benchmark] === 总结 ===
[benchmark] 成功率: 12/15 (80%)
[benchmark] 平均延迟: 3.2s
[benchmark] 总 token: 48213
[benchmark] 估算成本: $0.18
[benchmark] 工具调用数: 47
[benchmark]
[benchmark] === 按类型 ===
[benchmark] question:  3/3 (100%)
[benchmark] edit:      2/3 (67%)
[benchmark] refactor:  2/3 (67%)
[benchmark] debug:     3/3 (100%)
[benchmark] multi-file: 2/3 (67%)
```

### 查看任务列表

```bash
/benchmark list
```

### 任务类型

| 类型 | 难度 | 示例 |
|------|------|------|
| question | easy/medium/hard | "这个函数做什么" |
| edit | easy/medium/hard | "把 foo 改成线程安全" |
| refactor | easy/medium/hard | "拆分这个 God struct" |
| debug | easy/medium/hard | "为什么这个测试 flaky" |
| multi-file | easy/medium/hard | "统一所有 edit 工具的错误处理" |

## 6. A/B 反馈闭环

每轮回复后可以反馈质量，系统会学习优化。

```bash
# 这轮回复有用
/fb y

# 这轮回复有问题
/fb n

# 看当前状态
/fb show
```

## 下一步

- 完整功能列表：见 README.md
- 试用指南：`docs/trial-guide.md`
- 反馈模板：`docs/trial-feedback.md`
- 配置示例：`docs/semantic_index.example.yaml`
