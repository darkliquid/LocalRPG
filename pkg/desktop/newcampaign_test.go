package desktop

import (
	"context"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestCharacterSubmitFieldsUsesSystemFields(t *testing.T) {
	appState = &State{
		CharacterFields: []core.CharacterCreationField{
			{ID: "name", Label: "Name"},
			{ID: "class", Label: "Class", Kind: "select"},
			{ID: "voice", Label: "Voice", Kind: "voice"},
		},
		CharacterAnswers: map[string]string{"name": "Vance", "class": "Rogue", "voice": "af_bella"},
	}
	got := characterSubmitFields()
	if got["name"] != "Vance" || got["class"] != "Rogue" {
		t.Fatalf("answers = %#v", got)
	}
	if _, ok := got["voice"]; ok {
		t.Fatal("voice fields must not be submitted as character text")
	}
}

func TestNewCampaignSnapshot(t *testing.T) {
	appState = &State{
		Loaded:       true,
		Screen:       ScreenNewCampaign,
		PendingWorld: "realm",
		Worlds:       []gui.WorldSummaryDTO{{ID: "realm", Name: "The Sundered Realm"}},
		Systems:      []gui.SystemSummaryDTO{{ID: "dnd5e", Name: "Dungeons & Dragons 5e"}},
	}
	newForm = newCampaignForm{PlayerName: "Adventurer", Name: "Chronicles of The Sundered Realm", SystemID: "dnd5e"}
	ui.Snapshot(t, "new_campaign", 1280, 800, RootView)
}

func TestNewCampaignCreateInvokesCallback(t *testing.T) {
	appState = &State{Loaded: true, Screen: ScreenNewCampaign, PendingWorld: "realm"}
	liveService = gui.NewService(t.TempDir())
	t.Cleanup(func() { liveService = nil })
	newForm = newCampaignForm{Name: "Test", PlayerName: "Hero", SystemID: "dnd5e"}

	called := false
	createGame = func(context.Context, *gui.Service, gui.CreateGameRequestDTO) (*gui.GameSummaryDTO, error) {
		called = true
		return &gui.GameSummaryDTO{ID: "test"}, nil
	}
	t.Cleanup(func() { createGame = nil })

	submitCreate()
	if appState.Screen != ScreenLauncher {
		t.Fatal("submitCreate must return to the launcher")
	}
	deadline := time.Now().Add(time.Second)
	for !called && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !called {
		t.Fatal("create callback was not invoked")
	}
}

func TestPreviewGenerationStoresTempFile(t *testing.T) {
	appState = &State{Loaded: true, PendingWorld: "realm"}
	liveService = gui.NewService(t.TempDir())
	t.Cleanup(func() { liveService = nil })

	generatePreview = func(context.Context, *gui.Service, gui.GenerateAssetPreviewRequestDTO) ([]byte, string, error) {
		return []byte{0x89, 'P', 'N', 'G'}, "image/png", nil
	}
	t.Cleanup(func() { generatePreview = nil })

	if err := generateFormPreview(context.Background(), liveService, "banner"); err != nil {
		t.Fatalf("generateFormPreview: %v", err)
	}
	if appState.FormBannerPreview == "" {
		t.Fatal("expected a preview path to be stored")
	}
}
