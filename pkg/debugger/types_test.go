package debugger_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/debugger"
)

func TestActionRecordJSON(t *testing.T) {
	rec := debugger.ActionRecord{
		ID:         "act-123",
		StepIndex:  1,
		ActionType: "click",
		Selector:   "[data-testid='submit']",
		InputData:  "",
		Timestamp:  time.Unix(1700000000, 0).UTC(),
		DurationMs: 42,
		Status:     "passed",
		Spans: []debugger.SpanSummary{
			{
				SpanID:   "span-1",
				TraceID:  "trace-1",
				Name:     "POST /api/game/{id}/turn",
				Duration: 40 * time.Millisecond,
				Status:   "OK",
			},
		},
		Diagnostics: &debugger.TurnDiagnostics{
			AssembledPrompt: "You are the GM...",
			RawCompletion:   "",
			FailureCode:     "empty_response",
		},
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded debugger.ActionRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != "act-123" || decoded.Status != "passed" {
		t.Errorf("Unexpected record content: %+v", decoded)
	}
	if decoded.Diagnostics == nil || decoded.Diagnostics.FailureCode != "empty_response" {
		t.Errorf("Unexpected diagnostics: %+v", decoded.Diagnostics)
	}
}
