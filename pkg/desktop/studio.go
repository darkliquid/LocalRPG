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

// studioHeader renders a studio's top bar.
func studioHeader(title string, onNew, onBack func()) {
	p := ui.DefaultPalette()
	Container(Attrs(Row, CrossMid, Gap(10)), func() {
		Label(title, FontSize(24), FontWeight(WeightBold), TextColorVec(p.Text))
		if appState.StudioDirty {
			Label("unsaved", FontSize(11), TextColorVec(p.Accent))
		}
		Filler(1)
		NextAccessName("studio.new")
		if Button(NoIcon, "New") {
			onNew()
		}
		AssignAccess()
		NextAccessName("studio.back")
		if Button(NoIcon, "Back") {
			onBack()
		}
		AssignAccess()
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
