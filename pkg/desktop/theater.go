package desktop

import (
	"fmt"
	"time"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/gui"
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
			ArtPath:      sceneArtPath(turn),
		}
		for j := range turn.Segments {
			seg := &turn.Segments[j]
			kind := scene.BeatNarration
			if seg.Kind == "speech" {
				kind = scene.BeatSpeech
			}
			beat := scene.Beat{Kind: kind, Speaker: seg.Speaker, SpeakerID: seg.SpeakerID, Text: seg.Text}
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
	appState.TheaterBeatKey = ""
	if appState.TheaterSpeed == 0 {
		appState.TheaterSpeed = 1
	}
}

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

func changeTurn(delta int) {
	next := appState.TheaterTurn + delta
	if next < 0 {
		next = 0
	}
	if len(appState.Turns) > 0 && next >= len(appState.Turns) {
		next = len(appState.Turns) - 1
	}
	appState.TheaterTurn = next
	appState.TheaterBeat = 0
	appState.TheaterBeatKey = ""
}

func togglePlay() {
	appState.TheaterPlaying = !appState.TheaterPlaying
	if appState.TheaterPlaying && appState.TheaterTurn >= len(appState.Turns) {
		appState.TheaterTurn = 0
		appState.TheaterBeat = 0
		appState.TheaterBeatKey = ""
	}
}

// currentSegment returns the segment the theatre is showing, or nil.
func currentSegment() *gui.SegmentDTO {
	if appState.TheaterTurn < 0 || appState.TheaterTurn >= len(appState.Turns) {
		return nil
	}
	segs := appState.Turns[appState.TheaterTurn].Segments
	if appState.TheaterBeat < 0 || appState.TheaterBeat >= len(segs) {
		return nil
	}
	return &segs[appState.TheaterBeat]
}

// theaterFrame assembles the shared frame plus its stage presentation.
func theaterFrame(script *scene.Script) theater.Frame {
	f := theater.Frame{
		Script:   script,
		SceneIdx: appState.TheaterTurn,
		BeatIdx:  appState.TheaterBeat,
		Progress: 0.5,
	}
	playerLabel := "You"
	for i := range appState.Turns {
		for j := range appState.Turns[i].Segments {
			seg := &appState.Turns[i].Segments[j]
			if seg.Kind != "speech" {
				continue
			}
			if seg.Player {
				if seg.Speaker != "" {
					playerLabel = seg.Speaker
				}
				if f.PlayerPortrait == "" {
					f.PlayerPortrait = appState.Portraits[seg.SpeakerID]
				}
			} else if f.NPCPortrait == "" {
				f.NPCPortrait = appState.Portraits[seg.SpeakerID]
				f.NPCLabel = seg.Speaker
			}
		}
	}
	f.PlayerLabel = playerLabel
	if seg := currentSegment(); seg != nil && seg.Kind == "speech" {
		if seg.Player {
			f.PlayerActive = true
			if p := appState.Portraits[seg.SpeakerID]; p != "" {
				f.PlayerPortrait = p
			}
		} else {
			f.NPCActive = true
			if p := appState.Portraits[seg.SpeakerID]; p != "" {
				f.NPCPortrait = p
			}
			if seg.Speaker != "" {
				f.NPCLabel = seg.Speaker
			}
		}
	}
	return f
}

func theaterScreen() {
	script := theaterScript()
	Container(Attrs(Viewport, BackgroundVec(ui.CanvasBG)), func() {
		Container(Attrs(Grow(1), Expand, Clip), func() {
			theater.ViewWith(theaterFrame(script), func(text string) {
				proseBlocks(text, Fonts(ui.SerifStack...), FontSize(20), TextColorVec(ui.TextMain))
			})
		})
		theaterTransport()
	})
	if appState.TheaterPlaying {
		advanceTheaterClock()
	}
}

// advanceTheaterClock plays the current beat's audio once and advances the beat
// when its clip finishes or its reading time elapses.
func advanceTheaterClock() {
	key := fmt.Sprintf("%d:%d", appState.TheaterTurn, appState.TheaterBeat)
	if appState.TheaterBeatKey != key {
		appState.TheaterBeatKey = key
		appState.TheaterBeatStart = time.Now()
		if seg := currentSegment(); seg != nil && seg.AudioURL != "" {
			playSegment(appState.TheaterTurn, appState.TheaterBeat, false)
		}
	}
	if appState.TheaterBeatStart.IsZero() {
		appState.TheaterBeatStart = time.Now()
	}
	if time.Since(appState.TheaterBeatStart) >= theaterBeatDwell() {
		appState.TheaterBeatKey = ""
		advanceBeat()
		return
	}
	RequestNextFrame()
}

func theaterBeatDwell() time.Duration {
	seg := currentSegment()
	base := 2 * time.Second
	if seg != nil {
		if seg.Duration > 0 {
			base = time.Duration(seg.Duration * float64(time.Second))
		} else {
			base = scene.ReadingDuration(seg.Text)
		}
	}
	speed := appState.TheaterSpeed
	if speed <= 0 {
		speed = 1
	}
	return time.Duration(float64(base) / speed)
}

func theaterTransport() {
	Container(Attrs(Expand, Pad(16), Gap(10), BackgroundVec(ui.InputBG), BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
		position, total := 0, 0
		for i := range appState.Turns {
			n := len(appState.Turns[i].Segments)
			if i < appState.TheaterTurn {
				position += n
			} else if i == appState.TheaterTurn {
				position += appState.TheaterBeat
			}
			total += n
		}
		frac := float32(0)
		if total > 0 {
			frac = float32(position) / float32(total)
		}
		Container(Attrs(Expand, FixHeight(6), Corners(3), BackgroundVec(ui.RaisedBG)), func() {
			if frac > 0 {
				Container(Attrs(Grow(frac), FixHeight(6), Corners(3), BackgroundVec(ui.Accent)), func() {})
			}
		})

		Container(Attrs(Row, Center, CrossMid, Gap(12)), func() {
			NextAccessName("theater.prev")
			if roundIcon(SymPrev) {
				changeTurn(-1)
			}
			AssignAccess()
			NextAccessName("theater.play")
			if roundPrimary(pickIcon(appState.TheaterPlaying)) {
				togglePlay()
			}
			AssignAccess()
			NextAccessName("theater.next")
			if roundIcon(SymNext) {
				changeTurn(1)
			}
			AssignAccess()
			NextAccessName("theater.speed")
			if speedChip() {
				cycleSpeed()
			}
			AssignAccess()
			NextAccessName("theater.close")
			if Button(NoIcon, "Close") {
				appState.TheaterPlaying = false
				appState.Screen = ScreenChronicle
			}
			AssignAccess()
		})
	})
}

func pickIcon(playing bool) IconGlyph {
	if playing {
		return SymPause
	}
	return SymPlay
}

func cycleSpeed() {
	switch appState.TheaterSpeed {
	case 1:
		appState.TheaterSpeed = 1.5
	case 1.5:
		appState.TheaterSpeed = 2
	default:
		appState.TheaterSpeed = 1
	}
}

func roundIcon(icon IconGlyph) bool {
	clicked := false
	Container(Attrs(FixSize(40, 40), Corners(20), Center), func() {
		if IsHovered() {
			ModAttrs(BackgroundVec(ui.HoverFill))
		}
		if PressAction() {
			clicked = true
		}
		Icon(icon, FontSize(18), TextColorVec(ui.TextMain))
	})
	return clicked
}

func roundPrimary(icon IconGlyph) bool {
	clicked := false
	Container(Attrs(FixSize(52, 52), Corners(26), Center, BackgroundVec(ui.AccentBtn), BoxShadow(20)), func() {
		if IsHovered() {
			ModAttrs(BackgroundVec(ui.AccentBtnHover))
		}
		if PressAction() {
			clicked = true
		}
		Icon(icon, FontSize(22), TextColorVec(ui.TextMain))
	})
	return clicked
}

func speedChip() bool {
	clicked := false
	Container(Attrs(Pad2(6, 12), Corners(10), BackgroundVec(ui.RaisedBG), BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
		if IsHovered() {
			ModAttrs(BorderColorVec(ui.Accent))
		}
		if PressAction() {
			clicked = true
		}
		Label(fmt.Sprintf("%.1fx", appState.TheaterSpeed), Fonts(Monospace...), FontSize(12), FontWeight(WeightBold), TextColorVec(ui.Accent))
	})
	return clicked
}
