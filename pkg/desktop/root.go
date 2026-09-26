package desktop

// RootView renders the application frame for the active screen.
func RootView() {
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
	default:
		launcherView()
	}
	lightbox()
	campaignSettingsModal()
	confirmDiscardModal()
}
