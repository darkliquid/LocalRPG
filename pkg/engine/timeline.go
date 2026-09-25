package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/state"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// Timeline owns every write a turn makes across the campaign's entity notes,
// history.jsonl, and the SQLite index.
type Timeline struct {
	paths         *core.PathResolver
	store         *storage.Store
	history       *HistoryLogger
	gameID        string
	voiceProfiles []config.VoiceProfile
}

func NewTimeline(paths *core.PathResolver, store *storage.Store, history *HistoryLogger, gameID string) *Timeline {
	return &Timeline{paths: paths, store: store, history: history, gameID: gameID}
}

// GameID is the campaign this timeline records.
func (t *Timeline) GameID() string {
	return t.gameID
}

// SetVoiceProfiles provides the archetypes assigned to newly discovered characters.
func (t *Timeline) SetVoiceProfiles(profiles []config.VoiceProfile) {
	t.voiceProfiles = profiles
}

// VoiceProfiles returns the configured archetypes for prompt assembly.
func (t *Timeline) VoiceProfiles() []config.VoiceProfile {
	return t.voiceProfiles
}

// RecordTurn persists everything a turn changed: the affected entity notes, the
// turn record, and the index row. Notes are written before the turn is appended,
// so the record never points at a note that does not exist.
func (t *Timeline) RecordTurn(turn *Turn, extracted []harness.ExtractedEntity) error {
	return t.RecordTurnContext(context.Background(), turn, extracted)
}

// RecordTurnContext is RecordTurn with a caller context, so the write can join
// the turn's trace. Callers that have no context use RecordTurn.
func (t *Timeline) RecordTurnContext(ctx context.Context, turn *Turn, extracted []harness.ExtractedEntity) error {
	return t.RecordTurnContextStructured(ctx, turn, extracted, nil, nil, nil)
}

// RecordTurnContextStructured is RecordTurnContext with the structured turn's
// persona declarations and memories, which are staged as entity stubs and memory
// records (unlike extraction, they carry declared state).
func (t *Timeline) RecordTurnContextStructured(ctx context.Context, turn *Turn, extracted []harness.ExtractedEntity, personae []harness.PersonaDecl, memories []harness.MemoryDecl, checks []harness.CheckResult) error {
	_, span := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "timeline.record_turn",
		oteltrace.WithAttributes(attribute.Int("localrpg.turn.number", turn.Number)),
	)
	defer span.End()

	// A turn's mentions are what later recall reasons about, so the names its prose
	// contains are recorded here rather than left to whichever writer remembered to
	// resolve them. Extraction records its own, and a link or a spoken line is
	// already recorded, so this only adds what none of them saw.
	for _, mention := range harness.ResolveProseMentions(t.store, turn.Narration, turn.Input) {
		if !containsMention(turn.Entities, mention.ID) {
			turn.Entities = append(turn.Entities, mention)
		}
	}

	pending, err := t.stageEntities(turn, extracted, personae)
	if err != nil {
		span.RecordError(err)
		return err
	}

	if len(pending) > 0 {
		for id, ent := range pending {
			ent.History = appendTurnNumber(ent.History, turn.Number)
			pending[id] = ent
		}
		if err := t.writeEntities(pending); err != nil {
			span.RecordError(err)
			return err
		}
	}

	if err := t.stageMemories(turn, memories); err != nil {
		span.RecordError(err)
		return err
	}
	if err := t.writeMechanicalMemories(turn, checks); err != nil {
		span.RecordError(err)
		return err
	}

	if err := t.history.AppendTurn(*turn); err != nil {
		err = fmt.Errorf("append turn: %w", err)
		span.RecordError(err)
		return err
	}

	if turn.Context != nil && t.store != nil {
		rawCtx, err := json.Marshal(turn.Context)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("marshal turn context: %w", err)
		}
		if err := t.store.SaveTurnContext(turn.Number, turn.Prompt, rawCtx); err != nil {
			span.RecordError(err)
			return fmt.Errorf("save turn context: %w", err)
		}
	}

	if err := t.indexTurn(*turn); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}

func (t *Timeline) stageEntities(turn *Turn, extracted []harness.ExtractedEntity, personae []harness.PersonaDecl) (map[string]*entity.Entity, error) {
	pending := make(map[string]*entity.Entity, len(turn.Entities)+len(extracted)+len(personae))

	for _, mention := range turn.Entities {
		existing, err := t.store.GetEntity(mention.ID)
		if err != nil || existing == nil {
			continue
		}
		pending[existing.ID] = existing
	}

	for _, persona := range personae {
		if strings.TrimSpace(persona.Name) == "" {
			continue
		}
		id := entity.Slugify(persona.Name)
		if id == "" {
			continue
		}
		ent, err := t.store.GetEntity(id)
		if err != nil || ent == nil {
			ent = &entity.Entity{ID: id, Name: persona.Name, Type: persona.Type, Wikilinks: make([]string, 0)}
			if ent.Type == "" {
				ent.Type = "character"
			}
			if ent.State == nil {
				ent.State = state.NewState(nil)
			}
			if persona.Gender != "" {
				ent.State.Set("gender", persona.Gender)
			}
			if persona.Pronouns != "" {
				ent.State.Set("pronouns", persona.Pronouns)
			}
			if len(persona.RoleTags) > 0 {
				ent.Tags = append(ent.Tags, persona.RoleTags...)
			}
			ent.Body = persona.Description
		}
		if entity.IsCharacterType(ent.Type) {
			harness.AssignVoiceProfile(ent, t.voiceProfiles)
		}
		pending[ent.ID] = ent
		if !containsMention(turn.Entities, ent.ID) {
			turn.Entities = append(turn.Entities, entity.Mention{ID: ent.ID, Kind: entity.MentionExtracted})
		}
	}

	for _, raw := range extracted {
		if strings.TrimSpace(raw.Name) == "" {
			continue
		}

		ent := harness.MatchExistingEntity(t.store, &raw)
		if ent != nil {
			ent = harness.MergeExtractedEntity(ent, &raw)
		} else {
			id := entity.Slugify(raw.Name)
			if id == "" {
				continue
			}
			if existing, err := t.store.GetEntity(id); err == nil && existing != nil {
				ent = harness.MergeExtractedEntity(existing, &raw)
			} else {
				ent = &entity.Entity{
					ID:        id,
					Name:      raw.Name,
					Type:      raw.Type,
					Location:  raw.Location,
					Faction:   raw.Faction,
					Body:      raw.Body,
					Wikilinks: make([]string, 0),
				}
			}
		}

		if entity.IsCharacterType(ent.Type) {
			harness.AssignVoiceProfile(ent, t.voiceProfiles)
		}

		pending[ent.ID] = ent
		if !containsMention(turn.Entities, ent.ID) {
			turn.Entities = append(turn.Entities, entity.Mention{ID: ent.ID, Kind: entity.MentionExtracted})
		}
	}

	return pending, nil
}

func (t *Timeline) writeEntities(pending map[string]*entity.Entity) error {
	dir := t.EntitiesDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create entities dir: %w", err)
	}

	ids := make([]string, 0, len(pending))
	for id := range pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		data, err := pending[id].SerializeMarkdown()
		if err != nil {
			return fmt.Errorf("serialize entity %q: %w", id, err)
		}
		if err := os.WriteFile(filepath.Join(dir, id+".md"), data, 0644); err != nil {
			return fmt.Errorf("write entity %q: %w", id, err)
		}
	}

	if _, err := storage.NewSyncer(t.store).Sync(dir); err != nil {
		return fmt.Errorf("sync entities: %w", err)
	}
	return nil
}

func containsMention(mentions []entity.Mention, id string) bool {
	for _, mention := range mentions {
		if mention.ID == id {
			return true
		}
	}
	return false
}

func appendTurnNumber(history []int, turnNumber int) []int {
	for _, number := range history {
		if number == turnNumber {
			return history
		}
	}
	return append(history, turnNumber)
}

// SetPlayerLocation points a player note at a location, writing the note before the
// index so the Markdown stays the source of truth.
func (t *Timeline) SetPlayerLocation(playerID, locationID string) error {
	player, err := t.store.GetEntity(playerID)
	if err != nil || player == nil {
		return fmt.Errorf("player %q not found: %w", playerID, err)
	}

	player.Location = "[[" + locationID + "]]"
	return t.writeEntities(map[string]*entity.Entity{player.ID: player})
}

// SaveEntity persists one entity note and reindexes it. It lets collaborators
// outside the engine, such as the rules host bridge, write through the same path
// instead of inventing a second one.
func (t *Timeline) SaveEntity(ent *entity.Entity) error {
	return t.writeEntities(map[string]*entity.Entity{ent.ID: ent})
}

// RewindToTurn discards every turn after target from the log, the index, and each
// entity's history list. Entity prose and state are deliberately left alone:
// undo trims the record of what happened, not the world's memory of it.
func (t *Timeline) RewindToTurn(target int) error {
	if target < 0 {
		target = 0
	}

	turns, err := t.history.LoadHistory()
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}

	affected := make(map[string]bool)
	for _, turn := range turns {
		if turn.Number <= target {
			continue
		}
		for _, mention := range turn.Entities {
			affected[mention.ID] = true
		}
	}

	if len(affected) > 0 {
		if err := t.pruneEntityHistory(affected, target); err != nil {
			return err
		}
	}

	if err := t.history.RewindToTurn(target); err != nil {
		return fmt.Errorf("rewind history: %w", err)
	}
	if err := t.store.DeleteTurnsFrom(target + 1); err != nil {
		return fmt.Errorf("delete indexed turns: %w", err)
	}
	return nil
}

func (t *Timeline) pruneEntityHistory(affected map[string]bool, target int) error {
	pending := make(map[string]*entity.Entity, len(affected))

	for id := range affected {
		ent, err := t.store.GetEntity(id)
		if err != nil || ent == nil {
			continue
		}

		pruned := make([]int, 0, len(ent.History))
		for _, number := range ent.History {
			if number <= target {
				pruned = append(pruned, number)
			}
		}
		if len(pruned) == len(ent.History) {
			continue
		}
		ent.History = pruned
		pending[id] = ent
	}

	if len(pending) == 0 {
		return nil
	}
	return t.writeEntities(pending)
}

// EntitiesDir returns the campaign directory holding entity notes.
func (t *Timeline) EntitiesDir() string {
	return filepath.Join(t.paths.GameDir(t.gameID), "entities")
}

// SyncTurns replays history.jsonl into the index and drops rows for turns the log
// no longer contains. It returns the number of replayed turns.
func (t *Timeline) SyncTurns() (int, error) {
	turns, err := t.history.LoadHistory()
	if err != nil {
		return 0, fmt.Errorf("load history: %w", err)
	}
	if err := t.indexTurns(turns); err != nil {
		return 0, err
	}
	return len(turns), nil
}

// EnsureIndexed repairs the turn index when it disagrees with history.jsonl,
// which is the only durable record of the timeline.
func (t *Timeline) EnsureIndexed() error {
	turns, err := t.history.LoadHistory()
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}

	count, err := t.store.CountTurns()
	if err != nil {
		return fmt.Errorf("count indexed turns: %w", err)
	}
	max, err := t.store.MaxTurnNumber()
	if err != nil {
		return fmt.Errorf("read max indexed turn: %w", err)
	}

	if count != len(turns) || max != lastTurnNumber(turns) {
		if err := t.indexTurns(turns); err != nil {
			return err
		}
	}

	// Memories are canonical in the index but re-derivable from a turn's stored
	// records, so a rebuilt database regains them here.
	if t.store != nil {
		for i := range turns {
			if err := t.ensureTurnMemories(&turns[i]); err != nil {
				return err
			}
		}
	}

	if t.store != nil && len(turns) > 0 {
		wsEntries, err := t.store.LoadWorkingSet()
		if err == nil && len(wsEntries) == 0 {
			window := DefaultRederiveWindow
			start := len(turns) - window
			if start < 0 {
				start = 0
			}
			set := WorkingSet{}
			rederived := set.Rederive(turns[start:])
			_ = t.store.ReplaceWorkingSet(rederived.ToStorageRecords())
		}
	}

	return nil
}

func (t *Timeline) indexTurns(turns []Turn) error {
	for _, turn := range turns {
		if err := t.indexTurn(turn); err != nil {
			return err
		}
	}

	highest := lastTurnNumber(turns)
	indexed, err := t.store.MaxTurnNumber()
	if err != nil {
		return fmt.Errorf("read max indexed turn: %w", err)
	}
	if indexed > highest {
		if err := t.store.DeleteTurnsFrom(highest + 1); err != nil {
			return fmt.Errorf("prune indexed turns: %w", err)
		}
	}
	return nil
}

func (t *Timeline) indexTurn(turn Turn) error {
	if err := t.store.SaveTurn(turnRecord(turn)); err != nil {
		return fmt.Errorf("index turn %d: %w", turn.Number, err)
	}
	return nil
}

func lastTurnNumber(turns []Turn) int {
	if len(turns) == 0 {
		return 0
	}
	return turns[len(turns)-1].Number
}

func turnRecord(turn Turn) storage.TurnRecord {
	rec := storage.TurnRecord{
		Number:    turn.Number,
		Timestamp: turn.Timestamp,
		Mode:      turn.Mode,
		Input:     turn.Input,
		Narration: turn.Prose(),
		Location:  turn.Location,
		Outcome:   turn.Outcome,
	}

	if turn.Roll != nil {
		if data, err := json.Marshal(turn.Roll); err == nil {
			rec.RollJSON = string(data)
		}
	}
	for _, mention := range turn.Entities {
		rec.Entities = append(rec.Entities, storage.TurnEntityRef{
			EntityID: mention.ID,
			Mention:  mention.Kind,
			Outcome:  turn.Outcome,
		})
	}
	return rec
}

// stageMemories resolves and stores the GM's memory declarations, attaching the
// persisted records to the turn so history.jsonl stays canonical. An invalid
// memory is dropped rather than failing the turn.
func (t *Timeline) stageMemories(turn *Turn, decls []harness.MemoryDecl) error {
	for _, decl := range decls {
		refs := make([]string, 0, len(decl.EntityRefs))
		for _, raw := range decl.EntityRefs {
			if id := t.resolveMemoryRef(raw); id != "" {
				refs = append(refs, id)
			}
		}
		memory := entity.Memory{
			Turn:       turn.Number,
			Kind:       decl.Kind,
			EntityRefs: refs,
			Text:       decl.Text,
			Importance: decl.Importance,
			Tags:       decl.Tags,
			Source:     entity.SourceGM,
		}
		if err := entity.ValidateMemory(&memory); err != nil {
			continue
		}
		id, err := t.store.SaveMemory(&memory)
		if err != nil {
			return fmt.Errorf("save memory: %w", err)
		}
		memory.ID = id
		turn.Memories = append(turn.Memories, memory)
	}
	return nil
}

// writeMechanicalMemories records one engine-owned memory per resolved check,
// linked to the check and its actor/target.
func (t *Timeline) writeMechanicalMemories(turn *Turn, checks []harness.CheckResult) error {
	for _, check := range checks {
		if check.CheckID == "" {
			continue
		}
		refs := make([]string, 0, 2)
		if id := t.resolveMemoryRef(check.Actor); id != "" {
			refs = append(refs, id)
		}
		if check.Target != "" {
			if id := t.resolveMemoryRef(check.Target); id != "" && id != check.Actor {
				refs = append(refs, id)
			}
		}
		if len(refs) == 0 {
			continue
		}
		detail := check.Outcome
		if check.Roll != nil {
			detail = fmt.Sprintf("%s=%d, %s", check.Roll.Notation, check.Roll.Total, check.Outcome)
		}
		memory := entity.Memory{
			Turn:       turn.Number,
			Kind:       entity.MemoryMechanical,
			EntityRefs: refs,
			Text:       fmt.Sprintf("Resolved a check: %s.", detail),
			Importance: 3,
			Tags:       []string{"check"},
			Source:     entity.SourceEngine,
			CheckID:    check.CheckID,
		}
		if err := entity.ValidateMemory(&memory); err != nil {
			continue
		}
		id, err := t.store.SaveMemory(&memory)
		if err != nil {
			return fmt.Errorf("save mechanical memory: %w", err)
		}
		memory.ID = id
		turn.Memories = append(turn.Memories, memory)
	}
	return nil
}

// resolveMemoryRef turns an id or a name into an entity id, preferring an
// existing entity and falling back to the slug of the name.
func (t *Timeline) resolveMemoryRef(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if ent, err := t.store.GetEntity(raw); err == nil && ent != nil {
		return ent.ID
	}
	id := entity.Slugify(raw)
	if id == "" {
		return ""
	}
	if ent, err := t.store.GetEntity(id); err == nil && ent != nil {
		return ent.ID
	}
	return id
}

// ensureTurnMemories re-saves a turn's recorded memories when the index is
// missing them, keyed on content so a rebuild does not duplicate rows.
func (t *Timeline) ensureTurnMemories(turn *Turn) error {
	for _, memory := range turn.Memories {
		exists, err := t.store.HasMemory(memory.Turn, memory.Kind, memory.Text, memory.CheckID)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := entity.ValidateMemory(&memory); err != nil {
			continue
		}
		if _, err := t.store.SaveMemory(&memory); err != nil {
			return fmt.Errorf("rebuild memory: %w", err)
		}
	}
	return nil
}
