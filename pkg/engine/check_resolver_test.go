package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestDefaultCheckResolver(t *testing.T) {
	r := defaultCheckResolver{}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{
		Actor: "player", CheckKind: "skill", Notation: "1d6+10",
		Outcomes: map[string]string{"pass": "ok", "fail": "no"},
	}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Outcome != "pass" {
		t.Fatalf("outcome = %q, want pass", res.Outcome)
	}
	if res.CheckID == "" {
		t.Fatal("CheckID is empty")
	}
	if res.Roll == nil || res.Roll.Total < 11 {
		t.Fatalf("roll = %+v, want a total of at least 11", res.Roll)
	}
}
