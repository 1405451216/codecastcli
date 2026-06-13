# Codecast CLI Bug 修复清单

> 版本: v1.0 | 日期: 2026-06-12
> 范围: 四大模块（TUI / 结构化编辑 / MCP 持久化 / 会话恢复）实施后发现的关键问题

---

## BUG-01: edit_file 写入文件时权限模式为 0（文件损坏级）

**严重度:** P0 — 在 Unix 系统上会导致文件不可读写
**文件:** `internal/tools/edit.go` 第 111 行
**现状:**

```go
if err := os.WriteFile(params.FilePath, []byte(newContent), 0); err != nil {
```

`os.WriteFile` 的第三个参数是 `0`（即 `----------`），在 Unix 上写出的文件没有任何读写权限，等于文件损坏。Windows 不受影响（NTFS 忽略 POSIX 权限位），但跨平台项目必须修。

**修复方案:**

在读取文件时（第 81 行附近）保存原始文件信息，写回时保留原权限：

```go
// 读取文件（保留原始文件信息）
info, err := os.Stat(params.FilePath)
if err != nil {
    return ap.NewToolErrorResult(...), nil
}
content, err := os.ReadFile(params.FilePath)
// ...

// 写回文件（保留原始权限）
if err := os.WriteFile(params.FilePath, []byte(newContent), info.Mode()); err != nil {
```

**附带改进 — 原子写入:** 当前直接覆盖原文件，如果写入中途失败（磁盘满、断电），原内容丢失。建议改为写临时文件再 rename：

```go
tmpFile := params.FilePath + ".tmp"
os.WriteFile(tmpFile, []byte(newContent), info.Mode())
os.Rename(tmpFile, params.FilePath)
```

---

## BUG-02: /quit 使用 os.Exit(0) 跳过资源清理（资源泄漏）

**严重度:** P0 — 每次退出都泄漏 MCP 子进程、SQLite 连接、成本追踪器
**文件:** `cmd/interactive.go` 第 205 行
**现状:**

```go
func executor(input string) {
    // ...
    if handleSpecialCommand(input, codecastAgent, &running) {
        if !running {
            os.Exit(0)  // ← 直接退出，跳过 defer codecastAgent.Close()
        }
        return
    }
    // ...
}
```

`runInteractive()` 第 100 行有 `defer codecastAgent.Close()`，但 `os.Exit(0)` 不执行任何 defer。后果：MCP 服务器子进程变成孤儿进程，SQLite 连接未关闭（数据库文件可能处于锁定状态），成本追踪器的未刷写数据丢失。

**修复方案:**

go-prompt 没有内置的优雅退出 API，但可以通过抛出自定义 panic 配合外层 recover 来实现：

```go
// 定义退出信号类型
type promptExit struct{}

// executor 中：
if !running {
    panic(promptExit{})  // 抛出退出信号
}

// runGoPromptREPL 中：
defer func() {
    if r := recover(); r != nil {
        if _, ok := r.(promptExit); ok {
            return  // 正常退出，不重新抛出
        }
        // 其他 panic，转为 error 让上层回退到 bufio
        err = fmt.Errorf("go-prompt panic: %v", r)
    }
}()
```

这样 `runGoPromptREPL` 正常返回后，`runInteractive` 的 `defer codecastAgent.Close()` 会执行。

---

## BUG-03: go-prompt panic recovery 重新抛出异常（降级机制失效）

**严重度:** P1 — go-prompt 在不兼容终端上 panic 时整个进程崩溃
**文件:** `cmd/interactive.go` 第 147-151 行
**现状:**

```go
defer func() {
    if r := recover(); r != nil {
        panic(r) // 重新抛出，让 runBufioREPL 接管
    }
}()
```

注释说"让 runBufioREPL 接管"，但 `panic(r)` 会直接终止进程——第 120-122 行的回退逻辑永远不会执行：

```go
if err := runGoPromptREPL(); err != nil {
    // 这里永远到不了，因为 panic(r) 已经终止了进程
    runBufioREPL()
}
```

**修复方案:**

将 panic 转为 error 返回，让回退逻辑正常工作：

```go
defer func() {
    if r := recover(); r != nil {
        if _, ok := r.(promptExit); ok {
            return  // BUG-02 的退出信号，正常返回
        }
        err = fmt.Errorf("go-prompt 内部异常: %v", r)
    }
}()
```

注意：由于 `runGoPromptREPL` 使用命名返回值（需改为命名返回值），defer 中才能修改 `err`。

---

## BUG-04: New() 和 NewWithSession() 大量代码重复（维护炸弹）

**严重度:** P1 — 约 90 行几乎完全重复，任何改动都要做两遍
**文件:** `internal/agent/agent.go` 第 31-126 行 vs 第 129-224 行
**现状:**

两个函数的唯一区别是 Session 创建方式（第 108 行 vs 第 206 行）和 sessionID 赋值（第 122 行 vs 第 220 行）。Provider 创建、工具注册、MCP 连接、记忆存储初始化、系统提示词、成本追踪器——全部重复。

**修复方案:**

抽取内部工厂函数，用可选参数区分：

```go
func newAgent(cfg *config.Config, sessionID string) (*CodecastAgent, error) {
    // 共享的初始化逻辑（Provider / 工具 / MCP / 记忆 / 系统提示词 / 成本追踪器）
    // ...

    // 会话创建
    var session *ap.Session
    if sessionID != "" {
        session = ap.NewSession(agent, memory, ap.SessWithID(sessionID))
    } else {
        session = ap.NewSession(agent, memory)
        sessionID = session.SessionID()
    }

    return &CodecastAgent{...}, nil
}

// 公开 API 保持简洁
func New(cfg *config.Config) (*CodecastAgent, error) {
    return newAgent(cfg, "")
}

func NewWithSession(cfg *config.Config, sessionID string) (*CodecastAgent, error) {
    return newAgent(cfg, sessionID)
}
```

---

## BUG-05: 流式输出的 Markdown 渲染未生效（死代码）

**严重度:** P1 — 用户看到的仍然是原始 Markdown 标记
**文件:** `internal/agent/stream.go` 第 28-40 行
**现状:**

```go
var tokenBuf strings.Builder // 累积 token 用于最终 glamour 渲染
// ...
case ap.StreamEventToken:
    fmt.Print(evt.Content)           // ← 直接输出原始文本
    tokenBuf.WriteString(evt.Content) // ← 累积了但从未使用
```

`tokenBuf` 收集了完整响应，但在 `StreamEventComplete` 分支中完全没有调用 `ui.RenderMarkdown(tokenBuf.String())`。`internal/ui/markdown.go` 的 glamour 渲染器存在但只被 `PrintAssistant`（非流式路径）调用。

**修复方案:**

在 `StreamEventComplete` 时，清屏当前原始输出，替换为 glamour 渲染版本：

```go
case ap.StreamEventComplete:
    ui.StopSpinner()

    // 用 glamour 渲染完整响应
    if tokenBuf.Len() > 0 {
        rendered := ui.RenderMarkdown(tokenBuf.String())
        // 回到输出起始位置，清除原始文本，输出渲染版本
        fmt.Print("\r\033[J") // 清除当前行及以下
        fmt.Println(rendered)
    }
    fmt.Println()
    // ... 统计行
```

**注意事项:**
- 这个方案在工具调用穿插输出的场景下需要额外处理（不能在工具结果之后清除之前的内容）
- 备选方案：不替换，而是在流式输出完成后追加一个"渲染视图"区域
- 需要测试 CJK 字符宽度计算是否准确

---

## BUG-06: ClearContext 后 sessionID 未更新

**严重度:** P2 — 清除上下文后 `GetSessionID()` 返回过期的旧 ID
**文件:** `internal/agent/agent.go` 第 264-271 行
**现状:**

```go
func (a *CodecastAgent) ClearContext() {
    a.session = nil
    a.session = ap.NewSession(a.agent, a.memory)  // 新 Session，新 ID
    // a.sessionID 没有更新！
}
```

`/clear` 后 `a.sessionID` 仍然指向旧会话。如果用户随后执行 `/export` 或退出后用 `--continue`，会引用错误的会话。

**修复方案:**

```go
func (a *CodecastAgent) ClearContext() {
    a.session = nil
    a.session = ap.NewSession(a.agent, a.memory)
    a.sessionID = a.session.SessionID()  // 同步更新
}
```

---

## BUG-07: MCP 配置类型重复定义

**严重度:** P2 — 两个包各自维护一份相同的 struct，未来修改容易遗漏
**文件:** `cmd/mcp.go` 第 154-163 行 vs `internal/agent/mcp_integration.go` 第 17-26 行
**现状:**

`cmd/mcp.go` 定义了 `mcpServerConfig`（带 `json` + `yaml` tag）和 `mcpConfig`。
`internal/agent/mcp_integration.go` 也定义了 `mcpServerConfig`（仅 `yaml` tag）和 `mcpConfig`。

两者当前兼容，但一方新增字段另一方不改就会出 bug。

**修复方案:**

将类型抽取到共享包 `internal/mcpcfg`：

```
internal/mcpcfg/types.go     — mcpServerConfig + mcpConfig 定义
internal/mcpcfg/loader.go    — LoadMCPConfig() / SaveMCPConfig()
```

`cmd/mcp.go` 和 `internal/agent/mcp_integration.go` 都改为导入 `mcpcfg` 包。

---

## BUG-08: truncate 函数三处重复定义

**严重度:** P3 — 代码异味
**文件:**
- `cmd/mcp.go` 第 199 行 — `func truncate(s string, n int) string`
- `internal/tools/edit.go` 第 164 行 — `func truncate(s string, maxLen int) string`
- `internal/agent/stream.go` 第 115 行 — `func truncate(s string, maxLen int) string`

三处逻辑完全相同（`s[:maxLen] + "..."`），但 `cmd/mcp.go` 的实现用 `s[:n-3]` 而另两处用 `s[:maxLen]`，行为有细微差异。

**修复方案:**

抽取到 `internal/util/strings.go`：

```go
package util

func Truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    if maxLen <= 3 {
        return s[:maxLen]
    }
    return s[:maxLen-3] + "..."
}
```

---

## BUG-09: /mode 命令在补全列表中存在但未实现

**严重度:** P3 — 用户选择 `/mode` 后无任何响应
**文件:** `cmd/interactive.go` 第 37 行（补全定义）vs 第 285-328 行（命令处理）
**现状:**

`commandSuggestions` 包含 `/mode`，但 `handleSpecialCommand` 的 switch 里没有 `/mode` case。用户输入 `/mode` 后会被当作普通消息发给 Agent。

**修复方案:**

两种选择：
- **方案 A（推荐）:** 暂时从补全列表移除，等 Ask/Plan/Build 模式切换功能实现后再加回
- **方案 B:** 添加 case 返回提示 "模式切换功能尚未实现"

---

## BUG-10: mcp test 命令未校验 registry.Get 返回值

**严重度:** P2 — 连接失败后可能 nil pointer panic
**文件:** `cmd/mcp.go` 第 143-144 行
**现状:**

```go
entry, _ := registry.Get(name)
tools := entry.Tools  // 如果 entry 为 nil，这里 panic
```

`registry.Get` 的第二个返回值（`ok`）被丢弃，如果 Start 成功但 Get 失败（理论上不太可能但存在边界条件），会 nil pointer panic。

**修复方案:**

```go
entry, ok := registry.Get(name)
if !ok || entry == nil {
    color.Red("获取 MCP 服务器信息失败")
    os.Exit(1)
}
```

---

## BUG-11: 历史文件路径使用字符串拼接而非 filepath.Join

**严重度:** P3 — Windows 兼容性问题
**文件:** `cmd/interactive.go` 第 238 行
**现状:**

```go
dir := home + "/.codecast"
// ...
return dir + "/history", nil
```

项目中其他地方（如 `config.GetConfigDir()`）都使用 `filepath.Join`，这里不一致。

**修复方案:**

```go
dir := filepath.Join(home, ".codecast")
// ...
return filepath.Join(dir, "history"), nil
```

---

## BUG-12: MCP 连接错误被静默丢弃

**严重度:** P2 — 用户不知道 MCP 服务器为什么不可用
**文件:** `internal/agent/agent.go` 第 64 行
**现状:**

```go
mcpReg, _ := ConnectMCPServers(registry)
```

`ConnectMCPServers` 的错误被完全丢弃。如果所有 MCP 服务器都连接失败，用户不会收到任何提示。

**修复方案:**

```go
mcpReg, mcpErr := ConnectMCPServers(registry)
if mcpErr != nil {
    color.Yellow("⚠️  MCP 服务器连接异常: %v", mcpErr)
}
```

---

## 修复优先级汇总

| 编号 | 严重度 | 文件 | 简述 | 预估工时 |
|------|--------|------|------|----------|
| BUG-01 | P0 | edit.go:111 | 文件权限写为 0 | 15min |
| BUG-02 | P0 | interactive.go:205 | /quit 跳过 defer Close | 30min |
| BUG-03 | P1 | interactive.go:149 | panic recovery 失效 | 与 BUG-02 合并修 |
| BUG-04 | P1 | agent.go:31-224 | New/NewWithSession 重复 | 30min |
| BUG-05 | P1 | stream.go:28-40 | Markdown 渲染未接入流式 | 1h |
| BUG-06 | P2 | agent.go:270 | ClearContext 后 sessionID 过期 | 5min |
| BUG-07 | P2 | mcp.go + mcp_integration.go | 类型重复定义 | 30min |
| BUG-08 | P3 | 三处 truncate | 函数重复 | 15min |
| BUG-09 | P3 | interactive.go:37 | /mode 补全但未实现 | 5min |
| BUG-10 | P2 | mcp.go:143 | Get 返回值未校验 | 10min |
| BUG-11 | P3 | interactive.go:238 | 路径拼接不跨平台 | 5min |
| BUG-12 | P2 | agent.go:64 | MCP 错误静默丢弃 | 10min |

**总计预估：约 3.5 小时**
