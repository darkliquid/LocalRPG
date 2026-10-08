# Draft World Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A review surface that accepts or rejects a generated draft's sections and entities and commits the accepted set.

**Architecture:** `Draft` gains lore sections and a draft store under `worlds/.drafts/`; commit/discard endpoints write the accepted set atomically; a `WorldDraftReview` component reuses the entity editor.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-draft-world-review-design.md`
**Depends on:** WG-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- A draft lives only in `worlds/.drafts/`; never in `worlds/<id>/`.
- Commit is atomic (temp directory plus rename).
- Conventional Commits, subject under 72 chars.

---

### Task 1: Sections and the draft store

**Files:**
- Modify: `pkg/worldgen/worldgen.go`
- Create: `pkg/worldgen/store.go`
- Test: `pkg/worldgen/store_test.go`

**Interfaces:**
- Consumes: `Draft`.
- Produces: `DraftSection`, `Draft.Sections`, `func SaveDraft(dir string, d Draft) error`, `func LoadDraft(dir, id string) (Draft, error)`, `func DeleteDraft(dir, id string) error`, `func SplitLoreSections(lore string) []DraftSection`.

- [x] **Step 1: Write the failing tests**

```go
func TestSplitLoreSections(t *testing.T) {
	s := SplitLoreSections("# Lore\n\nA.\n\n## History\n\nB.\n")
	if len(s) != 2 || s[1].Title != "History" {
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
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/worldgen/ -run 'TestSplitLore|TestDraftStore' -v`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

Add `DraftSection` and `Draft.Sections`, `SplitLoreSections` (split on headings), and the store under
`worlds/.drafts/<id>.yaml`.

- [x] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/worldgen/ -run 'TestSplitLore|TestDraftStore' -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/worldgen/worldgen.go pkg/worldgen/store.go pkg/worldgen/store_test.go
git commit -m "feat(worldgen): add draft sections and a draft store"
```

---

### Task 2: Commit a new world

**Files:**
- Create: `pkg/gui/draft_review.go`
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/types.go`
- Test: `pkg/gui/draft_review_test.go`

**Interfaces:**
- Consumes: `LoadDraft`, `writeWorld`, `SaveWorldEntity`.
- Produces: `POST /api/world/draft/commit`, `POST /api/world/draft/discard`, `Service.CommitDraft(ctx, req DraftCommitRequestDTO) (WorldSummaryDTO, error)`.

- [x] **Step 1: Write the failing tests**

```go
func TestCommitDraftWritesOnlyAccepted(t *testing.T) { /* a rejected entity is absent */ }
func TestCommitDraftIsAtomic(t *testing.T) { /* a write failure leaves no world dir */ }
func TestCommitEmptySetIsRefused(t *testing.T) { /* an empty accepted set errors */ }
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestCommitDraft -v`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

Build the accepted lore (the accepted sections joined), create the world directory in a temp location,
write `world.yaml`, `prompts/lore.md`, and the accepted entities, then rename into place. Delete the
draft on success.

- [x] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestCommitDraft -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): commit a reviewed draft world"
```

---

### Task 3: Commit into an existing world

**Files:**
- Modify: `pkg/gui/draft_review.go`
- Test: `pkg/gui/draft_review_test.go` (append)

**Interfaces:**
- Consumes: `writeWorld`, `SaveWorldEntity`, WG-2's clash handling.
- Produces: the same commit endpoint with a target world id.

- [x] **Step 1: Write the failing tests**

```go
func TestCommitIntoExistingAppends(t *testing.T) { /* lore is appended, not replaced */ }
func TestCommitIntoExistingRefusesAClash(t *testing.T) { /* an existing entity id is refused */ }
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestCommitIntoExisting -v`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

When the request names an existing world, append the accepted lore sections, write the accepted
entities, and refuse an id clash with a clear error.

- [x] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestCommitIntoExisting -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): merge a draft into an existing world"
```

---

### Task 4: The review component

**Files:**
- Create: `frontend/src/components/WorldDraftReview.tsx`
- Modify: `frontend/src/components/WorldsStudio.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `frontend/src/components/WorldDraftReview.test.tsx`

**Interfaces:**
- Consumes: the draft, the commit/discard endpoints, the existing entity editor.
- Produces: `<WorldDraftReview draft onCommit onDiscard />`.

- [x] **Step 1: Write the failing tests**

```tsx
test("counts accepted items", () => {
  render(<WorldDraftReview draft={draft({ sections: [s1, s2], entities: [e1] })} onCommit={() => {}} onDiscard={() => {}} />);
  expect(screen.getByText(/3 of 3 accepted/i)).toBeInTheDocument();
});
test("rejecting disables commit when all are rejected", () => { /* … */ });
test("editing an entity updates it", () => { /* reuses the entity editor */ });
```

- [x] **Step 2: Run tests to verify they fail**

Run: `npm run test -- WorldDraftReview`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

Render the header, the lore sections, and the entity list with accept/reject toggles and the counts.
Editing opens the existing entity editor. Commit sends the accepted set; discard deletes the draft.

- [x] **Step 4: Run tests to verify it passes**

Run: `npm run test -- WorldDraftReview`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): review a draft world"
```

---

### Task 5: Verification

- [x] **Step 1: Regression guard**

Add a test that committing a fully-accepted WG-1 draft reproduces the world WG-1 intended.

- [x] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [x] **Step 3: Confirm the acceptance criteria**

- The review renders sections and entities with counts.
- Commit writes only the accepted set, atomically.
- An existing-world commit appends and refuses a clash.
- Discard deletes the draft; a reload restores an unfinished one.

- [x] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the draft commit round trip"
```
