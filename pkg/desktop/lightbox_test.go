package desktop

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/ui"
)

func writePNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 60, G: 40, B: 20, A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "banner.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLightboxSnapshot(t *testing.T) {
	appState = &State{Loaded: true, LightboxPath: writePNG(t, 8, 8)}
	ui.Snapshot(t, "lightbox", 800, 600, RootView)
}

func TestLightboxStateCloses(t *testing.T) {
	appState = &State{Loaded: true, LightboxPath: "/tmp/example.png"}
	dismiss := func() { appState.LightboxPath = "" }
	dismiss()
	if appState.LightboxPath != "" {
		t.Fatal("dismiss must clear the lightbox path")
	}
}
