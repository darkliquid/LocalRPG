package ttsgemini_test

import (
	"context"
	"encoding/json"
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
		Options: map[string]interface{}{"direction": "weary and guarded, speaking slowly"},
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

func TestGeminiTTSSynthesizeGroupBuildsTwoSpeakerConfig(t *testing.T) {
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
		Model:        "gemini-3.8-flash-tts",
		DefaultVoice: "Aoede",
	})
	if err != nil {
		t.Fatalf("NewGeminiTTSClientWithClient: %v", err)
	}

	_, err = ttsClient.SynthesizeGroup(ctx, []media.SpeakerLine{
		{SpeakerID: "narrator", Label: "Narrator", Voice: &entity.VoiceConfig{VoiceID: "Aoede"}, Text: "The door opens."},
		{SpeakerID: "garrick", Label: "Garrick", Voice: &entity.VoiceConfig{VoiceID: "Kore"}, Text: "Keep walking."},
	})
	if err != nil {
		t.Fatalf("SynthesizeGroup failed: %v", err)
	}
	for _, want := range []string{"multiSpeakerVoiceConfig", "Narrator", "Garrick", "Aoede", "Kore", "The door opens.", "Keep walking."} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("expected the request to contain %q, got: %s", want, gotBody)
		}
	}
}

// The 3.8 TTS models read the transcript verbatim, so a speaker cannot be named
// inside the text: every text part of a multi-speaker request must carry
// speech_metadata.speaker or the API rejects the call with a 400. This server
// stands in for that validation so the request shape is checked offline.
func TestGeminiTTSSynthesizeGroupAnnotatesEachPartWithSpeaker(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)

		var req struct {
			Contents []struct {
				Parts []struct {
					Text           string `json:"text"`
					SpeechMetadata *struct {
						Speaker string `json:"speaker"`
					} `json:"speechMetadata"`
				} `json:"parts"`
			} `json:"contents"`
			GenerationConfig struct {
				SpeechConfig *struct {
					MultiSpeakerVoiceConfig *struct{} `json:"multiSpeakerVoiceConfig"`
				} `json:"speechConfig"`
			} `json:"generationConfig"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		multi := req.GenerationConfig.SpeechConfig != nil && req.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig != nil
		if multi {
			for _, content := range req.Contents {
				for _, part := range content.Parts {
					if part.SpeechMetadata == nil || part.SpeechMetadata.Speaker == "" {
						w.WriteHeader(http.StatusBadRequest)
						fmt.Fprint(w, `{"error":{"code":400,"message":"Multi-speaker generation requests must specify speech_metadata.speaker for each text part in the contents.","status":"INVALID_ARGUMENT"}}`)
						return
					}
				}
			}
		}
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
		Model:        "gemini-3.8-flash-tts",
		DefaultVoice: "Aoede",
	})
	if err != nil {
		t.Fatalf("NewGeminiTTSClientWithClient: %v", err)
	}

	_, err = ttsClient.SynthesizeGroup(ctx, []media.SpeakerLine{
		{SpeakerID: "narrator", Label: "Narrator", Voice: &entity.VoiceConfig{VoiceID: "Aoede"}, Text: "The door opens."},
		{SpeakerID: "garrick", Label: "Garrick", Voice: &entity.VoiceConfig{VoiceID: "Kore"}, Text: "Keep walking."},
	})
	if err != nil {
		t.Fatalf("SynthesizeGroup failed: %v", err)
	}
	if strings.Contains(gotBody, "Narrator: ") || strings.Contains(gotBody, "Garrick: ") {
		t.Errorf("speaker labels must not be embedded in the transcript, got: %s", gotBody)
	}
}

func TestGeminiTTSSynthesizeGroupRejectsOneSpeaker(t *testing.T) {
	client := ttsgemini.NewGeminiTTSClientOffline("gemini-3.8-flash-tts", "Aoede")
	_, err := client.SynthesizeGroup(context.Background(), []media.SpeakerLine{
		{SpeakerID: "narrator", Label: "Narrator", Text: "Alone."},
	})
	if err == nil {
		t.Fatalf("expected an error for a single-speaker group")
	}
}

func TestGeminiTTSCapabilitiesDeclareTwoSpeakers(t *testing.T) {
	client := ttsgemini.NewGeminiTTSClientOffline("gemini-3.8-flash-tts", "Aoede")
	caps := client.TTSCapabilities()
	if caps.MaxSpeakers != 2 {
		t.Errorf("MaxSpeakers = %d, want 2", caps.MaxSpeakers)
	}
	if !caps.SupportsGrouping || !caps.SupportsBatch || !caps.SupportsStreaming {
		t.Errorf("unexpected capabilities %#v", caps)
	}
}
