package engine

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// TestTurnRecordsMetrics proves one turn exports the turn, provider, and
// context instruments from the catalogue.
func TestTurnRecordsMetrics(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	provider := &toolScriptProvider{replies: []toolReply{{text: "The gate stands open."}}}
	orchestrator, _ := toolLoopOrchestrator(t, provider)
	orchestrator.SetTools(&fakeExecutor{}, "yes")

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "look around", nil); err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}

	snapshot, err := recorder.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	names := metricNames(snapshot)
	for _, want := range []string{
		"localrpg.turn.completed",
		"localrpg.turn.duration",
		"localrpg.provider.request.duration",
		"localrpg.context.tokens",
	} {
		if !names[want] {
			t.Errorf("expected metric %q, got %v", want, names)
		}
	}
}

func metricNames(snapshot metricdata.ResourceMetrics) map[string]bool {
	names := map[string]bool{}
	for _, scope := range snapshot.ScopeMetrics {
		for _, metric := range scope.Metrics {
			names[metric.Name] = true
		}
	}
	return names
}
