package media

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestArtStoreResolvesOnceAndReuses(t *testing.T) {
	client := &stubImageClient{body: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)}
	store := NewArtStore(client, NewContentCache(t.TempDir()), "dark fantasy", "builtin:")

	location := &entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Warm."}

	first, err := store.SceneArt(context.Background(), location, false)
	if err != nil {
		t.Fatalf("SceneArt failed: %v", err)
	}
	if filepath.Ext(first) != ".svg" {
		t.Errorf("path = %q, want an svg", first)
	}

	second, err := store.SceneArt(context.Background(), location, false)
	if err != nil {
		t.Fatalf("second SceneArt failed: %v", err)
	}
	if second != first {
		t.Errorf("expected a cache hit at %q, got %q", first, second)
	}
	if client.calls != 1 {
		t.Errorf("expected 1 provider call, got %d", client.calls)
	}

	if _, err := store.SceneArt(context.Background(), location, true); err != nil {
		t.Fatalf("forced SceneArt failed: %v", err)
	}
	if client.calls != 2 {
		t.Errorf("expected the force flag to regenerate, got %d calls", client.calls)
	}
}

func TestArtStoreContextIsHonoured(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := NewArtStore(&stubImageClient{body: []byte("<svg")}, NewContentCache(t.TempDir()), "", "")
	if _, err := store.SceneArt(ctx, &entity.Entity{ID: "x", Type: "location"}, false); err == nil {
		t.Errorf("expected a cancelled context to fail the call")
	}
}

func TestArtStoreRejectsAMissingLocation(t *testing.T) {
	store := NewArtStore(&stubImageClient{}, NewContentCache(t.TempDir()), "", "")
	if _, err := store.SceneArt(context.Background(), nil, false); err == nil {
		t.Errorf("expected an error when there is no location to illustrate")
	}
}
