# Architecture

## Overview

nickbot is organized into three conceptual layers with strict import rules.

```
┌─────────────────────────────────────────────────────────────────┐
│  Application Layer  (future: coding harness, research agents)   │
├─────────────────────────────────────────────────────────────────┤
│  Orchestration Layer  (workflows, scheduling, persistence)       │
│    internal/orchestration/                                       │
├─────────────────────────────────────────────────────────────────┤
│  Execution Layer  (runners, event streaming, provider adapters)  │
│    internal/agent/      — interfaces and shared types           │
│    internal/runners/*   — runner implementations                │
│    internal/store/*     — persistence implementations           │
└─────────────────────────────────────────────────────────────────┘
  Presentation  (internal/ui/)   CLI/TUI — never imports above layers upward
```

## Core Abstractions

### Agent (`internal/agent/`)

The central contract between execution and orchestration.

**`Runner` interface** — every execution backend implements this:
```go
type Runner interface {
    Run(ctx context.Context, req RunRequest) (<-chan Event, error)
    Name() string
}
```

Contract:
- Always emits `EventStarted` as the first event.
- Always emits exactly one terminal event (`EventCompleted` or `EventFailed`) before closing.
- Respects `ctx` cancellation; emits `EventFailed` on cancellation.
- Safe for concurrent use.

**`Event`** — structured record of what happened:
```go
type Event struct {
    ID        string
    RunID     string
    Kind      EventKind       // started | text | tool_use | tool_result | completed | failed
    Timestamp time.Time
    Seq       int             // monotonic within a run
    Data      json.RawMessage // kind-specific payload
}
```

**`Run`** — persistent execution record:
```go
type Run struct {
    ID, AgentID, Prompt string
    Status   RunStatus           // pending | running | completed | failed | cancelled
    // ... timestamps, cost, tokens
}
```

### Orchestrator (`internal/orchestration/`)

Coordinates a `Runner` with a `Store`:

1. Creates a run record (status=pending)
2. Calls `runner.Run(ctx, req)`
3. Streams events to caller via `Execution.Events` channel
4. Persists each event with `context.Background()` (immune to execution ctx cancellation)
5. Updates run status on terminal events
6. Always persists final run state (deferred with background ctx)

**Race safety**: `Execution.Run` is an immutable snapshot at the time `Start()` returns.
The goroutine works on a private `localRun` copy. Callers query the store for final state.

### Store (`internal/store/`)

```go
type Store interface {
    CreateRun(ctx, run)  error
    UpdateRun(ctx, run)  error
    GetRun(ctx, id)      (*Run, error)
    ListRuns(ctx, limit) ([]*Run, error)
    AppendEvent(ctx, e)  error
    ListEvents(ctx, id)  ([]*Event, error)
    Close() error
}
```

**Current implementation**: SQLite via `modernc.org/sqlite` (pure Go, no cgo).
WAL mode enabled. Events use `INSERT OR IGNORE` for idempotent appends.

### Runners

**`fake.Runner`** — deterministic, configurable script for testing and demos. The channel
buffer equals `len(script)+1` so terminal events can always be delivered even after
context cancellation.

**`claudecode.Runner`** — invokes `claude --print --output-format stream-json`.
Parses NDJSON from stdout; maps Claude message types to `agent.Event`:

| Claude event type | agent.EventKind |
|---|---|
| `system` (init) | ignored |
| `assistant` content `text` | `EventText` |
| `assistant` content `tool_use` | `EventToolUse` |
| `user` content `tool_result` | `EventToolResult` |
| `result` (success) | `EventCompleted` |
| `result` (error) / stderr / EOF | `EventFailed` |

Stderr is captured via `cmd.Stderr = &stderrBuf` (not a pipe) to avoid goroutine races;
it is read safely after `cmd.Wait()`.

## Data Flow

```
CLI command
  │
  ▼
orchestration.Orchestrator.Start(ctx, req)
  │
  ├─ store.CreateRun()     ← immediate, synchronous
  ├─ runner.Run(ctx, req)  ← starts goroutine, returns channel
  │
  ▼
goroutine:
  for event := range runnerCh:
    store.AppendEvent(background, event)  ← persist
    outCh <- event                        ← forward to caller
  defer: store.UpdateRun(background, finalRun)
  defer: close(outCh)
  │
  ▼
caller (CLI / TUI):
  for event := range exec.Events:
    render / display event
```

## Presentation (`internal/ui/`)

`styles.go` — Lipgloss palette (Catppuccin Mocha-inspired, adaptive light/dark).
`output.go` — `Printer` with human/JSONL modes; `FormatRun*`, `FormatEvent` helpers.
`live/model.go` — Bubble Tea model; reads `<-chan Event`, renders live execution view.

The TUI uses alt-screen mode. On exit, a compact summary is printed to the normal terminal.

Non-interactive terminals (`NO_COLOR`, `TERM=dumb`, pipe) receive JSONL events.

## Package Import Rules

```
agent      ← (no internal imports)
runners/*  ← agent
store/*    ← agent
orchestration ← agent, store
ui         ← agent
cmd        ← agent, runners, store, orchestration, ui
```

Application-layer packages (future) import orchestration, not runners directly.
