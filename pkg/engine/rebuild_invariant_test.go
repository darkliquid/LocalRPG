package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// derivedSnapshot is everything the index derives from the canonical sources.
// If two snapshots differ, the index cannot be rebuilt from Markdown and
// history.jsonl alone.
type derivedSnapshot struct {
	TurnCount   int
	MaxTurn     int
	Entities    []storage.EntitySummary
	EntityTurns map[string][]int
	Memories    map[string][]string
	WorkingSet  []storage.WorkingSetRecord
}

// TestIndexRebuildsFromCanonicalSources deletes the index of a campaign that has
// entities, turns, mentions, checks, and memories, and proves that replaying the
// Markdown and history.jsonl reconstructs exactly the same derived state.
func TestIndexRebuildsFromCanonicalSources(t *testing.T) {
	root := t.TempDir()
	paths := core.NewPathResolver(root)
	gameDir := paths.GameDir("campaign-01")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	dbPath := paths.GameDBPath("campaign-01")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	historyPath := filepath.Join(gameDir, "history.jsonl")

	store := openStoreAt(t, dbPath)
	timeline := NewTimeline(paths, store, NewHistoryLogger(historyPath), "campaign-01")
	entitiesDir := timeline.EntitiesDir()

	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID: "alden-tavern", Name: "Alden Tavern", Type: "location",
		Body: "A warm room. See [[garrick]].",
	})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID: "garrick", Name: "Garrick", Type: "character",
		Location: "[[alden-tavern]]", Faction: "The Watch",
		Body: "A guarded innkeeper.",
	})

	if err := os.WriteFile(historyPath, []byte(campaignHistory()), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}
	if err := timeline.EnsureIndexed(); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	before := snapshotDerived(t, store)

	// Delete the index entirely, including any write-ahead sidecars.
	store.Close()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(dbPath + suffix)
	}

	reopened := openStoreAt(t, dbPath)
	defer reopened.Close()
	replayed := NewTimeline(paths, reopened, NewHistoryLogger(historyPath), "campaign-01")
	if _, err := storage.NewSyncer(reopened).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}
	if err := replayed.EnsureIndexed(); err != nil {
		t.Fatalf("EnsureIndexed after deletion: %v", err)
	}
	after := snapshotDerived(t, reopened)

	if !reflect.DeepEqual(before, after) {
		t.Fatalf("derived state differs after rebuilding\nbefore: %#v\nafter:  %#v", before, after)
	}

	// A second replay is a no-op, not a duplication.
	if err := replayed.EnsureIndexed(); err != nil {
		t.Fatalf("second EnsureIndexed: %v", err)
	}
	if again := snapshotDerived(t, reopened); !reflect.DeepEqual(after, again) {
		t.Fatalf("EnsureIndexed is not idempotent\nfirst:  %#v\nsecond: %#v", after, again)
	}
}

func openStoreAt(t *testing.T, path string) *storage.Store {
	t.Helper()
	store, err := storage.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore(%q): %v", path, err)
	}
	return store
}

// campaignHistory is three turns carrying segments, mentions, a check, and a
// memory, so every derived table has something to rebuild.
func campaignHistory() string {
	return `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"Warm light.","location":"alden-tavern","segments":[{"kind":"narration","text":"Warm light."}],"entities":[{"id":"alden-tavern","mention":"location"},{"id":"garrick","mention":"wikilink"}],"checks":[{"check_id":"c1","actor":"player","outcome":"strong"}]}` + "\n" +
		`{"number":2,"timestamp":"2026-09-21T10:05:00Z","mode":"Say","input":"hello","narration":"Garrick nods.","location":"alden-tavern","segments":[{"kind":"speech","speaker":"Garrick","speaker_id":"garrick","text":"Evening."}],"entities":[{"id":"garrick","mention":"speech"}],"memories":[{"turn":2,"kind":"fact","text":"Garrick greeted the player.","importance":3,"source":"extracted","entity_refs":["garrick"]}]}` + "\n" +
		`{"number":3,"timestamp":"2026-09-21T10:10:00Z","mode":"Story","input":"I leave","narration":"You step out.","location":"","segments":[{"kind":"narration","text":"You step out."}],"entities":[{"id":"alden-tavern","mention":"location"}]}` + "\n"
}

func snapshotDerived(t *testing.T, store *storage.Store) derivedSnapshot {
	t.Helper()

	snapshot := derivedSnapshot{
		EntityTurns: map[string][]int{},
		Memories:    map[string][]string{},
	}

	count, err := store.CountTurns()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.TurnCount = count

	maximum, err := store.MaxTurnNumber()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.MaxTurn = maximum

	entities, err := store.ListEntities()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Entities = entities

	for _, summary := range entities {
		turns, err := store.ListTurnsForEntity(summary.ID)
		if err != nil {
			t.Fatal(err)
		}
		sort.Ints(turns)
		snapshot.EntityTurns[summary.ID] = turns

		memories, err := store.ListMemoriesForEntity(summary.ID, 1000)
		if err != nil {
			t.Fatal(err)
		}
		texts := make([]string, 0, len(memories))
		for _, memory := range memories {
			texts = append(texts, memory.Kind+"|"+memory.Text)
		}
		sort.Strings(texts)
		snapshot.Memories[summary.ID] = texts
	}

	working, err := store.LoadWorkingSet()
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(working, func(i, j int) bool { return working[i].EntityID < working[j].EntityID })
	snapshot.WorkingSet = working

	return snapshot
}
