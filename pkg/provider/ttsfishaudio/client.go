package ttsfishaudio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// FishAudioTTSClient interacts with Fish Audio S2 Pro served via vLLM-Omni.
type FishAudioTTSClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

// NewFishAudioTTSClient constructs a new FishAudioTTSClient.
func NewFishAudioTTSClient(cfg config.TTSConfig) *FishAudioTTSClient {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		endpoint = "http://localhost:8091"
	}
	model := cfg.Model
	if model == "" {
		model = "fishaudio/s2-pro"
	}

	return &FishAudioTTSClient{
		endpoint: endpoint,
		model:    model,
		apiKey:   cfg.APIKey,
		client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 120 * time.Second},
	}
}

// Synthesize sends a speech synthesis request to vLLM-Omni's /v1/audio/speech endpoint.
func (c *FishAudioTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	speechURL := c.endpoint
	if !strings.HasSuffix(speechURL, "/v1/audio/speech") {
		speechURL = c.endpoint + "/v1/audio/speech"
	}

	voiceID := "default"
	if voice != nil && voice.VoiceID != "" {
		voiceID = voice.VoiceID
	}

	payloadMap := map[string]interface{}{
		"model":           c.model,
		"input":           text,
		"voice":           voiceID,
		"response_format": "wav",
	}

	if voice != nil && voice.SpeechRate > 0 && voice.SpeechRate != 1.0 {
		payloadMap["speed"] = voice.SpeechRate
	}

	if voice != nil && len(voice.Options) > 0 {
		if refAudioRaw, ok := voice.Options["ref_audio"]; ok {
			if refAudioStr, isStr := refAudioRaw.(string); isStr && strings.TrimSpace(refAudioStr) != "" {
				encodedAudio, err := resolveReferenceAudio(refAudioStr)
				if err != nil {
					return nil, fmt.Errorf("fish-audio: resolve reference audio: %w", err)
				}
				payloadMap["ref_audio"] = encodedAudio
			}
		}
		if refTextRaw, ok := voice.Options["ref_text"]; ok {
			if refTextStr, isStr := refTextRaw.(string); isStr && strings.TrimSpace(refTextStr) != "" {
				payloadMap["ref_text"] = refTextStr
			}
		}
	}

	bodyBytes, err := json.Marshal(payloadMap)
	if err != nil {
		return nil, fmt.Errorf("fish-audio: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", speechURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("fish-audio: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fish-audio: execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		detailBytes, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return nil, fmt.Errorf("fish-audio tts failed (%d): %s", resp.StatusCode, provider.TruncateDetail(detailBytes))
	}

	return io.ReadAll(resp.Body)
}

func resolveReferenceAudio(ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "data:") {
		return trimmed, nil
	}

	data, err := os.ReadFile(trimmed)
	if err != nil {
		return "", err
	}

	mimeType := "audio/wav"
	ext := strings.ToLower(filepath.Ext(trimmed))
	switch ext {
	case ".mp3":
		mimeType = "audio/mpeg"
	case ".flac":
		mimeType = "audio/flac"
	case ".ogg":
		mimeType = "audio/ogg"
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded), nil
}

// SpeechCueCapabilities advertises bracketed vocal cues support.
func (c *FishAudioTTSClient) SpeechCueCapabilities() media.SpeechCueCapabilities {
	return media.SpeechCueCapabilities{
		AudioTags:        true,
		MarkdownEmphasis: false,
		SupportedTags: []string{
			"whisper", "excited", "angry", "sad", "laugh",
			"sigh", "gasp", "cough", "cry", "screaming", "shouting",
		},
		PromptGuidance: "Use bracketed emotional cues like [whisper] or [excited] directly before dialogue lines to steer delivery and vocal expression.",
	}
}

// VoiceOptions declares provider tunables for reference audio and transcript cloning.
func (c *FishAudioTTSClient) VoiceOptions() []media.VoiceOption {
	return []media.VoiceOption{
		{
			Key:   "ref_audio",
			Label: "Reference Audio (Voice Clone)",
			Kind:  "string",
			Help:  "Local file path (e.g. assets/voices/hero.wav), URL, or base64 data URI for zero-shot cloning.",
		},
		{
			Key:   "ref_text",
			Label: "Reference Audio Transcript",
			Kind:  "string",
			Help:  "Exact transcript of the reference audio clip (required by S2 Pro for voice cloning).",
		},
	}
}

// ListVoices retrieves available voices from vLLM-Omni, with a fallback to the default speaker.
func (c *FishAudioTTSClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error) {
	voicesURL := c.endpoint
	if !strings.HasSuffix(voicesURL, "/v1/audio/voices") {
		voicesURL = c.endpoint + "/v1/audio/voices"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", voicesURL, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil || resp.StatusCode == http.StatusNotFound {
		return defaultFallbackVoices(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return defaultFallbackVoices(), nil
	}

	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return defaultFallbackVoices(), nil
	}

	var envelope struct {
		Voices []string `json:"voices"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.Voices) > 0 {
		out := make([]media.ProviderVoice, len(envelope.Voices))
		for i, v := range envelope.Voices {
			out[i] = media.ProviderVoice{
				ID:       v,
				Name:     v,
				Language: "Multi",
				Tags:     []string{"fish-audio", "s2-pro"},
			}
		}
		return out, nil
	}

	return defaultFallbackVoices(), nil
}

func defaultFallbackVoices() []media.ProviderVoice {
	return []media.ProviderVoice{
		{
			ID:       "default",
			Name:     "Default Speaker (Fish Audio S2)",
			Language: "Multi",
			Tags:     []string{"dual-ar", "44.1khz", "zero-shot-capable"},
			Defaults: map[string]interface{}{"pitch": 1.0, "speech_rate": 1.0},
		},
	}
}

var _ media.TTSClient = (*FishAudioTTSClient)(nil)
var _ media.VoiceCatalog = (*FishAudioTTSClient)(nil)
var _ media.VoiceOptions = (*FishAudioTTSClient)(nil)
var _ media.SpeechCueAdvertiser = (*FishAudioTTSClient)(nil)
