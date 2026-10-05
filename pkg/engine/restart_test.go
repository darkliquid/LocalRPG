package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// resetFixture builds a campaign whose world ships one character template, then
// leaves the campaign in a played-in state: the template modified, a note created
// during play, the protagonist carrying runtime state, artwork, a narrator voice
// and a usage row.
type resetFixture struct {
	paths    *core.PathResolver
	store    *storage.Store
	manifest *core.GameManifest
	gameDir  string
	session  *Session
	template []byte
}

func newResetFixture(t *testing.T) *resetFixture {
	t.Helper()
	root := t.TempDir()
	paths := core.NewPathResolver(root)

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
	template := []byte("---\nid: harbourmaster\nname: Harbourmaster\ntype: character\n---\nGruff and exacting.\n")
	if err := os.WriteFile(filepath.Join(worldDir, "entities", "harbourmaster.md"), template, 0644); err != nil {
		t.Fatal(err)
	}

	session, err := InitGame(paths, InitOptions{
		GameID:     "campaign-01",
		SystemID:   "freeform",
		WorldID:    "harbour-realm",
		PlayerName: "Sean",
	})
	if err != nil {
		t.Fatalf("InitGame: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	gameDir := paths.GameDir("campaign-01")
	manifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	manifest.Settings["narrator_voice"] = "narrator-01"
	if err := core.SaveGameManifest(filepath.Join(gameDir, "game.yaml"), manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}

	assetsDir := filepath.Join(gameDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"banner.png": "banner-bytes", "icon.png": "icon-bytes"} {
		if err := os.WriteFile(filepath.Join(assetsDir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	entitiesDir := filepath.Join(gameDir, "entities")
	played := []byte("---\nid: harbourmaster\nname: Harbourmaster\ntype: character\nhistory: [1]\n---\nNow friendly.\n")
	if err := os.WriteFile(filepath.Join(entitiesDir, "harbourmaster.md"), played, 0644); err != nil {
		t.Fatal(err)
	}
	crier := []byte("---\nid: gull-crier\nname: Gull Crier\ntype: character\n---\nA town crier.\n")
	if err := os.WriteFile(filepath.Join(entitiesDir, "gull-crier.md"), crier, 0644); err != nil {
		t.Fatal(err)
	}
	player := []byte("---\nid: sean\nname: Sean\ntype: character\nappearance: Weathered.\nhistory: [1]\nstate:\n  hp: 3\n---\nSean grew up on the docks.\n")
	if err := os.WriteFile(filepath.Join(entitiesDir, "sean.md"), player, 0644); err != nil {
		t.Fatal(err)
	}
	opening := []byte("---\nid: opening-scene\nname: Harbour Realm\ntype: location\nhistory: [1]\n---\nThe harbour at dawn.\n")
	if err := os.WriteFile(filepath.Join(entitiesDir, OpeningSceneEntityID+".md"), opening, 0644); err != nil {
		t.Fatal(err)
	}

	// Index the played-in state so the reset has stale rows to clear.
	if _, err := storage.NewSyncer(session.Store).Sync(entitiesDir); err != nil {
		t.Fatalf("sync played state: %v", err)
	}
	if err := session.Store.SaveUsage(storage.UsageRecord{GameID: "campaign-01", Role: "gm", Provider: "echo", Requests: 1}); err != nil {
		t.Fatalf("SaveUsage: %v", err)
	}

	return &resetFixture{
		paths:    paths,
		store:    session.Store,
		manifest: manifest,
		gameDir:  gameDir,
		session:  session,
		template: template,
	}
}

func TestResetCampaignRestoresWorldCastAndKeepsConfiguration(t *testing.T) {
	f := newResetFixture(t)

	if err := ResetCampaign(f.paths, f.store, f.manifest); err != nil {
		t.Fatalf("ResetCampaign: %v", err)
	}

	entitiesDir := filepath.Join(f.gameDir, "entities")

	restored, err := os.ReadFile(filepath.Join(entitiesDir, "harbourmaster.md"))
	if err != nil {
		t.Fatalf("read restored template: %v", err)
	}
	if string(restored) != string(f.template) {
		t.Errorf("harbourmaster.md = %q, want the world template", restored)
	}

	if _, err := os.Stat(filepath.Join(entitiesDir, "gull-crier.md")); !os.IsNotExist(err) {
		t.Errorf("gull-crier.md should have been deleted, stat err = %v", err)
	}

	playerData, err := os.ReadFile(filepath.Join(entitiesDir, "sean.md"))
	if err != nil {
		t.Fatalf("read protagonist note: %v", err)
	}
	player, err := entity.ParseMarkdownEntity(playerData)
	if err != nil {
		t.Fatalf("parse protagonist note: %v", err)
	}
	if player.Appearance != "Weathered." {
		t.Errorf("Appearance = %q, want the authored description kept", player.Appearance)
	}
	if strings.TrimSpace(player.Body) != "Sean grew up on the docks." {
		t.Errorf("Body = %q, want the authored prose kept", player.Body)
	}
	if len(player.History) != 0 {
		t.Errorf("History = %v, want cleared", player.History)
	}
	if player.State != nil && len(player.State.Raw()) != 0 {
		t.Errorf("State = %v, want cleared", player.State.Raw())
	}

	openingData, err := os.ReadFile(filepath.Join(entitiesDir, OpeningSceneEntityID+".md"))
	if err != nil {
		t.Fatalf("read opening scene: %v", err)
	}
	opening, err := entity.ParseMarkdownEntity(openingData)
	if err != nil {
		t.Fatalf("parse opening scene: %v", err)
	}
	if len(opening.History) != 0 {
		t.Errorf("opening scene History = %v, want cleared", opening.History)
	}
	if strings.TrimSpace(opening.Body) != "The harbour at dawn." {
		t.Errorf("opening scene Body = %q, want the authored prose kept", opening.Body)
	}

	manifest, err := core.LoadGameManifest(filepath.Join(f.gameDir, "game.yaml"))
	if err != nil {
		t.Fatalf("reload manifest: %v", err)
	}
	if got, _ := manifest.Settings["narrator_voice"].(string); got != "narrator-01" {
		t.Errorf("narrator_voice = %q, want the setting preserved", got)
	}

	for name, want := range map[string]string{"banner.png": "banner-bytes", "icon.png": "icon-bytes"} {
		got, err := os.ReadFile(filepath.Join(f.gameDir, "assets", name))
		if err != nil {
			t.Fatalf("read asset %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("asset %s = %q, want %q", name, got, want)
		}
	}

	turns, err := f.store.CountTurns()
	if err != nil {
		t.Fatalf("CountTurns: %v", err)
	}
	if turns != 0 {
		t.Errorf("CountTurns = %d, want 0", turns)
	}

	summaries, err := f.store.ListEntities()
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	ids := map[string]bool{}
	for _, summary := range summaries {
		ids[summary.ID] = true
	}
	for _, want := range []string{"harbourmaster", "sean", OpeningSceneEntityID} {
		if !ids[want] {
			t.Errorf("indexed entities %v missing %q", ids, want)
		}
	}
	if ids["gull-crier"] {
		t.Errorf("indexed entities %v still contain the play-created note", ids)
	}

	usage, err := f.store.UsageByGame("campaign-01")
	if err != nil {
		t.Fatalf("UsageByGame: %v", err)
	}
	if len(usage) != 1 {
		t.Errorf("usage rows = %d, want the ledger preserved", len(usage))
	}
}

func TestResetCampaignToleratesAMissingWorld(t *testing.T) {
	root := t.TempDir()
	paths := core.NewPathResolver(root)

	sysDir := paths.SystemDir("freeform")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: freeform\nname: Freeform\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	worldDir := paths.WorldDir("harbour-realm")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: harbour-realm\nname: Harbour Realm\n"), 0644); err != nil {
		t.Fatal(err)
	}

	session, err := InitGame(paths, InitOptions{GameID: "campaign-02", SystemID: "freeform", WorldID: "harbour-realm", PlayerName: "Ada"})
	if err != nil {
		t.Fatalf("InitGame: %v", err)
	}
	defer session.Close()

	manifest, err := core.LoadGameManifest(filepath.Join(paths.GameDir("campaign-02"), "game.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if err := ResetCampaign(paths, session.Store, manifest); err != nil {
		t.Fatalf("ResetCampaign with no world entities: %v", err)
	}

	if _, err := os.Stat(filepath.Join(paths.GameDir("campaign-02"), "entities", "ada.md")); err != nil {
		t.Errorf("protagonist note should survive a world-less reset: %v", err)
	}
}
