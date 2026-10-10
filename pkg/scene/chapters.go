package scene

import (
	"fmt"
	"strings"
	"time"
)

// ChaptersVTT renders a script's chapters as a WebVTT document: one cue per
// chapter, running from its start to the next chapter's, which is what a player's
// `chapters` track reads. total bounds the final chapter when it is the last one.
func ChaptersVTT(chapters []Chapter, total time.Duration) string {
	var builder strings.Builder
	builder.WriteString("WEBVTT\n\n")
	for i, chapter := range chapters {
		end := total
		if i+1 < len(chapters) {
			end = chapters[i+1].Start
		}
		if end <= chapter.Start {
			end = chapter.Start + time.Second
		}
		title := vttSafe(strings.TrimSpace(chapter.Title))
		fmt.Fprintf(&builder, "%s\n%s --> %s\n%s\n\n",
			title,
			vttTimestamp(chapter.Start),
			vttTimestamp(end),
			title,
		)
	}
	return builder.String()
}
