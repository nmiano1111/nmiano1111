package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/nmiano/nickbot/internal/api"
)

func newWebCmd() *cobra.Command {
	var flags struct {
		port int
		host string
	}

	cmd := &cobra.Command{
		Use:   "web",
		Short: "Start the web dashboard server",
		Long: `Start the HTTP API server that powers the web dashboard.

The server exposes a REST API for querying runs and events, and a
Server-Sent Events endpoint for live run streaming.

During development, run the Vite frontend separately:
  cd web && npm run dev

In production, build the frontend first:
  cd web && npm run build
  agentctl web  (serves API + embedded static files)

Examples:
  agentctl web
  agentctl web --port 8080
  agentctl web --host 0.0.0.0 --port 7545`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWeb(cmd.Context(), flags.host, flags.port)
		},
	}

	cmd.Flags().IntVar(&flags.port, "port", 7545, "port to listen on")
	cmd.Flags().StringVar(&flags.host, "host", "localhost", "host to bind to")

	return cmd
}

func runWeb(ctx context.Context, host string, port int) error {
	st, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	var log *slog.Logger
	if rootFlags.verbose {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	srv := api.New(st, log)

	addr := fmt.Sprintf("%s:%d", host, port)
	fmt.Printf("\n  nickbot web dashboard\n")
	fmt.Printf("  API  →  http://%s/api/v1/health\n\n", addr)
	fmt.Printf("  During development:\n")
	fmt.Printf("    cd web && npm run dev\n\n")

	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv,
	}
	go func() {
		<-ctx.Done()
		httpSrv.Shutdown(context.Background()) //nolint:errcheck
	}()

	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("web server: %w", err)
	}
	return nil
}
