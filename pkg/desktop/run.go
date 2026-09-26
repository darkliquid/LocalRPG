package desktop

import (
	"go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// Config configures the desktop application shell.
type Config struct {
	Dir     string // project root directory; unused until screens land
	PNGPath string // when set, render one frame to this path and return
	Width   int
	Height  int
}

// Run starts the desktop GUI, or renders a single headless frame when
// Config.PNGPath is set.
func Run(cfg Config) error {
	if cfg.Width == 0 {
		cfg.Width = 1280
	}
	if cfg.Height == 0 {
		cfg.Height = 800
	}
	if cfg.PNGPath != "" {
		return shirei.RenderToPNG(cfg.PNGPath, cfg.Width, cfg.Height, shirei.FrameFn(RootView))
	}
	ui.Run("LocalRPG", cfg.Width, cfg.Height, shirei.FrameFn(RootView))
	return nil
}
