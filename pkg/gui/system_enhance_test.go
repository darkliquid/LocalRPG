package gui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

// saveEnhanceFixture writes a system to enhance.
func saveEnhanceFixture(t *testing.T, svc *Service, id string) {
	t.Helper()
	_, err := svc.SaveSystem(context.Background(), CreateSystemRequestDTO{
		ID:   id,
		Name: "Enhance Me",
		Mechanics: &core.MechanicsSpec{
			Stats:  []core.StatSpec{{ID: "might"}},
			Checks: core.CheckConventions{Notation: "2d6"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func systemManifest(t *testing.T, svc *Service, id string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(svc.resolver.SystemDir(id), "system.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEnhanceSystemProposalsWriteNothing(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{
		`{"proposals":[{"kind":"stat","title":"Sanity","stat":{"id":"sanity"}}]}`,
	}}
	svc := sysGenService(t, provider)
	saveEnhanceFixture(t, svc, "enhance-me")
	before := systemManifest(t, svc, "enhance-me")

	resp, err := svc.EnhanceSystem(context.Background(), "enhance-me", SystemEnhanceRequestDTO{Instruction: "add sanity"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Proposals) != 1 || !resp.Proposals[0].Valid {
		t.Fatalf("proposals = %+v", resp.Proposals)
	}
	if after := systemManifest(t, svc, "enhance-me"); after != before {
		t.Fatal("enhancing should not write the system")
	}
}

func TestApplySystemEnhancementWritesOnlyAccepted(t *testing.T) {
	svc := sysGenService(t, &sequencedProvider{id: "gen"})
	saveEnhanceFixture(t, svc, "apply-me")

	result, err := svc.ApplySystemEnhancements(context.Background(), "apply-me", SystemEnhanceApplyRequestDTO{
		Proposals: []SystemProposalDTO{{Kind: "stat", Stat: &core.StatSpec{ID: "sanity"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Written) == 0 {
		t.Fatal("applying an accepted proposal should write")
	}
	detail, err := svc.GetSystem(context.Background(), "apply-me")
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]bool)
	for _, stat := range detail.Mechanics.Stats {
		ids[stat.ID] = true
	}
	if !ids["sanity"] || !ids["might"] {
		t.Fatalf("stats = %+v", detail.Mechanics.Stats)
	}
}

func TestApplySystemEnhancementRejectsABrokenProposal(t *testing.T) {
	svc := sysGenService(t, &sequencedProvider{id: "gen"})
	saveEnhanceFixture(t, svc, "broken-me")

	_, err := svc.ApplySystemEnhancements(context.Background(), "broken-me", SystemEnhanceApplyRequestDTO{
		Proposals: []SystemProposalDTO{{Kind: "skill", Skill: &core.SkillSpec{ID: "occult", Stat: "missing"}}},
	})
	if err == nil {
		t.Fatal("a dangling reference should be refused")
	}
	detail, err := svc.GetSystem(context.Background(), "broken-me")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Mechanics.Skills) != 0 {
		t.Fatalf("a refused enhancement should not write, got %+v", detail.Mechanics.Skills)
	}
}

func TestApplySystemEnhancementEmptyIsNoOp(t *testing.T) {
	svc := sysGenService(t, &sequencedProvider{id: "gen"})
	saveEnhanceFixture(t, svc, "noop-me")
	before := systemManifest(t, svc, "noop-me")

	result, err := svc.ApplySystemEnhancements(context.Background(), "noop-me", SystemEnhanceApplyRequestDTO{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Written) != 0 {
		t.Fatalf("applying nothing wrote %v", result.Written)
	}
	if after := systemManifest(t, svc, "noop-me"); after != before {
		t.Fatal("applying nothing should leave the system unchanged")
	}
}

func TestExplainSystemReturnsText(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{
		`{"explanation":"Roll 2d6 and add your stat."}`,
	}}
	svc := sysGenService(t, provider)
	saveEnhanceFixture(t, svc, "explain-me")

	resp, err := svc.ExplainSystem(context.Background(), "explain-me")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Explanation, "2d6") {
		t.Fatalf("explanation = %q", resp.Explanation)
	}
}
