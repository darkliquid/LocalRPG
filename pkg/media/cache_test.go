package media

import (
	"testing"
)

func TestStateAwareAudioCacheKey(t *testing.T) {
	speakerID := "lady-evelyn"
	voiceConfigA := "kokoro:bf_emma:1.0"
	voiceConfigB := "kokoro:bf_emma:0.8:raspy" // voice damaged/altered
	text := "Thank you, traveler."

	keyA1 := ComputeAudioCacheKey(speakerID, voiceConfigA, text)
	keyA2 := ComputeAudioCacheKey(speakerID, voiceConfigA, text)
	keyB := ComputeAudioCacheKey(speakerID, voiceConfigB, text)

	if keyA1 != keyA2 {
		t.Errorf("expected deterministic cache key for identical parameters")
	}
	if keyA1 == keyB {
		t.Errorf("expected altered voice config to produce different cache key")
	}
}

func TestStateAwareArtCacheKey(t *testing.T) {
	entityID := "alden-tavern"
	worldStyle := "oil painting, dark fantasy"
	appearanceA := "Cozy wooden tavern with glowing hearth"
	appearanceB := "Burned-out ruins of wooden tavern" // altered after fire

	keyA := ComputeArtCacheKey(entityID, appearanceA, worldStyle)
	keyB := ComputeArtCacheKey(entityID, appearanceB, worldStyle)

	if keyA == keyB {
		t.Errorf("expected altered appearance to produce different cache key")
	}
}

func TestContentCacheFileStorage(t *testing.T) {
	tempDir := t.TempDir()
	cache := NewContentCache(tempDir)

	data := []byte("audio payload bytes")
	path, err := cache.Put("audio", "sample-key.ogg", data)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if !cache.Exists("audio", "sample-key.ogg") {
		t.Errorf("expected file to exist at %s", path)
	}

	loaded, err := cache.Get("audio", "sample-key.ogg")
	if err != nil || string(loaded) != string(data) {
		t.Errorf("cache read mismatch: got %v, err=%v", string(loaded), err)
	}
}
