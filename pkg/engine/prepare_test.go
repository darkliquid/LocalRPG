package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestPrepareCampaignResolvesStartLocationAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	paths := core.NewCustomPathResolver(
		filepath.Join(root, "systems"),
		filepath.Join(root, "worlds"),
		filepath.Join(root, "games"),
		filepath.Join(root, "cache"),
	)
	entitiesDir := filepath.Join(paths.GameDir("campaign"), "entities")
	if err := os.MkdirAll(entitiesDir, 0o755); err != nil {
		t.Fatalf("mkdir entities: %v", err)
	}

	store := newTestStore(t)
	saveTestEntity(t, store, &entity.Entity{ID: "market", Name: "Old Market", Type: "location", Hash: "hash-market"})

	manifest := &core.GameManifest{
		ID:       "campaign",
		WorldID:  "realm",
		Player:   "hero",
		Settings: map[string]any{StartLocationSetting: "market"},
	}
	history := NewHistoryLogger(filepath.Join(paths.GameDir("campaign"), "history.jsonl"))
	timeline := NewTimeline(paths, store, history, "campaign")

	got, err := PrepareCampaign(paths, store, timeline, manifest, entitiesDir)
	if err != nil {
		t.Fatalf("PrepareCampaign failed: %v", err)
	}
	if got.StartLocation != "market" {
		t.Errorf("StartLocation = %q, want market", got.StartLocation)
	}
	if got.PlayerID == "" {
		t.Errorf("PlayerID is empty")
	}

	if _, err := PrepareCampaign(paths, store, timeline, manifest, entitiesDir); err != nil {
		t.Fatalf("second PrepareCampaign failed (must be idempotent): %v", err)
	}
}
