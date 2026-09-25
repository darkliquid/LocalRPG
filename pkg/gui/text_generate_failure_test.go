package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/trace"
)

func TestPickFailureCode(t *testing.T) {
	if got := pickFailureCode(nil); got != harness.FailureProviderUnavailable {
		t.Fatalf("pickFailureCode(nil) = %q, want provider_unavailable", got)
	}
	attempts := []harness.Attempt{
		{Code: harness.FailureProviderError},
		{Code: harness.FailureParseError},
	}
	if got := pickFailureCode(attempts); got != harness.FailureParseError {
		t.Fatalf("pickFailureCode() = %q, want the last attempt code", got)
	}
}

func TestGenerateTextEmitsTraceEvents(t *testing.T) {
	_, svc := setupTestGame(t)
	mem := trace.NewMemory(trace.LevelSummary)
	svc.logger = mem

	_, _ = svc.GenerateText(context.Background(), GenerateTextRequest{FormType: "world", FieldName: "name"})

	names := mem.Names()
	want := map[string]bool{"generate.request": false, "generate.attempt": false, "generate.error": false}
	errorCount := 0
	for _, name := range names {
		if name == "generate.error" {
			errorCount++
		}
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("trace events %v missing %q", names, name)
		}
	}
	// The service owns the event; the handler must not emit a duplicate.
	if errorCount != 1 {
		t.Fatalf("generate.error emitted %d times, want exactly 1 (%v)", errorCount, names)
	}
}
