package harness

import (
	"errors"
	"os"
	"strings"
)

// ErrGeminiAPIKeyRequired reports that no Gemini API key was found in config or
// the environment.
var ErrGeminiAPIKeyRequired = errors.New("gemini: an API key is required; set providers.gemini.api_key, agents.roles.<role>.api_key, or GEMINI_API_KEY")

// ResolveGeminiAPIKey resolves the key from a role override, the shared provider
// key, then the environment.
func ResolveGeminiAPIKey(roleKey, sharedKey string) (string, error) {
	if k := strings.TrimSpace(roleKey); k != "" {
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
	return "", ErrGeminiAPIKeyRequired
}
