package pricing

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestCostFromTokens(t *testing.T) {
	p := Price{PerMillionInput: 1_000_000, PerMillionOutput: 2_000_000} // 1 and 2 per 1M
	u := harness.Usage{InputTokens: 1_000_000, OutputTokens: 500_000}
	if got := CostMicros(u, p); got != 2_000_000 {
		t.Fatalf("CostMicros = %d, want 2000000 micros (1 + 1)", got)
	}
}

func TestCostFromCharactersAndRequests(t *testing.T) {
	p := Price{PerCharacter: 30, PerRequest: 1000}
	u := harness.Usage{Characters: 100, Requests: 1}
	if got := CostMicros(u, p); got != 4000 {
		t.Fatalf("CostMicros = %d, want 4000 micros", got)
	}
}

func TestCostOfAnUnpricedCallIsZero(t *testing.T) {
	if got := CostMicros(harness.Usage{InputTokens: 999, Characters: 999}, Price{}); got != 0 {
		t.Fatalf("CostMicros = %d, want 0 for an unpriced call", got)
	}
}

func TestResolvePrefersModelThenProviderThenZero(t *testing.T) {
	cfg := &config.Config{Providers: config.ProvidersConfig{Prices: []config.PriceConfig{
		{Provider: "builtin:gemini", PerMillionInput: 1},
		{Provider: "builtin:gemini", Model: "gemini-2.5-pro", PerMillionInput: 2},
	}}}
	if got := Resolve("builtin:gemini", "gemini-2.5-pro", cfg); got.PerMillionInput != 2 {
		t.Fatalf("model override not used: %+v", got)
	}
	if got := Resolve("builtin:gemini", "other", cfg); got.PerMillionInput != 1 {
		t.Fatalf("provider fallback not used: %+v", got)
	}
	if got := Resolve("unknown", "", cfg); got != (Price{}) {
		t.Fatalf("unknown provider = %+v, want zero", got)
	}
}
