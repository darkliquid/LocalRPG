package media

import "github.com/darkliquid/localrpg/pkg/config"

// KokoroVoiceProfiles are the eleven Kokoro voices as voice archetypes. They are
// exported so a preset can carry them: applying the Sherpa-ONNX or Kokoro-FastAPI
// preset seeds the voice-profile library, exactly as the settings UI used to.
func KokoroVoiceProfiles() []config.VoiceProfile {
	return []config.VoiceProfile{
		{ID: "af", Name: "Default (American Female)", VoiceID: "af", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "default", "neutral"}, Description: "The model's stock American female voice."},
		{ID: "af_bella", Name: "Bella (American Female)", VoiceID: "af_bella", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "bella", "warm", "friendly"}, Description: "American female voice, warm, approachable, and pleasant."},
		{ID: "af_nicole", Name: "Nicole (American Female)", VoiceID: "af_nicole", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "nicole", "youthful", "energetic"}, Description: "American female voice, brisk, youthful, and direct."},
		{ID: "af_sarah", Name: "Sarah (American Female)", VoiceID: "af_sarah", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "sarah", "poised", "narrative"}, Description: "American female voice, polished, measured, and story-oriented."},
		{ID: "af_sky", Name: "Sky (American Female)", VoiceID: "af_sky", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "sky", "light", "airy"}, Description: "American female voice, light, gentle, and breathy."},
		{ID: "am_adam", Name: "Adam (American Male)", VoiceID: "am_adam", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "adam", "deep", "authoritative"}, Description: "American male voice, deep, steady, and commanding."},
		{ID: "am_michael", Name: "Michael (American Male)", VoiceID: "am_michael", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "michael", "commanding", "formal"}, Description: "American male voice, disciplined, authoritative, and formal."},
		{ID: "bf_emma", Name: "Emma (British Female)", VoiceID: "bf_emma", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "emma", "gentle", "poised"}, Description: "British female voice, elegant, gentle, and softly spoken."},
		{ID: "bf_isabella", Name: "Isabella (British Female)", VoiceID: "bf_isabella", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "isabella", "noble", "melodic"}, Description: "British female voice, aristocratic, melodic, and graceful."},
		{ID: "bm_george", Name: "George (British Male)", VoiceID: "bm_george", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "male", "george", "mature", "distinguished"}, Description: "British male voice, mature, distinguished, and resonant."},
		{ID: "bm_lewis", Name: "Lewis (British Male)", VoiceID: "bm_lewis", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "male", "lewis", "thoughtful", "refined"}, Description: "British male voice, measured, polite, and reflective."},
	}
}
