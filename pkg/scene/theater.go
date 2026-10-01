package scene

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
)

// drawBackground paints the scene's imagery: the beat's art, the scene's art, or
// the campaign banner, cover-fit. A scene change blends in over the opening share
// of a beat, and the drift scale keeps a held beat alive. With no art at all the
// stage shows the theatre's own radial gradient.
func (r *Renderer) drawBackground(img *image.RGBA, req FrameRequest) {
	scale := driftStart + (driftEnd-driftStart)*clamp01(req.Progress)

	art := r.backgroundArt(req)
	if art == nil {
		drawRadialGradient(img, color.RGBA{38, 30, 27, 255}, baseColour)
		return
	}

	if blend := crossfadeAlpha(req.Progress); blend < 1 && req.PreviousArt != "" {
		if previous, err := r.art.load(req.PreviousArt); err == nil {
			drawCover(img, previous, scale)
			drawCoverAlpha(img, art, scale, blend)
			return
		}
	}
	drawCover(img, art, scale)
}

// backgroundArt is the first art the theatre would show: the beat's own, then the
// scene's, then the campaign's banner.
func (r *Renderer) backgroundArt(req FrameRequest) image.Image {
	for _, path := range []string{req.Beat.ArtPath, req.Scene.ArtPath} {
		if img, err := r.art.load(path); err == nil {
			return img
		}
	}
	if req.Script != nil && req.Script.Banner != "" {
		if img, err := r.art.load(req.Script.Banner); err == nil {
			return img
		}
	}
	return nil
}

// drawScrim is the theatre's bottom-heavy black gradient: black/90 at the base,
// black/45 in the middle, black/60 at the top.
func (r *Renderer) drawScrim(img *image.RGBA) {
	for y := 0; y < r.height; y++ {
		t := float64(y) / float64(max(1, r.height-1))
		row := color.RGBA{0, 0, 0, scrimAlpha(t)}
		draw.Draw(img, image.Rect(0, y, r.width, y+1), image.NewUniform(row), image.Point{}, draw.Over)
	}
}

func scrimAlpha(t float64) uint8 {
	var a float64
	switch {
	case t < 0.5:
		a = 0.60 + (0.45-0.60)*(t/0.5)
	default:
		a = 0.45 + (0.90-0.45)*((t-0.5)/0.5)
	}
	return uint8(a * 255)
}

// drawRadialGradient fills img with the theatre's no-art background: a warm
// centre fading to the base colour.
func drawRadialGradient(img *image.RGBA, centre, edge color.RGBA) {
	b := img.Bounds()
	cx, cy := float64(b.Dx())/2, float64(b.Dy())/2
	radius := math.Hypot(cx, cy)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy) / radius
			img.SetRGBA(x, y, lerpColour(centre, edge, clamp01(d)))
		}
	}
}

func lerpColour(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: 255,
	}
}

// upper is the theatre's label case: uppercase, trimmed.
func upper(text string) string { return strings.ToUpper(strings.TrimSpace(text)) }

func (r *Renderer) drawHeader(img *image.RGBA, req FrameRequest)    {}
func (r *Renderer) drawPortraits(img *image.RGBA, req FrameRequest) {}
func (r *Renderer) drawDialogue(img *image.RGBA, req FrameRequest)  {}
