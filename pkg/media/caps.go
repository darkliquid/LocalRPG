package media

import "github.com/darkliquid/localrpg/pkg/config"

// LiveGroupCaps clamps a capability set to what live audio can use: one speaker
// per request, because a multi-speaker request delays the first speaker's audio
// until the second speaker's text exists. The request limits are kept, so a
// single speaker's run is still split when it is too large for one request.
//
// Both the streaming fold and the turn's clip plan use it, so their groups agree
// and the streamed clips are the clips the turn records.
func LiveGroupCaps(caps TTSCapabilities) TTSCapabilities {
	caps = normalizeCaps(caps)
	caps.MaxSpeakers = 1
	return caps
}

// StreamerRuns reports whether the live sentence pre-synthesiser runs for a
// turn: sentence streaming is on, grouping is not pinned to the whole turn, and
// a provider is configured. "always" renders the whole turn in one batch and
// streams nothing.
func StreamerRuns(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	if !cfg.TTSStreamSentences() || cfg.TTSGrouping() == "always" {
		return false
	}
	return cfg.Media.TTS.Type != "" && cfg.Media.TTS.Type != "disabled"
}

// LiveGrouping reports whether the streamer folds a turn's audio into groups,
// so the turn's clip plan must fold under the same single-speaker caps.
func LiveGrouping(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return StreamerRuns(cfg) && cfg.TTSGrouping() != "off"
}

// TurnGroupCaps resolves the capability set a turn's groups are planned under:
// single-speaker when the streamer folds the turn live, otherwise the provider's
// own caps. Every path that names a turn's clips (the interactive play, a single
// segment's play, the uncached count, the offline batch backfill and export)
// resolves them through this one function, so the batch writes the clips the app
// looks up.
func TurnGroupCaps(cfg *config.Config, providerCaps TTSCapabilities) TTSCapabilities {
	if LiveGrouping(cfg) {
		return LiveGroupCaps(providerCaps)
	}
	return normalizeCaps(providerCaps)
}
