package provider

import (
	"fmt"
	"time"
)

// RateLimitedError is a provider refusal that should unblock after a delay. A
// zero RetryAfter means the provider advertised no window and the caller
// chooses a default.
type RateLimitedError struct {
	RetryAfter time.Duration
	Message    string
}

func (e *RateLimitedError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "provider rate limit reached"
}

// InsufficientFundsError is a provider refusal caused by the account having no
// credit. It is never retried automatically.
type InsufficientFundsError struct{ Message string }

func (e *InsufficientFundsError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "provider rejected the request: insufficient funds"
}

// RateLimitedf builds a rate-limit error with an advertised backoff.
func RateLimitedf(retryAfter time.Duration, format string, args ...interface{}) error {
	return &RateLimitedError{RetryAfter: retryAfter, Message: fmt.Sprintf(format, args...)}
}

// InsufficientFundsf builds an insufficient-funds error.
func InsufficientFundsf(format string, args ...interface{}) error {
	return &InsufficientFundsError{Message: fmt.Sprintf(format, args...)}
}
