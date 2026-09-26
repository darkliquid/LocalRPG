package gui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

type stubPortraitImageClient struct{ data []byte }

func (s stubPortraitImageClient) GenerateImage(context.Context, string) ([]byte, error) {
	return s.data, nil
}

func TestRegenerateCharacterPortraitEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)
	setupFreeformSystem(t, svc)
	game, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name: "Portrait Regen", SystemID: "freeform", WorldID: "harbour-realm",
		PlayerName: "Hero Vance",
		Player:     PlayerCharacterDTO{Appearance: "A tall adventurer.", Age: "30"},
	})
	if err != nil {
		t.Fatalf("CreateGame failed: %v", err)
	}

	cfg, _ := svc.GetSettings(context.Background())
	cfg.Config.Media.Image = config.ImageConfig{Type: "builtin", BuiltinName: "echo"}
	if _, err := svc.SaveSettings(context.Background(), cfg.Config); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	prev := imageClientFactory
	imageClientFactory = func(config.ImageConfig, string) (media.ImageClient, error) {
		return stubPortraitImageClient{data: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}}, nil
	}
	t.Cleanup(func() { imageClientFactory = prev })

	server := NewServer(svc, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/game/"+game.ID+"/character/hero-vance/portrait", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var dto CharacterPortraitDTO
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatalf("decode DTO: %v", err)
	}
	if dto.PortraitURL == "" {
		t.Fatalf("expected a portrait URL, got %+v", dto)
	}
	if _, err := os.Stat(filepath.Join(svc.GetResolver().GameDir(game.ID), "assets", "portraits", "hero-vance.png")); err != nil {
		t.Fatalf("expected the regenerated portrait on disk: %v", err)
	}
}

func TestRegenerateCharacterPortraitDisabledProvider(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)
	setupFreeformSystem(t, svc)
	game, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name: "Portrait Disabled", SystemID: "freeform", WorldID: "harbour-realm",
		PlayerName: "Hero Vance",
	})
	if err != nil {
		t.Fatalf("CreateGame failed: %v", err)
	}

	server := NewServer(svc, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/game/"+game.ID+"/character/hero-vance/portrait", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for a disabled image provider, got %d: %s", w.Code, w.Body.String())
	}
}
