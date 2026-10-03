package inworldstt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/provider/inworldshared"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

const (
	defaultEndpoint = "https://api.inworld.ai/stt/v1/transcribe"
	defaultModel    = "inworld/inworld-stt-1"
	defaultLanguage = "en-US"
	requestTimeout  = 60 * time.Second
)

// InworldSTTClient transcribes audio through Inworld's cloud STT API. It
// implements media.STTClient and media.MeteredProvider.
type InworldSTTClient struct {
	apiKey   string
	model    string
	language string
	endpoint string
	client   *http.Client
	logger   trace.Logger

	usageMu   sync.Mutex
	lastUsage media.Usage
}

// NewInworldSTTClient builds the Inworld STT client, resolving the credential
// from the media config, the shared providers.inworld.api_key, or the
// environment.
func NewInworldSTTClient(cfg config.STTConfig, sharedKey string) (*InworldSTTClient, error) {
	apiKey, err := inworldshared.ResolveAPIKey(cfg.APIKey, sharedKey)
	if err != nil {
		return nil, err
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}

	return &InworldSTTClient{
		apiKey:   apiKey,
		model:    model,
		language: defaultLanguage,
		endpoint: transcribeEndpoint(cfg.Endpoint),
		client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: requestTimeout},
	}, nil
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (c *InworldSTTClient) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

// Metered reports that Inworld STT charges per request.
func (c *InworldSTTClient) Metered() bool { return true }

// LastUsage reports what the last transcription consumed.
func (c *InworldSTTClient) LastUsage() media.Usage {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	return c.lastUsage
}

// Transcribe sends audio to Inworld and returns the transcript. The API takes
// base64 audio with an explicit encoding, so the container is detected and a WAV
// is reduced to the raw 16-bit PCM it wraps.
func (c *InworldSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	if len(audioData) == 0 {
		return "", fmt.Errorf("inworld stt: no audio data")
	}

	encoding, payload := encodeAudio(audioData)

	body, err := json.Marshal(map[string]interface{}{
		"transcribe_config": map[string]interface{}{
			"model_id":       c.model,
			"language":       c.language,
			"audio_encoding": encoding,
		},
		"audio_data": map[string]interface{}{
			"content": base64.StdEncoding.EncodeToString(payload),
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal inworld stt request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create inworld stt request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("inworld stt request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return "", inworldshared.MapError(resp.StatusCode, detail)
	}

	var decoded struct {
		Transcription struct {
			Transcript string `json:"transcript"`
		} `json:"transcription"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode inworld stt response: %w", err)
	}

	c.usageMu.Lock()
	c.lastUsage = media.Usage{Requests: 1}
	c.usageMu.Unlock()

	return decoded.Transcription.Transcript, nil
}

// encodeAudio names the encoding Inworld should expect and returns the bytes to
// send. A WAV is unwrapped to little-endian 16-bit PCM (LINEAR16); Ogg/Opus and
// MP3 are passed through with their encoding named.
func encodeAudio(audioData []byte) (string, []byte) {
	switch {
	case bytes.HasPrefix(audioData, []byte("OggS")):
		return "OGG_OPUS", audioData
	case bytes.HasPrefix(audioData, []byte("ID3")):
		return "MP3", audioData
	case len(audioData) > 1 && audioData[0] == 0xFF && audioData[1]&0xE0 == 0xE0:
		return "MP3", audioData
	case bytes.HasPrefix(audioData, []byte("RIFF")):
		pcm, _, _, err := media.DecodeProviderAudio(audioData, "audio/wav")
		if err != nil || len(pcm) == 0 {
			return "LINEAR16", audioData
		}
		raw := make([]byte, len(pcm)*2)
		for i, sample := range pcm {
			binary.LittleEndian.PutUint16(raw[i*2:], uint16(sample))
		}
		return "LINEAR16", raw
	default:
		return "LINEAR16", audioData
	}
}

// transcribeEndpoint resolves the transcription URL. A configured base with no
// path is given the standard path; a full endpoint is used as given.
func transcribeEndpoint(base string) string {
	trimmed := strings.TrimSpace(base)
	if trimmed == "" {
		return defaultEndpoint
	}
	if parsed, err := url.Parse(trimmed); err != nil || parsed.Path == "" || parsed.Path == "/" {
		return strings.TrimRight(trimmed, "/") + "/stt/v1/transcribe"
	}
	return trimmed
}
