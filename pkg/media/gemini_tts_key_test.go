package media_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
)

func TestResolveGeminiTTSAPIKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	// 1. None provided
	_, err := media.ResolveGeminiTTSAPIKey("", "")
	if err == nil {
		t.Errorf("expected error when no key provided")
	}

	// 2. Fallback to GOOGLE_API_KEY
	t.Setenv("GOOGLE_API_KEY", "env-google-key")
	k, err := media.ResolveGeminiTTSAPIKey("", "")
	if err != nil || k != "env-google-key" {
		t.Errorf("expected env-google-key, got %q", k)
	}

	// 3. Fallback to GEMINI_API_KEY
	t.Setenv("GEMINI_API_KEY", "env-gemini-key")
	k, err = media.ResolveGeminiTTSAPIKey("", "")
	if err != nil || k != "env-gemini-key" {
		t.Errorf("expected env-gemini-key, got %q", k)
	}

	// 4. Shared provider key
	k, err = media.ResolveGeminiTTSAPIKey("", "shared-key")
	if err != nil || k != "shared-key" {
		t.Errorf("expected shared-key, got %q", k)
	}

	// 5. Config TTS key override
	k, err = media.ResolveGeminiTTSAPIKey("override-key", "shared-key")
	if err != nil || k != "override-key" {
		t.Errorf("expected override-key, got %q", k)
	}
}
