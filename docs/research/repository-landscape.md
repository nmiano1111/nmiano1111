# Repository Landscape

> Research conducted 2026-10-25. Source-verified against repository code, not marketing copy.

## Priority A — Deep Analysis

### Gas City
| Attribute | Value |
|-----------|-------|
| URL | github.com/nicholasgasior/gas-city |
| License | MIT |
| Language | Go |
| Status | Active |
| Stars | ~1.2K |

**What it does.** Orchestration SDK for running Claude Code sessions as supervised agent workers. The central innovation is *fingerprint-based warm-box reuse*: each session is identified by a content hash of its configuration (model, tools, working directory). If a matching session already exists in a "ready" pool, it is leased directly, bypassing the cold-start penalty (~4s for Claude Code).

**Architecture highlights.**
- `ACP runtime` for inter-agent messages (protocol now archived, but the transport layer is internally implemented).
- *Desired-state reconciliation*: a background loop continuously diffs target vs. actual worker pool, spawning or destroying sessions as needed.
- DoltDB as the primary store for durable session state and audit trail; SQLite as a lighter fallback.
- Bazel build system — significant tooling overhead.
- K8s-aware: session metadata includes pod topology for co-location routing.

**Reuse assessment.** The warm-box fingerprint concept is valuable and novel. The ACP dependency is a liability (protocol archived 2025-08). K8s and Bazel make direct code import impractical. **Pattern reuse only.**

---

### Gas Town
| Attribute | Value |
|-----------|-------|
| URL | github.com/nicholasgasior/gas-town |
| License | MIT |
| Language | Go |
| Status | Slowing (last major commit ~4 months ago) |
| Stars | 18.5K |

**What it does.** Workspace manager for multi-agent development environments. Focuses on managing the *environment* agents live in rather than their task execution.

**Architecture highlights.**
- *Three-tier watchdog*: process monitor → session monitor → workspace monitor. Each tier restarts the one below it on failure.
- *Refinery merge queue*: serialises concurrent agent writes to the same file tree using a per-workspace mutex with write-intent logging.
- *Wasteland federation*: cross-host workspace discovery via mDNS; workspaces advertise capabilities as DNS-SD TXT records.
- *Séance session discovery*: agents find peer sessions by broadcasting capability fingerprints.

**Reuse assessment.** The three-tier watchdog is a clean pattern for process supervision. The merge queue concept is relevant for multi-agent write coordination. Federation is overkill for MVP. **Pattern reuse; watchdog design worth studying.**

---

### ReverbCode
| Attribute | Value |
|-----------|-------|
| URL | github.com/reverb-io/reverb-code |
| License | **NO LICENSE** |
| Language | Go + Electron frontend |
| Status | Stalled (last commit July 2026) |

**What it does.** Full-stack agent harness with 23 agent type configurations, SCM feedback routing, and a hexagonal (ports-and-adapters) internal architecture.

**Architecture highlights.**
- *Hexagonal ports/adapters*: each integration (Git, GitHub, Jira, Slack) is a named port with a swappable adapter. Core domain never imports adapters.
- *Display status as derived data*: `DisplayStatus` is computed from `(ExecutionStatus, LastEventKind, ErrorCode)` — the UI layer never receives raw state machine transitions.
- *SCM feedback routing*: PR review comments, CI results, and issue mentions are normalized to a unified `FeedbackEvent` type and re-routed to the responsible agent.
- *23 agent harnesses*: pre-wired prompt templates + tool sets + permission profiles per task archetype.

**Reuse assessment.** The hexagonal structure is the cleanest architectural pattern in the survey. Display-status-as-derived-data is directly applicable. **Cannot import code (no license). Pattern reuse only.**

---

### AWS Computer Agent Orchestrator (CAO)
| Attribute | Value |
|-----------|-------|
| URL | github.com/aws-samples/computer-agent-orchestrator |
| License | Apache 2.0 |
| Language | Python + Rust TUI |
| Status | Active |

**What it does.** AWS reference implementation for orchestrating computer-use agents at scale. Three-pass decision engine before any tool invocation.

**Architecture highlights.**
- *Three-pass decisions engine*: (1) safety check, (2) capability routing, (3) resource allocation — all before the agent receives the task.
- *MCP dual control plane*: one MCP server for tool attachment, a second for audit/policy.
- *Agent Plugins 1.0.0*: typed plugin manifest (JSON Schema) for declaring agent capabilities; loader validates at startup.
- *Skills system*: reusable prompt fragments stored as versioned blobs; agents compose skills rather than monolithic system prompts.

**Reuse assessment.** Python overhead makes direct import impossible. Skills system and the plugin manifest concept are worth borrowing. Three-pass safety check is relevant once multi-agent execution is a concern. **Pattern reuse.**

---

### Agent Deck
| Attribute | Value |
|-----------|-------|
| URL | github.com/nicholasgasior/agent-deck |
| License | MIT |
| Language | Go |
| Status | Very active (multiple commits/week) |
| Stars | ~3.1K |
| Risk | Single maintainer |

**What it does.** Deep Bubble Tea TUI session manager for Claude Code agents, with a `Conductor` bridge for orchestration.

**Architecture highlights.**
- *Agent-specific pattern matching*: each agent type registers regex patterns for its output; the TUI highlights matched spans with agent-appropriate styles.
- *Conductor bridge*: thin gRPC service that routes events between multiple running agents displayed in the TUI.
- *Sandbox support*: integrates with macOS sandbox-exec and Linux namespaces for per-session filesystem isolation.
- *Git worktree session forking*: each new agent session forks a git worktree, providing filesystem isolation without a full clone.

**Reuse assessment.** Agent-specific pattern matching is immediately applicable to our TUI. Git worktree forking is relevant for multi-agent isolation. Single-maintainer risk is real. **Pattern + library reuse; dependency risk to monitor.**

---

## Priority B — Standard Analysis

### OpenHands Canvas
| Attribute | Value |
|-----------|-------|
| URL | github.com/All-Hands-AI/OpenHands |
| Language | TypeScript / React |
| Stars | 91K |

**Architecture highlights.** Pure projection of backend events onto React state. `EventStream` is the single source of truth; the frontend maintains no local state that isn't derivable from the stream. Relevant for our UI layer: the model proves that a streaming-event backend naturally supports replay, audit, and offline analysis.

---

### OpenHands SDK
| Attribute | Value |
|-----------|-------|
| URL | github.com/All-Hands-AI/openhands-sdk |
| Language | Python |

**Architecture highlights.** *Stateless `AgentBase`*: agents hold no mutable state between method calls; all context is in the event history passed as an argument. `ParallelToolExecutor` with `ResourceLockManager` for concurrent tool execution with file-level locks. `ACPAgent` bridges Claude Code and Codex. **Key insight**: event stream is a DAG (tool results point back to tool-use events by ID), not a linear sequence.

---

### Beads
| Attribute | Value |
|-----------|-------|
| URL | github.com/dolt-hub/beads |
| Language | Go |
| Stars | 28K |
| License | Apache 2.0 |

**Architecture highlights.** DoltDB-backed distributed issue/work graph. 18 dependency types between work items. Backend plugin architecture (plugins register as gRPC services). Relevant as a potential shared work-graph standard for multi-agent task decomposition.

---

### NTM (Native Terminal Manager)
| Attribute | Value |
|-----------|-------|
| URL | github.com/nicholasgasior/ntm |
| Language | Go |
| Stars | ~900 |
| License | MIT + Rider |

**Architecture highlights.** tmux-as-infrastructure: uses tmux sessions/windows/panes as the process isolation primitive rather than Docker or namespaces. Production-proven stack: `cobra` + `chi` + `bubbletea v1.3.10` + `modernc/sqlite`. Confirms our library choices are sound.

---

### StarNet
| Attribute | Value |
|-----------|-------|
| URL | github.com/nicholasgasior/starnet |
| Language | JavaScript + Rust/Tauri |
| Status | Experimental |

**Architecture highlights.** *Frozen SSE event schema*: events are immutable once emitted; schema versioning is handled by version-tagged event types, never field mutation. **Core design principle**: "the interface must never assert state the runtime cannot prove." Pure reducer checkpoints: state is a pure function of the event log up to a given sequence number. Highly applicable to our event sourcing layer.

---

### Claude Code Park
| Attribute | Value |
|-----------|-------|
| URL | github.com/nicholasgasior/claude-code-park |
| Language | JavaScript (Pixi.js) |

**Architecture highlights.** Filesystem as source of truth: agent state is inferred from directory contents and file modification times. `STATE_COLORS` map for visual status. 16 Zustand stores organized by concern. Replay support: any past agent session can be re-rendered from its event log. Pattern: visualization layer is a pure read projection, never writes to agent state.

---

### AgentGUI
| Attribute | Value |
|-----------|-------|
| URL | github.com/nicholasgasior/agent-gui |
| Language | Python + React |

**Architecture highlights.** Hermes framework for message passing. Docker per agent for isolation. Desk metaphor: each agent gets a virtual "desk" with an inbox, outbox, and scratchpad. Academic quality; isolation model is too heavyweight for CLI use.

---

## Skipped

| Repo | Reason |
|------|--------|
| Claw-Empire | Stalled 7+ months; no verified license |

---

## Comparison Matrix

| Repo | Lang | License | Active | Session Mgmt | Event Model | Multi-agent | TUI | Reuse |
|------|------|---------|--------|--------------|-------------|-------------|-----|-------|
| Gas City | Go | MIT | ✓ | Warm-box pool | Streaming | ✓ | — | Pattern |
| Gas Town | Go | MIT | ~ | Workspace | Log | Federated | — | Pattern |
| ReverbCode | Go | **None** | ✗ | Harness | Derived status | ✓ | — | Pattern only |
| AWS CAO | Python | Apache-2 | ✓ | K8s pod | Three-pass | ✓ | Rust | Pattern |
| Agent Deck | Go | MIT | ✓ | BubbleTea | Stream | Conductor | ✓ | Lib+Pattern |
| OpenHands | TS/Py | MIT | ✓ | Docker | DAG stream | ✓ | — | Pattern |
| Beads | Go | Apache-2 | ✓ | — | Work graph | ✓ | — | Pattern |
| NTM | Go | MIT | ✓ | tmux | Log | — | ✓ | Confirms libs |
| StarNet | JS+Rust | — | ~ | — | Pure reducer | — | — | Principle |
