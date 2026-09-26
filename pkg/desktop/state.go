package desktop

import (
	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/gui"
)

// Screen identifies which top-level view the shell renders.
type Screen int

const (
	ScreenLauncher Screen = iota
	ScreenNewCampaign
	ScreenWorldGallery
	ScreenChronicle
)

// State is the desktop application's cached data. It is replaced wholesale by
// loadAll/reload and read by every view during a frame.
type State struct {
	Games   []gui.GameSummaryDTO
	Worlds  []gui.WorldSummaryDTO
	Systems []gui.SystemSummaryDTO
	Loaded  bool
	Err     error

	// Selected is the campaign highlighted in the dock/hero.
	Selected string
	// Screen is the active top-level view.
	Screen Screen
	// PendingWorld is the world chosen for a new campaign.
	PendingWorld string

	// LightboxPath is the image shown full-window, or empty.
	LightboxPath string

	// WorldFlyoutOpen shows the compact world strip beside the dock.
	WorldFlyoutOpen bool

	// Per-campaign settings modal state.
	SettingsGameID  string
	SettingsOpening string
	SettingsStart   string
	SettingsVoice   string
	SettingsConfirm bool
	VoiceProfiles   []config.VoiceProfile

	// FormBannerPreview and FormIconPreview are temp PNGs for the create form.
	FormBannerPreview string
	FormIconPreview   string

	// Chronicle state.
	OpenGame      string
	Turns         []gui.TurnDTO
	Prose         string
	TurnInFlight  bool
	PendingAction string
	ToolActivity  string
	TurnError     string
	ConsoleMode   string
	ConsoleText   string

	// Drawer is the active side drawer name, or empty.
	Drawer string

	// Codex state.
	Entities       []gui.EntitySummaryDTO
	Entity         *gui.EntityDTO
	EntityMarkdown string
	EntityQuery    string
	EntityType     string
	CodexTab       string
	Memories       []gui.MemoryDTO
	PortraitPath   string
	MergeOpen      bool
	MergeTarget    string

	// GameArt and WorldArt hold resolved on-disk banner/icon paths.
	GameArt  map[string]Art
	WorldArt map[string]Art
}

// Art holds the on-disk paths of a campaign's or world's banner and icon.
type Art struct {
	Banner string
	Icon   string
}

func (s *State) gameArt(id string) Art {
	if s.GameArt == nil {
		return Art{}
	}
	return s.GameArt[id]
}

func (s *State) worldArt(id string) Art {
	if s.WorldArt == nil {
		return Art{}
	}
	return s.WorldArt[id]
}

// appState is read by views and replaced under the frame lock by the loader.
var appState = &State{}

// SelectedGame returns the currently selected campaign, or nil.
func (s *State) SelectedGame() *gui.GameSummaryDTO {
	for i := range s.Games {
		if s.Games[i].ID == s.Selected {
			return &s.Games[i]
		}
	}
	return nil
}

// WorldName returns the display name for a world id, falling back to the id.
func (s *State) WorldName(id string) string {
	for _, w := range s.Worlds {
		if w.ID == id {
			return w.Name
		}
	}
	return id
}

// SystemName returns the display name for a system id, falling back to the id.
func (s *State) SystemName(id string) string {
	for _, system := range s.Systems {
		if system.ID == id {
			return system.Name
		}
	}
	return id
}

// GameName returns the selected campaign's name, or a default.
func (s *State) GameName() string {
	if game := s.SelectedGame(); game != nil && game.Name != "" {
		return game.Name
	}
	return "LocalRPG"
}
