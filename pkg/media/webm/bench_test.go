package webm

import (
	"fmt"
	"image"
	"image/color"
	"math/rand"
	"testing"

	"github.com/gen2brain/vpx/vp8"
)

// noisyFrame is a worst case for the encoder: high-frequency detail leaves it
// little to predict, so it bounds the frame cost rather than flattering it.
func noisyFrame(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	rng := rand.New(rand.NewSource(1))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(rng.Intn(60) + 10), uint8(rng.Intn(60) + 10), uint8(rng.Intn(60) + 10), 255})
		}
	}
	return img
}

func BenchmarkEncode1080p(b *testing.B) {
	img := noisyFrame(1920, 1080)
	pic := newPicture(1920, 1080)
	toYUV420(img, pic)

	b.Run("key", func(b *testing.B) {
		var enc vp8.Encoder
		for i := 0; i < b.N; i++ {
			if _, err := enc.Encode(pic, vp8.EncodeOptions{Quality: 80, Method: DefaultMethod}); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("inter", func(b *testing.B) {
		var enc vp8.Encoder
		if _, err := enc.Encode(pic, vp8.EncodeOptions{Quality: 80, Method: DefaultMethod}); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := enc.EncodeInter(pic, vp8.EncodeOptions{Quality: 80, Method: DefaultMethod}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkEncodeMethodCost records why the encoder runs at method 2: the higher
// methods buy a marginal size gain for a large per-frame cost.
func BenchmarkEncodeMethodCost(b *testing.B) {
	img := noisyFrame(1920, 1080)
	pic := newPicture(1920, 1080)
	toYUV420(img, pic)

	for _, method := range []int{2, 3, 5} {
		b.Run(fmt.Sprintf("method%d", method), func(b *testing.B) {
			var enc vp8.Encoder
			for i := 0; i < b.N; i++ {
				if _, err := enc.Encode(pic, vp8.EncodeOptions{Quality: 80, Method: method}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
