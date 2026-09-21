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

	startLocation, err := engine.ResolveStartLocation(paths, store, manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving start location: %v\n", err)
		os.Exit(1)
	}

	bridge := rules.NewHostBridge(store, timeline, manifest.Player)
	jsEngine := rules.NewJSEngine(bridge)

	ruleLoader := rules.NewRuleLoader(paths, jsEngine)
	_ = ruleLoader.LoadRules(manifest.SystemID, manifest.WorldID)

	router, err := harness.RouterFromConfig(cfg)
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
		manifest.Player,
	)
	orchestrator.SetExtractor(resolveExtractor(cfg, router))
	orchestrator.LoadPrompts(paths, manifest.SystemID, manifest.WorldID)

	app := tui.NewAppModel(orchestrator, 80, 24)
	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// resolveExtractor picks the provider used for per-turn entity extraction. An
// absent role still inherits gm, so configuration written before this role existed
// keeps working; `disabled` opts out; `inherit` follows the named role, which is
// what stops extraction silently pointing at a stale copy of gm.
func resolveExtractor(cfg *config.Config, router *harness.Router) *harness.Extractor {
	roleCfg, configured := cfg.Agents.Roles[config.RoleExtractor]
	if !configured {
		roleCfg = config.AgentRoleConfig{Type: "inherit", InheritFrom: config.RoleGM}
	}

	switch roleCfg.Type {
	case "disabled":
		return nil
	case "inherit", "":
		source := roleCfg.InheritFrom
		if source == "" {
			source = config.RoleGM
		}

		provider, err := router.GetProviderForRole(source)
		if err != nil {
			return nil
		}
		return harness.NewExtractor(provider)
	}

	provider, err := harness.NewModelProvider(config.RoleExtractor, harness.ProviderConfig{
		Type:        roleCfg.Type,
		BuiltinName: roleCfg.BuiltinName,
		Command:     roleCfg.Command,
		Args:        roleCfg.Args,
		Endpoint:    roleCfg.Endpoint,
		Model:       roleCfg.Model,
		APIKey:      roleCfg.APIKey,
		Temperature: roleCfg.Temperature,
		MaxTokens:   roleCfg.MaxTokens,
	})
	if err != nil {
		return nil
	}
	return harness.NewExtractor(provider)
}
