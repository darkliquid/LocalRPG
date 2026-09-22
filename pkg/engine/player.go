package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
	"gopkg.in/yaml.v3"
)

// ResolvePlayerID finds the protagonist's entity ID. A manifest's Player field
// may hold an entity ID, or a display name written by a version that named the
// note by its slug instead. Both are tried, then the player name, then a
// case-insensitive match over indexed character entities.
func ResolvePlayerID(store *storage.Store, manifest *core.GameManifest) (string, error) {
	if store == nil || manifest == nil {
		return "", nil
	}

	for _, candidate := range []string{manifest.Player, entity.Slugify(manifest.PlayerName), entity.Slugify(manifest.Player)} {
		if candidate == "" {
			continue
		}
		if ent, err := store.GetEntity(candidate); err == nil && ent != nil {
			return ent.ID, nil
		}
	}

	name := manifest.PlayerName
	if name == "" {
		name = manifest.Player
	}
	if name == "" {
		return "", nil
	}

	summaries, err := store.ListEntities()
	if err != nil {
		return "", fmt.Errorf("list entities: %w", err)
	}
	for _, summary := range summaries {
		if summary.Type == "character" && strings.EqualFold(summary.Name, name) {
			return summary.ID, nil
		}
	}

	return "", nil
}

// RepairPlayerIdentity reconciles a legacy manifest whose Player field holds a
// display name, rewriting game.yaml so every later read is exact. It returns the
// resolved ID, which is empty when the campaign has no player note.
func RepairPlayerIdentity(paths *core.PathResolver, store *storage.Store, manifest *core.GameManifest) (string, error) {
	id, err := ResolvePlayerID(store, manifest)
	if err != nil || id == "" {
		return id, err
	}

	if manifest.Player == id && manifest.PlayerName != "" {
		return id, nil
	}

	if manifest.PlayerName == "" && manifest.Player != "" && manifest.Player != id {
		manifest.PlayerName = manifest.Player
	}
	manifest.Player = id

	data, err := yaml.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal game manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(paths.GameDir(manifest.ID), "game.yaml"), data, 0644); err != nil {
		return "", fmt.Errorf("write game.yaml: %w", err)
	}
	return id, nil
}
