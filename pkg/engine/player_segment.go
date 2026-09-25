package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// playerSegment returns the speech beat for the player's own utterance, or nil
// when the input is an action rather than speech. It renders and plays exactly
// like any other speech beat; the Player flag lets the chronicle skip the
// duplicate action block that would otherwise print the same words.
func playerSegment(mode, input, playerID, playerName string) *entity.TurnSegment {
	if !strings.EqualFold(strings.TrimSpace(mode), "say") {
		return nil
	}
	text := strings.TrimSpace(input)
	if text == "" {
		return nil
	}
	return &entity.TurnSegment{
		Kind:      entity.SegmentSpeech,
		Speaker:   playerName,
		SpeakerID: playerID,
		Text:      text,
		Player:    true,
	}
}

// attachPlayerSegment ensures the player's spoken line leads the turn segments.
// If the generated segments already begin with a speech beat matching the player's
// utterance, it tags that segment with Player: true rather than prepending a duplicate.
func attachPlayerSegment(segments []entity.TurnSegment, mode, input, playerID, playerName string) []entity.TurnSegment {
	beat := playerSegment(mode, input, playerID, playerName)
	if beat == nil {
		return segments
	}

	cleanInput := strings.Trim(beat.Text, "\"' \t\r\n")

	if len(segments) > 0 {
		first := &segments[0]
		if first.Kind == entity.SegmentSpeech {
			speakerMatch := first.SpeakerID == playerID ||
				strings.EqualFold(first.SpeakerID, playerID) ||
				strings.EqualFold(first.Speaker, playerName)
			cleanFirst := strings.Trim(first.Text, "\"' \t\r\n")
			textMatch := strings.EqualFold(cleanFirst, cleanInput)

			if speakerMatch && textMatch {
				first.Player = true
				first.SpeakerID = playerID
				first.Speaker = playerName
				return segments
			}
		}
	}

	return append([]entity.TurnSegment{*beat}, segments...)
}

// playerDisplayName resolves the protagonist's written name, falling back to
// their id when the note cannot be read.
func (o *TurnOrchestrator) playerDisplayName() string {
	if o.store != nil {
		if ent, err := o.store.GetEntity(o.playerID); err == nil && ent != nil && strings.TrimSpace(ent.Name) != "" {
			return ent.Name
		}
	}
	return o.playerID
}
