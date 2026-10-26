package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/store"
)

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.st.GetRun(r.Context(), id); err != nil {
		if _, ok := err.(store.ErrNotFound); ok {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get run")
		return
	}

	events, err := s.st.ListEvents(r.Context(), id)
	if err != nil {
		s.log.Error("list events", slog.String("run_id", id), slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	if events == nil {
		events = []*agent.Event{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// handleStreamEvents streams run events as Server-Sent Events.
// It replays all existing events first, then polls the store every 500 ms
// for new events until the run reaches a terminal state.
func (s *Server) handleStreamEvents(w http.ResponseWriter, r *http.Request) {
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

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported by server")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send all existing events.
	events, err := s.st.ListEvents(r.Context(), id)
	if err != nil {
		sseWriteError(w, flusher, "failed to load events")
		return
	}

	lastSeq := -1
	for _, e := range events {
		sseWrite(w, flusher, "event", e)
		lastSeq = e.Seq
	}
	// Send current run state so the client has complete context.
	sseWrite(w, flusher, "run", run)

	if isTerminal(run.Status) {
		sseWriteRaw(w, flusher, "done", "{}")
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			allEvents, err := s.st.ListEvents(r.Context(), id)
			if err != nil {
				continue
			}
			for _, e := range allEvents {
				if e.Seq > lastSeq {
					sseWrite(w, flusher, "event", e)
					lastSeq = e.Seq
				}
			}

			updated, err := s.st.GetRun(r.Context(), id)
			if err != nil {
				continue
			}
			if updated.Status != run.Status {
				run = updated
				sseWrite(w, flusher, "run", run)
			}
			if isTerminal(run.Status) {
				sseWriteRaw(w, flusher, "done", "{}")
				return
			}
		}
	}
}

func isTerminal(s agent.RunStatus) bool {
	return s == agent.RunStatusCompleted ||
		s == agent.RunStatusFailed ||
		s == agent.RunStatusCancelled
}

func sseWrite(w http.ResponseWriter, f http.Flusher, eventType string, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, b) //nolint:errcheck
	f.Flush()
}

func sseWriteRaw(w http.ResponseWriter, f http.Flusher, eventType, raw string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, raw) //nolint:errcheck
	f.Flush()
}

func sseWriteError(w http.ResponseWriter, f http.Flusher, msg string) {
	b, _ := json.Marshal(map[string]string{"message": msg})
	fmt.Fprintf(w, "event: error\ndata: %s\n\n", b) //nolint:errcheck
	f.Flush()
}
