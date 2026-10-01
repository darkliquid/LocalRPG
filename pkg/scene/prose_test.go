package scene

import (
	"strings"
	"testing"
)

func plainRuns(blocks []Block) string {
	var b strings.Builder
	for _, block := range blocks {
		for _, run := range block.Runs {
			b.WriteString(run.Text)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestParseProseRendersAWikilinkLabel(t *testing.T) {
	blocks := ParseProse("She met [[lady-evelyn|Lady Evelyn]] there.", DisplayStageDirections)
	if got := plainRuns(blocks); !strings.Contains(got, "Lady Evelyn") || strings.Contains(got, "[[") {
		t.Fatalf("wikilink not unwrapped: %q", got)
	}
	run := blocks[0].Runs[1]
	if run.Style != StyleLink {
		t.Errorf("wikilink style = %v, want StyleLink", run.Style)
	}
}

func TestParseProseAppliesTheInlineStyles(t *testing.T) {
	blocks := ParseProse("a **bold** *ital* `code` end", DisplayStageDirections)
	styles := map[string]RunStyle{}
	for _, run := range blocks[0].Runs {
		styles[run.Text] = run.Style
	}
	if styles["bold"] != StyleBold || styles["ital"] != StyleItalic || styles["code"] != StyleCode {
		t.Fatalf("styles = %v", styles)
	}
}

func TestParseProseHonoursDisplayModeForDirections(t *testing.T) {
	text := "He paused [whispering] then spoke."

	shown := ParseProse(text, DisplayStageDirections)
	if plainRuns(shown) != "He paused whispering then spoke.\n" {
		t.Fatalf("stage_directions = %q", plainRuns(shown))
	}

	hidden := ParseProse(text, DisplayHidden)
	if strings.Contains(plainRuns(hidden), "whispering") {
		t.Fatalf("hidden kept the direction: %q", plainRuns(hidden))
	}

	raw := ParseProse(text, DisplayRaw)
	if !strings.Contains(plainRuns(raw), "[whispering]") {
		t.Fatalf("raw dropped the brackets: %q", plainRuns(raw))
	}
}

func TestParseProseSplitsParagraphsOnBlankLines(t *testing.T) {
	blocks := ParseProse("one\n\ntwo", DisplayStageDirections)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
}
