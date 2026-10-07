package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/refsystems"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// referenceSystemStore builds a store holding one player with the stats every
// reference system's script reads, so the onAction hooks resolve real values.
func referenceSystemStore(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	entitiesDir := filepath.Join(dir, "entities")
	if err := os.MkdirAll(entitiesDir, 0755); err != nil {
		t.Fatal(err)
	}
	player := "---\nid: hero\nname: Hero\ntype: character\nstate:\n  might: 1\n  edge: 1\n  heart: 1\n  strength: 12\n  dexterity: 10\n  dice: 5\n  grit: 2\n---\nA hero.\n"
	if err := os.WriteFile(filepath.Join(entitiesDir, "hero.md"), []byte(player), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewStore(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}
	return store
}

func referenceEngine(s refsystems.ReferenceSystem, store *storage.Store) (*rules.JSEngine, error) {
	engine := rules.NewJSEngine(rules.NewHostBridge(store, nil, "hero"))
	engine.SetManifest(&core.SystemManifest{ID: s.ID, Name: s.Name, Mechanics: s.Mechanics})
	if err := engine.LoadScript(s.Script); err != nil {
		return nil, err
	}
	return engine, nil
}

func TestEveryReferenceSystemResolvesACheck(t *testing.T) {
	for _, s := range refsystems.List() {
		engine, err := referenceEngine(s, nil)
		if err != nil {
			t.Errorf("%s script: %v", s.ID, err)
			continue
		}
		var profile string
		for name := range s.Mechanics.Checks.Profiles {
			profile = name
			break
		}
		res, err := engine.Resolve(context.Background(), harness.CheckRequest{Profile: profile}, nil)
		if err != nil {
			t.Errorf("%s resolve: %v", s.ID, err)
			continue
		}
		allowed := map[string]bool{}
		for _, outcome := range s.Mechanics.Checks.Outcome {
			allowed[outcome] = true
		}
		if res.Outcome == "" || !allowed[res.Outcome] {
			t.Errorf("%s resolved to %q, not one of %v", s.ID, res.Outcome, s.Mechanics.Checks.Outcome)
		}
	}
}

func TestEveryReferenceSystemRunsItsAction(t *testing.T) {
	store := referenceSystemStore(t)
	for _, s := range refsystems.List() {
		engine, err := referenceEngine(s, store)
		if err != nil {
			t.Errorf("%s script: %v", s.ID, err)
			continue
		}
		result, err := engine.ExecuteAction("do", map[string]interface{}{"player": "hero", "turn": 1, "action": "climb"})
		if err != nil {
			t.Errorf("%s action: %v", s.ID, err)
			continue
		}
		if result == nil || result.Outcome == "" {
			t.Errorf("%s action returned no outcome: %+v", s.ID, result)
		}
	}
}
