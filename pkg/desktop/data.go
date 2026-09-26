package desktop

import (
	"context"

	"go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/gui"
)

// loadAll reads the launcher data from the service. Directory-level failures
// (missing games/worlds/systems) are treated as empty lists, matching the SPA
// which swallowed each request's error independently.
func loadAll(ctx context.Context, svc *gui.Service) *State {
	st := &State{Loaded: true}

	if games, err := svc.ListGames(ctx); err == nil {
		st.Games = games
	} else {
		st.Games = []gui.GameSummaryDTO{}
	}
	if worlds, err := svc.ListWorlds(ctx); err == nil {
		st.Worlds = worlds
	} else {
		st.Worlds = []gui.WorldSummaryDTO{}
	}
	if systems, err := svc.ListSystems(ctx); err == nil {
		st.Systems = systems
	} else {
		st.Systems = []gui.SystemSummaryDTO{}
	}

	// Keep a selection that still exists; otherwise select the first campaign.
	if st.SelectedGame() == nil {
		st.Selected = ""
		if len(st.Games) > 0 {
			st.Selected = st.Games[0].ID
		}
	}
	return st
}

// reload refreshes the cached state from a background goroutine and asks the UI
// to redraw, preserving the current screen and pending world.
func reload(ctx context.Context, svc *gui.Service) {
	next := loadAll(ctx, svc)
	shirei.WithFrameLock(func() {
		next.Screen = appState.Screen
		next.PendingWorld = appState.PendingWorld
		*appState = *next
	})
	shirei.RequestNextFrame()
}
