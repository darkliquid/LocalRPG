package debugger

import (
	"strings"
)

// Correlator marries driver action executions to telemetry spans and failure diagnostics.
type Correlator struct {
	collector *Collector
}

// NewCorrelator constructs an action-telemetry correlator.
func NewCorrelator(collector *Collector) *Correlator {
	return &Correlator{collector: collector}
}

// EnrichAction populates an ActionRecord with matched spans and turn diagnostics.
func (c *Correlator) EnrichAction(action ActionRecord, spans []SpanSummary) ActionRecord {
	if len(spans) == 0 && c.collector != nil {
		spans = c.collector.FindSpansByActionID(action.ID)
	}

	action.Spans = spans
	if len(spans) == 0 {
		return action
	}

	// Identify root trace ID
	action.RootTraceID = spans[0].TraceID

	// Extract turn diagnostics if a turn span is present
	for _, span := range spans {
		if isTurnSpan(span.Name) || hasDiagnosticAttributes(span.Attributes) {
			action.Diagnostics = extractDiagnostics(span, spans)
			break
		}
	}

	return action
}

func isTurnSpan(name string) bool {
	return strings.Contains(name, "turn") || strings.Contains(name, "ProcessAction")
}

func hasDiagnosticAttributes(attrs map[string]string) bool {
	if attrs == nil {
		return false
	}
	_, hasCode := attrs["turn.failure_code"]
	_, hasPrompt := attrs["turn.assembled_prompt"]
	return hasCode || hasPrompt
}

func extractDiagnostics(root SpanSummary, allSpans []SpanSummary) *TurnDiagnostics {
	d := &TurnDiagnostics{
		FailureCode:     root.Attributes["turn.failure_code"],
		FailureMessage:  root.Attributes["turn.failure_message"],
		AssembledPrompt: root.Attributes["turn.assembled_prompt"],
		RawCompletion:   root.Attributes["turn.raw_completion"],
	}

	// Collect attempts from child spans if available
	for _, span := range allSpans {
		if strings.HasPrefix(span.Name, "provider.") || span.Attributes["provider.id"] != "" {
			d.Attempts = append(d.Attempts, ProviderAttempt{
				ProviderID: span.Attributes["provider.id"],
				Role:       span.Attributes["provider.role"],
				DurationMs: span.Duration.Milliseconds(),
				Success:    span.Status == "OK",
				Error:      span.StatusMessage,
			})
		}
	}

	return d
}
