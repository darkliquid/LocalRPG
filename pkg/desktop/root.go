package desktop

import (
	. "go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// RootView renders the application frame. It currently shows only a title;
// screens are added by later plans.
func RootView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg), Pad(24)), func() {
		Label("LocalRPG", FontSize(22), FontWeight(WeightBold), TextColorVec(p.Text))
		Label("GUI foundations", FontSize(13), TextColorVec(p.Muted))
	})
}
