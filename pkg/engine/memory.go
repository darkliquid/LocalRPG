package engine

import (
	"math"
	"sort"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

const (
	DefaultDecayFactor     = 0.8
	DefaultWeightFloor     = 0.1
	DefaultWeightIncrement = 1.0
	DefaultMaxAgeTurns     = 20
	DefaultMaxEntries      = 30
	DefaultRederiveWindow  = 30
)

// WorkingEntry is one active entity or thread in the persistent working set.
type WorkingEntry struct {
	Kind     harness.RefKind `json:"kind"`
	ID       string          `json:"id"`
	Weight   float64         `json:"weight"`
	LastTurn int             `json:"last_turn"`
	Role     string          `json:"role,omitempty"`
}

// WorkingSet is a bounded, weighted set of active entities and threads.
type WorkingSet struct {
	Entries  []WorkingEntry `json:"entries"`
	lastTurn int
}

// Apply updates the working set for a turn. Referenced entities/threads have their
// weight increased and are stamped with the turn number; unreferenced entries decay.
// Entries past the age cap or below the weight floor are evicted, and the set is
// capped to DefaultMaxEntries with ties broken by LastTurn then ID.
func (s *WorkingSet) Apply(turn int, refs []harness.Ref) {
	if s.Entries == nil {
		s.Entries = make([]WorkingEntry, 0)
	}

	refMap := make(map[string]harness.Ref)
	for _, r := range refs {
		if r.Kind == harness.RefEntity || r.Kind == harness.RefThread {
			if r.ID != "" {
				refMap[r.ID] = r
			}
		}
	}

	dt := 1
	if s.lastTurn > 0 && turn > s.lastTurn {
		dt = turn - s.lastTurn
	}

	surviving := make([]WorkingEntry, 0, len(s.Entries)+len(refMap))
	seenIDs := make(map[string]bool)

	for _, entry := range s.Entries {
		if ref, ok := refMap[entry.ID]; ok {
			entry.LastTurn = turn
			entry.Weight += DefaultWeightIncrement
			if ref.Relation != "" {
				entry.Role = ref.Relation
			}
			seenIDs[entry.ID] = true
			surviving = append(surviving, entry)
		} else {
			entry.Weight *= math.Pow(DefaultDecayFactor, float64(dt))
			if turn-entry.LastTurn > DefaultMaxAgeTurns || entry.Weight < DefaultWeightFloor {
				continue
			}
			seenIDs[entry.ID] = true
			surviving = append(surviving, entry)
		}
	}

	for id, ref := range refMap {
		if !seenIDs[id] {
			surviving = append(surviving, WorkingEntry{
				Kind:     ref.Kind,
				ID:       id,
				Weight:   DefaultWeightIncrement,
				LastTurn: turn,
				Role:     ref.Relation,
			})
		}
	}

	sort.SliceStable(surviving, func(i, j int) bool {
		if surviving[i].Weight != surviving[j].Weight {
			return surviving[i].Weight > surviving[j].Weight
		}
		if surviving[i].LastTurn != surviving[j].LastTurn {
			return surviving[i].LastTurn > surviving[j].LastTurn
		}
		return surviving[i].ID < surviving[j].ID
	})

	if len(surviving) > DefaultMaxEntries {
		surviving = surviving[:DefaultMaxEntries]
	}

	s.Entries = surviving
	s.lastTurn = turn
}

// Select returns the top N entries as prompt refs.
func (s *WorkingSet) Select(limit int) []harness.Ref {
	if len(s.Entries) == 0 || limit <= 0 {
		return nil
	}
	n := limit
	if n > len(s.Entries) {
		n = len(s.Entries)
	}
	refs := make([]harness.Ref, 0, n)
	for i := 0; i < n; i++ {
		e := s.Entries[i]
		refs = append(refs, harness.Ref{
			Kind:     e.Kind,
			ID:       e.ID,
			Relation: e.Role,
		})
	}
	return refs
}

// Rederive replays history turns in order through Apply to reconstruct the working set.
func (s *WorkingSet) Rederive(turns []Turn) WorkingSet {
	fresh := WorkingSet{}
	for _, turn := range turns {
		var refs []harness.Ref
		if turn.Context != nil && len(turn.Context.Refs) > 0 {
			refs = turn.Context.Refs
		} else {
			for _, m := range turn.Entities {
				refs = append(refs, harness.Ref{
					Kind:     harness.RefEntity,
					ID:       m.ID,
					Relation: m.Kind,
				})
			}
		}
		fresh.Apply(turn.Number, refs)
	}
	return fresh
}

// ToStorageRecords converts WorkingSet entries to storage.WorkingSetRecord slice.
func (s *WorkingSet) ToStorageRecords() []storage.WorkingSetRecord {
	records := make([]storage.WorkingSetRecord, 0, len(s.Entries))
	for _, e := range s.Entries {
		records = append(records, storage.WorkingSetRecord{
			EntityID: e.ID,
			Kind:     string(e.Kind),
			Weight:   e.Weight,
			LastTurn: e.LastTurn,
			Role:     e.Role,
		})
	}
	return records
}

// FromStorageRecords populates WorkingSet from storage.WorkingSetRecord slice.
func (s *WorkingSet) FromStorageRecords(records []storage.WorkingSetRecord) {
	s.Entries = make([]WorkingEntry, 0, len(records))
	maxTurn := 0
	for _, r := range records {
		s.Entries = append(s.Entries, WorkingEntry{
			Kind:     harness.RefKind(r.Kind),
			ID:       r.EntityID,
			Weight:   r.Weight,
			LastTurn: r.LastTurn,
			Role:     r.Role,
		})
		if r.LastTurn > maxTurn {
			maxTurn = r.LastTurn
		}
	}
	s.lastTurn = maxTurn
}
