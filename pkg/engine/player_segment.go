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
	text = strings.Trim(text, "\"' \t\r\n")
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

// isPlayerSpeech reports whether a turn segment is a speech beat by the protagonist.
func isPlayerSpeech(s entity.TurnSegment, playerID, playerName string) bool {
	if s.Kind != entity.SegmentSpeech {
		return false
	}
	if s.Player {
		return true
	}
	if s.SpeakerID != "" && (s.SpeakerID == playerID || strings.EqualFold(s.SpeakerID, playerID)) {
		return true
	}
	if s.Speaker != "" && strings.EqualFold(s.Speaker, playerName) {
		return true
	}
	return false
}

// attachPlayerSegment ensures the player's spoken line leads the turn segments.
// If the generated segments already contain a speech beat spoken by the player,
// it tags that segment with Player: true rather than prepending a duplicate.
func attachPlayerSegment(segments []entity.TurnSegment, mode, input, playerID, playerName string) []entity.TurnSegment {
	beat := playerSegment(mode, input, playerID, playerName)
	if beat == nil {
		return segments
	}

	for i := range segments {
		if isPlayerSpeech(segments[i], playerID, playerName) {
			segments[i].Player = true
			segments[i].SpeakerID = playerID
			segments[i].Speaker = playerName
			return segments
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
