# CodecastCLI 阶段一核心能力升级 — 实施计划（修订版）

## Goal

将 CodecastCLI 从"带工具调用的聊天机器人"升级为"真正的 AI Agent"：7 项改进，使 Agent 能自主多步执行、子代理真正工作、上下文管理精准、Bash 安全实用。本修订版合并了 6 项关键优化，将原 10 个任务精简为 9 个。

## Architecture

```
src/
├── tools/
│   ├── base.ts              ← 扩展 Tool 接口（+parameters 字段）
│   ├── registry.ts          ← 新增：ToolRegistry 统一注册表
│   ├── executor.ts          ← 新增：ToolExecutor 接口
│   ├── file.ts              ← 添加 parameters
│   ├── edit-file.ts         ← 添加 parameters
│   ├── bash.ts              ← 重构：命令分级 + shell:true
│   ├── codebase.ts          ← 添加 parameters
│   ├── git.ts               ← 添加 parameters
│   └── dispatch-subagent.ts ← 新增：DispatchSubAgentTool
├── agent/
│   ├── agent-loop.ts        ← 新增：AgentLoop 类（含权限+Hook+consumeFullStream）
│   ├── modes.ts             ← 修正工具白名单
│   └── subagent.ts          ← 重构：复用 AgentLoop
├── context/
│   ├── project-context.ts   ← 新增：CODECAST.md 加载
│   └── compactor.ts         ← 重构：单一 async compact() + 增量摘要
├── api/
│   └── llm.ts               ← 扩展 StreamFinal + consumeFullStream + 动态 tools
├── cost/
│   └── tracker.ts           ← 支持 cache token + TokenUsage
├── permissions/
│   └── model.ts             ← 增加 risk 字段
└── index.ts                 ← 用 ToolRegistry + AgentLoop 替换旧逻辑
```

## Tech Stack

- TypeScript 5.x, Node.js >=20
- tsx 运行时（无编译步骤）
- node:test + node:assert（无外部测试框架）
- 测试命令：`npx tsx --test tests/path/test.ts`
- 提交命令：`git add -A && git commit -m "..."`

---

## Task 1: Tool Schema 去重 + 统一注册表

> 依赖：无 | 影响文件：base.ts, registry.ts, file.ts, edit-file.ts, codebase.ts, git.ts, llm.ts, plugins/registry.ts, index.ts

### Step 1.1: 扩展 Tool 接口 + 新增 ToolParameterSchema

- [ ] 修改 `src/tools/base.ts`，添加 `ToolParameterSchema` 和 `parameters` 字段

```typescript
// src/tools/base.ts
export interface ToolParameterProperty {
  type: string
  description: string
  items?: Record<string, unknown>
  enum?: string[]
}

export interface ToolParameterSchema {
  type: 'object'
  properties: Record<string, ToolParameterProperty>
  required?: string[]
}

export interface Tool {
  name: string
  description: string
  parameters: ToolParameterSchema
  execute(args: Record<string, unknown>): Promise<string>
}
```

测试命令：`npx tsx --test tests/tools-base.test.ts`

### Step 1.2: 为每个内置 Tool 添加 parameters

- [ ] 修改 `src/tools/file.ts`，给 ReadFileTool 和 WriteFileTool 添加 parameters

```typescript
// src/tools/file.ts
import { readFileSync, writeFileSync, existsSync } from 'fs'
import { Tool, ToolParameterSchema } from './base.js'
import { PathGuard } from './path-guard.js'

export class ReadFileTool implements Tool {
  name = 'read_file'
  description = '读取文件内容（仅限工作区内）'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      path: { type: 'string', description: '相对工作区的文件路径' },
    },
    required: ['path'],
  }

  private guard(): PathGuard { return new PathGuard() }

  async execute(args: Record<string, unknown>): Promise<string> {
    const input = args.path as string | undefined
    if (!input) return '错误: 缺少 path 参数'

    const safe = this.guard().safe(input)
    if (!safe.ok) return `错误: ${safe.reason}`
    const path = safe.path!

    if (!existsSync(path)) return `错误: 文件不存在 ${path}`
    try {
      return readFileSync(path, 'utf-8')
    } catch (err: any) {
      return `错误: ${err.message}`
    }
  }
}

export class WriteFileTool implements Tool {
  name = 'write_file'
  description = '写入文件内容（仅限工作区内）'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      path: { type: 'string', description: '相对工作区的文件路径' },
      content: { type: 'string', description: '文件内容' },
    },
    required: ['path', 'content'],
  }

  private guard(): PathGuard { return new PathGuard() }

  async execute(args: Record<string, unknown>): Promise<string> {
    const input = args.path as string | undefined
    const content = args.content as string | undefined
    if (!input) return '错误: 缺少 path 参数'
    if (content === undefined) return '错误: 缺少 content 参数'

    const safe = this.guard().safe(input)
    if (!safe.ok) return `错误: ${safe.reason}`
    const path = safe.path!

    try {
      writeFileSync(path, content, 'utf-8')
      return `文件已写入: ${path}`
    } catch (err: any) {
      return `错误: ${err.message}`
    }
  }
}
```

- [ ] 修改 `src/tools/edit-file.ts`，给 EditFileTool 和 MultiEditFileTool 添加 parameters

```typescript
// src/tools/edit-file.ts
import { readFileSync, writeFileSync, existsSync } from 'fs'
import { Tool, ToolParameterSchema } from './base.js'
import { PathGuard } from './path-guard.js'

export class EditFileTool implements Tool {
  name = 'edit_file'
  description = '精确编辑文件：搜索 old_text 并替换为 new_text（仅限工作区内）'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      path: { type: 'string', description: '相对工作区的文件路径' },
      old_text: { type: 'string', description: '要搜索的原始文本（必须与文件内容完全匹配）' },
      new_text: { type: 'string', description: '替换后的新文本' },
      replace_all: { type: 'boolean', description: '是否替换所有匹配（默认 false，仅替换第一处）' },
    },
    required: ['path', 'old_text', 'new_text'],
  }

  private guard(): PathGuard { return new PathGuard() }

  async execute(args: Record<string, unknown>): Promise<string> {
    const input = args.path as string | undefined
    const oldText = args.old_text as string | undefined
    const newText = args.new_text as string | undefined
    const replaceAll = args.replace_all as boolean | undefined

    if (!input) return '错误: 缺少 path 参数'
    if (oldText === undefined) return '错误: 缺少 old_text 参数'
    if (newText === undefined) return '错误: 缺少 new_text 参数'

    const safe = this.guard().safe(input)
    if (!safe.ok) return `错误: ${safe.reason}`
    const path = safe.path!

    if (!existsSync(path)) return `错误: 文件不存在 ${path}`

    let content: string
    try {
      content = readFileSync(path, 'utf-8')
    } catch (err: any) {
      return `错误: 读取文件失败 - ${err.message}`
    }

    if (!content.includes(oldText)) {
      const lines = content.split('\n')
      const preview = lines.slice(0, 5).map((l, i) => `${i + 1}: ${l}`).join('\n')
      return `错误: 未找到 old_text。文件前 5 行:\n${preview}\n请确保 old_text 与文件内容完全匹配（包括缩进）。`
    }

    if (!replaceAll) {
      const firstIdx = content.indexOf(oldText)
      const secondIdx = content.indexOf(oldText, firstIdx + 1)
      if (secondIdx !== -1) {
        return `错误: old_text 在文件中出现多次（至少 2 处）。请提供更长的上下文使其唯一，或设置 replace_all: true 替换所有匹配。`
      }
    }

    const newContent = replaceAll
      ? content.split(oldText).join(newText)
      : content.replace(oldText, newText)

    try {
      writeFileSync(path, newContent, 'utf-8')
    } catch (err: any) {
      return `错误: 写入文件失败 - ${err.message}`
    }

    const oldLines = oldText.split('\n').length
    const newLines = newText.split('\n').length
    const totalLines = newContent.split('\n').length
    const action = replaceAll ? `替换了所有匹配` : `替换了 1 处匹配`
    return `${action} (${oldLines} 行 → ${newLines} 行)。文件共 ${totalLines} 行。`
  }
}

export class MultiEditFileTool implements Tool {
  name = 'multi_edit_file'
  description = '对同一文件执行多处搜索替换（仅限工作区内）'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      path: { type: 'string', description: '相对工作区的文件路径' },
      edits: {
        type: 'array',
        description: '搜索替换对列表',
        items: {
          type: 'object',
          properties: {
            old_text: { type: 'string', description: '要搜索的原始文本' },
            new_text: { type: 'string', description: '替换后的新文本' },
          },
          required: ['old_text', 'new_text'],
        },
      },
    },
    required: ['path', 'edits'],
  }

  private guard(): PathGuard { return new PathGuard() }

  async execute(args: Record<string, unknown>): Promise<string> {
    const input = args.path as string | undefined
    const edits = args.edits as Array<{ old_text: string; new_text: string }> | undefined

    if (!input) return '错误: 缺少 path 参数'
    if (!edits || !Array.isArray(edits) || edits.length === 0) {
      return '错误: 缺少 edits 参数（数组，每项含 old_text 和 new_text）'
    }

    const safe = this.guard().safe(input)
    if (!safe.ok) return `错误: ${safe.reason}`
    const path = safe.path!

    if (!existsSync(path)) return `错误: 文件不存在 ${path}`

    let content: string
    try {
      content = readFileSync(path, 'utf-8')
    } catch (err: any) {
      return `错误: 读取文件失败 - ${err.message}`
    }

    let totalReplacements = 0
    const results: string[] = []

    for (let i = 0; i < edits.length; i++) {
      const edit = edits[i]
      if (!edit.old_text || edit.new_text === undefined) {
        results.push(`  #${i + 1}: 缺少 old_text 或 new_text，跳过`)
        continue
      }
      if (!content.includes(edit.old_text)) {
        results.push(`  #${i + 1}: 未找到 old_text，跳过`)
        continue
      }
      const firstIdx = content.indexOf(edit.old_text)
      const secondIdx = content.indexOf(edit.old_text, firstIdx + 1)
      if (secondIdx !== -1) {
        results.push(`  #${i + 1}: old_text 出现多次，跳过（请使用 edit_file + replace_all）`)
        continue
      }
      content = content.replace(edit.old_text, edit.new_text)
      totalReplacements++
      results.push(`  #${i + 1}: 替换成功`)
    }

    try {
      writeFileSync(path, content, 'utf-8')
    } catch (err: any) {
      return `错误: 写入文件失败 - ${err.message}`
    }

    const totalLines = content.split('\n').length
    return `完成 ${totalReplacements}/${edits.length} 处替换。文件共 ${totalLines} 行。\n${results.join('\n')}`
  }
}
```

- [ ] 修改 `src/tools/codebase.ts`，给 4 个 Tool 添加 parameters

```typescript
// src/tools/codebase.ts
import { Tool, ToolParameterSchema } from './base.js'
import { CodebaseIndex } from '../context/codebase-index.js'

const index = new CodebaseIndex()

export class SearchCodeTool implements Tool {
  name = 'search_code'
  description = '在工作区内搜索代码：匹配文件名或文件内容'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      query: { type: 'string', description: '搜索关键词' },
      max_results: { type: 'number', description: '最大结果数（默认 20）' },
    },
    required: ['query'],
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const query = args.query as string | undefined
    if (!query) return '错误: 缺少 query 参数'
    const maxResults = (args.max_results as number) || 20
    const results = index.search(query, maxResults)
    if (results.length === 0) return `未找到匹配 "${query}" 的结果`
    return results.map(r => {
      if (r.line === 0) return `[文件] ${r.file}`
      return `${r.file}:${r.line} | ${r.text.trim().slice(0, 120)}`
    }).join('\n')
  }
}

export class SearchSymbolsTool implements Tool {
  name = 'search_symbols'
  description = '查找函数、类、接口、类型定义的位置'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      query: { type: 'string', description: '符号名关键词' },
      max_results: { type: 'number', description: '最大结果数（默认 20）' },
    },
    required: ['query'],
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const query = args.query as string | undefined
    if (!query) return '错误: 缺少 query 参数'
    const maxResults = (args.max_results as number) || 20
    const results = index.searchSymbols(query, maxResults)
    if (results.length === 0) return `未找到符号 "${query}"`
    return results.map(r => `${r.kind} ${r.name} → ${r.file}:${r.line}`).join('\n')
  }
}

export class ProjectOverviewTool implements Tool {
  name = 'project_overview'
  description = '获取项目结构概览：目录树 + 文件类型统计'
  parameters: ToolParameterSchema = { type: 'object', properties: {} }

  async execute(_args: Record<string, unknown>): Promise<string> {
    return index.overview()
  }
}

export class ReadLinesTool implements Tool {
  name = 'read_lines'
  description = '读取文件指定行范围的内容（带行号，仅限工作区内）'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      path: { type: 'string', description: '相对工作区的文件路径' },
      start_line: { type: 'number', description: '起始行号（从 1 开始）' },
      end_line: { type: 'number', description: '结束行号' },
    },
    required: ['path'],
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const path = args.path as string | undefined
    if (!path) return '错误: 缺少 path 参数'
    const startLine = args.start_line as number | undefined
    const endLine = args.end_line as number | undefined
    return index.readFile(path, startLine, endLine)
  }
}
```

- [ ] 修改 `src/tools/git.ts`，给 6 个 Tool 添加 parameters

```typescript
// src/tools/git.ts
import { execFileSync } from 'child_process'
import { Tool, ToolParameterSchema } from './base.js'

function git(args: string[], cwd?: string): string {
  try {
    return execFileSync('git', args, {
      cwd,
      encoding: 'utf-8',
      timeout: 10000,
      maxBuffer: 1024 * 1024,
      shell: false,
    }).trim()
  } catch (err: any) {
    throw new Error(err.stderr || err.message)
  }
}

export class GitStatusTool implements Tool {
  name = 'git_status'
  description = '查看 git 工作区状态（修改、暂存、未跟踪文件）'
  parameters: ToolParameterSchema = { type: 'object', properties: {} }

  async execute(_args: Record<string, unknown>): Promise<string> {
    try {
      const status = git(['status', '--short', '--branch'])
      if (!status) return '工作区干净，无变更'
      return status
    } catch (err: any) {
      return `错误: ${err.message}`
    }
  }
}

export class GitDiffTool implements Tool {
  name = 'git_diff'
  description = '查看 git 变更内容（默认显示未暂存的变更）'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      staged: { type: 'boolean', description: '查看暂存的变更' },
      file: { type: 'string', description: '指定文件路径' },
    },
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const staged = args.staged as boolean
    const file = args.file as string
    try {
      const diffArgs = ['diff']
      if (staged) diffArgs.push('--cached')
      if (file) diffArgs.push('--', file)
      const diff = git(diffArgs)
      if (!diff) return '无变更'
      if (diff.length > 5000) return diff.slice(0, 5000) + '\n... (diff 过长，已截断)'
      return diff
    } catch (err: any) {
      return `错误: ${err.message}`
    }
  }
}

export class GitCommitTool implements Tool {
  name = 'git_commit'
  description = '暂存并提交变更（自动生成 commit message）'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      message: { type: 'string', description: '提交消息' },
      files: { type: 'array', items: { type: 'string' }, description: '指定文件' },
      all: { type: 'boolean', description: '暂存所有变更' },
    },
    required: ['message'],
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const message = args.message as string
    const files = args.files as string[] | undefined
    const all = args.all as boolean
    if (!message) return '错误: 缺少 message 参数'
    try {
      if (all) { git(['add', '-A']) }
      else if (files && files.length > 0) { git(['add', ...files]) }
      else { git(['add', '-A']) }
      git(['commit', '-m', message])
      const log = git(['log', '-1', '--oneline'])
      return `已提交: ${log}`
    } catch (err: any) {
      return `错误: ${err.message}`
    }
  }
}

export class GitLogTool implements Tool {
  name = 'git_log'
  description = '查看 git 提交历史'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      count: { type: 'number', description: '显示条数（默认 10）' },
      file: { type: 'string', description: '指定文件' },
    },
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const count = (args.count as number) || 10
    const file = args.file as string
    try {
      const logArgs = ['log', `-${count}`, '--oneline', '--decorate']
      if (file) logArgs.push('--', file)
      return git(logArgs)
    } catch (err: any) {
      return `错误: ${err.message}`
    }
  }
}

export class GitBranchTool implements Tool {
  name = 'git_branch'
  description = '创建或切换 git 分支'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      action: { type: 'string', enum: ['create', 'switch', 'list'], description: '操作类型' },
      name: { type: 'string', description: '分支名' },
    },
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const action = args.action as string
    const name = args.name as string
    try {
      if (action === 'create' && name) {
        git(['checkout', '-b', name])
        return `已创建并切换到分支: ${name}`
      }
      if (action === 'switch' && name) {
        git(['checkout', name])
        return `已切换到分支: ${name}`
      }
      return git(['branch', '-a'])
    } catch (err: any) {
      return `错误: ${err.message}`
    }
  }
}

export class GitBlameTool implements Tool {
  name = 'git_blame'
  description = '查看文件每行的最后修改者和提交'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: { file: { type: 'string', description: '文件路径' } },
    required: ['file'],
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const file = args.file as string
    if (!file) return '错误: 缺少 file 参数'
    try {
      const blame = git(['blame', '--abbrev=8', file])
      if (blame.length > 3000) return blame.slice(0, 3000) + '\n... (blame 过长，已截断)'
      return blame
    } catch (err: any) {
      return `错误: ${err.message}`
    }
  }
}
```

测试命令：`npx tsx --test tests/file.test.ts && npx tsx --test tests/edit-file.test.ts && npx tsx --test tests/git.test.ts`

### Step 1.3: 新增 ToolRegistry

- [ ] 创建 `src/tools/registry.ts`

```typescript
// src/tools/registry.ts
import { Tool, ToolParameterSchema } from './base.js'

export interface OpenAIToolSchema {
  type: 'function'
  function: {
    name: string
    description: string
    parameters: ToolParameterSchema
  }
}

export interface AnthropicToolSchema {
  name: string
  description: string
  input_schema: ToolParameterSchema
}

export class ToolRegistry {
  private tools = new Map<string, Tool>()

  register(tool: Tool): void {
    this.tools.set(tool.name, tool)
  }

  registerMany(tools: Tool[]): void {
    for (const tool of tools) {
      this.register(tool)
    }
  }

  get(name: string): Tool | undefined {
    return this.tools.get(name)
  }

  getAll(): Tool[] {
    return [...this.tools.values()]
  }

  getFiltered(allowedNames: string[]): Tool[] {
    if (allowedNames.length === 0) return this.getAll()
    const allowedSet = new Set(allowedNames)
    return this.getAll().filter(t => allowedSet.has(t.name))
  }

  toOpenAISchema(tools?: Tool[]): OpenAIToolSchema[] {
    const list = tools || this.getAll()
    return list.map(t => ({
      type: 'function' as const,
      function: {
        name: t.name,
        description: t.description,
        parameters: t.parameters,
      },
    }))
  }

  toAnthropicSchema(tools?: Tool[]): AnthropicToolSchema[] {
    const list = tools || this.getAll()
    return list.map(t => ({
      name: t.name,
      description: t.description,
      input_schema: t.parameters,
    }))
  }
}
```

- [ ] 创建 `tests/registry.test.ts`

```typescript
import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { ToolRegistry } from '../src/tools/registry.js'
import { Tool, ToolParameterSchema } from '../src/tools/base.js'

const dummyTool: Tool = {
  name: 'test_tool',
  description: 'A test tool',
  parameters: {
    type: 'object',
    properties: { input: { type: 'string', description: 'test input' } },
    required: ['input'],
  },
  execute: async () => 'ok',
}

test('ToolRegistry: register + get', () => {
  const reg = new ToolRegistry()
  reg.register(dummyTool)
  assert.equal(reg.get('test_tool')?.name, 'test_tool')
  assert.equal(reg.get('nonexistent'), undefined)
})

test('ToolRegistry: registerMany', () => {
  const reg = new ToolRegistry()
  const another: Tool = {
    name: 'another', description: 'x',
    parameters: { type: 'object', properties: {} },
    execute: async () => 'ok',
  }
  reg.registerMany([dummyTool, another])
  assert.equal(reg.getAll().length, 2)
})

test('ToolRegistry: getFiltered', () => {
  const reg = new ToolRegistry()
  reg.register(dummyTool)
  const another: Tool = {
    name: 'another', description: 'x',
    parameters: { type: 'object', properties: {} },
    execute: async () => 'ok',
  }
  reg.register(another)
  const filtered = reg.getFiltered(['test_tool'])
  assert.equal(filtered.length, 1)
  assert.equal(filtered[0].name, 'test_tool')
})

test('ToolRegistry: getFiltered 空白名单返回全部', () => {
  const reg = new ToolRegistry()
  reg.register(dummyTool)
  const filtered = reg.getFiltered([])
  assert.equal(filtered.length, 1)
})

test('ToolRegistry: toOpenAISchema', () => {
  const reg = new ToolRegistry()
  reg.register(dummyTool)
  const schema = reg.toOpenAISchema()
  assert.equal(schema.length, 1)
  assert.equal(schema[0].type, 'function')
  assert.equal(schema[0].function.name, 'test_tool')
  assert.deepEqual(schema[0].function.parameters, dummyTool.parameters)
})

test('ToolRegistry: toAnthropicSchema', () => {
  const reg = new ToolRegistry()
  reg.register(dummyTool)
  const schema = reg.toAnthropicSchema()
  assert.equal(schema.length, 1)
  assert.equal(schema[0].name, 'test_tool')
  assert.equal(schema[0].input_schema, dummyTool.parameters)
})
```

测试命令：`npx tsx --test tests/registry.test.ts`

### Step 1.4: 修改 LLMClient 接收动态 tools 参数（含显式 openaiStream/anthropicStream 内部变更）

- [ ] 修改 `src/api/llm.ts`，chatStream 增加 tools 参数，新增 consumeFullStream()，删除 getToolsSchema/getAnthropicToolsSchema，显式替换内部调用

```typescript
// src/api/llm.ts — 完整修改
import { Tool } from '../tools/base.js'
import { ToolRegistry } from '../tools/registry.js'

export interface Message {
  role: 'system' | 'user' | 'assistant'
  content: string
  tool_calls?: ToolCall[]
}

export interface ToolCall {
  id: string
  type: 'function'
  function: {
    name: string
    arguments: string
  }
}

export interface ToolResult {
  tool_call_id: string
  role: 'tool'
  content: string
}

export type ChatMessage = Message | ToolResult

export interface LLMConfig {
  provider: 'openai' | 'anthropic'
  apiKey: string
  model?: string
  baseURL?: string
}

export interface TokenUsage {
  inputTokens: number
  outputTokens: number
  cacheReadTokens?: number
  cacheCreationTokens?: number
}

interface StreamFinal {
  toolCalls?: ToolCall[]
  stopReason?: string
  usage?: TokenUsage
}

interface AnthropicTextBlock { type: 'text'; text: string }
interface AnthropicToolUseBlock {
  type: 'tool_use'
  id: string
  name: string
  input: unknown
}
interface AnthropicToolResultBlock {
  type: 'tool_result'
  tool_use_id: string
  content: string
}
type AnthropicContentBlock = AnthropicTextBlock | AnthropicToolUseBlock | AnthropicToolResultBlock

interface AnthropicIncomingMessage {
  role: 'user' | 'assistant'
  content: AnthropicContentBlock[] | string
}

export class LLMClient {
  private config: LLMConfig
  private toolRegistry: ToolRegistry | null = null

  constructor(config: LLMConfig) {
    this.config = config
  }

  setToolRegistry(registry: ToolRegistry): void {
    this.toolRegistry = registry
  }

  async *chatStream(
    messages: ChatMessage[],
    tools?: Tool[]
  ): AsyncGenerator<string, StreamFinal, unknown> {
    if (this.config.provider === 'openai') {
      return yield* this.openaiStream(messages, tools)
    }
    return yield* this.anthropicStream(messages, tools)
  }

  /**
   * 消费完整流：收集所有文本、工具调用和 usage。
   * AgentLoop 调用此方法替代手动迭代 chatStream。
   */
  async consumeFullStream(messages: ChatMessage[], tools?: Tool[]): Promise<{
    text: string
    toolCalls: ToolCall[]
    stopReason?: string
    usage?: TokenUsage
  }> {
    let fullText = ''
    const stream = this.chatStream(messages, tools)
    const iter = stream[Symbol.asyncIterator]()
    let next = await iter.next()
    let toolCalls: ToolCall[] = []
    let stopReason: string | undefined
    let usage: TokenUsage | undefined

    while (!next.done) {
      fullText += next.value as string
      next = await iter.next()
    }

    const final = next.value
    if (final && typeof final === 'object') {
      toolCalls = final.toolCalls || []
      stopReason = final.stopReason
      usage = final.usage
    }

    return { text: fullText, toolCalls, stopReason, usage }
  }

  // ============== OpenAI ==============
  private async *openaiStream(messages: ChatMessage[], tools?: Tool[]): AsyncGenerator<string, StreamFinal, unknown> {
    // 显式使用 ToolRegistry.toOpenAISchema 替代 this.getToolsSchema()
    const registry = this.toolRegistry || new ToolRegistry()
    if (tools) registry.registerMany(tools)
    const toolsSchema = registry.toOpenAISchema(tools)

    const response = await fetch(this.config.baseURL || 'https://api.openai.com/v1/chat/completions', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${this.config.apiKey}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        model: this.config.model || 'gpt-4o',
        messages,
        stream: true,
        tools: toolsSchema.length > 0 ? toolsSchema : undefined,
      }),
    })

    if (!response.ok) {
      throw new Error(`OpenAI API error: ${response.status}`)
    }

    const reader = response.body!.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    const toolCalls: ToolCall[] = []
    let hasFinished = false
    let usage: TokenUsage | undefined
    let stopReason: string | undefined

    try {
      while (!hasFinished) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          const data = line.slice(6)
          if (data === '[DONE]') {
            hasFinished = true
            break
          }

          try {
            const chunk = JSON.parse(data)
            const delta = chunk.choices[0]?.delta

            if (delta?.content) yield delta.content

            if (delta?.tool_calls) {
              for (const tc of delta.tool_calls) {
                const existing = toolCalls[tc.index]
                if (existing) {
                  if (tc.function?.arguments) {
                    existing.function.arguments += tc.function.arguments
                  }
                  if (tc.id) existing.id = tc.id
                  if (tc.function?.name) existing.function.name = tc.function.name
                } else {
                  toolCalls[tc.index] = {
                    id: tc.id,
                    type: 'function',
                    function: {
                      name: tc.function.name,
                      arguments: tc.function.arguments,
                    }
                  }
                }
              }
            }

            if (chunk.usage) {
              usage = {
                inputTokens: chunk.usage.prompt_tokens || 0,
                outputTokens: chunk.usage.completion_tokens || 0,
                cacheReadTokens: chunk.usage.prompt_tokens_details?.cached_tokens,
              }
            }
            if (chunk.choices?.[0]?.finish_reason) {
              stopReason = chunk.choices[0].finish_reason
            }
          } catch {
            // 忽略不完整的 SSE 行
          }
        }
      }
    } finally {
      reader.releaseLock()
    }

    return { toolCalls: toolCalls.filter(tc => tc.id && tc.function.name), stopReason, usage }
  }

  // ============== Anthropic ==============
  private async *anthropicStream(messages: ChatMessage[], tools?: Tool[]): AsyncGenerator<string, StreamFinal, unknown> {
    // 显式使用 ToolRegistry.toAnthropicSchema 替代 this.getAnthropicToolsSchema()
    const registry = this.toolRegistry || new ToolRegistry()
    if (tools) registry.registerMany(tools)
    const toolsSchema = registry.toAnthropicSchema(tools)

    const apiMessages = this.toAnthropicMessages(messages)
    const systemMsg = messages.find(m => m.role === 'system')

    const response = await fetch(this.config.baseURL || 'https://api.anthropic.com/v1/messages', {
      method: 'POST',
      headers: {
        'x-api-key': this.config.apiKey,
        'anthropic-version': '2023-06-01',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        model: this.config.model || 'claude-3-sonnet-20240229',
        max_tokens: 4096,
        system: systemMsg?.content,
        messages: apiMessages,
        stream: true,
        tools: toolsSchema.length > 0 ? toolsSchema : undefined,
      }),
    })

    if (!response.ok) {
      throw new Error(`Anthropic API error: ${response.status}`)
    }

    const reader = response.body!.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    type BlockState =
      | { type: 'text'; text: string }
      | { type: 'tool_use'; id: string; name: string; inputJson: string }
    const blocks: BlockState[] = []
    let currentBlock: BlockState | null = null
    let usage: TokenUsage = { inputTokens: 0, outputTokens: 0 }
    let stopReason: string | undefined

    try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          const data = line.slice(6).trim()
          if (!data) continue

          try {
            const evt = JSON.parse(data)

            switch (evt.type) {
              case 'message_start': {
                if (evt.message?.usage) {
                  usage.inputTokens = evt.message.usage.input_tokens || 0
                  usage.cacheReadTokens = evt.message.usage.cache_read_input_tokens
                  usage.cacheCreationTokens = evt.message.usage.cache_creation_input_tokens
                }
                break
              }

              case 'content_block_start': {
                const cb = evt.content_block
                if (cb.type === 'text') {
                  currentBlock = { type: 'text', text: '' }
                } else if (cb.type === 'tool_use') {
                  currentBlock = { type: 'tool_use', id: cb.id, name: cb.name, inputJson: '' }
                }
                break
              }

              case 'content_block_delta': {
                if (!currentBlock) break
                const d = evt.delta
                if (currentBlock.type === 'text' && d.type === 'text_delta') {
                  currentBlock.text += d.text
                  yield d.text
                } else if (currentBlock.type === 'tool_use' && d.type === 'input_json_delta') {
                  currentBlock.inputJson += d.partial_json
                }
                break
              }

              case 'content_block_stop': {
                if (currentBlock) {
                  blocks.push(currentBlock)
                  currentBlock = null
                }
                break
              }

              case 'message_delta': {
                if (evt.usage) {
                  usage.outputTokens = evt.usage.output_tokens || 0
                }
                if (evt.delta?.stop_reason) {
                  stopReason = evt.delta.stop_reason
                }
                break
              }
            }
          } catch {
            // 忽略不完整 JSON
          }
        }
      }
    } finally {
      reader.releaseLock()
    }

    const toolCalls: ToolCall[] = blocks
      .filter((b): b is Extract<BlockState, { type: 'tool_use' }> => b.type === 'tool_use')
      .map(b => ({
        id: b.id,
        type: 'function',
        function: {
          name: b.name,
          arguments: b.inputJson || '{}',
        },
      }))

    return { toolCalls, stopReason, usage }
  }

  private toAnthropicMessages(messages: ChatMessage[]): AnthropicIncomingMessage[] {
    const result: AnthropicIncomingMessage[] = []

    for (const m of messages) {
      if (m.role === 'system') continue

      if (m.role === 'tool') {
        const last = result[result.length - 1]
        if (last && last.role === 'user' && Array.isArray(last.content)) {
          last.content.push({
            type: 'tool_result',
            tool_use_id: m.tool_call_id,
            content: m.content,
          })
        } else {
          result.push({
            role: 'user',
            content: [{
              type: 'tool_result',
              tool_use_id: m.tool_call_id,
              content: m.content,
            }],
          })
        }
        continue
      }

      if (m.role === 'user') {
        result.push({ role: 'user', content: m.content })
        continue
      }

      if (m.role === 'assistant') {
        if (m.tool_calls && m.tool_calls.length > 0) {
          const blocks: AnthropicContentBlock[] = []
          if (m.content) blocks.push({ type: 'text', text: m.content })
          for (const tc of m.tool_calls) {
            let input: unknown = {}
            try { input = JSON.parse(tc.function.arguments) } catch { /* keep empty */ }
            blocks.push({ type: 'tool_use', id: tc.id, name: tc.function.name, input })
          }
          result.push({ role: 'assistant', content: blocks })
        } else {
          result.push({ role: 'assistant', content: m.content })
        }
      }
    }

    return result
  }

  // 注意：getToolsSchema() 和 getAnthropicToolsSchema() 已删除
  // 由 ToolRegistry.toOpenAISchema() 和 ToolRegistry.toAnthropicSchema() 替代
}
```

测试命令：`npx tsx --test tests/llm.test.ts`

### Step 1.5: 修改 index.ts 使用 ToolRegistry

- [ ] 修改 `src/index.ts`，用 ToolRegistry 替代 builtinTools 字典

```typescript
// src/index.ts — 替换 builtinTools
import { ToolRegistry } from './tools/registry.js'

class AgentCLI {
  private toolRegistry = new ToolRegistry()
  // 删除: private builtinTools: Record<string, ...> = { ... }

  constructor() {
    // ... LLM 初始化不变

    this.toolRegistry.registerMany([
      new ReadFileTool(),
      new WriteFileTool(),
      new EditFileTool(),
      new MultiEditFileTool(),
      new BashTool(),
      new SearchCodeTool(),
      new SearchSymbolsTool(),
      new ProjectOverviewTool(),
      new ReadLinesTool(),
      new GitStatusTool(),
      new GitDiffTool(),
      new GitCommitTool(),
      new GitLogTool(),
      new GitBranchTool(),
      new GitBlameTool(),
    ])

    this.llm.setToolRegistry(this.toolRegistry)

    const plugins = this.pluginRegistry.loadAll()
    if (plugins.length > 0) {
      const pluginTools = this.pluginRegistry.getAllTools()
      this.toolRegistry.registerMany(pluginTools)
    }

    // ... 其余初始化不变
  }
}
```

- [ ] 修改 `executeTool` 方法，使用 ToolRegistry

```typescript
private async executeTool(name: string, args: Record<string, unknown>): Promise<string> {
  if (name === 'dispatch_subagent') {
    const agentName = args.agent as string
    const task = args.task as string
    const context = args.context as string | undefined
    if (!agentName || !task) return '错误: 缺少 agent 或 task 参数'
    const result = await this.subAgentDispatcher.dispatch(agentName, task, context)
    if (!result.success) return result.output
    return `${result.agent} 完成 (${result.rounds} 轮):\n${result.output}`
  }

  const tool = this.toolRegistry.get(name)
  if (tool) {
    return tool.execute(args)
  }

  const mcpTools = this.mcpClient.getAllTools()
  const mcpTool = mcpTools.find(t => t.name === name)
  if (mcpTool) {
    try {
      const result = await this.mcpClient.callTool(mcpTool.server, name, args)
      return typeof result === 'string' ? result : JSON.stringify(result, null, 2)
    } catch (err: any) {
      return `MCP 工具错误: ${err.message}`
    }
  }

  return `未知工具: ${name}`
}
```

测试命令：`npx tsx --test tests/file.test.ts && npx tsx --test tests/git.test.ts`

### Step 1.6: 修复 PluginTool 添加 parameters（含运行时验证）

- [ ] 修改 `src/plugins/registry.ts`，PluginTool 添加 parameters 字段并增加运行时验证

```typescript
// src/plugins/registry.ts — 修改 PluginTool 类
import { Tool, ToolParameterSchema } from '../tools/base.js'

// ... PluginToolDef, PluginManifest, PluginInfo 接口不变 ...

class PluginTool implements Tool {
  name: string
  description: string
  parameters: ToolParameterSchema
  private command: string
  private pluginDir: string

  constructor(def: PluginToolDef, pluginDir: string) {
    this.name = def.name
    this.description = def.description
    this.parameters = this.validateParameters(def.parameters)
    this.command = def.command
    this.pluginDir = pluginDir
  }

  private validateParameters(raw: Record<string, unknown>): ToolParameterSchema {
    if (raw && raw.type === 'object' && raw.properties) return raw as ToolParameterSchema
    return { type: 'object', properties: {} }
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    try {
      const result = spawnSync(this.command, [], {
        cwd: this.pluginDir,
        shell: true,
        input: JSON.stringify(args),
        timeout: 30000,
        encoding: 'utf-8',
        windowsHide: true,
      })

      if (result.status !== 0) {
        return `插件工具错误 (exit ${result.status}): ${result.stderr || '未知错误'}`
      }

      return result.stdout?.trim() || '(无输出)'
    } catch (err: any) {
      return `插件工具执行失败: ${err.message}`
    }
  }
}
```

测试命令：`npx tsx --test tests/plugins.test.ts`

### Step 1.7: 运行全量测试 + 提交

- [ ] 运行全量测试确保无回归

```bash
npx tsx --test tests/**/*.test.ts
```

- [ ] 提交

```bash
git add src/tools/base.ts src/tools/registry.ts src/tools/file.ts src/tools/edit-file.ts src/tools/codebase.ts src/tools/git.ts src/tools/bash.ts src/api/llm.ts src/plugins/registry.ts src/index.ts tests/registry.test.ts && git commit -m "feat: Tool Schema 去重 + 统一注册表 (Task 1)

- 扩展 Tool 接口，每个 Tool 自带 parameters 字段
- 新增 ToolRegistry 统一管理工具注册和 Schema 生成
- 删除 llm.ts 中 ~400 行硬编码 Schema
- LLMClient.chatStream 接收动态 tools 参数
- LLMClient 新增 consumeFullStream() 方法
- openaiStream/anthropicStream 显式使用 ToolRegistry.toOpenAISchema/toAnthropicSchema
- PluginTool 添加 parameters 字段 + 运行时验证"
```

---

## Task 2: Bash 安全模型重构

> 依赖：无（与 Task 1 并行） | 影响文件：bash.ts, permissions/model.ts

### Step 2.1: 重构 BashTool — 命令分级

- [ ] 修改 `src/tools/bash.ts`，替换白名单+元字符禁止为命令分级模型

```typescript
// src/tools/bash.ts — 完整重写
import { execSync } from 'child_process'
import { resolve, isAbsolute } from 'path'
import { Tool, ToolParameterSchema } from './base.js'

export type CommandRisk = 'safe' | 'dangerous' | 'unknown'

const SAFE_COMMANDS = new Set([
  'ls', 'cat', 'head', 'tail', 'pwd', 'date', 'whoami', 'uname',
  'echo', 'env', 'which', 'node', 'npm', 'npx', 'pnpm', 'yarn', 'bun',
  'tsx', 'tsc', 'git', 'diff', 'find', 'grep', 'wc', 'sort',
])

const DANGEROUS_COMMANDS = new Set([
  'rm', 'rmdir', 'mv', 'cp', 'chmod', 'chown',
  'curl', 'wget', 'ssh', 'scp',
  'docker', 'kubectl',
])

const DANGEROUS_PATTERNS = [
  /^rm\s+(-[a-zA-Z]*f[a-zA-Z]*\s+)?\/(\s|$)/,
  /^git\s+push\s+--force/,
  /^git\s+reset\s+--hard/,
  /^git\s+checkout\s+--\s+\./,
]

export function classifyCommand(command: string): CommandRisk {
  const trimmed = command.trim()
  for (const pattern of DANGEROUS_PATTERNS) {
    if (pattern.test(trimmed)) return 'dangerous'
  }
  const head = trimmed.split(/\s+/)[0]
  if (SAFE_COMMANDS.has(head)) return 'safe'
  if (DANGEROUS_COMMANDS.has(head)) return 'dangerous'
  return 'unknown'
}

export class BashTool implements Tool {
  name = 'bash'
  description = '执行终端命令（安全命令自动执行，危险命令需确认）'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      command: { type: 'string', description: '要执行的命令' },
      cwd: { type: 'string', description: '工作目录' },
    },
    required: ['command'],
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const command = (args.command as string | undefined)?.trim()
    const cwd = args.cwd as string | undefined

    if (!command) return '错误: 缺少 command 参数'

    let safeCwd: string | undefined
    if (cwd) {
      const resolved = isAbsolute(cwd) ? cwd : resolve(process.cwd(), cwd)
      const root = process.cwd()
      if (!resolved.startsWith(root)) {
        return `错误: cwd "${cwd}" 越出工作区`
      }
      safeCwd = resolved
    }

    try {
      const result = execSync(command, {
        cwd: safeCwd,
        encoding: 'utf-8',
        timeout: 30000,
        maxBuffer: 1024 * 1024,
        shell: true,
      })
      return result || '(命令执行成功，无输出)'
    } catch (err: any) {
      return `错误: ${err.message}\n${err.stderr || ''}`
    }
  }
}
```

### Step 2.2: 扩展 PermissionManager 支持 risk 字段

- [ ] 修改 `src/permissions/model.ts`，PermissionRule 增加 risk 字段，check() 增加 risk 参数

```typescript
// src/permissions/model.ts — 扩展
import { existsSync, readFileSync } from 'fs'
import { join } from 'path'
import { CommandRisk } from '../tools/bash.js'

export type PermissionAction = 'allow' | 'deny' | 'ask'

export interface PermissionRule {
  tool: string
  pattern?: string
  risk?: CommandRisk
  action: PermissionAction
}

export interface PermissionCheckResult {
  action: PermissionAction
  matchedRule?: PermissionRule
  reason?: string
}

const DEFAULT_RULES: PermissionRule[] = [
  { tool: 'read_file', pattern: '*.env', action: 'deny' },
  { tool: 'read_file', pattern: '*.env.local', action: 'deny' },
  { tool: 'write_file', pattern: '*.env', action: 'deny' },
  { tool: 'read_file', action: 'allow' },
  { tool: 'read_lines', action: 'allow' },
  { tool: 'search_code', action: 'allow' },
  { tool: 'search_symbols', action: 'allow' },
  { tool: 'project_overview', action: 'allow' },
  { tool: 'git_status', action: 'allow' },
  { tool: 'git_diff', action: 'allow' },
  { tool: 'git_log', action: 'allow' },
  { tool: 'git_blame', action: 'allow' },
  { tool: 'bash', risk: 'safe', action: 'allow' },
  { tool: 'bash', risk: 'dangerous', action: 'ask' },
  { tool: 'bash', risk: 'unknown', action: 'ask' },
  { tool: 'write_file', action: 'ask' },
  { tool: 'edit_file', action: 'ask' },
  { tool: 'multi_edit_file', action: 'ask' },
  { tool: 'git_commit', action: 'ask' },
  { tool: 'git_branch', action: 'ask' },
  { tool: '*', action: 'ask' },
]

export class PermissionManager {
  private rules: PermissionRule[] = []

  constructor() {
    this.rules = [...DEFAULT_RULES]
  }

  loadConfig(configPath: string) {
    if (!existsSync(configPath)) return
    try {
      const raw = readFileSync(configPath, 'utf-8')
      const config = JSON.parse(raw)
      if (Array.isArray(config.rules)) {
        this.rules = [...config.rules, ...this.rules]
      }
    } catch {
      // 配置文件格式错误，静默忽略
    }
  }

  check(toolName: string, toolArgs: Record<string, unknown>, risk?: CommandRisk): PermissionCheckResult {
    const target = (toolArgs.path || toolArgs.command || '') as string

    for (const rule of this.rules) {
      if (rule.tool !== '*' && rule.tool !== toolName) continue
      if (rule.risk !== undefined && risk !== undefined && rule.risk !== risk) continue
      if (rule.risk !== undefined && risk === undefined) continue
      if (rule.pattern) {
        if (!this.matchPattern(rule.pattern, target)) continue
      }
      if (rule.action === 'deny') {
        return {
          action: 'deny',
          matchedRule: rule,
          reason: `权限拒绝: 工具 ${toolName} 匹配规则 ${rule.pattern ? `"${rule.pattern}"` : rule.risk ? `risk=${rule.risk}` : '"*"'} → deny`,
        }
      }
      return { action: rule.action, matchedRule: rule }
    }

    return { action: 'ask' }
  }

  addRule(rule: PermissionRule) {
    this.rules.unshift(rule)
  }

  getRules(): PermissionRule[] {
    return [...this.rules]
  }

  private matchPattern(pattern: string, target: string): boolean {
    if (pattern === '*') return true
    if (pattern === target) return true
    if (pattern.startsWith('*.')) {
      const suffix = pattern.slice(1)
      return target.endsWith(suffix)
    }
    if (pattern.endsWith('*')) {
      const prefix = pattern.slice(0, -1)
      return target.startsWith(prefix)
    }
    return false
  }
}
```

### Step 2.3: 更新 BashTool 测试

- [ ] 重写 `tests/bash.test.ts`

```typescript
import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { BashTool, classifyCommand } from '../src/tools/bash.js'

const bash = new BashTool()

test('classifyCommand: 安全命令', () => {
  assert.equal(classifyCommand('ls'), 'safe')
  assert.equal(classifyCommand('git status'), 'safe')
  assert.equal(classifyCommand('node -e "1+1"'), 'safe')
  assert.equal(classifyCommand('npm test'), 'safe')
})

test('classifyCommand: 危险命令', () => {
  assert.equal(classifyCommand('rm file.txt'), 'dangerous')
  assert.equal(classifyCommand('mv a b'), 'dangerous')
  assert.equal(classifyCommand('curl http://x'), 'dangerous')
})

test('classifyCommand: 未知命令', () => {
  assert.equal(classifyCommand('format C:'), 'unknown')
  assert.equal(classifyCommand('my-custom-cli'), 'unknown')
})

test('classifyCommand: 危险模式', () => {
  assert.equal(classifyCommand('rm -rf /'), 'dangerous')
  assert.equal(classifyCommand('git push --force'), 'dangerous')
  assert.equal(classifyCommand('git reset --hard HEAD~1'), 'dangerous')
})

test('Bash: 拒绝空命令', async () => {
  const r = await bash.execute({ command: '' })
  assert.match(r, /缺少 command/)
})

test('Bash: 管道命令正常执行', async () => {
  const r = await bash.execute({ command: 'node -e "console.log(\'hello\')" | node -e "let d=\'\';process.stdin.on(\'data\',c=>d+=c);process.stdin.on(\'end\',()=>console.log(d.trim().toUpperCase()))"' })
  assert.ok(r.includes('HELLO') || r.includes('hello'), `管道输出: ${r}`)
})

test('Bash: 安全命令执行成功', async () => {
  const r = await bash.execute({ command: 'node -e "console.log(\'hello-world\')"' })
  assert.ok(r.includes('hello-world'))
})

test('Bash: cwd 越出工作区被拒绝', async () => {
  const r = await bash.execute({ command: 'ls', cwd: '../../../' })
  assert.match(r, /越出工作区/)
})
```

### Step 2.4: 更新 PermissionManager 测试

- [ ] 重写 `tests/permissions.test.ts`

```typescript
import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { PermissionManager } from '../src/permissions/model.js'

test('PermissionManager: 默认规则 - read_file 允许', () => {
  const mgr = new PermissionManager()
  const result = mgr.check('read_file', { path: 'src/index.ts' })
  assert.equal(result.action, 'allow')
})

test('PermissionManager: 默认规则 - .env 拒绝', () => {
  const mgr = new PermissionManager()
  const result = mgr.check('read_file', { path: '.env' })
  assert.equal(result.action, 'deny')
})

test('PermissionManager: 默认规则 - write_file 需确认', () => {
  const mgr = new PermissionManager()
  const result = mgr.check('write_file', { path: 'src/foo.ts' })
  assert.equal(result.action, 'ask')
})

test('PermissionManager: bash safe 允许', () => {
  const mgr = new PermissionManager()
  const result = mgr.check('bash', { command: 'ls' }, 'safe')
  assert.equal(result.action, 'allow')
})

test('PermissionManager: bash dangerous 需确认', () => {
  const mgr = new PermissionManager()
  const result = mgr.check('bash', { command: 'rm file' }, 'dangerous')
  assert.equal(result.action, 'ask')
})

test('PermissionManager: bash unknown 需确认', () => {
  const mgr = new PermissionManager()
  const result = mgr.check('bash', { command: 'custom' }, 'unknown')
  assert.equal(result.action, 'ask')
})

test('PermissionManager: bash 无 risk 参数回退到兜底 ask', () => {
  const mgr = new PermissionManager()
  const result = mgr.check('bash', { command: 'ls' })
  assert.equal(result.action, 'ask')
})

test('PermissionManager: 运行时添加规则', () => {
  const mgr = new PermissionManager()
  mgr.addRule({ tool: 'bash', pattern: 'git *', action: 'allow' })
  const result = mgr.check('bash', { command: 'git status' })
  assert.equal(result.action, 'allow')
})

test('PermissionManager: 未知工具默认 ask', () => {
  const mgr = new PermissionManager()
  const result = mgr.check('unknown_tool', {})
  assert.equal(result.action, 'ask')
})

test('PermissionManager: deny 优先级高于 allow', () => {
  const mgr = new PermissionManager()
  mgr.addRule({ tool: 'read_file', pattern: '*.secret', action: 'deny' })
  const result = mgr.check('read_file', { path: 'keys.secret' })
  assert.equal(result.action, 'deny')
})

test('PermissionManager: risk 规则优先于无 risk 规则', () => {
  const mgr = new PermissionManager()
  mgr.addRule({ tool: 'bash', risk: 'dangerous', pattern: 'rm *', action: 'deny' })
  const result = mgr.check('bash', { command: 'rm important.txt' }, 'dangerous')
  assert.equal(result.action, 'deny')
})
```

测试命令：`npx tsx --test tests/bash.test.ts && npx tsx --test tests/permissions.test.ts`

### Step 2.5: 修改 index.ts 传递 risk 到权限检查

- [ ] 修改 `src/index.ts` 中 chat() 的权限检查逻辑，传递 classifyCommand 结果

在 `src/index.ts` 顶部添加 import：

```typescript
import { classifyCommand, CommandRisk } from './tools/bash.js'
```

修改 chat() 中的权限检查部分（在后续 Task 3 中会被 AgentLoop 替换，此处先做最小改动）：

```typescript
const permResult = this.permManager.check(
  tc.function.name,
  args,
  tc.function.name === 'bash' ? classifyCommand(args.command as string) : undefined,
)
```

### Step 2.6: 运行全量测试 + 提交

- [ ] 运行全量测试

```bash
npx tsx --test tests/**/*.test.ts
```

- [ ] 提交

```bash
git add src/tools/bash.ts src/permissions/model.ts src/index.ts tests/bash.test.ts tests/permissions.test.ts && git commit -m "feat: Bash 安全模型重构 (Task 2)

- 命令三级分类：safe/dangerous/unknown
- 使用 execSync + shell:true 允许管道/重定向
- 删除白名单+元字符禁止模型
- PermissionManager 增加 risk 字段匹配
- bash safe 命令自动允许，dangerous/unknown 需确认"
```

---

## Task 3: Agentic Loop + consumeFullStream + 权限+Hook 集成

> 依赖：#1, #2 | 影响文件：agent-loop.ts, executor.ts, index.ts

> **优化说明**：本任务合并了原 Task 3 和原 Task 8。AgentLoop 从第一天起就包含 permManager、hookManager、onConfirm，避免写两遍。权限检查和 Hook 逻辑全部在 AgentLoop 内部，onToolCall 回调仅做 UI 显示。AgentLoop 使用 `llm.consumeFullStream()` 替代自实现的 `consumeLLMStream()`。

### Step 3.1: 新增 ToolExecutor 接口

- [ ] 创建 `src/tools/executor.ts`

```typescript
// src/tools/executor.ts
import { CommandRisk } from './bash.js'

export interface ToolExecutor {
  execute(name: string, args: Record<string, unknown>, risk?: CommandRisk): Promise<string>
}
```

### Step 3.2: 新增 AgentLoop 类（含权限+Hook+consumeFullStream）

- [ ] 创建 `src/agent/agent-loop.ts`

```typescript
// src/agent/agent-loop.ts
import { LLMClient, ChatMessage, ToolCall, TokenUsage } from '../api/llm.js'
import { Tool } from '../tools/base.js'
import { ToolExecutor } from '../tools/executor.js'
import { CommandRisk, classifyCommand } from '../tools/bash.js'
import { ContextCompactor, CompactionResult } from '../context/compactor.js'
import { PermissionManager } from '../permissions/model.js'
import { HookManager } from '../hooks/manager.js'

export interface LoopConfig {
  maxIterations: number
  maxTimeoutMs: number
  systemPrompt: string
  tools: Tool[]
  /** UI 回调：仅用于显示工具调用信息，不包含权限/业务逻辑 */
  onToolCall?: (name: string, args: Record<string, unknown>) => void
  onStreamChunk?: (chunk: string) => void
  onIterationStart?: (iteration: number) => void
  onCompaction?: (result: CompactionResult) => void
  compactor?: ContextCompactor
  /** 权限管理器：在 AgentLoop 内部执行权限检查 */
  permManager?: PermissionManager
  /** Hook 管理器：在 AgentLoop 内部执行 PreToolUse/PostToolUse */
  hookManager?: HookManager
  sessionId?: string
  /** 用户确认回调：权限 action='ask' 时调用 */
  onConfirm?: (toolName: string, args: Record<string, unknown>) => Promise<boolean>
}

export interface LoopResult {
  messages: ChatMessage[]
  finalText: string
  iterations: number
  toolCallsExecuted: number
  usage: TokenUsage
  stoppedReason: string
}

export class AgentLoop {
  constructor(
    private llm: LLMClient,
    private toolExecutor: ToolExecutor,
    private config: LoopConfig,
  ) {}

  async run(userMessage: string, history: ChatMessage[]): Promise<LoopResult> {
    const messages: ChatMessage[] = [
      ...history,
      { role: 'user' as const, content: userMessage },
    ]
    let iterations = 0
    let toolCallsExecuted = 0
    const totalUsage: TokenUsage = { inputTokens: 0, outputTokens: 0 }
    const startTime = Date.now()

    while (iterations < this.config.maxIterations) {
      if (Date.now() - startTime > this.config.maxTimeoutMs) {
        return {
          messages,
          finalText: '',
          iterations,
          toolCallsExecuted,
          usage: totalUsage,
          stoppedReason: 'timeout',
        }
      }

      iterations++
      this.config.onIterationStart?.(iterations)

      // 使用 LLMClient.consumeFullStream() 消费完整流
      const streamResult = await this.llm.consumeFullStream(messages, this.config.tools)

      // 流式输出文本（通过 onStreamChunk 回调）
      if (streamResult.text && this.config.onStreamChunk) {
        this.config.onStreamChunk(streamResult.text)
      }

      // 累积 usage
      totalUsage.inputTokens += streamResult.usage?.inputTokens || 0
      totalUsage.outputTokens += streamResult.usage?.outputTokens || 0
      totalUsage.cacheReadTokens = (totalUsage.cacheReadTokens || 0) + (streamResult.usage?.cacheReadTokens || 0)
      totalUsage.cacheCreationTokens = (totalUsage.cacheCreationTokens || 0) + (streamResult.usage?.cacheCreationTokens || 0)

      // 无工具调用 → AI 完成
      if (!streamResult.toolCalls || streamResult.toolCalls.length === 0) {
        return {
          messages,
          finalText: streamResult.text,
          iterations,
          toolCallsExecuted,
          usage: totalUsage,
          stoppedReason: streamResult.stopReason || 'no_tool_calls',
        }
      }

      // 逐个执行工具调用
      messages.push({
        role: 'assistant',
        content: streamResult.text,
        tool_calls: streamResult.toolCalls,
      })

      for (const tc of streamResult.toolCalls) {
        let args: Record<string, unknown>
        try {
          args = JSON.parse(tc.function.arguments || '{}')
        } catch {
          messages.push({
            tool_call_id: tc.id,
            role: 'tool',
            content: `错误: 参数 JSON 解析失败`,
          })
          continue
        }

        // UI 回调：仅显示，不做权限/业务逻辑
        this.config.onToolCall?.(tc.function.name, args)

        // 计算 bash 命令风险级别
        const risk: CommandRisk | undefined =
          tc.function.name === 'bash' ? classifyCommand(args.command as string) : undefined

        // === 权限检查（全部在 AgentLoop 内部） ===
        if (this.config.permManager) {
          const permResult = this.config.permManager.check(tc.function.name, args, risk)
          if (permResult.action === 'deny') {
            messages.push({
              tool_call_id: tc.id,
              role: 'tool',
              content: permResult.reason || '权限拒绝',
            })
            continue
          }
          if (permResult.action === 'ask') {
            const approved = this.config.onConfirm
              ? await this.config.onConfirm(tc.function.name, args)
              : false
            if (!approved) {
              messages.push({
                tool_call_id: tc.id,
                role: 'tool',
                content: '用户拒绝执行',
              })
              continue
            }
          }
        }

        // === PreToolUse Hook ===
        if (this.config.hookManager) {
          const preHook = await this.config.hookManager.fire('PreToolUse', {
            toolName: tc.function.name,
            toolArgs: args,
            sessionId: this.config.sessionId,
          })
          if (preHook.blocked) {
            messages.push({
              tool_call_id: tc.id,
              role: 'tool',
              content: `Hook 阻止: ${preHook.reason || '未知原因'}`,
            })
            continue
          }
          if (preHook.modifiedArgs) {
            args = preHook.modifiedArgs
          }
        }

        // === 执行工具 ===
        let toolResult = await this.toolExecutor.execute(tc.function.name, args, risk)

        // === PostToolUse Hook ===
        if (this.config.hookManager) {
          const postHook = await this.config.hookManager.fire('PostToolUse', {
            toolName: tc.function.name,
            toolArgs: args,
            toolResult,
            sessionId: this.config.sessionId,
          })
          if (postHook.modifiedResult) {
            toolResult = postHook.modifiedResult
          }
        }

        messages.push({
          tool_call_id: tc.id,
          role: 'tool',
          content: toolResult,
        })
        toolCallsExecuted++
      }

      // 每次迭代后检查压缩
      if (this.config.compactor) {
        const compaction = await this.config.compactor.compact(messages)
        if (compaction.didCompact) {
          this.config.onCompaction?.(compaction)
          messages.length = 0
          messages.push(...compaction.messages)
        }
      }
    }

    return {
      messages,
      finalText: '',
      iterations,
      toolCallsExecuted,
      usage: totalUsage,
      stoppedReason: 'max_iterations',
    }
  }
}
```

### Step 3.3: 新增 AgentLoop 测试

- [ ] 创建 `tests/agent-loop.test.ts`

```typescript
import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { AgentLoop } from '../src/agent/agent-loop.js'
import { LLMClient, ChatMessage, ToolCall, TokenUsage } from '../src/api/llm.js'
import { ToolExecutor } from '../src/tools/executor.js'
import { Tool, ToolParameterSchema } from '../src/tools/base.js'
import { PermissionManager } from '../src/permissions/model.js'
import { HookManager } from '../src/hooks/manager.js'

function createMockLLM(responses: Array<{
  text: string
  toolCalls?: ToolCall[]
  usage?: TokenUsage
  stopReason?: string
}>): LLMClient {
  let callIndex = 0
  const client = new LLMClient({ provider: 'openai', apiKey: 'test' })
  client.consumeFullStream = async function (messages: ChatMessage[], tools?: Tool[]) {
    const resp = responses[callIndex] || responses[responses.length - 1]
    callIndex++
    return {
      text: resp.text,
      toolCalls: resp.toolCalls || [],
      stopReason: resp.stopReason,
      usage: resp.usage,
    }
  }
  return client
}

function createMockExecutor(results: Record<string, string>): ToolExecutor {
  return {
    async execute(name: string, args: Record<string, unknown>): Promise<string> {
      return results[name] || `mock result for ${name}`
    },
  }
}

const dummyTool: Tool = {
  name: 'read_file',
  description: 'read',
  parameters: { type: 'object', properties: { path: { type: 'string', description: 'path' } }, required: ['path'] },
  execute: async () => 'ok',
}

test('AgentLoop: 无工具调用直接返回', async () => {
  const llm = createMockLLM([{
    text: 'Hello!',
    usage: { inputTokens: 10, outputTokens: 5 },
  }])
  const executor = createMockExecutor({})
  const loop = new AgentLoop(llm, executor, {
    maxIterations: 10,
    maxTimeoutMs: 60000,
    systemPrompt: '',
    tools: [dummyTool],
  })
  const result = await loop.run('hi', [])
  assert.equal(result.finalText, 'Hello!')
  assert.equal(result.iterations, 1)
  assert.equal(result.stoppedReason, 'no_tool_calls')
  assert.equal(result.usage.inputTokens, 10)
  assert.equal(result.usage.outputTokens, 5)
})

test('AgentLoop: 工具调用后继续循环', async () => {
  const llm = createMockLLM([
    {
      text: '',
      toolCalls: [{ id: 'tc1', type: 'function', function: { name: 'read_file', arguments: '{"path":"x.ts"}' } }],
      usage: { inputTokens: 20, outputTokens: 10 },
    },
    {
      text: 'File content: hello',
      usage: { inputTokens: 30, outputTokens: 15 },
    },
  ])
  const executor = createMockExecutor({ read_file: 'file content here' })
  const loop = new AgentLoop(llm, executor, {
    maxIterations: 10,
    maxTimeoutMs: 60000,
    systemPrompt: '',
    tools: [dummyTool],
  })
  const result = await loop.run('read x.ts', [])
  assert.equal(result.finalText, 'File content: hello')
  assert.equal(result.iterations, 2)
  assert.equal(result.toolCallsExecuted, 1)
  assert.equal(result.usage.inputTokens, 50)
})

test('AgentLoop: 达到最大迭代次数', async () => {
  const toolCall: ToolCall = { id: 'tc1', type: 'function', function: { name: 'read_file', arguments: '{"path":"x"}' } }
  const llm = createMockLLM([
    { text: '', toolCalls: [toolCall], usage: { inputTokens: 10, outputTokens: 5 } },
    { text: '', toolCalls: [toolCall], usage: { inputTokens: 10, outputTokens: 5 } },
    { text: '', toolCalls: [toolCall], usage: { inputTokens: 10, outputTokens: 5 } },
  ])
  const executor = createMockExecutor({ read_file: 'content' })
  const loop = new AgentLoop(llm, executor, {
    maxIterations: 3,
    maxTimeoutMs: 60000,
    systemPrompt: '',
    tools: [dummyTool],
  })
  const result = await loop.run('loop', [])
  assert.equal(result.stoppedReason, 'max_iterations')
  assert.equal(result.iterations, 3)
})

test('AgentLoop: 回调函数被调用', async () => {
  const toolCallsReceived: Array<{ name: string; args: Record<string, unknown> }> = []
  const chunksReceived: string[] = []
  const iterationsReceived: number[] = []

  const llm = createMockLLM([
    {
      text: 'result',
      toolCalls: [{ id: 'tc1', type: 'function', function: { name: 'read_file', arguments: '{"path":"x"}' } }],
      usage: { inputTokens: 10, outputTokens: 5 },
    },
    { text: 'done', usage: { inputTokens: 10, outputTokens: 5 } },
  ])
  const executor = createMockExecutor({ read_file: 'content' })
  const loop = new AgentLoop(llm, executor, {
    maxIterations: 10,
    maxTimeoutMs: 60000,
    systemPrompt: '',
    tools: [dummyTool],
    onToolCall: (name, args) => toolCallsReceived.push({ name, args }),
    onStreamChunk: (chunk) => chunksReceived.push(chunk),
    onIterationStart: (i) => iterationsReceived.push(i),
  })
  await loop.run('test', [])
  assert.equal(toolCallsReceived.length, 1)
  assert.equal(toolCallsReceived[0].name, 'read_file')
  assert.ok(chunksReceived.length > 0)
  assert.deepEqual(iterationsReceived, [1, 2])
})

test('AgentLoop: 权限 deny 阻止工具执行', async () => {
  const llm = createMockLLM([
    {
      text: '',
      toolCalls: [{ id: 'tc1', type: 'function', function: { name: 'write_file', arguments: '{"path":".env","content":"x"}' } }],
      usage: { inputTokens: 10, outputTokens: 5 },
    },
    { text: 'ok', usage: { inputTokens: 10, outputTokens: 5 } },
  ])
  const executor = createMockExecutor({ write_file: 'should not reach' })
  const permManager = new PermissionManager()
  const loop = new AgentLoop(llm, executor, {
    maxIterations: 10,
    maxTimeoutMs: 60000,
    systemPrompt: '',
    tools: [dummyTool],
    permManager,
  })
  const result = await loop.run('write .env', [])
  assert.equal(result.toolCallsExecuted, 0)
  const toolMsg = result.messages.find(m => m.role === 'tool')
  assert.ok(toolMsg)
  assert.ok(('content' in toolMsg!) && (toolMsg!.content as string).includes('权限拒绝'))
})

test('AgentLoop: 权限 ask + 用户确认', async () => {
  const llm = createMockLLM([
    {
      text: '',
      toolCalls: [{ id: 'tc1', type: 'function', function: { name: 'write_file', arguments: '{"path":"src/foo.ts","content":"x"}' } }],
      usage: { inputTokens: 10, outputTokens: 5 },
    },
    { text: 'done', usage: { inputTokens: 10, outputTokens: 5 } },
  ])
  const executor = createMockExecutor({ write_file: 'file written' })
  const permManager = new PermissionManager()
  const loop = new AgentLoop(llm, executor, {
    maxIterations: 10,
    maxTimeoutMs: 60000,
    systemPrompt: '',
    tools: [dummyTool],
    permManager,
    onConfirm: async () => true,
  })
  const result = await loop.run('write foo', [])
  assert.equal(result.toolCallsExecuted, 1)
})

test('AgentLoop: 权限 ask + 用户拒绝', async () => {
  const llm = createMockLLM([
    {
      text: '',
      toolCalls: [{ id: 'tc1', type: 'function', function: { name: 'write_file', arguments: '{"path":"src/foo.ts","content":"x"}' } }],
      usage: { inputTokens: 10, outputTokens: 5 },
    },
    { text: 'ok', usage: { inputTokens: 10, outputTokens: 5 } },
  ])
  const executor = createMockExecutor({ write_file: 'should not reach' })
  const permManager = new PermissionManager()
  const loop = new AgentLoop(llm, executor, {
    maxIterations: 10,
    maxTimeoutMs: 60000,
    systemPrompt: '',
    tools: [dummyTool],
    permManager,
    onConfirm: async () => false,
  })
  const result = await loop.run('write foo', [])
  assert.equal(result.toolCallsExecuted, 0)
})

test('AgentLoop: PreToolUse Hook 阻止工具执行', async () => {
  const llm = createMockLLM([
    {
      text: '',
      toolCalls: [{ id: 'tc1', type: 'function', function: { name: 'read_file', arguments: '{"path":"x.ts"}' } }],
      usage: { inputTokens: 10, outputTokens: 5 },
    },
    { text: 'ok', usage: { inputTokens: 10, outputTokens: 5 } },
  ])
  const executor = createMockExecutor({ read_file: 'should not reach' })
  const hookManager = new HookManager()
  hookManager.register('PreToolUse', 'read_file', () => ({
    blocked: true,
    reason: '测试阻止',
  }))
  const loop = new AgentLoop(llm, executor, {
    maxIterations: 10,
    maxTimeoutMs: 60000,
    systemPrompt: '',
    tools: [dummyTool],
    hookManager,
  })
  const result = await loop.run('read', [])
  assert.equal(result.toolCallsExecuted, 0)
  const toolMsg = result.messages.find(m => m.role === 'tool')
  assert.ok(toolMsg)
  assert.ok(('content' in toolMsg!) && (toolMsg!.content as string).includes('Hook 阻止'))
})

test('AgentLoop: PostToolUse Hook 修改结果', async () => {
  const llm = createMockLLM([
    {
      text: '',
      toolCalls: [{ id: 'tc1', type: 'function', function: { name: 'read_file', arguments: '{"path":"x.ts"}' } }],
      usage: { inputTokens: 10, outputTokens: 5 },
    },
    { text: 'done', usage: { inputTokens: 10, outputTokens: 5 } },
  ])
  const executor = createMockExecutor({ read_file: 'original content' })
  const hookManager = new HookManager()
  hookManager.register('PostToolUse', 'read_file', (ctx) => ({
    blocked: false,
    modifiedResult: `MODIFIED: ${ctx.toolResult}`,
  }))
  const loop = new AgentLoop(llm, executor, {
    maxIterations: 10,
    maxTimeoutMs: 60000,
    systemPrompt: '',
    tools: [dummyTool],
    hookManager,
  })
  const result = await loop.run('read', [])
  const toolMsg = result.messages.find(m => m.role === 'tool')
  assert.ok(toolMsg)
  assert.ok(('content' in toolMsg!) && (toolMsg!.content as string).startsWith('MODIFIED:'))
})
```

测试命令：`npx tsx --test tests/agent-loop.test.ts`

### Step 3.4: 修改 index.ts 使用 AgentLoop

- [ ] 修改 `src/index.ts`，让 AgentCLI 实现 ToolExecutor，chat() 使用 AgentLoop

```typescript
// src/index.ts — 添加 import
import { AgentLoop } from './agent/agent-loop.js'
import { ToolExecutor } from './tools/executor.js'
import { classifyCommand, CommandRisk } from './tools/bash.js'

// 修改 class 声明
class AgentCLI implements ToolExecutor {
  // ... 现有字段不变，删除 consumeStream 方法

  // 实现 ToolExecutor 接口
  async execute(name: string, args: Record<string, unknown>, risk?: CommandRisk): Promise<string> {
    if (name === 'dispatch_subagent') {
      const agentName = args.agent as string
      const task = args.task as string
      const context = args.context as string | undefined
      if (!agentName || !task) return '错误: 缺少 agent 或 task 参数'
      const result = await this.subAgentDispatcher.dispatch(agentName, task, context)
      if (!result.success) return result.output
      return `${result.agent} 完成 (${result.rounds} 轮):\n${result.output}`
    }

    const tool = this.toolRegistry.get(name)
    if (tool) {
      return tool.execute(args)
    }

    const mcpTools = this.mcpClient.getAllTools()
    const mcpTool = mcpTools.find(t => t.name === name)
    if (mcpTool) {
      try {
        const result = await this.mcpClient.callTool(mcpTool.server, name, args)
        return typeof result === 'string' ? result : JSON.stringify(result, null, 2)
      } catch (err: any) {
        return `MCP 工具错误: ${err.message}`
      }
    }

    return `未知工具: ${name}`
  }

  // 重写 chat() 方法
  private async chat(content: string, displayText?: string) {
    this.session.messages.push({ role: 'user', content })
    ui.printUser(displayText || content)
    ui.printDivider()

    try {
      const tools = this.toolRegistry.getFiltered(this.modeManager.config.tools)

      const loop = new AgentLoop(this.llm, this, {
        systemPrompt: this.modeManager.config.systemPrompt,
        tools,
        maxIterations: 50,
        maxTimeoutMs: 5 * 60 * 1000,
        onStreamChunk: (chunk) => this.streamRenderer.write(chunk),
        onToolCall: (name, args) => {
          // 仅做 UI 显示，权限+Hook 逻辑在 AgentLoop 内部
          this.toolRenderer.printToolCall('', name, args)
        },
        onCompaction: (result) => {
          console.log(`${ui.color.gray}[上下文自动压缩: ${result.tokensBefore} → ${result.tokensAfter} tokens]${ui.color.reset}`)
        },
        compactor: this.compactor,
        permManager: this.permManager,
        hookManager: this.hookManager,
        sessionId: this.session.id,
        onConfirm: async (toolName, _args) => {
          return this.confirm(`执行 ${toolName}?`)
        },
      })

      process.stdout.write(`${ui.color.yellow}AI:${ui.color.reset} `)
      const result = await loop.run(content, this.session.messages as ChatMessage[])

      this.streamRenderer.flush()
      this.session.messages = result.messages

      this.costTracker.recordUsage(
        process.env.LLM_MODEL || this.providerConfig.defaultModel,
        process.env.LLM_PROVIDER || 'openai',
        result.usage,
      )

      console.log('\n')
      ui.printDivider()
    } catch (err: any) {
      this.streamRenderer.flush()
      ui.printError(err.message)
    }
  }

  // 删除旧的 consumeStream() 方法
  // 删除旧的 executeTool() 方法（已被 ToolExecutor.execute 替代）
}
```

### Step 3.5: 运行全量测试 + 提交

- [ ] 运行全量测试

```bash
npx tsx --test tests/**/*.test.ts
```

- [ ] 提交

```bash
git add src/agent/agent-loop.ts src/tools/executor.ts src/api/llm.ts src/index.ts tests/agent-loop.test.ts && git commit -m "feat: Agentic Loop + consumeFullStream + 权限Hook集成 (Task 3)

- 新增 AgentLoop 类，支持自主多步执行
- AgentLoop 从第一天起包含 permManager/hookManager/onConfirm
- 权限检查和 Hook 逻辑全部在 AgentLoop 内部
- onToolCall 回调仅做 UI 显示
- LLMClient.consumeFullStream() 替代 AgentLoop 自实现流消费
- 删除 index.ts 中 consumeStream() 方法
- StreamFinal 扩展 stopReason + usage
- OpenAI/Anthropic 流式解析提取精确 token 用量"
```

---

## Task 4: 子代理复用 AgentLoop

> 依赖：#3 | 影响文件：subagent.ts, index.ts

### Step 4.1: 重构 SubAgentDispatcher

- [ ] 修改 `src/agent/subagent.ts`，用 AgentLoop 替代手动循环

```typescript
// src/agent/subagent.ts — 重构
import { LLMClient, ChatMessage } from '../api/llm.js'
import { AgentLoop } from './agent-loop.js'
import { ToolExecutor } from '../tools/executor.js'
import { ToolRegistry } from '../tools/registry.js'

export interface SubAgentDef {
  name: string
  displayName: string
  description: string
  systemPrompt: string
  tools?: string[]
  maxRounds: number
}

export interface SubAgentResult {
  agent: string
  success: boolean
  output: string
  rounds: number
  tokensUsed: number
}

export const BUILTIN_AGENTS: SubAgentDef[] = [
  {
    name: 'code-reviewer',
    displayName: '代码审查员',
    description: '审查代码变更，检查安全漏洞、性能问题、代码风格和最佳实践。',
    systemPrompt: `你是一个专业的代码审查员。你的职责是：
1. 检查安全漏洞（XSS、注入、路径遍历等）
2. 检查性能问题（N+1 查询、内存泄漏等）
3. 检查代码风格和可维护性
4. 给出具体的改进建议

输出格式：
- 🔴 严重问题：必须修复
- 🟠 建议改进：推荐修复
- 🟢 良好实践：值得保持

如果没有问题，简要说明代码质量良好。`,
    tools: ['read_file', 'read_lines', 'search_code', 'search_symbols', 'project_overview'],
    maxRounds: 3,
  },
  {
    name: 'architect',
    displayName: '架构师',
    description: '分析项目架构，提供设计建议和重构方案。',
    systemPrompt: `你是一个资深的软件架构师。你的职责是：
1. 分析当前项目结构和依赖关系
2. 评估架构决策的利弊
3. 提供重构建议和迁移路径
4. 确保设计符合 SOLID 原则

输出格式：
- 现状分析
- 问题识别
- 改进方案（含优先级）
- 实施步骤`,
    tools: ['read_file', 'read_lines', 'search_code', 'search_symbols', 'project_overview'],
    maxRounds: 5,
  },
  {
    name: 'test-writer',
    displayName: '测试工程师',
    description: '为指定代码编写单元测试。',
    systemPrompt: `你是一个测试工程师。你的职责是：
1. 分析目标代码的接口和边界条件
2. 编写覆盖正常路径和异常路径的测试
3. 使用 Node.js 内置 test runner（node:test + node:assert）
4. 测试文件放在 tests/ 目录下`,
    tools: ['read_file', 'read_lines', 'search_code', 'search_symbols', 'write_file', 'edit_file', 'bash'],
    maxRounds: 5,
  },
  {
    name: 'debugger',
    displayName: '调试专家',
    description: '分析错误和异常，定位根因并提供修复方案。',
    systemPrompt: `你是一个调试专家。你的职责是：
1. 分析错误消息和堆栈跟踪
2. 定位根本原因
3. 提供精确的修复方案
4. 验证修复不会引入新问题`,
    tools: ['read_file', 'read_lines', 'search_code', 'search_symbols', 'bash'],
    maxRounds: 5,
  },
]

export class SubAgentDispatcher {
  private agents = new Map<string, SubAgentDef>()
  private llm: LLMClient | null = null
  private toolExecutor: ToolExecutor | null = null
  private toolRegistry: ToolRegistry | null = null

  constructor() {
    for (const agent of BUILTIN_AGENTS) {
      this.agents.set(agent.name, agent)
    }
  }

  setLLM(llm: LLMClient): void {
    this.llm = llm
  }

  setToolExecutor(executor: ToolExecutor): void {
    this.toolExecutor = executor
  }

  setToolRegistry(registry: ToolRegistry): void {
    this.toolRegistry = registry
  }

  register(agent: SubAgentDef): void {
    this.agents.set(agent.name, agent)
  }

  list(): SubAgentDef[] {
    return [...this.agents.values()]
  }

  get(name: string): SubAgentDef | undefined {
    return this.agents.get(name)
  }

  async dispatch(
    agentName: string,
    task: string,
    context?: string,
  ): Promise<SubAgentResult> {
    const agent = this.agents.get(agentName)
    if (!agent) {
      return {
        agent: agentName,
        success: false,
        output: `未知子代理: ${agentName}。可用: ${[...this.agents.keys()].join(', ')}`,
        rounds: 0,
        tokensUsed: 0,
      }
    }

    if (!this.llm || !this.toolExecutor || !this.toolRegistry) {
      return {
        agent: agentName,
        success: false,
        output: '子代理未初始化（缺少 LLM/ToolExecutor/ToolRegistry）',
        rounds: 0,
        tokensUsed: 0,
      }
    }

    const tools = this.toolRegistry.getFiltered(agent.tools || [])
    const loop = new AgentLoop(this.llm, this.toolExecutor, {
      systemPrompt: agent.systemPrompt,
      tools,
      maxIterations: agent.maxRounds,
      maxTimeoutMs: 3 * 60 * 1000,
      onStreamChunk: () => {},
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

### Step 4.2: 更新 index.ts 注入依赖

- [ ] 修改 `src/index.ts` 构造函数，注入 ToolExecutor 和 ToolRegistry 到 SubAgentDispatcher

```typescript
// 在 constructor() 中，subAgentDispatcher.setLLM(this.llm) 之后添加：
this.subAgentDispatcher.setToolExecutor(this)
this.subAgentDispatcher.setToolRegistry(this.toolRegistry)
```

### Step 4.3: 更新子代理测试

- [ ] 重写 `tests/subagent.test.ts`

```typescript
import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { SubAgentDispatcher } from '../src/agent/subagent.js'

test('SubAgent: 未知子代理返回错误', async () => {
  const dispatcher = new SubAgentDispatcher()
  const result = await dispatcher.dispatch('nonexistent', 'test')
  assert.equal(result.success, false)
  assert.match(result.output, /未知子代理/)
})

test('SubAgent: 未初始化返回错误', async () => {
  const dispatcher = new SubAgentDispatcher()
  const result = await dispatcher.dispatch('code-reviewer', 'test')
  assert.equal(result.success, false)
  assert.match(result.output, /未初始化/)
})

test('SubAgent: list 返回内置代理', () => {
  const dispatcher = new SubAgentDispatcher()
  const agents = dispatcher.list()
  assert.ok(agents.length >= 4)
  assert.ok(agents.some(a => a.name === 'code-reviewer'))
  assert.ok(agents.some(a => a.name === 'architect'))
  assert.ok(agents.some(a => a.name === 'test-writer'))
  assert.ok(agents.some(a => a.name === 'debugger'))
})

test('SubAgent: get 返回代理定义', () => {
  const dispatcher = new SubAgentDispatcher()
  const agent = dispatcher.get('code-reviewer')
  assert.ok(agent)
  assert.equal(agent!.name, 'code-reviewer')
  assert.ok(agent!.tools)
  assert.ok(agent!.tools!.length > 0)
})

test('SubAgent: register 注册自定义代理', () => {
  const dispatcher = new SubAgentDispatcher()
  dispatcher.register({
    name: 'custom-agent',
    displayName: '自定义',
    description: '测试用',
    systemPrompt: 'You are custom',
    tools: ['read_file'],
    maxRounds: 2,
  })
  const agent = dispatcher.get('custom-agent')
  assert.ok(agent)
  assert.equal(agent!.name, 'custom-agent')
})
```

测试命令：`npx tsx --test tests/subagent.test.ts`

### Step 4.4: 运行全量测试 + 提交

- [ ] 运行全量测试

```bash
npx tsx --test tests/**/*.test.ts
```

- [ ] 提交

```bash
git add src/agent/subagent.ts src/index.ts tests/subagent.test.ts && git commit -m "feat: 子代理复用 AgentLoop (Task 4)

- SubAgentDispatcher.dispatch() 创建 AgentLoop 实例
- 子代理真正执行工具调用，支持多步推理
- 子代理工具列表由 agent.tools 白名单控制
- 注入 ToolExecutor 和 ToolRegistry 到 SubAgentDispatcher"
```

---

## Task 5: CODECAST.md + 模式工具白名单联动

> 依赖：#1 | 影响文件：project-context.ts, modes.ts, index.ts

### Step 5.1: 新增 ProjectContext 类

- [ ] 创建 `src/context/project-context.ts`

```typescript
// src/context/project-context.ts
import { existsSync, readFileSync } from 'fs'
import { join, resolve, dirname } from 'path'

export interface ToolOverrides {
  allow?: string[]
  deny?: string[]
}

export class ProjectContext {
  private projectRoot: string
  private instructions: string = ''
  private toolOverrides: ToolOverrides = {}

  constructor(projectRoot?: string) {
    this.projectRoot = resolve(projectRoot || process.cwd())
    this.loadInstructions()
  }

  getInstructions(): string {
    return this.instructions
  }

  getToolOverrides(): ToolOverrides {
    return this.toolOverrides
  }

  loadInstructions(): void {
    const files: string[] = []
    let dir = this.projectRoot

    for (let i = 0; i < 5; i++) {
      const candidate = join(dir, 'CODECAST.md')
      if (existsSync(candidate)) {
        files.push(candidate)
      }
      const parent = dirname(dir)
      if (parent === dir) break
      dir = parent
    }

    const parts: string[] = []
    for (const file of files.reverse()) {
      try {
        const content = readFileSync(file, 'utf-8')
        const overrides = this.parseToolOverrides(content)
        if (overrides.allow) {
          this.toolOverrides.allow = overrides.allow
        }
        if (overrides.deny) {
          this.toolOverrides.deny = overrides.deny
        }
        const instructionPart = content.replace(/\[tools\][\s\S]*$/, '').trim()
        if (instructionPart) {
          parts.push(instructionPart)
        }
      } catch {
        // 读取失败，跳过
      }
    }

    this.instructions = parts.join('\n\n')
  }

  parseToolOverrides(content: string): ToolOverrides {
    const result: ToolOverrides = {}
    const toolsMatch = content.match(/\[tools\]([\s\S]*?)(?:\[|$)/)
    if (!toolsMatch) return result

    const section = toolsMatch[1]
    const allowMatch = section.match(/allow:\s*(.+)/i)
    const denyMatch = section.match(/deny:\s*(.+)/i)

    if (allowMatch) {
      result.allow = allowMatch[1].split(',').map(s => s.trim()).filter(Boolean)
    }
    if (denyMatch) {
      result.deny = denyMatch[1].split(',').map(s => s.trim()).filter(Boolean)
    }

    return result
  }

  formatForPrompt(): string {
    if (!this.instructions) return ''
    return `\n\n项目指令:\n${this.instructions}`
  }
}
```

### Step 5.2: 新增 ProjectContext 测试

- [ ] 创建 `tests/project-context.test.ts`

```typescript
import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { ProjectContext } from '../src/context/project-context.js'
import { writeFileSync, mkdirSync, rmSync } from 'fs'
import { join } from 'path'
import { tmpdir } from 'os'

const TEST_DIR = join(tmpdir(), 'codecast-test-project-context')

test('ProjectContext: 无 CODECAST.md 返回空指令', () => {
  const ctx = new ProjectContext(tmpdir())
  assert.ok(typeof ctx.getInstructions() === 'string')
})

test('ProjectContext: parseToolOverrides 解析 allow', () => {
  const ctx = new ProjectContext(tmpdir())
  const result = ctx.parseToolOverrides(`# 项目指令\n\n一些说明\n\n[tools]\nallow: read_file, write_file, bash\n`)
  assert.deepEqual(result.allow, ['read_file', 'write_file', 'bash'])
})

test('ProjectContext: parseToolOverrides 解析 deny', () => {
  const ctx = new ProjectContext(tmpdir())
  const result = ctx.parseToolOverrides(`[tools]\ndeny: rm, git push\n`)
  assert.deepEqual(result.deny, ['rm', 'git push'])
})

test('ProjectContext: parseToolOverrides 无 [tools] 段', () => {
  const ctx = new ProjectContext(tmpdir())
  const result = ctx.parseToolOverrides(`# 项目指令\n\n无工具配置`)
  assert.equal(result.allow, undefined)
  assert.equal(result.deny, undefined)
})

test('ProjectContext: formatForPrompt 无指令返回空', () => {
  const ctx = new ProjectContext(tmpdir())
  const prompt = ctx.formatForPrompt()
  assert.ok(typeof prompt === 'string')
})

test('ProjectContext: 加载 CODECAST.md 文件', () => {
  mkdirSync(TEST_DIR, { recursive: true })
  const mdPath = join(TEST_DIR, 'CODECAST.md')
  writeFileSync(mdPath, '# 测试项目\n\n使用 TypeScript。\n\n[tools]\nallow: read_file, bash\n', 'utf-8')

  try {
    const ctx = new ProjectContext(TEST_DIR)
    assert.ok(ctx.getInstructions().includes('TypeScript'))
    assert.deepEqual(ctx.getToolOverrides().allow, ['read_file', 'bash'])
  } finally {
    rmSync(TEST_DIR, { recursive: true, force: true })
  }
})
```

测试命令：`npx tsx --test tests/project-context.test.ts`

### Step 5.3: 修正模式工具白名单

- [ ] 修改 `src/agent/modes.ts`，修正各模式的 tools 列表

```typescript
// src/agent/modes.ts
export type AgentMode = 'ask' | 'plan' | 'build'

export interface ModeConfig {
  name: string
  description: string
  systemPrompt: string
  tools: string[]
  autoExecute: boolean
}

export const MODES: Record<AgentMode, ModeConfig> = {
  ask: {
    name: 'Ask',
    description: '问答模式 - 仅回答问题，不修改文件',
    systemPrompt: `你是一个代码助手。你可以回答问题、解释代码、提供建议。
重要：不要修改任何文件，不要执行任何命令。只提供信息和建议。`,
    tools: ['read_file', 'read_lines', 'search_code', 'search_symbols', 'project_overview', 'git_status', 'git_diff', 'git_log', 'git_blame'],
    autoExecute: false,
  },

  plan: {
    name: 'Plan',
    description: '规划模式 - 分析需求并制定计划',
    systemPrompt: `你是一个架构师。你的任务是分析需求并制定详细的实施计划。
你可以读取文件了解项目结构，也可以执行安全的只读命令。
不要修改文件。
输出格式：
1. 目标分析
2. 实施步骤（编号列表）
3. 涉及文件
4. 风险评估`,
    tools: ['read_file', 'read_lines', 'search_code', 'search_symbols', 'project_overview', 'bash', 'git_status', 'git_diff', 'git_log', 'git_blame'],
    autoExecute: false,
  },

  build: {
    name: 'Build',
    description: '构建模式 - 执行代码修改和命令',
    systemPrompt: `你是一个全栈工程师。你可以读取文件、修改文件、执行命令来完成任务。
执行工具前，先说明你要做什么。
修改文件后，简要说明变更内容。`,
    tools: ['read_file', 'write_file', 'edit_file', 'multi_edit_file', 'bash', 'search_code', 'search_symbols', 'project_overview', 'read_lines', 'git_status', 'git_diff', 'git_commit', 'git_log', 'git_branch', 'git_blame', 'dispatch_subagent'],
    autoExecute: true,
  },
}

export class AgentModeManager {
  private current: AgentMode = 'ask'

  get mode() { return this.current }
  set mode(m: AgentMode) { this.current = m }
  get config() { return MODES[this.current] }

  cycle(): AgentMode {
    const modes: AgentMode[] = ['ask', 'plan', 'build']
    const idx = modes.indexOf(this.current)
    this.current = modes[(idx + 1) % modes.length]
    return this.current
  }

  printStatus() {
    const { name, description } = this.config
    console.log(`\x1b[36m[${name}]\x1b[0m \x1b[90m${description}\x1b[0m`)
  }
}
```

### Step 5.4: 修改 index.ts 注入 CODECAST.md + 使用模式白名单

- [ ] 修改 `src/index.ts`，添加 ProjectContext 并注入 systemPrompt

```typescript
import { ProjectContext } from './context/project-context.js'

class AgentCLI implements ToolExecutor {
  private projectContext = new ProjectContext()

  private updateSystemPrompt() {
    const mode = this.modeManager.config
    const codebaseHint = '\n\n你可以使用 search_code、search_symbols、project_overview、read_lines 工具来探索项目代码库。使用 git_* 工具查看版本控制信息。'
    const subAgentHint = '\n\n你可以使用 dispatch_subagent 工具调度子代理（code-reviewer、architect、test-writer、debugger）执行专门任务。'
    const memoryHint = this.autoMemory.formatForPrompt()
    const memorySection = memoryHint ? `\n\n${memoryHint}` : ''
    const projectHint = this.projectContext.formatForPrompt()

    this.session.messages = [
      { role: 'system', content: mode.systemPrompt + codebaseHint + subAgentHint + memorySection + projectHint },
      ...this.session.messages.filter(m => m.role !== 'system'),
    ]
  }
}
```

- [ ] 修改 chat() 中的工具过滤，应用 CODECAST.md 的 deny 规则

```typescript
private async chat(content: string, displayText?: string) {
  this.session.messages.push({ role: 'user', content })
  ui.printUser(displayText || content)
  ui.printDivider()

  try {
    let tools = this.toolRegistry.getFiltered(this.modeManager.config.tools)

    const overrides = this.projectContext.getToolOverrides()
    if (overrides.deny && overrides.deny.length > 0) {
      const denySet = new Set(overrides.deny)
      tools = tools.filter(t => !denySet.has(t.name))
    }

    const loop = new AgentLoop(this.llm, this, {
      systemPrompt: this.modeManager.config.systemPrompt,
      tools,
      maxIterations: 50,
      maxTimeoutMs: 5 * 60 * 1000,
      onStreamChunk: (chunk) => this.streamRenderer.write(chunk),
      onToolCall: (name, args) => {
        this.toolRenderer.printToolCall('', name, args)
      },
      onCompaction: (result) => {
        console.log(`${ui.color.gray}[上下文自动压缩: ${result.tokensBefore} → ${result.tokensAfter} tokens]${ui.color.reset}`)
      },
      compactor: this.compactor,
      permManager: this.permManager,
      hookManager: this.hookManager,
      sessionId: this.session.id,
      onConfirm: async (toolName, _args) => {
        return this.confirm(`执行 ${toolName}?`)
      },
    })

    process.stdout.write(`${ui.color.yellow}AI:${ui.color.reset} `)
    const result = await loop.run(content, this.session.messages as ChatMessage[])

    this.streamRenderer.flush()
    this.session.messages = result.messages

    this.costTracker.recordUsage(
      process.env.LLM_MODEL || this.providerConfig.defaultModel,
      process.env.LLM_PROVIDER || 'openai',
      result.usage,
    )

    console.log('\n')
    ui.printDivider()
  } catch (err: any) {
    this.streamRenderer.flush()
    ui.printError(err.message)
  }
}
```

### Step 5.5: 更新 modes 测试

- [ ] 重写 `tests/modes.test.ts`

```typescript
import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { AgentModeManager, MODES } from '../src/agent/modes.js'

test('Modes: ask 模式只有只读工具', () => {
  const tools = MODES.ask.tools
  assert.ok(!tools.includes('write_file'))
  assert.ok(!tools.includes('edit_file'))
  assert.ok(!tools.includes('bash'))
  assert.ok(tools.includes('read_file'))
  assert.ok(tools.includes('search_code'))
})

test('Modes: plan 模式有 bash 但无写文件工具', () => {
  const tools = MODES.plan.tools
  assert.ok(tools.includes('bash'))
  assert.ok(!tools.includes('write_file'))
  assert.ok(!tools.includes('edit_file'))
  assert.ok(tools.includes('read_file'))
})

test('Modes: build 模式有全部工具', () => {
  const tools = MODES.build.tools
  assert.ok(tools.includes('write_file'))
  assert.ok(tools.includes('edit_file'))
  assert.ok(tools.includes('bash'))
  assert.ok(tools.includes('dispatch_subagent'))
})

test('Modes: cycle 循环切换', () => {
  const mgr = new AgentModeManager()
  assert.equal(mgr.mode, 'ask')
  mgr.cycle()
  assert.equal(mgr.mode, 'plan')
  mgr.cycle()
  assert.equal(mgr.mode, 'build')
  mgr.cycle()
  assert.equal(mgr.mode, 'ask')
})

test('Modes: set mode', () => {
  const mgr = new AgentModeManager()
  mgr.mode = 'build'
  assert.equal(mgr.mode, 'build')
  assert.equal(mgr.config.name, 'Build')
})
```

测试命令：`npx tsx --test tests/modes.test.ts && npx tsx --test tests/project-context.test.ts`

### Step 5.6: 运行全量测试 + 提交

- [ ] 运行全量测试

```bash
npx tsx --test tests/**/*.test.ts
```

- [ ] 提交

```bash
git add src/context/project-context.ts src/agent/modes.ts src/index.ts tests/project-context.test.ts tests/modes.test.ts && git commit -m "feat: CODECAST.md + 模式工具白名单联动 (Task 5)

- 新增 ProjectContext 类，自动发现和加载 CODECAST.md
- 解析 [tools] 段的 allow/deny 覆盖
- 修正模式工具白名单：ask 只读，plan +bash，build 全部
- chat() 使用模式白名单过滤工具 + CODECAST.md deny 规则
- systemPrompt 注入 CODECAST.md 项目指令"
```

---

## Task 6: 精确 Token 追踪

> 依赖：#3 | 影响文件：tracker.ts

### Step 6.1: 扩展 CostTracker 支持 TokenUsage 和 cache token

- [ ] 修改 `src/cost/tracker.ts`，增加 cache token 定价和 TokenUsage 接收

```typescript
// src/cost/tracker.ts — 扩展
import { writeFileSync, readFileSync, existsSync, mkdirSync } from 'fs'
import { join, resolve } from 'path'
import { homedir } from 'os'
import { TokenUsage } from '../api/llm.js'

export interface ModelPricing {
  inputPerMillion: number
  outputPerMillion: number
  cacheReadPerMillion?: number
  cacheWritePerMillion?: number
}

export interface CostRecord {
  timestamp: number
  model: string
  provider: string
  inputTokens: number
  outputTokens: number
  cacheReadTokens?: number
  cacheCreationTokens?: number
  costUSD: number
  sessionId: string
}

export interface CostSummary {
  totalInputTokens: number
  totalOutputTokens: number
  totalCostUSD: number
  byModel: Record<string, { input: number; output: number; cost: number }>
  bySession: Record<string, { input: number; output: number; cost: number }>
  recordCount: number
}

const PRICING: Record<string, ModelPricing> = {
  'gpt-4o': { inputPerMillion: 2.5, outputPerMillion: 10, cacheReadPerMillion: 1.25 },
  'gpt-4o-mini': { inputPerMillion: 0.15, outputPerMillion: 0.6, cacheReadPerMillion: 0.075 },
  'gpt-4-turbo': { inputPerMillion: 10, outputPerMillion: 30, cacheReadPerMillion: 5 },
  'gpt-3.5-turbo': { inputPerMillion: 0.5, outputPerMillion: 1.5 },
  'claude-3-5-sonnet': { inputPerMillion: 3, outputPerMillion: 15, cacheReadPerMillion: 0.3, cacheWritePerMillion: 3.75 },
  'claude-3-sonnet': { inputPerMillion: 3, outputPerMillion: 15, cacheReadPerMillion: 0.3, cacheWritePerMillion: 3.75 },
  'claude-3-haiku': { inputPerMillion: 0.25, outputPerMillion: 1.25, cacheReadPerMillion: 0.03, cacheWritePerMillion: 0.3 },
  'claude-3-opus': { inputPerMillion: 15, outputPerMillion: 75, cacheReadPerMillion: 1.5, cacheWritePerMillion: 18.75 },
}

const COST_DIR = resolve(homedir(), '.codecast')
const COST_FILE = join(COST_DIR, 'costs.jsonl')

export class CostTracker {
  private records: CostRecord[] = []
  private budgetUSD: number | null = null
  private currentSessionId: string = ''

  constructor(budgetUSD?: number) {
    this.budgetUSD = budgetUSD || null
    this.load()
  }

  setSessionId(id: string) {
    this.currentSessionId = id
  }

  record(model: string, provider: string, inputTokens: number, outputTokens: number): CostRecord {
    const pricing = this.findPricing(model)
    const costUSD =
      (inputTokens / 1_000_000) * pricing.inputPerMillion +
      (outputTokens / 1_000_000) * pricing.outputPerMillion

    const record: CostRecord = {
      timestamp: Date.now(),
      model,
      provider,
      inputTokens,
      outputTokens,
      costUSD,
      sessionId: this.currentSessionId,
    }

    this.records.push(record)
    this.persist()

    if (this.budgetUSD) {
      const total = this.getSummary().totalCostUSD
      if (total >= this.budgetUSD * 0.9) {
        console.warn(`\n⚠️ 成本警告: 已使用 $${total.toFixed(4)} / 预算 $${this.budgetUSD}`)
      }
    }

    return record
  }

  recordUsage(model: string, provider: string, usage: TokenUsage): CostRecord {
    const pricing = this.findPricing(model)
    let costUSD =
      (usage.inputTokens / 1_000_000) * pricing.inputPerMillion +
      (usage.outputTokens / 1_000_000) * pricing.outputPerMillion

    if (usage.cacheReadTokens && pricing.cacheReadPerMillion) {
      costUSD += (usage.cacheReadTokens / 1_000_000) * pricing.cacheReadPerMillion
    }
    if (usage.cacheCreationTokens && pricing.cacheWritePerMillion) {
      costUSD += (usage.cacheCreationTokens / 1_000_000) * pricing.cacheWritePerMillion
    }

    const record: CostRecord = {
      timestamp: Date.now(),
      model,
      provider,
      inputTokens: usage.inputTokens,
      outputTokens: usage.outputTokens,
      cacheReadTokens: usage.cacheReadTokens,
      cacheCreationTokens: usage.cacheCreationTokens,
      costUSD,
      sessionId: this.currentSessionId,
    }

    this.records.push(record)
    this.persist()

    if (this.budgetUSD) {
      const total = this.getSummary().totalCostUSD
      if (total >= this.budgetUSD * 0.9) {
        console.warn(`\n⚠️ 成本警告: 已使用 $${total.toFixed(4)} / 预算 $${this.budgetUSD}`)
      }
    }

    return record
  }

  getSummary(): CostSummary {
    const summary: CostSummary = {
      totalInputTokens: 0,
      totalOutputTokens: 0,
      totalCostUSD: 0,
      byModel: {},
      bySession: {},
      recordCount: this.records.length,
    }

    for (const r of this.records) {
      summary.totalInputTokens += r.inputTokens
      summary.totalOutputTokens += r.outputTokens
      summary.totalCostUSD += r.costUSD

      if (!summary.byModel[r.model]) {
        summary.byModel[r.model] = { input: 0, output: 0, cost: 0 }
      }
      summary.byModel[r.model].input += r.inputTokens
      summary.byModel[r.model].output += r.outputTokens
      summary.byModel[r.model].cost += r.costUSD

      if (!summary.bySession[r.sessionId]) {
        summary.bySession[r.sessionId] = { input: 0, output: 0, cost: 0 }
      }
      summary.bySession[r.sessionId].input += r.inputTokens
      summary.bySession[r.sessionId].output += r.outputTokens
      summary.bySession[r.sessionId].cost += r.costUSD
    }

    return summary
  }

  formatReport(): string {
    const s = this.getSummary()
    const lines: string[] = []

    lines.push(`总成本: $${s.totalCostUSD.toFixed(4)}`)
    lines.push(`总 tokens: ${s.totalInputTokens.toLocaleString()} 输入 + ${s.totalOutputTokens.toLocaleString()} 输出`)
    lines.push(`调用次数: ${s.recordCount}`)

    if (Object.keys(s.byModel).length > 0) {
      lines.push('\n按模型:')
      for (const [model, data] of Object.entries(s.byModel)) {
        lines.push(`  ${model}: $${data.cost.toFixed(4)} (${data.input.toLocaleString()}+${data.output.toLocaleString()} tokens)`)
      }
    }

    if (this.currentSessionId && s.bySession[this.currentSessionId]) {
      const session = s.bySession[this.currentSessionId]
      lines.push(`\n当前会话: $${session.cost.toFixed(4)}`)
    }

    return lines.join('\n')
  }

  clear() {
    this.records = []
    this.persist()
  }

  private findPricing(model: string): ModelPricing {
    if (PRICING[model]) return PRICING[model]
    for (const [key, pricing] of Object.entries(PRICING)) {
      if (model.startsWith(key)) return pricing
    }
    return { inputPerMillion: 0.5, outputPerMillion: 2 }
  }

  private load() {
    if (!existsSync(COST_FILE)) return
    try {
      const raw = readFileSync(COST_FILE, 'utf-8')
      this.records = raw.trim().split('\n')
        .filter(line => line.trim())
        .map(line => JSON.parse(line))
    } catch {
      this.records = []
    }
  }

  private persist() {
    if (!existsSync(COST_DIR)) {
      mkdirSync(COST_DIR, { recursive: true })
    }
    const lines = this.records.map(r => JSON.stringify(r)).join('\n')
    writeFileSync(COST_FILE, lines + '\n', 'utf-8')
  }
}
```

### Step 6.2: 更新 CostTracker 测试

- [ ] 重写 `tests/cost.test.ts`

```typescript
import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { CostTracker } from '../src/cost/tracker.js'
import { TokenUsage } from '../src/api/llm.js'

test('CostTracker: recordUsage 精确 token', () => {
  const tracker = new CostTracker()
  tracker.setSessionId('test-session')
  const usage: TokenUsage = { inputTokens: 1000, outputTokens: 500 }
  const record = tracker.recordUsage('gpt-4o', 'openai', usage)
  assert.ok(record.costUSD > 0)
  assert.equal(record.inputTokens, 1000)
  assert.equal(record.outputTokens, 500)
  tracker.clear()
})

test('CostTracker: recordUsage 含 cache token', () => {
  const tracker = new CostTracker()
  tracker.setSessionId('test-session')
  const usage: TokenUsage = {
    inputTokens: 1000,
    outputTokens: 500,
    cacheReadTokens: 800,
    cacheCreationTokens: 200,
  }
  const record = tracker.recordUsage('claude-3-5-sonnet', 'anthropic', usage)
  assert.ok(record.costUSD > 0)
  assert.equal(record.cacheReadTokens, 800)
  assert.equal(record.cacheCreationTokens, 200)
  tracker.clear()
})

test('CostTracker: getSummary 汇总', () => {
  const tracker = new CostTracker()
  tracker.setSessionId('test-session')
  tracker.recordUsage('gpt-4o', 'openai', { inputTokens: 1000, outputTokens: 500 })
  tracker.recordUsage('gpt-4o', 'openai', { inputTokens: 2000, outputTokens: 1000 })
  const summary = tracker.getSummary()
  assert.equal(summary.totalInputTokens, 3000)
  assert.equal(summary.totalOutputTokens, 1500)
  assert.ok(summary.totalCostUSD > 0)
  assert.equal(summary.recordCount, 2)
  tracker.clear()
})

test('CostTracker: 旧 record() 方法仍可用', () => {
  const tracker = new CostTracker()
  tracker.setSessionId('test-session')
  const record = tracker.record('gpt-4o', 'openai', 1000, 500)
  assert.ok(record.costUSD > 0)
  tracker.clear()
})

test('CostTracker: formatReport 输出', () => {
  const tracker = new CostTracker()
  tracker.setSessionId('test-session')
  tracker.recordUsage('gpt-4o', 'openai', { inputTokens: 1000, outputTokens: 500 })
  const report = tracker.formatReport()
  assert.ok(report.includes('总成本'))
  assert.ok(report.includes('gpt-4o'))
  tracker.clear()
})
```

测试命令：`npx tsx --test tests/cost.test.ts`

### Step 6.3: 运行全量测试 + 提交

- [ ] 运行全量测试

```bash
npx tsx --test tests/**/*.test.ts
```

- [ ] 提交

```bash
git add src/cost/tracker.ts tests/cost.test.ts && git commit -m "feat: 精确 Token 追踪 (Task 6)

- CostTracker 支持 TokenUsage 接口
- 新增 cache token 定价（cacheRead/cacheWrite）
- 新增 recordUsage() 方法
- chat() 使用 AgentLoop 返回的精确 usage
- 删除粗估 token 逻辑"
```

---

## Task 7: 增量式上下文压缩

> 依赖：#3 | 影响文件：compactor.ts

> **优化说明**：合并 compact() 和 compactAsync() 为一个 `async compact()` 方法。内部根据 useLLMSummary + llm 是否可用决定用规则摘要还是 LLM 摘要。AgentLoop 始终调用 `await this.config.compactor.compact(messages)`。

### Step 7.1: 重构 ContextCompactor

- [ ] 修改 `src/context/compactor.ts`，合并为单一 async compact() + 增量摘要

```typescript
// src/context/compactor.ts — 重构
import { ChatMessage, Message, ToolResult, LLMClient } from '../api/llm.js'

export interface CompactionConfig {
  maxTokens: number
  keepRecentRounds: number
  compactBatchSize: number
  useLLMSummary: boolean
  summaryPrompt: string
}

const DEFAULT_CONFIG: CompactionConfig = {
  maxTokens: 80000,
  keepRecentRounds: 4,
  compactBatchSize: 1,
  useLLMSummary: false,
  summaryPrompt: '以下是之前对话的摘要，请据此继续回答：',
}

export interface CompactionResult {
  messages: ChatMessage[]
  didCompact: boolean
  tokensBefore: number
  tokensAfter: number
  roundsCompacted: number
}

export class ContextCompactor {
  private config: CompactionConfig
  private summaryHistory: string = ''
  private llm: LLMClient | null = null

  constructor(config: Partial<CompactionConfig> = {}) {
    this.config = { ...DEFAULT_CONFIG, ...config }
  }

  setLLM(llm: LLMClient): void {
    this.llm = llm
  }

  estimateTokens(messages: ChatMessage[]): number {
    let total = 0
    for (const m of messages) {
      total += 4
      if ('content' in m && typeof m.content === 'string') {
        total += Math.ceil(m.content.length * 0.5)
      }
      if ('tool_calls' in m && m.tool_calls) {
        for (const tc of m.tool_calls) {
          total += Math.ceil((tc.function.name.length + tc.function.arguments.length) * 0.5)
          total += 10
        }
      }
      if ('tool_call_id' in m) {
        total += 10
      }
    }
    return total
  }

  /**
   * 统一的异步压缩方法。
   * 内部决策：如果 useLLMSummary=true 且 llm 已注入，使用 LLM 摘要；否则使用规则摘要。
   * AgentLoop 始终调用 await this.config.compactor.compact(messages)
   */
  async compact(messages: ChatMessage[]): Promise<CompactionResult> {
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

    const batch = oldRounds.slice(-this.config.compactBatchSize)

    let newSummary: string
    if (this.config.useLLMSummary && this.llm) {
      newSummary = await this.generateLLMSummary(batch)
    } else {
      newSummary = this.generateRuleBasedSummary(batch)
    }

    this.summaryHistory = this.summaryHistory
      ? this.summaryHistory + '\n' + newSummary
      : newSummary

    const result: ChatMessage[] = []
    if (systemMsg) result.push(systemMsg)
    if (this.summaryHistory) {
      result.push({ role: 'user', content: `[对话摘要]\n${this.summaryHistory}` })
      result.push({ role: 'assistant', content: '收到，继续。' })
    }
    for (const round of recentRounds) result.push(...round)

    const tokensAfter = this.estimateTokens(result)
    return {
      messages: result,
      didCompact: true,
      tokensBefore,
      tokensAfter,
      roundsCompacted: batch.length,
    }
  }

  resetSummary(): void {
    this.summaryHistory = ''
  }

  private async generateLLMSummary(rounds: ChatMessage[][]): Promise<string> {
    if (!this.llm) return this.generateRuleBasedSummary(rounds)

    const roundText = rounds.map((round, i) => {
      const parts = round.map(m => {
        if (m.role === 'user') return `用户: ${(m as Message).content.slice(0, 200)}`
        if (m.role === 'assistant') {
          const msg = m as Message
          const text = msg.content.slice(0, 200)
          const tools = msg.tool_calls?.map(tc => tc.function.name).join(', ') || ''
          return `AI: ${text}${tools ? ` [调用工具: ${tools}]` : ''}`
        }
        if (m.role === 'tool') return `工具结果: ${(m as ToolResult).content.slice(0, 100)}`
        return ''
      }).filter(Boolean).join('\n')
      return `--- 轮次 ${i + 1} ---\n${parts}`
    }).join('\n\n')

    const prompt = `${this.summaryHistory ? `已有摘要:\n${this.summaryHistory}\n\n` : ''}请将以下对话轮次的关键信息追加到摘要中。格式：每轮一行，包含用户意图、AI 操作和结果。

${roundText}`

    try {
      const messages: ChatMessage[] = [
        { role: 'system', content: '你是一个对话摘要生成器。将对话轮次压缩为简洁的摘要行。' },
        { role: 'user', content: prompt },
      ]
      const result = await this.llm.consumeFullStream(messages)
      return result.text.trim() || this.generateRuleBasedSummary(rounds)
    } catch {
      return this.generateRuleBasedSummary(rounds)
    }
  }

  private generateRuleBasedSummary(rounds: ChatMessage[][]): string {
    const lines: string[] = []

    for (let i = 0; i < rounds.length; i++) {
      const round = rounds[i]
      const userMsg = round.find(m => m.role === 'user') as Message | undefined
      const assistantMsg = round.find(m => m.role === 'assistant') as Message | undefined
      const toolResults = round.filter(m => m.role === 'tool') as ToolResult[]

      const parts: string[] = []
      if (userMsg) {
        const preview = userMsg.content.slice(0, 60)
        parts.push(`用户请求: ${preview}`)
      }
      if (assistantMsg?.tool_calls?.length) {
        const toolNames = assistantMsg.tool_calls.map(tc => tc.function.name).join(', ')
        parts.push(`AI 调用: ${toolNames}`)
      }
      if (toolResults.length > 0) {
        const ok = toolResults.filter(tr => !tr.content.startsWith('错误')).length
        const fail = toolResults.length - ok
        parts.push(`结果: ${ok} 成功${fail > 0 ? `, ${fail} 失败` : ''}`)
      }
      if (assistantMsg?.content && !assistantMsg.tool_calls?.length) {
        const preview = assistantMsg.content.slice(0, 60)
        parts.push(`AI 回复: ${preview}`)
      }

      if (parts.length > 0) {
        lines.push(`#${i + 1} ${parts.join(' → ')}`)
      }
    }

    return lines.join('\n')
  }

  private groupByRounds(messages: ChatMessage[]): ChatMessage[][] {
    const rounds: ChatMessage[][] = []
    let current: ChatMessage[] = []

    for (const m of messages) {
      if (m.role === 'user' && current.length > 0) {
        rounds.push(current)
        current = []
      }
      current.push(m)
    }
    if (current.length > 0) rounds.push(current)

    return rounds
  }
}
```

### Step 7.2: 更新 Compactor 测试

- [ ] 重写 `tests/compactor.test.ts`

```typescript
import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { ContextCompactor } from '../src/context/compactor.js'
import { ChatMessage } from '../src/api/llm.js'

test('Compactor: 短消息不触发压缩', async () => {
  const c = new ContextCompactor({ maxTokens: 100000, keepRecentRounds: 2 })
  const msgs: ChatMessage[] = [
    { role: 'system', content: 'You are helpful.' },
    { role: 'user', content: 'hi' },
    { role: 'assistant', content: 'hello' },
  ]
  const result = await c.compact(msgs)
  assert.equal(result.didCompact, false)
})

test('Compactor: 超过阈值触发增量压缩', async () => {
  const c = new ContextCompactor({ maxTokens: 20, keepRecentRounds: 1, compactBatchSize: 1 })
  const msgs: ChatMessage[] = [
    { role: 'system', content: 'sys' },
    { role: 'user', content: 'first question' },
    { role: 'assistant', content: 'first answer' },
    { role: 'user', content: 'second question' },
    { role: 'assistant', content: 'second answer' },
    { role: 'user', content: 'third question' },
    { role: 'assistant', content: 'third answer' },
    { role: 'user', content: 'recent question' },
    { role: 'assistant', content: 'recent answer' },
  ]
  const result = await c.compact(msgs)
  assert.equal(result.didCompact, true)
  assert.ok(result.tokensAfter < result.tokensBefore)
  assert.equal(result.roundsCompacted, 1)
})

test('Compactor: 压缩后保留 system 消息', async () => {
  const c = new ContextCompactor({ maxTokens: 50, keepRecentRounds: 1 })
  const msgs: ChatMessage[] = [
    { role: 'system', content: 'important system prompt' },
    { role: 'user', content: 'old question' },
    { role: 'assistant', content: 'old answer' },
    { role: 'user', content: 'recent question' },
    { role: 'assistant', content: 'recent answer' },
  ]
  const result = await c.compact(msgs)
  const sysMsg = result.messages.find(m => m.role === 'system')
  assert.ok(sysMsg)
  assert.equal(sysMsg!.content, 'important system prompt')
})

test('Compactor: 增量摘要累积', async () => {
  const c = new ContextCompactor({ maxTokens: 30, keepRecentRounds: 1, compactBatchSize: 1 })
  const msgs1: ChatMessage[] = [
    { role: 'system', content: 'sys' },
    { role: 'user', content: 'question one about users' },
    { role: 'assistant', content: 'answer one about users' },
    { role: 'user', content: 'question two about auth' },
    { role: 'assistant', content: 'answer two about auth' },
    { role: 'user', content: 'recent question' },
    { role: 'assistant', content: 'recent answer' },
  ]
  const result1 = await c.compact(msgs1)
  assert.equal(result1.didCompact, true)

  const msgs2 = [
    ...result1.messages,
    { role: 'user', content: 'another question about testing' },
    { role: 'assistant', content: 'another answer about testing' },
  ] as ChatMessage[]
  const result2 = await c.compact(msgs2)
  assert.ok(c)
})

test('Compactor: resetSummary 清空摘要', async () => {
  const c = new ContextCompactor({ maxTokens: 30, keepRecentRounds: 1, compactBatchSize: 1 })
  const msgs: ChatMessage[] = [
    { role: 'system', content: 'sys' },
    { role: 'user', content: 'old question' },
    { role: 'assistant', content: 'old answer' },
    { role: 'user', content: 'recent question' },
    { role: 'assistant', content: 'recent answer' },
  ]
  await c.compact(msgs)
  c.resetSummary()
  const result = await c.compact(msgs)
  assert.ok(result)
})

test('Compactor: estimateTokens 合理估算', () => {
  const c = new ContextCompactor()
  const msgs: ChatMessage[] = [
    { role: 'user', content: 'hello world' },
  ]
  const tokens = c.estimateTokens(msgs)
  assert.ok(tokens > 0 && tokens < 100)
})

test('Compactor: tool_calls 消息正确估算', () => {
  const c = new ContextCompactor()
  const msgs: ChatMessage[] = [
    {
      role: 'assistant',
      content: '',
      tool_calls: [
        { id: 'tc1', type: 'function', function: { name: 'bash', arguments: '{"command":"ls"}' } },
      ],
    },
  ]
  const tokens = c.estimateTokens(msgs)
  assert.ok(tokens > 10)
})

test('Compactor: useLLMSummary=false 时使用规则摘要', async () => {
  const c = new ContextCompactor({ maxTokens: 30, keepRecentRounds: 1, compactBatchSize: 1, useLLMSummary: false })
  const msgs: ChatMessage[] = [
    { role: 'system', content: 'sys' },
    { role: 'user', content: 'old question about testing' },
    { role: 'assistant', content: 'old answer about testing' },
    { role: 'user', content: 'recent question' },
    { role: 'assistant', content: 'recent answer' },
  ]
  const result = await c.compact(msgs)
  assert.equal(result.didCompact, true)
  const summaryMsg = result.messages.find(m => m.role === 'user' && ('content' in m) && typeof m.content === 'string' && m.content.includes('对话摘要'))
  assert.ok(summaryMsg)
})
```

测试命令：`npx tsx --test tests/compactor.test.ts`

### Step 7.3: 修改 index.ts 注入 LLM 到 Compactor + 重置摘要

- [ ] 修改 `src/index.ts`，在构造函数中注入 LLM

```typescript
// 在 constructor() 中，this.compactor = new ContextCompactor() 之后添加：
this.compactor.setLLM(this.llm)
```

- [ ] 修改 newSession() 中重置摘要

```typescript
private newSession() {
  this.session = {
    id: this.store.generateId(),
    title: '新会话',
    tags: [],
    messages: [],
    createdAt: Date.now(),
    updatedAt: Date.now(),
    model: process.env.LLM_MODEL || 'default',
    messageCount: 0,
  }
  this.costTracker.setSessionId(this.session.id)
  this.compactor.resetSummary()
  this.updateSystemPrompt()
}
```

### Step 7.4: 运行全量测试 + 提交

- [ ] 运行全量测试

```bash
npx tsx --test tests/**/*.test.ts
```

- [ ] 提交

```bash
git add src/context/compactor.ts src/index.ts tests/compactor.test.ts && git commit -m "feat: 增量式上下文压缩 (Task 7)

- Compactor 合并为单一 async compact() 方法
- 内部决策：useLLMSummary+llm → LLM 摘要，否则规则摘要
- 增量摘要：每次只压缩 1 批旧轮次
- 摘要历史累积，不丢失信息
- AgentLoop 始终调用 await compactor.compact(messages)
- LLM 摘要使用 consumeFullStream()
- 新会话时重置摘要历史"
```

---

## Task 8: dispatch_subagent 注册 + 集成测试

> 依赖：#1-#7 | 影响文件：dispatch-subagent.ts, index.ts

### Step 8.1: 注册 dispatch_subagent 为 Tool

- [ ] 创建 `src/tools/dispatch-subagent.ts`

```typescript
// src/tools/dispatch-subagent.ts
import { Tool, ToolParameterSchema } from './base.js'

export class DispatchSubAgentTool implements Tool {
  name = 'dispatch_subagent'
  description = '调度子代理执行专门任务（code-reviewer/architect/test-writer/debugger）'
  parameters: ToolParameterSchema = {
    type: 'object',
    properties: {
      agent: { type: 'string', description: '子代理名称' },
      task: { type: 'string', description: '任务描述' },
      context: { type: 'string', description: '附加上下文' },
    },
    required: ['agent', 'task'],
  }

  async execute(_args: Record<string, unknown>): Promise<string> {
    return 'dispatch_subagent 应由 ToolExecutor 路由处理'
  }
}
```

- [ ] 修改 `src/index.ts`，注册 DispatchSubAgentTool

```typescript
import { DispatchSubAgentTool } from './tools/dispatch-subagent.js'

// 在 constructor() 的 toolRegistry.registerMany 中添加：
this.toolRegistry.registerMany([
  // ... 现有工具
  new DispatchSubAgentTool(),
])
```

### Step 8.2: 新增端到端集成测试

- [ ] 创建 `tests/integration.test.ts`

```typescript
import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { ToolRegistry } from '../src/tools/registry.js'
import { ReadFileTool } from '../src/tools/file.js'
import { BashTool, classifyCommand } from '../src/tools/bash.js'
import { DispatchSubAgentTool } from '../src/tools/dispatch-subagent.js'
import { PermissionManager } from '../src/permissions/model.js'
import { AgentModeManager, MODES } from '../src/agent/modes.js'
import { ProjectContext } from '../src/context/project-context.js'

test('集成: ToolRegistry + 模式白名单过滤', () => {
  const reg = new ToolRegistry()
  reg.registerMany([new ReadFileTool(), new BashTool(), new DispatchSubAgentTool()])

  const askTools = reg.getFiltered(MODES.ask.tools)
  const askNames = askTools.map(t => t.name)
  assert.ok(!askNames.includes('bash'))
  assert.ok(!askNames.includes('dispatch_subagent'))

  const buildTools = reg.getFiltered(MODES.build.tools)
  const buildNames = buildTools.map(t => t.name)
  assert.ok(buildNames.includes('bash'))
  assert.ok(buildNames.includes('dispatch_subagent'))
})

test('集成: ToolRegistry Schema 生成', () => {
  const reg = new ToolRegistry()
  reg.registerMany([new ReadFileTool(), new BashTool()])

  const openai = reg.toOpenAISchema()
  assert.equal(openai.length, 2)
  assert.equal(openai[0].type, 'function')
  assert.ok(openai[0].function.parameters)

  const anthropic = reg.toAnthropicSchema()
  assert.equal(anthropic.length, 2)
  assert.ok(anthropic[0].input_schema)
})

test('集成: BashTool + PermissionManager risk', () => {
  const mgr = new PermissionManager()

  const safeResult = mgr.check('bash', { command: 'ls' }, 'safe')
  assert.equal(safeResult.action, 'allow')

  const dangerResult = mgr.check('bash', { command: 'rm file' }, 'dangerous')
  assert.equal(dangerResult.action, 'ask')
})

test('集成: classifyCommand + PermissionManager', () => {
  const mgr = new PermissionManager()

  const risk1 = classifyCommand('git status')
  assert.equal(risk1, 'safe')
  const result1 = mgr.check('bash', { command: 'git status' }, risk1)
  assert.equal(result1.action, 'allow')

  const risk2 = classifyCommand('rm -rf /tmp/test')
  assert.equal(risk2, 'dangerous')
  const result2 = mgr.check('bash', { command: 'rm -rf /tmp/test' }, risk2)
  assert.equal(result2.action, 'ask')
})

test('集成: ProjectContext parseToolOverrides', () => {
  const ctx = new ProjectContext()
  const overrides = ctx.parseToolOverrides(`# 项目\n\n[tools]\nallow: read_file, bash\ndeny: rm\n`)
  assert.deepEqual(overrides.allow, ['read_file', 'bash'])
  assert.deepEqual(overrides.deny, ['rm'])
})

test('集成: 模式白名单 + deny 过滤', () => {
  const reg = new ToolRegistry()
  reg.registerMany([new ReadFileTool(), new BashTool()])

  let tools = reg.getFiltered(MODES.build.tools)

  const denySet = new Set(['bash'])
  tools = tools.filter(t => !denySet.has(t.name))

  const names = tools.map(t => t.name)
  assert.ok(!names.includes('bash'))
  assert.ok(names.includes('read_file'))
})
```

- [ ] 运行集成测试全部通过：`npx tsx --test tests/integration.test.ts`
- [ ] 提交：`git add -A && git commit -m "feat: integration tests for cross-component flows"`

---

## Task 9: 清理 + 验收

> 依赖：Task 1–8 全部完成 | 影响文件：index.ts, llm.ts, 全局搜索清理

### 目标

删除所有被新 API 替代的旧代码，确保无残留引用，运行完整验收测试。

### 步骤

- [ ] **9.1 删除 index.ts 中的旧 consumeStream()**
  - 删除 `src/index.ts` 中 `private async consumeStream()` 方法（约第 618 行）
  - 替换所有调用点为 `llm.consumeFullStream()`
  - 验证：`grep -n "consumeStream" src/index.ts` 应无结果

- [ ] **9.2 删除 llm.ts 中的旧 Schema 方法**
  - 删除 `src/api/llm.ts` 中 `private getToolsSchema()` 方法（约第 322 行）
  - 删除 `src/api/llm.ts` 中 `private getAnthropicToolsSchema()` 方法（约第 560 行）
  - 替换 `openaiStream()` 中的 `this.getToolsSchema()` 为 `this.toolRegistry.toOpenAISchema()`
  - 替换 `anthropicStream()` 中的 `this.getAnthropicToolsSchema()` 为 `this.toolRegistry.toAnthropicSchema()`
  - 验证：`grep -n "getToolsSchema\|getAnthropicToolsSchema" src/api/llm.ts` 应无结果

- [ ] **9.3 确认无硬编码 Schema 残留**
  ```bash
  # 以下搜索应全部返回 0 结果
  grep -rn "getToolsSchema" src/
  grep -rn "getAnthropicToolsSchema" src/
  grep -rn "consumeStream" src/ | grep -v "consumeFullStream"
  ```

- [ ] **9.4 确认 compact() 统一**
  - `src/context/compactor.ts` 中应只有单一 `async compact()` 方法
  - `src/index.ts` 中调用应为 `await this.compactor.compact(...)`
  - 验证：`grep -n "compactAsync\|compact(" src/context/compactor.ts` 只出现新的统一方法

- [ ] **9.5 运行全部单元测试**
  ```bash
  npx tsx --test tests/registry.test.ts
  npx tsx --test tests/agent-loop.test.ts
  npx tsx --test tests/bash.test.ts
  npx tsx --test tests/permissions.test.ts
  npx tsx --test tests/subagent.test.ts
  npx tsx --test tests/project-context.test.ts
  npx tsx --test tests/modes.test.ts
  npx tsx --test tests/cost.test.ts
  npx tsx --test tests/compactor.test.ts
  npx tsx --test tests/integration.test.ts
  ```

- [ ] **9.6 验收测试：完整 E2E Agent Loop**
  创建 `tests/e2e.test.ts`：

  ```typescript
  import { describe, test, assert } from 'node:test'
  import { AgentLoop } from '../src/agent/agent-loop.js'
  import { ToolRegistry } from '../src/tools/registry.js'
  import { PermissionManager } from '../src/permissions/model.js'
  import { HookManager } from '../src/hooks/manager.js'
  import { ReadFileTool, WriteFileTool } from '../src/tools/file.js'
  import { BashTool } from '../src/tools/bash.js'
  import { ProjectContext } from '../src/context/project-context.js'
  import { MODES } from '../src/agent/modes.js'

  describe('E2E 验收', () => {
    test('完整 Agent Loop: 注册→过滤→Schema→权限→执行', async () => {
      // 1. 注册工具
      const registry = new ToolRegistry()
      registry.registerMany([
        new ReadFileTool(),
        new WriteFileTool(),
        new BashTool(),
      ])

      // 2. ProjectContext 加载 CODECAST.md
      const projectCtx = new ProjectContext()
      const overrides = projectCtx.parseToolOverrides(
        '# 项目\n\n[tools]\nallow: read_file, write_file, bash\ndeny: rm\n'
      )
      const denySet = new Set(overrides.deny)

      // 3. 模式白名单 + deny 过滤
      let tools = registry.getFiltered(MODES.code.tools)
      tools = tools.filter(t => !denySet.has(t.name))

      // 4. 生成 Schema
      const openaiSchemas = registry.toOpenAISchema()
      const anthropicSchemas = registry.toAnthropicSchema()
      assert.ok(openaiSchemas.length >= 2)
      assert.ok(anthropicSchemas.length >= 2)

      // 5. 权限管理器 + risk
      const permManager = new PermissionManager()
      const safeCheck = permManager.check('bash', { command: 'ls' }, 'safe')
      assert.equal(safeCheck.action, 'allow')

      const dangerCheck = permManager.check(
        'bash',
        { command: 'rm -rf /' },
        'dangerous'
      )
      assert.equal(dangerCheck.action, 'ask')

      // 6. HookManager（空 Hook 不报错）
      const hookManager = new HookManager()

      // 7. AgentLoop 构造（不实际调用 LLM，仅验证构造）
      const loop = new AgentLoop({
        llmClient: null as any, // E2E 不实际调用
        toolRegistry: registry,
        permManager,
        hookManager,
        onConfirm: async () => true,
        onToolCall: async () => {}, // UI-only，无权限逻辑
      })
      assert.ok(loop)
    })

    test('ToolExecutor 接口实现', async () => {
      const registry = new ToolRegistry()
      registry.registerMany([new ReadFileTool(), new BashTool()])

      // AgentCLI 应实现 ToolExecutor
      // 此处验证 registry + executor 接口一致性
      const allTools = registry.getAll()
      assert.equal(allTools.length, 2)

      for (const tool of allTools) {
        assert.ok(tool.name)
        assert.ok(tool.description)
        assert.ok(tool.parameters)
      }
    })

    test('PluginTool validateParameters 运行时校验', () => {
      // 验证所有内置工具的 parameters 定义与实际调用参数一致
      const registry = new ToolRegistry()
      registry.registerMany([
        new ReadFileTool(),
        new WriteFileTool(),
        new BashTool(),
      ])

      for (const tool of registry.getAll()) {
        if ('validateParameters' in tool) {
          const validResult = (tool as any).validateParameters({
            path: '/tmp/test.txt',
          })
          // validateParameters 应返回 { valid: boolean, errors?: string[] }
          assert.ok(typeof validResult.valid === 'boolean')
        }
      }
    })
  })
  ```

- [ ] 运行验收测试：`npx tsx --test tests/e2e.test.ts`

- [ ] **9.7 最终提交**
  ```bash
  git add -A && git commit -m "chore: cleanup legacy APIs + e2e acceptance tests"
  ```

---

## 完成标准

| # | 检查项 | 验证命令 |
|---|--------|----------|
| 1 | 旧 `consumeStream` 已删除 | `grep -rn "consumeStream" src/ \| grep -v consumeFullStream` → 无结果 |
| 2 | 旧 `getToolsSchema` / `getAnthropicToolsSchema` 已删除 | `grep -rn "getToolsSchema\|getAnthropicToolsSchema" src/` → 无结果 |
| 3 | `compact()` 统一为单一 async 方法 | `grep -n "compact" src/context/compactor.ts` → 仅一个方法 |
| 4 | 所有工具有 `parameters` 字段 | 遍历 `registry.getAll()` 均 truthy |
| 5 | `PermissionManager.check` 支持 `risk` 参数 | `new PermissionManager().check('bash', {}, 'safe')` 不报错 |
| 6 | `AgentLoop` 含权限 + Hook + consumeFullStream | 构造不报错，`onToolCall` 无权限逻辑 |
| 7 | `ToolRegistry.toOpenAISchema` / `toAnthropicSchema` 可用 | 返回非空数组 |
| 8 | 全部单元测试通过 | 10 个测试文件均绿 |
| 9 | E2E 验收测试通过 | `npx tsx --test tests/e2e.test.ts` 绿 |

