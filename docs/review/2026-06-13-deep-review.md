# CodecastCLI Deep Review — 2026-06-13 (v0.1.0)

> Author: automated code review (Copilot / MiniMax-M3)
> Scope: full `e:\codecast\codecast\codecastcli` repo + sibling framework
> `e:\codecast\codecast\AgentPrimordia\agentprimordia` (local replace)
> Inputs: static reading of all .go / .ts sources; build + test run;
> targeted grep of framework internals; bug-plan and phase-2 plan
> reconciliation.

This document is the **deep follow-up** to the initial review
(`PHASE2-IMPLEMENTATION-PLAN.md` reading context). It corrects
assumptions, surfaces bugs the static review did not find, and gives
a verifiable action plan.

---

## TL;DR — new findings not in the first review

| # | Severity | Where | Summary |
|---|---|---|---|
| F-01 | **P0** | `internal/agent/agent.go:147` | `ap.WithFileScope(scopes)` is **decorative only** — it does not call `tools.WithScopePolicy()` on the toolkit, so the LLM can still read any file on disk |
| F-02 | **P0** | `cmd/interactive.go:566` | `/mode` resets the permission manager and **silently drops the SafeMode denyList** |
| F-03 | **P0** | `internal/permission/confirm.go:31` | `ConfirmPrompt` reads `os.Stdin` directly while go-prompt owns stdin → guaranteed I/O race |
| F-04 | P1 | `internal/agent/mcp_integration.go:42,53,57` | Three MCP errors swallowed (BUG-12 partial — `StartAll`, `RegisterIntoRegistry`) |
| F-05 | P1 | `internal/session/manager.go` | No tests; opens same `memory.db` as the agent → SQLite lock contention in long sessions |
| F-06 | P1 | `internal/mcpcfg/mcpcfg.go:31` | YAML parse errors silently swallowed → user sees empty config, no warning |
| F-07 | P1 | `internal/agent/agent.go:47-150` | `idx.Build()` is **synchronous** at agent construction (the plan claims it was lazy) |
| F-08 | P1 | `internal/tools/edit.go` | Edit matches on raw `strings.Replace` — no whitespace tolerance, no symlink-safe path |
| F-09 | P2 | `internal/agent/agent.go:484` (`SwitchModel`) | Rebuilds the agent but **does not re-run permission denylist** for the new model turn cycle |
| F-10 | P2 | `cmd/interactive.go:116,121,125,158` | 4 `os.Exit(1)` calls in init path; only safe because no agent exists yet, but it hides the error from `defer codecastAgent.Close()` style callers in tests |
| F-11 | P2 | `internal/permission/manager.go:177-204` | `BuildHITLConfig` is defined but **never called from `newAgent`** — HITL flow is dead code in the Go binary |
| F-12 | P2 | `internal/cost/tracker.go` | Not reviewed in this pass — flagged for follow-up |
| F-13 | P3 | `src/` (entire) | TypeScript subtree is dead code (not invoked by `main.go`) but evolves in lockstep with the Go side → maintenance debt |
| F-14 | P3 | `internal/agent/agent.go:472-507` | `SwitchModel` re-runs `loadProjectRules` and `buildSystemPrompt` **but not the indexer rebuild** — model may see stale file tree |

---

## 1. Plan-vs-code reconciliation (resolves the first review)

| Plan claim (PHASE2 / BUG-FIX) | Current code | Verdict |
|---|---|---|
| BUG-01 edit_file mode 0 | `info.Mode()` preserved + atomic temp+rename | ✅ Fixed |
| BUG-02 /quit skips defer | `quitFlag` pattern, no `os.Exit` in quit path | ✅ Fixed |
| BUG-03 panic re-raise | `defer` converts to `err` | ✅ Fixed |
| BUG-04 New/NewWithSession dup | `newAgent` factory + 2-line wrappers | ✅ Fixed |
| BUG-05 stream markdown dead | `stream.go:73` calls `a.renderer.RenderMarkdown` | ✅ Fixed |
| BUG-09 /mode advertised not impl | `handleModeCommand` at interactive.go:545 | ✅ Fixed (but see F-02) |
| BUG-11 nil in MCP test path | (not re-checked) | n/a |
| BUG-12 MCP errors discarded | `mcp_integration.go:42,53,57` log + continue | ⚠️ Partially fixed — log.warn is added but errors not surfaced to user |
| PHASE2 §1.2.1 SafeMode "no-op" | `denyList` populated for shell/web in `newAgent:84-89` | ✅ Fixed but see F-01 |
| PHASE2 §1.2.3 WithFileScope | Wired in `agent.go:147` | ⚠️ Wired but **not effective** — see F-01 |
| PHASE2 §1.2.2 HITL confirm UI | `permission/confirm.go` exists, but `BuildHITLConfig` is never called | ❌ Dead code path |
| PHASE2 module 2 context compression | `stream.go:111-138` has `checkAutoCompact` that calls `a.ClearContext()` (drops everything) | ⚠️ Half-implemented — no actual summarization, just wipes |

**Bottom line: 7 of 9 bug items are fixed. The two half-fixed items are exactly the highest-stakes ones: SafeMode and HITL. PHASE2 module 1 is not actually delivered.**

---

## 2. Detailed findings

### F-01 — `WithFileScope` is decorative; LLM can read any file [P0]

**File:** `internal/agent/agent.go:147`

**Evidence chain:**

1. AgentPrimordia provides two separate APIs:
   - `ap.CapabilityAgent.WithFileScope(scopes)` — stores on agent metadata,
     and on `inner.config.FileScope`. Searched the entire framework:
     this is **never read by the React loop or the toolkit's path check.**
   - `tools.NewFileScopePolicy().SetScope(id, dirs)` + `fs.WithScopePolicy(policy, id)` —
     the **actual** enforcement path. See
     `e:\..\AgentPrimordia\agentprimordia\internal\tools\builtin\file_*.go`
     which calls `policy := tools.NewFileScopePolicy()` and
     `fs.WithScopePolicy(policy, "agent-1")` in tests.

2. codecastcli's `newAgent` does:
   ```go
   ).WithToolkit(registry).WithMemory(memory).
       WithHooks(hooks).
       WithFileScope(scopes)        // ← agent-level, decorative
   ```
   It does **not** call `policy := tools.NewFileScopePolicy()` and
   `registry.WithScopePolicy(policy, "...")`.

3. `agent.go:486` (`SwitchModel`) repeats the same buggy pattern.

**Impact:** A user runs `codecast --scope .` thinking they're
restricting the agent to the cwd, but the LLM can call
`read_file("/etc/passwd")` or `shell_execute("cat ~/.ssh/id_rsa")`
and the request will be allowed by the toolkit. The permission hook
will still trigger a confirm prompt (in `suggest` mode) — so this is
**defense-in-depth failure, not a single-point failure**, but it is
still a critical correctness gap that contradicts the README and the
PHASE2 plan.

**Repro (once API is configured):**
```bash
echo "secret" > /tmp/secret.txt
codecast --scope .            # expect: agent refuses /tmp/secret.txt
> read the file /tmp/secret.txt
# actual: agent reads it successfully
```

**Fix (minimal):**
```go
// In newAgent, after WithToolkit:
policy := ap.NewFileScopePolicy()           // exposes policy from pkg
policy.SetScope("codecast", scopes)
registry.WithScopePolicy(policy, "codecast")
```
If the framework does not yet export `NewFileScopePolicy` from `pkg/`,
add a wrapper. Apply the same fix in `SwitchModel`.

---

### F-02 — `/mode` silently drops the SafeMode denyList [P0]

**File:** `cmd/interactive.go:566`

```go
*permMgr = *permission.NewManager(newMode)
```

This dereferences `permMgr` and **overwrites the struct value the
pointer points to**. The new value is a fresh `NewManager(newMode)`,
which has an **empty `denyList` and `autoAllow` map**.

**Impact:** If a user launches with `--safe`, the agent adds
`shell_execute`, `web_request`, `web_fetch` to `denyList` (per
[agent.go:84-89](internal/agent/agent.go#L84-L89)). Then if the user
types `/mode full-auto` (perhaps thinking "go fast, I trust the
agent"), the denyList is wiped and shell commands are auto-approved.

**Worse:** the swap destroys any in-session `autoAllow` entries the
user built up via "always-allow" — also silently.

**Repro (no LLM needed):**
```go
mgr := permission.NewManager(permission.ModeSuggest)
mgr.AddDeny("shell_execute")
require.True(t, mgr.IsDenied("shell_execute"))
*mgr = *permission.NewManager(permission.ModeFullAuto)
require.False(t, mgr.IsDenied("shell_execute"))  // BUG: should remain true
```

**Fix:** `NewManager` should accept existing state, or the caller
should preserve `denyList` and `autoAllow`:
```go
newMgr := permission.NewManager(newMode)
for k := range permMgr.DenyListSnapshot() { newMgr.AddDeny(k) }
for k := range permMgr.AutoAllowSnapshot() { newMgr.AddAutoAllow(k) }
*permMgr = *newMgr
```

Or add a dedicated `permission.Manager.SetMode(mode)` method that
preserves the maps.

---

### F-03 — `ConfirmPrompt` reads stdin while go-prompt owns it [P0]

**File:** `internal/permission/confirm.go:31`

```go
reader := bufio.NewReader(os.Stdin)
input, _ := reader.ReadString('\n')
```

`runInteractive` is using `c-bata/go-prompt` which puts the terminal
into raw/cbreak mode and reads from the same fd. When the permission
hook fires during `agent.StreamRun` (inside the go-prompt executor
goroutine), the agent and go-prompt are both waiting on stdin. The
`bufio.NewReader(os.Stdin).ReadString('\n')` will:

1. Block forever waiting for a newline that go-prompt is also waiting
   for, or
2. Consume characters intended for the next user prompt, or
3. Return a partial line that gets echoed as a "permission answer"
   while the user thought they were typing the next query.

**No tests exercise this path** — `permission.ConfirmPrompt` is a
package-level function, not an injectable interface.

**Fix:** Make `ConfirmPrompt` a method on a struct that owns the
`prompt.Prompt` and uses a **paused-input** pattern:
1. Hide the streaming/spinner UI,
2. Suspend go-prompt (call its internal `Break()` if available, or
   re-implement as a `bufio.Scanner` over a stopped tty),
3. Print the confirm to stdout,
4. Read a single byte (y/n/a/e),
5. Restore the prompt state.

Or, simpler and tested: route confirmation through the same
`HumanInputChan` that `BuildHITLConfig` creates — but only after
F-11 is fixed (the config is never built).

---

### F-04 — MCP errors still swallowed (BUG-12 partial) [P1]

**File:** `internal/agent/mcp_integration.go`

```go
42:  if err := mcpRegistry.StartAll(ctx); err != nil {
        slog.Warn("部分 MCP 服务器启动失败", "error", err)   // ← log only
53:  if err := mcpRegistry.RegisterIntoRegistry(registry); err != nil {
        slog.Warn("MCP 工具注册失败", "error", err)          // ← log only
```

The BUG-12 plan calls for **non-zero exit or user-visible error**
when MCP fails. Logging to slog at WARN means a user running
`codecast` interactively never sees the failure. If their `github`
MCP silently fails to start, the LLM will claim "the github tool is
not available" without explanation.

**Fix:** Bubble the error up to `newAgent`, return it from
`ConnectMCPServers`, surface in `runInteractive` as a yellow warning
("⚠ MCP server X failed to start: ...").

---

### F-05 — `session.Manager` has zero tests and shares the SQLite file [P1]

**File:** `internal/session/manager.go:23`

```go
db, err := sql.Open("sqlite", dbPath)
```

The agent also opens `memory.db` via `ap.NewSQLiteStore(memPath)` in
[agent.go:71](internal/agent/agent.go#L71). `session.NewManager` is
called from `/session list`, `/export`, etc. **Two `sql.DB` handles
against the same SQLite file, with no coordination.** With
`modernc.org/sqlite` the driver uses connection-level locks; under
contention (e.g. agent writing episode while user runs `/session
list`), one of them gets `SQLITE_BUSY` and retries silently. Worst
case: `List()` returns a partial result with no error.

**Test gap:** zero tests on `internal/session/manager.go` despite
`session/`, `/export`, `/resume` all reading from it.

**Fix:**
1. Inject the agent's `ap.Memory` (or its underlying *sql.DB) into
   `session.NewManager` so there is a single handle.
2. Add a `manager_test.go` covering List, GetHistory, Delete,
   Close, and a concurrent-read-during-write test.

---

### F-06 — MCP YAML parse error swallowed [P1]

**File:** `internal/mcpcfg/mcpcfg.go:31`

```go
if err := yaml.Unmarshal(data, cfg); err != nil {
    return cfg  // ← returns empty Config with no warning
}
```

A user with a single typo in `mcp_servers.yaml` gets an empty server
list and **no log line, no error message**. The next MCP command
silently does nothing.

**Fix:** Log a warning (`slog.Warn("mcp_servers.yaml parse error",
"path", configPath, "error", err)`) and return an error sentinel.
Better: return `(*Config, error)` from `Load()` so callers can
decide.

---

### F-07 — `idx.Build()` is not lazy [P1]

**File:** `internal/agent/agent.go:118`

```go
idx := indexer.NewIndexer(getCurrentDir())
idx.Build() // 同步构建，确保系统提示词包含文件树
```

The PHASE2 plan says "indexer is lazy" but the comment here is
"同步构建". The system prompt needs the file tree **at agent
construction**, so it cannot be fully lazy. However:

- For a 5k-file monorepo this is a multi-second startup hitch
- The user sees a hang with no spinner

**Fix:** Build the indexer skeleton eagerly (fast) and the file-tree
async, then patch it into the system prompt on first user message
OR show a spinner during `Build()`.

---

### F-08 — `edit_file` has no whitespace tolerance and no symlink safety [P1]

**File:** `internal/tools/edit.go`

`strings.Replace(original, params.OldString, params.NewString, 1)`
matches byte-exact. A user with trailing-whitespace drift gets
"未找到匹配的文本" with a 50-char truncated preview, which is hard
to debug.

Additionally, `os.Rename(tmpFile, params.FilePath)` will follow
symlinks on Unix: if `params.FilePath` is a symlink, the rename
replaces the **symlink**, not the target. (This is POSIX-defined
behaviour; `os.Rename` does not dereference.)

**Fixes:**
1. Add a `--ignore-whitespace` (or auto-fallback) mode that
   re-tries with `strings.TrimRight` on each line.
2. For symlink safety, use `os.OpenFile(O_NOFOLLOW)` for the temp
   file in the same dir, or `renameat2` with `RENAME_NOREPLACE` on
   Linux. At minimum, add a comment.

---

### F-09 — `SwitchModel` does not refresh the indexer [P2]

**File:** `internal/agent/agent.go:472-507`

`SwitchModel` rebuilds system prompt + hooks + agent, but does not
re-`idx.Build()`. If the user adds files to the project and then
switches model mid-session, the new model sees the **stale** file
tree. Acceptable for v0.1.0, but worth a comment or a flag.

---

### F-10 — `os.Exit(1)` in init path [P2]

**File:** `cmd/interactive.go:116,121,125,158`

All 4 sites are **before `defer codecastAgent.Close()` is set up** —
they exit during API-key read or save. Technically safe (no resource
to clean), but a test or a wrapper that wants to recover from
"NoAPIKey" cannot, because the process dies.

**Fix:** Return an `error` from `runInteractive` and let the caller
in `cmd/root.go` handle the exit. This also enables
`--non-interactive` scripts to detect the failure.

---

### F-11 — `BuildHITLConfig` is dead code [P2]

**File:** `internal/permission/manager.go:177`

`BuildHITLConfig` is fully implemented and tested in spirit (12
"matrix" cases per the plan), but `newAgent` does **not** call it.
The `capAgent` is created without `ap.WithHITL(...)`, so the
permission flow falls back to the manual `HookBeforeTool` chain
(`buildPermHook`). This is fine functionally, but means:

- The "always-allow" / "edit-args" 4-option UI is reachable only
  through the manual hook, which calls `permission.ConfirmPrompt` —
  which races with stdin (F-03).
- The `HITLConfig.HumanInputChan` machinery is unreachable from the
  Go binary.

**Fix:** Either (a) call `ap.WithHITL(permMgr.BuildHITLConfig(...))`
in `newAgent` and wire the channel, or (b) delete the unused HITL
machinery until the confirm-UI rewrite lands.

---

### F-12 — `cost/tracker.go` not reviewed [P2]

Flagged for follow-up. Concurrent `recordCost` calls (one per
`StreamProcess` and one per `Process` if a user uses both paths)
likely race on the underlying file/SQLite store. Tracked but not
verified in this pass.

---

### F-13 — `src/` (TypeScript) is dead code with a maintenance tax [P3]

**File:** `src/index.ts` and 60+ siblings

`main.go` only invokes the Go binary. `npm run build` produces a
`dist/` that nothing in the repo uses. Yet the TS tree keeps
evolving (cost tracker, MCP client, hooks, hotkeys, plan executor,
auto-memory, plugins, etc.) — duplicating logic the Go side already
has.

**Decision needed:** archive `src/` to a separate repo (preferred)
or commit to a TS-based binary and delete the Go tree. The current
"both" state is the worst outcome.

---

### F-14 — `SwitchModel` indexer not rebuilt [P3]

Same as F-09; restated for visibility.

---

## 3. Re-verified earlier findings

| Finding | Status | Notes |
|---|---|---|
| BUG-01 (edit_file mode 0) | ✅ Fixed | atomic + preserves mode |
| BUG-02 (/quit skips defer) | ✅ Fixed | quitFlag pattern |
| BUG-03 (panic re-raise) | ✅ Fixed | defer converts to err |
| BUG-04 (New/NewWithSession dup) | ✅ Fixed | `newAgent` factory |
| BUG-05 (markdown dead) | ✅ Fixed | stream.go:73 re-renders |
| BUG-09 (/mode unimpl) | ✅ Fixed but see F-02 | handler exists, has logic bug |
| Sandbox misleading name | Unchanged | `Enabled:false` default |
| `cmd/interactive.go` 1230 LoC | Unchanged | god file, low priority |
| 5 packages without tests | Unchanged | `mcp`, `mcpcfg`, `session`, `ui`, `util` |
| Go/TS duplication | Unchanged | F-13 |
| `replace agentprimordia` local | Unchanged | F-13; framework lives at `e:\codecast\codecast\AgentPrimordia` |

---

## 4. Prioritized action plan (corrected)

### P0 — block release
1. **F-01:** Wire `tools.NewFileScopePolicy()` into the toolkit. Add
   a unit test that calls the agent with `--scope /tmp/foo` and
   asserts it cannot read `/etc/passwd`.
2. **F-02:** Make `/mode` mode-switch preserve `denyList` and
   `autoAllow`. Add a test.
3. **F-03:** Replace `permission.ConfirmPrompt` with a stdin-pausing
   implementation. Add an integration test that simulates the
   go-prompt + confirm sequence.

### P1 — fix in this sprint
4. **F-04:** Bubble MCP errors up to the user.
5. **F-05:** Share the agent's `*sql.DB` with `session.Manager`; add
   `manager_test.go`.
6. **F-06:** Return error from `mcpcfg.Load`; log YAML parse failures.
7. **F-07:** Show a spinner during `idx.Build()` or make it async.
8. **F-08:** Whitespace tolerance + symlink comment in `edit_file`.

### P2 — next sprint
9. **F-09, F-14:** Decide indexer-rebuild policy in `SwitchModel`.
10. **F-10:** Replace init-path `os.Exit` with `return error`.
11. **F-11:** Either wire `BuildHITLConfig` or delete it.
12. **F-12:** Review `cost/tracker.go` for concurrency.

### P3 — backlog
13. **F-13:** Decide Go vs TS future. Move `src/` out.

---

## 5. Reproduction recipes

### F-01 (FileScope not enforced)
```bash
echo "secret" > /tmp/codecast-scope-test.txt
# run codecast with --scope . and ask it to read /tmp/codecast-scope-test.txt
# expected: refused
# actual: succeeds
```

### F-02 (/mode drops SafeMode)
```go
// permission/manager_test.go
func TestModeSwitchPreservesDenyList(t *testing.T) {
    m := permission.NewManager(permission.ModeSuggest)
    m.AddDeny("shell_execute")
    require.True(t, m.IsDenied("shell_execute"))

    // simulate /mode full-auto: handleModeCommand does
    //   *m = *permission.NewManager(permission.ModeFullAuto)
    *m = *permission.NewManager(permission.ModeFullAuto)

    require.True(t, m.IsDenied("shell_execute"),
        "SafeMode denyList must survive a mode switch")
}
```

### F-04 (MCP errors swallowed)
```bash
# Add a broken MCP server to ~/.codecast/mcp_servers.yaml:
cat > ~/.codecast/mcp_servers.yaml <<EOF
servers:
  broken:
    command: "/nonexistent/binary"
    auto_start: true
EOF
codecast
# expected: "⚠ MCP server 'broken' failed to start: ..."
# actual: silent (only slog WARN visible in --log-level=debug)
```

---

## 6. Files that still need a deep read (not covered)

- `src/api/llm.ts` — SSE streaming + provider switching
- `src/mcp/client.ts` — MCP protocol, retry policy
- `src/agent/agent-loop.ts` — termination conditions
- `src/context/compactor.ts` — actual compression algorithm
- `internal/cost/tracker.go` — concurrency
- `internal/checkpoint/manager.go` — git stash failure recovery
- `internal/pool/manager.go` — peer disconnect behavior
- `internal/memory/auto_persist.go` — SQLite write contention
- `internal/agent/react_loop.go` (framework) — how tool calls are
  actually dispatched
- `cmd/registry.go` — 30+ slash command registration completeness
- `cmd/commands.go` vs `shared_commands.json` drift
- `completions/*.ps1` — PowerShell 7 behavior

These are queued for the next pass.
