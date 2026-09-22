// pkg/gui/server_test.go
package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
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

func TestLocationArtRoute(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/location/aldon-harbour/art", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("Content-Type = %q, want image/svg+xml", ct)
	}
}

func TestSegmentAudioRoute(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/turn/1/segment/1/audio", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		t.Errorf("expected audio bytes")
	}
}

func TestSegmentAudioRouteSniffsTheContentType(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/turn/1/segment/1/audio", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// The type is read from the bytes rather than assumed, so a provider that
	// returns MP3 is served as MP3.
	ct := rec.Header().Get("Content-Type")
	if want := media.AudioContentType(rec.Body.Bytes()); ct != want {
		t.Errorf("Content-Type = %q, want %q", ct, want)
	}
	if !strings.HasPrefix(ct, "audio/") {
		t.Errorf("Content-Type = %q, want an audio type", ct)
	}
}

func TestSegmentAudioRouteReportsAnUnavailableProvider(t *testing.T) {
	// A service whose config never enabled TTS.
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	quiet := NewService(t.TempDir())
	if err := os.MkdirAll(quiet.GetResolver().GameDir(gameID), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(quiet.GetResolver().GameDir(gameID), "history.jsonl"),
		mustRead(t, filepath.Join(svc.GetResolver().GameDir(gameID), "history.jsonl")),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	server := NewServer(quiet, http.NotFoundHandler())
	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/turn/1/segment/1/audio", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 when no TTS provider is configured, got %d", rec.Code)
	}
}

func TestTurnEndpointStreamsNDJSON(t *testing.T) {
	gameID, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("POST", "/api/game/"+gameID+"/turn", strings.NewReader(`{"mode":"do","input":"I look around"}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Errorf("Content-Type = %q, want application/x-ndjson", ct)
	}

	var last TurnEvent
	lines := 0
	for _, line := range strings.Split(strings.TrimSpace(rec.Body.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines++

		var event TurnEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("line %d is not JSON: %v\n%s", lines, err, line)
		}
		last = event
	}

	if lines == 0 {
		t.Fatalf("expected at least one event")
	}
	if last.Type != "turn" || last.Turn == nil {
		t.Fatalf("expected a final turn event, got %+v", last)
	}
	if last.Turn.TurnNumber != 1 {
		t.Errorf("TurnNumber = %d, want 1", last.Turn.TurnNumber)
	}
}

func TestTurnEndpointRejectsBadRequests(t *testing.T) {
	gameID, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	cases := []struct {
		name       string
		path       string
		body       string
		wantStatus int
	}{
		{"unknown mode", "/api/game/" + gameID + "/turn", `{"mode":"dance","input":"hello"}`, http.StatusBadRequest},
		{"empty input", "/api/game/" + gameID + "/turn", `{"mode":"do","input":"   "}`, http.StatusBadRequest},
		{"malformed body", "/api/game/" + gameID + "/turn", `{`, http.StatusBadRequest},
		{"unknown game", "/api/game/absent-campaign/turn", `{"mode":"do","input":"hi"}`, http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestTurnEndpointReportsAnUnplayableCampaign(t *testing.T) {
	gameID, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	// A campaign whose manifest names a system that does not exist: playable
	// campaign files are missing, but the game itself is there.
	brokenDir := svc.GetResolver().GameDir("broken-campaign")
	if err := os.MkdirAll(brokenDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "id: broken-campaign\nname: Broken\nsystem: ghost-system\nworld: harbour-realm\nplayer: sean\n"
	if err := os.WriteFile(filepath.Join(brokenDir, "game.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/api/game/broken-campaign/turn", strings.NewReader(`{"mode":"do","input":"hello"}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ghost-system") {
		t.Errorf("expected the missing system named in the error, got %s", rec.Body.String())
	}

	_ = gameID
}

func TestTurnEndpointConflictsWhileATurnIsInFlight(t *testing.T) {
	gameID, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	// Hold the campaign's lock the way an in-flight turn would.
	session, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	defer session.Close()

	req := httptest.NewRequest("POST", "/api/game/"+gameID+"/turn", strings.NewReader(`{"mode":"do","input":"I wait"}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Errorf("expected a Retry-After header")
	}
}

func TestSTTEndpoint_TranscribesAudio(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	cfg, _ := svc.GetSettings(context.Background())
	cfg.Config.Media.STT = config.STTConfig{
		Type: "builtin",
	}
	_, _ = svc.SaveSettings(context.Background(), cfg.Config)

	server := NewServer(svc, http.NotFoundHandler())

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", "speech.webm")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	_, _ = part.Write([]byte("fake-audio-bytes"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/stt", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (%s)", w.Code, w.Body.String())
	}

	var res map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res["text"] == "" {
		t.Errorf("expected non-empty transcribed text")
	}
}

func TestSTTEndpoint_RejectsWhenDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	cfg, _ := svc.GetSettings(context.Background())
	cfg.Config.Media.STT = config.STTConfig{Type: "disabled"}
	_, _ = svc.SaveSettings(context.Background(), cfg.Config)

	server := NewServer(svc, http.NotFoundHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/stt", bytes.NewReader([]byte("audio")))
	req.Header.Set("Content-Type", "audio/webm")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}
func TestCampaignLifecycleRoutes(t *testing.T) {
	gameID, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	// Restart clears the campaign and returns its summary.
	req := httptest.NewRequest(http.MethodPost, "/api/game/"+gameID+"/restart", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("restart: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var summary GameSummaryDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode restart summary: %v", err)
	}
	if summary.ID != gameID {
		t.Errorf("restart summary ID = %q, want %q", summary.ID, gameID)
	}

	// A settings patch round-trips into the state route.
	req = httptest.NewRequest(http.MethodPatch, "/api/game/"+gameID+"/settings", strings.NewReader(`{"opening_prompt":"Begin at dusk."}`))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("settings: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/game/"+gameID+"/state", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	var state GameStateDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.OpeningPrompt != "Begin at dusk." {
		t.Errorf("OpeningPrompt = %q, want the saved prompt", state.OpeningPrompt)
	}

	// Delete removes the campaign; a second delete is a 404.
	req = httptest.NewRequest(http.MethodDelete, "/api/game/"+gameID, nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/game/"+gameID, nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("delete absent: expected 404, got %d", rec.Code)
	}
}
func TestAudioRoutesReportStatusAndStop(t *testing.T) {
	_, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/audio/status", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var status AudioStatusDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Playing {
		t.Errorf("expected nothing to be playing on a fresh service")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/audio/stop", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("stop: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/audio/nonsense", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown audio action: expected 404, got %d", rec.Code)
	}
}

func TestListEntitiesRouteReturnsTheCorpus(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/game/"+gameID+"/entities", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("entities: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var entities []EntitySummaryDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &entities); err != nil {
		t.Fatalf("decode entities: %v", err)
	}

	byID := make(map[string]EntitySummaryDTO, len(entities))
	for _, entity := range entities {
		byID[entity.ID] = entity
	}
	if _, ok := byID["aldon-harbour"]; !ok {
		t.Errorf("expected the location note in the listing, got %+v", entities)
	}
	if byID["aldon-harbour"].Type != "location" {
		t.Errorf("aldon-harbour type = %q, want location", byID["aldon-harbour"].Type)
	}
}
func TestTraceRouteReturnsTheMostRecentEvents(t *testing.T) {
	_, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	tracePath := filepath.Join(svc.GetResolver().CacheDir(), "trace", "trace.jsonl")
	if err := os.MkdirAll(filepath.Dir(tracePath), 0755); err != nil {
		t.Fatal(err)
	}
	lines := `{"ts":"2026-09-22T09:00:00.000Z","event":"turn.begin","level":"summary","game":"other","number":1}
{"ts":"2026-09-22T09:00:01.000Z","event":"context.assembled","level":"summary","game":"campaign-01","tokens":100}
{"ts":"2026-09-22T09:00:02.000Z","event":"record.turn","level":"summary","game":"campaign-01","number":1}
`
	if err := os.WriteFile(tracePath, []byte(lines), 0600); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/trace?limit=2&game=campaign-01", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("trace: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var events []TraceEventDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode trace: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 filtered events, got %d: %+v", len(events), events)
	}
	if events[0].Event != "context.assembled" || events[1].Event != "record.turn" {
		t.Errorf("unexpected ordering: %+v", events)
	}
	if events[1].Fields["number"] != float64(1) {
		t.Errorf("fields must survive the mapping: %+v", events[1].Fields)
	}
	if events[1].Time == "" || events[1].Level != "summary" {
		t.Errorf("the envelope must survive: %+v", events[1])
	}

	// Deleting clears what the view reads.
	req = httptest.NewRequest(http.MethodDelete, "/api/trace", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/trace", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	var after []TraceEventDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Errorf("expected the trace to be empty after delete, got %d", len(after))
	}
}

func TestMergeRouteFoldsOneNoteIntoAnother(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	if err := svc.SaveEntity(context.Background(), gameID, "the-ember-warden",
		"---\nid: the-ember-warden\nname: The Ember Warden\ntype: character\n---\nStands vigil.\n"); err != nil {
		t.Fatal(err)
	}

	body := `{"into":"captain-kaelen"}`
	req := httptest.NewRequest(http.MethodPost, "/api/game/"+gameID+"/entity/the-ember-warden/merge", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("merge: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var merged EntityDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &merged); err != nil {
		t.Fatalf("decode merged entity: %v", err)
	}
	if merged.ID != "captain-kaelen" {
		t.Errorf("merged into %q, want captain-kaelen", merged.ID)
	}

	// A merge with no target is a malformed request, not a silent no-op.
	req = httptest.NewRequest(http.MethodPost, "/api/game/"+gameID+"/entity/captain-kaelen/merge", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for a missing target, got %d", rec.Code)
	}
}
