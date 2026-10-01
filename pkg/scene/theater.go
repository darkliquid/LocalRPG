package scene

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	xdraw "golang.org/x/image/draw"
)

// Colours are the theatre's, copied from the Tailwind classes the components use.
var (
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
// the campaign banner, cover-fit and scaled once. A scene change blends in over
// the opening share of a beat. With no art at all the stage shows the theatre's
// own radial gradient.
//
// The background does not move. A drifting, re-scaled background shimmers as
// whole source pixels jump, and that high-frequency noise is exactly what an
// inter frame predicts badly, so the picture crawls and smears over a beat.
func (r *Renderer) drawBackground(img *image.RGBA, req FrameRequest) {
	current := r.backgroundArt(req)
	if current == nil {
		draw.Draw(img, img.Bounds(), r.gradientImage(), image.Point{}, draw.Src)
		return
	}

	if blend := crossfadeAlpha(req.Progress); blend < 1 && req.PreviousArt != "" {
		if previous := r.art.cover(req.PreviousArt, r.width, r.height); previous != nil {
			draw.Draw(img, img.Bounds(), previous, image.Point{}, draw.Src)
			blendImage(img, current, blend)
			return
		}
	}
	draw.Draw(img, img.Bounds(), current, image.Point{}, draw.Src)
}

// backgroundArt is the first art the theatre would show, cover-fit: the beat's
// own, then the scene's, then the campaign's banner.
func (r *Renderer) backgroundArt(req FrameRequest) *image.RGBA {
	for _, path := range []string{req.Beat.ArtPath, req.Scene.ArtPath} {
		if covered := r.art.cover(path, r.width, r.height); covered != nil {
			return covered
		}
	}
	if req.Script != nil {
		if covered := r.art.cover(req.Script.Banner, r.width, r.height); covered != nil {
			return covered
		}
	}
	return nil
}

// blendImage composites src over dst at the given alpha. Both are the same size,
// which is what the cached backgrounds are.
func blendImage(dst, src *image.RGBA, alpha float64) {
	a := clamp01(alpha)
	if a <= 0 {
		return
	}
	for y := 0; y < dst.Rect.Dy(); y++ {
		dstRow := dst.Pix[y*dst.Stride : y*dst.Stride+dst.Rect.Dx()*4]
		srcRow := src.Pix[y*src.Stride : y*src.Stride+src.Rect.Dx()*4]
		for x := 0; x < dst.Rect.Dx(); x++ {
			i := x * 4
			dstRow[i] = uint8(float64(dstRow[i])*(1-a) + float64(srcRow[i])*a)
			dstRow[i+1] = uint8(float64(dstRow[i+1])*(1-a) + float64(srcRow[i+1])*a)
			dstRow[i+2] = uint8(float64(dstRow[i+2])*(1-a) + float64(srcRow[i+2])*a)
			dstRow[i+3] = 255
		}
	}
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
// frame is held. Repeat marks a frame that shows the same picture as the one
// before it, which a renderer can reuse rather than draw again.
type FrameStep struct {
	Progress float64
	Span     time.Duration
	Repeat   bool
}

// holdFPS is the heartbeat rate for the tail of a beat. Once the reveal has
// finished the picture barely changes, so a slow cadence keeps the beat alive
// without drawing frames nobody can tell apart.
const holdFPS = 2

// BeatFramePlan is the frames a beat occupies: one when animation is off, and a
// fast reveal followed by a slow heartbeat when it is on. The spans sum to the
// beat's duration, so picture and sound stay in step. Every heartbeat frame
// repeats the last revealed one.
func BeatFramePlan(beat Beat, fps int, animate bool) []FrameStep {
	if !animate {
		return []FrameStep{{Progress: 1, Span: beat.Duration}}
	}
	if beat.Duration <= 0 {
		return []FrameStep{{Progress: 1}}
	}

	// When a beat has a clip, that clip is the narration of this very text, so the
	// reveal runs with it rather than finishing two fifths of the way in and
	// leaving the words sitting there while the voice catches up.
	revealSpan := time.Duration(float64(beat.Duration) * TypewriterFraction)
	if beat.AudioDuration > 0 && beat.AudioDuration < beat.Duration {
		revealSpan = beat.AudioDuration
	}
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
		steps = append(steps, FrameStep{Progress: progress, Span: span, Repeat: true})
	}
	return steps
}

// FramePlan is how many frames a script's render will produce, and how many of
// them are new images rather than a repeat of the frame before. It is known
// before rendering starts, which is what lets a progress bar be exact.
type FramePlan struct {
	Duration time.Duration
	Total    int
	Image    int
	Repeat   int
}

// PlanFrames counts a script's frames without drawing any of them.
func PlanFrames(script *Script, fps int, animate bool) FramePlan {
	var plan FramePlan
	if script == nil {
		return plan
	}
	for _, beat := range script.Beats() {
		steps := BeatFramePlan(beat, fps, animate)
		plan.Total += len(steps)
		for _, step := range steps {
			if step.Repeat {
				plan.Repeat++
			} else {
				plan.Image++
			}
			plan.Duration += step.Span
		}
	}
	return plan
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
	// The band ends well above the dialogue panel, which the theatre anchors near
	// the bottom: a portrait that reaches into it would sit under the narration.
	bandBottom := int(float64(r.height) * 0.58)
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
	if covered := r.art.cover(path, size, size); covered != nil {
		drawCoverRect(img, covered, rect, mirror)
	}
	strokeRoundRect(img, rect, radius, 2, edge)

	if label != "" {
		r.drawChip(img, x+size/2, y+size+int(float64(r.height)*0.02), label, active, accent)
	}
}

// drawDialogue draws the bottom panel: a scene card is a centred amber title,
// anything else is the name plate, the prose, and the advance caret. The panel is
// sized from the laid-out prose, so a long beat makes a taller panel rather than
// running past the bottom of a fixed one.
func (r *Renderer) drawDialogue(img *image.RGBA, req FrameRequest) {
	if req.Beat.Kind == BeatSceneCard {
		r.drawSceneCard(img, r.panelRect(r.minPanelHeight()), req)
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

	layout := r.layoutProse(shown, isSpeech, req.DisplayMode, r.panelWidth())
	panel := r.panelRect(clampInt(layout.height(), r.minPanelHeight(), r.maxPanelHeight()))

	radius := panel.Dy() / 10
	fillRoundRect(img, panel, radius, panelFill)
	strokeRoundRect(img, panel, radius, 2, edge)
	r.drawNamePlate(img, panel, name, nameFill)
	r.drawProse(img, panel, layout, isSpeech)

	if !req.Animate || req.Progress >= 1 {
		caretX := panel.Max.X - panel.Dy()/8
		caretY := panel.Max.Y - panel.Dy()/10
		drawDiamond(img, caretX, caretY, 5, color.RGBA{216, 180, 254, 200})
	}
}

// panelWidth is the dialogue panel's width, which does not depend on its height.
func (r *Renderer) panelWidth() int {
	return int(math.Min(float64(r.width)*0.72, float64(r.height)*1.15))
}

// panelRect places a panel of the given height above the bottom edge.
func (r *Renderer) panelRect(height int) image.Rectangle {
	width := r.panelWidth()
	left := (r.width - width) / 2
	bottom := int(float64(r.height) * 0.94)
	return image.Rect(left, bottom-height, left+width, bottom)
}

// minPanelHeight is the theatre's min-h of about a fifth of the frame, so a short
// line still reads as a panel rather than a strip.
func (r *Renderer) minPanelHeight() int { return int(float64(r.height) * 0.20) }

// maxPanelHeight bounds the panel so it cannot swallow the portraits, which is
// where the prose starts shrinking instead.
func (r *Renderer) maxPanelHeight() int { return int(float64(r.height) * 0.46) }

func clampInt(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
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

// proseLayout is laid-out prose and the metrics needed to draw it and to size the
// panel that holds it.
type proseLayout struct {
	lines      [][]Run // a nil line is a horizontal rule
	base       font.Face
	italic     font.Face
	mono       font.Face
	sans       font.Face
	lineHeight int
	padding    int
	ascent     int
}

// height is how tall a panel must be to hold the text.
func (l proseLayout) height() int { return len(l.lines)*l.lineHeight + 2*l.padding }

// layoutProse wraps a beat's prose at the panel's width. It steps the type down
// until the result fits the tallest panel the stage allows, so a long beat is not
// silently cut off at the bottom of a fixed box.
func (r *Renderer) layoutProse(text string, speech bool, mode DisplayMode, panelWidth int) proseLayout {
	body := text
	if speech {
		body = "\u201c" + text + "\u201d"
	}
	blocks := ParseProse(body, mode)
	if len(blocks) == 0 {
		return proseLayout{}
	}

	size := max(13, r.height/45)
	maxHeight := r.maxPanelHeight()
	for ; size > 11; size -= 2 {
		if layout := r.proseAt(blocks, speech, size, panelWidth); layout.height() <= maxHeight {
			return layout
		}
	}
	return r.proseAt(blocks, speech, size, panelWidth)
}

// proseAt lays the blocks out at one type size.
func (r *Renderer) proseAt(blocks []Block, speech bool, size, panelWidth int) proseLayout {
	base := r.face("serif", size)
	italic := r.face("serif-italic", size)
	if speech {
		base = italic
	}
	if base == nil {
		return proseLayout{}
	}

	padding := size * 2
	inner := max(1, panelWidth-2*padding)

	lines := make([][]Run, 0, 8)
	for _, block := range blocks {
		if block.Kind == BlockRule {
			lines = append(lines, nil)
			continue
		}
		lines = append(lines, wrapRuns(block.Runs, base, inner)...)
	}

	return proseLayout{
		lines:      lines,
		base:       base,
		italic:     italic,
		mono:       r.face("mono", size),
		sans:       r.face("sans", size),
		lineHeight: base.Metrics().Height.Ceil() + size/3,
		padding:    padding,
		ascent:     base.Metrics().Ascent.Ceil(),
	}
}

// drawProse draws laid-out prose inside the panel.
func (r *Renderer) drawProse(img *image.RGBA, panel image.Rectangle, layout proseLayout, speech bool) {
	if layout.base == nil {
		return
	}

	left := panel.Min.X + layout.padding
	right := panel.Max.X - layout.padding
	y := panel.Min.Y + layout.padding + layout.ascent

	for _, line := range layout.lines {
		if line == nil {
			draw.Draw(img, image.Rect(left, y, right, y+1), image.NewUniform(inactiveEdge), image.Point{}, draw.Over)
			y += layout.lineHeight
			continue
		}
		x := left
		for _, run := range line {
			face := r.runFace(run.Style, layout.base, layout.italic, layout.mono, layout.sans)
			col := runColour(run.Style, stoneText)
			if speech && run.Style == StyleRegular {
				col = stoneBright
			}
			r.drawText(img, face, run.Text, x, y, col, run.Style == StyleBold)
			x += font.MeasureString(face, run.Text).Ceil()
		}
		y += layout.lineHeight
		if y > panel.Max.Y-layout.padding {
			return
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
	face := r.face("serif", max(18, r.height/30))
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
	// The theatre's scene card is not bold, and faking bold on letters this large
	// puts a coloured fringe on every stroke.
	r.drawText(img, face, shown, x, baseline, amberLabel, false)
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

// drawText draws a line, synthesising bold by drawing it a half pixel to the
// right as well: the embedded families are variable fonts that x/image cannot
// instance, and a whole-pixel double draw leaves a coloured fringe on the edges.
func (r *Renderer) drawText(img *image.RGBA, face font.Face, text string, x, baseline int, col color.RGBA, bold bool) {
	if face == nil || strings.TrimSpace(text) == "" {
		return
	}
	dot := fixed.P(x, baseline)
	drawer := &font.Drawer{Dst: img, Src: image.NewUniform(col), Face: face, Dot: dot}
	drawer.DrawString(text)
	if bold {
		drawer.Dot = fixed.Point26_6{X: dot.X + 32, Y: dot.Y}
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

// drawCoverRect copies already-cover-fit art into a rectangle, optionally
// mirrored. The art is scaled once by the cache, so this is a copy.
func drawCoverRect(img *image.RGBA, art *image.RGBA, rect image.Rectangle, mirror bool) {
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			sx := x
			if mirror {
				sx = rect.Dx() - 1 - x
			}
			if !image.Pt(rect.Min.X+x, rect.Min.Y+y).In(img.Bounds()) {
				continue
			}
			img.Set(rect.Min.X+x, rect.Min.Y+y, art.At(sx, y))
		}
	}
}

// scaleToCover resamples src to fill width×height, cropping the overflow. It runs
// once per image rather than per frame, so it uses the smooth scaler: nearest
// neighbour makes a scaled portrait fringe with colour.
func scaleToCover(src image.Image, width, height int) *image.RGBA {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW < 1 || srcH < 1 || width < 1 || height < 1 {
		return image.NewRGBA(image.Rect(0, 0, max(1, width), max(1, height)))
	}

	scale := math.Max(float64(width)/float64(srcW), float64(height)/float64(srcH))
	targetW := max(1, int(float64(srcW)*scale))
	targetH := max(1, int(float64(srcH)*scale))

	resized := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.ApproxBiLinear.Scale(resized, resized.Bounds(), src, bounds, xdraw.Src, nil)

	out := image.NewRGBA(image.Rect(0, 0, width, height))
	offX := (targetW - width) / 2
	offY := (targetH - height) / 2
	for y := 0; y < height; y++ {
		copy(
			out.Pix[y*out.Stride:y*out.Stride+width*4],
			resized.Pix[(y+offY)*resized.Stride+offX*4:(y+offY)*resized.Stride+(offX+width)*4],
		)
	}
	return out
}
