package scene

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Colours are the theatre's, copied from the Tailwind classes the components use.
var (
	headerPurple   = color.RGBA{216, 180, 254, 255} // purple-300
	stoneText      = color.RGBA{231, 229, 228, 255} // stone-200
	stoneBright    = color.RGBA{250, 250, 249, 255} // stone-50
	panelFill      = color.RGBA{12, 10, 9, 230}     // stone-950/90
	amberLabel     = color.RGBA{252, 211, 77, 255}  // amber-300
	skyAccent      = color.RGBA{56, 189, 248, 255}  // sky-400
	purpleAccent   = color.RGBA{192, 132, 252, 255} // purple-400
	inactiveEdge   = color.RGBA{255, 255, 255, 76}  // white/30
	chipBackground = color.RGBA{0, 0, 0, 153}       // black/60
	panelBorder    = color.RGBA{255, 255, 255, 38}  // white/15
	stoneNameFill  = color.RGBA{68, 64, 60, 255}    // stone-700
	skyNameFill    = color.RGBA{2, 132, 199, 255}   // sky-600
	purpleNameFill = color.RGBA{147, 51, 234, 255}  // purple-600
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

func (r *Renderer) drawHeader(img *image.RGBA, req FrameRequest) {}

// drawPortraits paints the theatre's two square character boxes: the protagonist
// on the left for the whole story, and the current speaker on the right. The
// active speaker gets a coloured border and glow; a portrait is never dimmed.
func (r *Renderer) drawPortraits(img *image.RGBA, req FrameRequest) {
	playerPortrait := ""
	if req.Script != nil {
		playerPortrait = req.Script.PlayerPortrait
	}
	npcPortrait := ""
	npcLabel := ""
	if req.Beat.Kind == BeatSpeech && !req.Beat.Player {
		npcPortrait = req.Beat.PortraitPath
		npcLabel = req.Beat.Speaker
	}

	box := int(math.Min(float64(r.width)*0.24, float64(r.height)*0.34))
	box = max(box, 48)
	margin := int(float64(r.width) * 0.04)
	bandBottom := int(float64(r.height) * 0.70)
	top := bandBottom - box

	if playerPortrait != "" {
		label := ""
		if req.Script != nil {
			label = req.Script.PlayerName
		}
		r.drawPortraitBox(img, playerPortrait, margin, top, box, req.Beat.Player, skyAccent, upper(label), req.Beat.Player)
	}
	if npcPortrait != "" {
		right := r.width - margin - box
		r.drawPortraitBox(img, npcPortrait, right, top, box, true, purpleAccent, upper(npcLabel), true)
	}
}

// drawPortraitBox draws one rounded, bordered portrait with its label chip.
func (r *Renderer) drawPortraitBox(img *image.RGBA, path string, x, y, size int, active bool, accent color.RGBA, label string, mirror bool) {
	edge := inactiveEdge
	if active {
		edge = accent
	}
	rect := image.Rect(x, y, x+size, y+size)
	radius := size / 8

	if active {
		glowRoundRect(img, rect, radius, accent)
	}
	fillRoundRect(img, rect, radius, color.RGBA{28, 25, 23, 255})
	if art, err := r.art.load(path); err == nil {
		drawCoverRect(img, art, rect, mirror)
	}
	strokeRoundRect(img, rect, radius, 2, edge)

	if label != "" {
		r.drawChip(img, x+size/2, y+size+int(float64(r.height)*0.02), label, active, accent)
	}
}

func (r *Renderer) drawDialogue(img *image.RGBA, req FrameRequest) {}

// drawText draws a line, synthesising bold by drawing it twice a pixel apart
// because the embedded families are variable fonts x/image cannot instance.
func (r *Renderer) drawText(img *image.RGBA, face font.Face, text string, x, baseline int, col color.RGBA, bold bool) {
	if face == nil || strings.TrimSpace(text) == "" {
		return
	}
	drawer := &font.Drawer{Dst: img, Src: image.NewUniform(col), Face: face, Dot: fixed.P(x, baseline)}
	drawer.DrawString(text)
	if bold {
		drawer.Dot = fixed.P(x+1, baseline)
		drawer.DrawString(text)
	}
}

// drawChip draws a rounded label chip centred on cx, hanging below top.
func (r *Renderer) drawChip(img *image.RGBA, cx, top int, label string, active bool, accent color.RGBA) {
	face := r.face("sans", max(12, r.height/52))
	if face == nil {
		return
	}
	textWidth := font.MeasureString(face, label).Ceil()
	height := face.Metrics().Height.Ceil() + r.height/60
	pad := r.height / 80
	rect := image.Rect(cx-textWidth/2-pad, top, cx+textWidth/2+pad, top+height)

	fill := chipBackground
	if active {
		fill = accent
	}
	fillRoundRect(img, rect, height/2, fill)
	r.drawText(img, face, label, rect.Min.X+pad, rect.Min.Y+face.Metrics().Ascent.Ceil()+pad/2, color.RGBA{255, 255, 255, 255}, false)
}

// fillRoundRect fills a rounded rectangle.
func fillRoundRect(img *image.RGBA, rect image.Rectangle, radius int, fill color.RGBA) {
	drawRoundRect(img, rect, radius, func(x, y int) {
		img.SetRGBA(x, y, blendOver(img.RGBAAt(x, y), fill))
	})
}

// strokeRoundRect outlines a rounded rectangle with the given width.
func strokeRoundRect(img *image.RGBA, rect image.Rectangle, radius, width int, stroke color.RGBA) {
	for w := 0; w < width; w++ {
		outlineRoundRect(img, rect.Inset(w), radius, func(x, y int) {
			img.SetRGBA(x, y, blendOver(img.RGBAAt(x, y), stroke))
		})
	}
}

// glowRoundRect draws a soft coloured halo just outside a rounded rectangle.
func glowRoundRect(img *image.RGBA, rect image.Rectangle, radius int, accent color.RGBA) {
	const spread = 14
	for i := 0; i < spread; i++ {
		alpha := uint8(float64(90) * (1 - float64(i)/spread))
		tint := color.RGBA{accent.R, accent.G, accent.B, alpha}
		outlineRoundRect(img, rect.Inset(-i-1), radius+i, func(x, y int) {
			img.SetRGBA(x, y, blendOver(img.RGBAAt(x, y), tint))
		})
	}
}

// drawRoundRect visits the pixels inside a rounded rectangle.
func drawRoundRect(img *image.RGBA, rect image.Rectangle, radius int, visit func(x, y int)) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if !image.Pt(x, y).In(img.Bounds()) {
				continue
			}
			if insideRounded(rect, radius, x, y) {
				visit(x, y)
			}
		}
	}
}

// outlineRoundRect visits the one-pixel border of a rounded rectangle.
func outlineRoundRect(img *image.RGBA, rect image.Rectangle, radius int, visit func(x, y int)) {
	drawRoundRect(img, rect, radius, func(x, y int) {
		if !insideRounded(rect.Inset(1), radius-1, x, y) {
			visit(x, y)
		}
	})
}

// insideRounded reports whether a point is inside a rounded rectangle, rounding
// the four corners with a quarter circle of the given radius.
func insideRounded(rect image.Rectangle, radius, x, y int) bool {
	if !image.Pt(x, y).In(rect) {
		return false
	}
	if radius <= 0 {
		return true
	}

	dx := 0
	switch {
	case x < rect.Min.X+radius:
		dx = rect.Min.X + radius - x
	case x > rect.Max.X-1-radius:
		dx = x - (rect.Max.X - 1 - radius)
	}
	dy := 0
	switch {
	case y < rect.Min.Y+radius:
		dy = rect.Min.Y + radius - y
	case y > rect.Max.Y-1-radius:
		dy = y - (rect.Max.Y - 1 - radius)
	}
	if dx == 0 || dy == 0 {
		return true
	}
	return dx*dx+dy*dy <= radius*radius
}

// blendOver composites src over dst by its alpha.
func blendOver(dst, src color.RGBA) color.RGBA {
	a := float64(src.A) / 255
	return color.RGBA{
		R: uint8(float64(dst.R)*(1-a) + float64(src.R)*a),
		G: uint8(float64(dst.G)*(1-a) + float64(src.G)*a),
		B: uint8(float64(dst.B)*(1-a) + float64(src.B)*a),
		A: 255,
	}
}

// drawCoverRect cover-fits art into a rectangle, optionally mirrored.
func drawCoverRect(img *image.RGBA, art image.Image, rect image.Rectangle, mirror bool) {
	scaled := scaleToCover(art, rect.Dx(), rect.Dy())
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			sx := x
			if mirror {
				sx = rect.Dx() - 1 - x
			}
			if !image.Pt(rect.Min.X+x, rect.Min.Y+y).In(img.Bounds()) {
				continue
			}
			img.Set(rect.Min.X+x, rect.Min.Y+y, scaled.At(sx, y))
		}
	}
}

// scaleToCover resamples src to fill width×height, cropping the overflow.
func scaleToCover(src image.Image, width, height int) *image.RGBA {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW < 1 || srcH < 1 {
		return image.NewRGBA(image.Rect(0, 0, width, height))
	}

	scale := math.Max(float64(width)/float64(srcW), float64(height)/float64(srcH))
	targetW := int(float64(srcW) * scale)
	targetH := int(float64(srcH) * scale)
	resized := scaleImage(src, max(1, targetW), max(1, targetH))

	out := image.NewRGBA(image.Rect(0, 0, width, height))
	offX := (resized.Bounds().Dx() - width) / 2
	offY := (resized.Bounds().Dy() - height) / 2
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			out.Set(x, y, resized.At(x+offX, y+offY))
		}
	}
	return out
}
