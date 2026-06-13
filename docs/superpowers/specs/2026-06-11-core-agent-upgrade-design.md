# CodecastCLI 阶段一核心能力升级设计

## 背景

CodecastCLI 已有完整的模块骨架（多 LLM、Agent 模式、工具系统、MCP、Hook、权限、会话管理），但与世界顶级 CLI 工具（Claude Code、Codex CLI、OpenCode）相比，核心 Agent 能力存在根本性缺陷：无法自主多步执行、子代理是空壳、上下文管理粗糙。本设计聚焦阶段一的 7 项改进，将 CodecastCLI 从"带工具调用的聊天机器人"升级为"真正的 AI Agent"。

## 改进项总览

| 顺序 | 改进项 | 依赖 | 影响范围 |
|---|---|---|---|
| 1 | Tool Schema 去重 + 统一注册表 | 无 | base.ts, llm.ts, index.ts |
| 2 | Agentic Loop（含 consumeStream 重构） | #1 | index.ts, 新增 agent-loop.ts |
| 3 | 子代理复用 AgentLoop | #2 | subagent.ts |
| 4 | CODECAST.md + 模式工具白名单联动 | #1 | modes.ts, index.ts, 新增 project-context.ts |
| 5 | 精确 Token 追踪 | #2 | llm.ts, tracker.ts, agent-loop.ts |
| 6 | 增量式上下文压缩 | #2 | compactor.ts, agent-loop.ts |
| 7 | Bash 安全模型重构 | 无 | bash.ts, permissions/model.ts |

---

## 1. Tool Schema 去重 + 统一注册表

### 问题

- `llm.ts` 中 `getToolsSchema()` 和 `getAnthropicToolsSchema()` 有 ~400 行几乎完全重复的 Schema 定义
- Schema 和 Tool 实现分离——改了工具实现容易忘记改 Schema
- 无法动态过滤工具（子代理/模式需要限制工具集）

### 设计

扩展 `Tool` 接口，让每个 Tool 自带 Schema：

```typescript
// src/tools/base.ts
export interface ToolParameterSchema {
  type: 'object'
  properties: Record<string, {
    type: string
    description: string
    items?: Record<string, unknown>
    enum?: string[]
  }>
  required?: string[]
}

export interface Tool {
  name: string
  description: string
  parameters: ToolParameterSchema  // 新增：每个 Tool 自带参数 Schema
  execute(args: Record<string, unknown>): Promise<string>
}
```

新增 `ToolRegistry`，统一管理工具注册和 Schema 生成：

```typescript
// src/tools/registry.ts
export class ToolRegistry {
  private tools = new Map<string, Tool>()

  register(tool: Tool): void
  registerMany(tools: Tool[]): void
  get(name: string): Tool | undefined
  getAll(): Tool[]
  getFiltered(allowedNames: string[]): Tool[]  // 按白名单过滤

  // 自动生成两种格式的 Schema
  toOpenAISchema(tools?: Tool[]): OpenAIToolSchema[]
  toAnthropicSchema(tools?: Tool[]): AnthropicToolSchema[]
}
```

`LLMClient` 接收 `Tool[]` 参数而非自己硬编码 Schema：

```typescript
// src/api/llm.ts
class LLMClient {
  // chatStream 增加 tools 参数
  async *chatStream(
    messages: ChatMessage[],
    tools?: Tool[]  // 新增：动态传入工具列表
  ): AsyncGenerator<string, StreamFinal, unknown>
}
```

删除 `getToolsSchema()` 和 `getAnthropicToolsSchema()` 两个硬编码方法，改为从 `ToolRegistry` 动态生成。

### 文件变更

- 修改 `src/tools/base.ts`：扩展 Tool 接口
- 新增 `src/tools/registry.ts`：ToolRegistry 类
- 修改 `src/tools/file.ts`、`edit-file.ts`、`bash.ts`、`codebase.ts`、`git.ts`：每个 Tool 添加 parameters 字段
- 修改 `src/api/llm.ts`：删除硬编码 Schema，接收动态 tools 参数
- 修改 `src/index.ts`：用 ToolRegistry 替代 builtinTools 字典

---

## 2. Agentic Loop

### 问题

- `chat()` 只做两轮流：第一轮获取工具调用 → 执行 → 第二轮让 AI 总结
- AI 需要再调用工具时（如读文件后发现需要再读另一个文件），无法继续
- `consumeStream()` 不返回 `stopReason` 和 `usage`
- 没有中断/超时机制

### 设计

新增 `AgentLoop` 类，封装完整的 Agent 循环：

```typescript
// src/agent/agent-loop.ts

export interface LoopConfig {
  maxIterations: number     // 最大迭代次数，默认 50
  maxTimeoutMs: number      // 总超时，默认 5 分钟
  systemPrompt: string
  tools: Tool[]             // 本轮可用的工具
  onToolCall?: (name: string, args: Record<string, unknown>) => void  // 回调：UI 展示
  onStreamChunk?: (chunk: string) => void  // 回调：流式输出
  onIterationStart?: (iteration: number) => void  // 回调：迭代开始
}

export interface LoopResult {
  messages: ChatMessage[]       // 完整消息历史（含本轮新增）
  finalText: string            // AI 最终回复
  iterations: number           // 实际迭代次数
  toolCallsExecuted: number    // 执行的工具调用数
  usage: TokenUsage            // 精确 token 用量
  stoppedReason: string        // 结束原因：'no_tool_calls' | 'max_iterations' | 'timeout' | 'stop_reason'
}

export interface TokenUsage {
  inputTokens: number
  outputTokens: number
  cacheReadTokens?: number
  cacheCreationTokens?: number
}

export class AgentLoop {
  constructor(
    private llm: LLMClient,
    private toolExecutor: ToolExecutor,  // 工具执行器接口
    private config: LoopConfig
  ) {}

  async run(userMessage: string, history: ChatMessage[]): Promise<LoopResult> {
    const messages = [...history, { role: 'user' as const, content: userMessage }]
    let iterations = 0
    let toolCallsExecuted = 0
    const totalUsage: TokenUsage = { inputTokens: 0, outputTokens: 0 }
    const startTime = Date.now()

    while (iterations < this.config.maxIterations) {
      if (Date.now() - startTime > this.config.maxTimeoutMs) {
        return { messages, finalText: '', iterations, toolCallsExecuted, usage: totalUsage, stoppedReason: 'timeout' }
      }

      iterations++
      this.config.onIterationStart?.(iterations)

      // 调用 LLM（传入动态工具列表）
      const streamResult = await this.consumeLLMStream(messages, this.config.tools)
      totalUsage.inputTokens += streamResult.usage?.inputTokens || 0
      totalUsage.outputTokens += streamResult.usage?.outputTokens || 0
      totalUsage.cacheReadTokens = (totalUsage.cacheReadTokens || 0) + (streamResult.usage?.cacheReadTokens || 0)
      totalUsage.cacheCreationTokens = (totalUsage.cacheCreationTokens || 0) + (streamResult.usage?.cacheCreationTokens || 0)

      // 无工具调用 → AI 完成
      if (!streamResult.toolCalls || streamResult.toolCalls.length === 0) {
        return { messages, finalText: streamResult.text, iterations, toolCallsExecuted, usage: totalUsage, stoppedReason: 'no_tool_calls' }
      }

      // 逐个执行工具调用
      messages.push({ role: 'assistant', content: streamResult.text, tool_calls: streamResult.toolCalls })
      for (const tc of streamResult.toolCalls) {
        const args = JSON.parse(tc.function.arguments || '{}')
        this.config.onToolCall?.(tc.function.name, args)
        const result = await this.toolExecutor.execute(tc.function.name, args)
        messages.push({ tool_call_id: tc.id, role: 'tool', content: result })
        toolCallsExecuted++
      }
    }

    return { messages, finalText: '', iterations, toolCallsExecuted, usage: totalUsage, stoppedReason: 'max_iterations' }
  }
}
```

**工具执行器接口**（解耦 AgentLoop 和具体工具实现）：

```typescript
// src/tools/executor.ts
export interface ToolExecutor {
  execute(name: string, args: Record<string, unknown>): Promise<string>
}
```

主 Agent 和子代理都实现这个接口，但内部逻辑不同（主 Agent 含权限检查 + Hook，子代理直接执行）。

**consumeStream 重构**：

扩展 `StreamFinal` 返回值：

```typescript
interface StreamFinal {
  toolCalls?: ToolCall[]
  stopReason?: string     // 新增：'end_turn' | 'tool_use' | 'max_tokens'
  usage?: {               // 新增：精确 token 用量
    inputTokens: number
    outputTokens: number
    cacheReadTokens?: number
    cacheCreationTokens?: number
  }
}
```

OpenAI 流式解析中提取 usage：

```
最后一个 chunk: chunk.usage?.prompt_tokens / completion_tokens
```

Anthropic 流式解析中提取 usage：

```
evt.type === 'message_delta' → evt.usage.output_tokens
evt.type === 'message_start' → evt.message.usage.input_tokens
```

**chat() 简化**：

```typescript
// src/index.ts
private async chat(content: string, displayText?: string) {
  this.session.messages.push({ role: 'user', content })
  ui.printUser(displayText || content)
  ui.printDivider()

  const tools = this.toolRegistry.getFiltered(this.modeManager.config.tools)
  const loop = new AgentLoop(this.llm, this.mainToolExecutor, {
    systemPrompt: this.modeManager.config.systemPrompt,
    tools,
    maxIterations: 50,
    onStreamChunk: (chunk) => this.streamRenderer.write(chunk),
    onToolCall: (name, args) => this.toolRenderer.printToolCall(name, args),
  })

  const result = await loop.run(content, this.session.messages)

  // 更新会话
  this.session.messages = result.messages
  this.costTracker.record(
    process.env.LLM_MODEL || this.providerConfig.defaultModel,
    process.env.LLM_PROVIDER || 'openai',
    result.usage,
  )

  ui.printDivider()
}
```

### 文件变更

- 新增 `src/agent/agent-loop.ts`：AgentLoop 类
- 新增 `src/tools/executor.ts`：ToolExecutor 接口
- 修改 `src/api/llm.ts`：StreamFinal 扩展、流式解析提取 usage/stopReason、chatStream 接收 tools 参数
- 修改 `src/index.ts`：chat() 使用 AgentLoop，删除 consumeStream() 和手动工具执行逻辑

---

## 3. 子代理复用 AgentLoop

### 问题

- 子代理检测到工具调用后只收集文本，不执行工具
- 子代理没有自己的循环，无法多步推理

### 设计

子代理的 `dispatch()` 直接创建一个 AgentLoop 实例：

```typescript
// src/agent/subagent.ts
export class SubAgentDispatcher implements ToolExecutor {
  private toolExecutor: ToolExecutor | null = null  // 主 Agent 的工具执行器

  setToolExecutor(executor: ToolExecutor) {
    this.toolExecutor = executor
  }

  async dispatch(agentName: string, task: string, context?: string): Promise<SubAgentResult> {
    const agent = this.agents.get(agentName)
    if (!agent) return { agent: agentName, success: false, output: `未知子代理: ${agentName}`, rounds: 0, tokensUsed: 0 }
    if (!this.llm || !this.toolExecutor) return { agent: agentName, success: false, output: '未初始化', rounds: 0, tokensUsed: 0 }

    const tools = this.toolRegistry.getFiltered(agent.tools || [])  // 子代理的工具白名单
    const loop = new AgentLoop(this.llm, this.toolExecutor, {
      systemPrompt: agent.systemPrompt,
      tools,
      maxIterations: agent.maxRounds,
      onStreamChunk: () => {},  // 子代理不流式输出到终端
    })

    const userMessage = context
      ? `上下文信息:\n${context}\n\n---\n\n任务: ${task}`
      : task

    const result = await loop.run(userMessage, [])

    return {
      agent: agentName,
      success: result.stoppedReason !== 'max_iterations',
      output: result.finalText,
      rounds: result.iterations,
      tokensUsed: result.usage.inputTokens + result.usage.outputTokens,
    }
  }
}
```

关键点：
- 子代理复用主 Agent 的 `ToolExecutor`，权限检查和 Hook 自动生效
- 子代理的工具列表由 `agent.tools` 白名单控制
- 子代理不直接输出到终端（静默执行），结果返回给主 Agent

### 文件变更

- 修改 `src/agent/subagent.ts`：用 AgentLoop 替代手动循环，实现 ToolExecutor
- 修改 `src/index.ts`：注入 ToolExecutor 到 SubAgentDispatcher

---

## 4. CODECAST.md + 模式工具白名单联动

### 问题

- 没有项目级指令文件机制，无法自动注入项目知识
- modes.ts 的 `tools` 白名单从未被使用——所有工具都无条件注册

### 设计

**CODECAST.md 发现与加载**：

```typescript
// src/context/project-context.ts
export class ProjectContext {
  /**
   * 从项目根目录向上遍历，收集所有 CODECAST.md 文件。
   * 优先级：根目录 > 父目录 > 祖父目录（越近越优先）
   */
  loadInstructions(projectRoot: string): string

  /**
   * 解析 CODECAST.md 中的 [tools] 段。
   * 格式：
   * [tools]
   * allow: read_file, write_file, bash
   * deny: rm, git push
   */
  parseToolOverrides(content: string): { allow?: string[]; deny?: string[] }
}
```

CODECAST.md 格式示例：

```markdown
# 项目指令

本项目使用 Vue 3 + Pinia + TypeScript。
API 前缀为 /api/v2，所有请求需要 Bearer token。
代码风格：2 空格缩进，单引号，无分号。

[tools]
allow: read_file, write_file, edit_file, bash, search_code, git_status, git_diff
```

**模式工具白名单生效**：

修改 `AgentModeManager`，使其 `tools` 字段真正控制传给 LLM 的工具 Schema：

```typescript
// 在 chat() 中：
const allowedTools = this.modeManager.config.tools  // ['read_file'] for ask mode
const tools = this.toolRegistry.getFiltered(allowedTools)
// tools 只包含白名单内的工具，AI 看不到其他工具的 Schema
```

**systemPrompt 注入链**：

```
mode.systemPrompt          （模式基础提示）
  + CODECAST.md 内容       （项目知识）
  + autoMemory.formatForPrompt()  （记忆）
  + codebaseHint           （代码库工具提示）
  + subAgentHint           （子代理提示）
```

### 文件变更

- 新增 `src/context/project-context.ts`：ProjectContext 类
- 修改 `src/agent/modes.ts`：修正 tools 白名单（ask 只有 read-only 工具，plan 增加 bash，build 全部）
- 修改 `src/index.ts`：updateSystemPrompt() 注入 CODECAST.md，chat() 使用模式工具白名单

---

## 5. 精确 Token 追踪

### 问题

- 当前用 `字符数 × 0.5` 估算 token，误差 2-3 倍
- 不区分模型定价
- 无法获取 cache token

### 设计

在流式解析中提取 API 返回的精确 usage：

**OpenAI**：

```typescript
// 在 openaiStream() 中
// 最后一个 chunk 包含 usage
if (chunk.usage) {
  usage = {
    inputTokens: chunk.usage.prompt_tokens,
    outputTokens: chunk.usage.completion_tokens,
    cacheReadTokens: chunk.usage.prompt_tokens_details?.cached_tokens,
  }
}
```

**Anthropic**：

```typescript
// 在 anthropicStream() 中
// message_start 事件包含 input token
if (evt.type === 'message_start') {
  usage.inputTokens = evt.message.usage.input_tokens
  usage.cacheReadTokens = evt.message.usage.cache_creation_input_tokens
  usage.cacheCreationTokens = evt.message.usage.cache_read_input_tokens
}
// message_delta 事件包含 output token
if (evt.type === 'message_delta') {
  usage.outputTokens = evt.usage.output_tokens
}
```

**CostTracker 更新**：

```typescript
// 新增 cache token 定价支持
interface ModelPricing {
  inputPerMillion: number
  outputPerMillion: number
  cacheReadPerMillion?: number     // 新增
  cacheWritePerMillion?: number    // 新增
}
```

AgentLoop 在每次迭代累积 usage，最终传给 CostTracker：

```typescript
// AgentLoop.run() 中
const totalUsage: TokenUsage = { inputTokens: 0, outputTokens: 0 }
for (const iteration of iterations) {
  totalUsage.inputTokens += iteration.usage.inputTokens
  totalUsage.outputTokens += iteration.usage.outputTokens
  totalUsage.cacheReadTokens += iteration.usage.cacheReadTokens || 0
}
```

### 文件变更

- 修改 `src/api/llm.ts`：流式解析提取 usage，扩展 StreamFinal
- 修改 `src/cost/tracker.ts`：支持 cache token 定价，record() 接收 TokenUsage
- 修改 `src/agent/agent-loop.ts`：累积 usage 并传给 CostTracker
- 修改 `src/index.ts`：删除粗估 token 逻辑

---

## 6. 增量式上下文压缩

### 问题

- 当前"一刀切"压缩：保留最近 4 轮，其余全部压缩成规则摘要
- 规则摘要信息损失严重（`Q:添加用户接口 T:write_file R:2ok`）
- 一次性压缩多轮对话成本高

### 设计

**滑动窗口 + 增量摘要**：

```typescript
// src/context/compactor.ts 重构

export interface CompactionConfig {
  maxTokens: number          // 触发压缩的阈值，默认 80000
  keepRecentRounds: number   // 保留最近完整轮次，默认 4
  compactBatchSize: number   // 每次压缩的轮次批量，默认 1（增量）
  useLLMSummary: boolean     // 是否使用 LLM 生成摘要，默认 true
  summaryModel?: string      // 摘要使用的模型（可用便宜模型）
}

export class ContextCompactor {
  private summaryHistory: string = ''  // 已有的摘要（增量追加）

  compact(messages: ChatMessage[]): CompactionResult {
    const tokensBefore = this.estimateTokens(messages)
    if (tokensBefore <= this.config.maxTokens) {
      return { messages, didCompact: false, tokensBefore, tokensAfter: tokensBefore, roundsCompacted: 0 }
    }

    const systemMsg = messages.find(m => m.role === 'system')
    const nonSystem = messages.filter(m => m.role !== 'system')
    const rounds = this.groupByRounds(nonSystem)
    const keepCount = this.config.keepRecentRounds
    const recentRounds = rounds.slice(-keepCount)
    const oldRounds = rounds.slice(0, rounds.length - keepCount)

    if (oldRounds.length === 0) {
      return { messages, didCompact: false, tokensBefore, tokensAfter: tokensBefore, roundsCompacted: 0 }
    }

    // 增量摘要：只压缩最近的 1 批旧轮次
    const batch = oldRounds.slice(-this.config.compactBatchSize)
    const newSummary = this.config.useLLMSummary
      ? await this.generateLLMSummary(batch, this.summaryHistory)
      : this.generateRuleBasedSummary(batch)
    this.summaryHistory = this.summaryHistory
      ? this.summaryHistory + '\n' + newSummary
      : newSummary

    // 构建压缩后的消息
    const result: ChatMessage[] = []
    if (systemMsg) result.push(systemMsg)
    if (this.summaryHistory) {
      result.push({ role: 'user', content: `[对话摘要]\n${this.summaryHistory}` })
      result.push({ role: 'assistant', content: '收到，继续。' })
    }
    for (const round of recentRounds) result.push(...round)

    const tokensAfter = this.estimateTokens(result)
    return { messages: result, didCompact: true, tokensBefore, tokensAfter, roundsCompacted: batch.length }
  }

  /**
   * 用 LLM 生成增量摘要。
   * 只处理 1 轮对话，成本极低（~200 token 输出）。
   */
  private async generateLLMSummary(round: ChatMessage[], existingSummary: string): Promise<string> {
    // prompt: "基于已有摘要，将以下对话轮次的关键信息追加到摘要中"
  }
}
```

**压缩时机**：由 AgentLoop 在每次迭代后检查：

```typescript
// AgentLoop.run() 中
const compaction = this.compactor.compact(currentMessages)
if (compaction.didCompact) {
  currentMessages = compaction.messages
  // 通知用户
}
```

**摘要格式**：

```
[对话摘要]
#1 用户请求添加用户接口 → AI 创建了 src/api/users.ts，定义了 CRUD 接口
#2 用户要求添加测试 → AI 在 tests/users.test.ts 添加了 5 个测试用例
#3 用户发现登录 bug → AI 修复了 src/auth.ts 中的 token 过期逻辑
```

### 文件变更

- 修改 `src/context/compactor.ts`：增量摘要 + LLM 摘要 + 滑动窗口
- 修改 `src/agent/agent-loop.ts`：每次迭代后检查压缩

---

## 7. Bash 安全模型重构

### 问题

- 白名单包含 `rm`、`mv` 等危险命令（`rm -rf /` 是允许的）
- 禁止管道/重定向（`git log | head -5` 被拦截），不实用
- 用 `execFileSync` + `shell: false` 无法执行管道命令

### 设计

**命令分级**：

```typescript
// src/tools/bash.ts 重构

type CommandRisk = 'safe' | 'dangerous' | 'unknown'

const SAFE_COMMANDS = new Set([
  'ls', 'cat', 'head', 'tail', 'pwd', 'date', 'whoami', 'uname',
  'echo', 'env', 'which', 'node', 'npm', 'npx', 'pnpm', 'yarn', 'bun',
  'tsx', 'tsc', 'git', 'diff', 'find', 'grep', 'wc', 'sort',
])

const DANGEROUS_COMMANDS = new Set([
  'rm', 'rmdir', 'mv', 'cp', 'chmod', 'chown',
  'curl', 'wget', 'ssh', 'scp',
  'docker', 'kubectl',
  'npm publish', 'git push', 'git reset', 'git checkout --',
])

function classifyCommand(command: string): CommandRisk {
  const head = command.trim().split(/\s+/)[0]
  if (SAFE_COMMANDS.has(head)) return 'safe'
  if (DANGEROUS_COMMANDS.has(head)) return 'dangerous'
  return 'unknown'
}
```

**执行策略**：

```typescript
async execute(args: Record<string, unknown>): Promise<string> {
  const command = args.command as string
  const cwd = args.cwd as string | undefined

  // 1. 分类
  const risk = classifyCommand(command)

  // 2. 根据风险级别决定行为
  //    safe → 直接执行
  //    dangerous → 返回"需要确认"标记，由权限系统处理
  //    unknown → 返回"需要确认"标记

  // 3. 使用 execSync + shell: true（允许管道/重定向）
  //    但 cwd 限制在工作区内

  // 4. 超时 30 秒
}
```

**权限系统配合**：

```json
// .codecast/permissions.json
{
  "rules": [
    { "tool": "bash", "risk": "safe", "action": "allow" },
    { "tool": "bash", "risk": "dangerous", "action": "ask" },
    { "tool": "bash", "risk": "unknown", "action": "ask" },
    { "tool": "bash", "pattern": "rm -rf *", "action": "deny" }
  ]
}
```

PermissionManager 扩展以支持 `risk` 字段匹配：

```typescript
check(toolName: string, toolArgs: Record<string, unknown>, risk?: CommandRisk): PermissionCheckResult {
  // 匹配优先级：pattern > risk > tool > 默认
}
```

### 文件变更

- 修改 `src/tools/bash.ts`：命令分级 + shell: true 执行 + 删除白名单/元字符禁止
- 修改 `src/permissions/model.ts`：PermissionRule 增加 risk 字段，check() 增加 risk 参数
- 修改 `src/agent/agent-loop.ts`：工具执行时传递 risk 到权限检查

---

## 实施顺序

```
#1 Tool Schema 去重 + 统一注册表
  ↓
#2 Agentic Loop（含 consumeStream 重构）  ← #7 Bash 安全模型可并行
  ↓
#3 子代理复用 AgentLoop
  ↓
#4 CODECAST.md + 模式工具白名单联动
  ↓
#5 精确 Token 追踪
  ↓
#6 增量式上下文压缩
```

#7 Bash 安全模型与 #1 并行，无依赖关系。

## 验收标准

1. **Agentic Loop**：AI 能自主多步执行（读文件 → 发现问题 → 修改文件 → 运行测试 → 修复），无需用户中间干预
2. **子代理**：`dispatch_subagent` 调用后子代理能真正执行工具并返回结果
3. **CODECAST.md**：在项目根目录放置 CODECAST.md，AI 自动遵循其中的指令
4. **模式工具白名单**：Ask 模式下 AI 无法调用 write_file/bash
5. **精确 Token**：`/cost` 显示的 token 数与 API 实际用量误差 <5%
6. **增量压缩**：长对话中压缩后 AI 仍能回忆之前的关键决策
7. **Bash 安全**：`ls`/`git status` 直接执行，`rm`/`git push` 需确认，管道命令正常工作
