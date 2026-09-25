package harness

import (
	"context"
	"errors"
	"strings"
	"time"
)

// FailureCode is the bounded reason a generation failed. It is the single key
// shared by the JSON response, the trace, the OpenTelemetry spans, and the
// frontend, so a failure is never described by an arbitrary string.
type FailureCode string

const (
	FailureProviderUnavailable FailureCode = "provider_unavailable"
	FailureProviderError       FailureCode = "provider_error"
	FailureEmptyResponse       FailureCode = "empty_response"
	FailureParseError          FailureCode = "parse_error"
	FailureTimeout             FailureCode = "timeout"
	FailureContextTooLarge     FailureCode = "context_too_large"
	FailureInvalidRequest      FailureCode = "invalid_request"
)

// Attempt records one provider invocation in a fallback chain.
type Attempt struct {
	Role       string      `json:"role"`
	Provider   string      `json:"provider"`
	Code       FailureCode `json:"code"`
	Detail     string      `json:"detail,omitempty"`
	DurationMS int64       `json:"duration_ms"`
}

// GenerationFailure is the single error shape for every generation path.
type GenerationFailure struct {
	Code         FailureCode `json:"code"`
	Message      string      `json:"message"`
	Attempts     []Attempt   `json:"attempts,omitempty"`
	FinishReason string      `json:"finish_reason,omitempty"`
	PromptChars  int         `json:"prompt_chars,omitempty"`
	ContextChars int         `json:"context_chars,omitempty"`
	ElapsedMS    int64       `json:"elapsed_ms,omitempty"`
	// Cause is the underlying provider error, kept out of JSON so callers can
	// still errors.Is/As through the failure.
	Cause error `json:"-"`
}

func (f *GenerationFailure) Error() string {
	if f == nil {
		return ""
	}
	return f.Message
}

// Unwrap exposes the underlying provider error to errors.Is/As.
func (f *GenerationFailure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.Cause
}

// ClassifyProviderError maps an arbitrary provider error to a bounded code. The
// classification is textual because providers surface their own error types, and
// a context-length failure is the one case a user can act on.
func ClassifyProviderError(err error) FailureCode {
	if err == nil {
		return FailureProviderError
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureTimeout
	}
	lower := strings.ToLower(err.Error())
	for _, marker := range []string{
		"context length", "maximum context", "too many tokens", "token limit", "context window",
	} {
		if strings.Contains(lower, marker) {
			return FailureContextTooLarge
		}
	}
	return FailureProviderError
}

// FailureFrom extracts a *GenerationFailure from an error chain.
func FailureFrom(err error) (*GenerationFailure, bool) {
	var failure *GenerationFailure
	if errors.As(err, &failure) {
		return failure, true
	}
	return nil, false
}

// NewFailure builds a failure with a single attempt.
func NewFailure(code FailureCode, message, role, provider string, elapsed time.Duration) *GenerationFailure {
	return &GenerationFailure{
		Code:      code,
		Message:   message,
		ElapsedMS: elapsed.Milliseconds(),
		Attempts: []Attempt{{
			Role:       role,
			Provider:   provider,
			Code:       code,
			DurationMS: elapsed.Milliseconds(),
		}},
	}
}
