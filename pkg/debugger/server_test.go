package debugger_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

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

func TestDebuggerServerAutoSynthesizesTurnActions(t *testing.T) {
	collector := debugger.NewCollector(100, 100)
	srv := debugger.NewServer(collector, ":0")

	// Inject a completed turn span directly into the collector (as would happen in manual play)
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(collector))
	defer tp.Shutdown(context.Background())
	tracer := tp.Tracer("test")

	_, span := tracer.Start(context.Background(), "turn")
	span.SetAttributes(
		attribute.String("game.id", "campaign-1"),
		attribute.String("turn.number", "3"),
		attribute.String("turn.mode", "chat"),
		attribute.String("turn.assembled_prompt", "System: act as GM..."),
		attribute.String("turn.raw_completion", "Welcome traveler!"),
	)
	span.End()

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
	if len(actions) != 1 {
		t.Fatalf("Expected 1 auto-synthesized action, got %d", len(actions))
	}
	act := actions[0]
	if act.ActionType != "turn #3" {
		t.Errorf("Expected action type 'turn #3', got %s", act.ActionType)
	}
	if act.Diagnostics == nil || act.Diagnostics.AssembledPrompt != "System: act as GM..." {
		t.Errorf("Expected diagnostics with assembled prompt, got %+v", act.Diagnostics)
	}
}
