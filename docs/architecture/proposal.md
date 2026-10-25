# Architecture Proposal

> Recommended architecture for the nickbot AI agent orchestration platform.
> Based on research across 13 reference codebases and survey of relevant protocols and libraries.

---

## Summary

Build Alternative B: an event-sourced orchestrator with MCP exposure, layered incrementally on top of the existing Milestone 1 foundation. The core is an append-only event log from which all state is derived. The orchestrator exposes a thin MCP server for composability. Multi-agent coordination is achieved through event-sourced triggers — no external protocol required until M3+.

---

## Guiding Principles

1. **Provable state** (StarNet). The interface never asserts state the runtime cannot prove. `RunStatus` is derived from the event log, not stored as a mutable flag in a hot path.

2. **Ports and adapters** (ReverbCode). Domain logic (`internal/orchestration`, `internal/agent`) never imports adapters (`runners/claudecode`, `store/sqlite`). Wiring happens in `cmd/`.

3. **Display status as derived data** (ReverbCode). The TUI receives `DisplayStatus` (a domain type), not raw `RunStatus`. A pure function `ui.DeriveDisplayStatus(run)` is the single source of display logic.

4. **Event DAG** (OpenHands SDK). Events carry a `ParentSeq *int` to link `tool_result` events to their originating `tool_use`. Enables replay, parallel execution, and audit without schema changes.

5. **Composable from the start** (MCP). Expose run operations as MCP tools so the orchestrator is usable from any MCP host on day one.

---

## Component Map

```
cmd/agentctl/
    main.go                   ← entry point
    cmd/root.go               ← global flags, store init
    cmd/run.go                ← agentctl run
    cmd/runs.go               ← agentctl runs list
    cmd/status.go             ← agentctl status <run-id>
    cmd/logs.go               ← agentctl logs <run-id>
    cmd/inspect.go            ← agentctl inspect <run-id>

internal/
    agent/
        types.go              ← Event, Run, RunRequest, RunConfig, EventKind
        runner.go             ← Runner interface
        helpers.go            ← MakeEvent, DecodeData

    orchestration/
        orchestrator.go       ← Orchestrator: Start, GetRun, ListRuns, ListEvents
        fsm.go                ← Run status FSM (M2)
        subtask.go            ← EventSubtask detection and child-run spawning (M2)

    store/
        store.go              ← Store interface + ErrNotFound
        sqlite/
            store.go          ← SQLite implementation
            migrations/       ← goose migration files (M2+)

    runners/
        fake/runner.go        ← Deterministic script-driven runner (tests)
        claudecode/runner.go  ← Claude Code CLI adapter

    ui/
        styles.go             ← Lipgloss palette + status badges
        output.go             ← Printer (JSONL + plain text)
        display.go            ← DeriveDisplayStatus (M2)
        live/model.go         ← Bubble Tea live execution model

    mcp/
        server.go             ← MCP server (M2)
        tools.go              ← Tool handlers: start_run, get_run, list_runs, get_events
```

---

## Data Model

### `runs` table (current + M2 additions)

```sql
CREATE TABLE runs (
    id           TEXT PRIMARY KEY,
    agent_id     TEXT NOT NULL,
    prompt       TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',
    runner_name  TEXT NOT NULL,
    model        TEXT,
    created_at   TEXT NOT NULL,
    started_at   TEXT,
    completed_at TEXT,
    result       TEXT,
    error        TEXT,
    cost_usd     REAL NOT NULL DEFAULT 0,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    duration_ms  INTEGER NOT NULL DEFAULT 0,
    -- M2 additions:
    parent_run_id TEXT REFERENCES runs(id),  -- for child runs
    task_id      TEXT                         -- future task grouping
);
```

### `events` table (current + M2 additions)

```sql
CREATE TABLE events (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id),
    kind       TEXT NOT NULL,
    seq        INTEGER NOT NULL,
    timestamp  TEXT NOT NULL,
    data       TEXT,             -- JSON
    -- M2 addition:
    parent_seq INTEGER,          -- links tool_result to tool_use
    UNIQUE(run_id, seq)
);
```

---

## Run Lifecycle

```
              ┌──────────┐
              │ pending  │ ◀── created by Orchestrator.Start()
              └────┬─────┘
                   │ EventStarted emitted by runner
                   ▼
              ┌──────────┐
              │ running  │ ◀── EventText, EventToolUse, EventToolResult
              └────┬─────┘
           ┌───────┴───────┐
           ▼               ▼
    ┌──────────┐     ┌──────────┐
    │completed │     │  failed  │
    └──────────┘     └──────────┘
           ↑               ↑
    EventCompleted   EventFailed
    (from runner)    (from runner or
                      orchestrator on
                      ctx cancel)
```

Valid FSM transitions (enforced by `looplab/fsm` in M2):

| From | To | Trigger |
|------|----|---------|
| pending | running | EventStarted |
| running | completed | EventCompleted |
| running | failed | EventFailed |
| running | cancelled | context cancellation |
| pending | failed | runner returns error from Start() |

Invalid transitions panic in tests; return a wrapped error in production.

---

## Event Processing Pipeline

```
Runner goroutine
    │
    │ agent.Event (buffered chan, cap 64)
    ▼
Orchestrator event loop
    ├── 1. Persist event to SQLite (always, with context.Background())
    ├── 2. Update localRun (FSM transition if status-changing event)
    ├── 3. Detect EventSubtask (M2) → spawn child Orchestrator.Start()
    └── 4. Fan-out to subscribers (non-blocking, drop if full)
            ├── outCh (returned to caller via Execution.Events)
            └── (future) notifier.Notify(event)
```

The orchestrator's event loop is a single goroutine. SQLite writes are synchronous and blocking; this is intentional — we trade throughput for simplicity. If SQLite becomes a bottleneck, introduce a write-ahead buffer.

---

## MCP Server (Milestone 2)

The MCP server runs as a stdio subprocess (standard MCP deployment for CLI tools). It exposes the following tools:

| Tool | Arguments | Returns |
|------|-----------|---------|
| `start_run` | `agent_id`, `prompt`, `model?`, `workdir?` | `run_id` |
| `get_run` | `run_id` | `Run` JSON |
| `list_runs` | `agent_id?`, `status?`, `limit?` | `Run[]` JSON |
| `get_events` | `run_id`, `after_seq?` | `Event[]` JSON |

The MCP server is a thin wrapper over `internal/orchestration`. It shares the same SQLite database as `agentctl`. It is invoked via:

```json
{
  "mcpServers": {
    "nickbot": {
      "command": "agentctl",
      "args": ["mcp"]
    }
  }
}
```

---

## Multi-Agent Coordination (Milestone 2)

Multi-agent coordination does not require a new protocol. The mechanism:

1. A planner agent (running under Claude Code) emits a structured tool call, e.g.:
   ```json
   {"name": "delegate_task", "input": {"prompt": "...", "agent_id": "reviewer"}}
   ```

2. The `delegate_task` tool is handled by an MCP server tool handler, which calls `Orchestrator.Start()` with the child request.

3. The child run's ID is returned as the tool result. The planner can poll `get_run` or stream `get_events` to follow the child's progress.

4. The child run's `parent_run_id` is set to the parent run's ID. `agentctl runs list` renders the parent-child tree.

This is effectively Alternative B's "event-sourced triggers" pattern, surfaced through MCP. It requires:
- `parent_run_id` FK on the `runs` table (M2 schema migration).
- `delegate_task` MCP tool handler in `internal/mcp/tools.go`.
- Planner agent's system prompt includes `delegate_task` in its allowed tools.

---

## TUI Architecture

The TUI is a Bubble Tea v1.3.10 model. Key design decisions:

**Separate display status from run status.**
```go
// internal/ui/display.go
type DisplayStatus int
const (
    DisplayPending DisplayStatus = iota
    DisplayRunning
    DisplayCompleted
    DisplayFailed
    DisplayCancelled
)

func DeriveDisplayStatus(run *agent.Run) DisplayStatus { ... }
```

The live model receives `DisplayStatus`; it never branches on `agent.RunStatus`.

**Agent-specific pattern matching.** (Milestone 2)
```go
var claudeCodePatterns = []outputPattern{
    {re: regexp.MustCompile(`^Reading `),  style: StyleDim},
    {re: regexp.MustCompile(`^Writing `),  style: StyleSuccess},
    {re: regexp.MustCompile(`^Error:`),    style: StyleError},
}
```

**Event stream rendering.** The TUI maintains a bounded ring buffer of the last N events (default: 100). Older events are scrollable but not in the hot render path.

---

## Security Model

**Current (Milestone 1).**
- Trust boundary: working directory.
- Permission mode: `acceptEdits` by default (no interactive confirmation for file reads/writes).
- Shell execution: allowed (Claude Code's `Bash` tool).
- Network: unrestricted.

**Milestone 2 additions.**
- `RunConfig.IsolateWorktree bool`: when true, orchestrator forks a `git worktree` before passing `WorkDir` to the runner. The worktree is removed on run completion.
- `RunConfig.AllowedTools` already enforced via `--allowedTools` CLI flag.

**Future (Milestone 3+).**
- macOS sandbox-exec profile per run.
- `RunConfig.NetworkPolicy`: `none | outbound | unrestricted`.

**Not planned.**
- Docker-per-agent: too heavyweight for CLI tooling.
- Multi-user access control: single-user tool.

---

## API Contracts

### `agent.Runner` interface (stable)

```go
type Runner interface {
    Run(ctx context.Context, req RunRequest) (<-chan Event, error)
    Name() string
}
```

**Contract:**
- `Run` returns a channel and nil error if the process started successfully.
- The channel is closed exactly once, after the terminal event (`EventCompleted` or `EventFailed`).
- `EventStarted` is the first event; a terminal event is always the last.
- The runner must not panic; all errors become `EventFailed` events.

### `store.Store` interface (stable)

```go
type Store interface {
    CreateRun(ctx context.Context, run *Run) error
    UpdateRun(ctx context.Context, run *Run) error
    GetRun(ctx context.Context, id string) (*Run, error)
    ListRuns(ctx context.Context, filter RunFilter) ([]*Run, error)
    AppendEvent(ctx context.Context, event *Event) error
    ListEvents(ctx context.Context, runID string) ([]*Event, error)
    Close() error
}
```

**Contract:**
- `GetRun` returns `ErrNotFound{Kind: "run", ID: id}` if not found.
- `AppendEvent` uses `INSERT OR IGNORE` — idempotent for the same `(run_id, seq)`.
- `UpdateRun` with `context.Background()` must always succeed (called from deferred cleanup).

---

## Dependency Graph

```
cmd/agentctl ──imports──▶ internal/orchestration
                                  │
                     ┌────────────┴────────────┐
                     ▼                         ▼
              internal/agent            internal/store
                 (types,                  (interface)
                  interface)                   │
                     ▲                    ┌────▼────┐
                     │                   │ sqlite  │
             ┌───────┴──────┐            └─────────┘
             │              │
    runners/claudecode   runners/fake
    runners/...

internal/ui ──imports──▶ internal/agent (types only)
internal/mcp ──imports──▶ internal/orchestration
```

**Strict rule:** No package in `internal/` may import `cmd/`. No package in `internal/orchestration` may import `internal/runners/*` directly — runners are injected via the `Runner` interface.

---

## Rejected Approaches

| Approach | Reason rejected |
|----------|----------------|
| gRPC daemon (Alternative C) | Operational complexity; not justified for single-user CLI tool |
| DoltDB as primary store | Heavy dependency; SQLite handles our scale through M3+ |
| ACP protocol | Archived August 2025; A2A is the successor |
| BubbleTea v2 | Not yet production-proven at this scale; Agent Deck still on v1 |
| Docker-per-agent isolation | Too heavyweight; git worktrees provide sufficient isolation |
| Watermill event bus | Adds 15K LOC; overkill for ≤3 event consumers |
