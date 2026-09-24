package media

import (
	"errors"
	"os"
	"strings"
)

var ErrGeminiImageAPIKeyRequired = errors.New("gemini: an API key is required for image generation; set media.image.api_key, providers.gemini.api_key, or GEMINI_API_KEY")

// ResolveGeminiImageAPIKey resolves the API key prioritizing the image config override,
// then the shared providers.gemini.api_key, and finally the environment variables.
func ResolveGeminiImageAPIKey(imageKey, sharedKey string) (string, error) {
	if k := strings.TrimSpace(imageKey); k != "" {
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
	return "", ErrGeminiImageAPIKeyRequired
}
