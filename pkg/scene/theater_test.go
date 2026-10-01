package scene

import (
	"image"
	"image/color"
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
