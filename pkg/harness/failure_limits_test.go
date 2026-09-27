package harness

import (
	"errors"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestClassifyRateLimited(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want FailureCode
	}{
		{"typed with retry-after", &provider.RateLimitedError{RetryAfter: 12 * time.Second}, FailureRateLimited},
		{"typed without retry-after", &provider.RateLimitedError{}, FailureRateLimited},
		{"marker 429", errors.New("server returned 429 Too Many Requests"), FailureRateLimited},
		{"marker resource exhausted", errors.New("RESOURCE_EXHAUSTED"), FailureRateLimited},
		{"funds typed", &provider.InsufficientFundsError{Message: "insufficient_credits"}, FailureInsufficientFunds},
		{"funds marker", errors.New("insufficient_quota"), FailureInsufficientFunds},
	}
	for _, tc := range cases {
		if got := ClassifyProviderError(tc.err); got != tc.want {
			t.Errorf("%s: ClassifyProviderError = %q, want %q", tc.name, got, tc.want)
		}
	}
}
