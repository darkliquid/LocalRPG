package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func writeTestEntityNote(t *testing.T, dir string, ent *entity.Entity) {
	t.Helper()

	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ent.ID+".md"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestTimelineRecordTurnWritesNotesThenIndexes(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	store := newTestStore(t)
	history := NewHistoryLogger(filepath.Join(t.TempDir(), "history.jsonl"))
	timeline := NewTimeline(paths, store, history, "campaign-01")
	timeline.SetVoiceProfiles([]config.VoiceProfile{
		{ID: "elder_sage", VoiceID: "bm_george", Pitch: 0.85, SpeechRate: 0.9, Tags: []string{"elder", "veteran"}},
	})

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "hero", Name: "Sean", Type: "character", Body: "A traveller."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "aldon-tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}

	narration := "An elder veteran introduces himself."
	turn := Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I ask around",
		Narration: narration,
		Entities:  harness.ResolveEntityMentions(store, "hero", "aldon-tavern", narration),
	}
	extracted := []harness.ExtractedEntity{
		{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Location: "[[aldon-tavern]]", Body: "An elder veteran broker."},
	}

	if err := timeline.RecordTurn(&turn, extracted); err != nil {
		t.Fatalf("RecordTurn failed: %v", err)
	}

	garrickPath := filepath.Join(entitiesDir, "garrick-the-fence.md")
	data, err := os.ReadFile(garrickPath)
	if err != nil {
		t.Fatalf("expected a note for the extracted entity: %v", err)
	}
	garrick, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("parse extracted note: %v", err)
	}
	if !reflect.DeepEqual(garrick.History, []int{1}) {
		t.Errorf("History = %v, want [1]", garrick.History)
	}
	if garrick.Voice == nil || garrick.Voice.VoiceID != "bm_george" {
		t.Errorf("expected the elder voice profile to be assigned, got %+v", garrick.Voice)
	}

	refs, err := store.ListEntitiesForTurn(1)
	if err != nil {
		t.Fatalf("ListEntitiesForTurn failed: %v", err)
	}
	if len(refs) != 3 {
		t.Errorf("expected player, location, and extracted entity links, got %+v", refs)
	}

	turns, err := history.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(turns) != 1 || turns[0].Prose() != narration {
		t.Fatalf("expected the turn in history.jsonl, got %+v", turns)
	}
	if len(turns[0].Entities) != 3 {
		t.Errorf("expected involvement in the record, got %+v", turns[0].Entities)
	}

	// A later turn about the same entity accumulates history instead of duplicating.
	second := Turn{
		Number:    2,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I press him",
		Narration: "Garrick the Fence says nothing.",
		Entities:  harness.ResolveEntityMentions(store, "hero", "aldon-tavern", "Garrick the Fence says nothing."),
	}
	if err := timeline.RecordTurn(&second, []harness.ExtractedEntity{
		{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Body: "A broker who keeps his mouth shut."},
	}); err != nil {
		t.Fatalf("second RecordTurn failed: %v", err)
	}

	data, err = os.ReadFile(garrickPath)
	if err != nil {
		t.Fatal(err)
	}
	garrick, err = entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(garrick.History, []int{1, 2}) {
		t.Errorf("History = %v, want [1 2]", garrick.History)
	}
	if !strings.Contains(garrick.Body, "elder veteran broker") || !strings.Contains(garrick.Body, "keeps his mouth shut") {
		t.Errorf("expected accumulated body detail, got %q", garrick.Body)
	}
	if garrick.Location != "[[aldon-tavern]]" {
		t.Errorf("expected the authored location to survive merging, got %q", garrick.Location)
	}

	indexed, err := store.ListTurnsForEntity("garrick-the-fence")
	if err != nil {
		t.Fatalf("ListTurnsForEntity failed: %v", err)
	}
	if !reflect.DeepEqual(indexed, []int{1, 2}) {
		t.Errorf("indexed turns = %v, want [1 2]", indexed)
	}
}
