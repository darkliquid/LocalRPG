package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/dialogue"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// buildTurnSegments resolves the ordered playback script for a turn: the player's
// own utterance in Say mode, then the narration split into narration and speech,
// with extractor attributions applied where the prose parse found none.
func buildTurnSegments(store *storage.Store, mode, playerID, input, narration string, attributions []harness.ExtractedDialogue) []entity.TurnSegment {
	segments := make([]entity.TurnSegment, 0)

	if strings.EqualFold(mode, "Say") && strings.TrimSpace(input) != "" {
		segments = append(segments, entity.TurnSegment{
			Kind:      entity.SegmentSpeech,
			Speaker:   entityName(store, playerID),
			SpeakerID: playerID,
			Text:      strings.TrimSpace(input),
		})
	}

	resolve := func(candidate string) (string, bool) {
		id := harness.ResolveSpeakerID(store, candidate)
		return id, id != ""
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

	return mergeAttributions(segments, attributions, resolve)
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

func entityName(store *storage.Store, id string) string {
	if id == "" || store == nil {
		return ""
	}
	if ent, err := store.GetEntity(id); err == nil && ent != nil {
		return ent.Name
	}
	return id
}
