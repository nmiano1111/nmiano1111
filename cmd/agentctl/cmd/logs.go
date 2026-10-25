package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nmiano/nickbot/internal/ui"
)

func newLogsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs <run-id>",
		Short: "Show execution events for a run",
		Long: `Display all recorded events for a run in chronological order.

Examples:
  agentctl logs abc12345-...
  agentctl logs abc12345 --json | jq 'select(.kind=="text") | .data.content'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]

			st, err := openStore()
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer st.Close()

			// Verify the run exists first.
			run, err := st.GetRun(cmd.Context(), runID)
			if err != nil {
				p := ui.NewPrinter(os.Stdout, rootFlags.jsonOut)
				p.PrintError(fmt.Sprintf("Run %q not found", runID), err)
				return err
			}

			events, err := st.ListEvents(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("list events: %w", err)
			}

			p := ui.NewPrinter(os.Stdout, rootFlags.jsonOut)

			if !rootFlags.jsonOut {
				fmt.Fprintf(os.Stdout, "\n  %s  %s\n\n",
					ui.StyleDim.Render("Events for run"),
					ui.StyleRunID.Render(runID),
				)
				_ = run // used for context above
			}

			for _, e := range events {
				p.PrintEvent(e)
			}

			if !rootFlags.jsonOut {
				fmt.Fprintln(os.Stdout, "")
			}
			return nil
		},
	}
}
