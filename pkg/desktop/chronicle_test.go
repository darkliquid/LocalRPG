package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestChronicleSnapshot(t *testing.T) {
	appState = &State{
		Loaded:    true,
		Screen:    ScreenChronicle,
		OpenGame:  "campaign-01",
		Portraits: map[string]string{},
		SceneArt:  map[string]string{},
		Games: []gui.GameSummaryDTO{
			{ID: "campaign-01", Name: "The Hollow Crown", WorldID: "realm", SystemID: "dnd5e", PlayerName: "Vance"},
		},
		Turns: []gui.TurnDTO{
			{
				TurnNumber: 1,
				InputText:  "I open the door.",
				Mode:       "do",
				LocationID: "hall",
				LocationName: "The Hall",
				Segments: []gui.SegmentDTO{
					{Kind: "narration", Text: "The *door* groans open.", CheckRef: "c1"},
					{Kind: "speech", Speaker: "Vance", SpeakerID: "vance", Text: "Hello?", Player: true},
					{Kind: "speech", Speaker: "The Keeper", SpeakerID: "keeper", Text: "You are late."},
				},
				Checks: []harness.CheckResult{
					{CheckID: "c1", CheckKind: "skill", Stakes: "notice the trap", Outcome: "failure",
						Roll: &harness.RollSummary{Notation: "1d20", Total: 4, RollCount: 1}},
				},
				ToolCalls:   []gui.ToolCallDTO{{Name: "search_lore", ResultChars: 420}},
				EntitiesHit: []string{"keeper", "hall"},
			},
			{
				TurnNumber: 2,
				InputText:  "/say We should go.",
				Mode:       "say",
				LocationID: "hall",
				Segments: []gui.SegmentDTO{
					{Kind: "narration", Text: "The Keeper nods slowly and steps aside."},
				},
			},
		},
	}
	ui.Snapshot(t, "chronicle", 900, 700, RootView)
}
