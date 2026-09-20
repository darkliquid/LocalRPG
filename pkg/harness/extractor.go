package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type ExtractedEntity struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location string `json:"location,omitempty"`
	Faction  string `json:"faction,omitempty"`
	Body     string `json:"body"`
}

type EntityExtractor struct {
	model ModelProvider
	store *storage.Store
}

func NewEntityExtractor(model ModelProvider, store *storage.Store) *EntityExtractor {
	return &EntityExtractor{
		model: model,
		store: store,
	}
}

const extractorSystemPrompt = `You are a world-state extractor. Read the narrative turn and return a JSON list of any newly discovered or updated characters, locations, items, factions, or plot arcs. Format:
[
  {
    "id": "kebab-case-id",
    "name": "Full Name",
    "type": "character|location|item|faction|arc",
    "location": "[[Optional-Location]]",
    "body": "Description and known facts."
  }
]
If nothing new is discovered, return []`

func (e *EntityExtractor) ExtractFromTurn(ctx context.Context, narrativeOutput string) (int, error) {
	req := GenerateRequest{
		System: extractorSystemPrompt,
		Prompt: narrativeOutput,
	}

	res, err := e.model.Generate(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("extractor model failed: %w", err)
	}

	cleaned := strings.TrimSpace(res.Text)
	if idx := strings.Index(cleaned, "["); idx != -1 {
		cleaned = cleaned[idx:]
	}
	if idx := strings.LastIndex(cleaned, "]"); idx != -1 {
		cleaned = cleaned[:idx+1]
	}

	var extracted []ExtractedEntity
	if err := json.Unmarshal([]byte(cleaned), &extracted); err != nil {
		return 0, fmt.Errorf("parse extracted json %q: %w", cleaned, err)
	}

	savedCount := 0
	for _, raw := range extracted {
		if raw.ID == "" || raw.Name == "" {
			continue
		}

		ent := &entity.Entity{
			ID:        raw.ID,
			Name:      raw.Name,
			Type:      raw.Type,
			Location:  raw.Location,
			Faction:   raw.Faction,
			Body:      raw.Body,
			Wikilinks: make([]string, 0),
			Hash:      fmt.Sprintf("extracted-%s", raw.ID),
		}

		if err := e.store.SaveEntity(ent); err == nil {
			savedCount++
		}
	}

	return savedCount, nil
}
