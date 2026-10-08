package worldgen

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestSplitLoreSections(t *testing.T) {
	s := SplitLoreSections("# Lore\n\nA.\n\n## History\n\nB.\n")
	if len(s) != 2 || s[1].Title != "History" {
		t.Fatalf("sections = %+v", s)
	}
	if s[0].Title != "Lore" || s[0].Body != "A." {
		t.Fatalf("first section = %+v", s[0])
	}
	if s[1].Body != "B." {
		t.Fatalf("second section = %+v", s[1])
	}
}

func TestSplitLoreSectionsKeepsAPreamble(t *testing.T) {
	s := SplitLoreSections("An opening line.\n\n## History\n\nB.\n")
	if len(s) != 2 || s[0].Title != "" {
		t.Fatalf("sections = %+v", s)
	}
}

func TestDraftStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	d := Draft{ID: "w", World: core.WorldManifest{Name: "W"}}
	if err := SaveDraft(dir, d); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDraft(dir, "w")
	if err != nil || got.World.Name != "W" {
		t.Fatalf("loaded %+v err %v", got, err)
	}
	if err := DeleteDraft(dir, "w"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDraft(dir, "w"); err == nil {
		t.Fatal("a deleted draft should not load")
	}
}

func TestDraftStoreRoundTripsEntities(t *testing.T) {
	dir := t.TempDir()
	d := Draft{
		ID:       "w",
		World:    core.WorldManifest{ID: "w", Name: "W"},
		Lore:     "# W\n\nA world.\n",
		Entities: []DraftEntity{{ID: "a", Name: "A", Type: "location", Body: "A place."}},
	}
	if err := SaveDraft(dir, d); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDraft(dir, "w")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entities) != 1 || got.Entities[0].ID != "a" || got.Lore != d.Lore {
		t.Fatalf("loaded = %+v", got)
	}
}

func TestDraftStoreRefusesATraversalID(t *testing.T) {
	dir := t.TempDir()
	if err := SaveDraft(dir, Draft{ID: "../escape"}); err == nil {
		t.Fatal("a traversal id must be refused")
	}
}

func TestListDrafts(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"a", "b"} {
		if err := SaveDraft(dir, Draft{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := ListDrafts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("ids = %+v", ids)
	}
	if ids, err := ListDrafts(t.TempDir() + "/missing"); err != nil || ids != nil {
		t.Fatalf("a missing drafts dir is empty, not an error: %v %v", ids, err)
	}
}

func TestRenderEntityNoteParsesBack(t *testing.T) {
	note := RenderEntityNote(DraftEntity{ID: "a", Name: "A", Type: "location", Body: "A place."})
	if !containsAll(note, "---\n", "id: a", "type: location", "A place.") {
		t.Fatalf("note = %q", note)
	}
}

func TestRenderLoreJoinsSections(t *testing.T) {
	got := RenderLore([]DraftSection{{Title: "History", Body: "Long ago."}})
	if !containsAll(got, "## History", "Long ago.") {
		t.Fatalf("lore = %q", got)
	}
	if RenderLore(nil) != "\n" && RenderLore(nil) != "" {
		t.Fatalf("empty lore = %q", RenderLore(nil))
	}
}

func TestDraftStoreRejectsAnUnknownDraft(t *testing.T) {
	if _, err := LoadDraft(t.TempDir(), "nope"); err == nil {
		t.Fatal("loading a missing draft should error")
	}
}

func TestSaveDraftNeedsAValidID(t *testing.T) {
	if err := SaveDraft(t.TempDir(), Draft{}); err == nil {
		t.Fatal("an empty id must be refused")
	}
	_ = context.Background()
}
