# Codecast CLI 第二阶段实施计划 — 权限管理 / 上下文压缩 / 项目记忆 / Headless 模式

> 版本: v1.0 | 日期: 2026-06-12
> 前置: 第一阶段（TUI 升级 / 结构化编辑 / MCP 持久化 / 会话恢复）已完成，BUG-FIX-PLAN.md 中的 12 个 bug 应先行修复
> 目标: 补齐安全沙箱、长会话管理、项目知识沉淀、CI/CD 集成四项能力，达到与 Claude Code / Codex CLI 同一竞争梯队的功能水位

---

## 总体策略

本计划按"权限管理 → 上下文压缩 → 项目记忆 → Headless 模式"的顺序推进。前两个模块是安全和稳定性基础设施，后两个模块是差异化竞争力。四个模块之间存在松耦合关系，但建议严格按序实施以降低回归风险。

**实施顺序与依赖关系：**

```
模块 1: 权限管理 ──────────▶ 模块 4: Headless 模式（依赖权限管理的自动审批策略）
       │
       ▼
模块 2: 上下文压缩 ──▶ 模块 3: 项目记忆（压缩后的会话摘要可注入项目记忆）
```

**新增依赖：**

| 依赖包 | 用途 | 模块 |
|--------|------|------|
| `gopkg.in/yaml.v3`（已在 go.mod） | 项目规则文件解析 | 模块 3 |
| `encoding/json`（标准库） | Headless JSON 输出 | 模块 4 |
| `os/signal`（标准库） | Headless 优雅退出 | 模块 4 |

无需修改 `AgentPrimordia` 框架代码。所有新增功能均通过 AP 框架已有的公开 API（`WithHITL`、`WithContextWindow`、`WithSummarizer`、`WithFileScope`、`CompressStrategy`）完成集成。

**总预估工时：约 28 小时**

---

## 模块 1: 权限管理（Permission Management）

### 1.1 现状问题

当前 `internal/agent/agent.go` 的 `Process()` 方法（第 232-238 行）有一个 `SafeMode` 配置项，但实现形同虚设——仅打印一行警告，实际并未禁用任何工具。所有工具调用（包括 `shell_execute`、`write_file`、`edit_file`）均不经用户确认直接执行。

对比竞品的权限体系：

| 工具 | 权限模型 |
|------|----------|
| Claude Code | 三级审批（suggest / auto-edit / full-auto），ToolUse permission 层拦截每个工具调用，OS 级沙箱（Landmark/Seatbelt） |
| Codex CLI | Landlock（Linux）/ Seatbelt（macOS）内核沙箱 + Starlark 策略脚本 |
| Qoder CLI | 工具级权限声明 + 用户确认弹窗 |

AgentPrimordia 框架已经提供了完善的 HITL（Human-in-the-Loop）基础设施：`HITLConfig`、`HITLManager`、`ShouldInterrupt(toolName, reason)`、`RequestInterrupt(ctx, req)`、`Resume(response)`，以及 `ToolPermission` 的 `ConfirmationFunc` 回调。codecastcli 目前完全未使用这些能力。

### 1.2 技术方案

#### 1.2.1 三级审批模式

对标 Claude Code 的三级模式，新增 `--permission-mode` 全局 flag，支持三种审批策略：

| 模式 | 名称 | 行为 |
|------|------|------|
| `suggest` | 建议模式 | 所有工具调用前弹出确认提示，用户输入 y/n 决定是否执行。对标 Claude Code 默认模式 |
| `auto-edit` | 自动编辑 | 文件读写类工具自动放行，Shell 命令仍需确认。对标 Claude Code `--dangerously-skip-permissions` 的折中 |
| `full-auto` | 全自动 | 所有工具自动放行，无任何确认。仅建议在沙箱/CI 中使用 |

**新增文件：** `internal/permission/manager.go`

```go
package permission

// ApprovalMode 审批模式
type ApprovalMode int

const (
    ModeSuggest   ApprovalMode = iota // 全部确认
    ModeAutoEdit                       // 编辑类自动放行
    ModeFullAuto                       // 全部自动
)

// Manager 权限管理器
type Manager struct {
    mode       ApprovalMode
    autoAllow  map[string]bool // 工具级白名单
    denyList   map[string]bool // 工具级黑名单
}
```

核心方法：

1. `NewManager(mode ApprovalMode)` — 根据模式初始化，自动填充白名单/黑名单
2. `ShouldApprove(toolName string) bool` — 判断该工具是否需要用户确认
3. `BuildHITLConfig() ap.HITLConfig` — 生成 AP 框架的 `HITLConfig`，将确认逻辑桥接到 `HITLManager`

**工具分类规则（`auto-edit` 模式下）：**

| 类别 | 工具 | 行为 |
|------|------|------|
| 只读 | `read_file`, `list_dir`, `grep_search`, `glob_search`, `web_request` | 自动放行 |
| 编辑 | `write_file`, `edit_file` | 自动放行 |
| 危险 | `shell_execute` | 需确认 |
| MCP | 所有 MCP 工具 | 需确认（MCP 工具可能执行任意操作） |

#### 1.2.2 交互式确认流程

**改动文件：** `internal/permission/confirm.go`（新增）

当 `ShouldApprove` 返回 `true` 时，通过 AP 框架的 `HITLConfig.OnInterrupt` 回调触发确认流程：

```
Agent 请求调用: shell_execute
命令: rm -rf ./build
───────────────────────────────
允许执行? [y]es / [n]o / [a]lways-allow / [e]dit-args >
```

实现要点：

1. `OnInterrupt` 回调在 `cmd/interactive.go` 的终端上打印确认提示
2. 用户输入通过 `HITLConfig.HumanInputChan` 通道回传给 `HITLManager`
3. 支持 `always-allow` 选项：将该工具动态加入白名单，当前会话内不再询问
4. 支持 `edit-args` 选项：用户修改参数后重新提交

**与 AP 框架的对接方式：**

```go
// 在 agent.go 的 New() 中，构建 agent 时注入 HITL 能力：
hitlCfg := permMgr.BuildHITLConfig()
agent := ap.NewAgent("CodecastAgent", systemPrompt, llmProvider,
    ap.WithMaxTurns(20),
).WithToolkit(registry).WithMemory(memory).WithHITL(hitlCfg)
```

AP 的 ReAct 引擎在每次工具调用前自动调用 `HITLManager.ShouldInterrupt`，若需要确认则通过 `RequestInterrupt` 阻塞等待用户响应。

#### 1.2.3 SafeMode 的真正实现

**改动文件：** `internal/agent/agent.go`

当前 `Process()` 中的 SafeMode 仅是打印警告。修复方案：

1. 当 `cfg.SafeMode == true` 时，在创建 Registry 后移除 `shell_execute` 和 `web_request` 工具：
   ```go
   if cfg.SafeMode {
       registry.Unregister("shell_execute")
       registry.Unregister("web_request")
   }
   ```
2. 若 AP 的 Registry 不提供 `Unregister`，则用黑名单实现：`Manager` 的 `denyList` 中工具的 `ShouldApprove` 永远返回拒绝，`HITLManager` 的 `RequestInterrupt` 自动返回 `HumanResponse{Approved: false}`

#### 1.2.4 文件作用域限制

**改动文件：** `internal/agent/agent.go`

通过 AP 框架的 `WithFileScope` 限制 Agent 只能访问当前工作目录及其子目录：

```go
agent := ap.NewAgent(...).
    WithToolkit(registry).
    WithMemory(memory).
    WithHITL(hitlCfg).
    WithFileScope([]string{"."})  // 限制在 cwd 内
```

高级用法：通过 `--scope` flag 允许用户指定额外目录：
```bash
codecast --scope /home/user/projects --scope /tmp/scratch
```

### 1.3 文件变更清单

| 操作 | 文件路径 | 说明 |
|------|----------|------|
| 新增 | `internal/permission/manager.go` | 权限管理器核心（三级模式、工具分类、HITL 桥接） |
| 新增 | `internal/permission/confirm.go` | 交互式确认流程（终端提示、用户输入解析） |
| 新增 | `internal/permission/manager_test.go` | 单元测试（三种模式 × 四类工具 = 12 个用例） |
| 修改 | `internal/agent/agent.go` | 注入 `WithHITL` + `WithFileScope`，修复 SafeMode |
| 修改 | `cmd/root.go` | 新增 `--permission-mode` / `--scope` flag |
| 修改 | `cmd/interactive.go` | 在 REPL 中处理 `OnInterrupt` 回调，渲染确认提示 |
| 修改 | `internal/config/config.go` | 新增 `PermissionMode` 和 `Scopes` 配置字段 |

### 1.4 实施步骤

| 步骤 | 任务 | 预估 |
|------|------|------|
| 1 | 实现 `permission.Manager`：三级模式 + 工具分类 + `ShouldApprove` + `BuildHITLConfig` | 2h |
| 2 | 实现 `permission.confirm`：终端确认 UI + 四种用户操作（y/n/always/edit） | 1.5h |
| 3 | 改造 `agent.go`：注入 `WithHITL` + `WithFileScope`，修复 SafeMode | 1.5h |
| 4 | 改造 `cmd/root.go` + `config.go`：新增 flag 和配置字段 | 1h |
| 5 | 改造 `cmd/interactive.go`：OnInterrupt 回调与终端交互对接 | 1.5h |
| 6 | 编写单元测试 + 集成测试 | 1.5h |

**小计：9 小时**

### 1.5 风险评估

| 风险 | 等级 | 缓解方案 |
|------|------|----------|
| HITL 阻塞导致流式输出卡顿 | 中 | `RequestInterrupt` 使用带超时的 context，超时后自动拒绝 |
| go-prompt 与确认提示冲突 | 中 | 确认流程临时暂停 go-prompt，切换为 `bufio` 读取单字符输入 |
| `WithFileScope` 对 MCP 工具无效 | 低 | MCP 工具的权限由 `Manager` 的黑名单/确认流程覆盖 |

---

## 模块 2: 上下文压缩（Context Compression）

### 2.1 现状问题

当前 `CodecastAgent` 创建 Session 后，对话历史无限增长，无任何压缩或裁剪机制。长会话（超过 30-50 轮对话）会导致：

1. **Token 超限**：发送给 LLM 的上下文超过模型窗口限制，请求失败
2. **成本飙升**：即使未超限，超长上下文的 token 消耗呈线性增长
3. **响应变慢**：模型处理长上下文时延迟显著增加

竞品方案：

| 工具 | 压缩策略 |
|------|----------|
| Claude Code | 四阶段管线：保留最近 N 条 → LLM 摘要旧对话 → Token 计数裁剪 → 系统消息保护 |
| Codex CLI | "stop-the-world" 全量摘要：达到阈值时暂停对话，整体摘要后继续 |
| OpenCode | 滑动窗口 + 可配置的上下文预算 |

AP 框架已提供完整的上下文管理能力：

1. `ContextWindowStrategy` 接口 + `DefaultStrategy`（简单截断）+ `CompressStrategy`（LLM 摘要压缩）
2. `SummaryEngine` + `WindowSummaryStrategy` + `Summarizer`（记忆层摘要）
3. `CapabilityAgent.WithContextWindow(strategy)` 链式注入
4. `CapabilityAgent.WithSummarizer(summarizer)` 链式注入

### 2.2 技术方案

#### 2.2.1 双引擎架构

采用"上下文裁剪 + 记忆摘要"双引擎并行的架构，对标 Claude Code 的四阶段管线：

```
用户输入 → [引擎 1: 上下文裁剪] → [引擎 2: 记忆摘要] → 发送给 LLM
              ↓                        ↓
         CompressStrategy        SummaryEngine
         (裁剪消息历史)         (异步提取摘要存入记忆)
```

**引擎 1：上下文裁剪（同步，每次请求前执行）**

利用 AP 的 `CompressStrategy`，配置参数：

```go
compressCfg := agent.CompressConfig{
    MaxTokens:        8000,  // 压缩后最大 8K token
    SummaryModel:     llmProvider,  // 复用当前 Provider（后续可换 flash 模型降低成本）
    KeepSystemMessages: true,
    KeepRecentN:      4,     // 保留最近 4 条消息不压缩
    CompressRatio:    0.3,   // 保留 30% 的 token
}
strategy := agent.NewCompressStrategy(compressCfg)
agent.WithContextWindow(strategy)
```

`CompressStrategy.Trim()` 的行为：
1. 消息数 ≤ `effectiveMax`（默认 20）时原样返回
2. 否则分离系统消息（保留）和非系统消息
3. 非系统消息分为 `recentMsgs`（最近 4 条）和 `oldMsgs`
4. 用 LLM 将 `oldMsgs` 压缩为一条 `[对话摘要]` 系统消息
5. LLM 失败时降级为 `fallbackSummary`（取首尾消息拼接）

**引擎 2：记忆摘要（异步，后台定期执行）**

利用 AP 的 `SummaryEngine` + `WindowSummaryStrategy`：

```go
summarizer := ap.NewSummarizer(llmProvider)
windowStrategy := ap.NewWindowSummaryStrategy(20) // 每 20 条记忆触发一次摘要
summaryEngine := ap.NewSummaryEngine(windowStrategy, summarizer, memory)
```

在 `Process()` 中每轮对话后异步触发：
```go
go func() {
    if result, err := summaryEngine.RunAndStore(ctx, a.sessionID); err == nil && result != nil {
        // 摘要已存入 SQLite 记忆，后续会话恢复时可检索
    }
}()
```

#### 2.2.2 Token 预算管理

**新增文件：** `internal/context/budget.go`

```go
package context

// TokenBudget 管理上下文 Token 预算
type TokenBudget struct {
    MaxTokens      int     // 总预算（默认模型上下文的 60%）
    ReserveSystem  int     // 系统消息预留（默认 2000）
    ReserveReply   int     // 回复预留（默认 2000）
    Available      int     // 可用于对话历史的 Token
}
```

功能：
1. 根据 Provider 的模型自动检测上下文窗口大小（从 config 中获取）
2. 计算可用于对话历史的 Token 预算 = 总窗口 × 0.6 - 系统预留 - 回复预留
3. 将预算注入 `CompressStrategy.MaxTokens`

**模型上下文窗口参考表：**

| 模型 | 上下文窗口 | 60% 预算 |
|------|-----------|----------|
| GPT-4o | 128K | 76.8K |
| Claude Sonnet | 200K | 120K |
| Gemini Pro | 1M | 600K |
| DeepSeek V3 | 128K | 76.8K |

#### 2.2.3 手动压缩命令

**改动文件：** `cmd/interactive.go`

新增 `/compact` 斜杠命令，用户可手动触发上下文压缩：

```
> /compact
✓ 上下文已压缩。旧消息 32 条 → 摘要 1 条，Token 减少约 78%。
```

实现：
1. 调用 `summaryEngine.RunAndStore(ctx, sessionID)` 生成摘要
2. 调用 `a.ClearContext()` 清除当前会话（会创建新 Session）
3. 将摘要作为新会话的第一条系统消息注入

#### 2.2.4 摘要模型优化

为降低压缩成本，建议使用廉价模型执行摘要任务：

```go
// 如果配置了 summary_model，使用它；否则复用主模型
summaryProvider := llmProvider
if cfg.SummaryModel != "" {
    summaryCfg := *cfg
    summaryCfg.Model = cfg.SummaryModel
    summaryProvider, _ = provider.CreateProvider(&summaryCfg)
}
```

配置文件 `~/.codecast/config.yaml` 支持：
```yaml
summary_model: "gpt-4o-mini"  # 摘要专用模型（可选）
```

### 2.3 文件变更清单

| 操作 | 文件路径 | 说明 |
|------|----------|------|
| 新增 | `internal/context/budget.go` | Token 预算管理 |
| 新增 | `internal/context/compress.go` | 上下文压缩配置工厂（封装 AP 的 CompressStrategy） |
| 新增 | `internal/context/budget_test.go` | Token 预算计算测试 |
| 修改 | `internal/agent/agent.go` | 注入 `WithContextWindow` + `WithSummarizer`，启动异步摘要 |
| 修改 | `internal/agent/stream.go` | 在 `StreamEventComplete` 后触发异步摘要 |
| 修改 | `cmd/interactive.go` | 新增 `/compact` 命令 |
| 修改 | `internal/config/config.go` | 新增 `SummaryModel`、`ContextBudget` 配置字段 |

### 2.4 实施步骤

| 步骤 | 任务 | 预估 |
|------|------|------|
| 1 | 实现 `context.TokenBudget`：模型窗口检测 + 预算计算 | 1.5h |
| 2 | 实现 `context.NewCompressConfig`：封装 AP CompressStrategy 的创建 | 1h |
| 3 | 改造 `agent.go`：注入 `WithContextWindow` + `WithSummarizer`，启动异步摘要 | 2h |
| 4 | 实现 `/compact` 命令 | 1h |
| 5 | 改造 `config.go`：新增配置字段 + YAML 解析 | 0.5h |
| 6 | 编写测试（Token 预算 + 压缩策略 + 摘要触发） | 1.5h |

**小计：7.5 小时**

### 2.5 风险评估

| 风险 | 等级 | 缓解方案 |
|------|------|----------|
| LLM 摘要调用失败 | 中 | `CompressStrategy` 已有 `fallbackSummary` 降级方案 |
| 异步摘要与主对话竞争 Memory 写入 | 低 | AP 的 SQLiteStore 内部有互斥锁保护 |
| 摘要模型与主模型不同导致风格不一致 | 低 | 摘要作为 system 消息注入，不影响对话风格 |
| 上下文裁剪过于激进丢失关键信息 | 中 | `KeepRecentN=4` + `KeepSystemMessages=true` 保底 |

---

## 模块 3: 项目记忆（Project Memory）

### 3.1 现状问题

当前 codecastcli 的系统提示词硬编码在 `agent.go` 中，无法感知项目特有的约定和规范。每次新会话都需要用户重复告知项目背景、编码规范、偏好设置等信息。

竞品方案：

| 工具 | 项目记忆文件 | 加载机制 |
|------|-------------|----------|
| Claude Code | `CLAUDE.md`（项目根目录 + 子目录） | 自动检测，注入系统提示词，支持层级覆盖 |
| Codex CLI | `AGENTS.md` | 自动加载，支持目录层级 |
| Qoder CLI | `.qoder/rules/*.md` | 自动扫描 rules 目录 |
| OpenCode | `.opencode/config.yaml` | 配置 + 提示词注入 |

### 3.2 技术方案

#### 3.2.1 项目规则文件规范

**文件位置（按优先级从高到低）：**

1. `.codecast/rules.md` — 项目级规则（放在项目根目录）
2. `.codecast/rules/*.md` — 分模块规则（按文件名排序加载）
3. `~/.codecast/rules.md` — 用户级全局规则

**加载顺序和合并策略：**

```
系统提示词 = 基础提示词 + 用户全局规则 + 项目根规则 + 项目子模块规则
```

各规则之间用分隔标记拼接，便于 LLM 区分来源：

```
[基础系统提示词]

=== 用户全局规则 ===
{~/.codecast/rules.md 内容}

=== 项目规则 ===
{.codecast/rules.md 内容}

=== 项目子模块规则 ===
{.codecast/rules/*.md 内容，按文件名排序}
```

#### 3.2.2 规则文件加载器

**新增文件：** `internal/rules/loader.go`

```go
package rules

// Loader 项目规则加载器
type Loader struct {
    projectRoot string
    configDir   string  // ~/.codecast
}

// LoadAll 加载并合并所有层级的规则文件
func (l *Loader) LoadAll() (string, error)

// LoadProjectRules 仅加载项目级规则
func (l *Loader) LoadProjectRules() (string, error)

// LoadGlobalRules 仅加载用户全局规则
func (l *Loader) LoadGlobalRules() (string, error)
```

实现要点：

1. `projectRoot` 通过 `os.Getwd()` 获取，或允许用户通过 `--project-root` flag 指定
2. 文件不存在时静默跳过（不报错）
3. 规则文件总大小限制为 16KB，超出时截断并警告
4. 规则文件中的 `{{.ProjectName}}`、`{{.CWD}}` 等模板变量自动替换

#### 3.2.3 规则文件示例

**`.codecast/rules.md` 示例：**

```markdown
# 项目规则

## 基本信息
- 项目名称: codecastcli
- 语言: Go 1.26
- 框架: AgentPrimordia (本地开发框架，位于 ../AgentPrimordia/)

## 编码规范
- 错误处理：使用 fmt.Errorf 包装错误，不要用 panic
- 命名：遵循 Go 标准命名约定
- 测试：所有新工具必须有对应的 _test.go 文件

## 构建命令
- 构建: `go build -o codecast .`
- 测试: `go test ./...`
- Lint: `golangci-lint run`

## 架构约定
- 新增工具放在 internal/tools/ 目录
- 所有 AP 框架接口从 pkg/ 包导入，不要导入 internal/
- MCP 配置存放在 ~/.codecast/mcp_servers.yaml
```

#### 3.2.4 系统提示词动态组装

**改动文件：** `internal/agent/agent.go`

将当前硬编码的系统提示词替换为动态组装：

```go
// 1. 加载项目规则
rulesLoader := rules.NewLoader(getCurrentDir(), config.GetConfigDir())
projectRules, _ := rulesLoader.LoadAll()

// 2. 组装系统提示词
systemPrompt := buildSystemPrompt(runtime.GOOS, getCurrentDir(), projectRules)
```

`buildSystemPrompt` 函数：

```go
func buildSystemPrompt(goos, cwd, projectRules string) string {
    var sb strings.Builder
    
    // 基础提示词
    sb.WriteString(basePrompt)
    
    // 注入环境信息
    sb.WriteString(fmt.Sprintf("\n当前操作系统: %s\n当前工作目录: %s\n", goos, cwd))
    
    // 注入项目规则
    if projectRules != "" {
        sb.WriteString("\n=== 项目规则 ===\n")
        sb.WriteString(projectRules)
        sb.WriteString("\n=== 规则结束 ===\n")
        sb.WriteString("\n请严格遵守上述项目规则。")
    }
    
    return sb.String()
}
```

#### 3.2.5 `/rules` 命令

**改动文件：** `cmd/interactive.go`

新增 `/rules` 斜杠命令，显示当前加载的项目规则：

```
> /rules
已加载的规则:
  ✓ ~/.codecast/rules.md (234 bytes)
  ✓ .codecast/rules.md (1.2 KB)
  ✓ .codecast/rules/testing.md (456 bytes)

总大小: 1.9 KB | 来源: 3 个文件
───────────────────────────────────
[显示规则内容]
```

#### 3.2.6 自动创建模板

当用户首次在项目目录中使用 codecastcli 时，提供创建规则文件模板的功能：

```bash
$ codecast init
✓ 已创建 .codecast/rules.md 模板文件
  请根据项目需求编辑此文件。
```

**新增文件：** `cmd/init.go`

`init` 子命令创建一个带注释说明的 `rules.md` 模板。

### 3.3 文件变更清单

| 操作 | 文件路径 | 说明 |
|------|----------|------|
| 新增 | `internal/rules/loader.go` | 规则文件加载器（多层级加载、合并、大小限制） |
| 新增 | `internal/rules/template.go` | 模板变量替换（`{{.ProjectName}}` 等） |
| 新增 | `internal/rules/loader_test.go` | 加载器单元测试 |
| 新增 | `cmd/init.go` | `codecast init` 子命令（创建模板） |
| 修改 | `internal/agent/agent.go` | 系统提示词改为动态组装，注入项目规则 |
| 修改 | `cmd/interactive.go` | 新增 `/rules` 命令 |
| 修改 | `cmd/root.go` | 新增 `--project-root` flag |
| 修改 | `internal/config/config.go` | 新增 `ProjectRoot` 配置字段 |

### 3.4 实施步骤

| 步骤 | 任务 | 预估 |
|------|------|------|
| 1 | 实现 `rules.Loader`：多层级加载 + 合并 + 大小限制 + 模板变量 | 2h |
| 2 | 改造 `agent.go`：系统提示词动态组装 + 注入项目规则 | 1.5h |
| 3 | 实现 `codecast init` 子命令 + 模板文件 | 1h |
| 4 | 实现 `/rules` 斜杠命令 | 0.5h |
| 5 | 改造 `cmd/root.go` + `config.go`：新增 flag 和配置 | 0.5h |
| 6 | 编写测试（加载器 + 模板替换 + 合并策略） | 1h |

**小计：6.5 小时**

### 3.5 风险评估

| 风险 | 等级 | 缓解方案 |
|------|------|----------|
| 规则文件过大导致 Token 浪费 | 中 | 16KB 硬限制 + 加载时警告提示 |
| 规则文件语法错误（如无效模板变量） | 低 | 模板替换失败时保留原始文本，不报错 |
| 多层级规则冲突 | 低 | 按优先级顺序拼接，LLM 自行理解优先级 |
| 频繁 IO 读取规则文件影响性能 | 低 | 规则文件在 Agent 创建时一次性加载，不重复读取 |

---

## 模块 4: Headless 模式（CI/CD 集成）

### 4.1 现状问题

当前 codecastcli 只有交互式入口（`codecast` 默认进入 REPL），无法在 CI/CD 流水线、脚本自动化、定时任务等非交互场景中使用。

竞品方案：

| 工具 | Headless 入口 | 输出格式 |
|------|---------------|----------|
| Claude Code | `claude -p "prompt"` | 纯文本 / `--output-format json` |
| Codex CLI | `codex exec "prompt"` | 纯文本 / `--json` |
| OpenCode | `opencode --prompt "xxx" --non-interactive` | 纯文本 |

### 4.2 技术方案

#### 4.2.1 `exec` 子命令

**新增文件：** `cmd/exec.go`

```bash
# 基本用法
codecast exec "请分析这个项目的架构并生成文档"

# JSON 输出
codecast exec "列出所有 TODO 注释" --output-format json

# 管道输入
cat error.log | codecast exec "分析这个错误日志" --input-stdin

# 指定模型和 Provider
codecast exec "生成单元测试" --model gpt-4o --provider openai

# 限制最大轮次（防止无限循环）
codecast exec "重构这个函数" --max-turns 10

# 指定工作目录
codecast exec "分析项目结构" --work-dir /path/to/project

# 超时控制
codecast exec "完整代码审查" --timeout 5m
```

**参数设计：**

| Flag | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--output-format` / `-f` | string | `text` | 输出格式：`text` / `json` / `stream-json` |
| `--input-stdin` | bool | `false` | 从 stdin 读取输入（管道模式） |
| `--max-turns` | int | `20` | 最大 ReAct 轮次 |
| `--work-dir` | string | `cwd` | 工作目录 |
| `--timeout` | duration | `10m` | 总超时时间 |
| `--no-tools` | bool | `false` | 禁用所有工具（纯对话模式） |
| `--tools` | []string | `all` | 指定允许的工具列表 |
| `--quiet` / `-q` | bool | `false` | 静默模式，仅输出结果 |

#### 4.2.2 输出格式规范

**text 格式（默认）：** 仅输出 Agent 最终回复的纯文本内容，适合管道处理。

```bash
$ codecast exec "用一句话描述这个项目的用途"
codecastcli 是一个基于 AgentPrimordia 框架构建的 AI 编程助手命令行工具。
```

**json 格式：** 结构化 JSON 输出，适合程序化处理。

```json
{
  "session_id": "abc-123",
  "status": "completed",
  "result": "codecastcli 是一个基于...",
  "usage": {
    "prompt_tokens": 1523,
    "completion_tokens": 45,
    "total_tokens": 1568
  },
  "cost": {
    "usd": 0.0078,
    "cny": 0.056
  },
  "tools_used": ["read_file", "list_dir"],
  "turns": 3,
  "duration_ms": 4523
}
```

**stream-json 格式：** 逐事件流式输出 JSON（NDJSON 协议），适合实时监听。

```jsonl
{"type":"token","content":"code"}
{"type":"token","content":"cast"}
{"type":"tool_call","tool":"read_file","args":{"path":"main.go"}}
{"type":"tool_result","tool":"read_file","content":"..."}
{"type":"complete","result":"...","usage":{...}}
```

#### 4.2.3 核心实现

**`cmd/exec.go` 的核心流程：**

```go
func runExec(cmd *cobra.Command, args []string) error {
    // 1. 解析输入
    prompt := args[0]
    if inputStdin {
        stdinBytes, _ := io.ReadAll(os.Stdin)
        prompt = string(stdinBytes) + "\n\n" + prompt
    }

    // 2. 创建 Agent（复用现有逻辑，但使用 full-auto 权限模式）
    cfg := loadConfig()
    cfg.PermissionMode = "full-auto" // Headless 模式默认全自动
    agent, err := agentPkg.New(cfg)
    defer agent.Close()

    // 3. 设置超时
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    // 4. 执行
    switch outputFormat {
    case "text":
        return execText(ctx, agent, prompt)
    case "json":
        return execJSON(ctx, agent, prompt)
    case "stream-json":
        return execStreamJSON(ctx, agent, prompt)
    }
}
```

**退出码规范：**

| 退出码 | 含义 |
|--------|------|
| `0` | 成功完成 |
| `1` | Agent 执行错误（LLM 报错、工具失败等） |
| `2` | 超时 |
| `3` | 配置错误（API Key 缺失、模型不存在等） |
| `130` | 用户中断（SIGINT） |

#### 4.2.4 信号处理与优雅退出

```go
// 监听 SIGINT / SIGTERM
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

go func() {
    sig := <-sigCh
    fmt.Fprintf(os.Stderr, "\n收到信号 %s，正在优雅退出...\n", sig)
    cancel() // 取消 context
    agent.Close()
    os.Exit(130)
}()
```

#### 4.2.5 管道集成示例

```bash
# Git 提交信息生成
git diff --cached | codecast exec "为这些变更生成一条简洁的 git commit message" 

# 代码审查（CI 中使用）
codecast exec "审查最近一次提交的代码变更，关注安全和性能问题" -f json | jq '.result'

# 批量文件处理
find . -name "*.go" | codecast exec --input-stdin "为这些文件添加缺失的单元测试"

# 文档生成
codecast exec "为 internal/tools/ 目录下的所有工具生成 API 文档" > docs/tools.md
```

#### 4.2.6 与权限管理的交互

Headless 模式默认使用 `full-auto` 权限模式（所有工具自动放行），这是 CI/CD 场景的合理选择。但用户可以通过以下方式增强安全性：

1. `--permission-mode suggest` — 虽然不常见，但允许远程确认
2. `--tools read_file,grep_search,glob_search` — 限制允许的工具列表
3. `--scope ./src` — 限制文件访问范围
4. 配合 Docker / 沙箱使用

### 4.3 文件变更清单

| 操作 | 文件路径 | 说明 |
|------|----------|------|
| 新增 | `cmd/exec.go` | `exec` 子命令实现（三种输出格式 + 信号处理） |
| 新增 | `internal/output/formatter.go` | 输出格式化器（text / json / stream-json） |
| 新增 | `cmd/exec_test.go` | exec 命令测试 |
| 修改 | `cmd/root.go` | 注册 `exec` 子命令 |
| 修改 | `internal/agent/agent.go` | `Process` 方法支持返回结构化结果（而非仅打印） |

### 4.4 实施步骤

| 步骤 | 任务 | 预估 |
|------|------|------|
| 1 | 实现 `output.Formatter`：三种输出格式的序列化 | 1.5h |
| 2 | 实现 `cmd/exec.go`：核心流程 + 参数解析 + stdin 支持 | 2h |
| 3 | 实现 stream-json 格式：适配 StreamEvent → NDJSON | 1h |
| 4 | 实现信号处理 + 优雅退出 + 退出码规范 | 0.5h |
| 5 | 改造 `agent.go`：`Process` 返回结构化结果 | 1h |
| 6 | 编写测试 + CI 集成示例 | 1h |

**小计：7 小时**

### 4.5 风险评估

| 风险 | 等级 | 缓解方案 |
|------|------|----------|
| Headless 模式下工具无限循环消耗 Token | 高 | `--max-turns` 硬限制 + `--timeout` 超时兜底 |
| stream-json 与终端不兼容 | 低 | stream-json 仅在显式指定时启用，默认使用 text |
| CI 环境中无 TTY 导致 go-prompt 崩溃 | 中 | `exec` 命令完全不初始化 go-prompt，使用纯 Agent 调用 |
| 大文件通过 stdin 传入导致内存溢出 | 低 | stdin 读取限制 10MB，超出截断并警告 |

---

## 总览与里程碑

| 里程碑 | 模块 | 预估工时 | 累计 | 交付物 |
|--------|------|----------|------|--------|
| M1 | 权限管理 | 9h | 9h | 三级审批模式 + HITL 集成 + SafeMode 修复 |
| M2 | 上下文压缩 | 7.5h | 16.5h | 双引擎压缩 + Token 预算 + /compact 命令 |
| M3 | 项目记忆 | 6.5h | 23h | 多层级规则加载 + 动态提示词 + codecast init |
| M4 | Headless 模式 | 7h | 30h | exec 子命令 + 三种输出格式 + CI 集成 |

**建议开发节奏：**

每个模块完成后进行一次集成测试，确认不引入回归后再进入下一模块。全部完成后进行端到端的综合测试，包括：

1. 长会话测试：50 轮对话后验证压缩是否正常工作
2. 权限测试：三级模式下各类工具的放行/拦截行为
3. CI 测试：在 GitHub Actions 中使用 `codecast exec` 执行自动化任务
4. 项目记忆测试：在不同项目目录下验证规则加载和切换

---

## 后续展望（Phase 3 候选）

本阶段完成后，codecastcli 将具备与 Claude Code / Codex CLI 同台竞技的基础能力。后续可考虑的进阶方向包括：

1. **Sub-Agent 架构**：利用 AP 的 DAG 工作流引擎实现 Plan-Agent + Execute-Agent 双 Agent 协作
2. **Hooks 系统**：利用 AP 的 `WithHooks` 注入 Pre/Post 工具钩子，支持自定义脚本扩展
3. **多模态支持**：利用 AP 的 `ContentPart`（图片/音频/视频）实现截图分析、语音交互
4. **Plugin 生态**：利用 AP 的 `ToolPlugin` + `PluginLoader` 建立社区插件市场
5. **分布式 Agent Pool**：利用 AP 的 Pool 调度实现多 Agent 并行处理大型任务
