# TUI 全屏渲染设计文档（Ink）

## 目标

用 Ink (React for CLI) 完全重写 CodecastCLI 的 UI 层，实现三段式全屏 TUI：状态栏 + 消息滚动区 + 输入框。替代现有 readline 行式 UI。

## 架构

### 依赖

```json
{
  "dependencies": {
    "react": "^19.0.0",
    "ink": "^5.0.0",
    "ink-text-input": "^6.0.0",
    "ink-spinner": "^5.0.0",
    "chalk": "^5.0.0"
  }
}
```

### 布局

```
┌─ StatusBar ────────────────────────────────────────┐
│ 🤖 gpt-4o (openai) │ Build │ $0.12 │ /help │      │
├─ MessageList (Static + Active) ────────────────────┤
│ <Static items={completedMessages}>                 │
│   已完成的消息（不再 re-render）                      │
│ </Static>                                          │
│ <Box>                                              │
│   当前正在流式输出的消息（活跃区域，仅此区域 re-render） │
│ </Box>                                             │
├─ InputBar ─────────────────────────────────────────┤
│ > 帮我重构这个函数_                                 │
└────────────────────────────────────────────────────┘
```

### 性能关键设计：`<Static>` 分区

已完成的消息放入 `<Static>`，Ink 只渲染一次，后续不再处理。只有当前流式消息在活跃区域 re-render。

50 条消息时，每个 chunk 只 re-render 1 个 `<StreamMessage>` 组件，而非 50+。

### 组件树

```tsx
<App>
  <StatusBar model={...} mode={...} cost={...} />
  <Box flexDirection="column" flexGrow={1}>
    <Static items={completedMessages}>
      {msg => <MessageItem key={msg.id} message={msg} />}
    </Static>
    {activeMessage && <StreamMessage message={activeMessage} />}
  </Box>
  <InputBar onSubmit={handleSubmit} />
</App>
```

## 核心组件

| 组件 | 文件 | 职责 |
|---|---|---|
| `App` | `app.tsx` | 根组件，管理全局状态，渲染布局 |
| `StatusBar` | `status-bar.tsx` | 模型名 + 提供商 + 模式 + 累计 cost + 快捷键提示 |
| `MessageItem` | `message-item.tsx` | 单条已完成消息（用户/AI/工具调用/工具结果） |
| `StreamMessage` | `stream-message.tsx` | 当前流式 AI 回复，实时追加文本 |
| `ToolCallView` | `tool-call-view.tsx` | 工具调用展示（图标 + 名称 + 参数摘要 + 结果预览） |
| `CodeBlock` | `code-block.tsx` | 代码块 + 语法高亮 |
| `DiffView` | `diff-view.tsx` | 文件变更 diff 展示 |
| `InputBar` | `input-bar.tsx` | 底部输入框 + 历史记录 + 斜杠命令 |
| `Spinner` | 内联 | AI 思考中的加载指示器（ink-spinner） |
| `useAgent` | `use-agent.ts` | Hook：AgentLoop → React 状态桥接 |

## 数据流

```
用户输入 → InputBar.onSubmit
  → appState.addUserMessage(text)
  → agentLoop.run(text, history)
    → onChunk(chunk) → appState.appendActiveMessage(chunk)
    → onToolCall(call) → appState.addToolCall(call)
    → onToolResult(result) → appState.addToolResult(result)
    → onFinal(final) → appState.finalizeMessage(final)
```

## 状态管理

```typescript
interface Message {
  id: string
  role: 'user' | 'assistant' | 'tool_call' | 'tool_result'
  content: string
  toolName?: string
  toolArgs?: Record<string, unknown>
  toolResult?: string
  timestamp: number
}

interface ActiveMessage {
  id: string
  content: string       // 逐步追加
  toolCalls: Message[]  // 流式过程中穿插的工具调用
}

interface AppState {
  completedMessages: Message[]   // 已完成，放入 <Static>
  activeMessage: ActiveMessage   // 当前流式消息
  isStreaming: boolean
  model: string
  provider: string
  mode: string
  cost: number
  inputHistory: string[]
}
```

纯 `useState` 管理，不需要 Redux。

## useAgent Hook

```typescript
function useAgent(deps: {
  llm: LLMClient
  toolRegistry: ToolRegistry
  toolExecutor: ToolExecutor
  subAgentDispatcher: SubAgentDispatcher
  permManager: PermissionManager
  hookManager: HookManager
}) {
  const [completedMessages, setCompletedMessages] = useState<Message[]>([])
  const [activeMessage, setActiveMessage] = useState<ActiveMessage | null>(null)
  const [isStreaming, setIsStreaming] = useState(false)

  const sendMessage = useCallback(async (text: string) => {
    // 1. 添加用户消息
    setCompletedMessages(prev => [...prev, { id: genId(), role: 'user', content: text, timestamp: Date.now() }])

    // 2. 开始流式
    setIsStreaming(true)
    setActiveMessage({ id: genId(), content: '', toolCalls: [] })

    // 3. 创建 AgentLoop 并运行
    const loop = new AgentLoop(deps.llm, deps.toolExecutor, {
      maxIterations: 50,
      systemPrompt: '...',
      tools: deps.toolRegistry.getAll(),
      onChunk: (chunk) => {
        setActiveMessage(prev => prev ? { ...prev, content: prev.content + chunk } : null)
      },
      onToolCall: (call) => {
        setActiveMessage(prev => prev ? {
          ...prev,
          toolCalls: [...prev.toolCalls, { id: call.id, role: 'tool_call', content: '', toolName: call.name, toolArgs: call.args, timestamp: Date.now() }]
        } : null)
      },
      onToolResult: (result) => {
        setActiveMessage(prev => prev ? {
          ...prev,
          toolCalls: [...prev.toolCalls, { id: result.id, role: 'tool_result', content: result.output, timestamp: Date.now() }]
        } : null)
      },
      permManager: deps.permManager,
      hookManager: deps.hookManager,
    })

    const finalResult = await loop.run(text, history)

    // 4. 将活跃消息转为已完成
    setCompletedMessages(prev => [
      ...prev,
      { id: activeMessage.id, role: 'assistant', content: activeMessage.content, timestamp: Date.now() },
      ...activeMessage.toolCalls,
    ])
    setActiveMessage(null)
    setIsStreaming(false)
  }, [deps])

  return { completedMessages, activeMessage, isStreaming, sendMessage }
}
```

## 纯函数复用

从现有代码提取纯函数，Ink 组件调用：

| 现有代码 | 提取为纯函数 | 目标文件 | Ink 组件调用 |
|---|---|---|---|
| `StreamRenderer.highlight()` | `highlightCode(code, lang): string` | `tui/highlight.ts` | `<CodeBlock>` |
| `ToolRenderer.printDiff()` | `computeDiff(old, new): DiffLine[]` | `tui/diff.ts` | `<DiffView>` |
| `ToolRenderer.getIcon()` | `getToolIcon(name): string` | `tui/types.ts` | `<ToolCallView>` |

## 回退机制

```typescript
function supportsInk(): boolean {
  return process.stdout.isTTY
    && !!process.stdout.columns
    && process.env.TERM !== 'dumb'
}

if (supportsInk()) {
  renderTUI({ ... })
} else {
  renderLineUI({ ... })  // 现有 readline 模式
}
```

## 入口改造

```typescript
// src/index.ts
import { renderTUI } from './tui/app.js'

async function main() {
  const llm = new LLMClient(...)
  // ... 初始化（与现有逻辑相同）

  // 替换 readline 主循环
  renderTUI({ llm, toolRegistry, subAgentDispatcher, permManager, hookManager, ... })
}
```

## 构建配置

```json
// tsconfig.json 新增
{
  "compilerOptions": {
    "jsx": "react-jsx",
    "jsxImportSource": "react"
  }
}
```

package.json 新增 devDependencies：

```json
{
  "devDependencies": {
    "@types/react": "^19.0.0",
    "tsup": "^8.0.0"
  }
}
```

## 文件结构

```
src/
  tui/
    app.tsx              — Ink 根组件 + renderTUI 入口
    status-bar.tsx       — 状态栏
    message-list.tsx     — Static + 活跃消息区
    message-item.tsx     — 单条已完成消息
    stream-message.tsx   — 流式消息
    tool-call-view.tsx   — 工具调用展示
    code-block.tsx       — 代码块 + 高亮
    diff-view.tsx        — diff 展示
    input-bar.tsx        — 输入框
    use-agent.ts         — Hook：AgentLoop → React 状态桥接
    highlight.ts         — 纯函数：语法高亮（从 StreamRenderer 提取）
    diff.ts              — 纯函数：diff 计算（从 ToolRenderer 提取）
    types.ts             — Message / ActiveMessage / DiffLine 类型
  ui/                    — 保留，作为 fallback 行式 UI
  render/                — 保留，不再集成到主流程
  components/            — 保留，不再集成到主流程
```

## 验收标准

1. **全屏 TUI**：启动后进入 alternate screen buffer，三段式布局
2. **流式渲染**：AI 回复实时显示，代码块带语法高亮
3. **工具调用展示**：工具名称 + 参数摘要 + 结果预览 + diff
4. **输入框**：支持多行输入、历史记录（上下箭头）、斜杠命令
5. **状态栏**：显示模型/模式/cost，实时更新
6. **性能**：50+ 条消息对话中，流式输出不卡顿（`<Static>` 保证）
7. **回退**：非 TTY 环境自动回退到行式 UI
8. **快捷键**：Ctrl+C 中断、Ctrl+L 清屏、Escape 取消流式

## 修改文件清单

| 文件 | 操作 | 说明 |
|---|---|---|
| `src/tui/app.tsx` | 新增 | Ink 根组件 + renderTUI |
| `src/tui/status-bar.tsx` | 新增 | 状态栏 |
| `src/tui/message-list.tsx` | 新增 | 消息列表 |
| `src/tui/message-item.tsx` | 新增 | 单条消息 |
| `src/tui/stream-message.tsx` | 新增 | 流式消息 |
| `src/tui/tool-call-view.tsx` | 新增 | 工具调用展示 |
| `src/tui/code-block.tsx` | 新增 | 代码块 |
| `src/tui/diff-view.tsx` | 新增 | diff 展示 |
| `src/tui/input-bar.tsx` | 新增 | 输入框 |
| `src/tui/use-agent.ts` | 新增 | Agent 桥接 Hook |
| `src/tui/highlight.ts` | 新增 | 语法高亮纯函数 |
| `src/tui/diff.ts` | 新增 | diff 计算纯函数 |
| `src/tui/types.ts` | 新增 | 类型定义 |
| `src/index.ts` | 修改 | 入口改造，调用 renderTUI |
| `package.json` | 修改 | 添加 react/ink 等依赖 |
| `tsconfig.json` | 修改 | 添加 jsx 配置 |
| `tests/tui.test.tsx` | 新增 | TUI 组件测试 |

## 错误场景

| 场景 | 行为 |
|---|---|
| Yoga prebuilt 下载失败 | 回退到行式 UI + warn |
| 非 TTY 环境（管道/CI） | 自动回退到行式 UI |
| 终端窗口过小（< 40x10） | warn + 继续运行（布局压缩） |
| Ink render 异常 | catch + 回退到行式 UI |
| 流式中用户 Ctrl+C | 中断 AgentLoop + 保存已完成部分 |
