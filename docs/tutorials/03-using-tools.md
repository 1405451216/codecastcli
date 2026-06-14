# 工具全解

## 内置工具

Codecast 为 AI 提供了一组内置工具：

| 工具 | 说明 | 示例指令 |
|------|------|---------|
| `read_file` | 读取文件内容 | "看看 main.go" |
| `edit_file` | 修改文件 | "修改 config.go 添加 timeout 字段" |
| `create_file` | 创建新文件 | "创建一个 utils.go" |
| `delete_file` | 删除文件 | "删掉废弃的 old_helper.go" |
| `list_dir` | 列出目录 | "看看 src 目录结构" |
| `grep_search` | 正则搜索 | "搜索所有用了 os.Exit 的地方" |
| `glob_search` | 文件模式匹配 | "找到所有 _test.go 文件" |
| `shell_exec` | 执行命令 | "运行测试" |

## LSP 集成

如果项目中有 LSP 服务器（gopls、pyright、tsserver），AI 可以使用：

- **goto_definition** — 跳转到函数/变量定义
- **find_references** — 查找所有引用
- **diagnostics** — 获取编译/lint 错误

LSP 会自动检测，无需配置。如果 LSP 不可用，自动回退到 tree-sitter + regex。

## MCP 工具

通过 MCP 协议接入外部工具：

```
> /mcp list            # 查看已连接的 MCP 服务器
> /mcp add             # 添加 MCP 服务器
> /mcp templates       # 查看 10 个内置模板
```

### 内置模板

| 模板 | 说明 |
|------|------|
| `filesystem` | 文件系统读写 |
| `github` | GitHub API（Issue、PR） |
| `postgres` | PostgreSQL 查询 |
| `sqlite` | SQLite 数据库 |
| `puppeteer` | 浏览器自动化 |
| `slack` | Slack 消息 |
| `jira` | JIRA 工单管理 |
| `docker` | Docker 容器管理 |
| `kubernetes` | K8s 集群操作 |
| `memory` | 持久化记忆 |

### 自定义 MCP

在 `~/.codecast/mcp.yaml` 中配置：

```yaml
servers:
  - name: my-api
    command: npx
    args: ["-y", "@my-org/mcp-server"]
    env:
      API_KEY: "xxx"
```

## 项目规则

AI 遵循三级规则体系：

1. **全局规则** — `~/.codecast/rules.yaml`
2. **项目规则** — `.codecast/rules.yaml`
3. **自动学习** — 从历史交互中提取

```
> /rules               # 查看当前规则
> /rules add "使用 TypeScript strict 模式"
```

## 下一步

- [双 Agent 协作](04-multi-agent.md)
- [自定义提示词](05-custom-prompts.md)
