package scene

import (
	"image"
	"image/color"
	"math"
	"strings"
)

// Theatre effects. Every effect is a pure function of a beat's progress, so the
// renderer and the app (frontend/src/lib/effects.ts) compute the same values from
// the same inputs. Keep the two files in step.

// Effect magnitudes. They are deliberately small: a Ken Burns that is noticed is
// too strong.
const (
	// KenBurnsZoom is the extra scale at the end of a beat.
	KenBurnsZoom = 0.08
	// KenBurnsPan is the maximum translation, as a fraction of the frame.
	KenBurnsPan = 0.035
	// ParallaxPan is the extra translation the nearest layer receives at full
	// progress; a layer's own depth scales it.
	ParallaxPan = 0.05
)

// KenBurnsTransform is a still's slow zoom and pan.
type KenBurnsTransform struct {
	Scale float64
	DX    float64
	DY    float64
}

// KenBurns is a slow zoom with a small pan across a still, scaled to the beat's
// progress. Progress zero is the identity, so a beat starts on its own picture.
// The pan direction alternates by seed so consecutive beats do not all drift the
// same way.
func KenBurns(seed int, progress float64) KenBurnsTransform {
	t := clamp01(progress)
	if t <= 0 {
		return KenBurnsTransform{Scale: 1}
	}
	direction := 1.0
	if seed%2 == 0 {
		direction = -1
	}
	vertical := -1.0
	if (seed/2)%2 != 0 {
		vertical = 1
	}
	return KenBurnsTransform{
		Scale: 1 + KenBurnsZoom*t,
		DX:    direction * KenBurnsPan * t,
		DY:    vertical * KenBurnsPan * t,
	}
}

// ParallaxOffset is the translation a layer at depth receives for a beat's
// progress: the background (depth 0) does not move, the foreground (depth 1)
// moves most.
func ParallaxOffset(depth, progress float64) float64 {
	return clamp01(depth) * clamp01(progress) * ParallaxPan
}

// Tint is a low-opacity colour overlay drawn over the picture and under the scrim.
type Tint struct {
	Colour  color.RGBA
	Opacity float64
}

// The mood tints, mirroring the tone IMG-1 gives the scene prompt. Success reads
// warm and brighter, a failure reads cool and darker, and an unknown or neutral
// outcome is no tint at all.
var (
	tintWarm    = color.RGBA{245, 158, 11, 255}  // amber-500
	tintCool    = color.RGBA{56, 189, 248, 255}  // sky-400
	tintNeutral = color.RGBA{168, 162, 158, 255} // stone-400
)

// MoodTint maps a turn's outcome to a tint.
func MoodTint(outcome string) Tint {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "success":
		return Tint{Colour: tintWarm, Opacity: 0.10}
	case "strong":
		return Tint{Colour: tintWarm, Opacity: 0.12}
	case "weak", "partial":
		return Tint{Colour: tintNeutral}
	case "miss", "fail", "failure":
		return Tint{Colour: tintCool, Opacity: 0.14}
	default:
		return Tint{Colour: tintNeutral}
	}
}

// WeatherKind is a scene's weather overlay.
type WeatherKind string

const (
	// WeatherNone draws no overlay.
	WeatherNone WeatherKind = ""
	// WeatherRain draws angled streaks.
	WeatherRain WeatherKind = "rain"
	// WeatherSnow draws drifting dots.
	WeatherSnow WeatherKind = "snow"
	// WeatherFog draws a soft horizontal band.
	WeatherFog WeatherKind = "fog"
)

// WeatherOverlay maps a scene's weather to its overlay, or WeatherNone.
func WeatherOverlay(weather string) WeatherKind {
	switch strings.ToLower(strings.TrimSpace(weather)) {
	case "rain":
		return WeatherRain
	case "snow":
		return WeatherSnow
	case "fog":
		return WeatherFog
	default:
		return WeatherNone
	}
}

// blendPixel alpha-blends a colour onto img at (x, y).
func blendPixel(img *image.RGBA, x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= img.Rect.Dx() || y >= img.Rect.Dy() || c.A == 0 {
		return
	}
	i := img.PixOffset(x, y)
	a := float64(c.A) / 255
	img.Pix[i] = uint8(float64(img.Pix[i])*(1-a) + float64(c.R)*a)
	img.Pix[i+1] = uint8(float64(img.Pix[i+1])*(1-a) + float64(c.G)*a)
	img.Pix[i+2] = uint8(float64(img.Pix[i+2])*(1-a) + float64(c.B)*a)
	img.Pix[i+3] = 255
}

// fogProfile is the vertical weight of the fog band: zero at the edges, one in
// the middle, so the band never hides the text at the top or the dialogue at the
// bottom.
func fogProfile(t float64) float64 {
	return math.Sin(clamp01(t) * math.Pi)
}
