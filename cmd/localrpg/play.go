package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/models"
	"github.com/darkliquid/localrpg/pkg/paths"
	"github.com/darkliquid/localrpg/pkg/pricing"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/tools"
	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/darkliquid/localrpg/pkg/tui"
)

func handlePlayCommand(args []string) {
	code := runPlayCommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

func runPlayCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "Usage: localrpg play <game-id>")
		return 1
	}

	cfgMgr := config.NewConfigManager()
	cfg, err := cfgMgr.Load()
	if err != nil {
		fmt.Fprintf(stderr, "Error loading config: %v\n", err)
		return 1
	}

	telemetryProvider, err := telemetry.New(context.Background(), cfg.Telemetry, telemetry.BuildInfo{
		Version:    Version,
		ConfigFile: cfgMgr.ActiveFilePath(),
	})
	if err != nil {
		fmt.Fprintf(stderr, "Error starting telemetry: %v\n", err)
		return 1
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = telemetryProvider.Shutdown(shutdownCtx)
	}()

	gameID := args[0]
	dirs := paths.Resolve(paths.System(), cfg.Paths, "")
	resolver := core.NewCustomPathResolver(dirs.Systems, dirs.Worlds, dirs.Games, dirs.Cache)
	logger := telemetryProvider.Logger(buildTraceLogger(cfg, os.Args, resolver.CacheDir()))
	logger.SetGame(gameID)

	session, err := openPlaySession(gameID, cfg, resolver, logger, stderr)
	if err != nil {
		return 1
	}
	defer session.Close()

	app := tui.NewAppModel(session.orchestrator, 80, 24)
	seedOpeningScene(app, session, stderr)

	return runPlayTUI(app, stderr)
}

// runPlayTUI hands the terminal to the play UI and reports how it ended.
func runPlayTUI(app *tui.AppModel, stderr io.Writer) int {
	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(stderr, "Error running TUI: %v\n", err)
		return 1
	}
	return 0
}

// playSession is what a play run builds before the TUI starts: the campaign's
// timeline and the orchestrator the TUI drives. Close releases the background work
// the session started.
type playSession struct {
	orchestrator  *engine.TurnOrchestrator
	history       *engine.HistoryLogger
	manifest      *core.GameManifest
	stopEmbedding func()
}

func (s *playSession) Close() {
	if s.stopEmbedding != nil {
		s.stopEmbedding()
	}
}

// openPlaySession opens a campaign and builds everything a turn needs: its store
// and index, its timeline, the rules engine, the model router, and the tool
// executor the orchestrator runs.
func openPlaySession(gameID string, cfg *config.Config, resolver *core.PathResolver, logger trace.Logger, stderr io.Writer) (session *playSession, err error) {
	gameDir := resolver.GameDir(gameID)

	manifestPath := filepath.Join(gameDir, "game.yaml")
	manifest, err := core.LoadGameManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading game manifest %q: %v\n", manifestPath, err)
		return nil, err
	}

	store, err := storage.OpenGameStore(resolver, gameID)
	if err != nil {
		fmt.Fprintf(stderr, "Error opening game database: %v\n", err)
		return nil, err
	}

	// Reindex from Markdown so the session reflects any edits made outside the TUI
	syncer := storage.NewSyncer(store)
	if _, err := syncer.Sync(filepath.Join(gameDir, "entities")); err != nil {
		fmt.Fprintf(stderr, "Error syncing entities: %v\n", err)
		return nil, err
	}

	history := engine.NewHistoryLogger(filepath.Join(gameDir, "history.jsonl"))
	timeline := engine.NewTimeline(resolver, store, history, gameID)

	stopEmbedding := startEmbeddingWorker(cfg, resolver, store, timeline)
	// The caller owns the worker's lifetime once this returns, so until then a
	// failure has to stop it.
	defer func() {
		if err != nil {
			stopEmbedding()
		}
	}()

	if err := timeline.EnsureIndexed(); err != nil {
		fmt.Fprintf(stderr, "Error indexing turns: %v\n", err)
		return nil, err
	}

	playerID := resolvePlayerIdentity(manifest, resolver, store, stderr)

	startLocation, err := engine.ResolveStartLocation(resolver, store, manifest)
	if err != nil {
		fmt.Fprintf(stderr, "Error resolving start location: %v\n", err)
		return nil, err
	}

	orchestrator, err := buildOrchestrator(cfg, resolver, store, timeline, manifest, playerID, startLocation, logger, stderr)
	if err != nil {
		return nil, err
	}

	return &playSession{
		orchestrator:  orchestrator,
		history:       history,
		manifest:      manifest,
		stopEmbedding: stopEmbedding,
	}, nil
}

// resolvePlayerIdentity returns the player's entity id, repairing a campaign whose
// player: field holds a display name, which is what a campaign written before
// player_name existed carries.
func resolvePlayerIdentity(manifest *core.GameManifest, resolver *core.PathResolver, store *storage.Store, stderr io.Writer) string {
	playerID := manifest.Player
	if resolved, err := engine.RepairPlayerIdentity(resolver, store, manifest); err != nil {
		fmt.Fprintf(stderr, "Warning: could not reconcile player identity: %v\n", err)
	} else if resolved != "" {
		playerID = resolved
	}
	return playerID
}

// startEmbeddingWorker points the embedding factory at the local model cache and
// starts the worker that embeds new content, returning the function that stops it.
// It returns a no-op when the configuration has no embedding provider.
func startEmbeddingWorker(cfg *config.Config, resolver *core.PathResolver, store *storage.Store, timeline *engine.Timeline) func() {
	// Point the embedding factory at the local model cache so the ONNX encoder
	// resolves.
	embeddings.SetModelDir(models.NewManager(resolver.CacheDir()).ModelDir(models.EmbeddingEncoderModelID))

	embProvider, err := embeddings.NewProviderFromConfig(cfg.Embeddings)
	if err != nil || embProvider == nil {
		return func() {}
	}

	worker := storage.NewEmbeddingWorker(store, embProvider, storage.EmbeddingWorkerOptions{
		BatchSize: cfg.Embeddings.BatchSize,
	})
	worker.Start()
	timeline.SetEmbeddingWorker(worker)
	return worker.Stop
}

// buildOrchestrator wires the turn pipeline: the rules engine that runs a system's
// hooks, the router that picks a model per role, the extractor and completion
// provider, the tool executor, and the prompts the campaign overrides.
func buildOrchestrator(cfg *config.Config, resolver *core.PathResolver, store *storage.Store, timeline *engine.Timeline, manifest *core.GameManifest, playerID, startLocation string, logger trace.Logger, stderr io.Writer) (*engine.TurnOrchestrator, error) {
	bridge := rules.NewHostBridge(store, timeline, playerID)
	jsEngine := rules.NewJSEngine(bridge)

	ruleLoader := rules.NewRuleLoader(resolver, jsEngine)
	_ = ruleLoader.LoadRules(manifest.SystemID, manifest.WorldID)

	router, err := harness.RouterFromConfigWithLogger(cfg, logger)
	if err != nil {
		fmt.Fprintf(stderr, "Error building model router: %v\n", err)
		return nil, err
	}
	router.SetChainPrice(pricing.RouterChainPrice(cfg, router))

	orchestrator := engine.NewTurnOrchestrator(
		store,
		timeline,
		jsEngine,
		router,
		startLocation,
		playerID,
	)
	orchestrator.SetLogger(logger)
	orchestrator.SetExtractor(harness.ExtractorFromConfigWithLogger(cfg, router, logger))
	orchestrator.SetCompletionProvider(harness.CompletionFromConfig(cfg, router, logger))
	orchestrator.SetCompletionPolicy(engine.CompletionPolicy{
		Mode:        cfg.CompletionMode(),
		MaxAttempts: cfg.CompletionAttempts(),
		TailChars:   cfg.CompletionTailChars(),
		MinChars:    cfg.CompletionMinChars(),
		Timeout:     cfg.CompletionTimeout(),
	})

	toolExecutor := tools.NewExecutor(store, cfg.ToolResultChars())
	toolExecutor.SetVoiceProfiles(timeline.VoiceProfiles())
	toolExecutor.SetEntityWriter(timeline)
	if embProvider, err := embeddings.NewProviderFromConfig(cfg.Embeddings); err == nil && embProvider != nil {
		toolExecutor.SetEmbeddingsProvider(embProvider)
	}
	orchestrator.SetTools(toolExecutor, cfg.RoleSupportsTools("gm"))
	orchestrator.SetToolRounds(cfg.ToolRounds())
	orchestrator.SetActionEcho(cfg.ActionEcho())
	orchestrator.SetOpeningPrompt(engine.OpeningPrompt(manifest))
	orchestrator.LoadPrompts(resolver, manifest.SystemID, manifest.WorldID)

	if ttsCli, err := media.NewTTSClientWithSharedKey(cfg.Media.TTS, cfg.Providers.Gemini.APIKey); err == nil {
		cueCaps := media.ResolveSpeechCueCapabilities(cfg.Media.TTS, ttsCli)
		orchestrator.SetSpeechCues(harness.SpeechCueContext{
			AudioTags:        cueCaps.AudioTags,
			MarkdownEmphasis: cueCaps.MarkdownEmphasis,
			SampleTags:       cueCaps.SupportedTags,
			CustomGuidance:   cueCaps.PromptGuidance,
		})
	}

	return orchestrator, nil
}

// seedOpeningScene gives the TUI the quiet scene turn that opens a campaign with
// an opening prompt and no history, so the terminal restates what the GUI shows in
// its Prologue.
func seedOpeningScene(app *tui.AppModel, session *playSession, stderr io.Writer) {
	if engine.OpeningPrompt(session.manifest) == "" {
		return
	}
	turns, err := session.history.LoadHistory()
	if err != nil || len(turns) != 0 {
		return
	}

	session.orchestrator.SetSceneOnly(true)
	scene, err := session.orchestrator.ProcessAction(context.Background(), engine.OpeningMode, "")
	if err != nil {
		fmt.Fprintf(stderr, "Warning: could not establish the opening scene: %v\n", err)
		return
	}
	if scene != nil {
		app.Seed([]engine.Turn{*scene})
	}
}
