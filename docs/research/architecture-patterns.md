# Architecture Patterns

> Source-backed patterns drawn from repository research. Each pattern includes the originating codebase and the concrete mechanism.

---

## 1. Warm-Box Session Reuse (Gas City)

**Problem.** Every cold Claude Code start takes ~4 seconds. At scale, or with short-lived tasks, this dominates wall-clock time.

**Pattern.** Hash the session configuration (model, tools, working directory, system prompt) into a *fingerprint*. Maintain a ready pool keyed by fingerprint. Before spawning a new process, check the pool; lease an existing session if one matches.

**Mechanism in Gas City.**
```
type SessionFingerprint struct {
    Model      string
    ToolSet    []string // sorted
    WorkDir    string
    SystemHash [32]byte // SHA-256 of system prompt
}

func (p *Pool) Acquire(fp SessionFingerprint) (*Session, error) {
    p.mu.Lock()
    if s, ok := p.ready[fp]; ok {
        delete(p.ready, fp)
        p.mu.Unlock()
        return s, nil
    }
    // ... spawn new
}
```

**Applicability.** Directly applicable once we need throughput (>1 concurrent agent). For Milestone 1/2, defer. The fingerprint type is a clean pre-implementation of `RunConfig` hashing.

---

## 2. Hexagonal Ports/Adapters (ReverbCode)

**Problem.** Integration code bleeds into domain logic. Adding a new SCM provider requires touching orchestration.

**Pattern.** Define named *ports* as Go interfaces in the domain layer. Adapters (in separate packages) implement those interfaces; they are wired at startup by main, never imported by domain code.

**Mechanism in ReverbCode.**
```
// domain/ports/scm.go
type SCMPort interface {
    OpenPR(ctx context.Context, pr PRSpec) (PRID, error)
    PostComment(ctx context.Context, id PRID, body string) error
}

// adapters/github/adapter.go
type GitHubAdapter struct { client *github.Client }
func (a *GitHubAdapter) OpenPR(...) (PRID, error) { ... }

// cmd/main.go — wiring
var scm ports.SCMPort = &github.GitHubAdapter{...}
orch := orchestration.New(runner, store, scm)
```

**Applicability.** The `agent.Runner` interface already follows this pattern. Extend it: define a `store.Store` port (done), a future `notifier.Notifier` port (Slack, webhooks), and a `verifier.Verifier` port (test runner, linter). Never import adapter packages from `internal/orchestration`.

---

## 3. Display Status as Derived Data (ReverbCode)

**Problem.** The UI receives raw state machine transitions and must interpret them. Adding a new state means updating both the state machine and the UI.

**Pattern.** Compute `DisplayStatus` in the domain layer as a pure function of `(RunStatus, LastEventKind, ErrorCode)`. The UI receives a pre-classified value; it never branches on raw state.

**Mechanism in ReverbCode.**
```go
type DisplayStatus int

const (
    DisplayPending DisplayStatus = iota
    DisplayRunning
    DisplayWaitingForApproval
    DisplayCompleted
    DisplayFailed
    DisplayCancelled
)

func DeriveDisplayStatus(run Run) DisplayStatus {
    if run.Status == StatusFailed && run.ErrorCode == ErrPermissionDenied {
        return DisplayWaitingForApproval
    }
    // ...
}
```

**Applicability.** `internal/ui/styles.go` already has `EventKindDisplays`. Extend this: `ui.RunDisplayStatus(run agent.Run) DisplayStatus` should be the single source of display logic. The TUI model receives `DisplayStatus`, not `RunStatus`.

---

## 4. Three-Tier Watchdog (Gas Town)

**Problem.** Agent processes crash. A crashed agent that doesn't restart leaves tasks hanging indefinitely.

**Pattern.** Layer supervision: the workspace monitor watches the session monitor; the session monitor watches the process. Each tier is independently restartable. Tier N+1 restarts tier N with exponential backoff.

**Mechanism (conceptual from Gas Town).**
```
WorkspaceMonitor  ──restarts──▶  SessionMonitor  ──restarts──▶  ProcessMonitor
        ↑                                                               │
        └────────── restarts if both below fail ◀─────────────────────┘
```

**Applicability.** Relevant for Milestone 3+ (persistent agent daemons). For now, the orchestrator's `defer`-based cleanup provides a simpler single-tier recovery.

---

## 5. Event DAG, Not Linear Stream (OpenHands SDK)

**Problem.** Tool results reference specific tool-use events. A linear sequence makes this reference implicit (by position). Replay and parallel execution are harder.

**Pattern.** Events form a DAG. Each `tool_result` event carries the ID of the `tool_use` event it answers. The stream is topologically ordered, but clients can reconstruct the dependency graph.

**Mechanism.**
```go
// Our current model (linear):
type Event struct {
    Seq  int
    Kind EventKind
    Data json.RawMessage
}

// Extended (DAG-aware):
type Event struct {
    Seq       int
    Kind      EventKind
    ParentSeq *int  // nil for root events; set for tool_result → tool_use
    Data      json.RawMessage
}
```

**Applicability.** Add `ParentSeq *int` to `agent.Event`. Claude Code's stream-json already provides `tool_use_id` on tool results; the runner can wire this into `ParentSeq`. Enables future parallel tool execution without schema changes.

---

## 6. Provable State Principle (StarNet)

**Problem.** UI components display "loading…" or "running…" states that the runtime cannot guarantee are accurate (the process may have died).

**Pattern.** The interface must never assert state the runtime cannot prove. Display only what can be derived from persisted events. Distinguish *observed* state ("last event was started 3s ago") from *asserted* state ("currently running").

**Mechanism.** Status displayed in the TUI is computed from the event log, not from a flag:
```go
func DeriveStatus(events []Event) RunStatus {
    if len(events) == 0 {
        return RunStatusPending
    }
    last := events[len(events)-1]
    switch last.Kind {
    case EventCompleted: return RunStatusCompleted
    case EventFailed:    return RunStatusFailed
    // ...
    }
    return RunStatusRunning  // only if last event was started/text/tool_use
}
```

**Applicability.** The `runs list` command should derive status from the event log when the run record's status is `running` but no heartbeat has been received recently. Avoids displaying stale `running` for orphaned processes.

---

## 7. Pure Reducer Checkpoints (StarNet)

**Problem.** Replaying a long event log from the beginning is expensive.

**Pattern.** Periodically snapshot state as `state = reduce(events[:N])`. Replay from the most recent checkpoint + subsequent events. The checkpoint is a pure function of events; it can be recomputed and verified.

**Applicability.** Relevant for long-running sessions (>10K events). Not needed for MVP. The SQLite event store already has the prerequisite `seq` column for ordered replay.

---

## 8. Agent-Specific Pattern Matching (Agent Deck)

**Problem.** Different agent types produce different output formats. A generic text renderer treats everything as plain text.

**Pattern.** Each agent type registers a set of regex patterns with the renderer. Matched spans are highlighted with agent-specific styles. Patterns are loaded from a TOML/YAML config keyed by agent ID.

**Mechanism.**
```go
type AgentPattern struct {
    Pattern string
    Style   lipgloss.Style
}

var agentPatterns = map[string][]AgentPattern{
    "claude-code": {
        {Pattern: `\bReading\b.*`,  Style: StyleDim},
        {Pattern: `\bWriting\b.*`,  Style: StyleSuccess},
        {Pattern: `Error:.*`,       Style: StyleError},
    },
}
```

**Applicability.** Add to `internal/ui/live/model.go`. Start with hardcoded patterns for `claude-code`; make them configurable later.

---

## 9. Git Worktree Session Forking (Agent Deck)

**Problem.** Multiple concurrent agents writing to the same working directory corrupt each other's work.

**Pattern.** Each agent session forks a `git worktree` from the main repository. Agents work in isolation; results are merged back via PR or cherry-pick.

**Mechanism.**
```bash
git worktree add /tmp/agent-sessions/<run-id> HEAD
# agent runs in /tmp/agent-sessions/<run-id>
git -C /tmp/agent-sessions/<run-id> diff HEAD > patch.diff
git worktree remove /tmp/agent-sessions/<run-id>
```

**Applicability.** Implement as a `RunConfig.IsolateWorktree bool` flag. When true, orchestrator forks a worktree before passing `WorkDir` to the runner. The worktree path becomes the effective `WorkDir`.

---

## Pattern Applicability Summary

| Pattern | MVP | M2 | M3+ |
|---------|-----|----|-----|
| Hexagonal ports/adapters | ✓ (extend existing) | | |
| Display status as derived data | ✓ | | |
| Event DAG (ParentSeq) | ✓ (schema only) | ✓ (use) | |
| Provable state principle | ✓ | | |
| Agent-specific pattern matching | | ✓ | |
| Git worktree forking | | ✓ | |
| Warm-box session reuse | | | ✓ |
| Three-tier watchdog | | | ✓ |
| Pure reducer checkpoints | | | ✓ |
