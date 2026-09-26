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

	if cfg.Service != nil {
		liveService = cfg.Service
		createGame = func(ctx context.Context, svc *gui.Service, req gui.CreateGameRequestDTO) (*gui.GameSummaryDTO, error) {
			return svc.CreateGame(ctx, req)
		}
		generatePreview = func(ctx context.Context, svc *gui.Service, req gui.GenerateAssetPreviewRequestDTO) ([]byte, string, error) {
			return svc.GenerateAssetPreview(ctx, req)
		}
		saveAsset = func(ctx context.Context, svc *gui.Service, gameID, kind string, data []byte, ext string) error {
			_, err := svc.SaveGameAsset(gameID, kind, data, ext)
			return err
		}
		saveGameSettings = func(ctx context.Context, svc *gui.Service, gameID string, patch map[string]any) error {
			return svc.UpdateGameSettings(ctx, gameID, patch)
		}
		restartGame = func(ctx context.Context, svc *gui.Service, gameID string) error {
			_, err := svc.RestartGame(ctx, gameID)
			return err
		}
		deleteGame = func(ctx context.Context, svc *gui.Service, gameID string) error {
			return svc.DeleteGame(ctx, gameID)
		}
	} else {
		liveService = nil
		createGame = nil
		generatePreview = nil
		saveAsset = nil
		saveGameSettings = nil
		restartGame = nil
		deleteGame = nil
	}

	if cfg.PNGPath != "" {
		return shirei.RenderToPNG(cfg.PNGPath, cfg.Width, cfg.Height, shirei.FrameFn(RootView))
	}
	ui.Run("LocalRPG", cfg.Width, cfg.Height, shirei.FrameFn(RootView))
	return nil
}
