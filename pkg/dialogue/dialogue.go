// Package dialogue splits narration into ordered narration and speech spans,
// attributing speech to speakers the caller can resolve.
package dialogue

import (
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// Segment is one ordered span of a turn.
type Segment struct {
	Speaker   string
	SpeakerID string
	Text      string
	IsSpeech  bool
}

// Emphasis can sit between the colon and the quote, as in `**Name:** "…"`, so the
// separator allows whitespace and emphasis characters.
var attributedSpeakerRegex = regexp.MustCompile(`^([^:\n]+):[\s*_]*["“]([^"”]+)["”](.*)$`)

// maxSpeakerLength bounds a candidate so a sentence cannot masquerade as a name.
const maxSpeakerLength = 64

// Parse splits text into ordered narration and speech segments. resolve maps a
// candidate speaker to an entity ID, reporting whether the speaker is known. A
// candidate that does not resolve stays narration, so prose such as
// `As you declare: "I draw my blade"` never invents a character.
func Parse(text string, resolve func(candidate string) (string, bool)) []Segment {
	segments := make([]Segment, 0)

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if match := attributedSpeakerRegex.FindStringSubmatch(line); len(match) == 4 {
			candidate := cleanSpeaker(match[1])
			if candidate != "" {
				if id, ok := resolve(candidate); ok {
					segments = append(segments, Segment{
						Speaker:   candidate,
						SpeakerID: id,
						Text:      strings.TrimSpace(match[2]),
						IsSpeech:  true,
					})

					// Prose after the closing quote is narration, not lost.
					if rest := strings.TrimSpace(match[3]); rest != "" {
						segments = append(segments, Segment{Text: rest})
					}
					continue
				}
			}
		}

		// Also handle unquoted or loosely quoted `Name: text` lines where the candidate
		// speaker resolves to a known character.
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			candidate := cleanSpeaker(line[:idx])
			if candidate != "" {
				if id, ok := resolve(candidate); ok {
					utterance := strings.TrimSpace(line[idx+1:])
					utterance = strings.Trim(utterance, "\"“”")
					if utterance != "" {
						segments = append(segments, Segment{
							Speaker:   candidate,
							SpeakerID: id,
							Text:      utterance,
							IsSpeech:  true,
						})
						continue
					}
				}
			}
		}

		segments = append(segments, Segment{Text: line})
	}

	return segments
}

// cleanSpeaker strips markdown emphasis and a wikilink label from a candidate,
// returning "" when what remains cannot be a speaker name. Resolution, not
// punctuation, is what rejects sentence fragments, so titles such as
// "Mr. Garrick" survive.
func cleanSpeaker(raw string) string {
	// Wrapping emphasis and quotes are formatting, not part of the name.
	candidate := entity.WikilinkTarget(strings.Trim(strings.TrimSpace(raw), "*_\"'“”‘’"))
	if candidate == "" || len(candidate) > maxSpeakerLength {
		return ""
	}
	return candidate
}
