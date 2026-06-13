---
name: review
description: 代码审查指定文件或目录
mode: build
audience: senior
depth: detailed
format: structured-report
arguments:
  - name: target
    required: true
    description: 文件路径或 glob 模式（如 internal/agent/*.go）
  - name: focus
    required: false
    description: 可选 focus=security|performance|maintainability|all
example: /review internal/agent/agent.go focus=security
---

对 {{target}} 进行代码审查，目标读者是 audience={{audience | "senior"}} 工程师。

Focus: {{focus | "all"}}
解释粒度: {{depth | "detailed"}}
输出格式: {{format | "structured-report"}}

审查维度（按 focus 调整）：
- **正确性**：边界条件、错误处理、并发安全、资源泄漏
- **安全性**：输入校验、注入风险、权限提升、敏感信息泄露
- **性能**：时间/空间复杂度、I/O 热点、可缓存性
- **可维护性**：命名、抽象层次、测试覆盖、文档

输出结构（structured-report）：
1. 总体评价（1-2 句话）
2. 关键问题（必须修，1-3 条）
3. 改进建议（建议修，3-5 条）
4. 亮点（值得保持，1-3 条）
5. 验证建议：具体的测试 / 编译 / lint 命令

严格区分"事实"与"建议"；每条问题给出 file:line 引用。

用户输入：$ARGUMENTS
