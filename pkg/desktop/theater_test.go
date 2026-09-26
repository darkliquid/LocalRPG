package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestTheaterSnapshot(t *testing.T) {
	appState = &State{
		Loaded: true, Screen: ScreenTheater,
		TheaterTurn: 0, TheaterBeat: 1, TheaterPlaying: true, TheaterSpeed: 1,
		Turns: []gui.TurnDTO{{
			TurnNumber: 1, LocationName: "The Hall",
			Segments: []gui.SegmentDTO{
				{Kind: "narration", Text: "The *door* opens."},
				{Kind: "speech", Speaker: "Vance", Text: "Hello?"},
			},
		}},
	}
	ui.Snapshot(t, "theater", 900, 600, RootView)
}

func TestAdvanceBeatWalksSegmentsThenTurns(t *testing.T) {
	appState = &State{
		Loaded: true,
		Screen: ScreenTheater,
		Turns: []gui.TurnDTO{
			{TurnNumber: 1, Segments: []gui.SegmentDTO{{Kind: "narration", Text: "a"}, {Kind: "speech", Speaker: "V", Text: "b"}}},
			{TurnNumber: 2, Segments: []gui.SegmentDTO{{Kind: "narration", Text: "c"}}},
		},
		TheaterTurn: 0, TheaterBeat: 0, TheaterPlaying: true,
	}
	advanceBeat()
	if appState.TheaterBeat != 1 {
		t.Fatalf("beat = %d, want 1", appState.TheaterBeat)
	}
	advanceBeat()
	if appState.TheaterTurn != 1 || appState.TheaterBeat != 0 {
		t.Fatalf("turn/beat = %d/%d, want 1/0", appState.TheaterTurn, appState.TheaterBeat)
	}
	advanceBeat()
	if appState.TheaterPlaying {
		t.Fatal("playback should stop at the end of the script")
	}
}

func TestChangeTurnClamps(t *testing.T) {
	appState = &State{Loaded: true, Turns: []gui.TurnDTO{{TurnNumber: 1}, {TurnNumber: 2}}, TheaterTurn: 0}
	changeTurn(-1)
	if appState.TheaterTurn != 0 {
		t.Fatalf("turn = %d, want clamp at 0", appState.TheaterTurn)
	}
	changeTurn(1)
	if appState.TheaterTurn != 1 {
		t.Fatalf("turn = %d, want 1", appState.TheaterTurn)
	}
}
