# Changelog

本文件记录 CodecastCLI 的所有重要变更。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [1.0.0] - 2026-06-17

> 首个稳定版本：补齐与世界顶级工具（Aider / Claude Code）的差距

### 新增

- **智能模型路由（L1 + L2）**
  - L1 特征分类：按请求类型（问答/编辑/重构/调试/多文件）路由到合适模型
  - L2 学习型路由器：Wilson 置信区间 + epsilon-greedy，根据历史成功率自适应优化
  - `/route stats` 查看路由统计，`/route reset` 清空学习数据
  - 路由缓存避免重复决策

- **子 Agent 自动并行编排**
  - Plan→Execute DAG 编排，自动检测文件冲突决定并行/串行
  - 无冲突任务自动并行执行，有冲突自动串行
  - `/subagent "任务"` 手动触发，`/dag` 查看 DAG
  - 隔离沙箱执行，互不干扰

- **模糊编辑（Fuzzy Edit）**
  - Levenshtein 距离模糊匹配，容忍缩进/空白差异
  - 置信度 > 0.85 自动应用，0.6~0.85 询问确认，< 0.6 报错
  - 告别旧版"少一个空格就失败"的纯字符串替换

- **代码库语义索引（P3）**
  - tree-sitter 符号切块 + embedding 向量检索 + BM25 关键词检索
  - 混合检索：向量 0.7 + BM25 0.3 加权融合
  - 增量更新：文件 modtime 变化时只重算该文件
  - JSON 持久化，零外部依赖（无 CGO/sqlite-vec）
  - 支持 OpenAI / 智谱（Zhipu）/ 通义（DashScope）embedding provider
  - `/semantic index` 建索引，`/semantic query` 检索，`/semantic stats` 统计

- **基准测试框架**
  - 15 个内置任务（5 类型 × 3 难度）：question / edit / refactor / debug / multi-file
  - 指标：成功率 / 延迟 / token / 成本 / 工具调用数
  - Mock Runner 支持 CI 无网络测试
  - `/benchmark run` 运行，`/benchmark list` 查看任务

- **国内 Embedding Provider 支持**
  - 智谱（Zhipu）：embedding-3，2048 维，OpenAI 兼容接口
  - 通义（DashScope）：text-embedding-v3，1024 维，OpenAI 兼容接口
  - embedding 专用 API Key 和 BaseURL 配置（`embedding_api_key` / `embedding_base_url`）

- **tree-sitter 符号集成到语义索引**
  - 语义索引按 tree-sitter 提取的函数/类/方法边界切块（替代固定行数切块）
  - SymbolExtractor 接口解耦 semantic 与 indexer 包，避免循环依赖
  - 无提取器时自动退化为固定行数切块

- **试用指南与反馈模板**
  - `docs/trial-guide.md`：6 大新功能体验指南
  - `docs/trial-feedback.md`：结构化反馈收集模板

### 变更

- 版本号从 0.4.0 升至 1.0.0（首个稳定版）
- 语义索引配置新增 `embedding_provider: zhipu / dashscope` 选项
- 语义索引配置新增 `embedding_api_key` / `embedding_base_url` 字段（为空则复用主配置）

### 测试

- 新增 ~30 个测试覆盖路由 / 子 Agent / 模糊编辑 / 语义索引 / 基准测试
- 全部测试通过：`go test ./internal/... ./cmd/...`

## [0.4.0] - 2026-06-14

> Phase 2-5 全量交付：从「功能完整」到「生产就绪」

### 新增

- **智能代码理解（Phase 2）**
  - tree-sitter AST 解析（Go / Python / TypeScript / JavaScript）
  - LSP 集成（gopls / pyright / tsserver），自动检测并友好降级
  - 增量索引 + 磁盘缓存，大仓库索引 < 3s

- **Bubble Tea 现代 TUI（Phase 2）**
  - 可选 TUI 模式（`--tui=bubbletea`），go-prompt 仍为默认 REPL
  - 实时流式渲染 > 30fps，DAG 可视化

- **隔离子 Agent 并行执行（Phase 3）**
  - `/delegate` 触发多 Agent DAG 协作，各子 Agent 运行在独立沙箱
  - Plan+Execute 双 Agent 编排，支持 pipeline / parallel / handoff

- **@file 引用 + 多行输入 + 状态栏（Phase 3）**
  - `@path/to/file` 自动注入文件内容到 LLM 上下文
  - 多行编辑模式、实时 Token 计数与成本状态栏

- **Git-Aware AI（Phase 4）**
  - `/review [branch]` — AI 代码审查，自动 diff + blame 注入
  - `/blame <file>` — 将 Git blame 历史注入上下文
  - `/history <file>` — 将 commit 历史注入上下文
  - `/diff [branch]` — 分支 diff 分析

- **智能模型路由（Phase 4）**
  - `/route` — 按任务复杂度自动选择最优模型
  - 成本优化策略：简单任务用小模型，复杂任务用强模型
  - 路由缓存，避免重复决策

- **goreleaser 多平台分发（Phase 3）**
  - Homebrew（macOS/Linux）、Scoop（Windows）、DEB / RPM
  - 5 平台预编译二进制（linux-amd64/arm64、darwin-amd64/arm64、windows-amd64）

- **VS Code 扩展（Phase 3）**
  - 编辑器内启动 Codecast，文件上下文自动注入

- **错误码体系（Phase 5）**
  - `internal/errors` 包：`UserFacingError` + 20+ `ErrorCode` 常量
  - `DegradationMatrix`：优雅降级矩阵，跟踪各模块降级状态
  - 每个错误包含 Code / Message / Hint，用户可直接获得修复建议

### 改进

- **启动流水线并行化（Phase 5.1）**
  - `newAgent()` 重构为 5 阶段并行初始化（A-E 流水线）
  - Phase A 并行 6 个 goroutine（Provider、Toolkit、Memory、Permission、Rules、Indexer）
  - 冷启动时间大幅降低

- **代码拆分与质量（Phase 5.2）**
  - `cmd/interactive.go` 2250 行 → 456 行 + 5 个专职文件
    - `interactive_commands.go` (634 行) — /config /cost /session 等管理命令
    - `interactive_handlers.go` (381 行) — /rules /model /plan /delegate 等处理器
    - `interactive_git.go` (305 行) — /review /blame /history /diff
    - `interactive_files.go` (152 行) — @file 引用展开
    - `interactive_session.go` (172 行) — 会话导入导出
  - `internal/agent/agent.go` 1141 行 → 944 行 + `agent_hooks.go` (193 行)

- **Ctrl+C 中断当前请求** — SIGINT 立即中止流式输出
- **grep 性能升级** — 自动感知 .gitignore，跳过忽略文件
- **API 重试与 Provider 降级** — 网络错误自动重试 3 次，失败后降级到备用 Provider
- **摘要式上下文压缩** — 超长对话自动摘要，保留关键上下文
- **预算感知上下文压缩** — 按剩余预算动态调整上下文长度

### 测试

- 39 个包，全部通过（含 errors 包 4 个新增测试）
- `go build ./...` 零错误

---

## [0.3.0] - 2026-06-13

### 新增

- **提示词 A/B 框架（v0.3.0 核心特性）**
  - `internal/promptab` 包：Variant / Registry / Selector / Render
  - 11 个嵌入变体：
    - 通用风格：default / concise / safety-first
    - 借鉴 Claude Fable 5：claude-style / decision-tree / self-check / mcp-router / mentor-coach
    - 任务专精：code-reviewer / pair-programmer
    - 安全加固：scope-guard
  - 4 种外部化方式（按优先级）：
    1. 编译时嵌入（保证 fallback）
    2. `~/.codecast/prompts/*.yaml`（用户级）
    3. 项目级 `.codecast/prompts/*.yaml`（按 cwd 自动加载）
    4. 自定义目录（`--prompt-project-dir` 覆盖）
  - 3 种选择策略：
    - fixed（按名指定）
    - round-robin（按权重轮转）
    - weighted（按权重概率选）

- **`/prompt` 斜杠命令**（运行时管理变体）
  - `list` / `use <name>` / `show <name>` / `current` / `reload`
  - 别名 `/p`
  - 切换后立即生效，无需重启
  - `use` 命令持久化到 `~/.codecast/config.yaml`

- **`/config` 新字段**
  - `prompt_variant` / `prompt_strategy` / `prompt_weights` / `prompt_project_dir`
  - 校验：strategy 限定 `fixed|round-robin|weighted|weighted-random`

- **CLI flags**
  - `--prompt-variant <name>`
  - `--prompt-strategy <strategy>`
  - `--prompt-weight key=value`（可多次）
  - `--prompt-project-dir <path>`
  - 环境变量：`CODECAST_PROMPT_VARIANT` / `CODECAST_PROMPT_STRATEGY`

- **Cost 埋点（A/B 数据基础）**
  - `cost.Tracker` 加 `prompt_variant` 列（schema 幂等迁移）
  - `RecordWithVariant(...)` API
  - `SummaryByVariant()` 聚合查询
  - `RecentRecords` 读 `prompt_variant` 字段
  - `/cost by-variant` 子命令可视化

- **CI 变体兼容性测试**
  - 矩阵：3 变体 × 5 上下文 = 15 个组合
  - 验证：渲染成功 / 无 `{{var}}` 残留 / 长度合理
  - `.github/workflows/ci.yml` 新增 `prompt-compat` job（3 平台）
  - 验证 CLI flag 真实存在

- **斜杠命令参数化**（v0.3.0 早期合并）
  - 新包 `internal/commands` 解析 YAML frontmatter + 模板引擎
  - 支持 `{{var}}` / `{{var|"fallback"}}` / `$ARGUMENTS` / `$ARG0..9` / `key=value` 注入
  - 重写 `.codecast/commands/{explain,review,test}.md` 加上 audience/depth/format/focus

### 修复

- **F-01** `--scope` 文件作用域真正生效（之前 LLM 仍可读 `/etc/passwd`）
- **F-02** `/mode` 切换不再静默清除 SafeMode 黑名单
- **F-03** go-prompt 与 confirm 不再抢 stdin
- **F-04** MCP 启动错误暴露给用户
- **F-05** agent 与 session 共享 SQLite 连接
- **F-06** YAML 解析错误不再静默吞掉
- **F-07** 大仓库索引时显示 spinner
- **F-08** edit_file 加空白容差
- **F-09** SwitchModel 重建索引器并重新注入 scope policy
- **F-10** `os.Exit(1)` 替换为 `return error`
- **F-12** `cost.Tracker.Record` 加锁
- **顺带**：pickByWeight round-robin 永远返回 `available[0]`（加 strict 轮转）
- **顺带**：EmbeddedVariants() 不调 Parse() 导致 Render 0 字节（显式 Parse）
- **顺带**：COALESCE 不替换空串（改用 NULLIF）
- **顺带**：RecentRecords 没读 prompt_variant（修扫描列）

### 测试

- 35 个包，全部通过
- 60+ 个新增测试用例
  - 12 个 promptab 基础
  - 5 个 compat 矩阵
  - 3 个 claude-style
  - 1 个 mentor-coach
  - 4 个 decision-tree 系列
  - 4 个 cost variant
  - 4 个 prompt resolver
  - 11 个 SelectorConfig
  - 3 个 LoadProjectDir
  - 其余散落各处

### 文档

- README 新增"提示词 A/B 框架"章节（11 变体表 + 4 用法）
- 完整化所有变体的 description / author

### 新增（v0.3.0 增量）

在 v0.3.0 发布后从 Aider 提示词架构提炼 6 个新变体（累计 17）：

- **`search-then-edit`** —— 借鉴 Aider 两阶段工作流
  - Phase 1 Triage（只读 + 输出建议文件列表）→ 停止
  - → 用户 add 文件 → Phase 2 Edit（仅可改 add 列表里的文件）
  - 核心价值：模型永远不"擅自改用户没授权的文件"

- **`format-locked`** —— 借鉴 Aider 标准化约束词
  - 词典：MUST / NEVER / ONLY EVER / ALWAYS + repair prompt
  - 灵感：Aider editblock 99%+ 解析率靠这套
  - 适合：CI、自动化、agent 间通信

- **`architect-edit`** —— 借鉴 Aider 双 Agent 协作
  - Plan-Agent（出自然语言方案）→ Edit-Agent（转代码）
  - 核心价值："想"和"做"分离到两次 LLM 调用

- **`shell-only`** —— 借鉴 Aider shell 工具契约
  - 1-3 one-liner 硬约束、不写脚本、不用占位符
  - 按场景给分类示例（test/build/debug/cleanup/install）

- **`lazy-mode`** —— 借鉴 Aider `lazy_prompt`
  - 禁 TODO / 伪代码 / "..." / "实现略"
  - 强制：每个函数都有完整实现

- **`overeager-mode`** —— 借鉴 Aider `overeager_prompt`
  - 严禁"顺手"：不顺手格式化、不顺手修 bug、不顺手优化
  - 看到问题单独列出来等用户决定

测试：
- TestEmbeddedVariantsLoad 扩到 17 个变体
- 新增 aider_test.go (95 行, 6 测试)
- 35 包全过 / 0 FAIL

## [0.3.1] - 2026-06-14

在 v0.3.0 的 6 个新变体基础上**沉淀与文档化**：

### 新增

- **`docs/VARIANT-SELECTION-GUIDE.md`**（用户文档，~280 行）
  - 30 秒速选：3 个问题帮你选变体
  - 详细决策树（文本版）
  - 3 大场景实战指南（日常/A/B/项目级）
  - 速查表（粘桌面）
  - 常见错误选择 vs 正确选择
  - 17 变体速查卡
  - 试验性建议（6 步试用顺序）
  - 故障排查

- **`docs/review/PROMPT-DISTILLATION-NOTES.md`**（方法论文档，~290 行）
  - 4 步提炼法（结构 → 边界 → Few-shot → 适配）
  - 13+ 核心模式库（按 8 大主题分类）
  - 6 条"反模式"
  - 提炼总账（Claude Fable 5 + Aider 双资料）
  - 可继续提炼的资料清单
  - 关键原则沉淀

### 修复

- README Version badge 从 0.1.0 → 0.3.1（之前漏改）

### 维护

- 升级到 v0.3.1（patch bump，仅文档增量）
- 预编译 5 平台二进制
  - codecast-linux-amd64 / linux-arm64 / darwin-amd64 / darwin-arm64 / windows-amd64.exe
  - 每个 ~21MB
  - ldflags 注入 Version / GitCommit / BuildDate

## [0.2.0] - 2026-06-13

### Breaking Changes

### Breaking Changes

- **所有 `codecast <cmd>` 子命令已迁移到交互模式 `/<cmd>` 斜杠命令**（v0.2.0 全面迁移）
  - 涉及命令：`config`, `cost`, `session`, `mcp`, `plugin`, `pool`, `rag`, `sandbox`, `workflow`
  - 每个 shell 子命令的根命令现在仅显示迁移提示，不再注册任何子命令
  - 实际功能请在交互模式（`codecast`）中通过对应斜杠命令完成
  - 新增的 `/<cmd>` 斜杠命令：
    - `/config [list|get|set|wizard|providers|init]`
    - `/cost [summary|daily|list|clear]`
    - `/session [list|show|delete|export]`
    - `/mcp [list|add|remove|test|templates|categories|connect|disconnect]`
    - `/plugin [list|install|unload|available]`
    - `/pool [status|run]`
    - `/rag [index|query|chat]`
    - `/sandbox [status|build]`
    - `/workflow [pipeline|parallel|handoff]`
  - 保留的 shell 命令（不迁移）：`codecast chat`, `codecast exec`, `codecast init`, `codecast version`, `codecast man`
  - 理由：在交互模式内管理各类子命令免去用户退出 REPL 来回切换的成本
  - 受影响范围：所有使用 `codecast <cmd> <sub>` 的脚本需改为先 `codecast` 进入交互模式后调用 `/<cmd> <sub>`

### Internal

- 抽离 9 个子命令的内部实现为可复用函数（`costRunSummary`, `sessionRunList`, `mcpRunAdd`, `pluginRunList`, `poolRunRun`, `ragRunQuery`, `sandboxRunBuild`, `workflowRunPipeline` 等），供 `/<cmd>` 斜杠命令与将来的扩展使用
- 修正 Plugin API 使用：`InstallPlugin`/`UnloadPlugin`（原 `Install`/`Unload`），`PluginInfo` 字段 `Name/Version/Count`（无 `Description`）
- 修正 Pool API 使用：`DispatchTasks(ctx, []TaskDefinition)` 替代不存在的 `Submit([]string)`

### Removed

- **删除 TypeScript/Ink 子树**（F-13 闭环）
  - 删除目录：`src/`、`tests/`、`dist/`、`node_modules/`
  - 删除文件：`package.json`、`package-lock.json`、`tsconfig.json`、`tsconfig.test.json`
  - 原因：`src/` 是早期 TS/Ink 实验的遗留代码，从未被 `main.go` 调用、CI 不跑 npm test、`npm test` 通过的 268 个测试均为 TS 内部自洽性测试，与生产代码路径无关
  - 净释放：仓库约 45 MB（其中 node_modules 占 44.5 MB）
  - 移除 6 个第三方 npm 依赖：`chalk`、`dotenv`、`eventsource`、`ink`、`ink-spinner`、`ink-text-input`、`react`
  - 如需回滚：备份在 `$TEMP/codecast-ts-backup-20260613-1907.zip`（12.8 MB 压缩）

### Security

- **F-01 修复：`--scope` 文件作用域真正生效**（之前 `ap.WithFileScope` 是装饰性的，LLM 仍可读 `/etc/passwd`）
  - 在 AgentPrimordia 框架加 `Registry.WithScopePolicy` + executor 回退 + pkg 导出
  - codecastcli `newAgent` / `SwitchModel` 真正调用
- **F-02 修复：`/mode` 切换不再静默清除 SafeMode 黑名单**
  - `permission.Manager` 引入 `userAllowed` map，`SetMode` 保留用户构建的白名单
  - `handleModeCommand` 改用 `SetMode` 替代 struct 覆盖
  - 新增 2 个回归测试 `TestModeSwitchPreservesDenyList`、`TestSetModePreservesAutoAllow`
- **F-03 修复：go-prompt 与 confirm 不再抢 stdin**
  - `ConfirmPrompt` 加 ANSI 颜色边框、`os.Stdout.Sync()` 强 flush、EOF → deny 兜底
  - 8 个新测试覆盖 yes/no/always/edit/cancel/EOF/arg-truncation 路径
- **F-04 修复：MCP 启动错误暴露给用户**
  - `ConnectMCPServers` 返回 `[]MCPWarning`，`runInteractive` 启动时显示

### Fixed

- **F-05 修复：agent 与 session 共享 SQLite 连接**
  - 框架加 `SQLiteStore.GetDB()` 暴露底层 `*sql.DB`
  - agent 存 `sharedDB` 字段并通过 `GetSharedDB()` 暴露
  - `session.NewManager()` 优先级：显式参数 > 进程级 sharedDB > 自开
  - 2 个新测试验证 fallback + override 路径
- **F-06 修复：YAML 解析错误不再静默吞掉**
  - `mcpcfg.Load() (*Config, error)` 返回 error
  - 5 个 callsite 更新，2 个新测试
- **F-07 修复：大仓库索引时显示 spinner**
  - `indexer.BuildWithCallback` 支持每文件回调；agent 包裹 `ui.StartSpinner`
- **F-08 修复：edit_file 加空白容差**
  - `tolerantNormalize` + `findClosestMatch` 让 LLM 缩进漂移也能匹配
  - 加注释说明 `os.Rename` 对 symlink 的行为
- **F-09 修复：SwitchModel 重建索引器并重新注入 scope policy**
- **F-10 修复：`os.Exit(1)` 替换为 `return error`**
  - `runInteractive() error`；`cmd/root.go` 统一处理退出码
  - 测试和包装函数现在可以捕获失败

### Added

- **F-12 修复：cost.Tracker.Record 加锁**
  - `mu.Lock()` 包裹 INSERT，避免并发场景下的 `SQLITE_BUSY` 重试风暴
  - 新增 `TestRecordConcurrent`（10 goroutine × 20 record，验证 200 条全部落库）

## [0.1.0] - 2026-06-12

### 新增

- 基于 AgentPrimordia 框架的 AI 终端 Agent
- 13+ LLM Provider 支持 (OpenAI/Anthropic/Gemini/Ollama/DeepSeek/Qwen/GLM/Cohere/Mistral)
- 智能代码库索引 (自动注入文件树到系统提示词)
- 三级权限模型 (suggest/auto-edit/full-auto)
- TUI Markdown 实时渲染 (glamour + lipgloss)
- 流式输出 (逐 token 实时渲染 + 防抖)
- Plan+Execute 双 Agent DAG 协作
- Hooks 系统 (10 个钩子点，支持 Shell 脚本)
- 多模态图片分析
- 插件市场 (远程搜索/下载/缓存)
- 分布式 Agent Pool
- MCP 协议支持 (10 个内置模板)
- 自动 Git Checkpoint (stash/commit)
- Undo/Rollback (文件修改前自动备份)
- 成本预算控制 (日/会话级别)
- 智能上下文管理 (自动压缩)
- 交互式配置向导
- Shell 补全 (Bash/Zsh/Fish/PowerShell)
- Man Page 生成
- 懒加载模块 (减少启动时间)
- Headless 模式 (text/json/stream-json)
- 项目规则三级加载 (全局/项目/自动学习)
- Diff 预览 (红绿对比)
- 模型运行时切换 (重建 Provider)
- 多配置 Profile
- CI/CD (3 平台测试 + 5 目标交叉编译)
- 自动发布工作流

### 测试

- 33 个包，100+ 测试用例，全部通过
