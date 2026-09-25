// Package tools implements the read-only tools the GM may call mid-turn. Every
// tool reads the campaign's own index and store; none of them can write.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// defaultLimit caps a modest tool result when the model does not ask for a limit.
const defaultLimit = 10

// Executor runs tool calls against one campaign's index.
type Executor struct {
	store    *storage.Store
	maxChars int
}

// NewExecutor builds an executor. maxChars is agents.tool_result_chars; a
// non-positive value falls back to 4000.
func NewExecutor(store *storage.Store, maxChars int) *Executor {
	if maxChars <= 0 {
		maxChars = 4000
	}
	return &Executor{store: store, maxChars: maxChars}
}

// Execute runs one call and returns the text the model will read. ok is false
// when the call failed, but the result is still a readable message: a tool error
// must never become a failed turn.
func (e *Executor) Execute(ctx context.Context, call harness.ToolCall) (string, bool) {
	arguments := map[string]interface{}{}
	if strings.TrimSpace(call.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &arguments); err != nil {
			return e.cap(fmt.Sprintf("error: could not parse the arguments for %s: %v", call.Name, err)), false
		}
	}

	switch call.Name {
	case "search_entities":
		return e.searchEntities(arguments)
	case "get_entity":
		return e.getEntity(arguments)
	case "graph_neighbours":
		return e.graphNeighbours(arguments)
	case "search_timeline":
		return e.searchTimeline(arguments)
	case "search_memories":
		return e.searchMemories(arguments)
	case "get_entity_timeline":
		return e.getEntityTimeline(arguments)
	default:
		return e.cap(harness.UnknownToolMessage(call.Name)), false
	}
}

func (e *Executor) searchEntities(arguments map[string]interface{}) (string, bool) {
	match := stringArgument(arguments, "match")
	if match == "" {
		match = BuildMatch(stringArgument(arguments, "query"))
	}
	if match == "" {
		return "error: search_entities needs a query", false
	}

	hits, err := e.store.SearchEntities(match, stringArgument(arguments, "type"), intArgument(arguments, "limit", defaultLimit))
	if err != nil {
		return e.cap(fmt.Sprintf("error: search_entities failed: %v", err)), false
	}
	if len(hits) == 0 {
		return "No entities matched.", true
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d matching entities:\n", len(hits))
	for _, hit := range hits {
		fmt.Fprintf(&sb, "- %s (%s, id %s): %s\n", hit.Name, hit.Type, hit.ID, hit.Snippet)
	}
	return e.cap(sb.String()), true
}

func (e *Executor) getEntity(arguments map[string]interface{}) (string, bool) {
	ref := stringArgument(arguments, "id_or_name")
	if ref == "" {
		return "error: get_entity needs id_or_name", false
	}

	ent, err := e.findEntity(ref)
	if err != nil {
		return e.cap(fmt.Sprintf("error: %v", err)), false
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (%s, id %s)\n", ent.Name, ent.Type, ent.ID)
	if ent.Location != "" {
		fmt.Fprintf(&sb, "location: %s\n", ent.Location)
	}
	if ent.Faction != "" {
		fmt.Fprintf(&sb, "faction: %s\n", ent.Faction)
	}
	if len(ent.Aliases) > 0 {
		fmt.Fprintf(&sb, "aliases: %s\n", strings.Join(ent.Aliases, ", "))
	}
	if ent.State != nil {
		if raw, err := json.Marshal(ent.State.Raw()); err == nil {
			fmt.Fprintf(&sb, "state: %s\n", raw)
		}
	}
	sb.WriteString("\n")
	sb.WriteString(ent.Body)
	return e.cap(sb.String()), true
}

// findEntity resolves an exact id first, then a case-insensitive name.
func (e *Executor) findEntity(ref string) (*entity.Entity, error) {
	if ent, err := e.store.GetEntity(ref); err == nil && ent != nil {
		return ent, nil
	}
	summaries, err := e.store.ListEntities()
	if err != nil {
		return nil, err
	}
	for _, summary := range summaries {
		if strings.EqualFold(summary.Name, ref) || strings.EqualFold(summary.ID, ref) {
			return e.store.GetEntity(summary.ID)
		}
	}
	return nil, fmt.Errorf("no entity matching %q", ref)
}

func (e *Executor) graphNeighbours(arguments map[string]interface{}) (string, bool) {
	id := stringArgument(arguments, "id")
	if id == "" {
		return "error: graph_neighbours needs an id", false
	}
	direction := strings.ToLower(stringArgument(arguments, "direction"))
	if direction == "" {
		direction = "both"
	}
	limit := intArgument(arguments, "limit", 20)

	lines := make([]string, 0)
	if direction == "from" || direction == "both" {
		edges, err := e.store.GetEdgesFrom(id)
		if err != nil {
			return e.cap(fmt.Sprintf("error: graph_neighbours failed: %v", err)), false
		}
		for _, edge := range edges {
			lines = append(lines, fmt.Sprintf("- %s -> %s (%s)", id, edge.TargetID, edge.Relation))
		}
	}
	if direction == "to" || direction == "both" {
		edges, err := e.store.GetEdgesTo(id)
		if err != nil {
			return e.cap(fmt.Sprintf("error: graph_neighbours failed: %v", err)), false
		}
		for _, edge := range edges {
			lines = append(lines, fmt.Sprintf("- %s <- %s (%s)", id, edge.SourceID, edge.Relation))
		}
	}

	if len(lines) == 0 {
		return fmt.Sprintf("No neighbours for %q.", id), true
	}
	sort.Strings(lines)
	if len(lines) > limit {
		lines = lines[:limit]
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d neighbours of %s:\n", len(lines), id)
	sb.WriteString(strings.Join(lines, "\n"))
	return e.cap(sb.String()), true
}

func (e *Executor) searchTimeline(arguments map[string]interface{}) (string, bool) {
	match := stringArgument(arguments, "match")
	if match == "" {
		match = BuildMatch(stringArgument(arguments, "query"))
	}
	if match == "" {
		return "error: search_timeline needs a query", false
	}

	hits, err := e.store.SearchTurns(match, stringArgument(arguments, "entity"), intArgument(arguments, "limit", defaultLimit))
	if err != nil {
		return e.cap(fmt.Sprintf("error: search_timeline failed: %v", err)), false
	}
	if len(hits) == 0 {
		return "No turns matched.", true
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d matching turns:\n", len(hits))
	for _, hit := range hits {
		fmt.Fprintf(&sb, "- turn %d: %s\n", hit.Number, hit.Snippet)
	}
	return e.cap(sb.String()), true
}

// cap truncates a result and says so, because a model that cannot tell a capped
// result from a small world will conclude the world is small.
func (e *Executor) cap(text string) string {
	runes := []rune(text)
	if len(runes) <= e.maxChars {
		return text
	}
	return string(runes[:e.maxChars]) + fmt.Sprintf("\n... (truncated at %d characters; narrow the query to see more)", e.maxChars)
}

func stringArgument(arguments map[string]interface{}, key string) string {
	value, _ := arguments[key].(string)
	return strings.TrimSpace(value)
}

func intArgument(arguments map[string]interface{}, key string, fallback int) int {
	switch value := arguments[key].(type) {
	case float64:
		if value > 0 {
			return int(value)
		}
	case int:
		if value > 0 {
			return value
		}
	}
	return fallback
}
