# 贡献指南

感谢你对 CodecastCLI 的关注！我们欢迎各种形式的贡献。

## 开发环境

1. **Go 1.26+**（必须）
2. 克隆仓库：
   ```bash
   git clone https://github.com/codecast/codecastcli.git
   cd codecastcli
   ```
3. 安装依赖：
   ```bash
   go mod tidy
   ```
4. 确保全量测试通过：
   ```bash
   go test ./... -count=1
   ```
5. 构建：
   ```bash
   go build -o codecast .
   ```

## 项目结构

```
codecastcli/
├── cmd/                    # CLI 命令定义（cobra + 交互式 REPL）
│   ├── interactive.go      # 核心 REPL 循环
│   ├── interactive_*.go    # 按职责拆分的命令处理器
│   └── commands.go         # 斜杠命令注册
├── internal/
│   ├── agent/              # Agent 核心（工厂、流式处理、hooks）
│   ├── errors/             # 错误码体系（UserFacingError + 降级矩阵）
│   ├── promptab/           # 提示词 A/B 框架（17 变体）
│   ├── provider/           # LLM Provider 工厂
│   ├── config/             # 配置管理
│   ├── tui/                # Bubble Tea TUI 渲染
│   ├── subagent/           # 隔离子 Agent 编排
│   ├── budget/             # 预算控制
│   └── ...                 # 更多内部包
├── docs/                   # 文档与计划
└── .github/workflows/      # CI/CD
```

## 代码规范

- 遵循 [Effective Go](https://go.dev/doc/effective_go) 和 [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- 新公开函数必须有文档注释（`// FuncName ...`）
- 公开 API 必须配套 `_test.go` 测试文件
- 单个文件不超过 **800 行**，超过应拆分
- commit message 遵循 [Conventional Commits](https://www.conventionalcommits.org/)：
  - `feat:` 新功能
  - `fix:` Bug 修复
  - `refactor:` 重构（不改变行为）
  - `docs:` 文档
  - `test:` 测试
  - `chore:` 构建 / CI / 工具链

## PR 流程

1. **Fork** 本仓库并创建 feature branch：
   ```bash
   git checkout -b feat/my-feature
   ```
2. 提交代码，确保：
   - `go build ./...` 零错误
   - `go test ./... -count=1` 全部通过
   - `go vet ./...` 无警告
3. 推送并创建 Pull Request，描述：
   - **做了什么**（变更概述）
   - **为什么**（解决的问题或需求）
   - **如何验证**（测试命令或复现步骤）
4. CI 必须全绿（test + lint + race）
5. 至少 **1 个 maintainer review** 通过
6. 使用 **Squash merge** 合并

## Issue 提交

- Bug 报告：使用 [Bug Report 模板](.github/ISSUE_TEMPLATE/bug_report.md)
- 功能建议：使用 [Feature Request 模板](.github/ISSUE_TEMPLATE/feature_request.md)
- 其他：直接创建空白 Issue

## 行为准则

- 尊重所有贡献者和用户
- 技术讨论对事不对人
- 提供建设性反馈

## 联系方式

- GitHub Issues / Discussions
- 在交互模式输入 `/feedback <message>` 提交反馈
