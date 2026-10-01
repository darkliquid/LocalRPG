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
	if caps.Sessions || caps.ContextCache || caps.StructuredOutput {
		t.Fatalf("a plain provider must not claim sessions, caching, or structured output: %+v", caps)
	}
}

type stubStructuredProvider struct {
	stubToolProvider
}

func (s *stubStructuredProvider) StructuredOutputCapable() bool { return true }

func TestDescribeReportsStructuredOutput(t *testing.T) {
	caps := harness.Describe(&stubStructuredProvider{})
	if !caps.StructuredOutput {
		t.Fatalf("expected StructuredOutput to be true, got %+v", caps)
	}
}
