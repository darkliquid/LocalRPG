package scene

import (
	"fmt"
	"strings"
	"time"
)

// Captions renders a campaign's compiled beats as a WebVTT document. One cue is
// emitted per spoken beat, with the speaker as a voice tag, and narration and
// scene cards produce no cue: the narration is the prose, not speech. The timing
// comes from the compiled beats, so a cue and the beat it belongs to agree.
func Captions(beats []Beat) string {
	var builder strings.Builder
	builder.WriteString("WEBVTT\n\n")

	start := time.Duration(0)
	cue := 0
	for _, beat := range beats {
		duration := beat.Duration
		if beat.Kind == BeatSpeech && strings.TrimSpace(beat.Text) != "" {
			cue++
			fmt.Fprintf(&builder, "%d\n%s --> %s\n%s\n\n",
				cue,
				vttTimestamp(start),
				vttTimestamp(start+duration),
				vttCueText(beat),
			)
		}
		start += duration
	}
	return builder.String()
}

// vttCueText is a beat's cue: the spoken line with the speaker as a WebVTT voice
// tag, so a viewer knows who speaks.
func vttCueText(beat Beat) string {
	text := vttSafe(beat.Text)
	speaker := vttSafe(strings.TrimSpace(beat.Speaker))
	if speaker == "" {
		return text
	}
	return "<v " + speaker + ">" + text
}

// vttSafe makes text safe for a cue: a newline would split the cue and a literal
// "-->" would look like a timestamp.
func vttSafe(text string) string {
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	return strings.ReplaceAll(text, "-->", "\u2192")
}

// vttTimestamp formats a duration as WebVTT's hh:mm:ss.mmm.
func vttTimestamp(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	hours := int(d / time.Hour)
	minutes := int(d/time.Minute) % 60
	seconds := int(d/time.Second) % 60
	millis := int(d/time.Millisecond) % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}
