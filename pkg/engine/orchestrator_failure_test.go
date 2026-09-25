package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
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
