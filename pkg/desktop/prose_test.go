package desktop

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestProseSnapshot(t *testing.T) {
	src := "A *quiet* room.\n\n**Vance** waits.\n\n- one\n- two\n\n> a whisper\n\n---\n\nMeet [[hero|Vance]] using `1d20`."
	p := ui.DefaultPalette()
	view := func() {
		Container(Attrs(Viewport, BackgroundVec(p.Bg), Pad(16), Gap(8)), func() {
			proseBlocks(src, TextColorVec(p.Text))
		})
	}
	ui.Snapshot(t, "prose", 600, 500, view)
}
