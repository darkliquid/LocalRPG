package desktop

import (
	"context"
	"errors"

	"go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// Config configures the desktop application shell. Exactly one of State or
// Service must be set: State for deterministic snapshots, Service for a live
// app.
type Config struct {
	Service *gui.Service
	Dir     string
	PNGPath string
	Width   int
	Height  int
	State   *State
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

	switch {
	case cfg.State != nil:
		appState = cfg.State
	case cfg.Service != nil:
		appState = loadAll(context.Background(), cfg.Service)
	default:
		return errors.New("desktop: Config needs State or Service")
	}

	if cfg.PNGPath != "" {
		return shirei.RenderToPNG(cfg.PNGPath, cfg.Width, cfg.Height, shirei.FrameFn(RootView))
	}
	ui.Run("LocalRPG", cfg.Width, cfg.Height, shirei.FrameFn(RootView))
	return nil
}
