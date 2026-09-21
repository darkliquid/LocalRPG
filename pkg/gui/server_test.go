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

func TestSystemAndWorldStudioCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)
	server := NewServer(svc, AssetHandler())

	// 1. Create a new system via POST /api/systems
	sysPayload := `{
		"name": "Custom 2d6",
		"version": "1.0.0",
		"description": "Narrative two-dice resolution",
		"rules_prompt": "Evaluate rolls on a 2d6 ladder. 10+ is full success, 7-9 is partial success, 6- is failure.",
		"script": "function evaluateRoll(stats, dice) { return { total: 12 }; }"
	}`
	reqSys := httptest.NewRequest("POST", "/api/systems", strings.NewReader(sysPayload))
	recSys := httptest.NewRecorder()
	server.ServeHTTP(recSys, reqSys)
	if recSys.Code != http.StatusCreated {
		t.Fatalf("POST /api/systems failed (%d): %s", recSys.Code, recSys.Body.String())
	}

	var createdSys SystemDetailDTO
	_ = json.NewDecoder(recSys.Body).Decode(&createdSys)
	if createdSys.ID != "custom-2d6" || createdSys.Name != "Custom 2d6" {
		t.Errorf("unexpected created system: %+v", createdSys)
	}

	// 2. Fetch system detail via GET /api/system/custom-2d6
	reqGetSys := httptest.NewRequest("GET", "/api/system/custom-2d6", nil)
	recGetSys := httptest.NewRecorder()
	server.ServeHTTP(recGetSys, reqGetSys)
	if recGetSys.Code != http.StatusOK {
		t.Fatalf("GET /api/system/custom-2d6 failed (%d): %s", recGetSys.Code, recGetSys.Body.String())
	}
	var fetchedSys SystemDetailDTO
	_ = json.NewDecoder(recGetSys.Body).Decode(&fetchedSys)
	if !strings.Contains(fetchedSys.Script, "evaluateRoll") {
		t.Errorf("expected script in system detail, got: %s", fetchedSys.Script)
	}
	if !strings.Contains(fetchedSys.RulesPrompt, "2d6 ladder") {
		t.Errorf("expected rules_prompt in system detail, got: %s", fetchedSys.RulesPrompt)
	}

	// 3. Create a new world via POST /api/worlds
	worldPayload := `{
		"name": "The Sunken Bastion",
		"description": "An underwater gothic citadel",
		"genre": "Aquatic Gothic",
		"default_system": "custom-2d6",
		"art_style": "Moody oil painting with deep teal and amber lighting",
		"lore_prompt": "The sunken citadel smells of brine and ancient kelp.",
		"tags": ["gothic", "ocean"]
	}`
	reqWorld := httptest.NewRequest("POST", "/api/worlds", strings.NewReader(worldPayload))
	recWorld := httptest.NewRecorder()
	server.ServeHTTP(recWorld, reqWorld)
	if recWorld.Code != http.StatusCreated {
		t.Fatalf("POST /api/worlds failed (%d): %s", recWorld.Code, recWorld.Body.String())
	}

	var createdWorld WorldDetailDTO
	_ = json.NewDecoder(recWorld.Body).Decode(&createdWorld)
	if createdWorld.ID != "the-sunken-bastion" || createdWorld.DefaultSystem != "custom-2d6" {
		t.Errorf("unexpected created world: %+v", createdWorld)
	}

	// 4. Create starter entity in world via PUT /api/world/the-sunken-bastion/entity/sunken_throne
	entityMD := "---\nname: The Sunken Throne\ntype: location\n---\nAncient seat of forgotten sea kings."
	reqEnt := httptest.NewRequest("PUT", "/api/world/the-sunken-bastion/entity/sunken_throne", strings.NewReader(entityMD))
	recEnt := httptest.NewRecorder()
	server.ServeHTTP(recEnt, reqEnt)
	if recEnt.Code != http.StatusOK {
		t.Fatalf("PUT world entity failed (%d): %s", recEnt.Code, recEnt.Body.String())
	}

	// 5. Fetch world detail via GET /api/world/the-sunken-bastion
	reqGetWorld := httptest.NewRequest("GET", "/api/world/the-sunken-bastion", nil)
	recGetWorld := httptest.NewRecorder()
	server.ServeHTTP(recGetWorld, reqGetWorld)
	if recGetWorld.Code != http.StatusOK {
		t.Fatalf("GET /api/world failed: %d", recGetWorld.Code)
	}
	var fetchedWorld WorldDetailDTO
	_ = json.NewDecoder(recGetWorld.Body).Decode(&fetchedWorld)
	if len(fetchedWorld.Entities) != 1 || fetchedWorld.Entities[0].ID != "sunken_throne" {
		t.Errorf("expected 1 entity in world detail, got %+v", fetchedWorld.Entities)
	}
	if !strings.Contains(fetchedWorld.LorePrompt, "sunken citadel") {
		t.Errorf("expected lore_prompt in world detail, got: %s", fetchedWorld.LorePrompt)
	}

	// 6. Fetch entity markdown via GET /api/world/the-sunken-bastion/entity/sunken_throne
	reqGetEnt := httptest.NewRequest("GET", "/api/world/the-sunken-bastion/entity/sunken_throne", nil)
	recGetEnt := httptest.NewRecorder()
	server.ServeHTTP(recGetEnt, reqGetEnt)
	if recGetEnt.Code != http.StatusOK {
		t.Fatalf("GET world entity failed (%d)", recGetEnt.Code)
	}
	var fetchedEnt WorldEntityDetailDTO
	_ = json.NewDecoder(recGetEnt.Body).Decode(&fetchedEnt)
	if !strings.Contains(fetchedEnt.Markdown, "Ancient seat of forgotten sea kings") {
		t.Errorf("unexpected entity markdown: %s", fetchedEnt.Markdown)
	}
}

func TestSettingsEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)
	server := NewServer(svc, nil)

	// 1. GET /api/settings
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from GET /api/settings, got %d: %s", w.Code, w.Body.String())
	}

	var res SettingsResponseDTO
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode settings: %v", err)
	}
	if res.Config.Paths.Systems == "" {
		t.Errorf("expected non-empty systems path")
	}

	// 2. PUT /api/settings
	newSysPath := filepath.Join(tmpDir, "new_systems")
	res.Config.Paths.Systems = newSysPath
	putBody, _ := json.Marshal(res)
	req2 := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(string(putBody)))
	w2 := httptest.NewRecorder()
	server.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from PUT /api/settings, got %d: %s", w2.Code, w2.Body.String())
	}

	// Verify path resolver dynamically updated
	if svc.GetResolver().SystemsDir() != newSysPath {
		t.Errorf("expected service to dynamically update systems dir to %s, got %s", newSysPath, svc.GetResolver().SystemsDir())
	}

	// 3. POST /api/settings/test-provider
	testReqBody := `{"category":"llm","provider":{"type":"cli","command":"echo","args":["pong"]},"test_prompt":"ping"}`
	req3 := httptest.NewRequest(http.MethodPost, "/api/settings/test-provider", strings.NewReader(testReqBody))
	w3 := httptest.NewRecorder()
	server.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from POST /api/settings/test-provider, got %d: %s", w3.Code, w3.Body.String())
	}
	var testRes TestProviderResponseDTO
	_ = json.Unmarshal(w3.Body.Bytes(), &testRes)
	if !testRes.Success {
		t.Errorf("expected test provider success: %s", testRes.Message)
	}
}

func TestEntityTurnsRoute(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/entity/captain-kaelen/turns", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
