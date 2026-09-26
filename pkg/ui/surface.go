package ui

import (
	"go.hasen.dev/shirei"
	"go.hasen.dev/shirei/widgets"
)

// Design tokens matching the retired SPA's dark glassmorphic theme: a stone
// canvas, translucent raised surfaces, hairline borders, soft shadows, purple
// accents, and a serif body face.
var (
	CanvasBG       = shirei.Vec4{20, 13, 4, 1}     // #0c0a09 stone-950
	PanelBG        = shirei.Vec4{24, 12, 12, 0.70} // rgba(18,15,13,0.70)
	CardBG         = shirei.Vec4{24, 12, 10, 0.55} // rgba(22,19,17,0.55)
	DrawerBG       = shirei.Vec4{20, 11, 5, 0.90}  // rgba(12,10,9,0.90)
	RaisedBG       = shirei.Vec4{24, 8, 16, 1}     // #292524 stone-800
	TextMain       = shirei.Vec4{20, 6, 92, 1}     // #e7e5e4 stone-200
	TextMuted      = shirei.Vec4{24, 6, 63, 1}     // #a8a29e stone-400
	TextFaint      = shirei.Vec4{25, 5, 45, 1}     // #78716c stone-500
	Hairline       = shirei.Vec4{0, 0, 100, 0.08}  // rgba(255,255,255,0.08)
	Accent         = shirei.Vec4{272, 80, 75, 1}   // #c084fc purple-400
	Amber          = shirei.Vec4{38, 92, 50, 1}    // #f59e0b amber-500
	Danger         = shirei.Vec4{0, 70, 52, 1}     // #ef4444 red-500
	Success        = shirei.Vec4{160, 60, 45, 1}   // emerald
	DockBG         = shirei.Vec4{20, 13, 4, 0.90}  // stone-950/90 rail
	PillBG         = shirei.Vec4{24, 10, 8, 0.80}  // stone-900/80 pill
	HoverFill      = shirei.Vec4{0, 0, 100, 0.06}  // white/[0.06]
	AccentSoft     = shirei.Vec4{270, 60, 55, 0.30}
	AccentBtn      = shirei.Vec4{271, 81, 56, 1} // purple-600
	AccentBtnHover = shirei.Vec4{271, 91, 61, 1}
	AccentBorder   = shirei.Vec4{0, 0, 100, 0.20}
	VignetteTop    = shirei.Vec4{20, 13, 4, 0.55}
	VignetteBottom = shirei.Vec4{20, 13, 4, 0.85}
	InputBG        = shirei.Vec4{20, 13, 4, 0.55}
	PlayerTone     = shirei.Vec4{199, 89, 60, 1} // sky-400
	PartialTone    = shirei.Vec4{38, 92, 50, 1}  // amber-500
)

// System font stacks. Shirei falls back per rune, so an unavailable face falls
// through to the next installed one.
var (
	SerifStack = []string{"EB Garamond", "Georgia", "DejaVu Serif", "Liberation Serif", "Times New Roman"}
	SansStack  = []string{"Inter", "Segoe UI", "DejaVu Sans", "Liberation Sans", "Arial"}
)

// Theme installs the dark scheme so stock widgets render dark.
func init() {
	scheme := widgets.DarkColorScheme()
	scheme.Surfaces.Canvas.Background = CanvasBG
	scheme.Surfaces.Canvas.Text = TextMain
	scheme.Surfaces.Panel.Background = RaisedBG
	scheme.Surfaces.Panel.Text = TextMain
	scheme.Surfaces.Toolbar.Background = shirei.Vec4{24, 12, 12, 1}
	scheme.Surfaces.Toolbar.Text = TextMain
	scheme.FocusRing = Accent
	widgets.SetDarkColorScheme(scheme)
	widgets.SetDarkMode(true)
}

// Serif is an attribute applying the serif body face.
func Serif(mods ...shirei.TextStyleFn) shirei.AttrsFn {
	all := append([]shirei.TextStyleFn{shirei.Fonts(SerifStack...)}, mods...)
	return shirei.AmendTextStyle(all...)
}

// Sans is an attribute applying the sans heading face.
func Sans(mods ...shirei.TextStyleFn) shirei.AttrsFn {
	all := append([]shirei.TextStyleFn{shirei.Fonts(SansStack...), shirei.FontWeight(shirei.WeightBold)}, mods...)
	return shirei.AmendTextStyle(all...)
}

// Card is a translucent raised panel with a hairline border and soft shadow.
func Card(attrs ...shirei.AttrsFn) shirei.AttrsFn {
	return func(a *shirei.AttrSet) {
		base := []shirei.AttrsFn{
			shirei.Corners(14),
			shirei.BorderWidth(1),
			shirei.BorderColorVec(Hairline),
			shirei.BackgroundVec(CardBG),
			shirei.BoxShadow(18),
			shirei.Pad(16),
		}
		for _, fn := range append(base, attrs...) {
			fn(a)
		}
	}
}

// GlassSurface is a translucent full-bleed surface.
func GlassSurface(attrs ...shirei.AttrsFn) shirei.AttrsFn {
	return func(a *shirei.AttrSet) {
		base := []shirei.AttrsFn{shirei.BackgroundVec(PanelBG), shirei.BorderColorVec(Hairline)}
		for _, fn := range append(base, attrs...) {
			fn(a)
		}
	}
}

// HairlineBorder draws a 1px hairline outline.
func HairlineBorder(attrs ...shirei.AttrsFn) shirei.AttrsFn {
	return func(a *shirei.AttrSet) {
		base := []shirei.AttrsFn{shirei.BorderWidth(1), shirei.BorderColorVec(Hairline)}
		for _, fn := range append(base, attrs...) {
			fn(a)
		}
	}
}
