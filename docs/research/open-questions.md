# Open Questions

> Unresolved design decisions, risks, and experiments needed before committing to specific implementations.

---

## Q1: What is the correct unit of persistence?

**Context.** Currently each `Run` is standalone: one prompt → one run record → many events. The `Run` concept is documented as "tasks and runs are separate; a task may have multiple runs (retries). Milestone 1 omits the Task concept."

**Question.** When should we introduce a `Task` layer above `Run`?

- A task groups multiple runs (initial attempt + retries, or planned parallelism).
- A task has its own lifecycle (`pending → in-progress → done`), separate from individual run lifecycle.
- Without tasks, retries are ad-hoc: the caller creates a new `Run` and manually correlates.

**Options.**
1. Add `Task` now, with `task_id` FK on `Run`. Tasks are created but initially have only one run.
2. Add `task_id` as an optional nullable field on `Run` — tasks are implicit groupings, not first-class records.
3. Keep single-run model through M2; add `Task` in M3 when parallelism requires it.

**Experiment needed.** Model a retry scenario: does the CLI user want `agentctl retry <run-id>` (which creates a new Run in the same "task") or `agentctl run --retry <run-id>` (which re-runs with the same prompt)? The answer drives whether `Task` needs to be a visible concept.

---

## Q2: How should the orchestrator handle long-running agent sessions?

**Context.** The current runner model is stateless: one `Run()` call → one process → one event stream. Claude Code processes are ephemeral.

**Question.** Some agent workflows require a long-lived session: the agent reads a codebase once, then answers many questions without re-reading. How do we support this?

**Options.**
1. **Warm-box pool** (Gas City pattern): hash `RunConfig`, maintain a ready pool, lease existing sessions. Requires a session daemon.
2. **Conversation continuation**: pass `--resume <session-id>` to Claude Code on subsequent runs. The runner manages session IDs; the orchestrator passes them as `RunConfig.SessionID`.
3. **Stateless + caching**: accept re-read cost; rely on Claude's context caching to amortize it. Simpler; no session lifecycle.

**Experiment needed.** Measure actual cold-start penalty for Claude Code with a large codebase. If <2s, stateless + caching is likely acceptable through M3. If >4s, warm-box becomes necessary earlier.

---

## Q3: Where is the trust boundary for tool execution?

**Context.** Claude Code with `--permission-mode acceptEdits` will execute arbitrary tool calls (file reads, writes, shell commands) without confirmation. The working directory and environment are the only blast radius limits.

**Question.** What isolation guarantees do we need to provide?

**Options.**
1. **None** (current): trust the working directory as the isolation boundary. Suitable for developer use on their own machines.
2. **Git worktree forking** (Agent Deck pattern): each run gets a throwaway git worktree. Changes are isolated; results must be explicitly merged.
3. **macOS sandbox-exec**: restrict filesystem and network access per-process. Zero external dependencies; macOS only.
4. **Linux namespaces / containers**: full isolation; significant complexity; CI-friendly.

**Decision needed.** Answer depends on whether this will run unattended (CI, automated pipelines) or interactively. For CI use, at minimum worktree forking is needed. Document the intended deployment model first.

---

## Q4: How should multi-agent coordination work?

**Context.** The current orchestrator runs one agent per call. The research brief anticipates multi-agent workflows where a planner decomposes work and dispatches to specialist agents.

**Question.** What is the coordination primitive?

**Options.**
1. **Direct invocation**: the planner agent calls `orch.Start()` directly. Simple; no protocol overhead; agents must run in the same process.
2. **MCP tool**: expose `start_agent_run` as an MCP tool. The planner agent calls it via tool use. Decoupled; works across process boundaries; requires MCP server implementation (~2 days).
3. **A2A protocol**: full agent-to-agent delegation with Agent Cards, task lifecycle, SSE streaming. Most capable; most implementation effort.
4. **Event-sourced triggers**: a "planner run" produces `EventPlan` events with subtask specs; the orchestrator reads these and spawns child runs automatically.

**Recommendation.** Start with option 4 (event-sourced triggers) as an intermediate step: it requires only a new event kind and an orchestrator loop, no protocol implementation. Migrate to MCP tool (option 2) when cross-process composition is needed.

---

## Q5: What does "verification" mean for agent output?

**Context.** The research brief asks about verification. ReverbCode has a `Verifier` port; AWS CAO has a three-pass decision engine.

**Question.** What specifically needs to be verified, and when?

**Sub-questions.**
- Syntactic: does the agent's output parse as expected (valid JSON, valid code)?
- Semantic: does the agent's code pass tests?
- Policy: does the agent's action comply with declared allowed tools and working directory?
- Completeness: did the agent actually do what was asked?

**Options.**
1. **Run tests after file changes**: detect when the agent writes files; run the project's test suite; report pass/fail as a `EventVerification` event.
2. **Lint/format**: run `go vet`, `golint`, etc. on changed files.
3. **Structural checks only**: verify that the run produced at least one `EventText` event (not just tool use), and that no `EventFailed` was emitted.

**Experiment needed.** Implement option 3 now (trivial); option 1 for M2; defer 2 and 4.

---

## Q6: Bubble Tea v2 migration timing?

**Context.** Bubble Tea v2 exists and has breaking API changes (`View() tea.View` instead of `View() string`; declarative alt-screen config; new key/mouse message types). Lip Gloss v2 must be used with BubbleTea v2.

**Question.** Should we migrate to BubbleTea v2 now (before the TUI is complex) or stay on v1.3.10?

**Arguments for staying on v1.**
- NTM is production-proven on v1.3.10 (released ~8 months ago).
- BubbleTea v2 may still have rough edges.
- Agent Deck, the most relevant TUI reference, uses v1.

**Arguments for migrating to v2.**
- Migrating a complex TUI is harder than migrating early.
- v2 declarative alt-screen avoids the `tea.WithAltScreen()` option which has known edge cases.

**Recommendation.** Stay on v1.3.10 through M2. Re-evaluate when Agent Deck or NTM migrates to v2 (a strong signal that v2 is stable).

---

## Q7: Database schema evolution strategy?

**Context.** The SQLite schema is currently created inline in `store.go` with `CREATE TABLE IF NOT EXISTS`. There is no migration system.

**Question.** When column additions are needed (e.g., `parent_seq`, `task_id`), how do we migrate existing databases?

**Options.**
1. **Atlas or goose**: standard migration tools. Add a `schema_migrations` table; run migrations at startup.
2. **Manual `ALTER TABLE`**: check for column existence; add if missing. Simple; fragile for complex changes.
3. **Version field in schema**: store schema version in a `meta` table; run version-specific migration functions at startup.

**Recommendation.** Use `goose` (Go-native, no binary dependency, SQLite support) starting with M2 when the first schema change is needed. For now, the current inline approach is acceptable.

---

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Claude Code CLI API changes break runner | Medium | High | Version-pin in docs; integration tests against real CLI |
| Agent Deck goes unmaintained (single maintainer) | Medium | Medium | Treat as pattern reference, not hard dependency |
| BubbleTea v1→v2 forced migration | Low | Medium | Stay on v1.3.10; monitor upstream |
| SQLite WAL corruption on unclean shutdown | Low | High | Enable `PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;` (already done) |
| ACP protocol references in Gas City become dead ends | High | Low | Already noted; we're using A2A instead |
| `modernc.org/sqlite` falls behind upstream SQLite | Low | Medium | Alternative: `mattn/go-sqlite3` with cgo |
