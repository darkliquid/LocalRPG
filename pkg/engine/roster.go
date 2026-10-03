package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// roster resolves a speaker name to an entity id without a store query, so a
// streamed line can be attributed the moment its colon arrives. It is mutable: a
// persona record declares a speaker mid-stream, and the roster is reused when the
// turn's segments are finalised so a newly declared speaker stays attributable.
type roster struct {
	store *storage.Store
	byKey map[string]string
}

// newRoster seeds the roster from the store and the player. A nil store yields a
// roster that resolves only declared speakers.
func newRoster(store *storage.Store, playerID, playerName string) *roster {
	r := &roster{store: store, byKey: map[string]string{}}
	if store != nil {
		if summaries, err := store.ListEntities(); err == nil {
			for _, summary := range summaries {
				r.Declare(summary.Name, summary.ID)
				r.Declare(summary.ID, summary.ID)
				for _, alias := range summary.Aliases {
					r.Declare(alias, summary.ID)
				}
			}
		}
	}
	if playerID != "" {
		r.Declare(playerName, playerID)
		r.Declare(playerID, playerID)
	}
	return r
}

// Resolve maps a speaker name or slug to an entity id, case-insensitively.
func (r *roster) Resolve(name string) (string, bool) {
	if r == nil {
		return "", false
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if id, ok := r.byKey[key]; ok {
		return id, true
	}
	if slug := entity.Slugify(name); slug != "" {
		if id, ok := r.byKey[slug]; ok {
			return id, true
		}
	}
	return "", false
}

// Declare adds a name, its slug, and its lowercased form as keys for an entity
// id, so a declaration and a later lookup agree on spelling.
func (r *roster) Declare(name, id string) {
	if r == nil || id == "" {
		return
	}
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		r.byKey[strings.ToLower(trimmed)] = id
	}
	if slug := entity.Slugify(name); slug != "" {
		r.byKey[slug] = id
	}
}

// Voice returns the voice assigned to an entity, or nil. It reads the entity's
// own voice, which is where an assigned profile is stored.
func (r *roster) Voice(id string) *entity.VoiceConfig {
	if r == nil || r.store == nil {
		return nil
	}
	return harness.ResolveSpeakerVoice(r.store, id)
}
