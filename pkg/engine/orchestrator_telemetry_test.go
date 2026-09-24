package engine

import (
	"context"
	"testing"

	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// TestTurnProducesSpanTree proves one turn records a correlated tree: a root
// turn span covering context assembly, one provider span per tool round, and a
// tool span per executed call, all sharing one trace id.
func TestTurnProducesSpanTree(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	provider := &toolScriptProvider{replies: []toolReply{
		{text: "let me check", tools: []harness.ToolCall{{ID: "1", Name: "search_entities", Arguments: `{"query":"warden"}`}}},
		{text: "The Warden keeps the eastern gate."},
	}}
	executor := &fakeExecutor{results: []string{"The Warden (character, id warden): a grim guard."}}
	orchestrator, _ := toolLoopOrchestrator(t, provider)
	orchestrator.SetTools(executor, "yes")

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "who guards the gate?", nil); err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}

	names := map[string]int{}
	var turnTrace oteltrace.TraceID
	for _, span := range recorder.Spans() {
		names[span.Name()]++
		if span.Name() == "turn" {
			turnTrace = span.SpanContext().TraceID()
		}
	}

	for _, want := range []string{"turn", "context.assemble", "provider.generate", "tool.call", "timeline.record_turn"} {
		if names[want] == 0 {
			t.Errorf("expected a %q span, got %v", want, names)
		}
	}
	if names["provider.generate"] < 2 {
		t.Errorf("expected one provider span per round, got %d", names["provider.generate"])
	}
	if names["tool.call"] != 1 {
		t.Errorf("expected one tool span, got %d", names["tool.call"])
	}
	if !turnTrace.IsValid() {
		t.Fatal("expected a valid turn trace id")
	}
	for _, span := range recorder.Spans() {
		if span.Name() == "context.assemble" || span.Name() == "provider.generate" || span.Name() == "tool.call" {
			if span.SpanContext().TraceID() != turnTrace {
				t.Errorf("span %q did not share the turn trace id", span.Name())
			}
		}
	}
}
