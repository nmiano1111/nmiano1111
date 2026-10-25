package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/orchestration"
	"github.com/nmiano/nickbot/internal/store"
)

// toolDefinitions returns the MCP tool manifest for all exposed tools.
func toolDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "start_run",
			"description": "Start an agent run and wait for it to complete. Returns the run result.",
			"inputSchema": schema(map[string]any{
				"prompt":       prop("string", "The instruction to send to the agent", true),
				"agent_id":     prop("string", "Logical agent identifier (default: claude-code)", false),
				"model":        prop("string", "LLM model override (e.g. claude-sonnet-4-6)", false),
				"workdir":      prop("string", "Working directory for tool execution", false),
				"parent_run_id": prop("string", "Parent run ID, if this is a child/delegated run", false),
			}),
		},
		{
			"name":        "get_run",
			"description": "Retrieve a run record by ID, including status and result.",
			"inputSchema": schema(map[string]any{
				"run_id": prop("string", "Run ID to retrieve", true),
			}),
		},
		{
			"name":        "list_runs",
			"description": "List recent agent runs ordered by creation time (newest first).",
			"inputSchema": schema(map[string]any{
				"limit": prop("integer", "Maximum number of runs to return (default: 10)", false),
			}),
		},
		{
			"name":        "get_events",
			"description": "Retrieve the event log for a run, ordered by sequence number.",
			"inputSchema": schema(map[string]any{
				"run_id":    prop("string", "Run ID to retrieve events for", true),
				"after_seq": prop("integer", "Return only events with seq > after_seq", false),
			}),
		},
	}
}

// callTool dispatches a tool call to the appropriate handler.
func callTool(ctx context.Context, orch *orchestration.Orchestrator, name string, args map[string]any) (string, error) {
	switch name {
	case "start_run":
		return toolStartRun(ctx, orch, args)
	case "get_run":
		return toolGetRun(ctx, orch, args)
	case "list_runs":
		return toolListRuns(ctx, orch, args)
	case "get_events":
		return toolGetEvents(ctx, orch, args)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// ── Tool handlers ─────────────────────────────────────────────────────────────

func toolStartRun(ctx context.Context, orch *orchestration.Orchestrator, args map[string]any) (string, error) {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	agentID, _ := args["agent_id"].(string)
	model, _ := args["model"].(string)
	workdir, _ := args["workdir"].(string)
	parentRunID, _ := args["parent_run_id"].(string)

	req := agent.RunRequest{
		AgentID:     agentID,
		Prompt:      prompt,
		ParentRunID: parentRunID,
		Config: agent.RunConfig{
			Model:   model,
			WorkDir: workdir,
		},
	}

	exec, err := orch.Start(ctx, req)
	if err != nil {
		return "", fmt.Errorf("start run: %w", err)
	}

	// Drain events (they are persisted by the orchestrator; we just wait for completion).
	for range exec.Events {
	}

	// Fetch the final run record.
	run, err := orch.GetRun(ctx, exec.Run.ID)
	if err != nil {
		return "", fmt.Errorf("get run: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Run ID:  %s\n", run.ID)
	fmt.Fprintf(&sb, "Status:  %s\n", run.Status)
	if run.Result != "" {
		fmt.Fprintf(&sb, "Result:\n%s\n", run.Result)
	}
	if run.Error != "" {
		fmt.Fprintf(&sb, "Error:   %s\n", run.Error)
	}
	if run.CostUSD > 0 {
		fmt.Fprintf(&sb, "Cost:    $%.4f\n", run.CostUSD)
	}
	if run.InputTokens+run.OutputTokens > 0 {
		fmt.Fprintf(&sb, "Tokens:  %d in / %d out\n", run.InputTokens, run.OutputTokens)
	}
	return sb.String(), nil
}

func toolGetRun(ctx context.Context, orch *orchestration.Orchestrator, args map[string]any) (string, error) {
	runID, _ := args["run_id"].(string)
	if runID == "" {
		return "", fmt.Errorf("run_id is required")
	}
	run, err := orch.GetRun(ctx, runID)
	if err != nil {
		if _, ok := err.(store.ErrNotFound); ok {
			return "", fmt.Errorf("run not found: %s", runID)
		}
		return "", err
	}
	b, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func toolListRuns(ctx context.Context, orch *orchestration.Orchestrator, args map[string]any) (string, error) {
	limit := 10
	if l, ok := args["limit"]; ok {
		switch v := l.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}
	runs, err := orch.ListRuns(ctx, limit)
	if err != nil {
		return "", err
	}
	if len(runs) == 0 {
		return "No runs found.", nil
	}
	var sb strings.Builder
	for _, r := range runs {
		ts := r.CreatedAt.Format("2006-01-02 15:04:05")
		prompt := r.Prompt
		if len(prompt) > 60 {
			prompt = prompt[:57] + "..."
		}
		fmt.Fprintf(&sb, "[%s]  %-9s  %s  %s\n", r.ID[:8], r.Status, ts, prompt)
	}
	return sb.String(), nil
}

func toolGetEvents(ctx context.Context, orch *orchestration.Orchestrator, args map[string]any) (string, error) {
	runID, _ := args["run_id"].(string)
	if runID == "" {
		return "", fmt.Errorf("run_id is required")
	}
	afterSeq := -1
	if a, ok := args["after_seq"]; ok {
		switch v := a.(type) {
		case float64:
			afterSeq = int(v)
		case int:
			afterSeq = v
		}
	}

	events, err := orch.ListEvents(ctx, runID)
	if err != nil {
		return "", err
	}
	if len(events) == 0 {
		return "No events found.", nil
	}

	var sb strings.Builder
	for _, e := range events {
		if e.Seq <= afterSeq {
			continue
		}
		parentInfo := ""
		if e.ParentSeq != nil {
			parentInfo = fmt.Sprintf(" (parent: %d)", *e.ParentSeq)
		}
		ts := e.Timestamp.Format("15:04:05.000")
		fmt.Fprintf(&sb, "[%d]%s  %s  %-12s  %s\n",
			e.Seq, parentInfo, ts, e.Kind, summariseEventData(e))
	}
	return sb.String(), nil
}

// ── Schema helpers ────────────────────────────────────────────────────────────

func schema(properties map[string]any) map[string]any {
	var required []string
	for k, v := range properties {
		if p, ok := v.(map[string]any); ok {
			if req, _ := p["_required"].(bool); req {
				required = append(required, k)
			}
			delete(p, "_required") // always remove the internal sentinel
		}
	}
	s := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func prop(typ, description string, required bool) map[string]any {
	return map[string]any{
		"type":        typ,
		"description": description,
		"_required":   required,
	}
}

func summariseEventData(e *agent.Event) string {
	if len(e.Data) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(e.Data, &m); err != nil {
		return ""
	}
	switch e.Kind {
	case agent.EventText:
		if c, ok := m["content"].(string); ok {
			c = strings.TrimSpace(strings.ReplaceAll(c, "\n", " "))
			if len(c) > 60 {
				return c[:57] + "..."
			}
			return c
		}
	case agent.EventToolUse:
		name, _ := m["name"].(string)
		return name
	case agent.EventToolResult:
		if isErr, _ := m["is_error"].(bool); isErr {
			return "[error]"
		}
		if c, ok := m["content"].(string); ok {
			c = strings.TrimSpace(strings.ReplaceAll(c, "\n", " "))
			if len(c) > 60 {
				return c[:57] + "..."
			}
			return c
		}
	case agent.EventCompleted:
		if result, ok := m["result"].(string); ok {
			if len(result) > 60 {
				return result[:57] + "..."
			}
			return result
		}
	case agent.EventFailed:
		if errMsg, ok := m["error"].(string); ok {
			return errMsg
		}
	}
	return ""
}
