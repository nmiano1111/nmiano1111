package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nmiano/nickbot/internal/ui"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <run-id>",
		Short: "Show status of a run",
		Long: `Display the current status and details of a specific run.

Examples:
  agentctl status abc12345-...
  agentctl status abc12345 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]

			st, err := openStore()
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer st.Close()

			run, err := st.GetRun(cmd.Context(), runID)
			if err != nil {
				p := ui.NewPrinter(os.Stdout, rootFlags.jsonOut)
				p.PrintError(fmt.Sprintf("Run %q not found", runID), err)
				return err
			}

			p := ui.NewPrinter(os.Stdout, rootFlags.jsonOut)
			p.PrintRun(run)

			children, err := st.ListChildRuns(cmd.Context(), runID)
			if err == nil && len(children) > 0 {
				p.Println(ui.StyleDim.Render("  Delegated tasks") + "\n")
				p.PrintRunTable(children)
			}
			return nil
		},
	}
}
