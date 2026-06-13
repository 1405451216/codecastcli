# 提示词提炼沉淀笔记

> **目的**：记录从外部 LLM 系统提示词中提炼 Codecast 变体的完整方法论 + 已提炼的模式库
> **最后更新**: 2026-06-13

---

## 📚 已提炼资料

| # | 来源 | 提炼时间 | 提炼变体数 | 详细文档 |
|---|------|----------|-----------|----------|
| 1 | Claude Fable 5 系统提示词 | 2026-06-13 | 7 (claude-style / code-reviewer / pair-programmer / decision-tree / self-check / scope-guard / mcp-router / mentor-coach) | `claude-fable-5.md` (本地) |
| 2 | Aider 提示词架构 | 2026-06-13 | 6 (search-then-edit / format-locked / architect-edit / shell-only / lazy-mode / overeager-mode) | [`aider-system-prompt-reference.md`](aider-system-prompt-reference.md) |

**累计变体**: 17 个嵌入 + 任意数量用户自定义

---

## 🎯 提炼方法论（4 步法）

任何外部 LLM 提示词都可按以下步骤提炼：

### Step 1：找章节划分（Structure）

寻找结构标记：
- **XML 标签**（`<product_information>`、`<tool_usage>`、`<forbidden_behaviors>`）
- **Markdown 标题**（## / ###）
- **Python 类继承**（Aider 的 `CoderPrompts` 基类 + 子类覆盖）

**Codecast 适配**：每个章节映射为 `Variant.Sections` 的一个 key（`identity` / `tool_guide` / `anti_patterns` / ...）

### Step 2：提取显式边界（Boundaries）

寻找约束语言：
- `MUST` / `NEVER` / `ONLY EVER` / `ALWAYS`
- `*italic*` 强调
- "Do not" / "You should not"

**Codecast 适配**：反模式清单（`anti_patterns` section），必须**成对**——说"应该"之前先说"不应该"。

### Step 3：找 Few-shot 形态（Examples）

寻找示例的形态：
- `<good_response>` / `<bad_response>` 对照（Claude Fable 5）
- 工作流示例（Aider 的 "1. ... 2. ... 3. ..."）
- 工具调用示例（含输入输出）

**Codecast 适配**：`tool_guide` section 中嵌入完整调用示例 + 好坏对照。

### Step 4：判定领域适配（Adaptation）

每提炼一个模式，问：
- **保留**：通用行为约束 / 工具契约 / 工作流 / Few-shot
- **丢弃**：领域特定内容（医疗、心理、Claude 产品宣传、版权等）
- **改造**：把 Claude 的"内容审核"改成 Codecast 的"代码安全"；把 Aider 的"SEARCH/REPLACE 块"改成 Codecast 的 edit_file 行为

---

## 🧬 已提炼的核心模式库（按主题分类）

### A. 结构层

| 模式 | 来源 | 变体 |
|------|------|------|
| XML 标签分章节 | Claude Fable 5 | claude-style |
| 二维分解（mode × format） | Aider | architect-edit |
| Trust-declaration message 对 | Aider | （待集成到 search-then-edit 运行时） |
| 运行时动态注入槽位 | Aider `final_reminders` | Codecast 已有 RenderInputs 变体插值 |

### B. 行为约束

| 模式 | 来源 | 变体 |
|------|------|------|
| 标准化约束词（MUST/NEVER/ONLY EVER） | Aider | format-locked |
| 12+ 条反模式 | Claude Fable 5 | default / safety-first |
| 分级反馈（🔴/🟡/🟢） | Claude Fable 5 | code-reviewer |
| 反向 push back（事实 vs 风格 vs 方向） | Claude Fable 5 | mentor-coach |
| 严格 scope 控制 | Aider overeager | overeager-mode |

### C. 工作流

| 模式 | 来源 | 变体 |
|------|------|------|
| 决策流评估（Step 0/1/2/3） | Claude Fable 5 | decision-tree |
| 回复前 5 步自检 | Claude Fable 5 | self-check |
| 两阶段 Triage → Edit | Aider | search-then-edit |
| 双 Agent 协作（Plan → Edit） | Aider | architect-edit |
| 5 步评估（输出形态/信息需求/工具/排序/确认） | 自研 | decision-tree |

### D. Few-shot / 对话式

| 模式 | 来源 | 变体 |
|------|------|------|
| `<good_response>/<bad_response>` 对照 | Claude Fable 5 | claude-style |
| 对话式注解（💡✅🤔） | 自研 | pair-programmer |
| 教学机会主动指出 | 自研 | pair-programmer |
| 分类示例（test/build/cleanup） | Aider shell | shell-only |

### E. 工具与执行

| 模式 | 来源 | 变体 |
|------|------|------|
| 工具契约显式化 | Cursor/Cline | claude-style / mcp-router |
| 检索 vs 读盘偏好 | Cursor Composer | （待融入 default） |
| 增量编辑 vs 覆盖 | Cursor Composer | （待融入 default） |
| 1-3 one-liner 约束 | Aider | shell-only |
| 完整实现（反 TODO） | Aider lazy | lazy-mode |

### F. 角色与人格

| 模式 | 来源 | 变体 |
|------|------|------|
| 温暖 + 教学 + 边界 | Claude Fable 5 | mentor-coach |
| 拒绝讨好 | Claude Fable 5 | mentor-coach |
| 用户不友善时一次警告 | Claude Fable 5 | mentor-coach |

### G. 错误处理

| 模式 | 来源 | 变体 |
|------|------|------|
| Repair prompt（解析失败） | Aider | format-locked |
| 5 步自检（准确性/边界/可逆性/验证/诚实） | 自研 | self-check |

### H. 选择策略

| 模式 | 来源 | 变体 |
|------|------|------|
| A/B 框架（fixed / round-robin / weighted） | 自研 + Aider 模式 | promptab.Selector |
| 项目级 vs 用户级 vs 嵌入 | 自研 | 4 层 fallback |

---

## ❌ 已识别的"反模式"（不该做什么）

1. **不要伪造原文** —— 如果没找到完整提示词，明确说"没找到"，给来源 URL 让人工验证
2. **不要把领域特定内容搬过来** —— Claude 的儿童安全、医疗、Claude 产品宣传对开发工具无意义
3. **不要照搬格式而失本质** —— 比如 Claude 的 XML 标签如果只在不重要的章节有，不必每个变体都用
4. **不要 1:1 复制** —— 每个变体应该至少有一个"Codecast 专属"特征
5. **不要破坏现有架构** —— 17 个变体共享同一个 Variant/Registry 接口
6. **不要跳过测试** —— 每个新变体至少 1 个结构化测试（验证含特定 marker）

---

## 📊 提炼工作的总账

| 维度 | Claude Fable 5 | Aider | 累计 |
|------|---------------|-------|------|
| 提示词原文 | 3825 行 | 多个 _prompts.py | - |
| 核心模式 | 6 | 7 | 13+ |
| 新增变体 | 8 (claude-style / code-reviewer / pair-programmer / decision-tree / self-check / scope-guard / mcp-router / mentor-coach) | 6 (search-then-edit / format-locked / architect-edit / shell-only / lazy-mode / overeager-mode) | 14 |
| 测试用例 | 3 | 6 | 9+ |
| 借鉴价值 | 结构 + 人格 + 决策 | 约束词 + 工作流 + 工具契约 | - |

---

## ⏭️ 可继续提炼的资料

| 资料 | 预期价值 | 状态 |
|------|---------|------|
| **Claude Code** 提示词 | 极高（直接对标） | ❌ 未启动 |
| **Cursor Composer** 提示词 | 高（产品体验标杆） | ❌ 网络受限 |
| **Cline** 提示词 | 中高（VSCode 之王） | ❌ 未启动 |
| **Continue.dev** 提示词 | 中（开源） | ❌ 未启动 |
| **Aider prompt 细节补充** | 中 | 已有基础 |
| **OpenAI Codex CLI** | 中 | ❌ 未读 |
| **Devin / Cognition** | 高（agent 标杆） | ❌ 难拿 |

---

## 🛠️ 提炼工作流（建议步骤）

1. **找资料**（GitHub 仓库 / 官方文档 / 学术镜像 / 社区泄露）
2. **初步筛选**（看是否值得深入）
3. **提取原文**（优先 verbatim 提取，章节标注）
4. **模式提炼**（每个模式 1-2 句 + line:N 引用）
5. **变体设计**（"这个模式适合 codecast 什么场景？"）
6. **编码实现**（写 Variant + RawSections）
7. **测试**（结构化 marker 验证）
8. **文档**（CHANGELOG + README 更新）
9. **沉淀**（写本笔记或参考文档）

---

## 📌 关键原则（教训沉淀）

- **可重现优先于完整性** —— 宁可少提炼几个变体，每个都跑通测试
- **变体名要可解释** —— `search-then-edit` 比 `mode-2` 好
- **共享 Section 接口** —— 所有变体都用 identity / tool_guide / anti_patterns / workflow / output_format 这套骨架
- **Description 是用户决策依据** —— `/prompt list` 看到 description 必须能 5 秒判断要不要用
- **测试断言要有意义** —— 验证 "变体含 X marker" 不如验证 "X marker 的上下文语义"
- **失败要可见** —— 加载失败、变体缺失、模板渲染空都要有显式错误
- **反引号 raw string 限制** —— Go 反引号 raw string 不能含反引号，长模板要拆段或避免

---

## 🎓 推荐阅读路径

对于想理解 Codecast 提示词设计哲学的开发者：

1. 先读本笔记 → 了解"为什么有 17 个变体"
2. 读 [`aider-system-prompt-reference.md`](aider-system-prompt-reference.md) → 学习 Aider 模式
3. 读 Claude Fable 5 原始提示词 → 学习 Claude 模式
4. 读 `internal/promptab/embedded.go` → 看实现
5. 读 `internal/promptab/compat_test.go` → 看测试

---

**最后更新**: 2026-06-13 · 17 个变体 · 35 包 / 0 FAIL
