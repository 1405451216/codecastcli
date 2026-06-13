---
name: test
description: 为指定代码生成单元测试
mode: build
audience: intermediate
depth: detailed
format: code-only
arguments:
  - name: target
    required: true
    description: 文件路径或函数名
  - name: framework
    required: false
    description: 可选 framework=go-test|pytest|jest|vitest
example: /test internal/agent/agent.go::buildSystemPrompt framework=go-test
---

为 {{target}} 生成单元测试。

目标读者: {{audience | "intermediate"}}
解释粒度: {{depth | "detailed"}}
输出格式: {{format | "code-only"}}
测试框架: {{framework | "auto-detect"}}

要求：
1. 先读再写：必须先用 read_file 读取 {{target}}，理解现有结构
2. 测试命名遵循项目惯例（Go: TestXxx，Pytest: test_xxx，Jest: xxx.test）
3. 覆盖优先级：
   - 正常路径（happy path）
   - 边界条件（空、零、最大、单元素）
   - 错误路径（无效输入、权限拒绝、超时）
   - 状态变化（pure function 不写；stateful 函数必须测前后状态）
4. 每个测试独立：不依赖其他 test 的副作用
5. 使用表驱动（如适用）：Go 优先 t.Run 子测试，Python 优先 pytest.mark.parametrize
6. 不引入新依赖：用项目已有测试库
7. 断言信息明确：失败时能直接看出哪个 case 挂了

输出格式：仅输出测试代码 + 简短说明（< 50 字），不要长篇分析。

用户输入：$ARGUMENTS
