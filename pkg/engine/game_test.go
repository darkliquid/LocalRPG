package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestGameInitAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	paths := core.NewPathResolver(tempDir)

	// 1. Setup system
	sysDir := paths.SystemDir("d20-test")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: d20-test\nname: D20 Test\nversion: 1.0\n"), 0644)

	// 2. Setup world
	worldDir := paths.WorldDir("fantasy-realm")
	worldEntities := filepath.Join(worldDir, "entities")
	if err := os.MkdirAll(worldEntities, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: fantasy-realm\nname: Fantasy Realm\n"), 0644)
	tavernDoc := "---\nid: tavern\nname: Oakhaven Tavern\ntype: location\n---\nStarting tavern."
	os.WriteFile(filepath.Join(worldEntities, "Tavern.md"), []byte(tavernDoc), 0644)
	gafferDoc := "---\nname: Old Gaffer\ntype: character\n---\nAn old storyteller."
	os.WriteFile(filepath.Join(worldEntities, "gaffer.md"), []byte(gafferDoc), 0644)

	// 3. Initialize game
	session, err := InitGame(paths, InitOptions{
		GameID:     "campaign-01",
		SystemID:   "d20-test",
		WorldID:    "fantasy-realm",
		PlayerName: "Sean",
	})
	if err != nil {
		t.Fatalf("InitGame failed: %v", err)
	}
	defer session.Close()

	if session.Manifest.Player != "sean" {
		t.Errorf("expected the player entity ID sean, got %q", session.Manifest.Player)
	}
	if session.Manifest.PlayerName != "Sean" {
		t.Errorf("expected the player display name Sean, got %q", session.Manifest.PlayerName)
	}

	// Verify base entity was copied and indexed into game
	loaded, err := session.Store.GetEntity("tavern")
	if err != nil {
		t.Fatalf("failed to query tavern from game index: %v", err)
	}
	if loaded.Name != "Oakhaven Tavern" {
		t.Errorf("expected Oakhaven Tavern, got %q", loaded.Name)
	}

	// The resolved opening location is recorded on the manifest and persisted
	if got := session.Manifest.Settings[StartLocationSetting]; got != "tavern" {
		t.Errorf("expected start location tavern, got %v", got)
	}
	persisted, err := core.LoadGameManifest(filepath.Join(paths.GameDir("campaign-01"), "game.yaml"))
	if err != nil {
		t.Fatalf("reload game manifest failed: %v", err)
	}
	if got := persisted.Settings[StartLocationSetting]; got != "tavern" {
		t.Errorf("expected persisted start location tavern, got %v", got)
	}

	if _, err := os.Stat(paths.GameDBPath("campaign-01")); err != nil {
		t.Errorf("expected the canonical game database: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.GameDir("campaign-01"), "game.db")); !os.IsNotExist(err) {
		t.Errorf("expected no legacy game.db beside the game, stat err = %v", err)
	}

	if _, err := os.Stat(filepath.Join(paths.GameDir("campaign-01"), "entities", "tavern.md")); err != nil {
		t.Errorf("expected the world template to be copied as <id>.md: %v", err)
	}

	if _, err := os.Stat(filepath.Join(paths.GameDir("campaign-01"), "entities", "old-gaffer.md")); err != nil {
		t.Errorf("expected the world template without id to be copied as slugified-name.md: %v", err)
	}
	if gaffer, err := session.Store.GetEntity("old-gaffer"); err != nil || gaffer == nil {
		t.Errorf("expected old-gaffer entity in store: %v", err)
	}
}

// writeTestCampaignScaffold lays down the system and world manifests a campaign
// needs, and returns the paths a test can call InitGame with.
func writeTestCampaignScaffold(t *testing.T, tempDir string, worldEntities map[string]string) *core.PathResolver {
	t.Helper()

	paths := core.NewPathResolver(tempDir)

	sysDir := paths.SystemDir("d20-test")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: d20-test\nname: D20 Test\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	worldDir := paths.WorldDir("fantasy-realm")
	entitiesDir := filepath.Join(worldDir, "entities")
	if err := os.MkdirAll(entitiesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: fantasy-realm\nname: Fantasy Realm\n"), 0644); err != nil {
		t.Fatal(err)
	}

	for name, body := range worldEntities {
		if err := os.WriteFile(filepath.Join(entitiesDir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	locationDoc := "---\nid: aldon-harbour\nname: Aldon Harbour\ntype: location\n---\nSalt air.\n"
	if err := os.WriteFile(filepath.Join(entitiesDir, "Aldon-Harbour.md"), []byte(locationDoc), 0644); err != nil {
		t.Fatal(err)
	}

	return paths
}

func TestInitGameCreatesThePlayerNote(t *testing.T) {
	paths := writeTestCampaignScaffold(t, t.TempDir(), nil)

	session, err := InitGame(paths, InitOptions{
		GameID:     "campaign-02",
		SystemID:   "d20-test",
		WorldID:    "fantasy-realm",
		PlayerName: "Sean O'Neill",
	})
	if err != nil {
		t.Fatalf("InitGame failed: %v", err)
	}
	defer session.Close()

	// The slugified player name becomes the note's ID, so the path is predictable.
	// Punctuation is dropped rather than transliterated: the apostrophe in
	// "O'Neill" leaves "sean-oneill".
	notePath := filepath.Join(paths.GameDir("campaign-02"), "entities", "sean-oneill.md")
	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("expected the player note to exist: %v", err)
	}

	player, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("parse player note: %v", err)
	}
	if player.Name != "Sean O'Neill" || player.Type != "character" {
		t.Errorf("unexpected player note: %+v", player)
	}
	if player.Location == "" {
		t.Errorf("expected the player note to link the opening location")
	}

	indexed, err := session.Store.GetEntity(player.ID)
	if err != nil {
		t.Fatalf("expected the player in the index: %v", err)
	}
	if indexed.Location != player.Location {
		t.Errorf("index location = %q, note location = %q", indexed.Location, player.Location)
	}
}

func TestInitGameLeavesAnAuthoredPlayerNoteAlone(t *testing.T) {
	authored := "---\nid: sean\nname: Sean\ntype: character\n---\nHand written.\n"
	paths := writeTestCampaignScaffold(t, t.TempDir(), map[string]string{"sean.md": authored})

	session, err := InitGame(paths, InitOptions{
		GameID:     "campaign-03",
		SystemID:   "d20-test",
		WorldID:    "fantasy-realm",
		PlayerName: "Sean",
	})
	if err != nil {
		t.Fatalf("InitGame failed: %v", err)
	}
	defer session.Close()

	data, err := os.ReadFile(filepath.Join(paths.GameDir("campaign-03"), "entities", "sean.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Hand written.") {
		t.Errorf("expected the authored note to survive, got %s", data)
	}
}

func TestInitGameRejectsInvalidIDs(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	_, err := InitGame(paths, InitOptions{
		GameID:   "../escaped",
		SystemID: "sys",
		WorldID:  "world",
		Name:     "Test",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid game id") {
		t.Errorf("InitGame expected invalid game id error, got %v", err)
	}

	_, err = InitGame(paths, InitOptions{
		GameID:   "game",
		SystemID: "../sys",
		WorldID:  "world",
		Name:     "Test",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid system id") {
		t.Errorf("InitGame expected invalid system id error, got %v", err)
	}

	_, err = InitGame(paths, InitOptions{
		GameID:   "game",
		SystemID: "sys",
		WorldID:  "../world",
		Name:     "Test",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid world id") {
		t.Errorf("InitGame expected invalid world id error, got %v", err)
	}
}

