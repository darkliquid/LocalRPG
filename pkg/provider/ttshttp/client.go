package ttshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// NewHTTPTTSClient builds the OpenAI-compatible speech client.
func NewHTTPTTSClient(cfg config.TTSConfig) media.TTSClient {
	return &httpTTSClient{
		endpoint: cfg.Endpoint,
		model:    cfg.Model,
		apiKey:   cfg.APIKey,
		client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 30 * time.Second},
	}
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

var _ media.VoiceCatalog = (*httpTTSClient)(nil)

type kokoroVoiceItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	TargetQuality    string `json:"target_quality,omitempty"`
	TrainingDuration string `json:"training_duration,omitempty"`
	OverallGrade     string `json:"overall_grade,omitempty"`
}

func (h *httpTTSClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error) {
	_, voicesURL := ResolveHTTPEndpoints(h.endpoint)
	if voicesURL == "" {
		return []media.ProviderVoice{}, nil
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
		return []media.ProviderVoice{}, nil
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

	voices := make([]media.ProviderVoice, 0, len(items))
	for _, item := range items {
		voices = append(voices, formatKokoroVoice(item))
	}
	return voices, nil
}

func formatKokoroVoice(item kokoroVoiceItem) media.ProviderVoice {
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

	return media.ProviderVoice{
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

// media.SpeechCueCapabilities advertises Markdown emphasis support for HTTP TTS endpoints.
func (h *httpTTSClient) SpeechCueCapabilities() media.SpeechCueCapabilities {
	return media.SpeechCueCapabilities{
		AudioTags:        false,
		MarkdownEmphasis: true,
	}
}
