package gui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/ingest"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/worldgen"
)

// ErrWorldEntityExists reports an accept that would overwrite an existing note.
var ErrWorldEntityExists = errors.New("world entity already exists")

// worldContext reads a world into the shape the generators need: its manifest,
// its lore, and its existing entities.
func (s *Service) worldContext(worldID string) (worldgen.WorldContext, error) {
	if err := pathutil.ValidateID(worldID); err != nil {
		return worldgen.WorldContext{}, fmt.Errorf("invalid world id: %w", err)
	}
	worldDir := s.resolver.WorldDir(worldID)
	manifest, err := core.LoadWorldManifest(filepath.Join(worldDir, "world.yaml"))
	if err != nil {
		return worldgen.WorldContext{}, fmt.Errorf("%w: %s", ErrWorldNotFound, worldID)
	}

	ctx := worldgen.WorldContext{
		ID:          manifest.ID,
		Name:        manifest.Name,
		Genre:       manifest.Genre,
		Description: manifest.Description,
	}
	if ctx.ID == "" {
		ctx.ID = worldID
	}
	if data, err := os.ReadFile(filepath.Join(worldDir, "prompts", "lore.md")); err == nil {
		ctx.Lore = string(data)
	}

	_ = eachEntityNote(filepath.Join(worldDir, "entities"), func(path, folder string, data []byte) error {
		// A template is identified by its file name, the same way the studio
		// lists it, so a generated link resolves to what the user sees.
		id := strings.TrimSuffix(filepath.Base(path), ".md")
		name, kind, summary := id, "concept", ""
		if parsed, err := entity.ParseMarkdownEntity(data); err == nil {
			if parsed.Name != "" {
				name = parsed.Name
			}
			if parsed.Type != "" {
				kind = parsed.Type
			}
			summary = summaryLine(parsed.Body)
		}
		ctx.Entities = append(ctx.Entities, worldgen.EntitySummary{ID: id, Name: name, Type: kind, Summary: summary})
		return nil
	})
	return ctx, nil
}

// summaryLine is the first non-empty line of a note body, used as a one-line
// description in a generation prompt.
func summaryLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// PreviewWorldEntities generates a batch of entities for an existing world
// without writing anything. The batch comes from an instruction, or from a
// folder or a set of URLs when the request names a source. The user accepts or
// discards the preview.
func (s *Service) PreviewWorldEntities(ctx context.Context, worldID string, req WorldEntityBatchRequestDTO) (*WorldEntityBatchDTO, error) {
	return s.PreviewWorldEntitiesStream(ctx, worldID, req, nil)
}

// PreviewWorldEntitiesStream is PreviewWorldEntities with progress: one event per
// batch, so a long import can report what it is reading. An extraction of a large
// folder is many calls, and a caller that shows nothing for it looks hung.
func (s *Service) PreviewWorldEntitiesStream(ctx context.Context, worldID string, req WorldEntityBatchRequestDTO, emit func(TurnEvent) error) (*WorldEntityBatchDTO, error) {
	world, err := s.worldContext(worldID)
	if err != nil {
		return nil, err
	}

	limits := s.limitsFor(req.Limits)
	resolved := s.resolveWorldGenerator()

	var batch []worldgen.DraftEntity
	var cutOff int
	if req.Source != nil {
		// An import reads the user's source, so the template fallback cannot
		// stand in for a model: it would answer with fixed names and no error.
		gen, err := s.generatorForSource()
		if err != nil {
			return nil, err
		}
		chunks, err := s.extractSource(ctx, *req.Source, limits.MaxChunks)
		if err != nil {
			return nil, err
		}
		// An extraction is as big as the source is, so its budget follows the
		// chunk cap rather than the bounded pipeline's call cap.
		fromSource := &worldgen.BudgetGenerator{Inner: gen, Max: worldgen.ChunkCalls(len(chunks))}
		batch, cutOff, err = s.entitiesFromChunks(ctx, fromSource, world, req, chunks, emit)
		if err != nil {
			return nil, err
		}
	} else {
		budget := &worldgen.BudgetGenerator{Inner: resolved.Generator, Max: limits.MaxCalls}
		batch, err = worldgen.GenerateEntities(ctx, budget, world, worldgen.EntityRequest{
			WorldID:     worldID,
			Instruction: req.Instruction,
			Kinds:       req.Kinds,
			Count:       req.Count,
			Focus:       req.Focus,
		})
		if err != nil {
			if errors.Is(err, worldgen.ErrCallBudgetExceeded) {
				return nil, fmt.Errorf("generation stopped: %w", err)
			}
			return nil, err
		}
	}

	out := &WorldEntityBatchDTO{
		Entities: make([]WorldDraftEntityDTO, 0, len(batch)),
		Oracle:   resolved.Oracle,
		CutOff:   cutOff,
	}
	for _, e := range batch {
		out.Entities = append(out.Entities, draftEntityDTO(e))
	}
	// The batch is stored, not just returned: an extraction of a few hundred
	// entities is far more than a request body can carry back, so accepting it
	// names the batch and the entities are read from here.
	if err := worldgen.SaveDraft(s.batchStoreDir(), worldgen.Draft{ID: batchIDFor(worldID), Entities: batch}); err != nil {
		return nil, err
	}
	out.BatchID = batchIDFor(worldID)
	return out, nil
}

// batchStoreDir is where previewed batches live: beside the drafts, in the same
// dot-directory the syncer skips.
func (s *Service) batchStoreDir() string {
	return filepath.Join(s.worldDraftsDir(), worldgen.BatchesDirName)
}

// batchIDFor is the stored batch of the world's most recent extraction. One
// batch per world is enough, and a re-run replaces it.
func batchIDFor(worldID string) string {
	return worldID + "-entities"
}

// batchEntities reads the stored batch, narrowed to the named ids. An empty ids
// list means the whole batch.
func (s *Service) batchEntities(batchID string, ids []string) ([]worldgen.DraftEntity, error) {
	draft, err := worldgen.LoadDraft(s.batchStoreDir(), batchID)
	if err != nil {
		return nil, fmt.Errorf("read the extracted batch: %w", err)
	}
	if len(ids) == 0 {
		return draft.Entities, nil
	}

	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	accepted := make([]worldgen.DraftEntity, 0, len(ids))
	for _, e := range draft.Entities {
		if _, ok := wanted[e.ID]; ok {
			accepted = append(accepted, e)
		}
	}
	if len(accepted) == 0 {
		return nil, fmt.Errorf("none of the %d named entities are in the batch", len(ids))
	}
	return accepted, nil
}

// entitiesFromChunks extracts entities from already-read chunks, links them
// against the world they are being added to, and reports each batch. It also
// reports how many batches ran out of room, so a short batch is not mistaken for
// a complete one after the progress has gone.
func (s *Service) entitiesFromChunks(ctx context.Context, gen worldgen.Generator, world worldgen.WorldContext, req WorldEntityBatchRequestDTO, chunks []ingest.Chunk, emit func(TurnEvent) error) ([]worldgen.DraftEntity, int, error) {
	var cutOff int
	draft, err := ingest.BuildInto(ctx, gen, chunks, worldgen.Brief{Premise: req.Instruction}, ingest.BuildContext{
		Name:        world.Name,
		Genre:       world.Genre,
		Description: world.Description,
		Lore:        world.Lore,
		Entities:    world.Entities,
	}, func(p ingest.Progress) {
		cutOff = p.CutOff
		if emit != nil {
			_ = emit(TurnEvent{Type: WorldEventProgress, Progress: importProgressDTO(p)})
		}
	})
	if err != nil {
		if errors.Is(err, worldgen.ErrCallBudgetExceeded) {
			return nil, 0, fmt.Errorf("generation stopped: %w", err)
		}
		return nil, 0, err
	}
	return draft.Entities, cutOff, nil
}

// AcceptWorldEntities writes a reviewed batch into worlds/<id>/entities/. It
// refuses an id clash unless the caller asked for a rename.
func (s *Service) AcceptWorldEntities(ctx context.Context, worldID string, req WorldEntityAcceptRequestDTO) (*WorldEnhanceApplyResultDTO, error) {
	incoming := req.Entities
	if req.BatchID != "" {
		stored, err := s.batchEntities(req.BatchID, req.IDs)
		if err != nil {
			return nil, err
		}
		incoming = make([]WorldDraftEntityDTO, 0, len(stored))
		for _, e := range stored {
			incoming = append(incoming, draftEntityDTO(e))
		}
	}
	if len(incoming) == 0 {
		return nil, fmt.Errorf("no entities to accept")
	}
	worldDir := s.resolver.WorldDir(worldID)
	if _, err := os.Stat(filepath.Join(worldDir, "world.yaml")); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrWorldNotFound, worldID)
	}

	result := &WorldEnhanceApplyResultDTO{}
	seen := map[string]struct{}{}
	for _, incoming := range incoming {
		e := draftEntity(incoming)
		if e.ID == "" {
			e.ID = entity.Slugify(e.Name)
		}
		if err := pathutil.ValidateID(e.ID); err != nil {
			return nil, fmt.Errorf("invalid entity id %q: %w", e.ID, err)
		}
		if _, dup := seen[e.ID]; dup {
			return nil, fmt.Errorf("%w: %s (twice in the batch)", ErrWorldEntityExists, e.ID)
		}
		seen[e.ID] = struct{}{}

		existing, err := findWorldEntityNote(worldDir, e.ID)
		if err != nil {
			return nil, err
		}
		if existing != "" {
			if !req.Rename {
				return nil, fmt.Errorf("%w: %s", ErrWorldEntityExists, e.ID)
			}
			e.ID = uniqueEntityID(worldDir, e.ID)
			result.Renamed = append(result.Renamed, e.ID)
		}
		if err := s.SaveWorldEntity(ctx, worldID, e.ID, e.Folder, worldgen.RenderEntityNote(e)); err != nil {
			return nil, err
		}
		result.Written = append(result.Written, e.ID)
	}
	// The batch has been written, so the stored copy is spent. A partial failure
	// above leaves it in place, so a retry still has the entities.
	if req.BatchID != "" {
		_ = worldgen.DeleteDraft(s.batchStoreDir(), req.BatchID)
	}
	return result, nil
}

// uniqueEntityID finds a free id for a note whose preferred id is taken, by
// suffixing a counter. It never reuses a suffix that is already on disk.
func uniqueEntityID(worldDir, id string) string {
	for n := 2; n < 1000; n++ {
		candidate := fmt.Sprintf("%s-%d", id, n)
		path, err := findWorldEntityNote(worldDir, candidate)
		if err != nil {
			return candidate
		}
		if path == "" {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", id, len(id))
}

// EnhanceWorld proposes lore additions, new entities, and story hooks for an
// existing world. Nothing is written; the caller applies the accepted set.
func (s *Service) EnhanceWorld(ctx context.Context, worldID string, req WorldEnhanceRequestDTO) (*WorldEnhanceResponseDTO, error) {
	world, err := s.worldContext(worldID)
	if err != nil {
		return nil, err
	}

	resolved := s.resolveWorldGenerator()
	budget := &worldgen.BudgetGenerator{Inner: resolved.Generator, Max: s.configMgr.Get().GenerationMaxCalls()}

	proposals, err := worldgen.Enhance(ctx, budget, world, req.Instruction, req.Kinds)
	if err != nil {
		if errors.Is(err, worldgen.ErrCallBudgetExceeded) {
			return nil, fmt.Errorf("generation stopped: %w", err)
		}
		return nil, err
	}

	out := &WorldEnhanceResponseDTO{
		Proposals: make([]WorldEnhancementDTO, 0, len(proposals)),
		Oracle:    resolved.Oracle,
	}
	for _, p := range proposals {
		out.Proposals = append(out.Proposals, enhancementDTO(p))
	}
	return out, nil
}

// ApplyWorldEnhancements writes the accepted proposals: lore and hooks are
// appended to prompts/lore.md, and entity proposals are written like a batch.
// Applying an empty set changes nothing.
func (s *Service) ApplyWorldEnhancements(ctx context.Context, worldID string, req WorldEnhanceApplyRequestDTO) (*WorldEnhanceApplyResultDTO, error) {
	worldDir := s.resolver.WorldDir(worldID)
	if _, err := os.Stat(filepath.Join(worldDir, "world.yaml")); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrWorldNotFound, worldID)
	}

	result := &WorldEnhanceApplyResultDTO{}
	if len(req.Proposals) == 0 {
		return result, nil
	}

	var loreProposals, hookProposals []worldgen.Enhancement
	var entityProposals []WorldDraftEntityDTO
	for _, p := range req.Proposals {
		converted := enhancement(p)
		switch converted.Kind {
		case worldgen.KindLore:
			loreProposals = append(loreProposals, converted)
		case worldgen.KindHook:
			hookProposals = append(hookProposals, converted)
		case worldgen.KindEntity:
			if converted.Entity != nil {
				entityProposals = append(entityProposals, draftEntityDTO(*converted.Entity))
			}
		}
	}

	lorePath := filepath.Join(worldDir, "prompts", "lore.md")
	existing, err := os.ReadFile(lorePath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read lore: %w", err)
	}
	updated := worldgen.ApplyLore(string(existing), loreProposals)
	updated = worldgen.ApplyHooks(updated, hookProposals)
	if updated != string(existing) {
		if err := os.MkdirAll(filepath.Dir(lorePath), 0o755); err != nil {
			return nil, fmt.Errorf("create world prompts dir: %w", err)
		}
		if err := os.WriteFile(lorePath, []byte(updated), 0o644); err != nil {
			return nil, fmt.Errorf("write lore: %w", err)
		}
		result.Written = append(result.Written, "prompts/lore.md")
	}

	if len(entityProposals) > 0 {
		written, err := s.AcceptWorldEntities(ctx, worldID, WorldEntityAcceptRequestDTO{
			Entities: entityProposals,
			Rename:   req.Rename,
		})
		if err != nil {
			return nil, err
		}
		result.Written = append(result.Written, written.Written...)
		result.Renamed = append(result.Renamed, written.Renamed...)
	}
	return result, nil
}

func enhancementDTO(p worldgen.Enhancement) WorldEnhancementDTO {
	dto := WorldEnhancementDTO{
		Kind:   p.Kind,
		Title:  p.Title,
		Body:   p.Body,
		Target: p.Target,
		Reason: p.Reason,
	}
	if p.Entity != nil {
		entity := draftEntityDTO(*p.Entity)
		dto.Entity = &entity
	}
	return dto
}

func enhancement(p WorldEnhancementDTO) worldgen.Enhancement {
	out := worldgen.Enhancement{
		Kind:   p.Kind,
		Title:  p.Title,
		Body:   p.Body,
		Target: p.Target,
		Reason: p.Reason,
	}
	if p.Entity != nil {
		entity := draftEntity(*p.Entity)
		out.Entity = &entity
	}
	return out
}
