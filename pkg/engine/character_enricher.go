package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// RoleGenerator abstracts LLM generation per role.
type RoleGenerator interface {
	GenerateForRole(ctx context.Context, role string, req harness.GenerateRequest) (*harness.GenerateResponse, error)
}

// CharacterEnricher enriches character entities missing demographic and appearance attributes.
type CharacterEnricher struct {
	router RoleGenerator
}

// NewCharacterEnricher creates a new CharacterEnricher.
func NewCharacterEnricher(router RoleGenerator) *CharacterEnricher {
	return &CharacterEnricher{router: router}
}

type characterEnrichmentResult struct {
	Gender     string `json:"gender"`
	Age        string `json:"age"`
	Pronouns   string `json:"pronouns"`
	Appearance string `json:"appearance"`
}

// NeedsEnrichment returns true if the entity is a character and is missing appearance, gender, or age.
func (e *CharacterEnricher) NeedsEnrichment(ent *entity.Entity) bool {
	if ent == nil || ent.Type != "character" {
		return false
	}
	return strings.TrimSpace(ent.Appearance) == "" || strings.TrimSpace(ent.Gender) == "" || strings.TrimSpace(ent.Age) == ""
}

// Enrich queries the model to infer missing character fields based on world context and notes.
func (e *CharacterEnricher) Enrich(ctx context.Context, ent *entity.Entity, worldGenre string) (*entity.Entity, error) {
	if !e.NeedsEnrichment(ent) {
		return ent, nil
	}

	prompt := fmt.Sprintf(`Generate the physical appearance and demographic attributes for this RPG character based on their background and setting.
Character Name: %s
World Genre: %s
Current Notes: %s

Respond ONLY with a valid JSON object matching this schema:
{
  "gender": "demographic gender",
  "age": "approximate age in years or developmental stage",
  "pronouns": "preferred pronouns",
  "appearance": "2-3 concise sentences describing their face, build, distinguishing marks, clothing, and posture"
}`, ent.Name, worldGenre, ent.Body)

	resp, err := e.router.GenerateForRole(ctx, "extractor", harness.GenerateRequest{
		System: "You are a concise character designer for a tabletop RPG. Output only valid JSON without markdown fences.",
		Prompt: prompt,
	})
	if err != nil {
		// Fallback to GM role if extractor fails or is unconfigured
		resp, err = e.router.GenerateForRole(ctx, "gm", harness.GenerateRequest{
			System: "You are a concise character designer for a tabletop RPG. Output only valid JSON without markdown fences.",
			Prompt: prompt,
		})
		if err != nil {
			return ent, fmt.Errorf("enrich character model call: %w", err)
		}
	}

	cleanJSON := strings.TrimSpace(resp.Text)
	if strings.HasPrefix(cleanJSON, "```json") {
		cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
		cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	} else if strings.HasPrefix(cleanJSON, "```") {
		cleanJSON = strings.TrimPrefix(cleanJSON, "```")
		cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	}
	cleanJSON = strings.TrimSpace(cleanJSON)

	var res characterEnrichmentResult
	if err := json.Unmarshal([]byte(cleanJSON), &res); err != nil {
		return ent, fmt.Errorf("parse character enrichment JSON: %w (raw: %s)", err, cleanJSON)
	}

	if ent.Gender == "" {
		ent.Gender = res.Gender
	}
	if ent.Age == "" {
		ent.Age = res.Age
	}
	if ent.Appearance == "" {
		ent.Appearance = res.Appearance
	}
	if res.Pronouns != "" {
		if ent.ExtraMeta == nil {
			ent.ExtraMeta = make(map[string]interface{})
		}
		if _, ok := ent.ExtraMeta["pronouns"]; !ok {
			ent.ExtraMeta["pronouns"] = res.Pronouns
		}
	}

	return ent, nil
}
