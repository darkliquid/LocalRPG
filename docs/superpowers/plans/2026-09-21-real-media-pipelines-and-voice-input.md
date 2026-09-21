# Real Media Pipelines and Voice Input Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate production-ready media engines into LocalRPG: HTTP Speech-to-Text (Faster-Whisper / OpenAI Whisper), browser-native Web Speech API, Action Console click-to-toggle voice dictation, multi-format image generation (AUTOMATIC1111/Forge base64, OpenAI DALL-E, and ComfyUI), and AllTalk/Kokoro TTS payload adapters.

**Architecture:** 
- In `pkg/media`: Extend `providers.go` with `httpSTTClient`, multi-format decoding in `httpImageClient`, `comfyUIImageClient`, and AllTalk payload mapping in `httpTTSClient`.
- In `pkg/gui`: Expose `POST /api/stt` on `Server` and `Service`, delegating uploaded audio to `STTClient.Transcribe`.
- In `frontend`: Create `useVoiceInput` hook supporting both browser-native `SpeechRecognition` and `MediaRecorder` audio upload to `/api/stt`. Wire click-to-toggle recording into `ActionConsole.tsx`. Add Web Speech provider option to `SettingsStudio.tsx` and `providerPresets.ts`.

**Tech Stack:** Go 1.27 (`net/http`, `mime/multipart`, `encoding/base64`, `encoding/json`), React 19, TypeScript, Tailwind v4, Web Speech API, MediaRecorder API.

---

## File Map

- **Media Backend**:
  - `pkg/media/providers.go`: Implement `httpSTTClient`, update `NewSTTClient`, update `httpImageClient`, add `comfyUIImageClient`, update `httpTTSClient`.
  - `pkg/media/providers_test.go`: Unit tests for `httpSTTClient`, `httpImageClient` base64/URL decoding, `comfyUIImageClient`, and `httpTTSClient` AllTalk adapter.
- **GUI Server & API**:
  - `pkg/gui/types.go`: Add `STTResponse` DTO.
  - `pkg/gui/service.go`: Add `TranscribeAudio(ctx, audioData)` method to `Service`, improve `TestProvider` for `"stt"` and `"image"`.
  - `pkg/gui/server.go`: Add `POST /api/stt` route.
  - `pkg/gui/server_test.go`: Tests for `POST /api/stt`.
- **Frontend Voice Input & UI**:
  - `frontend/src/api/client.ts`: Add `APIClient.transcribeAudio(blob: Blob): Promise<{ text: string }>`.
  - `frontend/src/types.ts`: Update `STTConfig.type` to allow `'web-speech'`.
  - `frontend/src/hooks/useVoiceInput.ts`: New hook managing Web Speech API and MediaRecorder capture.
  - `frontend/src/components/ActionConsole.tsx`: Wire click-to-toggle microphone button, recording indicators, and input population.
  - `frontend/src/templates/providerPresets.ts`: Add Web Speech preset and ensure Faster-Whisper / OpenAI Whisper configs match.
  - `frontend/src/components/SettingsStudio.tsx`: Add Web Speech API option to provider dropdown.

---

### Task 1: Implement `httpSTTClient` with Multipart Upload

**Files:**
- Modify: `pkg/media/providers.go`
- Test: `pkg/media/providers_test.go`

- [ ] **Step 1: Write the failing test**

In `pkg/media/providers_test.go`, add `TestHTTPSTTClient_TranscribesMultipartAudio`:

```go
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
	client, err := NewSTTClient(cfg)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestHTTPSTTClient ./pkg/media`  
Expected: FAIL (`unsupported stt provider type: http`)

- [ ] **Step 3: Implement `httpSTTClient` in `pkg/media/providers.go`**

Add `httpSTTClient`:
```go
type httpSTTClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

func (h *httpSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	modelName := h.model
	if modelName == "" {
		modelName = "whisper-1"
	}
	if err := writer.WriteField("model", modelName); err != nil {
		return "", fmt.Errorf("write model field: %w", err)
	}

	part, err := writer.CreateFormFile("file", "audio.webm")
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return "", fmt.Errorf("write audio data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.endpoint, &body)
	if err != nil {
		return "", fmt.Errorf("create stt request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("stt request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("stt failed (%d): %s", resp.StatusCode, string(respBytes))
	}

	var res struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("decode stt response: %w", err)
	}

	return strings.TrimSpace(res.Text), nil
}
```

And in `NewSTTClient`:
```go
	case "http":
		return &httpSTTClient{
			endpoint: cfg.Endpoint,
			model:    cfg.Model,
			apiKey:   cfg.APIKey,
			client:   &http.Client{Timeout: 60 * time.Second},
		}, nil
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestHTTPSTTClient ./pkg/media`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/providers.go pkg/media/providers_test.go
git commit -m "feat(media): implement http STT client for OpenAI Whisper and Faster-Whisper"
```

---

### Task 2: Multi-Format Image Generation Decoding & ComfyUI Adapter

**Files:**
- Modify: `pkg/media/providers.go`
- Test: `pkg/media/providers_test.go`

- [ ] **Step 1: Write the failing tests**

In `pkg/media/providers_test.go`, add:
1. `TestHTTPImageClient_DecodesA1111Base64`
2. `TestHTTPImageClient_DecodesOpenAIBase64`
3. `TestComfyUIImageClient_GeneratesImage`

```go
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

	client, err := NewImageClient(config.ImageConfig{
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

	client, err := NewImageClient(config.ImageConfig{
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

	client, err := NewImageClient(config.ImageConfig{
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run "TestHTTPImageClient|TestComfyUIImageClient" ./pkg/media`  
Expected: FAIL

- [ ] **Step 3: Implement multi-format decoding & ComfyUI adapter in `pkg/media/providers.go`**

In `httpImageClient.GenerateImage`:
1. Check if endpoint targets ComfyUI (`strings.Contains(h.endpoint, ":8188")` or `strings.HasSuffix(h.endpoint, "/prompt")`): delegate to `comfyUIImageClient`.
2. Format payload:
   - If `strings.Contains(h.endpoint, "/sdapi/v1/txt2img")` (A1111/Forge):
     ```go
     payload, _ = json.Marshal(map[string]interface{}{
         "prompt":          prompt,
         "negative_prompt": "blurry, low quality, deformed",
         "steps":           20,
         "width":           512,
         "height":          512,
     })
     ```
   - Otherwise:
     ```go
     payload, _ = json.Marshal(map[string]interface{}{
         "model":           h.model,
         "prompt":          prompt,
         "n":               1,
         "size":            "512x512",
         "response_format": "b64_json",
     })
     ```
3. Process response body:
   - If starts with `\x89PNG`, `\xff\xd8\xff`, `RIFF` (WebP): return raw bytes.
   - If JSON:
     - Check `images: []string` (A1111): `base64.StdEncoding.DecodeString(images[0])`.
     - Check `data[0].b64_json` (OpenAI): `base64.StdEncoding.DecodeString(...)`.
     - Check `data[0].url`: HTTP GET the URL.
4. Add `comfyUIImageClient`:
   - Posts minimal Text-to-Image prompt workflow to `/prompt`.
   - Polls `/history/{prompt_id}` until complete.
   - Fetches image from `/view?filename=...`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestHTTPImageClient|TestComfyUIImageClient" ./pkg/media`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/providers.go pkg/media/providers_test.go
git commit -m "feat(media): add multi-format image decoding for A1111, DALL-E, and ComfyUI"
```

---

### Task 3: TTS Payload Adapter for AllTalk & OpenAI

**Files:**
- Modify: `pkg/media/providers.go`
- Test: `pkg/media/providers_test.go`

- [ ] **Step 1: Write the failing test**

In `pkg/media/providers_test.go`, add `TestHTTPTTSClient_AdaptsAllTalkPayload`:

```go
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

	client, err := NewTTSClient(config.TTSConfig{
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestHTTPTTSClient_AdaptsAllTalkPayload ./pkg/media`  
Expected: FAIL

- [ ] **Step 3: Implement AllTalk schema adapter in `httpTTSClient.Synthesize`**

In `pkg/media/providers.go:114`:
```go
func (h *httpTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	voiceID := "alloy"
	if voice != nil && voice.VoiceID != "" {
		voiceID = voice.VoiceID
	}

	var payload []byte
	if strings.Contains(h.endpoint, "/api/tts-generate") || strings.Contains(h.endpoint, "alltalk") {
		payload, _ = json.Marshal(map[string]interface{}{
			"text_input":          text,
			"character_voice_gen": voiceID,
			"narrator_voice_gen":  voiceID,
			"text_filtering":      "standard",
			"language":            "en",
		})
	} else {
		payloadMap := map[string]interface{}{
			"model": h.model,
			"input": text,
			"voice": voiceID,
		}
		if voice != nil && voice.SpeechRate > 0 {
			payloadMap["speed"] = voice.SpeechRate
		}
		payload, _ = json.Marshal(payloadMap)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.endpoint, bytes.NewReader(payload))
...
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestHTTPTTSClient ./pkg/media`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/providers.go pkg/media/providers_test.go
git commit -m "feat(media): add AllTalk TTS payload adapter in httpTTSClient"
```

---

### Task 4: Backend `POST /api/stt` Endpoint & Diagnostics

**Files:**
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/server.go`
- Test: `pkg/gui/server_test.go`

- [ ] **Step 1: Write failing tests in `pkg/gui/server_test.go`**

Add `TestSTTEndpoint_TranscribesAudio` and `TestSTTEndpoint_RejectsWhenDisabled`:

```go
func TestSTTEndpoint_TranscribesAudio(t *testing.T) {
	svc, rootDir := newTestService(t)
	// Configure mock/builtin echo STT
	cfg, _ := svc.GetSettings(context.Background())
	cfg.Config.Media.STT = config.STTConfig{
		Type: "builtin",
	}
	_ = svc.SaveSettings(context.Background(), cfg.Config)

	server := NewServer(svc, rootDir)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("audio", "speech.webm")
	_, _ = part.Write([]byte("fake-audio-bytes"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/stt", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (%s)", w.Code, w.Body.String())
	}

	var res map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res["text"] == "" {
		t.Errorf("expected non-empty transcribed text")
	}
}

func TestSTTEndpoint_RejectsWhenDisabled(t *testing.T) {
	svc, rootDir := newTestService(t)
	cfg, _ := svc.GetSettings(context.Background())
	cfg.Config.Media.STT = config.STTConfig{Type: "disabled"}
	_ = svc.SaveSettings(context.Background(), cfg.Config)

	server := NewServer(svc, rootDir)
	req := httptest.NewRequest(http.MethodPost, "/api/stt", bytes.NewReader([]byte("audio")))
	req.Header.Set("Content-Type", "audio/webm")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestSTTEndpoint ./pkg/gui`  
Expected: FAIL (`404 Not Found`)

- [ ] **Step 3: Implement `TranscribeAudio` and `POST /api/stt` route**

1. In `pkg/gui/types.go`:
```go
type STTResponse struct {
	Text string `json:"text"`
}
```

2. In `pkg/gui/service.go`:
```go
func (s *Service) TranscribeAudio(ctx context.Context, audioData []byte) (string, error) {
	cfg := s.configMgr.Get()
	if cfg.Media.STT.Type == "" || cfg.Media.STT.Type == "disabled" {
		return "", fmt.Errorf("STT engine is disabled or unconfigured")
	}

	client, err := media.NewSTTClient(cfg.Media.STT)
	if err != nil {
		return "", fmt.Errorf("initialize STT client: %w", err)
	}

	return client.Transcribe(ctx, audioData)
}
```
And in `Service.TestProvider` for `"stt"`:
Use a valid audio buffer (`media.GenerateTone(100*time.Millisecond)` or valid small WAV) rather than `"fake-audio-header"` so real Whisper servers don't reject the connection test.

3. In `pkg/gui/server.go`:
Handle `/api/stt`:
```go
	case "stt":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleSTTTranscribe(w, r)
```
Implement `handleSTTTranscribe`:
- Reads up to 25 MB using `http.MaxBytesReader`.
- Checks multipart form (`audio` or `file` part), or raw body.
- Calls `s.service.TranscribeAudio(r.Context(), audioData)`.
- Writes JSON `STTResponse{Text: text}`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestSTTEndpoint ./pkg/gui`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/server.go pkg/gui/server_test.go
git commit -m "feat(gui): add POST /api/stt endpoint for speech-to-text dictation"
```

---

### Task 5: Frontend API Client & `useVoiceInput` Hook

**Files:**
- Modify: `frontend/src/api/client.ts`
- Modify: `frontend/src/types.ts`
- Create: `frontend/src/hooks/useVoiceInput.ts`

- [ ] **Step 1: Add `transcribeAudio` to `frontend/src/api/client.ts`**

```typescript
  static async transcribeAudio(audioBlob: Blob): Promise<{ text: string }> {
    const formData = new FormData();
    formData.append('audio', audioBlob, 'speech.webm');

    const res = await fetch('/api/stt', {
      method: 'POST',
      body: formData,
    });

    if (!res.ok) {
      const errText = await res.text();
      throw new Error(errText || `STT failed with status ${res.status}`);
    }

    return res.json();
  }
```

- [ ] **Step 2: Update `frontend/src/types.ts`**

Update `STTConfig`:
```typescript
export interface STTConfig {
  type: 'disabled' | 'http' | 'cli' | 'builtin' | 'web-speech';
  endpoint?: string;
  model?: string;
  command?: string;
  args?: string[];
  api_key?: string;
}
```

- [ ] **Step 3: Create `frontend/src/hooks/useVoiceInput.ts`**

Create hook that manages Web Speech API (`window.SpeechRecognition` / `webkitSpeechRecognition`) and fallback to `MediaRecorder` + `APIClient.transcribeAudio`:

```typescript
import { useState, useRef, useCallback } from 'react';
import { APIClient } from '../api/client';

interface UseVoiceInputOptions {
  onTranscribed: (text: string) => void;
  sttType?: string;
}

export const useVoiceInput = ({ onTranscribed, sttType }: UseVoiceInputOptions) => {
  const [isRecording, setIsRecording] = useState(false);
  const [isTranscribing, setIsTranscribing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const audioChunksRef = useRef<Blob[]>([]);
  const speechRecognitionRef = useRef<any>(null);

  const startRecording = useCallback(async () => {
    setError(null);

    // 1. Web Speech Mode
    const SpeechRecognition =
      (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;

    if (sttType === 'web-speech' && SpeechRecognition) {
      try {
        const recognition = new SpeechRecognition();
        recognition.continuous = false;
        recognition.interimResults = false;
        recognition.lang = navigator.language || 'en-US';

        recognition.onresult = (event: any) => {
          const transcript = event.results[0]?.[0]?.transcript;
          if (transcript) {
            onTranscribed(transcript);
          }
        };

        recognition.onerror = (event: any) => {
          console.error('Speech recognition error:', event.error);
          setError(`Speech recognition: ${event.error}`);
          setIsRecording(false);
        };

        recognition.onend = () => {
          setIsRecording(false);
        };

        recognition.start();
        speechRecognitionRef.current = recognition;
        setIsRecording(true);
        return;
      } catch (err: any) {
        console.warn('Web speech failed to start, falling back to MediaRecorder:', err);
      }
    }

    // 2. MediaRecorder Mode
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      audioChunksRef.current = [];

      let mimeType = 'audio/webm;codecs=opus';
      if (!MediaRecorder.isTypeSupported(mimeType)) {
        mimeType = MediaRecorder.isTypeSupported('audio/webm')
          ? 'audio/webm'
          : MediaRecorder.isTypeSupported('audio/ogg')
          ? 'audio/ogg'
          : '';
      }

      const recorder = mimeType ? new MediaRecorder(stream, { mimeType }) : new MediaRecorder(stream);
      recorder.ondataavailable = (e) => {
        if (e.data && e.data.size > 0) {
          audioChunksRef.current.push(e.data);
        }
      };

      recorder.onstop = async () => {
        stream.getTracks().forEach((track) => track.stop());
        const audioBlob = new Blob(audioChunksRef.current, {
          type: mimeType || 'audio/webm',
        });

        if (audioBlob.size === 0) {
          return;
        }

        setIsTranscribing(true);
        try {
          const res = await APIClient.transcribeAudio(audioBlob);
          if (res.text) {
            onTranscribed(res.text);
          }
        } catch (err: any) {
          console.error('Transcription failed:', err);
          setError(err.message || 'Transcription failed');
        } finally {
          setIsTranscribing(false);
        }
      };

      recorder.start();
      mediaRecorderRef.current = recorder;
      setIsRecording(true);
    } catch (err: any) {
      console.error('Microphone access error:', err);
      setError(err.name === 'NotAllowedError' ? 'Microphone permission denied' : err.message);
      setIsRecording(false);
    }
  }, [onTranscribed, sttType]);

  const stopRecording = useCallback(() => {
    if (speechRecognitionRef.current) {
      speechRecognitionRef.current.stop();
      speechRecognitionRef.current = null;
    }
    if (mediaRecorderRef.current && mediaRecorderRef.current.state === 'recording') {
      mediaRecorderRef.current.stop();
      mediaRecorderRef.current = null;
    }
    setIsRecording(false);
  }, []);

  const toggleRecording = useCallback(() => {
    if (isRecording) {
      stopRecording();
    } else {
      void startRecording();
    }
  }, [isRecording, startRecording, stopRecording]);

  return {
    isRecording,
    isTranscribing,
    error,
    toggleRecording,
    startRecording,
    stopRecording,
  };
};
```

- [ ] **Step 4: Verify frontend type check**

Run: `cd frontend && npx tsc --noEmit`  
Expected: PASS with 0 errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/api/client.ts frontend/src/types.ts frontend/src/hooks/useVoiceInput.ts
git commit -m "feat(frontend): add transcribeAudio API and useVoiceInput hook"
```

---

### Task 6: Action Console Microphone UI Integration

**Files:**
- Modify: `frontend/src/components/ActionConsole.tsx`
- Modify: `frontend/src/App.tsx` (pass `config.media.stt.type` into `ActionConsole`)

- [ ] **Step 1: Wire `useVoiceInput` into `ActionConsole.tsx`**

Update `ActionConsoleProps`:
```typescript
interface ActionConsoleProps {
  onSubmit: (mode: string, text: string) => void;
  disabled?: boolean;
  streaming?: boolean;
  onStop?: () => void;
  sttType?: string;
}
```

In `ActionConsole`:
```typescript
  const handleVoiceTranscribed = (transcript: string) => {
    setText((prev) => (prev ? `${prev} ${transcript}` : transcript));
  };

  const { isRecording, isTranscribing, error: voiceError, toggleRecording } = useVoiceInput({
    onTranscribed: handleVoiceTranscribed,
    sttType,
  });
```

And update the `<Mic>` button:
```tsx
        <button
          type="button"
          onClick={toggleRecording}
          disabled={isInputDisabled || isTranscribing}
          className={`shrink-0 p-2.5 rounded-xl border transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed ${
            isRecording
              ? 'bg-red-900/60 text-red-300 border-red-500/80 animate-pulse shadow-lg shadow-red-900/40'
              : 'bg-stone-900/70 border-white/5 hover:bg-stone-800 text-stone-400 hover:text-amber-400'
          }`}
          title={
            isRecording
              ? 'Recording speech... Click to stop and transcribe'
              : isTranscribing
              ? 'Transcribing audio...'
              : 'Dictate action with voice'
          }
        >
          {isTranscribing ? (
            <Loader2 className="w-5 h-5 animate-spin text-amber-400" />
          ) : (
            <Mic className={`w-5 h-5 ${isRecording ? 'text-red-300' : ''}`} />
          )}
        </button>
```

Display `voiceError` subtly under the input box if present.

In `frontend/src/App.tsx`, pass `sttType={config?.media.stt?.type}` to `<ActionConsole />`.

- [ ] **Step 2: Verify frontend type check**

Run: `cd frontend && npx tsc --noEmit`  
Expected: PASS with 0 errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/ActionConsole.tsx frontend/src/App.tsx
git commit -m "feat(frontend): wire click-to-toggle microphone in ActionConsole"
```

---

### Task 7: Settings Studio Web Speech API Presets

**Files:**
- Modify: `frontend/src/templates/providerPresets.ts`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [ ] **Step 1: Add Web Speech preset to `providerPresets.ts`**

In `frontend/src/templates/providerPresets.ts`:
```typescript
export const STT_PRESETS: Record<string, PresetItem<STTConfig>> = {
  'web-speech': {
    label: 'Web Speech API (Browser Native)',
    description: 'Zero-setup, real-time in-browser speech recognition without a background server.',
    config: {
      type: 'web-speech',
    },
  },
  'faster-whisper': {
...
```

- [ ] **Step 2: Add Web Speech option to dropdown in `SettingsStudio.tsx`**

In `SettingsStudio.tsx:993`:
```tsx
                <label className="text-xs font-cinzel uppercase text-stone-300">STT Provider Type</label>
                <select
                  value={config.media.stt?.type || 'disabled'}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      media: {
                        ...config.media,
                        stt: { ...(config.media.stt || { type: 'disabled' }), type: e.target.value as any },
                      },
                    })
                  }
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-amber-500/60 cursor-pointer"
                >
                  <option value="disabled">Disabled</option>
                  <option value="web-speech">Web Speech API (Browser Native)</option>
                  <option value="http">HTTP (Faster-Whisper, OpenAI Whisper)</option>
                  <option value="cli">CLI Command (e.g. whisper-cli)</option>
                  <option value="builtin">Builtin / Mock</option>
                </select>
```

- [ ] **Step 3: Verify frontend type check**

Run: `cd frontend && npx tsc --noEmit`  
Expected: PASS with 0 errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/templates/providerPresets.ts frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): add Web Speech API option to Settings Studio"
```

---

### Task 8: End-to-End Verification & Full Build

**Files:** None (testing only)

- [ ] **Step 1: Run frontend test check**

Run: `mise run test:frontend`  
Expected: PASS with 0 errors.

- [ ] **Step 2: Run backend tests**

Run: `mise run test:backend`  
Expected: ALL PASS.

- [ ] **Step 3: Run full production build**

Run: `mise run build`  
Expected: PASS with binary at `bin/localrpg`.

- [ ] **Step 4: Restore `.gitkeep` placeholder**

Run: `git checkout pkg/gui/dist/.gitkeep`  
Run: `git status`  
Expected: Clean working tree.
