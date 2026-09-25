package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

type failingStreamProvider struct{ err error }

func (f failingStreamProvider) ID() string { return "failing" }

func (f failingStreamProvider) Generate(context.Context, harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return nil, f.err
}

func (f failingStreamProvider) Stream(_ context.Context, _ harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	close(out)
	return f.err
}

type emptyStreamProvider struct{}

func (emptyStreamProvider) ID() string { return "empty" }

func (emptyStreamProvider) Generate(context.Context, harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{}, nil
}

func (emptyStreamProvider) Stream(_ context.Context, _ harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	close(out)
	return nil
}

func TestStreamStalledIsTimeoutFailure(t *testing.T) {
	o := &TurnOrchestrator{chunkTimeout: time.Second}
	_, err := o.stream(context.Background(), failingStreamProvider{err: ErrGenerationStalled}, harness.GenerateRequest{}, nil)
	if !errors.Is(err, ErrGenerationStalled) {
		t.Fatalf("err = %v, want ErrGenerationStalled", err)
	}
	// stream returns the failure in the result too; re-run to inspect it.
	result, err := o.stream(context.Background(), failingStreamProvider{err: ErrGenerationStalled}, harness.GenerateRequest{}, nil)
	if err == nil {
		t.Fatal("stream returned no error")
	}
	if result.Failure == nil || result.Failure.Code != harness.FailureTimeout {
		t.Fatalf("result.Failure = %+v, want timeout", result.Failure)
	}
}

func TestStreamEmptyCloseIsEmptyResponseFailure(t *testing.T) {
	o := &TurnOrchestrator{chunkTimeout: time.Second}
	result, err := o.stream(context.Background(), emptyStreamProvider{}, harness.GenerateRequest{}, nil)
	if err != nil {
		t.Fatalf("stream returned an error for an empty provider: %v", err)
	}
	if result.Failure == nil || result.Failure.Code != harness.FailureEmptyResponse {
		t.Fatalf("result.Failure = %+v, want empty_response", result.Failure)
	}
	if result.ProviderID != "empty" {
		t.Fatalf("ProviderID = %q, want empty", result.ProviderID)
	}
}

func TestStreamCountsChunks(t *testing.T) {
	o := &TurnOrchestrator{chunkTimeout: time.Second}
	result, err := o.stream(context.Background(), countingProvider{chunks: []string{"a", "b", "c"}}, harness.GenerateRequest{}, nil)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if result.ChunkCount != 3 {
		t.Fatalf("ChunkCount = %d, want 3", result.ChunkCount)
	}
}

type countingProvider struct{ chunks []string }

func (c countingProvider) ID() string { return "counting" }

func (c countingProvider) Generate(context.Context, harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{}, nil
}

func (c countingProvider) Stream(_ context.Context, _ harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	for _, text := range c.chunks {
		out <- harness.StreamChunk{Text: text}
	}
	close(out)
	return nil
}

func TestTurnFailureMarksSpanAndMetric(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	orchestrator, _ := toolLoopOrchestrator(t, &toolScriptProvider{})
	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "hello", nil); err == nil {
		t.Fatal("ProcessActionStream returned no error for an empty provider")
	} else if _, ok := harness.FailureFrom(err); !ok {
		t.Fatalf("err = %v, want a *GenerationFailure", err)
	}

	var turnSpan sdktrace.ReadOnlySpan
	for _, span := range recorder.Spans() {
		if span.Name() == "turn" {
			turnSpan = span
		}
	}
	if turnSpan == nil {
		t.Fatal("no turn span was recorded")
	}
	if turnSpan.Status().Code != codes.Error {
		t.Fatalf("turn span status = %v, want error", turnSpan.Status().Code)
	}
	var outcomeErr bool
	for _, attr := range turnSpan.Attributes() {
		if attr.Key == attribute.Key("turn.outcome") && attr.Value.AsString() == "error" {
			outcomeErr = true
		}
	}
	if !outcomeErr {
		t.Fatal("turn span is missing turn.outcome=error")
	}

	rm, err := recorder.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if !engineMetricHasAttribute(rm, "turn.outcome", "error") {
		t.Fatal("localrpg.turn.completed is missing turn.outcome=error")
	}
}

// engineMetricHasAttribute scans recorded metrics for an attribute key/value pair.
func engineMetricHasAttribute(rm metricdata.ResourceMetrics, key, value string) bool {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, dp := range data.DataPoints {
					if v, ok := dp.Attributes.Value(attribute.Key(key)); ok && v.AsString() == value {
						return true
					}
				}
			case metricdata.Histogram[float64]:
				for _, dp := range data.DataPoints {
					if v, ok := dp.Attributes.Value(attribute.Key(key)); ok && v.AsString() == value {
						return true
					}
				}
			}
		}
	}
	return false
}
