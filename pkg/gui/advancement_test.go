package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// advancementFixture is turnFixture with a system that declares an advancement
// currency and one unlock, and a player who can afford it.
func advancementFixture(t *testing.T) (string, *Service) {
	t.Helper()

	gameID, svc := turnFixture(t)
	sysDir := svc.GetResolver().SystemDir("freeform")
	systemYAML := `id: freeform
name: Freeform
version: 1.0
mechanics:
  stats:
    - { id: xp, label: Experience, type: number, default: 0 }
  advancement:
    currency: { stat: xp, label: Experience }
    mode: spend
    unlocks:
      - id: stat-increase
        label: Increase a stat
        cost: 5
        effects:
          - { type: stat_increase, stat: might, amount: 1, max: 3 }
`
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte(systemYAML), 0644); err != nil {
		t.Fatal(err)
	}

	state, err := svc.GetGameState(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGameState: %v", err)
	}
	playerID := state.Player.ID
	path := filepath.Join(svc.GetResolver().GameDir(gameID), "entities", playerID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read player note: %v", err)
	}
	player, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("parse player note: %v", err)
	}
	if player.State == nil {
		player.InitState(map[string]interface{}{})
	}
	if err := player.State.Set("xp", 5); err != nil {
		t.Fatal(err)
	}
	normalised, err := player.SerializeMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveEntity(context.Background(), gameID, playerID, string(normalised)); err != nil {
		t.Fatalf("seed player xp: %v", err)
	}

	return gameID, svc
}

func TestAdvanceEndpointSpendsAnUnlock(t *testing.T) {
	gameID, svc := advancementFixture(t)
	server := NewServer(svc, nil)

	body := strings.NewReader(`{"unlock_id":"stat-increase"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/game/"+gameID+"/advance", body)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	state, err := svc.GetGameState(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGameState: %v", err)
	}
	if state.Advancement == nil || state.Advancement.Value != 0 {
		t.Fatalf("advancement after spend = %+v", state.Advancement)
	}

	// The change must reach the note on disk, not only the index.
	path := filepath.Join(svc.GetResolver().GameDir(gameID), "entities", state.Player.ID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	player, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatal(err)
	}
	if player.State == nil {
		t.Fatal("player note lost its state")
	}
	if xp, _ := player.State.Get("xp"); xp != 0 {
		t.Fatalf("note xp = %v, want 0", xp)
	}
}

func TestAdvanceEndpointRejectsUnaffordableSpend(t *testing.T) {
	gameID, svc := advancementFixture(t)
	server := NewServer(svc, nil)

	// Spend the only currency first.
	first := httptest.NewRequest(http.MethodPost, "/api/game/"+gameID+"/advance", strings.NewReader(`{"unlock_id":"stat-increase"}`))
	firstRec := httptest.NewRecorder()
	server.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first spend status = %d: %s", firstRec.Code, firstRec.Body.String())
	}

	second := httptest.NewRequest(http.MethodPost, "/api/game/"+gameID+"/advance", strings.NewReader(`{"unlock_id":"stat-increase"}`))
	secondRec := httptest.NewRecorder()
	server.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusBadRequest {
		t.Fatalf("second spend status = %d, want 400: %s", secondRec.Code, secondRec.Body.String())
	}
}
