package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// handleCharacterGenerateRoute serves POST /api/character/generate. It creates
// nothing; the caller decides whether to keep the values.
func (s *Server) handleCharacterGenerateRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req GenerateCharacterRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	resp, err := s.service.GenerateCharacter(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

// GenerateCharacterRequest asks for starter values for a system's character
// creation fields. It is side-effect free: no campaign is created and no files
// are written.
type GenerateCharacterRequest struct {
	SystemID string                        `json:"system_id"`
	WorldID  string                        `json:"world_id"`
	Name     string                        `json:"name"`
	Fields   []core.CharacterCreationField `json:"fields"`
	Seed     map[string]string             `json:"seed,omitempty"`
}

type GenerateCharacterResponse struct {
	Values      map[string]string `json:"values"`
	GeneratedBy string            `json:"generated_by"`
}

// GenerateCharacter fills starting values for the fields the client asks about.
// A model failure or an unconfigured provider is reported through GeneratedBy,
// never as an error: the player can always type the answers themselves.
func (s *Service) GenerateCharacter(ctx context.Context, req GenerateCharacterRequest) (*GenerateCharacterResponse, error) {
	resp := &GenerateCharacterResponse{Values: map[string]string{}, GeneratedBy: "none"}

	fields := req.Fields
	if len(fields) == 0 && req.SystemID != "" {
		path := filepath.Join(s.resolver.SystemDir(req.SystemID), "system.yaml")
		if manifest, err := core.LoadSystemManifest(path); err == nil {
			fields = manifest.CharacterCreation.Fields
		}
	}

	generatable := make([]core.CharacterCreationField, 0, len(fields))
	for _, field := range fields {
		// A voice is chosen from the configured profiles, never invented as text.
		if field.Kind == "voice" || !field.Generatable {
			continue
		}
		generatable = append(generatable, field)
	}
	if len(generatable) == 0 {
		return resp, nil
	}

	router, err := harness.RouterFromConfigWithLogger(s.configMgr.Get(), s.logger)
	if err != nil {
		return resp, nil
	}

	request := harness.GenerateRequest{
		System:    characterGeneratorSystemPrompt,
		Prompt:    characterGeneratorPrompt(req, generatable),
		MaxTokens: 700,
	}

	var text string
	for _, role := range []string{"character", "gm"} {
		result, err := router.GenerateForRole(ctx, role, request)
		if err != nil || result == nil || strings.TrimSpace(result.Text) == "" {
			continue
		}
		text = result.Text
		resp.GeneratedBy = role
		break
	}
	if text == "" {
		return resp, nil
	}

	values := decodeGeneratedValues(text)
	for _, field := range generatable {
		if value, ok := values[field.ID]; ok && strings.TrimSpace(value) != "" {
			resp.Values[field.ID] = strings.TrimSpace(value)
		}
	}
	if len(resp.Values) == 0 {
		resp.GeneratedBy = "none"
	}
	return resp, nil
}

const characterGeneratorSystemPrompt = `You invent player characters for a tabletop roleplaying game.
Return one JSON object and nothing else, with a string value for every requested field id.
Write in the second person, concrete and vivid, at most two sentences per field.
Do not add fields that were not requested.`

func characterGeneratorPrompt(req GenerateCharacterRequest, fields []core.CharacterCreationField) string {
	var b strings.Builder
	b.WriteString("Invent a player character.\n")
	if req.Name != "" {
		b.WriteString("Name: " + req.Name + "\n")
	}
	if req.WorldID != "" {
		b.WriteString("World: " + req.WorldID + "\n")
	}
	for _, field := range fields {
		b.WriteString("- " + field.ID + " (" + field.Label + ")")
		if field.Prompt != "" {
			b.WriteString(": " + field.Prompt)
		}
		b.WriteString("\n")
	}
	for id, value := range req.Seed {
		if strings.TrimSpace(value) != "" {
			b.WriteString("Given " + id + ": " + value + "\n")
		}
	}
	b.WriteString("\nReturn a JSON object keyed by those field ids.")
	return b.String()
}

// decodeGeneratedValues reads a model's JSON object, tolerating a fenced block
// and non-string values. Unknown keys are dropped by the caller.
func decodeGeneratedValues(text string) map[string]string {
	cleaned := strings.TrimSpace(text)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	if start := strings.Index(cleaned, "{"); start >= 0 {
		if end := strings.LastIndex(cleaned, "}"); end > start {
			cleaned = cleaned[start : end+1]
		}
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		return map[string]string{}
	}

	values := make(map[string]string, len(raw))
	for key, value := range raw {
		switch typed := value.(type) {
		case string:
			values[key] = typed
		case float64:
			values[key] = fmt.Sprintf("%v", typed)
		case bool:
			values[key] = fmt.Sprintf("%v", typed)
		}
	}
	return values
}

// ErrNoDecodableFields reports that a model returned text from which no field
// values could be decoded. It is distinct from an empty reply.
var ErrNoDecodableFields = errors.New("no decodable fields in model response")

// decodeGeneratedValuesChecked is decodeGeneratedValues with the parse failure
// made explicit, so a caller can record parse_error rather than guessing.
func decodeGeneratedValuesChecked(text string) (map[string]string, error) {
	values := decodeGeneratedValues(text)
	if len(values) == 0 {
		return nil, ErrNoDecodableFields
	}
	return values, nil
}
