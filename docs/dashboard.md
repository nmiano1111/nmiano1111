# Web Dashboard — Implementation Plan

## 1. Existing Capabilities (Reusable)

- Go orchestration engine with Orchestrator, Store interface, Runner interface
- SQLite persistence: runs table + events table, full JSON event payloads
- Domain types: Run, Event, all JSON-serializable
- MCP server as a reference for API operations
- Fake runner for development/testing

## 2. What Needs to Be Added

### Backend
- `internal/api/` package: HTTP server, run/event handlers, SSE streaming
- `cmd/agentctl/cmd/web.go`: new `agentctl web` CLI command

### Frontend
- `web/` directory: React + TypeScript + Vite + Tailwind + shadcn/ui
- API client, TanStack Query hooks, TanStack Table, React Flow

## 3. API Endpoints

```
GET  /api/v1/health
GET  /api/v1/runs?limit=50&status=running
GET  /api/v1/runs/{id}
POST /api/v1/runs          body: {prompt, runner, model, workdir, timeout_ms, verify, isolate}
POST /api/v1/runs/{id}/cancel
GET  /api/v1/runs/{id}/events
GET  /api/v1/runs/{id}/stream   (Server-Sent Events)
```

## 4. Frontend Pages

- `/` — Overview dashboard: stats, recent runs, activity
- `/runs` — Run list with TanStack Table, filtering, sorting
- `/runs/:id` — Run detail: metadata, event log, event graph (React Flow)

## 5. Implementation Sequence

### Phase 1: Go HTTP API
- internal/api/ package
- agentctl web command

### Phase 2: Frontend Foundation
- Vite scaffold, Tailwind, shadcn/ui
- Layout: sidebar + main area
- API client and TypeScript types
- TanStack Query setup

### Phase 3: Runs List + Detail
- Run list with TanStack Table
- Run detail with event log
- SSE for live runs

### Phase 4: Visualization
- Event DAG graph with React Flow (tool_use → tool_result links)
- Run hierarchy graph (parent_run_id relationships)

### Phase 5: Overview + Polish
- Stats summary
- Recent activity feed
- Keyboard shortcuts
- Empty/loading/error states

## 6. Architecture Decisions

- Server-Sent Events for streaming (simpler than WebSocket for read-heavy use case)
- Polling the SQLite store every 500ms for new events (sufficient for local use)
- Go 1.22+ stdlib ServeMux for routing (no third-party HTTP framework needed)
- Orchestrators created per-request for POST /runs (store is shared singleton)
- Frontend served separately during dev (Vite on :5173, API on :7545 with proxy)
- Production: embed web/dist with go:embed

## 7. Compatibility

- No changes to existing CLI commands
- No changes to MCP server
- No changes to Orchestrator, Store, or Runner interfaces
- Dashboard is an additive feature accessed via `agentctl web`
