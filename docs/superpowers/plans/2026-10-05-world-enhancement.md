# World Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Propose lore additions, new entities, and story hooks for an existing world as an accept-or-reject diff.

**Architecture:** `pkg/worldgen.Enhance` reads the world and returns capped proposals; endpoints return them and apply only the accepted set; the studio renders the diff.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-world-enhancement-design.md`
**Depends on:** WG-1, WG-2.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- Lore is appended, never replaced.
- Applying no proposals leaves every file unchanged.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The enhancement type and `Enhance`

**Files:**
- Create: `pkg/worldgen/enhance.go`
- Test: `pkg/worldgen/enhance_test.go`

**Interfaces:**
- Consumes: `Generator`, `WorldContext`, WG-2's link step.
- Produces: `Enhancement`, `func Enhance(ctx, Generator, WorldContext, instruction string, kinds []string) ([]Enhancement, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestEnhanceReturnsCappedProposals(t *testing.T) {
	g := &jsonGen{responses: []string{`{"proposals":[
	  {"kind":"lore","title":"History","body":"Long ago..."},
	  {"kind":"hook","title":"The debt","body":"A creditor arrives."}]}`}}
	got, err := Enhance(context.Background(), g, WorldContext{ID: "w"}, "deepen it", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Kind != "lore" || got[1].Kind != "hook" {
		t.Fatalf("proposals = %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/worldgen/ -run TestEnhance -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the type and `Enhance`: build a context prompt (reusing WG-2's bounded assembly), call the
generator, parse into proposals, cap the list, and link any entity proposal against the world.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/worldgen/ -run TestEnhance -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/worldgen/enhance.go pkg/worldgen/enhance_test.go
git commit -m "feat(worldgen): propose world enhancements"
```

---

### Task 2: Applying lore and hooks

**Files:**
- Create: `pkg/worldgen/apply.go`
- Test: `pkg/worldgen/apply_test.go`

**Interfaces:**
- Consumes: `Enhancement`.
- Produces: `func ApplyLore(existing string, proposals []Enhancement) string`, `func ApplyHooks(existing string, proposals []Enhancement) string`.

- [ ] **Step 1: Write the failing tests**

```go
func TestApplyLoreAppends(t *testing.T) {
	got := ApplyLore("# Lore\n\nOld.\n", []Enhancement{{Kind: "lore", Title: "History", Body: "New."}})
	if !strings.Contains(got, "Old.") || !strings.Contains(got, "New.") {
		t.Fatalf("lore = %q", got)
	}
}
func TestApplyHooksCreatesTheSection(t *testing.T) {
	got := ApplyHooks("# Lore\n", []Enhancement{{Kind: "hook", Title: "The debt", Body: "A creditor."}})
	if !strings.Contains(got, "## Hooks") || !strings.Contains(got, "The debt") {
		t.Fatalf("lore = %q", got)
	}
}
func TestApplyLoreWithNoProposalsIsUnchanged(t *testing.T) { /* identical string */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/worldgen/ -run TestApply -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Append lore sections under their target heading or at the end; ensure a `## Hooks` section exists and
append hooks beneath it. Both are pure string functions.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/worldgen/ -run TestApply -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/worldgen/apply.go pkg/worldgen/apply_test.go
git commit -m "feat(worldgen): apply lore and hook proposals"
```

---

### Task 3: The endpoints

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`, `pkg/gui/types.go`
- Test: `pkg/gui/world_enhance_test.go`

**Interfaces:**
- Consumes: `Enhance`, `ApplyLore`, `ApplyHooks`, `SaveWorldEntity`.
- Produces: `POST /api/world/{id}/enhance` and `POST /api/world/{id}/enhance/apply`.

- [ ] **Step 1: Write the failing tests**

```go
func TestEnhanceWritesNothing(t *testing.T) { /* the world dir is unchanged */ }
func TestEnhanceApplyWritesOnlyAccepted(t *testing.T) { /* a rejected proposal is absent */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestEnhance -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

The enhance endpoint returns the proposals. The apply endpoint takes the accepted set, applies lore
and hooks to `prompts/lore.md`, writes entity proposals, and saves. Applying an empty set is a no-op.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestEnhance -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): enhance a world through a diff"
```

---

### Task 4: The studio diff

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `frontend/src/components/WorldEnhanceDialog.test.tsx`

**Interfaces:**
- Consumes: the endpoints (Task 3).
- Produces: an Enhance action with per-proposal accept/reject.

- [ ] **Step 1: Write the failing test**

```tsx
test("accepts and rejects proposals", () => {
  render(<WorldEnhanceDialog worldId="w" proposals={[
    { kind: "lore", title: "History", body: "…", reason: "fills a gap" },
    { kind: "hook", title: "The debt", body: "…" }]} onApply={() => {}} onClose={() => {}} />);
  fireEvent.click(screen.getByRole("button", { name: /reject.*hook/i }));
  fireEvent.click(screen.getByRole("button", { name: /apply/i }));
  // Assert only the lore proposal was applied.
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- WorldEnhanceDialog`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add an Enhance action on a world that collects an instruction, shows the proposals as cards with
accept/reject toggles and their reason, and applies the accepted set.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- WorldEnhanceDialog`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): review world enhancements"
```

---

### Task 5: Verification

- [ ] **Step 1: No-op guard**

Add a test that applying no proposals leaves `lore.md` byte-identical.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Enhance proposes lore, entities, and hooks, capped.
- Lore is appended, hooks land under `## Hooks`.
- Only accepted proposals are written.
- Existing entities are untouched.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the no-proposal path"
```
