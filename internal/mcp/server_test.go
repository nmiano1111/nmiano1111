package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/orchestration"
	"github.com/nmiano/nickbot/internal/runners/fake"
	"github.com/nmiano/nickbot/internal/store/sqlite"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makeOrch(t *testing.T) *orchestration.Orchestrator {
	t.Helper()
	path := t.TempDir() + "/test.db"
	st, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	runner := &fake.Runner{}
	return orchestration.New(runner, st, nil)
}

func rpcMsg(obj any) []byte {
	body, _ := json.Marshal(obj)
	return []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
}

func runSession(t *testing.T, orch *orchestration.Orchestrator, requests ...any) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	for _, r := range requests {
		buf.Write(rpcMsg(r))
	}
	var out bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, orch, &buf, &out)
	}()

	// The server exits when stdin is exhausted.
	<-done

	// Parse all responses from the output.
	var results []map[string]any
	raw := out.String()
	for _, chunk := range strings.Split(raw, "Content-Length:") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		parts := strings.SplitN(chunk, "\r\n\r\n", 2)
		if len(parts) < 2 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(parts[1]), &m); err == nil {
			results = append(results, m)
		}
	}
	return results
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestMCPInitialize(t *testing.T) {
	orch := makeOrch(t)
	results := runSession(t, orch,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}},
	)
	if len(results) != 1 {
		t.Fatalf("expected 1 response, got %d", len(results))
	}
	res := results[0]
	if res["error"] != nil {
		t.Fatalf("unexpected error: %v", res["error"])
	}
	result, ok := res["result"].(map[string]any)
	if !ok {
		t.Fatalf("result is not an object: %v", res["result"])
	}
	if result["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v, want %s", result["protocolVersion"], protocolVersion)
	}
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != "nickbot" {
		t.Errorf("serverInfo.name = %v, want nickbot", info["name"])
	}
}

func TestMCPToolsList(t *testing.T) {
	orch := makeOrch(t)
	results := runSession(t, orch,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}},
	)
	if len(results) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(results))
	}
	toolsRes := results[1]
	result, _ := toolsRes["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) == 0 {
		t.Fatal("expected at least one tool")
	}
	// Check all expected tools are present.
	names := map[string]bool{}
	for _, tool := range tools {
		if m, ok := tool.(map[string]any); ok {
			names[m["name"].(string)] = true
		}
	}
	for _, want := range []string{"start_run", "get_run", "list_runs", "get_events"} {
		if !names[want] {
			t.Errorf("tool %q not found in tools/list", want)
		}
	}
}

func TestMCPToolsCallGetRunNotFound(t *testing.T) {
	orch := makeOrch(t)
	results := runSession(t, orch,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}},
		map[string]any{
			"jsonrpc": "2.0", "id": 2, "method": "tools/call",
			"params": map[string]any{
				"name":      "get_run",
				"arguments": map[string]any{"run_id": "nonexistent"},
			},
		},
	)
	if len(results) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(results))
	}
	callRes := results[1]
	if callRes["error"] != nil {
		t.Fatalf("unexpected RPC error: %v", callRes["error"])
	}
	result, _ := callRes["result"].(map[string]any)
	// Tool errors are returned in the result with isError=true.
	if result["isError"] != true {
		t.Errorf("expected isError=true for not-found run, got result=%v", result)
	}
}

func TestMCPToolsCallListRuns(t *testing.T) {
	orch := makeOrch(t)

	// Start a run with the fake runner to populate the DB.
	exec, err := orch.Start(context.Background(), agent.RunRequest{
		AgentID: "test",
		Prompt:  "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	for range exec.Events {
	} // drain

	results := runSession(t, orch,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}},
		map[string]any{
			"jsonrpc": "2.0", "id": 2, "method": "tools/call",
			"params": map[string]any{
				"name":      "list_runs",
				"arguments": map[string]any{"limit": 10},
			},
		},
	)
	if len(results) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(results))
	}
	callRes := results[1]
	result, _ := callRes["result"].(map[string]any)
	if result["isError"] == true {
		t.Errorf("unexpected tool error: %v", result)
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("expected content in list_runs response")
	}
	text, _ := content[0].(map[string]any)
	if !strings.Contains(fmt.Sprint(text["text"]), exec.Run.ID[:8]) {
		t.Errorf("expected run ID in list output, got: %v", text["text"])
	}
}

func TestMCPMethodNotFound(t *testing.T) {
	orch := makeOrch(t)
	results := runSession(t, orch,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "nonexistent/method", "params": map[string]any{}},
	)
	if len(results) != 1 {
		t.Fatalf("expected 1 response, got %d", len(results))
	}
	if results[0]["error"] == nil {
		t.Error("expected JSON-RPC error for unknown method")
	}
}

func TestMCPPing(t *testing.T) {
	orch := makeOrch(t)
	results := runSession(t, orch,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping", "params": map[string]any{}},
	)
	if len(results) != 1 {
		t.Fatalf("expected 1 response, got %d", len(results))
	}
	if results[0]["error"] != nil {
		t.Errorf("unexpected error on ping: %v", results[0]["error"])
	}
}

func TestMCPSchemaNoRequiredField(t *testing.T) {
	// Ensure _required is not leaked into the output schema.
	orch := makeOrch(t)
	results := runSession(t, orch,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{}},
	)
	raw, _ := json.Marshal(results)
	if strings.Contains(string(raw), "_required") {
		t.Error("_required leaked into tools/list schema output")
	}
}

func TestMCPResourcesList(t *testing.T) {
	orch := makeOrch(t)

	// Populate a run.
	exec, err := orch.Start(context.Background(), agent.RunRequest{
		AgentID: "test",
		Prompt:  "hello resources",
	})
	if err != nil {
		t.Fatal(err)
	}
	for range exec.Events {
	}

	results := runSession(t, orch,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "resources/list", "params": map[string]any{}},
	)
	if len(results) != 1 {
		t.Fatalf("expected 1 response, got %d", len(results))
	}
	res := results[0]
	if res["error"] != nil {
		t.Fatalf("unexpected error: %v", res["error"])
	}
	result, _ := res["result"].(map[string]any)
	resources, _ := result["resources"].([]any)
	if len(resources) == 0 {
		t.Fatal("expected at least one resource")
	}
	// Check the first resource has the expected URI prefix.
	first, _ := resources[0].(map[string]any)
	uri, _ := first["uri"].(string)
	if !strings.HasPrefix(uri, "nickbot://runs/") {
		t.Errorf("resource URI = %q, want prefix nickbot://runs/", uri)
	}
}

func TestMCPResourcesReadBadURI(t *testing.T) {
	orch := makeOrch(t)
	results := runSession(t, orch,
		map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "resources/read",
			"params": map[string]any{"uri": "not-a-nickbot-uri"},
		},
	)
	if len(results) != 1 {
		t.Fatalf("expected 1 response, got %d", len(results))
	}
	if results[0]["error"] == nil {
		t.Error("expected RPC error for bad URI, got nil")
	}
}

func TestMCPResourcesReadNotFound(t *testing.T) {
	orch := makeOrch(t)
	results := runSession(t, orch,
		map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "resources/read",
			"params": map[string]any{"uri": "nickbot://runs/nonexistent-id"},
		},
	)
	if len(results) != 1 {
		t.Fatalf("expected 1 response, got %d", len(results))
	}
	if results[0]["error"] == nil {
		t.Error("expected RPC error for non-existent run, got nil")
	}
}

func TestMCPResourcesRead(t *testing.T) {
	orch := makeOrch(t)

	exec, err := orch.Start(context.Background(), agent.RunRequest{
		AgentID: "test",
		Prompt:  "read me",
	})
	if err != nil {
		t.Fatal(err)
	}
	for range exec.Events {
	}
	runID := exec.Run.ID

	results := runSession(t, orch,
		map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "resources/read",
			"params": map[string]any{"uri": "nickbot://runs/" + runID},
		},
	)
	if len(results) != 1 {
		t.Fatalf("expected 1 response, got %d", len(results))
	}
	res := results[0]
	if res["error"] != nil {
		t.Fatalf("unexpected error: %v", res["error"])
	}
	result, _ := res["result"].(map[string]any)
	contents, _ := result["contents"].([]any)
	if len(contents) == 0 {
		t.Fatal("expected contents in resources/read response")
	}
	content, _ := contents[0].(map[string]any)
	text, _ := content["text"].(string)
	if !strings.Contains(text, runID) {
		t.Errorf("expected run ID in resource text, got: %s", text)
	}
}
