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
	default:
		launcherView()
	}
	lightbox()
	campaignSettingsModal()
}
