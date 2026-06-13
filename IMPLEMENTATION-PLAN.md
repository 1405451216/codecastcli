# Codecast CLI 追赶计划 — 四大核心模块实施方案

> 版本: v1.0 | 日期: 2026-06-12
> 目标: 在现有 Go 版本基础上，补齐与世界顶级 CLI 工具（Claude Code / Codex CLI / Qoder CLI / Trae CLI / OpenCode CLI）竞争所需的核心能力

---

## 总体策略

本计划聚焦四个投入产出比最高的模块，按依赖关系排序实施。每个模块内部按"最小可用 → 完善增强"的顺序递进，确保任何阶段中断都能交付可用产品。

**实施顺序与依赖关系：**

```
模块 1: TUI 体验升级  ──┐
                        ├──▶ 模块 4: 会话恢复 Resume
模块 2: 结构化编辑工具  ──┘
模块 3: MCP 配置持久化  ──── (独立)
```

模块 1 和 2 可并行开发，模块 3 完全独立，模块 4 依赖模块 1 的交互式 REPL 基础设施。

**新增依赖：**

| 依赖包 | 用途 | 模块 |
|--------|------|------|
| `github.com/c-bata/go-prompt` | 行编辑 / 命令历史 / 自动补全 | 模块 1 |
| `github.com/charmbracelet/glamour` | Markdown / 代码块终端渲染 | 模块 1 |
| `github.com/schollz/progressbar/v3` | Spinner / 进度条 | 模块 1 |

无需修改 `AgentPrimordia` 框架代码。所有新增工具和扩展均在 `codecastcli` 项目内部完成。

---

## 模块 1: TUI 体验升级

### 1.1 现状问题

当前 `cmd/interactive.go` 使用 `bufio.NewReader(os.Stdin)` 读取输入，`internal/ui/ui.go` 仅 89 行代码，基本是 `fmt.Println` + `fatih/color` 的简单封装。具体缺失：

- 无行编辑（方向键移动光标、Ctrl+A/E 首尾跳转、Ctrl+W 删除词、Backspace 等）
- 无命令历史（上下方向键翻阅历史）
- 无自动补全（`/` 命令补全）
- 无 Markdown 渲染（AI 返回的 Markdown 原文输出，代码块无高亮）
- 无加载指示（Agent 处理时无任何视觉反馈）
- 无流式输出的视觉区分（Token、工具调用、错误混在纯文本中）

### 1.2 技术方案

#### 1.2.1 交互式 REPL — go-prompt 替换 bufio

**改动文件：** `cmd/interactive.go`

用 `github.com/c-bata/go-prompt` 替换当前的 `bufio.NewReader` + `fmt.Print("❯ ")` 循环。go-prompt 提供开箱即用的行编辑、历史、补全能力。

**核心改动点：**

1. 将 REPL 主循环从 `bufio.ReadString` 改为 `prompt.New(executor, completer)` 模式
2. `executor(input string)` 回调函数承接原有的命令分发逻辑（`handleSpecialCommand` / `StreamProcess`）
3. `completer(d prompt.Document) []prompt.Suggest` 提供 `/` 斜杠命令的自动补全
4. 配置 prompt 选项：前缀 `❯ `、历史文件路径 `~/.codecast/history`、最大历史条数 1000

**补全列表：**

```
/help, /h          显示帮助
/quit, /q          退出
/clear             清除上下文
/tools             查看工具
/models            查看模型
/stats             查看统计
/sessions          查看会话
/export            导出会话
/mode              切换模式
/resume            恢复会话（模块 4 新增）
```

#### 1.2.2 Markdown 渲染 — glamour 集成

**改动文件：** `internal/ui/ui.go`，新增 `internal/ui/markdown.go`

用 `github.com/charmbracelet/glamour` 渲染 AI 响应的 Markdown 内容。glamour 支持终端语法高亮、表格、列表等，并可通过自定义样式表调整。

**核心改动点：**

1. 新增 `RenderMarkdown(content string) string` 函数，调用 glamour 渲染
2. 修改 `PrintAssistant` 函数，将 `fmt.Println` 替换为 glamour 渲染输出
3. 流式输出场景下，先累积完整响应，完成后一次性渲染（避免逐 Token 渲染的碎片化问题）

**注意事项：** glamour 渲染需要完整文本，流式输出期间用 spinner + 原始 Token 显示，完成后切换为渲染版本。

#### 1.2.3 Spinner 加载指示

**改动文件：** `cmd/interactive.go`，`internal/agent/stream.go`

在 Agent 开始处理时显示 spinner，处理完成后隐藏。

**方案选择：**

- 轻量方案：使用 Unicode spinner 字符（⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏）+ goroutine 定时刷新，无额外依赖
- 重量方案：使用 `schollz/progressbar` 的 spinner 组件

推荐轻量方案，减少依赖。

**核心改动点：**

1. 新增 `internal/ui/spinner.go`，实现 `StartSpinner(message string)` / `StopSpinner()` 
2. 在 `StreamProcess` 调用前启动 spinner，收到第一个 `StreamEventToken` 时停止
3. 收到 `StreamEventToolCall` 时短暂显示工具名称，然后继续 spinner

#### 1.2.4 流式输出视觉优化

**改动文件：** `internal/agent/stream.go`

当前流式输出的各类事件（Token/ToolCall/ToolResult/Complete/Error）都用纯文本 `fmt.Printf` 输出，视觉层次不清。

**改进方案：**

1. `StreamEventToken` → 正常文本输出（白色）
2. `StreamEventToolCall` → 用 `color.Yellow` 包裹，前缀 `⚙ ` 图标，例如 `⚙ 调用工具: read_file(path="main.go")`
3. `StreamEventToolResult` → 用 `color.Green` 包裹，前缀 `✓ `，结果内容折叠（超过 5 行只显示首尾各 2 行 + `... (共 N 行)`）
4. `StreamEventComplete` → 底部显示灰色统计行：`[完成] 轮数=3, 工具调用=2, Token=1,234`
5. `StreamEventError` → 用 `color.Red` 包裹，前缀 `✗ `

### 1.3 涉及文件清单

| 文件 | 改动类型 | 说明 |
|------|----------|------|
| `cmd/interactive.go` | 重写 | REPL 主循环改用 go-prompt |
| `internal/ui/ui.go` | 修改 | PrintAssistant 接入 glamour |
| `internal/ui/markdown.go` | 新增 | Markdown 渲染封装 |
| `internal/ui/spinner.go` | 新增 | Spinner 加载指示 |
| `internal/agent/stream.go` | 修改 | 流式输出视觉优化 |
| `go.mod` / `go.sum` | 修改 | 新增 go-prompt、glamour 依赖 |

### 1.4 分步实施计划

| 步骤 | 内容 | 预估工时 | 交付物 |
|------|------|----------|--------|
| 1.1 | 集成 go-prompt 替换 bufio，实现行编辑 + 历史 + `/` 命令补全 | 2h | 可用的增强 REPL |
| 1.2 | 实现 spinner 加载指示器 | 1h | 工具调用时有视觉反馈 |
| 1.3 | 集成 glamour Markdown 渲染 | 1.5h | AI 响应带语法高亮 |
| 1.4 | 优化流式输出的事件视觉层次 | 1h | 各类事件有清晰区分 |

**总计预估：5.5 小时**

---

## 模块 2: 结构化文件编辑工具

### 2.1 现状问题

当前 Agent 仅有 3 个工具（read_file、write_file、bash），缺少结构化编辑能力。LLM 修改文件时必须用 `write_file` 重写整个文件，导致：

- Token 浪费严重（修改 1 行代码需要输出整个文件内容）
- 大文件重写容易出错（LLM 经常遗漏或篡改未修改的部分）
- 无法实现精准的"查找替换"式编辑

### 2.2 技术方案

#### 2.2.1 新增 `edit_file` 工具

在 `codecastcli` 项目内新增一个自定义工具，实现 Tool 接口（`Name()` / `Description()` / `Parameters()` / `Execute()`），然后在 `internal/agent/agent.go` 中注册到 ToolRegistry。

**工具定义：**

```
名称: edit_file
描述: 通过精确字符串替换编辑文件。指定要被替换的旧文本和新文本，工具会在文件中查找并替换。
参数:
  - file_path (string, required): 要编辑的文件路径
  - old_string (string, required): 要被替换的原始文本（必须在文件中唯一匹配）
  - new_string (string, required): 替换后的新文本
  - replace_all (bool, optional, default false): 是否替换所有匹配项
```

**核心逻辑：**

1. 读取目标文件完整内容
2. 搜索 `old_string` 的出现次数
3. 如果出现 0 次 → 返回错误 "未找到匹配的文本"
4. 如果出现 1 次 → 执行替换
5. 如果出现多次且 `replace_all = true` → 全部替换
6. 如果出现多次且 `replace_all = false` → 返回错误，提示使用 `replace_all` 或提供更多上下文使匹配唯一
7. 将修改后的内容写回文件
8. 返回 diff 摘要（显示变更的行号和上下文）

**注册方式：**

在 `internal/agent/agent.go` 的 `New()` 函数中，在 `DefaultToolkit` 之后追加注册：

```go
editTool := tools.NewEditFileTool()
registry.Register(editTool)
```

#### 2.2.2 新增 `grep_search` 工具

对标 Claude Code 的 Grep 和 OpenCode 的 grep 工具，实现代码内容搜索。

**工具定义：**

```
名称: grep_search
描述: 在工作目录中搜索文件内容，支持正则表达式。
参数:
  - pattern (string, required): 搜索模式（正则表达式）
  - path (string, optional, default "."): 搜索的根目录
  - file_pattern (string, optional): 文件名过滤（如 "*.go"）
  - max_results (int, optional, default 50): 最大结果数
  - case_insensitive (bool, optional, default false): 忽略大小写
```

**核心逻辑：**

1. 遍历目标目录（跳过 .git、node_modules、vendor 等）
2. 按 file_pattern 过滤文件
3. 逐文件读取并用正则匹配
4. 返回匹配结果（文件名、行号、行内容、匹配上下文前后各 1 行）

#### 2.2.3 新增 `glob_search` 工具

对标 Claude Code 的 Glob 和 OpenCode 的 glob 工具，实现文件名模式搜索。

**工具定义：**

```
名称: glob_search
描述: 按文件名模式搜索文件。
参数:
  - pattern (string, required): glob 模式（如 "**/*.go", "src/**/*.ts"）
  - path (string, optional, default "."): 搜索的根目录
```

#### 2.2.4 更新系统提示词

在 `internal/agent/agent.go` 的系统提示词中添加新工具的说明和使用指引：

```
你可以使用以下工具：
- read_file: 读取文件内容
- write_file: 创建或覆盖整个文件
- edit_file: 通过精确字符串替换编辑文件（推荐！修改现有文件时优先使用）
- grep_search: 搜索文件内容（支持正则）
- glob_search: 按文件名模式搜索文件
- bash: 执行终端命令
- web: HTTP 请求

编辑文件时的最佳实践：
1. 优先使用 edit_file 而非 write_file，仅输出变更部分
2. old_string 必须在文件中唯一匹配，如不唯一请提供更多上下文
3. 编辑前先用 read_file 读取文件确认当前内容
```

### 2.3 涉及文件清单

| 文件 | 改动类型 | 说明 |
|------|----------|------|
| `internal/tools/edit.go` | 新增 | edit_file 工具实现 |
| `internal/tools/grep.go` | 新增 | grep_search 工具实现 |
| `internal/tools/glob.go` | 新增 | glob_search 工具实现 |
| `internal/agent/agent.go` | 修改 | 注册新工具到 Registry、更新系统提示词 |
| `internal/tools/edit_test.go` | 新增 | edit_file 单元测试 |

### 2.4 分步实施计划

| 步骤 | 内容 | 预估工时 | 交付物 |
|------|------|----------|--------|
| 2.1 | 实现 edit_file 工具（字符串替换编辑） | 2h | 可用的精准编辑 |
| 2.2 | 实现 grep_search 工具（内容搜索） | 1.5h | 代码搜索能力 |
| 2.3 | 实现 glob_search 工具（文件名搜索） | 1h | 文件查找能力 |
| 2.4 | 注册新工具 + 更新系统提示词 + 编写测试 | 1.5h | 集成验证 |

**总计预估：6 小时**

---

## 模块 3: MCP 配置持久化

### 3.1 现状问题

`cmd/mcp.go` 中的 `loadMCPConfig()` 和 `saveMCPConfig()` 函数体均为 `// TODO`，实际未做任何持久化。这意味着：

- 用户通过 `codecast mcp add` 注册的 MCP 服务器不会被保存
- 退出程序后所有 MCP 配置丢失
- 交互式对话中无法调用 MCP 工具

### 3.2 技术方案

#### 3.2.1 配置文件持久化

**改动文件：** `cmd/mcp.go`

将 MCP 配置保存到 `~/.codecast/mcp_servers.yaml`。

**核心改动点：**

1. `loadMCPConfig()` → 从 `~/.codecast/mcp_servers.yaml` 读取 YAML，文件不存在则返回空配置
2. `saveMCPConfig(cfg *mcpConfig)` → 将配置序列化为 YAML 写入文件
3. 添加文件锁机制（可选，防止并发写入冲突）

**配置文件格式：**

```yaml
servers:
  my-server:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
    base_url: ""
    auto_start: true
  remote-api:
    command: ""
    args: []
    base_url: "http://localhost:8080"
    auto_start: false
```

#### 3.2.2 交互式会话中的 MCP 工具集成

**改动文件：** `internal/agent/agent.go`

在 Agent 初始化时，加载已注册的 MCP 服务器配置，连接并注册其提供的工具到 ToolRegistry。

**核心改动点：**

1. 新增 `internal/agent/mcp_integration.go`，实现 `ConnectMCPServers(registry *ap.ToolRegistry) error`
2. 在 `agent.New()` 中调用该函数，将 MCP 工具注册到 Registry
3. 支持 stdio 和 HTTP 两种传输方式（利用 AP 框架已有的 `ap.NewMCPClient`）
4. 连接失败的 MCP 服务器记录警告日志，不阻塞 Agent 启动

#### 3.2.3 MCP 连接测试命令增强

**改动文件：** `cmd/mcp.go`

完善 `mcp test` 命令，支持 stdio 类型服务器的连接测试。

**核心改动点：**

1. 当 MCP 服务器配置了 `command` 时，启动子进程运行命令并通过 stdio 通信
2. 超时机制：10 秒内未完成初始化则判定失败
3. 列出发现的工具名称和描述

### 3.3 涉及文件清单

| 文件 | 改动类型 | 说明 |
|------|----------|------|
| `cmd/mcp.go` | 修改 | 实现 load/save 持久化 |
| `internal/agent/mcp_integration.go` | 新增 | Agent 启动时连接 MCP 服务器 |
| `internal/agent/agent.go` | 修改 | New() 中调用 MCP 集成 |

### 3.4 分步实施计划

| 步骤 | 内容 | 预估工时 | 交付物 |
|------|------|----------|--------|
| 3.1 | 实现 loadMCPConfig / saveMCPConfig 的 YAML 持久化 | 1h | MCP 配置可持久保存 |
| 3.2 | 实现 Agent 启动时自动连接 MCP 服务器并注册工具 | 2h | MCP 工具在对话中可用 |
| 3.3 | 增强 mcp test 命令（支持 stdio 传输） | 1h | 完善的连接测试 |

**总计预估：4 小时**

---

## 模块 4: 会话恢复（Resume）

### 4.1 现状问题

当前每次启动 `codecast` 都会创建一个全新的会话。如果用户中断了对话（关闭终端、网络断开、Ctrl+C），之前积累的上下文全部丢失，需要重新向 AI 解释项目背景和需求。

对比竞品：
- Claude Code: `--continue` 恢复最近会话，`--resume <id>` 恢复指定会话
- Codex CLI: `codex resume` 恢复会话，`/fork` 分支
- OpenCode: `-s <session>` 指定会话继续

### 4.2 技术方案

#### 4.2.1 新增 CLI Flags

**改动文件：** `cmd/root.go`

在根命令上添加两个新 Flag：

```
--continue, -c        继续最近一次会话
--resume <id>, -r     恢复指定会话 ID
```

#### 4.2.2 会话恢复逻辑

**改动文件：** `internal/agent/agent.go`

在 `New()` 函数中增加会话恢复路径：

1. 如果指定了 `--continue`：查询 SessionManager 获取最近更新的 session_id
2. 如果指定了 `--resume <id>`：直接使用指定的 session_id
3. 使用 `ap.SessWithID(sessionID)` 选项创建 Session，AP 框架会自动从 Memory（SQLite）中加载该 session_id 的历史消息
4. 在 REPL 启动时显示恢复信息：`✓ 已恢复会话: <session_id> (N 条历史消息)`

**关键依赖确认：** AP 框架的 `ap.NewSession(agent, memory, ap.SessWithID(id))` 已支持指定 session_id 创建会话，底层 SQLite Memory 存储会按 session_id 关联 episodes。这意味着历史消息会自动加载到上下文中。

#### 4.2.3 交互式恢复提示

**改动文件：** `cmd/interactive.go`

当使用 `--continue` 或 `--resume` 启动时：

1. 显示恢复的会话信息（ID、消息数、最后更新时间）
2. 显示最近 3 条消息摘要（角色 + 前 50 字符）
3. 用户可以直接输入继续对话，或使用 `/clear` 重新开始

#### 4.2.4 自动保存当前会话 ID

**改动文件：** `cmd/interactive.go`

每次启动交互式会话时，将当前 session_id 写入 `~/.codecast/last_session` 文件，供 `--continue` 读取。

**实现：**

1. 在 `agent.New()` 返回后，获取当前 session 的 ID
2. 写入 `~/.codecast/last_session` 文件（纯文本，仅一行 session_id）
3. `--continue` 时读取该文件得到 session_id

### 4.3 涉及文件清单

| 文件 | 改动类型 | 说明 |
|------|----------|------|
| `cmd/root.go` | 修改 | 添加 --continue / --resume flags |
| `cmd/interactive.go` | 修改 | 传递恢复参数、显示恢复信息、保存 last_session |
| `internal/agent/agent.go` | 修改 | New() 支持 session 恢复、新增 NewWithSession() |

### 4.4 分步实施计划

| 步骤 | 内容 | 预估工时 | 交付物 |
|------|------|----------|--------|
| 4.1 | 添加 --continue / --resume flags，实现 last_session 文件读写 | 1h | CLI 参数可用 |
| 4.2 | 实现 Agent 会话恢复逻辑（SessWithID + 历史加载） | 1.5h | 会话可恢复 |
| 4.3 | 交互式恢复提示（显示历史信息、最近消息摘要） | 1h | 用户友好的恢复体验 |

**总计预估：3.5 小时**

---

## 总工时与里程碑

| 模块 | 预估工时 | 优先级 | 可独立交付 |
|------|----------|--------|------------|
| 模块 1: TUI 体验升级 | 5.5h | P0 | 是 |
| 模块 2: 结构化编辑工具 | 6h | P0 | 是 |
| 模块 3: MCP 配置持久化 | 4h | P1 | 是 |
| 模块 4: 会话恢复 | 3.5h | P1 | 是（依赖模块 1 的基础设施） |
| **总计** | **19h** | | |

**推荐实施节奏：**

- 第一阶段（Day 1）: 模块 1 的步骤 1.1 + 1.2 → 立刻获得可用的增强 REPL
- 第二阶段（Day 2）: 模块 2 全部 → 补齐编辑和搜索能力
- 第三阶段（Day 3）: 模块 1 的步骤 1.3 + 1.4 + 模块 3 → 完善体验 + MCP
- 第四阶段（Day 4）: 模块 4 全部 → 会话恢复

---

## 风险与缓解

### 风险 1: go-prompt 在 Windows 上的兼容性

go-prompt 依赖 VT100 终端转义序列，Windows 旧版 cmd.exe 可能不支持。

**缓解：** 
- Windows 10+ 的 Windows Terminal 和 ConPTY 已原生支持 VT100
- go-prompt 内置 Windows 适配层
- 添加 fallback：如果 go-prompt 初始化失败，回退到当前的 bufio 方案

### 风险 2: glamour 渲染对 CJK 字符的宽度计算

glamour 依赖 `go-runewidth` 计算字符宽度，中文等宽字符偶有计算偏差。

**缓解：**
- 测试常见中文场景，必要时调整 glamour 的 WordWrap 参数
- glamour 渲染失败时 fallback 到原始文本输出

### 风险 3: edit_file 的 old_string 匹配不唯一

LLM 可能生成不够精确的 old_string，导致多处匹配。

**缓解：**
- 工具层面：多次匹配时返回错误并提示提供更多上下文
- 系统提示词层面：明确指引 LLM 使用足够长的 old_string（建议 3 行以上）
- 可选增强：返回每个匹配位置的前后上下文，帮助 LLM 调整

### 风险 4: MCP stdio 传输的子进程管理

stdio 模式的 MCP 服务器需要启动子进程并保持长连接，进程管理的健壮性需要验证。

**缓解：**
- 利用 AP 框架已有的 `ap.NewMCPClient` 实现，不重新造轮子
- 添加超时和健康检查机制
- 连接失败不阻塞主 Agent 启动

---

## 后续演进（不在本次范围内）

本次四个模块完成后，下一步可考虑的方向（按优先级排列）：

1. **上下文窗口管理** — 实现对话历史自动压缩/摘要（对标 Claude Code 的六层压缩系统）
2. **项目级记忆** — 实现 `.codecast/rules.md` 项目指令文件，Agent 自动加载项目约定
3. **基础权限控制** — 命令审批机制（Ask/Auto/FullAuto 三档模式）
4. **Headless 模式** — `codecast exec "prompt"` 非交互执行 + JSON 结构化输出
5. **LSP 集成** — 编译错误和类型诊断工具
6. **子 Agent 体系** — 独立上下文的 Explore/Plan 子 Agent
