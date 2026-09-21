package scene

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func renderFrame(t *testing.T, r *Renderer, req FrameRequest) []byte {
	t.Helper()

	img := r.Frame(req)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	return buf.Bytes()
}

func narrationBeat() Beat {
	return Beat{Kind: BeatNarration, Text: "The hall is quiet and the candles gutter.", Duration: 3 * time.Second}
}

func TestFrameIsTheRequestedSize(t *testing.T) {
	r, err := NewRenderer(640, 360)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	img := r.Frame(FrameRequest{Beat: narrationBeat(), Progress: 1})
	if img.Bounds().Dx() != 640 || img.Bounds().Dy() != 360 {
		t.Errorf("bounds = %v, want 640x360", img.Bounds())
	}
}

func TestFrameIsDeterministic(t *testing.T) {
	r, err := NewRenderer(320, 180)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	req := FrameRequest{Beat: narrationBeat(), Progress: 1}
	if !bytes.Equal(renderFrame(t, r, req), renderFrame(t, r, req)) {
		t.Errorf("the same frame request must render identically")
	}
}

func TestFrameRevealsTextOverTheBeat(t *testing.T) {
	r, err := NewRenderer(320, 180)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	early := renderFrame(t, r, FrameRequest{Beat: narrationBeat(), Progress: 0.1})
	whole := renderFrame(t, r, FrameRequest{Beat: narrationBeat(), Progress: 1})
	if bytes.Equal(early, whole) {
		t.Errorf("expected the reveal to change the frame")
	}

	// Past the typewriter window every character is shown. Frames do not settle
	// pixel-for-pixel, because the background drifts for the whole beat, so the
	// revealed text is what is checked here.
	beat := narrationBeat()
	if got := revealText(beat.Text, TypewriterFraction+0.1); got != beat.Text {
		t.Errorf("expected the text to be fully revealed after %.0f%% of the beat, got %q", TypewriterFraction*100, got)
	}
}

func TestFrameDrawsTextPixels(t *testing.T) {
	r, err := NewRenderer(320, 180)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	img := r.Frame(FrameRequest{Beat: narrationBeat(), Progress: 1})

	distinct := 0
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if img.RGBAAt(x, y) != baseColour {
				distinct++
			}
		}
	}
	if distinct < 100 {
		t.Errorf("expected visible text, found only %d non-background pixels", distinct)
	}
}

func TestFrameDrawsASpeakerLabelForSpeech(t *testing.T) {
	r, err := NewRenderer(320, 180)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	narration := renderFrame(t, r, FrameRequest{Beat: narrationBeat(), Progress: 1})
	speech := renderFrame(t, r, FrameRequest{
		Beat:     Beat{Kind: BeatSpeech, Speaker: "Garrick", Text: "Keep walking.", Duration: time.Second},
		Progress: 1,
	})
	if bytes.Equal(narration, speech) {
		t.Errorf("expected a speech beat to differ from narration")
	}
}

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

func TestWrapTextBoundsTheFrame(t *testing.T) {
	r, err := NewRenderer(320, 180)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	long := strings.Repeat("word ", 400)
	lines := wrapText(r.body, long, 200, 4)

	if len(lines) > 4 {
		t.Errorf("expected at most 4 lines, got %d", len(lines))
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[len(lines)-1]), "…") {
		t.Errorf("expected the last line to be elided, got %q", lines[len(lines)-1])
	}
}

func TestWrapTextKeepsShortTextOnOneLine(t *testing.T) {
	r, err := NewRenderer(320, 180)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	lines := wrapText(r.body, "Keep walking.", 200, 4)
	if len(lines) != 1 || lines[0] != "Keep walking." {
		t.Errorf("expected one unelided line, got %q", lines)
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

func TestProceduralBackgroundIsDeterministicAndVaried(t *testing.T) {
	first := proceduralBackground("aldon-harbour", 160, 90)
	repeat := proceduralBackground("aldon-harbour", 160, 90)
	if !bytes.Equal(pngBytes(t, first), pngBytes(t, repeat)) {
		t.Errorf("the same seed must produce the same background")
	}

	other := proceduralBackground("alden-tavern", 160, 90)
	if bytes.Equal(pngBytes(t, first), pngBytes(t, other)) {
		t.Errorf("different locations must not share a background")
	}
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func TestLoadArtDecodesRasterAndRejectsSVG(t *testing.T) {
	dir := t.TempDir()

	pngPath := filepath.Join(dir, "art.png")
	if err := os.WriteFile(pngPath, pngBytes(t, image.NewRGBA(image.Rect(0, 0, 8, 8))), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadArt(pngPath); err != nil {
		t.Errorf("expected a PNG to decode: %v", err)
	}

	svgPath := filepath.Join(dir, "art.svg")
	if err := os.WriteFile(svgPath, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadArt(svgPath); err == nil {
		t.Errorf("expected SVG art to be rejected so the procedural background is used")
	}

	if _, err := loadArt(filepath.Join(dir, "absent.png")); err == nil {
		t.Errorf("expected an error for a missing file")
	}

	if _, err := loadArt("   "); err == nil {
		t.Errorf("expected an error for an empty path")
	}
}

func TestFrameUsesSceneArtWhenItDecodes(t *testing.T) {
	r, err := NewRenderer(160, 90)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	dir := t.TempDir()
	artPath := filepath.Join(dir, "art.png")
	flat := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(flat, flat.Bounds(), image.NewUniform(color.RGBA{200, 30, 30, 255}), image.Point{}, draw.Src)
	if err := os.WriteFile(artPath, pngBytes(t, flat), 0644); err != nil {
		t.Fatal(err)
	}

	withArt := renderFrame(t, r, FrameRequest{
		Scene: Scene{LocationID: "alden-tavern", ArtPath: artPath},
		Beat:  Beat{Kind: BeatNarration, Text: "Red room."}, Progress: 1,
	})
	without := renderFrame(t, r, FrameRequest{Beat: narrationBeat(), Progress: 1})

	if bytes.Equal(withArt, without) {
		t.Errorf("expected scene art to change the frame")
	}
}

func TestFrameFallsBackToAProceduralBackgroundForSVG(t *testing.T) {
	r, err := NewRenderer(160, 90)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	dir := t.TempDir()
	svgPath := filepath.Join(dir, "art.svg")
	if err := os.WriteFile(svgPath, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0644); err != nil {
		t.Fatal(err)
	}

	frame := r.Frame(FrameRequest{
		Scene: Scene{LocationID: "alden-tavern", ArtPath: svgPath},
		Beat:  Beat{Kind: BeatNarration, Text: "Warm."}, Progress: 1,
	})

	// The frame is not the flat base colour, because the fallback drew something.
	distinct := 0
	for y := 0; y < frame.Bounds().Dy(); y++ {
		for x := 0; x < frame.Bounds().Dx(); x++ {
			if frame.RGBAAt(x, y) != baseColour {
				distinct++
			}
		}
	}
	if distinct == 0 {
		t.Errorf("expected the procedural background to cover the frame")
	}
}

func TestDrawCoverHandlesTinyArt(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 50))
	drawCover(img, image.NewRGBA(image.Rect(0, 0, 1, 1)), 1.05)
	drawCover(img, image.NewRGBA(image.Rect(0, 0, 400, 10)), 1.05)
	drawCover(img, image.NewRGBA(image.Rect(0, 0, 0, 0)), 1.05)
}

func TestFrameCrossfadesBetweenScenes(t *testing.T) {
	r, err := NewRenderer(160, 90)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	dir := t.TempDir()
	red := filepath.Join(dir, "red.png")
	blue := filepath.Join(dir, "blue.png")

	paint := func(path string, col color.RGBA) {
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		draw.Draw(img, img.Bounds(), image.NewUniform(col), image.Point{}, draw.Src)
		if err := os.WriteFile(path, pngBytes(t, img), 0644); err != nil {
			t.Fatal(err)
		}
	}
	paint(red, color.RGBA{220, 20, 20, 255})
	paint(blue, color.RGBA{20, 20, 220, 255})

	beat := Beat{Kind: BeatNarration, Text: "The docks.", Duration: 3 * time.Second}
	sc := Scene{LocationID: "aldon-harbour", ArtPath: blue}

	blending := renderFrame(t, r, FrameRequest{Scene: sc, Beat: beat, Progress: crossfadeShare / 2, PreviousArt: red})
	settled := renderFrame(t, r, FrameRequest{Scene: sc, Beat: beat, Progress: crossfadeShare + 0.1, PreviousArt: red})

	if bytes.Equal(blending, settled) {
		t.Errorf("expected the opening frames to differ while the scene blends in")
	}
}

func TestCrossfadeIsDoneByTheEndOfItsShare(t *testing.T) {
	if got := crossfadeAlpha(0); got != 0 {
		t.Errorf("crossfadeAlpha at the start = %v, want 0", got)
	}
	if got := crossfadeAlpha(crossfadeShare / 2); got < 0.4 || got > 0.6 {
		t.Errorf("crossfadeAlpha halfway = %v, want about 0.5", got)
	}
	if got := crossfadeAlpha(crossfadeShare + 0.1); got != 1 {
		t.Errorf("crossfadeAlpha past the share = %v, want 1", got)
	}
}

func TestSceneCardStandsOut(t *testing.T) {
	r, err := NewRenderer(160, 90)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	card := renderFrame(t, r, FrameRequest{
		Scene:    Scene{LocationID: "alden-tavern", LocationName: "Alden Tavern"},
		Beat:     SceneCard(Scene{LocationName: "Alden Tavern"}),
		Progress: 1,
	})
	narration := renderFrame(t, r, FrameRequest{Beat: narrationBeat(), Progress: 1})

	if bytes.Equal(card, narration) {
		t.Errorf("expected a scene card to look different from narration")
	}
}
