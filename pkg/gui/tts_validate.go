package gui

import (
	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

// validateVoiceOptionsInConfig clamps and drops provider options against the
// active provider's own declaration. A provider that declares nothing cannot
// validate, so its values are left untouched rather than discarded.
func (s *Service) validateVoiceOptionsInConfig(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	tts := cfg.Media.TTS
	client, err := s.ttsClientFor(tts)
	if err != nil {
		// The provider cannot be built, so there is no schema to validate against;
		// the save should still succeed and let the provider report its own error.
		return nil
	}
	schema, ok := client.(media.VoiceOptions)
	if !ok {
		return nil
	}
	declared := schema.VoiceOptions()

	if len(tts.Options) > 0 {
		canonical, _ := media.ValidateVoiceOptions(declared, tts.Options)
		cfg.Media.TTS.Options = canonical
	}
	for i := range cfg.Media.TTS.VoiceProfiles {
		profile := &cfg.Media.TTS.VoiceProfiles[i]
		if len(profile.Options) == 0 {
			continue
		}
		canonical, _ := media.ValidateVoiceOptions(declared, profile.Options)
		profile.Options = canonical
	}
	return nil
}
