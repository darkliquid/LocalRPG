package gui

import (
	"net/http"
	"sort"
)

// routeMount is one API mount: the mux pattern the server binds and the method
// expression that serves it. Keeping the method expression here means adding a
// route is one line and the checked-in manifest cannot drift from the server.
type routeMount struct {
	Pattern string
	serve   func(*Server, http.ResponseWriter, *http.Request)
}

// mounts is the single list of API mounts. registerRoutes iterates it.
var mounts = []routeMount{
	{"/api/game/", (*Server).handleGameRoutes},
	{"/api/games", (*Server).handleGamesRoutes},
	{"/api/usage", (*Server).handleUsageRoute},
	{"/api/limits", (*Server).handleLimitsRoute},
	{"/api/character/generate", (*Server).handleCharacterGenerateRoute},
	{"/api/generate-text", (*Server).handleGenerateTextRoute},
	{"/api/generate-asset-preview", (*Server).handleGenerateAssetPreview},
	{"/api/systems", (*Server).handleSystemsRoutes},
	{"/api/system/generate", (*Server).handleSystemGenerateRoute},
	{"/api/system/derive", (*Server).handleSystemDeriveRoute},
	{"/api/system/draft/commit", (*Server).handleSystemDraftCommitRoute},
	{"/api/system/draft/discard", (*Server).handleSystemDraftDiscardRoute},
	{"/api/system/draft/", (*Server).handleSystemDraftRoutes},
	{"/api/system/", (*Server).handleSystemRoutes},
	{"/api/system/test", (*Server).handleSystemTestRoute},
	{"/api/system/tests/", (*Server).handleSystemTestsRoute},
	{"/api/reference-systems", (*Server).handleReferenceSystemsRoute},
	{"/api/worlds", (*Server).handleWorldsRoutes},
	{"/api/world/generate", (*Server).handleWorldGenerateRoute},
	{"/api/world/ingest", (*Server).handleWorldIngestRoute},
	{"/api/world/draft/commit", (*Server).handleWorldDraftCommitRoute},
	{"/api/world/draft/discard", (*Server).handleWorldDraftDiscardRoute},
	{"/api/world/draft/", (*Server).handleWorldDraftRoutes},
	{"/api/world/", (*Server).handleWorldRoutes},
	{"/api/settings", (*Server).handleSettingsRoutes},
	{"/api/settings/test-provider", (*Server).handleTestProviderRoute},
	{"/api/config/offline-preset", (*Server).handleOfflinePresetRoute},
	{"/api/config/offline-report", (*Server).handleOfflineReportRoute},
	{"/api/styles", (*Server).handleStylesRoute},
	{"/api/open-url", (*Server).handleOpenURLRoute},
	{"/api/providers", (*Server).handleProviderCatalogRoute},
	{"/api/providers/models", (*Server).handleModelCatalogueRoute},
	{"/api/tts/inspect", (*Server).handleTTSInspectRoute},
	{"/api/media/inspect", (*Server).handleMediaInspectRoute},
	{"/api/tts/voices/search", (*Server).handleVoiceSearchRoute},
	{"/api/tts/batch", (*Server).handleTTSBatchRoute},
	{"/api/tts/batch/", (*Server).handleTTSBatchRoute},
	{"/api/audio/", (*Server).handleAudioRoutes},
	{"/api/stt", (*Server).handleSTTRoute},
	{"/api/export", (*Server).handleExportRoutes},
	{"/api/export/", (*Server).handleExportRoutes},
	{"/api/trace", (*Server).handleTraceRoute},
	{"/api/models", (*Server).handleModelsRoutes},
	{"/api/models/", (*Server).handleModelsRoutes},
	{"/api/docs", (*Server).handleDocsRoutes},
	{"/api/docs/", (*Server).handleDocsRoutes},
	{"/api/content/export", (*Server).handleContentExportRoute},
	{"/api/content/import", (*Server).handleContentImportRoute},
	{"/api/registry/search", (*Server).handleRegistrySearchRoute},
	{"/api/registry/install", (*Server).handleRegistryInstallRoute},
	{"/api/registry/updates", (*Server).handleRegistryUpdatesRoute},
	{"/api/registry/sources", (*Server).handleRegistrySourcesRoute},
	{"/api/schema/", (*Server).handleSchemaRoutes},
	{"/api/dialog/", (*Server).handleDialogRoutes},
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
