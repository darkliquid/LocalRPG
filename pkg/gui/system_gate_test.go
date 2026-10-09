package gui

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/sysgen"
)

const brokenScript = `onAction("do", (`

func TestSaveSystemWarnsOnFailedSmoke(t *testing.T) {
	svc := NewService(t.TempDir())
	detail, err := svc.SaveSystem(context.Background(), CreateSystemRequestDTO{
		ID:     "broken-system",
		Name:   "Broken System",
		Script: brokenScript,
	})
	if err != nil {
		t.Fatalf("a hand-authored system should save with a warning, not fail: %v", err)
	}
	if len(detail.Warnings) == 0 {
		t.Fatal("a system that fails the smoke test should warn")
	}
	if !strings.Contains(strings.Join(detail.Warnings, " "), "smoke test") {
		t.Fatalf("warnings = %v", detail.Warnings)
	}
}

func TestSaveSystemStrictBlocks(t *testing.T) {
	svc := NewService(t.TempDir())
	_, err := svc.SaveSystem(context.Background(), CreateSystemRequestDTO{
		ID:     "broken-system",
		Name:   "Broken System",
		Script: brokenScript,
		Strict: true,
	})
	if err == nil {
		t.Fatal("strict mode should refuse a system that fails the smoke test")
	}
	if _, err := svc.GetSystem(context.Background(), "broken-system"); err == nil {
		t.Fatal("a blocked save should not write the system")
	}
}

func TestSaveSystemGoodIsSilent(t *testing.T) {
	svc := NewService(t.TempDir())
	detail, err := svc.SaveSystem(context.Background(), CreateSystemRequestDTO{
		ID:   "good-system",
		Name: "Good System",
		Mechanics: &core.MechanicsSpec{Checks: core.CheckConventions{
			Notation: "2d6",
			Profiles: map[string]core.ResolutionProfile{"pbta": {Ladder: []core.LadderStep{
				{Min: 10, Outcome: "strong"}, {Min: 0, Outcome: "miss"},
			}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Warnings) != 0 {
		t.Fatalf("a good system should save silently, got %v", detail.Warnings)
	}
}

func TestSaveSystemSchemaAgnosticIsSilent(t *testing.T) {
	svc := NewService(t.TempDir())
	detail, err := svc.SaveSystem(context.Background(), CreateSystemRequestDTO{
		ID:   "schema-agnostic",
		Name: "Schema Agnostic",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Warnings) != 0 {
		t.Fatalf("a schema-agnostic system should save silently, got %v", detail.Warnings)
	}
}

func TestGeneratedSystemAcceptBlocksOnFailure(t *testing.T) {
	svc := NewService(t.TempDir())
	draft := sysgen.System{
		ID:     "generated-broken",
		Name:   "Generated Broken",
		Script: brokenScript,
	}
	if err := sysgen.SaveDraft(svc.systemDraftsDir(), draft); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CommitSystemDraft(context.Background(), SystemDraftCommitRequestDTO{
		DraftID: "generated-broken",
	}); err == nil {
		t.Fatal("a failing generated draft must not be accepted")
	}
}
