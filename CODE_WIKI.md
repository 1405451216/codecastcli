# Codecast CLI - Code Wiki

> AI Agent 终端 CLI 工具，基于 AgentPrimordia 框架构建  
> 版本: v1.0.0 | Go 1.26+

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
10. [高级特性](#10-高级特性)

---

## 1. 项目概述

Codecast CLI 是一个 AI 驱动的终端 Agent 工具，类似于 Claude Code、Gemini CLI。它允许用户通过命令行与大型语言模型（LLM）进行交互式对话，并支持文件操作、代码执行、知识库检索等高级功能。

### 核心特性

- **13+ LLM Provider 支持**: OpenAI / Anthropic / Gemini / Ollama / DeepSeek / Qwen / GLM / Azure / Cohere / Mistral 等
- **智能代码理解**: 自动索引代码库结构，注入系统提示词，上下文感知对话
- **tree-sitter AST 解析**: Go/Python/JavaScript/TypeScript 精确语法分析，提取函数、类、变量等符号
- **LSP 语言服务集成**: gopls / pyright / tsserver 深度集成，提供定义跳转、引用查找
- **Bubble Tea TUI**: 基于 Bubble Tea 的现代化终端 UI，支持流式渲染、Spinner、进度条
- **子 Agent 自动并行编排**: Plan+Execute DAG 编排，自动检测文件冲突决定并行/串行（v1.0+）
- **@file 智能引用**: 输入 @<path> 自动展开文件内容，支持语言检测、截断、补全
- **三级权限模型**: `suggest`（建议）/ `auto-edit`（自动编辑）/ `full-auto`（全自动），精细控制工具调用审批
- **流式 Markdown 实时渲染**: 基于 Glamour 的高质量终端 Markdown 渲染
- **模糊编辑（Fuzzy Edit）**: Levenshtein 距离模糊匹配，容忍缩进/空白差异（v1.0+）
- **自动 Git Checkpoint + Undo 回滚**: 文件修改前自动创建检查点，支持一键撤销
- **Git-Aware AI**: 自动注入 blame、diff、commit 历史，让 AI 理解代码演变上下文
- **智能模型路由（L1+L2）**: L1 特征分类 + L2 Wilson 置信区间学习路由器（v1.0+）
- **代码库语义索引**: tree-sitter 符号切块 + embedding + BM25 混合检索（v1.0+）
- **基准测试框架**: 15 个内置任务量化评估配置表现（v1.0+）
- **MCP 协议扩展**: 10+ 内置模板（filesystem / github / postgres / puppeteer 等）
- **成本预算控制**: 每日/每会话预算上限，Token 用量追踪，超限自动中断
- **国内 Embedding Provider**: 智谱（Zhipu）/ 通义（DashScope）OpenAI 兼容接口（v1.0+）
- **多平台分发**: goreleaser 支持 Linux/macOS/Windows 多架构，提供 Homebrew/Scoop/DEB/RPM
- **插件生态系统**: 可扩展的插件注册与管理机制
- **提示词 A/B 框架**: 17 种内置变体，支持任务感知路由和自动收敛评估

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
│  │ session │   ui    │ indexer │  tools  │     tui         ││
│  ├─────────┼─────────┼─────────┼─────────┼─────────────────┤│
│  │permission│subagent │ routing │promptab │   checkpoint    ││
│  └─────────┴─────────┴─────────┴─────────┴─────────────────┘│
├─────────────────────────────────────────────────────────────┤
│                    外部框架 (AgentPrimordia)                  │
│  ┌─────────┬─────────┬─────────┬─────────┬─────────────────┐│
│  │  Agent  │  LLM    │ Memory  │  Tools  │   RAG / MCP     ││
│  └─────────┴─────────┴─────────┴─────────┴─────────────────┘│
├─────────────────────────────────────────────────────────────┤
│                      第三方依赖                              │
│  Cobra + Viper │ Bubble Tea │ tree-sitter │ modernc/sqlite │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 目录结构

```
codecastcli/
├── main.go                      # 程序入口
├── go.mod                       # Go 模块定义
├── README.md                    # 项目说明
├── CODE_WIKI.md                 # 本文档
│
├── cmd/                         # CLI 命令定义 (Cobra)
│   ├── root.go                  # 根命令、全局配置初始化
│   ├── interactive.go           # 交互式 REPL 模式
│   ├── interactive_*.go         # 交互模式子命令处理器
│   ├── chat.go                  # 单轮对话命令
│   ├── exec.go                  # Headless 模式执行
│   ├── config.go                # 配置管理命令
│   ├── cost.go                  # 成本追踪命令
│   ├── session.go               # 会话管理
│   ├── mcp.go                   # MCP Server 管理
│   ├── plugin.go                # 插件管理
│   ├── rag.go                   # RAG 知识库
│   ├── workflow.go              # 多 Agent 工作流
│   ├── pool.go                  # Agent Pool
│   ├── sandbox.go               # 沙箱管理
│   ├── ab.go                    # A/B 测试管理
│   ├── feedback.go              # 反馈收集
│   └── init.go                  # 项目初始化
│
├── internal/                    # 内部实现
│   ├── agent/                   # Agent 封装
│   │   ├── agent.go             # CodecastAgent 核心
│   │   ├── stream.go            # 流式输出处理
│   │   ├── prompt.go            # 系统提示词构建
│   │   ├── prompt_resolver.go   # 提示词解析器
│   │   ├── router_*.go          # 模型路由缓存
│   │   ├── mcp_integration.go   # MCP 集成
│   │   ├── ab_integration.go    # A/B 测试集成
│   │   └── tui_adapter.go       # TUI 适配器
│   │
│   ├── config/                  # 配置管理
│   │   ├── config.go            # Config 结构体
│   │   └── config_test.go       # 配置测试
│   │
│   ├── cost/                    # 成本追踪
│   │   ├── tracker.go           # Tracker 实现
│   │   └── tracker_test.go      # 成本测试
│   │
│   ├── logx/                    # 日志系统
│   │   ├── logx.go              # slog 封装
│   │   └── logx_test.go         # 日志测试
│   │
│   ├── provider/                # LLM Provider 工厂
│   │   ├── factory.go           # CreateProvider
│   │   └── factory_test.go      # 工厂测试
│   │
│   ├── session/                 # 会话管理
│   │   ├── manager.go           # Session Manager
│   │   └── manager_test.go      # 会话测试
│   │
│   ├── ui/                      # UI 输出
│   │   ├── ui.go                # 终端输出辅助
│   │   ├── markdown.go          # Markdown 渲染
│   │   └── spinner.go           # 加载动画
│   │
│   ├── tui/                     # TUI 界面
│   │   ├── bubbletea.go         # Bubble Tea 主界面
│   │   ├── renderer.go          # 渲染器
│   │   ├── statusbar.go         # 状态栏
│   │   ├── dagview.go           # DAG 可视化
│   │   └── filecompleter.go     # 文件补全
│   │
│   ├── tools/                   # 内置工具
│   │   ├── read.go              # 文件读取
│   │   ├── edit.go              # 文件编辑
│   │   ├── multi_edit.go        # 多文件编辑
│   │   ├── delete.go            # 文件删除
│   │   ├── glob.go              # 文件搜索
│   │   ├── grep.go              # 内容搜索
│   │   ├── listdir.go           # 目录列表
│   │   ├── lsp.go               # LSP 工具
│   │   └── gitignore.go         # .gitignore 处理
│   │
│   ├── indexer/                 # 代码库索引器
│   │   ├── indexer.go           # 索引器核心
│   │   ├── treesitter.go        # tree-sitter 解析
│   │   ├── treesitter_ast.go    # AST 处理
│   │   └── cache.go             # 索引缓存
│   │
│   ├── permission/              # 权限管理
│   │   ├── manager.go           # 权限管理器
│   │   └── confirm.go           # 确认对话框
│   │
│   ├── subagent/                # 子 Agent 编排
│   │   ├── orchestrator.go      # 编排器
│   │   └── prompt.go            # 子 Agent 提示词
│   │
│   ├── routing/                 # 智能模型路由
│   │   ├── model_router.go      # 路由器
│   │   └── model_router_test.go # 路由测试
│   │
│   ├── promptab/                # 提示词 A/B 框架
│   │   ├── router.go            # 提示词路由
│   │   ├── variants.go          # 变体管理
│   │   ├── embedded.go          # 内置变体
│   │   ├── render.go            # 渲染器
│   │   └── state.go             # 状态管理
│   │
│   ├── checkpoint/              # Git 检查点
│   │   ├── manager.go           # 检查点管理器
│   │   └── manager_test.go      # 检查点测试
│   │
│   ├── undo/                    # 撤销管理
│   │   ├── manager.go           # 撤销管理器
│   │   └── manager_test.go      # 撤销测试
│   │
│   ├── budget/                  # 预算控制
│   │   ├── controller.go        # 预算控制器
│   │   └── controller_test.go   # 预算测试
│   │
│   ├── context/                 # 上下文管理
│   │   ├── budget.go            # 上下文预算
│   │   ├── compress.go          # 上下文压缩
│   │   └── compressor.go        # 压缩器
│   │
│   ├── mcp/                     # MCP 协议
│   │   └── templates.go         # MCP 模板
│   │
│   ├── mcpcfg/                  # MCP 配置
│   │   ├── mcpcfg.go            # 配置管理
│   │   └── mcpcfg_test.go       # 配置测试
│   │
│   ├── memory/                  # 自动记忆
│   │   ├── auto_persist.go      # 自动持久化
│   │   └── auto_persist_test.go # 持久化测试
│   │
│   ├── model/                   # 模型管理
│   │   ├── switcher.go          # 模型切换器
│   │   └── switcher_test.go     # 切换测试
│   │
│   ├── diff/                    # Diff 预览
│   │   ├── previewer.go         # 预览器
│   │   └── previewer_test.go    # 预览测试
│   │
│   ├── hooks/                   # 钩子管理
│   │   ├── manager.go           # 钩子管理器
│   │   └── script_runner.go     # 脚本运行器
│   │
│   ├── plugin/                  # 插件系统
│   │   ├── manager.go           # 插件管理器
│   │   └── registry.go          # 插件注册表
│   │
│   ├── pool/                    # Agent Pool
│   │   ├── manager.go           # Pool 管理器
│   │   └── manager_test.go      # Pool 测试
│   │
│   ├── sandbox/                 # 沙箱执行
│   │   ├── executor.go          # 执行器
│   │   └── executor_test.go     # 执行测试
│   │
│   ├── vision/                  # 图片分析
│   │   ├── analyzer.go          # 分析器
│   │   └── screenshot.go        # 截图工具
│   │
│   ├── wizard/                  # 配置向导
│   │   ├── wizard.go            # 向导逻辑
│   │   └── readinput_*.go       # 平台特定输入
│   │
│   ├── rules/                   # 项目规则
│   │   ├── loader.go            # 规则加载器
│   │   └── loader_test.go       # 加载测试
│   │
│   ├── profile/                 # Profile 管理
│   │   ├── manager.go           # Profile 管理器
│   │   └── manager_test.go      # Profile 测试
│   │
│   ├── lazy/                    # 懒加载
│   │   ├── loader.go            # 懒加载器
│   │   └── loader_test.go       # 加载测试
│   │
│   ├── lsp/                     # LSP 客户端
│   │   ├── client.go            # LSP 客户端
│   │   └── manager.go           # LSP 管理器
│   │
│   ├── docs/                    # 文档生成
│   │   └── manpage.go           # Man page 生成
│   │
│   ├── errors/                  # 错误处理
│   │   └── errors.go            # 错误定义
│   │
│   ├── util/                    # 工具函数
│   │   ├── paths.go             # 路径处理
│   │   └── strings.go           # 字符串处理
│   │
│   └── version/                 # 版本信息
│       ├── version.go           # 版本定义
│       └── version_test.go      # 版本测试
│
├── completions/                 # Shell 补全脚本
│   ├── codecast.bash
│   ├── codecast.zsh
│   ├── codecast.fish
│   └── codecast.ps1
│
├── docs/                        # 文档
│   ├── plans/                   # 实现计划
│   ├── review/                  # 代码审查
│   ├── superpowers/             # 高级特性
│   └── tutorials/               # 教程
│
├── manifests/                   # 包管理清单
│   └── c/Codecast/Codecast.yaml
│
└── vscode-extension/            # VS Code 扩展
    ├── src/extension.ts
    └── package.json
```

---

## 3. 模块职责

### 3.1 cmd 包 - 命令层

| 文件 | 职责 | 对应 CLI 命令 |
|------|------|--------------|
| `root.go` | 根命令定义、全局 Flag、Viper 配置绑定、日志初始化 | `codecast` |
| `interactive.go` | 交互式 REPL 模式，支持 `/` 特殊命令 | `codecast` (无参数) |
| `interactive_*.go` | 交互模式子命令处理器（命令、文件、Git、会话等） | 交互模式内 `/` 命令 |
| `chat.go` | 单轮对话，无上下文记忆 | `codecast chat <消息>` |
| `exec.go` | Headless 模式执行，适合脚本调用 | `codecast exec <命令>` |
| `config.go` | 配置管理（v0.2.0+ 已迁移到交互模式 `/config`） | `codecast config` (deprecated) |
| `cost.go` | 成本汇总、每日统计、调用记录、清空（已迁移到 `/cost`） | `codecast cost` (deprecated) |
| `session.go` | 会话列表、查看历史、删除、导出 Markdown（已迁移到 `/session`） | `codecast session` (deprecated) |
| `mcp.go` | MCP Server 注册、移除、连接测试（已迁移到 `/mcp`） | `codecast mcp` (deprecated) |
| `rag.go` | 文档索引、知识库查询、基于 RAG 的对话（已迁移到 `/rag`） | `codecast rag` (deprecated) |
| `plugin.go` | 插件列表、加载、卸载、创建模板（已迁移到 `/plugin`） | `codecast plugin` (deprecated) |
| `workflow.go` | Pipeline / Parallel / Handoff 工作流（已迁移到 `/workflow`） | `codecast workflow` (deprecated) |
| `pool.go` | Agent Pool 管理（已迁移到 `/pool`） | `codecast pool` (deprecated) |
| `sandbox.go` | 沙箱环境管理（已迁移到 `/sandbox`） | `codecast sandbox` (deprecated) |
| `ab.go` | A/B 测试管理（已迁移到 `/ab`） | `codecast ab` (deprecated) |
| `feedback.go` | 反馈收集（已迁移到 `/fb`） | `codecast feedback` (deprecated) |
| `init.go` | 项目初始化，生成配置文件 | `codecast init` |

### 3.2 internal/agent - Agent 封装

封装 AgentPrimordia 的 `CapabilityAgent`，提供 CLI 友好的接口。

**核心职责**:
- 聚合 Agent、Memory、Session、CostTracker、PermissionManager 等组件
- 提供同步处理 (`Process`) 和流式处理 (`StreamProcess`) 两种模式
- 管理上下文压缩、Git 检查点、撤销回滚
- 集成 A/B 测试、智能路由、MCP 协议
- 处理 SIGINT 信号（Ctrl+C 取消当前请求或退出 REPL）

**关键类型**:
```go
type CodecastAgent struct {
    agent         *ap.CapabilityAgent    // AP 框架 Agent
    config        *config.Config         // 当前配置
    memory        ap.Memory              // SQLite 记忆存储
    registry      *ap.ToolRegistry       // 工具注册表
    session       *ap.Session            // 当前会话
    costTracker   *cost.Tracker          // 成本追踪器
    permMgr       *permission.Manager    // 权限管理器
    indexer       *indexer.Indexer       // 代码索引器
    modelSwitcher *model.Switcher        // 模型切换器
    undoMgr       *undo.Manager          // 撤销管理器
    checkpointMgr *checkpoint.Manager    // Git 检查点管理器
    budgetCtrl    *budget.Controller     // 预算控制器
    router        *routing.ModelRouter   // 智能模型路由器
    ab            *ABIntegration         // A/B 测试集成
    // ... 其他字段
}
```

### 3.3 internal/config - 配置管理

**核心职责**:
- 定义配置结构体，支持 YAML 序列化
- 从文件 (`~/.codecast/config.yaml`) 和环境变量加载配置
- 提供默认配置和配置验证
- 支持配置持久化保存

**关键类型**:
```go
type Config struct {
    APIKey              string   `yaml:"api_key"`
    Model               string   `yaml:"model"`
    Provider            string   `yaml:"provider"`
    BaseURL             string   `yaml:"base_url,omitempty"`
    SafeMode            bool     `yaml:"safe_mode"`
    PermissionMode      string   `yaml:"permission_mode,omitempty"`
    Scopes              []string `yaml:"scopes,omitempty"`
    AutoCompact         bool     `yaml:"auto_compact,omitempty"`
    AutoCompactRatio    float64  `yaml:"auto_compact_ratio,omitempty"`
    AutoCheckpoint      bool     `yaml:"auto_checkpoint,omitempty"`
    AutoStash           bool     `yaml:"auto_stash,omitempty"`
    DailyBudgetUSD      float64  `yaml:"daily_budget_usd,omitempty"`
    SessionBudgetUSD    float64  `yaml:"session_budget_usd,omitempty"`
    PromptVariant       string   `yaml:"prompt_variant,omitempty"`
    PromptStrategy      string   `yaml:"prompt_strategy,omitempty"`
    PromptWeights       map[string]int `yaml:"prompt_weights,omitempty"`
    Routing             routing.RoutingConfig `yaml:"routing,omitempty"`
    // ... 其他字段
}
```

### 3.4 internal/cost - 成本追踪

**核心职责**:
- 使用 SQLite 持久化成本记录
- 自动计算 USD/CNY 费用
- 提供按模型、按天、按会话的汇总统计
- 支持预算控制和超限中断

**关键类型**:
```go
type Tracker struct {
    db *sql.DB
    mu sync.RWMutex
}

type Record struct {
    ID        int64
    Model     string
    Provider  string
    SessionID string
    Command   string
    InputTokens  int
    OutputTokens int
    CostUSD   float64
    CostCNY   float64
    CreatedAt time.Time
}
```

### 3.5 internal/logx - 日志系统

**核心职责**:
- 基于 Go 标准库 `log/slog`
- 支持 `debug/info/warn/error` 四级
- 支持 `text/json` 格式
- 默认输出到 `~/.codecast/codecast.log`

**关键函数**:
```go
func Init(opts ...Option)              // 初始化全局日志
func SetLevel(l Level)                 // 动态设置日志级别
func Debug/Info/Warn/Error(msg string, args ...any)  // 快捷日志输出
```

### 3.6 internal/provider - Provider 工厂

**核心职责**:
- 根据配置创建对应 LLM Provider
- 支持 13+ Provider：openai, anthropic, gemini, ollama, deepseek, qwen, glm, azure, cohere, mistral 等
- 自动验证连通性（30 秒超时）
- 提供友好的错误提示

**关键函数**:
```go
func CreateProvider(cfg *config.Config) (ap.Provider, error)
```

### 3.7 internal/session - 会话管理

**核心职责**:
- 查询 AP 框架的 SQLite 记忆数据库
- 提供会话列表、历史查看、删除、导出功能
- 支持会话恢复和继续

**关键类型**:
```go
type Manager struct {
    db *sql.DB
}

type Info struct {
    SessionID   string
    Title       string
    MessageCount int
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### 3.8 internal/ui - UI 输出

**核心职责**:
- 提供终端输出辅助函数
- Markdown 渲染（基于 Glamour）
- 加载动画（Spinner）
- 彩色输出（基于 fatih/color）

**关键函数**:
```go
func PrintBanner()                     // ASCII Logo
func PrintHelp()                       // 帮助信息
func PrintAssistant(msg string)        // AI 响应输出（带语法高亮）
func PrintSuccess/Error(msg string)    // 状态提示
```

### 3.9 internal/tui - TUI 界面

**核心职责**:
- 基于 Bubble Tea 框架实现现代化终端 UI
- 提供流式 Markdown 渲染
- 支持 DAG 可视化（多 Agent 协作）
- 文件补全和命令补全
- 状态栏显示（模型、成本、权限模式等）

**关键类型**:
```go
type Renderer struct {
    // Bubble Tea 渲染器
}

type DAGView struct {
    // DAG 可视化组件
}

type FileCompleter struct {
    // 文件补全器
}
```

### 3.10 internal/tools - 内置工具

**核心职责**:
- 提供文件操作工具（read/edit/delete/glob/grep/listdir）
- 支持多文件编辑和回滚
- 集成 LSP 语言服务
- 处理 .gitignore 规则

**关键工具**:

| 工具 | 功能 | 参数 |
|------|------|------|
| `read_file` | 读取文件内容，支持行号范围、大文件截断 | `file_path`, `start_line`, `end_line`, `encoding` |
| `edit_file` | 通过精确字符串替换编辑文件 | `file_path`, `old_string`, `new_string`, `replace_all` |
| `multi_edit` | 多文件批量编辑 | `edits` (数组) |
| `delete_file` | 删除文件 | `file_path` |
| `glob` | 文件搜索（基于模式匹配） | `pattern`, `directory` |
| `grep` | 内容搜索（正则表达式） | `pattern`, `path`, `include`, `exclude` |
| `list_dir` | 列出目录内容 | `path`, `recursive` |
| `lsp` | LSP 工具（定义跳转、引用查找） | `action`, `file_path`, `line`, `character` |

### 3.11 internal/indexer - 代码库索引器

**核心职责**:
- 扫描代码库，构建文件索引
- 使用 tree-sitter 解析 AST，提取函数、类、变量等符号
- 分析依赖关系（import/require/include）
- 支持增量更新和文件监听
- 识别编程语言和文件类型

**关键类型**:
```go
type Indexer struct {
    rootDir    string
    index      *Index
    mu         sync.RWMutex
    ignoreDirs []string
    ignoreExts map[string]bool
    watcher    *fsnotify.Watcher
}

type Index struct {
    Files        map[string]*FileEntry
    Dependencies []Dependency
    Languages    map[string]int
    TotalFiles   int
    TotalSize    int64
    RootDir      string
    IndexedAt    time.Time
}

type FileEntry struct {
    Path     string
    Name     string
    Ext      string
    Size     int64
    ModTime  time.Time
    Language string
    Imports  []string
    Exports  []string
    Tags     []Tag
    IsDir    bool
}
```

### 3.12 internal/permission - 权限管理

**核心职责**:
- 实现三级权限模型：`suggest` / `auto-edit` / `full-auto`
- 控制工具调用审批流程
- 提供确认对话框
- 支持文件访问范围控制（scope）

**关键类型**:
```go
type Manager struct {
    mode           PermissionMode
    scopes         []string
    autoApprove    map[string]bool
}

type PermissionMode string

const (
    ModeSuggest   PermissionMode = "suggest"    // 所有操作需确认
    ModeAutoEdit  PermissionMode = "auto-edit"  // 文件编辑自动批准，Shell 需确认
    ModeFullAuto  PermissionMode = "full-auto"  // 所有操作自动批准
)
```

### 3.13 internal/subagent - 子 Agent 编排

**核心职责**:
- 实现 Plan+Execute 双 Agent 协作模式
- Plan Agent 负责分析任务、制定计划
- Execute Agent 负责执行具体任务
- 支持 DAG 可视化和任务依赖管理
- 隔离子 Agent 内存，互不干扰

**关键类型**:
```go
type Orchestrator struct {
    config             *config.Config
    planAgent          *ap.CapabilityAgent
    execAgent          *ap.CapabilityAgent
    registry           *ap.ToolRegistry
    memory             ap.MemoryStore
    llmProvider        ap.Provider
    dagView            *tui.DAGView
    enableVisualization bool
}

type PlanResult struct {
    Tasks []PlanTask `json:"tasks"`
}

type PlanTask struct {
    ID          string   `json:"id"`
    Description string   `json:"description"`
    DependsOn   []string `json:"depends_on,omitempty"`
    Priority    int      `json:"priority"`
}
```

### 3.14 internal/routing - 智能模型路由

**核心职责**:
- 根据输入复杂度自动选择合适的模型
- 支持三级模型：Simple / Medium / Complex
- 基于关键词和复杂度评分进行路由决策
- 节省成本（简单任务使用便宜模型）

**关键类型**:
```go
type ModelRouter struct {
    cfg RoutingConfig
}

type RoutingConfig struct {
    SimpleModel  string `yaml:"simple_model"`   // gpt-4o-mini
    MediumModel  string `yaml:"medium_model"`   // gpt-4o
    ComplexModel string `yaml:"complex_model"`  // claude-opus-4
    Enabled      bool   `yaml:"enabled"`
}
```

**路由逻辑**:
```go
func (r *ModelRouter) Route(input string, fileCount int) string {
    score := r.complexityScore(input, fileCount)
    switch {
    case score >= 5:
        return r.cfg.ComplexModel   // 复杂任务
    case score >= 2:
        return r.cfg.MediumModel    // 中等任务
    default:
        return r.cfg.SimpleModel    // 简单任务
    }
}
```

### 3.15 internal/promptab - 提示词 A/B 框架

**核心职责**:
- 管理 17 种内置提示词变体
- 支持任务感知路由（L1 关键词 + L2 复杂度）
- 支持 A/B 测试和自动收敛评估
- 支持加权选择和轮询策略
- 支持项目级和用户级自定义变体

**关键类型**:
```go
type Router struct {
    variants    map[string]*Variant
    strategy    Strategy
    weights     map[string]int
    routingRules []RoutingRule
}

type Variant struct {
    Name        string
    Description string
    Author      string
    Sections    map[string]string
}

type Strategy string

const (
    StrategyFixed       Strategy = "fixed"
    StrategyRoundRobin  Strategy = "round-robin"
    StrategyWeighted    Strategy = "weighted"
    StrategyRouted      Strategy = "routed"
)
```

**内置变体**:
- `default`: 平衡模式（推荐）
- `concise`: 极简模式，节省 token
- `safety-first`: 保守模式，反模式强化
- `claude-style`: Claude Fable 5 风格
- `code-reviewer`: 专精代码审查
- `pair-programmer`: 双人编程模式
- `decision-tree`: 5 步评估用户意图
- `self-check`: 回复前 5 步自检
- `scope-guard`: 强制 scope 检查
- `mcp-router`: MCP 工具路由决策树
- `mentor-coach`: 导师教练风格
- `search-then-edit`: 两阶段工作流
- `format-locked`: 标准化约束词
- `architect-edit`: 双 Agent 模式
- `shell-only`: Shell 工具契约
- `lazy-mode`: 禁止 TODO/伪代码
- `overeager-mode`: 严格 scope 控制

### 3.16 internal/checkpoint - Git 检查点

**核心职责**:
- 文件修改前自动创建 Git 检查点
- 支持 `git stash` 和 `git commit` 两种模式
- 提供一键撤销功能
- 记录检查点历史

**关键类型**:
```go
type Manager struct {
    repoPath   string
    useStash   bool
    checkpoints []Checkpoint
    mu         sync.RWMutex
}

type Checkpoint struct {
    ID        string
    Message   string
    CreatedAt time.Time
    Files     []string
}
```

### 3.17 internal/undo - 撤销管理

**核心职责**:
- 实现文件操作的撤销功能
- 支持多步撤销和重做
- 记录操作历史
- 与 Git 检查点集成

**关键类型**:
```go
type Manager struct {
    history []Operation
    pointer int
    mu      sync.RWMutex
}

type Operation struct {
    Type      OperationType
    FilePath  string
    OldContent []byte
    NewContent []byte
    Timestamp time.Time
}
```

### 3.18 internal/budget - 预算控制

**核心职责**:
- 实现每日/每会话预算上限
- 追踪 Token 用量和费用
- 超限自动中断
- 提供预算查询和重置功能

**关键类型**:
```go
type Controller struct {
    dailyBudgetUSD    float64
    sessionBudgetUSD  float64
    dailyTokenLimit   int
    sessionTokenLimit int
    dailySpent        float64
    sessionSpent      float64
    dailyTokens       int
    sessionTokens     int
    mu                sync.RWMutex
}
```

### 3.19 internal/context - 上下文管理

**核心职责**:
- 实现智能上下文压缩
- 管理上下文预算和阈值
- 保留最近消息，压缩历史消息
- 支持自动压缩触发

**关键类型**:
```go
type Compressor struct {
    budget        int
    threshold     float64
    preserveRecent int
}

type Budget struct {
    maxTokens    int
    usedTokens   int
    threshold    float64
}
```

### 3.20 internal/mcp - MCP 协议

**核心职责**:
- 提供 MCP 服务器模板定义
- 支持 10+ 内置模板（filesystem / github / postgres / puppeteer 等）
- 管理 MCP 配置和连接
- 集成外部工具服务器

**内置模板**:

| 模板 | 类别 | 说明 |
|------|------|------|
| `filesystem` | core | 文件系统读写 |
| `github` | devops | GitHub PR/Issue/代码搜索 |
| `gitlab` | devops | GitLab 项目管理 |
| `postgres` | database | PostgreSQL 数据库操作 |
| `sqlite` | database | SQLite 数据库操作 |
| `brave-search` | search | Brave 网络搜索 |
| `puppeteer` | browser | 浏览器自动化 |
| `slack` | communication | Slack 消息管理 |
| `memory` | core | 记忆存储 |
| `sequential-thinking` | reasoning | 顺序思考（增强推理） |

### 3.21 其他内部模块

| 模块 | 职责 |
|------|------|
| `memory` | 自动记忆持久化，提取关键信息保存到长期记忆 |
| `model` | 模型切换器，支持运行时动态切换模型 |
| `diff` | Diff 预览器，展示文件修改前后对比 |
| `hooks` | 钩子管理器，支持生命周期钩子（PreToolUse / PostToolUse 等） |
| `plugin` | 插件系统，支持动态加载 Go 插件 |
| `pool` | Agent Pool 管理，支持多 Agent 并行执行 |
| `sandbox` | 沙箱执行器，隔离执行环境 |
| `vision` | 图片分析和截图工具，支持多模态输入 |
| `wizard` | 配置向导，交互式引导用户配置 |
| `rules` | 项目规则加载器，读取 `.codecast/rules.yaml` |
| `profile` | Profile 管理器，支持多配置方案切换 |
| `lazy` | 懒加载器，延迟初始化重量级模块 |
| `lsp` | LSP 客户端，集成语言服务器协议 |
| `docs` | 文档生成器，生成 man page |
| `errors` | 错误定义和处理 |
| `util` | 工具函数（路径处理、字符串处理） |
| `version` | 版本信息定义（v1.0.0） |
| `ab` | 提示词 A/B 收敛器，Wilson 95% CI + 显著性检验 |
| `commands` | 斜杠命令加载器，从 `.codecast/commands/*.md` 加载 |
| `mcpcfg` | MCP 配置管理，持久化 MCP 服务器配置 |
| `benchmark` | 基准测试框架（v1.0+），15 任务量化评估 |
| `semantic` | 代码库语义索引（v1.0+），embedding+BM25 混合检索 |
| `promptab` | 提示词 A/B 框架，17 内置变体 + 任务感知路由 |

---

## 4. 关键类与函数

### 4.1 CodecastAgent (internal/agent/agent.go)

```go
type CodecastAgent struct {
    agent         *ap.CapabilityAgent    // AP 框架 Agent
    config        *config.Config         // 当前配置
    memory        ap.Memory              // SQLite 记忆存储
    registry      *ap.ToolRegistry       // 工具注册表
    session       *ap.Session            // 当前会话
    sessionID     string
    costTracker   *cost.Tracker          // 成本追踪器
    mcpRegistry   *ap.MCPRegistry        // MCP 注册表
    permMgr       *permission.Manager    // 权限管理器
    indexer       *indexer.Indexer       // 代码索引器
    modelSwitcher *model.Switcher        // 模型切换器
    diffPreview   *diff.Previewer        // Diff 预览器
    renderer      *tui.Renderer          // TUI 渲染器
    undoMgr       *undo.Manager          // 撤销管理器
    checkpointMgr *checkpoint.Manager    // Git 检查点管理器
    budgetCtrl    *budget.Controller     // 预算控制器
    lazySandbox   *lazy.Value[*sandbox.Executor]
    lazyAutoMem   *lazy.Value[*automem.AutoPersister]
    mcpWarnings   []MCPWarning
    sharedDB      *sql.DB
    ab            *ABIntegration         // A/B 测试集成
    currentVariant string
    routerPrompt  *RouterCache
    router        *routing.ModelRouter   // 智能模型路由器
    processing    atomic.Bool
    sessionMu     sync.RWMutex
    compressedHistory []ap.Message
    compressedMu      sync.RWMutex
    summarizeMu sync.Mutex
    summarizing bool
}
```

| 方法 | 签名 | 说明 |
|------|------|------|
| `New` | `func New(cfg *config.Config, sessionID string) (*CodecastAgent, error)` | 创建 Agent，初始化 Provider、Memory、Registry、Session |
| `Process` | `func (a *CodecastAgent) Process(ctx context.Context, input string) error` | 同步处理，调用后自动记录成本 |
| `StreamProcess` | `func (a *CodecastAgent) StreamProcess(ctx context.Context, input string) error` | 流式处理，实时输出，完成后记录成本 |
| `ClearContext` | `func (a *CodecastAgent) ClearContext()` | 清除当前会话上下文 |
| `GetStats` | `func (a *CodecastAgent) GetStats() Stats` | 返回运行统计 |
| `Close` | `func (a *CodecastAgent) Close() error` | 关闭资源（costTracker、memory） |
| `GetSharedDB` | `func (a *CodecastAgent) GetSharedDB() *sql.DB` | 获取共享的 SQLite 连接 |
| `SummarizeContext` | `func (a *CodecastAgent) SummarizeContext(ctx context.Context) error` | 压缩上下文 |
| `SwitchModel` | `func (a *CodecastAgent) SwitchModel(model string) error` | 动态切换模型 |
| `Undo` | `func (a *CodecastAgent) Undo() error` | 撤销最近操作 |
| `CreateCheckpoint` | `func (a *CodecastAgent) CreateCheckpoint(message string) error` | 创建 Git 检查点 |

### 4.2 Config (internal/config/config.go)

```go
type Config struct {
    APIKey              string   `yaml:"api_key"`
    Model               string   `yaml:"model"`
    Provider            string   `yaml:"provider"`
    BaseURL             string   `yaml:"base_url,omitempty"`
    SafeMode            bool     `yaml:"safe_mode"`
    PermissionMode      string   `yaml:"permission_mode,omitempty"`
    Scopes              []string `yaml:"scopes,omitempty"`
    SummaryModel        string   `yaml:"summary_model,omitempty"`
    ContextBudget       int      `yaml:"context_budget,omitempty"`
    ContextThreshold    float64  `yaml:"context_threshold,omitempty"`
    ContextCompress     bool     `yaml:"context_compress,omitempty"`
    PreserveRecent      int      `yaml:"preserve_recent,omitempty"`
    ProjectRoot         string   `yaml:"project_root,omitempty"`
    AutoCompact         bool     `yaml:"auto_compact,omitempty"`
    AutoCompactRatio    float64  `yaml:"auto_compact_ratio,omitempty"`
    AutoCheckpoint      bool     `yaml:"auto_checkpoint,omitempty"`
    AutoStash           bool     `yaml:"auto_stash,omitempty"`
    DailyBudgetUSD      float64  `yaml:"daily_budget_usd,omitempty"`
    SessionBudgetUSD    float64  `yaml:"session_budget_usd,omitempty"`
    DailyTokenLimit     int      `yaml:"daily_token_limit,omitempty"`
    SessionTokenLimit   int      `yaml:"session_token_limit,omitempty"`
    PromptVariant       string   `yaml:"prompt_variant,omitempty"`
    PromptStrategy      string   `yaml:"prompt_strategy,omitempty"`
    PromptWeights       map[string]int `yaml:"prompt_weights,omitempty"`
    PromptProjectDir    string   `yaml:"prompt_project_dir,omitempty"`
    Routing             routing.RoutingConfig `yaml:"routing,omitempty"`
}
```

| 函数/方法 | 签名 | 说明 |
|-----------|------|------|
| `Default` | `func Default() *Config` | 返回默认配置（gpt-4o / openai） |
| `Load` | `func Load() *Config` | 从文件和环境变量加载 |
| `Save` | `func Save(cfg *Config) error` | 保存到 YAML 文件 |
| `Validate` | `func (c *Config) Validate() error` | 校验必填字段 |
| `GetConfigDir` | `func GetConfigDir() string` | 返回配置目录路径 |
| `MaskedAPIKey` | `func (c *Config) MaskedAPIKey() string` | 返回遮蔽后的 API Key |

### 4.3 Tracker (internal/cost/tracker.go)

```go
type Tracker struct {
    db *sql.DB
    mu sync.RWMutex
}

type Record struct {
    ID           int64
    Model        string
    Provider     string
    SessionID    string
    Command      string
    InputTokens  int
    OutputTokens int
    CostUSD      float64
    CostCNY      float64
    CreatedAt    time.Time
}

type Summary struct {
    TotalCostUSD   float64
    TotalCostCNY   float64
    TotalTokens    int
    CallCount      int
    ByModel        map[string]*ModelSummary
}

type Daily struct {
    Date         string
    CostUSD      float64
    CostCNY      float64
    Tokens       int
    CallCount    int
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
| `SessionSummary` | `func (t *Tracker) SessionSummary(sessionID string) (*Summary, error)` | 按会话汇总 |

### 4.4 Manager (internal/session/manager.go)

```go
type Manager struct {
    db *sql.DB
}

type Info struct {
    SessionID    string
    Title        string
    MessageCount int
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

type Message struct {
    Role      string
    Content   string
    Timestamp time.Time
}
```

| 方法 | 签名 | 说明 |
|------|------|------|
| `NewManager` | `func NewManager() (*Manager, error)` | 创建 Manager |
| `List` | `func (m *Manager) List() ([]Info, error)` | 查询所有会话 |
| `GetHistory` | `func (m *Manager) GetHistory(sessionID string, limit int) ([]Message, error)` | 获取会话历史 |
| `Delete` | `func (m *Manager) Delete(sessionID string) error` | 删除会话 |
| `Export` | `func (m *Manager) Export(sessionID string, outputPath string) error` | 导出会话为 Markdown |

### 4.5 Indexer (internal/indexer/indexer.go)

```go
type Indexer struct {
    rootDir    string
    index      *Index
    mu         sync.RWMutex
    ignoreDirs []string
    ignoreExts map[string]bool
    watcher    *fsnotify.Watcher
    done       chan struct{}
    stopOnce   sync.Once
}

type Index struct {
    Files        map[string]*FileEntry
    Dependencies []Dependency
    Languages    map[string]int
    TotalFiles   int
    TotalSize    int64
    RootDir      string
    IndexedAt    time.Time
}

type FileEntry struct {
    Path     string
    Name     string
    Ext      string
    Size     int64
    ModTime  time.Time
    Language string
    Imports  []string
    Exports  []string
    Tags     []Tag
    IsDir    bool
}

type Tag struct {
    Name string
    Kind string  // "function", "class", "variable", etc.
    Line int
}

type Dependency struct {
    From string
    To   string
    Type string  // "import", "require", "include"
}
```

| 方法 | 签名 | 说明 |
|------|------|------|
| `NewIndexer` | `func NewIndexer(rootDir string) *Indexer` | 创建索引器 |
| `Build` | `func (idx *Indexer) Build() error` | 构建索引 |
| `Update` | `func (idx *Indexer) Update(filePath string) error` | 更新单个文件索引 |
| `Watch` | `func (idx *Indexer) Watch() error` | 启动文件监听 |
| `Stop` | `func (idx *Indexer) Stop()` | 停止监听 |
| `GetIndex` | `func (idx *Indexer) GetIndex() *Index` | 获取当前索引 |
| `Search` | `func (idx *Indexer) Search(query string) ([]*FileEntry, error)` | 搜索文件 |

### 4.6 Orchestrator (internal/subagent/orchestrator.go)

```go
type Orchestrator struct {
    config             *config.Config
    planAgent          *ap.CapabilityAgent
    execAgent          *ap.CapabilityAgent
    registry           *ap.ToolRegistry
    memory             ap.MemoryStore
    llmProvider        ap.Provider
    dagView            *tui.DAGView
    enableVisualization bool
}

type PlanResult struct {
    Tasks []PlanTask `json:"tasks"`
}

type PlanTask struct {
    ID          string   `json:"id"`
    Description string   `json:"description"`
    DependsOn   []string `json:"depends_on,omitempty"`
    Priority    int      `json:"priority"`
}

type ExecutionResult struct {
    Plan    *PlanResult
    Results map[string]*TaskResult
}

type TaskResult struct {
    TaskID  string
    Success bool
    Output  string
    Error   error
}
```

| 方法 | 签名 | 说明 |
|------|------|------|
| `NewOrchestrator` | `func NewOrchestrator(cfg *config.Config, registry *ap.ToolRegistry, memory ap.MemoryStore) (*Orchestrator, error)` | 创建编排器 |
| `PlanAndExecute` | `func (o *Orchestrator) PlanAndExecute(ctx context.Context, task string) (*ExecutionResult, error)` | 规划并执行任务 |
| `EnableVisualization` | `func (o *Orchestrator) EnableVisualization(enable bool)` | 启用/禁用 DAG 可视化 |

### 4.7 ModelRouter (internal/routing/model_router.go)

```go
type ModelRouter struct {
    cfg RoutingConfig
}

type RoutingConfig struct {
    SimpleModel  string `yaml:"simple_model"`
    MediumModel  string `yaml:"medium_model"`
    ComplexModel string `yaml:"complex_model"`
    Enabled      bool   `yaml:"enabled"`
}
```

| 方法 | 签名 | 说明 |
|------|------|------|
| `NewModelRouter` | `func NewModelRouter(cfg RoutingConfig) *ModelRouter` | 创建模型路由器 |
| `Route` | `func (r *ModelRouter) Route(input string, fileCount int) string` | 根据输入选择合适的模型 |
| `IsEnabled` | `func (r *ModelRouter) IsEnabled() bool` | 返回路由器是否启用 |
| `SetEnabled` | `func (r *ModelRouter) SetEnabled(enabled bool)` | 动态启用或禁用路由器 |
| `Config` | `func (r *ModelRouter) Config() RoutingConfig` | 返回当前路由配置的副本 |

### 4.8 日志函数 (internal/logx/logx.go)

| 函数 | 签名 | 说明 |
|------|------|------|
| `Init` | `func Init(opts ...Option)` | 初始化全局日志（单例） |
| `SetLevel` | `func SetLevel(l Level)` | 动态设置日志级别 |
| `Debug/Info/Warn/Error` | `func XXX(msg string, args ...any)` | 快捷日志输出 |

### 4.9 Provider 工厂 (internal/provider/factory.go)

| 函数 | 签名 | 说明 |
|------|------|------|
| `CreateProvider` | `func CreateProvider(cfg *config.Config) (ap.Provider, error)` | 根据配置创建 LLM Provider |

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

### 5.3 工具调用数据流

```
LLM 响应 (包含工具调用)
    │
    ▼
┌──────────────┐
│ CodecastAgent│
│   Process()  │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Permission   │  检查权限
│  Manager     │
└──────┬───────┘
       │ 批准
       ▼
┌──────────────┐
│ ToolRegistry │  查找工具
│   .Get()     │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│  Tool.Run()  │  执行工具
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Checkpoint   │  创建检查点（如果是文件修改）
│  Manager     │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Undo Manager │  记录操作
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ LLM (继续)   │  将工具结果反馈给 LLM
└──────────────┘
```

### 5.4 Plan+Execute 双 Agent 数据流

```
用户任务
    │
    ▼
┌──────────────┐
│ Orchestrator │
│PlanAndExecute│
└──────┬───────┘
       │
       ▼
┌──────────────┐
│  Plan Agent  │  分析任务，制定计划
│  (max 5 turns)│
└──────┬───────┘
       │ PlanResult (Tasks[])
       ▼
┌──────────────┐
│  DAG View    │  可视化任务依赖
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Execute Agent│  逐个执行任务
│ (max 15 turns)│
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ExecutionResult│  汇总执行结果
└──────────────┘
```

### 5.5 智能模型路由数据流

```
用户输入
    │
    ▼
┌──────────────┐
│ModelRouter   │
│  .Route()    │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│complexityScore│  计算复杂度分数
│  (关键词+长度)│
└──────┬───────┘
       │
       ▼
┌──────────────┐
│  选择模型    │  score >= 5: ComplexModel
│              │  score >= 2: MediumModel
│              │  score < 2:  SimpleModel
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ CreateProvider│  创建对应 Provider
└──────────────┘
```

---

## 6. 依赖关系

### 6.1 直接依赖 (go.mod)

| 依赖 | 版本 | 用途 |
|------|------|------|
| `agentprimordia` | v0.0.0 (replace) | Agent 框架核心 |
| `github.com/spf13/cobra` | v1.8.0 | CLI 命令框架 |
| `github.com/spf13/viper` | v1.18.2 | 配置管理 |
| `github.com/fatih/color` | v1.16.0 | 终端彩色输出 |
| `github.com/charmbracelet/bubbletea` | v1.3.10 | TUI 框架 |
| `github.com/charmbracelet/glamour` | v1.0.0 | Markdown 渲染 |
| `github.com/charmbracelet/lipgloss` | v1.1.1 | TUI 样式 |
| `github.com/c-bata/go-prompt` | v0.2.6 | 命令行补全 |
| `github.com/sabhiram/go-gitignore` | v0.0.0-20210923224102-525f6e181f06 | .gitignore 解析 |
| `github.com/tree-sitter/go-tree-sitter` | v0.25.0 | tree-sitter 绑定 |
| `github.com/tree-sitter/tree-sitter-go` | v0.25.0 | Go 语法解析 |
| `github.com/tree-sitter/tree-sitter-python` | v0.25.0 | Python 语法解析 |
| `github.com/tree-sitter/tree-sitter-javascript` | v0.25.0 | JavaScript 语法解析 |
| `github.com/tree-sitter/tree-sitter-typescript` | v0.23.2 | TypeScript 语法解析 |
| `modernc.org/sqlite` | v1.50.1 | SQLite 数据库 (CGO-free) |
| `gopkg.in/yaml.v3` | v3.0.1 | YAML 配置解析 |
| `golang.org/x/term` | v0.44.0 | 密码输入隐藏 |
| `github.com/fsnotify/fsnotify` | v1.7.0 | 文件监听 |

### 6.2 模块依赖图

```
main.go
  │
  ├── cmd (所有命令)
  │     ├── internal/config
  │     ├── internal/agent
  │     │     ├── internal/provider
  │     │     ├── internal/cost
  │     │     ├── internal/permission
  │     │     ├── internal/indexer
  │     │     ├── internal/checkpoint
  │     │     ├── internal/undo
  │     │     ├── internal/budget
  │     │     ├── internal/context
  │     │     ├── internal/routing
  │     │     ├── internal/promptab
  │     │     ├── internal/subagent
  │     │     ├── internal/tui
  │     │     └── agentprimordia/pkg
  │     ├── internal/session
  │     ├── internal/ui
  │     ├── internal/logx
  │     ├── internal/tools
  │     │     ├── internal/util
  │     │     └── agentprimordia/pkg
  │     ├── internal/mcp
  │     ├── internal/mcpcfg
  │     ├── internal/plugin
  │     ├── internal/pool
  │     ├── internal/sandbox
  │     ├── internal/vision
  │     ├── internal/wizard
  │     ├── internal/rules
  │     ├── internal/profile
  │     ├── internal/lazy
  │     ├── internal/lsp
  │     ├── internal/docs
  │     ├── internal/errors
  │     └── internal/version
  │
  └── agentprimordia/pkg
```

### 6.3 外部框架依赖 (AgentPrimordia)

| AP 包 | 用途 |
|-------|------|
| `pkg` | 导出类型：Agent, Session, Memory, ToolRegistry, RAGStore, MCPClient 等 |
| `pkg/llm` | LLM Provider 接口、Usage、Pricing |
| `pkg/tools` | ToolRegistry、PluginLoader、Toolkit |

### 6.4 核心模块依赖关系

```
CodecastAgent
  ├── Config
  ├── Provider (via CreateProvider)
  ├── Memory (SQLite)
  ├── Session
  ├── ToolRegistry
  │     └── Tools (read/edit/delete/glob/grep/listdir/lsp)
  ├── CostTracker
  ├── PermissionManager
  ├── Indexer
  │     └── tree-sitter (Go/Python/JS/TS)
  ├── ModelSwitcher
  ├── UndoManager
  ├── CheckpointManager
  ├── BudgetController
  ├── ModelRouter
  ├── PromptRouter (promptab)
  ├── SubagentOrchestrator
  │     ├── PlanAgent
  │     └── ExecuteAgent
  └── TUIRenderer (Bubble Tea)
```

---

## 7. 项目运行方式

### 7.1 编译

```bash
# 克隆项目
git clone https://github.com/codecast/codecastcli.git
cd codecastcli

# 确保 AgentPrimordia 在同级目录
# ../AgentPrimordia/agentprimordia/

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

# Headless 模式执行
codecast exec "解释这个项目的架构"

# 继续最近的会话
codecast --continue

# 恢复指定会话
codecast --resume <session-id>

# 使用特定模型
codecast --model claude-opus-4 --provider anthropic

# 启用智能路由
codecast --prompt-strategy routed

# 启用自动压缩
codecast --auto-compact --auto-compact-ratio 0.8

# 启用自动检查点
codecast --auto-checkpoint --auto-stash

# 设置预算限制
codecast --daily-budget 10 --session-budget 5

# 使用特定提示词变体
codecast --prompt-variant concise

# 安全模式（禁用 Shell 和 Web）
codecast --safe

# 指定文件访问范围
codecast --scope ./src --scope ./tests
```

### 7.4 交互模式命令

在交互模式中，可以使用以下 `/` 命令：

| 命令 | 说明 |
|------|------|
| `/help`, `/h` | 显示帮助 |
| `/quit`, `/q` | 退出程序 |
| `/clear` | 清除对话上下文 |
| `/compact` | 压缩上下文 |
| `/config [list\|get\|set\|wizard]` | 配置管理 |
| `/cost [summary\|daily\|list\|clear\|by-variant]` | 成本管理 |
| `/session [list\|show\|delete\|export]` | 会话管理 |
| `/mcp [list\|add\|remove\|test\|connect]` | MCP 服务器管理 |
| `/plugin [list\|install\|unload]` | 插件管理 |
| `/pool [status\|run]` | Agent Pool 状态 |
| `/rag [index\|query\|chat]` | RAG 知识库 |
| `/sandbox [status\|build]` | 沙箱管理 |
| `/workflow [pipeline\|parallel\|handoff]` | 多 Agent 工作流 |
| `/model <id>` | 切换模型 |
| `/plan <task>` | 规划任务 |
| `/delegate <task>` | 双 Agent 协作 |
| `/undo` | 撤销最近修改 |
| `/budget` | 查看预算 |
| `/rules` | 查看项目规则 |
| `/tools` | 列出工具 |
| `/vision <path>` | 图片分析 |
| `/index` | 重建索引 |
| `/mode` | 切换权限模式 |
| `/screenshot` | 截图分析 |
| `/review [branch] [--json] [--pr]` | AI 审查代码变更 |
| `/blame <file> [line]` | 查看文件 Git Blame 信息 |
| `/history <file>` | 查看文件修改历史 |
| `/diff [branch]` | 查看当前代码变更 |
| `/route [on\|off\|test]` | 智能模型路由管理 |
| `/prompt [list\|use\|show\|current\|reload]` | 提示词变体管理 |
| `/ab [enable\|disable\|reset\|suggest\|apply\|epsilon]` | A/B 自动收敛管理 |
| `/fb [y\|n\|show\|enable\|disable]` | 主动反馈 A/B 评估 |

### 7.5 环境变量

| 变量 | 说明 |
|------|------|
| `CODECAST_API_KEY` | API Key（覆盖配置文件） |
| `CODECAST_MODEL` | 模型名称 |
| `CODECAST_PROVIDER` | Provider 名称 |
| `CODECAST_BASE_URL` | 自定义 API Base URL |
| `CODECAST_PERMISSION_MODE` | 权限模式 |
| `CODECAST_PROMPT_VARIANT` | 提示词变体 |
| `CODECAST_PROMPT_STRATEGY` | 提示词策略 |

---

## 8. 配置说明

### 8.1 配置文件位置

- **Linux/macOS**: `~/.codecast/config.yaml`
- **Windows**: `%USERPROFILE%\.codecast\config.yaml`

### 8.2 配置示例

```yaml
# 基础配置
api_key: sk-xxxxxxxxxxxxxxxxxxxxxxxx
model: gpt-4o
provider: openai
base_url: ""

# 权限和安全
permission_mode: auto-edit
safe_mode: false
scopes:
  - ./src
  - ./tests

# 上下文管理
auto_compact: true
auto_compact_ratio: 0.8
context_budget: 128000
context_threshold: 0.8
context_compress: true
preserve_recent: 10

# Git 检查点
auto_checkpoint: true
auto_stash: true

# 预算控制
daily_budget_usd: 10.0
session_budget_usd: 5.0
daily_token_limit: 1000000
session_token_limit: 500000

# 提示词 A/B 框架
prompt_variant: default
prompt_strategy: routed
prompt_weights:
  default: 5
  concise: 1
  safety-first: 1
prompt_project_dir: .codecast/prompts

# 智能模型路由
routing:
  enabled: true
  simple_model: gpt-4o-mini
  medium_model: gpt-4o
  complex_model: claude-opus-4
```

### 8.3 数据文件

| 文件 | 说明 |
|------|------|
| `~/.codecast/config.yaml` | 用户配置 |
| `~/.codecast/memory.db` | 对话记忆（AP 框架） |
| `~/.codecast/cost.db` | 成本记录 |
| `~/.codecast/rag_memory.db` | RAG 知识库 |
| `~/.codecast/codecast.log` | 应用日志 |
| `~/.codecast/mcp.json` | MCP 服务器配置 |
| `~/.codecast/plugins.json` | 插件配置 |

### 8.4 项目级配置

项目根目录可以包含以下配置文件：

| 文件 | 说明 |
|------|------|
| `.codecast/config.yaml` | 项目级配置（覆盖用户配置） |
| `.codecast/rules.yaml` | 项目规则（注入系统提示词） |
| `.codecast/prompts/*.yaml` | 项目级提示词变体 |
| `.codecast/prompts/routing.yaml` | 项目级路由规则 |
| `.codecast/commands/*.md` | 项目级自定义命令 |

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
go test ./internal/agent/... -v
go test ./internal/tools/... -v

# 运行基准测试
go test -bench=. ./internal/indexer/...
go test -bench=. ./internal/routing/...

# 生成测试覆盖率
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### 9.2 测试覆盖

| 包 | 测试文件 | 覆盖内容 |
|----|---------|---------|
| `internal/config` | `config_test.go` | 默认值、验证、保存/加载 |
| `internal/cost` | `tracker_test.go`, `tracker_variant_test.go` | 记录、汇总、查询、清空、变体 |
| `internal/logx` | `logx_test.go` | 级别解析、初始化、动态调整 |
| `internal/agent` | `agent_test.go`, `stream_test.go`, `prompt_test.go`, `routing_test.go`, `ab_integration_test.go` | Agent 生命周期、流式处理、提示词、路由、A/B 测试 |
| `internal/tools` | `read_test.go`, `edit_test.go`, `grep_test.go`, `multi_edit_test.go` | 工具功能、边界情况、错误处理 |
| `internal/indexer` | `indexer_test.go`, `treesitter_test.go`, `bench_test.go` | 索引构建、AST 解析、性能 |
| `internal/permission` | `manager_test.go`, `confirm_test.go` | 权限控制、确认逻辑 |
| `internal/subagent` | `orchestrator_test.go`, `orchestrator_dagview_test.go` | 编排逻辑、DAG 可视化 |
| `internal/routing` | `model_router_test.go`, `router_bench_test.go` | 路由决策、性能 |
| `internal/promptab` | `router_test.go`, `variants_test.go`, `decision_test.go` | 提示词路由、变体管理 |
| `internal/checkpoint` | `manager_test.go` | 检查点创建、恢复 |
| `internal/undo` | `manager_test.go` | 撤销、重做 |
| `internal/budget` | `controller_test.go` | 预算控制、超限中断 |
| `internal/context` | `budget_test.go`, `compress_test.go`, `compressor_test.go` | 上下文压缩、预算管理 |
| `internal/session` | `manager_test.go` | 会话管理、历史查询 |
| `internal/provider` | `factory_test.go` | Provider 创建、连通性验证 |

### 9.3 测试类型

| 类型 | 说明 | 示例 |
|------|------|------|
| 单元测试 | 测试单个函数/方法 | `config_test.go`, `tracker_test.go` |
| 集成测试 | 测试模块间交互 | `integration_test.go`, `ab_integration_test.go` |
| 端到端测试 | 测试完整流程 | `e2e_test.go` |
| 基准测试 | 测试性能 | `bench_test.go`, `router_bench_test.go` |
| 压力测试 | 测试高并发场景 | `stress_test.go` |
| 快照测试 | 测试输出一致性 | `snapshot_test.go` |

---

## 10. 高级特性

### 10.1 提示词 A/B 框架

Codecast 内置提示词 **A/B 测试与外部化框架**，可热替换系统提示词，无需重编译。

#### 十七种内置变体

| 变体 | 适用场景 |
|------|----------|
| `default` | 平衡：完整工具指南 + 关键反模式（推荐） |
| `concise` | 极简：节省 token，适合小模型 / 长任务 |
| `safety-first` | 保守：反模式强化 + 强制验证步骤 |
| `claude-style` | 借鉴 Claude Fable 5 风格：XML 标签分章节 + `<good_response>/<bad_response>` 对照 |
| `code-reviewer` | 专精代码审查：分级反馈（🔴/🟡/🟢）+ 安全/性能/可维护性三维度 |
| `pair-programmer` | 双人编程：边做边讲解，每步配意图说明（适合教学/学习场景） |
| `decision-tree` | 借鉴 Claude `request_evaluation_checklist`：5 步评估用户意图再选工具 |
| `self-check` | 回复前 5 步自检（准确性/边界/可逆性/验证/诚实），防低级错误 |
| `scope-guard` | 文件访问前强制 scope 检查（修 F-01 越权访问） |
| `mcp-router` | MCP 工具 vs 内置工具的路由决策树 |
| `mentor-coach` | 借鉴 Claude 的人格层：温暖 + 建设性 push back + 边界感（适合教学/辅导） |
| `search-then-edit` | 借鉴 Aider 两阶段工作流：先 repo-map triage 再 user-add-files（防越权改文件） |
| `format-locked` | 借鉴 Aider 标准化约束词 + repair prompt：MUST/NEVER/ONLY EVER |
| `architect-edit` | 借鉴 Aider 双 Agent：Plan-Agent 出方案 + Edit-Agent 落地 |
| `shell-only` | 借鉴 Aider shell 工具契约：1-3 one-liner、不写代码、分类示例 |
| `lazy-mode` | 借鉴 Aider `lazy_prompt`：禁止 TODO/伪代码，强制完整实现 |
| `overeager-mode` | 借鉴 Aider `overeager_prompt`：严格 scope 控制，绝不"顺手改" |

#### 使用方式

```bash
# CLI flag（最高优先级）
codecast --prompt-variant concise
codecast --prompt-strategy weighted --prompt-weight default=5 --prompt-weight concise=1

# 环境变量
export CODECAST_PROMPT_VARIANT=safety-first
codecast

# 配置文件 ~/.codecast/config.yaml
prompt_variant: concise
prompt_strategy: weighted
prompt_weights:
  default: 5
  concise: 1
```

#### 自定义变体

把 YAML 放到以下任一目录（后加载覆盖先加载）：

1. **编译时嵌入**（仅源码修改）— 见 `internal/promptab/embedded.go`
2. **用户级** `~/.codecast/prompts/*.yaml` — 个人定制
3. **项目级** `.codecast/prompts/*.yaml` — 团队共享，自动按 cwd 加载

YAML 格式：

```yaml
name: my-variant           # 必填，variant 名
description: 简要描述
author: 你的名字
sections:
  identity: |              # 这些 section key 必须存在（值可空，会被跳过）
    你是 ...
  tool_guide: |
    ...
  anti_patterns: |
    ...
  workflow: |
    ...
  output_format: |
    ...
```

支持的 `{{var}}` 变量：`os` / `cwd` / `mode` / `budget` / `mode_advice` / `file_tree` / `project_rules`，以及 `$ARGUMENTS` / `$ARG0`..`$ARG9`。

#### 任务感知路由（v0.3.2+ L1+L2）

`prompt_strategy: routed` 启用任务感知路由，让变体选择**不靠随机**：

| 层级 | 机制 | 例子 |
|------|------|------|
| **L1 关键词** | 关键词 → 固定变体 | "rm -rf" → safety-first；"review" → code-reviewer |
| **L2 复杂度** | 字符/工具/步骤打分 | 长任务 + 工具诉求 → default；极短问句 → concise |
| **A/B 兜底** | 都没命中 | 走 weighted 策略 |

`~/.codecast/config.yaml`：
```yaml
prompt_strategy: routed
```

团队级规则（项目根目录 `.codecast/prompts/routing.yaml`，见 [docs/routing.example.yaml](docs/routing.example.yaml)）：

```yaml
rules:
  - name: my-deploy
    variant: shell-only
    priority: 30
    keywords: ["deploy", "部署", "release"]
complexity:
  long_task_chars: 250
  short_question_chars: 40
  has_tool_hint: ["重构", "refactor", "fix", "添加"]
```

**为什么 L1/L2 不污染 A/B 评估**：路由命中时直接选变体，不进 weighted 抽样；只有"无信号"的轮次才被 A/B 框架处理。

### 10.2 智能模型路由

根据输入复杂度自动选择合适的模型，节省成本：

```yaml
# ~/.codecast/config.yaml
routing:
  enabled: true
  simple_model: gpt-4o-mini      # 简单任务（问答、解释）
  medium_model: gpt-4o           # 中等任务（单文件编辑）
  complex_model: claude-opus-4   # 复杂任务（多文件重构、架构设计）
```

**路由逻辑**:
- 复杂度分数 >= 5: 使用 ComplexModel
- 复杂度分数 >= 2: 使用 MediumModel
- 复杂度分数 < 2: 使用 SimpleModel

**复杂度评分因素**:
- 输入长度
- 文件数量
- 关键词（重构、架构、迁移等）
- 工具调用复杂度

### 10.3 Plan+Execute 双 Agent 协作

使用 `/delegate` 命令启动双 Agent 协作：

```bash
/delegate 实现用户登录功能，包括前端表单和后端 API
```

**工作流程**:
1. **Plan Agent** (最多 5 轮): 分析任务，制定详细计划
2. **Execute Agent** (最多 15 轮): 逐个执行计划中的任务
3. **DAG 可视化**: 实时展示任务依赖和执行状态

**适用场景**:
- 多步骤复杂任务
- 需要规划和执行分离的场景
- 需要任务依赖管理的工作流

### 10.4 Git-Aware AI

自动注入 Git 上下文，让 AI 理解代码演变：

```bash
/review main                    # 审查相对于 main 分支的变更
/blame main.go 42               # 查看 main.go 第 42 行的 blame 信息
/history config.go              # 查看 config.go 的修改历史
/diff feature-branch            # 查看与 feature-branch 的差异
```

**注入的上下文**:
- Git blame 信息（谁、何时、为什么修改）
- 文件修改历史
- 分支差异
- Commit 消息

### 10.5 自动 Git 检查点 + Undo 回滚

文件修改前自动创建检查点，支持一键撤销：

```bash
# 启用自动检查点
codecast --auto-checkpoint --auto-stash

# 在交互模式中
/undo                           # 撤销最近修改
```

**工作原理**:
1. 文件修改前自动创建 `git stash` 或 `git commit`
2. 记录检查点历史
3. `/undo` 命令恢复到最近的检查点
4. 支持多步撤销

### 10.6 成本预算控制

精细控制 API 使用成本：

```bash
# 设置预算限制
codecast --daily-budget 10 --session-budget 5

# 在交互模式中
/cost summary                   # 查看总成本
/cost daily 7                   # 查看最近 7 天每日成本
/cost list 20                   # 查看最近 20 条记录
/budget                         # 查看预算使用情况
```

**预算控制**:
- 每日预算上限（USD）
- 每会话预算上限（USD）
- 每日 Token 上限
- 每会话 Token 上限
- 超限自动中断

### 10.7 MCP 协议扩展

通过 MCP 协议扩展外部工具：

```bash
# 在交互模式中
/mcp list                       # 列出已配置的 MCP 服务器
/mcp add github --url https://mcp.github.com
/mcp test github                # 测试连接
/mcp connect github             # 连接服务器
```

**内置模板**:
- `filesystem`: 文件系统读写
- `github`: GitHub PR/Issue/代码搜索
- `gitlab`: GitLab 项目管理
- `postgres`: PostgreSQL 数据库操作
- `sqlite`: SQLite 数据库操作
- `brave-search`: Brave 网络搜索
- `puppeteer`: 浏览器自动化
- `slack`: Slack 消息管理
- `memory`: 记忆存储
- `sequential-thinking`: 顺序思考（增强推理）

### 10.8 插件系统

动态加载 Go 插件扩展功能：

```bash
# 在交互模式中
/plugin list                    # 列出已安装插件
/plugin install my-plugin       # 安装插件
/plugin unload my-plugin        # 卸载插件
```

**插件结构**:
```go
package main

import "agentprimordia/pkg"

func Init() error {
    // 初始化插件
    return nil
}

func RegisterTools(registry *pkg.ToolRegistry) error {
    // 注册自定义工具
    return nil
}
```

### 10.9 A/B 测试与自动收敛

通过 A/B 测试自动优化提示词：

```bash
# 在交互模式中
/ab enable                      # 启用 A/B 测试
/ab suggest                     # 建议最佳变体
/ab apply                       # 应用建议
/ab epsilon 0.1                 # 设置探索率
/fb y                           # 反馈：好
/fb n                           # 反馈：差
/fb show                        # 查看反馈历史
```

**工作原理**:
1. 随机选择不同提示词变体
2. 收集用户反馈（/fb y/n）
3. 使用 Wilson 95% CI 计算置信区间
4. 自动收敛到最佳变体
5. 支持 HTML 导出测试报告

### 10.10 子 Agent 自动并行编排（v1.0+）

Plan+Execute DAG 编排，自动检测文件冲突决定并行/串行。

**工作流**:
1. Plan Agent 拆分用户请求为多个子任务
2. AutoParallel 检测每个子任务读写的文件集合
3. 无文件冲突的任务自动并行执行
4. 有文件冲突的任务自动串行执行
5. 各子 Agent 在隔离沙箱运行，互不干扰

```bash
# 触发自动并行编排
/subagent "重构 internal/tools 下所有 edit 工具的错误处理"

# 查看当前 DAG
/dag

# 强制串行（避免并行冲突）
/subagent --serial "任务描述"
```

**关键文件**: `internal/subagent/auto_parallel.go`

### 10.11 模糊编辑 Fuzzy Edit（v1.0+）

基于 Levenshtein 距离的模糊匹配，容忍缩进/空白差异。

**置信度策略**:
- `> 0.85`: 自动应用（高置信度）
- `0.6 ~ 0.85`: 询问用户确认
- `< 0.6`: 报错，让用户重述

**对比旧版**: 旧版 Edit 工具是纯字符串替换，少一个空格就失败。新版用模糊匹配自动找最近的位置。

**关键文件**: `internal/tools/fuzzy.go`

### 10.12 代码库语义索引（v1.0+）

tree-sitter 符号切块 + embedding 向量检索 + BM25 关键词检索的混合融合。

**架构**:
```
代码文件
  → tree-sitter 提取符号（函数/类/方法）
  → 按符号边界切块
  → embedding 向量（语义相似）+ BM25 索引（关键词匹配）
  → 混合检索：向量 0.7 + BM25 0.3 加权融合
```

**特性**:
- 增量更新：文件 modtime 变化时只重算该文件
- JSON 持久化：零外部依赖（无 CGO/sqlite-vec）
- 检索延迟 < 50ms（纯本地余弦相似度）

**支持的 Embedding Provider**:
| Provider | 模型 | 维度 | 适用 |
|----------|------|------|------|
| OpenAI | text-embedding-3-small | 1536 | 海外 |
| 智谱（Zhipu）| embedding-3 | 2048 | 国内推荐 |
| 通义（DashScope）| text-embedding-v3 | 1024 | 阿里云 |
| Mock | — | 32 | 测试 |

```bash
# 建索引
/semantic index

# 语义检索
/semantic query "用户认证逻辑"

# 查看统计
/semantic stats
```

**关键文件**: `internal/semantic/`（index.go / chunker.go / embedding.go / vector_store.go / bm25.go / retriever.go）

### 10.13 学习型模型路由（v1.0+）

L1 特征分类 + L2 Wilson 置信区间学习路由器。

**L1 特征分类**（即时）:
- 简单问答 → 轻量模型（省钱）
- 代码编辑 / 重构 / 调试 → 主力模型（保质量）
- 多文件任务 → 主力模型 + 子 Agent

**L2 学习型路由器**（自适应）:
- Wilson 置信区间评估每个模型的历史成功率
- epsilon-greedy 探索：90% 用当前最优，10% 探索新选择
- 样本足够多时自动收敛到全局最优

```bash
# 查看路由统计
/route stats

# 清空学习数据重来
/route reset
```

**关键文件**: `internal/routing/learning_router.go`

### 10.14 基准测试框架（v1.0+）

15 个内置任务（5 类型 × 3 难度）量化评估配置表现。

**任务类型**: question / edit / refactor / debug / multi-file
**难度**: easy / medium / hard
**指标**: 成功率 / 延迟 / token / 成本 / 工具调用数

```bash
# 跑全部 15 个任务
/benchmark run

# 查看任务列表
/benchmark list
```

**Mock Runner**: 支持 CI 无网络测试，用关键词匹配模拟 LLM 响应。

**关键文件**: `internal/benchmark/`（benchmark.go / tasks.go / mock_runner.go）

---

## 附录

### A. 支持的模型

#### Anthropic

| 模型 | 上下文 | 最大输出 | 视觉 | 输入价格 | 输出价格 |
|------|--------|----------|------|----------|----------|
| `claude-sonnet-4-20250514` | 200K | 64K | ✅ | $0.003/1K | $0.015/1K |
| `claude-opus-4-20250514` | 200K | 32K | ✅ | $0.015/1K | $0.075/1K |
| `claude-haiku-3-5-20241022` | 200K | 8K | ✅ | $0.001/1K | $0.005/1K |

#### OpenAI

| 模型 | 上下文 | 最大输出 | 视觉 | 输入价格 | 输出价格 |
|------|--------|----------|------|----------|----------|
| `gpt-4o` | 128K | 16K | ✅ | $0.005/1K | $0.015/1K |
| `gpt-4o-mini` | 128K | 16K | ✅ | $0.00015/1K | $0.0006/1K |
| `o3` | 200K | 100K | ❌ | $0.002/1K | $0.008/1K |

#### Google

| 模型 | 上下文 | 最大输出 | 视觉 | 输入价格 | 输出价格 |
|------|--------|----------|------|----------|----------|
| `gemini-2.5-pro` | 1M | 65K | ✅ | $0.00125/1K | $0.005/1K |
| `gemini-2.5-flash` | 1M | 65K | ✅ | $0.00015/1K | $0.0006/1K |

#### DeepSeek

| 模型 | 上下文 | 最大输出 | 视觉 | 输入价格 | 输出价格 |
|------|--------|----------|------|----------|----------|
| `deepseek-chat` | 64K | 8K | ❌ | $0.00014/1K | $0.00028/1K |
| `deepseek-reasoner` | 64K | 8K | ❌ | $0.00055/1K | $0.00219/1K |

#### 通义千问 (Qwen)

| 模型 | 上下文 | 最大输出 | 视觉 | 输入价格 | 输出价格 |
|------|--------|----------|------|----------|----------|
| `qwen-max` | 32K | 8K | ❌ | $0.002/1K | $0.006/1K |
| `qwen-plus` | 131K | 8K | ❌ | $0.0004/1K | $0.0012/1K |

#### 智谱 GLM

| 模型 | 上下文 | 最大输出 | 视觉 | 输入价格 | 输出价格 |
|------|--------|----------|------|----------|----------|
| `glm-4-plus` | 128K | 4K | ✅ | $0.05/1K | $0.05/1K |

#### Ollama (本地)

支持所有 Ollama 兼容模型，默认连接 `http://localhost:11434`。

### B. 命令行 Flags

| Flag | 短写 | 默认值 | 说明 |
|------|------|--------|------|
| `--model` | `-m` | `gpt-4o` | 使用的 AI 模型 |
| `--provider` | `-p` | `openai` | LLM Provider |
| `--api-key` | `-k` | — | API Key |
| `--base-url` | `-u` | — | API Base URL |
| `--config` | — | — | 配置文件路径 |
| `--permission-mode` | — | `auto-edit` | 权限审批模式 |
| `--scope` | — | `[]` | 文件访问范围（可多次指定） |
| `--safe` | — | `false` | 安全模式 |
| `--continue` | `-c` | `false` | 继续最近的会话 |
| `--resume` | `-r` | — | 恢复指定会话 ID |
| `--auto-compact` | — | `false` | 自动压缩上下文 |
| `--auto-compact-ratio` | — | `0.8` | 自动压缩触发比例 |
| `--auto-checkpoint` | — | `false` | 自动 Git 检查点 |
| `--auto-stash` | — | `true` | 使用 git stash |
| `--daily-budget` | — | `0` | 每日预算上限（USD） |
| `--session-budget` | — | `0` | 每会话预算上限（USD） |
| `--log-level` | — | `info` | 日志级别 |
| `--log-format` | — | `text` | 日志格式 |
| `--prompt-variant` | — | `default` | 系统提示词变体 |
| `--prompt-strategy` | — | `fixed` | 变体选择策略 |
| `--prompt-weight` | — | — | 加权选择权重 |
| `--prompt-project-dir` | — | `.codecast/prompts` | 项目级 prompts 目录 |
| `--tui` | — | `false` | 启用 Bubble Tea TUI 界面 |

### C. 贡献指南

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing-feature'`)
4. 推送分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

### D. License

MIT

---

**文档版本**: v0.4.0  
**最后更新**: 2026-06-16  
**Go 版本**: 1.26+
