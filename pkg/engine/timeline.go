package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
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
	// A turn's mentions are what later recall reasons about, so the names its prose
	// contains are recorded here rather than left to whichever writer remembered to
	// resolve them. Extraction records its own, and a link or a spoken line is
	// already recorded, so this only adds what none of them saw.
	for _, mention := range harness.ResolveProseMentions(t.store, turn.Narration, turn.Input) {
		if !containsMention(turn.Entities, mention.ID) {
			turn.Entities = append(turn.Entities, mention)
		}
	}

	pending, err := t.stageEntities(turn, extracted)
	if err != nil {
		return err
	}

	if len(pending) > 0 {
		for id, ent := range pending {
			ent.History = appendTurnNumber(ent.History, turn.Number)
			pending[id] = ent
		}
		if err := t.writeEntities(pending); err != nil {
			return err
		}
	}

	if err := t.history.AppendTurn(*turn); err != nil {
		return fmt.Errorf("append turn: %w", err)
	}

	return t.indexTurn(*turn)
}

func (t *Timeline) stageEntities(turn *Turn, extracted []harness.ExtractedEntity) (map[string]*entity.Entity, error) {
	pending := make(map[string]*entity.Entity, len(turn.Entities)+len(extracted))

	for _, mention := range turn.Entities {
		existing, err := t.store.GetEntity(mention.ID)
		if err != nil || existing == nil {
			continue
		}
		pending[existing.ID] = existing
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

		if ent.Type == "character" {
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

	if count == len(turns) && max == lastTurnNumber(turns) {
		return nil
	}
	return t.indexTurns(turns)
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
