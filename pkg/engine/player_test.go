package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// legacyPlayerCampaign writes a campaign the way versions before player_name did:
// the manifest holds the display name while the note is named by its slug.
func legacyPlayerCampaign(t *testing.T, player string) (*core.PathResolver, *storage.Store) {
	t.Helper()

	paths := writeTestCampaignScaffold(t, t.TempDir(), nil)
	session, err := InitGame(paths, InitOptions{
		GameID:     "campaign-legacy",
		SystemID:   "d20-test",
		WorldID:    "fantasy-realm",
		PlayerName: player,
	})
	if err != nil {
		t.Fatalf("InitGame failed: %v", err)
	}
	_ = session.Close()

	// Rewrite the manifest into the legacy shape: display name in player, no
	// player_name, note still under the slug.
	legacy := "id: campaign-legacy\nname: campaign-legacy\nsystem: d20-test\nworld: fantasy-realm\nplayer: " + player + "\n"
	if err := os.WriteFile(filepath.Join(paths.GameDir("campaign-legacy"), "game.yaml"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := storage.OpenGameStore(paths, "campaign-legacy")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return paths, store
}

func TestResolvePlayerIDReadsALegacyDisplayName(t *testing.T) {
	paths, store := legacyPlayerCampaign(t, "Elena Nightshade")
	defer store.Close()

	manifest, err := core.LoadGameManifest(filepath.Join(paths.GameDir("campaign-legacy"), "game.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	id, err := ResolvePlayerID(store, manifest)
	if err != nil {
		t.Fatalf("ResolvePlayerID failed: %v", err)
	}
	if id != "elena-nightshade" {
		t.Errorf("ResolvePlayerID = %q, want elena-nightshade", id)
	}

	persisted, err := RepairPlayerIdentity(paths, store, manifest)
	if err != nil {
		t.Fatalf("RepairPlayerIdentity failed: %v", err)
	}
	if persisted != "elena-nightshade" {
		t.Errorf("RepairPlayerIdentity = %q, want elena-nightshade", persisted)
	}

	reloaded, err := core.LoadGameManifest(filepath.Join(paths.GameDir("campaign-legacy"), "game.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Player != "elena-nightshade" {
		t.Errorf("persisted player = %q, want the entity ID", reloaded.Player)
	}
	if reloaded.PlayerName != "Elena Nightshade" {
		t.Errorf("persisted player_name = %q, want the display name", reloaded.PlayerName)
	}
}
