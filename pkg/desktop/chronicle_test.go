package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestChronicleSnapshot(t *testing.T) {
	appState = &State{
		Loaded:   true,
		Screen:   ScreenChronicle,
		OpenGame: "campaign-01",
		Turns: []gui.TurnDTO{
			{
				TurnNumber: 1,
				InputText:  "I open the door.",
				Mode:       "do",
				LocationID: "hall",
				Segments: []gui.SegmentDTO{
					{Kind: "narration", Text: "The *door* groans open."},
					{Kind: "speech", Speaker: "Vance", Text: "Hello?", Player: true},
				},
				Checks: []harness.CheckResult{
					{CheckID: "c1", CheckKind: "skill", Stakes: "notice the trap", Outcome: "failure",
						Roll: &harness.RollSummary{Notation: "1d20", Total: 4, RollCount: 1}},
				},
			},
		},
	}
	ui.Snapshot(t, "chronicle", 800, 600, RootView)
}
