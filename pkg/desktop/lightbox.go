package desktop

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// lightbox draws a full-window overlay of the image named by
// appState.LightboxPath. Clicking the scrim or pressing Escape (Modal's
// built-in dismiss) clears it.
func lightbox() {
	if appState.LightboxPath == "" {
		return
	}
	p := ui.DefaultPalette()
	path := appState.LightboxPath
	dismiss := func() { appState.LightboxPath = "" }
	stage := ModalStyle{Background: p.Panel, Text: p.Text, Scrim: Vec4{0, 0, 0, 0.6}}
	// Size from the window, never from the container: the card's height is
	// derived from the image, so querying it back would shrink every frame.
	winW := GetHost().WindowSize[0]
	winH := GetHost().WindowSize[1]
	ModalStyled(winW*0.9, dismiss, stage, func() {
		NextAccessName("lightbox.image")
		AssignAccess()
		Image(path, Vec2{winW * 0.82, winH * 0.78})
		Container(Attrs(Row, CrossMid), func() {
			Filler(1)
			NextAccessName("lightbox.close")
			if Button(NoIcon, "Close") {
				dismiss()
			}
			AssignAccess()
		})
	})
}
