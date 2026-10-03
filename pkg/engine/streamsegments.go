package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/turnstream"
)

// segmentsFromEvents maps parsed stream events to the turn's playback script.
// Records carry no speech and are skipped here; the orchestrator consumes them
// separately.
func segmentsFromEvents(events []turnstream.Event) []entity.TurnSegment {
	segments := make([]entity.TurnSegment, 0, len(events))
	for _, event := range events {
		switch event.Kind {
		case turnstream.KindSpeech:
			if text := strings.TrimSpace(event.Text); text != "" {
				segments = append(segments, entity.TurnSegment{
					Kind:      entity.SegmentSpeech,
					Speaker:   event.Speaker,
					SpeakerID: event.SpeakerID,
					Text:      text,
				})
			}
		case turnstream.KindNarration:
			if text := strings.TrimSpace(event.Text); text != "" {
				segments = append(segments, entity.TurnSegment{Kind: entity.SegmentNarration, Text: text})
			}
		}
	}
	return segments
}

// stripRecordLines removes control-record lines from prose, so the recorded
// narration is the story rather than the protocol. It leaves every other line,
// including blockquote speech, untouched, and returns the text unchanged when
// there is nothing to remove, so whitespace is never disturbed needlessly.
func stripRecordLines(text string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "@") {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	if !removed {
		return text
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}
