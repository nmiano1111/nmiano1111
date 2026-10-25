// Package orchestration coordinates agent execution: it creates run records,
// dispatches work to runners, persists events, and maintains run lifecycle.
package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/store"
	"github.com/nmiano/nickbot/internal/verifier"
)

// Execution is an in-progress or completed agent run.
// The Run field reflects the state at the moment Start() returned; it is not
// updated after that. Callers should re-fetch from the store for final state.
type Execution struct {
	Run    *agent.Run
	Events <-chan agent.Event
}

// Orchestrator coordinates agent execution.
type Orchestrator struct {
	runner agent.Runner
	store  store.Store
	log    *slog.Logger
}

// New creates an Orchestrator backed by the given runner and store.
func New(runner agent.Runner, st store.Store, log *slog.Logger) *Orchestrator {
	if log == nil {
		log = slog.Default()
	}
	return &Orchestrator{runner: runner, store: st, log: log}
}

// Start creates a run record, begins execution, and returns an Execution handle.
// The caller drains handle.Events; the orchestrator persists events and updates
// the run concurrently. The Events channel is closed when execution (and any
// post-run verification) ends.
func (o *Orchestrator) Start(ctx context.Context, req agent.RunRequest) (*Execution, error) {
	if req.RunID == "" {
		req.RunID = uuid.New().String()
	}
	if req.AgentID == "" {
		req.AgentID = o.runner.Name()
	}

	// Apply timeout to the execution context.
	execCtx := ctx
	var timeoutCancel context.CancelFunc
	if req.Config.TimeoutMS > 0 {
		execCtx, timeoutCancel = context.WithTimeout(ctx, time.Duration(req.Config.TimeoutMS)*time.Millisecond)
	}

	// Fork a git worktree for isolation if requested.
	var worktreePath, gitRoot string
	if req.Config.IsolateWorktree {
		var err error
		baseDir := req.Config.WorkDir
		if baseDir == "" {
			baseDir, err = os.Getwd()
			if err != nil {
				if timeoutCancel != nil {
					timeoutCancel()
				}
				return nil, fmt.Errorf("orchestrator: get workdir: %w", err)
			}
		}
		var wt string
		wt, gitRoot, err = forkWorktree(baseDir, req.RunID)
		if err != nil {
			if timeoutCancel != nil {
				timeoutCancel()
			}
			return nil, fmt.Errorf("orchestrator: worktree: %w", err)
		}
		worktreePath = wt
		req.Config.WorkDir = wt
	}

	// Persist the initial run record.
	run := &agent.Run{
		ID:          req.RunID,
		AgentID:     req.AgentID,
		Prompt:      req.Prompt,
		Status:      agent.RunStatusPending,
		CreatedAt:   time.Now(),
		RunnerName:  o.runner.Name(),
		Model:       req.Config.Model,
		ParentRunID: req.ParentRunID,
	}
	if err := o.store.CreateRun(ctx, run); err != nil {
		if timeoutCancel != nil {
			timeoutCancel()
		}
		if worktreePath != "" {
			removeWorktree(gitRoot, worktreePath)
		}
		return nil, fmt.Errorf("orchestrator: persist run: %w", err)
	}

	// Start the runner.
	runnerCh, err := o.runner.Run(execCtx, req)
	if err != nil {
		run.Status = agent.RunStatusFailed
		run.Error = err.Error()
		now := time.Now()
		run.CompletedAt = &now
		_ = o.store.UpdateRun(ctx, run)
		if timeoutCancel != nil {
			timeoutCancel()
		}
		if worktreePath != "" {
			removeWorktree(gitRoot, worktreePath)
		}
		return nil, fmt.Errorf("orchestrator: start runner: %w", err)
	}

	now := time.Now()
	run.Status = agent.RunStatusRunning
	run.StartedAt = &now
	if err := o.store.UpdateRun(context.Background(), run); err != nil {
		o.log.Warn("failed to persist running status", slog.String("run_id", run.ID), slog.Any("err", err))
	}

	callerRun := *run
	localRun := *run
	outCh := make(chan agent.Event, 64)

	go func() {
		defer func() {
			if timeoutCancel != nil {
				timeoutCancel()
			}
			// agentctl cancel writes RunStatusCancelled to the store before sending
			// SIGTERM. The runner may still emit EventFailed when it exits, so we
			// check the store for external cancellation regardless of current status.
			if localRun.Status == agent.RunStatusRunning || localRun.Status == agent.RunStatusFailed {
				if stored, err := o.store.GetRun(context.Background(), localRun.ID); err == nil &&
					stored.Status == agent.RunStatusCancelled {
					localRun.Status = agent.RunStatusCancelled
					localRun.Error = ""
					if stored.CompletedAt != nil {
						localRun.CompletedAt = stored.CompletedAt
					}
				} else if localRun.Status == agent.RunStatusRunning {
					// No terminal event and not cancelled: generic failure.
					localRun.Status = agent.RunStatusFailed
					localRun.Error = "runner closed without terminal event"
					t := time.Now()
					localRun.CompletedAt = &t
				}
			}
			if err := o.store.UpdateRun(context.Background(), &localRun); err != nil {
				o.log.Warn("failed to persist final run state",
					slog.String("run_id", localRun.ID), slog.Any("err", err))
			}
			if worktreePath != "" {
				removeWorktree(gitRoot, worktreePath)
			}
			close(outCh)
		}()

		seq := 0 // tracks next event seq for verifier event
		for e := range runnerCh {
			seq = e.Seq + 1

			if err := o.store.AppendEvent(context.Background(), &e); err != nil {
				o.log.Warn("failed to persist event",
					slog.String("run_id", localRun.ID),
					slog.String("kind", string(e.Kind)),
					slog.Any("err", err))
			}

			switch e.Kind {
			case agent.EventStarted:
				// Capture the subprocess PID so agentctl cancel can signal it.
				var sd agent.StartedData
				if e.DecodeData(&sd) == nil && sd.PID > 0 {
					localRun.PID = sd.PID
					_ = o.store.UpdateRun(context.Background(), &localRun)
				}
			case agent.EventCompleted:
				applyCompleted(&localRun, e)
			case agent.EventFailed:
				applyFailed(&localRun, e)
			}

			outCh <- e // block until caller drains; never drop events
		}

		// Clear PID now that the subprocess has exited.
		if localRun.PID != 0 {
			localRun.PID = 0
		}

		// Run post-completion verifier if configured and run succeeded.
		if req.Config.Verify != "" && localRun.Status == agent.RunStatusCompleted {
			workDir := req.Config.WorkDir
			if worktreePath != "" {
				workDir = worktreePath
			}
			v := &verifier.Shell{Command: req.Config.Verify}
			result, err := v.Verify(context.Background(), workDir) // detached: caller ctx may already be done
			if err != nil {
				result.Passed = false
				result.Output = err.Error()
			}

			ve := agent.MakeEvent(req.RunID, seq, agent.EventVerification, agent.VerificationData{
				Command: result.Command,
				Passed:  result.Passed,
				Output:  result.Output,
			})
			if persistErr := o.store.AppendEvent(context.Background(), &ve); persistErr != nil {
				o.log.Warn("failed to persist verification event",
					slog.String("run_id", localRun.ID), slog.Any("err", persistErr))
			}
			outCh <- ve // block until caller drains
		}
	}()

	return &Execution{Run: &callerRun, Events: outCh}, nil
}

// GetRun retrieves a run record from the store.
func (o *Orchestrator) GetRun(ctx context.Context, id string) (*agent.Run, error) {
	return o.store.GetRun(ctx, id)
}

// ListRuns retrieves run records ordered by creation time descending.
func (o *Orchestrator) ListRuns(ctx context.Context, limit int) ([]*agent.Run, error) {
	return o.store.ListRuns(ctx, limit)
}

// ListEvents retrieves events for a run.
func (o *Orchestrator) ListEvents(ctx context.Context, runID string) ([]*agent.Event, error) {
	return o.store.ListEvents(ctx, runID)
}

// ── State updaters ────────────────────────────────────────────────────────────

func applyCompleted(run *agent.Run, e agent.Event) {
	var d agent.CompletedData
	if err := json.Unmarshal(e.Data, &d); err == nil {
		run.Result = d.Result
		run.CostUSD = d.CostUSD
		run.InputTokens = d.InputTokens
		run.OutputTokens = d.OutputTokens
		run.DurationMS = d.DurationMS
	}
	run.Status = agent.RunStatusCompleted
	now := e.Timestamp
	run.CompletedAt = &now
}

func applyFailed(run *agent.Run, e agent.Event) {
	var d agent.FailedData
	if err := json.Unmarshal(e.Data, &d); err == nil {
		run.Error = d.Error
	}
	run.Status = agent.RunStatusFailed
	now := e.Timestamp
	run.CompletedAt = &now
}

// ── Worktree isolation ────────────────────────────────────────────────────────

func forkWorktree(baseDir, runID string) (string, string, error) {
	out, err := exec.Command("git", "-C", baseDir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", "", fmt.Errorf("not a git repository: %w", err)
	}
	gitRoot := strings.TrimSpace(string(out))
	worktreePath := filepath.Join(os.TempDir(), "nickbot-worktrees", runID) // full ID avoids collisions
	cmd := exec.Command("git", "-C", gitRoot, "worktree", "add", "--detach", worktreePath, "HEAD")
	if combined, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("git worktree add: %w\n%s", err, combined)
	}
	return worktreePath, gitRoot, nil
}

func removeWorktree(gitRoot, path string) {
	exec.Command("git", "-C", gitRoot, "worktree", "remove", "--force", path).Run() //nolint:errcheck
}
