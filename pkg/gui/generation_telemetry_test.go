package gui

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// metricHasAttribute scans recorded metrics for an attribute key/value pair.
func metricHasAttribute(rm metricdata.ResourceMetrics, key, value string) bool {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, dp := range data.DataPoints {
					if v, ok := dp.Attributes.Value(attribute.Key(key)); ok && v.AsString() == value {
						return true
					}
				}
			case metricdata.Histogram[float64]:
				for _, dp := range data.DataPoints {
					if v, ok := dp.Attributes.Value(attribute.Key(key)); ok && v.AsString() == value {
						return true
					}
				}
			}
		}
	}
	return false
}

func TestRecordGenerationFailureEmitsSpanAndMetric(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	service := &Service{}
	ctx, span := startGenerationSpan(context.Background(), service.logger, "generate.text", "world", "lore_prompt")
	failure := &harness.GenerationFailure{Code: harness.FailureEmptyResponse, Message: "no text"}
	service.recordGeneration(ctx, span, "world", "gm", time.Now(), failure)
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
	if !metricHasAttribute(rm, "localrpg.role", "gm") {
		t.Fatal("localrpg.role was not recorded on a generation metric")
	}
}

func TestRecordGenerationAttemptsCountsFallbacks(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	service := &Service{}
	ctx, span := startGenerationSpan(context.Background(), service.logger, "generate.text", "world", "_all")
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

func TestSetTextOutcomeAttributes(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	service := &Service{}
	_, span := startGenerationSpan(context.Background(), service.logger, "generate.text", "world", "_all")
	service.setTextOutcome(span, generationOutcome{
		Attempts:    []harness.Attempt{{Role: "gm", Provider: "p1"}},
		GeneratedBy: "gm",
	}, 5)
	span.End()

	var attempts, fields bool
	for _, attr := range recorder.Spans()[0].Attributes() {
		switch attr.Key {
		case attribute.Key("localrpg.generation.attempts"):
			attempts = attr.Value.AsInt64() == 1
		case attribute.Key("localrpg.field_count"):
			fields = attr.Value.AsInt64() == 5
		}
	}
	if !attempts || !fields {
		t.Fatal("setTextOutcome did not set attempts and field_count")
	}
}

func TestRoleForAttempts(t *testing.T) {
	if got := roleForAttempts(nil); got != "" {
		t.Fatalf("roleForAttempts(nil) = %q, want empty", got)
	}
	got := roleForAttempts([]harness.Attempt{{Role: "character"}, {Role: "gm"}})
	if got != "gm" {
		t.Fatalf("roleForAttempts() = %q, want the last role", got)
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
