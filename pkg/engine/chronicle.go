package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/state"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// ChronicleEntityID is the note holding a campaign's rolling summary. It is a
// normal entity on purpose: the player can read it, edit it, and export it, and a
// rebuilt index cannot lose it.
const ChronicleEntityID = "chronicle"

// ChronicleThroughTurnKey is the state key recording how far the summary reaches.
const ChronicleThroughTurnKey = "through_turn"

// Chronicle is a campaign's memory of everything older than the recall window.
type Chronicle struct {
	Summary     string
	ThroughTurn int
}

// ReadChronicle returns the campaign's summary, or an empty one when it has none.
func ReadChronicle(store *storage.Store) (Chronicle, error) {
	if store == nil {
		return Chronicle{}, nil
	}

	ent, err := store.GetEntity(ChronicleEntityID)
	if err != nil || ent == nil {
		return Chronicle{}, nil
	}

	chronicle := Chronicle{Summary: strings.TrimSpace(ent.Body)}
	if ent.State != nil {
		if raw, ok := ent.State.Get(ChronicleThroughTurnKey); ok {
			chronicle.ThroughTurn = intFromAny(raw)
		}
	}
	return chronicle, nil
}

// WriteChronicle writes the summary as a note and indexes it. The write precedes
// the index, so an indexing failure leaves the summary on disk rather than the
// other way round.
func WriteChronicle(store *storage.Store, entitiesDir string, chronicle Chronicle) error {
	note := &entity.Entity{
		ID:    ChronicleEntityID,
		Name:  "Story So Far",
		Type:  "chronicle",
		Body:  strings.TrimSpace(chronicle.Summary),
		State: state.NewState(map[string]any{ChronicleThroughTurnKey: chronicle.ThroughTurn}),
	}

	data, err := note.SerializeMarkdown()
	if err != nil {
		return fmt.Errorf("serialize chronicle: %w", err)
	}

	path := filepath.Join(entitiesDir, ChronicleEntityID+".md")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write chronicle: %w", err)
	}

	if store != nil {
		if err := storage.NewSyncer(store).SyncFile(path); err != nil {
			return fmt.Errorf("index chronicle: %w", err)
		}
	}
	return nil
}

// intFromAny converts a state value to an int. YAML and JSON disagree on numeric
// types, and the store decodes frontmatter from JSON.
func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return 0
}
