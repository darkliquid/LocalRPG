package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestGameSettingsPatchAndState(t *testing.T) {
	tempDir := t.TempDir()

	gameDir := filepath.Join(tempDir, "games", "settings-game")
	entitiesDir := filepath.Join(gameDir, "entities")
	if err := os.MkdirAll(entitiesDir, 0755); err != nil {
		t.Fatalf("mkdir entities: %v", err)
	}

	gameYAML := "id: settings-game\nname: Settings Game\nsystem: test-sys\nworld: test-world\nplayer: hero\n"
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(gameYAML), 0644); err != nil {
		t.Fatalf("write game.yaml: %v", err)
	}

	playerMD := "---\nid: hero\nname: Hero\ntype: character\n---\n"
	if err := os.WriteFile(filepath.Join(entitiesDir, "hero.md"), []byte(playerMD), 0644); err != nil {
		t.Fatalf("write hero.md: %v", err)
	}

	dbPath := filepath.Join(gameDir, "cache", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	syncer := storage.NewSyncer(store)
	_, _ = syncer.Sync(entitiesDir)
	_ = store.Close()

	svc := NewService(tempDir)
	srv := NewServer(svc, http.NotFoundHandler())

	// Test initial GetGameState has empty narrator_voice and start_location
	state, err := svc.GetGameState(context.Background(), "settings-game")
	if err != nil {
		t.Fatalf("GetGameState: %v", err)
	}
	if state.NarratorVoice != "" {
		t.Errorf("expected empty NarratorVoice, got %q", state.NarratorVoice)
	}
	if state.StartLocation != "" {
		t.Errorf("expected empty StartLocation, got %q", state.StartLocation)
	}

	// Test PATCH /api/game/settings-game/settings
	prompt := "You wake up in a quiet inn."
	voice := "en-US-Standard-A"
	loc := "tavern-inn"
	patchBody := GameSettingsPatchDTO{
		NarratorVoice: &voice,
		StartLocation: &loc,
		OpeningPrompt: &prompt,
	}
	bodyBytes, _ := json.Marshal(patchBody)
	req := httptest.NewRequest(http.MethodPatch, "/api/game/settings-game/settings", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("PATCH settings expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify updated GetGameState
	updatedState, err := svc.GetGameState(context.Background(), "settings-game")
	if err != nil {
		t.Fatalf("GetGameState after patch: %v", err)
	}
	if updatedState.NarratorVoice != "en-US-Standard-A" {
		t.Errorf("expected NarratorVoice 'en-US-Standard-A', got %q", updatedState.NarratorVoice)
	}
	if updatedState.StartLocation != "tavern-inn" {
		t.Errorf("expected StartLocation 'tavern-inn', got %q", updatedState.StartLocation)
	}
	if updatedState.OpeningPrompt != "You wake up in a quiet inn." {
		t.Errorf("expected OpeningPrompt 'You wake up in a quiet inn.', got %q", updatedState.OpeningPrompt)
	}
}
