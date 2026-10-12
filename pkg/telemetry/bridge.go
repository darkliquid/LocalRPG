package telemetry

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	logglobal "go.opentelemetry.io/otel/log/global"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/darkliquid/localrpg/pkg/trace"
)

// Logger wraps the local JSONL logger so every event is also recorded as an
// OpenTelemetry log record, and, when a context is supplied, as a span event.
// The local sink stays authoritative; this only adds the correlatable layer.
func (p *Provider) Logger(local trace.Logger) trace.Logger {
	if p == nil || !p.Enabled() {
		return trace.OrNil(local)
	}
	return &bridgeLogger{local: trace.OrNil(local)}
}

type bridgeLogger struct {
	local trace.Logger
}

func (b *bridgeLogger) Enabled(level trace.Level) bool { return b.local.Enabled(level) }

func (b *bridgeLogger) SetGame(gameID string) { b.local.SetGame(gameID) }

// Event records the event locally and as an OTel log record.
func (b *bridgeLogger) Event(name string, fields map[string]any) {
	b.local.Event(name, fields)
	b.emit(context.Background(), name, fields)
}

// EventCtx is Event with a context, so the same event also lands on the active
// span. It is reached through trace.LogEvent.
func (b *bridgeLogger) EventCtx(ctx context.Context, name string, fields map[string]any) {
	b.local.Event(name, fields)
	b.emit(ctx, name, fields)
	span := oteltrace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, oteltrace.WithAttributes(attributesFromFields(fields)...))
	}
}

func (b *bridgeLogger) emit(ctx context.Context, name string, fields map[string]any) {
	logger := logglobal.GetLoggerProvider().Logger(MeterName)
	var record otellog.Record
	record.SetEventName(name)
	record.SetBody(attribute.StringValue(name))
	record.AddAttributes(attributesFromFields(fields)...)
	logger.Emit(ctx, record)
}

// attributesFromFields turns sanitized event fields into span/log attributes,
// encoding anything nested as JSON so a map never reaches the wire as a Go
// stringification.
func attributesFromFields(fields map[string]any) []attribute.KeyValue {
	clean := trace.SanitizeFields(fields)
	attrs := make([]attribute.KeyValue, 0, len(clean))
	for key, value := range clean {
		switch typed := value.(type) {
		case string:
			attrs = append(attrs, attribute.String(key, typed))
		case bool:
			attrs = append(attrs, attribute.Bool(key, typed))
		case int:
			attrs = append(attrs, attribute.Int(key, typed))
		case int64:
			attrs = append(attrs, attribute.Int64(key, typed))
		case float64:
			attrs = append(attrs, attribute.Float64(key, typed))
		default:
			encoded, err := json.Marshal(typed)
			if err != nil {
				attrs = append(attrs, attribute.String(key, fmt.Sprintf("%v", typed)))
				continue
			}
			attrs = append(attrs, attribute.String(key, string(encoded)))
		}
	}
	return attrs
}
