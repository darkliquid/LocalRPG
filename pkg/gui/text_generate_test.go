package gui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestBuildTextGeneratorPrompt_AllFields(t *testing.T) {
	req := GenerateTextRequest{
		FormType:  "world",
		FieldName: "_all",
		Context: map[string]string{
			"genre": "Dark Fantasy",
		},
	}

	prompt := buildTextGeneratorPrompt(req, nil)

	if !strings.Contains(prompt, "Generate values for the following fields:") {
		t.Errorf("expected prompt to instruct generating all fields, got %q", prompt)
	}
	if !strings.Contains(prompt, "genre: Dark Fantasy") {
		t.Errorf("expected prompt to contain context, got %q", prompt)
	}
	if !strings.Contains(prompt, "- name") || !strings.Contains(prompt, "- description") {
		t.Errorf("expected prompt to list default world fields, got %q", prompt)
	}
}

func TestBuildTextGeneratorPrompt_SingleFieldWithSeed(t *testing.T) {
	req := GenerateTextRequest{
		FormType:  "system",
		FieldName: "rules_prompt",
		Seed:      "Use 2d6 rolling over a target difficulty",
		Context: map[string]string{
			"name": "Narrative 2d6",
		},
	}

	prompt := buildTextGeneratorPrompt(req, nil)

	if !strings.Contains(prompt, "Generate a value for the 'rules_prompt' field.") {
		t.Errorf("expected single-field instruction, got %q", prompt)
	}
	if !strings.Contains(prompt, "Use 2d6 rolling over a target difficulty") {
		t.Errorf("expected seed in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "name: Narrative 2d6") {
		t.Errorf("expected context in prompt, got %q", prompt)
	}
}

func TestBuildTextGeneratorPrompt_CharacterFields(t *testing.T) {
	customFields := []core.CharacterCreationField{
		{ID: "hometown", Label: "Hometown", Prompt: "Where they grew up", Generatable: true},
	}
	req := GenerateTextRequest{
		FormType:  "character",
		FieldName: "_all",
		Context: map[string]string{
			"name": "Elena",
		},
	}

	prompt := buildTextGeneratorPrompt(req, customFields)

	if !strings.Contains(prompt, "- hometown (Hometown): Where they grew up") {
		t.Errorf("expected custom character field in prompt, got %q", prompt)
	}
}

func TestHandleGenerateTextRoute_InvalidMethod(t *testing.T) {
	_, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/generate-text", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed, got %d", rec.Code)
	}
}

func TestHandleGenerateTextRoute_EmptyBody(t *testing.T) {
	_, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/generate-text", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with empty response, got %d: %s", rec.Code, rec.Body.String())
	}
}
