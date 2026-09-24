package harness_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestSelectStrategy(t *testing.T) {
	cases := []struct {
		name       string
		caps       harness.Capabilities
		stored     *harness.ProviderSession
		tip        int
		prefixHash string
		model      string
		want       harness.ContextStrategy
	}{
		{
			name:       "stateless",
			caps:       harness.Capabilities{},
			stored:     nil,
			tip:        5,
			prefixHash: "h",
			model:      "m",
			want:       harness.StrategyFullPrompt,
		},
		{
			name:       "cached prefix",
			caps:       harness.Capabilities{ContextCache: true},
			stored:     nil,
			tip:        5,
			prefixHash: "h",
			model:      "m",
			want:       harness.StrategyCachedPrefix,
		},
		{
			name: "session mismatched tip",
			caps: harness.Capabilities{Sessions: true},
			stored: &harness.ProviderSession{
				Provider:    "gemini",
				ID:          "x",
				ThroughTurn: 1,
				Model:       "m",
				PrefixHash:  "h",
			},
			tip:        5,
			prefixHash: "h",
			model:      "m",
			want:       harness.StrategyFullPrompt,
		},
		{
			name: "session mismatched model",
			caps: harness.Capabilities{Sessions: true},
			stored: &harness.ProviderSession{
				Provider:    "gemini",
				ID:          "x",
				ThroughTurn: 5,
				Model:       "old-model",
				PrefixHash:  "h",
			},
			tip:        5,
			prefixHash: "h",
			model:      "m",
			want:       harness.StrategyFullPrompt,
		},
		{
			name: "session mismatched prefix",
			caps: harness.Capabilities{Sessions: true},
			stored: &harness.ProviderSession{
				Provider:    "gemini",
				ID:          "x",
				ThroughTurn: 5,
				Model:       "m",
				PrefixHash:  "old-hash",
			},
			tip:        5,
			prefixHash: "h",
			model:      "m",
			want:       harness.StrategyFullPrompt,
		},
		{
			name: "session matching tip",
			caps: harness.Capabilities{Sessions: true},
			stored: &harness.ProviderSession{
				Provider:    "gemini",
				ID:          "x",
				ThroughTurn: 5,
				Model:       "m",
				PrefixHash:  "h",
			},
			tip:        5,
			prefixHash: "h",
			model:      "m",
			want:       harness.StrategyServerSession,
		},
		{
			name: "mismatched session with context cache falls back to cached prefix",
			caps: harness.Capabilities{Sessions: true, ContextCache: true},
			stored: &harness.ProviderSession{
				Provider:    "gemini",
				ID:          "x",
				ThroughTurn: 1,
				Model:       "m",
				PrefixHash:  "h",
			},
			tip:        5,
			prefixHash: "h",
			model:      "m",
			want:       harness.StrategyCachedPrefix,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := harness.SelectStrategy(tc.caps, tc.stored, tc.tip, tc.prefixHash, tc.model)
			if got != tc.want {
				t.Errorf("%s: got %s want %s", tc.name, got, tc.want)
			}
		})
	}
}

func TestBuildPrefixAndHash(t *testing.T) {
	rules := "Roll 1d20 for checks."
	lore := "A dark and stormy world."
	profiles := []config.VoiceProfile{
		{ID: "narrator", Name: "Narrator", Description: "Deep and calm"},
	}

	prefix := harness.BuildPrefix(rules, lore, profiles)
	if prefix == "" {
		t.Fatal("expected non-empty prefix")
	}

	hash1 := harness.PrefixHash(prefix)
	if hash1 == "" {
		t.Fatal("expected non-empty hash")
	}

	hash2 := harness.PrefixHash(prefix)
	if hash1 != hash2 {
		t.Fatalf("expected deterministic hash: %s != %s", hash1, hash2)
	}

	emptyHash := harness.PrefixHash("")
	if emptyHash != "" {
		t.Fatalf("expected empty string hash for empty prefix, got: %s", emptyHash)
	}
}
