package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestTimelineEnsureIndexedReplaysHistory(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	store := newTestStore(t)
	history := NewHistoryLogger(filepath.Join(t.TempDir(), "history.jsonl"))
	timeline := NewTimeline(paths, store, history, "campaign-01")

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"You look around.","entities":[{"id":"hero","mention":"player"},{"id":"garrick","mention":"extracted"}]}` + "\n"
	record += `{"number":2,"timestamp":"2026-09-21T10:05:00Z","mode":"Say","input":"hello","narration":"Garrick nods.","entities":[{"id":"garrick","mention":"wikilink"}]}` + "\n"

	if err := os.WriteFile(history.path, []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	if err := timeline.EnsureIndexed(); err != nil {
		t.Fatalf("EnsureIndexed failed: %v", err)
	}

	count, err := store.CountTurns()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("CountTurns = %d, want 2", count)
	}

	garrick, err := store.ListTurnsForEntity("garrick")
	if err != nil {
		t.Fatal(err)
	}
	if len(garrick) != 2 || garrick[0] != 1 || garrick[1] != 2 {
		t.Errorf("expected garrick in turns 1,2, got %v", garrick)
	}

	// Second run is a no-op.
	if err := timeline.EnsureIndexed(); err != nil {
		t.Fatalf("second EnsureIndexed failed: %v", err)
	}
	if count, _ := store.CountTurns(); count != 2 {
		t.Errorf("CountTurns = %d after a no-op reindex, want 2", count)
	}

	// A rewind that only touched the log is repaired on the next open.
	truncated := strings.SplitAfter(record, "\n")[0]
	if err := os.WriteFile(history.path, []byte(truncated), 0644); err != nil {
		t.Fatal(err)
	}
	if err := timeline.EnsureIndexed(); err != nil {
		t.Fatalf("third EnsureIndexed failed: %v", err)
	}
	if count, _ := store.CountTurns(); count != 1 {
		t.Errorf("CountTurns = %d after truncating the log, want 1", count)
	}
	if indexed, _ := store.MaxTurnNumber(); indexed != 1 {
		t.Errorf("MaxTurnNumber = %d after truncating the log, want 1", indexed)
	}
}

func TestTimelineMapsEntityMentions(t *testing.T) {
	store := newTestStore(t)
	history := NewHistoryLogger(filepath.Join(t.TempDir(), "history.jsonl"))
	timeline := NewTimeline(core.NewPathResolver(t.TempDir()), store, history, "campaign-01")

	turn := Turn{
		Number:    1,
		Mode:      "Do",
		Input:     "look",
		Narration: "You look around.",
		Entities: []entity.Mention{
			{ID: "hero", Kind: entity.MentionPlayer},
			{ID: "garrick", Kind: entity.MentionExtracted},
		},
	}
	if err := timeline.indexTurn(turn); err != nil {
		t.Fatalf("indexTurn failed: %v", err)
	}

	rec, err := store.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn failed: %v", err)
	}
	if len(rec.Entities) != 2 {
		t.Fatalf("expected 2 links, got %+v", rec.Entities)
	}
	if rec.Narration != turn.Narration {
		t.Errorf("Narration = %q, want %q", rec.Narration, turn.Narration)
	}
}

func TestTimelineIndexesMinimalRecords(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	store := newTestStore(t)

	historyPath := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(historyPath, []byte(`{"number":1,"mode":"Do"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	timeline := NewTimeline(paths, store, NewHistoryLogger(historyPath), "campaign-01")
	indexed, err := timeline.SyncTurns()
	if err != nil {
		t.Fatalf("SyncTurns failed: %v", err)
	}
	if indexed != 1 {
		t.Errorf("indexed = %d, want 1", indexed)
	}

	if _, err := store.GetTurn(1); err != nil {
		t.Errorf("expected turn 1 in the index: %v", err)
	}
}

func TestRecordTurnRecordsProseMentions(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "guard-kael", Name: "Guard Kael", Type: "character", Body: "A warden."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "sera-vane", Name: "Sera Vane", Type: "character", Body: "A smuggler."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	turn := Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I ask after Sera Vane",
		Narration: "Kael says nothing, and the water keeps moving.",
		Entities:  []entity.Mention{{ID: "guard-kael", Kind: entity.MentionWikilink}},
	}
	if err := timeline.RecordTurn(&turn, nil); err != nil {
		t.Fatalf("RecordTurn failed: %v", err)
	}

	refs, err := store.ListEntitiesForTurn(1)
	if err != nil {
		t.Fatal(err)
	}

	kinds := make(map[string][]string)
	for _, ref := range refs {
		kinds[ref.EntityID] = append(kinds[ref.EntityID], ref.Mention)
	}
	if len(kinds["sera-vane"]) != 1 || kinds["sera-vane"][0] != entity.MentionProse {
		t.Errorf("expected Sera Vane recorded from the prose, got %+v", kinds["sera-vane"])
	}
	// Kael was already linked, so the scan must not add a second row for him.
	if len(kinds["guard-kael"]) != 1 || kinds["guard-kael"][0] != entity.MentionWikilink {
		t.Errorf("expected Kael recorded once as a wikilink, got %+v", kinds["guard-kael"])
	}
}

func TestRecordTurnSavesTurnContextSnapshot(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	store := newTestStore(t)
	history := NewHistoryLogger(filepath.Join(t.TempDir(), "history.jsonl"))
	timeline := NewTimeline(paths, store, history, "campaign-01")

	turn := Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "look around",
		Narration: "The harbour is quiet.",
		Prompt:    "System rules, lore, action: look around",
		Context: &harness.TurnContext{
			TurnNumber: 1,
			PromptHash: "abc123",
			Strategy:   harness.StrategyFullPrompt,
		},
	}
	if err := timeline.RecordTurn(&turn, nil); err != nil {
		t.Fatalf("RecordTurn failed: %v", err)
	}

	rawCtx, prompt, err := store.GetTurnContext(1)
	if err != nil {
		t.Fatalf("GetTurnContext: %v", err)
	}
	if prompt != "System rules, lore, action: look around" {
		t.Errorf("expected prompt %q, got %q", "System rules, lore, action: look around", prompt)
	}
	var decoded harness.TurnContext
	if err := json.Unmarshal(rawCtx, &decoded); err != nil {
		t.Fatalf("json unmarshal context: %v", err)
	}
	if decoded.PromptHash != "abc123" || decoded.TurnNumber != 1 {
		t.Errorf("decoded context mismatch: %+v", decoded)
	}
}

func TestEnsureIndexedRederivesWorkingSetWhenEmpty(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	store := newTestStore(t)
	historyPath := filepath.Join(t.TempDir(), "history.jsonl")
	history := NewHistoryLogger(historyPath)
	timeline := NewTimeline(paths, store, history, "campaign-01")

	turn := Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "look around",
		Narration: "Kaelen nods at Elena.",
		Entities: []entity.Mention{
			{ID: "kaelen", Kind: "present"},
			{ID: "elena", Kind: "player"},
		},
	}
	if err := history.AppendTurn(turn); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	if err := timeline.EnsureIndexed(); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}

	ws, err := store.LoadWorkingSet()
	if err != nil {
		t.Fatalf("LoadWorkingSet: %v", err)
	}
	if len(ws) != 2 {
		t.Fatalf("expected 2 working set entries, got %d", len(ws))
	}
}

func TestStageEntitiesMergesPreviousIdentity(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	store := newTestStore(t)
	history := NewHistoryLogger(filepath.Join(t.TempDir(), "history.jsonl"))
	timeline := NewTimeline(paths, store, history, "campaign-01")

	// Seed existing generic-scout entity note in store.
	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID:      "generic-scout",
		Name:    "Generic Scout",
		Type:    "character",
		Body:    "A cloaked scout watching from the woods.",
		Tags:    []string{"scout", "ranger"},
		History: []int{1},
		Voice:   &entity.VoiceConfig{VoiceID: "am_adam"},
	})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	turn := &Turn{
		Number:    2,
		Narration: "The scout steps forward and introduces himself as Valen.",
		Entities:  []entity.Mention{{ID: "generic-scout", Kind: entity.MentionExtracted}},
		Segments: []entity.TurnSegment{
			{Kind: entity.SegmentSpeech, Speaker: "Generic Scout", SpeakerID: "generic-scout", Text: "I am Valen."},
		},
	}

	personae := []harness.PersonaDecl{
		{
			Name:        "Valen",
			Type:        "character",
			Reveals:     "Generic Scout",
			Description: "A veteran scout of the Silver Guard.",
		},
	}

	pending, err := timeline.stageEntities(turn, nil, personae)
	if err != nil {
		t.Fatalf("stageEntities: %v", err)
	}

	valen, ok := pending["valen"]
	if !ok {
		t.Fatalf("expected valen in pending entities, got %+v", pending)
	}
	if valen.Name != "Valen" {
		t.Errorf("valen name = %q, want Valen", valen.Name)
	}
	if len(valen.Aliases) == 0 || valen.Aliases[0] != "Generic Scout" {
		t.Errorf("expected Generic Scout in aliases, got %v", valen.Aliases)
	}
	if valen.Voice == nil || valen.Voice.VoiceID != "am_adam" {
		t.Errorf("expected inherited voice am_adam, got %+v", valen.Voice)
	}
	// generic-scout should no longer be in pending
	if _, exists := pending["generic-scout"]; exists {
		t.Errorf("generic-scout should have been merged and removed from pending")
	}
	// turn.Entities should point to valen instead of generic-scout
	for _, m := range turn.Entities {
		if m.ID == "generic-scout" {
			t.Errorf("turn.Entities still mentions generic-scout")
		}
	}
	// turn.Segments should have SpeakerID mapped to valen
	if turn.Segments[0].SpeakerID != "valen" {
		t.Errorf("segment SpeakerID = %q, want valen", turn.Segments[0].SpeakerID)
	}
}

