package theater

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// TextFunc draws a block of prose. The desktop app supplies a Markdown-aware
// renderer; the default is a plain label.
type TextFunc func(text string)

// View renders a theatre frame with the default plain-text renderer.
func View(f Frame) {
	ViewWith(f, nil)
}

// ViewWith renders a visual-novel frame: a full-bleed scene, left/right square
// character boxes, and one line of dialogue with a speaker plate. All motion is
// a function of f.Progress so the live and offline renderers agree.
func ViewWith(f Frame, text TextFunc) {
	sc, beat, ok := BeatAt(f)
	if !ok {
		return
	}
	draw := text
	if draw == nil {
		draw = func(s string) { Label(s, Fonts(ui.SerifStack...), FontSize(20), TextColorVec(ui.TextMain)) }
	}

	winW := GetHost().WindowSize[0]
	sprite := winW * 0.24
	if sprite < 140 {
		sprite = 140
	}
	if sprite > 340 {
		sprite = 340
	}

	Container(Attrs(Viewport, BackgroundVec(ui.CanvasBG)), func() {
		w := GetContentWidth()
		h := GetContentHeight()
		if w < 1 || h < 1 {
			RequestNextFrame()
			return
		}

		// Full-bleed scene.
		Container(Attrs(FixSize(w, h), Float(0, 0), Clip, NoAnimate), func() {
			if sc.ArtPath != "" {
				coverImage("bg:"+sc.ArtPath, sc.ArtPath, w, h)
			} else {
				Container(Attrs(Expand, Expand, BackgroundVec(ui.RaisedBG)), func() {})
			}
		})
		// Cinematic scrim.
		Container(Attrs(FixSize(w, h), Float(0, 0), NoAnimate), func() {
			Element(Attrs(Expand, Grow(1), BackgroundVec(ui.VignetteTop)))
			Element(Attrs(Expand, FixHeight(h*0.5), BackgroundVec(ui.VignetteBottom)))
		})

		// Stage and dialogue.
		Container(Attrs(FixSize(w, h), Float(0, 0), Pad(24)), func() {
			if sc.LocationName != "" {
				Label(sc.LocationName, Fonts(ui.SansStack...), FontSize(12), FontWeight(WeightBold), TextColorVec(ui.Accent))
			}
			Filler(1)
			Container(Attrs(Row, Expand, CrossAlign(AlignEnd), Pad2(0, winW*0.04)), func() {
				spriteBox(f.PlayerPortrait, f.PlayerLabel, f.PlayerActive, ui.PlayerTone, sprite, false)
				Filler(1)
				spriteBox(f.NPCPortrait, f.NPCLabel, f.NPCActive, ui.Accent, sprite, true)
			})
			Filler(1)
			dialogueBox(f, beat, draw)
		})
	})
}

func spriteBox(portrait, label string, active bool, tone Vec4, size float32, mirror bool) {
	if portrait == "" {
		Container(Attrs(FixWidth(size), FixHeight(size)), func() {})
		return
	}
	_ = mirror // shirei cannot flip an image; the sprite reads the same either way
	Container(Attrs(Gap(8), Center), func() {
		Container(Attrs(FixSize(size, size), Corners(16), Clip, BackgroundVec(ui.RaisedBG)), func() {
			if active {
				ModAttrs(BorderWidth(2), BorderColorVec(tone), BoxShadow(28))
			} else {
				ModAttrs(BorderWidth(2), BorderColorVec(ui.Hairline))
			}
			coverImage("sprite:"+portrait, portrait, size, size)
		})
		if label != "" {
			Container(Attrs(Pad2(4, 12), Corners(999), BorderWidth(1)), func() {
				if active {
					ModAttrs(BackgroundVec(tone), BorderColorVec(ui.AccentBorder))
				} else {
					ModAttrs(BackgroundVec(ui.InputBG), BorderColorVec(ui.Hairline))
				}
				Label(label, Fonts(ui.SansStack...), FontSize(12), FontWeight(WeightBold), TextColorVec(ui.TextMain))
			})
		}
	})
}

func dialogueBox(f Frame, beat scene.Beat, draw TextFunc) {
	isSpeech := beat.Kind == scene.BeatSpeech
	name := "Narrator"
	plate := ui.RaisedBG
	portrait := ""
	if isSpeech {
		name = beat.Speaker
		if name == "" {
			name = "Unknown"
		}
		plate = ui.AccentBtn
		if beat.Speaker != "" {
			portrait = f.NPCPortrait
		}
		if f.PlayerActive {
			plate = ui.PlayerTone
			portrait = f.PlayerPortrait
		}
	}

	Container(Attrs(Expand, Center), func() {
		Container(Attrs(Row, Expand, CrossAlign(AlignEnd), Gap(12), MaxWidth(896)), func() {
			if isSpeech && portrait != "" {
				Container(Attrs(FixSize(64, 64), Corners(10), Clip, BorderWidth(2), BorderColorVec(plate)), func() {
					coverImage("dbox:"+portrait, portrait, 64, 64)
				})
			}
			Container(Attrs(Grow(1), Gap(8), Pad(20), Corners(16), BackgroundVec(ui.CardBG),
				BorderWidth(1), BorderColorVec(plate)), func() {
				Container(Attrs(Pad2(3, 12), Corners(8), BackgroundVec(plate)), func() {
					Label(name, Fonts(ui.SansStack...), FontSize(13), FontWeight(WeightBold), TextColorVec(ui.TextMain))
				})
				Container(Attrs(Expand, FixHeight(140), Clip), func() {
					if isSpeech {
						draw("\u201c" + beat.Text + "\u201d")
					} else {
						draw(beat.Text)
					}
				})
				Container(Attrs(Row, Expand), func() {
					Filler(1)
					Label("▾", FontSize(12), TextColorVec(ui.Accent))
				})
			})
		})
	})
}
