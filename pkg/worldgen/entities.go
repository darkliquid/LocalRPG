package worldgen

import (
	"context"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// EntityRequest asks for a batch of entities in an existing world.
type EntityRequest struct {
	WorldID     string   `json:"world_id,omitempty"`
	Instruction string   `json:"instruction"`
	Kinds       []string `json:"kinds,omitempty"`
	Count       int      `json:"count,omitempty"`
	// Focus anchors the batch to an existing entity or location.
	Focus string `json:"focus,omitempty"`
}

// EntitySummary is one existing entity as the generator sees it.
type EntitySummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Summary string `json:"summary,omitempty"`
}

// WorldContext is everything a generation needs to know about a world: its
// manifest, its lore, and the entities already in it.
type WorldContext struct {
	ID          string          `json:"id"`
	Name        string          `json:"name,omitempty"`
	Genre       string          `json:"genre,omitempty"`
	Description string          `json:"description,omitempty"`
	Lore        string          `json:"lore,omitempty"`
	Entities    []EntitySummary `json:"entities,omitempty"`
}

// entitiesSchema is the batch shape, distinct from the pipeline's link step in
// that each item carries a type and a description rather than a body.
const entitiesSchema = `{"type":"object","properties":{"entities":{"type":"array","items":{` +
	`"type":"object","properties":{"name":{"type":"string"},"type":{"type":"string"},` +
	`"description":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}}},` +
	`"required":["name","type"]}}}}`

const entityGenSystem = `You add new entities to an existing tabletop roleplaying world. ` +
	`Match the world's existing tone and naming. Return one JSON object and nothing else.`

// Prompt size caps, so a large world still fits a model's context.
const (
	entityPromptLoreLimit   = 2000
	entityPromptEntityLimit = 40
	entityPromptFocusLimit  = 200
	entityPromptInstrLimit  = 500
)

// GenerateEntities produces a batch of entities seeded with the world's lore and
// its existing entities, with every wikilink resolved against both.
func GenerateEntities(ctx context.Context, gen Generator, world WorldContext, req EntityRequest) ([]DraftEntity, error) {
	if gen == nil {
		return nil, fmt.Errorf("worldgen: no generator configured")
	}
	req = normalizeEntityRequest(req)

	raw, err := gen.GenerateJSON(ctx, buildEntityPrompt(world, req), entitiesSchema)
	if err != nil {
		return nil, err
	}
	var out struct {
		Entities []struct {
			Name        string   `json:"name"`
			Type        string   `json:"type"`
			Description string   `json:"description"`
			Tags        []string `json:"tags"`
		} `json:"entities"`
	}
	if err := decodeJSON(raw, &out); err != nil {
		return nil, err
	}

	batch := make([]DraftEntity, 0, len(out.Entities))
	for _, spec := range out.Entities {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		kind := strings.TrimSpace(spec.Type)
		if kind == "" {
			kind = defaultKind(req)
		}
		body := firstNonEmpty(spec.Description, name+".")
		batch = append(batch, DraftEntity{
			ID:   entity.Slugify(name),
			Name: name,
			Type: kind,
			Tags: cleanTags(spec.Tags),
			Body: body,
		})
		if len(batch) >= req.Count {
			break
		}
	}
	return linkBatch(batch, world.Entities), nil
}

// normalizeEntityRequest clamps a request so one call cannot ask for an
// unbounded batch.
func normalizeEntityRequest(req EntityRequest) EntityRequest {
	req.Instruction = strings.TrimSpace(req.Instruction)
	req.Focus = strings.TrimSpace(req.Focus)
	req.Kinds = cleanTags(req.Kinds)
	if req.Count <= 0 {
		req.Count = 3
	}
	if req.Count > MaxBatch {
		req.Count = MaxBatch
	}
	return req
}

// defaultKind is the entity type a batch falls back to when neither the request
// nor the reply names one.
func defaultKind(req EntityRequest) string {
	if len(req.Kinds) > 0 {
		return req.Kinds[0]
	}
	return "character"
}

// buildEntityPrompt assembles the batch prompt, hard-capping the lore and the
// entity list so a large world does not overflow a model's context.
func buildEntityPrompt(world WorldContext, req EntityRequest) string {
	var b strings.Builder
	b.WriteString(entityGenSystem)
	b.WriteString("\n\nAdd new entities to this world.\n")
	b.WriteString("\nWorld: " + firstNonEmpty(world.Name, world.ID) + "\n")
	if world.Genre != "" {
		b.WriteString("Genre: " + world.Genre + "\n")
	}
	if world.Description != "" {
		b.WriteString("Description: " + truncate(world.Description, 400) + "\n")
	}
	if lore := truncate(world.Lore, entityPromptLoreLimit); lore != "" {
		b.WriteString("\nLore:\n" + lore + "\n")
	}
	if list := entitySummaryList(world.Entities, entityPromptEntityLimit); list != "" {
		b.WriteString("\nExisting entities (do not duplicate these; link to them where it fits):\n" + list)
	}
	if req.Focus != "" {
		b.WriteString("\nFocus on: " + truncate(req.Focus, entityPromptFocusLimit) + "\n")
	}
	if len(req.Kinds) > 0 {
		b.WriteString("Kinds: " + strings.Join(req.Kinds, ", ") + "\n")
	}
	b.WriteString("\nInstruction: " + truncate(req.Instruction, entityPromptInstrLimit) + "\n")
	b.WriteString(fmt.Sprintf("\nProduce %d entities.\n", req.Count))
	b.WriteString(`Return {"entities":[{"name":...,"type":...,"description":...,"tags":[...]}]}.`)
	return b.String()
}

// entitySummaryList renders a bounded list of existing entities.
func entitySummaryList(entities []EntitySummary, limit int) string {
	var b strings.Builder
	count := 0
	for _, e := range entities {
		if count >= limit {
			break
		}
		if e.ID == "" && e.Name == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("- id=%s name=%s type=%s", e.ID, e.Name, e.Type))
		if e.Summary != "" {
			b.WriteString(": " + truncate(e.Summary, 120))
		}
		b.WriteString("\n")
		count++
	}
	return b.String()
}

// linkBatch validates a batch's wikilinks against the batch itself plus the
// world's existing entities, dropping an unresolved link and noting it.
func linkBatch(batch []DraftEntity, existing []EntitySummary) []DraftEntity {
	known := make(map[string]struct{}, len(existing)+len(batch))
	for _, e := range existing {
		if e.ID != "" {
			known[e.ID] = struct{}{}
		}
		if e.Name != "" {
			known[entity.Slugify(e.Name)] = struct{}{}
		}
	}
	for _, e := range batch {
		if e.ID != "" {
			known[e.ID] = struct{}{}
		}
	}
	linked := linkAgainst(Draft{Entities: batch}, known)
	return linked.Entities
}
