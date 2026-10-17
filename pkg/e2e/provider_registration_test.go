//go:build e2e

package e2e

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/provider"
)

// TestProvidersAreRegisteredInE2E verifies that provider/all is linked and
// built-in provider adapters like CLI and OpenAIChat are registered in e2e tests.
func TestProvidersAreRegisteredInE2E(t *testing.T) {
	if _, ok := provider.Lookup(string(provider.KeyLLMCLI)); !ok {
		t.Fatalf("provider %s is not registered in e2e package!", provider.KeyLLMCLI)
	}
	if _, ok := provider.Lookup(string(provider.KeyLLMOpenAIChat)); !ok {
		t.Fatalf("provider %s is not registered in e2e package!", provider.KeyLLMOpenAIChat)
	}
}
