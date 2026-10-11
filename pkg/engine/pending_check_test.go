package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestProposeCheckEndsTheTurnPending(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "p1", Name: "propose_check", Arguments: `{"actor":"player","check_kind":"skill","stakes":"the bridge","outcomes":{"pass":"cross","fail":"fall"}}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetMechanicsEngagement("ask")
	o.SetTools(&fakeExecutor{}, "yes")

	turn, err := o.ProcessActionStream(context.Background(), "Do", "cross the rope bridge", nil)
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if turn.PendingCheck == nil || turn.PendingCheck.Ref != "p1" {
		t.Fatalf("expected a pending check, got %+v", turn.PendingCheck)
	}
}

func TestRollingAPendingCheckResolvesAndContinues(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{text: "You cross the bridge."},
	}}
	o, timeline := toolLoopOrchestrator(t, provider)
	o.SetMechanicsEngagement("ask")
	o.SetTools(&fakeExecutor{}, "yes")

	pending := &harness.PendingCheck{
		Ref:        "p1",
		ProposedBy: "gm",
		Request:    harness.CheckRequest{Actor: "player", CheckKind: "skill", Stakes: "the bridge"},
	}
	if err := timeline.history.AppendTurn(Turn{Number: 1, Mode: "Do", PendingCheck: pending}); err != nil {
		t.Fatal(err)
	}
	o.SetPendingCheckRef("p1")

	turn, err := o.ProcessActionStream(context.Background(), "Roll", "roll", nil)
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if len(turn.Checks) != 1 {
		t.Fatalf("expected the pending check resolved, got %+v", turn.Checks)
	}
	if turn.PendingCheck != nil {
		t.Fatalf("the continuation must not be pending: %+v", turn.PendingCheck)
	}
	if turn.ContinuationOf != 1 {
		t.Fatalf("ContinuationOf = %d, want the pending turn's number", turn.ContinuationOf)
	}
}

func TestARetriedRollReusesTheRecordedResult(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{text: "You cross."},
		{text: "You cross again."},
	}}
	o, timeline := toolLoopOrchestrator(t, provider)
	o.SetMechanicsEngagement("ask")
	o.SetTools(&fakeExecutor{}, "yes")

	pending := &harness.PendingCheck{
		Ref:        "p1",
		ProposedBy: "gm",
		Request:    harness.CheckRequest{Actor: "player", CheckKind: "skill", Stakes: "the bridge"},
	}
	if err := timeline.history.AppendTurn(Turn{Number: 1, Mode: "Do", PendingCheck: pending}); err != nil {
		t.Fatal(err)
	}

	o.SetPendingCheckRef("p1")
	first, err := o.ProcessActionStream(context.Background(), "Roll", "roll", nil)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	o.SetPendingCheckRef("p1")
	second, err := o.ProcessActionStream(context.Background(), "Roll", "roll", nil)
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if len(first.Checks) != 1 || len(second.Checks) != 1 {
		t.Fatalf("checks = %v / %v", first.Checks, second.Checks)
	}
	if first.Checks[0].CheckID != second.Checks[0].CheckID || first.Checks[0].Outcome != second.Checks[0].Outcome {
		t.Fatalf("a retry re-rolled: %+v vs %+v", first.Checks[0], second.Checks[0])
	}
}

func TestAResolvedPendingCheckAttachesToTheContinuation(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{text: "You cross the bridge."},
	}}
	o, timeline := toolLoopOrchestrator(t, provider)
	o.SetMechanicsEngagement("ask")
	o.SetTools(&fakeExecutor{}, "yes")

	pending := &harness.PendingCheck{
		Ref:        "p1",
		ProposedBy: "gm",
		Request:    harness.CheckRequest{Actor: "player", CheckKind: "skill", Stakes: "the bridge"},
	}
	if err := timeline.history.AppendTurn(Turn{Number: 1, Mode: "Do", PendingCheck: pending}); err != nil {
		t.Fatal(err)
	}
	o.SetPendingCheckRef("p1")

	turn, err := o.ProcessActionStream(context.Background(), "Roll", "roll", nil)
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if len(turn.Checks) == 0 {
		t.Fatal("expected the pending check resolved")
	}

	checkID := turn.Checks[0].CheckID
	for _, segment := range turn.Segments {
		if segment.CheckRef == checkID {
			return
		}
	}
	t.Fatalf("the resolved check %q is not attached to any segment: %+v", checkID, turn.Segments)
}
