# Development Guide

## Prerequisites

- Go 1.21+ (tested with 1.27.0)
- Claude Code CLI (`claude`) for the real agent adapter
- No external database required (SQLite is embedded)

## Setup

```bash
git clone <repo>
cd nickbot
go build -o agentctl ./cmd/agentctl/
```

## Testing

```bash
# All tests with race detection (recommended)
go test -race ./...

# Specific package
go test -race -v ./internal/orchestration/...
go test -race -v ./internal/store/sqlite/...
go test -race -v ./internal/runners/fake/...

# Static analysis
go vet ./...
```

All tests are deterministic and use no external services. SQLite tests use
`t.TempDir()` for isolated databases.

## Running the CLI

```bash
# Build
go build -o agentctl ./cmd/agentctl/

# Default database: ~/.nickbot/nickbot.db
./agentctl run --runner fake "Analyze the test task"

# Custom database
./agentctl --db /tmp/test.db run --runner fake "Test"

# Machine-readable output
./agentctl --json run --runner fake "Test"
./agentctl --json runs | jq .

# Claude Code runner (requires being outside a Claude Code session)
CLAUDECODE= ./agentctl run "Write a haiku"
```

## Claude Code Integration Constraint

The `claude` CLI refuses to launch inside another Claude Code session (detected via
the `CLAUDECODE` environment variable). When developing inside Claude Code, use the
fake runner for all testing.

To test the real Claude Code runner, run from a plain terminal:
```bash
unset CLAUDECODE
./agentctl run "your prompt here"
```

## Adding a New Runner

1. Create `internal/runners/<name>/runner.go`
2. Implement `agent.Runner` interface:
   - `Name() string`
   - `Run(ctx, req) (<-chan Event, error)`
3. Runner contract:
   - First event must be `EventStarted`
   - Last event must be `EventCompleted` or `EventFailed`
   - Channel must always be closed
   - Must respect `ctx` cancellation
4. Add to runner selection in `cmd/agentctl/cmd/run.go`
5. Add tests

## Database Schema

```sql
CREATE TABLE runs (
    id            TEXT PRIMARY KEY,
    agent_id      TEXT,
    prompt        TEXT,
    status        TEXT,    -- pending|running|completed|failed|cancelled
    runner_name   TEXT,
    model         TEXT,
    created_at    DATETIME,
    started_at    DATETIME,
    completed_at  DATETIME,
    result        TEXT,
    error         TEXT,
    cost_usd      REAL,
    input_tokens  INTEGER,
    output_tokens INTEGER,
    duration_ms   INTEGER
);

CREATE TABLE events (
    id         TEXT PRIMARY KEY,
    run_id     TEXT REFERENCES runs(id),
    seq        INTEGER,
    kind       TEXT,      -- started|text|tool_use|tool_result|completed|failed
    timestamp  DATETIME,
    data       TEXT       -- JSON payload
);
```

## Architecture Reference

See [`docs/architecture.md`](architecture.md).
