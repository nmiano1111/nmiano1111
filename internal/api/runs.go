package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/orchestration"
	"github.com/nmiano/nickbot/internal/runners/claudecode"
	"github.com/nmiano/nickbot/internal/runners/fake"
	"github.com/nmiano/nickbot/internal/store"
)

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	runs, err := s.st.ListRuns(r.Context(), limit)
	if err != nil {
		s.log.Error("list runs", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	if runs == nil {
		runs = []*agent.Run{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.st.GetRun(r.Context(), id)
	if err != nil {
		if _, ok := err.(store.ErrNotFound); ok {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		s.log.Error("get run", slog.String("id", id), slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "failed to get run")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

type createRunReq struct {
	Prompt    string `json:"prompt"`
	AgentID   string `json:"agent_id"`
	Model     string `json:"model"`
	WorkDir   string `json:"workdir"`
	Runner    string `json:"runner"`
	TimeoutMS int64  `json:"timeout_ms"`
	Verify    string `json:"verify"`
	Isolate   bool   `json:"isolate"`
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req createRunReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	var runner agent.Runner
	switch req.Runner {
	case "fake":
		runner = &fake.Runner{}
	case "claude", "claude-code", "":
		runner = &claudecode.Runner{}
	default:
		writeError(w, http.StatusBadRequest, "unknown runner: "+req.Runner)
		return
	}

	if req.AgentID == "" {
		req.AgentID = runner.Name()
	}

	orch := orchestration.New(runner, s.st, s.log)
	runReq := agent.RunRequest{
		AgentID: req.AgentID,
		Prompt:  req.Prompt,
		Config: agent.RunConfig{
			Model:           req.Model,
			WorkDir:         req.WorkDir,
			TimeoutMS:       req.TimeoutMS,
			Verify:          req.Verify,
			IsolateWorktree: req.Isolate,
		},
	}

	exec, err := orch.Start(r.Context(), runReq)
	if err != nil {
		s.log.Error("start run", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "failed to start run: "+err.Error())
		return
	}

	// Drain events in background; the orchestrator persists them to the store.
	go func() {
		for range exec.Events {
		}
	}()

	writeJSON(w, http.StatusAccepted, exec.Run)
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.st.GetRun(r.Context(), id)
	if err != nil {
		if _, ok := err.(store.ErrNotFound); ok {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get run")
		return
	}

	if run.Status != agent.RunStatusRunning {
		writeError(w, http.StatusConflict,
			"run is not running (status: "+string(run.Status)+")")
		return
	}
	if run.PID == 0 {
		writeError(w, http.StatusConflict, "run has no tracked process ID")
		return
	}

	proc, err := os.FindProcess(run.PID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "process not found")
		return
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "already_finished"})
			return
		}
		s.log.Error("signal process", slog.Int("pid", run.PID), slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "failed to signal process")
		return
	}

	// Optimistically mark as cancelled.
	now := time.Now()
	run.Status = agent.RunStatusCancelled
	run.CompletedAt = &now
	if err := s.st.UpdateRun(r.Context(), run); err != nil {
		s.log.Warn("mark cancelled", slog.String("run_id", id), slog.Any("err", err))
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
