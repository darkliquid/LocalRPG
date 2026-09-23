package media

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/models"
)

func TestKokoroVariantMatchesModelSpec(t *testing.T) {
	if models.KokoroTTSVariant != KokoroModelV019 {
		t.Errorf("model spec variant %q does not match media variant %q", models.KokoroTTSVariant, KokoroModelV019)
	}
}

func TestKokoroV019SpeakerTable(t *testing.T) {
	want := map[string]int{
		"af": 0, "af_bella": 1, "af_nicole": 2, "af_sarah": 3, "af_sky": 4,
		"am_adam": 5, "am_michael": 6, "bf_emma": 7, "bf_isabella": 8,
		"bm_george": 9, "bm_lewis": 10,
	}
	speakers := KokoroSpeakersForModel(KokoroModelV019)
	if len(speakers) != 11 {
		t.Fatalf("expected 11 v0.19 speakers, got %d", len(speakers))
	}
	for _, s := range speakers {
		if want[s.Name] != s.SID {
			t.Errorf("speaker %q: got SID %d, want %d", s.Name, s.SID, want[s.Name])
		}
	}
	for name, sid := range want {
		if got := ResolveKokoroSpeakerID(KokoroModelV019, name); got != sid {
			t.Errorf("ResolveKokoroSpeakerID(%q) = %d, want %d", name, got, sid)
		}
	}
}

func TestKokoroUnknownVoiceAndModelFallback(t *testing.T) {
	if got := ResolveKokoroSpeakerID(KokoroModelV019, "af_alloy"); got != 0 {
		t.Errorf("unsupported voice should fall back to SID 0, got %d", got)
	}
	if got := ResolveKokoroSpeakerID("no-such-model", "af_bella"); got != 0 {
		t.Errorf("unknown model should fall back to SID 0, got %d", got)
	}
}

func TestKokoroProfilesMatchPinnedModel(t *testing.T) {
	speakers := KokoroSpeakersForModel(KokoroModelV019)
	if len(config.KokoroVoiceProfiles) != len(speakers) {
		t.Fatalf("profile count %d != speaker count %d", len(config.KokoroVoiceProfiles), len(speakers))
	}
	known := make(map[string]bool, len(speakers))
	for _, s := range speakers {
		known[s.Name] = true
	}
	for _, p := range config.KokoroVoiceProfiles {
		if !known[p.VoiceID] {
			t.Errorf("profile %q uses voice %q absent from %s", p.ID, p.VoiceID, KokoroModelV019)
		}
	}
}
