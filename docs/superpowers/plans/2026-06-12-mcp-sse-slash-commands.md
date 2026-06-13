# MCP SSE + 自定义 Slash Commands 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 MCPClient 添加 SSE 传输和 Resource 支持，并实现自定义 Slash Commands 系统。

**Architecture:** MCPClient 扩展 `MCPServerConfig` 支持 `sse` 传输，新增 `connectSSE()` / `sendSSERequest()` / `handleSSEMessage()` 方法，统一 `sendRequestByTransport()` 方法。自定义命令通过 `.codecast/commands/*.md` 文件定义，启动时加载到 `CustomCommand[]` 数组。

**Tech Stack:** TypeScript, Node.js fetch API, EventSource (eventsource npm 包)

---

## 文件结构

| 文件 | 职责 |
|---|---|
| `src/mcp/client.ts` (修改) | SSE 传输 + Resource 支持 + 统一请求方法 |
| `src/commands/loader.ts` (新增) | 自定义命令加载器 + YAML 解析 |
| `src/index.ts` (修改) | 加载自定义命令 + /help + handleCommand 集成 |
| `.codecast/commands/review.md` (新增) | 内置命令示例 |
| `.codecast/commands/explain.md` (新增) | 内置命令示例 |
| `.codecast/commands/test.md` (新增) | 内置命令示例 |
| `tests/mcp-sse.test.ts` (新增) | SSE 传输测试 |
| `tests/custom-commands.test.ts` (新增) | 自定义命令测试 |

---

### Task 1: MCPServerConfig 类型扩展 + ServerEntry 重构

**Files:**
- Modify: `src/mcp/client.ts`

- [ ] **Step 1: 扩展 MCPServerConfig 接口**

将现有的 `MCPServerConfig` 接口替换为：

```typescript
export interface MCPServerConfig {
  /** 服务器唯一标识 */
  name: string
  /** stdio 传输：启动命令 */
  command?: string
  /** 命令参数 */
  args?: string[]
  /** 环境变量 */
  env?: Record<string, string>
  /** SSE 传输：服务器 URL */
  url?: string
  /** SSE 传输：自定义请求头 */
  headers?: Record<string, string>
  /** 传输方式 */
  transport: 'stdio' | 'sse'
}
```

- [ ] **Step 2: 新增 SSEConnection 接口和 ServerEntry 类型**

在 `MCPResource` 接口之后、`MCPClient` 类之前添加：

```typescript
/** SSE 连接信息 */
interface SSEConnection {
  eventSource: EventSource
  messagesUrl: string
}

/** 服务器内部条目 */
interface ServerEntry {
  config: MCPServerConfig
  process: ChildProcess | null
  sseConnection: SSEConnection | null
  tools: MCPTool[]
  resources: MCPResource[]
}
```

- [ ] **Step 3: 更新 servers Map 的类型**

将 `MCPClient` 类中的：

```typescript
private servers = new Map<string, { config: MCPServerConfig; process: ChildProcess | null; tools: MCPTool[] }>()
```

替换为：

```typescript
private servers = new Map<string, ServerEntry>()
```

- [ ] **Step 4: 更新 registerServer 方法**

```typescript
registerServer(config: MCPServerConfig) {
  this.servers.set(config.name, { config, process: null, sseConnection: null, tools: [], resources: [] })
}
```

- [ ] **Step 5: 类型检查**

```bash
npx tsc -p tsconfig.test.json --noEmit
```
Expected: 可能有类型错误（因为 ServerEntry 字段变了），后续 Task 修复

- [ ] **Step 6: Commit**

```bash
git add src/mcp/client.ts
git commit -m "refactor(mcp): extend MCPServerConfig for SSE and restructure ServerEntry"
```

---

### Task 2: SSE 传输实现

**Files:**
- Modify: `src/mcp/client.ts`

- [ ] **Step 1: 安装 eventsource 依赖**

```bash
cd e:\codecast\codecast\codecastcli
npm install eventsource
npm install -D @types/eventsource
```

- [ ] **Step 2: 在 client.ts 顶部添加 import**

```typescript
import EventSource from 'eventsource'
```

- [ ] **Step 3: 添加 connectSSE 方法**

在 `connectServer()` 方法之后添加：

```typescript
/**
 * 连接 SSE 传输的 MCP 服务器。
 */
private async connectSSE(name: string): Promise<void> {
  const server = this.servers.get(name)
  if (!server) throw new Error(`MCP 服务器 "${name}" 未注册`)

  const { config } = server
  const url = config.url!
  const messagesUrl = url.replace(/\/sse$/, '/messages')

  // 建立 SSE 连接
  const eventSource = new EventSource(url, {
    headers: config.headers,
  })

  // 监听消息
  eventSource.onmessage = (event) => {
    this.handleSSEMessage(name, event.data)
  }

  eventSource.onerror = () => {
    // SSE 连接错误，不抛出，让后续请求自然失败
  }

  server.sseConnection = { eventSource, messagesUrl }

  // 等待连接就绪
  await new Promise(resolve => setTimeout(resolve, 500))

  // 发送 initialize
  await this.sendSSERequest(name, 'initialize', {
    protocolVersion: '2024-11-05',
    capabilities: {},
    clientInfo: { name: 'codecast-agent', version: '0.1.0' },
  })

  // 发送 initialized 通知
  this.sendSSENotification(name, 'notifications/initialized', {})

  // 获取工具列表
  const toolsResult = await this.sendSSERequest(name, 'tools/list', {}) as { tools: MCPTool[] }
  server.tools = toolsResult.tools || []

  // 获取资源列表（可选）
  try {
    const resourcesResult = await this.sendSSERequest(name, 'resources/list', {}) as { resources: MCPResource[] }
    server.resources = resourcesResult.resources || []
  } catch {
    server.resources = []
  }
}
```

- [ ] **Step 4: 添加 sendSSERequest 方法**

在 `sendRequest()` 方法之后添加：

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

    const url = server.sseConnection!.messagesUrl
    fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...server.config.headers,
      },
      body: JSON.stringify(request),
    }).catch(err => {
      this.pendingRequests.delete(id)
      reject(new Error(`SSE 请求发送失败: ${(err as Error).message}`))
    })

    setTimeout(() => {
      if (this.pendingRequests.has(id)) {
        this.pendingRequests.delete(id)
        reject(new Error(`SSE 请求超时: ${method}`))
      }
    }, 30000)
  })
}
```

- [ ] **Step 5: 添加 sendSSENotification 方法**

在 `sendNotification()` 方法之后添加：

```typescript
private sendSSENotification(serverName: string, method: string, params: Record<string, unknown>) {
  const server = this.servers.get(serverName)
  if (!server?.sseConnection) return

  const notification: JsonRpcNotification = { jsonrpc: '2.0', method, params }
  const url = server.sseConnection.messagesUrl

  fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...server.config.headers,
    },
    body: JSON.stringify(notification),
  }).catch(() => {
    // 通知发送失败不阻塞
  })
}
```

- [ ] **Step 6: 添加 handleSSEMessage 方法**

在 `handleData()` 方法之后添加：

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

- [ ] **Step 7: 类型检查**

```bash
npx tsc -p tsconfig.test.json --noEmit
```
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add src/mcp/client.ts package.json package-lock.json
git commit -m "feat(mcp): add SSE transport support for remote MCP servers"
```

---

### Task 3: 统一请求方法 + connectAll/disconnectAll/callTool 改造

**Files:**
- Modify: `src/mcp/client.ts`

- [ ] **Step 1: 添加 sendRequestByTransport 统一方法**

在 `sendSSERequest()` 方法之后添加：

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

- [ ] **Step 2: 改造 connectAll**

替换现有的 `connectAll()` 方法：

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

- [ ] **Step 3: 改造 disconnectAll**

替换现有的 `disconnectAll()` 方法：

```typescript
disconnectAll() {
  for (const [name, server] of this.servers) {
    if (server.process) {
      try { server.process.kill() } catch { /* ignore */ }
      server.process = null
    }
    if (server.sseConnection) {
      server.sseConnection.eventSource.close()
      server.sseConnection = null
    }
    this.buffers.delete(name)
  }
  for (const [, pending] of this.pendingRequests) {
    pending.reject(new Error('MCP 客户端断开连接'))
  }
  this.pendingRequests.clear()
}
```

- [ ] **Step 4: 改造 callTool**

替换现有的 `callTool()` 方法：

```typescript
async callTool(serverName: string, toolName: string, args: Record<string, unknown>): Promise<unknown> {
  return this.sendRequestByTransport(serverName, 'tools/call', {
    name: toolName,
    arguments: args,
  })
}
```

- [ ] **Step 5: 在 connectServer 末尾添加 Resource 发现**

在 `connectServer()` 方法中，`server.tools = toolsResult.tools || []` 之后添加：

```typescript
    // 获取资源列表（可选）
    try {
      const resourcesResult = await this.sendRequest(name, 'resources/list', {}) as { resources: MCPResource[] }
      server.resources = resourcesResult.resources || []
    } catch {
      server.resources = []
    }
```

- [ ] **Step 6: 类型检查**

```bash
npx tsc -p tsconfig.test.json --noEmit
```
Expected: PASS

- [ ] **Step 7: 运行已有测试**

```bash
npx tsx --test tests/**/*.test.ts
```
Expected: 所有测试 PASS

- [ ] **Step 8: Commit**

```bash
git add src/mcp/client.ts
git commit -m "feat(mcp): unify transport layer, add Resource support, refactor connectAll/disconnectAll/callTool"
```

---

### Task 4: MCP Resource 公开方法

**Files:**
- Modify: `src/mcp/client.ts`

- [ ] **Step 1: 添加 listResources / readResource / getAllResources 方法**

在 `getAllTools()` 方法之后添加：

```typescript
/**
 * 列出服务器的资源。
 */
async listResources(serverName: string): Promise<MCPResource[]> {
  const result = await this.sendRequestByTransport(serverName, 'resources/list', {}) as { resources: MCPResource[] }
  const server = this.servers.get(serverName)
  if (server) {
    server.resources = result.resources || []
  }
  return result.resources || []
}

/**
 * 读取资源内容。
 */
async readResource(serverName: string, uri: string): Promise<string> {
  const result = await this.sendRequestByTransport(serverName, 'resources/read', { uri }) as { contents: Array<{ uri: string; text?: string; blob?: string }> }
  if (result.contents && result.contents.length > 0) {
    return result.contents[0].text || result.contents[0].blob || ''
  }
  return ''
}

/**
 * 获取所有已连接服务器的资源列表。
 */
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

- [ ] **Step 2: 类型检查**

```bash
npx tsc -p tsconfig.test.json --noEmit
```
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add src/mcp/client.ts
git commit -m "feat(mcp): add listResources, readResource, getAllResources public methods"
```

---

### Task 5: loadMCPConfig 适配 SSE

**Files:**
- Modify: `src/mcp/client.ts`

- [ ] **Step 1: 更新 loadMCPConfig 函数**

替换现有的 `loadMCPConfig()` 函数：

```typescript
export function loadMCPConfig(configPath: string): MCPServerConfig[] {
  if (!existsSync(configPath)) return []

  try {
    const raw = readFileSync(configPath, 'utf-8')
    const config = JSON.parse(raw)
    const servers: MCPServerConfig[] = []

    for (const [name, serverConfig] of Object.entries(config.mcpServers || {})) {
      const sc = serverConfig as any
      if (sc.url) {
        // SSE 传输
        servers.push({
          name,
          url: sc.url,
          headers: sc.headers,
          transport: 'sse',
        })
      } else if (sc.command) {
        // stdio 传输
        servers.push({
          name,
          command: sc.command,
          args: sc.args,
          env: sc.env,
          transport: 'stdio',
        })
      }
    }

    return servers
  } catch {
    return []
  }
}
```

- [ ] **Step 2: 类型检查**

```bash
npx tsc -p tsconfig.test.json --noEmit
```
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add src/mcp/client.ts
git commit -m "feat(mcp): update loadMCPConfig to support SSE transport config"
```

---

### Task 6: 自定义命令加载器

**Files:**
- Create: `src/commands/loader.ts`

- [ ] **Step 1: 创建 loader.ts**

```typescript
// src/commands/loader.ts
import { readFileSync, readdirSync, existsSync } from 'fs'
import { join } from 'path'

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

/**
 * 从 .codecast/commands/ 目录加载自定义命令。
 */
export function loadCustomCommands(commandsDir: string): CustomCommand[] {
  if (!existsSync(commandsDir)) return []

  const commands: CustomCommand[] = []

  for (const file of readdirSync(commandsDir)) {
    if (!file.endsWith('.md')) continue

    const filePath = join(commandsDir, file)
    const content = readFileSync(filePath, 'utf-8')

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
export function parseSimpleYAML(yaml: string): Record<string, string> {
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

- [ ] **Step 2: 类型检查**

```bash
npx tsc -p tsconfig.test.json --noEmit
```
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add src/commands/loader.ts
git commit -m "feat(commands): add custom command loader with YAML frontmatter parsing"
```

---

### Task 7: 内置命令示例文件

**Files:**
- Create: `.codecast/commands/review.md`
- Create: `.codecast/commands/explain.md`
- Create: `.codecast/commands/test.md`

- [ ] **Step 1: 创建 review.md**

```markdown
---
name: review
description: 代码审查指定文件
mode: build
---

请对以下文件进行代码审查，关注安全性、性能和可维护性：
$ARGUMENTS
```

- [ ] **Step 2: 创建 explain.md**

```markdown
---
name: explain
description: 解释代码逻辑
mode: ask
---

请解释以下代码的逻辑：
$ARGUMENTS
```

- [ ] **Step 3: 创建 test.md**

```markdown
---
name: test
description: 为指定代码生成测试
mode: build
---

请为以下代码生成单元测试：
$ARGUMENTS
```

- [ ] **Step 4: Commit**

```bash
git add .codecast/commands/
git commit -m "feat(commands): add built-in custom command examples (review, explain, test)"
```

---

### Task 8: index.ts 集成自定义命令

**Files:**
- Modify: `src/index.ts`

- [ ] **Step 1: 添加 import**

在 `src/index.ts` 顶部 import 区域添加：

```typescript
import { loadCustomCommands, CustomCommand } from './commands/loader.js'
```

- [ ] **Step 2: 添加 customCommands 字段**

在 `private modelConfig` 字段之后添加：

```typescript
  /** 自定义命令列表 */
  private customCommands: CustomCommand[] = []
```

- [ ] **Step 3: 在 constructor 中加载自定义命令**

在 `this.applyModelConfig()` 之后添加：

```typescript
    this.customCommands = loadCustomCommands(resolve(process.cwd(), '.codecast', 'commands'))
```

注意：`resolve` 已在顶部 import。

- [ ] **Step 4: 在 /help 中显示自定义命令**

找到 `case 'help':` 的输出内容，在现有命令列表之后添加：

```typescript
    if (this.customCommands.length > 0) {
      lines.push('')
      lines.push(`${ui.color.bold}自定义命令:${ui.color.reset}`)
      for (const cmd of this.customCommands) {
        lines.push(`  /${cmd.name.padEnd(16)} ${cmd.description}`)
      }
    }
```

- [ ] **Step 5: 在 handleCommand 的 default 分支中处理自定义命令**

找到 `handleCommand()` 的 `default:` 分支，替换为：

```typescript
      default: {
        // 检查自定义命令
        const cmd = this.customCommands.find(c => c.name === name)
        if (cmd) {
          const prompt = cmd.template.replace('$ARGUMENTS', args.join(' '))
          if (cmd.mode && this.modeManager) {
            this.modeManager.setMode(cmd.mode as any)
          }
          // 发送给 AI（调用现有的 processUserInput 或直接发消息）
          await this.processUserInput(prompt)
        } else {
          console.log(`未知命令: /${name}，输入 /help 查看可用命令`)
        }
        break
      }
```

注意：需要确认 `this.processUserInput` 方法是否存在。如果不存在，使用 `this.sendMessage(prompt)` 或其他现有的消息发送方法。读取 `src/index.ts` 确认实际方法名。

- [ ] **Step 6: 类型检查**

```bash
npx tsc -p tsconfig.test.json --noEmit
```
Expected: PASS

- [ ] **Step 7: 运行已有测试**

```bash
npx tsx --test tests/**/*.test.ts
```
Expected: 所有测试 PASS

- [ ] **Step 8: Commit**

```bash
git add src/index.ts
git commit -m "feat(commands): integrate custom slash commands into index.ts"
```

---

### Task 9: MCP SSE 测试

**Files:**
- Create: `tests/mcp-sse.test.ts`

- [ ] **Step 1: 创建测试文件**

```typescript
// tests/mcp-sse.test.ts
import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { MCPClient, MCPServerConfig, MCPTool, MCPResource, loadMCPConfig } from '../src/mcp/client.js'

// ============== 类型检查 ==============

test('MCPServerConfig supports sse transport', () => {
  const config: MCPServerConfig = {
    name: 'test-sse',
    url: 'http://localhost:3000/sse',
    headers: { Authorization: 'Bearer test' },
    transport: 'sse',
  }
  assert.equal(config.transport, 'sse')
  assert.equal(config.url, 'http://localhost:3000/sse')
})

test('MCPServerConfig supports stdio transport', () => {
  const config: MCPServerConfig = {
    name: 'test-stdio',
    command: 'npx',
    args: ['-y', 'some-server'],
    transport: 'stdio',
  }
  assert.equal(config.transport, 'stdio')
  assert.equal(config.command, 'npx')
})

// ============== MCPClient 基础 ==============

test('MCPClient registerServer with SSE config', () => {
  const client = new MCPClient()
  client.registerServer({
    name: 'remote-api',
    url: 'http://localhost:3000/sse',
    transport: 'sse',
  })
  // 不连接，只验证注册不报错
  const tools = client.getAllTools()
  assert.equal(tools.length, 0)
})

test('MCPClient registerServer with stdio config', () => {
  const client = new MCPClient()
  client.registerServer({
    name: 'local-fs',
    command: 'npx',
    args: ['-y', 'some-server'],
    transport: 'stdio',
  })
  const tools = client.getAllTools()
  assert.equal(tools.length, 0)
})

test('MCPClient disconnectAll with no servers', () => {
  const client = new MCPClient()
  client.disconnectAll()  // 不应报错
})

// ============== loadMCPConfig ==============

test('loadMCPConfig returns empty for non-existent file', () => {
  const configs = loadMCPConfig('/nonexistent/mcp.json')
  assert.deepEqual(configs, [])
})

test('loadMCPConfig parses SSE config', () => {
  // 创建临时配置文件测试
  const { writeFileSync, mkdirSync, rmSync } = require('fs')
  const { join } = require('path')
  const tmpDir = join(process.cwd(), '.test-mcp-tmp')
  const tmpFile = join(tmpDir, 'mcp.json')

  try {
    mkdirSync(tmpDir, { recursive: true })
    writeFileSync(tmpFile, JSON.stringify({
      mcpServers: {
        'remote-api': {
          url: 'http://localhost:3000/sse',
          headers: { Authorization: 'Bearer test' },
        },
        'local-fs': {
          command: 'npx',
          args: ['-y', 'server'],
        },
      },
    }))

    const configs = loadMCPConfig(tmpFile)
    assert.equal(configs.length, 2)

    const sseConfig = configs.find(c => c.name === 'remote-api')!
    assert.equal(sseConfig.transport, 'sse')
    assert.equal(sseConfig.url, 'http://localhost:3000/sse')
    assert.deepEqual(sseConfig.headers, { Authorization: 'Bearer test' })

    const stdioConfig = configs.find(c => c.name === 'local-fs')!
    assert.equal(stdioConfig.transport, 'stdio')
    assert.equal(stdioConfig.command, 'npx')
  } finally {
    rmSync(tmpDir, { recursive: true, force: true })
  }
})

// ============== Resource 类型 ==============

test('MCPResource interface', () => {
  const resource: MCPResource = {
    uri: 'file:///test.txt',
    name: 'test.txt',
    description: 'A test file',
    mimeType: 'text/plain',
  }
  assert.equal(resource.uri, 'file:///test.txt')
  assert.equal(resource.name, 'test.txt')
})

test('MCPClient getAllResources returns empty with no servers', () => {
  const client = new MCPClient()
  const resources = client.getAllResources()
  assert.equal(resources.length, 0)
})
```

- [ ] **Step 2: 运行测试**

```bash
npx tsx --test tests/mcp-sse.test.ts
```
Expected: 所有测试 PASS

- [ ] **Step 3: Commit**

```bash
git add tests/mcp-sse.test.ts
git commit -m "test(mcp): add SSE transport and Resource tests"
```

---

### Task 10: 自定义命令测试

**Files:**
- Create: `tests/custom-commands.test.ts`

- [ ] **Step 1: 创建测试文件**

```typescript
// tests/custom-commands.test.ts
import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { loadCustomCommands, parseSimpleYAML, CustomCommand } from '../src/commands/loader.js'
import { writeFileSync, mkdirSync, rmSync } from 'fs'
import { join } from 'path'

// ============== parseSimpleYAML ==============

test('parseSimpleYAML parses key: value pairs', () => {
  const result = parseSimpleYAML('name: review\ndescription: Code review\nmode: build')
  assert.equal(result.name, 'review')
  assert.equal(result.description, 'Code review')
  assert.equal(result.mode, 'build')
})

test('parseSimpleYAML handles quoted values', () => {
  const result = parseSimpleYAML('name: "review"\ndescription: \'Code review\'')
  assert.equal(result.name, 'review')
  assert.equal(result.description, 'Code review')
})

test('parseSimpleYAML ignores empty lines', () => {
  const result = parseSimpleYAML('name: test\n\ndescription: desc')
  assert.equal(result.name, 'test')
  assert.equal(result.description, 'desc')
})

test('parseSimpleYAML returns empty for no matches', () => {
  const result = parseSimpleYAML('just some text\nno colons here')
  assert.deepEqual(result, {})
})

// ============== loadCustomCommands ==============

test('loadCustomCommands returns empty for non-existent directory', () => {
  const commands = loadCustomCommands('/nonexistent/commands')
  assert.deepEqual(commands, [])
})

test('loadCustomCommands loads .md files with frontmatter', () => {
  const tmpDir = join(process.cwd(), '.test-commands-tmp')

  try {
    mkdirSync(tmpDir, { recursive: true })
    writeFileSync(join(tmpDir, 'review.md'), `---
name: review
description: Code review
mode: build
---

Please review this code:
$ARGUMENTS`)

    const commands = loadCustomCommands(tmpDir)
    assert.equal(commands.length, 1)
    assert.equal(commands[0].name, 'review')
    assert.equal(commands[0].description, 'Code review')
    assert.equal(commands[0].mode, 'build')
    assert.ok(commands[0].template.includes('$ARGUMENTS'))
    assert.ok(commands[0].template.includes('Please review this code'))
  } finally {
    rmSync(tmpDir, { recursive: true, force: true })
  }
})

test('loadCustomCommands uses filename as default name', () => {
  const tmpDir = join(process.cwd(), '.test-commands-tmp')

  try {
    mkdirSync(tmpDir, { recursive: true })
    writeFileSync(join(tmpDir, 'deploy.md'), `---
description: Deploy command
---

Deploy: $ARGUMENTS`)

    const commands = loadCustomCommands(tmpDir)
    assert.equal(commands[0].name, 'deploy')
  } finally {
    rmSync(tmpDir, { recursive: true, force: true })
  }
})

test('loadCustomCommands skips files without frontmatter', () => {
  const tmpDir = join(process.cwd(), '.test-commands-tmp')

  try {
    mkdirSync(tmpDir, { recursive: true })
    writeFileSync(join(tmpDir, 'no-frontmatter.md'), 'Just some text without frontmatter')
    writeFileSync(join(tmpDir, 'valid.md'), `---
name: valid
description: Valid command
---

Valid: $ARGUMENTS`)

    const commands = loadCustomCommands(tmpDir)
    assert.equal(commands.length, 1)
    assert.equal(commands[0].name, 'valid')
  } finally {
    rmSync(tmpDir, { recursive: true, force: true })
  }
})

test('loadCustomCommands skips non-.md files', () => {
  const tmpDir = join(process.cwd(), '.test-commands-tmp')

  try {
    mkdirSync(tmpDir, { recursive: true })
    writeFileSync(join(tmpDir, 'notes.txt'), 'Not a command')
    writeFileSync(join(tmpDir, 'cmd.md'), `---
name: cmd
description: A command
---

Test: $ARGUMENTS`)

    const commands = loadCustomCommands(tmpDir)
    assert.equal(commands.length, 1)
    assert.equal(commands[0].name, 'cmd')
  } finally {
    rmSync(tmpDir, { recursive: true, force: true })
  }
})

test('CustomCommand template replaces $ARGUMENTS', () => {
  const cmd: CustomCommand = {
    name: 'review',
    description: 'Review code',
    template: 'Please review: $ARGUMENTS',
  }
  const result = cmd.template.replace('$ARGUMENTS', 'src/utils.ts')
  assert.equal(result, 'Please review: src/utils.ts')
})

test('CustomCommand template with empty $ARGUMENTS', () => {
  const cmd: CustomCommand = {
    name: 'review',
    description: 'Review code',
    template: 'Please review: $ARGUMENTS',
  }
  const result = cmd.template.replace('$ARGUMENTS', '')
  assert.equal(result, 'Please review: ')
})
```

- [ ] **Step 2: 运行测试**

```bash
npx tsx --test tests/custom-commands.test.ts
```
Expected: 所有测试 PASS

- [ ] **Step 3: 运行全部测试**

```bash
npx tsx --test tests/**/*.test.ts
```
Expected: 所有测试 PASS

- [ ] **Step 4: Commit**

```bash
git add tests/custom-commands.test.ts
git commit -m "test(commands): add custom command loader tests"
```

---

## 自检

**1. Spec 覆盖检查：**

| 验收标准 | 对应 Task |
|---|---|
| SSE 传输：url + transport: 'sse' 配置 | Task 1 (类型) + Task 2 (实现) + Task 5 (配置加载) |
| SSE 请求：initialize + tools/list + tools/call | Task 2 (connectSSE) + Task 3 (callTool 改造) |
| SSE 断开：disconnectAll 关闭 EventSource | Task 3 (disconnectAll 改造) |
| Resource 支持：listResources + readResource | Task 4 (公开方法) |
| Resource 容错：不支持时不报错 | Task 2 (try/catch) + Task 4 (空数组返回) |
| 自定义命令加载：.codecast/commands/*.md | Task 6 (loader) + Task 8 (index.ts 集成) |
| 自定义命令执行：/review src/utils.ts | Task 8 (handleCommand default 分支) |
| 自定义命令 /help：显示在列表中 | Task 8 (/help 集成) |
| 向后兼容 | Task 3 (connectAll 按类型分发) + Task 8 (default 分支兜底) |

**2. 占位符检查：** 无 TBD/TODO/占位符。

**3. 类型一致性检查：**
- `MCPServerConfig` 定义在 Task 1，在 Task 2/3/5/9 中使用，字段一致
- `ServerEntry` 定义在 Task 1，在 Task 2/3/4 中使用，字段一致
- `SSEConnection` 定义在 Task 1，在 Task 2/3 中使用，字段一致
- `CustomCommand` 定义在 Task 6，在 Task 8/10 中使用，字段一致
- `MCPResource` 已在现有代码中定义，Task 4 使用一致
- `parseSimpleYAML` 定义在 Task 6，在 Task 10 中测试，签名一致
- `loadCustomCommands` 定义在 Task 6，在 Task 8/10 中使用，签名一致