package pricing

import (
	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// Micros is a currency amount in millionths, so costs are exact integers.
type Micros int64

// Price is a provider's rate card. All fields are in micros.
type Price struct {
	PerMillionInput  Micros
	PerMillionOutput Micros
	PerCharacter     Micros
	PerRequest       Micros
}

// BuiltinPrices are published list rates for the metered adapters LocalRPG ships
// presets for. A config price always wins. Local and built-in adapters are absent
// on purpose: they run on the user's own machine and are not metered.
//
// Rates are USD micros, captured on 2026-09-28 from Google Cloud "Agent Platform
// Pricing" and the OpenAI pricing page; see
// docs/proposals/2026-09-28-provider-costs-and-usage-research.md. A model with no
// row here falls back to its adapter-wide row when one exists.
var BuiltinPrices = []config.PriceConfig{
	// Gemini text. The 3.8 Flash row is the introductory rate, which doubles on
	// 2027-01-01; the adapter-wide row follows the shipped preset model.
	{Provider: string(provider.KeyLLMGemini), Model: "gemini-3.8-flash", PerMillionInput: 750_000, PerMillionOutput: 3_750_000},
	{Provider: string(provider.KeyLLMGemini), Model: "gemini-3.5-flash-lite", PerMillionInput: 300_000, PerMillionOutput: 2_500_000},
	{Provider: string(provider.KeyLLMGemini), Model: "gemini-3.1-flash-lite", PerMillionInput: 250_000, PerMillionOutput: 1_500_000},
	{Provider: string(provider.KeyLLMGemini), Model: "gemini-2.5-flash", PerMillionInput: 300_000, PerMillionOutput: 2_500_000},
	{Provider: string(provider.KeyLLMGemini), Model: "gemini-2.5-flash-lite", PerMillionInput: 100_000, PerMillionOutput: 400_000},
	{Provider: string(provider.KeyLLMGemini), Model: "gemini-2.0-flash", PerMillionInput: 150_000, PerMillionOutput: 600_000},
	{Provider: string(provider.KeyLLMGemini), PerMillionInput: 750_000, PerMillionOutput: 3_750_000},

	// OpenAI chat, keyed by the OpenAI endpoint because llm:openaichat is a
	// generic adapter that usually points at a local server.
	{Provider: string(openAI('l', "api.openai.com")), Model: "gpt-4o", PerMillionInput: 2_500_000, PerMillionOutput: 10_000_000},
	{Provider: string(openAI('l', "api.openai.com")), Model: "gpt-4o-mini", PerMillionInput: 150_000, PerMillionOutput: 600_000},
	{Provider: string(openAI('l', "api.openai.com")), PerMillionInput: 150_000, PerMillionOutput: 600_000},

	// Speech, billed per character. ElevenLabs is a cloud-only adapter; the
	// generic HTTP adapter is priced at the OpenAI endpoint only.
	{Provider: string(provider.KeyTTSElevenLabs), PerCharacter: 80},
	{Provider: string(openAI('t', "api.openai.com")), PerCharacter: 15},

	// Gemini image generation, billed per image.
	{Provider: string(provider.KeyImageGemini), Model: "imagen-3.0-generate-002", PerRequest: 40_000},
	{Provider: string(provider.KeyImageGemini), Model: "imagen-3.0-fast-generate-001", PerRequest: 20_000},

	// OpenAI embeddings, keyed by endpoint for the same reason as chat.
	{Provider: string(openAI('e', "api.openai.com")), Model: "text-embedding-3-small", PerMillionInput: 20_000},
	{Provider: string(openAI('e', "api.openai.com")), Model: "text-embedding-3-large", PerMillionInput: 130_000},
}

// openAI builds the endpoint-scoped key for an OpenAI adapter: 'l' for chat, 't'
// for speech, 'e' for embeddings.
func openAI(kind rune, host string) provider.Key {
	base := provider.KeyLLMOpenAIChat
	switch kind {
	case 't':
		base = provider.KeyTTSHTTP
	case 'e':
		base = provider.KeyEmbeddingOpenAI
	}
	return provider.InstanceOrSelf(base, host)
}

// BatchDiscountNumerator and BatchDiscountDenominator express a batch call's
// price as a fraction of the interactive rate. The Gemini batch API is half
// price; an operator can change these if a provider's discount differs.
var (
	BatchDiscountNumerator   Micros = 1
	BatchDiscountDenominator Micros = 2
)

// CostMicros returns the cost of one usage record under a price. A zero price
// yields zero, never a guess. A batch record is priced at the batch discount.
func CostMicros(u harness.Usage, p Price) Micros {
	cost := Micros(u.InputTokens) * p.PerMillionInput / 1_000_000
	cost += Micros(u.OutputTokens) * p.PerMillionOutput / 1_000_000
	cost += Micros(u.Characters) * p.PerCharacter
	cost += Micros(u.Requests) * p.PerRequest
	if u.Batch && BatchDiscountDenominator != 0 {
		cost = cost * BatchDiscountNumerator / BatchDiscountDenominator
	}
	return cost
}

// Resolve finds the price for a recorded key and model, most specific first: the
// exact key and model, the exact key, the parent adapter and model, then the
// parent adapter. Configured entries win over the built-in table at every step.
func Resolve(providerKey, model string, cfg *config.Config) Price {
	candidates := candidateKeys(providerKey)
	if cfg != nil {
		for _, candidate := range candidates {
			if p, ok := lookup(cfg.Providers.Prices, candidate, model); ok {
				return p
			}
		}
	}
	for _, candidate := range candidates {
		if p, ok := lookup(BuiltinPrices, candidate, model); ok {
			return p
		}
	}
	return Price{}
}

// candidateKeys is the lookup ladder for a recorded key, most specific first. A
// key that does not parse is used verbatim, so a legacy row that matches no
// entry simply yields no price.
func candidateKeys(key string) []string {
	parsed, err := provider.ParseKey(key)
	if err != nil {
		return nil
	}
	if _, ok := parsed.Instance(); ok {
		return []string{string(parsed), string(parsed.Parent())}
	}
	return []string{string(parsed)}
}

func lookup(entries []config.PriceConfig, providerKey, model string) (Price, bool) {
	var providerWide *config.PriceConfig
	for i := range entries {
		entry := entries[i]
		if entry.Provider != providerKey {
			continue
		}
		if entry.Model == model && model != "" {
			return toPrice(entry), true
		}
		if entry.Model == "" && providerWide == nil {
			providerWide = &entries[i]
		}
	}
	if providerWide != nil {
		return toPrice(*providerWide), true
	}
	return Price{}, false
}

func toPrice(c config.PriceConfig) Price {
	return Price{
		PerMillionInput:  Micros(c.PerMillionInput),
		PerMillionOutput: Micros(c.PerMillionOutput),
		PerCharacter:     Micros(c.PerCharacter),
		PerRequest:       Micros(c.PerRequest),
	}
}

// Weight is a single comparable cost figure for a rate card, so a provider chain
// can be ordered cheapest first. Within one family only one dimension is priced,
// so the sum ranks instances of that family the way a user expects.
func (p Price) Weight() Micros {
	return p.PerMillionInput + p.PerMillionOutput + p.PerCharacter + p.PerRequest
}

// Weight resolves a provider key and model to a comparable cost in micros. ok is
// false when the ledger knows no price for the pair, so an unpriced instance
// sorts last in a cheapest chain.
func Weight(providerKey, model string, cfg *config.Config) (int64, bool) {
	p := Resolve(providerKey, model, cfg)
	if p == (Price{}) {
		return 0, false
	}
	return int64(p.Weight()), true
}

// RouterChainPrice returns the accessor Router.SetChainPrice wants, so a
// cheapest chain orders by the ledger's price for each member's key and model.
func RouterChainPrice(cfg *config.Config, router *harness.Router) func(string) (int64, bool) {
	return func(providerID string) (int64, bool) {
		key, ok := router.ProviderKeyForRole(providerID)
		if !ok || key == "" {
			return 0, false
		}
		model := ""
		if role, ok := cfg.Agents.Roles[providerID]; ok {
			model = role.Model
		}
		return Weight(string(key), model, cfg)
	}
}
