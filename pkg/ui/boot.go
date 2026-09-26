package ui

import (
	"go.hasen.dev/shirei"
	app "go.hasen.dev/shirei/app"
)

// Run opens the native window and enters the shirei frame loop. It does not
// return; quit paths exit the process. SetupWindow must be called first, which
// Run does, so callers only supply the title and content size.
func Run(title string, w, h int, view shirei.FrameFn) {
	app.SetupWindow(title, w, h)
	app.Run(view)
}
