# Release v1.0.0

**发布日期**: 2026-06-17
**代号**: WorldClass（世界级）
**重点**: 补齐与 Aider / Claude Code 的差距，四大能力全面升级

---

## 🌟 核心特性

### 1. 智能模型路由（L1 + L2 双层）

告别"一个模型打天下"。系统现在按请求类型自动路由到最合适的模型。

**L1 特征分类**（即时）：
- 简单问答 → 轻量模型（省钱）
- 代码编辑 / 重构 / 调试 → 主力模型（保质量）
- 多文件任务 → 主力模型 + 子 Agent

**L2 学习型路由器**（自适应）：
- Wilson 置信区间评估每个模型的历史成功率
- epsilon-greedy 探索：90% 用当前最优，10% 探索新选择
- 样本足够多时自动收敛到全局最优

**命令**：
- `/route stats` — 查看路由统计
- `/route reset` — 清空学习数据重来
- `/model <name>` — 手动覆盖

### 2. 子 Agent 自动并行编排

多文件任务不再串行等待。系统自动拆分为 DAG，无冲突任务并行执行。

**工作流**：
```
用户请求 → Plan Agent 拆分任务 → 检测文件冲突 → 并行/串行执行 → 汇总
```

**冲突检测**：两个任务修改同一文件 → 自动串行；不同文件 → 自动并行。

**命令**：
- `/subagent "重构 internal/tools 的错误处理"` — 触发
- `/dag` — 查看 DAG 可视化
- `/subagent --serial "任务"` — 强制串行

### 3. 模糊编辑（Fuzzy Edit）

旧版 Edit 工具是纯字符串替换，少一个空格就失败。新版用 Levenshtein 距离做模糊匹配。

**置信度策略**：
- > 0.85：自动应用（高置信度）
- 0.6 ~ 0.85：询问用户确认
- < 0.6：报错，让用户重述

**效果**：你给的代码片段缩进/空白不必完全匹配，系统自己找最近的位置。

### 4. 代码库语义索引

不再是简单的文件名/关键词匹配。现在按 tree-sitter 符号切块，用 embedding 做语义检索。

**架构**：
```
代码文件 → tree-sitter 提取符号 → 按符号边界切块
  → embedding 向量（语义相似）+ BM25 索引（关键词匹配）
  → 混合检索：向量 0.7 + BM25 0.3 加权融合
```

**特性**：
- 增量更新：只重算变更文件
- JSON 持久化：零外部依赖（无 CGO/sqlite-vec）
- 检索延迟 < 50ms（纯本地余弦相似度）

**支持的 Embedding Provider**：
| Provider | 模型 | 维度 | 适用 |
|----------|------|------|------|
| OpenAI | text-embedding-3-small | 1536 | 海外 |
| 智谱（Zhipu）| embedding-3 | 2048 | 国内推荐 |
| 通义（DashScope）| text-embedding-v3 | 1024 | 阿里云 |
| Mock | — | 32 | 测试 |

**命令**：
- `/semantic index` — 建索引
- `/semantic query "用户认证逻辑"` — 语义检索
- `/semantic stats` — 索引统计
- `/semantic clear` — 清空

**配置**（`~/.codecast/config.yaml`）：
```yaml
semantic_index:
  enabled: true
  embedding_provider: zhipu          # openai / zhipu / dashscope / mock
  embedding_api_key: "your-key"      # 为空则复用主 api_key
  embedding_model: embedding-3
  max_chunk_lines: 100
```

### 5. 基准测试框架

内置 15 个任务（5 类型 × 3 难度），量化评估你的配置表现。

**任务类型**：question / edit / refactor / debug / multi-file
**难度**：easy / medium / hard
**指标**：成功率 / 延迟 / token / 成本 / 工具调用数

**命令**：
- `/benchmark run` — 跑全部 15 个任务
- `/benchmark list` — 查看任务列表

### 6. A/B 反馈闭环

每轮回复后可反馈质量，系统持续学习优化。

**命令**：
- `/fb y` — 这轮有用
- `/fb n` — 这轮有问题
- `/fb show` — 看当前状态

---

## 📊 与顶级工具对比

| 能力 | Aider | Claude Code | codecastcli 0.4 | **codecastcli 1.0** |
|------|-------|-------------|------------------|---------------------|
| 模型路由 | 固定 | 固定 | 规则路由 | **L1+L2 学习路由** |
| 子 Agent | 串行 | 串行 | 串行 | **自动并行 DAG** |
| Edit/Apply | SEARCH/REPLACE | 精确匹配 | 纯字符串 | **模糊匹配** |
| 语义索引 | 无 | 无 | 无 | **embedding+BM25** |
| 基准测试 | 有 | 无 | 无 | **有** |
| A/B 优化 | 无 | 无 | 有 | 有（增强）|

---

## 🔧 升级指南

### 从 0.4.x 升级

1. 更新二进制：
   ```bash
   go install github.com/codecast/codecastcli@latest
   ```

2. （可选）启用语义索引，编辑 `~/.codecast/config.yaml`：
   ```yaml
   semantic_index:
     enabled: true
     embedding_provider: zhipu
     embedding_api_key: "your-zhipu-key"
     embedding_model: embedding-3
   ```

3. （可选）启用学习型路由：
   ```yaml
   learning_routing:
     enabled: true
   ```

4. 验证：
   ```bash
   codecast --version
   # codecast v1.0.0
   ```

### 配置兼容性

- 旧配置完全兼容，无需修改
- 新字段（`embedding_api_key` / `embedding_base_url`）为可选
- 不启用新功能则行为与 0.4 一致

---

## 📝 试用

- 试用指南：`docs/trial-guide.md`
- 反馈模板：`docs/trial-feedback.md`

---

## 🧪 测试

- 新增 ~30 个测试
- 全部通过：`go test ./internal/... ./cmd/...`
- 覆盖：路由 / 子 Agent / 模糊编辑 / 语义索引 / 基准测试 / 智谱通义 provider

---

## 🙏 致谢

感谢所有试用者和反馈者。1.0 是起点，不是终点。
