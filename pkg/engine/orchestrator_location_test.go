package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// locationOrchestrator builds a campaign with two known places, a player note
// pointing at one of them, and a pinned start location that differs.
func locationOrchestrator(t *testing.T, playerLocation string) (*TurnOrchestrator, *Timeline, *storage.Store) {
	t.Helper()

	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt air."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Warm."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID: "player", Name: "Sean", Type: "character", Body: "A traveller.",
		Location: playerLocation,
	})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(&mockOrchestratorModel{response: "The tavern hums."})
	router.AssignRole("gm", "mock-gm")

	// The pinned start location is deliberately not the player's location, so the
	// assertions below can tell which one was used.
	o := NewTurnOrchestrator(store, timeline, nil, router, "aldon-harbour", "player")
	return o, timeline, store
}

func TestProcessActionRecordsThePlayersLocation(t *testing.T) {
	o, _, store := locationOrchestrator(t, "[[alden-tavern]]")

	turn, err := o.ProcessAction(context.Background(), "Do", "I look around")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	if turn.Location != "alden-tavern" {
		t.Errorf("Location = %q, want the player's own note location", turn.Location)
	}

	indexed, err := store.GetTurn(turn.Number)
	if err != nil {
		t.Fatal(err)
	}
	if indexed.Location != "alden-tavern" {
		t.Errorf("indexed Location = %q, want alden-tavern", indexed.Location)
	}
}

func TestLocationBootstrapsFromThePinnedStart(t *testing.T) {
	// A player whose location points at a note that does not exist is exactly the
	// stale-reference case, and with no history the pinned start is the answer.
	o, _, _ := locationOrchestrator(t, "[[somewhere-that-was-deleted]]")

	first, err := o.ProcessAction(context.Background(), "Do", "I wait")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	if first.Location != "aldon-harbour" {
		t.Errorf("Location = %q, want the pinned start location", first.Location)
	}
}

func TestLocationContinuityBeatsThePinnedStart(t *testing.T) {
	o, timeline, _ := locationOrchestrator(t, "[[alden-tavern]]")

	if _, err := o.ProcessAction(context.Background(), "Do", "I settle in"); err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	// Break the note mid-campaign: the party stays where the last turn happened
	// rather than teleporting back to where the campaign opened.
	if err := timeline.SetPlayerLocation("player", "somewhere-that-was-deleted"); err != nil {
		t.Fatalf("SetPlayerLocation failed: %v", err)
	}

	second, err := o.ProcessAction(context.Background(), "Do", "I wait again")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	if second.Location != "alden-tavern" {
		t.Errorf("Location = %q, want the previous turn's location, not the pinned start", second.Location)
	}
}

func TestGoMovesThePlayerAndRecordsASystemTurn(t *testing.T) {
	o, timeline, store := locationOrchestrator(t, "[[alden-tavern]]")

	move, err := o.ProcessAction(context.Background(), "System", "/go Aldon Harbour")
	if err != nil {
		t.Fatalf("/go failed: %v", err)
	}
	if move.Mode != "System" || move.Location != "aldon-harbour" {
		t.Errorf("unexpected move turn: %+v", move)
	}

	player, err := store.GetEntity("player")
	if err != nil {
		t.Fatal(err)
	}
	if player.Location != "[[aldon-harbour]]" {
		t.Errorf("player location = %q, want [[aldon-harbour]]", player.Location)
	}

	// The move is part of the timeline, not a side effect.
	recorded, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 || recorded[0].Location != "aldon-harbour" {
		t.Errorf("expected the move recorded with its destination, got %+v", recorded)
	}

	// The player's own note carries the turn, so involvement is recorded too.
	if len(player.History) != 1 || player.History[0] != move.Number {
		t.Errorf("expected the player's history to include the move, got %v", player.History)
	}

	if _, err := o.ProcessAction(context.Background(), "System", "/go Nowhere At All"); err == nil {
		t.Errorf("expected an error for an unknown location")
	}
	if _, err := o.ProcessAction(context.Background(), "System", "/go player"); err == nil {
		t.Errorf("expected an error when the target is not a location")
	}
}

func TestExtractorProposedLocationAppliesOnlyWhenItResolves(t *testing.T) {
	o, _, store := locationOrchestrator(t, "[[alden-tavern]]")

	o.SetExtractor(harness.NewExtractor(&mockTimelineModel{
		response: `{"entities":[],"dialogue":[],"player_location":"[[Aldon Harbour]]"}`,
	}))

	turn, err := o.ProcessAction(context.Background(), "Do", "I walk to the harbour")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	// The turn happened where it started; the move applies from the next turn.
	if turn.Location != "alden-tavern" {
		t.Errorf("Location = %q, want where the turn started", turn.Location)
	}

	player, err := store.GetEntity("player")
	if err != nil {
		t.Fatal(err)
	}
	if player.Location != "[[aldon-harbour]]" {
		t.Errorf("player location = %q, want the applied move", player.Location)
	}

	// A proposal that does not resolve is ignored rather than teleporting anyone.
	o.SetExtractor(harness.NewExtractor(&mockTimelineModel{
		response: `{"entities":[],"dialogue":[],"player_location":"[[Place That Does Not Exist]]"}`,
	}))
	if _, err := o.ProcessAction(context.Background(), "Do", "I walk somewhere odd"); err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	player, err = store.GetEntity("player")
	if err != nil {
		t.Fatal(err)
	}
	if player.Location != "[[aldon-harbour]]" {
		t.Errorf("an unresolvable proposal must be ignored, got %q", player.Location)
	}
}

func TestCurrentLocationName(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Warm."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller.", Location: "[[alden-tavern]]"})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	o := NewTurnOrchestrator(store, timeline, nil, harness.NewRouter(), "alden-tavern", "player")
	if name := o.CurrentLocationName(); name != "Alden Tavern" {
		t.Errorf("CurrentLocationName = %q, want Alden Tavern", name)
	}
}
