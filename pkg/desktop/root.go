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
	default:
		launcherView()
	}
	lightbox()
	campaignSettingsModal()
	confirmDiscardModal()
}
