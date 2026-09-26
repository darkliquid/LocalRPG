package harness

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestClassifyProviderError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want FailureCode
	}{
		{"deadline", context.DeadlineExceeded, FailureTimeout},
		{"wrapped deadline", fmt.Errorf("call: %w", context.DeadlineExceeded), FailureTimeout},
		{"context length", errors.New("This model's maximum context length is 8192 tokens"), FailureContextTooLarge},
		{"too many tokens", errors.New("too many tokens requested"), FailureContextTooLarge},
		{"generic", errors.New("connection reset by peer"), FailureProviderError},
		{"nil", nil, FailureProviderError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyProviderError(tt.err); got != tt.want {
				t.Fatalf("ClassifyProviderError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFailureFrom(t *testing.T) {
	want := &GenerationFailure{Code: FailureEmptyResponse, Message: "empty"}
	got, ok := FailureFrom(fmt.Errorf("role failed: %w", want))
	if !ok || got != want {
		t.Fatalf("FailureFrom() = %v, %v; want the original failure", got, ok)
	}
	if _, ok := FailureFrom(errors.New("plain")); ok {
		t.Fatal("FailureFrom() found a failure in a plain error")
	}
}

func TestGenerationFailureError(t *testing.T) {
	var nilFailure *GenerationFailure
	if nilFailure.Error() != "" {
		t.Fatal("nil failure should have an empty message")
	}
	if (&GenerationFailure{Message: "boom"}).Error() != "boom" {
		t.Fatal("failure message not returned by Error()")
	}
}

func TestSummarizeAttemptsPrefersLastDetail(t *testing.T) {
	attempts := []Attempt{
		{Role: "gm", Provider: "a", Code: FailureProviderError, Detail: "first"},
		{Role: "gm", Provider: "b", Code: FailureEmptyResponse, Detail: "second"},
	}
	if got := SummarizeAttempts(attempts, "generic"); got != "second" {
		t.Fatalf("got %q, want %q", got, "second")
	}
	if got := SummarizeAttempts(nil, "generic"); got != "generic" {
		t.Fatalf("got %q, want %q", got, "generic")
	}
}

func TestGenerationFailureSummary(t *testing.T) {
	if got := (&GenerationFailure{Code: FailureTimeout}).Summary(); got != "timeout" {
		t.Fatalf("got %q, want %q", got, "timeout")
	}
	f := &GenerationFailure{Message: "boom"}
	if got := f.Summary(); got != "boom" {
		t.Fatalf("got %q, want %q", got, "boom")
	}
}
