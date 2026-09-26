package media

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
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

func TestComputeAudioCacheKeyForVoice(t *testing.T) {
	base := &entity.VoiceConfig{VoiceID: "af_bella", Pitch: 1, SpeechRate: 1}

	t.Run("no options reproduces the legacy key", func(t *testing.T) {
		got := ComputeAudioCacheKeyForVoice("speaker", base, "hello")
		want := ComputeAudioCacheKeyWithRate("speaker", "af_bella", 1, 1, "hello")
		if got != want {
			t.Errorf("key = %q, want the legacy key %q", got, want)
		}
	})

	t.Run("a nil voice reproduces the legacy key", func(t *testing.T) {
		got := ComputeAudioCacheKeyForVoice("speaker", nil, "hello")
		want := ComputeAudioCacheKeyWithRate("speaker", "", 0, 0, "hello")
		if got != want {
			t.Errorf("key = %q, want the legacy key %q", got, want)
		}
	})

	t.Run("changing one option changes the key", func(t *testing.T) {
		low := &entity.VoiceConfig{VoiceID: "af_bella", Pitch: 1, SpeechRate: 1, Options: map[string]interface{}{"stability": 0.35}}
		high := &entity.VoiceConfig{VoiceID: "af_bella", Pitch: 1, SpeechRate: 1, Options: map[string]interface{}{"stability": 0.8}}
		if ComputeAudioCacheKeyForVoice("speaker", low, "hello") == ComputeAudioCacheKeyForVoice("speaker", high, "hello") {
			t.Errorf("expected different keys for different options")
		}
	})

	t.Run("insertion order does not change the key", func(t *testing.T) {
		first := &entity.VoiceConfig{VoiceID: "af_bella", Options: map[string]interface{}{"stability": 0.35, "style": 0.2}}
		second := &entity.VoiceConfig{VoiceID: "af_bella", Options: map[string]interface{}{"style": 0.2, "stability": 0.35}}
		if ComputeAudioCacheKeyForVoice("speaker", first, "hello") != ComputeAudioCacheKeyForVoice("speaker", second, "hello") {
			t.Errorf("expected map order not to change the key")
		}
	})

	t.Run("provider distinguishes two voices with the same id", func(t *testing.T) {
		one := &entity.VoiceConfig{Provider: "builtin:kokoro", VoiceID: "af_bella", Options: map[string]interface{}{"style": 0.2}}
		two := &entity.VoiceConfig{Provider: "builtin:elevenlabs", VoiceID: "af_bella", Options: map[string]interface{}{"style": 0.2}}
		if ComputeAudioCacheKeyForVoice("speaker", one, "hello") == ComputeAudioCacheKeyForVoice("speaker", two, "hello") {
			t.Errorf("expected the provider to separate the keys")
		}
	})
}

func TestPipelineKeyFollowsVoiceOptions(t *testing.T) {
	cache := NewContentCache(t.TempDir())
	pipeline := NewTTSPipeline(&echoTTSClient{}, cache)

	low := &entity.VoiceConfig{VoiceID: "af_bella", Options: map[string]interface{}{"stability": 0.35}}
	high := &entity.VoiceConfig{VoiceID: "af_bella", Options: map[string]interface{}{"stability": 0.8}}

	lowKey := ComputeAudioCacheKeyForVoice("speaker", low, "hello")
	highKey := ComputeAudioCacheKeyForVoice("speaker", high, "hello")

	if _, err := pipeline.SynthesizeUtterance(context.Background(), "speaker", low, "hello"); err != nil {
		t.Fatalf("SynthesizeUtterance: %v", err)
	}
	if !cache.Exists("audio", lowKey+".opus") {
		t.Errorf("expected a clip stored under the low-options key %q", lowKey)
	}
	if cache.Exists("audio", highKey+".opus") {
		t.Errorf("a second options set must not share the first clip")
	}
}
