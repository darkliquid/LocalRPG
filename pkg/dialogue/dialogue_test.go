package dialogue

import (
	"reflect"
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
