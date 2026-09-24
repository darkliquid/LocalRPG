package media

import (
	"errors"
	"os"
	"strings"
)

// ErrGeminiTTSAPIKeyRequired reports that no Gemini API key was found in config or environment.
var ErrGeminiTTSAPIKeyRequired = errors.New("gemini: an API key is required for speech synthesis; set media.tts.api_key, providers.gemini.api_key, or GEMINI_API_KEY")

// ResolveGeminiTTSAPIKey resolves the API key prioritizing the TTS config override,
// then the shared providers.gemini.api_key, and finally the environment variables.
func ResolveGeminiTTSAPIKey(ttsKey, sharedKey string) (string, error) {
	if k := strings.TrimSpace(ttsKey); k != "" {
		return k, nil
	}
	if k := strings.TrimSpace(sharedKey); k != "" {
		return k, nil
	}
	if k := strings.TrimSpace(os.Getenv("GEMINI_API_KEY")); k != "" {
		return k, nil
	}
	if k := strings.TrimSpace(os.Getenv("GOOGLE_API_KEY")); k != "" {
		return k, nil
	}
	return "", ErrGeminiTTSAPIKeyRequired
}
