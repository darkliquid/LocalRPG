# Mechanics Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Read and write a system's whole `mechanics` block from the Systems Studio.

**Architecture:** The studio DTOs carry `*core.MechanicsSpec`; `GetSystem`/`SaveSystem` map it; a new `MechanicsEditor.tsx` edits stats, skills, health, checks (with profiles), advancement, freeform state, and engagement; validation runs client-side with server-side warnings as the backstop.

**Tech Stack:** React 19 + Tailwind v4; Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-mechanics-editor-design.md`
**Depends on:** SYS-2.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- A system with no mechanics saved unedited must leave `system.yaml` unchanged.
- `noUnusedLocals`/`noUnusedParameters` are on.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The DTO fields and the service mapping

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`
- Test: `pkg/gui/system_mechanics_test.go`

**Interfaces:**
- Consumes: `core.MechanicsSpec`, `core.LoadSystemManifest`, the `system.yaml` writer.
- Produces: `SystemDetailDTO.Mechanics`, `CreateSystemRequestDTO.Mechanics`.

- [ ] **Step 1: Write the failing test**

```go
func TestSystemMechanicsRoundTrip(t *testing.T) {
	svc := newTestService(t)
	req := CreateSystemRequestDTO{
		ID: "test_sys", Name: "Test", Version: "1",
		Mechanics: &core.MechanicsSpec{Stats: []core.StatSpec{{ID: "might"}}},
	}
	if _, err := svc.SaveSystem(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetSystem(context.Background(), "test_sys")
	if err != nil {
		t.Fatal(err)
	}
	if got.Mechanics == nil || len(got.Mechanics.Stats) != 1 {
		t.Fatalf("mechanics = %+v", got.Mechanics)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestSystemMechanicsRoundTrip -v`
Expected: FAIL, `Mechanics` unknown.

- [ ] **Step 3: Write minimal implementation**

Add `Mechanics *core.MechanicsSpec \`json:"mechanics,omitempty"\`` to both DTOs. In `GetSystem`
(`pkg/gui/service.go:4011-4041`), set it from the loaded manifest's `Mechanics`. In `SaveSystem`
(`pkg/gui/service.go:4043-4097`), set `manifest.Mechanics = req.Mechanics` before marshalling, so a
nil value writes no `mechanics` key.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestSystemMechanicsRoundTrip -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/system_mechanics_test.go
git commit -m "feat(gui): expose the system mechanics block"
```

---

### Task 2: The frontend types

**Files:**
- Modify: `frontend/src/types.ts`

**Interfaces:**
- Consumes: `core.MechanicsSpec` and its nested types.
- Produces: `MechanicsSpec`, `StatSpec`, `SkillSpec`, `HealthSpec`, `CheckConventions`, `ResolutionProfile`, `AdvancementSpec`, `UnlockSpec`, `EffectSpec`, `LevelSpec`.

- [ ] **Step 1: Add the types**

Mirror the Go structs field-for-field, with optional fields where Go uses `omitempty`. Add
`mechanics?: MechanicsSpec;` to `SystemDetail` and `CreateSystemRequest`.

- [ ] **Step 2: Typecheck**

Run: `npx tsc --noEmit` (in `frontend/`)
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/types.ts
git commit -m "feat(frontend): type the system mechanics schema"
```

---

### Task 3: Stats and skills editors

**Files:**
- Create: `frontend/src/components/MechanicsEditor.tsx`
- Create: `frontend/src/components/mechanics/RowList.tsx`
- Test: `frontend/src/components/MechanicsEditor.test.tsx`

**Interfaces:**
- Consumes: the TS types (Task 2).
- Produces: `<MechanicsEditor mechanics onChange />` with stats and skills sections.

- [ ] **Step 1: Write the failing test**

```tsx
test("adds and removes a stat", () => {
  const onChange = vi.fn();
  render(<MechanicsEditor mechanics={{}} onChange={onChange} />);
  fireEvent.click(screen.getByText(/add stat/i));
  const next = onChange.mock.calls.at(-1)[0];
  expect(next.stats).toHaveLength(1);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- MechanicsEditor`
Expected: FAIL, module not found.

- [ ] **Step 3: Write minimal implementation**

`RowList` renders a labelled list with add/remove and a render prop per row. `MechanicsEditor`
mounts a stats list (`id, label, type, default, min, max`) and a skills list (`id, label, stat`
where `stat` is a dropdown of declared stat ids).

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- MechanicsEditor`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/MechanicsEditor.tsx frontend/src/components/mechanics/RowList.tsx frontend/src/components/MechanicsEditor.test.tsx
git commit -m "feat(frontend): edit system stats and skills"
```

---

### Task 4: Health and checks editors

**Files:**
- Modify: `frontend/src/components/MechanicsEditor.tsx`
- Test: `frontend/src/components/MechanicsEditor.test.tsx` (append)

**Interfaces:**
- Consumes: Task 3, the profile schema (SYS-2).
- Produces: health and checks sections, including a profiles sub-editor.

- [ ] **Step 1: Write the failing tests**

```tsx
test("edits the outcome vocabulary order", () => { /* … */ });
test("adds a difficulty", () => { /* … */ });
test("adds a pbta profile with a ladder step", () => { /* … */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- MechanicsEditor`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Health: `stat` and `max_stat` dropdowns and a `zero_effect` text field. Checks: a notation field, an
ordered outcome-vocabulary list (best first, reorderable), a difficulties list, and a profiles
sub-editor. Each profile picks a shape (ladder, DC, pool, blades) and edits its fields; the editor
keeps only the chosen shape's fields to avoid a contradictory profile.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- MechanicsEditor`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/MechanicsEditor.tsx frontend/src/components/MechanicsEditor.test.tsx
git commit -m "feat(frontend): edit health, checks, and resolution profiles"
```

---

### Task 5: Advancement, freeform, and engagement

**Files:**
- Modify: `frontend/src/components/MechanicsEditor.tsx`
- Test: `frontend/src/components/MechanicsEditor.test.tsx` (append)

**Interfaces:**
- Consumes: Task 3.
- Produces: an advancement section, a freeform toggle, and an engagement selector.

- [ ] **Step 1: Write the failing tests**

```tsx
test("toggles allow_freeform_state", () => { /* … */ });
test("sets the engagement level", () => { /* … */ });
test("adds an unlock with an effect", () => { /* … */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- MechanicsEditor`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Advancement: currency, mode (`spend`/`track`/`threshold`), earn rules, unlocks (each with effects),
and levels, matching `pkg/core/mechanics.go:22-73`. A freeform-state toggle and an engagement
`off`/`auto`/`ask` selector complete the block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- MechanicsEditor`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/MechanicsEditor.tsx frontend/src/components/MechanicsEditor.test.tsx
git commit -m "feat(frontend): edit advancement, freeform state, and engagement"
```

---

### Task 6: Mount the editor and validate on save

**Files:**
- Modify: `frontend/src/components/SystemsStudio.tsx` (mount `MechanicsEditor`)
- Modify: `pkg/gui/service.go` (`SaveSystem` validation)
- Test: `pkg/gui/system_mechanics_test.go` (append)

**Interfaces:**
- Consumes: Tasks 3-5.
- Produces: the editor visible in the studio; server-side warnings for an invalid block.

- [ ] **Step 1: Write the failing test**

```go
func TestSaveSystemWarnsOnBadMechanics(t *testing.T) {
	svc := newTestService(t)
	req := CreateSystemRequestDTO{ID: "bad", Name: "Bad", Version: "1",
		Mechanics: &core.MechanicsSpec{Skills: []core.SkillSpec{{ID: "stealth", Stat: "missing"}}}}
	res, err := svc.SaveSystem(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("a skill naming an unknown stat should warn")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestSaveSystemWarnsOnBadMechanics -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Mount `<MechanicsEditor>` as a Mechanics tab in `SystemsStudio.tsx`. In `SaveSystem`, run the
validation from the spec §4.3 and append findings to the returned `Warnings`, reusing
`CheckConventions.Validate` (SYS-2) and a small id/reference check.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/SystemsStudio.tsx pkg/gui/service.go pkg/gui/system_mechanics_test.go
git commit -m "feat: mount the mechanics editor and validate on save"
```

---

### Task 7: Verification

- [ ] **Step 1: Round-trip guard**

Add a test that a `system.yaml` with every mechanics field set round-trips byte-identically through
`GetSystem`/`SaveSystem`, and that a system with no mechanics writes no key.

- [ ] **Step 2: Typecheck and full suite**

Run: `npx tsc --noEmit`, `npm run test` (in `frontend/`), and `mise run test:backend`
Expected: PASS.

- [ ] **Step 3: Confirm the acceptance criteria**

- The studio reads and writes the whole mechanics block.
- Invalid ids, references, and profiles are caught.
- An unedited system with no mechanics is unchanged.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the mechanics round trip"
```
