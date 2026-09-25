package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestValidateSubmission(t *testing.T) {
	base := func() *harness.TurnSubmission {
		return &harness.TurnSubmission{
			Verdict:  harness.ActionVerdict{Feasibility: harness.FeasibilityUncertain, Reason: "gap"},
			Segments: []harness.SegmentSpec{{Kind: "narration", Text: "You leap."}},
		}
	}
	if err := validateSubmission(base(), nil, nil); err == nil {
		t.Fatal("uncertain with no check should fail")
	}
	sub := base()
	sub.Segments[0].CheckRef = "1"
	if err := validateSubmission(sub, []harness.CheckResult{{CheckID: "1"}}, nil); err != nil {
		t.Fatalf("valid submission rejected: %v", err)
	}
	imp := base()
	imp.Verdict.Feasibility = harness.FeasibilityImpossible
	if err := validateSubmission(imp, []harness.CheckResult{{CheckID: "1"}}, nil); err == nil {
		t.Fatal("impossible with a check should fail")
	}
	unknown := base()
	unknown.Segments[0].CheckRef = "9"
	if err := validateSubmission(unknown, []harness.CheckResult{{CheckID: "1"}}, nil); err == nil {
		t.Fatal("unknown check ref should fail")
	}
	stat := base()
	stat.Segments[0].CheckRef = "1"
	stat.StateChanges = []harness.StateChangeDecl{{Entity: "player", Path: "gold", Op: "set", Value: 1}}
	if err := validateSubmission(stat, []harness.CheckResult{{CheckID: "1"}}, map[string]bool{"hp": true}); err == nil {
		t.Fatal("undeclared stat should fail under a declared schema")
	}
}
