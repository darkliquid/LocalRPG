package gui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestDecodeGeneratedValues(t *testing.T) {
	cases := map[string]map[string]string{
		`{"appearance":"tall","age":34}`:                   {"appearance": "tall", "age": "34"},
		"```json\n{\"appearance\": \"tall\"}\n```":         {"appearance": "tall"},
		"Here you go:\n{\"appearance\":\"tall\"}\nEnjoy!":  {"appearance": "tall"},
		`not json at all`:                                  {},
		`{"appearance":"tall","nested":{"a":1},"ok":true}`: {"appearance": "tall", "ok": "true"},
	}

	for in, want := range cases {
		got := decodeGeneratedValues(in)
		for key, value := range want {
			if got[key] != value {
				t.Errorf("decodeGeneratedValues(%q)[%q] = %q, want %q", in, key, got[key], value)
			}
		}
		if len(got) != len(want) {
			t.Errorf("decodeGeneratedValues(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCharacterGenerateRouteIsSideEffectFree(t *testing.T) {
	_, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	body := `{"system_id":"freeform","name":"Elena","fields":[{"id":"appearance","label":"Appearance","kind":"long","generatable":true}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/character/generate", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		var resp GenerateCharacterResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.GeneratedBy == "" {
			t.Errorf("expected a generated_by marker, got %+v", resp)
		}
	} else {
		var body struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failure body is not a JSON error: %v (%s)", err, rec.Body.String())
		}
		if body.Error.Code == "" {
			t.Fatalf("failure body is missing a code: %s", rec.Body.String())
		}
	}

	// The route must not invent a campaign, whether it generated or failed.
	games, err := svc.ListGames(context.Background())
	if err != nil {
		t.Fatalf("ListGames failed: %v", err)
	}
	if len(games) != 1 {
		t.Errorf("expected only the setup campaign, got %d", len(games))
	}
}

func TestCharacterGenerateSkipsVoiceFields(t *testing.T) {
	_, svc := setupTestGame(t)

	resp, err := svc.GenerateCharacter(context.Background(), GenerateCharacterRequest{
		Fields: []core.CharacterCreationField{
			{ID: "voice", Label: "Voice", Kind: "voice", Generatable: true},
		},
	})
	if err != nil {
		t.Fatalf("GenerateCharacter failed: %v", err)
	}
	if len(resp.Values) != 0 || resp.GeneratedBy != "none" {
		t.Errorf("expected no generation for a voice-only request, got %+v", resp)
	}
}
