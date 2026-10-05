# Genre-Aware Fallbacks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One genre palette and copy source used by the launcher, the app background, empty states, loading states, the site, and the export.

**Architecture:** `frontend/src/lib/genre.ts` holds the palette and copy tables; consumers read it; a Go mirror in `pkg/scene` for the export gradient, kept in agreement by a parity test.

**Tech Stack:** React 19 + Tailwind v4; Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-genre-fallbacks-design.md`

## Global Constraints

- `noUnusedLocals`/`noUnusedParameters` are on.
- Palettes must keep contrast for the app's light text.
- The neutral default preserves the current look for a genre-less world.
- The frontend and Go tables must agree.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The genre module

**Files:**
- Create: `frontend/src/lib/genre.ts`
- Test: `frontend/src/lib/genre.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces: `GenrePalette`, `genrePalette(genre?: string): GenrePalette`, `genreCopy(genre?: string): { empty: string; loading: string }`.

- [ ] **Step 1: Write the failing tests**

```ts
import { genrePalette, genreCopy } from "./genre";

test("resolves known genres and defaults", () => {
  expect(genrePalette("cyberpunk").id).toBe("cyberpunk");
  expect(genrePalette("nonsense").id).toBe("neutral");
  expect(genrePalette(undefined).id).toBe("neutral");
});
test("copy has a neutral fallback", () => {
  expect(genreCopy("horror").loading).not.toBe("");
  expect(genreCopy("nonsense").loading).toBe(genreCopy(undefined).loading);
});
test("palettes keep contrast", () => {
  for (const g of ["fantasy", "cyberpunk", "horror", "scifi", "western", "modern", "historical", "neutral"]) {
    expect(contrastRatio(genrePalette(g).from, "#f5f5f4")).toBeGreaterThan(4.5);
  }
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- genre`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the palette and copy tables for the genres, resolve case-insensitively, and default to neutral.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- genre`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/genre.ts frontend/src/lib/genre.test.ts
git commit -m "feat(frontend): add the genre palette and copy source"
```

---

### Task 2: The launcher reads it

**Files:**
- Modify: `frontend/src/components/launcher/ProceduralAsset.tsx`
- Test: `frontend/src/components/launcher/ProceduralAsset.test.tsx` (append)

**Interfaces:**
- Consumes: `genrePalette` (Task 1).
- Produces: the launcher's assets using the shared palette.

- [ ] **Step 1: Write the failing test**

```tsx
test("uses the shared palette for a genre", () => {
  render(<ProceduralBanner genre="cyberpunk" />);
  // The rendered style uses the cyberpunk palette's colours.
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- ProceduralAsset`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Replace the component's local palette table with `genrePalette`, keeping the icon map in the module.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- ProceduralAsset`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/launcher
git commit -m "refactor(frontend): use the shared genre palette in the launcher"
```

---

### Task 3: The app background and banners

**Files:**
- Modify: `frontend/src/App.tsx`, `frontend/src/components/launcher/CampaignHeroStage.tsx`
- Test: `frontend/src/App.test.tsx` (append)

**Interfaces:**
- Consumes: `genrePalette` (Task 1), the current world's genre.
- Produces: a genre-tinted background and genre banners.

- [ ] **Step 1: Write the failing test**

```tsx
test("tints the background by genre", () => { /* the root style reflects the campaign's genre */ });
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- App`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Read the current world's genre and apply the palette's gradient to the root; use the palette for a
banner fallback when none is set.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- App`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): tint the app by genre"
```

---

### Task 4: Empty and loading states

**Files:**
- Modify: the launcher, the chronicle, the codex, and the search empty states; the turn loading indicator
- Test: `frontend/src/components/ChronicleView.test.tsx` (append)

**Interfaces:**
- Consumes: `genreCopy` (Task 1).
- Produces: genre-flavoured copy.

- [ ] **Step 1: Write the failing tests**

```tsx
test("shows a genre empty line", () => { /* the empty chronicle uses genreCopy */ });
test("shows a genre loading message", () => { /* the turn loading uses genreCopy */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- ChronicleView`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Use `genreCopy(genre)` for the empty and loading strings in each surface, keeping the neutral fallback.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- ChronicleView`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): use genre-aware empty and loading copy"
```

---

### Task 5: The Go palette and the site

**Files:**
- Create: `pkg/scene/genre.go`
- Modify: `pkg/scene/theater.go` (the no-art gradient)
- Modify: `tools/sitegen` (the site background)
- Test: `pkg/scene/genre_test.go`, `tools/sitegen/genre_test.go`

**Interfaces:**
- Consumes: the genre table (Task 1).
- Produces: `func GenrePalette(genre string) GenrePalette` in Go, used by the export and the site.

- [ ] **Step 1: Write the failing tests**

```go
func TestGenrePaletteMatchesTheFrontend(t *testing.T) {
	// The Go table's values for each genre equal the frontend's (a shared fixture).
}
func TestExportGradientUsesTheGenre(t *testing.T) {
	// The theatre's no-art background for a cyberpunk campaign differs from fantasy.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/scene/ -run TestGenrePalette -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Mirror the palette table in Go, use it for the export's no-art gradient, and have the site generator
read it for its background. Add a shared fixture (a small JSON) both read in their tests, so a
divergence fails.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/scene/ ./tools/sitegen/ -run TestGenre -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/scene tools/sitegen
git commit -m "feat(scene): use the genre palette in the export and the site"
```

---

### Task 6: Verification

- [ ] **Step 1: Regression guard**

Add a test that a campaign with a banner renders the banner, not the fallback.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- The palette resolves known genres and defaults.
- The launcher, background, banners, empty states, and loading states use it.
- The export and the site use the same values.
- Contrast is preserved.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the banner-set path"
```
