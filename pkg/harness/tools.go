package harness

import (
	"fmt"
	"strings"
)

// stringProperty is one JSON Schema string parameter.
func stringProperty(description string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": description}
}

// intProperty is one JSON Schema integer parameter.
func intProperty(description string) map[string]interface{} {
	return map[string]interface{}{"type": "integer", "description": description}
}

// objectSchema is a JSON Schema object with the given required keys.
func objectSchema(properties map[string]interface{}, required ...string) map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

// ToolSpecs is the whole tool surface. It is deliberately one table, so the list
// offered to a model, its documentation, and the dispatcher cannot drift apart.
// The surface is a permanent ceiling: read-only, internal, and never a shell,
// filesystem, network, or code-execution tool.
func ToolSpecs() []ToolSpec {
	return []ToolSpec{
		{
			Name:        "search_entities",
			Description: "Search the campaign's entities by words. Returns matching ids, names, types, and a body snippet.",
			Parameters: objectSchema(map[string]interface{}{
				"query": stringProperty("Words to search for, for example 'warden eastern gate'."),
				"type":  stringProperty("Optional entity type filter, for example 'character' or 'location'."),
				"limit": intProperty("Maximum matches to return. Defaults to 10."),
				"match": stringProperty("Optional raw FTS5 MATCH expression, for callers who know the syntax."),
			}, "query"),
		},
		{
			Name:        "get_entity",
			Description: "Read one entity by id or name. Returns its frontmatter, state, and the start of its note.",
			Parameters: objectSchema(map[string]interface{}{
				"id_or_name": stringProperty("The entity's id or its display name."),
			}, "id_or_name"),
		},
		{
			Name:        "graph_neighbours",
			Description: "List the entities connected to one entity, and the relation that connects them.",
			Parameters: objectSchema(map[string]interface{}{
				"id":        stringProperty("The entity's id."),
				"direction": stringProperty("Optional: 'from', 'to', or 'both'. Defaults to both."),
				"limit":     intProperty("Maximum neighbours to return. Defaults to 20."),
			}, "id"),
		},
		{
			Name:        "search_timeline",
			Description: "Search the campaign's past turns by words. Returns turn numbers with a narration snippet.",
			Parameters: objectSchema(map[string]interface{}{
				"query":  stringProperty("Words to search for in past narration and player input."),
				"entity": stringProperty("Optional entity id to restrict the search to turns mentioning it."),
				"limit":  intProperty("Maximum matches to return. Defaults to 10."),
				"match":  stringProperty("Optional raw FTS5 MATCH expression, for callers who know the syntax."),
			}, "query"),
		},
		{
			Name:        "search_memories",
			Description: "Search what characters, places, and factions remember by words. Returns turn, kind, importance, and a snippet.",
			Parameters: objectSchema(map[string]interface{}{
				"query":          stringProperty("Words to search for in memory text and tags."),
				"entity":         stringProperty("Optional entity id to restrict the search to its memories."),
				"kind":           stringProperty("Optional memory kind: event, relationship, discovery, dialogue, or mechanical."),
				"min_importance": intProperty("Optional minimum importance 1-5."),
				"limit":          intProperty("Maximum matches to return. Defaults to 10."),
			}, "query"),
		},
		{
			Name:        "get_entity_timeline",
			Description: "Read an entity's memories newest-first: its own timeline of key events, relationships, and discoveries.",
			Parameters: objectSchema(map[string]interface{}{
				"entity": stringProperty("The entity's id or name."),
				"limit":  intProperty("Maximum memories to return. Defaults to 20."),
			}, "entity"),
		},
		{
			Name:        "search_voice_profiles",
			Description: "Search available NPC voice profiles by traits, gender, age, tone, or style keywords (e.g. 'gruff elder', 'cheerful young pilot', 'sinister whisper'). Returns matching profile IDs and descriptions.",
			Parameters: objectSchema(map[string]interface{}{
				"query": stringProperty("Trait, gender, age, tone, or style keywords to search for."),
				"limit": intProperty("Optional maximum matches to return. Defaults to 5."),
			}, "query"),
		},
		{
			Name:        "assign_voice",
			Description: "Assign a voice profile to an NPC character. Use search_voice_profiles to find an appropriate profile ID first.",
			Parameters: objectSchema(map[string]interface{}{
				"entity":     stringProperty("The character's name or ID."),
				"profile_id": stringProperty("The voice profile ID to assign (e.g. 'fenrir', 'aoede')."),
			}, "entity", "profile_id"),
		},
	}
}

// ToolNames is the surface's names, in the order it is offered.
func ToolNames() []string {
	specs := ToolSpecs()
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	return names
}

// UnknownToolMessage is the readable result for an unknown or hallucinated tool,
// so the model can correct itself rather than retry the same call.
func UnknownToolMessage(name string) string {
	return fmt.Sprintf("error: unknown tool %q. Available tools: %s.", name, strings.Join(ToolNames(), ", "))
}
