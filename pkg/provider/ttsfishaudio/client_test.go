package ttsfishaudio_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/provider/ttsfishaudio"
)

func TestFishAudioSynthesis_Standard(t *testing.T) {
	expectedAudio := []byte("RIFFmockwavdata")
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &receivedBody)

		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expectedAudio)
	}))
	defer server.Close()

	client := ttsfishaudio.NewFishAudioTTSClient(config.TTSConfig{
		Endpoint: server.URL,
		Model:    "fishaudio/s2-pro",
		APIKey:   "test-key",
	})

	audio, err := client.Synthesize(context.Background(), "[excited] Greetings traveler!", &entity.VoiceConfig{
		VoiceID:    "speaker-1",
		SpeechRate: 1.2,
	})
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}

	if string(audio) != string(expectedAudio) {
		t.Errorf("expected audio %q, got %q", expectedAudio, audio)
	}

	if receivedBody["input"] != "[excited] Greetings traveler!" {
		t.Errorf("expected input %q, got %q", "[excited] Greetings traveler!", receivedBody["input"])
	}
	if receivedBody["voice"] != "speaker-1" {
		t.Errorf("expected voice %q, got %q", "speaker-1", receivedBody["voice"])
	}
	if receivedBody["speed"] != 1.2 {
		t.Errorf("expected speed 1.2, got %v", receivedBody["speed"])
	}
}

func TestFishAudioSynthesis_VoiceCloning_LocalFile(t *testing.T) {
	tmpDir := t.TempDir()
	sampleWavPath := filepath.Join(tmpDir, "sample.wav")
	sampleBytes := []byte("RIFFfakeaudio")
	if err := os.WriteFile(sampleWavPath, sampleBytes, 0644); err != nil {
		t.Fatalf("failed to write sample wav: %v", err)
	}

	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &receivedBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	client := ttsfishaudio.NewFishAudioTTSClient(config.TTSConfig{
		Endpoint: server.URL,
		Model:    "fishaudio/s2-pro",
	})

	_, err := client.Synthesize(context.Background(), "Cloned dialogue", &entity.VoiceConfig{
		VoiceID: "default",
		Options: map[string]interface{}{
			"ref_audio": sampleWavPath,
			"ref_text":  "This is the reference transcript.",
		},
	})
	if err != nil {
		t.Fatalf("Synthesize with cloning failed: %v", err)
	}

	refAudio, ok := receivedBody["ref_audio"].(string)
	if !ok || !strings.HasPrefix(refAudio, "data:audio/wav;base64,") {
		t.Fatalf("expected ref_audio data URL, got %v", receivedBody["ref_audio"])
	}
	b64Part := strings.TrimPrefix(refAudio, "data:audio/wav;base64,")
	decoded, err := base64.StdEncoding.DecodeString(b64Part)
	if err != nil || string(decoded) != string(sampleBytes) {
		t.Errorf("decoded ref_audio mismatch: got %q, want %q", decoded, sampleBytes)
	}

	if receivedBody["ref_text"] != "This is the reference transcript." {
		t.Errorf("expected ref_text %q, got %q", "This is the reference transcript.", receivedBody["ref_text"])
	}
}

func TestFishAudioSynthesis_VoiceCloning_DirectURL(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &receivedBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	client := ttsfishaudio.NewFishAudioTTSClient(config.TTSConfig{
		Endpoint: server.URL,
		Model:    "fishaudio/s2-pro",
	})

	directURL := "https://example.com/voice.wav"
	_, err := client.Synthesize(context.Background(), "Cloned dialogue", &entity.VoiceConfig{
		VoiceID: "default",
		Options: map[string]interface{}{
			"ref_audio": directURL,
			"ref_text":  "Transcript.",
		},
	})
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}

	if receivedBody["ref_audio"] != directURL {
		t.Errorf("expected direct URL passthrough %q, got %v", directURL, receivedBody["ref_audio"])
	}
}

func TestFishAudioSpeechCueCapabilities(t *testing.T) {
	client := ttsfishaudio.NewFishAudioTTSClient(config.TTSConfig{})
	caps := client.SpeechCueCapabilities()

	if !caps.AudioTags {
		t.Errorf("expected AudioTags to be true")
	}
	if len(caps.SupportedTags) == 0 {
		t.Errorf("expected non-empty SupportedTags")
	}
}

func TestFishAudioVoiceCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/audio/voices" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"voices": ["voice-alpha", "voice-beta"]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := ttsfishaudio.NewFishAudioTTSClient(config.TTSConfig{Endpoint: server.URL})
	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices failed: %v", err)
	}
	if len(voices) != 2 {
		t.Fatalf("expected 2 voices, got %d", len(voices))
	}
	if voices[0].ID != "voice-alpha" {
		t.Errorf("expected first voice ID voice-alpha, got %s", voices[0].ID)
	}
}

func TestFishAudioVoiceCatalogFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := ttsfishaudio.NewFishAudioTTSClient(config.TTSConfig{Endpoint: server.URL})
	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices fallback failed: %v", err)
	}
	if len(voices) != 1 || voices[0].ID != "default" {
		t.Errorf("expected single default fallback voice, got %+v", voices)
	}
}

func TestFishAudioVoiceOptions(t *testing.T) {
	client := ttsfishaudio.NewFishAudioTTSClient(config.TTSConfig{})
	options := client.VoiceOptions()
	if len(options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(options))
	}
	if options[0].Key != "ref_audio" || options[1].Key != "ref_text" {
		t.Errorf("unexpected options: %+v", options)
	}
}
