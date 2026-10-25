// Package claudecode provides a runner that invokes the Claude Code CLI
// (the `claude` binary) in non-interactive print mode with streaming JSON output.
//
// Claude Code integration mechanism:
//   - Command: claude --print --output-format stream-json [--model <m>] <prompt>
//   - Output: newline-delimited JSON (NDJSON) on stdout
//   - Each line is a JSON object with a "type" field
//
// Event mapping:
//   - type="system"    → (ignored; init metadata)
//   - type="assistant" → message content blocks → EventText / EventToolUse
//   - type="user"      → tool_result blocks → EventToolResult
//   - type="result"    → EventCompleted or EventFailed
package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/nmiano/nickbot/internal/agent"
)

// Runner invokes the claude CLI to execute agent tasks.
type Runner struct {
	// ClaudePath is the path to the claude binary.
	// Defaults to "claude" (looked up via PATH).
	ClaudePath string
}

// Name implements agent.Runner.
func (r *Runner) Name() string { return "claude-code" }

// Run implements agent.Runner.
func (r *Runner) Run(ctx context.Context, req agent.RunRequest) (<-chan agent.Event, error) {
	claudePath := r.ClaudePath
	if claudePath == "" {
		claudePath = "claude"
	}
	if _, err := exec.LookPath(claudePath); err != nil {
		return nil, fmt.Errorf("claude-code: binary %q not found in PATH: %w", claudePath, err)
	}

	args := buildArgs(req)
	cmd := exec.CommandContext(ctx, claudePath, args...)
	if req.Config.WorkDir != "" {
		cmd.Dir = req.Config.WorkDir
	}
	for k, v := range req.Config.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Use a buffer for stderr so we can include it in error messages.
	// This is safe: cmd.Wait() ensures all subprocess I/O has completed
	// before the buffer is read.
	var stderrBuf strings.Builder
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("claude-code: stdout pipe: %w", err)
	}
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude-code: start: %w", err)
	}

	ch := make(chan agent.Event, 64)

	go func() {
		defer close(ch)

		seq := 0
		startTime := time.Now()
		sentTerminal := false
		// toolUseSeqs maps tool_use_id → the seq of that tool_use event,
		// so we can link tool_result events back to their originating tool_use.
		toolUseSeqs := map[string]int{}

		// emit sends an event to the channel and returns the event's seq.
		emit := func(kind agent.EventKind, payload any, parentSeq *int) int {
			thisSeq := seq
			e := agent.MakeEvent(req.RunID, seq, kind, payload)
			e.ParentSeq = parentSeq
			seq++
			select {
			case ch <- e:
			case <-ctx.Done():
			}
			if kind == agent.EventCompleted || kind == agent.EventFailed {
				sentTerminal = true
			}
			return thisSeq
		}

		emit(agent.EventStarted, agent.StartedData{PID: cmd.Process.Pid}, nil)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4 MB

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var raw ccRawEvent
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				continue // skip malformed lines
			}

			switch raw.Type {
			case "assistant":
				processAssistantMessage(raw.Message, emit, toolUseSeqs)
			case "user":
				processUserMessage(raw.Message, emit, toolUseSeqs)
			case "result":
				processResult(raw, startTime, emit)
				sentTerminal = true
				return // result is always the last event
			}
		}

		// Wait for the process to exit so stderrBuf is fully populated.
		cmd.Wait() //nolint:errcheck

		if sentTerminal {
			return
		}

		// Build a diagnostic error message from scanner error, context, or stderr.
		var errMsg string
		switch {
		case scanner.Err() != nil:
			errMsg = fmt.Sprintf("reading claude output: %v", scanner.Err())
		case ctx.Err() != nil:
			errMsg = fmt.Sprintf("cancelled: %v", ctx.Err())
		default:
			// Process exited without writing a result event — use stderr content.
			if s := strings.TrimSpace(stderrBuf.String()); s != "" {
				errMsg = s
			} else {
				errMsg = "claude exited without producing a result"
			}
		}
		emit(agent.EventFailed, agent.FailedData{Error: errMsg}, nil)
	}()

	return ch, nil
}

// ── CLI argument construction ─────────────────────────────────────────────────

func buildArgs(req agent.RunRequest) []string {
	permMode := req.Config.PermissionMode
	if permMode == "" {
		permMode = "acceptEdits"
	}
	args := []string{
		"--print", "--verbose",
		"--output-format", "stream-json",
		"--permission-mode", permMode,
	}
	if req.Config.Model != "" {
		args = append(args, "--model", req.Config.Model)
	}
	if len(req.Config.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(req.Config.AllowedTools, " "))
	}
	args = append(args, req.Prompt)
	return args
}

// ── Claude Code stream-json types ─────────────────────────────────────────────

type ccRawEvent struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype,omitempty"`
	Message json.RawMessage `json:"message,omitempty"`
	// result fields
	Result     string   `json:"result,omitempty"`
	IsError    bool     `json:"is_error,omitempty"`
	DurationMS int64    `json:"duration_ms,omitempty"`
	TotalCost  float64  `json:"total_cost_usd,omitempty"`
	Usage      *ccUsage `json:"usage,omitempty"`
}

type ccMessage struct {
	ID      string           `json:"id"`
	Role    string           `json:"role"`
	Content []ccContentBlock `json:"content"`
	Usage   *ccUsage         `json:"usage,omitempty"`
}

type ccContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type ccUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// emitFn is the signature for the emit function passed to message processors.
// It returns the seq assigned to the emitted event.
type emitFn func(kind agent.EventKind, payload any, parentSeq *int) int

// ── Message processors ────────────────────────────────────────────────────────

func processAssistantMessage(raw json.RawMessage, emit emitFn, toolUseSeqs map[string]int) {
	if len(raw) == 0 {
		return
	}
	var msg ccMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				emit(agent.EventText, agent.TextData{Content: block.Text}, nil)
			}
		case "tool_use":
			data := agent.ToolUseData{
				ToolID: block.ID,
				Name:   block.Name,
				Input:  block.Input,
			}
			usedSeq := emit(agent.EventToolUse, data, nil)
			toolUseSeqs[block.ID] = usedSeq
		}
	}
}

func processUserMessage(raw json.RawMessage, emit emitFn, toolUseSeqs map[string]int) {
	if len(raw) == 0 {
		return
	}
	var msg ccMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	for _, block := range msg.Content {
		if block.Type != "tool_result" {
			continue
		}
		// Content may be a string or an array; normalise to string.
		content := rawContentToString(block.Content)
		var parentSeq *int
		if s, ok := toolUseSeqs[block.ToolUseID]; ok {
			n := s
			parentSeq = &n
		}
		emit(agent.EventToolResult, agent.ToolResultData{
			ToolID:  block.ToolUseID,
			Content: content,
			IsError: block.IsError,
		}, parentSeq)
	}
}

func processResult(raw ccRawEvent, startTime time.Time, emit emitFn) {
	elapsed := time.Since(startTime)
	if raw.IsError {
		emit(agent.EventFailed, agent.FailedData{Error: raw.Result}, nil)
		return
	}
	var inputTokens, outputTokens int
	if raw.Usage != nil {
		inputTokens = raw.Usage.InputTokens
		outputTokens = raw.Usage.OutputTokens
	}
	durationMS := raw.DurationMS
	if durationMS == 0 {
		durationMS = elapsed.Milliseconds()
	}
	emit(agent.EventCompleted, agent.CompletedData{
		Result:       raw.Result,
		CostUSD:      raw.TotalCost,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		DurationMS:   durationMS,
	}, nil)
}

// rawContentToString converts a JSON content field to a plain string.
// Claude tool results may be a plain string or an array of content blocks.
func rawContentToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try plain string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Try array of content blocks.
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}
