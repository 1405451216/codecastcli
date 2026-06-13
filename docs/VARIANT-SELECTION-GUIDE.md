# Codecast 变体选择指南

> **目的**：帮助用户在 17 个变体中快速找到适合自己的那一个
> **最后更新**: 2026-06-13 · v0.3.1

---

## 🚀 30 秒速选（回答 3 个问题）

### Q1：你在做什么任务？

| 你的任务 | 推荐变体 |
|---------|----------|
| **改 bug / 加功能 / 一般编码** | `default` 或 `concise` |
| **审查 PR / 找代码问题** | `code-reviewer` |
| **学习新技术 / 跟 agent 边做边学** | `pair-programmer` 或 `mentor-coach` |
| **探索架构 / 设计方案** | `architect-edit` 或 `decision-tree` |
| **写脚本 / 一行命令解决问题** | `shell-only` |
| **跨多文件的大改动** | `search-then-edit` 或 `architect-edit` |
| **生产代码 / 关键修复** | `safety-first` 或 `self-check` |
| **CI / 自动化 / agent 链式调用** | `format-locked` |

### Q2：你用什么 LLM？

| 模型 | 建议 |
|------|------|
| **GPT-4 / Claude Opus** | 默认所有变体都能跑，可直接用 `default` |
| **GPT-4-mini / Claude Haiku** | 避免 `claude-style`（XML 标签占 token）→ 用 `concise` |
| **本地小模型（≤7B）** | **必须**用 `concise` 或 `lazy-mode`（避免偷懒） |
| **不同模型做 A/B** | `prompt_strategy=weighted`，按权重分配 |

### Q3：你有什么特殊约束？

| 约束 | 推荐 |
|------|------|
| **想要严格 scope（不顺手改）** | `overeager-mode` |
| **想让 agent 偷懒的别偷懒** | `lazy-mode` |
| **担心 agent 改错文件** | `search-then-edit`（必须 user add 才能改） |
| **想控制 agent 不越权读系统文件** | `scope-guard` |
| **MCP 工具多，怕选错** | `mcp-router` |
| **想看 agent "思考过程"** | `decision-tree`（5 步评估显式化） |
| **输出会被自动解析** | `format-locked` |

---

## 🌳 决策树（详细版）

```
START
│
├─ 你在做什么？
│  │
│  ├─ 通用编码 / 不知道选啥 ────────────→ default
│  │
│  ├─ 边做边学 / 教学 ──────────────────→ pair-programmer
│  │                                    ├─ 想要人格感 → mentor-coach
│  │                                    └─ 想要严格 step-by-step → decision-tree
│  │
│  ├─ 审查代码 / PR review ──────────────→ code-reviewer
│  │
│  ├─ 大改动 / 重构 / 跨文件 ────────────→ search-then-edit
│  │                                    └─ 想要 Plan/Edit 分开 → architect-edit
│  │
│  ├─ shell 脚本 / 一行命令 ─────────────→ shell-only
│  │
│  ├─ 关键修复 / 生产代码 ───────────────→ safety-first
│  │                                    └─ 想要回复前自检 → self-check
│  │
│  └─ agent 间通信 / 自动化 ─────────────→ format-locked
│
└─ 你的模型 / 约束？
   │
   ├─ 小模型（<7B） ─────────────────────→ concise
   │                                    └─ 容易偷懒 → lazy-mode
   │
   ├─ 担心 agent 改错范围 ──────────────→ overeager-mode
   │
   ├─ 想看 agent 决策过程 ──────────────→ decision-tree
   │
   ├─ MCP 多 / 怕选错 ───────────────────→ mcp-router
   │
   └─ 文件权限严格 / 安全敏感 ───────────→ scope-guard
```

---

## 🎯 三大场景实战指南

### 场景 A：日常开发（90% 的时间）

```bash
# 推荐启动方式
codecast --prompt-variant default
# 或
CODECAST_PROMPT_VARIANT=default codecast
```

**为什么 default 够用？**
- 包含工具使用规范
- 含反模式清单
- 含工作流（简单/复杂/调试 三种）
- 含权限模式边界
- 含成本预算提醒

**什么时候切换到其他？**
- 任务重 → `safety-first`
- 时间紧 → `concise`（省 token）
- 在评审同事的 PR → `code-reviewer`
- 在写测试 → `default`（够用）

### 场景 B：A/B 测试（评估哪个变体最有效）

```yaml
# ~/.codecast/config.yaml
prompt_strategy: weighted
prompt_weights:
  default: 5
  concise: 3
  safety-first: 2
```

**怎么判断哪个好？**
```bash
/cost by-variant
# 看 1-2 周：哪个变体在同样任务下 cost 低 + 完成率高
```

**然后收敛到固定变体**：
```bash
# 看到 concise 胜出
codecast --prompt-variant concise
```

### 场景 C：项目级定制（团队统一风格）

```bash
# 项目根目录创建 .codecast/prompts/
mkdir -p .codecast/prompts

# 写一个 team-style.yaml（参考 .codecast/prompts/codegen-focused.yaml）
cat > .codecast/prompts/team-style.yaml <<EOF
name: team-style
description: 团队统一的编码风格
author: platform-team
sections:
  identity: |
    你是 CodecastAgent（团队风格），遵循本团队编码规范。
  tool_guide: |
    [团队特定工具说明]
  anti_patterns: |
    [团队禁止的反模式]
  workflow: |
    [团队工作流]
EOF

# 启动时自动加载
codecast --prompt-variant team-style
```

**注意**：项目级 variant 在用户级之后加载，会**覆盖**用户级同名 section。

---

## ⚡ 快速决策表（粘到桌面上）

| 场景 | 首选 | 备选 | 兜底 |
|------|------|------|------|
| 不知道选啥 | `default` | `concise` | `default` |
| 改 bug | `default` | `safety-first` | `lazy-mode` |
| 加功能 | `default` | `architect-edit` | `default` |
| 重构 | `search-then-edit` | `architect-edit` | `default` |
| 写测试 | `default` | `lazy-mode` | `default` |
| 写文档 | `default` | `claude-style` | `default` |
| 审查 PR | `code-reviewer` | `default` | `safety-first` |
| 调试 | `self-check` | `decision-tree` | `default` |
| 学习 | `pair-programmer` | `mentor-coach` | `claude-style` |
| 教别人 | `mentor-coach` | `pair-programmer` | `claude-style` |
| shell 任务 | `shell-only` | `default` | `concise` |
| agent 流水线 | `format-locked` | `default` | `concise` |
| 大改动 | `search-then-edit` | `architect-edit` | `default` |
| 紧急修复 | `concise` | `default` | `concise` |
| A/B 评估 | `weighted` 策略 | `round-robin` | `fixed` |
| 想看思考 | `decision-tree` | `mentor-coach` | `claude-style` |
| MCP 多 | `mcp-router` | `default` | `claude-style` |
| 安全敏感 | `scope-guard` | `safety-first` | `search-then-edit` |

---

## 🚫 常见的错误选择

| 错误选择 | 为什么不合适 | 换成什么 |
|----------|------------|----------|
| 任何场景都用 `claude-style` | XML 标签占 token，小模型容易跑偏 | 用 `default` 或 `concise` |
| 日常用 `safety-first` | 太保守，反而拖慢 | 用 `default` + `/mode full-auto` |
| 一直用 `lazy-mode` | 解释太重，不必要的啰嗦 | 用 `concise` 即可 |
| 用 `architect-edit` 做小改 | 双阶段过重 | 用 `default` |
| 用 `format-locked` 跟 agent 聊天 | 格式太硬，难调试 | 用 `default` |
| 把 `decision-tree` 当万能变体 | 评估过程占 token | 仅在复杂任务用 |

---

## 🔄 切换变体的工作流

### 场景 1：先试再说（推荐）

```bash
# 启动后用 /prompt 命令
/prompt use claude-style
# 跑 1-2 个任务，看效果
/prompt show claude-style      # 看完整 prompt
# 不满意：
/prompt use default
```

### 场景 2：直接选一个

```bash
# 启动时通过 CLI flag
codecast --prompt-variant claude-style
```

### 场景 3：按项目

```bash
# 项目根目录 .codecast/prompts/team-style.yaml
# 启动时 codecast 自动加载
codecast  # 自动用 team-style
```

### 场景 4：A/B 评估

```bash
# 配置
echo "prompt_strategy: weighted" >> ~/.codecast/config.yaml
echo "prompt_weights:" >> ~/.codecast/config.yaml
echo "  default: 5" >> ~/.codecast/config.yaml
echo "  concise: 1" >> ~/.codecast/config.yaml

# 1-2 周后看数据
/cost by-variant
# 收敛到胜出者
/prompt use <winner>
```

---

## 📊 17 变体速查卡

### 通用风格（不知道选啥就从这里挑）

| 变体 | 一句话 | 适合 |
|------|--------|------|
| `default` | 平衡版（推荐起点） | 90% 场景 |
| `concise` | 极简省 token | 小模型/紧急任务 |
| `safety-first` | 偏保守 | 关键修复/生产 |

### 借鉴 Claude Fable 5

| 变体 | 一句话 | 适合 |
|------|--------|------|
| `claude-style` | XML 章节 + good/bad 对照 | 喜欢严谨/想看 LLM 怎么"想" |
| `code-reviewer` | 分级反馈（🔴/🟡/🟢） | PR 审查 |
| `pair-programmer` | 边做边讲解 | 学习新技术 |
| `decision-tree` | 5 步评估显式化 | 复杂任务/想看决策过程 |
| `self-check` | 回复前 5 步自检 | 生产代码/关键修复 |
| `scope-guard` | 文件访问 scope 强制 | 安全敏感 |
| `mcp-router` | MCP vs 内置决策 | MCP 工具多 |
| `mentor-coach` | 温暖 + 建设性 push back | 教学/辅导 |

### 借鉴 Aider

| 变体 | 一句话 | 适合 |
|------|--------|------|
| `search-then-edit` | 两阶段：triage → user-add → edit | 防止越权改文件 |
| `format-locked` | MUST/NEVER 严格输出格式 | CI/agent 通信 |
| `architect-edit` | Plan-Agent → Edit-Agent 双 agent | 大改动 |
| `shell-only` | 1-3 one-liner 约束 | 脚本任务 |
| `lazy-mode` | 禁止 TODO/伪代码 | 防止偷懒 |
| `overeager-mode` | 严禁"顺手改" | 严格 scope |

---

## 🧪 试验性建议

如果你想**第一次**用 Codecast 的 prompt 框架，**建议这个顺序**：

1. **`default`** —— 跑 1 周，熟悉基础
2. **`concise`** —— 跑 1 天，对比 token 用量
3. **`safety-first`** —— 跑 1 个关键任务，对比质量
4. **`code-reviewer`** —— 下次 PR 审查时试
5. **`pair-programmer`** —— 学新技术时试
6. 然后根据经验**做 A/B 评估**

**别一上来就跑全部 17 个**——信息过载反而选不好。

---

## ❓ 遇到问题怎么办

| 问题 | 排查 |
|------|------|
| 变体没生效 | `/prompt current` 看实际选中的 |
| 变体渲染是空的 | `/prompt show <name>` 看完整 prompt，可能是变量缺失 |
| 想恢复默认 | `/prompt use default` |
| 想看所有变体 | `/prompt list` |
| 想重载 prompts 目录 | `/prompt reload` |
| 变体没出现 | 检查 `~/.codecast/prompts/*.yaml` 或 `.codecast/prompts/*.yaml` 的 YAML 语法 |

---

**更多细节**：
- 变体完整列表：[`README.md` 提示词 A/B 框架](../../README.md#提示词-ab-框架)
- 变体设计原理：[`PROMPT-DISTILLATION-NOTES.md`](review/PROMPT-DISTILLATION-NOTES.md)
- 变体源代码：[`internal/promptab/embedded.go`](../internal/promptab/embedded.go)
