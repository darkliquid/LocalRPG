package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
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

// applyRecords decodes the stream's control records into the turn's
// declarations: personae, memories, state changes, and a location move. A record
// that failed to parse is logged and skipped, so a malformed declaration never
// costs the turn.
func (o *TurnOrchestrator) applyRecords() ([]harness.PersonaDecl, []harness.MemoryDecl, []harness.StateChangeDecl, string) {
	if o.parser == nil {
		return nil, nil, nil, ""
	}
	var personae []harness.PersonaDecl
	var memories []harness.MemoryDecl
	var stateChanges []harness.StateChangeDecl
	moveRef := ""
	for _, record := range o.parser.Records() {
		if record.Err != nil {
			o.logger.Event("turn.record_error", map[string]interface{}{"type": record.Type, "error": record.Err.Error()})
			continue
		}
		switch record.Type {
		case turnstream.RecordPersona:
			if decl, err := record.DecodePersona(); err == nil {
				personae = append(personae, decl)
			}
		case turnstream.RecordMemory:
			if memory, err := record.DecodeMemory(); err == nil {
				memories = append(memories, memory)
			}
		case turnstream.RecordState:
			if change, err := record.DecodeState(); err == nil {
				stateChanges = append(stateChanges, change)
			}
		case turnstream.RecordMove:
			if location, err := record.DecodeMove(); err == nil && strings.TrimSpace(location) != "" {
				moveRef = location
			}
		}
	}
	return personae, memories, stateChanges, moveRef
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
