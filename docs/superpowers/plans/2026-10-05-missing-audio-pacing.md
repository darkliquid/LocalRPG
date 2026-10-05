# Missing Audio and Pacing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a silent beat visible and pace it by reading speed, in both the theatre and the export.

**Architecture:** A shared `readingDurationMs`; a per-beat audio state (audio/silent/pending) driving a chip and the pacing; the export renderer adopts the same reading rule.

**Tech Stack:** React 19 + Tailwind v4; Go standard library for the renderer.

**Spec:** `docs/superpowers/specs/2026-10-05-missing-audio-pacing-design.md`
**Depends on:** TH-1.

## Global Constraints

- `noUnusedLocals`/`noUnusedParameters` are on.
- A beat with audio must be unchanged.
- The frontend and the Go renderer must use the same reading rule (a shared constant and a golden test).
- Conventional Commits, subject under 72 chars.

---

### Task 1: The shared reading estimate

**Files:**
- Create: `frontend/src/lib/pacing.ts`
- Test: `frontend/src/lib/pacing.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces: `readingDurationMs(text: string, wpm?: number, minMs?: number): number`.

- [ ] **Step 1: Write the failing test**

```ts
import { readingDurationMs } from "./pacing";

test("scales with length and floors at the minimum", () => {
  const short = readingDurationMs("Go.");
  const long = readingDurationMs("A".repeat(0) + "word ".repeat(60));
  expect(short).toBeGreaterThanOrEqual(900);
  expect(long).toBeGreaterThan(short);
});

test("scales by the speed factor", () => {
  const normal = readingDurationMs("word ".repeat(40));
  const fast = readingDurationMs("word ".repeat(40), 400);
  expect(fast).toBeLessThan(normal);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- pacing`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```ts
export function readingDurationMs(text: string, wpm = 200, minMs = 900): number {
  const words = text.trim().split(/\s+/).filter(Boolean).length;
  const ms = (words / wpm) * 60_000;
  return Math.max(minMs, Math.round(ms));
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- pacing`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/pacing.ts frontend/src/lib/pacing.test.ts
git commit -m "feat(frontend): add a reading-speed pacing estimate"
```

---

### Task 2: The beat audio state and the chip

**Files:**
- Modify: `frontend/src/components/StoryTheater.tsx`, `frontend/src/components/theater/TheaterDialogue.tsx`
- Test: `frontend/src/components/StoryTheater.test.tsx` (append)

**Interfaces:**
- Consumes: `segment.audio_urls`, the audio-progress stages.
- Produces: a per-beat state and a "no audio" chip.

- [ ] **Step 1: Write the failing tests**

```tsx
test("a silent beat shows the no-audio chip", () => { /* … */ });
test("a beat with audio shows no chip", () => { /* … */ });
test("a pending beat shows the synthesizing indicator", () => { /* … */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- StoryTheater`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Derive the state from the beat's clip presence and the progress stages, and render the chip or the
indicator on the beat. Reuse the synthesizing indicator styling from `TurnSegments.tsx`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- StoryTheater`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/StoryTheater.tsx frontend/src/components/theater/TheaterDialogue.tsx frontend/src/components/StoryTheater.test.tsx
git commit -m "feat(frontend): show a silent theatre beat"
```

---

### Task 3: Reading-speed pacing in the theatre

**Files:**
- Modify: `frontend/src/components/StoryTheater.tsx`
- Test: `frontend/src/components/StoryTheater.test.tsx` (append)

**Interfaces:**
- Consumes: `readingDurationMs` (Task 1).
- Produces: a silent beat advancing by the estimate.

- [ ] **Step 1: Write the failing test**

```tsx
test("paces a silent beat by reading speed", () => {
  // A silent long beat advances later than a silent short one.
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- StoryTheater`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Replace the fixed silent-beat timer with `readingDurationMs(text)`, scaled by the speed control. For
an audio beat whose completion event is lost, use `max(readingDurationMs(text), expectedClipMs)`
instead of a fixed bound.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- StoryTheater`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/StoryTheater.tsx frontend/src/components/StoryTheater.test.tsx
git commit -m "feat(frontend): pace silent beats by reading speed"
```

---

### Task 4: Export renderer parity

**Files:**
- Modify: `pkg/scene/compile.go` (the silent-beat duration)
- Test: `pkg/scene/compile_test.go` (append)

**Interfaces:**
- Consumes: the reading rule.
- Produces: a silent beat's duration derived from reading speed.

- [ ] **Step 1: Write the failing test**

```go
func TestSilentBeatUsesReadingDuration(t *testing.T) {
	// A silent beat with 40 words is longer than one with 5, and both are >= the floor.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/scene/ -run TestSilentBeatUsesReadingDuration -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `readingDuration(text string, wpm int, minMs int) time.Duration` mirroring the frontend
constants (200 wpm, 900 ms floor) and use it wherever a silent beat currently gets a fixed duration
(`pkg/scene/compile.go:130-162,385-397`). Keep the constants in one place per language and note the
parity in a comment.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/scene/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/scene/compile.go pkg/scene/compile_test.go
git commit -m "feat(scene): pace a silent export beat by reading speed"
```

---

### Task 5: Verification

- [ ] **Step 1: Regression guard**

Add a test asserting a fully-audio turn's timing is unchanged.

- [ ] **Step 2: Golden test**

Add a golden test pinning the reading estimate for a fixed line in both languages.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- A silent beat shows the chip and paces by reading speed.
- A pending beat shows the indicator.
- A beat with audio is unchanged.
- The export paces a silent beat the same way.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: guard the audio-beat timing"
```
