package gui

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestRuntimeIsReusedUntilSomethingChanges(t *testing.T) {
	_, svc := turnFixture(t)
	paths := svc.GetResolver()
	manifestPath := filepath.Join(paths.GameDir("campaign-01"), "game.yaml")
	manifest, err := core.LoadGameManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	first, err := svc.runtimeFor("campaign-01", manifest)
	if err != nil {
		t.Fatalf("runtimeFor: %v", err)
	}
	second, err := svc.runtimeFor("campaign-01", manifest)
	if err != nil {
		t.Fatalf("runtimeFor: %v", err)
	}
	if first != second {
		t.Fatal("runtime was rebuilt with nothing changed")
	}

	// A config save must invalidate it.
	if err := svc.configMgr.Save(svc.configMgr.Get()); err != nil {
		t.Fatal(err)
	}
	third, err := svc.runtimeFor("campaign-01", manifest)
	if err != nil {
		t.Fatalf("runtimeFor: %v", err)
	}
	if third == second {
		t.Fatal("runtime was not rebuilt after a config change")
	}
}
