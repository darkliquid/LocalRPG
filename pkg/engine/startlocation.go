package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/storage"
)

const locationEntityType = "location"

// StartLocationSetting is the game manifest setting that pins a campaign's
// opening location once it has been resolved.
const StartLocationSetting = "start_location"

// OpeningSceneEntityID is the identifier used for the location entity generated
// when a campaign's world defines no locations of its own.
const OpeningSceneEntityID = "opening-scene"

// ResolveStartLocation decides which location a campaign opens in, preferring
// state that already exists: an explicit start_location setting, the player's
// own location reference, then any indexed location. A location derived from the
// world's opening scene is created when everything else comes up empty.
func ResolveStartLocation(paths *core.PathResolver, store *storage.Store, manifest *core.GameManifest) (string, error) {
	if manifest == nil {
		return "", fmt.Errorf("resolve start location: missing game manifest")
	}
	if store == nil {
		return "", fmt.Errorf("resolve start location: missing entity store")
	}

	if pinned := manifestSettingString(manifest, StartLocationSetting); pinned != "" {
		if ent, err := store.GetEntity(pinned); err == nil && ent != nil && ent.Type == locationEntityType {
			return ent.ID, nil
		}
	}

	if id, err := ResolvePlayerID(store, manifest); err == nil && id != "" {
		if player, err := store.GetEntity(id); err == nil && player != nil {
			for _, ref := range playerLocationRefs(player) {
				if ent := findLocationByRef(store, ref); ent != nil {
					return ent.ID, nil
				}
			}
		}
	}

	if id := firstLocationID(store); id != "" {
		return id, nil
	}

	return createOpeningSceneLocation(paths, store, manifest)
}

func manifestSettingString(manifest *core.GameManifest, key string) string {
	if manifest.Settings == nil {
		return ""
	}
	value, ok := manifest.Settings[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func playerLocationRefs(player *entity.Entity) []string {
	refs := make([]string, 0, len(player.Wikilinks)+1)
	if strings.TrimSpace(player.Location) != "" {
		refs = append(refs, player.Location)
	}
	return append(refs, player.Wikilinks...)
}

func findLocationByRef(store *storage.Store, ref string) *entity.Entity {
	for _, candidate := range normalizeRefCandidates(ref) {
		if ent, err := store.GetEntity(candidate); err == nil && ent != nil && ent.Type == locationEntityType {
			return ent
		}
	}

	summaries, err := store.ListEntities()
	if err != nil {
		return nil
	}
	for _, summary := range summaries {
		if summary.Type != locationEntityType {
			continue
		}
		nameKey := entity.Slugify(summary.Name)
		for _, candidate := range normalizeRefCandidates(ref) {
			if candidate != nameKey {
				continue
			}
			if ent, err := store.GetEntity(summary.ID); err == nil && ent != nil {
				return ent
			}
		}
	}
	return nil
}

// normalizeRefCandidates turns a wikilink-style reference such as
// "[[Alden-Tavern|the tavern]]" into the identifier forms worth looking up.
func normalizeRefCandidates(ref string) []string {
	cleaned := entity.WikilinkTarget(ref)

	candidates := make([]string, 0, 2)
	if slug := entity.Slugify(cleaned); slug != "" {
		candidates = append(candidates, slug)
	}
	if lower := strings.ToLower(strings.TrimSpace(cleaned)); lower != "" && !slices.Contains(candidates, lower) {
		candidates = append(candidates, lower)
	}
	return candidates
}

func firstLocationID(store *storage.Store) string {
	summaries, err := store.ListEntities()
	if err != nil {
		return ""
	}
	for _, summary := range summaries {
		if summary.Type == locationEntityType {
			return summary.ID
		}
	}
	return ""
}

// createOpeningSceneLocation derives a starting location from the world's own
// metadata so a campaign always has somewhere to begin.
func createOpeningSceneLocation(paths *core.PathResolver, store *storage.Store, manifest *core.GameManifest) (string, error) {
	if existing, err := store.GetEntity(OpeningSceneEntityID); err == nil && existing != nil && existing.Type == locationEntityType {
		return existing.ID, nil
	}

	loc := &entity.Entity{
		ID:   OpeningSceneEntityID,
		Name: manifest.ID,
		Type: locationEntityType,
		Body: fmt.Sprintf("The opening scene of %s.", manifest.ID),
	}

	if world := loadWorldManifest(paths, manifest.WorldID); world != nil {
		if world.Name != "" {
			loc.Name = world.Name
		}
		if world.Description != "" {
			loc.Body = world.Description
		}
	}
	if strings.TrimSpace(loc.Name) == "" {
		loc.Name = loc.ID
	}

	if paths == nil {
		if err := store.SaveEntity(loc); err != nil {
			return "", fmt.Errorf("save opening scene location: %w", err)
		}
		return loc.ID, nil
	}

	safeGameID := pathutil.SanitizeID(manifest.ID)
	gameEntitiesDir := filepath.Join(paths.GameDir(safeGameID), "entities")
	if err := os.MkdirAll(gameEntitiesDir, 0755); err != nil {
		return "", fmt.Errorf("create entities dir: %w", err)
	}

	markdown, err := loc.SerializeMarkdown()
	if err != nil {
		return "", fmt.Errorf("serialize opening scene location: %w", err)
	}

	path, err := pathutil.ResolveSafeChild(gameEntitiesDir, OpeningSceneEntityID+".md")
	if err != nil {
		return "", fmt.Errorf("invalid opening scene location path: %w", err)
	}
	if err := os.WriteFile(path, markdown, 0644); err != nil {
		return "", fmt.Errorf("write opening scene location: %w", err)
	}

	if err := storage.NewSyncer(store).SyncFile(path); err != nil {
		return "", fmt.Errorf("index opening scene location: %w", err)
	}

	return loc.ID, nil
}

func loadWorldManifest(paths *core.PathResolver, worldID string) *core.WorldManifest {
	if paths == nil || worldID == "" {
		return nil
	}
	world, err := core.LoadWorldManifest(filepath.Join(paths.WorldDir(worldID), "world.yaml"))
	if err != nil {
		return nil
	}
	return world
}
