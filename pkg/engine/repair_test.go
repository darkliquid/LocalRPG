package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
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

func TestRepairInstructionIsSpecific(t *testing.T) {
	if !strings.Contains(repairInstruction(ProblemMalformed, "no roll"), "@roll") {
		t.Fatal("a missing roll should name the record")
	}
	if !strings.Contains(repairInstruction(ProblemMalformed, ""), "empty") {
		t.Fatal("a generic malformed reply should say so")
	}
}

func TestEmptyReplyIsRetriedOnce(t *testing.T) {
	// The stub provider returns empty then a valid reply; the turn uses the second.
	callCount := 0
	provider := &scriptedStreamProvider{
		chunksPerCall: [][]string{
			{""},
			{"The cavern is cold and dark."},
		},
		onRequest: func(req harness.GenerateRequest) {
			callCount++
		},
	}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", nil)
	if err != nil {
		t.Fatalf("expected successful turn after retry, got: %v", err)
	}
	if turn.Narration != "The cavern is cold and dark." {
		t.Fatalf("turn narration = %q, want %q", turn.Narration, "The cavern is cold and dark.")
	}
	if callCount != 2 {
		t.Fatalf("expected 2 calls, got %d", callCount)
	}
}

func TestMalformedTwiceAborts(t *testing.T) {
	// Two malformed replies abort, recording two attempts.
	callCount := 0
	provider := &scriptedStreamProvider{
		chunksPerCall: [][]string{
			{""},
			{""},
			{""},
		},
		onRequest: func(req harness.GenerateRequest) {
			callCount++
		},
	}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	orchestrator.SetCompletionPolicy(CompletionPolicy{MaxRepairAttempts: 2})
	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", nil)
	if err == nil || turn != nil {
		t.Fatalf("expected error and nil turn after exhausting retries, got turn=%v, err=%v", turn, err)
	}
	failure, ok := harness.FailureFrom(err)
	if !ok || failure == nil {
		t.Fatalf("expected harness.GenerationFailure, got %T: %v", err, err)
	}
	// Initial attempt + 2 repair attempts = 3 calls total
	if callCount != 3 {
		t.Fatalf("expected 3 calls (initial + 2 retries), got %d", callCount)
	}
	if len(failure.Attempts) != 3 {
		t.Fatalf("expected 3 attempts recorded, got %d (%+v)", len(failure.Attempts), failure.Attempts)
	}
}

func TestValidReplyIsNotRetried(t *testing.T) {
	// Exactly one generation call.
	callCount := 0
	provider := &scriptedStreamProvider{
		chunksPerCall: [][]string{
			{"The torchlight flickers on the walls."},
		},
		onRequest: func(req harness.GenerateRequest) {
			callCount++
		},
	}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turn.Narration != "The torchlight flickers on the walls." {
		t.Fatalf("turn narration = %q", turn.Narration)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call for valid reply, got %d", callCount)
	}
}

func TestRepairRecordsAttempts(t *testing.T) {
	// A malformed-then-valid turn records one parse_error attempt in the failure taxonomy.
	provider := &scriptedStreamProvider{
		chunksPerCall: [][]string{
			{""},
			{"The door swings wide."},
		},
	}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I open the door", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turn.Narration != "The door swings wide." {
		t.Fatalf("turn narration = %q", turn.Narration)
	}
}


