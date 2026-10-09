package config

// Selection rules for a provider chain. A rule orders the chain, and the chain
// is then tried in order, so a rule and a fallback are the same mechanism.
const (
	// SelectFirst keeps the declared order: the first instance that builds.
	SelectFirst = "first"
	// SelectCheapest orders by the pricing ledger, cheapest first. An instance
	// the ledger cannot price is unknown and sorts last.
	SelectCheapest = "cheapest"
	// SelectLocalFirst orders by provider tier: offline first, then a local
	// server, then cloud. The declared order is kept within a tier.
	SelectLocalFirst = "local-first"
	// SelectByTag prefers instances whose provider descriptor carries the tag as
	// a feature name or a tier name.
	SelectByTag = "by-tag"
)

// ChainConfig declares an ordered provider chain and the rule that orders it. A
// media purpose stores one; a role carries the same three fields directly.
type ChainConfig struct {
	// Chain names provider instances, tried in order. Empty means the existing
	// single provider, with no fallback.
	Chain []string `yaml:"chain,omitempty" json:"chain,omitempty"`
	// Select orders the chain: first (the default), cheapest, local-first, or
	// by-tag.
	Select string `yaml:"select,omitempty" json:"select,omitempty"`
	// Tag is the feature or tier name by-tag prefers.
	Tag string `yaml:"tag,omitempty" json:"tag,omitempty"`
}

// Rule returns the configured selection rule, defaulting to first so a chain
// written before the key existed keeps its declared order.
func (c ChainConfig) Rule() string {
	if c.Select == "" {
		return SelectFirst
	}
	return c.Select
}

// ValidSelectRule reports whether rule is a selection rule this build
// understands.
func ValidSelectRule(rule string) bool {
	switch rule {
	case SelectFirst, SelectCheapest, SelectLocalFirst, SelectByTag:
		return true
	default:
		return false
	}
}
