# nickbot — AI Agent Orchestration Platform

A Go-first platform for coordinating AI agents, managing execution state, and building
sophisticated multi-agent workflows. The first application is an AI-assisted software
engineering harness.

## Status

**Milestone 1 complete**: single-agent execution foundation with persistent history and
a polished CLI/TUI.

## Quick Start

```bash
# Build
go build -o agentctl ./cmd/agentctl/

# Run a task (fake runner — no credentials required)
./agentctl run --runner fake "Analyze the codebase"

# Run with real Claude Code (outside a Claude Code session)
./agentctl run "Write a haiku about distributed systems"

# Inspect results
./agentctl runs
./agentctl status <run-id>
./agentctl logs <run-id>
./agentctl inspect <run-id>

# Machine-readable output (JSONL)
./agentctl run --runner fake "test" --json
./agentctl runs --json
```

## Architecture

Three conceptual layers with strict separation:

```
Orchestration Layer    task/workflow coordination, scheduling, persistence
Execution Layer        agent invocation, event streaming, provider adapters
Application Layer      domain-specific workflows (coding harness, etc.)
```

See [`docs/architecture.md`](docs/architecture.md) for detail.

## Requirements

- Go 1.21+
- No external databases required (SQLite embedded)
- Claude Code CLI (`claude`) for the real agent adapter

## Development

```bash
go test -race ./...   # run all tests
go vet ./...          # static analysis
```

See [`docs/development.md`](docs/development.md) for full setup instructions.
