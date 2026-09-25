package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestGuardImageBytes(t *testing.T) {
	if _, failure := guardImageBytes(nil); failure == nil || failure.Code != harness.FailureProviderError {
		t.Fatalf("guardImageBytes(nil) = %v, want provider_error", failure)
	}
	if _, failure := guardImageBytes([]byte{}); failure == nil {
		t.Fatal("guardImageBytes(empty) returned no failure")
	}
	if got, failure := guardImageBytes([]byte("<svg/>")); failure != nil || string(got) != "<svg/>" {
		t.Fatalf("guardImageBytes(svg) = %q, %v; want the bytes and no failure", got, failure)
	}
}
