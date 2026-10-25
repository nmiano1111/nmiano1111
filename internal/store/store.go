// Package store defines the persistence interface for orchestration state.
// Implementations must be safe for concurrent use.
package store

import (
	"context"

	"github.com/nmiano/nickbot/internal/agent"
)

// Store persists runs and their events.
// All methods accept a context for cancellation.
type Store interface {
	// CreateRun persists a new run record.
	CreateRun(ctx context.Context, run *agent.Run) error

	// UpdateRun updates a run record in place.
	UpdateRun(ctx context.Context, run *agent.Run) error

	// GetRun retrieves a run by ID. Returns ErrNotFound if absent.
	GetRun(ctx context.Context, id string) (*agent.Run, error)

	// ListRuns returns runs ordered by created_at descending.
	// limit ≤ 0 means no limit.
	ListRuns(ctx context.Context, limit int) ([]*agent.Run, error)

	// AppendEvent persists an event. The event's RunID must match an existing run.
	AppendEvent(ctx context.Context, event *agent.Event) error

	// ListEvents returns all events for a run, ordered by seq ascending.
	ListEvents(ctx context.Context, runID string) ([]*agent.Event, error)

	// Close releases any resources held by the store.
	Close() error
}

// ErrNotFound is returned when a requested entity does not exist.
type ErrNotFound struct {
	Kind string
	ID   string
}

func (e ErrNotFound) Error() string {
	return e.Kind + " not found: " + e.ID
}
