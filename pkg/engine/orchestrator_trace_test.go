package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/trace"
)

func TestATurnProducesAnOrderedTrace(t *testing.T) {
	memory := trace.NewMemory(trace.LevelFull)
	provider := &scriptedStreamProvider{chunks: []string{"The docks are quiet."}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	orchestrator.SetLogger(memory)

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", nil); err != nil {
		t.Fatalf("ProcessActionStream failed: %v", err)
	}

	order := []string{
		"turn.begin",
		"context.assembled",
		"generation.complete",
		"segment.build",
		"record.turn",
	}
	names := memory.Names()
	position := 0
	for _, name := range names {
		if position < len(order) && name == order[position] {
			position++
		}
	}
	if position != len(order) {
		t.Fatalf("trace is missing events in order: got %v, wanted %v", names, order)
	}

	assembled, ok := memory.Find("context.assembled")
	if !ok {
		t.Fatal("expected context.assembled")
	}
	if _, present := assembled.Fields["prompt"]; !present {
		t.Errorf("full level must record the assembled prompt, got %+v", assembled.Fields)
	}
	if assembled.Fields["estimated_tokens"] == nil {
		t.Errorf("expected an estimated token count, got %+v", assembled.Fields)
	}

	generated, ok := memory.Find("generation.complete")
	if !ok {
		t.Fatal("expected generation.complete")
	}
	if generated.Fields["narration_chars"] != 20 {
		t.Errorf("narration_chars = %v, want 20", generated.Fields["narration_chars"])
	}
	if generated.Fields["truncated"] != false {
		t.Errorf("truncated = %v, want false", generated.Fields["truncated"])
	}

	segments, ok := memory.Find("segment.build")
	if !ok {
		t.Fatal("expected segment.build")
	}
	if segments.Fields["count"] != 1 {
		t.Errorf("segment count = %v, want 1", segments.Fields["count"])
	}

	recorded, ok := memory.Find("record.turn")
	if !ok {
		t.Fatal("expected record.turn")
	}
	if recorded.Fields["number"] != 1 {
		t.Errorf("recorded turn number = %v, want 1", recorded.Fields["number"])
	}
}

func TestTraceIsSilentWhenOff(t *testing.T) {
	memory := trace.NewMemory(trace.LevelOff)
	provider := &scriptedStreamProvider{chunks: []string{"Silence."}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	orchestrator.SetLogger(memory)

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait", nil); err != nil {
		t.Fatal(err)
	}
	if len(memory.Events()) != 0 {
		t.Errorf("expected no events at level off, got %v", memory.Names())
	}
}
