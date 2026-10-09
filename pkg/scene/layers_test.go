package scene

import (
	"image"
	"image/color"
	"testing"
	"time"
)

// TestDrawLayersCompositesBackToFront guards that a layered scene is drawn in the
// declared order, so a scene's layers produce the picture its flat art does.
func TestDrawLayersCompositesBackToFront(t *testing.T) {
	renderer, err := NewRenderer(8, 8)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	background := writeArtFile(t, "bg.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><rect width="8" height="8" fill="#204060"/></svg>`))
	foreground := writeArtFile(t, "fg.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><rect x="0" y="0" width="8" height="2" fill="#c02020"/></svg>`))

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	req := FrameRequest{Scene: Scene{Layers: []SceneLayer{
		{Depth: 0, Art: background},
		{Depth: 1, Art: foreground},
	}}}
	if !renderer.drawLayers(img, req) {
		t.Fatal("a layered scene should draw at least one layer")
	}

	// The foreground's red band is over the background's blue fill.
	top := img.RGBAAt(4, 1)
	if top.R < 120 || top.B > 120 {
		t.Fatalf("the foreground layer was not drawn on top: %+v", top)
	}
	bottom := img.RGBAAt(4, 6)
	if bottom.B < 60 || bottom.R > 120 {
		t.Fatalf("the background layer was not drawn beneath: %+v", bottom)
	}
}

func TestDrawLayersReportsNoArt(t *testing.T) {
	renderer, err := NewRenderer(8, 8)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if renderer.drawLayers(img, FrameRequest{Scene: Scene{Layers: []SceneLayer{{Depth: 0}}}}) {
		t.Fatal("a layer with no art should report that nothing was drawn")
	}
}

// TestFrameWithLayersFillsTheStage guards that a layered scene reaches the frame
// path, so a scene whose art is layered still paints the stage.
func TestFrameWithLayersFillsTheStage(t *testing.T) {
	renderer, err := NewRenderer(64, 36)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	script := stageScript()
	script.Scenes[0].Layers = []SceneLayer{{Depth: 0, Art: writeArtFile(t, "bg.svg",
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 36"><rect width="64" height="36" fill="#204060"/></svg>`))}}

	frame := renderer.Frame(FrameRequest{
		Script: script, Scene: script.Scenes[0], Beat: script.Scenes[0].Beats[0],
		Progress: 1, Animate: false,
	})
	if frame.RGBAAt(32, 2) == (color.RGBA{0, 0, 0, 0}) {
		t.Fatal("a layered scene should fill the frame")
	}
}

// TestFlatSceneIsUnchanged guards the flat path: a scene with no layers draws its
// single image exactly as it did before layers existed.
func TestFlatSceneIsUnchanged(t *testing.T) {
	renderer, err := NewRenderer(32, 18)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	art := writeArtFile(t, "flat.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 18"><rect width="32" height="18" fill="#204060"/></svg>`))
	script := stageScript()
	script.Scenes[0].ArtPath = art
	beat := Beat{Kind: BeatNarration, Text: "", Duration: time.Second}

	frame := renderer.Frame(FrameRequest{Script: script, Scene: script.Scenes[0], Beat: beat, Progress: 1})
	if frame.RGBAAt(16, 1) == (color.RGBA{0, 0, 0, 0}) {
		t.Fatal("the flat art should fill the frame")
	}
}
