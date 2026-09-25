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
	code := root.Attributes["turn.failure_code"]
	if code == "" {
		code = root.Attributes["localrpg.generation.failure_code"]
	}
	msg := root.Attributes["turn.failure_message"]
	prompt := root.Attributes["turn.assembled_prompt"]
	comp := root.Attributes["turn.raw_completion"]

	for _, span := range allSpans {
		if prompt == "" && span.Attributes["turn.assembled_prompt"] != "" {
			prompt = span.Attributes["turn.assembled_prompt"]
		}
		if comp == "" && span.Attributes["turn.raw_completion"] != "" {
			comp = span.Attributes["turn.raw_completion"]
		}
		if code == "" && span.Attributes["turn.failure_code"] != "" {
			code = span.Attributes["turn.failure_code"]
		}
		if msg == "" && span.Attributes["turn.failure_message"] != "" {
			msg = span.Attributes["turn.failure_message"]
		}
	}

	d := &TurnDiagnostics{
		FailureCode:     code,
		FailureMessage:  msg,
		AssembledPrompt: prompt,
		RawCompletion:   comp,
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
