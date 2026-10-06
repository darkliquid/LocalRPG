package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/paths"
	"github.com/darkliquid/localrpg/pkg/refsystems"
	"github.com/darkliquid/localrpg/pkg/systemtest"
)

// runTestSystem loads a system and its scenarios, runs them, and prints any
// failures. It returns the process exit code: non-zero on any failure.
func runTestSystem(cfg debugConfig) int {
	sys, scenarios, err := loadSystemForTest(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "test-system: %v\n", err)
		return 1
	}
	if len(scenarios) == 0 {
		fmt.Printf("no scenarios for %s\n", sys.ID)
		return 0
	}
	failures := systemtest.RunAll(sys, scenarios)
	if len(failures) == 0 {
		fmt.Printf("%s: %d scenario(s) passed\n", sys.ID, len(scenarios))
		return 0
	}
	for _, f := range failures {
		fmt.Printf("FAIL %s step %d: %s\n", f.Scenario, f.Step, f.Detail)
	}
	fmt.Printf("%s: %d failure(s)\n", sys.ID, len(failures))
	return 1
}

func loadSystemForTest(cfg debugConfig) (systemtest.System, []systemtest.Scenario, error) {
	if cfg.Reference {
		ref, ok := refsystems.Get(cfg.SystemID)
		if !ok {
			return systemtest.System{}, nil, fmt.Errorf("unknown reference system %q", cfg.SystemID)
		}
		sys := systemtest.System{ID: ref.ID, Script: ref.Script, Mechanics: ref.Mechanics}
		var scenarios []systemtest.Scenario
		for _, file := range refsystems.ScenarioFiles(ref.ID) {
			scenario, err := systemtest.LoadScenario(file.Data)
			if err != nil {
				return sys, nil, err
			}
			if cfg.Scenario != "" && scenario.Name != cfg.Scenario {
				continue
			}
			scenarios = append(scenarios, scenario)
		}
		return sys, scenarios, nil
	}

	if cfg.SystemID == "" {
		return systemtest.System{}, nil, fmt.Errorf("a system id is required")
	}
	cfgMgr := config.NewConfigManager()
	loaded, err := cfgMgr.Load()
	if err != nil {
		return systemtest.System{}, nil, fmt.Errorf("load config: %w", err)
	}
	dirs := paths.Resolve(paths.System(), loaded.Paths, "")
	sysDir := filepath.Join(dirs.Systems, cfg.SystemID)
	manifest, err := core.LoadSystemManifest(filepath.Join(sysDir, "system.yaml"))
	if err != nil {
		return systemtest.System{}, nil, fmt.Errorf("load system: %w", err)
	}
	script, _ := os.ReadFile(filepath.Join(sysDir, "mechanics.js"))
	sys := systemtest.System{ID: manifest.ID, Script: string(script), Mechanics: manifest.Mechanics}

	entries, err := os.ReadDir(filepath.Join(sysDir, "tests"))
	if err != nil {
		return sys, nil, nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	var scenarios []systemtest.Scenario
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(sysDir, "tests", name))
		if err != nil {
			return sys, nil, err
		}
		scenario, err := systemtest.LoadScenario(data)
		if err != nil {
			return sys, nil, err
		}
		if cfg.Scenario != "" && scenario.Name != cfg.Scenario {
			continue
		}
		scenarios = append(scenarios, scenario)
	}
	return sys, scenarios, nil
}
