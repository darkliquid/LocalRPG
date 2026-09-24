package media_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

// TestHTTPTTSBuildsFromTheRegistry proves a type: http TTS config builds the
// HTTP client through the registry, not the echo fallback (which has no voice
// catalogue).
func TestHTTPTTSBuildsFromTheRegistry(t *testing.T) {
	client, err := media.NewTTSClient(config.TTSConfig{Type: "http", Endpoint: "http://localhost:8880", Model: "kokoro"})
	if err != nil {
		t.Fatalf("NewTTSClient: %v", err)
	}
	if _, ok := client.(media.VoiceCatalog); !ok {
		t.Fatalf("expected the registry HTTP client, got %T", client)
	}
}

// TestDisabledTTSFactoryReturnsPlaceholder covers the fallback path.
func TestDisabledTTSFactoryReturnsPlaceholder(t *testing.T) {
	client, err := media.NewTTSClient(config.TTSConfig{Type: "disabled"})
	if err != nil || client == nil {
		t.Fatalf("expected a disabled placeholder, got %v %v", client, err)
	}
}
