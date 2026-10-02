package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// A model that writes the turn as prose and then submits it as a tool call is
// the shape that showed the turn twice and had the narrator read a speaker
// prefix aloud. The prose is discarded, so the listener must not see it.
func TestDiscardedToolRoundProseIsNotNarrated(t *testing.T) {
	submit := `{"action_verdict":{"feasibility":"automatic","reason":"clear"},"segments":[{"kind":"narration","text":"You cross the square."}]}`
	provider := &toolScriptProvider{replies: []toolReply{{
		text:  "Narrator: You cross the square.",
		tools: []harness.ToolCall{{ID: "1", Name: "submit_turn", Arguments: submit}},
	}}}

	orchestrator, _ := toolLoopOrchestrator(t, provider)
	orchestrator.SetTools(&fakeExecutor{}, "yes")

	var received []string
	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I cross the square", func(text string) error {
		received = append(received, text)
		return nil
	})
	if err != nil {
		t.Fatalf("ProcessActionStream failed: %v", err)
	}

	if len(received) != 0 {
		t.Errorf("discarded tool-round prose was narrated: %v", received)
	}
	if turn.Verdict == nil {
		t.Fatal("expected the tool call to submit a structured turn")
	}
}

// A structured reply is a JSON payload, not narration, so a schema round must
// never stream it to the client or the sentence synthesizer.
func TestStructuredJSONReplyIsNotNarrated(t *testing.T) {
	payload := `{"action_verdict":{"feasibility":"automatic","reason":"clear"},"segments":[{"kind":"narration","text":"The gate opens."}]}`
	provider := &mockStructuredGM{capable: true, response: payload}

	orchestrator, _ := toolLoopOrchestrator(t, provider)

	var received []string
	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I open the gate", func(text string) error {
		received = append(received, text)
		return nil
	})
	if err != nil {
		t.Fatalf("ProcessActionStream failed: %v", err)
	}

	if len(received) != 0 {
		t.Errorf("structured JSON was narrated: %v", received)
	}
	if turn.Verdict == nil {
		t.Fatal("expected a structured turn from the JSON reply")
	}
}

// The GM often names a speaker the way the narration links them. The wikilink
// must be unwrapped before the speaker is displayed and voiced.
func TestStructuredSpeakerUnwrapsWikilink(t *testing.T) {
	payload := `{
		"action_verdict": {"feasibility": "automatic", "reason": "clear"},
		"segments": [{"kind": "speech", "speaker": "[[Garrick]]", "text": "Quick as always."}],
		"personae": [{"name": "Garrick", "type": "character"}]
	}`
	provider := &mockStructuredGM{capable: true, response: payload}

	orchestrator, _ := toolLoopOrchestrator(t, provider)

	turn, err := orchestrator.ProcessAction(context.Background(), "Do", "I nod to Garrick")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	if len(turn.Segments) != 1 {
		t.Fatalf("expected one segment, got %d", len(turn.Segments))
	}
	segment := turn.Segments[0]
	if segment.Kind != "speech" {
		t.Fatalf("expected a speech segment, got %q", segment.Kind)
	}
	if segment.Speaker != "Garrick" {
		t.Errorf("Speaker = %q, want the wikilink unwrapped to %q", segment.Speaker, "Garrick")
	}
	if segment.SpeakerID != "garrick" {
		t.Errorf("SpeakerID = %q, want %q", segment.SpeakerID, "garrick")
	}
}
