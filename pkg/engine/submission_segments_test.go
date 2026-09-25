package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestBuildSegmentsFromSubmission(t *testing.T) {
	sub := &harness.TurnSubmission{
		Segments: []harness.SegmentSpec{
			{Kind: "narration", Text: "The hall is cold."},
			{Kind: "speech", Speaker: "Kae", Text: "We should leave."},
		},
	}
	narration, segments := buildSegments(sub, func(name string) (string, bool) {
		if name == "Kae" {
			return "kae", true
		}
		return "", false
	})
	if narration != "The hall is cold." {
		t.Fatalf("narration = %q", narration)
	}
	if len(segments) != 2 || segments[1].SpeakerID != "kae" || segments[1].Kind != entity.SegmentSpeech {
		t.Fatalf("segments = %+v", segments)
	}
}

func TestUnresolvedSpeechFallsBackToNarration(t *testing.T) {
	sub := &harness.TurnSubmission{
		Segments: []harness.SegmentSpec{{Kind: "speech", Speaker: "Nobody", Text: "Hello?"}},
	}
	narration, segments := buildSegments(sub, func(string) (string, bool) { return "", false })
	if narration != "Hello?" {
		t.Fatalf("narration = %q", narration)
	}
	if len(segments) != 1 || segments[0].Kind != entity.SegmentNarration {
		t.Fatalf("segments = %+v", segments)
	}
}

func TestPersonaStubCreatedFromSubmission(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "submit_turn", Arguments: `{"action_verdict":{"feasibility":"automatic","reason":"greeting"},"segments":[{"kind":"speech","speaker":"Kae","text":"Well met."}],"personae":[{"name":"Kae","type":"character","new":true,"gender":"woman","pronouns":"she/her","role_tags":["scout"],"description":"A wary scout."}]}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")
	turn, err := o.ProcessActionStream(context.Background(), "Do", "hello", nil)
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if len(turn.Personae) != 1 || turn.Personae[0] != "kae" {
		t.Fatalf("personae = %v", turn.Personae)
	}
	stub, err := o.store.GetEntity("kae")
	if err != nil || stub == nil {
		t.Fatalf("stub not written: %v", err)
	}
	if stub.Name != "Kae" || stub.Type != "character" {
		t.Fatalf("stub = %+v", stub)
	}
	if gender, _ := stub.State.Get("gender"); gender != "woman" {
		t.Fatalf("gender = %v", gender)
	}
}
