package inworldtts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestInworldTTSSynthesizeAndAuth(t *testing.T) {
	var gotAuth string
	var gotBody map[string]interface{}
	fakeAudio := []byte("ID3fake-mp3-audio-bytes")
	encoded := base64.StdEncoding.EncodeToString(fakeAudio)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"audioContent": encoded})
	}))
	defer server.Close()

	client, err := NewInworldTTSClient(config.TTSConfig{Endpoint: server.URL, APIKey: "custom-key"}, "shared-key")
	if err != nil {
		t.Fatalf("NewInworldTTSClient: %v", err)
	}

	audio, err := client.Synthesize(context.Background(), "Hello world", &entity.VoiceConfig{VoiceID: "Ashley"})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(audio) != string(fakeAudio) {
		t.Errorf("audio = %q, want the decoded MP3", string(audio))
	}
	if gotAuth != "Basic custom-key" {
		t.Errorf("Authorization = %q, want Basic custom-key", gotAuth)
	}
	if gotBody["voice_id"] != "Ashley" {
		t.Errorf("voice_id = %v, want Ashley", gotBody["voice_id"])
	}
	if gotBody["model_id"] != "inworld-tts-2" {
		t.Errorf("model_id = %v, want inworld-tts-2", gotBody["model_id"])
	}
	if client.LastUsage().Characters != 11 {
		t.Errorf("LastUsage().Characters = %d, want 11", client.LastUsage().Characters)
	}
	if !client.Metered() {
		t.Errorf("expected the Inworld client to be metered")
	}
}

func TestInworldTTSUsesSharedKeyAndDefaultVoice(t *testing.T) {
	var gotAuth string
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"audioContent": base64.StdEncoding.EncodeToString([]byte("audio")),
		})
	}))
	defer server.Close()

	client, err := NewInworldTTSClient(config.TTSConfig{Endpoint: server.URL}, "shared-key")
	if err != nil {
		t.Fatalf("NewInworldTTSClient: %v", err)
	}
	if _, err := client.Synthesize(context.Background(), "Hi", nil); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if gotAuth != "Basic shared-key" {
		t.Errorf("Authorization = %q, want Basic shared-key", gotAuth)
	}
	if gotBody["voice_id"] != "Ashley" {
		t.Errorf("voice_id = %v, want the default Ashley", gotBody["voice_id"])
	}
}

func TestInworldTTSListVoicesOfflineFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewInworldTTSClient(config.TTSConfig{Endpoint: server.URL, APIKey: "key"}, "")
	if err != nil {
		t.Fatalf("NewInworldTTSClient: %v", err)
	}

	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices: %v", err)
	}
	if len(voices) == 0 {
		t.Fatalf("expected fallback voices, got none")
	}
	var found bool
	for _, v := range voices {
		if v.ID == "Ashley" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Ashley in the fallback voices, got %#v", voices)
	}
}

func TestInworldTTSListVoicesReadsTheCatalogue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/voices") {
			t.Errorf("voices path = %q, want a /voices suffix", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"voices": []map[string]interface{}{
				{"voice_id": "Dennis", "name": "Dennis", "gender": "male"},
			},
		})
	}))
	defer server.Close()

	client, err := NewInworldTTSClient(config.TTSConfig{Endpoint: server.URL, APIKey: "key"}, "")
	if err != nil {
		t.Fatalf("NewInworldTTSClient: %v", err)
	}
	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices: %v", err)
	}
	if len(voices) != 1 || voices[0].ID != "Dennis" {
		t.Errorf("voices = %#v, want the catalogue entry", voices)
	}
}

func TestInworldTTSMissingKey(t *testing.T) {
	os.Unsetenv("INWORLD_API_KEY")
	_, err := NewInworldTTSClient(config.TTSConfig{}, "")
	if err == nil || !strings.Contains(err.Error(), "inworld: an API key is required") {
		t.Fatalf("expected missing key error, got: %v", err)
	}
}
