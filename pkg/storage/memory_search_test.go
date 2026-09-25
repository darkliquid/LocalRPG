package storage

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestSearchMemoriesAndRank(t *testing.T) {
	store := openMemoryTestStore(t)
	must := func(m *entity.Memory) {
		if _, err := store.SaveMemory(m); err != nil {
			t.Fatal(err)
		}
	}
	must(&entity.Memory{Turn: 1, Kind: entity.MemoryEvent, EntityRefs: []string{"kae"}, Text: "Found a silver locket.", Importance: 2, Source: entity.SourceGM})
	must(&entity.Memory{Turn: 40, Kind: entity.MemoryEvent, EntityRefs: []string{"kae"}, Text: "The silver locket is cursed.", Importance: 5, Source: entity.SourceGM})

	hits, err := store.SearchMemories("silver locket", "kae", "", 0, 10)
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestRankPrefersImportantRecent(t *testing.T) {
	hits := []MemoryHit{{ID: 1, Turn: 1, Importance: 2}, {ID: 2, Turn: 40, Importance: 5}}
	ranked := RankMemoryHits(hits, 41, 20)
	if ranked[0].ID != 2 {
		t.Fatalf("ranked = %+v, want id 2 first", ranked)
	}
}
