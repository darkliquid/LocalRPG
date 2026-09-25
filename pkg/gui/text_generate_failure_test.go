package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestPickFailureCode(t *testing.T) {
	if got := pickFailureCode(nil); got != harness.FailureProviderUnavailable {
		t.Fatalf("pickFailureCode(nil) = %q, want provider_unavailable", got)
	}
	attempts := []harness.Attempt{
		{Code: harness.FailureProviderError},
		{Code: harness.FailureParseError},
	}
	if got := pickFailureCode(attempts); got != harness.FailureParseError {
		t.Fatalf("pickFailureCode() = %q, want the last attempt code", got)
	}
}
