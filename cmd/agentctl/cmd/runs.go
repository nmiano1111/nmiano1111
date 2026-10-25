package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nmiano/nickbot/internal/ui"
)

func newRunsCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "runs",
		Short: "List recent runs",
		Long: `List recent agent execution runs.

Examples:
  agentctl runs
  agentctl runs --limit 50
  agentctl runs --json | jq .`,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openStore()
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer st.Close()

			runs, err := st.ListRuns(cmd.Context(), limit)
			if err != nil {
				return fmt.Errorf("list runs: %w", err)
			}

			p := ui.NewPrinter(os.Stdout, rootFlags.jsonOut)
			p.PrintRunTable(runs)
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "maximum number of runs to show")
	return cmd
}
