package fake_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/runners/fake"
)

func TestRunner_DefaultScript(t *testing.T) {
	r := &fake.Runner{}
	req := agent.RunRequest{
		RunID:   "test-run-001",
		AgentID: "test-agent",
		Prompt:  "Hello, world!",
	}

	ch, err := r.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var events []agent.Event
	for e := range ch {
		events = append(events, e)
	}

	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}

	// First event must be EventStarted.
	if events[0].Kind != agent.EventStarted {
		t.Errorf("first event kind = %q, want %q", events[0].Kind, agent.EventStarted)
	}

	// Last event must be EventCompleted or EventFailed.
	last := events[len(events)-1]
	if last.Kind != agent.EventCompleted && last.Kind != agent.EventFailed {
		t.Errorf("last event kind = %q, want completed or failed", last.Kind)
	}

	// Verify sequence numbers are monotonic.
	for i, e := range events {
		if e.Seq != i {
			t.Errorf("event[%d].Seq = %d, want %d", i, e.Seq, i)
		}
	}

	// Verify all events have the correct RunID.
	for _, e := range events {
		if e.RunID != req.RunID {
			t.Errorf("event.RunID = %q, want %q", e.RunID, req.RunID)
		}
	}

	// Verify CompletedData is decodable.
	if last.Kind == agent.EventCompleted {
		var d agent.CompletedData
		if err := last.DecodeData(&d); err != nil {
			t.Errorf("decode CompletedData: %v", err)
		}
		if d.CostUSD <= 0 {
			t.Errorf("CompletedData.CostUSD = %f, want > 0", d.CostUSD)
		}
	}
}

func TestRunner_Cancellation(t *testing.T) {
	// Use a long script to ensure cancellation happens mid-run.
	script := fake.Script{
		{Delay: 0, Kind: agent.EventStarted, Payload: nil},
		{Delay: 5 * time.Second, Kind: agent.EventText, Payload: agent.TextData{Content: "Should not reach this"}},
		{Delay: 0, Kind: agent.EventCompleted, Payload: agent.CompletedData{Result: "done"}},
	}

	r := &fake.Runner{Script: script}
	req := agent.RunRequest{RunID: "test-cancel-001"}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ch, err := r.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var events []agent.Event
	for e := range ch {
		events = append(events, e)
	}

	// Channel must close within a reasonable time after context cancellation.
	// (This is verified by the test timeout itself.)

	// Should receive at least the started event and a failed event.
	if len(events) < 1 {
		t.Fatal("expected at least one event")
	}
	if events[0].Kind != agent.EventStarted {
		t.Errorf("first event = %q, want started", events[0].Kind)
	}

	// After cancellation, last event must be EventFailed.
	last := events[len(events)-1]
	if last.Kind != agent.EventFailed {
		t.Errorf("last event after cancellation = %q, want failed", last.Kind)
	}
}

func TestRunner_CustomScript(t *testing.T) {
	script := fake.Script{
		{Delay: 0, Kind: agent.EventStarted, Payload: nil},
		{Delay: 0, Kind: agent.EventText, Payload: agent.TextData{Content: "Custom response"}},
		{Delay: 0, Kind: agent.EventCompleted, Payload: agent.CompletedData{
			Result:       "Custom complete",
			CostUSD:      0.005,
			InputTokens:  100,
			OutputTokens: 50,
		}},
	}

	r := &fake.Runner{Script: script}
	ch, err := r.Run(context.Background(), agent.RunRequest{RunID: "custom-001"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var kinds []agent.EventKind
	for e := range ch {
		kinds = append(kinds, e.Kind)
	}

	expected := []agent.EventKind{agent.EventStarted, agent.EventText, agent.EventCompleted}
	if len(kinds) != len(expected) {
		t.Fatalf("got %d events, want %d: %v", len(kinds), len(expected), kinds)
	}
	for i, k := range expected {
		if kinds[i] != k {
			t.Errorf("event[%d].Kind = %q, want %q", i, kinds[i], k)
		}
	}
}

func TestRunner_Name(t *testing.T) {
	r := &fake.Runner{}
	if r.Name() != "fake" {
		t.Errorf("Name() = %q, want %q", r.Name(), "fake")
	}
}

func TestRunner_ConcurrentRuns(t *testing.T) {
	r := &fake.Runner{}

	const n = 5
	done := make(chan struct{}, n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			req := agent.RunRequest{
				RunID:  fmt.Sprintf("concurrent-%d", i),
				Prompt: "concurrent test",
			}
			ch, err := r.Run(context.Background(), req)
			if err != nil {
				t.Errorf("goroutine %d: Run() error = %v", i, err)
				return
			}
			for range ch {
			}
		}(i)
	}

	timeout := time.After(5 * time.Second)
	for i := 0; i < n; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("timeout waiting for concurrent runs")
		}
	}
}
