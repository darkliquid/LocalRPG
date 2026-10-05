# Local-First Onboarding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One page that says what can run locally, on what hardware, and what it will be like, reachable from the launcher.

**Architecture:** A new embedded docs article with a capability matrix built from LF-3's tiers, registered with the site generator and the Vale scope, linked from the launcher.

**Tech Stack:** Markdown; the docs lint and Vale.

**Spec:** `docs/superpowers/specs/2026-10-05-local-first-onboarding-design.md`
**Depends on:** LF-3.

## Global Constraints

- Every provider named must exist and its tier must match LF-3.
- The article is Vale-linted; keep the prose clean and use the project's vocabulary.
- Register it in `tools/sitegen/content.go` and `scripts/lint-prose.sh`.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The article

**Files:**
- Create: `pkg/gui/docs/21-local-first.md`

**Interfaces:**
- Consumes: `provider.TierCaveat` strings (LF-3), the provider catalogue, the per-tool articles.
- Produces: the article.

- [ ] **Step 1: Write the short answer and the matrix**

One paragraph stating the app runs fully offline with the basic providers and that better output
needs a local model or a key, then the capability matrix from the spec §4.1 with a provider and a
quality/speed note per cell.

- [ ] **Step 2: Write the tiers section**

Restate LF-3's four tiers and their caveats, using the exact strings from `provider.TierCaveat`.

- [ ] **Step 3: Write the offline preset and links sections**

Describe the one-action offline stack (LF-4) and link the per-tool articles
(`14-local-llm-ollama` … `19-local-stack-docker-compose`).

- [ ] **Step 4: Commit**

```bash
git add pkg/gui/docs/21-local-first.md
git commit -m "docs: add the local-first capability matrix"
```

---

### Task 2: Register and link

**Files:**
- Modify: `tools/sitegen/content.go`, `scripts/lint-prose.sh`
- Modify: `frontend/src/components/LauncherHub.tsx` (or the About/Docs affordance)

**Interfaces:**
- Consumes: the article (Task 1).
- Produces: it rendered on the site, linted, and reachable from the launcher.

- [ ] **Step 1: Register in the site generator and the Vale scope**

Add the article to both lists.

- [ ] **Step 2: Link from the launcher**

Add a link in the new-user path (and the docs list) that opens the article in the existing
`MarkdownDocViewer`.

- [ ] **Step 3: Lint and build**

Run: `mise run lint:docs && mise run lint:prose && mise run site:build`
Expected: PASS and the article appears in `website/dist`.

- [ ] **Step 4: Typecheck**

Run: `npx tsc --noEmit` (in `frontend/`)
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tools/sitegen/content.go scripts/lint-prose.sh frontend/src
git commit -m "docs: render and link the local-first page"
```

---

### Task 3: The tier-consistency test

**Files:**
- Test: `pkg/provider/docs_test.go`

**Interfaces:**
- Consumes: `provider.TierCaveat`, the article file.
- Produces: a test asserting the page contains each tier's label.

- [ ] **Step 1: Write the failing test**

```go
func TestLocalFirstPageMatchesTiers(t *testing.T) {
	page, err := os.ReadFile("../../pkg/gui/docs/21-local-first.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, tier := range []provider.Tier{provider.TierOfflineBasic, provider.TierOfflineNeural, provider.TierLocalServer, provider.TierCloud} {
		if !bytes.Contains(page, []byte(provider.TierCaveat(tier))) {
			t.Errorf("the page is missing the caveat for %s", tier)
		}
	}
}
```

Adjust the path and package to wherever the test lives cleanly.

- [ ] **Step 2: Run it**

Run: `go test ./pkg/provider/ -run TestLocalFirstPageMatchesTiers -v`
Expected: PASS once the page uses the exact caveats.

- [ ] **Step 3: Commit**

```bash
git add pkg/provider/docs_test.go
git commit -m "test: keep the local-first page aligned with the tiers"
```

---

### Task 4: Verification

- [ ] **Step 1: Full docs verification**

Run: `mise run lint:docs && mise run lint:prose && mise run site:build`
Expected: PASS.

- [ ] **Step 2: Confirm the acceptance criteria**

- The page presents the capability matrix.
- Every provider named exists and matches its tier.
- The launcher links to it.
- It renders on the site and passes the lint.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "docs: finalise the local-first page"
```
