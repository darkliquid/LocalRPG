package harness

import (
	"slices"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestOrderChainFirst(t *testing.T) {
	got := OrderChain([]string{"a", "b"}, config.SelectFirst, "", nil, nil)
	if got[0] != "a" {
		t.Fatalf("first = %v", got)
	}
}

func TestOrderChainCheapest(t *testing.T) {
	price := func(id string) (int64, bool) { return map[string]int64{"a": 10, "b": 1}[id], true }
	if got := OrderChain([]string{"a", "b"}, config.SelectCheapest, "", price, nil); got[0] != "b" {
		t.Fatalf("cheapest = %v", got)
	}
}

func TestOrderChainCheapestSortsUnpricedLast(t *testing.T) {
	price := func(id string) (int64, bool) {
		if id == "b" {
			return 0, false
		}
		return 5, true
	}
	got := OrderChain([]string{"b", "a"}, config.SelectCheapest, "", price, nil)
	if got[0] != "a" {
		t.Fatalf("an unpriced instance must sort last, got %v", got)
	}
}

func TestOrderChainLocalFirst(t *testing.T) {
	tier := func(id string) (provider.Tier, []provider.Feature) {
		return map[string]provider.Tier{"cloud": provider.TierCloud, "local": provider.TierOfflineBasic}[id], nil
	}
	if got := OrderChain([]string{"cloud", "local"}, config.SelectLocalFirst, "", nil, tier); got[0] != "local" {
		t.Fatalf("local-first = %v", got)
	}
}

func TestOrderChainLocalFirstKeepsDeclaredOrderWithinATier(t *testing.T) {
	tier := func(id string) (provider.Tier, []provider.Feature) {
		return map[string]provider.Tier{"a": provider.TierCloud, "b": provider.TierCloud}[id], nil
	}
	got := OrderChain([]string{"a", "b"}, config.SelectLocalFirst, "", nil, tier)
	if got[0] != "a" || got[1] != "b" {
		t.Fatalf("a stable sort must keep the declared order, got %v", got)
	}
}

func TestOrderChainByTag(t *testing.T) {
	tier := func(id string) (provider.Tier, []provider.Feature) {
		if id == "y" {
			return provider.TierCloud, []provider.Feature{provider.FeatureTools}
		}
		return provider.TierCloud, nil
	}
	got := OrderChain([]string{"x", "y"}, config.SelectByTag, string(provider.FeatureTools), nil, tier)
	if got[0] != "y" {
		t.Fatalf("by-tag = %v", got)
	}
}

func TestOrderChainByTagMatchesATierName(t *testing.T) {
	tier := func(id string) (provider.Tier, []provider.Feature) {
		return map[string]provider.Tier{"local": provider.TierOfflineBasic, "cloud": provider.TierCloud}[id], nil
	}
	got := OrderChain([]string{"cloud", "local"}, config.SelectByTag, string(provider.TierOfflineBasic), nil, tier)
	if got[0] != "local" {
		t.Fatalf("by-tag should match a tier name, got %v", got)
	}
}

func TestOrderChainUnknownRuleKeepsTheDeclaredOrder(t *testing.T) {
	got := OrderChain([]string{"a", "b"}, "bogus", "", nil, nil)
	if got[0] != "a" || got[1] != "b" {
		t.Fatalf("an unknown rule must keep the declared order, got %v", got)
	}
}

// TestOrderChainIsAPermutation guards the invariant every rule shares: the
// ordered chain holds exactly the declared members, once each.
func TestOrderChainIsAPermutation(t *testing.T) {
	in := []string{"a", "b", "c", "d"}
	price := func(id string) (int64, bool) { return map[string]int64{"a": 4, "c": 1}[id], id != "d" }
	tier := func(id string) (provider.Tier, []provider.Feature) {
		rank := map[string]provider.Tier{"a": provider.TierCloud, "b": provider.TierOfflineNeural, "c": provider.TierLocalServer}
		return rank[id], nil
	}
	for _, rule := range []string{config.SelectFirst, config.SelectCheapest, config.SelectLocalFirst, config.SelectByTag} {
		got := OrderChain(in, rule, string(provider.FeatureTools), price, tier)
		if len(got) != len(in) {
			t.Fatalf("%s: length = %d, want %d", rule, len(got), len(in))
		}
		sorted := slices.Clone(got)
		slices.Sort(sorted)
		if !slices.Equal(sorted, in) {
			t.Fatalf("%s: %v is not a permutation of %v", rule, got, in)
		}
	}
}

// TestOrderChainDoesNotMutateTheInput keeps the caller's declared chain intact,
// because it is also the config's.
func TestOrderChainDoesNotMutateTheInput(t *testing.T) {
	in := []string{"a", "b"}
	price := func(id string) (int64, bool) { return map[string]int64{"a": 10, "b": 1}[id], true }
	OrderChain(in, config.SelectCheapest, "", price, nil)
	if in[0] != "a" || in[1] != "b" {
		t.Fatalf("OrderChain mutated its input: %v", in)
	}
}
