package media_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestMediaProviders_Disabled(t *testing.T) {
	ttsClient, err := media.NewTTSClient(config.TTSConfig{Type: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = ttsClient.Synthesize(context.Background(), "Hello", nil)
	if !errors.Is(err, media.ErrProviderDisabled) {
		t.Errorf("expected media.ErrProviderDisabled, got %v", err)
	}

	sttClient, err := media.NewSTTClient(config.STTConfig{Type: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = sttClient.Transcribe(context.Background(), []byte("audio"))
	if !errors.Is(err, media.ErrProviderDisabled) {
		t.Errorf("expected media.ErrProviderDisabled, got %v", err)
	}

	imgClient, err := media.NewImageClient(config.ImageConfig{Type: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = imgClient.GenerateImage(context.Background(), "A dark tower")
	if !errors.Is(err, media.ErrProviderDisabled) {
		t.Errorf("expected media.ErrProviderDisabled, got %v", err)
	}
}

func TestMediaProviders_BuiltinEcho(t *testing.T) {
	ttsClient, err := media.NewTTSClient(config.TTSConfig{Type: "builtin", BuiltinName: "echo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bytes, err := ttsClient.Synthesize(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("synthesize failed: %v", err)
	}
	if len(bytes) == 0 {
		t.Errorf("expected non-empty audio bytes from builtin echo")
	}

	sttClient, err := media.NewSTTClient(config.STTConfig{Type: "builtin", BuiltinName: "echo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text, err := sttClient.Transcribe(context.Background(), []byte("test audio"))
	if err != nil {
		t.Fatalf("transcribe failed: %v", err)
	}
	if text == "" {
		t.Errorf("expected non-empty transcript from builtin echo")
	}

	imgClient, err := media.NewImageClient(config.ImageConfig{Type: "builtin", BuiltinName: "echo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	imgBytes, err := imgClient.GenerateImage(context.Background(), "a dragon")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if len(imgBytes) == 0 {
		t.Errorf("expected non-empty image bytes from builtin echo")
	}
}

func TestSceneImageClientAlwaysProducesArtByDefault(t *testing.T) {
	// No provider configured, fallback on: the built-in generator stands in.
	client, err := media.NewSceneImageClient(config.ImageConfig{Type: "disabled", BuiltinFallback: true})
	if err != nil {
		t.Fatalf("NewSceneImageClient failed: %v", err)
	}

	data, err := client.GenerateImage(context.Background(), "moonlit harbour")
	if err != nil {
		t.Fatalf("expected the built-in generator to stand in: %v", err)
	}
	if !bytes.Contains(bytes.ToLower(data), []byte("<svg")) {
		t.Errorf("expected SVG art from the built-in generator")
	}
}

func TestSceneImageClientRespectsAFailedProvider(t *testing.T) {
	client, err := media.NewSceneImageClient(config.ImageConfig{
		Type:            "http",
		Endpoint:        "http://127.0.0.1:1/unreachable",
		BuiltinFallback: true,
	})
	if err != nil {
		t.Fatalf("NewSceneImageClient failed: %v", err)
	}

	data, err := client.GenerateImage(context.Background(), "moonlit harbour")
	if err != nil {
		t.Fatalf("expected the built-in generator to cover a provider failure: %v", err)
	}
	if !bytes.Contains(bytes.ToLower(data), []byte("<svg")) {
		t.Errorf("expected fallback art")
	}
}

func TestSceneImageClientWithoutFallbackStaysDisabled(t *testing.T) {
	client, err := media.NewSceneImageClient(config.ImageConfig{Type: "disabled", BuiltinFallback: false})
	if err != nil {
		t.Fatalf("NewSceneImageClient failed: %v", err)
	}

	if _, err := client.GenerateImage(context.Background(), "moonlit harbour"); !errors.Is(err, media.ErrProviderDisabled) {
		t.Errorf("expected media.ErrProviderDisabled, got %v", err)
	}
}

func TestHTTPSTTClient_TranscribesMultipartAudio(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Authorization Bearer test-key, got %s", auth)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected file field: %v", err)
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		if string(content) != "audio-sample-bytes" {
			t.Errorf("unexpected file content: %s", string(content))
		}
		if model := r.FormValue("model"); model != "whisper-1" {
			t.Errorf("expected model whisper-1, got %s", model)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "I enter the dark dungeon."})
	}))
	defer ts.Close()

	cfg := config.STTConfig{
		Type:     "http",
		Endpoint: ts.URL,
		Model:    "whisper-1",
		APIKey:   "test-key",
	}
	client, err := media.NewSTTClient(cfg)
	if err != nil {
		t.Fatalf("NewSTTClient failed: %v", err)
	}

	result, err := client.Transcribe(context.Background(), []byte("audio-sample-bytes"))
	if err != nil {
		t.Fatalf("Transcribe failed: %v", err)
	}
	if result != "I enter the dark dungeon." {
		t.Errorf("unexpected transcription: %s", result)
	}
}

func TestHTTPImageClient_DecodesA1111Base64(t *testing.T) {
	rawPng := []byte("\x89PNG\r\n\x1a\nfake-png-content")
	encoded := base64.StdEncoding.EncodeToString(rawPng)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["prompt"] != "a misty graveyard" {
			t.Errorf("unexpected prompt: %v", req["prompt"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"images": []string{encoded},
		})
	}))
	defer ts.Close()

	client, err := media.NewImageClient(config.ImageConfig{
		Type:     "http",
		Endpoint: ts.URL + "/sdapi/v1/txt2img",
	})
	if err != nil {
		t.Fatalf("NewImageClient failed: %v", err)
	}

	data, err := client.GenerateImage(context.Background(), "a misty graveyard")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if !bytes.Equal(data, rawPng) {
		t.Errorf("expected decoded png bytes, got %v", data)
	}
}

func TestHTTPImageClient_DecodesOpenAIBase64(t *testing.T) {
	rawPng := []byte("\x89PNG\r\n\x1a\nopenai-image-bytes")
	encoded := base64.StdEncoding.EncodeToString(rawPng)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{
				{"b64_json": encoded},
			},
		})
	}))
	defer ts.Close()

	client, err := media.NewImageClient(config.ImageConfig{
		Type:     "http",
		Endpoint: ts.URL + "/v1/images/generations",
	})
	if err != nil {
		t.Fatalf("NewImageClient failed: %v", err)
	}

	data, err := client.GenerateImage(context.Background(), "a glowing orb")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if !bytes.Equal(data, rawPng) {
		t.Errorf("expected decoded png bytes, got %v", data)
	}
}

func TestComfyUIImageClient_GeneratesImage(t *testing.T) {
	rawPng := []byte("\x89PNG\r\n\x1a\ncomfy-image-bytes")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prompt":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"prompt_id": "prompt-123"})
		case "/history/prompt-123":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"prompt-123": map[string]interface{}{
					"outputs": map[string]interface{}{
						"9": map[string]interface{}{
							"images": []map[string]string{
								{"filename": "out_001.png", "subfolder": "", "type": "output"},
							},
						},
					},
				},
			})
		case "/view":
			if r.URL.Query().Get("filename") != "out_001.png" {
				t.Errorf("unexpected filename query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(rawPng)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client, err := media.NewImageClient(config.ImageConfig{
		Type:     "http",
		Endpoint: ts.URL + ":8188", // ComfyUI endpoint marker
	})
	if err != nil {
		t.Fatalf("NewImageClient failed: %v", err)
	}

	data, err := client.GenerateImage(context.Background(), "ancient citadel")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if !bytes.Equal(data, rawPng) {
		t.Errorf("expected comfyui image bytes, got %v", data)
	}
}

func TestHTTPTTSClient_AdaptsAllTalkPayload(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["text_input"] != "Greetings, traveler." {
			t.Errorf("expected text_input, got %v", req["text_input"])
		}
		if req["character_voice_gen"] != "elder_sage" {
			t.Errorf("expected character_voice_gen elder_sage, got %v", req["character_voice_gen"])
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFF1234WAVEfmt audio-clip"))
	}))
	defer ts.Close()

	client, err := media.NewTTSClient(config.TTSConfig{
		Type:     "http",
		Endpoint: ts.URL + "/api/tts-generate",
	})
	if err != nil {
		t.Fatalf("NewTTSClient failed: %v", err)
	}

	data, err := client.Synthesize(context.Background(), "Greetings, traveler.", &entity.VoiceConfig{VoiceID: "elder_sage"})
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if !strings.HasPrefix(string(data), "RIFF") {
		t.Errorf("unexpected audio data: %s", string(data))
	}
}

func TestNewTTSClientBuildsElevenLabs(t *testing.T) {
	t.Setenv("ELEVENLABS_API_KEY", "")
	_, err := media.NewTTSClient(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"})
	if !errors.Is(err, media.ErrMissingAPIKey) {
		t.Fatalf("err = %v, want ErrMissingAPIKey when no key is set", err)
	}

	client, err := media.NewTTSClient(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs", APIKey: "abc"})
	if err != nil {
		t.Fatalf("NewTTSClient: %v", err)
	}
	if _, ok := client.(*media.ElevenLabsTTSClient); !ok {
		t.Errorf("client = %T, want *media.ElevenLabsTTSClient", client)
	}
}
