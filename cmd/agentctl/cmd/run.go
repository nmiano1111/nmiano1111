package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/orchestration"
	"github.com/nmiano/nickbot/internal/runners/claudecode"
	"github.com/nmiano/nickbot/internal/runners/fake"
	"github.com/nmiano/nickbot/internal/ui"
	"github.com/nmiano/nickbot/internal/ui/live"
)

func newRunCmd() *cobra.Command {
	var flags struct {
		runner   string
		model    string
		agent    string
		tools    []string
		workDir  string
		permMode string
		isolate  bool
		timeout  float64
		verify   string
	}

	cmd := &cobra.Command{
		Use:   "run <prompt>",
		Short: "Execute an agent task",
		Long: `Execute an agent task and stream its events live.

The prompt is passed directly to the configured agent runner.

Examples:
  agentctl run "List the files in the current directory"
  agentctl run --runner fake "Test task"
  agentctl run --runner claude --model claude-sonnet-4-6 "Write a haiku about Go"
  agentctl run --timeout 120 "Long-running task"
  agentctl run --verify "go test ./..." "Fix the failing tests"
  agentctl run --json "Analyse the repository" | jq .`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := strings.Join(args, " ")
			return runExecute(cmd.Context(), prompt, flags.runner, flags.model, flags.agent, flags.tools, flags.workDir, flags.permMode, flags.isolate, flags.timeout, flags.verify)
		},
	}

	cmd.Flags().StringVar(&flags.runner, "runner", "claude", "runner to use: claude or fake")
	cmd.Flags().StringVar(&flags.model, "model", "", "model to use (runner-specific)")
	cmd.Flags().StringVar(&flags.agent, "agent", "", "logical agent ID (default: runner name)")
	cmd.Flags().StringSliceVar(&flags.tools, "tools", nil, "allowed tool names (comma-separated)")
	cmd.Flags().StringVar(&flags.workDir, "workdir", "", "working directory for tool execution")
	cmd.Flags().StringVar(&flags.permMode, "permission-mode", "", "claude permission mode: acceptEdits|bypassPermissions|default|dontAsk (default: acceptEdits)")
	cmd.Flags().BoolVar(&flags.isolate, "isolate", false, "fork a git worktree for filesystem isolation")
	cmd.Flags().Float64Var(&flags.timeout, "timeout", 0, "cancel the run after this many seconds (0 = no timeout)")
	cmd.Flags().StringVar(&flags.verify, "verify", "", "shell command to run after successful completion to validate output")

	return cmd
}

func runExecute(ctx context.Context, prompt, runnerName, model, agentID string, tools []string, workDir, permMode string, isolate bool, timeoutSec float64, verify string) error {
	// Select runner.
	var runner agent.Runner
	switch runnerName {
	case "fake":
		runner = &fake.Runner{}
	case "claude", "claude-code", "":
		runner = &claudecode.Runner{}
	default:
		return fmt.Errorf("unknown runner %q (available: claude, fake)", runnerName)
	}

	if agentID == "" {
		agentID = runner.Name()
	}

	// Open store.
	st, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	// Build logger.
	var log *slog.Logger
	if rootFlags.verbose {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	orch := orchestration.New(runner, st, log)

	req := agent.RunRequest{
		AgentID: agentID,
		Prompt:  prompt,
		Config: agent.RunConfig{
			Model:           model,
			AllowedTools:    tools,
			WorkDir:         workDir,
			PermissionMode:  permMode,
			IsolateWorktree: isolate,
			TimeoutMS:       int64(timeoutSec * 1000),
			Verify:          verify,
		},
	}

	exec, err := orch.Start(ctx, req)
	if err != nil {
		p := ui.NewPrinter(os.Stdout, rootFlags.jsonOut)
		p.PrintError("Failed to start execution", err)
		return err
	}

	if rootFlags.jsonOut || !isInteractiveTTY() {
		// Non-interactive: emit JSONL events to stdout.
		return streamJSONL(exec)
	}

	// Interactive: launch Bubble Tea TUI.
	return streamTUI(exec)
}

// streamTUI renders the live Bubble Tea execution view.
func streamTUI(exec *orchestration.Execution) error {
	m := live.New(exec.Run, exec.Events)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	// Print a compact completion summary after the TUI exits.
	// Re-fetch the run from the store to get the final status (exec.Run is a
	// snapshot taken at Start() time and always shows "running").
	if lm, ok := final.(live.Model); ok {
		fmt.Print(lm.FinalView())
		st, err := openStore()
		if err == nil {
			if r, err := st.GetRun(context.Background(), exec.Run.ID); err == nil {
				printRunSummary(r)
			} else {
				printRunSummary(exec.Run)
			}
			st.Close()
		} else {
			printRunSummary(exec.Run)
		}
	}
	return nil
}

// streamJSONL emits events as JSONL on stdout.
func streamJSONL(exec *orchestration.Execution) error {
	p := ui.NewPrinter(os.Stdout, true)
	for e := range exec.Events {
		eCopy := e
		p.PrintEvent(&eCopy)
	}
	return nil
}

// printRunSummary prints a compact one-line summary after TUI exits.
func printRunSummary(run *agent.Run) {
	badge, style := "", ui.StyleBase
	switch run.Status {
	case agent.RunStatusCompleted:
		badge, style = ui.BadgeCompleted, ui.StyleSuccess
	case agent.RunStatusFailed:
		badge, style = ui.BadgeFailed, ui.StyleError
	case agent.RunStatusCancelled:
		badge, style = ui.BadgeCancelled, ui.StyleDim
	default:
		badge, style = ui.BadgePending, ui.StyleWarning
	}

	cost := ""
	if run.CostUSD > 0 {
		cost = fmt.Sprintf("  ·  $%.4f", run.CostUSD)
	}
	tokens := ""
	if run.InputTokens+run.OutputTokens > 0 {
		tokens = fmt.Sprintf("  ·  %d tokens", run.InputTokens+run.OutputTokens)
	}

	fmt.Printf("\n  %s  %s%s%s\n  %s\n\n",
		style.Render(badge),
		ui.StyleDim.Render("run/"+shortRunID(run.ID)),
		ui.StyleDim.Render(cost),
		ui.StyleDim.Render(tokens),
		ui.StyleDim.Render("agentctl status "+run.ID),
	)
}

func shortRunID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// isInteractiveTTY reports whether stdout is a terminal.
func isInteractiveTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
