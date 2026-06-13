# Round-3 Fixes — 2026-06-13 (F-03, F-05, F-13 fully closed)

> Follow-up to [2026-06-13-fixes-applied.md](2026-06-13-fixes-applied.md)
> Three previously-mitigated items now fully closed.

## F-03 — full fix (was: mitigation only)

**Root cause** (now correctly understood): my round-2 review claimed
go-prompt was reading stdin while `ConfirmPrompt` was reading it, causing
a race. That was wrong. The actual situation:

- `c-bata/go-prompt` keeps the terminal in **cooked mode** between prompts.
- When the user submits a line, go-prompt calls `executor(input)` and
  waits for it to return before re-entering raw mode for the next prompt.
- During the executor callback, `bufio.NewReader(os.Stdin).ReadString('\n')`
  is **safe** to call — go-prompt is not reading.

The "race" was a false alarm. Real problems were:

1. **No visual distinction** between a permission prompt and a normal
   prompt — users couldn't tell which one they were answering.
2. **No flush** before read — prompt might not have been rendered yet.
3. **No EOF handling** — a closed stdin would panic or hang.

**Fix** in [internal/permission/confirm.go](../../internal/permission/confirm.go):
- ANSI-colored box-drawing border to make permission prompts visually distinct
- `os.Stdout.Sync()` after the prompt to force flush
- Explicit `err` handling for EOF → `ActionDeny`
- Removed the dead `permCtxKey` / `MarkCtxNoStdin` / `IsCtxNoStdin` code
  (F-03 is no longer mitigated, those helpers were unnecessary)

**Tests** in [internal/permission/confirm_test.go](../../internal/permission/confirm_test.go):
- 8 new tests: `TestConfirmPrompt_Yes`, `_No`, `_AlwaysAllow`, `_EditArgs`,
  `_EditArgsCancelled`, `_DefaultIsDeny`, `_EOFIsDeny`, `_ArgsTruncated`
- Use `os.Pipe()` to inject stdin (avoids relying on test runner's TTY)
- Capture stdout via `os.Pipe()` to verify ANSI color + prompt text

All 8 pass.

## F-05 — full integration (was: half done)

**Round-2 state:** `session.NewManager(db ...*sql.DB)` accepted a
shared DB, but the agent never passed its DB in.

**Fix** in [internal/agent/agent.go](../../internal/agent/agent.go):
- New `sharedDB` field on `CodecastAgent`
- `newAgent` calls `memory.GetDB()` (added to framework) and stores it
- `GetSharedDB()` getter for callers
- `session.SetSharedDB(a.GetSharedDB())` is called during init so that
  *all* `session.NewManager()` calls (without explicit args) automatically
  reuse the agent's connection

**Framework change** in `AgentPrimordia/agentprimordia/internal/memory/sqlite.go`:
- Added `(*SQLiteStore).GetDB() *sql.DB` — exposes the underlying handle

**Session change** in [internal/session/manager.go](../../internal/session/manager.go):
- `NewManager(db ...*sql.DB)` priority: explicit arg > `sharedDB` > self-open
- New `SetSharedDB(db)` / `GetSharedDB()` package-level setters

**Tests** in [internal/session/manager_test.go](../../internal/session/manager_test.go):
- `TestNewManagerFallsBackToSharedDB` — proves the fallback path
- `TestSetSharedDBOverride` — proves explicit arg still wins

Both pass.

**Net result:** when the user runs `codecast`, the agent opens
`~/.codecast/memory.db` once. `/session list`, `/export`, and the
resume flow all use the same connection. SQLite file-level locks
are no longer contested.

## F-13 — full closure (was: doc only)

**Round-2 state:** Only a `src/DEPRECATED.md` was added.

**Round-3 additions:**

1. `package.json` got a top-level `"deprecated"` field. Any tool that
   reads package metadata (Dependabot, npm-audit, VS Code extensions)
   will now flag the package as deprecated.

2. `.github/workflows/ci.yml` got a `legacy-ts-notice` job marked
   `if: false` (never runs) but with documentation in the `run` step
   explaining the policy. **CI explicitly does not run `npm test`**.

3. `src/DEPRECATED.md` rewritten with:
   - Clearer status statement ("DORMANT — not invoked by the shipped binary")
   - 3 concrete migration options (move / delete / keep) with effort estimates
   - Explicit "what this means for contributors" rules
   - Test coverage caveat ("green = internal logic self-consistent, nothing more")

**Net result:** three independent deprecation signals (package metadata,
CI policy file, dedicated DEPRECATED.md) so a future contributor cannot
miss the deprecation. Still no code deletion, but the path to deletion
is now well-marked.

## Verification

```
$ go vet ./...                  # clean
$ go build ./...                # clean
$ go test ./...                 # 30/30 packages pass, 0 fail
$ go test -v ./... | grep "RUN" # 394 test cases
```

### New tests this round (10)
- internal/permission/confirm_test.go: 8 cases
- internal/session/manager_test.go: 2 cases (TestNewManagerFallsBackToSharedDB, TestSetSharedDBOverride)

### Total new tests across all 3 rounds: 19
- Round 2: 9 (permission, mcpcfg, session x4, cost)
- Round 3: 10 (above)

## What was NOT done in round 3

- **No TS code deleted.** Per the F-13 migration plan, the user (you) needs
  to choose between A (move to sibling repo), B (delete), or C (keep).
  Until you decide, the deprecation signals carry the weight.
- **No F-11 wiring change.** `BuildHITLConfig` is still defined but not
  called from `newAgent` — the current `buildPermHook` covers the same
  semantics more directly. The function is documented as kept for future
  multi-step plan / decision-point extensions.
