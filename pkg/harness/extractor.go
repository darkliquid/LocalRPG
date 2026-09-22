package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
)

type ExtractedEntity struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Location   string `json:"location,omitempty"`
	Faction    string `json:"faction,omitempty"`
	Appearance string `json:"appearance,omitempty"`
	Body       string `json:"body"`
}

// ExtractedDialogue is one utterance the model attributed to a speaker.
type ExtractedDialogue struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

// Extraction is everything one extraction pass returned for a turn.
type Extraction struct {
	Entities       []ExtractedEntity   `json:"entities"`
	Dialogue       []ExtractedDialogue `json:"dialogue,omitempty"`
	PlayerLocation string              `json:"player_location,omitempty"`
}

type Extractor struct {
	model         ModelProvider
	voiceProfiles []config.VoiceProfile
	id            string
	logger        trace.Logger
}

func NewExtractor(model ModelProvider) *Extractor {
	return &Extractor{model: model, id: model.ID()}
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (e *Extractor) SetLogger(logger trace.Logger) {
	e.logger = trace.OrNil(logger)
}

func (e *Extractor) SetVoiceProfiles(profiles []config.VoiceProfile) {
	e.voiceProfiles = profiles
}

func AssignVoiceProfile(ent *entity.Entity, profiles []config.VoiceProfile) {
	if len(profiles) == 0 || ent == nil || ent.Type != "character" || ent.Voice != nil {
		return
	}

	searchContent := strings.ToLower(ent.Name + " " + ent.Body)

	// 1. Check direct profile ID match
	for _, p := range profiles {
		if strings.Contains(searchContent, strings.ToLower(p.ID)) {
			ent.Voice = &entity.VoiceConfig{
				VoiceID:    p.VoiceID,
				Pitch:      p.Pitch,
				SpeechRate: p.SpeechRate,
			}
			return
		}
	}

	// 2. Score by tag matches
	bestScore := 0
	var bestProfile *config.VoiceProfile
	for i := range profiles {
		score := 0
		for _, tag := range profiles[i].Tags {
			if strings.Contains(searchContent, strings.ToLower(tag)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestProfile = &profiles[i]
		}
	}

	if bestProfile != nil {
		ent.Voice = &entity.VoiceConfig{
			VoiceID:    bestProfile.VoiceID,
			Pitch:      bestProfile.Pitch,
			SpeechRate: bestProfile.SpeechRate,
		}
		return
	}

	// 3. Deterministic hash fallback
	h := fnv.New32a()
	h.Write([]byte(ent.ID))
	idx := int(h.Sum32()) % len(profiles)
	p := profiles[idx]
	ent.Voice = &entity.VoiceConfig{
		VoiceID:    p.VoiceID,
		Pitch:      p.Pitch,
		SpeechRate: p.SpeechRate,
	}
}

const minSharedNameLength = 4

// MatchExistingEntity returns the indexed entity an extraction most likely
// describes, or nil when it is genuinely new. Identity is resolved by ID, then
// name, then contextual state: entity type, location, and role tags mentioned in
// the extraction itself.
func MatchExistingEntity(store *storage.Store, raw *ExtractedEntity) *entity.Entity {
	if store == nil || raw == nil {
		return nil
	}

	if raw.ID != "" {
		if ent, err := store.GetEntity(raw.ID); err == nil && ent != nil {
			return ent
		}
	}

	nameKey := entity.Slugify(raw.Name)
	if nameKey == "" {
		return nil
	}

	summaries, err := store.ListEntities()
	if err != nil {
		return nil
	}

	for _, summary := range summaries {
		if summary.ID == raw.ID || summary.ID == nameKey || entity.Slugify(summary.Name) == nameKey {
			if ent, err := store.GetEntity(summary.ID); err == nil && ent != nil {
				return ent
			}
		}
	}

	for _, summary := range summaries {
		if sharesNameTokens(nameKey, summary.ID) || sharesNameTokens(nameKey, entity.Slugify(summary.Name)) {
			if ent, err := store.GetEntity(summary.ID); err == nil && ent != nil {
				return ent
			}
		}
	}

	return matchEntityByContext(store, summaries, raw)
}

// sharesNameTokens reports whether one kebab-case name is token-wise contained
// in the other, so "evelyn" and "lady-evelyn" resolve to the same person.
func sharesNameTokens(a, b string) bool {
	if a == "" || b == "" || a == b {
		return false
	}

	shorter, longer := a, b
	if len(b) < len(a) {
		shorter, longer = b, a
	}
	if len(shorter) < minSharedNameLength {
		return false
	}

	longerTokens := strings.Split(longer, "-")
	for _, token := range strings.Split(shorter, "-") {
		if token == "" {
			continue
		}
		if !slices.Contains(longerTokens, token) {
			return false
		}
	}
	return true
}

// matchEntityByContext handles renames and role descriptions: an entity of the
// same type sharing a location and at least one role tag, or otherwise matching
// on several role tags, is treated as the same entity.
func matchEntityByContext(store *storage.Store, summaries []storage.EntitySummary, raw *ExtractedEntity) *entity.Entity {
	locationKey := entity.Slugify(entity.WikilinkTarget(raw.Location))
	if locationKey == "" {
		locationKey = entity.Slugify(entity.WikilinkTarget(raw.Faction))
	}
	haystack := strings.ToLower(raw.Name + " " + raw.Body)

	bestScore := 0
	bestID := ""
	for _, summary := range summaries {
		if raw.Type != "" && summary.Type != raw.Type {
			continue
		}

		roleHits := 0
		for _, tag := range summary.Tags {
			tagKey := strings.ToLower(strings.TrimSpace(tag))
			if tagKey != "" && strings.Contains(haystack, tagKey) {
				roleHits++
			}
		}
		locationMatched := locationKey != "" && entity.Slugify(entity.WikilinkTarget(summary.Location)) == locationKey

		switch {
		case roleHits == 0:
			continue
		case !locationMatched && roleHits < 2:
			continue
		}

		score := roleHits
		if locationMatched {
			score++
		}
		if score > bestScore {
			bestScore = score
			bestID = summary.ID
		}
	}

	if bestID == "" {
		return nil
	}
	if ent, err := store.GetEntity(bestID); err == nil && ent != nil {
		return ent
	}
	return nil
}

// MergeExtractedEntity folds new narrative detail into an existing entity. The
// authored identity, voice, state, and file hash are preserved so the index
// stays in step with the Markdown entity on disk.
func MergeExtractedEntity(existing *entity.Entity, raw *ExtractedEntity) *entity.Entity {
	merged := *existing

	if merged.Name == "" {
		merged.Name = raw.Name
	}
	if merged.Type == "" {
		merged.Type = raw.Type
	}
	if merged.Location == "" {
		merged.Location = raw.Location
	}
	if merged.Faction == "" {
		merged.Faction = raw.Faction
	}
	if merged.Appearance == "" {
		merged.Appearance = raw.Appearance
	}

	body := strings.TrimSpace(raw.Body)
	if body != "" && !strings.Contains(merged.Body, body) {
		if strings.TrimSpace(merged.Body) == "" {
			merged.Body = body
		} else {
			merged.Body = strings.TrimSpace(merged.Body) + "\n\n" + body
		}
	}

	return &merged
}

const extractorSystemPrompt = `You are a world-state extractor. Read the narrative turn and return a JSON object. Format:
{
  "entities": [
    {
      "id": "kebab-case-id",
      "name": "Full Name",
      "type": "character|location|item|faction|arc",
      "location": "[[Optional-Location]]",
      "appearance": "How this place looks right now, when it has visibly changed.",
      "body": "Description and known facts."
    }
  ],
  "dialogue": [
    { "speaker": "Full Name", "text": "Exactly what they said." }
  ]
}
List every line of direct speech in "dialogue", attributed to the speaker, using the same names as the entity list. Return empty arrays when nothing new is discovered.

If the narration moves the player to a different place, set "player_location" to a [[wikilink]] of that location; otherwise omit it.`

// Extract asks the model for the entities and attributed speech a turn contains.
// Persisting them is the caller's job.
func (e *Extractor) Extract(ctx context.Context, narrativeOutput string) (*Extraction, error) {
	req := GenerateRequest{
		System: extractorSystemPrompt,
		Prompt: narrativeOutput,
	}

	res, err := e.model.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("extractor model failed: %w", err)
	}

	cleaned := strings.TrimSpace(res.Text)
	if idx := strings.IndexAny(cleaned, "[{"); idx != -1 {
		cleaned = cleaned[idx:]
	}
	if idx := strings.LastIndexAny(cleaned, "]}"); idx != -1 {
		cleaned = cleaned[:idx+1]
	}

	if strings.HasPrefix(cleaned, "{") {
		var result Extraction
		if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
			return nil, fmt.Errorf("parse extracted json %q: %w", cleaned, err)
		}
		return &result, nil
	}

	// Older prompts asked for a bare array of entities.
	var entities []ExtractedEntity
	if err := json.Unmarshal([]byte(cleaned), &entities); err != nil {
		return nil, fmt.Errorf("parse extracted json %q: %w", cleaned, err)
	}
	return &Extraction{Entities: entities}, nil
}

// ResolveEntityMentions returns the entities a turn touched, tagged by how. It
// needs no model, so a zero-GPU game still records involvement.
func ResolveEntityMentions(store *storage.Store, playerID, locationID string, texts ...string) []entity.Mention {
	if store == nil {
		return nil
	}

	mentions := make([]entity.Mention, 0)
	seen := make(map[string]bool, 4)

	add := func(id, kind string) {
		if id == "" || seen[id] {
			return
		}
		if ent, err := store.GetEntity(id); err != nil || ent == nil {
			return
		}
		seen[id] = true
		mentions = append(mentions, entity.Mention{ID: id, Kind: kind})
	}

	add(playerID, entity.MentionPlayer)
	add(locationID, entity.MentionLocation)

	for _, text := range texts {
		for _, target := range entity.WikilinkTargets(text) {
			add(ResolveSpeakerID(store, target), entity.MentionWikilink)
		}
	}
	return mentions
}

// ResolveSpeakerID maps a written speaker name to an entity ID using identity
// signals only: exact ID, exact name, then partial name tokens.
func ResolveSpeakerID(store *storage.Store, name string) string {
	cleaned := entity.WikilinkTarget(name)
	if store == nil || cleaned == "" {
		return ""
	}

	slug := entity.Slugify(cleaned)
	if slug == "" {
		return ""
	}

	if ent, err := store.GetEntity(slug); err == nil && ent != nil {
		return ent.ID
	}

	summaries, err := store.ListEntities()
	if err != nil {
		return ""
	}

	for _, summary := range summaries {
		if entity.Slugify(summary.Name) == slug {
			return summary.ID
		}
	}
	for _, summary := range summaries {
		if sharesNameTokens(slug, summary.ID) || sharesNameTokens(slug, entity.Slugify(summary.Name)) {
			return summary.ID
		}
	}
	return ""
}
