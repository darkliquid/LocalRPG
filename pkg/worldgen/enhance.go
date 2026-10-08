package worldgen

import (
	"context"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// Enhancement is one proposed change to an existing world.
type Enhancement struct {
	// Kind is "lore", "entity", or "hook".
	Kind  string `json:"kind"`
	Title string `json:"title"`
	// Body is Markdown for lore or a hook, and the note body for an entity.
	Body string `json:"body"`
	// Entity carries the entity a "entity" proposal would write.
	Entity *DraftEntity `json:"entity,omitempty"`
	// Target names the lore section a "lore" proposal appends under; empty means a
	// new section at the end.
	Target string `json:"target,omitempty"`
	// Reason is why the model proposes the change, shown in the diff.
	Reason string `json:"reason,omitempty"`
}

// The kinds an enhancement can be.
const (
	KindLore   = "lore"
	KindEntity = "entity"
	KindHook   = "hook"
)

// enhanceSchema is the diff shape: a list of proposals, each either prose for
// the lore or a whole entity.
const enhanceSchema = `{"type":"object","properties":{"proposals":{"type":"array","items":{` +
	`"type":"object","properties":{"kind":{"type":"string","enum":["lore","entity","hook"]},` +
	`"title":{"type":"string"},"body":{"type":"string"},"target":{"type":"string"},` +
	`"reason":{"type":"string"},"entity":{"type":"object","properties":{` +
	`"name":{"type":"string"},"type":{"type":"string"},"description":{"type":"string"},` +
	`"tags":{"type":"array","items":{"type":"string"}}}}},` +
	`"required":["kind","title"]}}}}`

const enhanceSystem = `You propose additions to an existing tabletop roleplaying world. ` +
	`Only propose new material; never rewrite what is already there. ` +
	`Return one JSON object and nothing else.`

// Enhance reads a world and proposes additions: lore sections, new entities, and
// story hooks. Nothing is written; the caller applies the accepted set.
func Enhance(ctx context.Context, gen Generator, world WorldContext, instruction string, kinds []string) ([]Enhancement, error) {
	if gen == nil {
		return nil, fmt.Errorf("worldgen: no generator configured")
	}
	raw, err := gen.GenerateJSON(ctx, buildEnhancePrompt(world, instruction, kinds), enhanceSchema)
	if err != nil {
		return nil, err
	}
	var out struct {
		Proposals []struct {
			Kind   string `json:"kind"`
			Title  string `json:"title"`
			Body   string `json:"body"`
			Target string `json:"target"`
			Reason string `json:"reason"`
			Entity *struct {
				Name        string   `json:"name"`
				Type        string   `json:"type"`
				Description string   `json:"description"`
				Tags        []string `json:"tags"`
			} `json:"entity"`
		} `json:"proposals"`
	}
	if err := decodeJSON(raw, &out); err != nil {
		return nil, err
	}

	proposals := make([]Enhancement, 0, len(out.Proposals))
	for _, p := range out.Proposals {
		if len(proposals) >= MaxProposals {
			break
		}
		kind := normalizeKind(p.Kind)
		if kind == "" {
			continue
		}
		e := Enhancement{
			Kind:   kind,
			Title:  strings.TrimSpace(p.Title),
			Body:   strings.TrimSpace(p.Body),
			Target: strings.TrimSpace(p.Target),
			Reason: strings.TrimSpace(p.Reason),
		}
		if e.Title == "" && e.Body == "" && p.Entity == nil {
			continue
		}
		if kind == KindEntity {
			draft, ok := enhancementEntity(p.Entity, world)
			if !ok {
				continue
			}
			e.Entity = &draft
			if e.Title == "" {
				e.Title = draft.Name
			}
		}
		proposals = append(proposals, e)
	}
	return proposals, nil
}

// enhancementEntity turns a proposed entity into a linked DraftEntity, resolving
// its wikilinks against the world's existing entities.
func enhancementEntity(spec *struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}, world WorldContext) (DraftEntity, bool) {
	if spec == nil {
		return DraftEntity{}, false
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return DraftEntity{}, false
	}
	kind := strings.TrimSpace(spec.Type)
	if kind == "" {
		kind = "concept"
	}
	batch := []DraftEntity{{
		ID:   entity.Slugify(name),
		Name: name,
		Type: kind,
		Tags: cleanTags(spec.Tags),
		Body: firstNonEmpty(spec.Description, name+"."),
	}}
	return linkBatch(batch, world.Entities)[0], true
}

// normalizeKind maps a reply's kind onto a known one, or "" when it is not.
func normalizeKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case KindLore:
		return KindLore
	case KindEntity:
		return KindEntity
	case KindHook:
		return KindHook
	default:
		return ""
	}
}

// buildEnhancePrompt assembles the enhancement context, reusing the batch
// prompt's bounds so a large world still fits.
func buildEnhancePrompt(world WorldContext, instruction string, kinds []string) string {
	var b strings.Builder
	b.WriteString(enhanceSystem)
	b.WriteString("\n\nPropose additions to this world.\n")
	b.WriteString("\nWorld: " + firstNonEmpty(world.Name, world.ID) + "\n")
	if world.Genre != "" {
		b.WriteString("Genre: " + world.Genre + "\n")
	}
	if world.Description != "" {
		b.WriteString("Description: " + truncate(world.Description, 400) + "\n")
	}
	if lore := truncate(world.Lore, entityPromptLoreLimit); lore != "" {
		b.WriteString("\nExisting lore (add to it, never rewrite it):\n" + lore + "\n")
	}
	if list := entitySummaryList(world.Entities, entityPromptEntityLimit); list != "" {
		b.WriteString("\nExisting entities (link to these; do not modify them):\n" + list)
	}
	if len(kinds) > 0 {
		b.WriteString("Kinds: " + strings.Join(cleanTags(kinds), ", ") + "\n")
	}
	if instruction != "" {
		b.WriteString("\nInstruction: " + truncate(instruction, entityPromptInstrLimit) + "\n")
	}
	b.WriteString(fmt.Sprintf("\nPropose at most %d changes. For lore, name the section to append "+
		"under in target (or leave it empty for a new section).\n", MaxProposals))
	b.WriteString(`Return {"proposals":[{"kind":...,"title":...,"body":...,"reason":...}]}.`)
	return b.String()
}
