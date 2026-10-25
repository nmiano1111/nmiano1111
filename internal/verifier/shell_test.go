package verifier_test

import (
	"context"
	"testing"
	"time"

	"github.com/nmiano/nickbot/internal/verifier"
)

func TestShell_Pass(t *testing.T) {
	v := &verifier.Shell{Command: "exit 0"}
	result, err := v.Verify(context.Background(), "")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !result.Passed {
		t.Errorf("Passed = false, want true")
	}
}

func TestShell_Fail(t *testing.T) {
	v := &verifier.Shell{Command: "exit 1"}
	result, err := v.Verify(context.Background(), "")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if result.Passed {
		t.Errorf("Passed = true, want false")
	}
}

func TestShell_CapturesOutput(t *testing.T) {
	v := &verifier.Shell{Command: "echo hello"}
	result, err := v.Verify(context.Background(), "")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if result.Output == "" {
		t.Error("Output is empty, want non-empty")
	}
}

func TestShell_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	v := &verifier.Shell{Command: "sleep 10"}

	// Cancel the context after the process has had a chance to start.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := v.Verify(ctx, "")
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}
