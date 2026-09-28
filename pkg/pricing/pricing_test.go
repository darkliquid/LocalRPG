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
		{Provider: "tts:gemini", PerMillionInput: 1},
		{Provider: "tts:gemini", Model: "gemini-2.5-pro", PerMillionInput: 2},
	}}}
	if got := Resolve("tts:gemini", "gemini-2.5-pro", cfg); got.PerMillionInput != 2 {
		t.Fatalf("model override not used: %+v", got)
	}
	if got := Resolve("tts:gemini", "other", cfg); got.PerMillionInput != 1 {
		t.Fatalf("provider fallback not used: %+v", got)
	}
	if got := Resolve("unknown", "", cfg); got != (Price{}) {
		t.Fatalf("unknown provider = %+v, want zero", got)
	}
}

func TestResolveFallsBackFromInstanceToAdapter(t *testing.T) {
	cfg := &config.Config{Providers: config.ProvidersConfig{Prices: []config.PriceConfig{
		{Provider: "tts:http", PerRequest: 10},
		{Provider: "tts:http@host-a", PerRequest: 99},
	}}}
	if got := Resolve("tts:http@host-a", "", cfg); got.PerRequest != 99 {
		t.Errorf("instance price = %d, want 99", got.PerRequest)
	}
	if got := Resolve("tts:http@host-b", "", cfg); got.PerRequest != 10 {
		t.Errorf("fallback price = %d, want 10", got.PerRequest)
	}
}

func TestResolvePrefersInstanceModelOverAdapterWide(t *testing.T) {
	cfg := &config.Config{Providers: config.ProvidersConfig{Prices: []config.PriceConfig{
		{Provider: "llm:openaichat", PerMillionInput: 1},
		{Provider: "llm:openaichat@host", PerMillionInput: 5},
		{Provider: "llm:openaichat@host", Model: "gpt-4o-mini", PerMillionInput: 9},
	}}}
	if got := Resolve("llm:openaichat@host", "gpt-4o-mini", cfg); got.PerMillionInput != 9 {
		t.Errorf("instance+model = %d, want 9", got.PerMillionInput)
	}
	if got := Resolve("llm:openaichat@host", "other", cfg); got.PerMillionInput != 5 {
		t.Errorf("instance-wide = %d, want 5", got.PerMillionInput)
	}
	if got := Resolve("llm:openaichat@other", "other", cfg); got.PerMillionInput != 1 {
		t.Errorf("adapter fallback = %d, want 1", got.PerMillionInput)
	}
}

func TestResolveIgnoresAKeyThatIsNotCanonical(t *testing.T) {
	cfg := &config.Config{Providers: config.ProvidersConfig{Prices: []config.PriceConfig{
		{Provider: "openaichat", PerMillionInput: 1},
	}}}
	if got := Resolve("openaichat", "", cfg); got != (Price{}) {
		t.Errorf("a non-canonical key must match nothing, got %+v", got)
	}
	if got := Resolve("llm:gemini", "", cfg); got.PerMillionInput != 750_000 {
		t.Errorf("canonical key must use the built-in rate, got %d", got.PerMillionInput)
	}
}

func TestBuiltinsAreScopedToTheVendorEndpoint(t *testing.T) {
	if got := Resolve("llm:openaichat@api.openai.com", "gpt-4o", nil); got.PerMillionInput != 2_500_000 {
		t.Errorf("endpoint-scoped built-in input = %d, want 2500000", got.PerMillionInput)
	}
	if got := Resolve("llm:openaichat@localhost:11434", "gpt-4o", nil); got != (Price{}) {
		t.Errorf("a local endpoint must not be billed at vendor rates, got %+v", got)
	}
	if got := Resolve("tts:http@api.openai.com", "", nil); got.PerCharacter != 15 {
		t.Errorf("OpenAI speech per character = %d, want 15", got.PerCharacter)
	}
	if got := Resolve("tts:http@localhost:8880", "", nil); got != (Price{}) {
		t.Errorf("a local speech server must not be billed, got %+v", got)
	}
}
