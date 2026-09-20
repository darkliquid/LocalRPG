// pkg/gui/server_test.go
package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestDiscoveryAndCreationEndpoints(t *testing.T) {
	tmpDir := t.TempDir()

	// Scaffold mock systems and worlds in tmpDir
	sysDir := filepath.Join(tmpDir, "systems", "mock-sys")
	_ = os.MkdirAll(sysDir, 0755)
	_ = os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: mock-sys\nname: Mock System\ndescription: A test system\nversion: 1.0.0\n"), 0644)

	worldDir := filepath.Join(tmpDir, "worlds", "mock-world")
	_ = os.MkdirAll(filepath.Join(worldDir, "entities"), 0755)
	_ = os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: mock-world\nname: Mock World\ndescription: A test world\ngenre: Fantasy\ncompatible_systems: [mock-sys]\n"), 0644)

	svc := NewService(tmpDir)
	server := NewServer(svc, AssetHandler())

	// 1. Test GET /api/systems
	reqSys := httptest.NewRequest("GET", "/api/systems", nil)
	recSys := httptest.NewRecorder()
	server.ServeHTTP(recSys, reqSys)
	if recSys.Code != http.StatusOK {
		t.Fatalf("GET /api/systems expected 200, got %d", recSys.Code)
	}

	var systems []SystemSummaryDTO
	if err := json.NewDecoder(recSys.Body).Decode(&systems); err != nil {
		t.Fatalf("decode systems failed: %v", err)
	}
	if len(systems) != 1 || systems[0].ID != "mock-sys" {
		t.Errorf("unexpected systems: %+v", systems)
	}

	// 2. Test GET /api/worlds
	reqWorld := httptest.NewRequest("GET", "/api/worlds", nil)
	recWorld := httptest.NewRecorder()
	server.ServeHTTP(recWorld, reqWorld)
	if recWorld.Code != http.StatusOK {
		t.Fatalf("GET /api/worlds expected 200, got %d", recWorld.Code)
	}

	var worlds []WorldSummaryDTO
	if err := json.NewDecoder(recWorld.Body).Decode(&worlds); err != nil {
		t.Fatalf("decode worlds failed: %v", err)
	}
	if len(worlds) != 1 || worlds[0].ID != "mock-world" {
		t.Errorf("unexpected worlds: %+v", worlds)
	}

	// 3. Test POST /api/games (create new game)
	createBody := strings.NewReader(`{
		"name": "My Epic Campaign",
		"system_id": "mock-sys",
		"world_id": "mock-world",
		"player_name": "Valerius"
	}`)
	reqCreate := httptest.NewRequest("POST", "/api/games", createBody)
	recCreate := httptest.NewRecorder()
	server.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("POST /api/games expected 201, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var createdGame GameSummaryDTO
	if err := json.NewDecoder(recCreate.Body).Decode(&createdGame); err != nil {
		t.Fatalf("decode created game failed: %v", err)
	}
	if createdGame.Name != "My Epic Campaign" || createdGame.PlayerName != "Valerius" {
		t.Errorf("unexpected created game: %+v", createdGame)
	}

	// 4. Test GET /api/games (should now contain created game)
	reqGames := httptest.NewRequest("GET", "/api/games", nil)
	recGames := httptest.NewRecorder()
	server.ServeHTTP(recGames, reqGames)
	if recGames.Code != http.StatusOK {
		t.Fatalf("GET /api/games expected 200, got %d", recGames.Code)
	}

	var games []GameSummaryDTO
	if err := json.NewDecoder(recGames.Body).Decode(&games); err != nil {
		t.Fatalf("decode games failed: %v", err)
	}
	if len(games) != 1 || games[0].ID != createdGame.ID {
		t.Errorf("expected 1 game in list, got %+v", games)
	}
}


