package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func baseSubmission() *harness.TurnSubmission {
	return &harness.TurnSubmission{
		Verdict:  harness.ActionVerdict{Feasibility: harness.FeasibilityAutomatic, Reason: "safe"},
		Segments: []harness.SegmentSpec{{Kind: "narration", Text: "You walk on."}},
	}
}

func TestValidationRejectsChecksWhenOff(t *testing.T) {
	withCheck := baseSubmission()
	withCheck.Segments[0].CheckRef = "c1"
	err := validateSubmission(withCheck, []harness.CheckResult{{CheckID: "c1"}}, nil, nil, "off")
	if err == nil {
		t.Fatal("off policy should reject a resolved check")
	}
}

func TestValidationRejectsSelfResolvedCheckWhenAsking(t *testing.T) {
	withCheck := baseSubmission()
	withCheck.Verdict.Feasibility = harness.FeasibilityUncertain
	withCheck.Segments[0].CheckRef = "c1"
	err := validateSubmission(withCheck, []harness.CheckResult{{CheckID: "c1"}}, nil, nil, "ask")
	if err == nil {
		t.Fatal("ask policy should reject a model-resolved check")
	}
}

func TestToolSpecsPerPolicy(t *testing.T) {
	off := harness.TurnToolSpecsFor("off")
	for _, spec := range off {
		if spec.Name == "request_check" || spec.Name == "propose_check" {
			t.Errorf("off should not offer %q", spec.Name)
		}
	}
	ask := harness.TurnToolSpecsFor("ask")
	var hasPropose, hasRequest bool
	for _, spec := range ask {
		hasPropose = hasPropose || spec.Name == "propose_check"
		hasRequest = hasRequest || spec.Name == "request_check"
	}
	if !hasPropose || hasRequest {
		t.Errorf("ask tools wrong: propose=%v request=%v", hasPropose, hasRequest)
	}
}
