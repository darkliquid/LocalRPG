package scene

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"time"

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
		draw.Draw(img, img.Bounds(), r.gradientImage(), image.Point{}, draw.Src)
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

// gradientImage is the theatre's no-art background, built once per renderer
// because it depends only on the frame size.
func (r *Renderer) gradientImage() *image.RGBA {
	if r.gradient == nil {
		gradient := image.NewRGBA(image.Rect(0, 0, r.width, r.height))
		drawRadialGradient(gradient, color.RGBA{38, 30, 27, 255}, baseColour)
		r.gradient = gradient
	}
	return r.gradient
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

// FrameStep is one frame of a beat: the reveal progress to draw and how long the
// frame is held.
type FrameStep struct {
	Progress float64
	Span     time.Duration
}

// holdFPS is the heartbeat rate for the tail of a beat. Once the reveal has
// finished the picture barely changes, so a slow cadence keeps the background
// drift alive without paying for frames nobody can tell apart.
const holdFPS = 2

// BeatFramePlan is the frames a beat occupies: one when animation is off, and a
// fast reveal followed by a slow heartbeat when it is on. The spans sum to the
// beat's duration, so picture and sound stay in step.
func BeatFramePlan(beat Beat, fps int, animate bool) []FrameStep {
	if !animate {
		return []FrameStep{{Progress: 1, Span: beat.Duration}}
	}
	if beat.Duration <= 0 {
		return []FrameStep{{Progress: 1}}
	}

	revealSpan := time.Duration(float64(beat.Duration) * TypewriterFraction)
	holdSpan := beat.Duration - revealSpan

	revealFrames := FramesFor(revealSpan, fps)
	holdFrames := FramesFor(holdSpan, holdFPS)

	steps := make([]FrameStep, 0, revealFrames+holdFrames)
	revealStep := revealSpan / time.Duration(revealFrames)
	for i := 0; i < revealFrames; i++ {
		progress := TypewriterFraction
		if revealFrames > 1 {
			progress = float64(i) / float64(revealFrames-1) * TypewriterFraction
		}
		span := revealStep
		if i == revealFrames-1 {
			span = revealSpan - revealStep*time.Duration(revealFrames-1)
		}
		steps = append(steps, FrameStep{Progress: progress, Span: span})
	}

	holdStep := holdSpan / time.Duration(holdFrames)
	for i := 0; i < holdFrames; i++ {
		progress := 1.0
		if holdFrames > 1 {
			progress = TypewriterFraction + float64(i)/float64(holdFrames-1)*(1-TypewriterFraction)
		}
		span := holdStep
		if i == holdFrames-1 {
			span = holdSpan - holdStep*time.Duration(holdFrames-1)
		}
		steps = append(steps, FrameStep{Progress: progress, Span: span})
	}
	return steps
}

// drawHeader is the theatre's top band: the campaign's name, the location pill,
// and the scene counter.
func (r *Renderer) drawHeader(img *image.RGBA, req FrameRequest) {
	if req.Script == nil {
		return
	}
	face := r.face("sans", max(14, r.height/40))
	if face == nil {
		return
	}
	top := int(float64(r.height) * 0.03)
	r.drawText(img, face, upper(req.Script.GameName), int(float64(r.width)*0.03), top+face.Metrics().Ascent.Ceil(), headerPurple, true)

	x := int(float64(r.width) * 0.03)
	y := top + face.Metrics().Height.Ceil() + r.height/60
	if req.Scene.LocationName != "" {
		x = r.drawPill(img, face, req.Scene.LocationName, x, y, chipBackground, stoneText) + r.height/80
	}
	if len(req.Script.Scenes) > 0 {
		counter := fmt.Sprintf("Scene %d of %d", req.SceneIndex+1, len(req.Script.Scenes))
		r.drawPill(img, r.face("mono", max(12, r.height/56)), counter, x, y, chipBackground, stoneText)
	}
}

// drawPill draws a rounded chip and returns the x it ended at.
func (r *Renderer) drawPill(img *image.RGBA, face font.Face, label string, x, top int, fill, text color.RGBA) int {
	if face == nil {
		return x
	}
	padding := r.height / 60
	textWidth := font.MeasureString(face, label).Ceil()
	height := face.Metrics().Height.Ceil() + padding
	rect := image.Rect(x, top, x+textWidth+2*padding, top+height)
	fillRoundRect(img, rect, height/3, fill)
	r.drawText(img, face, label, rect.Min.X+padding, rect.Min.Y+face.Metrics().Ascent.Ceil()+padding/2, text, false)
	return rect.Max.X
}

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

// drawDialogue draws the bottom panel: a scene card is a centred amber title,
// anything else is the name plate, the prose, and the advance caret.
func (r *Renderer) drawDialogue(img *image.RGBA, req FrameRequest) {
	panel := r.dialogueRect()
	if req.Beat.Kind == BeatSceneCard {
		r.drawSceneCard(img, panel, req)
		return
	}

	shown := req.Beat.Text
	if req.Animate {
		shown = revealText(shown, req.Progress)
	}

	isPlayer := req.Beat.Kind == BeatSpeech && req.Beat.Player
	isSpeech := req.Beat.Kind == BeatSpeech
	edge := panelBorder
	name := "Narrator"
	nameFill := stoneNameFill
	switch {
	case isPlayer:
		edge, nameFill = skyAccent, skyNameFill
	case isSpeech:
		edge, nameFill = purpleAccent, purpleNameFill
	}
	if isSpeech {
		if strings.TrimSpace(req.Beat.Speaker) != "" {
			name = req.Beat.Speaker
		} else {
			name = "Unknown"
		}
	}

	radius := panel.Dy() / 10
	fillRoundRect(img, panel, radius, panelFill)
	strokeRoundRect(img, panel, radius, 2, edge)
	r.drawNamePlate(img, panel, name, nameFill)
	r.drawProse(img, panel, shown, isSpeech, req.DisplayMode)

	if !req.Animate || req.Progress >= 1 {
		caretX := panel.Max.X - panel.Dy()/8
		caretY := panel.Max.Y - panel.Dy()/10
		drawDiamond(img, caretX, caretY, 5, color.RGBA{216, 180, 254, 200})
	}
}

// dialogueRect is the theatre's panel: max-w-4xl, centred, min-h 20vh, above the
// bottom edge.
func (r *Renderer) dialogueRect() image.Rectangle {
	width := int(math.Min(float64(r.width)*0.72, float64(r.height)*1.15))
	height := int(math.Max(float64(r.height)*0.20, float64(r.height)*0.22))
	left := (r.width - width) / 2
	bottom := int(float64(r.height) * 0.94)
	return image.Rect(left, bottom-height, left+width, bottom)
}

// drawNamePlate draws the panel's name chip, straddling the top edge.
func (r *Renderer) drawNamePlate(img *image.RGBA, panel image.Rectangle, name string, fill color.RGBA) {
	face := r.face("sans", max(13, r.height/44))
	if face == nil {
		return
	}
	padding := r.height / 90
	textWidth := font.MeasureString(face, name).Ceil()
	height := face.Metrics().Height.Ceil() + padding
	rect := image.Rect(panel.Min.X+panel.Dy()/12, panel.Min.Y-height/2, panel.Min.X+panel.Dy()/12+textWidth+2*padding, panel.Min.Y+height/2)
	fillRoundRect(img, rect, height/4, fill)
	r.drawText(img, face, name, rect.Min.X+padding, rect.Min.Y+face.Metrics().Ascent.Ceil()+padding/2, color.RGBA{255, 255, 255, 255}, true)
}

// drawProse lays out the beat's prose inside the panel, applying the inline
// grammar the exported page applies.
func (r *Renderer) drawProse(img *image.RGBA, panel image.Rectangle, text string, speech bool, mode DisplayMode) {
	body := text
	if speech {
		body = "\u201c" + text + "\u201d"
	}
	blocks := ParseProse(body, mode)
	if len(blocks) == 0 {
		return
	}

	size := max(16, r.height/26)
	base := r.face("serif", size)
	italic := r.face("serif-italic", size)
	if speech {
		base = italic
	}
	mono := r.face("mono", max(14, r.height/30))
	sans := r.face("sans", max(13, r.height/34))
	if base == nil {
		return
	}

	padding := panel.Dy() / 8
	left := panel.Min.X + padding
	width := panel.Dx() - 2*padding
	lineHeight := base.Metrics().Height.Ceil() + r.height/120
	y := panel.Min.Y + padding + base.Metrics().Ascent.Ceil()

	for _, block := range blocks {
		if block.Kind == BlockRule {
			draw.Draw(img, image.Rect(left, y, left+width, y+1), image.NewUniform(inactiveEdge), image.Point{}, draw.Over)
			y += lineHeight
			continue
		}
		for _, line := range wrapRuns(block.Runs, base, width) {
			x := left
			for _, run := range line {
				face := r.runFace(run.Style, base, italic, mono, sans)
				col := runColour(run.Style, stoneText)
				if speech && run.Style == StyleRegular {
					col = stoneBright
				}
				r.drawText(img, face, run.Text, x, y, col, run.Style == StyleBold)
				x += font.MeasureString(face, run.Text).Ceil()
			}
			y += lineHeight
			if y > panel.Max.Y-padding {
				return
			}
		}
	}
}

// runFace picks the face a run draws with.
func (r *Renderer) runFace(style RunStyle, base, italic, mono, sans font.Face) font.Face {
	switch style {
	case StyleCode:
		return mono
	case StyleDirection:
		return sans
	case StyleItalic:
		return italic
	default:
		return base
	}
}

func runColour(style RunStyle, body color.RGBA) color.RGBA {
	switch style {
	case StyleLink:
		return purpleAccent
	case StyleDirection:
		return color.RGBA{192, 132, 252, 230}
	case StyleCode:
		return stoneText
	case StyleBold:
		return stoneBright
	default:
		return body
	}
}

// drawSceneCard centres the location name in amber, the theatre's title beat.
func (r *Renderer) drawSceneCard(img *image.RGBA, panel image.Rectangle, req FrameRequest) {
	face := r.face("serif", max(22, r.height/16))
	if face == nil {
		return
	}
	shown := upper(req.Beat.Text)
	if req.Animate {
		shown = upper(revealText(req.Beat.Text, req.Progress))
	}
	textWidth := font.MeasureString(face, shown).Ceil()
	x := (r.width - textWidth) / 2
	baseline := panel.Min.Y + panel.Dy()/2 + face.Metrics().Ascent.Ceil()/2
	r.drawText(img, face, shown, x, baseline, amberLabel, true)
}

// drawDiamond draws the small advance caret as a rotated square.
func drawDiamond(img *image.RGBA, cx, cy, radius int, fill color.RGBA) {
	for dy := -radius; dy <= radius; dy++ {
		span := radius - abs(dy)
		for dx := -span; dx <= span; dx++ {
			x, y := cx+dx, cy+dy
			if image.Pt(x, y).In(img.Bounds()) {
				img.SetRGBA(x, y, blendOver(img.RGBAAt(x, y), fill))
			}
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// wrapRuns breaks a block's runs into lines that fit width, preserving styles.
func wrapRuns(runs []Run, face font.Face, width int) [][]Run {
	var lines [][]Run
	current := []Run{}
	currentWidth := 0

	flush := func() {
		if len(current) > 0 {
			lines = append(lines, current)
			current = []Run{}
			currentWidth = 0
		}
	}

	for _, run := range runs {
		words := strings.SplitAfter(run.Text, " ")
		for _, word := range words {
			if word == "" {
				continue
			}
			wordWidth := font.MeasureString(face, word).Ceil()
			if currentWidth > 0 && currentWidth+wordWidth > width {
				flush()
			}
			if len(current) > 0 && current[len(current)-1].Style == run.Style {
				current[len(current)-1].Text += word
			} else {
				current = append(current, Run{Text: word, Style: run.Style})
			}
			currentWidth += wordWidth
		}
	}
	flush()
	return lines
}

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
