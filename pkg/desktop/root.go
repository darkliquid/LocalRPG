package desktop

// RootView renders the application frame for the active screen.
func RootView() {
	switch appState.Screen {
	case ScreenNewCampaign:
		newCampaignView()
	case ScreenWorldGallery:
		worldsView()
	default:
		launcherView()
	}
	lightbox()
	campaignSettingsModal()
}
