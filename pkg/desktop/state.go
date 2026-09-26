package desktop

import "github.com/darkliquid/localrpg/pkg/gui"

// Screen identifies which top-level view the shell renders.
type Screen int

const (
	ScreenLauncher Screen = iota
	ScreenNewCampaign
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
