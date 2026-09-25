package gui

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

func TestRecordGenerationFailureEmitsSpanAndMetric(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	service := &Service{}
	ctx, span := service.startGenerationSpan(context.Background(), "generate.text", "world", "lore_prompt")
	failure := &harness.GenerationFailure{Code: harness.FailureEmptyResponse, Message: "no text"}
	service.recordGeneration(ctx, span, "world", time.Now(), failure)
	span.End()

	spans := recorder.Spans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	var found bool
	for _, attr := range spans[0].Attributes() {
		if attr.Key == attribute.Key("localrpg.generation.failure_code") && attr.Value.AsString() == "empty_response" {
			found = true
		}
	}
	if !found {
		t.Fatal("span is missing the localrpg.generation.failure_code attribute")
	}

	rm, err := recorder.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if len(rm.ScopeMetrics) == 0 || len(rm.ScopeMetrics[0].Metrics) == 0 {
		t.Fatal("no generation metrics were recorded")
	}
}

func TestRecordGenerationAttemptsCountsFallbacks(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	service := &Service{}
	ctx, span := service.startGenerationSpan(context.Background(), "generate.text", "world", "_all")
	service.recordGenerationAttempts(ctx, span, []harness.Attempt{
		{Role: "character", Provider: "p1", Code: harness.FailureEmptyResponse},
		{Role: "gm", Provider: "p2", Code: harness.FailureEmptyResponse},
	})
	span.End()

	spans := recorder.Spans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	var attempts, fallbacks int
	for _, event := range spans[0].Events() {
		switch event.Name {
		case "attempt":
			attempts++
		case "fallback":
			fallbacks++
		}
	}
	if attempts != 2 || fallbacks != 1 {
		t.Fatalf("events = %d attempts = %d fallbacks = %d, want 3 events with 2 attempts and 1 fallback", len(spans[0].Events()), attempts, fallbacks)
	}
}

func TestOutcomeLabel(t *testing.T) {
	if got := outcomeLabel(nil); got != "success" {
		t.Fatalf("outcomeLabel(nil) = %q, want success", got)
	}
	if got := outcomeLabel(&harness.GenerationFailure{Code: harness.FailureTimeout}); got != "failure" {
		t.Fatalf("outcomeLabel(failure) = %q, want failure", got)
	}
}
