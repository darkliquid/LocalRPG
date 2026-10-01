package scene

import (
	"image/color"
	"testing"
	"time"
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
