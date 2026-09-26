package desktop

import (
	"math"
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestWorldDrawerSnapshot(t *testing.T) {
	appState = &State{
		Loaded: true,
		Screen: ScreenChronicle,
		Drawer: "world",
		Turns:  []gui.TurnDTO{{TurnNumber: 1, Mode: "do", Segments: []gui.SegmentDTO{{Kind: "narration", Text: "A hall."}}}},
		GameState: &gui.GameStateDTO{
			Player: gui.PlayerDTO{Name: "Vance", Type: "character", State: map[string]any{"hp": 7, "max_hp": 20, "level": 3}},
			Arcs:   []gui.NarrativeArcDTO{{ID: "a", Name: "The Crown", Progress: 3, MaxProgress: 6}},
			Clocks: []gui.FactionClockDTO{{Name: "Guild War", Faction: "Thieves", Ticks: 2, MaxTicks: 6}},
		},
		Recap: &gui.RecapDTO{Enabled: true, ThroughTurn: 12, Summary: "The party broke the seal.", Threads: []gui.ThreadDTO{{Name: "The missing heir", Status: "open", LastAdvanced: 9}}},
	}
	ui.Snapshot(t, "drawer_world", 1000, 700, RootView)
}

func TestProgressFraction(t *testing.T) {
	if got := progressFraction(2, 4); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("progressFraction(2,4) = %v, want 0.5", got)
	}
	if got := progressFraction(5, 0); got != 0 {
		t.Fatalf("zero max must yield 0, got %v", got)
	}
	appState = &State{Loaded: true, GameState: &gui.GameStateDTO{Arcs: []gui.NarrativeArcDTO{{ID: "a", Name: "A", Progress: 3, MaxProgress: 6}}}}
	if got := progressFraction(3, 6); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("fraction = %v", got)
	}
}

func TestNumericField(t *testing.T) {
	state := map[string]any{"hp": 7, "level": 3.0, "name": "Vance"}
	if v, ok := numericField(state, "hp"); !ok || v != 7 {
		t.Fatalf("hp = %d,%v", v, ok)
	}
	if v, ok := numericField(state, "level"); !ok || v != 3 {
		t.Fatalf("level = %d,%v", v, ok)
	}
	if _, ok := numericField(state, "name"); ok {
		t.Fatal("string field must not be numeric")
	}
}
