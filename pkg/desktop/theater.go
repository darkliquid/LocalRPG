package desktop

import (
	"fmt"
	"time"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/theater"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// theaterScript builds the script the shared theatre view plays.
func theaterScript() *scene.Script {
	script := &scene.Script{}
	for i := range appState.Turns {
		turn := &appState.Turns[i]
		sc := scene.Scene{
			LocationID:   turn.LocationID,
			LocationName: turn.LocationName,
		}
		for j := range turn.Segments {
			seg := &turn.Segments[j]
			kind := scene.BeatNarration
			if seg.Kind == "speech" {
				kind = scene.BeatSpeech
			}
			beat := scene.Beat{Kind: kind, Speaker: seg.Speaker, Text: seg.Text}
			if seg.Duration > 0 {
				beat.Duration = time.Duration(seg.Duration * float64(time.Second))
			} else {
				beat.Duration = scene.ReadingDuration(seg.Text)
			}
			sc.Beats = append(sc.Beats, beat)
		}
		if len(sc.Beats) == 0 && turn.Prose != "" {
			sc.Beats = append(sc.Beats, scene.Beat{Kind: scene.BeatNarration, Text: turn.Prose, Duration: 3 * time.Second})
		}
		script.Scenes = append(script.Scenes, sc)
	}
	return script
}

func openTheater() {
	appState.Screen = ScreenTheater
	appState.TheaterTurn = 0
	appState.TheaterBeat = 0
	appState.TheaterPlaying = true
	if appState.TheaterSpeed == 0 {
		appState.TheaterSpeed = 1
	}
}

// advanceBeat moves to the next segment, or the next turn when the current one
// is exhausted.
func advanceBeat() {
	if appState.TheaterTurn >= len(appState.Turns) {
		return
	}
	segments := len(appState.Turns[appState.TheaterTurn].Segments)
	if appState.TheaterBeat+1 < segments {
		appState.TheaterBeat++
		return
	}
	if appState.TheaterTurn+1 < len(appState.Turns) {
		appState.TheaterTurn++
		appState.TheaterBeat = 0
		return
	}
	appState.TheaterPlaying = false
}

// changeTurn moves between turns, clamping at the ends.
func changeTurn(delta int) {
	next := appState.TheaterTurn + delta
	if next < 0 {
		next = 0
	}
	if next >= len(appState.Turns) {
		next = len(appState.Turns) - 1
	}
	if next < 0 {
		next = 0
	}
	appState.TheaterTurn = next
	appState.TheaterBeat = 0
}

func togglePlay() {
	appState.TheaterPlaying = !appState.TheaterPlaying
	if appState.TheaterPlaying && appState.TheaterTurn >= len(appState.Turns) {
		appState.TheaterTurn = 0
		appState.TheaterBeat = 0
	}
}

func theaterScreen() {
	p := ui.DefaultPalette()
	script := theaterScript()
	frame := theater.Frame{
		Script:   script,
		SceneIdx: appState.TheaterTurn,
		BeatIdx:  appState.TheaterBeat,
		Progress: 0.5,
	}
	Container(Attrs(Viewport, BackgroundVec(p.Bg)), func() {
		Container(Attrs(Grow(1), Expand, Clip), func() {
			theater.ViewWith(frame, func(text string) {
				proseBlocks(text, TextColorVec(p.Text))
			})
		})
		Container(Attrs(Row, CrossMid, Gap(10), Pad(12), BackgroundVec(p.Panel)), func() {
			NextAccessName("theater.prev")
			if Button(NoIcon, "◀") {
				changeTurn(-1)
			}
			AssignAccess()
			NextAccessName("theater.play")
			label := "Play"
			if appState.TheaterPlaying {
				label = "Pause"
			}
			if Button(NoIcon, label) {
				togglePlay()
			}
			AssignAccess()
			NextAccessName("theater.next")
			if Button(NoIcon, "▶") {
				changeTurn(1)
			}
			AssignAccess()
			Label(fmt.Sprintf("%.1fx", appState.TheaterSpeed), FontSize(12), TextColorVec(p.Muted))
			Filler(1)
			NextAccessName("theater.close")
			if Button(NoIcon, "Close") {
				appState.Screen = ScreenChronicle
			}
			AssignAccess()
		})
	})
}
