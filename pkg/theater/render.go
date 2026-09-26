package theater

import (
	"image"
	"image/png"
	"os"

	"go.hasen.dev/shirei"
)

// theaterScope keeps the offline view's identity stable across the settle
// passes of one frame. Because View reads all state from the Frame argument,
// retained identity state cannot change output.
var theaterScope = new(int)

// RenderFrame software-rasterises one theatre frame at scale 1.
//
// It deliberately does not use shirei.RenderToImage, which hardcodes a 2x
// headless scale and would turn a 1920x1080 export frame into a 3840x2160
// rasterisation.
func RenderFrame(f Frame, w, h int) *image.RGBA {
	shirei.ResetInputSession()
	host := shirei.GetHost()
	host.WindowSize = shirei.Vec2{float32(w), float32(h)}
	host.WindowScale = 1
	host.HeadlessRender = true
	defer func() { host.HeadlessRender = false }()
	if host.GlyphCacheBudgetBytes == 0 {
		host.GlyphCacheBudgetBytes = 16 << 20
	}

	var out shirei.FrameOutputData
	for i := 0; i < 8; i++ {
		out = shirei.RunFrameFn(func() {
			shirei.ModAttrs(func(a *shirei.AttrSet) { a.Animations = 0 })
			shirei.ContainerWithKey(theaterScope, shirei.Attrs(shirei.Viewport), func() {
				View(f)
			})
		})
		if i >= 1 && !out.NextFrameRequested {
			break
		}
	}

	var renderer shirei.SoftRenderer
	fb := renderer.Render(out.Surfaces, out.GlyphRuns, w, h, 1)
	return fb.ToRGBA()
}

// WriteFramePNG renders f and writes it as a PNG with fast compression
// (1080p size is irrelevant next to the x264 encode).
func WriteFramePNG(path string, f Frame, w, h int) error {
	img := RenderFrame(f, w, h)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	return encoder.Encode(file, img)
}
