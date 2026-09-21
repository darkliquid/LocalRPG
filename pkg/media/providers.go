package media

import (
	"bytes"
	"context"
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
	payloadMap := map[string]interface{}{
		"model": h.model,
		"input": text,
		"voice": voiceID,
	}
	if voice != nil && voice.SpeechRate > 0 {
		payloadMap["speed"] = voice.SpeechRate
	}
	payload, _ := json.Marshal(payloadMap)
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
		return nil, fmt.Errorf("http tts failed (%d): %s", resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

type httpImageClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

func (h *httpImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	payload, _ := json.Marshal(map[string]interface{}{
		"model":  h.model,
		"prompt": prompt,
		"n":      1,
		"size":   "512x512",
	})
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
	return io.ReadAll(resp.Body)
}

// Factory Constructors
func NewTTSClient(cfg config.TTSConfig) (TTSClient, error) {
	switch cfg.Type {
	case "disabled", "":
		return &disabledTTSClient{}, nil
	case "builtin":
		if cfg.BuiltinName == "native-os" {
			return NewNativeOSTTSClient(), nil
		}
		return &echoTTSClient{}, nil
	case "cli":
		return &cliTTSClient{command: cfg.Command, args: cfg.Args}, nil
	case "http":
		return &httpTTSClient{endpoint: cfg.Endpoint, model: cfg.Model, apiKey: cfg.APIKey, client: &http.Client{Timeout: 30 * time.Second}}, nil
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
			client:   &http.Client{Timeout: 60 * time.Second},
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
	primary, err := NewImageClient(cfg)
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
	switch cfg.Type {
	case "disabled", "":
		return &disabledImageClient{}, nil
	case "builtin":
		if cfg.BuiltinName == "procedural-art" {
			return NewProceduralArtClient(), nil
		}
		return &echoImageClient{}, nil
	case "cli":
		return &cliImageClient{command: cfg.Command, args: cfg.Args}, nil
	case "http":
		return &httpImageClient{endpoint: cfg.Endpoint, model: cfg.Model, apiKey: cfg.APIKey, client: &http.Client{Timeout: 60 * time.Second}}, nil
	default:
		return nil, fmt.Errorf("unsupported image provider type: %s", cfg.Type)
	}
}
