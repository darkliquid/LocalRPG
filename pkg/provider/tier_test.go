package provider

import "testing"

func TestTierCaveatIsNonEmptyForEveryTier(t *testing.T) {
	for _, tier := range []Tier{TierOfflineBasic, TierOfflineNeural, TierLocalServer, TierCloud} {
		if !tier.Valid() {
			t.Errorf("%q is not valid", tier)
		}
		if TierCaveat(tier) == "" {
			t.Errorf("%q has no caveat", tier)
		}
	}
	if Tier("nonsense").Valid() {
		t.Error("an unknown tier should be invalid")
	}
}

func TestEffectiveCaveatPrefersTheDescriptor(t *testing.T) {
	custom := Descriptor{Tier: TierCloud, Caveat: "Only the good stuff."}
	if got := custom.EffectiveCaveat(); got != "Only the good stuff." {
		t.Errorf("EffectiveCaveat = %q, want the descriptor's own", got)
	}
	fallback := Descriptor{Tier: TierOfflineBasic}
	if got := fallback.EffectiveCaveat(); got != TierCaveat(TierOfflineBasic) {
		t.Errorf("EffectiveCaveat = %q, want the tier default", got)
	}
}

func TestValidateRejectsTierFeatureMismatch(t *testing.T) {
	cases := []struct {
		name string
		d    Descriptor
	}{
		{"cloud without key", Descriptor{ID: "llm:x", Family: FamilyLLM, Tier: TierCloud}},
		{"offline without offline", Descriptor{ID: "tts:x", Family: FamilyTTS, Tier: TierOfflineBasic}},
		{"empty tier", Descriptor{ID: "stt:x", Family: FamilySTT}},
	}
	for _, c := range cases {
		if err := validateOne(c.d); err == nil {
			t.Errorf("%s: expected an error", c.name)
		}
	}
	if err := validateOne(Descriptor{ID: "tts:x", Family: FamilyTTS, Tier: TierOfflineBasic, Features: []Feature{FeatureOffline}}); err != nil {
		t.Errorf("a consistent descriptor should pass: %v", err)
	}
}
