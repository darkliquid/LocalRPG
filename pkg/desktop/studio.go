package desktop

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// applySelection switches the studio's current item, refusing to silently
// replace a dirty draft.
func applySelection(next Selection, dirty func() bool) {
	if appState.Studio.Kind == "draft" && dirty() {
		appState.PendingStudio = next
		appState.ConfirmDiscard = true
		return
	}
	appState.Studio = next
	appState.StudioDirty = false
}

// studioHeader renders a studio's top bar: a glass rail with a Back pill on the
// left and an accent New pill on the right.
func studioHeader(title string, onNew, onBack func()) {
	showLabels := GetContentWidth() > 760
	Container(Attrs(Row, CrossMid, Gap(12), Expand, Corners(16), BackgroundVec(ui.PanelBG),
		BorderWidth(1), BorderColorVec(ui.Hairline), Pad2(10, 16), BoxShadow(22)), func() {
		Container(Attrs(Row, CrossMid, Gap(6), Corners(12), Pad2(6, 12), BackgroundVec(ui.HoverFill),
			BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
			if IsHovered() {
				ModAttrs(BackgroundVec(ui.AccentSoft))
			}
			NextAccessName("studio.back")
			if PressAction() {
				onBack()
			}
			AssignAccess()
			Icon(TypArrowLeft, FontSize(14), TextColorVec(ui.TextMain))
			if showLabels {
				Label("Back", Fonts(ui.SansStack...), FontSize(12), TextColorVec(ui.TextMain))
			}
		})
		Label(title, Fonts(ui.SansStack...), FontSize(16), FontWeight(WeightBold), TextColorVec(ui.TextMain))
		if appState.StudioDirty {
			Label("unsaved", FontSize(11), TextColorVec(ui.Amber))
		}
		Filler(1)
		Container(Attrs(Row, CrossMid, Gap(6), Corners(12), Pad2(6, 14), BackgroundVec(ui.AccentBtn),
			BorderWidth(1), BorderColorVec(ui.AccentBorder), BoxShadow(14)), func() {
			if IsHovered() {
				ModAttrs(BackgroundVec(ui.AccentBtnHover))
			}
			NextAccessName("studio.new")
			if PressAction() {
				onNew()
			}
			AssignAccess()
			Icon(TypPlus, FontSize(14), TextColorVec(ui.TextMain))
			Label("New", Fonts(ui.SansStack...), FontSize(12), FontWeight(WeightBold), TextColorVec(ui.TextMain))
		})
	})
}

// studioPill is a selectable rounded chip used for item rails and tab strips.
func studioPill(name, label string, selected bool, action func()) {
	Container(Attrs(Row, CrossMid, Gap(6), Corners(10), Pad2(6, 12)), func() {
		switch {
		case selected:
			ModAttrs(BackgroundVec(ui.AccentBtn), BoxShadow(12))
		case IsHovered():
			ModAttrs(BackgroundVec(ui.HoverFill))
		}
		if name != "" {
			NextAccessName(name)
		}
		if PressAction() {
			action()
		}
		AssignAccess()
		weight := WeightNormal
		if selected {
			weight = WeightBold
		}
		Label(label, Fonts(ui.SansStack...), FontSize(12), FontWeight(weight), TextColorVec(ui.TextMain))
	})
}

// studioTabStrip is the recessed pill rail above a studio's editor.
func studioTabStrip(prefix string, tabs []studioTab, current string, onPick func(string)) {
	Container(Attrs(Row, CrossMid, Gap(4), Corners(12), Pad(4), BackgroundVec(ui.DockBG),
		BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
		for _, tab := range tabs {
			tab := tab
			studioPill(prefix+"."+tab.key, tab.label, current == tab.key, func() { onPick(tab.key) })
		}
	})
}

// studioTab names one editor tab.
type studioTab struct {
	key   string
	label string
}

// studioForm is the translucent card wrapping a studio's editable fields.
func studioForm(body func()) {
	Container(Attrs(Expand, ui.Card(Gap(12), Pad(20))), body)
}

// studioFieldLabel is a muted sans caption above a form control.
func studioFieldLabel(text string) {
	Label(text, FontSize(11), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
}

// studioPrimaryButton is the accent action button (Save) at the foot of a form.
func studioPrimaryButton(name, label string, action func()) {
	Container(Attrs(FixHeight(36), Corners(12), Pad2(0, 20), Center, BackgroundVec(ui.AccentBtn),
		BorderWidth(1), BorderColorVec(ui.AccentBorder), BoxShadow(18)), func() {
		if IsHovered() {
			ModAttrs(BackgroundVec(ui.AccentBtnHover))
		}
		NextAccessName(name)
		if PressAction() {
			action()
		}
		AssignAccess()
		Label(label, Fonts(ui.SansStack...), FontSize(12), FontWeight(WeightBold), TextColorVec(ui.TextMain))
	})
}

// confirmDiscardModal asks before throwing away an unsaved draft.
func confirmDiscardModal() {
	if !appState.ConfirmDiscard {
		return
	}
	p := ui.DefaultPalette()
	stage := ModalStyle{Background: p.Panel, Text: p.Text, Scrim: Vec4{0, 0, 0, 0.6}}
	ModalStyled(460, func() { appState.ConfirmDiscard = false }, stage, func() {
		Label("Discard unsaved changes?", FontSize(15), FontWeight(WeightBold), TextColorVec(p.Text))
		Container(Attrs(Row, Gap(8)), func() {
			NextAccessName("studio.discard")
			if Button(NoIcon, "Discard") {
				appState.Studio = appState.PendingStudio
				appState.StudioDirty = false
				appState.ConfirmDiscard = false
			}
			AssignAccess()
			NextAccessName("studio.discard.cancel")
			if Button(NoIcon, "Cancel") {
				appState.ConfirmDiscard = false
			}
			AssignAccess()
		})
	})
}
