package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/ui"
)

func newCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <run-id>",
		Short: "Cancel a running agent execution",
		Long: `Cancel a running agent execution by sending SIGTERM to its process.

The run must be in "running" state and have a tracked process ID.
After cancellation, the run will transition to "cancelled" status.

Examples:
  agentctl cancel <run-id>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cancelRun(cmd.Context(), args[0])
		},
	}
}

func cancelRun(ctx context.Context, runID string) error {
	st, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	run, err := st.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("get run %s: %w", shortRunID(runID), err)
	}

	if run.Status != agent.RunStatusRunning {
		return fmt.Errorf("run %s is not running (status: %s)", shortRunID(runID), run.Status)
	}

	if run.PID == 0 {
		return fmt.Errorf("run %s has no tracked process ID (cannot cancel)", shortRunID(runID))
	}

	proc, err := os.FindProcess(run.PID)
	if err != nil {
		return fmt.Errorf("find process %d: %w", run.PID, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// Process already exited between our lookup and the signal — not an error.
		if errors.Is(err, os.ErrProcessDone) {
			fmt.Printf("  %s  run %s already finished\n",
				ui.StyleDim.Render("·"),
				ui.StyleRunID.Render(shortRunID(runID)),
			)
			return nil
		}
		return fmt.Errorf("signal process %d: %w", run.PID, err)
	}

	// Optimistically mark as cancelled so the orchestrator goroutine preserves
	// the status even if it races with the process exit.
	now := time.Now()
	run.Status = agent.RunStatusCancelled
	run.CompletedAt = &now
	if err := st.UpdateRun(ctx, run); err != nil {
		// Non-fatal: the status will still end up as "failed" via orchestrator.
		fmt.Printf("  %s  warning: could not mark run as cancelled: %v\n",
			ui.StyleWarning.Render("!"), err)
	}

	fmt.Printf("  %s  sent SIGTERM to pid %d  (%s)\n",
		ui.StyleAccent.Render("→"),
		run.PID,
		ui.StyleRunID.Render(shortRunID(runID)),
	)
	return nil
}
