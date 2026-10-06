package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/jsonrepair"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
)

type ExtractedEntity struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Type              string `json:"type"`
	Location          string `json:"location,omitempty"`
	Faction           string `json:"faction,omitempty"`
	Appearance        string `json:"appearance,omitempty"`
	Age               string `json:"age,omitempty"`
	AppearanceChanged bool   `json:"appearance_changed,omitempty"`
	Body              string `json:"body"`
}

// ExtractedDialogue is one utterance the model attributed to a speaker.
type ExtractedDialogue struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

// ExtractedSceneBreak represents a major temporal leap or scene transition.
type ExtractedSceneBreak struct {
	Occurred  bool   `json:"occurred"`
	VisualCue string `json:"visual_cue,omitempty"`
}

// Extraction is everything one extraction pass returned for a turn.
type Extraction struct {
	Entities       []ExtractedEntity    `json:"entities"`
	Dialogue       []ExtractedDialogue  `json:"dialogue,omitempty"`
	PlayerLocation string               `json:"player_location,omitempty"`
	SceneBreak     *ExtractedSceneBreak `json:"scene_break,omitempty"`
}

type Extractor struct {
	model         ModelProvider
	voiceProfiles []config.VoiceProfile
	id            string
	logger        trace.Logger
	recorder      UsageRecorder
	key           provider.Key
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

// SetUsageRecorder installs the sink extraction usage is reported to.
func (e *Extractor) SetUsageRecorder(rec UsageRecorder) { e.recorder = rec }

// SetProviderKey names the canonical key extraction usage is recorded under, so
// it agrees with the role's own spend.
func (e *Extractor) SetProviderKey(key provider.Key) { e.key = key }

// inferGender attempts to deduce a character's gender from gender fields, state, or prose pronouns.
func inferGender(ent *entity.Entity) string {
	if ent == nil {
		return ""
	}
	if g := strings.ToLower(strings.TrimSpace(ent.Gender)); g != "" {
		return normalizeGender(g)
	}
	if ent.State != nil {
		if raw, ok := ent.State.Get("gender"); ok {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				return normalizeGender(strings.ToLower(strings.TrimSpace(s)))
			}
		}
	}
	// Heuristic pronoun inference
	text := strings.ToLower(strings.Join([]string{ent.Name, ent.Body, ent.Appearance}, " "))
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z')
	})
	maleScore, femaleScore := 0, 0
	for _, w := range words {
		switch w {
		case "he", "him", "his", "himself", "man", "boy", "sir", "lord", "brother", "father", "king", "prince":
			maleScore++
		case "she", "her", "hers", "herself", "woman", "girl", "lady", "sister", "mother", "queen", "princess":
			femaleScore++
		}
	}
	if maleScore > femaleScore && maleScore > 0 {
		return "male"
	}
	if femaleScore > maleScore && femaleScore > 0 {
		return "female"
	}
	return ""
}

func normalizeGender(raw string) string {
	switch raw {
	case "m", "male", "masculine", "man":
		return "male"
	case "f", "female", "feminine", "woman":
		return "female"
	case "neutral", "nonbinary", "non-binary", "agender":
		return "neutral"
	default:
		return raw
	}
}

// filterProfilesByGender filters profiles strictly by matching gender tag.
func filterProfilesByGender(profiles []config.VoiceProfile, gender string) []config.VoiceProfile {
	if gender == "" {
		return profiles
	}
	var matched []config.VoiceProfile
	for _, p := range profiles {
		hasMale, hasFemale, hasNeutral := false, false, false
		for _, tag := range p.Tags {
			t := strings.ToLower(tag)
			if t == "male" {
				hasMale = true
			} else if t == "female" {
				hasFemale = true
			} else if t == "neutral" || t == "nonbinary" {
				hasNeutral = true
			}
		}
		if gender == "male" && hasMale && !hasFemale {
			matched = append(matched, p)
		} else if gender == "female" && hasFemale && !hasMale {
			matched = append(matched, p)
		} else if (gender == "neutral" || gender == "non-binary") && (hasNeutral || (!hasMale && !hasFemale)) {
			matched = append(matched, p)
		}
	}
	if len(matched) == 0 {
		// Fallback to full list if no profiles exist for this gender
		return profiles
	}
	return matched
}

func AssignVoiceProfile(ent *entity.Entity, profiles []config.VoiceProfile) {
	if len(profiles) == 0 || ent == nil || !entity.IsCharacterType(ent.Type) || ent.Voice != nil {
		return
	}

	gender := inferGender(ent)
	candidateProfiles := filterProfilesByGender(profiles, gender)

	searchContent := strings.ToLower(strings.Join([]string{
		ent.Name,
		ent.Body,
		ent.Appearance,
		strings.Join(ent.Aliases, " "),
	}, " "))

	// 1. Check direct profile ID match within candidate set
	for _, p := range candidateProfiles {
		if strings.Contains(searchContent, strings.ToLower(p.ID)) {
			ent.Voice = voiceFromProfile(p)
			return
		}
	}

	// 2. Score by tag matches within candidate set
	bestScore := 0
	var bestProfile *config.VoiceProfile
	for i := range candidateProfiles {
		score := 0
		for _, tag := range candidateProfiles[i].Tags {
			if strings.Contains(searchContent, strings.ToLower(tag)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestProfile = &candidateProfiles[i]
		}
	}

	if bestProfile != nil {
		ent.Voice = voiceFromProfile(*bestProfile)
		return
	}

	// 3. Deterministic hash fallback within candidate set
	h := fnv.New32a()
	h.Write([]byte(ent.ID))
	idx := int(h.Sum32()) % len(candidateProfiles)
	p := candidateProfiles[idx]
	ent.Voice = voiceFromProfile(p)
}

// voiceFromProfile is the one place a profile becomes a voice, so a provider
// tunable an operator authored travels with the character.
func voiceFromProfile(profile config.VoiceProfile) *entity.VoiceConfig {
	return &entity.VoiceConfig{
		Provider:   profile.Provider,
		VoiceID:    profile.VoiceID,
		Pitch:      profile.Pitch,
		SpeechRate: profile.SpeechRate,
		Options:    profile.Options,
	}
}

const minSharedNameLength = 4

// aliasMatches reports whether a candidate names an entity through one of its
// aliases. Aliases are matched after IDs and names, never before: a note's own name
// is always the stronger signal.
func aliasMatches(candidate string, aliases []string) bool {
	slug := entity.Slugify(entity.WikilinkTarget(candidate))
	if slug == "" {
		return false
	}
	for _, alias := range aliases {
		if entity.Slugify(alias) == slug {
			return true
		}
	}
	return false
}

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
		if aliasMatches(raw.Name, summary.Aliases) {
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
	if raw.AppearanceChanged || merged.Appearance == "" {
		if strings.TrimSpace(raw.Appearance) != "" {
			merged.Appearance = raw.Appearance
		}
	}
	if strings.TrimSpace(raw.Age) != "" {
		merged.Age = raw.Age
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
      "appearance": "Visual physical traits or how this place looks right now.",
      "age": "Apparent or stated age if character.",
      "appearance_changed": true,
      "body": "Description and known facts."
    }
  ],
  "dialogue": [
    { "speaker": "Full Name", "text": "Exactly what they said." }
  ],
  "scene_break": {
    "occurred": true,
    "visual_cue": "Concise visual description of the new scene moment after a temporal jump or dramatic change of setting."
  }
}
Set "appearance_changed": true ONLY when an existing character or location has noticeably altered in appearance or age (e.g. time skip, aging, scars, transformation, haircuts).
If the turn includes a significant temporal leap (such as '10 years later'), scene break, or dramatic change of setting, populate "scene_break"; otherwise omit it.
List every line of direct speech in "dialogue", attributed to the speaker, using the same names as the entity list. Return empty arrays when nothing new is discovered.

If the narration moves the player to a different place, set "player_location" to a [[wikilink]] of that location; otherwise omit it.`

// Extract asks the model for the entities and attributed speech a turn contains.
// Persisting them is the caller's job.
func (e *Extractor) Extract(ctx context.Context, narrativeOutput string) (*Extraction, error) {
	req := GenerateRequest{
		System: extractorSystemPrompt,
		Prompt: narrativeOutput,
	}

	e.logger = trace.OrNil(e.logger)
	e.logger.Event("extraction.request", map[string]interface{}{
		"role":         e.id,
		"prompt":       narrativeOutput,
		"prompt_chars": len([]rune(narrativeOutput)),
	})

	res, err := e.model.Generate(ctx, req)
	if err != nil {
		e.logger.Event("provider.error", map[string]interface{}{"role": e.id, "error": err.Error()})
		return nil, fmt.Errorf("extractor model failed: %w", err)
	}
	if res != nil && res.Usage != nil && e.recorder != nil {
		if e.key != "" {
			res.Usage.Provider = string(e.key)
		}
		e.recorder.RecordUsage("extractor", *res.Usage)
	}

	raw := []byte(strings.TrimSpace(res.Text))
	shape := jsonrepair.Repair(raw)
	if !shape.OK {
		return nil, fmt.Errorf("extractor returned no valid JSON")
	}

	if bytes.HasPrefix(shape.Payload, []byte("{")) {
		var result Extraction
		if err := decodeExtractedJSON(raw, &result); err != nil {
			return nil, fmt.Errorf("parse extracted json %q: %w", raw, err)
		}
		e.logger.Event("extraction.result", map[string]interface{}{
			"role":            e.id,
			"entities":        len(result.Entities),
			"dialogue":        len(result.Dialogue),
			"player_location": result.PlayerLocation,
		})
		return &result, nil
	}

	// Older prompts asked for a bare array of entities.
	var entities []ExtractedEntity
	if err := decodeExtractedJSON(raw, &entities); err != nil {
		return nil, fmt.Errorf("parse extracted json %q: %w", raw, err)
	}
	return &Extraction{Entities: entities}, nil
}

// decodeExtractedJSON decodes a JSON value a model returned, repairing fences,
// surrounding prose, and unclosed structure first.
func decodeExtractedJSON(payload []byte, v interface{}) error {
	res := jsonrepair.Repair(payload)
	if !res.OK {
		return fmt.Errorf("parse extracted json: not valid JSON")
	}
	if err := json.Unmarshal(res.Payload, v); err != nil {
		return fmt.Errorf("parse extracted json: %w", err)
	}
	return nil
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
		if entity.Slugify(summary.Name) == slug || aliasMatches(cleaned, summary.Aliases) {
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

// ResolveSpeakerVoice finds the configured voice for a speaker reference, which
// may be an entity ID or a written display name. It returns nil when no entity or
// no voice matches, so callers fall back to the narrator voice.
func ResolveSpeakerVoice(store *storage.Store, speakerRef string) *entity.VoiceConfig {
	if store == nil || strings.TrimSpace(speakerRef) == "" {
		return nil
	}
	if ent, err := store.GetEntity(speakerRef); err == nil && ent != nil {
		return ent.Voice
	}
	if id := ResolveSpeakerID(store, speakerRef); id != "" {
		if ent, err := store.GetEntity(id); err == nil && ent != nil {
			return ent.Voice
		}
	}
	return nil
}

const (
	// proseNameMinRunes is the shortest whole name worth searching for.
	proseNameMinRunes = 4
	// proseWordMinRunes is the shortest single word of a name worth searching for on
	// its own, which is how "Guard Kael" is found when the prose says only "Kael".
	proseWordMinRunes = 4
)

// ResolveProseMentions finds known characters the given texts name in prose. It is
// how a turn records involvement when extraction is disabled, when extraction
// misses someone, or when the character neither spoke nor was linked.
//
// Names are matched as they are written, because prose capitalises proper nouns and
// a description is not a name: "Kael waits by the water" finds Guard Kael, while
// "The woman said nothing" does not find a character called The Woman. It is
// deterministic on purpose: no model call, so it is cheap enough for every turn.
func ResolveProseMentions(store *storage.Store, texts ...string) []entity.Mention {
	if store == nil || len(texts) == 0 {
		return nil
	}

	joined := strings.Join(texts, "\n")
	if strings.TrimSpace(joined) == "" {
		return nil
	}

	summaries, err := store.ListEntities()
	if err != nil {
		return nil
	}

	mentions := make([]entity.Mention, 0)
	for _, summary := range summaries {
		if summary.Type != "character" {
			continue
		}

		if name := strings.TrimSpace(summary.Name); len([]rune(name)) >= proseNameMinRunes {
			if containsWord(joined, name) {
				mentions = append(mentions, entity.Mention{ID: summary.ID, Kind: entity.MentionProse})
				continue
			}
		}

		// A titled name is also known by its own words: Guard Kael is called Kael far
		// more often than he is called by his title. Only the last word and the
		// longest are tried, because trying every word would match a title used
		// generically, and "the guard waits" is not evidence that Kael is present.
		for _, word := range nameWords(summary.Name) {
			if containsWord(joined, word) {
				mentions = append(mentions, entity.Mention{ID: summary.ID, Kind: entity.MentionProse})
				break
			}
		}
	}
	return mentions
}

// containsWord reports whether the text contains the word with boundaries, so
// "Kael" does not match "Kaeldrin". The comparison is case-sensitive, so a name is
// only found where it is used as one.
func containsWord(text, word string) bool {
	index := 0
	for {
		found := strings.Index(text[index:], word)
		if found == -1 {
			return false
		}
		found += index

		beforeOK := found == 0 || !isNameRune(rune(text[found-1]))
		after := found + len(word)
		afterOK := after >= len(text) || !isNameRune(rune(text[after]))
		if beforeOK && afterOK {
			return true
		}
		index = found + len(word)
	}
}

// isNameRune reports whether a rune can be part of a name, which is what makes the
// boundary check meaningful. Case is deliberately not folded here.
func isNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '\'' || r == '-'
}

// nameWords returns the words of a name that could stand for it alone, most
// distinctive first: the last word, then the longest. Capitalisation is preserved,
// because a name is matched as it is written.
func nameWords(name string) []string {
	words := make([]string, 0, 2)
	for _, word := range strings.FieldsFunc(name, func(r rune) bool {
		return !isNameRune(r)
	}) {
		if len([]rune(word)) < proseWordMinRunes {
			continue
		}
		words = append(words, word)
	}
	if len(words) == 0 {
		return nil
	}

	last := words[len(words)-1]
	longest := words[0]
	for _, word := range words {
		if len([]rune(word)) > len([]rune(longest)) {
			longest = word
		}
	}
	if longest == last {
		return []string{last}
	}
	return []string{last, longest}
}
