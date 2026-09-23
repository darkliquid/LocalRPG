package media_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
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

func TestGeminiTTSVoiceCatalog(t *testing.T) {
	client := media.NewGeminiTTSClientOffline("gemini-3.1-flash-tts-preview", "Aoede")
	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(voices) != 30 {
		t.Fatalf("expected 30 voices, got %d", len(voices))
	}

	for _, v := range voices {
		if v.ID == "" || v.Name == "" {
			t.Errorf("expected non-empty voice ID and Name: %+v", v)
		}
		if len(v.Tags) == 0 {
			t.Errorf("expected non-empty tags for voice %s", v.ID)
		}
	}
}

func TestGeminiTTSSpeechCueCapabilities(t *testing.T) {
	client := media.NewGeminiTTSClientOffline("gemini-3.1-flash-tts-preview", "Aoede")
	caps := client.SpeechCueCapabilities()
	if !caps.AudioTags {
		t.Errorf("expected AudioTags to be true")
	}
	if caps.MarkdownEmphasis {
		t.Errorf("expected MarkdownEmphasis to be false")
	}
	if len(caps.SupportedTags) == 0 {
		t.Errorf("expected non-empty SupportedTags")
	}
	if !strings.Contains(caps.PromptGuidance, "[tag]") {
		t.Errorf("expected PromptGuidance to describe tag usage")
	}
}

func TestGeminiTTSSynthesize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)

		if !strings.Contains(bodyStr, "Hello world") {
			t.Errorf("expected text in request body, got: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "Kore") {
			t.Errorf("expected voice Kore in request body, got: %s", bodyStr)
		}

		w.Header().Set("Content-Type", "application/json")
		// Return dummy base64 RIFF wav audio bytes: "RIFF....WAVE"
		fmt.Fprint(w, `{
			"candidates": [
				{
					"content": {
						"parts": [
							{
								"inlineData": {
									"data": "UklGRiQAAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YQAAAAA=",
									"mimeType": "audio/wav"
								}
							}
						],
						"role": "model"
					}
				}
			]
		}`)
	}))
	defer server.Close()

	ctx := context.Background()
	genaiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     "test-key",
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{
			BaseURL: server.URL,
		},
	})
	if err != nil {
		t.Fatalf("create genai client: %v", err)
	}

	ttsClient, err := media.NewGeminiTTSClientWithClient(genaiClient, config.TTSConfig{
		Model:        "gemini-3.1-flash-tts-preview",
		DefaultVoice: "Aoede",
	})
	if err != nil {
		t.Fatalf("NewGeminiTTSClientWithClient: %v", err)
	}

	audioBytes, err := ttsClient.Synthesize(ctx, "Hello world", &entity.VoiceConfig{VoiceID: "Kore"})
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if ext := media.AudioExtension(audioBytes); ext != ".wav" {
		t.Errorf("expected .wav audio extension, got %s", ext)
	}
}
