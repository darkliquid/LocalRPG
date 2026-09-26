package desktop

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestSubmitTurnAppendsChunksAndTurn(t *testing.T) {
	appState = &State{Loaded: true, OpenGame: "campaign-01", Screen: ScreenChronicle}

	runTurn = func(ctx context.Context, svc *gui.Service, gameID string, req gui.TurnRequest, emit func(gui.TurnEvent) error) error {
		if req.Mode != "do" || req.Input != "look" {
			t.Fatalf("req = %+v", req)
		}
		_ = emit(gui.TurnEvent{Type: "chunk", Text: "The "})
		_ = emit(gui.TurnEvent{Type: "chunk", Text: "door opens."})
		_ = emit(gui.TurnEvent{Type: "turn", Turn: &gui.TurnDTO{TurnNumber: 2, InputText: "look"}})
		return nil
	}
	t.Cleanup(func() { runTurn = nil })

	submitTurn(context.Background(), nil, "campaign-01", "do", "look")

	if appState.TurnInFlight {
		t.Error("turn should not still be in flight")
	}
	if len(appState.Turns) != 1 || appState.Turns[0].TurnNumber != 2 {
		t.Errorf("turns = %+v", appState.Turns)
	}
	if appState.Prose != "" {
		t.Errorf("streamed prose should be cleared after the turn event, got %q", appState.Prose)
	}
}

func TestSubmitTurnReportsInFlight(t *testing.T) {
	appState = &State{Loaded: true, OpenGame: "campaign-01"}
	runTurn = func(context.Context, *gui.Service, string, gui.TurnRequest, func(gui.TurnEvent) error) error {
		return gui.ErrTurnInFlight
	}
	t.Cleanup(func() { runTurn = nil })

	submitTurn(context.Background(), nil, "campaign-01", "do", "look")
	if appState.TurnError == "" {
		t.Fatal("expected an in-flight error message")
	}
}
