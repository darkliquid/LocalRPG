package gui

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestSystemMechanicsRoundTrip(t *testing.T) {
	_, svc := setupTestGame(t)
	req := CreateSystemRequestDTO{
		ID: "test_sys", Name: "Test", Version: "1",
		Mechanics: &core.MechanicsSpec{Stats: []core.StatSpec{{ID: "might"}}},
	}
	if _, err := svc.SaveSystem(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetSystem(context.Background(), "test_sys")
	if err != nil {
		t.Fatal(err)
	}
	if got.Mechanics == nil || len(got.Mechanics.Stats) != 1 {
		t.Fatalf("mechanics = %+v", got.Mechanics)
	}
}

func TestSaveSystemWarnsOnBadMechanics(t *testing.T) {
	_, svc := setupTestGame(t)
	req := CreateSystemRequestDTO{ID: "bad", Name: "Bad", Version: "1",
		Mechanics: &core.MechanicsSpec{Skills: []core.SkillSpec{{ID: "stealth", Stat: "missing"}}}}
	res, err := svc.SaveSystem(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("a skill naming an unknown stat should warn")
	}
}

func TestSaveSystemWithoutMechanicsWritesNoKey(t *testing.T) {
	_, svc := setupTestGame(t)
	req := CreateSystemRequestDTO{ID: "plain", Name: "Plain", Version: "1"}
	if _, err := svc.SaveSystem(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetSystem(context.Background(), "plain")
	if err != nil {
		t.Fatal(err)
	}
	if got.Mechanics != nil {
		t.Fatalf("a system saved without mechanics should carry none, got %+v", got.Mechanics)
	}
}

func TestSystemMechanicsRoundTripsByteIdentical(t *testing.T) {
	_, svc := setupTestGame(t)
	full := &core.MechanicsSpec{
		Stats:  []core.StatSpec{{ID: "might", Label: "Might", Type: "number"}},
		Skills: []core.SkillSpec{{ID: "stealth", Label: "Stealth", Stat: "might"}},
		Health: &core.HealthSpec{Stat: "might", ZeroEffect: "incapacitated"},
		Checks: core.CheckConventions{
			Notation:   "2d6",
			Outcome:    []string{"strong", "weak", "miss"},
			Difficulty: []core.DifficultySpec{{ID: "hard", Target: 12}},
			Profiles:   map[string]core.ResolutionProfile{"pbta": {Ladder: []core.LadderStep{{Min: 10, Outcome: "strong"}}}},
		},
		AllowFreeformState: true,
		Engagement:         "auto",
		Advancement:        &core.AdvancementSpec{Currency: core.CurrencySpec{Stat: "might"}, Mode: "spend"},
	}
	ctx := context.Background()
	if _, err := svc.SaveSystem(ctx, CreateSystemRequestDTO{ID: "full", Name: "Full", Version: "1", Mechanics: full}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(svc.resolver.SystemDir("full"), "system.yaml")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.GetSystem(ctx, "full")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveSystem(ctx, CreateSystemRequestDTO{
		ID: got.ID, Name: got.Name, Version: got.Version, Description: got.Description,
		Script: got.Script, RulesPrompt: got.RulesPrompt, CharacterCreation: got.CharacterCreation,
		Mechanics: got.Mechanics,
	}); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("system.yaml changed on round trip:\n%s\n---\n%s", first, second)
	}
}
