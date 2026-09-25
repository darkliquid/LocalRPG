package rules

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestHostBridgeExposesSchema(t *testing.T) {
	bridge := NewHostBridge(nil, nil, "player")
	bridge.SetManifest(&core.SystemManifest{Mechanics: &core.MechanicsSpec{
		Stats:  []core.StatSpec{{ID: "might"}},
		Skills: []core.SkillSpec{{ID: "athletics", Stat: "might"}},
		Checks: core.CheckConventions{Notation: "2d6"},
	}})
	if got := bridge.ListStats(); len(got) != 1 || got[0].ID != "might" {
		t.Fatalf("ListStats = %+v", got)
	}
	if got := bridge.ListSkills(); len(got) != 1 || got[0].Stat != "might" {
		t.Fatalf("ListSkills = %+v", got)
	}
	if got := bridge.CheckConventions(); got.Notation != "2d6" {
		t.Fatalf("CheckConventions = %+v", got)
	}
}

func TestHostBridgeWithoutSchema(t *testing.T) {
	bridge := NewHostBridge(nil, nil, "player")
	if got := bridge.ListStats(); got != nil {
		t.Fatalf("ListStats = %+v, want nil", got)
	}
	if got := bridge.CheckConventions(); got.Notation != "" {
		t.Fatalf("CheckConventions = %+v, want empty", got)
	}
}
