package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/tui"
)

func handlePlayCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: localrpg play <game-id>")
		os.Exit(1)
	}

	cfgMgr := config.NewConfigManager()
	cfg, _ := cfgMgr.Load()

	gameID := args[0]
	paths := core.NewCustomPathResolver(cfg.Paths.Systems, cfg.Paths.Worlds, cfg.Paths.Games, cfg.Paths.Cache)
	gameDir := paths.GameDir(gameID)

	manifestPath := filepath.Join(gameDir, "game.yaml")
	manifest, err := core.LoadGameManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading game manifest %q: %v\n", manifestPath, err)
		os.Exit(1)
	}

	store, err := storage.OpenGameStore(paths, gameID)
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

	timeline := engine.NewTimeline(paths, store, history, gameID)
	if err := timeline.EnsureIndexed(); err != nil {
		fmt.Fprintf(os.Stderr, "Error indexing turns: %v\n", err)
		os.Exit(1)
	}

	// A campaign written before player_name existed holds a display name in
	// player:, which is repaired once here so the turn pipeline uses the real ID.
	playerID := manifest.Player
	if resolved, err := engine.RepairPlayerIdentity(paths, store, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not reconcile player identity: %v\n", err)
	} else if resolved != "" {
		playerID = resolved
	}

	startLocation, err := engine.ResolveStartLocation(paths, store, manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving start location: %v\n", err)
		os.Exit(1)
	}

	bridge := rules.NewHostBridge(store, timeline, playerID)
	jsEngine := rules.NewJSEngine(bridge)

	ruleLoader := rules.NewRuleLoader(paths, jsEngine)
	_ = ruleLoader.LoadRules(manifest.SystemID, manifest.WorldID)

	logger := buildTraceLogger(cfg, os.Args, paths.CacheDir())
	logger.SetGame(gameID)

	router, err := harness.RouterFromConfigWithLogger(cfg, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building model router: %v\n", err)
		os.Exit(1)
	}

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
	orchestrator.LoadPrompts(paths, manifest.SystemID, manifest.WorldID)

	app := tui.NewAppModel(orchestrator, 80, 24)
	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
