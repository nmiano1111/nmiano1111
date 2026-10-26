// Package api implements the HTTP API server for the web dashboard.
// It delegates all business logic to the existing orchestration and store
// packages; no domain logic lives here.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/nmiano/nickbot/internal/store"
)

// Server is the HTTP API server for the web dashboard.
type Server struct {
	st  store.Store
	log *slog.Logger
	mux *http.ServeMux
}

// New creates a new API server backed by the given store.
func New(st store.Store, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	s := &Server{
		st:  st,
		log: log,
		mux: http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Health
	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)

	// Runs
	s.mux.HandleFunc("GET /api/v1/runs", s.handleListRuns)
	s.mux.HandleFunc("POST /api/v1/runs", s.handleCreateRun)
	s.mux.HandleFunc("GET /api/v1/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/cancel", s.handleCancelRun)

	// Events
	s.mux.HandleFunc("GET /api/v1/runs/{id}/events", s.handleListEvents)
	s.mux.HandleFunc("GET /api/v1/runs/{id}/stream", s.handleStreamEvents)
}

// ServeHTTP implements http.Handler with CORS preflight support and
// request logging.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
	w.Header().Set("Access-Control-Max-Age", "86400")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.log.Debug("api request", slog.String("method", r.Method), slog.String("path", r.URL.Path))
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server on addr, shutting down gracefully
// when ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("api: listen %s: %w", addr, err)
	}
	srv := &http.Server{Handler: s}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("api: serve: %w", err)
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": "0.3.0"})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
