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
