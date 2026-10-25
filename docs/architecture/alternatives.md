# Architecture Alternatives

> Three credible architectural directions for the orchestration platform. Each is a coherent, complete approach — not just a variation on one theme.

---

## Alternative A: Thin CLI Wrapper (Minimal)

**Core idea.** `agentctl` is a well-structured CLI around the Claude Code CLI. Orchestration is limited to run tracking, event persistence, and TUI rendering. The system does not attempt to schedule, supervise, or coordinate agents beyond single-run execution.

### Component diagram

```
┌─────────────────────────────────────────────┐
│                   agentctl                  │
│                                             │
│  ┌──────────┐   ┌─────────────┐   ┌──────┐ │
│  │ Cobra CLI│──▶│ Orchestrator│──▶│ TUI  │ │
│  └──────────┘   └──────┬──────┘   └──────┘ │
│                        │                    │
│                   ┌────▼─────┐              │
│                   │  SQLite  │              │
│                   └──────────┘              │
└─────────────────────────────────────────────┘
                        │
                   ┌────▼─────┐
                   │ claude   │  (subprocess)
                   └──────────┘
```

### Key decisions
- No inter-agent communication; each run is isolated.
- `RunConfig.WorkDir` is the only isolation boundary.
- No task graph, no parallelism, no verification pipeline.
- SQLite as the only store; no migration system until forced.

### Strengths
- Simplest to implement and reason about.
- Zero protocol overhead.
- Easiest to test (fake runner already covers all paths).
- Fast to reach a working, shippable state.

### Weaknesses
- Cannot orchestrate multi-agent workflows.
- No composition with other tools (no MCP server, no A2A endpoint).
- Verification requires manual post-hoc inspection.
- Long-running workflows require external orchestration.

### When to choose
Choose Alternative A if the primary use case is solo developer tooling: a single engineer running one agent at a time, reviewing results, and iterating manually. This is the current Milestone 1 state extended with polish.

---

## Alternative B: Event-Sourced Orchestrator with MCP Exposure (Recommended)

**Core idea.** The orchestrator becomes the source of truth via an append-only event log. Agents consume and produce events. A thin MCP server exposes run operations as tools, making the orchestrator composable with any MCP host. Multi-agent coordination happens through event-sourced triggers: a planner run produces `EventSubtask` events; the orchestrator detects them and spawns child runs.

### Component diagram

```
┌──────────────────────────────────────────────────────────────┐
│                          nickbot                             │
│                                                              │
│  ┌──────────┐   ┌─────────────────────────────────────┐    │
│  │ Cobra CLI│──▶│           Orchestrator               │    │
│  └──────────┘   │                                     │    │
│                 │  ┌──────────┐  ┌────────────────┐  │    │
│  ┌──────────┐   │  │ Run FSM  │  │ Event Dispatcher│  │    │
│  │ MCP      │──▶│  └──────────┘  └────────────────┘  │    │
│  │ Server   │   │       │                │            │    │
│  └──────────┘   │  ┌────▼─────┐   ┌─────▼──────┐    │    │
│                 │  │  SQLite  │   │  Fan-out   │    │    │
│                 │  │  Store   │   │  (TUI, log)│    │    │
│                 │  └──────────┘   └────────────┘    │    │
│                 └─────────────────────────────────────┘    │
│                              │                              │
│                    ┌─────────▼──────────┐                  │
│                    │  Runner Pool       │                  │
│                    │  ┌────────────┐    │                  │
│                    │  │ ClaudeCode │    │                  │
│                    │  │ Runner     │──▶ claude (sub)       │
│                    │  └────────────┘    │                  │
│                    │  ┌────────────┐    │                  │
│                    │  │ Fake Runner│    │                  │
│                    │  └────────────┘    │                  │
│                    └────────────────────┘                  │
└──────────────────────────────────────────────────────────────┘
```

### Key decisions

**Event log as primary store.** Every state transition is an event; run status is derived from the event log (`DeriveStatus(events)`), not stored as a mutable flag. This implements the StarNet "provable state" principle.

**FSM for run status.** `looplab/fsm` enforces valid transitions. Invalid transitions (e.g., `completed → running`) panic in tests, fail gracefully in production.

**Event fan-out.** The orchestrator's event stream is published to multiple subscribers: the TUI, the structured log, and (later) webhook notifiers. Each subscriber maintains its own cursor into the event log.

**`EventSubtask` for multi-agent coordination.** When an agent's output includes a structured subtask spec (detected via regex or a dedicated tool call), the orchestrator creates a child run. The child run's ID is linked to the parent via a `parent_run_id` FK. No external protocol needed.

**MCP server.** A stdio MCP server exposes `start_run`, `get_run`, `list_runs`, `get_events`. This makes the orchestrator callable from Claude desktop, Cursor, or any MCP host. Implementation: ~300 LOC; the MCP Go SDK provides the transport.

**Schema evolution.** Add `goose` for migrations when the first schema change is required (next: `parent_run_id` on `runs`, `parent_seq` on `events`).

### Data flow

```
User prompt
    │
    ▼
agentctl run "..."
    │
    ▼
Orchestrator.Start()
    ├── create Run record (pending)
    ├── start Runner goroutine
    │       │
    │       ├── emit EventStarted → persist → fan-out
    │       ├── emit EventText    → persist → fan-out
    │       ├── emit EventToolUse → persist → fan-out
    │       │       │
    │       │       └── detect EventSubtask?
    │       │               └── Orchestrator.Start(childReq)
    │       ├── emit EventToolResult → persist → fan-out
    │       └── emit EventCompleted → persist → fan-out
    │               └── update Run record (completed)
    │
    └── fan-out subscribers:
            ├── TUI model (live rendering)
            ├── Structured logger
            └── (future) Webhook notifier
```

### Strengths
- Composable from day one (MCP server unlocks a large ecosystem).
- Multi-agent coordination without a new protocol (event-sourced triggers).
- Audit log is a natural by-product (every event is persisted).
- Provable state: display is always derived from events, never from mutable flags.
- FSM prevents invalid state transitions at compile time.

### Weaknesses
- More complex than Alternative A (~3x implementation effort for the orchestrator layer).
- MCP server adds a dependency (`github.com/mark3labs/mcp-go` or equivalent).
- `EventSubtask` detection requires a structured output convention from the agent.
- Fan-out adds coordination overhead vs. single-consumer model.

### When to choose
Choose Alternative B if the goal is a platform that supports multi-agent workflows and integrates with the broader Claude ecosystem. This is the recommended path for the stated goals.

---

## Alternative C: Distributed Agent Daemon (Ambitious)

**Core idea.** Each agent type is a long-lived daemon process. The orchestrator is a gRPC server that dispatches tasks to daemons. Daemons maintain warm sessions (Gas City warm-box pattern) and report results via bidirectional streaming RPC.

### Component diagram

```
┌──────────────────────────────────────┐
│         Orchestrator (gRPC server)   │
│                                      │
│  ┌─────────────┐   ┌──────────────┐ │
│  │ Task Router │   │ Session Pool │ │
│  └─────────────┘   └──────────────┘ │
│         │                │          │
└─────────┼────────────────┼──────────┘
          │                │
    ┌─────▼────┐     ┌─────▼────┐
    │  Agent   │     │  Agent   │
    │  Daemon  │     │  Daemon  │
    │  (claude)│     │  (fake)  │
    └──────────┘     └──────────┘
          │
     ┌────▼────┐
     │  claude │ (warm session, kept alive)
     └─────────┘
```

### Key decisions
- gRPC for daemon ↔ orchestrator communication (bidirectional streaming).
- Agent daemons implement `AgentService` (proto-defined RPC interface).
- Session pool per daemon: fingerprint-based warm-box reuse (Gas City).
- SQLite per daemon for local event buffering; Postgres or DoltDB for shared state (multi-host).
- A2A protocol for daemon-to-daemon delegation.

### Strengths
- Maximum throughput (warm sessions, no cold-start penalty).
- True process isolation (each daemon is a separate process/container).
- Horizontally scalable (add daemon instances).
- A2A compliance from the start.

### Weaknesses
- 5-10x implementation effort vs. Alternative A.
- gRPC adds proto compilation step and binary dependency.
- Multi-host deployment requires service discovery (mDNS, Consul, K8s).
- Warm sessions complicate the trust model (a session might be re-used across different users' runs).
- Significant operational overhead (process supervision, health checks, graceful shutdown).

### When to choose
Choose Alternative C if the deployment target is a shared team server or CI infrastructure handling >10 concurrent agent runs. Not appropriate for developer tooling or MVP.

---

## Recommendation

**Implement Alternative B**, beginning with the components needed for Alternative A and layering B's additions in sequence.

### Implementation sequence

| Step | Component | Milestone |
|------|-----------|-----------|
| 1 | Current Milestone 1 state (already done) | M1 ✓ |
| 2 | `looplab/fsm` for run status | M2 |
| 3 | `parent_seq` on events + `goose` migrations | M2 |
| 4 | `EventSubtask` detection + child run spawning | M2 |
| 5 | MCP server (read-only: `get_run`, `list_runs`, `get_events`) | M2 |
| 6 | MCP server (write: `start_run`) | M3 |
| 7 | Git worktree forking for isolation | M3 |
| 8 | A2A endpoint | M4 |

The key insight from the research: the current Milestone 1 architecture is already a correct foundation for Alternative B. The event log, `agent.Runner` interface, and SQLite store are exactly right. The additions are layered on top without restructuring the core.
