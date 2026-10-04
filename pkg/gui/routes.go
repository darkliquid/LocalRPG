package gui

import (
	"net/http"
	"sort"
)

// routeMount is one API mount: the mux pattern the server binds, the handler it
// dispatches to, and the method expression that serves it. Keeping the handler
// expression here means adding a route is one line and the checked-in manifest
// cannot drift from the server.
type routeMount struct {
	Pattern string
	Handler string
	serve   func(*Server, http.ResponseWriter, *http.Request)
}

// mounts is the single list of API mounts. registerRoutes iterates it.
var mounts = []routeMount{
	{"/api/game/", "handleGameRoutes", (*Server).handleGameRoutes},
	{"/api/games", "handleGamesRoutes", (*Server).handleGamesRoutes},
	{"/api/usage", "handleUsageRoute", (*Server).handleUsageRoute},
	{"/api/limits", "handleLimitsRoute", (*Server).handleLimitsRoute},
	{"/api/character/generate", "handleCharacterGenerateRoute", (*Server).handleCharacterGenerateRoute},
	{"/api/generate-text", "handleGenerateTextRoute", (*Server).handleGenerateTextRoute},
	{"/api/generate-asset-preview", "handleGenerateAssetPreview", (*Server).handleGenerateAssetPreview},
	{"/api/systems", "handleSystemsRoutes", (*Server).handleSystemsRoutes},
	{"/api/system/", "handleSystemRoutes", (*Server).handleSystemRoutes},
	{"/api/worlds", "handleWorldsRoutes", (*Server).handleWorldsRoutes},
	{"/api/world/", "handleWorldRoutes", (*Server).handleWorldRoutes},
	{"/api/settings", "handleSettingsRoutes", (*Server).handleSettingsRoutes},
	{"/api/settings/test-provider", "handleTestProviderRoute", (*Server).handleTestProviderRoute},
	{"/api/open-url", "handleOpenURLRoute", (*Server).handleOpenURLRoute},
	{"/api/providers", "handleProviderCatalogRoute", (*Server).handleProviderCatalogRoute},
	{"/api/providers/models", "handleModelCatalogueRoute", (*Server).handleModelCatalogueRoute},
	{"/api/tts/inspect", "handleTTSInspectRoute", (*Server).handleTTSInspectRoute},
	{"/api/tts/voices/search", "handleVoiceSearchRoute", (*Server).handleVoiceSearchRoute},
	{"/api/tts/batch", "handleTTSBatchRoute", (*Server).handleTTSBatchRoute},
	{"/api/tts/batch/", "handleTTSBatchRoute", (*Server).handleTTSBatchRoute},
	{"/api/audio/", "handleAudioRoutes", (*Server).handleAudioRoutes},
	{"/api/stt", "handleSTTRoute", (*Server).handleSTTRoute},
	{"/api/export", "handleExportRoutes", (*Server).handleExportRoutes},
	{"/api/export/", "handleExportRoutes", (*Server).handleExportRoutes},
	{"/api/trace", "handleTraceRoute", (*Server).handleTraceRoute},
	{"/api/models", "handleModelsRoutes", (*Server).handleModelsRoutes},
	{"/api/models/", "handleModelsRoutes", (*Server).handleModelsRoutes},
	{"/api/docs", "handleDocsRoutes", (*Server).handleDocsRoutes},
	{"/api/docs/", "handleDocsRoutes", (*Server).handleDocsRoutes},
}

// Routes returns the mounted API patterns, sorted.
func Routes() []string {
	patterns := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		patterns = append(patterns, mount.Pattern)
	}
	sort.Strings(patterns)
	return patterns
}

// mountCovers reports whether a concrete API path is served by one of the mounts.
// A trailing-slash mount covers everything beneath it.
func mountCovers(path string) bool {
	for _, mount := range mounts {
		if mount.Pattern == path {
			return true
		}
		if len(mount.Pattern) > 0 && mount.Pattern[len(mount.Pattern)-1] == '/' &&
			len(path) >= len(mount.Pattern) && path[:len(mount.Pattern)] == mount.Pattern {
			return true
		}
	}
	return false
}
