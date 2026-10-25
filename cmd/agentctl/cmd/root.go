// Package cmd defines the agentctl CLI command tree using Cobra.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nmiano/nickbot/internal/store/sqlite"
)

// rootFlags holds global flags shared across all commands.
var rootFlags struct {
	dbPath  string
	jsonOut bool
	verbose bool
}

var rootCmd = &cobra.Command{
	Use:   "agentctl",
	Short: "AI agent orchestration platform",
	Long: `agentctl — orchestrate AI agents, track execution, and observe results.

Run 'agentctl help <command>' for detailed help on any command.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		os.Exit(1)
	}
}

func init() {
	defaultDB := filepath.Join(homeDir(), ".nickbot", "nickbot.db")

	rootCmd.PersistentFlags().StringVar(&rootFlags.dbPath, "db", defaultDB,
		"path to the SQLite database")
	rootCmd.PersistentFlags().BoolVar(&rootFlags.jsonOut, "json", false,
		"output results as JSONL (machine-readable)")
	rootCmd.PersistentFlags().BoolVar(&rootFlags.verbose, "verbose", false,
		"enable verbose logging")

	rootCmd.AddCommand(
		newRunCmd(),
		newRunsCmd(),
		newStatusCmd(),
		newLogsCmd(),
		newInspectCmd(),
		newMCPCmd(),
	)
}

// openStore opens the SQLite store using the global --db flag.
func openStore() (*sqlite.Store, error) {
	return sqlite.Open(rootFlags.dbPath)
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}
