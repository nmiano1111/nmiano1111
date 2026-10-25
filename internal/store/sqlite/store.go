// Package sqlite provides a SQLite-backed implementation of store.Store.
// It uses modernc.org/sqlite (pure Go, no cgo required).
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/store"
	_ "modernc.org/sqlite" // register sqlite driver
)

const schema = `
CREATE TABLE IF NOT EXISTS runs (
    id            TEXT PRIMARY KEY,
    agent_id      TEXT NOT NULL,
    prompt        TEXT NOT NULL,
    status        TEXT NOT NULL,
    runner_name   TEXT NOT NULL DEFAULT '',
    model         TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL,
    started_at    DATETIME,
    completed_at  DATETIME,
    result        TEXT NOT NULL DEFAULT '',
    error         TEXT NOT NULL DEFAULT '',
    cost_usd      REAL NOT NULL DEFAULT 0,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    duration_ms   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS events (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id),
    seq        INTEGER NOT NULL,
    kind       TEXT NOT NULL,
    timestamp  DATETIME NOT NULL,
    data       TEXT NOT NULL DEFAULT '',
    UNIQUE(run_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_events_run_id ON events(run_id);
CREATE INDEX IF NOT EXISTS idx_runs_created_at ON runs(created_at DESC);
`

// Store implements store.Store using SQLite.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) a SQLite database at path.
// Parent directories are created as needed.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("sqlite: create parent dirs: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	// SQLite performs best with a single writer; WAL mode improves concurrency.
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: pragma: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close implements store.Store.
func (s *Store) Close() error { return s.db.Close() }

// ── Run operations ────────────────────────────────────────────────────────────

// CreateRun implements store.Store.
func (s *Store) CreateRun(ctx context.Context, r *agent.Run) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runs
			(id, agent_id, prompt, status, runner_name, model,
			 created_at, started_at, completed_at,
			 result, error, cost_usd, input_tokens, output_tokens, duration_ms)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.AgentID, r.Prompt, string(r.Status), r.RunnerName, r.Model,
		r.CreatedAt.UTC(), nullTime(r.StartedAt), nullTime(r.CompletedAt),
		r.Result, r.Error, r.CostUSD, r.InputTokens, r.OutputTokens, r.DurationMS,
	)
	if err != nil {
		return fmt.Errorf("sqlite: create run: %w", err)
	}
	return nil
}

// UpdateRun implements store.Store.
func (s *Store) UpdateRun(ctx context.Context, r *agent.Run) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE runs SET
			status=?, runner_name=?, model=?,
			started_at=?, completed_at=?,
			result=?, error=?, cost_usd=?, input_tokens=?, output_tokens=?, duration_ms=?
		WHERE id=?`,
		string(r.Status), r.RunnerName, r.Model,
		nullTime(r.StartedAt), nullTime(r.CompletedAt),
		r.Result, r.Error, r.CostUSD, r.InputTokens, r.OutputTokens, r.DurationMS,
		r.ID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: update run: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound{Kind: "run", ID: r.ID}
	}
	return nil
}

// GetRun implements store.Store.
func (s *Store) GetRun(ctx context.Context, id string) (*agent.Run, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, prompt, status, runner_name, model,
		       created_at, started_at, completed_at,
		       result, error, cost_usd, input_tokens, output_tokens, duration_ms
		FROM runs WHERE id=?`, id)
	r, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound{Kind: "run", ID: id}
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get run: %w", err)
	}
	return r, nil
}

// ListRuns implements store.Store.
func (s *Store) ListRuns(ctx context.Context, limit int) ([]*agent.Run, error) {
	q := `SELECT id, agent_id, prompt, status, runner_name, model,
	             created_at, started_at, completed_at,
	             result, error, cost_usd, input_tokens, output_tokens, duration_ms
	      FROM runs ORDER BY created_at DESC`
	var args []any
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list runs: %w", err)
	}
	defer rows.Close()
	var runs []*agent.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan run: %w", err)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// ── Event operations ──────────────────────────────────────────────────────────

// AppendEvent implements store.Store.
func (s *Store) AppendEvent(ctx context.Context, e *agent.Event) error {
	data := "{}"
	if len(e.Data) > 0 {
		data = string(e.Data)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO events (id, run_id, seq, kind, timestamp, data)
		VALUES (?,?,?,?,?,?)`,
		e.ID, e.RunID, e.Seq, string(e.Kind), e.Timestamp.UTC(), data,
	)
	if err != nil {
		return fmt.Errorf("sqlite: append event: %w", err)
	}
	return nil
}

// ListEvents implements store.Store.
func (s *Store) ListEvents(ctx context.Context, runID string) ([]*agent.Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, seq, kind, timestamp, data
		FROM events WHERE run_id=? ORDER BY seq ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list events: %w", err)
	}
	defer rows.Close()
	var events []*agent.Event
	for rows.Next() {
		var (
			e        agent.Event
			dataStr  string
			tsStr    string
		)
		if err := rows.Scan(&e.ID, &e.RunID, &e.Seq, (*string)(&e.Kind), &tsStr, &dataStr); err != nil {
			return nil, fmt.Errorf("sqlite: scan event: %w", err)
		}
		e.Timestamp, _ = time.Parse(time.RFC3339Nano, tsStr)
		if dataStr != "" && dataStr != "{}" {
			e.Data = json.RawMessage(dataStr)
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}

// ── Helpers ───────────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanRun(row scanner) (*agent.Run, error) {
	var (
		r           agent.Run
		status      string
		createdStr  string
		startedStr  sql.NullString
		completedStr sql.NullString
	)
	err := row.Scan(
		&r.ID, &r.AgentID, &r.Prompt, &status, &r.RunnerName, &r.Model,
		&createdStr, &startedStr, &completedStr,
		&r.Result, &r.Error, &r.CostUSD, &r.InputTokens, &r.OutputTokens, &r.DurationMS,
	)
	if err != nil {
		return nil, err
	}
	r.Status = agent.RunStatus(status)
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	if startedStr.Valid {
		t, _ := time.Parse(time.RFC3339Nano, startedStr.String)
		r.StartedAt = &t
	}
	if completedStr.Valid {
		t, _ := time.Parse(time.RFC3339Nano, completedStr.String)
		r.CompletedAt = &t
	}
	return &r, nil
}

func nullTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{Valid: true, String: t.UTC().Format(time.RFC3339Nano)}
}
