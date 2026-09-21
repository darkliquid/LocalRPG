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

var attributedSpeakerRegex = regexp.MustCompile(`^([^:\n]+):\s*["“]([^"”]+)["”]`)

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

		if match := attributedSpeakerRegex.FindStringSubmatch(line); len(match) == 3 {
			candidate := entity.WikilinkTarget(strings.TrimSpace(match[1]))
			if id, ok := resolve(candidate); ok {
				segments = append(segments, Segment{
					Speaker:   candidate,
					SpeakerID: id,
					Text:      strings.TrimSpace(match[2]),
					IsSpeech:  true,
				})
				continue
			}
		}

		segments = append(segments, Segment{Text: line})
	}

	return segments
}
