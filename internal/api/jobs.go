package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/store"
)

type jobResponse struct {
	*agent.Run
	RunCount  int          `json:"run_count"`
	TotalCost float64      `json:"total_cost"`
	Runs      []*agent.Run `json:"runs,omitempty"`
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	roots, err := s.st.ListJobs(r.Context(), limit)
	if err != nil {
		s.log.Error("list jobs", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}

	jobs := make([]jobResponse, 0, len(roots))
	for _, root := range roots {
		allRuns, err := s.st.GetJobRuns(r.Context(), root.JobID)
		if err != nil {
			s.log.Warn("get job runs", slog.String("job_id", root.JobID), slog.Any("err", err))
			allRuns = []*agent.Run{root}
		}
		total := 0.0
		for _, run := range allRuns {
			total += run.CostUSD
		}
		jobs = append(jobs, jobResponse{
			Run:       root,
			RunCount:  len(allRuns),
			TotalCost: total,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	root, err := s.st.GetRun(r.Context(), id)
	if err != nil {
		if _, ok := err.(store.ErrNotFound); ok {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		s.log.Error("get job root", slog.String("id", id), slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "failed to get job")
		return
	}

	jobID := root.JobID
	if jobID == "" {
		jobID = root.ID // legacy run without job_id
	}

	allRuns, err := s.st.GetJobRuns(r.Context(), jobID)
	if err != nil {
		s.log.Error("get job runs", slog.String("job_id", jobID), slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "failed to get job runs")
		return
	}

	total := 0.0
	for _, run := range allRuns {
		total += run.CostUSD
	}

	writeJSON(w, http.StatusOK, jobResponse{
		Run:       root,
		RunCount:  len(allRuns),
		TotalCost: total,
		Runs:      allRuns,
	})
}
