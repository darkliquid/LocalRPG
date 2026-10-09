package media

import (
	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/pricing"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// SelectChain orders a chain of named media providers by rule and returns the
// names in the order to try. Each name resolves in the given family ("tts",
// "stt", or "image"), or the family default when empty. A name that resolves to
// no registered adapter is dropped, because validation already rejected an
// unknown name and a stale one must not fail a turn.
func SelectChain(names []string, rule, tag, family string, cfg *config.Config) []string {
	resolved := make([]string, 0, len(names))
	tiers := make(map[string]provider.Tier, len(names))
	features := make(map[string][]provider.Feature, len(names))
	prices := make(map[string]int64, len(names))
	priced := make(map[string]bool, len(names))

	for _, name := range names {
		tier, feats, key, model, ok := mediaDescriptor(family, name, cfg)
		if !ok {
			continue
		}
		resolved = append(resolved, name)
		tiers[name] = tier
		features[name] = feats
		if weight, ok := pricing.Weight(string(key), model, cfg); ok {
			prices[name] = weight
			priced[name] = true
		}
	}

	return harness.OrderChain(resolved, rule, tag, func(name string) (int64, bool) {
		return prices[name], priced[name]
	}, func(name string) (provider.Tier, []provider.Feature) {
		return tiers[name], features[name]
	})
}

// PurposeChainNames returns the names to try for a purpose, in order: the
// declared chain ordered by its rule, or the purpose's single configured
// provider when no chain is declared.
func PurposeChainNames(cfg *config.Config, p config.Purpose) []string {
	if cfg == nil {
		return nil
	}
	chain := cfg.Media.PurposeChain(p)
	if len(chain.Chain) == 0 {
		return []string{cfg.Media.ProviderForPurpose(p)}
	}
	return SelectChain(chain.Chain, chain.Rule(), chain.Tag, config.PurposeFamily(p), cfg)
}

// mediaDescriptor resolves a family name to its configuration, its registered
// adapter's tier and features, and its canonical key and model. ok is false when
// the name resolves to no registered adapter.
func mediaDescriptor(family, name string, cfg *config.Config) (provider.Tier, []provider.Feature, provider.Key, string, bool) {
	var key provider.Key
	var model string
	var known bool
	switch family {
	case "tts":
		if !familyKnows(cfg.Media.TTSProviders, name) {
			return "", nil, "", "", false
		}
		tts := cfg.Media.TTSFor(name)
		key, known = TTSKeyFor(tts)
		model = tts.Model
	case "stt":
		if !familyKnows(cfg.Media.STTProviders, name) {
			return "", nil, "", "", false
		}
		stt := cfg.Media.STTFor(name)
		key, known = STTKeyFor(stt)
		model = stt.Model
	case "image":
		if !familyKnows(cfg.Media.ImageProviders, name) {
			return "", nil, "", "", false
		}
		image := cfg.Media.ImageFor(name)
		key, known = ImageKeyFor(image)
		model = image.Model
	default:
		return "", nil, "", "", false
	}
	if !known {
		return "", nil, "", "", false
	}
	reg, ok := provider.Lookup(string(key.Parent()))
	if !ok {
		return "", nil, "", "", false
	}
	return reg.Descriptor.Tier, reg.Descriptor.Features, key, model, true
}

// familyKnows reports whether a name is the family default or a configured entry
// of that family. It is what lets a stale chain name be dropped rather than
// silently resolving to the default, because TTSFor and its siblings fall back.
func familyKnows[V interface{}](providers map[string]V, name string) bool {
	if name == config.ReservedProviderName {
		return true
	}
	_, ok := providers[name]
	return ok
}
