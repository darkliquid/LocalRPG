package debugger_test

import (
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/debugger"
)

func TestCorrelateActionSpansAndDiagnostics(t *testing.T) {
	coll := debugger.NewCollector(100, 100)

	spans := []debugger.SpanSummary{
		{
			SpanID:   "s1",
			TraceID:  "t1",
			Name:     "turn.process",
			Duration: 50 * time.Millisecond,
			Status:   "ERROR",
			Attributes: map[string]string{
				"localrpg.action.id":    "act-1",
				"turn.failure_code":     "empty_response",
				"turn.failure_message":  "Narrative model returned blank completion",
				"turn.assembled_prompt": "System prompt: behave as GM...",
				"turn.raw_completion":   "",
			},
		},
		{
			SpanID:       "s2",
			TraceID:      "t1",
			ParentSpanID: "s1",
			Name:         "provider.generate",
			Duration:     40 * time.Millisecond,
			Status:       "OK",
			Attributes: map[string]string{
				"provider.id":   "narrative-oracle",
				"provider.role": "gm",
			},
		},
	}

	correlator := debugger.NewCorrelator(coll)
	action := debugger.ActionRecord{
		ID:         "act-1",
		ActionType: "click",
		Selector:   "#submit",
	}

	enriched := correlator.EnrichAction(action, spans)

	if enriched.RootTraceID != "t1" {
		t.Errorf("Expected RootTraceID t1, got %s", enriched.RootTraceID)
	}
	if enriched.Diagnostics == nil {
		t.Fatalf("Expected non-nil Diagnostics")
	}
	if enriched.Diagnostics.FailureCode != "empty_response" {
		t.Errorf("Expected failure code empty_response, got %s", enriched.Diagnostics.FailureCode)
	}
	if enriched.Diagnostics.AssembledPrompt != "System prompt: behave as GM..." {
		t.Errorf("Unexpected prompt: %s", enriched.Diagnostics.AssembledPrompt)
	}
	if len(enriched.Diagnostics.Attempts) != 1 || enriched.Diagnostics.Attempts[0].ProviderID != "narrative-oracle" {
		t.Errorf("Unexpected attempts: %+v", enriched.Diagnostics.Attempts)
	}
}
