package debugger

import (
	"time"
)

// SpanSummary is a lightweight representation of an OpenTelemetry span for debugging.
type SpanSummary struct {
	SpanID        string            `json:"span_id"`
	TraceID       string            `json:"trace_id"`
	ParentSpanID  string            `json:"parent_span_id,omitempty"`
	Name          string            `json:"name"`
	StartTime     time.Time         `json:"start_time"`
	EndTime       time.Time         `json:"end_time"`
	Duration      time.Duration     `json:"duration"`
	Status        string            `json:"status"`
	StatusMessage string            `json:"status_message,omitempty"`
	Attributes    map[string]string `json:"attributes,omitempty"`
	Events        []SpanEvent       `json:"events,omitempty"`
}

// SpanEvent represents a span lifecycle or milestone event.
type SpanEvent struct {
	Name       string            `json:"name"`
	Timestamp  time.Time         `json:"timestamp"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// ProviderAttempt records one model attempt during role generation.
type ProviderAttempt struct {
	ProviderID string `json:"provider_id"`
	Role       string `json:"role"`
	DurationMs int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	Tokens     int    `json:"tokens,omitempty"`
}

// TurnDiagnostics captures extracted generation details when inspecting turns.
type TurnDiagnostics struct {
	AssembledPrompt string            `json:"assembled_prompt,omitempty"`
	RawCompletion   string            `json:"raw_completion,omitempty"`
	FailureCode     string            `json:"failure_code,omitempty"`
	FailureMessage  string            `json:"failure_message,omitempty"`
	Attempts        []ProviderAttempt `json:"attempts,omitempty"`
}

// ActionRecord represents an individual driver or user action correlated with telemetry.
type ActionRecord struct {
	ID            string           `json:"id"`
	StepIndex     int              `json:"step_index"`
	ActionType    string           `json:"action_type"`
	Selector      string           `json:"selector,omitempty"`
	InputData     string           `json:"input_data,omitempty"`
	Timestamp     time.Time        `json:"timestamp"`
	DurationMs    int64            `json:"duration_ms"`
	ScreenshotB64 string           `json:"screenshot_b64,omitempty"`
	RootTraceID   string           `json:"root_trace_id,omitempty"`
	Status        string           `json:"status"` // "passed" | "failed"
	FailureReason string           `json:"failure_reason,omitempty"`
	Spans         []SpanSummary    `json:"spans,omitempty"`
	Diagnostics   *TurnDiagnostics `json:"diagnostics,omitempty"`
}

// TestReport encapsulates a complete execution report for an automated scenario run.
type TestReport struct {
	ScenarioName string         `json:"scenario_name"`
	Description  string         `json:"description,omitempty"`
	StartTime    time.Time      `json:"start_time"`
	EndTime      time.Time      `json:"end_time"`
	DurationMs   int64          `json:"duration_ms"`
	Passed       bool           `json:"passed"`
	Actions      []ActionRecord `json:"actions"`
	TotalActions int            `json:"total_actions"`
	FailedAction *ActionRecord  `json:"failed_action,omitempty"`
}
