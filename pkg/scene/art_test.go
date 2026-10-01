package scene

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
)

func writeArtFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestArtCacheRasterizesTheProceduralBust(t *testing.T) {
	path := writeArtFile(t, "bust.svg", media.GenerateProceduralBustSVG("evelyn", "Evelyn", "female"))
	img, err := newArtCache().load(path)
	if err != nil {
		t.Fatalf("load SVG: %v", err)
	}
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatalf("rasterized SVG is empty: %v", img.Bounds())
	}
}

func TestArtCacheLoadsRasterArt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scene.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	file.Close()

	img, err := newArtCache().load(path)
	if err != nil {
		t.Fatalf("load PNG: %v", err)
	}
	if img.Bounds().Dx() != 8 {
		t.Fatalf("bounds = %v", img.Bounds())
	}
}

func TestArtCacheReportsAMissingFile(t *testing.T) {
	if _, err := newArtCache().load(filepath.Join(t.TempDir(), "absent.png")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
