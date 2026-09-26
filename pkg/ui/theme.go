package ui

import "go.hasen.dev/shirei"

// Palette is the application colour scheme in shirei's HSLA form:
// hue 0-360, saturation 0-100, lightness 0-100, alpha 0-1.
//
// Values are first-pass matches for the retired SPA's stone/amber styling.
// Tune them against the SPA screenshot while porting screens; this is not a
// pixel-parity exercise.
type Palette struct {
	Bg     shirei.Vec4
	Panel  shirei.Vec4
	Text   shirei.Vec4
	Muted  shirei.Vec4
	Border shirei.Vec4
	Accent shirei.Vec4
	Danger shirei.Vec4
}

// DefaultPalette returns the light-on-dark palette used by the desktop app.
func DefaultPalette() Palette {
	return Palette{
		Bg:     shirei.Vec4{20, 13, 4, 1},  // stone-950
		Panel:  shirei.Vec4{24, 8, 10, 1},  // raised surface
		Text:   shirei.Vec4{20, 6, 92, 1},  // stone-200
		Muted:  shirei.Vec4{25, 5, 45, 1},  // stone-500
		Border: shirei.Vec4{24, 6, 20, 1},  // hairline
		Accent: shirei.Vec4{30, 65, 55, 1}, // amber
		Danger: shirei.Vec4{0, 65, 55, 1},
	}
}
