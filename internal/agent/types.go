// Package agent defines the core types and interfaces for agent execution.
// This is the foundational contract between the orchestration layer and all
// execution backends (runners).
package agent

import (
	"encoding/json"
	"time"
)

// ── Event ────────────────────────────────────────────────────────────────────

// EventKind classifies what happened during execution.
type EventKind string

const (
	// EventStarted is emitted once when the runner begins processing.
	EventStarted EventKind = "started"
	// EventText carries a chunk of assistant text output.
	EventText EventKind = "text"
	// EventToolUse is emitted when the agent invokes a tool.
	EventToolUse EventKind = "tool_use"
	// EventToolResult carries the output of a tool invocation.
	EventToolResult EventKind = "tool_result"
	// EventCompleted signals successful completion of the run.
	EventCompleted EventKind = "completed"
	// EventFailed signals that the run ended with an error.
	EventFailed EventKind = "failed"
	// EventVerification is emitted after completion when a verifier is configured.
	// It follows EventCompleted and precedes channel close.
	EventVerification EventKind = "verification"
)

// Event is a structured record of something that occurred during execution.
// The Data field is a JSON-encoded payload whose schema depends on Kind.
type Event struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id"`
	Kind      EventKind       `json:"kind"`
	Timestamp time.Time       `json:"timestamp"`
	Seq       int             `json:"seq"`        // monotonic within a run
	ParentSeq *int            `json:"parent_seq,omitempty"` // links tool_result → tool_use
	Data      json.RawMessage `json:"data,omitempty"`
}

// StartedData is the payload for EventStarted events.
type StartedData struct {
	// PID is the OS process ID of the runner subprocess, if applicable.
	PID int `json:"pid,omitempty"`
}

// TextData is the payload for EventText events.
type TextData struct {
	Content string `json:"content"`
}

// ToolUseData is the payload for EventToolUse events.
type ToolUseData struct {
	ToolID string          `json:"tool_id"`
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input,omitempty"`
}

// ToolResultData is the payload for EventToolResult events.
type ToolResultData struct {
	ToolID  string `json:"tool_id"`
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

// CompletedData is the payload for EventCompleted events.
type CompletedData struct {
	Result       string  `json:"result"`
	CostUSD      float64 `json:"cost_usd"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	DurationMS   int64   `json:"duration_ms"`
}

// FailedData is the payload for EventFailed events.
type FailedData struct {
	Error string `json:"error"`
}

// VerificationData is the payload for EventVerification events.
type VerificationData struct {
	Command string `json:"command"`
	Passed  bool   `json:"passed"`
	Output  string `json:"output,omitempty"`
}

// ── Run ──────────────────────────────────────────────────────────────────────

// RunStatus represents the lifecycle state of an execution run.
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

// Run is a persistent record of a single execution attempt.
// Tasks and Runs are separate: a task may have multiple runs (retries).
// Milestone 1 omits the Task concept; each Run is standalone.
type Run struct {
	ID           string     `json:"id"`
	AgentID      string     `json:"agent_id"`
	Prompt       string     `json:"prompt"`
	Status       RunStatus  `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Result       string     `json:"result,omitempty"`
	Error        string     `json:"error,omitempty"`
	CostUSD      float64    `json:"cost_usd"`
	InputTokens  int        `json:"input_tokens"`
	OutputTokens int        `json:"output_tokens"`
	DurationMS   int64      `json:"duration_ms"`
	// RunnerName identifies which backend executed this run.
	RunnerName string `json:"runner_name"`
	// Model is the LLM model used, if applicable.
	Model string `json:"model,omitempty"`
	// ParentRunID is set when this run was spawned as a child of another run.
	ParentRunID string `json:"parent_run_id,omitempty"`
	// JobID is the ID of the root run of this run's tree.
	// For root runs JobID == ID. For child runs it is inherited from the parent.
	// Empty for runs created before this field was introduced.
	JobID string `json:"job_id,omitempty"`
	// PID is the OS process ID of the runner subprocess while running.
	// Zero if the runner doesn't track process IDs.
	PID int `json:"pid,omitempty"`
}

// ── RunRequest ───────────────────────────────────────────────────────────────

// RunRequest carries everything a Runner needs to execute an agent task.
type RunRequest struct {
	// RunID is the pre-assigned run ID; must be non-empty.
	RunID string
	// AgentID is the logical agent identifier (e.g. "planner", "reviewer").
	AgentID string
	// Prompt is the instruction delivered to the agent.
	Prompt string
	// Config controls execution environment details.
	Config RunConfig
	// ParentRunID links this run as a child of another run.
	ParentRunID string
}

// RunConfig configures the execution environment for a run.
type RunConfig struct {
	// Model is the LLM model to use, or empty for the runner's default.
	Model string
	// AllowedTools lists tool names the agent is permitted to use.
	// An empty list means runner-default permissions.
	AllowedTools []string
	// WorkDir is the working directory for tool execution.
	WorkDir string
	// Env provides additional environment variables for the execution.
	Env map[string]string
	// PermissionMode controls how the agent handles permission requests.
	// For the claude-code runner: acceptEdits | bypassPermissions | default | dontAsk
	// Empty defaults to "acceptEdits" in non-interactive (--print) mode.
	PermissionMode string
	// IsolateWorktree, when true, forks a git worktree from WorkDir (or the
	// current directory) before starting the runner. The agent works in the
	// worktree; changes are isolated until explicitly merged.
	IsolateWorktree bool
	// TimeoutMS, if > 0, cancels the run after this many milliseconds.
	TimeoutMS int64
	// Verify is a shell command run after successful completion to validate
	// the agent's output. Results are emitted as EventVerification.
	Verify string
}
