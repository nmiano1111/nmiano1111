package job

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/orchestration"
	"github.com/nmiano/nickbot/internal/runners/claudecode"
	"github.com/nmiano/nickbot/internal/runners/fake"
	"github.com/nmiano/nickbot/internal/store"
)

// StageResult holds the outcome of a completed stage.
type StageResult struct {
	StageID string
	Run     *agent.Run
	Err     error
}

// Executor runs a job Definition against a store.
type Executor struct {
	st  store.Store
	log *slog.Logger
}

// NewExecutor creates an Executor backed by the given store.
func NewExecutor(st store.Store, log *slog.Logger) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{st: st, log: log}
}

// Run executes a job definition and streams StageResults as stages complete.
// The returned channel is closed when the job finishes or fails.
func (e *Executor) Run(ctx context.Context, def *Definition) (<-chan StageResult, *agent.Run, error) {
	if err := def.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid job: %w", err)
	}

	// Create a synthetic coordinator run as the job root.
	jobID := uuid.New().String()
	jobName := def.Name
	if jobName == "" {
		jobName = "job"
	}
	rootRun := &agent.Run{
		ID:         jobID,
		AgentID:    jobName,
		Prompt:     fmt.Sprintf("Job: %s (%d stages)", jobName, len(def.Stages)),
		Status:     agent.RunStatusRunning,
		CreatedAt:  time.Now(),
		RunnerName: "job",
		JobID:      jobID,
	}
	if err := e.st.CreateRun(ctx, rootRun); err != nil {
		return nil, nil, fmt.Errorf("create job root run: %w", err)
	}

	results := make(chan StageResult, len(def.Stages))

	go func() {
		defer close(results)

		done := make(map[string]bool)
		running := make(map[string]bool)
		stageResults := make(map[string]string) // stage id -> result text
		var mu sync.Mutex

		// completionCh receives stage outcomes from goroutines.
		completionCh := make(chan StageResult, len(def.Stages))

		jobFailed := false

		for {
			mu.Lock()
			ready := ReadyStages(def.Stages, done, running)
			allDone := len(done) == len(def.Stages)
			mu.Unlock()

			if allDone {
				break
			}
			if jobFailed && len(running) == 0 {
				break
			}

			// Fire all ready stages concurrently.
			for _, stage := range ready {
				stage := stage
				mu.Lock()
				running[stage.ID] = true
				mu.Unlock()

				go func() {
					sr := e.runStage(ctx, def, stage, jobID, stageResults)
					mu.Lock()
					delete(running, stage.ID)
					done[stage.ID] = true
					if sr.Err == nil {
						stageResults[stage.ID] = sr.Run.Result
					}
					mu.Unlock()
					completionCh <- sr
				}()
			}

			// Wait for at least one stage to finish before re-evaluating.
			if len(running) > 0 || len(ready) > 0 {
				select {
				case sr := <-completionCh:
					results <- sr
					if sr.Err != nil {
						jobFailed = true
					}
				case <-ctx.Done():
					jobFailed = true
				}
			} else if !allDone {
				// No ready stages and nothing running — shouldn't happen if DAG is valid.
				break
			}
		}

		// Drain any remaining completions from concurrent goroutines.
		for len(running) > 0 {
			sr := <-completionCh
			mu.Lock()
			running[sr.StageID] = false
			mu.Unlock()
			results <- sr
		}

		// Mark root run complete.
		now := time.Now()
		rootRun.Status = agent.RunStatusCompleted
		if jobFailed {
			rootRun.Status = agent.RunStatusFailed
		}
		rootRun.CompletedAt = &now
		e.st.UpdateRun(context.Background(), rootRun) //nolint:errcheck
	}()

	return results, rootRun, nil
}

func (e *Executor) runStage(
	ctx context.Context,
	def *Definition,
	stage Stage,
	jobID string,
	results map[string]string,
) StageResult {
	// Interpolate prompt with prior stage results.
	prompt := Interpolate(stage.Prompt, results)

	runnerName := stage.Runner
	if runnerName == "" {
		runnerName = def.Runner
	}
	if runnerName == "" {
		runnerName = "claude"
	}

	model := stage.Model
	if model == "" {
		model = def.Model
	}

	agentID := stage.Agent
	if agentID == "" {
		agentID = stage.ID
	}

	var runner agent.Runner
	switch runnerName {
	case "fake":
		runner = &fake.Runner{}
	case "claude", "claude-code", "":
		runner = &claudecode.Runner{}
	default:
		return StageResult{
			StageID: stage.ID,
			Err:     fmt.Errorf("unknown runner %q", runnerName),
		}
	}

	orch := orchestration.New(runner, e.st, e.log)
	req := agent.RunRequest{
		AgentID:     agentID,
		Prompt:      prompt,
		ParentRunID: jobID,
		Config: agent.RunConfig{
			Model:     model,
			TimeoutMS: int64(stage.Timeout * 1000),
		},
	}

	exec, err := orch.Start(ctx, req)
	if err != nil {
		return StageResult{StageID: stage.ID, Err: fmt.Errorf("start stage %q: %w", stage.ID, err)}
	}

	// Drain events (orchestrator persists them).
	for range exec.Events {
	}

	// Fetch final run state.
	run, err := e.st.GetRun(context.Background(), exec.Run.ID)
	if err != nil {
		return StageResult{StageID: stage.ID, Run: exec.Run, Err: err}
	}

	var stageErr error
	if run.Status == agent.RunStatusFailed || run.Status == agent.RunStatusCancelled {
		stageErr = fmt.Errorf("stage %q %s: %s", stage.ID, run.Status, run.Error)
	}
	return StageResult{StageID: stage.ID, Run: run, Err: stageErr}
}
