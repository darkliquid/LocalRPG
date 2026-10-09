package gui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveEndpointWritesNothing(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{`{}`}}
	svc := sysGenService(t, provider)

	draft, err := svc.DeriveSystem(context.Background(), SystemDeriveRequestDTO{
		BaseID:      "narrative_2d6",
		Instruction: "keep the base",
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft == nil {
		t.Fatal("expected a draft")
	}
	sysDir := svc.resolver.SystemDir(draft.ID)
	if _, err := os.Stat(filepath.Join(sysDir, "system.yaml")); err == nil {
		t.Fatal("deriving a system should not write systems/<id>/system.yaml")
	}
	if _, err := os.Stat(filepath.Join(svc.systemDraftsDir(), draft.ID+".yaml")); err != nil {
		t.Fatalf("a derived draft should be saved: %v", err)
	}
}

func TestDeriveEndpointUnknownBase(t *testing.T) {
	svc := sysGenService(t, &sequencedProvider{id: "gen"})
	_, err := svc.DeriveSystem(context.Background(), SystemDeriveRequestDTO{
		BaseID:      "no-such-base",
		Instruction: "x",
	})
	if !errors.Is(err, ErrReferenceSystemNotFound) {
		t.Fatalf("err = %v, want ErrReferenceSystemNotFound", err)
	}
}

func TestDeriveRequiresAnInstruction(t *testing.T) {
	svc := sysGenService(t, &sequencedProvider{id: "gen"})
	if _, err := svc.DeriveSystem(context.Background(), SystemDeriveRequestDTO{BaseID: "narrative_2d6"}); err == nil {
		t.Fatal("an empty instruction should error")
	}
}

func TestDeriveOffersTheBaseUnchangedWithNoProvider(t *testing.T) {
	// The oracle returns an empty object, so the base is offered as the draft and
	// its own scenarios run against it.
	svc := sysGenService(t, &sequencedProvider{id: "gen", responses: []string{`{}`}})
	draft, err := svc.DeriveSystem(context.Background(), SystemDeriveRequestDTO{
		BaseID:      "narrative_2d6",
		Instruction: "keep the base",
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Mechanics == nil || len(draft.Mechanics.Stats) != 3 {
		t.Fatalf("expected the base's three stats, got %+v", draft.Mechanics)
	}
	if !draft.Verify.OK {
		t.Fatalf("the base should verify, got %v", draft.Verify.Failures)
	}
}
