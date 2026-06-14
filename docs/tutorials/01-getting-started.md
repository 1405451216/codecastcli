# 5 分钟上手 CodecastCLI

## 安装

选择一种安装方式：

```bash
# Go install（需要 Go 1.26+）
go install github.com/codecast/codecastcli@latest

# macOS / Linux (Homebrew)
brew install --cask codecast/codecast/codecast

# Windows (Scoop)
scoop bucket add codecast https://github.com/codecast/scoop-bucket
scoop install codecast
```

## 配置

首次运行会启动交互式配置向导：

```bash
codecast
```

按提示输入：
1. **Provider**：选择 `openai`、`anthropic`、`gemini` 等
2. **API Key**：粘贴你的 API Key
3. **Model**：选择模型（如 `gpt-4o`、`claude-3.5-sonnet`）

配置保存在 `~/.codecast/config.yaml`。

### 环境变量（可选）

```bash
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
```

## 第一次对话

启动后直接输入自然语言：

```
> 解释这个项目的架构

AI 会自动：
1. 索引项目文件树
2. 分析代码结构
3. 给出架构概述
```

### @file 引用

将特定文件注入上下文：

```
> 看看 @src/main.go 有什么问题
```

### 斜杠命令

```
> /help              # 查看所有命令
> /model             # 切换模型
> /cost summary      # 查看本次会话费用
> /config list       # 查看配置
```

## 编辑代码

让 AI 修改代码：

```
> 在 auth 模块中添加一个 rate limiter

AI 会：
1. 定位相关文件
2. 生成修改方案
3. 在 suggest 模式下请求确认
4. 应用修改
```

### 权限模式

| 模式 | 说明 | 适合场景 |
|------|------|---------|
| `suggest` | 只建议不修改 | 学习、审查 |
| `auto-edit` | 自动修改，每次确认 | 日常开发 |
| `full-auto` | 完全自动（含 shell） | 批量任务 |

切换模式：

```
> /mode auto-edit
```

## 撤销修改

每次修改前会自动创建 checkpoint：

```
> /undo              # 撤销上一步修改
> /undo list         # 查看可撤销的历史
```

## 下一步

- [编辑代码详解](02-editing-code.md)
- [工具全解](03-using-tools.md)
- [成本优化](06-cost-optimization.md)
