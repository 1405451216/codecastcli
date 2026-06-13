# Codecast Agent CLI 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建一个支持多 Agent 模式（Ask/Plan/Build）、工具调用、流式输出、文件上传、会话管理的 AI Agent 命令行终端工具。

**Architecture:** 纯 TypeScript/Node.js 实现，零前端框架依赖。核心架构：ANSI 渲染引擎 + 组件系统 + LLM API 适配层 + 工具注册表 + 会话存储。TUI 采用帧缓冲双缓冲机制避免闪烁，所有交互基于纯键盘（无鼠标）。

**Tech Stack:** TypeScript 5.x, Node.js 20+, native fetch (SSE), fs (会话存储), readline (交互), dotenv (配置)

---

## 文件结构

```
codecast-agent/
├── src/
│   ├── ansi/
│   │   ├── cursor.ts         # 光标控制转义序列
│   │   ├── screen.ts         # 屏幕控制（清屏、尺寸获取）
│   │   ├── color.ts          # 颜色系统（16色/256色/真彩色）
│   │   └── style.ts          # 文本样式（粗体/下划线/反色）
│   ├── render/
│   │   ├── buffer.ts         # 帧缓冲（双缓冲差分渲染）
│   │   └── renderer.ts       # 主渲染器（60fps 循环）
│   ├── components/
│   │   ├── base.ts           # 组件基类接口
│   │   ├── box.ts            # 容器组件（flex 布局子集）
│   │   ├── text.ts           # 文本组件（颜色/截断/填充）
│   │   ├── border.ts         # 边框组件（单线/双线/圆角）
│   │   ├── input.ts          # 输入框组件（光标/编辑）
│   │   ├── list.ts           # 列表组件（选择/滚动）
│   │   ├── progress-bar.ts   # 进度条组件
│   │   ├── spinner.ts        # 加载动画组件
│   │   ├── confirm.ts        # 确认对话框组件
│   │   ├── select.ts         # 单选列表组件
│   │   ├── multi-select.ts   # 多选列表组件
│   │   └── table.ts          # 表格组件
│   ├── ui/
│   │   ├── render.ts         # 基础渲染辅助函数
│   │   ├── stream-renderer.ts # 流式输出渲染器（代码块/高亮）
│   │   └── tool-renderer.ts   # 工具调用可视化（diff/折叠）
│   ├── input/
│   │   └── keyboard.ts       # 键盘事件解析（特殊键/组合键）
│   ├── api/
│   │   └── llm.ts            # LLM API 封装（OpenAI/Anthropic）
│   ├── tools/
│   │   ├── base.ts           # 工具接口定义
│   │   ├── file.ts           # 文件读写工具
│   │   └── bash.ts           # 终端命令工具
│   ├── session/
│   │   └── store.ts          # 会话存储（JSON/搜索/导出）
│   ├── agent/
│   │   └── modes.ts          # Agent 模式配置（Ask/Plan/Build）
│   └── index.ts              # 应用入口（主循环/命令路由）
├── sessions/                 # 会话数据目录（gitignore）
├── tests/                    # 测试文件
├── .env                      # API 密钥配置
├── .env.example              # 配置模板
├── package.json
├── tsconfig.json
└── README.md
```

---

## Task 1: 项目初始化与基础配置

**Files:**
- Create: `package.json`
- Create: `tsconfig.json`
- Create: `.env.example`
- Create: `.gitignore`

- [ ] **Step 1: 初始化 package.json**

```json
{
  "name": "codecast-agent",
  "version": "0.1.0",
  "description": "AI Agent CLI for codecast",
  "type": "module",
  "bin": {
    "codecast-agent": "./dist/index.js"
  },
  "scripts": {
    "build": "tsc",
    "dev": "tsx src/index.ts",
    "test": "node --test dist/**/*.test.js"
  },
  "dependencies": {
    "dotenv": "^16.3.0"
  },
  "devDependencies": {
    "@types/node": "^20.0.0",
    "tsx": "^4.0.0",
    "typescript": "^5.0.0"
  },
  "engines": {
    "node": ">=20.0.0"
  }
}
```

- [ ] **Step 2: 初始化 tsconfig.json**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "outDir": "./dist",
    "rootDir": "./src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "resolveJsonModule": true,
    "declaration": true,
    "declarationMap": true,
    "sourceMap": true
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}
```

- [ ] **Step 3: 创建 .env.example**

```bash
# LLM Provider: openai | anthropic
LLM_PROVIDER=openai

# API Key
LLM_API_KEY=your-api-key-here

# Model (optional, defaults shown)
# LLM_MODEL=gpt-4o
# LLM_MODEL=claude-3-sonnet-20240229

# Custom API Base URL (optional)
# LLM_BASE_URL=https://custom-api.example.com/v1
```

- [ ] **Step 4: 创建 .gitignore**

```gitignore
node_modules/
dist/
sessions/
.env
*.log
.DS_Store
```

- [ ] **Step 5: 安装依赖并验证**

Run: `npm install`
Expected: `node_modules/` 目录创建，无错误

- [ ] **Step 6: Commit**

```bash
git add package.json tsconfig.json .env.example .gitignore
 git commit -m "chore: initialize project structure"
```

---

## Task 2: ANSI 底层库

**Files:**
- Create: `src/ansi/cursor.ts`
- Create: `src/ansi/screen.ts`
- Create: `src/ansi/color.ts`
- Create: `src/ansi/style.ts`
- Test: `tests/ansi.test.ts`

- [ ] **Step 1: 实现光标控制**

```typescript
// src/ansi/cursor.ts

export const cursor = {
  up: (n = 1): string => `\x1b[${n}A`,
  down: (n = 1): string => `\x1b[${n}B`,
  right: (n = 1): string => `\x1b[${n}C`,
  left: (n = 1): string => `\x1b[${n}D`,
  moveTo: (row: number, col: number): string => `\x1b[${row};${col}H`,
  save: (): string => '\x1b[s',
  restore: (): string => '\x1b[u',
  hide: (): string => '\x1b[?25l',
  show: (): string => '\x1b[?25h',
}
```

- [ ] **Step 2: 实现屏幕控制**

```typescript
// src/ansi/screen.ts

import { execSync } from 'child_process'

export const screen = {
  clear: (): string => '\x1b[2J\x1b[H',
  clearLineEnd: (): string => '\x1b[K',
  clearLine: (): string => '\x1b[2K',
  clearDown: (): string => '\x1b[J',
  enterAlternate: (): string => '\x1b[?1049h',
  exitAlternate: (): string => '\x1b[?1049l',
  
  getSize(): { width: number; height: number } {
    return {
      width: process.stdout.columns || 80,
      height: process.stdout.rows || 24,
    }
  },
}
```

- [ ] **Step 3: 实现颜色系统**

```typescript
// src/ansi/color.ts

export type Color = 
  | 'black' | 'red' | 'green' | 'yellow' 
  | 'blue' | 'magenta' | 'cyan' | 'white'
  | 'gray' | 'brightRed' | 'brightGreen' | 'brightYellow'
  | 'brightBlue' | 'brightMagenta' | 'brightCyan' | 'brightWhite'
  | { r: number; g: number; b: number }
  | number

const colorMap: Record<string, number> = {
  black: 30, red: 31, green: 32, yellow: 33,
  blue: 34, magenta: 35, cyan: 36, white: 37,
  gray: 90, brightRed: 91, brightGreen: 92, brightYellow: 93,
  brightBlue: 94, brightMagenta: 95, brightCyan: 96, brightWhite: 97,
}

export function fg(color: Color): string {
  if (typeof color === 'number') return `\x1b[38;5;${color}m`
  if (typeof color === 'object') return `\x1b[38;2;${color.r};${color.g};${color.b}m`
  return `\x1b[${colorMap[color] || 37}m`
}

export function bg(color: Color): string {
  if (typeof color === 'number') return `\x1b[48;5;${color}m`
  if (typeof color === 'object') return `\x1b[48;2;${color.r};${color.g};${color.b}m`
  return `\x1b[${(colorMap[color] || 37) + 10}m`
}

export const reset = '\x1b[0m'
```

- [ ] **Step 4: 实现文本样式**

```typescript
// src/ansi/style.ts

export const style = {
  bold: '\x1b[1m',
  dim: '\x1b[2m',
  italic: '\x1b[3m',
  underline: '\x1b[4m',
  blink: '\x1b[5m',
  reverse: '\x1b[7m',
  hidden: '\x1b[8m',
  strikethrough: '\x1b[9m',
  reset: '\x1b[0m',
}
```

- [ ] **Step 5: 编写测试**

```typescript
// tests/ansi.test.ts

import { cursor } from '../src/ansi/cursor.js'
import { screen } from '../src/ansi/screen.js'
import { fg, bg, reset } from '../src/ansi/color.js'
import { style } from '../src/ansi/style.js'
import { describe, it } from 'node:test'
import assert from 'node:assert'

describe('ANSI', () => {
  it('cursor up', () => {
    assert.strictEqual(cursor.up(3), '\x1b[3A')
  })
  
  it('cursor moveTo', () => {
    assert.strictEqual(cursor.moveTo(10, 20), '\x1b[10;20H')
  })
  
  it('screen clear', () => {
    assert.strictEqual(screen.clear(), '\x1b[2J\x1b[H')
  })
  
  it('fg color name', () => {
    assert.strictEqual(fg('red'), '\x1b[31m')
  })
  
  it('fg color rgb', () => {
    assert.strictEqual(fg({ r: 255, g: 0, b: 0 }), '\x1b[38;2;255;0;0m')
  })
  
  it('fg color 256', () => {
    assert.strictEqual(fg(196), '\x1b[38;5;196m')
  })
  
  it('style bold', () => {
    assert.strictEqual(style.bold, '\x1b[1m')
  })
})
```

- [ ] **Step 6: 运行测试**

Run: `npm run build && node --test dist/tests/ansi.test.js`
Expected: 7 tests passed

- [ ] **Step 7: Commit**

```bash
git add src/ansi/ tests/ansi.test.ts
git commit -m "feat(ansi): implement cursor, screen, color, style primitives"
```

---

## Task 3: 渲染引擎

**Files:**
- Create: `src/render/buffer.ts`
- Create: `src/render/renderer.ts`
- Test: `tests/render.test.ts`

- [ ] **Step 1: 实现帧缓冲**

```typescript
// src/render/buffer.ts

export class FrameBuffer {
  private front: string[][] = []
  private back: string[][] = []
  width = 0
  height = 0
  
  resize(width: number, height: number) {
    this.width = width
    this.height = height
    this.front = Array.from({ length: height }, () => Array(width).fill(' '))
    this.back = Array.from({ length: height }, () => Array(width).fill(' '))
  }
  
  write(row: number, col: number, char: string) {
    if (row >= 0 && row < this.height && col >= 0 && col < this.width) {
      this.back[row][col] = char
    }
  }
  
  writeLine(row: number, line: string) {
    for (let i = 0; i < Math.min(line.length, this.width); i++) {
      this.write(row, i, line[i])
    }
  }
  
  clear() {
    for (let r = 0; r < this.height; r++) {
      for (let c = 0; c < this.width; c++) {
        this.back[r][c] = ' '
      }
    }
  }
  
  swap(): string {
    let output = ''
    let cursorRow = -1
    let cursorCol = -1
    
    for (let row = 0; row < this.height; row++) {
      for (let col = 0; col < this.width; col++) {
        if (this.front[row][col] !== this.back[row][col]) {
          if (cursorRow !== row || cursorCol !== col) {
            output += `\x1b[${row + 1};${col + 1}H`
            cursorRow = row
            cursorCol = col
          }
          output += this.back[row][col]
          cursorCol++
        }
      }
    }
    
    // Swap references
    const temp = this.front
    this.front = this.back
    this.back = temp
    
    return output
  }
  
  get(row: number, col: number): string {
    if (row >= 0 && row < this.height && col >= 0 && col < this.width) {
      return this.front[row][col]
    }
    return ' '
  }
}
```

- [ ] **Step 2: 实现主渲染器**

```typescript
// src/render/renderer.ts

import { screen, cursor } from '../ansi/index.js'
import { FrameBuffer } from './buffer.js'

export interface RendererConfig {
  fps?: number
  showCursor?: boolean
}

export class Renderer {
  private buffer = new FrameBuffer()
  private stdout: NodeJS.WriteStream
  private config: Required<RendererConfig>
  private cursorRow = 0
  private cursorCol = 0
  private cursorVisible = false
  private running = false
  private animationId: ReturnType<typeof setInterval> | null = null
  
  constructor(stdout = process.stdout, config: RendererConfig = {}) {
    this.stdout = stdout
    this.config = {
      fps: config.fps || 30,
      showCursor: config.showCursor ?? false,
    }
  }
  
  init() {
    this.stdout.write(screen.enterAlternate())
    this.stdout.write(cursor.hide())
    this.resize()
    
    process.on('SIGWINCH', () => this.resize())
    process.on('exit', () => this.destroy())
  }
  
  destroy() {
    if (this.animationId) {
      clearInterval(this.animationId)
      this.animationId = null
    }
    this.stdout.write(cursor.show())
    this.stdout.write(screen.exitAlternate())
  }
  
  private resize() {
    const { width, height } = screen.getSize()
    this.buffer.resize(width, height)
  }
  
  setCursor(row: number, col: number, visible = true) {
    this.cursorRow = Math.max(0, Math.min(row, this.buffer.height - 1))
    this.cursorCol = Math.max(0, Math.min(col, this.buffer.width - 1))
    this.cursorVisible = visible
  }
  
  clear() {
    this.buffer.clear()
  }
  
  render(drawFn: (buf: FrameBuffer) => void) {
    this.buffer.clear()
    drawFn(this.buffer)
    
    const diff = this.buffer.swap()
    if (diff) {
      this.stdout.write(diff)
    }
    
    // Update cursor
    if (this.cursorVisible) {
      this.stdout.write(cursor.moveTo(this.cursorRow + 1, this.cursorCol + 1))
      this.stdout.write(cursor.show())
    } else {
      this.stdout.write(cursor.hide())
    }
  }
  
  startRenderLoop(drawFn: (buf: FrameBuffer) => void) {
    this.running = true
    const interval = 1000 / this.config.fps
    
    this.animationId = setInterval(() => {
      if (!this.running) return
      this.render(drawFn)
    }, interval)
  }
  
  stopRenderLoop() {
    this.running = false
    if (this.animationId) {
      clearInterval(this.animationId)
      this.animationId = null
    }
  }
  
  getSize() {
    return { width: this.buffer.width, height: this.buffer.height }
  }
}
```

- [ ] **Step 3: 创建 ANSI 模块入口**

```typescript
// src/ansi/index.ts

export { cursor } from './cursor.js'
export { screen } from './screen.js'
export { fg, bg, reset, type Color } from './color.js'
export { style } from './style.js'
```

- [ ] **Step 4: 编写测试**

```typescript
// tests/render.test.ts

import { FrameBuffer } from '../src/render/buffer.js'
import { describe, it } from 'node:test'
import assert from 'node:assert'

describe('FrameBuffer', () => {
  it('resize and write', () => {
    const buf = new FrameBuffer()
    buf.resize(10, 5)
    
    buf.write(0, 0, 'H')
    buf.write(0, 1, 'i')
    
    assert.strictEqual(buf.get(0, 0), ' ')
    assert.strictEqual(buf.get(0, 1), ' ')
    
    const diff = buf.swap()
    assert.ok(diff.includes('H'))
    assert.ok(diff.includes('i'))
    
    assert.strictEqual(buf.get(0, 0), 'H')
    assert.strictEqual(buf.get(0, 1), 'i')
  })
  
  it('writeLine', () => {
    const buf = new FrameBuffer()
    buf.resize(20, 3)
    buf.writeLine(1, 'Hello')
    
    buf.swap()
    assert.strictEqual(buf.get(1, 0), 'H')
    assert.strictEqual(buf.get(1, 4), 'o')
    assert.strictEqual(buf.get(1, 5), ' ')
  })
  
  it('clear', () => {
    const buf = new FrameBuffer()
    buf.resize(5, 3)
    buf.write(0, 0, 'X')
    buf.swap()
    
    buf.clear()
    const diff = buf.swap()
    assert.ok(diff.includes('X'))
    assert.strictEqual(buf.get(0, 0), ' ')
  })
})
```

- [ ] **Step 5: 运行测试**

Run: `npm run build && node --test dist/tests/render.test.js`
Expected: 3 tests passed

- [ ] **Step 6: Commit**

```bash
git add src/render/ tests/render.test.ts src/ansi/index.ts
git commit -m "feat(render): implement double-buffered frame renderer"
```

---

## Task 4: 键盘输入系统

**Files:**
- Create: `src/input/keyboard.ts`
- Test: `tests/keyboard.test.ts`

- [ ] **Step 1: 实现键盘事件解析**

```typescript
// src/input/keyboard.ts

import { EventEmitter } from 'events'

export interface KeyEvent {
  name: string
  ctrl: boolean
  shift: boolean
  meta: boolean
  sequence: string
  raw: Buffer
}

export class Keyboard extends EventEmitter {
  private stdin: NodeJS.ReadStream
  private active = false
  
  constructor(stdin = process.stdin) {
    super()
    this.stdin = stdin
  }
  
  start() {
    if (this.active) return
    this.active = true
    
    this.stdin.setRawMode(true)
    this.stdin.resume()
    this.stdin.setEncoding(null)
    this.stdin.on('data', this.handleData.bind(this))
  }
  
  stop() {
    if (!this.active) return
    this.active = false
    
    this.stdin.setRawMode(false)
    this.stdin.pause()
    this.stdin.removeAllListeners('data')
  }
  
  private handleData(data: Buffer) {
    const key = this.parseKey(data)
    if (key) {
      this.emit('key', key)
      
      // Emit named event for convenience
      if (key.ctrl && key.name.length === 1) {
        this.emit(`ctrl+${key.name}`, key)
      }
      this.emit(key.name, key)
    }
  }
  
  private parseKey(data: Buffer): KeyEvent | null {
    const str = data.toString('utf8')
    
    // Handle multi-byte sequences
    if (str.length > 1 || data[0] > 127) {
      return this.parseSequence(data, str)
    }
    
    const ch = data[0]
    
    // Ctrl+letter (1-26)
    if (ch >= 1 && ch <= 26) {
      return {
        name: String.fromCharCode(ch + 96),
        ctrl: true,
        shift: false,
        meta: false,
        sequence: str,
        raw: data,
      }
    }
    
    // Special characters
    const specialMap: Record<number, string> = {
      9: 'tab',
      13: 'return',
      27: 'escape',
      32: 'space',
      127: 'backspace',
    }
    
    if (specialMap[ch]) {
      return {
        name: specialMap[ch],
        ctrl: false,
        shift: false,
        meta: false,
        sequence: str,
        raw: data,
      }
    }
    
    // Regular printable character
    if (ch >= 32 && ch <= 126) {
      return {
        name: str,
        ctrl: false,
        shift: false,
        meta: false,
        sequence: str,
        raw: data,
      }
    }
    
    return null
  }
  
  private parseSequence(data: Buffer, str: string): KeyEvent | null {
    const sequences: Record<string, Partial<KeyEvent>> = {
      '\x1b[A': { name: 'up' },
      '\x1b[B': { name: 'down' },
      '\x1b[C': { name: 'right' },
      '\x1b[D': { name: 'left' },
      '\x1b[H': { name: 'home' },
      '\x1b[F': { name: 'end' },
      '\x1b[3~': { name: 'delete' },
      '\x1b[5~': { name: 'pageup' },
      '\x1b[6~': { name: 'pagedown' },
      '\x1b\x1b[A': { name: 'up', meta: true },
      '\x1b\x1b[B': { name: 'down', meta: true },
      '\x1b\x1b[C': { name: 'right', meta: true },
      '\x1b\x1b[D': { name: 'left', meta: true },
      '\x1b[1;2A': { name: 'up', shift: true },
      '\x1b[1;2B': { name: 'down', shift: true },
      '\x1b[1;2C': { name: 'right', shift: true },
      '\x1b[1;2D': { name: 'left', shift: true },
      '\x1b[1;5A': { name: 'up', ctrl: true },
      '\x1b[1;5B': { name: 'down', ctrl: true },
      '\x1b[1;5C': { name: 'right', ctrl: true },
      '\x1b[1;5D': { name: 'left', ctrl: true },
    }
    
    const match = sequences[str]
    if (match) {
      return {
        name: match.name!,
        ctrl: match.ctrl || false,
        shift: match.shift || false,
        meta: match.meta || false,
        sequence: str,
        raw: data,
      }
    }
    
    // Meta+key (ESC followed by char)
    if (data[0] === 0x1b && data.length === 2) {
      const char = String.fromCharCode(data[1])
      return {
        name: char,
        ctrl: false,
        shift: false,
        meta: true,
        sequence: str,
        raw: data,
      }
    }
    
    // Unicode character
    if (str.length > 0 && data[0] > 127) {
      return {
        name: str,
        ctrl: false,
        shift: false,
        meta: false,
        sequence: str,
        raw: data,
      }
    }
    
    return null
  }
}
```

- [ ] **Step 2: 编写测试**

```typescript
// tests/keyboard.test.ts

import { Keyboard } from '../src/input/keyboard.js'
import { describe, it } from 'node:test'
import assert from 'node:assert'

class MockStream {
  handlers: ((data: Buffer) => void)[] = []
  isRaw = false
  
  setRawMode(mode: boolean) {
    this.isRaw = mode
  }
  
  resume() {}
  pause() {}
  setEncoding() {}
  
  on(event: string, handler: (data: Buffer) => void) {
    if (event === 'data') this.handlers.push(handler)
  }
  
  removeAllListeners() {
    this.handlers = []
  }
  
  emit(data: Buffer) {
    this.handlers.forEach(h => h(data))
  }
}

describe('Keyboard', () => {
  it('parses regular character', () => {
    const stream = new MockStream()
    const kb = new Keyboard(stream as any)
    
    let captured: any = null
    kb.on('key', (k) => { captured = k })
    kb.start()
    
    stream.emit(Buffer.from('a'))
    assert.strictEqual(captured?.name, 'a')
    assert.strictEqual(captured?.ctrl, false)
  })
  
  it('parses ctrl+c', () => {
    const stream = new MockStream()
    const kb = new Keyboard(stream as any)
    
    let captured: any = null
    kb.on('key', (k) => { captured = k })
    kb.start()
    
    stream.emit(Buffer.from([3]))
    assert.strictEqual(captured?.name, 'c')
    assert.strictEqual(captured?.ctrl, true)
  })
  
  it('parses arrow keys', () => {
    const stream = new MockStream()
    const kb = new Keyboard(stream as any)
    
    const keys: string[] = []
    kb.on('key', (k) => keys.push(k.name))
    kb.start()
    
    stream.emit(Buffer.from('\x1b[A'))
    stream.emit(Buffer.from('\x1b[B'))
    stream.emit(Buffer.from('\x1b[C'))
    stream.emit(Buffer.from('\x1b[D'))
    
    assert.deepStrictEqual(keys, ['up', 'down', 'right', 'left'])
  })
  
  it('parses escape', () => {
    const stream = new MockStream()
    const kb = new Keyboard(stream as any)
    
    let captured: any = null
    kb.on('key', (k) => { captured = k })
    kb.start()
    
    stream.emit(Buffer.from([27]))
    assert.strictEqual(captured?.name, 'escape')
  })
})
```

- [ ] **Step 3: 运行测试**

Run: `npm run build && node --test dist/tests/keyboard.test.js`
Expected: 4 tests passed

- [ ] **Step 4: Commit**

```bash
git add src/input/ tests/keyboard.test.ts
git commit -m "feat(input): implement keyboard event parser with modifier support"
```

---

## Task 5: 基础 UI 组件

**Files:**
- Create: `src/components/base.ts`
- Create: `src/components/box.ts`
- Create: `src/components/text.ts`
- Create: `src/components/border.ts`
- Create: `src/components/input.ts`
- Create: `src/ui/render.ts`
- Test: `tests/components.test.ts`

- [ ] **Step 1: 定义组件基类**

```typescript
// src/components/base.ts

import { FrameBuffer } from '../render/buffer.js'

export interface Component {
  width: number
  height: number
  render(buf: FrameBuffer, x: number, y: number): void
}

export interface LayoutProps {
  direction?: 'row' | 'column'
  gap?: number
  padding?: number
  paddingTop?: number
  paddingBottom?: number
  paddingLeft?: number
  paddingRight?: number
}
```

- [ ] **Step 2: 实现 Box 容器**

```typescript
// src/components/box.ts

import { Component, LayoutProps } from './base.js'
import { FrameBuffer } from '../render/buffer.js'

export interface BoxProps extends LayoutProps {
  width: number
  height: number
  bg?: string
  children?: Component[]
}

export class Box implements Component {
  width: number
  height: number
  private props: BoxProps
  
  constructor(props: BoxProps) {
    this.props = props
    this.width = props.width
    this.height = props.height
  }
  
  render(buf: FrameBuffer, x: number, y: number) {
    const { direction = 'column', gap = 0, children = [] } = this.props
    
    const pl = this.props.paddingLeft ?? this.props.padding ?? 0
    const pr = this.props.paddingRight ?? this.props.padding ?? 0
    const pt = this.props.paddingTop ?? this.props.padding ?? 0
    const pb = this.props.paddingBottom ?? this.props.padding ?? 0
    
    let cx = x + pl
    let cy = y + pt
    
    for (const child of children) {
      child.render(buf, cx, cy)
      
      if (direction === 'row') {
        cx += child.width + gap
      } else {
        cy += child.height + gap
      }
    }
  }
}
```

- [ ] **Step 3: 实现 Text 组件**

```typescript
// src/components/text.ts

import { Component } from './base.js'
import { FrameBuffer } from '../render/buffer.js'
import { fg, bg, reset, type Color } from '../ansi/color.js'

export interface TextProps {
  content: string
  width?: number
  color?: Color
  bgColor?: Color
  bold?: boolean
  align?: 'left' | 'center' | 'right'
}

export class Text implements Component {
  width: number
  height = 1
  private props: TextProps
  
  constructor(props: TextProps) {
    this.props = props
    this.width = props.width ?? props.content.length
  }
  
  render(buf: FrameBuffer, x: number, y: number) {
    let text = this.props.content.slice(0, this.width)
    
    // Alignment
    if (this.props.align === 'center') {
      text = text.padStart((this.width + text.length) / 2).padEnd(this.width)
    } else if (this.props.align === 'right') {
      text = text.padStart(this.width)
    } else {
      text = text.padEnd(this.width)
    }
    
    for (let i = 0; i < text.length; i++) {
      buf.write(y, x + i, text[i])
    }
  }
  
  toString(): string {
    let out = ''
    if (this.props.bold) out += '\x1b[1m'
    if (this.props.color) out += fg(this.props.color)
    if (this.props.bgColor) out += bg(this.props.bgColor)
    out += this.props.content
    out += reset
    return out
  }
}
```

- [ ] **Step 4: 实现 Border 组件**

```typescript
// src/components/border.ts

import { Component } from './base.js'
import { FrameBuffer } from '../render/buffer.js'

export type BorderStyle = 'single' | 'double' | 'round'

const chars: Record<BorderStyle, { h: string; v: string; tl: string; tr: string; bl: string; br: string }> = {
  single: { h: '─', v: '│', tl: '┌', tr: '┐', bl: '└', br: '┘' },
  double: { h: '═', v: '║', tl: '╔', tr: '╗', bl: '╚', br: '╝' },
  round:  { h: '─', v: '│', tl: '╭', tr: '╮', bl: '╰', br: '╯' },
}

export interface BorderProps {
  width: number
  height: number
  style?: BorderStyle
  title?: string
}

export class Border implements Component {
  width: number
  height: number
  private props: BorderProps
  private c: ReturnType<typeof chars[keyof typeof chars]>
  
  constructor(props: BorderProps) {
    this.props = props
    this.width = props.width
    this.height = props.height
    this.c = chars[props.style || 'single']
  }
  
  render(buf: FrameBuffer, x: number, y: number) {
    const { width, height, title } = this.props
    const { c } = this
    
    // Top border
    for (let i = 1; i < width - 1; i++) {
      buf.write(y, x + i, c.h)
    }
    buf.write(y, x, c.tl)
    buf.write(y, x + width - 1, c.tr)
    
    // Title
    if (title) {
      const start = Math.floor((width - title.length) / 2)
      for (let i = 0; i < title.length && start + i < width - 1; i++) {
        buf.write(y, x + start + i, title[i])
      }
    }
    
    // Side borders
    for (let i = 1; i < height - 1; i++) {
      buf.write(y + i, x, c.v)
      buf.write(y + i, x + width - 1, c.v)
    }
    
    // Bottom border
    for (let i = 1; i < width - 1; i++) {
      buf.write(y + height - 1, x + i, c.h)
    }
    buf.write(y + height - 1, x, c.bl)
    buf.write(y + height - 1, x + width - 1, c.br)
  }
}
```

- [ ] **Step 5: 实现 Input 组件**

```typescript
// src/components/input.ts

import { Component } from './base.js'
import { FrameBuffer } from '../render/buffer.js'

export interface InputProps {
  width: number
  placeholder?: string
  value?: string
  focused?: boolean
  password?: boolean
}

export class Input implements Component {
  width: number
  height = 1
  private props: InputProps
  private _value = ''
  private _cursor = 0
  
  constructor(props: InputProps) {
    this.props = props
    this.width = props.width
    this._value = props.value || ''
    this._cursor = this._value.length
  }
  
  get value() { return this._value }
  get cursor() { return this._cursor }
  get focused() { return this.props.focused ?? false }
  
  set value(v: string) {
    this._value = v
    this._cursor = Math.min(this._cursor, v.length)
  }
  
  insert(char: string) {
    const before = this._value.slice(0, this._cursor)
    const after = this._value.slice(this._cursor)
    this._value = before + char + after
    this._cursor++
  }
  
  backspace() {
    if (this._cursor > 0) {
      const before = this._value.slice(0, this._cursor - 1)
      const after = this._value.slice(this._cursor)
      this._value = before + after
      this._cursor--
    }
  }
  
  delete() {
    if (this._cursor < this._value.length) {
      const before = this._value.slice(0, this._cursor)
      const after = this._value.slice(this._cursor + 1)
      this._value = before + after
    }
  }
  
  moveLeft() { this._cursor = Math.max(0, this._cursor - 1) }
  moveRight() { this._cursor = Math.min(this._value.length, this._cursor + 1) }
  moveHome() { this._cursor = 0 }
  moveEnd() { this._cursor = this._value.length }
  
  render(buf: FrameBuffer, x: number, y: number) {
    const display = this.props.password 
      ? '•'.repeat(this._value.length)
      : this._value
    
    const text = display || this.props.placeholder || ''
    const padded = text.slice(0, this.width).padEnd(this.width, ' ')
    
    for (let i = 0; i < padded.length; i++) {
      buf.write(y, x + i, padded[i])
    }
  }
  
  getCursorPosition(x: number): { row: number; col: number } {
    const display = this.props.password 
      ? '•'.repeat(this._value.length)
      : this._value
    
    return {
      row: 0,
      col: x + Math.min(this._cursor, this.width - 1)
    }
  }
}
```

- [ ] **Step 6: 实现基础 UI 辅助函数**

```typescript
// src/ui/render.ts

import { fg, bg, reset } from '../ansi/color.js'

export const color = {
  reset, bold: '\x1b[1m',
  black: '\x1b[30m', red: '\x1b[31m', green: '\x1b[32m', yellow: '\x1b[33m',
  blue: '\x1b[34m', magenta: '\x1b[35m', cyan: '\x1b[36m', white: '\x1b[37m',
  gray: '\x1b[90m',
}

export function printUser(text: string) {
  console.log(`${color.cyan}You:${color.reset} ${text}`)
}

export function printAI(text: string) {
  process.stdout.write(`${color.yellow}AI:${color.reset} ${text}`)
}

export function printStream(chunk: string) {
  process.stdout.write(chunk)
}

export function printTool(name: string, args: Record<string, unknown>) {
  console.log(`${color.magenta}🔧 ${name}${color.reset} ${JSON.stringify(args)}`)
}

export function printError(text: string) {
  console.log(`${color.red}错误: ${text}${color.reset}`)
}

export function printInfo(text: string) {
  console.log(`${color.gray}${text}${color.reset}`)
}

export function printDivider(width = 60) {
  console.log(color.gray + '─'.repeat(width) + color.reset)
}

export function clearScreen() {
  process.stdout.write('\x1b[2J\x1b[H')
}
```

- [ ] **Step 7: 编写测试**

```typescript
// tests/components.test.ts

import { Box } from '../src/components/box.js'
import { Text } from '../src/components/text.js'
import { Border } from '../src/components/border.js'
import { Input } from '../src/components/input.js'
import { FrameBuffer } from '../src/render/buffer.js'
import { describe, it } from 'node:test'
import assert from 'node:assert'

describe('Components', () => {
  it('Text renders content', () => {
    const buf = new FrameBuffer()
    buf.resize(20, 5)
    
    const text = new Text({ content: 'Hello' })
    text.render(buf, 0, 0)
    
    buf.swap()
    assert.strictEqual(buf.get(0, 0), 'H')
    assert.strictEqual(buf.get(0, 4), 'o')
  })
  
  it('Border renders frame', () => {
    const buf = new FrameBuffer()
    buf.resize(10, 5)
    
    const border = new Border({ width: 10, height: 5 })
    border.render(buf, 0, 0)
    
    buf.swap()
    assert.strictEqual(buf.get(0, 0), '┌')
    assert.strictEqual(buf.get(0, 9), '┐')
    assert.strictEqual(buf.get(4, 0), '└')
    assert.strictEqual(buf.get(4, 9), '┘')
  })
  
  it('Input handles editing', () => {
    const input = new Input({ width: 20 })
    
    input.insert('H')
    input.insert('i')
    assert.strictEqual(input.value, 'Hi')
    assert.strictEqual(input.cursor, 2)
    
    input.moveLeft()
    input.insert('e')
    assert.strictEqual(input.value, 'Hei')
    
    input.backspace()
    assert.strictEqual(input.value, 'Hi')
  })
})
```

- [ ] **Step 8: 运行测试**

Run: `npm run build && node --test dist/tests/components.test.js`
Expected: 3 tests passed

- [ ] **Step 9: Commit**

```bash
git add src/components/ src/ui/render.ts tests/components.test.ts
git commit -m "feat(components): implement Box, Text, Border, Input primitives"
```

---

## Task 6: LLM API 封装

**Files:**
- Create: `src/api/llm.ts`
- Test: `tests/llm.test.ts`

- [ ] **Step 1: 实现 LLM 客户端**

```typescript
// src/api/llm.ts

export interface Message {
  role: 'system' | 'user' | 'assistant'
  content: string
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

export class LLMClient {
  private config: LLMConfig
  
  constructor(config: LLMConfig) {
    this.config = config
  }
  
  async *chatStream(messages: ChatMessage[]): AsyncGenerator<string, { toolCalls?: ToolCall[] }, unknown> {
    if (this.config.provider === 'openai') {
      return yield* this.openaiStream(messages)
    } else {
      return yield* this.anthropicStream(messages)
    }
  }
  
  private async *openaiStream(messages: ChatMessage[]): AsyncGenerator<string, { toolCalls?: ToolCall[] }, unknown> {
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
        tools: this.getToolsSchema(),
      }),
    })
    
    if (!response.ok) {
      throw new Error(`API error: ${response.status}`)
    }
    
    const reader = response.body!.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let toolCalls: ToolCall[] = []
    
    try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        
        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''
        
        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          const data = line.slice(6)
          if (data === '[DONE]') return { toolCalls }
          
          try {
            const chunk = JSON.parse(data)
            const delta = chunk.choices[0]?.delta
            
            if (delta?.content) yield delta.content
            
            if (delta?.tool_calls) {
              for (const tc of delta.tool_calls) {
                const existing = toolCalls[tc.index]
                if (existing) {
                  existing.function.arguments += tc.function.arguments
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
          } catch { /* ignore */ }
        }
      }
    } finally {
      reader.releaseLock()
    }
    
    return { toolCalls }
  }
  
  private async *anthropicStream(messages: ChatMessage[]): AsyncGenerator<string, { toolCalls?: ToolCall[] }, unknown> {
    const apiMessages = messages
      .filter(m => m.role !== 'system')
      .map(m => ({
        role: m.role === 'tool' ? 'user' : m.role,
        content: m.content,
      }))
    
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
      }),
    })
    
    if (!response.ok) {
      throw new Error(`API error: ${response.status}`)
    }
    
    const reader = response.body!.getReader()
    const decoder = new TextDecoder()
    
    try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        
        const text = decoder.decode(value)
        const lines = text.split('\n')
        
        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          const data = line.slice(6)
          
          try {
            const chunk = JSON.parse(data)
            if (chunk.type === 'content_block_delta') {
              yield chunk.delta.text || ''
            }
          } catch { /* ignore */ }
        }
      }
    } finally {
      reader.releaseLock()
    }
    
    return { toolCalls: [] }
  }
  
  private getToolsSchema() {
    return [
      {
        type: 'function',
        function: {
          name: 'read_file',
          description: '读取文件内容',
          parameters: {
            type: 'object',
            properties: {
              path: { type: 'string', description: '文件路径' },
            },
            required: ['path'],
          },
        },
      },
      {
        type: 'function',
        function: {
          name: 'write_file',
          description: '写入文件内容',
          parameters: {
            type: 'object',
            properties: {
              path: { type: 'string', description: '文件路径' },
              content: { type: 'string', description: '文件内容' },
            },
            required: ['path', 'content'],
          },
        },
      },
      {
        type: 'function',
        function: {
          name: 'bash',
          description: '执行终端命令',
          parameters: {
            type: 'object',
            properties: {
              command: { type: 'string', description: '命令' },
              cwd: { type: 'string', description: '工作目录' },
            },
            required: ['command'],
          },
        },
      },
    ]
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add src/api/llm.ts
git commit -m "feat(api): implement OpenAI/Anthropic streaming client with tool support"
```

---

## Task 7: 工具系统

**Files:**
- Create: `src/tools/base.ts`
- Create: `src/tools/file.ts`
- Create: `src/tools/bash.ts`
- Test: `tests/tools.test.ts`

- [ ] **Step 1: 定义工具接口**

```typescript
// src/tools/base.ts

export interface Tool {
  name: string
  description: string
  execute(args: Record<string, unknown>): Promise<string>
}
```

- [ ] **Step 2: 实现文件工具**

```typescript
// src/tools/file.ts

import { readFileSync, writeFileSync, existsSync } from 'fs'
import { Tool } from './base.js'

export class ReadFileTool implements Tool {
  name = 'read_file'
  description = '读取文件内容'
  
  async execute(args: Record<string, unknown>): Promise<string> {
    const path = args.path as string
    if (!existsSync(path)) {
      return `错误: 文件不存在 ${path}`
    }
    try {
      return readFileSync(path, 'utf-8')
    } catch (err: any) {
      return `错误: ${err.message}`
    }
  }
}

export class WriteFileTool implements Tool {
  name = 'write_file'
  description = '写入文件内容'
  
  async execute(args: Record<string, unknown>): Promise<string> {
    const path = args.path as string
    const content = args.content as string
    try {
      writeFileSync(path, content, 'utf-8')
      return `文件已写入: ${path}`
    } catch (err: any) {
      return `错误: ${err.message}`
    }
  }
}
```

- [ ] **Step 3: 实现 Bash 工具**

```typescript
// src/tools/bash.ts

import { execSync } from 'child_process'
import { Tool } from './base.js'

export class BashTool implements Tool {
  name = 'bash'
  description = '执行终端命令'
  
  async execute(args: Record<string, unknown>): Promise<string> {
    const command = args.command as string
    const cwd = args.cwd as string | undefined
    
    try {
      const result = execSync(command, {
        cwd,
        encoding: 'utf-8',
        timeout: 30000,
        maxBuffer: 1024 * 1024,
      })
      return result || '(命令执行成功，无输出)'
    } catch (err: any) {
      return `错误: ${err.message}\n${err.stderr || ''}`
    }
  }
}
```

- [ ] **Step 4: Commit**

```bash
git add src/tools/
git commit -m "feat(tools): implement file read/write and bash execution tools"
```

---

## Task 8: 会话存储系统

**Files:**
- Create: `src/session/store.ts`
- Test: `tests/session.test.ts`

- [ ] **Step 1: 实现会话存储**

```typescript
// src/session/store.ts

import { writeFileSync, readFileSync, existsSync, mkdirSync, readdirSync, unlinkSync } from 'fs'
import { join } from 'path'
import { ChatMessage } from '../api/llm.js'

export interface Session {
  id: string
  title: string
  tags: string[]
  messages: ChatMessage[]
  createdAt: number
  updatedAt: number
  model: string
  messageCount: number
}

const SESSIONS_DIR = './sessions'

export class SessionStore {
  constructor() {
    if (!existsSync(SESSIONS_DIR)) {
      mkdirSync(SESSIONS_DIR, { recursive: true })
    }
  }
  
  save(session: Session): void {
    const path = join(SESSIONS_DIR, `${session.id}.json`)
    session.updatedAt = Date.now()
    session.messageCount = session.messages.filter(m => m.role === 'user').length
    writeFileSync(path, JSON.stringify(session, null, 2))
  }
  
  load(id: string): Session | null {
    const path = join(SESSIONS_DIR, `${id}.json`)
    if (!existsSync(path)) return null
    try {
      return JSON.parse(readFileSync(path, 'utf-8'))
    } catch {
      return null
    }
  }
  
  list(search?: string, tags?: string[]): Session[] {
    const files = readdirSync(SESSIONS_DIR).filter(f => f.endsWith('.json'))
    
    let sessions = files
      .map(f => this.load(f.replace('.json', ''))!)
      .filter(Boolean)
      .sort((a, b) => b.updatedAt - a.updatedAt)
    
    if (search) {
      const lower = search.toLowerCase()
      sessions = sessions.filter(s => 
        s.title.toLowerCase().includes(lower) ||
        s.messages.some(m => m.content.toLowerCase().includes(lower))
      )
    }
    
    if (tags?.length) {
      sessions = sessions.filter(s => tags.some(t => s.tags.includes(t)))
    }
    
    return sessions
  }
  
  delete(id: string): void {
    const path = join(SESSIONS_DIR, `${id}.json`)
    if (existsSync(path)) unlinkSync(path)
  }
  
  export(sessionId: string, format: 'md' | 'json'): string {
    const session = this.load(sessionId)
    if (!session) return ''
    
    if (format === 'json') return JSON.stringify(session, null, 2)
    
    let md = `# ${session.title}\n\n`
    md += `- ID: ${session.id}\n`
    md += `- 模型: ${session.model}\n`
    md += `- 时间: ${new Date(session.createdAt).toLocaleString()}\n\n---\n\n`
    
    for (const msg of session.messages) {
      if (msg.role === 'system') continue
      md += `## ${msg.role === 'user' ? '用户' : 'AI'}\n\n${msg.content}\n\n---\n\n`
    }
    
    return md
  }
  
  generateId(): string {
    return `sess_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`
  }
  
  autoTitle(messages: ChatMessage[]): string {
    const firstUser = messages.find(m => m.role === 'user')
    if (!firstUser) return '新会话'
    const text = firstUser.content.slice(0, 30)
    return text + (firstUser.content.length > 30 ? '...' : '')
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add src/session/
git commit -m "feat(session): implement JSON-based session storage with search and export"
```

---

## Task 9: Agent 模式系统

**Files:**
- Create: `src/agent/modes.ts`

- [ ] **Step 1: 实现模式管理**

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
    tools: ['read_file'],
    autoExecute: false,
  },
  
  plan: {
    name: 'Plan',
    description: '规划模式 - 分析需求并制定计划',
    systemPrompt: `你是一个架构师。你的任务是分析需求并制定详细的实施计划。
你可以读取文件了解项目结构，但不要修改文件。
输出格式：
1. 目标分析
2. 实施步骤（编号列表）
3. 涉及文件
4. 风险评估`,
    tools: ['read_file', 'bash'],
    autoExecute: false,
  },
  
  build: {
    name: 'Build',
    description: '构建模式 - 执行代码修改和命令',
    systemPrompt: `你是一个全栈工程师。你可以读取文件、修改文件、执行命令来完成任务。
执行工具前，先说明你要做什么。
修改文件后，简要说明变更内容。`,
    tools: ['read_file', 'write_file', 'bash'],
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

- [ ] **Step 2: Commit**

```bash
git add src/agent/
git commit -m "feat(agent): implement Ask/Plan/Build mode system"
```

---

## Task 10: 高级 UI 组件

**Files:**
- Create: `src/components/list.ts`
- Create: `src/components/select.ts`
- Create: `src/components/multi-select.ts`
- Create: `src/components/table.ts`
- Create: `src/components/progress-bar.ts`
- Create: `src/components/spinner.ts`
- Create: `src/components/confirm.ts`
- Create: `src/ui/stream-renderer.ts`
- Create: `src/ui/tool-renderer.ts`
- Create: `src/ui/upload.ts`

- [ ] **Step 1-9: 实现所有高级组件**

（代码已在之前回复中提供，此处省略重复）

- [ ] **Step 10: Commit**

```bash
git add src/components/ src/ui/
git commit -m "feat(ui): implement advanced components and renderers"
```

---

## Task 11: 主应用入口

**Files:**
- Create: `src/index.ts`

- [ ] **Step 1: 实现主应用**

（代码已在之前回复中提供，此处省略重复）

- [ ] **Step 2: Commit**

```bash
git add src/index.ts
git commit -m "feat(app): implement main CLI application with all features"
```

---

## Task 12: 文档与发布

**Files:**
- Create: `README.md`

- [ ] **Step 1: 编写 README**

```markdown
# Codecast Agent CLI

AI Agent 命令行终端工具，支持多 Agent 模式、工具调用、流式输出。

## 安装

```bash
npm install -g codecast-agent
```

## 配置

```bash
cp .env.example .env
# 编辑 .env 填入 API 密钥
```

## 使用

```bash
# 启动
npm run dev

# 命令
/help          显示帮助
/mode          切换模式 (Ask/Plan/Build)
/save [title]  保存会话
/list          列出会话
/clear         清空会话
/exit          退出
```

## 模式

- **Ask**: 问答模式，仅回答问题
- **Plan**: 规划模式，分析并制定计划
- **Build**: 构建模式，执行代码修改

## 工具

- read_file: 读取文件
- write_file: 写入文件
- bash: 执行命令
```

- [ ] **Step 2: Final Commit**

```bash
git add README.md
git commit -m "docs: add README with usage instructions"
```

---

## 总结

| 阶段 | 任务数 | 预估时间 |
|---|---|---|
| 基础架构 | 4 | 3-4 天 |
| 核心功能 | 4 | 5-7 天 |
| 高级功能 | 3 | 4-5 天 |
| 测试与文档 | 1 | 1-2 天 |
| **总计** | **12** | **13-18 天** |

**关键风险点：**
1. Windows 终端 ANSI 支持不完整
2. 不同终端的键盘序列差异
3. SSE 流式解析的稳定性
4. 工具调用的安全性（需沙箱）

**下一步：**
- 选择 Task 1 开始实施
- 或调整优先级（如先实现核心对话功能，再添加 TUI）
