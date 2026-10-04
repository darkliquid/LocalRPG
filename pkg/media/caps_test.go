package media

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestLiveGroupCapsClampToSingleSpeaker(t *testing.T) {
	got := LiveGroupCaps(TTSCapabilities{MaxSpeakers: 3, MaxCharsPerRequest: 500, MaxTokensPerRequest: 200})
	if got.MaxSpeakers != 1 {
		t.Fatalf("MaxSpeakers = %d, want 1", got.MaxSpeakers)
	}
	if got.MaxCharsPerRequest != 500 || got.MaxTokensPerRequest != 200 {
		t.Fatalf("live caps dropped a request limit: %+v", got)
	}
}

func boolPtr(v bool) *bool { return &v }

// A turn the streamer folds live is single-speaker, so its clip plan must clamp
// the provider's two-speaker caps to one.
func TestTurnGroupCapsClampWhenLiveGrouping(t *testing.T) {
	cfg := &config.Config{}
	cfg.Media.TTS.Type = "builtin"
	cfg.Media.TTS.Grouping = "auto"
	cfg.Media.TTS.StreamSentences = boolPtr(true)

	got := TurnGroupCaps(cfg, TTSCapabilities{MaxSpeakers: 2, MaxCharsPerRequest: 500})
	if got.MaxSpeakers != 1 {
		t.Fatalf("MaxSpeakers = %d, want 1 for a live-grouped turn", got.MaxSpeakers)
	}
	if got.MaxCharsPerRequest != 500 {
		t.Fatalf("live caps dropped the request limit: %+v", got)
	}
}

// Without live grouping the provider's own caps apply, so a batch backfill and a
// finalised turn still agree on multi-speaker groups.
func TestTurnGroupCapsKeepProviderCapsWithoutLiveGrouping(t *testing.T) {
	cases := map[string]*config.Config{
		"streaming off": func() *config.Config {
			cfg := &config.Config{}
			cfg.Media.TTS.Type = "builtin"
			cfg.Media.TTS.Grouping = "auto"
			cfg.Media.TTS.StreamSentences = boolPtr(false)
			return cfg
		}(),
		"grouping always": func() *config.Config {
			cfg := &config.Config{}
			cfg.Media.TTS.Type = "builtin"
			cfg.Media.TTS.Grouping = "always"
			cfg.Media.TTS.StreamSentences = boolPtr(true)
			return cfg
		}(),
		"no provider": func() *config.Config {
			cfg := &config.Config{}
			cfg.Media.TTS.Grouping = "auto"
			cfg.Media.TTS.StreamSentences = boolPtr(true)
			return cfg
		}(),
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if got := TurnGroupCaps(cfg, TTSCapabilities{MaxSpeakers: 2}); got.MaxSpeakers != 2 {
				t.Errorf("MaxSpeakers = %d, want the provider's 2", got.MaxSpeakers)
			}
		})
	}
}

