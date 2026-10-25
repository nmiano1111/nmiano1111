package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/ui"
)

func newInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <run-id>",
		Short: "Detailed view of a run including all events",
		Long: `Show comprehensive details about a run: metadata, prompt, result, and all events.

Examples:
  agentctl inspect abc12345-...`,
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

			events, err := st.ListEvents(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("list events: %w", err)
			}

			p := ui.NewPrinter(os.Stdout, rootFlags.jsonOut)
			p.PrintRun(run)

			if !rootFlags.jsonOut {
				fmt.Fprintf(os.Stdout, "  %s  (%d events)\n\n", ui.StyleDim.Render("Events"), len(events))
			}

			// For inspect, show full event content without truncation.
			for _, e := range events {
				if rootFlags.jsonOut {
					p.PrintEvent(e)
					continue
				}
				fmt.Fprintln(os.Stdout, formatEventFull(e))
			}

			if !rootFlags.jsonOut {
				fmt.Fprintln(os.Stdout, "")
			}
			return nil
		},
	}
}

// formatEventFull renders an event without truncation (for inspect).
func formatEventFull(e *agent.Event) string {
	ts := e.Timestamp.Format("15:04:05")
	display, ok := ui.EventKindDisplays[string(e.Kind)]
	if !ok {
		display = ui.EventKindDisplay{Label: string(e.Kind), Style: ui.StyleDim}
	}

	header := fmt.Sprintf("  %s  %s",
		ui.StyleDim.Render(ts),
		display.Style.Render(display.Label),
	)

	switch e.Kind {
	case agent.EventText:
		var d agent.TextData
		_ = e.DecodeData(&d)
		if d.Content == "" {
			return header
		}
		indented := "    " + strings.ReplaceAll(strings.TrimSpace(d.Content), "\n", "\n    ")
		return header + "\n" + ui.StyleBase.Render(indented)

	case agent.EventToolUse:
		var d agent.ToolUseData
		_ = e.DecodeData(&d)
		inputStr := ""
		if len(d.Input) > 0 {
			inputStr = "\n    " + ui.StyleDim.Render("input: ") + string(d.Input)
		}
		return header + "  " + ui.StyleMauve.Render(d.Name) + inputStr

	case agent.EventToolResult:
		var d agent.ToolResultData
		_ = e.DecodeData(&d)
		prefix := ""
		if d.IsError {
			prefix = ui.StyleError.Render("[error] ")
		}
		content := strings.TrimSpace(d.Content)
		if len(content) > 500 {
			content = content[:497] + "..."
		}
		indented := "    " + strings.ReplaceAll(content, "\n", "\n    ")
		return header + "\n" + prefix + ui.StyleDim.Render(indented)

	default:
		return header + "  " + ui.StyleDim.Render(string(e.Data))
	}
}
