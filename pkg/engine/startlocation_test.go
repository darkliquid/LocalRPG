package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()

	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func saveTestEntity(t *testing.T, store *storage.Store, ent *entity.Entity) {
	t.Helper()

	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("SaveEntity(%q) failed: %v", ent.ID, err)
	}
}

func TestResolveStartLocationPrefersPinnedSetting(t *testing.T) {
	store := newTestStore(t)
	saveTestEntity(t, store, &entity.Entity{ID: "harbour", Name: "The Harbour", Type: "location", Hash: "hash-harbour"})
	saveTestEntity(t, store, &entity.Entity{ID: "market", Name: "Old Market", Type: "location", Hash: "hash-market"})

	manifest := &core.GameManifest{
		ID:       "campaign",
		WorldID:  "realm",
		Player:   "hero",
		Settings: map[string]any{StartLocationSetting: "market"},
	}

	got, err := ResolveStartLocation(nil, store, manifest)
	if err != nil {
		t.Fatalf("ResolveStartLocation failed: %v", err)
	}
	if got != "market" {
		t.Errorf("expected pinned start location market, got %q", got)
	}
}

func TestResolveStartLocationUsesPlayerLocationReference(t *testing.T) {
	store := newTestStore(t)
	saveTestEntity(t, store, &entity.Entity{ID: "harbour", Name: "The Harbour", Type: "location", Hash: "hash-harbour"})
	saveTestEntity(t, store, &entity.Entity{ID: "market", Name: "Old Market", Type: "location", Hash: "hash-market"})
	saveTestEntity(t, store, &entity.Entity{
		ID:       "hero",
		Name:     "Hero",
		Type:     "character",
		Location: "[[Old Market|the old market]]",
		Hash:     "hash-hero",
	})

	manifest := &core.GameManifest{ID: "campaign", Player: "hero"}

	got, err := ResolveStartLocation(nil, store, manifest)
	if err != nil {
		t.Fatalf("ResolveStartLocation failed: %v", err)
	}
	if got != "market" {
		t.Errorf("expected player location market, got %q", got)
	}
}

func TestResolveStartLocationFallsBackToIndexedLocation(t *testing.T) {
	store := newTestStore(t)
	saveTestEntity(t, store, &entity.Entity{ID: "abbey", Name: "Silent Abbey", Type: "location", Hash: "hash-abbey"})
	saveTestEntity(t, store, &entity.Entity{ID: "hero", Name: "Hero", Type: "character", Hash: "hash-hero"})

	manifest := &core.GameManifest{ID: "campaign", Player: "hero"}

	got, err := ResolveStartLocation(nil, store, manifest)
	if err != nil {
		t.Fatalf("ResolveStartLocation failed: %v", err)
	}
	if got != "abbey" {
		t.Errorf("expected indexed location abbey, got %q", got)
	}
}

func TestResolveStartLocationCreatesOpeningSceneFromWorld(t *testing.T) {
	tempDir := t.TempDir()
	paths := core.NewPathResolver(tempDir)

	worldDir := paths.WorldDir("ashen-reach")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	worldYAML := "id: ashen-reach\nname: Ashen Reach\ndescription: A land of cold embers and long roads.\n"
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatal(err)
	}

	store := newTestStore(t)
	manifest := &core.GameManifest{ID: "campaign-01", WorldID: "ashen-reach", Player: "hero"}

	got, err := ResolveStartLocation(paths, store, manifest)
	if err != nil {
		t.Fatalf("ResolveStartLocation failed: %v", err)
	}
	if got != OpeningSceneEntityID {
		t.Fatalf("expected generated %q location, got %q", OpeningSceneEntityID, got)
	}

	generated, err := store.GetEntity(OpeningSceneEntityID)
	if err != nil {
		t.Fatalf("generated location missing from index: %v", err)
	}
	if generated.Type != "location" {
		t.Errorf("expected generated entity type location, got %q", generated.Type)
	}
	if generated.Name != "Ashen Reach" {
		t.Errorf("expected generated location to take the world name, got %q", generated.Name)
	}
	if !strings.Contains(generated.Body, "cold embers") {
		t.Errorf("expected generated body to describe the opening scene, got %q", generated.Body)
	}

	markdownPath := filepath.Join(paths.GameDir("campaign-01"), "entities", OpeningSceneEntityID+".md")
	if _, err := os.Stat(markdownPath); err != nil {
		t.Errorf("expected opening scene markdown on disk: %v", err)
	}

	again, err := ResolveStartLocation(paths, store, manifest)
	if err != nil {
		t.Fatalf("second ResolveStartLocation failed: %v", err)
	}
	if again != got {
		t.Errorf("expected stable start location %q, got %q", got, again)
	}
}
