# CodecastCLI - AI 驱动的终端编程助手

![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)
![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)
![Version](https://img.shields.io/badge/Version-0.1.0-blue?style=flat-square)

基于 [AgentPrimordia](https://github.com/agentprimordia) 框架的 AI 终端 Agent 工具，支持多 LLM Provider、智能代码理解、三级权限模型、Plan+Execute 双 Agent 协作等企业级特性。

## 核心特性

- 🌐 **13+ LLM Provider 支持** — OpenAI / Anthropic / Gemini / Ollama / DeepSeek / Qwen / GLM / Azure / Cohere / Mistral 等
- 🧠 **智能代码理解** — 自动索引代码库结构，注入系统提示词，上下文感知对话
- 🔐 **三级权限模型** — `suggest`（建议）/ `auto-edit`（自动编辑）/ `full-auto`（全自动），精细控制工具调用审批
- 📺 **流式 Markdown 实时渲染** — 基于 Glamour 的高质量终端 Markdown 渲染
- 🤝 **Plan+Execute 双 Agent 协作** — 规划 Agent 制定方案，执行 Agent 落地实现
- 🔄 **自动 Git Checkpoint + Undo 回滚** — 文件修改前自动创建检查点，支持一键撤销
- 🔌 **MCP 协议扩展** — 10+ 内置模板（filesystem / github / postgres / puppeteer 等）
- 💰 **成本预算控制** — 每日/每会话预算上限，Token 用量追踪，超限自动中断
- 🏖️ **沙箱隔离执行** — 危险命令在沙箱中运行，保障系统安全
- 🖼️ **多模态图片分析** — 支持截图分析、本地图片理解
- 🧩 **插件生态系统** — 可扩展的插件注册与管理机制

## 快速安装

```bash
# go install
go install codecast/cli@latest

# 从源码
git clone https://github.com/codecast/codecastcli.git
cd codecastcli && go build -o codecast .
```

## 快速开始

```bash
# 启动交互模式
codecast

# 在交互模式中使用 /config 子命令管理配置
/config wizard
/config set api_key sk-xxx
/config set provider openai
/config set model gpt-4o

# 或直接通过环境变量
export CODECAST_API_KEY=sk-xxx
export CODECAST_PROVIDER=openai
export CODECAST_MODEL=gpt-4o

# Headless 模式
codecast exec "解释这个项目的架构"
```

> **v0.2.0 变更**：`codecast config` 子命令已迁移到交互模式内的 `/config` 斜杠命令。
> 在 shell 中运行 `codecast config` 会显示迁移提示。

## 交互命令列表

| 命令 | 说明 |
|------|------|
| `/help` | 显示帮助 |
| `/quit` | 退出 |
| `/clear` | 清屏 |
| `/compact` | 压缩上下文 |
| `/config [list\|get\|set\|wizard]` | 配置管理（v0.2.0+ 替代 `codecast config`） |
| `/cost [summary\|daily\|list\|clear]` | 成本管理 |
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
| `/plugins` | 插件管理（别名） |
| `/index` | 重建索引 |
| `/mode` | 切换权限模式 |
| `/screenshot` | 截图分析 |

## 配置项

配置文件位于 `~/.codecast/config.yaml`，支持环境变量覆盖（前缀 `CODECAST_`）。

| 配置键 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `api_key` | string | — | LLM API Key |
| `model` | string | `gpt-4o` | 使用的模型 |
| `provider` | string | `openai` | LLM Provider |
| `base_url` | string | — | 自定义 API Base URL |
| `permission_mode` | string | `suggest` | 权限审批模式 |
| `safe_mode` | bool | `false` | 安全模式（禁用 Shell/Web） |
| `auto_compact` | bool | `false` | 自动压缩上下文 |
| `auto_compact_ratio` | float | `0.8` | 自动压缩触发比例 |
| `auto_checkpoint` | bool | `false` | 自动 Git 检查点 |
| `auto_stash` | bool | `true` | 使用 git stash 代替 commit |
| `daily_budget_usd` | float | `0` | 每日预算上限（USD） |
| `session_budget_usd` | float | `0` | 每会话预算上限（USD） |
| `daily_token_limit` | int | `0` | 每日 Token 上限 |
| `session_token_limit` | int | `0` | 每会话 Token 上限 |

## 命令行 Flags

| Flag | 短写 | 默认值 | 说明 |
|------|------|--------|------|
| `--model` | `-m` | `gpt-4o` | 使用的 AI 模型 |
| `--provider` | `-p` | `openai` | LLM Provider |
| `--api-key` | `-k` | — | API Key |
| `--base-url` | `-u` | — | API Base URL |
| `--config` | — | — | 配置文件路径 |
| `--permission-mode` | — | `suggest` | 权限审批模式 |
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
| `--prompt-variant` | — | `default` | 系统提示词变体（`default` / `concise` / `safety-first` / 用户自定义） |
| `--prompt-strategy` | — | `fixed` | 变体选择策略（`fixed` / `round-robin` / `weighted`） |
| `--prompt-weight` | — | — | 加权选择权重，可多次指定：`--prompt-weight default=5 --prompt-weight concise=1` |
| `--prompt-project-dir` | — | `.codecast/prompts` | 项目级 prompts 目录 |

## 提示词 A/B 框架

Codecast 内置提示词 **A/B 测试与外部化框架**，可热替换系统提示词，无需重编译。

### 十一种内置变体

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

### 使用方式

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

### 自定义变体

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

## MCP 服务器模板

内置 10 个 MCP 服务器模板，可通过 `codecast mcp` 一键启用：

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

## 支持的模型

### Anthropic

| 模型 | 上下文 | 最大输出 | 视觉 | 输入价格 | 输出价格 |
|------|--------|----------|------|----------|----------|
| `claude-sonnet-4-20250514` | 200K | 64K | ✅ | $0.003/1K | $0.015/1K |
| `claude-opus-4-20250514` | 200K | 32K | ✅ | $0.015/1K | $0.075/1K |
| `claude-haiku-3-5-20241022` | 200K | 8K | ✅ | $0.001/1K | $0.005/1K |

### OpenAI

| 模型 | 上下文 | 最大输出 | 视觉 | 输入价格 | 输出价格 |
|------|--------|----------|------|----------|----------|
| `gpt-4o` | 128K | 16K | ✅ | $0.005/1K | $0.015/1K |
| `gpt-4o-mini` | 128K | 16K | ✅ | $0.00015/1K | $0.0006/1K |
| `o3` | 200K | 100K | ❌ | $0.002/1K | $0.008/1K |

### Google

| 模型 | 上下文 | 最大输出 | 视觉 | 输入价格 | 输出价格 |
|------|--------|----------|------|----------|----------|
| `gemini-2.5-pro` | 1M | 65K | ✅ | $0.00125/1K | $0.005/1K |
| `gemini-2.5-flash` | 1M | 65K | ✅ | $0.00015/1K | $0.0006/1K |

### DeepSeek

| 模型 | 上下文 | 最大输出 | 视觉 | 输入价格 | 输出价格 |
|------|--------|----------|------|----------|----------|
| `deepseek-chat` | 64K | 8K | ❌ | $0.00014/1K | $0.00028/1K |
| `deepseek-reasoner` | 64K | 8K | ❌ | $0.00055/1K | $0.00219/1K |

### 通义千问 (Qwen)

| 模型 | 上下文 | 最大输出 | 视觉 | 输入价格 | 输出价格 |
|------|--------|----------|------|----------|----------|
| `qwen-max` | 32K | 8K | ❌ | $0.002/1K | $0.006/1K |
| `qwen-plus` | 131K | 8K | ❌ | $0.0004/1K | $0.0012/1K |

### 智谱 GLM

| 模型 | 上下文 | 最大输出 | 视觉 | 输入价格 | 输出价格 |
|------|--------|----------|------|----------|----------|
| `glm-4-plus` | 128K | 4K | ✅ | $0.05/1K | $0.05/1K |

### Ollama (本地)

支持所有 Ollama 兼容模型，默认连接 `http://localhost:11434`。

## 项目结构

```
codecastcli/
├── main.go                     # 入口
├── cmd/                        # Cobra 命令定义
│   ├── root.go                 # 根命令 & 全局 Flags
│   ├── interactive.go          # 交互式 REPL
│   ├── config.go               # 配置管理
│   ├── exec.go                 # Headless 模式
│   ├── mcp.go                  # MCP 子命令
│   ├── plugin.go               # 插件子命令
│   ├── sandbox.go              # 沙箱子命令
│   ├── session.go              # 会话管理
│   ├── cost.go                 # 成本查询
│   ├── rag.go                  # RAG 索引
│   ├── pool.go                 # Agent Pool
│   ├── workflow.go             # 工作流
│   ├── chat.go                 # 聊天模式
│   └── init.go                 # 项目初始化
├── internal/                   # 核心实现
│   ├── agent/                  # Agent 主循环 & 流式处理
│   ├── budget/                 # 预算控制器
│   ├── checkpoint/             # Git 检查点管理
│   ├── config/                 # 配置加载与持久化
│   ├── context/                # 上下文压缩
│   ├── cost/                   # Token 成本追踪
│   ├── diff/                   # Diff 预览
│   ├── hooks/                  # 钩子管理器
│   ├── indexer/                # 代码库索引器
│   ├── mcp/                    # MCP 模板定义
│   ├── mcpcfg/                 # MCP 配置管理
│   ├── memory/                 # 自动记忆持久化
│   ├── model/                  # 模型切换器
│   ├── permission/             # 三级权限模型
│   ├── plugin/                 # 插件注册与管理
│   ├── pool/                   # Agent Pool 管理
│   ├── profile/                # Profile 管理
│   ├── provider/               # LLM Provider 工厂
│   ├── rules/                  # 项目规则加载
│   ├── sandbox/                # 沙箱执行器
│   ├── session/                # 会话存储
│   ├── subagent/               # 双 Agent 编排器
│   ├── tools/                  # 内置工具（edit/glob/grep）
│   ├── tui/                    # TUI 渲染器
│   ├── ui/                     # UI 辅助（Markdown/Spinner）
│   ├── undo/                   # 撤销管理器
│   ├── vision/                 # 图片分析 & 截图
│   └── wizard/                 # 交互式配置向导
├── completions/                # Shell 补全脚本
│   ├── codecast.bash
│   ├── codecast.zsh
│   ├── codecast.fish
│   └── codecast.ps1
└── docs/                       # 文档
```

## 开发

```bash
# 运行测试
go test ./...

# 构建
go build -o codecast .

# 生成 man page
./codecast man --format markdown
```

### 贡献指南

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

## License

MIT
