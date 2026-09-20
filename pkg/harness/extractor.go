package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
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
	model         ModelProvider
	store         *storage.Store
	voiceProfiles []config.VoiceProfile
}

func NewEntityExtractor(model ModelProvider, store *storage.Store) *EntityExtractor {
	return &EntityExtractor{
		model: model,
		store: store,
	}
}

func (e *EntityExtractor) SetVoiceProfiles(profiles []config.VoiceProfile) {
	e.voiceProfiles = profiles
}

var wikilinkExtractorRegex = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|[^\]]+)?\]\]`)

func ExtractEntitiesWithProfiles(prose string, profiles []config.VoiceProfile) []*entity.Entity {
	matches := wikilinkExtractorRegex.FindAllStringSubmatch(prose, -1)
	var entities []*entity.Entity
	for _, m := range matches {
		if len(m) >= 2 {
			name := m[1]
			id := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
			ent := &entity.Entity{
				ID:   id,
				Name: name,
				Type: "character",
				Body: prose,
			}
			AssignVoiceProfile(ent, profiles)
			entities = append(entities, ent)
		}
	}
	return entities
}

func AssignVoiceProfile(ent *entity.Entity, profiles []config.VoiceProfile) {
	if len(profiles) == 0 || ent == nil || ent.Type != "character" || ent.Voice != nil {
		return
	}

	searchContent := strings.ToLower(ent.Name + " " + ent.Body)

	// 1. Check direct profile ID match
	for _, p := range profiles {
		if strings.Contains(searchContent, strings.ToLower(p.ID)) {
			ent.Voice = &entity.VoiceConfig{
				VoiceID:    p.VoiceID,
				Pitch:      p.Pitch,
				SpeechRate: p.SpeechRate,
			}
			return
		}
	}

	// 2. Score by tag matches
	bestScore := 0
	var bestProfile *config.VoiceProfile
	for i := range profiles {
		score := 0
		for _, tag := range profiles[i].Tags {
			if strings.Contains(searchContent, strings.ToLower(tag)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestProfile = &profiles[i]
		}
	}

	if bestProfile != nil {
		ent.Voice = &entity.VoiceConfig{
			VoiceID:    bestProfile.VoiceID,
			Pitch:      bestProfile.Pitch,
			SpeechRate: bestProfile.SpeechRate,
		}
		return
	}

	// 3. Deterministic hash fallback
	h := fnv.New32a()
	h.Write([]byte(ent.ID))
	idx := int(h.Sum32()) % len(profiles)
	p := profiles[idx]
	ent.Voice = &entity.VoiceConfig{
		VoiceID:    p.VoiceID,
		Pitch:      p.Pitch,
		SpeechRate: p.SpeechRate,
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

		if ent.Type == "character" {
			AssignVoiceProfile(ent, e.voiceProfiles)
		}

		if err := e.store.SaveEntity(ent); err == nil {
			savedCount++
		}
	}

	return savedCount, nil
}
