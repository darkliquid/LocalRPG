package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

var ErrProviderDisabled = errors.New("provider is disabled")

// Disabled implementations
type disabledTTSClient struct{}

func (d *disabledTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return nil, ErrProviderDisabled
}

type disabledSTTClient struct{}

func (d *disabledSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	return "", ErrProviderDisabled
}

type disabledImageClient struct{}

func (d *disabledImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	return nil, ErrProviderDisabled
}

// Builtin / Echo implementations
type echoTTSClient struct{}

func (e *echoTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return []byte("RIFF....WAVEfmt ....data" + text), nil
}

type echoSTTClient struct{}

func (e *echoSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	return "Transcribed audio sample", nil
}

type echoImageClient struct{}

func (e *echoImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	return []byte("fake-image-bytes-for-" + prompt), nil
}

// CLI implementations
type cliTTSClient struct {
	command string
	args    []string
}

func (c *cliTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.command, c.args...)
	cmd.Stdin = bytes.NewBufferString(text)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cli tts error: %w", err)
	}
	return out.Bytes(), nil
}

type cliSTTClient struct {
	command string
	args    []string
}

func (c *cliSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	cmd := exec.CommandContext(ctx, c.command, c.args...)
	cmd.Stdin = bytes.NewReader(audioData)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cli stt error: %w", err)
	}
	return out.String(), nil
}

type cliImageClient struct {
	command string
	args    []string
}

func (c *cliImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.command, append(c.args, prompt)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cli image error: %w", err)
	}
	return out.Bytes(), nil
}

// ResolveHTTPEndpoints normalizes an HTTP TTS endpoint into a speech synthesis URL
// and a voice catalog URL. If the endpoint is already a full speech URL, it extracts
// the base URL to resolve the voices endpoint. Custom/AllTalk endpoints keep their speech URL
// and return an empty voices URL.
func ResolveHTTPEndpoints(endpoint string) (speechURL, voicesURL string) {
	raw := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if raw == "" {
		return "", ""
	}

	if strings.Contains(raw, "/api/tts-generate") || strings.Contains(raw, "alltalk") {
		return raw, ""
	}

	baseURL := raw
	if strings.HasSuffix(baseURL, "/v1/audio/speech") {
		baseURL = strings.TrimSuffix(baseURL, "/v1/audio/speech")
	} else if strings.HasSuffix(baseURL, "/v1/audio") {
		baseURL = strings.TrimSuffix(baseURL, "/v1/audio")
	} else if strings.HasSuffix(baseURL, "/v1") {
		baseURL = strings.TrimSuffix(baseURL, "/v1")
	}
	baseURL = strings.TrimRight(baseURL, "/")

	return baseURL + "/v1/audio/speech", baseURL + "/v1/audio/voices"
}

// HTTP implementations (OpenAI compatible)
type httpTTSClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

func (h *httpTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	voiceID := "alloy"
	if voice != nil && voice.VoiceID != "" {
		voiceID = voice.VoiceID
	}
	speechURL, _ := ResolveHTTPEndpoints(h.endpoint)
	if speechURL == "" {
		speechURL = h.endpoint
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
		isKokoro := strings.Contains(strings.ToLower(h.model), "kokoro") || strings.Contains(h.endpoint, "8880")
		payloadMap := map[string]interface{}{
			"model": h.model,
			"input": text,
			"voice": voiceID,
		}
		if isKokoro {
			payloadMap["response_format"] = "mp3"
			payloadMap["allow_voice_tags"] = true
		}
		if voice != nil && voice.SpeechRate > 0 {
			payloadMap["speed"] = voice.SpeechRate
		}
		payload, _ = json.Marshal(payloadMap)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", speechURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http tts failed (%d): %s", resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

var _ VoiceCatalog = (*httpTTSClient)(nil)

type kokoroVoiceItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	TargetQuality    string `json:"target_quality,omitempty"`
	TrainingDuration string `json:"training_duration,omitempty"`
	OverallGrade     string `json:"overall_grade,omitempty"`
}

func (h *httpTTSClient) ListVoices(ctx context.Context) ([]ProviderVoice, error) {
	_, voicesURL := ResolveHTTPEndpoints(h.endpoint)
	if voicesURL == "" {
		return []ProviderVoice{}, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", voicesURL, nil)
	if err != nil {
		return nil, err
	}
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []ProviderVoice{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http voices failed (%d): %s", resp.StatusCode, string(b))
	}

	var rawData json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&rawData); err != nil {
		return nil, fmt.Errorf("failed to decode voices response: %w", err)
	}

	var items []kokoroVoiceItem
	var envelope struct {
		Voices []kokoroVoiceItem `json:"voices"`
	}
	if err := json.Unmarshal(rawData, &envelope); err == nil && len(envelope.Voices) > 0 {
		items = envelope.Voices
	} else if err := json.Unmarshal(rawData, &items); err == nil && len(items) > 0 {
		// parsed array of objects
	} else {
		// try string array
		var stringIDs []string
		if err := json.Unmarshal(rawData, &stringIDs); err == nil {
			for _, id := range stringIDs {
				items = append(items, kokoroVoiceItem{ID: id, Name: id})
			}
		}
	}

	voices := make([]ProviderVoice, 0, len(items))
	for _, item := range items {
		voices = append(voices, formatKokoroVoice(item))
	}
	return voices, nil
}

func formatKokoroVoice(item kokoroVoiceItem) ProviderVoice {
	id := strings.TrimSpace(item.ID)
	if id == "" {
		id = strings.TrimSpace(item.Name)
	}
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = id
	}

	var lang, gender, accent string
	tags := []string{}

	parts := strings.SplitN(id, "_", 2)
	if len(parts) == 2 && len(parts[0]) == 2 {
		langCode := parts[0][0]
		genderCode := parts[0][1]

		switch langCode {
		case 'a':
			lang = "en-US"
			accent = "American"
			tags = append(tags, "american")
		case 'b':
			lang = "en-GB"
			accent = "British"
			tags = append(tags, "british")
		case 'e':
			lang = "es"
			accent = "Spanish"
			tags = append(tags, "spanish")
		case 'f':
			lang = "fr"
			accent = "French"
			tags = append(tags, "french")
		case 'h':
			lang = "hi"
			accent = "Hindi"
			tags = append(tags, "hindi")
		case 'i':
			lang = "it"
			accent = "Italian"
			tags = append(tags, "italian")
		case 'j':
			lang = "ja"
			accent = "Japanese"
			tags = append(tags, "japanese")
		case 'p':
			lang = "pt-BR"
			accent = "Portuguese"
			tags = append(tags, "portuguese")
		case 'z':
			lang = "zh"
			accent = "Chinese"
			tags = append(tags, "chinese")
		}

		switch genderCode {
		case 'f':
			gender = "female"
			tags = append(tags, "female")
		case 'm':
			gender = "male"
			tags = append(tags, "male")
		}

		voiceName := parts[1]
		tags = append(tags, strings.ToLower(voiceName))
		tags = append(tags, "kokoro")

		if name == id || name == voiceName {
			displayName := strings.ToUpper(voiceName[:1]) + voiceName[1:]
			if accent != "" && gender != "" {
				displayName = fmt.Sprintf("%s (%s %s)", displayName, accent, strings.ToUpper(gender[:1])+gender[1:])
			}
			name = displayName
		}
	} else {
		tags = append(tags, "kokoro")
	}

	if item.OverallGrade != "" {
		tags = append(tags, "grade:"+item.OverallGrade)
	}
	if item.TargetQuality != "" {
		tags = append(tags, "quality:"+item.TargetQuality)
	}

	return ProviderVoice{
		ID:       id,
		Name:     name,
		Language: lang,
		Gender:   gender,
		Accent:   accent,
		Tags:     tags,
		Defaults: map[string]interface{}{
			"pitch":       1.0,
			"speech_rate": 1.0,
		},
	}
}

// SpeechCueCapabilities advertises Markdown emphasis support for HTTP TTS endpoints.
func (h *httpTTSClient) SpeechCueCapabilities() SpeechCueCapabilities {
	return SpeechCueCapabilities{
		AudioTags:        false,
		MarkdownEmphasis: true,
	}
}

func isComfyUI(endpoint string) bool {
	return strings.Contains(endpoint, ":8188") || strings.HasSuffix(endpoint, "/prompt")
}

func getComfyBaseURL(endpoint string) string {
	ep := strings.TrimRight(endpoint, "/")
	ep = strings.TrimSuffix(ep, "/prompt")
	if strings.HasSuffix(ep, ":8188") && strings.Count(ep, ":") > 2 {
		ep = strings.TrimSuffix(ep, ":8188")
	}
	return ep
}

type comfyUIImageClient struct {
	endpoint string
	client   *http.Client
}

func (c *comfyUIImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	baseURL := getComfyBaseURL(c.endpoint)

	workflow := map[string]interface{}{
		"prompt": map[string]interface{}{
			"3": map[string]interface{}{
				"inputs": map[string]interface{}{
					"seed":         156680208700286,
					"steps":        20,
					"cfg":          8,
					"sampler_name": "euler",
					"scheduler":    "normal",
					"denoise":      1,
					"model":        []interface{}{"4", 0},
					"positive":     []interface{}{"6", 0},
					"negative":     []interface{}{"7", 0},
					"latent_image": []interface{}{"5", 0},
				},
				"class_type": "KSampler",
			},
			"4": map[string]interface{}{
				"inputs": map[string]interface{}{
					"ckpt_name": "v1-5-pruned-emaonly.ckpt",
				},
				"class_type": "CheckpointLoaderSimple",
			},
			"5": map[string]interface{}{
				"inputs": map[string]interface{}{
					"width":      512,
					"height":     512,
					"batch_size": 1,
				},
				"class_type": "EmptyLatentImage",
			},
			"6": map[string]interface{}{
				"inputs": map[string]interface{}{
					"text": prompt,
					"clip": []interface{}{"4", 1},
				},
				"class_type": "CLIPTextEncode",
			},
			"7": map[string]interface{}{
				"inputs": map[string]interface{}{
					"text": "bad quality, blurry",
					"clip": []interface{}{"4", 1},
				},
				"class_type": "CLIPTextEncode",
			},
			"8": map[string]interface{}{
				"inputs": map[string]interface{}{
					"samples": []interface{}{"3", 0},
					"vae":     []interface{}{"4", 2},
				},
				"class_type": "VAEDecode",
			},
			"9": map[string]interface{}{
				"inputs": map[string]interface{}{
					"filename_prefix": "LocalRPG",
					"images":          []interface{}{"8", 0},
				},
				"class_type": "SaveImage",
			},
		},
	}

	bodyBytes, err := json.Marshal(workflow)
	if err != nil {
		return nil, fmt.Errorf("marshal comfyui prompt: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/prompt", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create comfyui prompt request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post comfyui prompt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("comfyui prompt failed (%d): %s", resp.StatusCode, string(b))
	}

	var promptResp struct {
		PromptID string `json:"prompt_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&promptResp); err != nil {
		return nil, fmt.Errorf("decode comfyui prompt response: %w", err)
	}
	if promptResp.PromptID == "" {
		return nil, fmt.Errorf("comfyui returned empty prompt_id")
	}

	pollTicker := time.NewTicker(200 * time.Millisecond)
	defer pollTicker.Stop()

	timeout := time.After(60 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for comfyui image generation")
		case <-pollTicker.C:
			histReq, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/history/%s", baseURL, promptResp.PromptID), nil)
			if err != nil {
				return nil, err
			}
			histResp, err := c.client.Do(histReq)
			if err != nil {
				continue
			}
			if histResp.StatusCode != http.StatusOK {
				histResp.Body.Close()
				continue
			}

			var histData map[string]interface{}
			err = json.NewDecoder(histResp.Body).Decode(&histData)
			histResp.Body.Close()
			if err != nil {
				continue
			}

			pData, ok := histData[promptResp.PromptID].(map[string]interface{})
			if !ok {
				continue
			}
			outputs, ok := pData["outputs"].(map[string]interface{})
			if !ok {
				continue
			}

			var filename, subfolder, imgType string
			found := false
			for _, nodeOut := range outputs {
				nodeMap, ok := nodeOut.(map[string]interface{})
				if !ok {
					continue
				}
				images, ok := nodeMap["images"].([]interface{})
				if !ok || len(images) == 0 {
					continue
				}
				imgMap, ok := images[0].(map[string]interface{})
				if !ok {
					continue
				}
				filename, _ = imgMap["filename"].(string)
				subfolder, _ = imgMap["subfolder"].(string)
				imgType, _ = imgMap["type"].(string)
				if filename != "" {
					found = true
					break
				}
			}

			if !found {
				continue
			}

			viewURL := fmt.Sprintf("%s/view?filename=%s&subfolder=%s&type=%s", baseURL, filename, subfolder, imgType)
			viewReq, err := http.NewRequestWithContext(ctx, "GET", viewURL, nil)
			if err != nil {
				return nil, fmt.Errorf("create comfyui view request: %w", err)
			}
			viewResp, err := c.client.Do(viewReq)
			if err != nil {
				return nil, fmt.Errorf("fetch comfyui image: %w", err)
			}
			defer viewResp.Body.Close()

			if viewResp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(viewResp.Body)
				return nil, fmt.Errorf("comfyui view failed (%d): %s", viewResp.StatusCode, string(b))
			}

			return io.ReadAll(viewResp.Body)
		}
	}
}

type httpImageClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

func (h *httpImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	if isComfyUI(h.endpoint) {
		comfyClient := &comfyUIImageClient{
			endpoint: h.endpoint,
			client:   h.client,
		}
		return comfyClient.GenerateImage(ctx, prompt)
	}

	var payload []byte
	if strings.Contains(h.endpoint, "/sdapi/v1/txt2img") {
		payload, _ = json.Marshal(map[string]interface{}{
			"prompt":          prompt,
			"negative_prompt": "blurry, low quality, deformed",
			"steps":           20,
			"width":           512,
			"height":          512,
		})
	} else {
		payloadMap := map[string]interface{}{
			"prompt":          prompt,
			"n":               1,
			"size":            "512x512",
			"response_format": "b64_json",
		}
		if h.model != "" {
			payloadMap["model"] = h.model
		}
		payload, _ = json.Marshal(payloadMap)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http image failed (%d): %s", resp.StatusCode, string(b))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 1. Raw Binary detection: PNG, JPEG, WebP
	if bytes.HasPrefix(bodyBytes, []byte("\x89PNG")) ||
		bytes.HasPrefix(bodyBytes, []byte("\xff\xd8\xff")) ||
		(bytes.HasPrefix(bodyBytes, []byte("RIFF")) && bytes.Contains(bodyBytes[:min(len(bodyBytes), 16)], []byte("WEBP"))) {
		return bodyBytes, nil
	}

	// 2. Parse JSON response
	var respData map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &respData); err != nil {
		return bodyBytes, nil
	}

	// AUTOMATIC1111 / Forge WebUI: {"images": ["base64..."]}
	if images, ok := respData["images"].([]interface{}); ok && len(images) > 0 {
		if imgStr, ok := images[0].(string); ok {
			if idx := strings.Index(imgStr, ","); idx != -1 && strings.HasPrefix(imgStr, "data:") {
				imgStr = imgStr[idx+1:]
			}
			decoded, err := base64.StdEncoding.DecodeString(imgStr)
			if err != nil {
				return nil, fmt.Errorf("decode a1111 base64: %w", err)
			}
			return decoded, nil
		}
	}

	// OpenAI format: {"data": [{"b64_json": "..."} | {"url": "..."}]}
	if data, ok := respData["data"].([]interface{}); ok && len(data) > 0 {
		if item, ok := data[0].(map[string]interface{}); ok {
			if b64, ok := item["b64_json"].(string); ok && b64 != "" {
				decoded, err := base64.StdEncoding.DecodeString(b64)
				if err != nil {
					return nil, fmt.Errorf("decode openai b64_json: %w", err)
				}
				return decoded, nil
			}
			if imgURL, ok := item["url"].(string); ok && imgURL != "" {
				getReq, err := http.NewRequestWithContext(ctx, "GET", imgURL, nil)
				if err != nil {
					return nil, fmt.Errorf("create image get request: %w", err)
				}
				getResp, err := h.client.Do(getReq)
				if err != nil {
					return nil, fmt.Errorf("fetch image url: %w", err)
				}
				defer getResp.Body.Close()
				if getResp.StatusCode != http.StatusOK {
					return nil, fmt.Errorf("fetch image url failed (%d)", getResp.StatusCode)
				}
				return io.ReadAll(getResp.Body)
			}
		}
	}

	return bodyBytes, nil
}

// Factory Constructors
func NewTTSClient(cfg config.TTSConfig) (TTSClient, error) {
	return NewTTSClientWithSharedKey(cfg, "")
}

// NewTTSClientWithSharedKey builds a TTSClient from configuration and an optional shared key.
func NewTTSClientWithSharedKey(cfg config.TTSConfig, sharedKey string) (TTSClient, error) {
	// The registry is authoritative when the binary imported pkg/provider/all;
	// otherwise the inline switch below still builds the client.
	if regID := TTSProviderIDFor(cfg); regID != "" {
		if client, err := BuildTTS(regID, cfg, sharedKey); err == nil {
			return client, nil
		}
	}
	switch cfg.Type {
	case "disabled", "":
		return &disabledTTSClient{}, nil
	case "gemini":
		return NewGeminiTTSClient(cfg, sharedKey)
	case "builtin":
		switch cfg.BuiltinName {
		case "gemini":
			return NewGeminiTTSClient(cfg, sharedKey)
		case "sherpa-onnx", "kokoro":
			modelDir := cfg.ModelPath
			if modelDir == "" {
				modelDir = "./cache/models/tts/kokoro"
			}
			return NewSherpaTTSClient(modelDir), nil
		case "native-os":
			return NewNativeOSTTSClient(), nil
		case "elevenlabs":
			return NewElevenLabsTTSClient(cfg)
		default:
			return &echoTTSClient{}, nil
		}
	case "cli":
		return &cliTTSClient{command: cfg.Command, args: cfg.Args}, nil
	case "http":
		return &httpTTSClient{endpoint: cfg.Endpoint, model: cfg.Model, apiKey: cfg.APIKey, client: &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 30 * time.Second}}, nil
	default:
		return nil, fmt.Errorf("unsupported tts provider type: %s", cfg.Type)
	}
}

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

func NewSTTClient(cfg config.STTConfig) (STTClient, error) {
	// Registry-first when pkg/provider/all was imported; inline otherwise.
	if regID := STTProviderIDFor(cfg); regID != "" {
		if client, err := BuildSTT(regID, cfg); err == nil {
			return client, nil
		}
	}
	switch cfg.Type {
	case "disabled", "":
		return &disabledSTTClient{}, nil
	case "builtin":
		return &echoSTTClient{}, nil
	case "cli":
		return &cliSTTClient{command: cfg.Command, args: cfg.Args}, nil
	case "http":
		return &httpSTTClient{
			endpoint: cfg.Endpoint,
			model:    cfg.Model,
			apiKey:   cfg.APIKey,
			client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 60 * time.Second},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported stt provider type: %s", cfg.Type)
	}
}

// fallbackImageClient covers a disabled or failing provider with the built-in
// generator, so a scene always has art offline.
type fallbackImageClient struct {
	primary  ImageClient
	fallback ImageClient
}

func (c *fallbackImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	data, err := c.primary.GenerateImage(ctx, prompt)
	if err == nil {
		return data, nil
	}
	return c.fallback.GenerateImage(ctx, prompt)
}

// NewSceneImageClient builds the image client used for scene art: the configured
// provider, wrapping the built-in generator when the fallback is enabled.
func NewSceneImageClient(cfg config.ImageConfig) (ImageClient, error) {
	return NewSceneImageClientWithSharedKey(cfg, "")
}

func NewSceneImageClientWithSharedKey(cfg config.ImageConfig, sharedKey string) (ImageClient, error) {
	primary, err := NewImageClientWithSharedKey(cfg, sharedKey)
	if err != nil {
		return nil, err
	}
	if !cfg.BuiltinFallback {
		return primary, nil
	}

	fallback, err := NewImageClient(config.ImageConfig{Type: "builtin", BuiltinName: "procedural-art"})
	if err != nil {
		return nil, err
	}
	return &fallbackImageClient{primary: primary, fallback: fallback}, nil
}

func NewImageClient(cfg config.ImageConfig) (ImageClient, error) {
	return NewImageClientWithSharedKey(cfg, "")
}

func NewImageClientWithSharedKey(cfg config.ImageConfig, sharedKey string) (ImageClient, error) {
	// Registry-first when pkg/provider/all was imported; inline otherwise.
	if regID := ImageProviderIDFor(cfg); regID != "" {
		if client, err := BuildImage(regID, cfg, sharedKey); err == nil {
			return client, nil
		}
	}
	switch cfg.Type {
	case "disabled", "":
		return &disabledImageClient{}, nil
	case "builtin":
		if cfg.BuiltinName == "procedural-art" {
			return NewProceduralArtClient(), nil
		}
		if cfg.BuiltinName == "gemini" {
			return NewGeminiImageClient(cfg, sharedKey)
		}
		return &echoImageClient{}, nil
	case "gemini":
		return NewGeminiImageClient(cfg, sharedKey)
	case "cli":
		return &cliImageClient{command: cfg.Command, args: cfg.Args}, nil
	case "comfyui":
		return &comfyUIImageClient{
			endpoint: cfg.Endpoint,
			client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 120 * time.Second},
		}, nil
	case "http":
		if isComfyUI(cfg.Endpoint) {
			return &comfyUIImageClient{
				endpoint: cfg.Endpoint,
				client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 120 * time.Second},
			}, nil
		}
		return &httpImageClient{endpoint: cfg.Endpoint, model: cfg.Model, apiKey: cfg.APIKey, client: &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 60 * time.Second}}, nil
	default:
		return nil, fmt.Errorf("unsupported image provider type: %s", cfg.Type)
	}
}
