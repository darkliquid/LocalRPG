package pricing

import (
	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
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

// BuiltinPrices are the known rates for providers LocalRPG ships presets for.
// They are deliberately conservative defaults; a config price always wins.
var BuiltinPrices = []config.PriceConfig{
	{Provider: "gemini", PerMillionInput: 125_000, PerMillionOutput: 500_000},
	{Provider: "openaichat", PerMillionInput: 150_000, PerMillionOutput: 600_000},
	{Provider: "elevenlabs", PerCharacter: 0},
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

// Resolve finds the price for a provider and model: an exact provider+model
// config entry, then a provider entry, then a built-in, then zero.
func Resolve(providerKey, model string, cfg *config.Config) Price {
	if cfg != nil {
		if p, ok := lookup(cfg.Providers.Prices, providerKey, model); ok {
			return p
		}
	}
	if p, ok := lookup(BuiltinPrices, providerKey, model); ok {
		return p
	}
	return Price{}
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
