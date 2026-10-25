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
	drainExecution(exec) // blocks until channel closes, which happens after UpdateRun

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

func TestStore_RecoverOrphans(t *testing.T) {
	dir := t.TempDir()

	// Phase 1: create a DB with a run stuck in "running" (PID=0, process gone).
	st1, err := sqlite.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	run := &agent.Run{
		ID:      "orphan-run-id",
		AgentID: "test",
		Prompt:  "orphaned",
		Status:  agent.RunStatusRunning,
		// PID=0: no live process, so recoverOrphans should mark it failed.
	}
	if err := st1.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	st1.Close()

	// Phase 2: re-open the DB; recoverOrphans runs automatically in Open.
	st2, err := sqlite.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("re-open store: %v", err)
	}
	defer st2.Close()

	recovered, err := st2.GetRun(context.Background(), "orphan-run-id")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if recovered.Status != agent.RunStatusFailed {
		t.Errorf("recovered status = %q, want failed", recovered.Status)
	}
	if recovered.CompletedAt == nil {
		t.Error("CompletedAt is nil after recovery")
	}
}

func TestOrchestrator_Timeout(t *testing.T) {
	orch, st := openOrchestrator(t, fake.Script{
		{Kind: agent.EventStarted},
		{Delay: 10 * time.Second, Kind: agent.EventText, Payload: agent.TextData{Content: "slow"}},
		{Kind: agent.EventCompleted, Payload: agent.CompletedData{Result: "done"}},
	})

	exec, err := orch.Start(context.Background(), agent.RunRequest{
		Prompt: "slow task",
		Config: agent.RunConfig{TimeoutMS: 50}, // 50ms timeout
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	runID := exec.Run.ID
	drainExecution(exec) // blocks until channel closes, which happens after UpdateRun

	run, err := st.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run.Status != agent.RunStatusFailed {
		t.Errorf("run status after timeout = %q, want failed", run.Status)
	}
}

func TestOrchestrator_Verification_Pass(t *testing.T) {
	orch, st := openOrchestrator(t, nil) // DefaultScript → completes successfully

	exec, err := orch.Start(context.Background(), agent.RunRequest{
		Prompt: "test with verify",
		Config: agent.RunConfig{Verify: "echo verified"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	runID := exec.Run.ID
	events := drainExecution(exec)

	var gotVerify bool
	for _, e := range events {
		if e.Kind == agent.EventVerification {
			gotVerify = true
			var d agent.VerificationData
			if err := e.DecodeData(&d); err != nil {
				t.Fatalf("DecodeData: %v", err)
			}
			if !d.Passed {
				t.Errorf("verification Passed = false, want true")
			}
		}
	}
	if !gotVerify {
		t.Error("expected EventVerification event, got none")
	}

	// Verification event must be persisted to the store.
	stored, err := st.ListEvents(context.Background(), runID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	var storedVerify bool
	for _, e := range stored {
		if e.Kind == agent.EventVerification {
			storedVerify = true
		}
	}
	if !storedVerify {
		t.Error("EventVerification not persisted to store")
	}
}

func TestOrchestrator_Verification_Fail(t *testing.T) {
	orch, _ := openOrchestrator(t, nil) // DefaultScript → completes successfully

	exec, err := orch.Start(context.Background(), agent.RunRequest{
		Prompt: "test with failing verify",
		Config: agent.RunConfig{Verify: "exit 1"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	events := drainExecution(exec)

	var gotVerify bool
	for _, e := range events {
		if e.Kind == agent.EventVerification {
			gotVerify = true
			var d agent.VerificationData
			if err := e.DecodeData(&d); err != nil {
				t.Fatalf("DecodeData: %v", err)
			}
			if d.Passed {
				t.Errorf("verification Passed = true, want false")
			}
		}
	}
	if !gotVerify {
		t.Error("expected EventVerification event for failing verifier, got none")
	}
}

func TestOrchestrator_Verification_NotEmittedOnFailure(t *testing.T) {
	// Verifier must NOT run when the run itself fails.
	orch, _ := openOrchestrator(t, fake.Script{
		{Kind: agent.EventStarted},
		{Kind: agent.EventFailed, Payload: agent.FailedData{Error: "run error"}},
	})

	exec, err := orch.Start(context.Background(), agent.RunRequest{
		Prompt: "failing run",
		Config: agent.RunConfig{Verify: "echo should-not-run"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	events := drainExecution(exec)

	for _, e := range events {
		if e.Kind == agent.EventVerification {
			t.Error("EventVerification emitted after a failed run; want none")
		}
	}
}

func TestOrchestrator_PreservesCancelledStatus(t *testing.T) {
	// Verify that when agentctl cancel writes RunStatusCancelled to the store
	// before the context is cancelled, the orchestrator does not overwrite it
	// with "failed".
	orch, st := openOrchestrator(t, fake.Script{
		{Kind: agent.EventStarted},
		{Delay: 5 * time.Second, Kind: agent.EventText, Payload: agent.TextData{Content: "slow"}},
	})

	ctx, cancel := context.WithCancel(context.Background())
	exec, err := orch.Start(ctx, agent.RunRequest{Prompt: "slow task"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	runID := exec.Run.ID

	// Simulate agentctl cancel: write cancelled status to the store, then stop the context.
	go func() {
		time.Sleep(50 * time.Millisecond)
		run, err := st.GetRun(context.Background(), runID)
		if err != nil {
			return
		}
		now := time.Now()
		run.Status = agent.RunStatusCancelled
		run.CompletedAt = &now
		_ = st.UpdateRun(context.Background(), run)
		cancel()
	}()

	drainExecution(exec)

	run, err := st.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run.Status != agent.RunStatusCancelled {
		t.Errorf("run status = %q, want cancelled", run.Status)
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
