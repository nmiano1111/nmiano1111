// Package fake provides a deterministic fake runner for testing orchestration
// logic without requiring any external services.
package fake

import (
	"context"
	"fmt"
	"time"

	"github.com/nmiano/nickbot/internal/agent"
)

// Script defines the sequence of events a FakeRunner emits.
type Script []ScriptEvent

// ScriptEvent is a single step in a fake execution script.
type ScriptEvent struct {
	// Delay before emitting this event.
	Delay time.Duration
	// Kind of event to emit.
	Kind agent.EventKind
	// Payload is the event data. Must be compatible with Kind.
	Payload any
}

// Runner is a deterministic agent runner that emits a pre-defined script.
// It is useful for testing orchestration logic, CLI output, and TUI rendering.
type Runner struct {
	// Script is the sequence of events emitted per run.
	// If nil, DefaultScript is used.
	Script Script
}

// DefaultScript simulates a plausible short agent interaction.
var DefaultScript = Script{
	{Delay: 0, Kind: agent.EventStarted, Payload: nil},
	{Delay: 150 * time.Millisecond, Kind: agent.EventText, Payload: agent.TextData{
		Content: "I'll begin by analyzing the task.",
	}},
	{Delay: 200 * time.Millisecond, Kind: agent.EventToolUse, Payload: agent.ToolUseData{
		ToolID: "tool-001",
		Name:   "Bash",
		Input:  []byte(`{"command":"echo hello"}`),
	}},
	{Delay: 100 * time.Millisecond, Kind: agent.EventToolResult, Payload: agent.ToolResultData{
		ToolID:  "tool-001",
		Content: "hello",
		IsError: false,
	}},
	{Delay: 200 * time.Millisecond, Kind: agent.EventText, Payload: agent.TextData{
		Content: "The command executed successfully. Task analysis complete.",
	}},
	{Delay: 100 * time.Millisecond, Kind: agent.EventCompleted, Payload: agent.CompletedData{
		Result:       "Task completed successfully.",
		CostUSD:      0.0012,
		InputTokens:  248,
		OutputTokens: 64,
		DurationMS:   850,
	}},
}

// Name implements agent.Runner.
func (r *Runner) Name() string { return "fake" }

// Run implements agent.Runner.
func (r *Runner) Run(ctx context.Context, req agent.RunRequest) (<-chan agent.Event, error) {
	script := r.Script
	if script == nil {
		script = DefaultScript
	}

	// Buffer is large enough to hold the whole script plus a failure event,
	// so the goroutine can always deliver the terminal event even after
	// context cancellation.
	ch := make(chan agent.Event, len(script)+1)

	go func() {
		defer close(ch)

		seq := 0
		// emit sends an event unconditionally (non-blocking into the buffer).
		emit := func(kind agent.EventKind, payload any) {
			e := agent.MakeEvent(req.RunID, seq, kind, payload)
			seq++
			ch <- e // always succeeds: buffer is sized for worst case
		}
		// emitFailed sends a terminal failure and returns.
		emitFailed := func() {
			emit(agent.EventFailed, agent.FailedData{
				Error: fmt.Sprintf("cancelled: %v", ctx.Err()),
			})
		}

		for _, step := range script {
			if step.Delay > 0 {
				select {
				case <-ctx.Done():
					emitFailed()
					return
				case <-time.After(step.Delay):
				}
			}

			// Check cancellation before each event.
			select {
			case <-ctx.Done():
				emitFailed()
				return
			default:
			}

			emit(step.Kind, step.Payload)

			// If this was a terminal event, stop.
			if step.Kind == agent.EventCompleted || step.Kind == agent.EventFailed {
				return
			}
		}
	}()

	return ch, nil
}
