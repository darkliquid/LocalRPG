package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// catalogHTTPClient is the outbound client for catalogue REST calls, wrapped so
// the requests become client spans.
var catalogHTTPClient = &http.Client{Transport: telemetry.HTTPTransport(nil)}

// GeminiAPIBaseURL is the public Gemini API host. The model and voice
// catalogues are plain JSON REST endpoints, so they are called directly rather
// than through the genai client (the pinned SDK has no voices surface). It is a
// variable so a test can point it at a stub server.
var GeminiAPIBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// geminiCatalogTimeout bounds one catalogue request.
const geminiCatalogTimeout = 20 * time.Second

// GeminiModel is one entry from the models.list catalogue, reduced to the
// fields the configuration editor needs.
type GeminiModel struct {
	ID               string   `json:"id"`
	DisplayName      string   `json:"display_name,omitempty"`
	Description      string   `json:"description,omitempty"`
	SupportedActions []string `json:"supported_actions,omitempty"`
	InputTokenLimit  int      `json:"input_token_limit,omitempty"`
	OutputTokenLimit int      `json:"output_token_limit,omitempty"`
	Thinking         bool     `json:"thinking,omitempty"`
}

type geminiModelsResponse struct {
	Models []struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
		// The wire field is supportedGenerationMethods; supportedActions is a
		// newer spelling, so accept either.
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		SupportedActions           []string `json:"supportedActions"`
		InputTokenLimit            int      `json:"inputTokenLimit"`
		OutputTokenLimit           int      `json:"outputTokenLimit"`
		Thinking                   bool     `json:"thinking"`
	} `json:"models"`
	NextPageToken string `json:"nextPageToken"`
}

// ListGeminiModels enumerates every model the supplied key can reach, so the
// editor lists the live catalogue (Gemini 3, Gemma, and whatever else the
// account has) instead of a hardcoded snapshot. It follows pagination.
func ListGeminiModels(ctx context.Context, apiKey string) ([]GeminiModel, error) {
	key, err := requireGeminiKey(apiKey)
	if err != nil {
		return nil, err
	}

	var models []GeminiModel
	pageToken := ""
	for {
		query := url.Values{"pageSize": {"1000"}}
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		var payload geminiModelsResponse
		if err := geminiGET(ctx, "/models", key, query, &payload); err != nil {
			return nil, err
		}
		for _, m := range payload.Models {
			actions := m.SupportedGenerationMethods
			if len(actions) == 0 {
				actions = m.SupportedActions
			}
			models = append(models, GeminiModel{
				ID:               strings.TrimPrefix(m.Name, "models/"),
				DisplayName:      m.DisplayName,
				Description:      m.Description,
				SupportedActions: actions,
				InputTokenLimit:  m.InputTokenLimit,
				OutputTokenLimit: m.OutputTokenLimit,
				Thinking:         m.Thinking,
			})
		}
		if payload.NextPageToken == "" {
			break
		}
		pageToken = payload.NextPageToken
	}

	sort.SliceStable(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

// GeminiVoiceSearch narrows one call to the extended voice library.
type GeminiVoiceSearch struct {
	// Query is a free-text substring matched against a voice's display name and
	// description.
	Query string
	// VoiceType filters by origin: "prebuilt", "replicated", or "prompted".
	// Empty means the provider default of "prebuilt".
	VoiceType    string
	LanguageCode string
	Gender       string
	Accent       string
	Persona      string
	PageSize     int
}

type geminiVoiceEntry struct {
	ID           string `json:"id"`
	DisplayName  string `json:"displayName"`
	Description  string `json:"description"`
	LanguageCode string `json:"languageCode"`
	RegionCode   string `json:"regionCode"`
	Gender       string `json:"gender"`
	Accent       string `json:"accent"`
	Context      string `json:"context"`
	Persona      string `json:"persona"`
	Type         string `json:"type"`
}

type geminiVoicesResponse struct {
	Voices        []geminiVoiceEntry `json:"voices"`
	NextPageToken string             `json:"nextPageToken"`
}

// ListGeminiVoices queries the extended Gemini voice library. The library holds
// thousands of voices on top of the 30 studio prebuilt voices, and can be
// searched by name/description or filtered by language, gender, accent, and
// persona.
func ListGeminiVoices(ctx context.Context, apiKey string, search GeminiVoiceSearch) ([]ProviderVoice, error) {
	key, err := requireGeminiKey(apiKey)
	if err != nil {
		return nil, err
	}

	voiceType := strings.ToLower(strings.TrimSpace(search.VoiceType))
	if voiceType == "" {
		voiceType = "prebuilt"
	}
	pageSize := search.PageSize
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 1000
	}

	var voices []ProviderVoice
	pageToken := ""
	for {
		query := url.Values{
			"type":     {voiceType},
			"pageSize": {strconv.Itoa(pageSize)},
		}
		if search.Query != "" {
			query.Set("search", search.Query)
		}
		if search.LanguageCode != "" {
			query.Set("language_code", search.LanguageCode)
		}
		if search.Gender != "" {
			query.Set("gender", search.Gender)
		}
		if search.Accent != "" {
			query.Set("accent", search.Accent)
		}
		if search.Persona != "" {
			query.Set("persona", search.Persona)
		}
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}

		var payload geminiVoicesResponse
		if err := geminiGET(ctx, "/voices", key, query, &payload); err != nil {
			return nil, err
		}
		for _, v := range payload.Voices {
			voices = append(voices, geminiVoiceToProviderVoice(v))
		}
		if payload.NextPageToken == "" {
			break
		}
		pageToken = payload.NextPageToken
	}

	return voices, nil
}

func geminiVoiceToProviderVoice(v geminiVoiceEntry) ProviderVoice {
	id := strings.TrimSpace(v.ID)
	if id == "" {
		id = strings.TrimSpace(v.DisplayName)
	}
	name := strings.TrimSpace(v.DisplayName)
	if name == "" {
		name = id
	}

	tags := make([]string, 0, 4)
	for _, value := range []string{v.Gender, v.Accent, v.Persona, v.Context} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			tags = append(tags, strings.ToLower(trimmed))
		}
	}

	var categories []string
	if context := strings.TrimSpace(v.Context); context != "" {
		categories = append(categories, context)
	}

	metadata := map[string]interface{}{}
	for key, value := range map[string]string{
		"type":          v.Type,
		"region_code":   v.RegionCode,
		"persona":       v.Persona,
		"context":       v.Context,
		"language_code": v.LanguageCode,
	} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			metadata[key] = trimmed
		}
	}

	return ProviderVoice{
		ID:          id,
		Name:        name,
		Language:    v.LanguageCode,
		Gender:      v.Gender,
		Accent:      v.Accent,
		Categories:  categories,
		Tags:        tags,
		Description: v.Description,
		Metadata:    metadata,
	}
}

func requireGeminiKey(apiKey string) (string, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return "", ErrGeminiTTSAPIKeyRequired
	}
	return key, nil
}

// geminiGET issues one authenticated GET against the Gemini API and decodes the
// JSON body into out.
func geminiGET(ctx context.Context, path, apiKey string, query url.Values, out interface{}) error {
	ctx, cancel := context.WithTimeout(ctx, geminiCatalogTimeout)
	defer cancel()

	endpoint := GeminiAPIBaseURL + path
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("gemini: build %s request: %w", path, err)
	}
	req.Header.Set("x-goog-api-key", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := catalogHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("gemini: %s request failed: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("gemini: read %s response: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gemini: %s returned %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("gemini: decode %s response: %w", path, err)
	}
	return nil
}
