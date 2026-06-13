# MCP SSE 传输 + 自定义 Slash Commands 设计文档

## 目标

1. 为 MCPClient 添加 SSE (Server-Sent Events) 传输支持，可连接远程 MCP 服务器
2. 添加 MCP Resource 支持（listResources + readResource）
3. 支持用户在 `.codecast/commands/` 目录下创建自定义 Slash Commands

---

## 一、MCP SSE 传输

### 现状

`MCPClient`（`src/mcp/client.ts`）仅支持 stdio 传输（spawn 子进程）。`MCPServerConfig.transport` 类型为 `'stdio'`。

### 配置格式

```json
{
  "mcpServers": {
    "local-fs": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
      "transport": "stdio"
    },
    "remote-api": {
      "url": "http://localhost:3000/sse",
      "transport": "sse",
      "headers": { "Authorization": "Bearer xxx" }
    }
  }
}
```

### 类型扩展

```typescript
export interface MCPServerConfig {
  name: string
  /** stdio 传输：启动命令 */
  command?: string
  args?: string[]
  env?: Record<string, string>
  /** SSE 传输：服务器 URL */
  url?: string
  /** SSE 传输：自定义请求头 */
  headers?: Record<string, string>
  /** 传输方式 */
  transport: 'stdio' | 'sse'
}
```

### SSE 传输实现

SSE 传输使用 HTTP 长连接：
- **接收消息**：GET `/sse` 建立 EventSource 连接，监听 `message` 事件
- **发送请求**：POST `/messages` 发送 JSON-RPC 请求

```typescript
// MCPClient 新增方法
private async connectSSE(name: string): Promise<void> {
  const server = this.servers.get(name)
  if (!server) throw new Error(`MCP 服务器 "${name}" 未注册`)

  const { config } = server
  const url = config.url!
  const messagesUrl = url.replace('/sse', '/messages')

  // 1. 建立 SSE 连接
  const eventSource = new EventSource(url, {
    headers: config.headers,
  })

  // 2. 监听消息
  eventSource.onmessage = (event) => {
    this.handleSSEMessage(name, event.data)
  }

  // 3. 存储 SSE 连接信息
  server.process = null  // SSE 没有子进程
  server.sseConnection = { eventSource, messagesUrl }

  // 4. 发送 initialize + tools/list（与 stdio 相同的协议）
  const initResult = await this.sendSSERequest(name, 'initialize', {
    protocolVersion: '2024-11-05',
    capabilities: {},
    clientInfo: { name: 'codecast-agent', version: '0.1.0' },
  })

  this.sendSSENotification(name, 'notifications/initialized', {})

  const toolsResult = await this.sendSSERequest(name, 'tools/list', {}) as { tools: MCPTool[] }
  server.tools = toolsResult.tools || []
}
```

### SSE 请求发送

```typescript
private async sendSSERequest(serverName: string, method: string, params: Record<string, unknown>): Promise<unknown> {
  const server = this.servers.get(serverName)
  if (!server?.sseConnection) {
    return Promise.reject(new Error(`MCP SSE 服务器 "${serverName}" 未连接`))
  }

  const id = ++this.requestId
  const request: JsonRpcRequest = { jsonrpc: '2.0', id, method, params }

  return new Promise((resolve, reject) => {
    this.pendingRequests.set(id, { resolve, reject })

    const url = server.sseConnection.messagesUrl
    fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...server.config.headers,
      },
      body: JSON.stringify(request),
    }).catch(err => {
      this.pendingRequests.delete(id)
      reject(new Error(`SSE 请求发送失败: ${err.message}`))
    })

    // 30 秒超时
    setTimeout(() => {
      if (this.pendingRequests.has(id)) {
        this.pendingRequests.delete(id)
        reject(new Error(`SSE 请求超时: ${method}`))
      }
    }, 30000)
  })
}
```

### SSE 消息接收

```typescript
private handleSSEMessage(serverName: string, data: string) {
  try {
    const response: JsonRpcResponse = JSON.parse(data)
    if (response.id !== undefined && this.pendingRequests.has(response.id)) {
      const pending = this.pendingRequests.get(response.id)!
      this.pendingRequests.delete(response.id)
      if (response.error) {
        pending.reject(new Error(`MCP 错误 [${response.error.code}]: ${response.error.message}`))
      } else {
        pending.resolve(response.result)
      }
    }
  } catch {
    // 忽略非 JSON 消息
  }
}
```

### 服务器内部数据扩展

```typescript
// servers Map 的 value 类型扩展
interface ServerEntry {
  config: MCPServerConfig
  process: ChildProcess | null          // stdio 连接
  sseConnection: {                      // SSE 连接
    eventSource: EventSource
    messagesUrl: string
  } | null
  tools: MCPTool[]
  resources: MCPResource[]              // 新增：资源列表
}
```

### connectAll 改造

```typescript
async connectAll(): Promise<{ connected: string[]; failed: Array<{ name: string; error: string }> }> {
  const connected: string[] = []
  const failed: Array<{ name: string; error: string }> = []

  for (const [name, server] of this.servers) {
    try {
      if (server.config.transport === 'sse') {
        await this.connectSSE(name)
      } else {
        await this.connectServer(name)
      }
      connected.push(name)
    } catch (err: any) {
      failed.push({ name, error: err.message })
    }
  }

  return { connected, failed }
}
```

### disconnectAll 改造

```typescript
disconnectAll() {
  for (const [name, server] of this.servers) {
    // 关闭 stdio 进程
    if (server.process) {
      try { server.process.kill() } catch { /* ignore */ }
      server.process = null
    }
    // 关闭 SSE 连接
    if (server.sseConnection) {
      server.sseConnection.eventSource.close()
      server.sseConnection = null
    }
    this.buffers.delete(name)
  }
  // 清理 pending
  for (const [, pending] of this.pendingRequests) {
    pending.reject(new Error('MCP 客户端断开连接'))
  }
  this.pendingRequests.clear()
}
```

### callTool 改造

```typescript
async callTool(serverName: string, toolName: string, args: Record<string, unknown>): Promise<unknown> {
  const server = this.servers.get(serverName)
  if (!server) throw new Error(`MCP 服务器 "${serverName}" 未注册`)

  if (server.config.transport === 'sse' && server.sseConnection) {
    return this.sendSSERequest(serverName, 'tools/call', { name: toolName, arguments: args })
  }

  if (server.process) {
    return this.sendRequest(serverName, 'tools/call', { name: toolName, arguments: args })
  }

  throw new Error(`MCP 服务器 "${serverName}" 未连接`)
}
```

---

## 二、MCP Resource 支持

### 新增方法

```typescript
/** 列出服务器的资源 */
async listResources(serverName: string): Promise<MCPResource[]> {
  const server = this.servers.get(serverName)
  if (!server) throw new Error(`MCP 服务器 "${serverName}" 未注册`)

  const result = await this.sendRequestByTransport(serverName, 'resources/list', {}) as { resources: MCPResource[] }
  server.resources = result.resources || []
  return server.resources
}

/** 读取资源内容 */
async readResource(serverName: string, uri: string): Promise<string> {
  const result = await this.sendRequestByTransport(serverName, 'resources/read', { uri }) as { contents: Array<{ uri: string; text?: string; blob?: string }> }
  if (result.contents && result.contents.length > 0) {
    return result.contents[0].text || result.contents[0].blob || ''
  }
  return ''
}

/** 获取所有已连接服务器的资源列表 */
getAllResources(): Array<MCPResource & { server: string }> {
  const result: Array<MCPResource & { server: string }> = []
  for (const [name, server] of this.servers) {
    for (const resource of server.resources) {
      result.push({ ...resource, server: name })
    }
  }
  return result
}
```

### 统一请求发送方法

```typescript
/** 根据传输方式自动选择发送方法 */
private sendRequestByTransport(serverName: string, method: string, params: Record<string, unknown>): Promise<unknown> {
  const server = this.servers.get(serverName)
  if (!server) return Promise.reject(new Error(`MCP 服务器 "${serverName}" 未注册`))

  if (server.config.transport === 'sse' && server.sseConnection) {
    return this.sendSSERequest(serverName, method, params)
  }

  if (server.process) {
    return this.sendRequest(serverName, method, params)
  }

  return Promise.reject(new Error(`MCP 服务器 "${serverName}" 未连接`))
}
```

### connectServer 中增加 Resource 发现

在 `connectServer()` 和 `connectSSE()` 的末尾，initialize + tools/list 之后，追加 resources/list：

```typescript
// 获取资源列表（可选，失败不阻塞）
try {
  const resourcesResult = await this.sendRequestByTransport(name, 'resources/list', {}) as { resources: MCPResource[] }
  server.resources = resourcesResult.resources || []
} catch {
  server.resources = []  // 服务器不支持 Resource 也不报错
}
```

---

## 三、自定义 Slash Commands

### 命令文件格式

在 `.codecast/commands/` 目录下创建 `.md` 文件：

```markdown
---
name: review
description: 代码审查当前文件
mode: build
---

请对以下文件进行代码审查，关注安全性、性能和可维护性：
$ARGUMENTS
```

### 类型定义

```typescript
export interface CustomCommand {
  /** 命令名（不含 / 前缀） */
  name: string
  /** 命令描述（显示在 /help 中） */
  description: string
  /** 执行时切换的模式（可选） */
  mode?: string
  /** 命令模板内容（$ARGUMENTS 为用户参数占位符） */
  template: string
}
```

### 加载逻辑

```typescript
// src/commands/loader.ts
import { readFileSync, readdirSync, existsSync } from 'fs'
import { join } from 'path'

export function loadCustomCommands(commandsDir: string): CustomCommand[] {
  if (!existsSync(commandsDir)) return []

  const commands: CustomCommand[] = []

  for (const file of readdirSync(commandsDir)) {
    if (!file.endsWith('.md')) continue

    const filePath = join(commandsDir, file)
    const content = readFileSync(filePath, 'utf-8')

    // 解析 YAML frontmatter
    const match = content.match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/)
    if (!match) continue

    try {
      const frontmatter = parseSimpleYAML(match[1])
      const template = match[2].trim()

      commands.push({
        name: frontmatter.name || file.replace('.md', ''),
        description: frontmatter.description || '',
        mode: frontmatter.mode,
        template,
      })
    } catch {
      // 解析失败跳过
    }
  }

  return commands
}

/** 简易 YAML 解析（仅支持 key: value 格式） */
function parseSimpleYAML(yaml: string): Record<string, string> {
  const result: Record<string, string> = {}
  for (const line of yaml.split('\n')) {
    const match = line.match(/^(\w+):\s*(.+)$/)
    if (match) {
      result[match[1]] = match[2].trim().replace(/^['"]|['"]$/g, '')
    }
  }
  return result
}
```

### 执行逻辑

在 `handleCommand()` 中，switch-case 的 default 分支检查自定义命令：

```typescript
default: {
  // 检查自定义命令
  const cmd = this.customCommands.find(c => c.name === name)
  if (cmd) {
    const prompt = cmd.template.replace('$ARGUMENTS', args.join(' '))
    if (cmd.mode) this.modeManager.setMode(cmd.mode)
    // 发送给 AI
    await this.processUserInput(prompt)
  } else {
    console.log(`未知命令: /${name}，输入 /help 查看可用命令`)
  }
  break
}
```

### /help 集成

在 `/help` 输出中追加自定义命令列表：

```typescript
if (this.customCommands.length > 0) {
  lines.push('')
  lines.push(`${ui.color.bold}自定义命令:${ui.color.reset}`)
  for (const cmd of this.customCommands) {
    lines.push(`  /${cmd.name.padEnd(16)} ${cmd.description}`)
  }
}
```

### 内置命令示例

在 `.codecast/commands/` 下提供几个内置示例：

**review.md**：
```markdown
---
name: review
description: 代码审查指定文件
mode: build
---

请对以下文件进行代码审查，关注安全性、性能和可维护性：
$ARGUMENTS
```

**explain.md**：
```markdown
---
name: explain
description: 解释代码逻辑
mode: ask
---

请解释以下代码的逻辑：
$ARGUMENTS
```

**test.md**：
```markdown
---
name: test
description: 为指定代码生成测试
mode: build
---

请为以下代码生成单元测试：
$ARGUMENTS
```

---

## 文件结构

| 文件 | 操作 | 说明 |
|---|---|---|
| `src/mcp/client.ts` | 修改 | SSE 传输 + Resource 支持 + 统一请求方法 |
| `src/commands/loader.ts` | 新增 | 自定义命令加载器 |
| `src/index.ts` | 修改 | 加载自定义命令 + /help 集成 + handleCommand 集成 |
| `.codecast/commands/review.md` | 新增 | 内置命令示例 |
| `.codecast/commands/explain.md` | 新增 | 内置命令示例 |
| `.codecast/commands/test.md` | 新增 | 内置命令示例 |
| `tests/mcp-sse.test.ts` | 新增 | SSE 传输测试 |
| `tests/custom-commands.test.ts` | 新增 | 自定义命令测试 |

## 验收标准

1. **SSE 传输**：可通过 `url` + `transport: 'sse'` 配置连接远程 MCP 服务器
2. **SSE 请求**：SSE 服务器可正常 initialize + tools/list + tools/call
3. **SSE 断开**：disconnectAll 正确关闭 EventSource 连接
4. **Resource 支持**：可 listResources + readResource
5. **Resource 容错**：服务器不支持 Resource 时不报错
6. **自定义命令加载**：`.codecast/commands/*.md` 自动加载
7. **自定义命令执行**：`/review src/utils.ts` 将模板 + 参数发送给 AI
8. **自定义命令 /help**：自定义命令显示在 /help 列表中
9. **向后兼容**：不写 mcp.json 或 commands 目录时，行为与当前完全一致

## 错误场景

| 场景 | 行为 |
|---|---|
| SSE URL 不可达 | connectAll 报错但不阻塞其他服务器 |
| SSE 连接中断 | warn + 标记服务器为断开 |
| 命令文件 YAML 解析失败 | 跳过该文件 + warn |
| 命令文件名与内置命令冲突 | 内置命令优先 + warn |
| $ARGUMENTS 未提供 | 替换为空字符串 |
| Resource 不支持 | 返回空数组，不报错 |
