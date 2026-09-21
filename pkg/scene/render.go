package scene

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"
	"os"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	// Decoders register themselves, so raster art of these kinds can be read.
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

const (
	// textMargin is the share of the width kept clear on each side.
	textMargin = 0.08
	// maxTextLines bounds a frame's text so long prose cannot overflow it.
	maxTextLines = 8
	// crossfadeShare is the share of a scene's first beat spent blending in.
	crossfadeShare = 0.12
	// driftStart and driftEnd are the background scale at a beat's ends.
	driftStart = 1.02
	driftEnd   = 1.06
)

// baseColour matches the app and the player's background.
var baseColour = color.RGBA{12, 10, 9, 255}

// FrameRequest describes one frame to draw.
type FrameRequest struct {
	Scene       Scene
	Beat        Beat
	Progress    float64 // 0 at the beat's start, 1 at its end
	PreviousArt string  // the outgoing scene's art, for the crossfade
}

// Renderer draws video frames. Faces live as long as the renderer, which is what
// keeps parsing the bundled fonts to once per export.
type Renderer struct {
	width  int
	height int
	body   font.Face
	label  font.Face
}

// NewRenderer builds a frame renderer at the given size.
func NewRenderer(width, height int) (*Renderer, error) {
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse regular font: %w", err)
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse bold font: %w", err)
	}

	bodySize := float64(height) / 28
	body, err := opentype.NewFace(regular, &opentype.FaceOptions{Size: bodySize, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, fmt.Errorf("new body face: %w", err)
	}
	label, err := opentype.NewFace(bold, &opentype.FaceOptions{Size: bodySize * 0.55, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, fmt.Errorf("new label face: %w", err)
	}

	return &Renderer{width: width, height: height, body: body, label: label}, nil
}

// Frame draws one frame: the scene's imagery behind, the beat's text over it.
func (r *Renderer) Frame(req FrameRequest) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.width, r.height))
	draw.Draw(img, img.Bounds(), image.NewUniform(baseColour), image.Point{}, draw.Src)

	r.drawBackground(img, req)
	r.drawText(img, req)
	return img
}

// drawBackground paints the scene's imagery: decoded raster art when the file is
// one, otherwise a raster background drawn from the location's identity, since
// x/image cannot rasterise the SVG the built-in generator produces. A scene
// change blends in over the opening share of a beat, and the drift scale makes a
// held beat feel alive rather than frozen.
func (r *Renderer) drawBackground(img *image.RGBA, req FrameRequest) {
	scale := driftStart + (driftEnd-driftStart)*clamp01(req.Progress)

	if blend := crossfadeAlpha(req.Progress); blend < 1 && req.PreviousArt != "" {
		if previous, err := loadArt(req.PreviousArt); err == nil {
			drawCover(img, previous, scale)
			drawCoverAlpha(img, r.sceneArt(req), scale, blend)
			return
		}
	}
	drawCover(img, r.sceneArt(req), scale)
}

// sceneArt is the scene's decoded art, or a procedural background when the file
// is missing or not a raster image.
func (r *Renderer) sceneArt(req FrameRequest) image.Image {
	if art, err := loadArt(req.Scene.ArtPath); err == nil {
		return art
	}

	seed := req.Scene.LocationID
	if seed == "" {
		seed = req.Beat.ArtPath
	}
	return proceduralBackground(seed, r.width, r.height)
}

// crossfadeAlpha is how opaque the incoming scene is: it rises across the first
// share of a beat so a scene change reads as a transition, not a glitch.
func crossfadeAlpha(progress float64) float64 {
	if progress >= crossfadeShare {
		return 1
	}
	return clamp01(progress / crossfadeShare)
}

// loadArt decodes a raster scene image. SVG is deliberately rejected: the caller
// falls back to the procedural background rather than failing the frame.
func loadArt(path string) (image.Image, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("load art: no path")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("load art %q: %w", path, err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode art %q: %w", path, err)
	}
	return img, nil
}

// proceduralBackground draws a deterministic background from a seed, so a location
// looks the same every time it appears and different from every other location.
func proceduralBackground(seed string, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	hasher := fnv.New64a()
	hasher.Write([]byte(seed))
	h := hasher.Sum64()

	// A sky-to-ground gradient tinted by the seed.
	top := color.RGBA{
		R: uint8(18 + (h>>8)%40),
		G: uint8(16 + (h>>16)%32),
		B: uint8(28 + (h>>24)%48),
		A: 255,
	}
	bottom := color.RGBA{R: 8, G: 7, B: 6, A: 255}

	for y := 0; y < height; y++ {
		t := float64(y) / float64(max(1, height-1))
		row := color.RGBA{
			R: uint8(float64(top.R)*(1-t) + float64(bottom.R)*t),
			G: uint8(float64(top.G)*(1-t) + float64(bottom.G)*t),
			B: uint8(float64(top.B)*(1-t) + float64(bottom.B)*t),
			A: 255,
		}
		draw.Draw(img, image.Rect(0, y, width, y+1), image.NewUniform(row), image.Point{}, draw.Src)
	}

	// Ridgelines: three silhouettes whose heights come from the seed.
	rng := rand.New(rand.NewSource(int64(h)))
	ground := color.RGBA{R: 5, G: 5, B: 6, A: 255}
	for layer := 0; layer < 3; layer++ {
		baseline := height - (height/6)*(layer+1)
		amplitude := height / (6 + layer)
		phase := rng.Float64() * 6.28

		for x := 0; x < width; x++ {
			wave := math.Sin(float64(x)/float64(max(1, width))*6.28+phase) * float64(amplitude)
			ridge := int(float64(baseline) - wave)
			if ridge < 0 {
				ridge = 0
			}
			draw.Draw(img, image.Rect(x, ridge, x+1, height), image.NewUniform(ground), image.Point{}, draw.Src)
		}
	}

	return img
}

// drawCover scales src to cover dst, centred, so art of any aspect ratio fills the
// frame without distortion.
func drawCover(dst *image.RGBA, src image.Image, scale float64) {
	bounds := dst.Bounds()
	targetW := int(float64(bounds.Dx()) * scale)
	targetH := int(float64(bounds.Dy()) * scale)
	if targetW < 1 || targetH < 1 || src.Bounds().Dx() < 1 || src.Bounds().Dy() < 1 {
		return
	}

	scaled := scaleImage(src, targetW, targetH)
	offset := image.Pt((bounds.Dx()-targetW)/2, (bounds.Dy()-targetH)/2)
	draw.Draw(dst, image.Rect(offset.X, offset.Y, offset.X+targetW, offset.Y+targetH), scaled, image.Point{}, draw.Over)
}

// scaleImage resamples with nearest-neighbour, which is enough for a drifting
// background and keeps the export dependency-free.
func scaleImage(src image.Image, width, height int) *image.RGBA {
	srcBounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		sy := srcBounds.Min.Y + y*srcBounds.Dy()/height
		for x := 0; x < width; x++ {
			sx := srcBounds.Min.X + x*srcBounds.Dx()/width
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// drawCoverAlpha scales src to cover dst, centred, blended at alpha over whatever
// is already there.
func drawCoverAlpha(dst *image.RGBA, src image.Image, scale, alpha float64) {
	if alpha <= 0 {
		return
	}
	if alpha >= 1 {
		drawCover(dst, src, scale)
		return
	}

	bounds := dst.Bounds()
	targetW := int(float64(bounds.Dx()) * scale)
	targetH := int(float64(bounds.Dy()) * scale)
	if targetW < 1 || targetH < 1 || src.Bounds().Dx() < 1 || src.Bounds().Dy() < 1 {
		return
	}

	scaled := scaleImage(src, targetW, targetH)
	offset := image.Pt((bounds.Dx()-targetW)/2, (bounds.Dy()-targetH)/2)
	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			dstX, dstY := offset.X+x, offset.Y+y
			if !image.Pt(dstX, dstY).In(bounds) {
				continue
			}

			r16, g16, b16, _ := scaled.At(x, y).RGBA()
			existing := dst.RGBAAt(dstX, dstY)
			dst.SetRGBA(dstX, dstY, color.RGBA{
				R: uint8(float64(existing.R)*(1-alpha) + float64(r16>>8)*alpha),
				G: uint8(float64(existing.G)*(1-alpha) + float64(g16>>8)*alpha),
				B: uint8(float64(existing.B)*(1-alpha) + float64(b16>>8)*alpha),
				A: 255,
			})
		}
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// revealText shows the share of text a beat has reached, so the video's
// typewriter matches the player's.
func revealText(text string, progress float64) string {
	if progress >= 1 {
		return text
	}
	if progress <= 0 {
		return ""
	}

	runes := []rune(text)
	revealed := progress / TypewriterFraction
	if revealed > 1 {
		revealed = 1
	}

	count := int(float64(len(runes)) * revealed)
	if count > len(runes) {
		count = len(runes)
	}
	return string(runes[:count])
}

// wrapText breaks text into lines that fit width, capping the count and eliding
// the remainder rather than letting prose run off the frame.
func wrapText(face font.Face, text string, width, maxLines int) []string {
	words := strings.Fields(text)
	lines := make([]string, 0, maxLines)
	current := ""

	flush := func() {
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
	}

	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if font.MeasureString(face, candidate).Ceil() <= width {
			current = candidate
			continue
		}
		flush()
		current = word
	}
	flush()

	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = strings.TrimSpace(lines[maxLines-1]) + "…"
	}
	return lines
}

// drawText centres a beat's speaker label and its revealed text in the frame.
// A scene card is a title, so it borrows the accented label face and colour and
// skips the speaker and wrap machinery entirely.
func (r *Renderer) drawText(img *image.RGBA, req FrameRequest) {
	if req.Beat.Kind == BeatSceneCard {
		r.drawCentred(img, r.label, strings.ToUpper(req.Beat.Text), r.height/2, color.RGBA{245, 158, 11, 255})
		return
	}

	revealed := revealText(req.Beat.Text, req.Progress)
	wrapWidth := r.width - 2*int(float64(r.width)*textMargin)
	lines := wrapText(r.body, revealed, wrapWidth, maxTextLines)

	lineHeight := r.body.Metrics().Height.Ceil() + 8
	labelHeight := 0
	if req.Beat.Speaker != "" {
		labelHeight = r.label.Metrics().Height.Ceil() + 18
	}

	blockHeight := lineHeight * len(lines)
	baseline := (r.height-blockHeight-labelHeight)/2 + lineHeight

	if req.Beat.Speaker != "" {
		r.drawCentred(img, r.label, strings.ToUpper(req.Beat.Speaker), baseline-18, color.RGBA{245, 158, 11, 255})
	}
	for _, line := range lines {
		r.drawCentred(img, r.body, line, baseline, color.RGBA{231, 229, 228, 255})
		baseline += lineHeight
	}
}

func (r *Renderer) drawCentred(img *image.RGBA, face font.Face, text string, baseline int, col color.Color) {
	if strings.TrimSpace(text) == "" {
		return
	}

	width := font.MeasureString(face, text).Ceil()
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P((r.width-width)/2, baseline),
	}
	drawer.DrawString(text)
}
