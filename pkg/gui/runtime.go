package gui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// turnRuntime is the per-campaign wiring that does not depend on the turn. It is
// rebuilt only when the configuration revision or an on-disk source changes, so
// hand edits still take effect on the next turn.
type turnRuntime struct {
	router          *harness.Router
	rulesPrompt     string
	lorePrompt      string
	mechanicsPrompt string
	mechanics       *core.MechanicsSpec
	engagement      string
	declaredStats   map[string]core.StatSpec
	allowFreeform   bool
}

// runtimeKey identifies the inputs a runtime was built from. Any change to one
// of these mtimes, or to the config revision, invalidates the cache.
type runtimeKey struct {
	revision  uint64
	game      int64
	system    int64
	mechanics int64
	hooks     int64
	rules     int64
	lore      int64
}

func fileMtime(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

// runtimeFor returns the cached turn wiring, rebuilding it only when the config
// revision or a source file mtime changed.
func (s *Service) runtimeFor(gameID string, manifest *core.GameManifest) (*turnRuntime, error) {
	sysDir := s.resolver.SystemDir(manifest.SystemID)
	worldDir := s.resolver.WorldDir(manifest.WorldID)
	key := runtimeKey{
		revision:  s.configMgr.Revision(),
		game:      fileMtime(filepath.Join(s.resolver.GameDir(gameID), "game.yaml")),
		system:    fileMtime(filepath.Join(sysDir, "system.yaml")),
		mechanics: fileMtime(filepath.Join(sysDir, "mechanics.js")),
		hooks:     fileMtime(filepath.Join(worldDir, "system_overrides", manifest.SystemID, "hooks.js")),
		rules:     fileMtime(filepath.Join(sysDir, "prompts", "rules.md")),
		lore:      fileMtime(filepath.Join(worldDir, "prompts", "lore.md")),
	}

	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.runtime != nil && s.runtimeGame == gameID && s.runtimeKey == key {
		return s.runtime, nil
	}

	cfg := s.configMgr.Get()
	router, err := harness.RouterFromConfigWithLogger(cfg, trace.OrNil(s.logger))
	if err != nil {
		return nil, fmt.Errorf("build router: %w", err)
	}

	runtime := &turnRuntime{router: router}
	if data, err := os.ReadFile(filepath.Join(sysDir, "prompts", "rules.md")); err == nil {
		runtime.rulesPrompt = string(data)
	}
	if data, err := os.ReadFile(filepath.Join(worldDir, "prompts", "lore.md")); err == nil {
		runtime.lorePrompt = string(data)
	}

	sm, _ := core.LoadSystemManifest(filepath.Join(sysDir, "system.yaml"))
	runtime.engagement = engine.ResolveEngagement(manifest, sm, cfg)
	if sm != nil && sm.Mechanics != nil {
		runtime.mechanics = sm.Mechanics
		runtime.declaredStats = make(map[string]core.StatSpec, len(sm.Mechanics.Stats))
		for _, stat := range sm.Mechanics.Stats {
			runtime.declaredStats[stat.ID] = stat
		}
		runtime.allowFreeform = sm.Mechanics.AllowFreeformState
		runtime.mechanicsPrompt = harness.FormatMechanicsInstructions(sm.Mechanics, runtime.engagement)
	} else if _, err := os.Stat(filepath.Join(sysDir, "mechanics.js")); err == nil {
		runtime.mechanicsPrompt = harness.FormatMechanicsInstructions(nil, runtime.engagement)
	}

	s.runtime, s.runtimeKey, s.runtimeGame = runtime, key, gameID
	return runtime, nil
}

// cachedHistory returns the parsed timeline, re-reading history.jsonl only when
// its size or mtime changed. The log stays canonical; this is a read-through.
func (s *Service) cachedHistory(gameID string) ([]engine.Turn, error) {
	path := filepath.Join(s.resolver.GameDir(gameID), "history.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	if s.historyCache == nil {
		s.historyCache = map[string][]engine.Turn{}
		s.historySize = map[string]int64{}
		s.historyMtime = map[string]int64{}
	}
	if turns, ok := s.historyCache[gameID]; ok && s.historySize[gameID] == info.Size() && s.historyMtime[gameID] == info.ModTime().UnixNano() {
		return turns, nil
	}

	turns, err := engine.NewHistoryLogger(path).LoadHistory()
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	s.historyCache[gameID] = turns
	s.historySize[gameID] = info.Size()
	s.historyMtime[gameID] = info.ModTime().UnixNano()
	return turns, nil
}
