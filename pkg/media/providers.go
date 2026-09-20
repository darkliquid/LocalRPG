package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
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
	payload, _ := json.Marshal(map[string]interface{}{
		"model": h.model,
		"input": text,
		"voice": voiceID,
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
		return &echoTTSClient{}, nil
	case "cli":
		return &cliTTSClient{command: cfg.Command, args: cfg.Args}, nil
	case "http":
		return &httpTTSClient{endpoint: cfg.Endpoint, model: cfg.Model, apiKey: cfg.APIKey, client: &http.Client{Timeout: 30 * time.Second}}, nil
	default:
		return nil, fmt.Errorf("unsupported tts provider type: %s", cfg.Type)
	}
}

func NewSTTClient(cfg config.STTConfig) (STTClient, error) {
	switch cfg.Type {
	case "disabled", "":
		return &disabledSTTClient{}, nil
	case "builtin":
		return &echoSTTClient{}, nil
	case "cli":
		return &cliSTTClient{command: cfg.Command, args: cfg.Args}, nil
	default:
		return nil, fmt.Errorf("unsupported stt provider type: %s", cfg.Type)
	}
}

func NewImageClient(cfg config.ImageConfig) (ImageClient, error) {
	switch cfg.Type {
	case "disabled", "":
		return &disabledImageClient{}, nil
	case "builtin":
		return &echoImageClient{}, nil
	case "cli":
		return &cliImageClient{command: cfg.Command, args: cfg.Args}, nil
	case "http":
		return &httpImageClient{endpoint: cfg.Endpoint, model: cfg.Model, apiKey: cfg.APIKey, client: &http.Client{Timeout: 60 * time.Second}}, nil
	default:
		return nil, fmt.Errorf("unsupported image provider type: %s", cfg.Type)
	}
}
