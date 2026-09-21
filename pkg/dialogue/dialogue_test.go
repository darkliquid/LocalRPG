package dialogue

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseAttributesResolvedSpeakers(t *testing.T) {
	known := map[string]string{
		"Garrick the Fence": "garrick-the-fence",
		"Garrick":           "garrick-the-fence",
		"Lady Evelyn Vance": "lady-evelyn",
	}
	resolve := func(candidate string) (string, bool) {
		id, ok := known[candidate]
		return id, ok
	}

	text := "The docks are quiet.\nGarrick the Fence: \"You didn't see me here.\"\nAs you declare: \"I draw my blade.\"\n[[Lady Evelyn Vance|Evelyn]]: \"Later.\""
	segments := Parse(text, resolve)

	want := []Segment{
		{Text: "The docks are quiet."},
		{Speaker: "Garrick the Fence", SpeakerID: "garrick-the-fence", Text: "You didn't see me here.", IsSpeech: true},
		{Text: `As you declare: "I draw my blade."`},
		{Speaker: "Lady Evelyn Vance", SpeakerID: "lady-evelyn", Text: "Later.", IsSpeech: true},
	}
	if !reflect.DeepEqual(segments, want) {
		t.Fatalf("Parse() = %#v, want %#v", segments, want)
	}
}

func TestParseKeepsUnresolvedAndUnattributedProse(t *testing.T) {
	segments := Parse("Someone whispers: \"not me\".\n\nA plain line.", func(string) (string, bool) { return "", false })

	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %#v", segments)
	}
	for _, segment := range segments {
		if segment.IsSpeech {
			t.Errorf("expected no speech segments, got %#v", segment)
		}
	}
}

func TestParseEmptyText(t *testing.T) {
	if segments := Parse("   \n\n", func(string) (string, bool) { return "", false }); len(segments) != 0 {
		t.Errorf("expected no segments for blank text, got %#v", segments)
	}
}

func TestParseAcceptsTheFormsAGMWrites(t *testing.T) {
	resolve := func(candidate string) (string, bool) {
		if candidate == "Garrick the Fence" {
			return "garrick-the-fence", true
		}
		return "", false
	}

	cases := map[string]string{
		`Garrick the Fence: "Plain."`:                              "Plain.",
		`**Garrick the Fence:** "Bold."`:                           "Bold.",
		`*Garrick the Fence:* "Italic."`:                           "Italic.",
		`[[Garrick the Fence]]: "Wikilink."`:                       "Wikilink.",
		`[[Garrick the Fence|Garrick]]: "Labelled."`:               "Labelled.",
		"\u201cGarrick the Fence\u201d: \u201cSmart quotes.\u201d": "Smart quotes.",
	}

	for line, want := range cases {
		segments := Parse(line, resolve)
		if len(segments) != 1 || !segments[0].IsSpeech {
			t.Errorf("%q produced %#v, want one speech segment", line, segments)
			continue
		}
		if segments[0].Text != want || segments[0].SpeakerID != "garrick-the-fence" {
			t.Errorf("%q produced %#v, want %q", line, segments[0], want)
		}
	}
}

func TestParseKeepsProseAfterASpokenLine(t *testing.T) {
	resolve := func(candidate string) (string, bool) { return "garrick-the-fence", true }

	segments := Parse(`Garrick the Fence: "Keep walking." He turns away.`, resolve)

	if len(segments) != 2 {
		t.Fatalf("expected a speech segment and a narration segment, got %#v", segments)
	}
	if !segments[0].IsSpeech || segments[0].Text != "Keep walking." {
		t.Errorf("unexpected speech segment %#v", segments[0])
	}
	if segments[1].IsSpeech || segments[1].Text != "He turns away." {
		t.Errorf("unexpected trailing segment %#v", segments[1])
	}
}

func TestParseRejectsImpossibleSpeakers(t *testing.T) {
	resolve := func(candidate string) (string, bool) { return "", false }

	long := strings.Repeat("a", 70)
	for _, line := range []string{
		long + `: "Too long to be a name."`,
		`: "No name at all."`,
		`Garrick the Fence: "Unterminated.`,
	} {
		segments := Parse(line, resolve)
		for _, segment := range segments {
			if segment.IsSpeech {
				t.Errorf("%q produced a speech segment %#v", line, segment)
			}
		}
	}
}
