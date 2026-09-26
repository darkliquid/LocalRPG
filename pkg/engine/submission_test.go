package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestValidateSubmission(t *testing.T) {
	base := func() *harness.TurnSubmission {
		return &harness.TurnSubmission{
			Verdict:  harness.ActionVerdict{Feasibility: harness.FeasibilityUncertain, Reason: "gap"},
			Segments: []harness.SegmentSpec{{Kind: "narration", Text: "You leap."}},
		}
	}
	if err := validateSubmission(base(), nil, nil, nil); err == nil {
		t.Fatal("uncertain with no check should fail")
	}
	sub := base()
	sub.Segments[0].CheckRef = "1"
	if err := validateSubmission(sub, []harness.CheckResult{{CheckID: "1"}}, nil, nil); err != nil {
		t.Fatalf("valid submission rejected: %v", err)
	}
	imp := base()
	imp.Verdict.Feasibility = harness.FeasibilityImpossible
	if err := validateSubmission(imp, []harness.CheckResult{{CheckID: "1"}}, nil, nil); err == nil {
		t.Fatal("impossible with a check should fail")
	}
	unknown := base()
	unknown.Segments[0].CheckRef = "9"
	if err := validateSubmission(unknown, []harness.CheckResult{{CheckID: "1"}}, nil, nil); err == nil {
		t.Fatal("unknown check ref should fail")
	}
	stat := base()
	stat.Segments[0].CheckRef = "1"
	stat.StateChanges = []harness.StateChangeDecl{{Entity: "player", Path: "gold", Op: "set", Value: 1}}
	if err := validateSubmission(stat, []harness.CheckResult{{CheckID: "1"}}, map[string]core.StatSpec{"hp": {ID: "hp"}}, nil); err == nil {
		t.Fatal("undeclared stat should fail under a declared schema")
	}
}

func TestValidateSubmissionProposedCheck(t *testing.T) {
	proposed := &harness.ProposedCheck{Ref: "player-roll", Actor: "player", Description: "pick the lock"}

	base := func() *harness.TurnSubmission {
		return &harness.TurnSubmission{
			Verdict:  harness.ActionVerdict{Feasibility: harness.FeasibilityUncertain, Reason: "gap"},
			Segments: []harness.SegmentSpec{{Kind: "narration", Text: "You try."}},
		}
	}

	if err := validateSubmission(base(), nil, nil, proposed); err == nil {
		t.Fatal("proposed check with no resolution or dismissal should fail")
	}

	resolved := base()
	resolved.Segments[0].CheckRef = "c1"
	if err := validateSubmission(resolved, []harness.CheckResult{{CheckID: "c1"}}, nil, proposed); err != nil {
		t.Fatalf("resolved proposed check rejected: %v", err)
	}

	dismissed := base()
	dismissed.Verdict = harness.ActionVerdict{Feasibility: harness.FeasibilityAutomatic, Reason: "safe"}
	dismissed.DismissedChecks = []harness.DismissedCheck{{CheckRef: "player-roll", Reason: "no risk"}}
	if err := validateSubmission(dismissed, nil, nil, proposed); err != nil {
		t.Fatalf("dismissed proposed check rejected: %v", err)
	}

	wrongRef := base()
	wrongRef.Verdict = harness.ActionVerdict{Feasibility: harness.FeasibilityAutomatic, Reason: "safe"}
	wrongRef.DismissedChecks = []harness.DismissedCheck{{CheckRef: "other", Reason: "no risk"}}
	if err := validateSubmission(wrongRef, nil, nil, proposed); err == nil {
		t.Fatal("dismissal with the wrong ref should fail")
	}
}
