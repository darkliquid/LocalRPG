package engine

import (
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/state"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestArcStatusFallsBackToOpen(t *testing.T) {
	if got := ArcStatus(nil); got != "open" {
		t.Errorf("ArcStatus(nil) = %q, want open", got)
	}

	unset := &entity.Entity{ID: "a", Name: "A", Type: "arc"}
	if got := ArcStatus(unset); got != "open" {
		t.Errorf("ArcStatus(unset) = %q, want open", got)
	}

	// A typo degrades rather than breaking the section that counts unresolved arcs.
	typo := &entity.Entity{Type: "arc", State: state.NewState(map[string]interface{}{"status": "compleated"})}
	if got := ArcStatus(typo); got != "open" {
		t.Errorf("ArcStatus(typo) = %q, want open", got)
	}

	resolved := &entity.Entity{Type: "arc", State: state.NewState(map[string]interface{}{"status": "resolved"})}
	if got := ArcStatus(resolved); got != "resolved" {
		t.Errorf("ArcStatus(resolved) = %q", got)
	}
}

func TestOpenThreadsReportsIdleUnresolvedArcs(t *testing.T) {
	store := newTestStore(t)
	if err := store.SaveEntity(&entity.Entity{ID: "the-miasma", Name: "The Creeping Miasma", Type: "arc", Body: "A creeping fog."}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEntity(&entity.Entity{
		ID: "the-siege", Name: "The Iron Siege", Type: "arc", Body: "The siege grinds on.",
		State: state.NewState(map[string]interface{}{"status": "resolved"}),
	}); err != nil {
		t.Fatal(err)
	}

	// The miasma was last touched at turn 2, and the campaign is at turn 9.
	if err := store.SaveTurn(storage.TurnRecord{
		Number: 2, Timestamp: time.Now(), Mode: "Do", Narration: "The fog rose.",
		Entities: []storage.TurnEntityRef{{EntityID: "the-miasma", Mention: "wikilink"}},
	}); err != nil {
		t.Fatal(err)
	}

	threads, err := OpenThreads(store, 9)
	if err != nil {
		t.Fatalf("OpenThreads failed: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected one open thread, got %+v", threads)
	}
	if threads[0].ID != "the-miasma" {
		t.Errorf("thread = %q, want the-miasma", threads[0].ID)
	}
	if threads[0].LastAdvanced != 2 || threads[0].Idle != 7 {
		t.Errorf("LastAdvanced = %d, Idle = %d; want 2 and 7", threads[0].LastAdvanced, threads[0].Idle)
	}
}

func TestOpenThreadsPutsTheStalestFirst(t *testing.T) {
	store := newTestStore(t)
	for _, id := range []string{"fresh", "stale"} {
		if err := store.SaveEntity(&entity.Entity{ID: id, Name: id, Type: "arc", Body: "An arc."}); err != nil {
			t.Fatal(err)
		}
	}
	for number, id := range map[int]string{1: "stale", 8: "fresh"} {
		if err := store.SaveTurn(storage.TurnRecord{
			Number: number, Timestamp: time.Now(), Mode: "Do", Narration: "Something.",
			Entities: []storage.TurnEntityRef{{EntityID: id, Mention: "wikilink"}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	threads, err := OpenThreads(store, 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 2 {
		t.Fatalf("expected two threads, got %+v", threads)
	}
	// A thread nobody has touched for eight turns is the one worth surfacing.
	if threads[0].ID != "stale" {
		t.Errorf("expected the staler thread first, got %+v", threads)
	}
}
