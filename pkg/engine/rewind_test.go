package engine

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestTimelineRewindPrunesLinksButKeepsNotes(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	store := newTestStore(t)
	history := NewHistoryLogger(filepath.Join(t.TempDir(), "history.jsonl"))
	timeline := NewTimeline(paths, store, history, "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "hero", Name: "Sean", Type: "character", Body: "A traveller."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	first := Turn{Number: 1, Timestamp: time.Now(), Mode: "Do", Input: "I ask", Narration: "Garrick the Fence frowns."}
	first.Entities = harness.ResolveEntityMentions(store, "hero", "", first.Narration)
	if err := timeline.RecordTurn(&first, []harness.ExtractedEntity{
		{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Body: "A broker."},
	}); err != nil {
		t.Fatal(err)
	}

	second := Turn{Number: 2, Timestamp: time.Now(), Mode: "Do", Input: "I push", Narration: "Garrick the Fence relents."}
	second.Entities = harness.ResolveEntityMentions(store, "hero", "", second.Narration)
	if err := timeline.RecordTurn(&second, []harness.ExtractedEntity{
		{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Body: "He admits the debt is real."},
	}); err != nil {
		t.Fatal(err)
	}

	if err := timeline.RewindToTurn(1); err != nil {
		t.Fatalf("RewindToTurn failed: %v", err)
	}

	turns, err := history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Number != 1 {
		t.Errorf("expected only turn 1 in the log, got %+v", turns)
	}

	count, err := store.CountTurns()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("CountTurns = %d, want 1", count)
	}

	indexed, err := store.ListTurnsForEntity("garrick-the-fence")
	if err != nil {
		t.Fatal(err)
	}
	if len(indexed) != 1 || indexed[0] != 1 {
		t.Errorf("indexed turns = %v, want [1]", indexed)
	}

	data, err := os.ReadFile(filepath.Join(entitiesDir, "garrick-the-fence.md"))
	if err != nil {
		t.Fatal(err)
	}
	garrick, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(garrick.History, []int{1}) {
		t.Errorf("History = %v, want [1]", garrick.History)
	}
	if !strings.Contains(garrick.Body, "admits the debt is real") {
		t.Errorf("expected the world's memory to survive an undo, got %q", garrick.Body)
	}
}

func TestProcessActionUndoRewindsIndex(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "test-campaign")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(&mockOrchestratorModel{response: "You step inside."})
	router.AssignRole("gm", "mock-gm")

	orchestrator := NewTurnOrchestrator(store, timeline, nil, router, "tavern", "player")

	if _, err := orchestrator.ProcessAction(context.Background(), "Do", "I open the door"); err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.ProcessAction(context.Background(), "Do", "I sit down"); err != nil {
		t.Fatal(err)
	}

	undo, err := orchestrator.ProcessAction(context.Background(), "System", "/undo")
	if err != nil {
		t.Fatalf("undo failed: %v", err)
	}
	if undo.Mode != "System" {
		t.Errorf("expected a system turn for the undo, got %+v", undo)
	}

	count, err := store.CountTurns()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("CountTurns = %d, want 1 after undo", count)
	}
}
