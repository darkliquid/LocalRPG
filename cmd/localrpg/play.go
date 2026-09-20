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

	dbPath := filepath.Join(gameDir, "cache", "index.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening game database %q: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer store.Close()

	historyPath := filepath.Join(gameDir, "history.jsonl")
	history := engine.NewHistoryLogger(historyPath)

	bridge := rules.NewHostBridge(store)
	jsEngine := rules.NewJSEngine(bridge)

	ruleLoader := rules.NewRuleLoader(paths, jsEngine)
	_ = ruleLoader.LoadRules(manifest.SystemID, manifest.WorldID)

	// Setup model router from global config
	router := harness.NewRouter()
	for role, roleCfg := range cfg.Agents.Roles {
		p, err := harness.NewModelProvider(role, harness.ProviderConfig{
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
		if err == nil {
			router.RegisterProvider(p)
			router.AssignRole(role, role)
		}
	}
	for role, fb := range cfg.Agents.Fallbacks {
		if fb != "" {
			router.SetFallback(role, fb)
		}
	}
	if _, err := router.GetProviderForRole("gm"); err != nil {
		router.RegisterProvider(harness.NewCLIProvider("default-echo", "echo", []string{}))
		router.AssignRole("gm", "default-echo")
	}

	orchestrator := engine.NewTurnOrchestrator(
		store,
		history,
		jsEngine,
		router,
		"tavern",
		manifest.Player,
	)
	orchestrator.LoadPrompts(paths, manifest.SystemID, manifest.WorldID)

	app := tui.NewAppModel(orchestrator, 80, 24)
	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
