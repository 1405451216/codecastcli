---
name: explain
description: 解释代码逻辑
mode: ask
audience: intermediate
depth: medium
format: prose-with-code
arguments:
  - name: target
    required: true
    description: 文件路径、函数名或代码片段
example: /explain src/agent.go
---

请向 audience={{audience | "intermediate"}} 的开发者解释 {{target}}。

要求：
- 解释粒度：{{depth | "medium"}}（brief=1-2 句话，medium=逐段解释，detailed=逐行+设计意图）
- 重点：核心思路、关键设计决策、潜在陷阱
- 输出格式：{{format | "prose-with-code"}}（prose-with-code=散文+关键代码，code-only=只代码注释，bullets=要点列表）
- 不要复读代码本身；从"为什么这么写"切入

用户输入：$ARGUMENTS
