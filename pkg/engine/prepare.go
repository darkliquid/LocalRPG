package engine

import (
	"fmt"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// Prepared holds the per-campaign facts a play session needs before its first
// turn: the authoritative player ID and the resolved opening location.
type Prepared struct {
	PlayerID      string
	StartLocation string
}

// PrepareCampaign reflects on-disk entity edits into the index, ensures the
// turn log is indexed, reconciles the campaign player identity, and resolves
// the opening location. It is the one-time setup a play session used to do
// inline; the desktop game-open path calls it per campaign.
//
// A failed player-identity repair is not fatal, matching the previous
// play-session behaviour: the manifest value is kept.
func PrepareCampaign(paths *core.PathResolver, store *storage.Store, timeline *Timeline, manifest *core.GameManifest, entitiesDir string) (Prepared, error) {
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		return Prepared{}, fmt.Errorf("sync entities: %w", err)
	}
	if err := timeline.EnsureIndexed(); err != nil {
		return Prepared{}, fmt.Errorf("ensure indexed: %w", err)
	}

	playerID := manifest.Player
	if resolved, err := RepairPlayerIdentity(paths, store, manifest); err == nil && resolved != "" {
		playerID = resolved
	}

	startLocation, err := ResolveStartLocation(paths, store, manifest)
	if err != nil {
		return Prepared{}, fmt.Errorf("resolve start location: %w", err)
	}

	return Prepared{PlayerID: playerID, StartLocation: startLocation}, nil
}
