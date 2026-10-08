package gui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/worldgen"
	"gopkg.in/yaml.v3"
)

// ErrEmptyDraftSelection reports a commit with nothing accepted.
var ErrEmptyDraftSelection = errors.New("nothing is accepted, so there is nothing to commit")

// GetDraft loads a persisted draft, so a reload resumes review.
func (s *Service) GetDraft(ctx context.Context, id string) (*WorldDraftDTO, error) {
	draft, err := worldgen.LoadDraft(s.worldDraftsDir(), id)
	if err != nil {
		return nil, err
	}
	dto := worldDraftDTO(draft, false)
	return &dto, nil
}

// ListDraftIDs reports the drafts awaiting review.
func (s *Service) ListDraftIDs(ctx context.Context) ([]string, error) {
	return worldgen.ListDrafts(s.worldDraftsDir())
}

// DiscardDraft deletes a draft. A draft that is already gone is not an error.
func (s *Service) DiscardDraft(ctx context.Context, id string) error {
	return worldgen.DeleteDraft(s.worldDraftsDir(), id)
}

// CommitDraft writes the accepted set from a draft, either as a new world or
// merged into an existing one, and deletes the draft on success.
func (s *Service) CommitDraft(ctx context.Context, req DraftCommitRequestDTO) (*WorldSummaryDTO, error) {
	draft, err := worldgen.LoadDraft(s.worldDraftsDir(), req.DraftID)
	if err != nil {
		return nil, err
	}

	// An absent list means "everything in the draft"; an empty one means the
	// user rejected everything, which is refused rather than guessed at.
	sections := draft.Sections
	if req.Sections != nil {
		sections = draftSections(req.Sections)
	}
	entities := draft.Entities
	if req.Entities != nil {
		entities = draftEntities(req.Entities)
	}
	if len(sections) == 0 && len(entities) == 0 {
		return nil, ErrEmptyDraftSelection
	}

	var summary *WorldSummaryDTO
	if req.TargetWorldID == "" {
		summary, err = s.commitNewWorld(draft, sections, entities, req.Meta)
	} else {
		summary, err = s.commitIntoWorld(ctx, req.TargetWorldID, sections, entities)
	}
	if err != nil {
		return nil, err
	}
	if err := worldgen.DeleteDraft(s.worldDraftsDir(), draft.ID); err != nil {
		return nil, err
	}
	return summary, nil
}

// commitNewWorld writes a new world atomically: everything is written to a
// temporary directory inside worlds/ and renamed into place, so a failure
// midway leaves no half-built world.
func (s *Service) commitNewWorld(draft worldgen.Draft, sections []worldgen.DraftSection, entities []worldgen.DraftEntity, meta *CreateWorldRequestDTO) (*WorldSummaryDTO, error) {
	manifest := core.WorldManifest{
		Name:        firstNonEmptyString(metaName(meta), draft.World.Name),
		Description: firstNonEmptyString(metaDescription(meta), draft.World.Description),
		Genre:       firstNonEmptyString(metaGenre(meta), draft.World.Genre),
		ArtStyle:    firstNonEmptyString(metaArtStyle(meta), draft.World.ArtStyle),
		Tags:        metaTags(meta, draft.World.Tags),
	}

	id := firstNonEmptyString(metaID(meta), draft.ID, slugify(manifest.Name))
	if err := pathutil.ValidateID(id); err != nil {
		return nil, fmt.Errorf("invalid world id %q: %w", id, err)
	}
	if _, err := os.Stat(filepath.Join(s.resolver.WorldDir(id), "world.yaml")); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrWorldExists, id)
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return nil, fmt.Errorf("world name is required")
	}
	manifest.ID = id

	worldsDir := s.resolver.WorldsDir()
	if err := os.MkdirAll(worldsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create worlds dir: %w", err)
	}
	tmpDir, err := os.MkdirTemp(worldsDir, ".commit-"+id+"-")
	if err != nil {
		return nil, fmt.Errorf("create staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := writeStagedWorld(tmpDir, manifest, sections, entities); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpDir, s.resolver.WorldDir(id)); err != nil {
		return nil, fmt.Errorf("commit world: %w", err)
	}

	return &WorldSummaryDTO{
		ID:          id,
		Name:        manifest.Name,
		Description: manifest.Description,
		Genre:       manifest.Genre,
		ArtStyle:    manifest.ArtStyle,
		Tags:        manifest.Tags,
	}, nil
}

// writeStagedWorld lays out a world inside a staging directory.
func writeStagedWorld(dir string, manifest core.WorldManifest, sections []worldgen.DraftSection, entities []worldgen.DraftEntity) error {
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal world manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "world.yaml"), data, 0o644); err != nil {
		return fmt.Errorf("write world.yaml: %w", err)
	}

	if lore := worldgen.RenderLore(sections); strings.TrimSpace(lore) != "" {
		if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0o755); err != nil {
			return fmt.Errorf("create world prompts dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "prompts", "lore.md"), []byte(lore), 0o644); err != nil {
			return fmt.Errorf("write lore.md: %w", err)
		}
	}

	if len(entities) == 0 {
		return nil
	}
	entitiesDir := filepath.Join(dir, "entities")
	if err := os.MkdirAll(entitiesDir, 0o755); err != nil {
		return fmt.Errorf("create world entities dir: %w", err)
	}
	seen := map[string]struct{}{}
	for _, e := range entities {
		id := e.ID
		if id == "" {
			id = slugify(e.Name)
		}
		if err := pathutil.ValidateID(id); err != nil {
			return fmt.Errorf("invalid entity id %q: %w", id, err)
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("%w: %s (twice in the draft)", ErrWorldEntityExists, id)
		}
		seen[id] = struct{}{}
		e.ID = id
		// A draft entity may name a folder; an import groups its notes by kind, so
		// a source that yields sixty entities does not land as one flat list.
		target := entitiesDir
		if folder := strings.Trim(strings.TrimSpace(e.Folder), "/"); folder != "" {
			if _, err := ValidateFolderPath(folder); err != nil {
				return fmt.Errorf("invalid entity folder %q: %w", folder, err)
			}
			target = filepath.Join(entitiesDir, filepath.FromSlash(folder))
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create entity folder %s: %w", folder, err)
			}
		}
		path, err := pathutil.ResolveSafeChild(target, id+".md")
		if err != nil {
			return fmt.Errorf("resolve entity %s: %w", id, err)
		}
		if err := os.WriteFile(path, []byte(worldgen.RenderEntityNote(e)), 0o644); err != nil {
			return fmt.Errorf("write entity %s: %w", id, err)
		}
	}
	return nil
}

// commitIntoWorld merges a draft into an existing world: the accepted lore
// sections are appended (never replacing the user's prose) and the accepted
// entities are written, refusing an id clash.
func (s *Service) commitIntoWorld(ctx context.Context, worldID string, sections []worldgen.DraftSection, entities []worldgen.DraftEntity) (*WorldSummaryDTO, error) {
	world, err := s.worldContext(worldID)
	if err != nil {
		return nil, err
	}

	if len(sections) > 0 {
		lorePath := filepath.Join(s.resolver.WorldDir(worldID), "prompts", "lore.md")
		existing, err := os.ReadFile(lorePath)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read lore: %w", err)
		}
		proposals := make([]worldgen.Enhancement, 0, len(sections))
		for _, section := range sections {
			proposals = append(proposals, worldgen.Enhancement{
				Kind: worldgen.KindLore, Title: section.Title, Body: section.Body,
			})
		}
		updated := worldgen.ApplyLore(string(existing), proposals)
		if updated != string(existing) {
			if err := os.MkdirAll(filepath.Dir(lorePath), 0o755); err != nil {
				return nil, fmt.Errorf("create world prompts dir: %w", err)
			}
			if err := os.WriteFile(lorePath, []byte(updated), 0o644); err != nil {
				return nil, fmt.Errorf("write lore: %w", err)
			}
		}
	}

	if len(entities) > 0 {
		incoming := make([]WorldDraftEntityDTO, 0, len(entities))
		for _, e := range entities {
			incoming = append(incoming, draftEntityDTO(e))
		}
		if _, err := s.AcceptWorldEntities(ctx, worldID, WorldEntityAcceptRequestDTO{Entities: incoming}); err != nil {
			return nil, err
		}
	}

	summary := &WorldSummaryDTO{
		ID:          worldID,
		Name:        world.Name,
		Description: world.Description,
		Genre:       world.Genre,
	}
	return summary, nil
}

// draftEntities converts client-supplied entities back into the pipeline's type.
func draftEntities(entities []WorldDraftEntityDTO) []worldgen.DraftEntity {
	out := make([]worldgen.DraftEntity, 0, len(entities))
	for _, e := range entities {
		out = append(out, draftEntity(e))
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func metaID(meta *CreateWorldRequestDTO) string {
	if meta == nil {
		return ""
	}
	return meta.ID
}

func metaName(meta *CreateWorldRequestDTO) string {
	if meta == nil {
		return ""
	}
	return meta.Name
}

func metaDescription(meta *CreateWorldRequestDTO) string {
	if meta == nil {
		return ""
	}
	return meta.Description
}

func metaGenre(meta *CreateWorldRequestDTO) string {
	if meta == nil {
		return ""
	}
	return meta.Genre
}

func metaArtStyle(meta *CreateWorldRequestDTO) string {
	if meta == nil {
		return ""
	}
	return meta.ArtStyle
}

func metaTags(meta *CreateWorldRequestDTO, fallback []string) []string {
	if meta == nil || len(meta.Tags) == 0 {
		return fallback
	}
	return meta.Tags
}
