package export

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/scene"
)

// bigPNG writes an opaque image larger than the display size.
func bigPNG(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "big.png")
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

func TestEmbeddedImageIsDownscaled(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	big := bigPNG(t, 4000, 3000)

	script := &scene.Script{
		GameName: "Campaign One",
		Banner:   big,
		Scenes: []scene.Scene{{
			LocationName: "Alden Tavern",
			ArtPath:      big,
			Beats:        []scene.Beat{{Kind: scene.BeatNarration, Text: "Warm light.", Duration: time.Second}},
		}},
		TotalDuration: time.Second,
	}
	if _, err := testExporter().Export(context.Background(), script, out); err != nil {
		t.Fatalf("Export: %v", err)
	}

	payload := readWebPayload(t, out)
	uri := payload.Banner
	if uri == "" {
		uri = payload.Scenes[0].Art
	}
	if uri == "" {
		t.Fatal("the payload carries no image")
	}
	data, err := decodeDataURI(uri)
	if err != nil {
		t.Fatalf("decode data URI: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode embedded image: %v", err)
	}
	if img.Bounds().Dx() > maxEmbedWidth || img.Bounds().Dy() > maxEmbedHeight {
		t.Fatalf("embedded image is %dx%d, want no larger than %dx%d",
			img.Bounds().Dx(), img.Bounds().Dy(), maxEmbedWidth, maxEmbedHeight)
	}

	// The campaign's own file is untouched.
	source, err := os.Open(big)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	original, _, err := image.Decode(source)
	if err != nil {
		t.Fatal(err)
	}
	if original.Bounds().Dx() != 4000 || original.Bounds().Dy() != 3000 {
		t.Fatalf("the source image was modified: %v", original.Bounds())
	}
}
