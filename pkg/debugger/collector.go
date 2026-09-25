package debugger

import (
	"context"
	"sync"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Collector is an in-process, thread-safe OpenTelemetry span and log receiver with bounded ring buffers.
type Collector struct {
	mu         sync.RWMutex
	maxSpans   int
	maxLogs    int
	spans      []SpanSummary
	logs       []LogRecord
	spanIndex  map[string][]SpanSummary // key: actionID
	traceIndex map[string][]SpanSummary // key: traceID
}

// LogRecord captures a structured log event.
type LogRecord struct {
	Timestamp  time.Time         `json:"timestamp"`
	Severity   string            `json:"severity"`
	Message    string            `json:"message"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// NewCollector constructs a collector with maximum capacity limits.
func NewCollector(maxSpans, maxLogs int) *Collector {
	if maxSpans <= 0 {
		maxSpans = 1000
	}
	if maxLogs <= 0 {
		maxLogs = 5000
	}
	return &Collector{
		maxSpans:   maxSpans,
		maxLogs:    maxLogs,
		spans:      make([]SpanSummary, 0, maxSpans),
		logs:       make([]LogRecord, 0, maxLogs),
		spanIndex:  make(map[string][]SpanSummary),
		traceIndex: make(map[string][]SpanSummary),
	}
}

// ExportSpans implements sdktrace.SpanExporter.
func (c *Collector) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, s := range spans {
		summary := spanToSummary(s)

		// Append to ring buffer
		if len(c.spans) >= c.maxSpans {
			oldest := c.spans[0]
			c.spans = c.spans[1:]
			c.evictFromIndices(oldest)
		}
		c.spans = append(c.spans, summary)

		// Index by action ID if present
		if actionID, ok := summary.Attributes["localrpg.action.id"]; ok && actionID != "" {
			c.spanIndex[actionID] = append(c.spanIndex[actionID], summary)
		}
		if summary.TraceID != "" {
			c.traceIndex[summary.TraceID] = append(c.traceIndex[summary.TraceID], summary)
		}
	}
	return nil
}

// Shutdown implements sdktrace.SpanExporter.
func (c *Collector) Shutdown(_ context.Context) error { return nil }

func (c *Collector) evictFromIndices(s SpanSummary) {
	if actionID, ok := s.Attributes["localrpg.action.id"]; ok {
		c.spanIndex[actionID] = filterOutSpan(c.spanIndex[actionID], s.SpanID)
	}
	if s.TraceID != "" {
		c.traceIndex[s.TraceID] = filterOutSpan(c.traceIndex[s.TraceID], s.SpanID)
	}
}

func filterOutSpan(list []SpanSummary, spanID string) []SpanSummary {
	for i, item := range list {
		if item.SpanID == spanID {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}

func spanToSummary(s sdktrace.ReadOnlySpan) SpanSummary {
	attrs := make(map[string]string, len(s.Attributes()))
	for _, kv := range s.Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}

	events := make([]SpanEvent, 0, len(s.Events()))
	for _, ev := range s.Events() {
		evAttrs := make(map[string]string, len(ev.Attributes))
		for _, kv := range ev.Attributes {
			evAttrs[string(kv.Key)] = kv.Value.Emit()
		}
		events = append(events, SpanEvent{
			Name:       ev.Name,
			Timestamp:  ev.Time,
			Attributes: evAttrs,
		})
	}

	parentSpanID := ""
	if s.Parent().IsValid() {
		parentSpanID = s.Parent().SpanID().String()
	}

	return SpanSummary{
		SpanID:        s.SpanContext().SpanID().String(),
		TraceID:       s.SpanContext().TraceID().String(),
		ParentSpanID:  parentSpanID,
		Name:          s.Name(),
		StartTime:     s.StartTime(),
		EndTime:       s.EndTime(),
		Duration:      s.EndTime().Sub(s.StartTime()),
		Status:        s.Status().Code.String(),
		StatusMessage: s.Status().Description,
		Attributes:    attrs,
		Events:        events,
	}
}

// GetSpans returns all spans currently held in the ring buffer.
func (c *Collector) GetSpans() []SpanSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]SpanSummary, len(c.spans))
	copy(res, c.spans)
	return res
}

// FindSpansByActionID returns spans associated with a given action ID.
func (c *Collector) FindSpansByActionID(actionID string) []SpanSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()
	spans := c.spanIndex[actionID]
	res := make([]SpanSummary, len(spans))
	copy(res, spans)
	return res
}

// FindSpansByTraceID returns all spans belonging to a trace ID.
func (c *Collector) FindSpansByTraceID(traceID string) []SpanSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()
	spans := c.traceIndex[traceID]
	res := make([]SpanSummary, len(spans))
	copy(res, spans)
	return res
}

// Clear flushes all buffered records.
func (c *Collector) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spans = c.spans[:0]
	c.logs = c.logs[:0]
	c.spanIndex = make(map[string][]SpanSummary)
	c.traceIndex = make(map[string][]SpanSummary)
}
