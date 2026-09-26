package desktop

import (
	. "go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// RootView renders the application frame for the active screen.
func RootView() {
	Container(Attrs(Viewport, BackgroundVec(ui.CanvasBG)), func() {
		switch appState.Screen {
		case ScreenNewCampaign:
			newCampaignView()
		case ScreenWorldGallery:
			worldsView()
		case ScreenChronicle:
			chronicleView()
		case ScreenSettings:
			settingsView()
		case ScreenSystemsStudio:
			systemsStudioView()
		case ScreenWorldsStudio:
			worldsStudioView()
		case ScreenTheater:
			theaterScreen()
		default:
			launcherView()
		}
		lightbox()
		campaignSettingsModal()
		confirmDiscardModal()
	})
}
