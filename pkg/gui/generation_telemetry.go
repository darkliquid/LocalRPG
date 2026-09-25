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
	errors        otelmetric.Int64Counter
	duration      otelmetric.Float64Histogram
	fallbacks     otelmetric.Int64Counter
	imageDuration otelmetric.Float64Histogram
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
		errors:        telemetry.Int64Counter(meter, "localrpg.generation.errors", "1", "Generation requests that failed."),
		duration:      telemetry.Float64Histogram(meter, "localrpg.generation.duration", "ms", "Wall-clock duration of one generation request."),
		fallbacks:     telemetry.Int64Counter(meter, "localrpg.provider.fallbacks", "1", "Fallback providers engaged after a failed attempt."),
		imageDuration: telemetry.Float64Histogram(meter, "localrpg.media.image.duration", "ms", "Duration of one image generation."),
	}
	genInstrumentsProvider = provider
	return genInstruments
}

// startGenerationSpan opens the span for one generation request and logs the
// request event. The caller must end it. A disabled provider yields a no-op span.
func startGenerationSpan(ctx context.Context, logger trace.Logger, name, formType, fieldName string) (context.Context, oteltrace.Span) {
	trace.LogEvent(ctx, trace.OrNil(logger), "generate.request", map[string]interface{}{
		"span":       name,
		"form_type":  formType,
		"field_name": fieldName,
	})
	return telemetry.Tracer("github.com/darkliquid/localrpg/pkg/gui").Start(ctx, name,
		oteltrace.WithAttributes(
			attribute.String("localrpg.form_type", formType),
			attribute.String("localrpg.field_name", fieldName),
		),
	)
}

// startImageSpan opens the image span. The image attributes are set on
// completion, when the provider identity and byte count are known.
func startImageSpan(ctx context.Context, logger trace.Logger, kind string) (context.Context, oteltrace.Span) {
	trace.LogEvent(ctx, trace.OrNil(logger), "generate.request", map[string]interface{}{
		"span":       "generate.image",
		"form_type":  "image",
		"field_name": kind,
	})
	return telemetry.Tracer("github.com/darkliquid/localrpg/pkg/gui").Start(ctx, "generate.image",
		oteltrace.WithAttributes(attribute.String("localrpg.image.kind", kind)),
	)
}

// roleForAttempts names the role that produced the last attempt, for the
// failure metric.
func roleForAttempts(attempts []harness.Attempt) string {
	if len(attempts) == 0 {
		return ""
	}
	return attempts[len(attempts)-1].Role
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
func (s *Service) recordGeneration(ctx context.Context, span oteltrace.Span, formType, role string, started time.Time, failure *harness.GenerationFailure) {
	elapsed := time.Since(started)
	fields := map[string]interface{}{
		"form_type":   formType,
		"role":        role,
		"outcome":     outcomeLabel(failure),
		"duration_ms": elapsed.Milliseconds(),
	}
	if failure != nil {
		fields["code"] = string(failure.Code)
		fields["message"] = failure.Message
		fields["attempts"] = len(failure.Attempts)
		generationMetrics().errors.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.form_type", formType),
			attribute.String("localrpg.role", role),
			attribute.String("localrpg.generation.failure_code", string(failure.Code)),
		))
		if span != nil {
			span.SetAttributes(attribute.String("localrpg.generation.failure_code", string(failure.Code)))
			span.SetStatus(codes.Error, string(failure.Code))
			span.RecordError(failure)
		}
		trace.LogEvent(ctx, trace.OrNil(s.logger), "generate.error", fields)
	} else {
		trace.LogEvent(ctx, trace.OrNil(s.logger), "generate.complete", fields)
	}
	generationMetrics().duration.Record(ctx, float64(elapsed.Milliseconds()), otelmetric.WithAttributes(
		attribute.String("localrpg.form_type", formType),
		attribute.String("localrpg.role", role),
		attribute.String("localrpg.generation.outcome", outcomeLabel(failure)),
	))
}

// recordImage writes the outcome of one image request: span attributes, the
// image duration, the shared generation metrics, and the unified generate.*
// events.
func (s *Service) recordImage(ctx context.Context, span oteltrace.Span, kind, provider string, size int, started time.Time, failure *harness.GenerationFailure) {
	elapsed := time.Since(started)
	fields := map[string]interface{}{
		"form_type":   "image",
		"field_name":  kind,
		"provider":    provider,
		"bytes":       size,
		"duration_ms": elapsed.Milliseconds(),
	}
	if span != nil {
		span.SetAttributes(
			attribute.String("localrpg.image.provider", provider),
			attribute.Int("localrpg.image.bytes", size),
		)
	}
	generationMetrics().imageDuration.Record(ctx, float64(elapsed.Milliseconds()), otelmetric.WithAttributes(
		attribute.String("localrpg.image.provider", provider),
	))
	if failure != nil {
		fields["code"] = string(failure.Code)
		fields["message"] = failure.Message
		generationMetrics().errors.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.form_type", "image"),
			attribute.String("localrpg.role", "image"),
			attribute.String("localrpg.generation.failure_code", string(failure.Code)),
		))
		if span != nil {
			span.SetAttributes(attribute.String("localrpg.generation.failure_code", string(failure.Code)))
			span.SetStatus(codes.Error, string(failure.Code))
			span.RecordError(failure)
			span.AddEvent("error", oteltrace.WithAttributes(attribute.String("localrpg.generation.failure_code", string(failure.Code))))
		}
		trace.LogEvent(ctx, trace.OrNil(s.logger), "generate.error", fields)
	} else {
		trace.LogEvent(ctx, trace.OrNil(s.logger), "generate.complete", fields)
	}
	generationMetrics().duration.Record(ctx, float64(elapsed.Milliseconds()), otelmetric.WithAttributes(
		attribute.String("localrpg.form_type", "image"),
		attribute.String("localrpg.role", "image"),
		attribute.String("localrpg.generation.outcome", outcomeLabel(failure)),
	))
}

// setTextOutcome records the result-dependent span attributes. An empty attempt
// list (a request that never reached a provider) omits them.
func (s *Service) setTextOutcome(span oteltrace.Span, outcome generationOutcome, fieldCount int) {
	if span == nil {
		return
	}
	attrs := make([]attribute.KeyValue, 0, 4)
	if len(outcome.Attempts) > 0 {
		last := outcome.Attempts[len(outcome.Attempts)-1]
		attrs = append(attrs,
			attribute.Int("localrpg.generation.attempts", len(outcome.Attempts)),
			attribute.String("gen_ai.system", last.Provider),
		)
	}
	if outcome.GeneratedBy != "" {
		attrs = append(attrs,
			attribute.String("localrpg.generated_by", outcome.GeneratedBy),
			attribute.Int("localrpg.field_count", fieldCount),
		)
	}
	span.SetAttributes(attrs...)
}

// recordGenerationAttempts makes the fallback chain visible as span events and
// counts each engaged fallback. The free-form detail stays in the JSONL trace
// and the structured failure body, never on the span.
func (s *Service) recordGenerationAttempts(ctx context.Context, span oteltrace.Span, attempts []harness.Attempt) {
	for i, attempt := range attempts {
		trace.LogEvent(ctx, trace.OrNil(s.logger), "generate.attempt", map[string]interface{}{
			"role":         attempt.Role,
			"provider":     attempt.Provider,
			"code":         string(attempt.Code),
			"duration_ms":  attempt.DurationMS,
		})
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
		// The fallback engaged because the previous attempt failed, so its code
		// explains why, not the fallback's own outcome.
		generationMetrics().fallbacks.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.role", attempt.Role),
			attribute.String("localrpg.generation.failure_code", string(attempts[i-1].Code)),
		))
		if span != nil {
			span.AddEvent("fallback", oteltrace.WithAttributes(
				attribute.String("localrpg.role", attempts[i-1].Role),
				attribute.String("localrpg.role.next", attempt.Role),
				attribute.String("localrpg.generation.failure_code", string(attempts[i-1].Code)),
			))
		}
	}
}
