package media_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
)

func TestListGeminiModelsFollowsPagination(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Errorf("expected api key header, got %q", got)
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = w.Write([]byte(`{
				"models": [
					{"name": "models/gemini-3.8-flash", "displayName": "Gemini 3.8 Flash", "supportedGenerationMethods": ["generateContent"], "thinking": true},
					{"name": "models/gemma-4-31b-it", "displayName": "Gemma 4 31B", "supportedActions": ["generateContent"]}
				],
				"nextPageToken": "page2"
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"models": [
				{"name": "models/gemini-3.1-flash-tts-preview", "displayName": "Gemini 3.1 TTS", "supportedActions": ["generateContent"]}
			]
		}`))
	}))
	defer server.Close()

	restore := media.GeminiAPIBaseURL
	media.GeminiAPIBaseURL = server.URL
	defer func() { media.GeminiAPIBaseURL = restore }()

	models, err := media.ListGeminiModels(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("ListGeminiModels: %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests for pagination, got %d", requests)
	}
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}
	if models[0].ID != "gemini-3.1-flash-tts-preview" || models[2].ID != "gemma-4-31b-it" {
		t.Errorf("expected sorted, prefix-trimmed IDs, got %+v", models)
	}
	if !models[1].Thinking || models[1].DisplayName != "Gemini 3.8 Flash" {
		t.Errorf("expected thinking and display name to be preserved, got %+v", models[1])
	}
}

func TestListGeminiVoicesMapsFieldsAndFilters(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/voices" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"voices": [
				{
					"id": "voice_abc",
					"displayName": "Mexican Storyteller",
					"description": "A warm narrator",
					"languageCode": "es-MX",
					"regionCode": "MX",
					"gender": "female",
					"accent": "Mexican",
					"context": "Audiobook",
					"persona": "Narrator",
					"type": "prebuilt"
				}
			]
		}`))
	}))
	defer server.Close()

	restore := media.GeminiAPIBaseURL
	media.GeminiAPIBaseURL = server.URL
	defer func() { media.GeminiAPIBaseURL = restore }()

	voices, err := media.ListGeminiVoices(context.Background(), "test-key", media.GeminiVoiceSearch{
		Query:        "storyteller",
		LanguageCode: "es-MX",
		Gender:       "female",
	})
	if err != nil {
		t.Fatalf("ListGeminiVoices: %v", err)
	}
	if got := gotQuery.Get("type"); got != "prebuilt" {
		t.Errorf("expected default type=prebuilt, got %q", got)
	}
	if got := gotQuery.Get("search"); got != "storyteller" {
		t.Errorf("expected search param, got %q", got)
	}
	if got := gotQuery.Get("language_code"); got != "es-MX" {
		t.Errorf("expected language_code param, got %q", got)
	}
	if len(voices) != 1 {
		t.Fatalf("expected 1 voice, got %d", len(voices))
	}
	voice := voices[0]
	if voice.ID != "voice_abc" || voice.Name != "Mexican Storyteller" {
		t.Errorf("unexpected voice identity: %+v", voice)
	}
	if voice.Gender != "female" || voice.Accent != "Mexican" || voice.Language != "es-MX" {
		t.Errorf("expected gender/accent/language mapped, got %+v", voice)
	}
	if len(voice.Tags) != 4 {
		t.Errorf("expected gender/accent/persona/context tags, got %v", voice.Tags)
	}
	if voice.Metadata["persona"] != "Narrator" || voice.Metadata["region_code"] != "MX" {
		t.Errorf("expected metadata mapped, got %v", voice.Metadata)
	}
}

func TestListGeminiVoicesRequiresKey(t *testing.T) {
	_, err := media.ListGeminiVoices(context.Background(), "", media.GeminiVoiceSearch{})
	if err == nil {
		t.Fatal("expected an error when no API key is supplied")
	}
}

func TestGeminiVoiceFallsBackToDisplayNameForID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"voices": []map[string]any{
				{"displayName": "No ID Voice", "type": "prebuilt"},
			},
		})
	}))
	defer server.Close()

	restore := media.GeminiAPIBaseURL
	media.GeminiAPIBaseURL = server.URL
	defer func() { media.GeminiAPIBaseURL = restore }()

	voices, err := media.ListGeminiVoices(context.Background(), "test-key", media.GeminiVoiceSearch{})
	if err != nil {
		t.Fatalf("ListGeminiVoices: %v", err)
	}
	if len(voices) != 1 || voices[0].ID != "No ID Voice" || voices[0].Name != "No ID Voice" {
		t.Fatalf("expected display name to stand in for ID, got %+v", voices)
	}
}
