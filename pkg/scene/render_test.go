package scene

import (
	"image"
	"image/color"
	"testing"
)

func TestRevealTextFollowsTheTypewriterWindow(t *testing.T) {
	text := "abcdefghij"

	if got := revealText(text, 0); got != "" {
		t.Errorf("revealText at progress 0 = %q, want empty", got)
	}
	if got := revealText(text, TypewriterFraction); got != text {
		t.Errorf("revealText at the window = %q, want the whole text", got)
	}
	half := revealText(text, TypewriterFraction/2)
	if len(half) != 5 {
		t.Errorf("revealText halfway = %q (%d runes), want 5", half, len(half))
	}
	if got := revealText(text, 1); got != text {
		t.Errorf("revealText at progress 1 = %q, want the whole text", got)
	}
}

// TestBaseColourMatchesTheApp pins the background against the colour the player
// and the chronicle use, so a rendered frame and the app agree.
func TestBaseColourMatchesTheApp(t *testing.T) {
	want := color.RGBA{12, 10, 9, 255}
	if baseColour != want {
		t.Errorf("baseColour = %v, want %v", baseColour, want)
	}
}

func TestScaleToCoverHandlesTinyArt(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1, 1))
	src.SetRGBA(0, 0, color.RGBA{200, 100, 50, 255})

	covered := scaleToCover(src, 32, 18)

	if covered.Bounds().Dx() != 32 || covered.Bounds().Dy() != 18 {
		t.Fatalf("bounds = %v, want 32x18", covered.Bounds())
	}
	if covered.RGBAAt(16, 9).A != 255 {
		t.Error("a one-pixel source must still cover the frame")
	}
}

func TestCrossfadeIsDoneByTheEndOfItsShare(t *testing.T) {
	if got := crossfadeAlpha(0); got != 0 {
		t.Errorf("crossfadeAlpha(0) = %v, want 0", got)
	}
	if got := crossfadeAlpha(CrossfadeShare); got != 1 {
		t.Errorf("crossfadeAlpha(share) = %v, want 1", got)
	}
	if got := crossfadeAlpha(1); got != 1 {
		t.Errorf("crossfadeAlpha(1) = %v, want 1", got)
	}
}
