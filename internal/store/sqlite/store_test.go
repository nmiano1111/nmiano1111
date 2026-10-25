package sqlite_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/store"
	"github.com/nmiano/nickbot/internal/store/sqlite"
)

func openTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_CreateAndGetRun(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	run := &agent.Run{
		ID:         "run-001",
		AgentID:    "test-agent",
		Prompt:     "Do something",
		Status:     agent.RunStatusPending,
		CreatedAt:  now,
		RunnerName: "fake",
		Model:      "test-model",
	}

	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	got, err := s.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	if got.ID != run.ID {
		t.Errorf("ID = %q, want %q", got.ID, run.ID)
	}
	if got.AgentID != run.AgentID {
		t.Errorf("AgentID = %q, want %q", got.AgentID, run.AgentID)
	}
	if got.Prompt != run.Prompt {
		t.Errorf("Prompt = %q, want %q", got.Prompt, run.Prompt)
	}
	if got.Status != run.Status {
		t.Errorf("Status = %q, want %q", got.Status, run.Status)
	}
	if got.RunnerName != run.RunnerName {
		t.Errorf("RunnerName = %q, want %q", got.RunnerName, run.RunnerName)
	}
	if got.Model != run.Model {
		t.Errorf("Model = %q, want %q", got.Model, run.Model)
	}
}

func TestStore_UpdateRun(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	run := &agent.Run{
		ID:         "run-002",
		AgentID:    "agent",
		Prompt:     "test",
		Status:     agent.RunStatusPending,
		CreatedAt:  time.Now(),
		RunnerName: "fake",
	}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	now := time.Now()
	run.Status = agent.RunStatusCompleted
	run.StartedAt = &now
	run.CompletedAt = &now
	run.Result = "All done"
	run.CostUSD = 0.005
	run.InputTokens = 100
	run.OutputTokens = 50
	run.DurationMS = 1234

	if err := s.UpdateRun(ctx, run); err != nil {
		t.Fatalf("UpdateRun() error = %v", err)
	}

	got, err := s.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	if got.Status != agent.RunStatusCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if got.Result != "All done" {
		t.Errorf("Result = %q, want %q", got.Result, "All done")
	}
	if got.CostUSD != 0.005 {
		t.Errorf("CostUSD = %f, want 0.005", got.CostUSD)
	}
	if got.StartedAt == nil {
		t.Error("StartedAt = nil, want non-nil")
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt = nil, want non-nil")
	}
}

func TestStore_GetRun_NotFound(t *testing.T) {
	s := openTestStore(t)

	_, err := s.GetRun(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("GetRun() expected error, got nil")
	}
	var notFound store.ErrNotFound
	if _, ok := err.(store.ErrNotFound); !ok {
		t.Errorf("error type = %T, want ErrNotFound", err)
	}
	_ = notFound
}

func TestStore_ListRuns(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		run := &agent.Run{
			ID:        string(rune('a'+i)) + "-run",
			AgentID:   "agent",
			Prompt:    "prompt",
			Status:    agent.RunStatusCompleted,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := s.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun(%d) error = %v", i, err)
		}
	}

	runs, err := s.ListRuns(ctx, 3)
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("ListRuns(3) = %d runs, want 3", len(runs))
	}

	// Verify descending order.
	for i := 1; i < len(runs); i++ {
		if runs[i].CreatedAt.After(runs[i-1].CreatedAt) {
			t.Errorf("runs not in descending order: [%d].CreatedAt > [%d].CreatedAt",
				i, i-1)
		}
	}
}

func TestStore_AppendAndListEvents(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	run := &agent.Run{
		ID:        "event-run-001",
		AgentID:   "agent",
		Prompt:    "test",
		Status:    agent.RunStatusRunning,
		CreatedAt: time.Now(),
	}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	events := []*agent.Event{
		{
			ID:        "event-run-001-e0000",
			RunID:     "event-run-001",
			Kind:      agent.EventStarted,
			Timestamp: time.Now(),
			Seq:       0,
		},
		{
			ID:        "event-run-001-e0001",
			RunID:     "event-run-001",
			Kind:      agent.EventText,
			Timestamp: time.Now(),
			Seq:       1,
			Data:      json.RawMessage(`{"content":"hello"}`),
		},
		{
			ID:        "event-run-001-e0002",
			RunID:     "event-run-001",
			Kind:      agent.EventCompleted,
			Timestamp: time.Now(),
			Seq:       2,
			Data:      json.RawMessage(`{"result":"done","cost_usd":0.001}`),
		},
	}

	for _, e := range events {
		if err := s.AppendEvent(ctx, e); err != nil {
			t.Fatalf("AppendEvent(%s) error = %v", e.ID, err)
		}
	}

	got, err := s.ListEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}

	if len(got) != len(events) {
		t.Fatalf("ListEvents() = %d events, want %d", len(got), len(events))
	}

	for i, e := range got {
		if e.Kind != events[i].Kind {
			t.Errorf("event[%d].Kind = %q, want %q", i, e.Kind, events[i].Kind)
		}
		if e.Seq != events[i].Seq {
			t.Errorf("event[%d].Seq = %d, want %d", i, e.Seq, events[i].Seq)
		}
	}
}

func TestStore_AppendEvent_Idempotent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	run := &agent.Run{
		ID: "idem-run", AgentID: "a", Prompt: "p",
		Status: agent.RunStatusRunning, CreatedAt: time.Now(),
	}
	s.CreateRun(ctx, run)

	e := &agent.Event{
		ID: "idem-e0", RunID: "idem-run",
		Kind: agent.EventStarted, Timestamp: time.Now(), Seq: 0,
	}

	// Insert twice — should not fail (INSERT OR IGNORE).
	if err := s.AppendEvent(ctx, e); err != nil {
		t.Fatalf("first AppendEvent() error = %v", err)
	}
	if err := s.AppendEvent(ctx, e); err != nil {
		t.Fatalf("second AppendEvent() error = %v", err)
	}

	events, _ := s.ListEvents(ctx, "idem-run")
	if len(events) != 1 {
		t.Errorf("expected 1 event after duplicate insert, got %d", len(events))
	}
}

func TestStore_UpdateRun_NotFound(t *testing.T) {
	s := openTestStore(t)

	run := &agent.Run{ID: "ghost", Status: agent.RunStatusCompleted}
	err := s.UpdateRun(context.Background(), run)
	if err == nil {
		t.Fatal("UpdateRun() expected error for non-existent run")
	}
}

func TestStore_OpenCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "test.db")
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open() with nested path error = %v", err)
	}
	s.Close()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("database file not created at %s", path)
	}
}

func TestStore_ParentRunID(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	parent := &agent.Run{
		ID: "parent-run", AgentID: "planner", Prompt: "plan it",
		Status: agent.RunStatusCompleted, CreatedAt: time.Now(), RunnerName: "fake",
	}
	if err := s.CreateRun(ctx, parent); err != nil {
		t.Fatal(err)
	}

	child := &agent.Run{
		ID: "child-run", AgentID: "reviewer", Prompt: "review it",
		Status:      agent.RunStatusPending,
		CreatedAt:   time.Now(),
		RunnerName:  "fake",
		ParentRunID: "parent-run",
	}
	if err := s.CreateRun(ctx, child); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetRun(ctx, "child-run")
	if err != nil {
		t.Fatal(err)
	}
	if got.ParentRunID != "parent-run" {
		t.Errorf("ParentRunID = %q, want %q", got.ParentRunID, "parent-run")
	}
}

func TestStore_EventParentSeq(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	run := &agent.Run{
		ID: "seq-run", AgentID: "a", Prompt: "p",
		Status: agent.RunStatusRunning, CreatedAt: time.Now(), RunnerName: "fake",
	}
	s.CreateRun(ctx, run)

	toolUseSeq := 1
	events := []*agent.Event{
		{ID: "e0", RunID: "seq-run", Kind: agent.EventStarted, Timestamp: time.Now(), Seq: 0},
		{ID: "e1", RunID: "seq-run", Kind: agent.EventToolUse, Timestamp: time.Now(), Seq: 1},
		{ID: "e2", RunID: "seq-run", Kind: agent.EventToolResult, Timestamp: time.Now(), Seq: 2,
			ParentSeq: &toolUseSeq},
	}
	for _, e := range events {
		if err := s.AppendEvent(ctx, e); err != nil {
			t.Fatalf("AppendEvent(%s) = %v", e.ID, err)
		}
	}

	got, err := s.ListEvents(ctx, "seq-run")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len(events) = %d, want 3", len(got))
	}
	if got[0].ParentSeq != nil {
		t.Error("started event should have nil ParentSeq")
	}
	if got[1].ParentSeq != nil {
		t.Error("tool_use event should have nil ParentSeq")
	}
	if got[2].ParentSeq == nil || *got[2].ParentSeq != 1 {
		t.Errorf("tool_result ParentSeq = %v, want &1", got[2].ParentSeq)
	}
}

func TestStore_MigrateIdempotent(t *testing.T) {
	// Open the same DB twice — migrations must not error on second open.
	dir := t.TempDir()
	path := filepath.Join(dir, "migrate.db")

	s1, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("first Open() = %v", err)
	}
	s1.Close()

	s2, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("second Open() after migration = %v", err)
	}
	s2.Close()
}
