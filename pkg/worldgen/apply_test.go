package worldgen

import (
	"strings"
	"testing"
)

func TestApplyLoreAppends(t *testing.T) {
	got := ApplyLore("# Lore\n\nOld.\n", []Enhancement{{Kind: "lore", Title: "History", Body: "New."}})
	if !strings.Contains(got, "Old.") || !strings.Contains(got, "New.") {
		t.Fatalf("lore = %q", got)
	}
	if !strings.HasPrefix(got, "# Lore\n\nOld.") {
		t.Fatalf("the original prose must be untouched: %q", got)
	}
}

func TestApplyLoreInsertsUnderATargetSection(t *testing.T) {
	doc := "# Lore\n\nA.\n\n## History\n\nOld history.\n\n## Themes\n\nB.\n"
	got := ApplyLore(doc, []Enhancement{{Kind: "lore", Title: "Deep History", Body: "Older.", Target: "History"}})
	history := strings.Index(got, "## History")
	added := strings.Index(got, "Deep History")
	themes := strings.Index(got, "## Themes")
	if !(history < added && added < themes) {
		t.Fatalf("the addition should land inside History: %q", got)
	}
}

func TestApplyHooksCreatesTheSection(t *testing.T) {
	got := ApplyHooks("# Lore\n", []Enhancement{{Kind: "hook", Title: "The debt", Body: "A creditor."}})
	if !strings.Contains(got, "## Hooks") || !strings.Contains(got, "The debt") {
		t.Fatalf("lore = %q", got)
	}
}

func TestApplyHooksReusesAnExistingSection(t *testing.T) {
	doc := "# Lore\n\n## Hooks\n\n### Old\n\nSomething.\n"
	got := ApplyHooks(doc, []Enhancement{{Kind: "hook", Title: "New", Body: "Else."}})
	if strings.Count(got, "## Hooks") != 1 {
		t.Fatalf("the Hooks section should not be duplicated: %q", got)
	}
	if !strings.Contains(got, "The debt") && !strings.Contains(got, "New") {
		t.Fatalf("the hook should be present: %q", got)
	}
}

func TestApplyLoreWithNoProposalsIsUnchanged(t *testing.T) {
	doc := "# Lore\n\nOld.\n"
	if got := ApplyLore(doc, nil); got != doc {
		t.Fatalf("lore = %q, want unchanged", got)
	}
	if got := ApplyHooks(doc, nil); got != doc {
		t.Fatalf("hooks = %q, want unchanged", got)
	}
	if got := ApplyLore(doc, []Enhancement{{Kind: "hook", Title: "x", Body: "y"}}); got != doc {
		t.Fatalf("a hook must not change the lore: %q", got)
	}
	if got := ApplyHooks(doc, []Enhancement{{Kind: "lore", Title: "x", Body: "y"}}); got != doc {
		t.Fatalf("lore must not change the hooks: %q", got)
	}
}

func TestApplyLoreOnAnEmptyDocument(t *testing.T) {
	got := ApplyLore("", []Enhancement{{Kind: "lore", Title: "History", Body: "New."}})
	if !containsAll(got, "## History", "New.") {
		t.Fatalf("lore = %q", got)
	}
}
