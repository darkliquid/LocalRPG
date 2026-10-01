package scene

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"

	"github.com/darkliquid/localrpg/pkg/scene/fonts"
)

const (
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
	Script      *Script
	SceneIndex  int
	Scene       Scene
	Beat        Beat
	Progress    float64 // 0 at the beat's start, 1 at its end
	PreviousArt string  // the outgoing scene's art, for the crossfade
	Animate     bool
	DisplayMode DisplayMode
}

// Renderer draws the story theatre's own stage into video frames. Faces live as
// long as the renderer, which keeps parsing the embedded fonts to once per export.
type Renderer struct {
	width  int
	height int
	fonts  *fonts.Set
	art    *artCache
	faces  map[faceKey]font.Face
}

// faceKey names a sized face so it is parsed once per renderer.
type faceKey struct {
	family string
	size   int
}

// NewRenderer builds a frame renderer at the given size.
func NewRenderer(width, height int) (*Renderer, error) {
	set, err := fonts.Load()
	if err != nil {
		return nil, fmt.Errorf("load fonts: %w", err)
	}
	return &Renderer{
		width:  width,
		height: height,
		fonts:  set,
		art:    newArtCache(),
		faces:  map[faceKey]font.Face{},
	}, nil
}

// Frame draws one frame: the stage behind, the beat's dialogue over it.
func (r *Renderer) Frame(req FrameRequest) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.width, r.height))
	draw.Draw(img, img.Bounds(), image.NewUniform(baseColour), image.Point{}, draw.Src)

	r.drawBackground(img, req)
	r.drawScrim(img)
	r.drawHeader(img, req)
	r.drawPortraits(img, req)
	r.drawDialogue(img, req)
	return img
}

// face returns a sized face for a family, parsing it once per renderer.
func (r *Renderer) face(family string, size int) font.Face {
	key := faceKey{family: family, size: size}
	if cached, ok := r.faces[key]; ok {
		return cached
	}
	parsed := r.familyFont(family)
	if parsed == nil {
		return nil
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	r.faces[key] = face
	return face
}

// familyFont resolves a family name to an embedded typeface.
func (r *Renderer) familyFont(family string) *opentype.Font {
	switch family {
	case "serif-italic":
		return r.fonts.SerifItalic
	case "sans":
		return r.fonts.Sans
	case "sans-italic":
		return r.fonts.SansItalic
	case "mono":
		return r.fonts.Mono
	default:
		return r.fonts.Serif
	}
}

// clamp01 bounds v to [0, 1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// crossfadeAlpha is how opaque the incoming scene is: it rises across the first
// share of a beat so a scene change reads as a transition, not a glitch.
func crossfadeAlpha(progress float64) float64 {
	if progress >= crossfadeShare {
		return 1
	}
	return clamp01(progress / crossfadeShare)
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
