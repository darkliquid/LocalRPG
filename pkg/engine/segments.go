package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/dialogue"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// buildTurnSegments resolves the ordered playback script for a turn: the
// narration split into narration and speech, with extractor attributions applied
// where the prose parse found none. The player's own line is not a segment
// because the chronicle already shows their submitted action, and repeating it
// reads as a duplicate.
func buildTurnSegments(store *storage.Store, narration string, extraction harness.Extraction) []entity.TurnSegment {
	segments := make([]entity.TurnSegment, 0)

	// A character introduced in this very turn is not in the index yet: extraction
	// is persisted by RecordTurn, after segments are built. Resolving against the
	// proposed entities too is what lets a new NPC's first line be attributed
	// instead of falling back to narration.
	resolve := func(candidate string) (string, bool) {
		if id := harness.ResolveSpeakerID(store, candidate); id != "" {
			return id, true
		}
		return proposedSpeakerID(extraction.Entities, candidate)
	}

	for _, segment := range dialogue.Parse(narration, resolve) {
		if !segment.IsSpeech {
			segments = append(segments, entity.TurnSegment{Kind: entity.SegmentNarration, Text: segment.Text})
			continue
		}
		segments = append(segments, entity.TurnSegment{
			Kind:      entity.SegmentSpeech,
			Speaker:   segment.Speaker,
			SpeakerID: segment.SpeakerID,
			Text:      segment.Text,
		})
	}

	return mergeAttributions(segments, extraction.Dialogue, resolve)
}

// proposedSpeakerID resolves a speaker against entities the extractor is about to
// create.
func proposedSpeakerID(entities []harness.ExtractedEntity, candidate string) (string, bool) {
	slug := entity.Slugify(entity.WikilinkTarget(candidate))
	if slug == "" {
		return "", false
	}
	for i := range entities {
		proposed := entities[i]
		if proposed.ID != slug && entity.Slugify(proposed.Name) != slug {
			continue
		}
		if proposed.ID != "" {
			return proposed.ID, true
		}
		if id := entity.Slugify(proposed.Name); id != "" {
			return id, true
		}
	}
	return "", false
}

// mergeAttributions splits narration spans so model-attributed speech is recorded
// even when the prose did not follow the `Name: "…"` convention.
func mergeAttributions(segments []entity.TurnSegment, attributions []harness.ExtractedDialogue, resolve func(string) (string, bool)) []entity.TurnSegment {
	for _, attribution := range attributions {
		text := strings.TrimSpace(attribution.Text)
		speaker := strings.TrimSpace(attribution.Speaker)
		if text == "" || speaker == "" {
			continue
		}

		speakerID, ok := resolve(speaker)
		if !ok {
			continue
		}

		for i, segment := range segments {
			if segment.Kind != entity.SegmentNarration || !strings.Contains(segment.Text, text) {
				continue
			}

			idx := strings.Index(segment.Text, text)
			before := strings.TrimSpace(strings.Trim(segment.Text[:idx], "\"“”"))
			after := strings.TrimSpace(strings.Trim(segment.Text[idx+len(text):], "\"“”"))

			replacement := make([]entity.TurnSegment, 0, 3)
			if before != "" {
				replacement = append(replacement, entity.TurnSegment{Kind: entity.SegmentNarration, Text: before})
			}
			replacement = append(replacement, entity.TurnSegment{
				Kind:      entity.SegmentSpeech,
				Speaker:   speaker,
				SpeakerID: speakerID,
				Text:      text,
			})
			if after != "" {
				replacement = append(replacement, entity.TurnSegment{Kind: entity.SegmentNarration, Text: after})
			}

			merged := make([]entity.TurnSegment, 0, len(segments)+2)
			merged = append(merged, segments[:i]...)
			merged = append(merged, replacement...)
			merged = append(merged, segments[i+1:]...)
			segments = merged
			break
		}
	}
	return segments
}

// speechMentions returns the speakers of a segment list, deduplicated.
func speechMentions(segments []entity.TurnSegment) []entity.Mention {
	mentions := make([]entity.Mention, 0)
	seen := make(map[string]bool)
	for _, segment := range segments {
		if segment.SpeakerID == "" || seen[segment.SpeakerID] {
			continue
		}
		seen[segment.SpeakerID] = true
		mentions = append(mentions, entity.Mention{ID: segment.SpeakerID, Kind: entity.MentionSpeech})
	}
	return mentions
}
