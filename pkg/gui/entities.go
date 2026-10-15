// Entity notes and the codex graph: the files, the folders they live in, and their links.

package gui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// ListEntities returns every note in a campaign, ordered by name, which is what
// the codex browser draws. It reads the notes themselves rather than the index so
// a note that has not been synced yet still appears, and so the browser can never
// disagree with the graph about what exists.
func (s *Service) ListEntities(ctx context.Context, gameID string) ([]EntitySummaryDTO, error) {
	entitiesDir := filepath.Join(s.resolver.GameDir(gameID), "entities")

	summaries := make([]EntitySummaryDTO, 0)
	err := eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		filenameID := strings.TrimSuffix(filepath.Base(path), ".md")
		parsed, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			// A note that fails to parse is still a note the player wrote. Show
			// it so it can be repaired instead of silently vanishing.
			summaries = append(summaries, EntitySummaryDTO{
				ID:         filenameID,
				Name:       filenameID,
				Folder:     folder,
				ParseError: true,
			})
			return nil
		}

		// The declared id is the identity; the file name is only a convention.
		id := parsed.ID
		if id == "" {
			id = filenameID
		}
		name := parsed.Name
		if name == "" {
			name = id
		}

		hasPortrait := parsed.Portrait != "" && s.hasCustomPortrait(gameID, id)
		portraitURL := ""
		if entity.IsCharacterType(parsed.Type) {
			portraitURL = fmt.Sprintf("/api/game/%s/character/%s/portrait", gameID, id)
		}

		summaries = append(summaries, EntitySummaryDTO{
			ID:               id,
			Name:             name,
			Type:             parsed.Type,
			Location:         parsed.Location,
			Tags:             parsed.Tags,
			Aliases:          parsed.Aliases,
			Folder:           folder,
			FilenameMismatch: id != filenameID,
			HasPortrait:      hasPortrait,
			PortraitURL:      portraitURL,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		return strings.ToLower(summaries[i].Name) < strings.ToLower(summaries[j].Name)
	})
	return summaries, nil
}

// GetEntity reads one note of a campaign for the editor: its Markdown, the fields
// the index derived from it, and whether it parsed at all.
func (s *Service) GetEntity(ctx context.Context, gameID, entityID string) (*EntityDTO, error) {
	if err := pathutil.ValidateID(gameID); err != nil {
		return nil, fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return nil, fmt.Errorf("invalid entity id: %w", err)
	}

	entitiesDir := filepath.Join(s.resolver.GameDir(gameID), "entities")

	path, err := s.findEntityNote(entitiesDir, entityID)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("read entity file: %w", fs.ErrNotExist)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read entity file: %w", err)
	}
	folder := folderForNotePath(entitiesDir, path)

	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		// Return the raw note so the codex can still open and repair it, rather
		// than failing the read outright.
		return &EntityDTO{
			ID:         entityID,
			Name:       entityID,
			Markdown:   string(data),
			Folder:     folder,
			ParseError: true,
		}, nil
	}

	var stateMap map[string]any
	if ent.State != nil {
		stateMap = ent.State.Raw()
	}

	var backlinks []string
	if store, err := s.store(gameID); err == nil {
		edges, _ := store.GetEdgesTo(entityID)
		for _, e := range edges {
			backlinks = append(backlinks, e.SourceID)
		}
	}

	return &EntityDTO{
		ID:        entityID,
		Name:      ent.Name,
		Type:      ent.Type,
		Markdown:  string(data),
		State:     stateMap,
		Backlinks: backlinks,
		Folder:    folder,
		History:   ent.History,
	}, nil
}

// ErrDuplicateEntityID reports a save whose frontmatter id already belongs to a
// different note. The id is the identity, so a second claim is refused rather
// than silently overwriting a note in another folder.
var ErrDuplicateEntityID = errors.New("entity id already in use")

// SaveEntity writes a note at the collection root.
func (s *Service) SaveEntity(ctx context.Context, gameID, entityID, rawMarkdown string) error {
	return s.SaveEntityInFolder(ctx, gameID, entityID, "", rawMarkdown)
}

// SaveEntityInFolder writes a note at folder, moving it when it already lives
// somewhere else. The file name stays the id, because the id is the identity and
// the folder is only location: no inbound link is rewritten, which is the whole
// point of decoupling the two.
func (s *Service) SaveEntityInFolder(ctx context.Context, gameID, entityID, folder, rawMarkdown string) error {
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return fmt.Errorf("invalid entity id: %w", err)
	}
	cleanFolder, err := ValidateFolderPath(folder)
	if err != nil {
		return err
	}

	ent, err := entity.ParseMarkdownEntity([]byte(rawMarkdown))
	if err != nil {
		return fmt.Errorf("save entity %q: %w", entityID, err)
	}
	// The file name is the note's identity, so a hand-edited or copied id can
	// never index a note under another note's key.
	ent.ID = entityID
	ent.Folder = cleanFolder

	gameDir := s.resolver.GameDir(gameID)
	entitiesDir := filepath.Join(gameDir, "entities")

	if err := s.assertIDIsFree(entitiesDir, entityID, rawMarkdown); err != nil {
		return err
	}

	existingPath, err := s.findEntityNote(entitiesDir, entityID)
	if err != nil {
		return err
	}

	targetDir := entitiesDir
	if cleanFolder != "" {
		targetDir = filepath.Join(entitiesDir, filepath.FromSlash(cleanFolder))
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create folder %q: %w", cleanFolder, err)
	}

	normalised, err := ent.SerializeMarkdown()
	if err != nil {
		return fmt.Errorf("normalise entity %q: %w", entityID, err)
	}

	targetPath, err := pathutil.ResolveSafeChild(targetDir, entityID+".md")
	if err != nil {
		return fmt.Errorf("resolve entity path: %w", err)
	}
	if strings.Contains(targetPath, "..") {
		return fmt.Errorf("invalid entity path: contains traversal")
	}
	if err := os.WriteFile(targetPath, normalised, 0o644); err != nil {
		return fmt.Errorf("write entity file: %w", err)
	}
	if existingPath != "" && existingPath != targetPath {
		if err := os.Remove(existingPath); err != nil {
			return fmt.Errorf("remove old note %q: %w", existingPath, err)
		}
	}

	store, err := s.store(gameID)
	if err != nil {
		return nil // Non-fatal if db sync fails temporarily
	}

	syncer := storage.NewSyncer(store)
	return syncer.SyncFile(targetPath)
}

// findEntityNote returns the path a note currently occupies, or "" when it is
// new. A note is found by the id it declares or by its file name, so a note that
// an author renamed by hand is still reachable.
func (s *Service) findEntityNote(entitiesDir, entityID string) (string, error) {
	found := ""
	err := eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		filenameID := strings.TrimSuffix(filepath.Base(path), ".md")
		if filenameID == entityID {
			found = path
			return nil
		}
		if entity.DeclaredID(data) == entityID {
			found = path
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return found, nil
}

// assertIDIsFree refuses a save whose frontmatter id already names another note.
// The requested file name is the id, so a body declaring a different id is a
// collision with that other note rather than a rename.
func (s *Service) assertIDIsFree(entitiesDir, entityID, rawMarkdown string) error {
	declared := entity.DeclaredID([]byte(rawMarkdown))
	if declared == "" || declared == entityID {
		return nil
	}
	if _, err := os.Stat(filepath.Join(entitiesDir, declared+".md")); err == nil {
		return fmt.Errorf("%w: %q; use a different id", ErrDuplicateEntityID, declared)
	}
	existing, err := s.findEntityNote(entitiesDir, declared)
	if err == nil && existing != "" {
		return fmt.Errorf("%w: %q; use a different id", ErrDuplicateEntityID, declared)
	}
	return nil
}

// MergeEntities folds one note into another: the survivor keeps its identity and
// gains the source's prose, tags, aliases, and turn history, every note that linked
// to the source is rewritten to point at the survivor, and the source is removed.
//
// It is deliberately explicit. Deciding that two names are one being is a judgement
// the engine cannot make, but it can carry the decision out once a player makes it.
// MergeEntities folds one note into another: the survivor keeps its identity and
// gains the source's prose, tags, aliases, and turn history, every note that linked
// to the source is rewritten to point at the survivor, and the source is removed.
//
// It is deliberately explicit. Deciding that two names are one being is a judgement
// the engine cannot make, but it can carry the decision out once a player makes it.
func (s *Service) MergeEntities(ctx context.Context, gameID, sourceID, targetID string) (*EntityDTO, error) {
	if err := validateMergeIDs(gameID, sourceID, targetID); err != nil {
		return nil, err
	}

	gameDir := s.resolver.GameDir(gameID)
	entitiesDir := filepath.Join(gameDir, "entities")
	sourcePath, sourceData, err := readEntityNote(entitiesDir, "source", sourceID)
	if err != nil {
		return nil, err
	}
	targetPath, targetData, err := readEntityNote(entitiesDir, "target", targetID)
	if err != nil {
		return nil, err
	}

	source, err := entity.ParseMarkdownEntity(sourceData)
	if err != nil {
		return nil, fmt.Errorf("parse source %q: %w", sourceID, err)
	}
	target, err := entity.ParseMarkdownEntity(targetData)
	if err != nil {
		return nil, fmt.Errorf("parse target %q: %w", targetID, err)
	}

	mergeEntityNotes(target, source)

	merged, err := target.SerializeMarkdown()
	if err != nil {
		return nil, fmt.Errorf("serialize merged note: %w", err)
	}

	// Write the survivor first: a failure after this point leaves both notes rather
	// than losing the source's content.
	if err := os.WriteFile(targetPath, merged, 0644); err != nil {
		return nil, fmt.Errorf("write merged note: %w", err)
	}

	if err := s.rewriteInboundLinks(gameDir, sourceID, targetID); err != nil {
		return nil, err
	}

	if err := os.Remove(sourcePath); err != nil {
		return nil, fmt.Errorf("remove source note: %w", err)
	}

	if err := s.reindexAfterMerge(gameID, targetPath, sourcePath, sourceID); err != nil {
		return nil, err
	}

	return s.GetEntity(ctx, gameID, targetID)
}

// validateMergeIDs checks the three ids a merge names, and that it is not a note
// merged into itself.
func validateMergeIDs(gameID, sourceID, targetID string) error {
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(sourceID); err != nil {
		return fmt.Errorf("invalid source id: %w", err)
	}
	if err := pathutil.ValidateID(targetID); err != nil {
		return fmt.Errorf("invalid target id: %w", err)
	}
	if sourceID == targetID {
		return fmt.Errorf("cannot merge %q into itself", sourceID)
	}
	return nil
}

// readEntityNote resolves one note's path under the entities directory and reads it.
// The label names the side of the merge in the error, and the containment check stays
// here because this is where the path is read.
func readEntityNote(entitiesDir, label, id string) (string, []byte, error) {
	path, err := pathutil.ResolveSafeChild(entitiesDir, id+".md")
	if err != nil {
		return "", nil, fmt.Errorf("resolve %s path: %w", label, err)
	}
	if strings.Contains(path, "..") {
		return "", nil, fmt.Errorf("invalid %s path: contains traversal", label)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read %s %q: %w", label, id, err)
	}
	return path, data, nil
}

// mergeEntityNotes folds what the source knew into the survivor, which keeps its own
// name and identity.
func mergeEntityNotes(target, source *entity.Entity) {
	if body := strings.TrimSpace(source.Body); body != "" {
		target.Body = strings.TrimSpace(target.Body) + "\n\n" + body
	}
	target.Aliases = appendUnique(target.Aliases, source.Name)
	target.Aliases = appendUnique(target.Aliases, source.Aliases...)
	target.Tags = appendUnique(target.Tags, source.Tags...)
	for _, number := range source.History {
		already := false
		for _, known := range target.History {
			if known == number {
				already = true
				break
			}
		}
		if !already {
			target.History = append(target.History, number)
		}
	}
}

// reindexAfterMerge drops the source from the index and syncs both notes, so a caller
// that reads straight back sees the merge rather than the old pair.
func (s *Service) reindexAfterMerge(gameID, targetPath, sourcePath, sourceID string) error {
	store, err := s.store(gameID)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	if err := store.DeleteEntity(sourceID); err != nil {
		return fmt.Errorf("remove source from the index: %w", err)
	}

	syncer := storage.NewSyncer(store)
	for _, p := range []string{targetPath, sourcePath} {
		_ = syncer.SyncFile(p)
	}
	return nil
}

// DeleteGameEntity removes one campaign entity note and its index entry.
func (s *Service) DeleteGameEntity(_ context.Context, gameID, entityID string) error {
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return fmt.Errorf("invalid entity id: %w", err)
	}

	entitiesDir := filepath.Join(s.resolver.GameDir(gameID), "entities")
	path, err := s.findEntityNote(entitiesDir, entityID)
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("delete entity %s: %w", entityID, fs.ErrNotExist)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove entity note: %w", err)
	}

	store, err := s.store(gameID)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	if err := store.DeleteEntity(entityID); err != nil {
		return fmt.Errorf("remove entity from the index: %w", err)
	}
	syncer := storage.NewSyncer(store)
	_ = syncer.SyncFile(path)
	return nil
}

// rewriteInboundLinks points every note that linked to the source at the survivor,
// so no note is left pointing at an entity that no longer exists.
func (s *Service) rewriteInboundLinks(gameDir, sourceID, targetID string) error {
	entitiesDir := filepath.Join(gameDir, "entities")

	pattern := regexp.MustCompile(`\[\[\s*` + regexp.QuoteMeta(sourceID) + `(\s*\|[^\]]*)?\]\]`)
	replacement := "[[" + targetID + "$1]]"

	return eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		if strings.TrimSuffix(filepath.Base(path), ".md") == sourceID {
			return nil
		}
		if !pattern.Match(data) {
			return nil
		}

		updated := pattern.ReplaceAll(data, []byte(replacement))
		if err := os.WriteFile(path, updated, 0644); err != nil {
			return fmt.Errorf("rewrite links in %s: %w", filepath.Base(path), err)
		}
		return nil
	})
}

// ListFolders returns the folder tree under an entities directory. Folders are
// read from disk rather than from the index, so a folder with no notes in it is
// still visible and can be dragged into.
func (s *Service) ListFolders(entitiesDir string) ([]FolderDTO, error) {
	paths := make([]string, 0)

	err := filepath.WalkDir(entitiesDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return fmt.Errorf("walk %q: %w", path, walkErr)
		}
		if !entry.IsDir() || path == entitiesDir {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "assets" {
			return fs.SkipDir
		}
		rel, err := filepath.Rel(entitiesDir, path)
		if err != nil {
			return fmt.Errorf("relative path for %q: %w", path, err)
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return BuildFolderTree(paths), nil
}

// CreateFolder creates a folder and any missing parents.
func (s *Service) CreateFolder(entitiesDir, path string) error {
	clean, err := ValidateFolderPath(path)
	if err != nil {
		return err
	}
	if clean == "" {
		return fmt.Errorf("%w: a folder needs a name", ErrInvalidFolderPath)
	}
	return os.MkdirAll(filepath.Join(entitiesDir, filepath.FromSlash(clean)), 0o755)
}

// MoveFolder renames a folder, taking its notes with it. No link is rewritten,
// because a note is linked by id and not by the path it happens to sit at.
func (s *Service) MoveFolder(entitiesDir, from, to string) error {
	cleanFrom, err := ValidateFolderPath(from)
	if err != nil {
		return err
	}
	cleanTo, err := ValidateFolderPath(to)
	if err != nil {
		return err
	}
	if cleanFrom == "" || cleanTo == "" {
		return fmt.Errorf("%w: cannot move the entities root", ErrInvalidFolderPath)
	}

	source := filepath.Join(entitiesDir, filepath.FromSlash(cleanFrom))
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("move folder %q: %w", cleanFrom, err)
	}

	target := filepath.Join(entitiesDir, filepath.FromSlash(cleanTo))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent of %q: %w", cleanTo, err)
	}
	return os.Rename(source, target)
}

// DeleteFolder removes a folder. A folder holding notes is refused unless
// recursive is set, so a stray click cannot delete a campaign's lore.
func (s *Service) DeleteFolder(entitiesDir, path string, recursive bool) error {
	clean, err := ValidateFolderPath(path)
	if err != nil {
		return err
	}
	if clean == "" {
		return fmt.Errorf("%w: cannot delete the entities root", ErrInvalidFolderPath)
	}

	target := filepath.Join(entitiesDir, filepath.FromSlash(clean))
	empty, err := folderIsEmpty(target)
	if err != nil {
		return err
	}
	if !empty && !recursive {
		return fmt.Errorf("folder %q is not empty; pass recursive=true to delete its notes", clean)
	}
	return os.RemoveAll(target)
}

// WorldFolders lists a world's folder tree.
func (s *Service) WorldFolders(worldID string) ([]FolderDTO, error) {
	return s.ListFolders(filepath.Join(s.resolver.WorldDir(worldID), "entities"))
}

// GetGraph builds the campaign's entity graph from its notes: a node per note and
// an edge per wikilink, location or faction field. It reads the files rather than
// the index, so a hand-edited note appears without a reindex.
func (s *Service) GetGraph(ctx context.Context, gameID string) (*GraphDTO, error) {
	gameDir := s.resolver.GameDir(gameID)
	entitiesDir := filepath.Join(gameDir, "entities")
	nodes := make([]GraphNodeDTO, 0)
	links := make([]GraphLinkDTO, 0)
	known := make(map[string]struct{})

	err := eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		ent, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			return nil
		}
		// The declared id is the node's identity, so a nested note is a
		// first-class node rather than a file name that happens to be unique.
		id := ent.ID
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(path), ".md")
		}
		known[id] = struct{}{}
		nodes = append(nodes, GraphNodeDTO{ID: id, Label: ent.Name, Type: ent.Type})

		for _, target := range ent.Wikilinks {
			links = append(links, GraphLinkDTO{Source: id, Target: target})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// A hand-written path-qualified link lands on the note whose id is its final
	// segment, so the graph and the mention resolver agree.
	for i := range links {
		if _, ok := known[links[i].Target]; ok {
			continue
		}
		if base := entity.WikilinkBasename(links[i].Target); base != links[i].Target {
			if _, ok := known[base]; ok {
				links[i].Target = base
			}
		}
	}

	return &GraphDTO{
		Nodes: nodes,
		Links: links,
	}, nil
}

func (s *Service) hasCustomPortrait(gameID, characterID string) bool {
	if s == nil || s.resolver == nil || gameID == "" || characterID == "" {
		return false
	}
	gameDir := s.resolver.GameDir(gameID)
	portraitsDir := filepath.Join(gameDir, "assets", "portraits")
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp", ".svg"} {
		// Check both unversioned and versioned files
		matches, err := filepath.Glob(filepath.Join(portraitsDir, characterID+"*"+ext))
		if err == nil && len(matches) > 0 {
			for _, m := range matches {
				if info, err := os.Stat(m); err == nil && !info.IsDir() && info.Size() > 0 {
					return true
				}
			}
		}
	}
	return false
}

// scanAndEnrichCharacters scans a campaign's character entities, enriching missing attributes and generating portraits.
func (s *Service) scanAndEnrichCharacters(gameID string, store *storage.Store) {
	if store == nil {
		return
	}
	worldStyle := s.worldArtStyle(gameID)
	cfg := s.Config()

	ents, err := store.ListEntities()
	if err != nil {
		return
	}

	var enricher *engine.CharacterEnricher
	if cfg != nil {
		if router, err := harness.RouterFromConfig(cfg); err == nil && router != nil {
			enricher = engine.NewCharacterEnricher(router)
		}
	}

	var portraitWorker *engine.PortraitWorker
	if cfg != nil && cfg.Media.Image.Type != "" && cfg.Media.Image.Type != "disabled" {
		if imgClient, err := imageClientFactory(cfg.Media.Image, cfg.Providers.Gemini.APIKey); err == nil && imgClient != nil {
			portraitWorker = engine.NewPortraitWorker(s.resolver, store, imgClient)
			portraitWorker.SetOnReady(func(gID, charID, relPath string) {
				s.broadcastPortraitReady(gID, charID, relPath)
			})
		}
	}

	for _, eSummary := range ents {
		if eSummary.Type != "character" {
			continue
		}
		ent, err := store.GetEntity(eSummary.ID)
		if err != nil || ent == nil {
			continue
		}

		if enricher != nil && enricher.NeedsEnrichment(ent) {
			if enriched, err := enricher.Enrich(context.Background(), ent, worldStyle); err == nil && enriched != nil {
				notePath := filepath.Join(s.resolver.GameDir(gameID), "entities", enriched.ID+".md")
				if data, err := enriched.SerializeMarkdown(); err == nil {
					_ = os.WriteFile(notePath, data, 0644)
					_ = storage.NewSyncer(store).SyncFile(notePath)
				}
				ent = enriched
			}
		}

		if portraitWorker != nil && (ent.Portrait == "" || !s.hasCustomPortrait(gameID, ent.ID)) {
			portraitWorker.Enqueue(gameID, ent, worldStyle)
		}
	}
}

// ListEntityMemories returns an entity's memories newest-first for the codex
// timeline.
func (s *Service) ListEntityMemories(gameID, entityID string, limit int) ([]MemoryDTO, error) {
	s.ensureIndexed(gameID)
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	memories, err := store.ListMemoriesForEntity(entityID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]MemoryDTO, 0, len(memories))
	for _, memory := range memories {
		out = append(out, MemoryDTO{
			Turn:       memory.Turn,
			Kind:       memory.Kind,
			Text:       memory.Text,
			Importance: memory.Importance,
			Tags:       memory.Tags,
		})
	}
	return out, nil
}
