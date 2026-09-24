package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
)

// SelectStrategy determines how the assembled context should reach the provider,
// choosing the most efficient mechanism the provider and session state support.
func SelectStrategy(caps Capabilities, stored *ProviderSession, tip int, prefixHash, model string) ContextStrategy {
	if caps.Sessions && stored != nil && stored.ID != "" && stored.ThroughTurn == tip && stored.Model == model && stored.PrefixHash == prefixHash {
		return StrategyServerSession
	}
	if caps.ContextCache && prefixHash != "" {
		return StrategyCachedPrefix
	}
	return StrategyFullPrompt
}

// BuildPrefix renders the stable, cacheable prefix sections of the prompt:
// rules, lore, and voice profiles catalog.
func BuildPrefix(rulesPrompt, lorePrompt string, profiles []config.VoiceProfile) string {
	var sb strings.Builder
	if rules := rulesSection(rulesPrompt); rules != "" {
		sb.WriteString(rules)
	}
	if lore := loreSection(lorePrompt); lore != "" {
		sb.WriteString(lore)
	}
	if len(profiles) > 0 {
		sb.WriteString(FormatVoiceProfilesCatalog(profiles) + "\n")
	}
	return sb.String()
}

// PrefixHash computes the SHA256 hex digest of the stable prefix. An empty
// prefix produces an empty hash.
func PrefixHash(prefix string) string {
	if strings.TrimSpace(prefix) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(prefix))
	return hex.EncodeToString(sum[:])
}
