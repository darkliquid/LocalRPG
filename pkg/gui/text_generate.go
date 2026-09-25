package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// GenerateTextRequest asks the model for one field or a whole form's worth of
// values. It is side-effect free: nothing is written to disk.
type GenerateTextRequest struct {
	FormType  string            `json:"form_type"` // "character", "world", "system", "campaign"
	FieldName string            `json:"field_name"` // field id or "_all"
	Context   map[string]string `json:"context"`
	WorldID   string            `json:"world_id,omitempty"`
	SystemID  string            `json:"system_id,omitempty"`
	Seed      string            `json:"seed,omitempty"`
}

type GenerateTextResponse struct {
	Fields      map[string]string `json:"fields"`
	GeneratedBy string            `json:"generated_by"`
}

const (
	textGeneratorSystemPromptCharacter = `You invent player characters for a tabletop roleplaying game.
Return one JSON object and nothing else, with a string value for every requested field.
Write in the second person, concrete and vivid, at most two sentences per field.
Do not add fields that were not requested.`

	textGeneratorSystemPromptWorld = `You are a creative world builder for a tabletop roleplaying game.
Return one JSON object and nothing else, with a string value for every requested field.
Write evocative, vivid text. Do not add fields that were not requested.`

	textGeneratorSystemPromptSystem = `You are a technical game system designer for a tabletop roleplaying game.
Return one JSON object and nothing else, with a string value for every requested field.
Write clear, concise technical text. Do not add fields that were not requested.`

	textGeneratorSystemPromptCampaign = `You are a game master planning a tabletop roleplaying game campaign.
Return one JSON object and nothing else, with a string value for every requested field.
Write engaging, atmospheric text. Do not add fields that were not requested.`
)

func buildTextGeneratorPrompt(req GenerateTextRequest, systemFields []core.CharacterCreationField) string {
	var b strings.Builder

	if req.FieldName == "_all" {
		b.WriteString("Generate values for the following fields:\n")
	} else {
		b.WriteString(fmt.Sprintf("Generate a value for the '%s' field.\n", req.FieldName))
	}

	if len(req.Context) > 0 {
		b.WriteString("\nContext (use this to ensure consistency):\n")
		for k, v := range req.Context {
			trimmed := strings.TrimSpace(v)
			if trimmed != "" {
				b.WriteString(fmt.Sprintf("- %s: %s\n", k, trimmed))
			}
		}
	}

	b.WriteString("\nRequested Fields:\n")

	if req.FormType == "character" {
		standardFields := []core.CharacterCreationField{
			{ID: "name", Label: "Character Name"},
			{ID: "age", Label: "Age"},
			{ID: "gender", Label: "Gender"},
			{ID: "pronouns", Label: "Pronouns"},
			{ID: "appearance", Label: "Physical Appearance"},
			{ID: "background", Label: "Background / Backstory"},
		}
		allFields := append(standardFields, systemFields...)
		for _, f := range allFields {
			if req.FieldName == "_all" || req.FieldName == f.ID {
				b.WriteString(fmt.Sprintf("- %s (%s)", f.ID, f.Label))
				if f.Prompt != "" {
					b.WriteString(": " + f.Prompt)
				}
				b.WriteString("\n")
			}
		}
	} else {
		var fields []string
		switch req.FormType {
		case "world":
			fields = []string{"name", "description", "genre", "art_style", "lore_prompt"}
		case "system":
			fields = []string{"name", "description", "rules_prompt"}
		case "campaign":
			fields = []string{"name", "start_location", "opening_prompt"}
		}

		for _, f := range fields {
			if req.FieldName == "_all" || req.FieldName == f {
				b.WriteString(fmt.Sprintf("- %s\n", f))
			}
		}
	}

	if strings.TrimSpace(req.Seed) != "" {
		b.WriteString("\nSeed/Existing Text (extend or improve this):\n" + strings.TrimSpace(req.Seed) + "\n")
	}

	b.WriteString("\nReturn a JSON object keyed by the requested field names.")
	return b.String()
}

// GenerateText fills one field or a whole form's worth of values. A model
// failure or an unconfigured provider is reported through GeneratedBy, never as
// an error: the user can always type the answers themselves.
func (s *Service) GenerateText(ctx context.Context, req GenerateTextRequest) (*GenerateTextResponse, error) {
	resp := &GenerateTextResponse{Fields: map[string]string{}, GeneratedBy: "none"}

	systemPrompt := textGeneratorSystemPromptWorld
	role := "gm"
	var systemFields []core.CharacterCreationField

	switch req.FormType {
	case "character":
		systemPrompt = textGeneratorSystemPromptCharacter
		role = "character"
		if req.SystemID != "" {
			path := filepath.Join(s.resolver.SystemDir(req.SystemID), "system.yaml")
			if manifest, err := core.LoadSystemManifest(path); err == nil {
				for _, f := range manifest.CharacterCreation.Fields {
					if f.Kind != "voice" && f.Generatable {
						systemFields = append(systemFields, f)
					}
				}
			}
		}
	case "system":
		systemPrompt = textGeneratorSystemPromptSystem
	case "campaign":
		systemPrompt = textGeneratorSystemPromptCampaign
	}

	router, err := harness.RouterFromConfigWithLogger(s.configMgr.Get(), s.logger)
	if err != nil {
		return resp, nil
	}

	request := harness.GenerateRequest{
		System:    systemPrompt,
		Prompt:    buildTextGeneratorPrompt(req, systemFields),
		MaxTokens: 1000,
	}

	for _, tryRole := range []string{role, "gm"} {
		result, err := router.GenerateForRole(ctx, tryRole, request)
		if err != nil || result == nil || strings.TrimSpace(result.Text) == "" {
			continue
		}
		resp.GeneratedBy = tryRole
		values := decodeGeneratedValues(result.Text)
		for k, v := range values {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				resp.Fields[k] = trimmed
			}
		}
		break
	}

	if len(resp.Fields) == 0 {
		resp.GeneratedBy = "none"
	}
	return resp, nil
}

// handleGenerateTextRoute serves POST /api/generate-text. It creates nothing;
// the caller decides whether to keep the returned values.
func (s *Server) handleGenerateTextRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req GenerateTextRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	resp, err := s.service.GenerateText(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}
