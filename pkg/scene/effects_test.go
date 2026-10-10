package scene

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// writeTestPNG writes a small gradient PNG, so a scaled composite differs from
// the original frame.
func writeTestPNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 6), B: 128, A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "art.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKenBurnsIsBoundedAndSeeded(t *testing.T) {
	if got := KenBurns(1, 0); got != (KenBurnsTransform{Scale: 1}) {
		t.Fatalf("progress zero should be the identity, got %+v", got)
	}
	half := KenBurns(1, 0.5)
	if half.Scale <= 1 || half.Scale >= 1.1 {
		t.Fatalf("scale = %v, want a small zoom in (1, 1.1)", half.Scale)
	}
	if KenBurns(1, 0.5) != half {
		t.Fatal("ken burns is not deterministic")
	}
	if KenBurns(1, 2).Scale != KenBurns(1, 1).Scale {
		t.Fatal("progress should clamp to [0,1]")
	}
}

// TestKenBurnsMatchesTheAppRule pins the exact values the frontend's kenBurns
// produces for the same inputs, so the two renderers cannot drift apart.
func TestKenBurnsMatchesTheAppRule(t *testing.T) {
	cases := []struct {
		seed     int
		progress float64
		want     KenBurnsTransform
	}{
		{1, 0, KenBurnsTransform{Scale: 1}},
		{1, 1, KenBurnsTransform{Scale: 1.08, DX: 0.035, DY: -0.035}},
		{2, 1, KenBurnsTransform{Scale: 1.08, DX: -0.035, DY: 0.035}},
		{3, 1, KenBurnsTransform{Scale: 1.08, DX: 0.035, DY: 0.035}},
		{4, 1, KenBurnsTransform{Scale: 1.08, DX: -0.035, DY: -0.035}},
		{0, 1, KenBurnsTransform{Scale: 1.08, DX: -0.035, DY: -0.035}},
		{1, 0.5, KenBurnsTransform{Scale: 1.04, DX: 0.0175, DY: -0.0175}},
	}
	for _, tc := range cases {
		got := KenBurns(tc.seed, tc.progress)
		if math.Abs(got.Scale-tc.want.Scale) > 1e-9 ||
			math.Abs(got.DX-tc.want.DX) > 1e-9 ||
			math.Abs(got.DY-tc.want.DY) > 1e-9 {
			t.Errorf("KenBurns(%d, %v) = %+v, want %+v", tc.seed, tc.progress, got, tc.want)
		}
	}
}

func TestParallaxMovesTheForegroundMore(t *testing.T) {
	if got := ParallaxOffset(0, 1); got != 0 {
		t.Fatalf("the background should not move, got %v", got)
	}
	if ParallaxOffset(1, 1) <= ParallaxOffset(0.5, 1) {
		t.Fatal("a nearer layer should move more than a farther one")
	}
}

func TestMoodTintFollowsOutcome(t *testing.T) {
	if MoodTint("miss").Opacity <= 0 {
		t.Fatal("a failure should tint the picture")
	}
	if MoodTint("").Opacity != 0 {
		t.Fatal("an unknown outcome should not tint")
	}
	if MoodTint("SUCCESS").Opacity != MoodTint("success").Opacity {
		t.Fatal("the outcome match should be case-insensitive")
	}
}

func TestWeatherOverlayOnlyForKnownWeather(t *testing.T) {
	for _, want := range []WeatherKind{WeatherRain, WeatherSnow, WeatherFog} {
		if got := WeatherOverlay(string(want)); got != want {
			t.Errorf("WeatherOverlay(%q) = %q", want, got)
		}
	}
	for _, unknown := range []string{"", "sunny", "  "} {
		if got := WeatherOverlay(unknown); got != WeatherNone {
			t.Errorf("WeatherOverlay(%q) = %q, want none", unknown, got)
		}
	}
}

// TestTintAndOverlayDrawn proves the tint and the weather overlay reach the
// frame: a failure scene and a rainy scene each differ from a plain one.
func TestTintAndOverlayDrawn(t *testing.T) {
	renderer, err := NewRenderer(64, 36)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	frame := func(scene Scene, outcome string) *image.RGBA {
		return renderer.Frame(FrameRequest{
			Scene:    scene,
			Beat:     Beat{Kind: BeatNarration, Text: "A line.", Outcome: outcome},
			Progress: 0.5,
			Animate:  true,
		})
	}

	plain := frame(Scene{}, "")
	miss := frame(Scene{}, "miss")
	if bytes.Equal(plain.Pix, miss.Pix) {
		t.Fatal("a failure should tint the frame")
	}

	rain := frame(Scene{Weather: "rain"}, "")
	if bytes.Equal(plain.Pix, rain.Pix) {
		t.Fatal("rain should draw an overlay")
	}
}

// TestKenBurnsMovesTheFrame proves an animated beat's frame differs across its
// span, which is what makes the export drift rather than sit still.
func TestKenBurnsMovesTheFrame(t *testing.T) {
	renderer, err := NewRenderer(64, 36)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	art := writeTestPNG(t, 64, 36)

	at := func(progress float64) *image.RGBA {
		return renderer.Frame(FrameRequest{
			Scene:    Scene{ArtPath: art},
			Beat:     Beat{Kind: BeatNarration, Text: "A line.", TurnNumber: 1, ArtPath: art},
			Progress: progress,
			Animate:  true,
		})
	}
	if bytes.Equal(at(0).Pix, at(0.9).Pix) {
		t.Fatal("ken burns should move the picture over the beat")
	}

	// A still render never drifts: two still frames of the same beat are identical.
	stillBeat := Beat{Kind: BeatNarration, TurnNumber: 1, ArtPath: art}
	still := renderer.Frame(FrameRequest{Scene: Scene{ArtPath: art}, Beat: stillBeat, Progress: 0, Animate: false})
	again := renderer.Frame(FrameRequest{Scene: Scene{ArtPath: art}, Beat: stillBeat, Progress: 0.9, Animate: false})
	if !bytes.Equal(still.Pix, again.Pix) {
		t.Fatal("a still render should not drift")
	}
}
