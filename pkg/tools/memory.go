package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/storage"
)

// searchMemories answers search_memories: FTS and vector similarity over memory
// text, fused via Reciprocal Rank Fusion, ranked by importance and recency.
func (e *Executor) searchMemories(ctx context.Context, arguments map[string]interface{}) (string, bool) {
	rawQuery := stringArgument(arguments, "query")
	match := BuildMatch(rawQuery)
	if match == "" && rawQuery == "" {
		return "error: search_memories needs a query", false
	}
	entityArg := stringArgument(arguments, "entity")
	kindArg := stringArgument(arguments, "kind")
	minImportance := intArgument(arguments, "min_importance", 0)
	limit := intArgument(arguments, "limit", defaultLimit)

	var ftsHits []storage.MemoryHit
	if match != "" {
		hits, err := e.store.SearchMemories(
			match,
			entityArg,
			kindArg,
			minImportance,
			limit*2,
		)
		if err == nil {
			ftsHits = hits
		}
	}

	var vecMemoryIDs []string
	if e.embeddingsProvider != nil && rawQuery != "" {
		vecs, err := e.embeddingsProvider.Embed(ctx, []string{rawQuery})
		if err == nil && len(vecs) > 0 {
			vHits, err := e.store.SearchSimilarVectors(ctx, []string{"memory"}, e.embeddingsProvider.ID(), vecs[0], limit*2)
			if err == nil {
				for _, vh := range vHits {
					vecMemoryIDs = append(vecMemoryIDs, vh.TargetID)
				}
			}
		}
	}

	if len(ftsHits) == 0 && len(vecMemoryIDs) == 0 {
		return "No memories matched.", true
	}

	ftsIDs := make([]string, len(ftsHits))
	ftsMap := make(map[string]storage.MemoryHit, len(ftsHits))
	for i, hit := range ftsHits {
		strID := strconv.FormatInt(hit.ID, 10)
		ftsIDs[i] = strID
		ftsMap[strID] = hit
	}

	fusedIDs := FuseRankings(ftsIDs, vecMemoryIDs, limit)
	if len(fusedIDs) == 0 {
		return "No memories matched.", true
	}

	hits := make([]storage.MemoryHit, 0, len(fusedIDs))
	for _, strID := range fusedIDs {
		if hit, ok := ftsMap[strID]; ok {
			hits = append(hits, hit)
		} else {
			id, err := strconv.ParseInt(strID, 10, 64)
			if err == nil {
				if hit, err := e.store.GetMemoryHit(id); err == nil && hit != nil {
					if kindArg != "" && hit.Kind != kindArg {
						continue
					}
					if minImportance > 0 && hit.Importance < minImportance {
						continue
					}
					hits = append(hits, *hit)
				}
			}
		}
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
