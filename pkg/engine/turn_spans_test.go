package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// TestTurnRecordsFinaliseSpan asserts the post-stream phase is instrumented, so
// its cost can be read from a trace.
func TestTurnRecordsFinaliseSpan(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	orchestrator, _ := toolLoopOrchestrator(t, &scriptedStreamProvider{chunks: []string{"You swing and miss."}})
	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "attack", func(string) error { return nil }); err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}

	found := false
	for _, span := range recorder.Spans() {
		if span.Name() == "turn.finalise" {
			found = true
		}
	}
	if !found {
		t.Fatal("turn.finalise span was not recorded")
	}
}
