package orchestration_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/orchestration"
	"github.com/nmiano/nickbot/internal/runners/fake"
	"github.com/nmiano/nickbot/internal/store/sqlite"
)

func openOrchestrator(t *testing.T, script fake.Script) (*orchestration.Orchestrator, *sqlite.Store) {
	t.Helper()
	st, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	runner := &fake.Runner{Script: script}
	orch := orchestration.New(runner, st, nil)
	t.Cleanup(func() { st.Close() })
	return orch, st
}

func drainExecution(exec *orchestration.Execution) []agent.Event {
	var events []agent.Event
	for e := range exec.Events {
		events = append(events, e)
	}
	return events
}

func TestOrchestrator_SuccessfulRun(t *testing.T) {
	orch, st := openOrchestrator(t, nil) // nil = DefaultScript

	req := agent.RunRequest{
		AgentID: "test-agent",
		Prompt:  "Do the thing",
	}

	exec, err := orch.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if exec.Run.ID == "" {
		t.Error("Run.ID is empty; orchestrator must assign a run ID")
	}
	if exec.Run.Status != agent.RunStatusRunning {
		t.Errorf("Run.Status after Start = %q, want running", exec.Run.Status)
	}

	runID := exec.Run.ID
	events := drainExecution(exec)

	// Run must be persisted and in completed state.
	run, err := st.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run.Status != agent.RunStatusCompleted {
		t.Errorf("final run status = %q, want completed", run.Status)
	}
	if run.CompletedAt == nil {
		t.Error("CompletedAt is nil, want non-nil")
	}
	if run.CostUSD <= 0 {
		t.Errorf("CostUSD = %f, want > 0", run.CostUSD)
	}

	// Events must be persisted.
	storedEvents, err := st.ListEvents(context.Background(), runID)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(storedEvents) != len(events) {
		t.Errorf("stored %d events, emitted %d", len(storedEvents), len(events))
	}
}

func TestOrchestrator_AssignsRunID(t *testing.T) {
	orch, _ := openOrchestrator(t, fake.Script{
		{Kind: agent.EventStarted},
		{Kind: agent.EventCompleted, Payload: agent.CompletedData{Result: "ok"}},
	})

	exec, err := orch.Start(context.Background(), agent.RunRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	drainExecution(exec)

	if exec.Run.ID == "" {
		t.Error("expected non-empty run ID")
	}
}

func TestOrchestrator_PreserveProvidedRunID(t *testing.T) {
	orch, _ := openOrchestrator(t, fake.Script{
		{Kind: agent.EventStarted},
		{Kind: agent.EventCompleted, Payload: agent.CompletedData{Result: "ok"}},
	})

	const wantID = "my-custom-run-id"
	exec, err := orch.Start(context.Background(), agent.RunRequest{
		RunID:  wantID,
		Prompt: "test",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	drainExecution(exec)

	if exec.Run.ID != wantID {
		t.Errorf("Run.ID = %q, want %q", exec.Run.ID, wantID)
	}
}

func TestOrchestrator_EventsHaveRunID(t *testing.T) {
	orch, _ := openOrchestrator(t, nil)

	exec, err := orch.Start(context.Background(), agent.RunRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	runID := exec.Run.ID
	events := drainExecution(exec)

	for _, e := range events {
		if e.RunID != runID {
			t.Errorf("event.RunID = %q, want %q", e.RunID, runID)
		}
	}
}

func TestOrchestrator_Cancellation(t *testing.T) {
	orch, st := openOrchestrator(t, fake.Script{
		{Kind: agent.EventStarted},
		{Delay: 5 * time.Second, Kind: agent.EventText, Payload: agent.TextData{Content: "slow"}},
		{Kind: agent.EventCompleted, Payload: agent.CompletedData{Result: "done"}},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	exec, err := orch.Start(ctx, agent.RunRequest{Prompt: "slow task"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	runID := exec.Run.ID
	drainExecution(exec)

	// Allow a moment for the goroutine to persist the final state.
	time.Sleep(100 * time.Millisecond)

	run, err := st.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run.Status != agent.RunStatusFailed {
		t.Errorf("run status after cancellation = %q, want failed", run.Status)
	}
}

func TestOrchestrator_ListRuns(t *testing.T) {
	orch, _ := openOrchestrator(t, fake.Script{
		{Kind: agent.EventStarted},
		{Kind: agent.EventCompleted, Payload: agent.CompletedData{Result: "ok"}},
	})

	for i := 0; i < 3; i++ {
		exec, err := orch.Start(context.Background(), agent.RunRequest{Prompt: "task"})
		if err != nil {
			t.Fatalf("Start(%d) error = %v", i, err)
		}
		drainExecution(exec)
	}

	runs, err := orch.ListRuns(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("ListRuns() = %d runs, want 3", len(runs))
	}
}

func TestOrchestrator_GetRun(t *testing.T) {
	orch, _ := openOrchestrator(t, fake.Script{
		{Kind: agent.EventStarted},
		{Kind: agent.EventCompleted, Payload: agent.CompletedData{Result: "success", CostUSD: 0.002}},
	})

	exec, err := orch.Start(context.Background(), agent.RunRequest{
		AgentID: "my-agent",
		Prompt:  "Do something useful",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	runID := exec.Run.ID
	drainExecution(exec)

	run, err := orch.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run.AgentID != "my-agent" {
		t.Errorf("AgentID = %q, want %q", run.AgentID, "my-agent")
	}
	if run.Status != agent.RunStatusCompleted {
		t.Errorf("Status = %q, want completed", run.Status)
	}
}
