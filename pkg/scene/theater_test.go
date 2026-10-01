package scene

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/media"
)

func stageScript() *Script {
	return &Script{
		GameID:   "campaign-01",
		GameName: "Campaign One",
		Scenes: []Scene{{
			LocationID:   "alden-tavern",
			LocationName: "Alden Tavern",
			Beats:        []Beat{{Kind: BeatNarration, Text: "Warm light.", Duration: 2000 * time.Millisecond}},
		}},
	}
}

func TestFrameDrawsTheBackgroundAndScrim(t *testing.T) {
	renderer, err := NewRenderer(320, 180)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	script := stageScript()
	frame := renderer.Frame(FrameRequest{
		Script: script, Scene: script.Scenes[0], Beat: script.Scenes[0].Beats[0],
		Progress: 1, Animate: false,
	})

	if frame.Bounds().Dx() != 320 || frame.Bounds().Dy() != 180 {
		t.Fatalf("frame bounds = %v", frame.Bounds())
	}
	// A blank canvas would be fully transparent; the stage must fill every pixel.
	if frame.RGBAAt(160, 2) == (color.RGBA{0, 0, 0, 0}) {
		t.Fatal("expected the stage to fill the frame")
	}
	if frame.RGBAAt(10, 170).A != 255 {
		t.Fatal("expected the scrim to be opaque at the base")
	}
}

func TestFrameDrawsTheActiveSpeakerBorder(t *testing.T) {
	renderer, err := NewRenderer(640, 360)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	script := stageScript()
	script.PlayerPortrait = writeArtFile(t, "player.svg", media.GenerateProceduralBustSVG("hero", "Hero", "female"))
	beat := Beat{Kind: BeatSpeech, Speaker: "Hero", Player: true, Text: "Hello.", Duration: 2000 * time.Millisecond}

	frame := renderer.Frame(FrameRequest{
		Script: script, Scene: script.Scenes[0], Beat: beat, Progress: 1, Animate: false,
	})

	if !hasColourNear(frame, skyAccent, 60) {
		t.Fatal("expected the player's active border colour somewhere in the frame")
	}
}

// hasColourNear reports whether any pixel is within tolerance of want.
func hasColourNear(img *image.RGBA, want color.RGBA, tolerance int) bool {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			c := img.RGBAAt(x, y)
			if absDiff(c.R, want.R) <= tolerance && absDiff(c.G, want.G) <= tolerance && absDiff(c.B, want.B) <= tolerance {
				return true
			}
		}
	}
	return false
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a) - int(b)
	}
	return int(b) - int(a)
}

func TestFrameDrawsTheSpeakerNamePlate(t *testing.T) {
	renderer, _ := NewRenderer(640, 360)
	script := stageScript()
	beat := Beat{Kind: BeatSpeech, Speaker: "Evelyn", Text: "Well met.", Duration: 2000 * time.Millisecond}

	frame := renderer.Frame(FrameRequest{Script: script, Scene: script.Scenes[0], Beat: beat, Progress: 1})

	if !hasColourNear(frame, purpleAccent, 40) {
		t.Fatal("expected the speech panel's purple accent")
	}
}

func TestFrameDrawsTheSceneCardInAmber(t *testing.T) {
	renderer, _ := NewRenderer(640, 360)
	script := stageScript()
	beat := SceneCard(script.Scenes[0])

	frame := renderer.Frame(FrameRequest{Script: script, Scene: script.Scenes[0], Beat: beat, Progress: 1})

	if !hasColourNear(frame, amberLabel, 30) {
		t.Fatal("expected the scene card's amber title")
	}
}

func TestPlanFramesCountsRepeats(t *testing.T) {
	script := &Script{Scenes: []Scene{{Beats: []Beat{
		{Kind: BeatNarration, Text: "A line.", Duration: 2 * time.Second},
	}}}}

	plan := PlanFrames(script, 15, true)
	if plan.Total != plan.Image+plan.Repeat {
		t.Fatalf("total %d != image %d + repeat %d", plan.Total, plan.Image, plan.Repeat)
	}
	if plan.Repeat == 0 {
		t.Error("expected the still tail of the beat to be repeat frames")
	}
	if plan.Image == 0 {
		t.Error("expected the reveal to be image frames")
	}
	if plan.Duration != 2*time.Second {
		t.Errorf("duration = %v, want 2s", plan.Duration)
	}

	static := PlanFrames(script, 15, false)
	if static.Total != 1 || static.Repeat != 0 || static.Image != 1 {
		t.Errorf("static plan = %+v, want one image frame", static)
	}
	if static.Duration != 2*time.Second {
		t.Errorf("static duration = %v, want 2s", static.Duration)
	}
}

func TestBeatFramePlanSplitsRevealAndHold(t *testing.T) {
	beat := Beat{Kind: BeatNarration, Text: "A line.", Duration: 2 * time.Second}

	if plan := BeatFramePlan(beat, 15, false); len(plan) != 1 || plan[0].Progress != 1 {
		t.Fatalf("static plan = %+v, want one fully revealed frame", plan)
	}

	plan := BeatFramePlan(beat, 15, true)
	if len(plan) < 2 {
		t.Fatalf("animated plan has %d frames, want a reveal and a hold", len(plan))
	}
	if plan[0].Progress != 0 {
		t.Errorf("first frame at progress %v, want 0", plan[0].Progress)
	}
	if last := plan[len(plan)-1]; last.Progress != 1 {
		t.Errorf("last frame at progress %v, want 1", last.Progress)
	}

	var total time.Duration
	for _, step := range plan {
		total += step.Span
	}
	if total != beat.Duration {
		t.Errorf("plan spans %v, want %v", total, beat.Duration)
	}
}

// TestBeatFramePlanRevealsWithTheAudio pins the pacing fix: a beat whose clip is
// the narration of its own text must reveal across the clip, not across the first
// three fifths of the beat, or the words run ahead of the voice.
func TestBeatFramePlanRevealsWithTheAudio(t *testing.T) {
	beat := Beat{
		Kind:          BeatNarration,
		Text:          "A line.",
		Duration:      4400 * time.Millisecond,
		AudioDuration: 4 * time.Second,
	}

	var revealSpan time.Duration
	for _, step := range BeatFramePlan(beat, 10, true) {
		if !step.Repeat {
			revealSpan += step.Span
		}
	}
	// The reveal runs with the clip but finishes before it, so the closing word is
	// on screen while it is still being spoken rather than arriving on the last
	// frame the audio reaches.
	if revealSpan >= beat.AudioDuration {
		t.Errorf("reveal spans %v, want it to finish before the clip's %v", revealSpan, beat.AudioDuration)
	}
	if revealSpan < beat.AudioDuration*3/4 {
		t.Errorf("reveal spans %v, want most of the clip's %v", revealSpan, beat.AudioDuration)
	}
}

// TestLayoutProseFitsThePanel pins the other half: a long beat must produce a
// layout the panel can hold, rather than running off the bottom of a fixed box.
func TestLayoutProseFitsThePanel(t *testing.T) {
	renderer, err := NewRenderer(960, 540)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	text := strings.Repeat("The hall is quiet and the candles gutter. ", 6)
	layout := renderer.layoutProse(text, false, DisplayStageDirections, renderer.panelWidth())

	if len(layout.lines) == 0 {
		t.Fatal("expected the prose to lay out into lines")
	}
	if layout.height() > renderer.maxPanelHeight() {
		t.Errorf("layout is %d tall, the tallest panel is %d", layout.height(), renderer.maxPanelHeight())
	}
	if layout.height() < renderer.minPanelHeight() {
		t.Errorf("layout is %d tall, shorter than the minimum panel %d", layout.height(), renderer.minPanelHeight())
	}
}
