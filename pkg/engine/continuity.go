package engine

import (
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// ContinuityFinding is one thing a turn's prose does that canon disagrees with. It
// is advisory: the engine reports, and the player decides.
type ContinuityFinding struct {
	Rule string
	Note string
}

// Rule names, as they appear in the trace and in the sidecar that records which
// findings a player has dealt with.
const (
	RuleUnknownEntity      = "unknown-entity"
	RuleLocationDrift      = "location-drift"
	RuleUnresolvedSpeaker  = "unresolved-speaker"
	RuleReintroduction     = "reintroduction"
	RuleStateContradiction = "state-contradiction"
)

// discoveryCues are the phrasings that introduce someone as if for the first time.
var discoveryCues = []string{"a stranger", "an unfamiliar", "a figure", "a newcomer", "introduces himself", "introduces herself", "you do not recognise"}

// contradictionCues describe a state being false, for the boolean keys a note
// tracks. They are deliberately few: this rule is the most likely to be wrong, and
// a short list keeps it from crying wolf.
var contradictionCues = []string{"cold", "dark", "unlit", "doused", "out", "dead", "extinguished", "guttered"}

// CheckContinuity compares a turn against what the campaign knows. It is
// deterministic and model-free, which is what makes it safe to run every turn.
func CheckContinuity(store *storage.Store, turn *Turn, locationID, playerID string) []ContinuityFinding {
	if store == nil || turn == nil {
		return nil
	}

	findings := make([]ContinuityFinding, 0)
	findings = append(findings, checkUnknownEntities(store, turn)...)
	findings = append(findings, checkLocationDrift(store, turn, locationID)...)
	findings = append(findings, checkUnresolvedSpeakers(store, turn)...)
	findings = append(findings, checkReintroductions(store, turn)...)
	findings = append(findings, checkStateContradictions(store, turn, locationID)...)
	return findings
}

// checkUnknownEntities flags a name that claims canon and has no note. It fires on a
// speaker attribution or a naming construction, never on a capitalised phrase alone:
// scenery is invented legitimately, and flagging it would make the rule noise.
func checkUnknownEntities(store *storage.Store, turn *Turn) []ContinuityFinding {
	findings := make([]ContinuityFinding, 0)

	for _, line := range strings.Split(turn.Narration, "\n") {
		candidate := speakingName(line)
		if candidate == "" {
			continue
		}
		if resolvesToEntity(store, candidate) {
			continue
		}
		findings = append(findings, ContinuityFinding{
			Rule: RuleUnknownEntity,
			Note: fmt.Sprintf("%q speaks but has no note", candidate),
		})
	}

	for _, phrase := range namedPhrases(turn.Narration) {
		if resolvesToEntity(store, phrase) {
			continue
		}
		findings = append(findings, ContinuityFinding{
			Rule: RuleUnknownEntity,
			Note: fmt.Sprintf("%q is named but has no note", phrase),
		})
	}
	return findings
}

// speakingName returns the speaker of a line shaped like `Name: "…"`, or "".
func speakingName(line string) string {
	trimmed := strings.TrimSpace(line)
	colon := strings.Index(trimmed, ":")
	if colon <= 0 {
		return ""
	}

	rest := strings.TrimSpace(trimmed[colon+1:])
	if !strings.HasPrefix(rest, "\"") && !strings.HasPrefix(rest, "“") {
		return ""
	}

	return strings.TrimSpace(entity.WikilinkTarget(strings.Trim(trimmed[:colon], "*_ ")))
}

// namedPhrases returns the names a line claims through a naming construction.
func namedPhrases(text string) []string {
	phrases := make([]string, 0)
	lowered := strings.ToLower(text)

	for _, cue := range []string{"named ", "called ", "known as "} {
		index := 0
		for {
			found := strings.Index(lowered[index:], cue)
			if found == -1 {
				break
			}
			start := index + found + len(cue)

			phrase := ""
			for _, word := range strings.Fields(text[start:]) {
				if word == "" || !isCapitalised(word) {
					break
				}
				phrase = strings.TrimSpace(phrase + " " + strings.Trim(word, ".,;:\"'"))
			}
			if phrase != "" {
				phrases = append(phrases, phrase)
			}
			index = start
		}
	}
	return phrases
}

func isCapitalised(word string) bool {
	runes := []rune(strings.Trim(word, ".,;:\"'"))
	if len(runes) == 0 {
		return false
	}
	return runes[0] >= 'A' && runes[0] <= 'Z'
}

// resolvesToEntity reports whether a name belongs to something the campaign knows,
// by name or by alias.
func resolvesToEntity(store *storage.Store, name string) bool {
	return harness.ResolveSpeakerID(store, name) != ""
}

// checkLocationDrift flags prose that names a known location other than the one the
// party is standing in, because that is the prose teleporting them.
func checkLocationDrift(store *storage.Store, turn *Turn, locationID string) []ContinuityFinding {
	findings := make([]ContinuityFinding, 0)

	summaries, err := store.ListEntities()
	if err != nil {
		return findings
	}

	for _, summary := range summaries {
		if summary.Type != "location" || summary.ID == locationID {
			continue
		}
		if !strings.Contains(turn.Narration, summary.Name) {
			continue
		}
		findings = append(findings, ContinuityFinding{
			Rule: RuleLocationDrift,
			Note: fmt.Sprintf("%q is named while the party is elsewhere", summary.Name),
		})
	}
	return findings
}

// checkUnresolvedSpeakers flags a line shaped like speech whose speaker is unknown.
// The parser leaves such a line in the narration, so this is the only place the
// loss is visible.
func checkUnresolvedSpeakers(store *storage.Store, turn *Turn) []ContinuityFinding {
	findings := make([]ContinuityFinding, 0)

	for _, line := range strings.Split(turn.Narration, "\n") {
		candidate := speakingName(line)
		if candidate == "" || resolvesToEntity(store, candidate) {
			continue
		}
		findings = append(findings, ContinuityFinding{
			Rule: RuleUnresolvedSpeaker,
			Note: fmt.Sprintf("%q speaks but is not a known character", candidate),
		})
	}
	return findings
}

// checkReintroductions flags a known character introduced as if new, which is how a
// model quietly forgets the party has met them.
func checkReintroductions(store *storage.Store, turn *Turn) []ContinuityFinding {
	findings := make([]ContinuityFinding, 0)
	lowered := strings.ToLower(turn.Narration)

	for _, cue := range discoveryCues {
		if !strings.Contains(lowered, cue) {
			continue
		}

		summaries, err := store.ListEntities()
		if err != nil {
			return findings
		}
		for _, summary := range summaries {
			if summary.Type != "character" || summary.Name == "" {
				continue
			}
			if strings.Contains(turn.Narration, summary.Name) {
				findings = append(findings, ContinuityFinding{
					Rule: RuleReintroduction,
					Note: fmt.Sprintf("%q is introduced as new but is already known", summary.Name),
				})
			}
		}
		break
	}
	return findings
}

// checkStateContradictions flags prose that describes a tracked boolean state as
// false. It is scoped to the scene and to those present, because a state mentioned
// elsewhere is not the narrator contradicting anything.
func checkStateContradictions(store *storage.Store, turn *Turn, locationID string) []ContinuityFinding {
	findings := make([]ContinuityFinding, 0)

	ids := []string{locationID, turn.Location}
	if edges, err := store.GetEdgesFrom(locationID); err == nil {
		for _, edge := range edges {
			ids = append(ids, edge.TargetID)
		}
	}

	lowered := strings.ToLower(turn.Narration)
	seen := make(map[string]bool, len(ids))

	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true

		ent, err := store.GetEntity(id)
		if err != nil || ent == nil || ent.State == nil {
			continue
		}

		for key, value := range ent.State.Raw() {
			lit, ok := value.(bool)
			if !ok || !lit {
				continue
			}
			if !strings.Contains(lowered, strings.ToLower(contradictionSubject(key))) {
				continue
			}
			for _, cue := range contradictionCues {
				if strings.Contains(lowered, cue) {
					findings = append(findings, ContinuityFinding{
						Rule: RuleStateContradiction,
						Note: fmt.Sprintf("%q is described as %s while %s says it is true", contradictionSubject(key), cue, ent.Name),
					})
					break
				}
			}
		}
	}
	return findings
}

// contradictionSubject turns a state key into the word prose would use: brazier_lit
// is described by talking about the brazier.
func contradictionSubject(key string) string {
	if index := strings.IndexAny(key, "_-"); index > 0 {
		return key[:index]
	}
	return key
}
