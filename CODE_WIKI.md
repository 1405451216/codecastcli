# Codecast CLI - Code Wiki

> AI Agent 终端 CLI 工具，基于 AgentPrimordia 框架构建
> 版本: v0.1.0 | Go 1.22+

---

## 目录

1. [项目概述](#1-项目概述)
2. [整体架构](#2-整体架构)
3. [模块职责](#3-模块职责)
4. [关键类与函数](#4-关键类与函数)
5. [数据流与交互](#5-数据流与交互)
6. [依赖关系](#6-依赖关系)
7. [项目运行方式](#7-项目运行方式)
8. [配置说明](#8-配置说明)
9. [测试](#9-测试)

---

## 1. 项目概述

Codecast CLI 是一个 AI 驱动的 Agent 终端工具，类似于 Claude Code、Gemini CLI。它允许用户通过命令行与大型语言模型（LLM）进行交互式对话，并支持文件操作、代码执行、知识库检索等高级功能。

### 核心特性

- **交互式对话**: 多轮对话，上下文记忆
- **单轮对话**: 快速单次查询 (`codecast chat`)
- **流式输出**: 实时打字机效果
- **工具调用**: 文件系统、Shell、Web 请求
- **RAG 知识库**: 文档索引与检索增强生成
- **多 Agent 编排**: Pipeline / Parallel / Handoff 模式
- **MCP 协议**: 外部工具服务器扩展
- **插件系统**: 动态加载 Go 插件
- **成本追踪**: Token 消耗与费用记录
- **会话管理**: 历史会话查看与导出

---

## 2. 整体架构

### 2.1 分层架构

```
┌─────────────────────────────────────────────────────────────┐
│                        命令层 (cmd)                          │
│  ┌─────────┬─────────┬─────────┬─────────┬─────────────────┐│
│  │  chat   │ config  │  cost   │ session │   interactive   ││
│  ├─────────┼─────────┼─────────┼─────────┼─────────────────┤│
│  │   mcp   │   rag   │ plugin  │ workflow│     root        ││
│  └─────────┴─────────┴─────────┴─────────┴─────────────────┘│
├─────────────────────────────────────────────────────────────┤
│                      内部模块 (internal)                      │
│  ┌─────────┬─────────┬─────────┬─────────┬─────────────────┐│
│  │  agent  │ config  │  cost   │  logx   │    provider     ││
│  ├─────────┼─────────┼─────────┼─────────┼─────────────────┤│
│  │ session │   ui    │         │         │                 ││
│  └─────────┴─────────┴─────────┴─────────┴─────────────────┘│
├─────────────────────────────────────────────────────────────┤
│                    外部框架 (AgentPrimordia)                  │
│  ┌─────────┬─────────┬─────────┬─────────┬─────────────────┐│
│  │  Agent  │  LLM    │ Memory  │  Tools  │   RAG / MCP     ││
│  └─────────┴─────────┴─────────┴─────────┴─────────────────┘│
├─────────────────────────────────────────────────────────────┤
│                      第三方依赖                              │
│  Cobra + Viper │ fatih/color │ modernc.org/sqlite │ slog    │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 目录结构

```
codecastcli/
├── main.go                      # 程序入口
├── go.mod                       # Go 模块定义
│
├── cmd/                         # CLI 命令定义 (Cobra)
│   ├── root.go                  # 根命令、全局配置初始化
│   ├── chat.go                  # 单轮对话命令
│   ├── config.go                # 配置管理命令
│   ├── cost.go                  # 成本追踪命令
│   ├── interactive.go           # 交互式对话模式
│   ├── mcp.go                   # MCP Server 管理
│   ├── plugin.go                # 插件管理
│   ├── rag.go                   # RAG 知识库
│   ├── session.go               # 会话管理
│   └── workflow.go              # 多 Agent 工作流
│
├── internal/                    # 内部实现
│   ├── agent/                   # Agent 封装
│   │   ├── agent.go             # CodecastAgent 核心
│   │   └── stream.go            # 流式输出处理
│   ├── config/                  # 配置管理
│   │   ├── config.go            # Config 结构体
│   │   └── config_test.go       # 配置测试
│   ├── cost/                    # 成本追踪
│   │   ├── tracker.go           # Tracker 实现
│   │   └── tracker_test.go      # 成本测试
│   ├── logx/                    # 日志系统
│   │   ├── logx.go              # slog 封装
│   │   └── logx_test.go         # 日志测试
│   ├── provider/                # LLM Provider 工厂
│   │   └── factory.go           # CreateProvider
│   ├── session/                 # 会话管理
│   │   └── manager.go           # Session Manager
│   └── ui/                      # UI 输出
│       └── ui.go                # 终端输出辅助
│
└── CODE_WIKI.md                 # 本文档
```

---

## 3. 模块职责

### 3.1 cmd 包 - 命令层

| 文件 | 职责 | 对应 CLI 命令 |
|------|------|--------------|
| `root.go` | 根命令定义、全局 Flag、Viper 配置绑定、日志初始化 | `codecast` |
| `chat.go` | 单轮对话，无上下文记忆 | `codecast chat <消息>` |
| `interactive.go` | 交互式 REPL 模式，支持 `/` 特殊命令 | `codecast` (无参数) |
| `config.go` | 配置管理（v0.2.0+ 已迁移到交互模式 `/config`） | `codecast config` (deprecated) |
| `cost.go` | 成本汇总、每日统计、调用记录、清空（已迁移到 `/cost`） | `codecast cost` (deprecated) |
| `session.go` | 会话列表、查看历史、删除、导出 Markdown（已迁移到 `/session`） | `codecast session` (deprecated) |
| `mcp.go` | MCP Server 注册、移除、连接测试（已迁移到 `/mcp`） | `codecast mcp` (deprecated) |
| `rag.go` | 文档索引、知识库查询、基于 RAG 的对话（已迁移到 `/rag`） | `codecast rag` (deprecated) |
| `plugin.go` | 插件列表、加载、卸载、创建模板（已迁移到 `/plugin`） | `codecast plugin` (deprecated) |
| `workflow.go` | Pipeline / Parallel / Handoff 工作流（已迁移到 `/workflow`） | `codecast workflow` (deprecated) |

### 3.2 internal/agent - Agent 封装

封装 AgentPrimordia 的 `CapabilityAgent`，提供 CLI 友好的接口。

- **`CodecastAgent`**: 核心结构体，聚合 Agent、Memory、Session、CostTracker
- **`Process()`**: 同步处理用户输入，输出完整响应
- **`StreamProcess()`**: 流式处理，实时输出 token
- **`ClearContext()`**: 清除对话上下文
- **`GetStats()`**: 获取 Agent 运行统计

### 3.3 internal/config - 配置管理

- **`Config`**: API Key、模型、Provider、BaseURL
- **`Load()`**: 从 YAML 文件和环境变量加载
- **`Save()`**: 保存到 `~/.codecast/config.yaml`
- **`Validate()`**: 校验必填字段

### 3.4 internal/cost - 成本追踪

- **`Tracker`**: SQLite 持久化成本记录
- **`Record()`**: 记录单次 LLM 调用（自动计算 USD/CNY 费用）
- **`Summary()`**: 按模型汇总统计
- **`DailySummary()`**: 最近 N 天每日统计
- **`RecentRecords()`**: 最近 N 条调用记录

### 3.5 internal/logx - 日志系统

- 基于 Go 标准库 `log/slog`
- 支持 `debug/info/warn/error` 四级
- 支持 `text/json` 格式
- 默认输出到 `~/.codecast/codecast.log`
- 全局 Flag: `--log-level`, `--log-format`

### 3.6 internal/provider - Provider 工厂

- **`CreateProvider()`**: 根据配置创建对应 LLM Provider
- 支持: openai, anthropic, gemini, deepseek, qwen, glm, ollama 等

### 3.7 internal/session - 会话管理

- **`Manager`**: 查询 AP 框架的 SQLite 记忆数据库
- **`List()`**: 获取所有会话列表（按更新时间排序）
- **`GetHistory()`**: 获取指定会话的消息历史
- **`Delete()`**: 删除会话

### 3.8 internal/ui - UI 输出

- **`PrintBanner()`**: ASCII Logo
- **`PrintHelp()`**: 帮助信息
- **`PrintAssistant()`**: AI 响应输出（带语法高亮）
- **`PrintSuccess()` / `PrintError()`**: 状态提示

---

## 4. 关键类与函数

### 4.1 CodecastAgent (internal/agent/agent.go)

```go
type CodecastAgent struct {
    agent       *ap.CapabilityAgent    // AP 框架 Agent
    config      *config.Config         // 当前配置
    memory      ap.Memory              // SQLite 记忆存储
    registry    *ap.ToolRegistry       // 工具注册表
    session     *ap.Session            // 当前会话
    costTracker *cost.Tracker          // 成本追踪器
}
```

| 方法 | 签名 | 说明 |
|------|------|------|
| `New` | `func New(cfg *config.Config) (*CodecastAgent, error)` | 创建 Agent，初始化 Provider、Memory、Registry、Session |
| `Process` | `func (a *CodecastAgent) Process(ctx, input string) error` | 同步处理，调用后自动记录成本 |
| `StreamProcess` | `func (a *CodecastAgent) StreamProcess(ctx, input string) error` | 流式处理，实时输出，完成后记录成本 |
| `ClearContext` | `func (a *CodecastAgent) ClearContext()` | 清除当前会话上下文 |
| `GetStats` | `func (a *CodecastAgent) GetStats() Stats` | 返回运行统计 |
| `Close` | `func (a *CodecastAgent) Close() error` | 关闭资源（costTracker、memory） |

### 4.2 Config (internal/config/config.go)

```go
type Config struct {
    APIKey   string `yaml:"api_key"`
    Model    string `yaml:"model"`
    Provider string `yaml:"provider"`
    BaseURL  string `yaml:"base_url,omitempty"`
}
```

| 函数/方法 | 签名 | 说明 |
|-----------|------|------|
| `Default` | `func Default() *Config` | 返回默认配置（gpt-4o / openai） |
| `Load` | `func Load() *Config` | 从文件和环境变量加载 |
| `Save` | `func Save(cfg *Config) error` | 保存到 YAML 文件 |
| `Validate` | `func (c *Config) Validate() error` | 校验必填字段 |
| `GetConfigDir` | `func GetConfigDir() string` | 返回配置目录路径 |

### 4.3 Tracker (internal/cost/tracker.go)

```go
type Tracker struct {
    db *sql.DB
    mu sync.RWMutex
}
```

| 方法 | 签名 | 说明 |
|------|------|------|
| `NewTracker` | `func NewTracker() (*Tracker, error)` | 创建 Tracker，自动初始化 SQLite 表 |
| `Record` | `func (t *Tracker) Record(model, provider, sessionID, command string, usage ap.Usage) error` | 记录调用，自动计算费用 |
| `Summary` | `func (t *Tracker) Summary() (*Summary, error)` | 全局成本汇总 |
| `DailySummary` | `func (t *Tracker) DailySummary(days int) (map[string]*Daily, error)` | 按天汇总 |
| `RecentRecords` | `func (t *Tracker) RecentRecords(limit int) ([]Record, error)` | 最近记录 |
| `Clear` | `func (t *Tracker) Clear() error` | 清空所有记录 |

### 4.4 Manager (internal/session/manager.go)

```go
type Manager struct {
    db *sql.DB
}
```

| 方法 | 签名 | 说明 |
|------|------|------|
| `NewManager` | `func NewManager() (*Manager, error)` | 创建 Manager |
| `List` | `func (m *Manager) List() ([]Info, error)` | 查询所有会话 |
| `GetHistory` | `func (m *Manager) GetHistory(sessionID string, limit int) ([]Message, error)` | 获取会话历史 |
| `Delete` | `func (m *Manager) Delete(sessionID string) error` | 删除会话 |

### 4.5 日志函数 (internal/logx/logx.go)

| 函数 | 签名 | 说明 |
|------|------|------|
| `Init` | `func Init(opts ...Option)` | 初始化全局日志（单例） |
| `SetLevel` | `func SetLevel(l Level)` | 动态设置日志级别 |
| `Debug/Info/Warn/Error` | `func XXX(msg string, args ...any)` | 快捷日志输出 |

---

## 5. 数据流与交互

### 5.1 交互式对话数据流

```
用户输入
    │
    ▼
┌──────────────┐
│ interactive  │  解析 / 命令
│   .go        │
└──────┬───────┘
       │ 普通消息
       ▼
┌──────────────┐
│ CodecastAgent│
│   Process()  │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│   Session    │  Ask() -> 调用 LLM
│   .Ask()     │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│   Memory     │  保存到 SQLite
│   (SQLite)   │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│   Tracker    │  记录成本
│   .Record()  │
└──────────────┘
```

### 5.2 流式输出数据流

```
用户输入
    │
    ▼
┌──────────────┐
│ StreamProcess│
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ agent.Stream │  返回 channel
│    Run()     │
└──────┬───────┘
       │ StreamEventToken
       ▼
┌──────────────┐
│  fmt.Print   │  实时输出
└──────┬───────┘
       │ StreamEventComplete
       ▼
┌──────────────┐
│   Tracker    │  记录成本
└──────────────┘
```

---

## 6. 依赖关系

### 6.1 直接依赖 (go.mod)

| 依赖 | 版本 | 用途 |
|------|------|------|
| `agentprimordia` | v0.0.0 (replace) | Agent 框架核心 |
| `github.com/spf13/cobra` | v1.8.1 | CLI 命令框架 |
| `github.com/spf13/viper` | v1.19.0 | 配置管理 |
| `github.com/fatih/color` | v1.18.0 | 终端彩色输出 |
| `modernc.org/sqlite` | v1.34.1 | SQLite 数据库 (CGO-free) |
| `gopkg.in/yaml.v3` | v3.0.1 | YAML 配置解析 |
| `golang.org/x/term` | v0.27.0 | 密码输入隐藏 |

### 6.2 模块依赖图

```
main.go
  │
  ├── cmd (所有命令)
  │     ├── internal/config
  │     ├── internal/agent
  │     │     ├── internal/provider
  │     │     ├── internal/cost
  │     │     └── agentprimordia/pkg
  │     ├── internal/session
  │     ├── internal/ui
  │     └── internal/logx
  │
  └── agentprimordia/pkg
```

### 6.3 外部框架依赖 (AgentPrimordia)

| AP 包 | 用途 |
|-------|------|
| `pkg` | 导出类型：Agent, Session, Memory, ToolRegistry, RAGStore, MCPClient 等 |
| `pkg/llm` | LLM Provider 接口、Usage、Pricing |
| `pkg/tools` | ToolRegistry、PluginLoader、Toolkit |

---

## 7. 项目运行方式

### 7.1 编译

```bash
# 克隆项目
git clone <repo-url>
cd codecastcli

# 确保 AgentPrimordia 在同级目录
# ../agentprimordia/

# 编译
go build -o codecast .

# 或安装到 $GOPATH/bin
go install
```

### 7.2 首次配置

> **v0.2.0 变更**：`codecast config` 子命令已迁移到交互模式内的 `/config` 斜杠命令。

```bash
# 启动交互模式
codecast

# 在交互模式内使用 /config 子命令:
/config wizard                  # 启动交互式配置向导
/config set api_key <key>       # 设置 API Key
/config set model gpt-4o        # 设置模型
/config set provider openai     # 设置 Provider

# 或通过环境变量配置 (无需进入交互模式)
export CODECAST_API_KEY=<your-api-key>
export CODECAST_MODEL=gpt-4o
export CODECAST_PROVIDER=openai
```

### 7.3 常用命令

```bash
# 交互式对话
codecast

# 单轮对话
codecast chat "写一个 Go 的 HTTP 服务器"

# 以下命令在 v0.2.0+ 需在交互模式中使用 /<cmd> 形式：
# /cost summary
# /cost daily 7
# /cost list 20
# /session list
# /session show <session-id>
# /session export <session-id> output.md
# /rag index ./docs --recursive
# /rag query "什么是 Agent"
# /workflow pipeline "开发登录功能" --steps "需求分析,代码开发,测试验证"
# /mcp add my-server --url http://localhost:8080
# /mcp test my-server
# /plugin install my-plugin
```

### 7.4 环境变量

| 变量 | 说明 |
|------|------|
| `CODECAST_API_KEY` | API Key（覆盖配置文件） |
| `CODECAST_MODEL` | 模型名称 |
| `CODECAST_PROVIDER` | Provider 名称 |
| `CODECAST_BASE_URL` | 自定义 API Base URL |

---

## 8. 配置说明

### 8.1 配置文件位置

- **Linux/macOS**: `~/.codecast/config.yaml`
- **Windows**: `%USERPROFILE%\.codecast\config.yaml`

### 8.2 配置示例

```yaml
api_key: sk-xxxxxxxxxxxxxxxxxxxxxxxx
model: gpt-4o
provider: openai
base_url: ""
```

### 8.3 数据文件

| 文件 | 说明 |
|------|------|
| `~/.codecast/config.yaml` | 用户配置 |
| `~/.codecast/memory.db` | 对话记忆（AP 框架） |
| `~/.codecast/cost.db` | 成本记录 |
| `~/.codecast/rag_memory.db` | RAG 知识库 |
| `~/.codecast/codecast.log` | 应用日志 |

---

## 9. 测试

### 9.1 运行测试

```bash
# 全部测试
go test ./...

# 指定包测试
go test ./internal/config/... -v
go test ./internal/cost/... -v
go test ./internal/logx/... -v
```

### 9.2 测试覆盖

| 包 | 测试文件 | 覆盖内容 |
|----|---------|---------|
| `internal/config` | `config_test.go` | 默认值、验证、保存/加载 |
| `internal/cost` | `tracker_test.go` | 记录、汇总、查询、清空 |
| `internal/logx` | `logx_test.go` | 级别解析、初始化、动态调整 |

---

## 附录：交互模式命令

在交互式对话中，输入以下命令：

| 命令 | 说明 |
|------|------|
| `/help`, `/h` | 显示帮助 |
| `/quit`, `/q` | 退出程序 |
| `/clear` | 清除对话上下文 |
| `/tools` | 查看可用工具 |
| `/models` | 查看支持的模型 |
| `/stats` | 查看 Agent 统计 |
| `/sessions` | 查看会话列表 |
| `/export` | 导出当前会话为 Markdown |
| `/export <文件>` | 导出到指定文件 |
