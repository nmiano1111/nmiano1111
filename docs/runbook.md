# Runbook — Common Agent Task Patterns

Practical copy-paste examples for recurring engineering tasks. Every recipe is a real `agentctl run` invocation with notes on when to use it and what to watch for.

---

## Setup

**1. Build the binary** (once, from the repo root):

```bash
go build -o agentctl ./cmd/agentctl/
```

Optionally put it on your PATH so you don't need `./` everywhere:

```bash
mv agentctl /usr/local/bin/agentctl
```

**2. Run from your project directory**

All the examples below assume you `cd` into the repository you want the agent to work on before running `agentctl`. The agent inherits the current working directory as its workspace.

```bash
cd ~/your-project
agentctl run "..."
```

Use `--workdir` to point at a subdirectory without changing your shell's working directory:

```bash
agentctl run --workdir ./backend "..."
```

**3. Real runs vs. dry runs**

| Situation | Command |
|-----------|---------|
| Try a recipe without spending tokens | add `--runner fake` |
| Real Claude run from a normal terminal | `agentctl run "..."` |
| Real Claude run from **inside a Claude Code session** | `env -u CLAUDECODE agentctl run "..."` |

The `CLAUDECODE` guard exists because Claude Code sets that variable in its environment; the runner refuses to nest by default. Unsetting it opts out of the guard.

**4. Multi-line shell scripts**

Some recipes below are full bash scripts (they start with `#!/usr/bin/env bash`). Save them as `.sh` files and run them:

```bash
# save the script
vim ~/scripts/run-tests.sh   # paste the recipe

# make it executable
chmod +x ~/scripts/run-tests.sh

# run it from your project
cd ~/your-project
~/scripts/run-tests.sh
```

---

## Table of contents

1. [Codebase exploration](#1-codebase-exploration)
2. [Test generation and repair](#2-test-generation-and-repair)
3. [Safe refactoring with worktree isolation](#3-safe-refactoring-with-worktree-isolation)
4. [Verified changes](#4-verified-changes)
5. [Long-running tasks with timeouts](#5-long-running-tasks-with-timeouts)
6. [Batch runs and scripting](#6-batch-runs-and-scripting)
7. [Observability — inspecting what happened](#7-observability--inspecting-what-happened)
8. [Dry-running with the fake runner](#8-dry-running-with-the-fake-runner)
9. [MCP integration](#9-mcp-integration)

---

## 1. Codebase exploration

Good for onboarding, pre-PR review, or understanding an unfamiliar module.

### Summarise a repo

```bash
agentctl run "Give me a concise overview of this codebase: what it does, how it's structured, and any notable patterns or potential issues."
```

### Explain a specific package

```bash
agentctl run --workdir ./internal/orchestration \
  "Explain what this package does and how its components interact. Include any edge cases or assumptions that are baked in."
```

### Find hidden complexity

```bash
agentctl run "Identify the five most complex or risky parts of this codebase. For each one explain why it's complex and what could go wrong."
```

### Dependency audit

```bash
agentctl run "List all external dependencies in go.mod / package.json. Flag any that are: (1) unmaintained, (2) have known CVEs, or (3) could be replaced with a smaller alternative."
```

---

## 2. Test generation and repair

Always pair test tasks with `--verify` so the run is only considered successful if the new tests actually pass.

### Generate tests for a package

```bash
agentctl run \
  --verify "go test ./internal/store/..." \
  "Write comprehensive unit tests for internal/store/sqlite/store.go. Cover happy paths, error paths, and edge cases. Do not modify the implementation."
```

### Fix failing tests

```bash
agentctl run \
  --verify "go test -race ./..." \
  "The following tests are failing: $(go test ./... 2>&1 | grep FAIL). Diagnose each failure and fix the root cause. Do not change the test assertions — fix the implementation."
```

### Raise coverage on a specific file

```bash
agentctl run \
  --verify "go test -coverprofile=/tmp/cov.out ./internal/verifier/... && go tool cover -func=/tmp/cov.out | grep verifier" \
  "Add tests for internal/verifier/shell.go until line coverage is above 90%."
```

### Add property-based / table-driven tests

```bash
agentctl run \
  --verify "go test ./internal/agent/..." \
  "Convert the existing unit tests in internal/agent/ to table-driven tests. Add at least three additional table cases per test function that weren't covered before."
```

---

## 3. Safe refactoring with worktree isolation

Use `--isolate` whenever a task might leave the working tree in a broken state. The agent gets a fresh git worktree; your branch is untouched until you decide to merge.

### Structural refactor

```bash
agentctl run \
  --isolate \
  --verify "go build ./... && go test -race ./..." \
  "Refactor internal/orchestration/orchestrator.go to separate the worktree management into its own file internal/orchestration/worktree.go. All existing tests must continue to pass."
```

### Rename a type across the codebase

```bash
agentctl run \
  --isolate \
  --verify "go build ./..." \
  "Rename agent.RunRequest to agent.TaskRequest everywhere in the codebase — types, function signatures, tests, and comments."
```

### Risky dependency upgrade

```bash
agentctl run \
  --isolate \
  --verify "go build ./... && go test ./..." \
  "Upgrade modernc.org/sqlite to the latest version. Fix any API breakage that results."
```

### Extract a helper package

```bash
agentctl run \
  --isolate \
  --verify "go build ./... && go vet ./..." \
  "The functions ShortID and HumanDuration in internal/ui/output.go are used in multiple packages. Move them to a new package internal/format so they can be imported without pulling in the UI dependency tree."
```

---

## 4. Verified changes

`--verify` runs a shell command after the agent finishes. If it exits non-zero the run is marked failed and the verification output is captured as an event. Combine it with anything — builds, tests, linters, custom scripts.

### Must compile

```bash
agentctl run --verify "go build ./..." \
  "Add a Config struct to internal/api/server.go with fields for Port, Host, and ReadTimeout. Wire it into the existing New() constructor."
```

### Must compile and pass tests

```bash
agentctl run --verify "go build ./... && go test -race ./..." \
  "Add request-level logging middleware to internal/api/server.go that records method, path, status, and latency for every request."
```

### Must pass linter

```bash
agentctl run --verify "golangci-lint run ./..." \
  "Fix all golangci-lint warnings in internal/mcp/. Do not suppress warnings with nolint directives — fix the underlying issues."
```

### Database migration must be reversible

```bash
agentctl run \
  --verify "goose -dir migrations sqlite3 /tmp/test.db up && goose -dir migrations sqlite3 /tmp/test.db down && goose -dir migrations sqlite3 /tmp/test.db up" \
  "Write a goose migration that adds an index on the events table for (run_id, seq)."
```

### Frontend must build

```bash
agentctl run \
  --workdir ./web \
  --verify "npm run build" \
  "Add a keyboard shortcut (cmd+k / ctrl+k) that focuses the search box on the Runs page."
```

---

## 5. Long-running tasks with timeouts

Set `--timeout` (in seconds) for tasks that should have a hard deadline. The run is cancelled and marked failed if it runs over.

### Time-boxed investigation

```bash
# 5-minute budget to find a bug
agentctl run \
  --timeout 300 \
  "Investigate why TestOrchestrator_Cancellation is flaky. Look at the test, the orchestrator, and the fake runner. Propose a fix but do not implement it yet."
```

### Large codebase documentation pass

```bash
# 30-minute cap
agentctl run \
  --timeout 1800 \
  --verify "go build ./..." \
  "Add package-level doc comments to every package in internal/ that is currently missing one. Follow the Go doc comment conventions."
```

### Cancel a live run manually

```bash
# Start a run and note the ID
agentctl run --timeout 0 "Generate exhaustive documentation for the entire repo" &

# In another terminal, cancel it by ID
agentctl cancel <run-id>
```

---

## 6. Batch runs and scripting

### Sequential pipeline — output of each run informs the next

```bash
#!/usr/bin/env bash
set -e

# Step 1: analysis
ANALYSIS_ID=$(agentctl --json run \
  "Analyse internal/store/sqlite/store.go and list every SQL query that is missing an index. Output JSON: [{\"table\": ..., \"columns\": ...}]" \
  | jq -r 'select(.kind=="completed") | .run_id' | head -1)

echo "Analysis run: $ANALYSIS_ID"
agentctl status "$ANALYSIS_ID"

# Step 2: implementation based on the analysis result
RESULT=$(agentctl logs --json "$ANALYSIS_ID" \
  | jq -r 'select(.kind=="completed") | .data.result')

agentctl run \
  --verify "go build ./..." \
  "Add the following missing indexes to the SQLite schema in internal/store/sqlite/store.go: $RESULT"
```

### Run over a list of packages

```bash
#!/usr/bin/env bash
PACKAGES=(agent orchestration store/sqlite mcp verifier)

for pkg in "${PACKAGES[@]}"; do
  echo "=== $pkg ==="
  agentctl run \
    --agent "doc-writer" \
    --verify "go build ./internal/${pkg}/..." \
    "Add godoc comments to all exported symbols in internal/${pkg}/. Follow standard Go doc style."
done
```

### Parallel runs with `&` and `wait`

```bash
#!/usr/bin/env bash
agentctl run --verify "go test ./internal/mcp/..." \
  "Write tests for the resources/read MCP endpoint covering: bad URI, not-found, and happy path." &
PID1=$!

agentctl run --verify "go test ./internal/verifier/..." \
  "Write tests for the Shell verifier covering: timeout, non-zero exit, stderr capture." &
PID2=$!

wait $PID1 && wait $PID2
echo "Both done"
agentctl runs --limit 2
```

### Machine-readable output for CI

```bash
# Emit JSONL, parse with jq
agentctl --json run \
  --verify "go test ./..." \
  "Fix the race condition in TestOrchestrator_Cancellation" \
| tee run-events.jsonl \
| jq -r 'select(.kind=="completed" or .kind=="failed") | [.kind, .data.result // .data.error] | @tsv'
```

### Check run outcomes in a script

```bash
#!/usr/bin/env bash
RUN_ID=$(agentctl --json run "Add error handling to handlers" \
  | jq -r 'select(.kind=="completed") | .run_id' | head -1)

STATUS=$(agentctl --json status "$RUN_ID" | jq -r .status)

if [[ "$STATUS" == "completed" ]]; then
  echo "✓ Run succeeded"
else
  echo "✗ Run failed — see: agentctl inspect $RUN_ID"
  exit 1
fi
```

---

## 7. Observability — inspecting what happened

### Recent runs at a glance

```bash
agentctl runs
agentctl runs --limit 20
```

### Full detail on a specific run

```bash
agentctl status <run-id>
```

Shows prompt, result, cost, token counts, timing, and any error.

### Stream of events

```bash
agentctl logs <run-id>
```

Every event in order: `started`, `text`, `tool_use`, `tool_result`, `completed` / `failed`, `verification`.

### Full payloads (no truncation)

```bash
agentctl inspect <run-id>
```

Use this when you want to see the exact tool inputs and outputs the agent used.

### Filter events with jq

```bash
# See only tool calls
agentctl --json logs <run-id> | jq 'select(.kind=="tool_use") | .data'

# Total cost across the last 10 runs
agentctl --json runs --limit 10 | jq -s '[.[].cost_usd] | add'

# Find all runs that used a specific tool
agentctl --json runs --limit 100 | jq -r '.id' \
  | while read id; do
      agentctl --json logs "$id" \
        | jq --arg id "$id" 'select(.kind=="tool_use" and .data.name=="Write") | $id'
    done | sort -u
```

### Watch a live run from a second terminal

```bash
# Terminal 1
agentctl run "Long task..."

# Terminal 2 — tail the most recent run's events as they stream in
watch -n1 "agentctl logs \$(agentctl --json runs --limit 1 | jq -r '.id')"
```

---

## 8. Dry-running with the fake runner

The fake runner emits a deterministic sequence of events without any LLM calls or credentials. Use it to test orchestration logic, CI pipelines, or the dashboard itself.

### Smoke test the pipeline

```bash
agentctl run --runner fake "Test the pipeline end-to-end"
```

### Test the verify flag without spending tokens

```bash
agentctl run \
  --runner fake \
  --verify "go test ./..." \
  "Pretend to write a feature"
```

### Test timeout behaviour

```bash
# The fake runner completes almost instantly, so timeout testing
# requires injecting a sleep in the verify step or using a script
agentctl run \
  --runner fake \
  --timeout 5 \
  --verify "sleep 10" \  # verify will exceed the timeout window — run marked failed
  "Quick task"
```

### Populate the dashboard with demo data

```bash
PROMPTS=(
  "Analyse the codebase and identify potential improvements"
  "Write unit tests for the authentication package"
  "Refactor the database layer to use connection pooling"
  "Add error handling to all API handlers"
  "Generate OpenAPI documentation from source code"
)

for prompt in "${PROMPTS[@]}"; do
  agentctl run --runner fake "$prompt"
done

agentctl web   # then: cd web && npm run dev
```

---

## 9. MCP integration

When `agentctl mcp` is running, any MCP-compatible host (Claude Desktop, Cursor, etc.) can orchestrate runs as tool calls.

### Claude Desktop config

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "nickbot": {
      "command": "/path/to/agentctl",
      "args": ["mcp"]
    }
  }
}
```

### Available MCP tools

| Tool | What it does |
|------|--------------|
| `start_run` | Start an agent run (blocks until complete) |
| `get_run` | Fetch a run record by ID |
| `list_runs` | List recent runs |
| `get_events` | Fetch all events for a run |

### MCP resources

Every run is also browsable as an MCP resource at `nickbot://runs/<run-id>`. Hosts that support MCP resources can attach run records directly to conversations.

### Use a custom DB path in the MCP server

```json
{
  "mcpServers": {
    "nickbot": {
      "command": "/path/to/agentctl",
      "args": ["--db", "/path/to/custom.db", "mcp"]
    }
  }
}
```

---

## Quick reference

| Goal | Key flags |
|------|-----------|
| Test a change actually works | `--verify "go test ./..."` |
| Never break your branch | `--isolate` |
| Hard deadline | `--timeout 300` |
| Machine-readable output | `--json` piped to `jq` |
| No LLM / no credentials | `--runner fake` |
| Specific model | `--model claude-opus-4-6` |
| Specific working directory | `--workdir ./some/subdir` |
| Label the run | `--agent my-task-name` |
