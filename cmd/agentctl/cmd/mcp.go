package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/nmiano/nickbot/internal/mcp"
	"github.com/nmiano/nickbot/internal/orchestration"
	"github.com/nmiano/nickbot/internal/runners/claudecode"
)

func newMCPCmd() *cobra.Command {
	var model string

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run as an MCP server (stdio transport)",
		Long: `Start agentctl as a Model Context Protocol (MCP) server over stdio.

This makes agentctl composable with any MCP-compatible host, including
Claude desktop and Cursor. Add it to your MCP configuration:

  {
    "mcpServers": {
      "nickbot": {
        "command": "agentctl",
        "args": ["mcp"]
      }
    }
  }

Exposed tools: start_run, get_run, list_runs, get_events`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpServe(cmd, model)
		},
	}

	cmd.Flags().StringVar(&model, "model", "", "default model for started runs")
	return cmd
}

func mcpServe(cmd *cobra.Command, _ string) error {
	st, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	var log *slog.Logger
	if rootFlags.verbose {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	runner := &claudecode.Runner{}
	orch := orchestration.New(runner, st, log)

	return mcp.Serve(cmd.Context(), orch, os.Stdin, os.Stdout)
}
