# CLAUDE.md — Engineering Guidance for Claude Code Sessions

This file contains persistent guidance for any Claude Code session working on this project.
Read it at the start of every session before making changes.

## Project Summary

`nickbot` is a Go-first AI agent orchestration platform. The first application is
an AI-assisted software engineering harness. The platform must be general-purpose
and not coupled to any single AI provider or domain.

## Repository Structure

```
cmd/
  agentctl/           # CLI binary
    main.go           # entry point
    cmd/              # Cobra command tree
      root.go         # global flags, store opening
      run.go          # agentctl run
      runs.go         # agentctl runs
      status.go       # agentctl status
      logs.go         # agentctl logs
      inspect.go      # agentctl inspect

internal/
  agent/              # Core interfaces and domain types
    types.go          # Event, Run, RunRequest, RunConfig (all shared types)
    runner.go         # Runner interface
    helpers.go        # MakeEvent, Event.DecodeData

  runners/
    fake/             # Deterministic fake runner (testing and demos)
    claudecode/       # Claude Code CLI adapter

  store/
    store.go          # Store interface
    sqlite/           # SQLite implementation (pure Go, no cgo)

  orchestration/
    orchestrator.go   # Coordinates runner + store

  ui/
    styles.go         # Lipgloss palette and shared styles
    output.go         # Printer, run/event formatters
    live/
      model.go        # Bubble Tea model for live execution view

docs/
  architecture.md
  roadmap.md
  development.md
  decisions/
```

## Architecture Rules

1. **No imports upward**: `agent` ← `runners/*`, `store/*`, `orchestration`, `ui`, `cmd`
2. **No domain leakage**: orchestration packages must not import Git, test frameworks, etc.
3. **Runner contract**: every Runner must emit EventStarted first, then exactly one terminal
   event (EventCompleted or EventFailed) before closing the channel.
4. **Store persistence**: always use `context.Background()` for final run state persistence
   in the orchestrator goroutine — the execution context may already be cancelled.
5. **No shared mutable state**: the `Execution.Run` field is an immutable snapshot; the
   goroutine works on a private copy to avoid data races.
6. **Presentation isolation**: orchestration and execution code must never import `ui`.

## Key Interfaces

```go
// agent.Runner
type Runner interface {
    Run(ctx context.Context, req RunRequest) (<-chan Event, error)
    Name() string
}

// store.Store
type Store interface {
    CreateRun(ctx, run) error
    UpdateRun(ctx, run) error
    GetRun(ctx, id) (*Run, error)
    ListRuns(ctx, limit) ([]*Run, error)
    AppendEvent(ctx, event) error
    ListEvents(ctx, runID) ([]*Event, error)
    Close() error
}
```

## Development Commands

```bash
# Run all tests with race detection
go test -race ./...

# Build the CLI
go build -o agentctl ./cmd/agentctl/

# Run a task with the fake runner
./agentctl run --runner fake "Your task here"

# Run with real Claude Code (requires unset CLAUDECODE env var)
CLAUDECODE= ./agentctl run "Your task here"

# List runs
./agentctl runs

# Show run details
./agentctl status <run-id>

# Show run events
./agentctl logs <run-id>

# Inspect a run (full event content)
./agentctl inspect <run-id>
```

## Current Status (Milestone 1 complete)

Implemented:
- Core `agent.Runner` interface and event model
- Fake deterministic runner with configurable scripts
- Claude Code CLI adapter (stream-json parsing)
- SQLite persistence via `modernc.org/sqlite` (pure Go)
- Orchestrator with concurrent-safe run lifecycle
- Cobra CLI with `run`, `runs`, `status`, `logs`, `inspect` commands
- Lipgloss-styled human output + JSONL machine output
- Bubble Tea live execution view with spinner
- Race-free implementation (verified with `-race`)

Limitations:
- Claude Code runner cannot be used inside a Claude Code session (CLAUDECODE env var guard).
  Unset `CLAUDECODE` before invoking the binary from a shell outside Claude Code.
- No multi-agent workflows yet (Milestone 2).
- No task retry logic yet.
- No workspace isolation yet.

## Next Milestone

Milestone 2: Multi-Agent Workflow Execution
- Task dependency graphs
- Sequential and parallel task scheduling
- Retry policies
- Additional runner adapters

## Important Decisions

- **SQLite over Postgres**: Sufficient for single-machine Milestone 1; interface is storage-agnostic.
- **Pure Go SQLite** (`modernc.org/sqlite`): No cgo, simpler cross-compilation.
- **Immutable Execution.Run snapshot**: Goroutine works on a private copy to avoid data races.
- **Background context for persistence**: Execution context cancellation must not prevent run state from being saved.
- **Buffered event channel**: Sized to the script length + 1 in fake runner to allow terminal events after cancellation.
