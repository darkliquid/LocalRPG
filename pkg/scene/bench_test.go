package scene

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"
)

func benchScript(artPath string) *Script {
	return &Script{
		GameID:   "campaign-01",
		GameName: "Campaign One",
		Scenes: []Scene{{
			LocationID:   "alden-tavern",
			LocationName: "Alden Tavern",
			ArtPath:      artPath,
			Beats:        []Beat{{Kind: BeatNarration, Text: "The hall is quiet and the candles gutter.", Duration: 3 * time.Second}},
		}},
	}
}

func BenchmarkFrame1080pNoArt(b *testing.B) {
	r, err := NewRenderer(1920, 1080)
	if err != nil {
		b.Fatal(err)
	}
	script := benchScript("")
	beat := script.Scenes[0].Beats[0]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Frame(FrameRequest{Script: script, Scene: script.Scenes[0], Beat: beat, Progress: float64(i%15) / 15, Animate: true})
	}
}

func BenchmarkFrame1080pWithArt(b *testing.B) {
	art := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			art.SetRGBA(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 100, 255})
		}
	}
	path := b.TempDir() + "/art.png"
	file, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	if err := png.Encode(file, art); err != nil {
		b.Fatal(err)
	}
	file.Close()

	r, err := NewRenderer(1920, 1080)
	if err != nil {
		b.Fatal(err)
	}
	script := benchScript(path)
	beat := script.Scenes[0].Beats[0]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Frame(FrameRequest{Script: script, Scene: script.Scenes[0], Beat: beat, Progress: float64(i%15) / 15, Animate: true})
	}
}
