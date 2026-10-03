package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// roster resolves a speaker name to an entity id without a store query, so a
// streamed line can be attributed the moment its colon arrives. It is mutable: a
// persona record declares a speaker mid-stream, and the roster is reused when the
// turn's segments are finalised so a newly declared speaker stays attributable.
type roster struct {
	store    *storage.Store
	byKey    map[string]string
	personae map[string]harness.PersonaDecl
	profiles []config.VoiceProfile
}

// newRoster seeds the roster from the store, the player, and configured voice profiles.
// A nil store yields a roster that resolves only declared speakers.
func newRoster(store *storage.Store, playerID, playerName string, profiles ...[]config.VoiceProfile) *roster {
	r := &roster{
		store:    store,
		byKey:    map[string]string{},
		personae: map[string]harness.PersonaDecl{},
	}
	if len(profiles) > 0 {
		r.profiles = profiles[0]
	}
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

// DeclarePersona adds a declared persona so their voice profile can be assigned
// and queried mid-stream before the entity is staged or saved to store.
func (r *roster) DeclarePersona(id string, decl harness.PersonaDecl) {
	if r == nil || id == "" {
		return
	}
	r.personae[id] = decl
}

// Voice returns the voice assigned to an entity, or nil. It reads the entity's
// own voice from the store, falling back to persona declaration with gender-matched voice.
func (r *roster) Voice(id string) *entity.VoiceConfig {
	if r == nil {
		return nil
	}
	if r.store != nil {
		if voice := harness.ResolveSpeakerVoice(r.store, id); voice != nil {
			return voice
		}
	}
	if decl, ok := r.personae[id]; ok {
		temp := &entity.Entity{
			ID:          id,
			Name:        decl.Name,
			Type:        "character",
			Gender:      decl.Gender,
			Body:        decl.Description,
			Tags:        decl.RoleTags,
		}
		harness.AssignVoiceProfile(temp, r.profiles)
		return temp.Voice
	}
	return nil
}
