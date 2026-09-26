package gui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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
	Fields      map[string]string          `json:"fields"`
	GeneratedBy string                     `json:"generated_by"`
	Warning     *harness.GenerationFailure `json:"warning,omitempty"`
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

// GenerateText fills one field or a whole form's worth of values. A failure is
// returned as a *harness.GenerationFailure: an unconfigured provider, a provider
// error, an empty reply, or an unparseable reply. A partial success is a 200 with
// Warning set. The user can always type the answers themselves.
func (s *Service) GenerateText(ctx context.Context, req GenerateTextRequest) (*GenerateTextResponse, error) {
	started := time.Now()
	ctx, span := startGenerationSpan(ctx, s.logger, "generate.text", req.FormType, req.FieldName)
	defer span.End()

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

	router, err := textRouterFactory(s.configMgr.Get(), s.logger)
	if err != nil {
		failure := &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: fmt.Sprintf("no model provider is configured: %v", err),
		}
		s.recordGeneration(ctx, span, req.FormType, "", started, failure)
		return nil, failure
	}

	request := harness.GenerateRequest{
		System:    systemPrompt,
		Prompt:    buildTextGeneratorPrompt(req, systemFields),
		MaxTokens: 1000,
	}

	roles := []string{role}
	if role != "gm" {
		roles = append(roles, "gm")
	}

	outcome := collectTextAttempts(ctx, router, roles, request)
	attempts := outcome.Attempts
	resp.GeneratedBy = outcome.GeneratedBy
	for k, v := range outcome.Values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			resp.Fields[k] = trimmed
		}
	}

	s.recordGenerationAttempts(ctx, span, attempts)

	if len(resp.Fields) == 0 {
		resp.GeneratedBy = "none"
		failure := &harness.GenerationFailure{
			Code:        pickFailureCode(attempts),
			Message:     harness.SummarizeAttempts(attempts, "the model did not return any usable text"),
			Attempts:    attempts,
			PromptChars: len([]rune(request.PromptText())),
			ElapsedMS:   time.Since(started).Milliseconds(),
		}
		s.setTextOutcome(span, generationOutcome{Attempts: attempts}, 0)
		s.recordGeneration(ctx, span, req.FormType, roleForAttempts(attempts), started, failure)
		return nil, failure
	}

	if len(attempts) > 0 && missingFields(requestedTextFields(req, systemFields), resp.Fields) > 0 {
		resp.Warning = &harness.GenerationFailure{
			Code:      attempts[len(attempts)-1].Code,
			Message:   "some requested fields were not generated",
			Attempts:  attempts,
			ElapsedMS: time.Since(started).Milliseconds(),
		}
	}
	s.setTextOutcome(span, outcome, len(resp.Fields))
	s.recordGeneration(ctx, span, req.FormType, outcome.GeneratedBy, started, nil)
	return resp, nil
}

// pickFailureCode chooses the most informative code from a fallback chain. An
// empty chain means no role could even be built.
func pickFailureCode(attempts []harness.Attempt) harness.FailureCode {
	if len(attempts) == 0 {
		return harness.FailureProviderUnavailable
	}
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].Code != "" {
			return attempts[i].Code
		}
	}
	return harness.FailureProviderError
}

// requestedTextFields lists the field ids a request asks for, so a fallback that
// filled every field does not raise a partial warning.
func requestedTextFields(req GenerateTextRequest, systemFields []core.CharacterCreationField) []string {
	if req.FieldName != "_all" {
		return []string{req.FieldName}
	}
	switch req.FormType {
	case "character":
		ids := []string{"name", "age", "gender", "pronouns", "appearance", "background"}
		for _, f := range systemFields {
			ids = append(ids, f.ID)
		}
		return ids
	case "world":
		return []string{"name", "description", "genre", "art_style", "lore_prompt"}
	case "system":
		return []string{"name", "description", "rules_prompt"}
	case "campaign":
		return []string{"name", "start_location", "opening_prompt"}
	}
	return nil
}

// missingFields counts requested field ids absent from a decoded result.
func missingFields(requested []string, got map[string]string) int {
	missing := 0
	for _, id := range requested {
		if _, ok := got[id]; !ok {
			missing++
		}
	}
	return missing
}
