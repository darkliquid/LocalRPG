package tools

import "sort"

const rrfConstant = 60.0

// FuseRankings combines two ordered candidate lists (FTS BM25 rank and Vector rank)
// using Reciprocal Rank Fusion (RRF) with equal 0.5 weights.
// Score(d) = 0.5 / (60 + rank_fts) + 0.5 / (60 + rank_vec)
func FuseRankings(ftsIDs, vecIDs []string, limit int) []string {
	if len(ftsIDs) == 0 && len(vecIDs) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}

	scores := make(map[string]float64)

	for rank, id := range ftsIDs {
		// 1-indexed rank
		scores[id] += 0.5 / (rrfConstant + float64(rank+1))
	}
	for rank, id := range vecIDs {
		scores[id] += 0.5 / (rrfConstant + float64(rank+1))
	}

	type scoredItem struct {
		id    string
		score float64
	}
	items := make([]scoredItem, 0, len(scores))
	for id, score := range scores {
		items = append(items, scoredItem{id: id, score: score})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].id < items[j].id
		}
		return items[i].score > items[j].score
	})

	if len(items) > limit {
		items = items[:limit]
	}

	result := make([]string, len(items))
	for i, item := range items {
		result[i] = item.id
	}
	return result
}
