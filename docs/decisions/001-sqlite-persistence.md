# ADR 001: SQLite for Milestone 1 Persistence

**Status**: Accepted
**Date**: 2026-10-21

## Context

Milestone 1 requires persistent run records and event history for a single-machine
orchestrator. We need to choose a storage backend.

## Decision

Use SQLite via `modernc.org/sqlite` (pure Go driver, no cgo required).

## Rationale

**For SQLite:**
- Zero infrastructure: no separate process to manage
- Sufficient for single-machine, single-orchestrator workloads
- WAL mode provides adequate concurrent read performance
- Pure Go driver (`modernc.org/sqlite`) avoids cgo complexity, simplifies cross-compilation
- The `store.Store` interface is storage-agnostic; migrating to Postgres later is straightforward

**Against Postgres at this stage:**
- Requires a running server, complicating local development
- Connection pooling and network considerations add complexity
- No current requirement for multi-machine coordination

**Rejected: embedded key-value stores (bbolt, badger):**
- Would require custom SQL-like query implementation for `ListRuns` ordering
- Less familiar tooling for inspecting state during debugging

## Consequences

- Simplest possible setup for developers (just `go build`)
- SQLite WAL handles concurrent readers; single writer at a time is acceptable
- When we need Postgres (distributed execution, high concurrency), implement
  `store.Store` as a Postgres adapter without changing any other code
- Do not rely on SQLite-specific behavior (e.g., `ROWID` tricks, FTS) in the schema
