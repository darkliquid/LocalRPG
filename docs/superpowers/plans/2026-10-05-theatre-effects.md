# Theatre Effects Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ken Burns, parallax, a mood tint, and weather overlays, identical in the app and the export.

**Architecture:** Effects are pure functions of beat progress (scale, offset, tint, overlay); the app applies them with CSS, the export renderer with compositing; a parity test keeps them equal.

**Tech Stack:** React 19 + Tailwind v4 (CSS); Go (`pkg/scene`).

**Spec:** `docs/superpowers/specs/2026-10-05-theatre-effects-design.md`
**Depends on:** PH-3, TH-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- Magnitudes are small: scale to ~1.08, translation a few percent.
- A reduced-motion preference disables the motion.
- The app's and the export's transforms must agree.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The effect functions

**Files:**
- Create: `frontend/src/lib/effects.ts`
- Test: `frontend/src/lib/effects.test.ts`

**Interfaces:**
- Consumes: a beat's seed, progress, outcome, weather.
- Produces: `kenBurns(seed, progress) => {scale, dx, dy}`, `moodTint(outcome) => {color, opacity}`, `weatherOverlay(weather) => overlay|null`.

- [ ] **Step 1: Write the failing tests**

```ts
import { kenBurns, moodTint, weatherOverlay } from "./effects";

test("ken burns is bounded and seeded", () => {
  const a = kenBurns(1, 0.5);
  expect(a.scale).toBeGreaterThan(1);
  expect(a.scale).toBeLessThan(1.1);
  expect(kenBurns(1, 0)).toEqual({ scale: 1, dx: 0, dy: 0 });
});
test("mood tint follows the outcome", () => {
  expect(moodTint("miss").opacity).toBeGreaterThan(0);
  expect(moodTint("").opacity).toBe(0);
});
test("weather overlay only for known weather", () => {
  expect(weatherOverlay("rain")).not.toBeNull();
  expect(weatherOverlay("")).toBeNull();
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- effects`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement the three pure functions with the small magnitudes and the outcome/weather tables.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- effects`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/effects.ts frontend/src/lib/effects.test.ts
git commit -m "feat(frontend): add the theatre effect functions"
```

---

### Task 2: The app theatre

**Files:**
- Modify: `frontend/src/components/theater/TheaterStage.tsx`, `StoryTheater.tsx`
- Test: `frontend/src/components/theater/TheaterStage.test.tsx`

**Interfaces:**
- Consumes: the effect functions (Task 1), the beat's progress.
- Produces: Ken Burns, tint, and overlays in the app.

- [ ] **Step 1: Write the failing tests**

```tsx
test("applies a ken burns transform", () => { /* the image style has a scale > 1 */ });
test("applies a mood tint for a miss", () => { /* an overlay element is present */ });
test("shows a rain overlay", () => { /* an overlay element is present */ });
test("respects reduced motion", () => { /* no transform */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- TheaterStage`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Drive the image's CSS transform from `kenBurns` and the beat's progress (a transition or an animation),
draw the tint under the scrim, and draw the weather overlay. Read `prefers-reduced-motion` (or the
app's setting) and skip the motion when set.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- TheaterStage`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components
git commit -m "feat(frontend): animate the theatre"
```

---

### Task 3: The export renderer

**Files:**
- Modify: `pkg/scene/render.go`, `pkg/scene/theater.go`
- Test: `pkg/scene/effects_test.go`

**Interfaces:**
- Consumes: the same effect rules.
- Produces: the export draws the same transform, tint, and overlay.

- [ ] **Step 1: Write the failing tests**

```go
func TestKenBurnsMatchesTheAppRule(t *testing.T) {
	// For a set of progresses, the Go scale/offset equal the documented values.
}
func TestTintAndOverlayDrawn(t *testing.T) { /* a frame for a miss differs from one for a success */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/scene/ -run 'TestKenBurns|TestTintAndOverlay' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement `kenBurns`/`moodTint`/`weatherOverlay` in Go with the same constants, and composite the
image at the interpolated scale and offset, draw the tint, and draw the overlay, per frame.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/scene/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/scene
git commit -m "feat(scene): render the theatre effects in the export"
```

---

### Task 4: Parallax

**Files:**
- Modify: `frontend/src/components/theater/TheaterStage.tsx`, `pkg/scene/render.go`
- Test: `frontend/src/components/theater/TheaterStage.test.tsx` (append), `pkg/scene/effects_test.go` (append)

**Interfaces:**
- Consumes: PH-3's layers.
- Produces: per-layer depth translation.

- [ ] **Step 1: Write the failing tests**

```tsx
test("moves layers at different rates", () => { /* the background offset differs from the foreground */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- TheaterStage`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

When a scene has layers, translate each by the beat progress scaled by its depth; a single-layer image
gets no parallax.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- TheaterStage` and `go test ./pkg/scene/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src pkg/scene
git commit -m "feat: add parallax to layered scenes"
```

---

### Task 5: Verification

- [ ] **Step 1: Parity guard**

Add a test that the app's and the export's effect values agree for a set of progresses.

- [ ] **Step 2: Regression guard**

Add a test that a beat with no layers, outcome, or weather renders as a plain still.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- Ken Burns is bounded and deterministic.
- The tint follows the outcome; the overlay follows the weather.
- Layered scenes parallax.
- Reduced motion disables the motion.
- The app and the export agree.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: guard the plain-still beat"
```
