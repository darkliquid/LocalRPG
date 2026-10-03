// Package inworldshared holds what every Inworld provider needs: the credential
// precedence and the HTTP status mapping, so a key is resolved and a failure is
// explained the same way for the LLM router, TTS, and STT.
package inworldshared

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// ErrMissingAPIKey is returned when no credential resolves.
var ErrMissingAPIKey = errors.New("inworld: an API key is required; set providers.inworld.api_key, agents.roles.<role>.api_key (or media.tts/stt.api_key), or INWORLD_API_KEY")

// ResolveAPIKey applies the credential precedence: an explicit role or media
// key, then the shared providers.inworld.api_key, then INWORLD_API_KEY. The
// resolved key is registered for redaction.
func ResolveAPIKey(explicit, shared string) (string, error) {
	for _, candidate := range []string{explicit, shared, os.Getenv("INWORLD_API_KEY")} {
		if key := strings.TrimSpace(candidate); key != "" {
			trace.RegisterSecret(key)
			return key, nil
		}
	}
	return "", ErrMissingAPIKey
}

// MapError turns an Inworld HTTP status into an actionable error.
func MapError(status int, body []byte) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("inworld: invalid API key or permission denied; check providers.inworld.api_key or INWORLD_API_KEY")
	case http.StatusTooManyRequests:
		return errors.New("inworld: quota exceeded or rate limit reached; check your Inworld account credits")
	default:
		if detail := provider.TruncateDetail(body); detail != "" {
			return fmt.Errorf("inworld: %s (status %d)", detail, status)
		}
		return fmt.Errorf("inworld: request failed with status %d", status)
	}
}
