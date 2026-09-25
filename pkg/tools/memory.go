package tools

import (
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/storage"
)

// searchMemories answers search_memories: FTS over memory text, ranked by
// importance and recency, with an optional entity restriction.
func (e *Executor) searchMemories(arguments map[string]interface{}) (string, bool) {
	match := BuildMatch(stringArgument(arguments, "query"))
	if match == "" {
		return "error: search_memories needs a query", false
	}
	hits, err := e.store.SearchMemories(
		match,
		stringArgument(arguments, "entity"),
		stringArgument(arguments, "kind"),
		intArgument(arguments, "min_importance", 0),
		intArgument(arguments, "limit", defaultLimit),
	)
	if err != nil {
		return e.cap(fmt.Sprintf("error: search_memories failed: %v", err)), false
	}
	if len(hits) == 0 {
		return "No memories matched.", true
	}

	latest := 0
	for _, hit := range hits {
		if hit.Turn > latest {
			latest = hit.Turn
		}
	}
	hits = storage.RankMemoryHits(hits, latest, 20)

	lines := make([]string, 0, len(hits))
	for _, hit := range hits {
		lines = append(lines, fmt.Sprintf("- turn %d (%s, importance %d): %s", hit.Turn, hit.Kind, hit.Importance, hit.Snippet))
	}
	return e.cap(strings.Join(lines, "\n")), true
}

// getEntityTimeline answers get_entity_timeline: an entity's memories newest-first.
func (e *Executor) getEntityTimeline(arguments map[string]interface{}) (string, bool) {
	ref := stringArgument(arguments, "entity")
	if ref == "" {
		return "error: get_entity_timeline needs an entity", false
	}
	id := ref
	if e.store != nil {
		if entity, err := e.store.GetEntity(ref); err == nil && entity != nil {
			id = entity.ID
		}
	}
	memories, err := e.store.ListMemoriesForEntity(id, intArgument(arguments, "limit", 20))
	if err != nil {
		return e.cap(fmt.Sprintf("error: get_entity_timeline failed: %v", err)), false
	}
	if len(memories) == 0 {
		return fmt.Sprintf("No memories recorded for %s.", id), true
	}
	lines := make([]string, 0, len(memories))
	for _, memory := range memories {
		lines = append(lines, fmt.Sprintf("- turn %d (%s, importance %d): %s", memory.Turn, memory.Kind, memory.Importance, memory.Text))
	}
	return e.cap(strings.Join(lines, "\n")), true
}
