package ttsgemini_test

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
	"github.com/darkliquid/localrpg/pkg/provider/ttsgemini"
)

func TestGeminiTTSSynthesizeAppliesDirection(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"candidates": [
				{"content": {"parts": [{"inlineData": {"data": "UklGRiQAAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YQAAAAA=", "mimeType": "audio/wav"}}], "role": "model"}}
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

	ttsClient, err := ttsgemini.NewGeminiTTSClientWithClient(genaiClient, config.TTSConfig{
		Model:        "gemini-3.1-flash-tts-preview",
		DefaultVoice: "Aoede",
	})
	if err != nil {
		t.Fatalf("NewGeminiTTSClientWithClient: %v", err)
	}

	_, err = ttsClient.Synthesize(ctx, "Hold the line.", &entity.VoiceConfig{
		VoiceID: "Kore",
		Options: map[string]any{"direction": "weary and guarded, speaking slowly"},
	})
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if !strings.Contains(gotBody, "DIRECTOR'S NOTES") || !strings.Contains(gotBody, "weary and guarded") {
		t.Errorf("expected direction to be prepended to the transcript, got: %s", gotBody)
	}
	if !strings.Contains(gotBody, "Hold the line.") {
		t.Errorf("expected the spoken text to remain in the transcript, got: %s", gotBody)
	}
}

func TestGeminiTTSVoiceCatalog(t *testing.T) {
	client := ttsgemini.NewGeminiTTSClientOffline("gemini-3.1-flash-tts-preview", "Aoede")
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
	client := ttsgemini.NewGeminiTTSClientOffline("gemini-3.1-flash-tts-preview", "Aoede")
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

	ttsClient, err := ttsgemini.NewGeminiTTSClientWithClient(genaiClient, config.TTSConfig{
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

func TestGeminiTTSWrapsPCM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Gemini returns headerless L16 PCM for some TTS models.
		fmt.Fprint(w, `{
			"candidates": [
				{
					"content": {
						"parts": [
							{
								"inlineData": {
									"data": "AQIDBA==",
									"mimeType": "audio/L16;codec=pcm;rate=24000"
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

	genaiClient, err := genai.NewClient(context.Background(), &genai.ClientConfig{
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

	ttsClient, err := ttsgemini.NewGeminiTTSClientWithClient(genaiClient, config.TTSConfig{
		Model:        "gemini-2.5-flash-preview-tts",
		DefaultVoice: "Aoede",
	})
	if err != nil {
		t.Fatalf("NewGeminiTTSClientWithClient: %v", err)
	}

	audio, err := ttsClient.Synthesize(context.Background(), "Hello", nil)
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if !strings.HasPrefix(string(audio), "RIFF") {
		t.Fatalf("expected raw PCM to be wrapped in a WAV container")
	}
	if len(audio) != 44+4 {
		t.Fatalf("length = %d, want %d", len(audio), 44+4)
	}
	if ext := media.AudioExtension(audio); ext != ".wav" {
		t.Fatalf("extension = %q, want .wav", ext)
	}
}
