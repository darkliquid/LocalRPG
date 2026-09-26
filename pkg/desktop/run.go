package desktop

import (
	"context"
	"errors"

	"go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/config"
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
		saveEntity = func(ctx context.Context, svc *gui.Service, gameID, entityID, markdown string) error {
			return svc.SaveEntity(ctx, gameID, entityID, markdown)
		}
		regeneratePortrait = func(ctx context.Context, svc *gui.Service, gameID, characterID string) error {
			_, err := svc.RegenerateCharacterPortrait(ctx, gameID, characterID)
			return err
		}
		mergeEntities = func(ctx context.Context, svc *gui.Service, gameID, sourceID, targetID string) error {
			_, err := svc.MergeEntities(ctx, gameID, sourceID, targetID)
			return err
		}
		portraitPath = writePortrait
		saveSettings = func(ctx context.Context, svc *gui.Service, cfg config.Config) error {
			_, err := svc.SaveSettings(ctx, cfg)
			return err
		}
		testProvider = func(ctx context.Context, svc *gui.Service, req gui.TestProviderRequestDTO) (*gui.TestProviderResponseDTO, error) {
			return svc.TestProvider(ctx, req)
		}
		inspectTTS = func(ctx context.Context, svc *gui.Service, req gui.TTSInspectRequestDTO) (*gui.TTSInspectResponseDTO, error) {
			return svc.InspectTTS(ctx, req)
		}
		downloadModel = func(ctx context.Context, svc *gui.Service, id string) error {
			return svc.DownloadModel(ctx, id)
		}
		saveSystem = func(ctx context.Context, svc *gui.Service, req gui.CreateSystemRequestDTO) error {
			_, err := svc.SaveSystem(ctx, req)
			return err
		}
		generateText = func(ctx context.Context, svc *gui.Service, req gui.GenerateTextRequest) (*gui.GenerateTextResponse, error) {
			return svc.GenerateText(ctx, req)
		}
		saveWorld = func(ctx context.Context, svc *gui.Service, req gui.CreateWorldRequestDTO) error {
			if req.ID == "" {
				_, err := svc.CreateWorld(ctx, req)
				return err
			}
			_, err := svc.UpdateWorld(ctx, req)
			return err
		}
		saveWorldEntity = func(ctx context.Context, svc *gui.Service, worldID, entityID, markdown string) error {
			return svc.SaveWorldEntity(ctx, worldID, entityID, markdown)
		}
		deleteWorldEntity = func(ctx context.Context, svc *gui.Service, worldID, entityID string) error {
			return svc.DeleteWorldEntity(ctx, worldID, entityID)
		}
		refreshModels(cfg.Service)
		startModelEvents(cfg.Service)
	} else {
		liveService = nil
		createGame = nil
		generatePreview = nil
		saveAsset = nil
		saveGameSettings = nil
		restartGame = nil
		deleteGame = nil
		saveEntity = nil
		regeneratePortrait = nil
		mergeEntities = nil
		portraitPath = nil
		saveSettings = nil
		testProvider = nil
		inspectTTS = nil
		downloadModel = nil
		saveSystem = nil
		generateText = nil
		saveWorld = nil
		saveWorldEntity = nil
		deleteWorldEntity = nil
	}

	if cfg.PNGPath != "" {
		return shirei.RenderToPNG(cfg.PNGPath, cfg.Width, cfg.Height, shirei.FrameFn(RootView))
	}
	ui.Run("LocalRPG", cfg.Width, cfg.Height, shirei.FrameFn(RootView))
	return nil
}
