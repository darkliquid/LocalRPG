package theater

import (
	. "go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/scene"
)

// TextFunc draws a block of prose. The desktop app supplies a Markdown-aware
// renderer; the default is a plain label.
type TextFunc func(text string)

// View renders a theatre frame with the default plain-text renderer.
func View(f Frame) {
	ViewWith(f, nil)
}

// ViewWith renders a theatre frame, using text for narrative prose. All
// animation is a function of f.Progress so the live and offline renderers
// agree.
func ViewWith(f Frame, text TextFunc) {
	sc, beat, ok := BeatAt(f)
	if !ok {
		return
	}
	draw := text
	if draw == nil {
		draw = func(s string) { Label(s) }
	}

	bg := Vec4{20, 13, 4, 1}
	panel := Vec4{24, 8, 10, 1}
	muted := Vec4{25, 5, 45, 1}
	accent := Vec4{30, 65, 55, 1}

	Container(Attrs(Viewport, BackgroundVec(bg)), func() {
		if sc.ArtPath != "" {
			Image(sc.ArtPath, Vec2{GetContentWidth(), GetContentHeight()})
		}
		// A scrim over the art keeps the dialogue legible.
		Container(Attrs(Expand, Grow(1), BackgroundVec(Vec4{0, 0, 0, 0.35})), func() {
			if sc.LocationName != "" {
				Container(Attrs(Pad(16)), func() {
					Label(sc.LocationName, FontSize(14), FontWeight(WeightBold), TextColorVec(accent))
				})
			}
		})
		Container(Attrs(Expand, FixHeight(170), BackgroundVec(panel), Pad(18), Gap(8)), func() {
			if beat.Kind == scene.BeatSpeech {
				Label(beat.Speaker, FontSize(15), FontWeight(WeightBold), TextColorVec(accent))
			} else {
				Label("Narrator", FontSize(12), TextColorVec(muted))
			}
			Container(Attrs(Expand, MaxWidth(760)), func() {
				draw(beat.Text)
			})
			// Progress bar for the current beat.
			Container(Attrs(Expand, FixHeight(3), BackgroundVec(bg)), func() {
				frac := float32(f.Progress)
				if frac <= 0 {
					return
				}
				Container(Attrs(Grow(frac), FixHeight(3), BackgroundVec(accent)), func() {})
			})
		})
	})
}
