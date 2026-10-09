package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/turnstream"
)

func TestClassifyReply(t *testing.T) {
	if classifyReply(streamResult{Text: "A valid reply."}, turnstream.RepairReport{}) != ProblemNone {
		t.Fatal("a valid reply is not a problem")
	}
	if classifyReply(streamResult{Text: ""}, turnstream.RepairReport{}) != ProblemMalformed {
		t.Fatal("an empty reply is malformed")
	}
	if classifyReply(streamResult{Text: "Half a sent", FinishReason: "length"}, turnstream.RepairReport{}) != ProblemCut {
		t.Fatal("a length finish is a cut")
	}
}
