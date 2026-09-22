package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// ArcStatuses is the closed set an arc's status may take. Anything else is treated
// as open, so a typo degrades rather than hiding a live thread.
var ArcStatuses = []string{"open", "complicated", "resolved"}

// Thread is one unresolved arc as the prompt and the UI see it.
type Thread struct {
	ID           string
	Name         string
	Status       string
	LastAdvanced int
	Idle         int
}

// ArcStatus reads an arc's status, treating anything outside the set as open.
func ArcStatus(ent *entity.Entity) string {
	if ent == nil || ent.State == nil {
		return "open"
	}

	raw, ok := ent.State.Get("status")
	if !ok {
		return "open"
	}
	status, ok := raw.(string)
	if !ok {
		return "open"
	}

	normalised := strings.ToLower(strings.TrimSpace(status))
	for _, allowed := range ArcStatuses {
		if normalised == allowed {
			return allowed
		}
	}
	return "open"
}

// OpenThreads lists the arcs that are still unresolved, stalest first, each with
// how long since a turn last touched it. It reads the index rather than the notes,
// because "when was this last advanced" is a question about turns.
func OpenThreads(store *storage.Store, latestTurn int) ([]Thread, error) {
	if store == nil {
		return nil, nil
	}

	summaries, err := store.ListEntities()
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", err)
	}

	threads := make([]Thread, 0)
	for _, summary := range summaries {
		if summary.Type != "arc" {
			continue
		}

		ent, err := store.GetEntity(summary.ID)
		if err != nil || ent == nil {
			continue
		}
		if status := ArcStatus(ent); status == "resolved" {
			continue
		}

		lastAdvanced := 0
		if numbers, err := store.ListTurnsForEntity(summary.ID); err == nil && len(numbers) > 0 {
			lastAdvanced = numbers[0]
			for _, number := range numbers {
				if number > lastAdvanced {
					lastAdvanced = number
				}
			}
		}

		threads = append(threads, Thread{
			ID:           summary.ID,
			Name:         summary.Name,
			Status:       ArcStatus(ent),
			LastAdvanced: lastAdvanced,
			Idle:         latestTurn - lastAdvanced,
		})
	}

	// Stalest first: the thread nobody has touched is the one most likely to be
	// forgotten, and the one worth a nudge.
	sort.SliceStable(threads, func(i, j int) bool {
		if threads[i].Idle == threads[j].Idle {
			return threads[i].ID < threads[j].ID
		}
		return threads[i].Idle > threads[j].Idle
	})
	return threads, nil
}
