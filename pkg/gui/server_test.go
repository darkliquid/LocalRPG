// pkg/gui/server_test.go
package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGUIServerRoutes(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	// Test GET /api/game/:id/state
	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/state", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var state GameStateDTO
	if err := json.NewDecoder(rec.Body).Decode(&state); err != nil {
		t.Fatalf("decode state failed: %v", err)
	}
	if state.Player.Name != "Elena Nightshade" {
		t.Errorf("expected Elena Nightshade, got %s", state.Player.Name)
	}

	// Test GET /api/game/:id/graph
	req = httptest.NewRequest("GET", "/api/game/"+gameID+"/graph", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var graph GraphDTO
	if err := json.NewDecoder(rec.Body).Decode(&graph); err != nil {
		t.Fatalf("decode graph failed: %v", err)
	}
	if len(graph.Nodes) < 2 {
		t.Errorf("expected nodes in graph, got %d", len(graph.Nodes))
	}
}

func TestSPARouting(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	server := NewServer(svc, AssetHandler())

	// Test GET / returns HTML
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("expected text/html content type for /, got %s", contentType)
	}

	// Test GET /chronicle (client-side route) also returns HTML
	reqRoute := httptest.NewRequest("GET", "/chronicle", nil)
	wRoute := httptest.NewRecorder()
	server.ServeHTTP(wRoute, reqRoute)

	if wRoute.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /chronicle, got %d", wRoute.Code)
	}
	if !strings.Contains(wRoute.Header().Get("Content-Type"), "text/html") {
		t.Errorf("expected text/html for client route, got %s", wRoute.Header().Get("Content-Type"))
	}
}

