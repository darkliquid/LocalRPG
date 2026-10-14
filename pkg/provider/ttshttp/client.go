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
	"github.com/darkliquid/localrpg/pkg/provider"
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
		payload, _ = json.Marshal(map[string]any{
			"text_input":          text,
			"character_voice_gen": voiceID,
			"narrator_voice_gen":  voiceID,
			"text_filtering":      "standard",
			"language":            "en",
		})
	} else {
		isKokoro := strings.Contains(strings.ToLower(h.model), "kokoro") || strings.Contains(h.endpoint, "8880")
		payloadMap := map[string]any{
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
		b, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return nil, fmt.Errorf("http tts failed (%d): %s", resp.StatusCode, provider.TruncateDetail(b))
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
		b, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return nil, fmt.Errorf("http voices failed (%d): %s", resp.StatusCode, provider.TruncateDetail(b))
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
		var langTag, genderTag string
		lang, accent, langTag = kokoroLanguage(parts[0][0])
		gender, genderTag = kokoroGender(parts[0][1])
		if langTag != "" {
			tags = append(tags, langTag)
		}
		if genderTag != "" {
			tags = append(tags, genderTag)
		}

		name = kokoroDisplayName(id, name, parts[1], accent, gender)
		tags = append(tags, strings.ToLower(parts[1]))
		tags = append(tags, "kokoro")
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
		Defaults: map[string]any{
			"pitch":       1.0,
			"speech_rate": 1.0,
		},
	}
}

// kokoroLanguage is the language, accent and search tag a Kokoro voice id's first letter
// names. Kokoro encodes the language and the gender in a two-letter prefix, so the id is
// the only description its catalogue gives.
func kokoroLanguage(code byte) (lang, accent, tag string) {
	switch code {
	case 'a':
		return "en-US", "American", "american"
	case 'b':
		return "en-GB", "British", "british"
	case 'e':
		return "es", "Spanish", "spanish"
	case 'f':
		return "fr", "French", "french"
	case 'h':
		return "hi", "Hindi", "hindi"
	case 'i':
		return "it", "Italian", "italian"
	case 'j':
		return "ja", "Japanese", "japanese"
	case 'p':
		return "pt-BR", "Portuguese", "portuguese"
	case 'z':
		return "zh", "Chinese", "chinese"
	}
	return "", "", ""
}

// kokoroGender is the gender a Kokoro voice id's second letter names, and the tag a search
// matches it by.
func kokoroGender(code byte) (gender, tag string) {
	switch code {
	case 'f':
		return "female", "female"
	case 'm':
		return "male", "male"
	}
	return "", ""
}

// kokoroDisplayName is what a voice is shown as. A catalogue name that is only the id says
// nothing, so it is replaced with the voice's own name title-cased, with its accent and
// gender when the id named both.
func kokoroDisplayName(id, name, voiceName, accent, gender string) string {
	if name != id && name != voiceName {
		return name
	}
	displayName := strings.ToUpper(voiceName[:1]) + voiceName[1:]
	if accent != "" && gender != "" {
		displayName = fmt.Sprintf("%s (%s %s)", displayName, accent, strings.ToUpper(gender[:1])+gender[1:])
	}
	return displayName
}

// media.SpeechCueCapabilities advertises Markdown emphasis support for HTTP TTS endpoints.
func (h *httpTTSClient) SpeechCueCapabilities() media.SpeechCueCapabilities {
	return media.SpeechCueCapabilities{
		AudioTags:        false,
		MarkdownEmphasis: true,
	}
}
