package main

import (
	"context"
	"fmt"
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
	"github.com/darkliquid/localrpg/pkg/tui"
)

func handlePlayCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: localrpg play <game-id>")
		os.Exit(1)
	}

	cfgMgr := config.NewConfigManager()
	cfg, _ := cfgMgr.Load()

	telemetryProvider, err := telemetry.New(context.Background(), cfg.Telemetry, telemetry.BuildInfo{
		Version:    Version,
		ConfigFile: cfgMgr.ActiveFilePath(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting telemetry: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = telemetryProvider.Shutdown(shutdownCtx)
	}()

	gameID := args[0]
	dirs := paths.Resolve(paths.System(), cfg.Paths, "")
	resolver := core.NewCustomPathResolver(dirs.Systems, dirs.Worlds, dirs.Games, dirs.Cache)
	gameDir := resolver.GameDir(gameID)

	manifestPath := filepath.Join(gameDir, "game.yaml")
	manifest, err := core.LoadGameManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading game manifest %q: %v\n", manifestPath, err)
		os.Exit(1)
	}

	store, err := storage.OpenGameStore(resolver, gameID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening game database: %v\n", err)
		os.Exit(1)
	}

	// Reindex from Markdown so the session reflects any edits made outside the TUI
	syncer := storage.NewSyncer(store)
	if _, err := syncer.Sync(filepath.Join(gameDir, "entities")); err != nil {
		fmt.Fprintf(os.Stderr, "Error syncing entities: %v\n", err)
		os.Exit(1)
	}

	historyPath := filepath.Join(gameDir, "history.jsonl")
	history := engine.NewHistoryLogger(historyPath)

	timeline := engine.NewTimeline(resolver, store, history, gameID)
	// Point the embedding factory at the local model cache so the ONNX encoder
	// resolves.
	embeddings.SetModelDir(models.NewManager(resolver.CacheDir()).ModelDir(models.EmbeddingEncoderModelID))
	if embProvider, err := embeddings.NewProviderFromConfig(cfg.Embeddings); err == nil && embProvider != nil {
		worker := storage.NewEmbeddingWorker(store, embProvider, storage.EmbeddingWorkerOptions{
			BatchSize: cfg.Embeddings.BatchSize,
		})
		worker.Start()
		defer worker.Stop()
		timeline.SetEmbeddingWorker(worker)
	}
	if err := timeline.EnsureIndexed(); err != nil {
		fmt.Fprintf(os.Stderr, "Error indexing turns: %v\n", err)
		os.Exit(1)
	}

	// A campaign written before player_name existed holds a display name in
	// player:, which is repaired once here so the turn pipeline uses the real ID.
	playerID := manifest.Player
	if resolved, err := engine.RepairPlayerIdentity(resolver, store, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not reconcile player identity: %v\n", err)
	} else if resolved != "" {
		playerID = resolved
	}

	startLocation, err := engine.ResolveStartLocation(resolver, store, manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving start location: %v\n", err)
		os.Exit(1)
	}

	bridge := rules.NewHostBridge(store, timeline, playerID)
	jsEngine := rules.NewJSEngine(bridge)

	ruleLoader := rules.NewRuleLoader(resolver, jsEngine)
	_ = ruleLoader.LoadRules(manifest.SystemID, manifest.WorldID)

	localLogger := buildTraceLogger(cfg, os.Args, resolver.CacheDir())
	logger := telemetryProvider.Logger(localLogger)
	logger.SetGame(gameID)

	router, err := harness.RouterFromConfigWithLogger(cfg, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building model router: %v\n", err)
		os.Exit(1)
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

	app := tui.NewAppModel(orchestrator, 80, 24)

	// A campaign with an opening prompt and no history opens with the quiet scene
	// turn, so the TUI restates the scene the GUI shows in its Prologue.
	if engine.OpeningPrompt(manifest) != "" {
		if turns, err := history.LoadHistory(); err == nil && len(turns) == 0 {
			orchestrator.SetSceneOnly(true)
			if scene, err := orchestrator.ProcessAction(context.Background(), engine.OpeningMode, ""); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not establish the opening scene: %v\n", err)
			} else if scene != nil {
				app.Seed([]engine.Turn{*scene})
			}
		}
	}

	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
