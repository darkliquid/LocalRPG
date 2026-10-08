package worldgen

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
)

// wikilinkRe matches a [[target]] or [[target|label]] reference.
var wikilinkRe = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|[^\]]+)?\]\]`)

// The JSON schemas each step declares, so a provider that supports structured
// outputs returns data rather than prose to be parsed.
const (
	outlineSchema = `{"type":"object","properties":{` +
		`"name":{"type":"string"},"genre":{"type":"string"},"premise":{"type":"string"},` +
		`"description":{"type":"string"},"themes":{"type":"array","items":{"type":"string"}},` +
		`"tone":{"type":"string"},"art_style":{"type":"string"},"lore":{"type":"string"}},` +
		`"required":["name","premise"]}`

	placesSchema = `{"type":"object","properties":{` +
		`"locations":{"type":"array","items":` + placeSchemaBody + `},` +
		`"factions":{"type":"array","items":` + placeSchemaBody + `}}}`

	charactersSchema = `{"type":"object","properties":{"characters":{"type":"array","items":{` +
		`"type":"object","properties":{"name":{"type":"string"},"role":{"type":"string"},` +
		`"faction":{"type":"string"},"home_location":{"type":"string"},` +
		`"description":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}},` +
		`"state":{"type":"object"}},"required":["name"]}}}}`

	linkSchema = `{"type":"object","properties":{"entities":{"type":"array","items":{` +
		`"type":"object","properties":{"id":{"type":"string"},"body":{"type":"string"}},` +
		`"required":["id","body"]}}}}`

	placeSchemaBody = `{"type":"object","properties":{"name":{"type":"string"},` +
		`"description":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}}},` +
		`"required":["name"]}`
)

const worldGenSystem = `You are a world builder for a tabletop roleplaying game. ` +
	`Return one JSON object and nothing else, matching the requested schema exactly.`

// runOutline produces the world's identity, description, and lore.
func runOutline(ctx context.Context, gen Generator, brief Brief, draft *Draft) error {
	raw, err := gen.GenerateJSON(ctx, outlinePrompt(brief), outlineSchema)
	if err != nil {
		return err
	}
	var out struct {
		Name        string   `json:"name"`
		Genre       string   `json:"genre"`
		Premise     string   `json:"premise"`
		Description string   `json:"description"`
		Themes      []string `json:"themes"`
		Tone        string   `json:"tone"`
		ArtStyle    string   `json:"art_style"`
		Lore        string   `json:"lore"`
	}
	if err := decodeJSON(raw, &out); err != nil {
		return err
	}

	name := firstNonEmpty(out.Name, brief.Name, "Untitled World")
	id := entity.Slugify(name)
	if id == "" {
		id = "world"
	}
	description := firstNonEmpty(out.Description, out.Premise, brief.Premise)
	lore := strings.TrimSpace(out.Lore)
	if lore == "" {
		lore = buildLore(name, description, out.Tone, mergeTags(brief.Themes, out.Themes))
	}

	draft.ID = id
	draft.World = core.WorldManifest{
		ID:          id,
		Name:        name,
		Description: description,
		Genre:       firstNonEmpty(out.Genre, brief.Genre),
		ArtStyle:    strings.TrimSpace(out.ArtStyle),
		Tags:        mergeTags(brief.Themes, out.Themes),
	}
	draft.Lore = lore
	return nil
}

// runPlaces produces the world's locations and factions.
func runPlaces(ctx context.Context, gen Generator, brief Brief, draft *Draft) error {
	raw, err := gen.GenerateJSON(ctx, placesPrompt(brief, *draft), placesSchema)
	if err != nil {
		return err
	}
	var out struct {
		Locations []placeSpec `json:"locations"`
		Factions  []placeSpec `json:"factions"`
	}
	if err := decodeJSON(raw, &out); err != nil {
		return err
	}

	seen := existingIDs(draft.Entities)
	for _, loc := range out.Locations {
		if e, ok := loc.entity("location", seen); ok {
			draft.Entities = append(draft.Entities, e)
		}
	}
	for _, faction := range out.Factions {
		if e, ok := faction.entity("faction", seen); ok {
			draft.Entities = append(draft.Entities, e)
		}
	}
	draft.Entities = clampType(draft.Entities, "location", brief.Counts.Locations)
	draft.Entities = clampType(draft.Entities, "faction", brief.Counts.Factions)
	return nil
}

// runCharacters produces the world's characters, seeded with what exists so far.
func runCharacters(ctx context.Context, gen Generator, brief Brief, draft *Draft) error {
	raw, err := gen.GenerateJSON(ctx, charactersPrompt(brief, *draft), charactersSchema)
	if err != nil {
		return err
	}
	var out struct {
		Characters []characterSpec `json:"characters"`
	}
	if err := decodeJSON(raw, &out); err != nil {
		return err
	}

	seen := existingIDs(draft.Entities)
	for _, c := range out.Characters {
		if e, ok := c.entity(seen); ok {
			draft.Entities = append(draft.Entities, e)
		}
	}
	draft.Entities = clampType(draft.Entities, "character", brief.Counts.Characters)
	return nil
}

// runLink asks for the entities' bodies written with wikilinks between them, then
// validates every link against the draft's own ids.
func runLink(ctx context.Context, gen Generator, brief Brief, draft *Draft) error {
	raw, err := gen.GenerateJSON(ctx, linkPrompt(*draft), linkSchema)
	if err != nil {
		return err
	}
	var out struct {
		Entities []struct {
			ID   string `json:"id"`
			Body string `json:"body"`
		} `json:"entities"`
	}
	if err := decodeJSON(raw, &out); err != nil {
		return err
	}

	index := make(map[string]int, len(draft.Entities))
	for i, e := range draft.Entities {
		index[e.ID] = i
	}
	for _, written := range out.Entities {
		idx, ok := index[entity.Slugify(written.ID)]
		if !ok {
			continue
		}
		if body := strings.TrimSpace(written.Body); body != "" {
			draft.Entities[idx].Body = body
		}
	}
	*draft = linkDraft(*draft)
	return nil
}

// placeSpec is one location or faction as a step returns it.
type placeSpec struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

func (p placeSpec) entity(kind string, seen map[string]struct{}) (DraftEntity, bool) {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return DraftEntity{}, false
	}
	id := entity.Slugify(name)
	if _, clash := seen[id]; clash {
		return DraftEntity{}, false
	}
	seen[id] = struct{}{}
	body := strings.TrimSpace(p.Description)
	if body == "" {
		body = name + "."
	}
	return DraftEntity{ID: id, Name: name, Type: kind, Folder: EntityFolderFor(kind), Tags: cleanTags(p.Tags), Body: body}, true
}

// characterSpec is one character as a step returns it.
type characterSpec struct {
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	Faction      string   `json:"faction"`
	HomeLocation string   `json:"home_location"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	State        any      `json:"state"`
}

func (c characterSpec) entity(seen map[string]struct{}) (DraftEntity, bool) {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return DraftEntity{}, false
	}
	id := entity.Slugify(name)
	if _, clash := seen[id]; clash {
		return DraftEntity{}, false
	}
	seen[id] = struct{}{}

	tags := cleanTags(c.Tags)
	if role := strings.TrimSpace(c.Role); role != "" {
		tags = appendUnique(tags, role)
	}
	body := strings.TrimSpace(c.Description)
	if body == "" {
		body = name + "."
	}
	return DraftEntity{
		ID: id, Name: name, Type: "character", Folder: EntityFolderFor("character"), Tags: tags, Body: body,
	}, true
}

// existingIDs is the id set already present in a draft, so a later step cannot
// produce a duplicate note.
func existingIDs(entities []DraftEntity) map[string]struct{} {
	seen := make(map[string]struct{}, len(entities))
	for _, e := range entities {
		seen[e.ID] = struct{}{}
	}
	return seen
}

// clampType keeps at most limit entities of one type, leaving other types alone.
func clampType(entities []DraftEntity, kind string, limit int) []DraftEntity {
	if limit < 0 {
		limit = 0
	}
	out := make([]DraftEntity, 0, len(entities))
	kept := 0
	for _, e := range entities {
		if e.Type == kind {
			if kept >= limit {
				continue
			}
			kept++
		}
		out = append(out, e)
	}
	return out
}

// buildLore assembles a lore document when the model did not return one.
func buildLore(name, description, tone string, themes []string) string {
	var b strings.Builder
	b.WriteString("# " + name + "\n\n")
	if description != "" {
		b.WriteString(description + "\n\n")
	}
	if len(themes) > 0 {
		b.WriteString("## Themes\n\n")
		for _, theme := range themes {
			b.WriteString("- " + theme + "\n")
		}
		b.WriteString("\n")
	}
	if tone != "" {
		b.WriteString("## Tone\n\n" + tone + "\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func outlinePrompt(brief Brief) string {
	var b strings.Builder
	b.WriteString(worldGenSystem)
	b.WriteString("\n\nProduce the outline of a new world.\n")
	b.WriteString("\nPremise: " + brief.Premise + "\n")
	if brief.Name != "" {
		b.WriteString("Preferred name: " + brief.Name + "\n")
	}
	if brief.Genre != "" {
		b.WriteString("Genre: " + brief.Genre + "\n")
	}
	if len(brief.Themes) > 0 {
		b.WriteString("Themes: " + strings.Join(brief.Themes, ", ") + "\n")
	}
	b.WriteString("\nReturn the object described by the schema. Write lore as Markdown with a short " +
		"overview, a history section, and a themes section.")
	return b.String()
}

func placesPrompt(brief Brief, draft Draft) string {
	var b strings.Builder
	b.WriteString(worldGenSystem)
	b.WriteString("\n\nProduce the locations and factions of this world.\n")
	b.WriteString("\nWorld: " + draft.World.Name + "\n")
	if draft.World.Genre != "" {
		b.WriteString("Genre: " + draft.World.Genre + "\n")
	}
	if draft.World.Description != "" {
		b.WriteString("Description: " + truncate(draft.World.Description, 600) + "\n")
	}
	b.WriteString(fmt.Sprintf("\nProduce %d location(s) and %d faction(s). "+
		"Each needs a name and a one-line description.\n", brief.Counts.Locations, brief.Counts.Factions))
	b.WriteString("Return the object described by the schema.")
	return b.String()
}

func charactersPrompt(brief Brief, draft Draft) string {
	var b strings.Builder
	b.WriteString(worldGenSystem)
	b.WriteString("\n\nProduce the characters of this world.\n")
	b.WriteString("\nWorld: " + draft.World.Name + "\n")
	if draft.World.Description != "" {
		b.WriteString("Description: " + truncate(draft.World.Description, 400) + "\n")
	}
	if places := entityIndex(draft.Entities, "location", "faction"); places != "" {
		b.WriteString("\nPlaces and factions:\n" + places)
	}
	b.WriteString(fmt.Sprintf("\nProduce %d character(s). Each needs a name, a role, a description, "+
		"and optionally a faction and a home location drawn from the list above.\n", brief.Counts.Characters))
	b.WriteString("Return the object described by the schema.")
	return b.String()
}

func linkPrompt(draft Draft) string {
	var b strings.Builder
	b.WriteString(worldGenSystem)
	b.WriteString("\n\nWrite each entity's note body as short Markdown prose.\n")
	b.WriteString("Link to other entities with [[Name]] wikilinks where it reads naturally; " +
		"only link to the names listed below, and only a few times per note.\n\n")
	b.WriteString("Entities:\n")
	for _, e := range draft.Entities {
		b.WriteString(fmt.Sprintf("- id=%s name=%s type=%s\n", e.ID, e.Name, e.Type))
	}
	if draft.Lore != "" {
		b.WriteString("\nLore:\n" + truncate(draft.Lore, 1200) + "\n")
	}
	b.WriteString("\nReturn {\"entities\":[{\"id\":...,\"body\":...}]} for every entity above.")
	return b.String()
}

// entityIndex renders a bounded list of entities of the named types.
func entityIndex(entities []DraftEntity, kinds ...string) string {
	want := make(map[string]struct{}, len(kinds))
	for _, k := range kinds {
		want[k] = struct{}{}
	}
	var b strings.Builder
	count := 0
	for _, e := range entities {
		if _, ok := want[e.Type]; !ok {
			continue
		}
		if count >= 40 {
			break
		}
		b.WriteString(fmt.Sprintf("- %s (%s): %s\n", e.Name, e.Type, truncate(e.Body, 160)))
		count++
	}
	return b.String()
}

// truncate bounds a string to limit runes, appending an ellipsis when it cut.
func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 || len(s) <= limit {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cleanTags(tags []string) []string {
	var out []string
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		out = appendUnique(out, trimmed)
	}
	return out
}

func appendUnique(list []string, value string) []string {
	for _, existing := range list {
		if strings.EqualFold(existing, value) {
			return list
		}
	}
	return append(list, value)
}

func mergeTags(groups ...[]string) []string {
	var out []string
	for _, group := range groups {
		out = append(out, cleanTags(group)...)
	}
	return out
}
