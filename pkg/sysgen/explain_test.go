package sysgen

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestExplainReturnsText(t *testing.T) {
	g := &jsonGen{responses: []string{`{"explanation":"Roll 2d6 and add Edge."}`}}
	got, err := Explain(context.Background(), g, System{ID: "s", Mechanics: &core.MechanicsSpec{}})
	if err != nil || got == "" {
		t.Fatalf("explanation %q err %v", got, err)
	}
}

func TestExplainRejectsNoGenerator(t *testing.T) {
	if _, err := Explain(context.Background(), nil, System{}); err == nil {
		t.Fatal("a nil generator should error")
	}
}
