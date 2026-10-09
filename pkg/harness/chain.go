package harness

import (
	"cmp"
	"slices"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// OrderChain orders a provider chain by its selection rule and returns a
// permutation of ids. A rule only reorders: the chain is then tried in order, so
// a rule and a fallback are the same mechanism.
//
// price reports an instance's cost in micros and whether the ledger knows it; an
// unpriced instance is unknown and sorts last. tier reports an instance's tier
// and features. Either accessor may be nil, in which case a rule that needs it
// keeps the declared order.
func OrderChain(ids []string, rule, tag string, price func(string) (int64, bool), tier func(string) (provider.Tier, []provider.Feature)) []string {
	out := slices.Clone(ids)
	switch rule {
	case config.SelectCheapest:
		slices.SortStableFunc(out, func(a, b string) int {
			pa, aKnown := chainPrice(price, a)
			pb, bKnown := chainPrice(price, b)
			switch {
			case aKnown && !bKnown:
				return -1
			case bKnown && !aKnown:
				return 1
			case aKnown && bKnown:
				return cmp.Compare(pa, pb)
			default:
				return 0
			}
		})
	case config.SelectLocalFirst:
		slices.SortStableFunc(out, func(a, b string) int {
			return cmp.Compare(chainTierRank(tier, a), chainTierRank(tier, b))
		})
	case config.SelectByTag:
		tag = strings.TrimSpace(tag)
		slices.SortStableFunc(out, func(a, b string) int {
			hasA, hasB := chainHasTag(tier, a, tag), chainHasTag(tier, b, tag)
			switch {
			case hasA == hasB:
				return 0
			case hasA:
				return -1
			default:
				return 1
			}
		})
	}
	return out
}

func chainPrice(price func(string) (int64, bool), id string) (int64, bool) {
	if price == nil {
		return 0, false
	}
	return price(id)
}

// chainTierRank orders tiers most local first. An instance the accessor does not
// know sorts last.
func chainTierRank(tier func(string) (provider.Tier, []provider.Feature), id string) int {
	if tier == nil {
		return 0
	}
	t, _ := tier(id)
	switch t {
	case provider.TierOfflineBasic:
		return 0
	case provider.TierOfflineNeural:
		return 1
	case provider.TierLocalServer:
		return 2
	case provider.TierCloud:
		return 3
	default:
		return 4
	}
}

// chainHasTag reports whether an instance's descriptor carries tag as a feature
// name or names it as its tier.
func chainHasTag(tier func(string) (provider.Tier, []provider.Feature), id, tag string) bool {
	if tier == nil || tag == "" {
		return false
	}
	t, features := tier(id)
	if string(t) == tag {
		return true
	}
	for _, feature := range features {
		if string(feature) == tag {
			return true
		}
	}
	return false
}
