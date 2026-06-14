# 自定义提示词

## 概述

Codecast 内置 17 个提示词变体，支持 4 种外部化方式和 3 种选择策略。

## 查看当前变体

```
> /prompt list         # 查看所有可用变体
> /prompt current      # 查看当前使用的变体
> /prompt show <name>  # 查看变体内容
```

## 切换变体

```
> /prompt use concise          # 切换到简洁模式
> /prompt use code-reviewer    # 切换到代码审查模式
> /prompt use lazy-mode        # 禁止 TODO 和伪代码
```

切换立即生效，无需重启。

## 内置变体一览

| 变体 | 适用场景 |
|------|---------|
| `default` | 通用开发 |
| `concise` | 简短回答 |
| `safety-first` | 安全敏感代码 |
| `claude-style` | 结构化推理 |
| `code-reviewer` | 代码审查 |
| `pair-programmer` | 结对编程 |
| `search-then-edit` | 两阶段工作流 |
| `format-locked` | 严格格式约束 |
| `architect-edit` | 设计+实现分离 |
| `shell-only` | 仅 Shell 操作 |
| `lazy-mode` | 禁止懒加载 |
| `overeager-mode` | 禁止顺手修改 |

## 自定义变体

在 `~/.codecast/prompts/` 或 `.codecast/prompts/` 下创建 YAML 文件：

```yaml
# ~/.codecast/prompts/my-style.yaml
name: my-style
description: 我的团队风格
author: team-lead
weight: 1.0

system: |
  你是一个专业的 {{language}} 开发者。
  
  规则：
  - 使用 {{indent_style}} 缩进
  - 注释使用 {{comment_lang}}
  - 每个函数必须有文档注释
  
  当前项目：{{project_name}}
  工作目录：{{cwd}}
```

### 可用变量

| 变量 | 说明 |
|------|------|
| `{{language}}` | 项目主语言 |
| `{{cwd}}` | 当前工作目录 |
| `{{os}}` | 操作系统 |
| `{{mode}}` | 权限模式 |
| `{{budget}}` | 剩余预算 |
| `{{project_name}}` | 项目名 |
| `{{file_tree}}` | 文件树 |
| `{{rules}}` | 项目规则 |

## 选择策略

```yaml
# ~/.codecast/config.yaml
prompt_variant: default
prompt_strategy: weighted        # fixed | round-robin | weighted
prompt_weights:
  default: 3
  code-reviewer: 2
  pair-programmer: 1
```

### 策略说明

- **fixed** — 固定使用指定变体
- **round-robin** — 按权重轮流切换
- **weighted** — 按权重随机选择

## A/B 测试

查看各变体的成本对比：

```
> /cost by-variant
```

输出示例：
```
变体成本对比（最近 30 天）:
  default:       $12.50 (120 次)  平均 $0.10/次
  code-reviewer: $8.20  (45 次)   平均 $0.18/次
  concise:       $3.10  (80 次)   平均 $0.04/次
```

## CLI 参数

```bash
codecast --prompt-variant concise
codecast --prompt-strategy weighted
codecast --prompt-weight default=3 --prompt-weight concise=1
codecast --prompt-project-dir ./my-prompts
```

## 下一步

- [成本优化](06-cost-optimization.md)
