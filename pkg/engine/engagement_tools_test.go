package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

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
