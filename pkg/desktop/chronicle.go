package desktop

import (
	"context"
	"fmt"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// openCampaign switches to the chronicle and loads the campaign's turns.
func openCampaign(gameID string) {
	appState.OpenGame = gameID
	appState.Screen = ScreenChronicle
	appState.Turns = []gui.TurnDTO{}
	svc := liveService
	if svc == nil {
		return
	}
	go func() {
		turns := loadChronicle(context.Background(), svc, gameID)
		WithFrameLock(func() { appState.Turns = turns })
		RequestNextFrame()
	}()
}

func chronicleView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg)), func() {
		ScrollOnInput()
		ScrollBars()
		Container(Attrs(Expand, Pad(20), Gap(12)), func() {
			if len(appState.Turns) == 0 {
				Label("The story has not begun yet.", FontSize(14), TextColorVec(p.Muted))
				return
			}
			prevLocation := ""
			for i := range appState.Turns {
				turn := &appState.Turns[i]
				locationChanged := turn.LocationID != "" && turn.LocationID != prevLocation
				prevLocation = turn.LocationID
				turnView(p, turn, locationChanged)
			}
			if appState.TurnInFlight {
				inFlightView(p)
			}
		})
	})
}

func turnView(p ui.Palette, turn *gui.TurnDTO, locationChanged bool) {
	Container(Attrs(Expand, Gap(8), Pad(12), Corners(8), BackgroundVec(p.Panel)), func() {
		if turn.InputText != "" {
			Label("["+turn.Mode+"] "+turn.InputText, FontSize(12), TextColorVec(p.Muted))
		}
		if locationChanged && turn.LocationName != "" {
			Label(turn.LocationName, FontSize(14), FontWeight(WeightBold), TextColorVec(p.Accent))
		}

		checks := make(map[string]harness.CheckResult, len(turn.Checks))
		for _, c := range turn.Checks {
			checks[c.CheckID] = c
		}
		for i := range turn.Segments {
			seg := &turn.Segments[i]
			if seg.CheckRef != "" {
				if c, ok := checks[seg.CheckRef]; ok {
					checkCard(p, c)
				}
			}
			if seg.Kind == "speech" {
				Label(seg.Speaker, FontSize(13), FontWeight(WeightBold), TextColorVec(speakerColor(p, seg.Player)))
			}
			proseBlocks(seg.Text, TextColorVec(p.Text))
		}

		attached := make(map[string]bool, len(turn.Segments))
		for _, seg := range turn.Segments {
			if seg.CheckRef != "" {
				attached[seg.CheckRef] = true
			}
		}
		for _, c := range turn.Checks {
			if !attached[c.CheckID] {
				checkCard(p, c)
			}
		}

		if len(turn.ToolCalls) > 0 {
			names := ""
			for i, tc := range turn.ToolCalls {
				if i > 0 {
					names += ", "
				}
				names += tc.Name
			}
			Label("tools: "+names, FontSize(11), TextColorVec(p.Muted))
		}
		if turn.Rejected && turn.Verdict != nil && turn.Verdict.Reason != "" {
			Label("Rejected: "+turn.Verdict.Reason, FontSize(12), TextColorVec(p.Danger))
		}
		if turn.Truncated {
			Label("(truncated)", FontSize(11), TextColorVec(p.Muted))
		}
		for _, note := range turn.ContextNotes {
			Label(note, FontSize(11), TextColorVec(p.Muted))
		}
	})
}

func checkCard(p ui.Palette, c harness.CheckResult) {
	notation := ""
	total := 0
	if c.Roll != nil {
		notation = c.Roll.Notation
		total = c.Roll.Total
	}
	Container(Attrs(Pad(8), Corners(6), BackgroundVec(p.Bg), Gap(2)), func() {
		Label(fmt.Sprintf("%s %s → %d (%s)", c.CheckKind, notation, total, c.Outcome),
			FontSize(12), TextColorVec(outcomeColor(p, c.Outcome)))
		if c.Stakes != "" {
			Label(c.Stakes, FontSize(11), TextColorVec(p.Muted))
		}
	})
}

func inFlightView(p ui.Palette) {
	Container(Attrs(Expand, Pad(12), Corners(8), BackgroundVec(p.Panel), Gap(4)), func() {
		if appState.PendingAction != "" {
			Label("> "+appState.PendingAction, FontSize(12), TextColorVec(p.Muted))
		}
		if appState.Prose != "" {
			proseBlocks(appState.Prose, TextColorVec(p.Text))
		} else {
			Label("The narrator is drafting…", FontSize(12), TextColorVec(p.Muted))
		}
	})
}

func speakerColor(p ui.Palette, player bool) Vec4 {
	if player {
		return p.Accent
	}
	return p.Text
}

func outcomeColor(p ui.Palette, outcome string) Vec4 {
	switch outcome {
	case "success", "critical_success":
		return Vec4{140, 45, 45, 1}
	case "failure", "critical_failure":
		return p.Danger
	default:
		return p.Muted
	}
}
