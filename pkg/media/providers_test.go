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

func TestNewImageClientBuildsGemini(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	client, err := media.NewImageClient(config.ImageConfig{
		Type:  "gemini",
		Model: "imagen-3.0-generate-002",
	})
	if err != nil {
		t.Fatalf("NewImageClient failed for gemini: %v", err)
	}
	if client == nil {
		t.Fatalf("expected non-nil image client")
	}

	builtinClient, err := media.NewImageClient(config.ImageConfig{
		Type:        "builtin",
		BuiltinName: "gemini",
		Model:       "gemini-3.1-flash-image",
	})
	if err != nil {
		t.Fatalf("NewImageClient failed for builtin gemini: %v", err)
	}
	if builtinClient == nil {
		t.Fatalf("expected non-nil builtin gemini image client")
	}
}

func TestNewTTSClientBuildsGemini(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "env-key")

	// 1. type: "gemini"
	client, err := media.NewTTSClientWithSharedKey(config.TTSConfig{
		Type: "gemini",
	}, "")
	if err != nil {
		t.Fatalf("expected gemini client to build, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	// 2. type: "builtin", builtin_name: "gemini"
	client, err = media.NewTTSClientWithSharedKey(config.TTSConfig{
		Type:        "builtin",
		BuiltinName: "gemini",
	}, "")
	if err != nil {
		t.Fatalf("expected builtin gemini client to build, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestResolveHTTPEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSpeech string
		wantVoices string
	}{
		{
			name:       "bare base url",
			input:      "http://localhost:8880",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "base url with trailing slash",
			input:      "http://localhost:8880/",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "full speech endpoint",
			input:      "http://localhost:8880/v1/audio/speech",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "v1 endpoint",
			input:      "http://localhost:8880/v1",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "alltalk endpoint preserved",
			input:      "http://localhost:7851/api/tts-generate",
			wantSpeech: "http://localhost:7851/api/tts-generate",
			wantVoices: "",
		},
		{
			name:       "empty endpoint",
			input:      "",
			wantSpeech: "",
			wantVoices: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSpeech, gotVoices := media.ResolveHTTPEndpoints(tt.input)
			if gotSpeech != tt.wantSpeech {
				t.Errorf("speechURL = %q, want %q", gotSpeech, tt.wantSpeech)
			}
			if gotVoices != tt.wantVoices {
				t.Errorf("voicesURL = %q, want %q", gotVoices, tt.wantVoices)
			}
		})
	}
}

func TestHTTPTTSClientSynthesizeKokoro(t *testing.T) {
	var receivedPath string
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("fake-mp3-audio"))
	}))
	defer server.Close()

	// Given a base URL (without /v1/audio/speech)
	client, err := media.NewTTSClient(config.TTSConfig{
		Type:     "http",
		Endpoint: server.URL,
		Model:    "kokoro",
	})
	if err != nil {
		t.Fatalf("NewTTSClient failed: %v", err)
	}

	audio, err := client.Synthesize(context.Background(), "Hello test", &entity.VoiceConfig{
		VoiceID:    "af_bella",
		SpeechRate: 1.25,
	})
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if string(audio) != "fake-mp3-audio" {
		t.Errorf("unexpected audio: %q", string(audio))
	}
	if receivedPath != "/v1/audio/speech" {
		t.Errorf("receivedPath = %q, want /v1/audio/speech", receivedPath)
	}
	if receivedBody["model"] != "kokoro" {
		t.Errorf("model = %v, want kokoro", receivedBody["model"])
	}
	if receivedBody["voice"] != "af_bella" {
		t.Errorf("voice = %v, want af_bella", receivedBody["voice"])
	}
	if receivedBody["allow_voice_tags"] != true {
		t.Errorf("allow_voice_tags = %v, want true", receivedBody["allow_voice_tags"])
	}
	if receivedBody["response_format"] != "mp3" {
		t.Errorf("response_format = %v, want mp3", receivedBody["response_format"])
	}
	if speed, ok := receivedBody["speed"].(float64); !ok || speed != 1.25 {
		t.Errorf("speed = %v, want 1.25", receivedBody["speed"])
	}
}

func TestHTTPTTSClientListVoices(t *testing.T) {
	t.Run("kokoro fastapi voices response object", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/audio/voices" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"voices": [
					{
						"id": "af_heart",
						"name": "af_heart",
						"target_quality": "A",
						"overall_grade": "A"
					},
					{
						"id": "bm_george",
						"name": "bm_george",
						"target_quality": "B",
						"overall_grade": "C"
					},
					{
						"id": "zf_xiaobei",
						"name": "zf_xiaobei"
					}
				],
				"default_voice": "af_heart"
			}`))
		}))
		defer server.Close()

		client, err := media.NewTTSClient(config.TTSConfig{
			Type:     "http",
			Endpoint: server.URL,
			Model:    "kokoro",
		})
		if err != nil {
			t.Fatalf("NewTTSClient: %v", err)
		}

		catalog, ok := client.(media.VoiceCatalog)
		if !ok {
			t.Fatalf("client does not implement media.VoiceCatalog")
		}

		voices, err := catalog.ListVoices(context.Background())
		if err != nil {
			t.Fatalf("ListVoices failed: %v", err)
		}

		if len(voices) != 3 {
			t.Fatalf("got %d voices, want 3", len(voices))
		}

		// Check af_heart
		heart := voices[0]
		if heart.ID != "af_heart" {
			t.Errorf("ID = %q, want af_heart", heart.ID)
		}
		if heart.Name != "Heart (American Female)" {
			t.Errorf("Name = %q, want Heart (American Female)", heart.Name)
		}
		if heart.Gender != "female" {
			t.Errorf("Gender = %q, want female", heart.Gender)
		}
		if heart.Language != "en-US" {
			t.Errorf("Language = %q, want en-US", heart.Language)
		}
		if heart.Accent != "American" {
			t.Errorf("Accent = %q, want American", heart.Accent)
		}

		// Check bm_george
		george := voices[1]
		if george.ID != "bm_george" {
			t.Errorf("ID = %q, want bm_george", george.ID)
		}
		if george.Name != "George (British Male)" {
			t.Errorf("Name = %q, want George (British Male)", george.Name)
		}
		if george.Gender != "male" {
			t.Errorf("Gender = %q, want male", george.Gender)
		}
		if george.Language != "en-GB" {
			t.Errorf("Language = %q, want en-GB", george.Language)
		}
		if george.Accent != "British" {
			t.Errorf("Accent = %q, want British", george.Accent)
		}

		// Check zf_xiaobei
		xiaobei := voices[2]
		if xiaobei.ID != "zf_xiaobei" {
			t.Errorf("ID = %q, want zf_xiaobei", xiaobei.ID)
		}
		if xiaobei.Name != "Xiaobei (Chinese Female)" {
			t.Errorf("Name = %q, want Xiaobei (Chinese Female)", xiaobei.Name)
		}
		if xiaobei.Language != "zh" {
			t.Errorf("Language = %q, want zh", xiaobei.Language)
		}
	})

	t.Run("bare array response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id": "af_bella", "name": "af_bella"},
				{"id": "am_adam", "name": "am_adam"}
			]`))
		}))
		defer server.Close()

		client, err := media.NewTTSClient(config.TTSConfig{
			Type:     "http",
			Endpoint: server.URL,
		})
		if err != nil {
			t.Fatalf("NewTTSClient: %v", err)
		}

		catalog, ok := client.(media.VoiceCatalog)
		if !ok {
			t.Fatalf("client does not implement media.VoiceCatalog")
		}

		voices, err := catalog.ListVoices(context.Background())
		if err != nil {
			t.Fatalf("ListVoices failed: %v", err)
		}

		if len(voices) != 2 {
			t.Fatalf("got %d voices, want 2", len(voices))
		}
		if voices[0].ID != "af_bella" || voices[0].Name != "Bella (American Female)" {
			t.Errorf("unexpected voice 0: %+v", voices[0])
		}
	})

	t.Run("unsupported endpoint returns empty with no error", func(t *testing.T) {
		client, err := media.NewTTSClient(config.TTSConfig{
			Type:     "http",
			Endpoint: "http://localhost:7851/api/tts-generate",
		})
		if err != nil {
			t.Fatalf("NewTTSClient: %v", err)
		}

		catalog, ok := client.(media.VoiceCatalog)
		if !ok {
			t.Fatalf("client does not implement media.VoiceCatalog")
		}

		voices, err := catalog.ListVoices(context.Background())
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if len(voices) != 0 {
			t.Errorf("expected 0 voices, got %d", len(voices))
		}
	})
}

