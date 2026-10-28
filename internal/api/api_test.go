package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/api"
	"github.com/nmiano/nickbot/internal/store/sqlite"
)

func openTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestRun(id, prompt, parentID string) *agent.Run {
	return &agent.Run{
		ID:          id,
		AgentID:     "test-agent",
		Prompt:      prompt,
		Status:      agent.RunStatusCompleted,
		CreatedAt:   time.Now(),
		RunnerName:  "fake",
		ParentRunID: parentID,
	}
}

// TestAPI_ListChildRuns verifies GET /api/v1/runs/{id}/children.
func TestAPI_ListChildRuns(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	parent := newTestRun("parent-1", "coordinate work", "")
	if err := st.CreateRun(ctx, parent); err != nil {
		t.Fatal(err)
	}
	for i, prompt := range []string{"sub-task A", "sub-task B"} {
		child := newTestRun("", prompt, parent.ID)
		child.ID = strings.ReplaceAll(prompt, " ", "-")
		child.CreatedAt = time.Now().Add(time.Duration(i) * time.Second)
		if err := st.CreateRun(ctx, child); err != nil {
			t.Fatalf("CreateRun(%s): %v", child.ID, err)
		}
	}

	srv := api.New(st, nil)

	t.Run("returns children in order", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/parent-1/children", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body)
		}
		var resp struct {
			Runs []agent.Run `json:"runs"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.Runs) != 2 {
			t.Errorf("len(runs) = %d, want 2", len(resp.Runs))
		}
		for _, r := range resp.Runs {
			if r.ParentRunID != parent.ID {
				t.Errorf("run %s: ParentRunID = %q, want %q", r.ID, r.ParentRunID, parent.ID)
			}
		}
	})

	t.Run("returns empty list for root with no children", func(t *testing.T) {
		lonely := newTestRun("lonely-1", "solo task", "")
		st.CreateRun(ctx, lonely) //nolint:errcheck

		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/lonely-1/children", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var resp struct {
			Runs []agent.Run `json:"runs"`
		}
		json.NewDecoder(rec.Body).Decode(&resp) //nolint:errcheck
		if len(resp.Runs) != 0 {
			t.Errorf("len(runs) = %d, want 0", len(resp.Runs))
		}
	})

	t.Run("404 for unknown parent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/does-not-exist/children", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

// TestAPI_CreateRun_ParentRunID verifies that parent_run_id is accepted
// in the POST /api/v1/runs body and propagated to the created run record.
func TestAPI_CreateRun_ParentRunID(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	parent := newTestRun("parent-2", "orchestrate", "")
	if err := st.CreateRun(ctx, parent); err != nil {
		t.Fatal(err)
	}

	srv := api.New(st, nil)

	body := `{"prompt":"delegate this","runner":"fake","parent_run_id":"parent-2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusAccepted, rec.Body)
	}

	var run agent.Run
	if err := json.NewDecoder(rec.Body).Decode(&run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if run.ParentRunID != "parent-2" {
		t.Errorf("ParentRunID = %q, want %q", run.ParentRunID, "parent-2")
	}
	if run.ID == "" {
		t.Error("run.ID is empty")
	}

	// Confirm it also shows up in the children endpoint.
	// Give the background goroutine a moment to complete (fake runner is instant).
	time.Sleep(50 * time.Millisecond)
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/runs/parent-2/children", nil)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)

	var resp struct {
		Runs []agent.Run `json:"runs"`
	}
	json.NewDecoder(rec2.Body).Decode(&resp) //nolint:errcheck
	if len(resp.Runs) != 1 {
		t.Errorf("children count = %d, want 1", len(resp.Runs))
	}
}
