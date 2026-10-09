package sysgen

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestGatePassesAGoodSystem(t *testing.T) {
	sys := System{Script: `onAction("do", () => ({ outcome: "miss" }))`, Mechanics: &core.MechanicsSpec{}}
	if got := Gate(sys); !got.OK {
		t.Fatalf("gate = %+v", got)
	}
}

func TestGateFailsABrokenScript(t *testing.T) {
	sys := System{Script: `onAction("do", (`}
	got := Gate(sys)
	if got.OK {
		t.Fatal("a broken script should fail the gate")
	}
	if got.FailureText() == "" {
		t.Fatal("a failed gate should explain why")
	}
}

func TestGateTriviallyPassesSchemaAgnostic(t *testing.T) {
	if got := Gate(System{}); !got.OK {
		t.Fatalf("gate = %+v", got)
	}
}

func TestGateFailsAProfileThatDoesNotResolve(t *testing.T) {
	sys := System{Mechanics: &core.MechanicsSpec{Checks: core.CheckConventions{
		Notation: "2d6",
		Profiles: map[string]core.ResolutionProfile{"broken": {Notation: "not-dice"}},
	}}}
	if got := Gate(sys); got.OK {
		t.Fatal("a profile that does not resolve should fail the gate")
	}
}

func TestGateReportsWhetherAScriptIsPresent(t *testing.T) {
	if got := Gate(System{Script: "onAction('do', () => ({}));"}); !got.Script {
		t.Fatal("a system with a script should report one")
	}
	if got := Gate(System{}); got.Script {
		t.Fatal("a system with no script should report none")
	}
}
