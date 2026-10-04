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
	dynamic  bool
}

// newRoster seeds the roster from the store, the player, and configured voice profiles.
// A nil store yields a roster that resolves only declared speakers.
func newRoster(store *storage.Store, playerID, playerName string, profiles ...[]config.VoiceProfile) *roster {
	r := &roster{
		store:    store,
		byKey:    map[string]string{},
		personae: map[string]harness.PersonaDecl{},
		dynamic:  true,
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
	clean := strings.TrimSpace(entity.WikilinkTarget(strings.Trim(name, "*_\"'“”‘’")))
	if clean == "" {
		return "", false
	}
	key := strings.ToLower(clean)
	if id, ok := r.byKey[key]; ok {
		return id, true
	}
	if slug := entity.Slugify(clean); slug != "" {
		if id, ok := r.byKey[slug]; ok {
			return id, true
		}
	}
	if r.store != nil {
		if id := harness.ResolveSpeakerID(r.store, clean); id != "" {
			r.Declare(clean, id)
			return id, true
		}
	}
	if r.dynamic && isValidSpeakerName(clean) {
		id := entity.Slugify(clean)
		if id != "" {
			r.Declare(clean, id)
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
	if prev := strings.TrimSpace(decl.PreviousIdentity()); prev != "" {
		r.Declare(prev, id)
		if slug := entity.Slugify(prev); slug != "" {
			r.Declare(slug, id)
		}
	}
}

// Voice returns the voice assigned to an entity, or nil. It reads the entity's
// own voice from the store, falling back to persona declaration with gender-matched voice,
// and finally to deterministic profile assignment.
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
	if len(r.profiles) > 0 && id != "" {
		temp := &entity.Entity{
			ID:   id,
			Name: id,
			Type: "character",
		}
		harness.AssignVoiceProfile(temp, r.profiles)
		return temp.Voice
	}
	return nil
}

// isValidSpeakerName reports whether a string looks like a genuine character or
// speaker name rather than prose, punctuation, or a system tag.
func isValidSpeakerName(raw string) bool {
	name := strings.TrimSpace(raw)
	name = strings.Trim(name, "*_\"'“”‘’")
	if len(name) < 2 || len(name) > 40 {
		return false
	}

	// Must not contain punctuation that indicates sentence structure or markdown markup.
	if strings.ContainsAny(name, "!?;\n\t\r{}[]<>|`~@#$%^&*()=+") {
		return false
	}
	if strings.Contains(name, ",") {
		return false
	}

	// Handle periods: allow common titles (e.g. Dr., Mr.), but reject sentence-ending periods
	// or arbitrary abbreviations.
	if strings.Contains(name, ".") {
		allowed := false
		for _, title := range []string{"dr.", "mr.", "mrs.", "ms.", "prof.", "st.", "sgt.", "cpl.", "lt.", "capt.", "gen.", "col."} {
			if strings.HasPrefix(strings.ToLower(name), title) {
				rest := strings.TrimSpace(name[len(title):])
				if !strings.Contains(rest, ".") {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			return false
		}
	}

	lower := strings.ToLower(name)

	// Blocked system / meta keywords and transition words.
	keywords := []string{
		"note", "warning", "caution", "tip", "important", "turn", "status", "location",
		"scene", "gm", "narrator", "directive", "roll", "check", "action", "outcome",
		"chapter", "act", "part", "summary", "recap", "inventory", "quest", "stats",
		"suddenly", "meanwhile", "however", "finally", "afterward", "afterwards", "later",
	}
	for _, kw := range keywords {
		if lower == kw || strings.HasPrefix(lower, kw+" ") {
			return false
		}
	}

	// Sentence-starting conjunctions, prepositions, or pronouns that precede a colon in prose.
	clauseStarters := []string{
		"as ", "when ", "then ", "if ", "while ", "after ", "before ", "because ", "since ",
		"although ", "though ", "where ", "why ", "how ", "what ", "who ", "which ", "there ",
		"here ", "it ", "they ", "he ", "she ", "you ", "we ", "i ", "meanwhile ", "suddenly ",
		"slowly ", "quietly ", "looking ", "standing ", "turning ", "walking ", "running ",
		"reaching ", "drawing ", "stepping ",
	}
	for _, starter := range clauseStarters {
		if strings.HasPrefix(lower, starter) {
			return false
		}
	}

	// Dialogue speech verbs.
	speechVerbs := []string{
		" said", " says", " asked", " asks", " replied", " replies", " declared", " declares",
		" shouted", " shouts", " whispered", " whispers", " cried", " cries", " called", " calls",
		" yelled", " yells", " muttered", " mutters", " barked", " barks", " snapped", " snaps",
		" hissed", " hisses", " growled", " growls",
	}
	for _, verb := range speechVerbs {
		if strings.Contains(lower, verb) {
			return false
		}
	}

	return true
}

