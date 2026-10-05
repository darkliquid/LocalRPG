package gui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/storage"
)

// TestCampaignArtPromptUsesCampaignMaterial covers the fix where generated
// campaign artwork was described almost entirely by its world, so a fresh
// generation looked like the world's own art rather than the campaign's.
func TestCampaignArtPromptUsesCampaignMaterial(t *testing.T) {
	gameID, svc := turnFixture(t)
	paths := svc.GetResolver()
	gameDir := paths.GameDir(gameID)

	worldDir := paths.WorldDir("harbour-realm")
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"),
		[]byte("id: harbour-realm\nname: Harbour Realm\nart_style: Inked Woodcut\n"), 0644); err != nil {
		t.Fatal(err)
	}

	playerNote := "---\nid: sean\nname: Sean\ntype: character\nappearance: A weathered sailor in a storm-grey coat.\n---\nSean grew up on the docks.\n"
	if err := os.WriteFile(filepath.Join(gameDir, "entities", "sean.md"), []byte(playerNote), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := svc.store(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.NewSyncer(store).Sync(filepath.Join(gameDir, "entities")); err != nil {
		t.Fatal(err)
	}

	if err := svc.UpdateGameSettings(context.Background(), gameID, map[string]interface{}{
		"start_location": "The Salted Anchor",
		"opening_prompt": "A storm batters the harbour.",
	}); err != nil {
		t.Fatal(err)
	}

	prompt := svc.campaignArtPrompt(gameID, "banner")
	for _, want := range []string{
		"Inked Woodcut",
		"Harbour Realm",
		"The Salted Anchor",
		"A storm batters the harbour.",
		"Sean",
		"A weathered sailor in a storm-grey coat.",
		"campaign-01",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if !strings.Contains(prompt, "widescreen cinematic") {
		t.Errorf("prompt should read as a campaign banner:\n%s", prompt)
	}
}

// TestCampaignArtPromptFallsBackToTheCampaignName covers a campaign with no
// world art style and nothing campaign-specific to describe.
func TestCampaignArtPromptFallsBackToTheCampaignName(t *testing.T) {
	gameID, svc := turnFixture(t)

	prompt := svc.campaignArtPrompt(gameID, "icon")
	if !strings.Contains(prompt, "campaign-01") {
		t.Errorf("prompt missing the campaign name:\n%s", prompt)
	}
	if !strings.Contains(prompt, "app icon emblem") {
		t.Errorf("prompt should read as a campaign icon:\n%s", prompt)
	}
}
