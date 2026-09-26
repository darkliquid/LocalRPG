package desktop

import (
	"context"
	"errors"

	. "go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/gui"
)

// runTurn is the turn seam. It mirrors BeginTurn + TurnSession.Run so tests can
// drive the stream without a model or a service.
var runTurn = func(ctx context.Context, svc *gui.Service, gameID string, req gui.TurnRequest, emit func(gui.TurnEvent) error) error {
	session, err := svc.BeginTurn(gameID)
	if err != nil {
		return err
	}
	defer session.Close()
	return session.Run(ctx, req, emit)
}

// submitTurn runs one turn, publishing streamed progress under the frame lock.
func submitTurn(ctx context.Context, svc *gui.Service, gameID, mode, input string) {
	WithFrameLock(func() {
		appState.TurnInFlight = true
		appState.PendingAction = input
		appState.Prose = ""
		appState.ToolActivity = ""
		appState.TurnError = ""
	})
	RequestNextFrame()

	emit := func(ev gui.TurnEvent) error {
		WithFrameLock(func() {
			switch ev.Type {
			case "chunk":
				appState.Prose += ev.Text
			case "tool":
				appState.ToolActivity = ev.ToolSummary
			case "turn":
				if ev.Turn != nil {
					appState.Turns = append(appState.Turns, *ev.Turn)
				}
				appState.Prose = ""
			case "model_missing":
				appState.TurnError = "model missing: " + ev.Name
			case "error":
				appState.TurnError = ev.Message
			}
		})
		RequestNextFrame()
		return nil
	}

	err := runTurn(ctx, svc, gameID, gui.TurnRequest{Mode: mode, Input: input}, emit)
	WithFrameLock(func() {
		appState.TurnInFlight = false
		appState.PendingAction = ""
		switch {
		case err == nil:
		case errors.Is(err, gui.ErrTurnInFlight):
			appState.TurnError = "A turn is already in flight."
		case errors.Is(err, gui.ErrCampaignNotPlayable):
			appState.TurnError = "The campaign could not be prepared."
		default:
			appState.TurnError = err.Error()
		}
	})
	RequestNextFrame()
}
