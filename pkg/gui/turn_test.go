package gui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
)

// turnFixture builds a playable campaign: a config whose gm provider is the
// built-in echo engine, a game, a system, and a world.
func turnFixture(t *testing.T) (string, *Service) {
	t.Helper()

	root := t.TempDir()
	configYAML := "agents:\n  roles:\n    gm:\n      type: builtin\n"
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(root)
	paths := svc.GetResolver()

	sysDir := paths.SystemDir("freeform")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: freeform\nname: Freeform\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	worldDir := paths.WorldDir("harbour-realm")
	if err := os.MkdirAll(filepath.Join(worldDir, "entities"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: harbour-realm\nname: Harbour Realm\n"), 0644); err != nil {
		t.Fatal(err)
	}

	session, err := engine.InitGame(paths, engine.InitOptions{
		GameID:     "campaign-01",
		SystemID:   "freeform",
		WorldID:    "harbour-realm",
		PlayerName: "Sean",
	})
	if err != nil {
		t.Fatalf("InitGame failed: %v", err)
	}
	_ = session.Close()

	return "campaign-01", svc
}

func TestBeginTurnSerialisesTurns(t *testing.T) {
	gameID, svc := turnFixture(t)

	first, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("first BeginTurn failed: %v", err)
	}
	defer first.Close()

	if _, err := svc.BeginTurn(gameID); !errors.Is(err, ErrTurnInFlight) {
		t.Fatalf("expected ErrTurnInFlight, got %v", err)
	}

	// The lock is released on Close, not before.
	first.Close()

	second, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn after Close failed: %v", err)
	}
	second.Close()
}

func TestBeginTurnRejectsAnUnplayableCampaign(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)

	if _, err := svc.BeginTurn("absent-campaign"); !errors.Is(err, ErrCampaignNotPlayable) {
		t.Errorf("expected ErrCampaignNotPlayable, got %v", err)
	}
}

func TestTurnSessionRunsAndRecordsATurn(t *testing.T) {
	gameID, svc := turnFixture(t)

	session, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	defer session.Close()

	var chunks []string
	var final *TurnDTO
	err = session.Run(context.Background(), TurnRequest{Mode: "Do", Input: "I look around"}, func(event TurnEvent) error {
		switch event.Type {
		case "chunk":
			chunks = append(chunks, event.Text)
		case "turn":
			final = event.Turn
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Errorf("expected streamed chunks from the built-in provider")
	}
	if final == nil {
		t.Fatalf("expected a final turn event")
	}
	if !strings.Contains(final.Prose, "Echo:") {
		t.Errorf("Prose = %q, want the provider's narration", final.Prose)
	}
	if final.LocationID == "" {
		t.Errorf("expected the turn to record where it happened")
	}

	// The live DTO and the replayed one agree, which is the whole point of the
	// shared mapping.
	chronicle, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chronicle) != 1 {
		t.Fatalf("expected 1 recorded turn, got %d", len(chronicle))
	}
	if chronicle[0].Prose != final.Prose || chronicle[0].LocationID != final.LocationID {
		t.Errorf("live and replayed turns differ:\n%+v\n%+v", *final, chronicle[0])
	}
}

func TestTurnSessionRecordsNothingWhenEmitFails(t *testing.T) {
	gameID, svc := turnFixture(t)

	session, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	defer session.Close()

	clientGone := errors.New("client disconnected")
	if err := session.Run(context.Background(), TurnRequest{Mode: "Do", Input: "I look"}, func(TurnEvent) error {
		return clientGone
	}); !errors.Is(err, clientGone) {
		t.Fatalf("expected the emit error, got %v", err)
	}

	chronicle, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chronicle) != 0 {
		t.Errorf("expected nothing recorded, got %+v", chronicle)
	}
}

func TestTurnSessionRunsAnOpeningTurn(t *testing.T) {
	gameID, svc := turnFixture(t)

	session, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	defer session.Close()

	var final *TurnDTO
	if err := session.Run(context.Background(), TurnRequest{Mode: "Opening", Input: ""}, func(event TurnEvent) error {
		if event.Type == "turn" {
			final = event.Turn
		}
		return nil
	}); err != nil {
		t.Fatalf("opening turn failed: %v", err)
	}
	if final == nil {
		t.Fatal("expected the opening turn to be recorded")
	}
	if final.TurnNumber != 1 || final.Mode != engine.OpeningMode {
		t.Errorf("unexpected opening turn: number %d, mode %q", final.TurnNumber, final.Mode)
	}
}

func TestRestartGameClearsHistoryAndKeepsTheCampaign(t *testing.T) {
	gameID, svc := turnFixture(t)

	session, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Run(context.Background(), TurnRequest{Mode: "Do", Input: "I look around"}, func(TurnEvent) error {
		return nil
	}); err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	session.Close()

	restarted, err := svc.RestartGame(context.Background(), gameID)
	if err != nil {
		t.Fatalf("RestartGame failed: %v", err)
	}
	if restarted.TurnCount != 0 {
		t.Errorf("TurnCount = %d, want 0", restarted.TurnCount)
	}

	chronicle, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chronicle) != 0 {
		t.Errorf("expected an empty chronicle after a restart, got %d turns", len(chronicle))
	}

	state, err := svc.GetGameState(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGameState failed after a restart: %v", err)
	}
	if state.Player.Name != "Sean" {
		t.Errorf("Player.Name = %q, want the protagonist preserved", state.Player.Name)
	}
}

func TestDeleteGameRemovesTheCampaign(t *testing.T) {
	gameID, svc := turnFixture(t)

	if err := svc.DeleteGame(context.Background(), gameID); err != nil {
		t.Fatalf("DeleteGame failed: %v", err)
	}

	games, err := svc.ListGames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 0 {
		t.Errorf("expected no campaigns, got %+v", games)
	}
	if _, err := os.Stat(svc.GetResolver().GameDir(gameID)); !os.IsNotExist(err) {
		t.Errorf("expected the campaign directory to be gone, stat err = %v", err)
	}

	if err := svc.DeleteGame(context.Background(), gameID); err == nil {
		t.Errorf("expected deleting an absent campaign to fail")
	}
}

func TestGameSettingsRoundTrip(t *testing.T) {
	gameID, svc := turnFixture(t)

	if err := svc.UpdateGameSettings(context.Background(), gameID, map[string]interface{}{
		engine.OpeningPromptSetting: "Begin in the rain.",
	}); err != nil {
		t.Fatalf("UpdateGameSettings failed: %v", err)
	}

	state, err := svc.GetGameState(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if state.OpeningPrompt != "Begin in the rain." {
		t.Errorf("OpeningPrompt = %q, want the saved prompt", state.OpeningPrompt)
	}
}
