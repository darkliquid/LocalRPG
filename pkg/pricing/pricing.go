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

// BuiltinPrices are the known rates for adapters LocalRPG ships presets for.
// They are deliberately conservative defaults; a config price always wins.
var BuiltinPrices = []config.PriceConfig{
	{Provider: string(provider.KeyLLMGemini), PerMillionInput: 125_000, PerMillionOutput: 500_000},
	{Provider: string(provider.KeyLLMOpenAIChat), PerMillionInput: 150_000, PerMillionOutput: 600_000},
}

// CostMicros returns the cost of one usage record under a price. A zero price
// yields zero, never a guess.
func CostMicros(u harness.Usage, p Price) Micros {
	cost := Micros(u.InputTokens) * p.PerMillionInput / 1_000_000
	cost += Micros(u.OutputTokens) * p.PerMillionOutput / 1_000_000
	cost += Micros(u.Characters) * p.PerCharacter
	cost += Micros(u.Requests) * p.PerRequest
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
