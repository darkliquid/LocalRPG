package harness_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

type stubToolProvider struct{}

func (s *stubToolProvider) ID() string { return "stub" }

func (s *stubToolProvider) Generate(context.Context, harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{}, nil
}

func (s *stubToolProvider) Stream(_ context.Context, _ harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	close(out)
	return nil
}

func (s *stubToolProvider) ToolCallerCapable() bool { return true }

func TestDescribeReportsToolsForToolCaller(t *testing.T) {
	caps := harness.Describe(&stubToolProvider{})
	if !caps.Tools || !caps.Streaming {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
	if caps.Sessions || caps.ContextCache {
		t.Fatalf("a plain provider must not claim sessions or caching: %+v", caps)
	}
}
