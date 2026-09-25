package debugger_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/debugger"
)

func TestDebuggerServerEndpoints(t *testing.T) {
	collector := debugger.NewCollector(100, 100)
	srv := debugger.NewServer(collector, ":0")

	// Post an action update
	srv.RecordAction(debugger.ActionRecord{
		ID:         "act-1",
		ActionType: "navigate",
		Status:     "passed",
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/actions", nil)
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	var actions []debugger.ActionRecord
	if err := json.NewDecoder(rec.Body).Decode(&actions); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(actions) != 1 || actions[0].ID != "act-1" {
		t.Errorf("Unexpected actions output: %+v", actions)
	}

	// Test dashboard HTML endpoint
	recUI := httptest.NewRecorder()
	reqUI := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.Handler().ServeHTTP(recUI, reqUI)
	if recUI.Code != http.StatusOK {
		t.Errorf("Expected 200 for dashboard UI, got %d", recUI.Code)
	}
}
