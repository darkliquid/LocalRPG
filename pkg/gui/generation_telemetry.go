package gui

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// generationInstruments caches the one-shot generation instruments. It is
// rebuilt whenever the global meter provider changes so a test that installs an
// in-memory provider binds to it.
type generationInstruments struct {
	errors    otelmetric.Int64Counter
	duration  otelmetric.Float64Histogram
	fallbacks otelmetric.Int64Counter
}

var (
	genInstrumentsMu       sync.Mutex
	genInstrumentsProvider otelmetric.MeterProvider
	genInstruments         generationInstruments
)

func generationMetrics() generationInstruments {
	provider := otel.GetMeterProvider()
	genInstrumentsMu.Lock()
	defer genInstrumentsMu.Unlock()
	if provider == genInstrumentsProvider {
		return genInstruments
	}
	meter := provider.Meter(telemetry.MeterName)
	genInstruments = generationInstruments{
		errors:    telemetry.Int64Counter(meter, "localrpg.generation.errors", "1", "Generation requests that failed."),
		duration:  telemetry.Float64Histogram(meter, "localrpg.generation.duration", "ms", "Wall-clock duration of one generation request."),
		fallbacks: telemetry.Int64Counter(meter, "localrpg.provider.fallbacks", "1", "Fallback providers engaged after a failed attempt."),
	}
	genInstrumentsProvider = provider
	return genInstruments
}

// startGenerationSpan opens the span for one generation request. The caller must
// end it. A disabled provider yields a no-op span.
func (s *Service) startGenerationSpan(ctx context.Context, name, formType, fieldName string) (context.Context, oteltrace.Span) {
	return telemetry.Tracer("github.com/darkliquid/localrpg/pkg/gui").Start(ctx, name,
		oteltrace.WithAttributes(
			attribute.String("localrpg.form_type", formType),
			attribute.String("localrpg.field_name", fieldName),
		),
	)
}

func outcomeLabel(failure *harness.GenerationFailure) string {
	if failure == nil {
		return "success"
	}
	return "failure"
}

// recordGeneration writes the outcome of one generation request to the trace
// logger (and, through the telemetry bridge, to an OTel log record and span
// event), marks the span, and records the duration and failure counters.
func (s *Service) recordGeneration(ctx context.Context, span oteltrace.Span, formType string, started time.Time, failure *harness.GenerationFailure) {
	elapsed := time.Since(started)
	fields := map[string]interface{}{
		"form_type":   formType,
		"outcome":     outcomeLabel(failure),
		"duration_ms": elapsed.Milliseconds(),
	}
	if failure != nil {
		fields["code"] = string(failure.Code)
		fields["message"] = failure.Message
		fields["attempts"] = len(failure.Attempts)
		generationMetrics().errors.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.form_type", formType),
			attribute.String("localrpg.generation.failure_code", string(failure.Code)),
		))
		if span != nil {
			span.SetAttributes(attribute.String("localrpg.generation.failure_code", string(failure.Code)))
			span.SetStatus(codes.Error, string(failure.Code))
			span.RecordError(failure)
		}
		trace.LogEvent(ctx, trace.OrNil(s.logger), "generate.error", fields)
	} else {
		if span != nil {
			span.SetAttributes(attribute.String("localrpg.generation.failure_code", ""))
		}
		trace.LogEvent(ctx, trace.OrNil(s.logger), "generate.complete", fields)
	}
	generationMetrics().duration.Record(ctx, float64(elapsed.Milliseconds()), otelmetric.WithAttributes(
		attribute.String("localrpg.form_type", formType),
		attribute.String("localrpg.generation.outcome", outcomeLabel(failure)),
	))
}

// recordGenerationAttempts makes the fallback chain visible as span events and
// counts each engaged fallback. The free-form detail stays in the JSONL trace
// and the structured failure body, never on the span.
func (s *Service) recordGenerationAttempts(ctx context.Context, span oteltrace.Span, attempts []harness.Attempt) {
	for i, attempt := range attempts {
		if span != nil {
			span.AddEvent("attempt", oteltrace.WithAttributes(
				attribute.String("localrpg.role", attempt.Role),
				attribute.String("gen_ai.system", attempt.Provider),
				attribute.String("localrpg.generation.failure_code", string(attempt.Code)),
				attribute.Int64("duration_ms", attempt.DurationMS),
			))
		}
		if i == 0 {
			continue
		}
		generationMetrics().fallbacks.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.role", attempt.Role),
			attribute.String("localrpg.generation.failure_code", string(attempt.Code)),
		))
		if span != nil {
			span.AddEvent("fallback", oteltrace.WithAttributes(
				attribute.String("localrpg.role", attempts[i-1].Role),
				attribute.String("localrpg.role.next", attempt.Role),
				attribute.String("localrpg.generation.failure_code", string(attempt.Code)),
			))
		}
	}
}
