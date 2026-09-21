package entity

import (
	"reflect"
	"strings"
	"testing"
)

func TestEntityHistoryRoundTrip(t *testing.T) {
	doc := `---
id: garrick-the-fence
name: Garrick the Fence
type: character
history: [1, 4, 9]
---
A shadowy broker.
`
	ent, err := ParseMarkdownEntity([]byte(doc))
	if err != nil {
		t.Fatalf("ParseMarkdownEntity failed: %v", err)
	}
	if !reflect.DeepEqual(ent.History, []int{1, 4, 9}) {
		t.Fatalf("History = %v, want [1 4 9]", ent.History)
	}

	serialized, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown failed: %v", err)
	}
	reparsed, err := ParseMarkdownEntity(serialized)
	if err != nil {
		t.Fatalf("reparse failed: %v", err)
	}
	if !reflect.DeepEqual(reparsed.History, ent.History) {
		t.Errorf("History after round trip = %v, want %v", reparsed.History, ent.History)
	}
}

func TestEntityWithoutHistoryOmitsFrontmatter(t *testing.T) {
	ent := &Entity{ID: "hero", Name: "Sean", Type: "character", Body: "A traveller."}
	serialized, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown failed: %v", err)
	}
	if strings.Contains(string(serialized), "history:") {
		t.Errorf("expected no history frontmatter when empty, got %s", serialized)
	}
}
