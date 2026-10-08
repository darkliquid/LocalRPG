package sysgen

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestDraftStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := System{
		ID:          "gritty-d20",
		Name:        "Gritty D20",
		Version:     "1.0.0",
		Description: "A gritty system",
		Mechanics: &core.MechanicsSpec{
			Stats: []core.StatSpec{{ID: "grit", Label: "Grit"}},
		},
		Script:      "onAction('do', function(ctx) {});",
		RulesPrompt: "# Rules\n\nRoll dice.",
		Verify:      VerifyResult{OK: true, Script: true},
	}
	if err := SaveDraft(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDraft(dir, "gritty-d20")
	if err != nil || got.Name != "Gritty D20" {
		t.Fatalf("loaded %+v err %v", got, err)
	}
	if got.Verify.OK != true || got.Script != s.Script {
		t.Fatalf("mismatched fields: %+v", got)
	}
	if err := DeleteDraft(dir, "gritty-d20"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDraft(dir, "gritty-d20"); err == nil {
		t.Fatal("a deleted draft should not load")
	}
}

func TestDraftStoreRefusesATraversalID(t *testing.T) {
	dir := t.TempDir()
	if err := SaveDraft(dir, System{ID: "../escape"}); err == nil {
		t.Fatal("a traversal id must be refused")
	}
}

func TestDraftStoreRejectsAnUnknownDraft(t *testing.T) {
	if _, err := LoadDraft(t.TempDir(), "nope"); err == nil {
		t.Fatal("loading a missing draft should error")
	}
}

func TestSaveDraftNeedsAValidID(t *testing.T) {
	if err := SaveDraft(t.TempDir(), System{}); err == nil {
		t.Fatal("an empty id must be refused")
	}
}

func TestListDraftIDs(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"a", "b"} {
		if err := SaveDraft(dir, System{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := ListDraftIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("ids = %+v", ids)
	}
	if ids, err := ListDraftIDs(t.TempDir() + "/missing"); err != nil || ids != nil {
		t.Fatalf("a missing drafts dir is empty, not an error: %v %v", ids, err)
	}
}
