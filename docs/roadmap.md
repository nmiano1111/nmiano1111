# Roadmap

## Milestone 1: Single-Agent Execution Foundation ✓ COMPLETE

Implemented:
- CLI entry point (`agentctl`) with Cobra
- `agent.Runner` interface + `RunRequest`/`Event`/`Run` types
- Fake deterministic runner with configurable scripts
- Claude Code CLI adapter (stream-json → agent events)
- SQLite store via `modernc.org/sqlite` (pure Go)
- Orchestrator: lifecycle management, event persistence, race-free
- Lipgloss-styled output: tables, detail views, event stream
- Bubble Tea live execution view (alt-screen, spinner, scrolling events)
- JSONL machine-readable output
- Race-free concurrent implementation
- Full test suite: fake runner (5), SQLite store (8), orchestrator (7)

## Milestone 2: Event-Sourced Orchestrator + MCP Exposure

Goal: multi-step agent coordination via event-sourced triggers; composability via MCP.

Architecture: see `docs/architecture/proposal.md`.

### Acceptance criteria
- [ ] Run status is derived from the event log (`DeriveStatus(events)`), not from a mutable `status` flag in a hot path
- [ ] `looplab/fsm` enforces valid status transitions; invalid transitions return errors
- [ ] Events carry `parent_seq *int`; `tool_result` events link back to their `tool_use` origin
- [ ] SQLite schema managed by `goose`; first migration adds `parent_run_id` to `runs` and `parent_seq` to `events`
- [ ] MCP server (`agentctl mcp`) exposes `get_run`, `list_runs`, `get_events`, `start_run`
- [ ] MCP server is invokable from Claude desktop with a standard `mcpServers` config block
- [ ] Planner agent can delegate subtasks via `delegate_task` MCP tool; child runs appear in `agentctl runs list` tree
- [ ] `agentctl run --isolate` forks a git worktree before passing `WorkDir` to the runner
- [ ] `ui.DeriveDisplayStatus(run)` is the single source of display logic; TUI never branches on raw `RunStatus`
- [ ] TUI renders agent-specific output patterns (hardcoded for `claude-code`: Reading/Writing/Error prefixes)
- [ ] All new code has test coverage ≥80% (unit + integration)

### Implementation sequence

1. **Schema + migrations** — add `goose`, write migration 001 (`parent_run_id`, `parent_seq`).
2. **FSM** — add `internal/orchestration/fsm.go` with `looplab/fsm`; wire into `Orchestrator`.
3. **Event DAG** — add `ParentSeq *int` to `agent.Event`; update claudecode runner to populate from `tool_use_id`.
4. **Display status** — add `internal/ui/display.go`; update TUI model.
5. **MCP server** — add `internal/mcp/`; add `agentctl mcp` command.
6. **Child runs** — add `parent_run_id` field; add `delegate_task` tool handler; update `runs list` to render tree.
7. **Worktree isolation** — add `RunConfig.IsolateWorktree`; implement in orchestrator pre-start hook.
8. **Pattern matching** — add pattern registry to TUI; hardcode claude-code patterns.

### Dependencies to add
- `github.com/looplab/fsm` (Apache 2.0)
- `github.com/dominikbraun/graph` (Apache 2.0) — for subtask DAG rendering
- `github.com/pressly/goose/v3` (MIT) — schema migrations
- MCP Go SDK (TBD: `github.com/mark3labs/mcp-go` or `github.com/modelcontextprotocol/go-sdk`)

---

## Milestone 3: Durable Execution + Verification

Goal: survive process restart; verify agent output.

### Planned
- Process-restart recovery: on startup, detect runs stuck in `running` status; re-evaluate from event log
- Timeout enforcement independent of process lifetime (`RunConfig.TimeoutMS`)
- Task leasing to prevent duplicate execution (advisory lock via SQLite `BEGIN EXCLUSIVE`)
- Verifier port (`internal/verifier/verifier.go`): `Verify(ctx, run, changedFiles) VerificationResult`
- Built-in verifiers: `go test ./...`, `go vet ./...`, arbitrary shell command
- `EventVerification` event kind: emitted after run completion, before `EventCompleted`
- MCP read-only resource server: expose run events as MCP resources (not just tools)
- macOS sandbox-exec profile per run (`RunConfig.SandboxProfile`)
- Stronger cancellation: `agentctl cancel <run-id>` sends SIGTERM to runner subprocess

---

## Milestone 4: Multi-Agent Coding Harness

Goal: AI-assisted software development as a first-class workflow.

### Planned
- Repository analysis agent (reads codebase, produces structured summary)
- Implementation planning agent (decomposes task into subtasks with dependency graph)
- Code generation in isolated git worktrees (M2 foundation)
- Automated test execution (M3 verifier)
- Independent reviewer agent (reads diff, produces structured review)
- Repair loops (bounded attempt count: planner → impl → verify → repair → verify)
- Structured final report: diff, test results, reviewer comments, cost, duration
- `agentctl workflow run <spec.yaml>` command for declarative multi-agent pipelines

---

## Milestone 5: Advanced Orchestration

Goal: sophisticated multi-agent patterns and broader ecosystem integration.

### Planned
- A2A (Agent-to-Agent) protocol endpoint: expose runs as A2A tasks
- Warm-box session pool (Gas City fingerprint pattern): eliminate cold-start penalty for repeated configs
- Dynamic task decomposition: planner re-plans based on intermediate results
- Competing implementations (multiple agents, best-of-N selection by reviewer)
- Human-in-the-loop approval gates: pause workflow at named checkpoints, await CLI confirmation
- Distributed execution (multi-machine) via A2A delegation
- Native LLM agent runtime: direct Anthropic API calls without Claude Code subprocess
- Adaptive workflow planning: replanning if subtask fails

---

*Milestones are subject to revision based on implementation experience. The intended result of M2 is a defensible architecture running a real multi-step workflow, not a complete platform.*
