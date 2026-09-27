package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestQuietTurnsCountsTrailingEmptyChecks(t *testing.T) {
	turns := []Turn{
		{Number: 1, Checks: []harness.CheckResult{{CheckID: "a"}}},
		{Number: 2},
		{Number: 3},
	}
	if got := quietTurns(turns); got != 2 {
		t.Fatalf("quietTurns = %d, want 2", got)
	}
	if got := quietTurns([]Turn{{Number: 1, Checks: []harness.CheckResult{{CheckID: "a"}}}}); got != 0 {
		t.Fatalf("quietTurns with a recent check = %d, want 0", got)
	}
}

func TestShouldForceCheckOnlyInAutoAtCadence(t *testing.T) {
	turns := []Turn{{Number: 1}, {Number: 2}, {Number: 3}}
	if !shouldForceCheck("auto", 3, turns) {
		t.Fatal("auto at cadence should force a check")
	}
	if shouldForceCheck("auto", 4, turns) {
		t.Fatal("below cadence should not force")
	}
	if shouldForceCheck("ask", 3, turns) {
		t.Fatal("ask should never force: the player rolls")
	}
	if shouldForceCheck("off", 3, turns) {
		t.Fatal("off should never force")
	}
	if shouldForceCheck("auto", -1, turns) {
		t.Fatal("a negative cadence disables the floor")
	}
}
