package desktop

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestSettingsModalSnapshot(t *testing.T) {
	appState = &State{
		Loaded:   true,
		Selected: "campaign-01",
		Games:    []gui.GameSummaryDTO{{ID: "campaign-01", Name: "The Hollow Crown"}},
		SettingsGameID:  "campaign-01",
		SettingsStart:   "market",
		SettingsOpening: "Begin at the gate",
		VoiceProfiles: []config.VoiceProfile{
			{ID: "af_bella", Name: "Bella", VoiceID: "af_bella"},
			{ID: "am_adam", Name: "Adam", VoiceID: "am_adam"},
		},
	}
	ui.Snapshot(t, "settings_modal", 1000, 700, RootView)
}

func TestSettingsSavePatchBuildsMap(t *testing.T) {
	appState = &State{Loaded: true, SettingsGameID: "campaign-01"}
	appState.SettingsOpening = "Begin at the gate"
	appState.SettingsStart = "market"
	appState.SettingsVoice = "af_bella"

	patch := settingsPatch()
	if patch["opening_prompt"] != "Begin at the gate" {
		t.Errorf("opening_prompt = %v", patch["opening_prompt"])
	}
	if patch["start_location"] != "market" {
		t.Errorf("start_location = %v", patch["start_location"])
	}
	if patch["narrator_voice"] != "af_bella" {
		t.Errorf("narrator_voice = %v", patch["narrator_voice"])
	}
}

func TestDeleteConfirmRequiresSecondAction(t *testing.T) {
	liveService = gui.NewService(t.TempDir())
	t.Cleanup(func() { liveService = nil })

	called := false
	deleteGame = func(context.Context, *gui.Service, string) error {
		called = true
		return nil
	}
	t.Cleanup(func() { deleteGame = nil })

	appState = &State{Loaded: true, SettingsGameID: "campaign-01"}
	requestDelete() // first click arms the confirm
	if called {
		t.Fatal("delete must not fire on the first click")
	}
	requestDelete() // second click confirms
	if !called {
		t.Fatal("delete must fire once confirmed")
	}
}
