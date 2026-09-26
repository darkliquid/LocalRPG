package desktop

import (
	"context"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// Injected per-campaign write operations; nil outside a live app.
var (
	saveGameSettings func(ctx context.Context, svc *gui.Service, gameID string, patch map[string]any) error
	restartGame      func(ctx context.Context, svc *gui.Service, gameID string) error
	deleteGame       func(ctx context.Context, svc *gui.Service, gameID string) error
)

func settingsPatch() map[string]any {
	return map[string]any{
		"opening_prompt": appState.SettingsOpening,
		"start_location": appState.SettingsStart,
		"narrator_voice": appState.SettingsVoice,
	}
}

func deleteLabel() string {
	if appState.SettingsConfirm {
		return "Confirm delete"
	}
	return "Delete"
}

// requestDelete arms on the first click and deletes on the second.
func requestDelete() {
	if !appState.SettingsConfirm {
		appState.SettingsConfirm = true
		return
	}
	appState.SettingsConfirm = false
	svc := liveService
	id := appState.SettingsGameID
	appState.SettingsGameID = ""
	if deleteGame == nil || svc == nil {
		return
	}
	if err := deleteGame(context.Background(), svc, id); err == nil {
		reload(context.Background(), svc)
	}
}

// openSettings loads a campaign's editable settings and opens the modal.
func openSettings(gameID string) {
	appState.SettingsGameID = gameID
	appState.SettingsConfirm = false
	svc := liveService
	if svc == nil {
		return
	}
	go func() {
		ctx := context.Background()
		if state, err := svc.GetGameState(ctx, gameID); err == nil && state != nil {
			WithFrameLock(func() {
				appState.SettingsOpening = state.OpeningPrompt
				appState.SettingsStart = state.StartLocation
				appState.SettingsVoice = state.NarratorVoice
			})
		}
		if settings, err := svc.GetSettings(ctx); err == nil && settings != nil {
			WithFrameLock(func() { appState.VoiceProfiles = settings.Config.Media.TTS.VoiceProfiles })
		}
		RequestNextFrame()
	}()
}

func campaignSettingsModal() {
	if appState.SettingsGameID == "" {
		return
	}
	p := ui.DefaultPalette()
	title := appState.SettingsGameID
	if game := appState.SelectedGame(); game != nil && game.Name != "" {
		title = game.Name
	}
	dismiss := func() {
		appState.SettingsGameID = ""
		appState.SettingsConfirm = false
	}
	stage := ModalStyle{Background: p.Panel, Text: p.Text, Scrim: Vec4{0, 0, 0, 0.6}}
	ModalStyled(560, dismiss, stage, func() {
		Label(title, FontSize(18), FontWeight(WeightBold), TextColorVec(p.Text))

		Label("Narrator voice", FontSize(12), FontWeight(WeightBold), TextColorVec(p.Muted))
		Container(Attrs(Row, Gap(8), Wrap), func() {
			for i := range appState.VoiceProfiles {
				voice := &appState.VoiceProfiles[i]
				selected := voice.ID == appState.SettingsVoice || voice.VoiceID == appState.SettingsVoice
				Container(Attrs(Pad(8), Corners(6), BackgroundVec(p.Bg)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					}
					NextAccessName("settings.voice." + voice.ID)
					if PressAction() {
						appState.SettingsVoice = voice.ID
					}
					AssignAccess()
					Label(voice.Name, FontSize(12), TextColorVec(p.Text))
				})
			}
		})

		Label("Start location", FontSize(12), FontWeight(WeightBold), TextColorVec(p.Muted))
		FieldInput(&appState.SettingsStart)
		Label("Opening prompt", FontSize(12), FontWeight(WeightBold), TextColorVec(p.Muted))
		FieldArea(&appState.SettingsOpening)

		Container(Attrs(Row, CrossMid, Gap(8)), func() {
			NextAccessName("settings.save")
			if Button(NoIcon, "Save") {
				svc := liveService
				patch := settingsPatch()
				id := appState.SettingsGameID
				if saveGameSettings != nil && svc != nil {
					go func() { _ = saveGameSettings(context.Background(), svc, id, patch) }()
				}
				appState.SettingsGameID = ""
			}
			AssignAccess()

			NextAccessName("settings.restart")
			if Button(NoIcon, "Restart") {
				svc := liveService
				id := appState.SettingsGameID
				if restartGame != nil && svc != nil {
					go func() {
						_ = restartGame(context.Background(), svc, id)
						reload(context.Background(), svc)
					}()
				}
				appState.SettingsGameID = ""
			}
			AssignAccess()

			NextAccessName("settings.delete")
			if Button(NoIcon, deleteLabel()) {
				requestDelete()
			}
			AssignAccess()

			Filler(1)
			NextAccessName("settings.close")
			if Button(NoIcon, "Close") {
				dismiss()
			}
			AssignAccess()
		})
	})
}
