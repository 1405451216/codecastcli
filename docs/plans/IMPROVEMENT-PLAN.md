# CodecastCLI 改进实施计划

> 目标：从"能用"到"世界级" AI 编程 CLI 工具
> 对标：Claude Code CLI / Codex CLI / Qoder CLI / Trae CLI / OpenCode CLI

---

## 目录

- [Phase 1：补齐基础（及格线）](#phase-1补齐基础)
- [Phase 2：核心升级（竞争力）](#phase-2核心升级)
- [Phase 3：生态建设（规模化）](#phase-3生态建设)
- [Phase 4：差异化突破（超越）](#phase-4差异化突破)
- [Phase 5：打磨与上线（生产就绪）](#phase-5打磨与上线)
- [附录：模块负责人与验收标准](#附录)

---

## Phase 1：补齐基础

> 时间：1-2 个月 | 目标：消除导致 Agent 频繁失败的根本缺陷

### Task 1.1 — 补全核心工具集

**问题**：`internal/tools/` 仅有 edit/glob/grep 三个工具，Agent 编码能力被严重限制。

**交付物**：

| 新工具 | 文件 | 功能要点 |
|--------|------|---------|
| `list_files` | `internal/tools/listdir.go` | 列出目录内容，支持深度限制、文件类型过滤、排序 |
| `delete_file` | `internal/tools/delete.go` | 安全删除文件，自动备份到 undo 管理器 |
| `multi_edit` | `internal/tools/multi_edit.go` | 一次修改多个文件，原子提交（全部成功或全部回滚） |
| `read_file` 增强 | `internal/tools/read.go`（新建，覆盖框架默认） | 支持行号范围 `start_line`/`end_line`、大文件自动截断提示、编码检测 |

**实施细节**：

```
list_files:
  参数: path, max_depth(int), pattern(glob), sort(name|size|modified)
  输出: 树形结构，标注文件大小和修改时间
  对标: Claude Code 的 ListFiles，但增加排序和深度控制

delete_file:
  参数: file_path
  逻辑: 1) undoMgr.Backup(path) 2) os.Remove(path) 3) 返回 diff 摘要
  Hook: 复用现有 buildUndoHook + buildCheckpointHook

multi_edit:
  参数: edits[] = [{file_path, old_string, new_string}]
  逻辑:
    1) 预检：所有 old_string 必须唯一匹配
    2) 预检失败：返回第一个失败的 edit 索引和原因
    3) 全部通过：原子写入（临时文件 → rename）
    4) 任一 rename 失败：回滚已写入的文件
  对标: Claude Code 的 MultiEdit

read_file 增强:
  参数: file_path, start_line(int), end_line(int), encoding(string)
  新增:
    - 行号范围读取（大文件 >500 行时提示 LLM 用范围读取）
    - 输出带行号（方便 edit_file 定位）
    - 文件编码自动检测（用 go-charset）
```

**验收标准**：
- [ ] 每个工具配套 `_test.go`，覆盖 happy path + 3 个 edge case
- [ ] `multi_edit` 测试：5 个文件同时修改、1 个失败全部回滚
- [ ] `read_file` 测试：10MB 文件分段读取、二进制文件检测

---

### Task 1.2 — grep 性能升级 + .gitignore 感知

**问题**：`internal/tools/grep.go` 使用 `filepath.WalkDir` + Go `regexp`，大仓库搜索慢 10-100x。

**方案 A（推荐）：调用 ripgrep 二进制**

```go
// internal/tools/grep.go 改造
func (t *GrepSearchTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
    // 1. 检测 rg 是否可用
    rgPath, err := exec.LookPath("rg")
    if err == nil {
        return t.executeWithRipgrep(ctx, rgPath, params)
    }
    // 2. 回退到 Go 原生实现
    return t.executeNative(ctx, params)
}

func (t *GrepSearchTool) executeWithRipgrep(ctx context.Context, rgPath string, params grepSearchParams) (*ap.ToolResult, error) {
    args := []string{
        "--json",                    // JSON 输出，方便解析
        "--max-count", "5",         // 每文件最多 5 个匹配
        "--max-filesize", "1M",     // 跳过超大文件
        "--smart-case",             // 智能大小写
    }
    if params.FilePattern != "" {
        args = append(args, "--glob", params.FilePattern)
    }
    args = append(args, params.Pattern, params.Path)
    
    cmd := exec.CommandContext(ctx, rgPath, args...)
    // ... 解析 JSON 输出
}
```

**方案 B（备选）：添加 .gitignore 感知**

```go
// 使用 go-gitignore 库
import "github.com/sabhiram/go-gitignore"

// 在 WalkDir 中检查
ig := ignore.CompileIgnoreFile(gitignorePath)
if ig.MatchesPath(relPath) {
    return filepath.SkipDir
}
```

**验收标准**：
- [ ] 10K 文件仓库搜索 "func Test" 耗时 < 2s（当前约 8-15s）
- [ ] `.gitignore` 中的文件不出现在结果中
- [ ] 回退到 Go 原生实现时行为与之前一致

---

### Task 1.3 — 修复 JSON 解析安全漏洞

**问题**：`internal/agent/agent.go` 的 `extractJSONField`（第 370-416 行）是手写 JSON 解析器，无法处理 `\uXXXX` 转义、嵌套对象、数组值，且可能被精心构造的输入绕过权限检查。

**改造**：

```go
// internal/agent/agent.go — 删除 extractJSONField，改用标准库

// buildDiffPreviewHook 改造
func buildDiffPreviewHook(prev *diff.Previewer) ap.HookFunc {
    return func(ctx context.Context, hctx *ap.HookContext) error {
        if hctx.ToolCall == nil || prev == nil {
            return nil
        }
        toolName := hctx.ToolCall.Name
        if toolName != "edit_file" && toolName != "write_file" {
            return nil
        }

        var args map[string]json.RawMessage
        if err := json.Unmarshal([]byte(hctx.ToolCall.Args), &args); err != nil {
            return nil // 解析失败不阻塞
        }

        filePath := jsonGetString(args, "file_path")
        if filePath == "" {
            filePath = jsonGetString(args, "path")
        }
        if filePath == "" {
            return nil
        }

        if toolName == "edit_file" {
            oldStr := jsonGetString(args, "old_string")
            newStr := jsonGetString(args, "new_string")
            if oldStr != "" {
                change := prev.PreviewEdit(filePath, oldStr, newStr)
                fmt.Println(tui.Styles.Warning.Render("即将修改文件: " + filePath))
                fmt.Println(tui.NewRenderer().RenderDiff(change.Diff))
            }
        } else {
            content := jsonGetString(args, "content")
            _, err := os.Stat(filePath)
            change := prev.PreviewWrite(filePath, content, err == nil)
            fmt.Println(tui.NewRenderer().RenderDiff(change.Diff))
        }
        return nil
    }
}

// jsonGetString 从 map[string]json.RawMessage 安全提取字符串值
func jsonGetString(m map[string]json.RawMessage, key string) string {
    raw, ok := m[key]
    if !ok {
        return ""
    }
    var s string
    if err := json.Unmarshal(raw, &s); err != nil {
        return ""
    }
    return s
}
```

**同步修改**：`buildUndoHook`、`buildCheckpointHook` 中所有 `extractJSONField` 调用统一替换。

**验收标准**：
- [ ] 删除 `extractJSONField` 和 `unescapeJSONString` 函数
- [ ] 所有 Hook 使用 `json.Unmarshal` 解析参数
- [ ] 添加安全测试：`\u0022` 转义路径、嵌套 JSON 值、空 args

---

### Task 1.4 — 摘要式上下文压缩

**问题**：`internal/agent/stream.go` 第 136 行 `a.ClearContext()` 直接清空所有历史。

**改造**：

```go
// internal/context/compressor.go — 新文件

package context

import (
    "context"
    "fmt"
    ap "agentprimordia/pkg"
)

// Compressor 智能上下文压缩器
type Compressor struct {
    summaryModel  string // 用便宜模型做摘要
    preserveRecent int   // 保留最近 N 轮对话
}

// Compress 对历史消息做摘要压缩，保留最近 N 轮
func (c *Compressor) Compress(ctx context.Context, messages []ap.Message, llm ap.LLMProvider) ([]ap.Message, error) {
    if len(messages) <= c.preserveRecent*2 {
        return messages, nil // 消息太少，不压缩
    }

    // 1. 分离：旧消息 vs 最近消息
    splitIdx := len(messages) - c.preserveRecent*2
    oldMessages := messages[:splitIdx]
    recentMessages := messages[splitIdx:]

    // 2. 提取高价值信息（文件修改、错误、测试结果）
    highlights := c.extractHighlights(oldMessages)

    // 3. 用 LLM 生成摘要
    summaryPrompt := fmt.Sprintf(`请用 200 字以内总结以下对话的关键信息：
- 用户要求了什么
- 做了哪些文件修改（列出文件名）
- 遇到了哪些错误及如何解决的
- 当前任务进度

对话内容：
%s`, c.formatMessages(oldMessages))

    summary, err := c.callSummaryLLM(ctx, llm, summaryPrompt)
    if err != nil {
        // 降级：保留更多原始消息
        return messages[len(messages)/2:], nil
    }

    // 4. 组装压缩后的消息
    compressed := []ap.Message{
        ap.SystemMessage(fmt.Sprintf("[上下文摘要]\n%s\n\n[关键信息]\n%s", summary, highlights)),
    }
    compressed = append(compressed, recentMessages...)
    return compressed, nil
}

// extractHighlights 提取高价值信息
func (c *Compressor) extractHighlights(messages []ap.Message) string {
    var highlights []string
    for _, msg := range messages {
        // 文件修改记录
        if strings.Contains(msg.Content, "edit_file") || strings.Contains(msg.Content, "write_file") {
            highlights = append(highlights, msg.Content[:min(200, len(msg.Content))])
        }
        // 错误信息
        if strings.Contains(msg.Content, "error") || strings.Contains(msg.Content, "failed") {
            highlights = append(highlights, msg.Content[:min(200, len(msg.Content))])
        }
    }
    return strings.Join(highlights, "\n---\n")
}
```

**验收标准**：
- [ ] 压缩后上下文 < 原始 50%
- [ ] 文件修改记录 100% 保留
- [ ] 压缩使用 Haiku/GPT-4o-mini，单次成本 < $0.001

---

### Task 1.5 — API 重试与 Provider 降级

**问题**：`internal/agent/agent.go` 的 `Process`/`StreamProcess` 没有任何重试。

**改造**：

```go
// internal/agent/retry.go — 新文件

package agent

import (
    "context"
    "fmt"
    "math"
    "math/rand"
    "strings"
    "time"
)

// RetryConfig 重试配置
type RetryConfig struct {
    MaxRetries     int           // 最大重试次数
    BaseDelay      time.Duration // 基础延迟
    MaxDelay       time.Duration // 最大延迟
    RetryableCodes []string      // 可重试的错误码
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() RetryConfig {
    return RetryConfig{
        MaxRetries: 3,
        BaseDelay:  1 * time.Second,
        MaxDelay:   30 * time.Second,
        RetryableCodes: []string{
            "429", "rate_limit", "overloaded",
            "500", "502", "503", "504",
            "timeout", "connection_reset", "connection_refused",
        },
    }
}

// WithRetry 带重试的执行包装器
func WithRetry(ctx context.Context, cfg RetryConfig, fn func() error) error {
    var lastErr error
    for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
        lastErr = fn()
        if lastErr == nil {
            return nil
        }
        if !isRetryable(lastErr, cfg.RetryableCodes) {
            return lastErr // 不可重试错误直接返回
        }
        if attempt < cfg.MaxRetries {
            delay := exponentialBackoff(cfg.BaseDelay, cfg.MaxDelay, attempt)
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay):
            }
            fmt.Fprintf(os.Stderr, "⚠ 请求失败 (%v)，%v 后重试 (%d/%d)...\n",
                lastErr, delay, attempt+1, cfg.MaxRetries)
        }
    }
    return fmt.Errorf("重试 %d 次后仍然失败: %w", cfg.MaxRetries, lastErr)
}

func exponentialBackoff(base, max time.Duration, attempt int) time.Duration {
    delay := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
    if delay > max {
        delay = max
    }
    // 添加抖动
    jitter := time.Duration(rand.Int63n(int64(delay) / 4))
    return delay + jitter
}

func isRetryable(err error, codes []string) bool {
    msg := strings.ToLower(err.Error())
    for _, code := range codes {
        if strings.Contains(msg, code) {
            return true
        }
    }
    return false
}
```

**集成到 StreamProcess**：

```go
// stream.go 改造
func (a *CodecastAgent) StreamProcess(ctx context.Context, userInput string) error {
    // ... 现有预算检查 ...
    
    msg := ap.UserMessage(userInput)
    a.selectVariantForInput(userInput, false)
    
    retryCfg := DefaultRetryConfig()
    return WithRetry(ctx, retryCfg, func() error {
        streamCh, err := a.agent.StreamRun(ctx, msg)
        if err != nil {
            return err
        }
        // ... 现有流式处理逻辑 ...
        return nil
    })
}
```

**验收标准**：
- [ ] 429 错误：指数退避重试 3 次
- [ ] 500 错误：最多重试 3 次
- [ ] 401/403：不重试，直接报错
- [ ] 用户 Ctrl+C 立即中断重试循环

---

### Task 1.6 — Ctrl+C 中断当前请求而非退出

**问题**：`context.Background()` 没有取消机制，Ctrl+C 直接退出进程。

**改造**：

```go
// cmd/interactive.go 改造

func runInteractive() error {
    // ... 现有初始化 ...
    
    // 创建可取消的根 context
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    // 捕获 SIGINT
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt)
    go func() {
        for range sigCh {
            if codecastAgent != nil && codecastAgent.IsProcessing() {
                // 正在处理请求 → 取消当前请求
                cancel()
                ctx, cancel = context.WithCancel(context.Background())
                fmt.Println("\n⚠ 已中断当前请求")
            } else {
                // 空闲状态 → 退出
                fmt.Println("\n再见！")
                os.Exit(0)
            }
        }
    }()
    
    // ... 现有 REPL 逻辑，使用 ctx 而非 context.Background() ...
}
```

**验收标准**：
- [ ] 处理中按 Ctrl+C：中断当前请求，返回 REPL
- [ ] 空闲时按 Ctrl+C：正常退出
- [ ] 中断后 Agent 状态正常（可继续对话）

---

## Phase 2：核心升级

> 时间：2-3 个月 | 目标：在核心编码能力上与 Claude Code 正面比较

### Task 2.1 — Tree-sitter 集成 + Repo Map

**问题**：`internal/indexer/indexer.go` 用正则提取 import/export，无法生成函数/类签名摘要。

**方案**：

```go
// internal/indexer/treesitter.go — 新文件

package indexer

import (
    sit "github.com/smacker/go-tree-sitter"
    "github.com/smacker/go-tree-sitter/golang"
    "github.com/smacker/go-tree-sitter/python"
    "github.com/smacker/go-tree-sitter/typescript/tsx"
    "github.com/smacker/go-tree-sitter/typescript/typescript"
    "github.com/smacker/go-tree-sitter/javascript"
)

// Tag 代码标签（函数签名、类定义、接口等）
type Tag struct {
    Name       string `json:"name"`
    Kind       string `json:"kind"`       // function, class, interface, method, variable
    Line       int    `json:"line"`
    Signature  string `json:"signature"`  // 完整签名
    Receiver   string `json:"receiver"`   // Go: receiver type; OOP: class name
}

// RepoMap 生成代码库结构摘要（目标 < 4KB）
func (idx *Indexer) RepoMap(maxTags int) string {
    var sb strings.Builder
    for dir, tags := range idx.tagsByDir {
        sb.WriteString(fmt.Sprintf("\n%s:\n", dir))
        for _, tag := range tags {
            if tag.Kind == "function" || tag.Kind == "class" || tag.Kind == "interface" {
                sb.WriteString(fmt.Sprintf("  %s (L%d)\n", tag.Signature, tag.Line))
            }
        }
    }
    return truncateToTokens(sb.String(), maxTags*20) // 粗略 token 估算
}

// extractTags 使用 tree-sitter 提取代码标签
func extractTags(path string, content []byte) []Tag {
    ext := filepath.Ext(path)
    var parser *sit.Parser
    switch ext {
    case ".go":
        parser = sit.NewParser(golang.GetLanguage())
    case ".py":
        parser = sit.NewParser(python.GetLanguage())
    case ".ts":
        parser = sit.NewParser(typescript.GetLanguage())
    case ".tsx":
        parser = sit.NewParser(tsx.GetLanguage())
    case ".js", ".jsx":
        parser = sit.NewParser(javascript.GetLanguage())
    default:
        return nil
    }
    
    tree := parser.Parse(content)
    return walkTree(tree.RootNode(), content)
}
```

**Query 示例（Go）**：

```
; functions
(function_declaration name: (identifier) @name) @func
; methods
(method_declaration receiver: (parameter_list) @recv name: (field_identifier) @name) @method
; types
(type_declaration (type_spec name: (type_identifier) @name)) @type
; interfaces
(type_declaration (type_spec type: (interface_type) @iface name: (type_identifier) @name))
```

**验收标准**：
- [ ] 支持 Go/Python/TypeScript/JavaScript 四种语言
- [ ] Repo Map 输出 < 4KB，包含所有公开函数签名
- [ ] 10K 文件仓库索引耗时 < 5s

---

### Task 2.2 — LSP 工具集成

**新增工具**：

```go
// internal/tools/lsp.go — 新文件

// LSPTool 通过 LSP 协议查询代码信息
type LSPTool struct {
    clients map[string]*lsp.Client // language -> client
}

// 子命令：
// goto_definition: 跳转到定义
// find_references: 查找所有引用
// hover: 获取符号类型信息
// diagnostics: 获取编译错误/警告

func (t *LSPTool) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": ["goto_definition", "find_references", "hover", "diagnostics"],
                "description": "LSP 操作类型"
            },
            "file_path": { "type": "string" },
            "line": { "type": "integer", "description": "行号（1-based）" },
            "character": { "type": "integer", "description": "列号（1-based）" }
        },
        "required": ["action", "file_path"]
    }`)
}
```

**LSP 客户端管理**：

```go
// internal/lsp/manager.go — 新文件

package lsp

import (
    "context"
    "github.com/sourcegraph/jsonrpc2"
)

// Manager 管理多个 LSP 服务器实例
type Manager struct {
    servers map[string]*Server // language -> server
}

// Server 封装一个 LSP 服务器进程
type Server struct {
    cmd     *exec.Cmd
    conn    *jsonrpc2.Conn
    lang    string
    rootURI string
}

// NewManager 根据项目语言自动启动对应 LSP
// Go → gopls
// Python → pyright / pylsp
// TypeScript → typescript-language-server
func NewManager(rootDir string) (*Manager, error) { ... }
```

**验收标准**：
- [ ] Go 项目自动启动 gopls
- [ ] goto_definition 正确率 > 95%
- [ ] LSP 崩溃自动重启

---

### Task 2.3 — 增量索引 + 索引缓存

**改造 `internal/indexer/indexer.go`**：

```go
// 新增：持久化索引到磁盘
type CachedIndex struct {
    Version   string    `json:"version"`    // 索引版本号
    IndexedAt time.Time `json:"indexed_at"`
    Files     map[string]*CachedFile `json:"files"`
}

type CachedFile struct {
    Path    string    `json:"path"`
    ModTime time.Time `json:"mod_time"`
    Size    int64     `json:"size"`
    Tags    []Tag     `json:"tags"`
    Hash    string    `json:"hash"` // 内容 MD5
}

// BuildOrLoad 启动时加载缓存，后台增量更新
func (idx *Indexer) BuildOrLoad() error {
    cachePath := filepath.Join(idx.rootDir, ".codecast", "index.json")
    
    // 1. 尝试加载缓存
    cached, err := loadCache(cachePath)
    if err == nil && cached.Version == IndexVersion {
        idx.index = cached.toIndex()
        // 2. 后台增量更新
        go idx.incrementalUpdate(cached)
        return nil
    }
    
    // 3. 缓存不存在/版本不匹配 → 全量构建
    return idx.Build()
}

// incrementalUpdate 只更新变更的文件
func (idx *Indexer) incrementalUpdate(cached *CachedIndex) {
    // 用 fsnotify 监听变更
    watcher, _ := fsnotify.NewWatcher()
    defer watcher.Close()
    // ... 只重建变更文件的 tags
}
```

**验收标准**：
- [ ] 第二次启动加载缓存 < 500ms
- [ ] 文件修改后 3s 内索引自动更新
- [ ] 缓存文件大小 < 1MB（10K 文件项目）

---

### Task 2.4 — TUI 迁移到 Bubble Tea

**改造范围**：

| 现有 | 改为 | 文件 |
|------|------|------|
| go-prompt REPL | Bubble Tea `textinput` + `viewport` | `internal/tui/app.go`（新） |
| glamour 纯渲染 | Bubble Tea `viewport` 内嵌 glamour | `internal/tui/render.go`（改造） |
| 手动 spinner | Bubble Tea `spinner` model | `internal/tui/spinner.go`（改造） |
| fatih/color 散落调用 | lipgloss styles 统一管理 | `internal/tui/styles.go`（新） |

**架构设计**：

```
┌─────────────────────────────────────────────┐
│  Status Bar: model=gpt-4o | tokens=12.3K    │
│            | budget=$2.50 | mode=auto-edit  │
├─────────────────────────────────────────────┤
│                                             │
│  Chat Viewport (scrollable)                 │
│  - User messages (right-aligned, blue)      │
│  - Assistant messages (left, markdown)      │
│  - Tool calls (collapsible, yellow)         │
│  - Diff previews (inline, red/green)        │
│                                             │
├─────────────────────────────────────────────┤
│  ❯ Type your message...           [Send]   │
│  (multi-line: Shift+Enter, @file: Tab)      │
└─────────────────────────────────────────────┘
```

**关键交互**：
- `Ctrl+C`：中断当前请求
- `Ctrl+L`：清屏
- `Tab`：补全 @文件引用 / 斜杠命令
- `Shift+Enter`：多行输入
- `↑/↓`：历史消息浏览
- `Ctrl+K`：命令面板

**验收标准**：
- [ ] 流式输出无闪烁
- [ ] 代码块语法高亮（至少 Go/Python/TypeScript）
- [ ] 工具调用可折叠/展开
- [ ] 状态栏实时显示模型/Token/预算

---

### Task 2.5 — 子 Agent 并行执行修复

**问题**：`internal/subagent/orchestrator.go` 第 95 行变量丢弃、共享上下文污染。

**改造**：

```go
// ParallelExecute 修复版
func (o *Orchestrator) ParallelExecute(ctx context.Context, tasks []PlanTask) (*ExecutionResult, error) {
    builder := ap.NewDAGBuilder("parallel-execute")
    builder.DelegateNode("plan", o.planAgent)

    for _, task := range tasks {
        nodeID := fmt.Sprintf("exec_%s", task.ID)
        
        // 为每个子任务创建独立的 Agent 实例（独立上下文）
        execAgent, err := o.createIsolatedExecAgent(task.ID)
        if err != nil {
            return nil, fmt.Errorf("创建子 Agent 失败: %w", err)
        }
        
        builder.DelegateNode(nodeID, execAgent)
        
        if len(task.DependsOn) == 0 {
            builder.Edge("plan", nodeID)
        } else {
            for _, dep := range task.DependsOn {
                builder.Edge(fmt.Sprintf("exec_%s", dep), nodeID)
            }
        }
    }

    // 添加聚合节点
    aggregator := ap.NewAgent("Aggregator", aggregatorPrompt, llmProvider)
    builder.DelegateNode("aggregate", aggregator)
    for _, task := range tasks {
        builder.Edge(fmt.Sprintf("exec_%s", task.ID), "aggregate")
    }
    
    dag, err := builder.Build()
    if err != nil {
        return nil, err
    }

    result, err := dag.Run(ctx, fmt.Sprintf("并行执行 %d 个子任务", len(tasks)))
    // ...
}

func (o *Orchestrator) createIsolatedExecAgent(taskID string) (*ap.CapabilityAgent, error) {
    // 独立的 memory（不共享）
    mem, _ := ap.WithInMemory()
    
    llmProvider, err := provider.CreateProvider(o.config)
    if err != nil {
        return nil, err
    }
    
    return ap.NewAgent(
        fmt.Sprintf("ExecAgent_%s", taskID),
        execSystemPrompt,
        llmProvider,
        ap.WithMaxTurns(15),
    ).WithToolkit(o.registry).WithMemory(mem), nil
}
```

**验收标准**：
- [ ] 3 个并行子任务同时执行，上下文互不干扰
- [ ] 1 个失败不影响其他 2 个
- [ ] 聚合节点正确合并 3 个子任务结果

---

### Task 2.6 — 多行输入 + @文件引用 + 状态栏

**多行输入**：在 Bubble Tea 的 `textinput` 中启用多行模式，`Shift+Enter` 换行，`Enter` 提交。

**@文件引用**：

```go
// internal/tui/filecompleter.go — 新文件

// FileCompleter 当用户输入 @ 时触发文件路径补全
type FileCompleter struct {
    rootDir string
    indexer *indexer.Indexer
}

func (fc *FileCompleter) Complete(prefix string) []string {
    // 1. 从 indexer 搜索匹配的文件
    entries := fc.indexer.SearchFiles(prefix)
    // 2. 返回相对路径列表
    var results []string
    for _, e := range entries {
        results = append(results, e.Path)
    }
    return results
}

// 在 Bubble Tea Update 中处理 @ 引用
func (m Model) handleFileReference(input string) string {
    // 匹配 @filename 模式，读取文件内容注入到 user message
    re := regexp.MustCompile(`@(\S+)`)
    return re.ReplaceAllStringFunc(input, func(match string) string {
        path := match[1:] // 去掉 @
        content, err := os.ReadFile(path)
        if err != nil {
            return match
        }
        return fmt.Sprintf("\n```%s\n// File: %s\n%s\n```\n", 
            detectLanguage(path), path, truncate(string(content), 4000))
    })
}
```

**验收标准**：
- [ ] `@src/auth/handler.go` 自动补全文件路径
- [ ] 文件内容自动注入到用户消息中
- [ ] 多行粘贴不触发提交

---

## Phase 3：生态建设

> 时间：3-4 个月 | 目标：建立用户增长飞轮

### Task 3.1 — 多平台分发

```yaml
# .goreleaser.yml
project_name: codecast
builds:
  - id: codecast
    main: .
    binary: codecast
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w -X codecast/cli/internal/version.version={{.Version}}

brews:
  - name: codecast
    repository:
      owner: codecast
      name: homebrew-tap
    description: "AI-powered terminal coding assistant"

scoops:
  - name: codecast
    repository:
      owner: codecast
      name: scoop-bucket

nfpms:
  - package_name: codecast
    formats: [deb, rpm]
    description: "AI-powered terminal coding assistant"
```

**额外渠道**：
- `winget` 提交到 winget-pkgs 仓库
- `npm` 发布 `@codecast/cli`（通过 bin wrapper）
- Docker Hub 发布 `codecast/cli`

**验收标准**：
- [ ] `brew install codecast/tap/codecast` 可用
- [ ] `scoop install codecast` 可用
- [ ] `winget install codecast` 可用
- [ ] GitHub Release 自动生成所有平台二进制

---

### Task 3.2 — VS Code 扩展（基础版）

**架构**：

```
VS Code Extension (TypeScript)
  └── Terminal Panel
       └── 嵌入 codecast CLI 进程
            └── stdin/stdout 桥接到 VS Code UI
```

**核心功能**：
1. 在 VS Code 内嵌终端中启动 Codecast
2. 文件引用：右键文件 → "Send to Codecast"
3. Diff 视图：Codecast 的文件修改在 VS Code Diff View 中展示
4. 状态栏集成：底部状态栏显示 Codecast 状态

**验收标准**：
- [ ] VS Code Marketplace 发布
- [ ] 右键菜单集成
- [ ] 文件修改实时反映在编辑器

---

### Task 3.3 — Mock Provider 测试框架

```go
// internal/testutil/mock_provider.go — 新文件

package testutil

import (
    "context"
    ap "agentprimordia/pkg"
)

// MockProvider 模拟 LLM Provider
type MockProvider struct {
    responses    []ap.Response
    streamChunks []string
    toolCalls    []ap.ToolCall
    errors       []error
    callIndex    int
}

// NewMockProvider 创建 Mock Provider
func NewMockProvider() *MockProvider { return &MockProvider{} }

// WithResponse 预设响应
func (m *MockProvider) WithResponse(content string) *MockProvider {
    m.responses = append(m.responses, ap.Response{Content: content})
    return m
}

// WithToolCall 预设工具调用
func (m *MockProvider) WithToolCall(name string, args string) *MockProvider {
    m.toolCalls = append(m.toolCalls, ap.ToolCall{Name: name, Args: args})
    return m
}

// WithStreamChunks 预设流式输出
func (m *MockProvider) WithStreamChunks(chunks ...string) *MockProvider {
    m.streamChunks = append(m.streamChunks, chunks...)
    return m
}

// WithError 预设错误
func (m *MockProvider) WithError(err error) *MockProvider {
    m.errors = append(m.errors, err)
    return m
}

// Run 实现 LLMProvider 接口
func (m *MockProvider) Run(ctx context.Context, messages []ap.Message) (*ap.Response, error) {
    idx := m.callIndex
    m.callIndex++
    if idx < len(m.errors) && m.errors[idx] != nil {
        return nil, m.errors[idx]
    }
    if idx < len(m.responses) {
        return &m.responses[idx], nil
    }
    return &ap.Response{Content: "mock response"}, nil
}
```

**E2E 测试示例**：

```go
// internal/agent/e2e_test.go

func TestEditFileWorkflow(t *testing.T) {
    provider := testutil.NewMockProvider().
        WithToolCall("read_file", `{"file_path":"main.go"}`).
        WithResponse("main.go 内容是 ...").
        WithToolCall("edit_file", `{"file_path":"main.go","old_string":"func main()","new_string":"func main() {\n\tfmt.Println(\"hello\")"}`).
        WithResponse("已完成修改")
    
    agent, cleanup := testutil.NewTestAgent(t, provider)
    defer cleanup()
    
    err := agent.Process(context.Background(), "在 main.go 中添加 hello world 输出")
    require.NoError(t, err)
    
    // 验证文件被正确修改
    content, _ := os.ReadFile("main.go")
    assert.Contains(t, string(content), `fmt.Println("hello")`)
}
```

**验收标准**：
- [ ] Mock Provider 支持流式、工具调用、错误模拟
- [ ] 至少 10 个 E2E 测试覆盖核心工作流
- [ ] CI 中运行 E2E 测试 < 30s

---

## Phase 4：差异化突破

> 时间：4-6 个月 | 目标：在特定维度做到竞品没有的

### Task 4.1 — 智能模型路由

```go
// internal/routing/model_router.go — 新文件

// ModelRouter 根据任务复杂度自动选择最优模型
type ModelRouter struct {
    config *RoutingConfig
}

type RoutingConfig struct {
    // 简单任务（问答、解释）→ 便宜模型
    SimpleModel  string  `yaml:"simple_model"`   // e.g. gpt-4o-mini
    // 中等任务（单文件编辑）→ 主力模型
    MediumModel  string  `yaml:"medium_model"`   // e.g. gpt-4o
    // 复杂任务（多文件重构、架构设计）→ 最强模型
    ComplexModel string  `yaml:"complex_model"`  // e.g. claude-opus-4
}

// Route 分析用户输入，返回推荐模型
func (r *ModelRouter) Route(input string, indexer *indexer.Indexer) string {
    score := r.complexityScore(input, indexer)
    switch {
    case score < 3:
        return r.config.SimpleModel
    case score < 7:
        return r.config.MediumModel
    default:
        return r.config.ComplexModel
    }
}

func (r *ModelRouter) complexityScore(input string, idx *indexer.Indexer) int {
    score := 0
    // 字符长度
    if len(input) > 200 { score += 2 }
    if len(input) > 500 { score += 1 }
    // 关键词
    complexKeywords := []string{"重构", "架构", "迁移", "refactor", "design", "migrate"}
    for _, kw := range complexKeywords {
        if strings.Contains(strings.ToLower(input), kw) { score += 2 }
    }
    // 涉及文件数
    fileRefs := countFileReferences(input, idx)
    if fileRefs > 3 { score += 2 }
    if fileRefs > 8 { score += 2 }
    return score
}
```

**验收标准**：
- [ ] "解释这段代码" → 自动选 GPT-4o-mini（成本降低 90%）
- [ ] "重构整个认证模块" → 自动选 Claude Opus
- [ ] 路由延迟 < 50ms

---

### Task 4.2 — 多 Agent 协作可视化

在终端中实时渲染 DAG 执行进度：

```
┌─────────────────────────────────────────────┐
│  🔄 多 Agent 协作                           │
│                                             │
│  [✓] Plan Agent    ─ 3 tasks planned        │
│   ├── [✓] Exec_1   ─ auth module updated    │
│   ├── [⟳] Exec_2   ─ writing tests...       │
│   └── [⏳] Exec_3   ─ waiting for Exec_2    │
│                                             │
│  Progress: ████████░░ 67%  Tokens: 8.2K    │
└─────────────────────────────────────────────┘
```

**验收标准**：
- [ ] 实时显示每个子 Agent 状态
- [ ] 进度条准确反映完成比例
- [ ] 支持折叠/展开每个子 Agent 的详细输出

---

### Task 4.3 — Git-aware AI

```go
// internal/git/analyzer.go — 新文件

// Analyzer Git 仓库分析器
type Analyzer struct {
    repoPath string
}

// RecentChanges 分析最近 N 次提交的变更模式
func (a *Analyzer) RecentChanges(n int) (*ChangeReport, error) { ... }

// PRReview 自动审查 PR 的 diff
func (a *Analyzer) PRReview(baseBranch string) (*ReviewResult, error) {
    diff, err := a.getDiff(baseBranch)
    if err != nil {
        return nil, err
    }
    // 分析变更的语义影响
    // 检查是否遗漏了相关文件的修改
    // 检查测试覆盖
    return &ReviewResult{...}, nil
}

// BlameContext 获取某段代码的 git blame 信息，帮助 LLM 理解修改意图
func (a *Analyzer) BlameContext(file string, startLine, endLine int) (*BlameInfo, error) { ... }
```

**新增斜杠命令**：
- `/review [branch]`：审查当前分支相对 base 的所有变更
- `/blame <file> <line>`：获取 git blame 上下文注入对话
- `/history <file>`：查看文件的修改历史摘要

**验收标准**：
- [ ] `/review main` 输出结构化代码审查
- [ ] blame 信息正确注入 LLM 上下文
- [ ] 支持 GitHub/GitLab PR 评论发布

---

## Phase 5：打磨与上线

> 时间：1-2 个月 | 目标：从"功能完整"到"生产就绪"，推向市场

### Task 5.1 — 性能优化与基准测试

**问题**：Phase 1-4 新增了 tree-sitter、LSP、Bubble Tea、智能路由等模块，启动时间和内存占用明显增加。当前没有性能基准，无法量化优化效果。

**启动速度优化**：

```go
// internal/agent/agent.go — 当前问题：newAgent() 同步执行索引、MCP 连接、DB 初始化
// 优化方案：并行化启动流水线

type startupStage struct {
    Name string
    Fn   func() error
    Critical bool // 失败是否阻塞启动
}

func parallelStartup(stages []startupStage) error {
    var wg sync.WaitGroup
    errCh := make(chan error, len(stages))
    for _, s := range stages {
        wg.Add(1)
        go func(stage startupStage) {
            defer wg.Done()
            if err := stage.Fn(); err != nil {
                if stage.Critical {
                    errCh <- fmt.Errorf("%s: %w", stage.Name, err)
                } else {
                    slog.Warn("启动阶段失败，已跳过", "stage", stage.Name, "err", err)
                }
            }
        }(s)
    }
    wg.Wait()
    close(errCh)
    for err := range errCh {
        return err // 返回第一个致命错误
    }
    return nil
}
```

**内存占用优化**：

```go
// internal/indexer/indexer.go — 索引完成后释放中间数据结构
func (idx *Indexer) Build() error {
    // ... 现有构建逻辑 ...
    
    // 构建完成后，释放临时缓冲区
    idx.tempBuffers = nil
    idx.parseCache = nil
    runtime.GC() // 主动回收大对象
    
    return nil
}

// 内存预算：索引器内存上限
func (idx *Indexer) SetMemoryBudget(maxMB int) {
    idx.memoryBudget = maxMB * 1024 * 1024
}
```

**性能基准测试**：

```go
// internal/agent/startup_bench_test.go — 新文件

func BenchmarkStartup(b *testing.B) {
    for i := 0; i < b.N; i++ {
        cfg := &config.Config{
            APIKey: "test-key",
            Model:  "gpt-4o",
            Provider: "openai",
        }
        agent, err := New(cfg)
        if err != nil {
            b.Fatal(err)
        }
        agent.Close()
    }
}

func BenchmarkStreamFirstToken(b *testing.B) {
    // 测量从请求发出到首个 token 输出的延迟
    provider := testutil.NewMockProvider().WithStreamChunks("Hello", " world")
    agent, cleanup := testutil.NewTestAgent(b, provider)
    defer cleanup()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        agent.StreamProcess(context.Background(), "Hello")
    }
}
```

**验收标准**：
- [ ] 冷启动时间 < 1.5s（当前估计 ~3-5s，含索引）
- [ ] 热启动（索引缓存命中）< 500ms
- [ ] 稳态内存占用 < 100MB（10K 文件项目）
- [ ] 二进制体积 < 25MB（当前 ~21MB，Phase 2 新增 tree-sitter 后可能增长）
- [ ] 新增 3+ 个 `Benchmark` 函数，CI 中跑 `go test -bench`

---

### Task 5.2 — 代码质量与重构

**问题**：`cmd/interactive.go` 已达 2250 行（Phase 1 时 1382 行，后续新增斜杠命令继续膨胀），`internal/agent/agent.go` 达 1074 行。大型文件影响可读性和可维护性。

**5.2.1 — 拆分 interactive.go**

```
cmd/
├── interactive.go          # REPL 主循环 (~200行)
├── interactive_executor.go # executor / completer (~150行)
├── interactive_config.go   # handleConfigCommand 及相关函数 (~150行)
├── interactive_session.go  # handleSessionCommand / handleCostCommand (~200行)
├── interactive_git.go      # handleReviewCommand / handleBlameCommand / handleHistoryCommand (~250行)
├── interactive_agent.go    # handleDelegateCommand / handlePlanCommand / handlePoolCommand (~200行)
├── interactive_misc.go     # handleVisionCommand / handleSandboxCommand / 其他零散命令 (~200行)
└── interactive_signal.go   # setupSignalHandler / acquireProcessingCtx / HITL watcher (~100行)
```

**拆分原则**：
- 每个文件只放同一职责的函数
- 全局变量（`codecastAgent`、`fileCompleter`、`processingCancel`）抽到 `interactive.go` 顶部的 `replState` 结构体
- 不改变任何公开 API，纯内部重组

**5.2.2 — 错误消息标准化**

```go
// internal/errors/errors.go — 新文件：统一错误类型

type UserFacingError struct {
    Code    string // 机器可读码，如 "E001_PROVIDER_FAIL"
    Message string // 用户可读描述
    Hints   []string // 修复建议
    Err     error    // 原始错误
}

func (e *UserFacingError) Error() string {
    return e.Message
}

func (e *UserFacingError) Unwrap() error {
    return e.Err
}

// Render 渲染为用户友好的终端输出
func (e *UserFacingError) Render() {
    color.Red("✗ [%s] %s", e.Code, e.Message)
    if e.Err != nil {
        color.HiBlack("  原因: %v", e.Err)
    }
    for _, hint := range e.Hints {
        color.Yellow("  💡 %s", hint)
    }
}

// 预定义错误码
var (
    ErrProviderAuth    = &UserFacingError{Code: "E001", Message: "API 认证失败"}
    ErrRateLimit       = &UserFacingError{Code: "E002", Message: "API 请求频率超限"}
    ErrModelNotFound   = &UserFacingError{Code: "E003", Message: "模型不存在"}
    ErrNetworkTimeout  = &UserFacingError{Code: "E004", Message: "网络连接超时"}
    ErrBudgetExceeded  = &UserFacingError{Code: "E005", Message: "预算已超限"}
    ErrPermissionDenied = &UserFacingError{Code: "E006", Message: "工具调用被拒绝"}
)
```

**验收标准**：
- [ ] `cmd/interactive.go` 拆分后主文件 < 250 行，所有拆分文件均 < 300 行
- [ ] 所有用户可见错误使用 `UserFacingError` 类型，含错误码 + 修复建议
- [ ] 全量测试通过，行为无变化

---

### Task 5.3 — 稳定性与压力测试

**问题**：Phase 1-4 新增了多个并发组件（SIGINT handler、HITL watcher、asyncSummarize、并行子Agent），但缺乏系统性的压力测试。

**5.3.1 — 并发压力测试**

```go
// internal/agent/stress_test.go — 新文件

func TestStressConcurrentStreamProcess(t *testing.T) {
    // 模拟快速连续发送 10 个请求
    provider := testutil.NewMockProvider().WithResponse("ok")
    agent, cleanup := testutil.NewTestAgent(t, provider)
    defer cleanup()
    
    var wg sync.WaitGroup
    errors := make([]error, 10)
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()
            errors[idx] = agent.StreamProcess(ctx, fmt.Sprintf("request %d", idx))
        }(i)
    }
    wg.Wait()
    
    // 验证：不应有 panic、data race 或死锁
    for i, err := range errors {
        t.Logf("request %d: err=%v", i, err)
    }
}

func TestStressInterruptDuringStream(t *testing.T) {
    // 在流式输出中途反复 Ctrl+C
    provider := testutil.NewMockProvider().
        WithStreamChunks("token1", "token2", "token3", "token4", "token5")
    agent, cleanup := testutil.NewTestAgent(t, provider)
    defer cleanup()
    
    for i := 0; i < 20; i++ {
        ctx, cancel := context.WithCancel(context.Background())
        go func() {
            time.Sleep(50 * time.Millisecond) // 中途取消
            cancel()
        }()
        _ = agent.StreamProcess(ctx, "test")
        // 验证 agent 状态正常，可以接受下一个请求
        if agent.IsProcessing() {
            t.Fatal("agent should not be processing after cancel")
        }
    }
}
```

**5.3.2 — 优雅降级矩阵**

| 组件故障 | 期望行为 | 当前状态 |
|---------|---------|----------|
| LSP 服务器崩溃 | 回退到 regex 索引，不阻塞 Agent | 待实现 |
| MCP 服务器超时 | 跳过该工具，警告用户，继续运行 | 部分实现 |
| tree-sitter 解析失败 | 回退到 regex 提取 | 已实现 |
| 索引缓存损坏 | 删除缓存，全量重建 | 待验证 |
| SQLite 锁竞争 | 重试 + 降级到内存模式 | 部分实现 |
| 网络断开 | 重试 3 次后报错，保留本地功能 | 已实现 |
| 预算超限 | 拒绝新请求，但允许 /cost /session 查询 | 已实现 |

**5.3.3 — Race 检测集成到 CI**

```yaml
# .github/workflows/ci.yml — 新增 race job
  race:
    name: Race Detection
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - name: Run tests with race detector
        run: go test ./... -race -count=3 -timeout 5m
```

**验收标准**：
- [ ] 并发压力测试通过（无 race、无 panic、无死锁）
- [ ] 优雅降级矩阵 7 项全部通过
- [ ] CI 中 race detector 稳定绿灯（连续 3 次 count=3）
- [ ] 新增 5+ 个压力测试用例

---

### Task 5.4 — 用户文档体系

**问题**：README 未更新 Phase 2-4 的新特性，缺少使用教程和故障排查指南。

**5.4.1 — README 更新**

新增章节：
- **智能代码理解**：tree-sitter Repo Map、LSP 集成、增量索引
- **现代 TUI**：Bubble Tea 架构、状态栏、@file 引用
- **多平台安装**：brew / scoop / winget / npm / Docker 五种渠道
- **VS Code 集成**：安装扩展、右键菜单、Diff 视图
- **智能模型路由**：自动选择模型、成本优化策略
- **Git-aware AI**：/review、/blame、/history 命令

**5.4.2 — 快速上手教程**

```
docs/tutorials/
├── 01-getting-started.md     # 5 分钟上手：安装→配置→第一次对话
├── 02-editing-code.md        # 编辑代码：edit/multi_edit/delete + undo
├── 03-using-tools.md         # 工具全解：内置工具 + LSP + MCP
├── 04-multi-agent.md         # 双 Agent 协作：/plan + /delegate + DAG 可视化
├── 05-custom-prompts.md      # 自定义提示词：YAML 格式、变量、A/B 测试
└── 06-cost-optimization.md   # 成本优化：预算控制 + 智能路由 + 摘要压缩
```

**5.4.3 — 故障排查指南**

```
docs/troubleshooting.md
├── API 认证失败（E001）：Key 格式、Provider 匹配、Base URL
├── 模型不可用（E003）：模型 ID 拼写、Provider 支持列表
├── 网络超时（E004）：代理设置、Base URL、防火墙
├── 索引失败：大仓库、权限、符号链接
├── TUI 渲染异常：终端兼容性、$TERM 设置、Windows ConPTY
└── 性能问题：禁用增量索引、关闭 LSP、减少 MCP 服务器
```

**验收标准**：
- [ ] README 新增 Phase 2-4 特性章节，覆盖所有新功能
- [ ] 6 篇教程全部完成，每篇含实操示例
- [ ] 故障排查指南覆盖 10+ 常见问题
- [ ] 所有文档链接在 CI 中检查有效性

---

### Task 5.5 — 社区建设与反馈渠道

**5.5.1 — 贡献指南与模板**

```markdown
# CONTRIBUTING.md — 新建

## 开发环境
1. Go 1.26+
2. `git clone` + `go mod tidy`
3. `go test ./...` 确保全过

## 代码规范
- 遵循 Effective Go
- 新函数必须有文档注释
- 公开 API 必须配套 _test.go
- commit message 遵循 Conventional Commits

## PR 流程
1. Fork → feature branch → PR
2. CI 必须全绿（test + lint + race）
3. 至少 1 个 review
4. Squash merge
```

**5.5.2 — Issue / PR 模板**

```yaml
# .github/ISSUE_TEMPLATE/bug_report.md
---
name: Bug Report
about: 报告 Bug
labels: bug
---

**描述**：简明描述问题
**复现步骤**：
1. ...
2. ...
**期望行为**：
**实际行为**：
**环境**：
- Codecast 版本：
- OS：
- 终端：
- Provider/Model：

# .github/ISSUE_TEMPLATE/feature_request.md
---
name: Feature Request
about: 功能建议
labels: enhancement
---

**场景**：描述你遇到的问题
**方案**：你希望的解决方案
**替代**：考虑过的替代方案
```

**5.5.3 — Discord/社区渠道**

- 创建 Discord 服务器（或用 GitHub Discussions）
- README 加社区链接
- 设置 `feedback` 命令：`/feedback <message>` 直接提交反馈到 GitHub Issue

**验收标准**：
- [ ] CONTRIBUTING.md 已创建并完善
- [ ] 2 个 Issue 模板 + 1 个 PR 模板已配置
- [ ] 至少 1 个社区渠道可用（Discord 或 GitHub Discussions）
- [ ] `/feedback` 斜杠命令可提交反馈

---

### Task 5.6 — 真实项目验证

**问题**：Phase 1-4 的功能尚未在真实项目中经过充分验证。

**验证矩阵**：

| 项目类型 | 语言 | 验证目标 | 预期结果 |
|---------|------|---------|----------|
| CodecastCLI 自身 | Go | 用 Codecast 修改 Codecast | 能正确编辑、索引、undo |
| 中型 Web 项目 | TypeScript/React | tree-sitter + LSP + 多文件编辑 | 跨文件重构不丢引用 |
| Python ML 项目 | Python | 单文件编辑 + /review | review 发现常见问题 |
| 开源项目 | 多语言 | /delegate 多 Agent 协作 | DAG 执行正确、隔离无污染 |

**验证流程**：

```
1. 启动 Codecast，进入目标项目目录
2. 执行典型工作流：
   a. "解释这个项目的架构" → 验证 Repo Map + 索引
   b. "在 auth 模块中添加一个 rate limiter" → 验证多文件编辑 + undo
   c. "/review main" → 验证 Git-aware 审查
   d. "/delegate 重构数据库连接池" → 验证双 Agent + DAG 可视化
3. 记录：成功率、响应时间、Token 消耗、错误类型
```

**验收标准**：
- [ ] 4 个验证项目全部完成
- [ ] 工具调用成功率 > 90%
- [ ] 端到端工作流（编辑→测试→提交）无阻塞性 Bug
- [ ] 收集至少 10 个实际使用中的问题并创建 Issue

---

### Task 5.7 — 发布 v0.4.0

**5.7.1 — 版本号与 CHANGELOG**

```markdown
## [0.4.0] - 2026-XX-XX

### 新增
- tree-sitter 代码理解（Go/Python/TS/JS）
- LSP 集成（gopls/pyright/tsserver）
- 增量索引 + 磁盘缓存
- Bubble Tea 现代 TUI
- 隔离子 Agent 并行执行
- @file 引用 + 多行输入 + 状态栏
- goreleaser 多平台分发
- VS Code 扩展
- Mock Provider 测试框架
- 智能模型路由
- DAG 可视化
- Git-aware AI（/review /blame /history）
- 预算感知上下文压缩

### 改进
- API 重试与 Provider 降级
- 摘要式上下文压缩
- Ctrl+C 中断当前请求
- grep 性能升级 + .gitignore 感知
- JSON 解析安全修复
- 核心工具集补全
```

**5.7.2 — Beta 测试计划**

```
Week 1: 内部测试（团队自用 Codecast 开发 Codecast）
Week 2: 邀请 10-20 名外部用户 Beta 测试
Week 3: 收集反馈、修复阻塞性 Bug
Week 4: 正式发布 v0.4.0
```

**5.7.3 — 发布检查清单**

```
[ ] CHANGELOG.md 更新
[ ] README.md 更新（Phase 2-4 特性）
[ ] 版本号更新（internal/version、README badge）
[ ] go mod tidy 无变更
[ ] go test ./... -race 全过
[ ] go vet ./... 无警告
[ ] golangci-lint 无新增 warning
[ ] goreleaser release --snapshot 本地验证
[ ] GitHub Release 创建
[ ] brew/scoop/winget 提交
[ ] VS Code Marketplace 发布
[ ] 发布公告（博客/Twitter/社区）
```

**验收标准**：
- [ ] Beta 测试至少 2 周，修复所有 P0/P1 Bug
- [ ] 发布检查清单 13 项全部完成
- [ ] 5 个安装渠道可用（go install / brew / scoop / winget / GitHub Release）
- [ ] 发布后 1 周内无 P0 Bug 报告

---

### Task 5.8 — 风险识别与应对

| 风险 | 概率 | 影响 | 应对措施 |
|------|------|------|----------|
| tree-sitter CGO 依赖导致跨平台构建失败 | 中 | 高 | 已有 regex 回退方案；CGO_ENABLED=0 时自动降级；CI 矩阵覆盖 5 平台 |
| Bubble Tea 与 go-prompt 共存冲突 | 低 | 中 | 当前架构为并行共存（Bubble Tea 为可选 TUI，go-prompt 为默认 REPL）；提供 `--tui=bubbletea` 开关 |
| LSP 服务器未安装导致功能缺失 | 高 | 低 | 自动检测 + 友好提示安装命令；回退到 tree-sitter + regex |
| AgentPrimordia 框架更新破坏 API | 中 | 高 | 锁定框架版本；replace 指令确保一致性；CI 中跑集成测试 |
| 大型项目索引内存溢出 | 低 | 高 | 设置内存预算上限；超过 50K 文件时自动降级为懒加载模式 |
| SQLite 并发写入冲突 | 中 | 中 | 已用共享连接 + WAL 模式；压力测试验证；降级到内存模式 |
| API Key 泄露 | 低 | 极高 | config.yaml 权限 0600；安全警告注释；.gitignore 默认排除 ~/.codecast |

---

## 附录

### A. 验收检查清单

| 阶段 | 核心指标 | 目标值 |
|------|---------|--------|
| Phase 1 | 工具调用成功率 | > 90%（当前估计 ~70%） |
| Phase 1 | 首次请求到首 Token 延迟 (TTFT) | < 2s |
| Phase 1 | Ctrl+C 中断响应 | < 500ms |
| Phase 2 | Repo Map 生成耗时 | < 3s |
| Phase 2 | LSP goto_definition 正确率 | > 95% |
| Phase 2 | 索引缓存加载 | < 500ms |
| Phase 2 | TUI 流式输出帧率 | > 30fps |
| Phase 3 | 支持的安装渠道 | >= 5 |
| Phase 4 | 智能路由成本节省 | > 40% |
| Phase 5 | 冷启动时间 | < 1.5s |
| Phase 5 | 热启动时间 | < 500ms |
| Phase 5 | 稳态内存占用 | < 100MB |
| Phase 5 | interactive.go 主文件行数 | < 250 |
| Phase 5 | 并发压力测试 | 无 race/panic/死锁 |
| Phase 5 | 真实项目工具调用成功率 | > 90% |

### B. 技术债务清单

| 文件 | 问题 | 优先级 |
|------|------|--------|
| `cmd/interactive.go` | 2250 行巨型文件（Phase 1 时 1382 行） | P1 → Phase 5.2 |
| `internal/agent/agent.go` | 1074 行，职责混合 | P1 → Phase 5.2 |
| `extractJSONField` | 手写 JSON 解析，安全隐患 | P0 ✅ 已修复 |
| `subagent/orchestrator.go:95` | 变量丢弃 bug | P0 ✅ 已修复 |
| `grep.go` | 无 .gitignore 感知 | P1 ✅ 已修复 |
| `compress.go` | 定义了配置但无实际压缩逻辑 | P1 ✅ 已修复 |
| `interactive.go:71` | 全局变量 `codecastAgent` | P2 → Phase 5.2 |
| 错误消息不统一 | 散落 `fmt.Errorf` 无错误码和修复建议 | P2 → Phase 5.2 |
| 缺乏压力测试 | 并发组件多但无系统性压测 | P1 → Phase 5.3 |

### C. 依赖引入清单

| 依赖 | 用途 | Phase |
|------|------|-------|
| `github.com/charmbracelet/bubbletea` | TUI 框架 | Phase 2 |
| `github.com/charmbracelet/lipgloss` | TUI 样式 | Phase 2 |
| `github.com/smacker/go-tree-sitter` | AST 解析 | Phase 2 |
| `github.com/fsnotify/fsnotify` | 文件监听 | Phase 2 |
| `github.com/sabhiram/go-gitignore` | .gitignore 解析 | Phase 1 |
| `github.com/sourcegraph/jsonrpc2` | LSP 通信 | Phase 2 |
| `github.com/goreleaser/goreleaser` | 多平台发布 | Phase 3 |

### D. 里程碑时间线

```
Month 1  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ├── Week 1-2: Task 1.1 (工具集) + Task 1.3 (JSON安全)
  ├── Week 2-3: Task 1.2 (grep性能) + Task 1.5 (重试机制)
  └── Week 3-4: Task 1.4 (上下文压缩) + Task 1.6 (Ctrl+C)

Month 2  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ├── Week 1-2: Task 2.1 (tree-sitter) + Task 2.3 (增量索引)
  ├── Week 2-3: Task 2.2 (LSP集成)
  └── Week 3-4: Task 2.5 (子Agent修复) + Task 2.6 (@引用)

Month 3  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ├── Week 1-4: Task 2.4 (Bubble Tea TUI) — 大工程
  └── 穿插: Task 3.3 (测试框架)

Month 4  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ├── Week 1-2: Task 3.1 (多平台分发) + Task 3.2 (VS Code)
  └── Week 3-4: Task 4.1 (智能路由) + Task 4.2 (可视化)

Month 5-6 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  └── Task 4.3 (Git-aware) + Phase 5 打磨与上线

Month 7  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ├── Week 1-2: Task 5.1 (性能优化) + Task 5.2 (代码重构)
  └── Week 3-4: Task 5.3 (压力测试) + Task 5.4 (文档)

Month 8  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ├── Week 1-2: Task 5.5 (社区) + Task 5.6 (真实项目验证)
  └── Week 3-4: Task 5.7 (发布 v0.4.0)
```
