package ui

import "go.hasen.dev/shirei"

// Palette is the application colour scheme in shirei's HSLA form:
// hue 0-360, saturation 0-100, lightness 0-100, alpha 0-1.
type Palette struct {
	Bg     shirei.Vec4
	Panel  shirei.Vec4
	Text   shirei.Vec4
	Muted  shirei.Vec4
	Border shirei.Vec4
	Accent shirei.Vec4
	Danger shirei.Vec4
}

// DefaultPalette returns the dark stone/purple palette used by the desktop app.
func DefaultPalette() Palette {
	return Palette{
		Bg:     CanvasBG,
		Panel:  RaisedBG,
		Text:   TextMain,
		Muted:  TextMuted,
		Border: Hairline,
		Accent: Accent,
		Danger: Danger,
	}
}
