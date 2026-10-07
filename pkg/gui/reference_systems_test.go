package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/refsystems"
)

func TestReferenceSystemsEndpoint(t *testing.T) {
	_, svc := setupTestGame(t)
	got, err := svc.ListReferenceSystems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Systems) != 3 {
		t.Fatalf("systems = %d, want 3", len(got.Systems))
	}
}

func TestReferenceSystemsMatchTheEmbed(t *testing.T) {
	_, svc := setupTestGame(t)
	got, err := svc.ListReferenceSystems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := refsystems.List()
	if len(got.Systems) != len(want) {
		t.Fatalf("served %d systems, embed has %d", len(got.Systems), len(want))
	}
	for i, sys := range want {
		if got.Systems[i].ID != sys.ID || got.Systems[i].Script != sys.Script {
			t.Fatalf("system %d drifted: got %q want %q", i, got.Systems[i].ID, sys.ID)
		}
	}
}
