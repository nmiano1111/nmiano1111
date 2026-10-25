# Reuse Opportunities

> Prioritized inventory of libraries, tools, and protocols worth integrating. Sorted by expected near-term impact.

---

## Tier 1 — Already Using (Confirm & Lock Versions)

| Library | Version | Role | Notes |
|---------|---------|------|-------|
| `modernc.org/sqlite` | v1.61.0 | Persistent store | Pure Go, no cgo. WAL mode confirmed. Keep. |
| `github.com/charmbracelet/bubbletea` | v1.3.10 | TUI runtime | NTM production-proven at this version. Do **not** upgrade to v2 yet (breaking API). |
| `github.com/charmbracelet/lipgloss` | v1.1.0 | Terminal styling | Must match BubbleTea major version (v1). |
| `github.com/charmbracelet/bubbles` | v1.0.0 | TUI components (spinner) | Compatible with BubbleTea v1. |
| `github.com/spf13/cobra` | v1.10.2 | CLI framework | No issues. |
| `github.com/google/uuid` | v1.6.0 | Run ID generation | No issues. |
| `golang.org/x/term` | v0.47.0 | TTY detection | No issues. |

---

## Tier 2 — High Value, Add Soon

### `dominikbraun/graph`
- **Import**: `github.com/dominikbraun/graph`
- **License**: Apache 2.0
- **What**: Zero-dependency DAG library with topological sort, cycle detection, shortest path, and Graphviz/Mermaid rendering.
- **Use case**: Model task dependencies for multi-step agent plans. When an orchestrator decomposes a prompt into subtasks, the dependency graph ensures correct execution order and prevents cycles.
- **Size**: ~1,200 LOC, no transitive dependencies.
- **Add when**: Milestone 2 (multi-step plans).

### `looplab/fsm`
- **Import**: `github.com/looplab/fsm`
- **License**: Apache 2.0
- **What**: Go state machine with enter/leave/after callbacks, concurrent-safe, Mermaid/Graphviz visualization.
- **Use case**: Model `RunStatus` transitions formally. Prevent invalid transitions (e.g., `completed → running`) at compile time via the FSM definition rather than ad-hoc if-statements.
- **Size**: ~500 LOC.
- **Add when**: Milestone 2 or when the orchestrator's status logic becomes complex.

---

## Tier 3 — Situational

### `ThreeDotsLabs/watermill`
- **Import**: `github.com/ThreeDotsLabs/watermill`
- **License**: MIT
- **What**: Event bus with multiple backends (NATS, Kafka, SQL, in-memory). Has a `watermillsql` package for SQLite.
- **Use case**: Replace the current manual event fan-out with a proper pub/sub bus. Enables multiple consumers of the event stream (TUI, logger, webhook notifier) without coordination.
- **Tradeoff**: Adds ~15K LOC of dependencies. Overkill if we have ≤3 consumers.
- **Add when**: When we have more than one durable event consumer per run.

### `creack/pty`
- **Import**: `github.com/creack/pty`
- **License**: MIT
- **What**: PTY allocation for subprocess management.
- **Use case**: Run Claude Code in a PTY so it behaves as if it has a terminal (avoids some non-interactive mode differences). Also needed for tmux-as-infrastructure pattern (Gas Town).
- **Add when**: If non-interactive mode limitations become a problem.

### `go.opentelemetry.io/otel`
- **License**: Apache 2.0
- **What**: OpenTelemetry SDK for traces and metrics.
- **Use case**: Instrument orchestrator spans per run. Useful for identifying latency sources in multi-agent chains.
- **Add when**: Production deployment with observability requirements.

---

## Tier 4 — Protocols to Implement Against

### MCP (Model Context Protocol) — 2025-03-26 revision
- **Spec**: modelcontextprotocol.io
- **Transport**: stdio (subprocess) or Streamable HTTP (persistent)
- **What it provides**: Tool attachment, resource exposure, prompt templates, sampling (nested LLM calls).
- **What it does NOT provide**: Agent-to-agent communication, task lifecycle, Agent Cards.
- **Relevance**: We should expose our orchestrator's tools (run agent, get status, list runs) as an MCP server. This makes us composable with any MCP-aware host (Claude desktop, Cursor, etc.).
- **Implementation effort**: ~2 days for a read-only MCP server exposing run status and event streams.

### A2A (Agent-to-Agent Protocol) — Linux Foundation
- **Spec**: a2aprotocol.io (supersedes archived ACP)
- **Transport**: JSON-RPC 2.0 over HTTP
- **What it provides**: Agent Cards (capability declaration), task lifecycle (`submitted → working → completed`), streaming via SSE, push notifications.
- **Relevance**: When we run multiple agents that need to delegate to each other, A2A is the correct inter-agent protocol. Complementary to MCP (MCP = tools; A2A = agents).
- **Implementation effort**: ~1 week for a minimal A2A task endpoint.
- **Add when**: Milestone 3+ (multi-agent orchestration).

---

## Patterns Worth Copying (No Library)

| Pattern | Source | Description |
|---------|--------|-------------|
| Fingerprint-based warm-box | Gas City | SHA-256 hash of `RunConfig` → session pool key |
| Git worktree forking | Agent Deck | `git worktree add` per run for filesystem isolation |
| Agent-specific pattern matching | Agent Deck | Per-agent-type regex → Lipgloss style in TUI |
| `ParentSeq` on events | OpenHands SDK | Link `tool_result` events to their `tool_use` origin |
| Display status as derived data | ReverbCode | `ui.DeriveDisplayStatus(run)` — never branch on raw `RunStatus` in UI |

---

## Explicitly Not Recommended

| Library/Approach | Reason |
|-----------------|--------|
| DoltDB | Heavy dependency; SQLite is sufficient through M3 |
| Docker-per-agent isolation | Too heavyweight for CLI tool; git worktrees are sufficient |
| ACP (BeeAI) | Archived August 2025; use A2A instead |
| Bubble Tea v2 | Breaking API; v1.3.10 is production-stable and NTM-proven |
| Bazel | Build system complexity outweighs benefits for this project size |
