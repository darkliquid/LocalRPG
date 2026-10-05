package engine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// ResetCampaign returns a campaign to its opening state without touching its
// configuration. The world's template cast is restored, notes created during
// play are removed, the protagonist keeps its authored sheet but loses its
// runtime state, and the timeline is cleared. The manifest, the assets directory,
// the usage ledger and the TTS job table are left alone, so a restart costs the
// player their story and nothing else.
//
// The initial set is derived from the world rather than snapshotted at creation,
// so a world author's correction reaches a restarted campaign.
func ResetCampaign(paths *core.PathResolver, store *storage.Store, manifest *core.GameManifest) error {
	if paths == nil || store == nil || manifest == nil {
		return fmt.Errorf("reset campaign: missing paths, store, or manifest")
	}

	gameDir := paths.GameDir(manifest.ID)
	entitiesDir := filepath.Join(gameDir, "entities")

	templates, err := worldEntityTemplates(paths, manifest.WorldID)
	if err != nil {
		return err
	}

	playerID := manifest.Player
	if id, err := ResolvePlayerID(store, manifest); err == nil && id != "" {
		playerID = id
	}

	if err := resetEntityNotes(entitiesDir, templates, playerID); err != nil {
		return err
	}

	if err := store.ResetDerivedState(); err != nil {
		return fmt.Errorf("reset derived state: %w", err)
	}

	history := NewHistoryLogger(filepath.Join(gameDir, "history.jsonl"))
	if err := history.RewindToTurn(0); err != nil {
		return fmt.Errorf("clear history: %w", err)
	}

	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		return fmt.Errorf("reindex entities: %w", err)
	}
	return nil
}

// worldEntityTemplates reads the world's starting cast, keyed by the identifier
// InitGame would give each note. It mirrors the import's shape: top-level notes
// only, and the frontmatter id wins over the filename.
func worldEntityTemplates(paths *core.PathResolver, worldID string) (map[string][]byte, error) {
	templates := map[string][]byte{}
	if strings.TrimSpace(worldID) == "" {
		return templates, nil
	}

	dir := filepath.Join(paths.WorldDir(worldID), "entities")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return templates, nil
		}
		return nil, fmt.Errorf("read world entities: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read world entity template %q: %w", entry.Name(), err)
		}
		id := templateID(data, entry.Name())
		if id == "" {
			continue
		}
		templates[id] = data
	}
	return templates, nil
}

// templateID is the identifier InitGame would derive for a template note, or an
// empty string when the note is too malformed to place.
func templateID(data []byte, filename string) string {
	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return ""
	}
	id := ent.ID
	if id == "" {
		id = entity.Slugify(ent.Name)
	}
	if id == "" {
		id = strings.TrimSuffix(filename, ".md")
	}
	return pathutil.SanitizeID(id)
}

// resetEntityNotes classifies every note under entities/ against the world's
// templates and rewrites it: the protagonist loses its runtime state, the
// opening scene is left alone, a template note is restored from the world, and
// anything else was created during play and is removed.
func resetEntityNotes(entitiesDir string, templates map[string][]byte, playerID string) error {
	if _, err := os.Stat(entitiesDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect entities dir: %w", err)
	}

	playerID = pathutil.SanitizeID(playerID)

	notes := make([]string, 0)
	err := filepath.WalkDir(entitiesDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			if path == entitiesDir {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "assets" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			return nil
		}
		notes = append(notes, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk entities: %w", err)
	}

	for _, path := range notes {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read entity note %q: %w", path, err)
		}
		id := noteID(data, path)

		switch {
		case id != "" && (id == playerID || id == OpeningSceneEntityID):
			// The protagonist and the opening scene are part of the campaign's
			// opening state, so they keep their authored content and only lose
			// the runtime record of turns that no longer exist.
			if err := clearRuntimeFields(path, data); err != nil {
				return err
			}
		default:
			template, ok := templates[id]
			if !ok {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("delete entity note %q: %w", path, err)
				}
				continue
			}
			if err := restoreTemplate(entitiesDir, id, path, template); err != nil {
				return err
			}
		}
	}
	return nil
}

// noteID is the identifier a note is filed under, falling back to its filename
// when the frontmatter will not parse, which is the same tolerance Sync shows.
func noteID(data []byte, path string) string {
	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil || ent.ID == "" {
		return pathutil.SanitizeID(strings.TrimSuffix(filepath.Base(path), ".md"))
	}
	return pathutil.SanitizeID(ent.ID)
}

// restoreTemplate rewrites a note from its world template at the entities root,
// where InitGame would have placed it, and removes the original when it sat
// somewhere else so the note is not duplicated.
func restoreTemplate(entitiesDir, id, currentPath string, data []byte) error {
	target, err := pathutil.ResolveSafeChild(entitiesDir, id+".md")
	if err != nil {
		return fmt.Errorf("invalid entity template id %q: %w", id, err)
	}
	if err := os.WriteFile(target, data, 0644); err != nil {
		return fmt.Errorf("restore entity template %q: %w", id, err)
	}
	if filepath.Clean(currentPath) != filepath.Clean(target) {
		if err := os.Remove(currentPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove relocated note %q: %w", currentPath, err)
		}
	}
	return nil
}

// clearRuntimeFields drops a note's turn history and mechanical state while
// leaving every authored field intact, so a restart resets what play did to a
// campaign entity without discarding what the entity is.
func clearRuntimeFields(path string, data []byte) error {
	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return fmt.Errorf("parse entity note %q: %w", path, err)
	}
	if len(ent.History) == 0 && ent.State == nil {
		return nil
	}
	ent.History = nil
	ent.State = nil

	out, err := ent.SerializeMarkdown()
	if err != nil {
		return fmt.Errorf("serialize entity note %q: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		return fmt.Errorf("write entity note %q: %w", path, err)
	}
	return nil
}
