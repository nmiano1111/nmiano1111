// Package orchestration coordinates agent execution: it creates run records,
// dispatches work to runners, persists events, and maintains run lifecycle.
package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/store"
)

// Execution is an in-progress or completed agent run.
// The Run field is updated as the execution progresses; callers must not
// assume it is final until the Events channel is closed.
type Execution struct {
	// Run is the initial run record. It is updated in place by the orchestrator.
	Run *agent.Run
	// Events streams execution events. Closed when execution terminates.
	Events <-chan agent.Event
}

// Orchestrator coordinates single-agent execution for Milestone 1.
// In future milestones it will extend to multi-agent workflows.
type Orchestrator struct {
	runner agent.Runner
	store  store.Store
	log    *slog.Logger
}

// New creates an Orchestrator backed by the given runner and store.
func New(runner agent.Runner, st store.Store, log *slog.Logger) *Orchestrator {
	if log == nil {
		log = slog.Default()
	}
	return &Orchestrator{runner: runner, store: st, log: log}
}

// Start creates a run record, begins execution, and returns an Execution handle.
// The caller consumes handle.Events; the orchestrator persists events and updates
// the run concurrently. The Events channel is closed when execution ends.
//
// Cancelling ctx propagates to the runner and terminates execution.
func (o *Orchestrator) Start(ctx context.Context, req agent.RunRequest) (*Execution, error) {
	// Assign a run ID if the caller didn't supply one.
	if req.RunID == "" {
		req.RunID = uuid.New().String()
	}
	if req.AgentID == "" {
		req.AgentID = o.runner.Name()
	}

	// Persist the initial run record (status = pending).
	run := &agent.Run{
		ID:         req.RunID,
		AgentID:    req.AgentID,
		Prompt:     req.Prompt,
		Status:     agent.RunStatusPending,
		CreatedAt:  time.Now(),
		RunnerName: o.runner.Name(),
		Model:      req.Config.Model,
	}
	if err := o.store.CreateRun(ctx, run); err != nil {
		return nil, fmt.Errorf("orchestrator: persist run: %w", err)
	}

	// Start the runner. This may fail immediately (e.g. binary not found).
	runnerCh, err := o.runner.Run(ctx, req)
	if err != nil {
		run.Status = agent.RunStatusFailed
		run.Error = err.Error()
		now := time.Now()
		run.CompletedAt = &now
		_ = o.store.UpdateRun(ctx, run)
		return nil, fmt.Errorf("orchestrator: start runner: %w", err)
	}

	// Transition to running. Snapshot the run for the caller; the goroutine
	// works on a private mutable copy to avoid data races.
	now := time.Now()
	run.Status = agent.RunStatusRunning
	run.StartedAt = &now
	_ = o.store.UpdateRun(ctx, run)

	// callerRun is the immutable snapshot returned to the caller.
	// It reflects the state at the moment Start() returns.
	callerRun := *run

	// localRun is the private copy mutated by the background goroutine.
	localRun := *run

	// The outbound channel that callers observe.
	outCh := make(chan agent.Event, 64)

	go func() {
		// Always persist final run state, even on cancellation.
		defer func() {
			if localRun.Status == agent.RunStatusRunning {
				// Runner closed channel without a terminal event.
				localRun.Status = agent.RunStatusFailed
				localRun.Error = "runner closed without terminal event"
				t := time.Now()
				localRun.CompletedAt = &t
			}
			if err := o.store.UpdateRun(context.Background(), &localRun); err != nil {
				o.log.Warn("failed to persist final run state",
					slog.String("run_id", localRun.ID),
					slog.Any("err", err))
			}
			close(outCh)
		}()

		for e := range runnerCh {
			// Persist using a background context so cancellation of the
			// execution context does not prevent us from recording events.
			if err := o.store.AppendEvent(context.Background(), &e); err != nil {
				o.log.Warn("failed to persist event",
					slog.String("run_id", localRun.ID),
					slog.String("kind", string(e.Kind)),
					slog.Any("err", err))
			}

			// Update local run state based on terminal events.
			switch e.Kind {
			case agent.EventCompleted:
				applyCompleted(&localRun, e)
			case agent.EventFailed:
				applyFailed(&localRun, e)
			}

			// Forward to caller (outCh is buffered; non-blocking under normal load).
			select {
			case outCh <- e:
			default:
				// Drop the event if the caller isn't consuming fast enough.
				// This is acceptable: the event is already persisted.
			}
		}
	}()

	return &Execution{Run: &callerRun, Events: outCh}, nil
}

// GetRun retrieves a run record from the store.
func (o *Orchestrator) GetRun(ctx context.Context, id string) (*agent.Run, error) {
	return o.store.GetRun(ctx, id)
}

// ListRuns retrieves recent run records.
func (o *Orchestrator) ListRuns(ctx context.Context, limit int) ([]*agent.Run, error) {
	return o.store.ListRuns(ctx, limit)
}

// ListEvents retrieves events for a run.
func (o *Orchestrator) ListEvents(ctx context.Context, runID string) ([]*agent.Event, error) {
	return o.store.ListEvents(ctx, runID)
}

// ── State updaters ────────────────────────────────────────────────────────────

func applyCompleted(run *agent.Run, e agent.Event) {
	var d agent.CompletedData
	if err := json.Unmarshal(e.Data, &d); err == nil {
		run.Result = d.Result
		run.CostUSD = d.CostUSD
		run.InputTokens = d.InputTokens
		run.OutputTokens = d.OutputTokens
		run.DurationMS = d.DurationMS
	}
	run.Status = agent.RunStatusCompleted
	now := e.Timestamp
	run.CompletedAt = &now
}

func applyFailed(run *agent.Run, e agent.Event) {
	var d agent.FailedData
	if err := json.Unmarshal(e.Data, &d); err == nil {
		run.Error = d.Error
	}
	run.Status = agent.RunStatusFailed
	now := e.Timestamp
	run.CompletedAt = &now
}
