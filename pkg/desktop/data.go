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

	st.GameArt = make(map[string]Art, len(st.Games))
	for _, game := range st.Games {
		var art Art
		if path, _, err := svc.GetGameAsset(game.ID, "banner"); err == nil {
			art.Banner = path
		}
		if path, _, err := svc.GetGameAsset(game.ID, "icon"); err == nil {
			art.Icon = path
		}
		st.GameArt[game.ID] = art
	}
	st.WorldArt = make(map[string]Art, len(st.Worlds))
	for _, world := range st.Worlds {
		var art Art
		if path, _, err := svc.GetWorldAsset(world.ID, "banner"); err == nil {
			art.Banner = path
		}
		if path, _, err := svc.GetWorldAsset(world.ID, "icon"); err == nil {
			art.Icon = path
		}
		st.WorldArt[world.ID] = art
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

// loadEntities reads the campaign's entity summaries.
func loadEntities(ctx context.Context, svc *gui.Service, gameID string) []gui.EntitySummaryDTO {
	entities, err := svc.ListEntities(ctx, gameID)
	if err != nil || entities == nil {
		return []gui.EntitySummaryDTO{}
	}
	return entities
}

// loadEntity reads one entity note and its raw markdown.
func loadEntity(ctx context.Context, svc *gui.Service, gameID, entityID string) (*gui.EntityDTO, string) {
	entity, err := svc.GetEntity(ctx, gameID, entityID)
	if err != nil || entity == nil {
		return nil, ""
	}
	return entity, entity.Markdown
}

// loadMemories reads an entity's memory timeline.
func loadMemories(svc *gui.Service, gameID, entityID string) []gui.MemoryDTO {
	memories, err := svc.ListEntityMemories(gameID, entityID, 20)
	if err != nil || memories == nil {
		return []gui.MemoryDTO{}
	}
	return memories
}

// loadChronicle reads the campaign's turn history.
func loadChronicle(ctx context.Context, svc *gui.Service, gameID string) []gui.TurnDTO {
	turns, err := svc.GetChronicle(ctx, gameID)
	if err != nil || turns == nil {
		return []gui.TurnDTO{}
	}
	return turns
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
