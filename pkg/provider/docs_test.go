package provider_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestLocalFirstPageMatchesTiers(t *testing.T) {
	pagePath := filepath.Join("..", "gui", "docs", "21-local-first.md")
	page, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatalf("read %s: %v", pagePath, err)
	}

	tiers := []provider.Tier{
		provider.TierOfflineBasic,
		provider.TierOfflineNeural,
		provider.TierLocalServer,
		provider.TierCloud,
	}

	for _, tier := range tiers {
		caveat := provider.TierCaveat(tier)
		if !bytes.Contains(page, []byte(caveat)) {
			t.Errorf("page %s is missing the caveat for %s (%q)", pagePath, tier, caveat)
		}

		tierCode := "`" + string(tier) + "`"
		if !bytes.Contains(page, []byte(tierCode)) {
			t.Errorf("page %s is missing the tier code for %s (%q)", pagePath, tier, tierCode)
		}
	}
}
