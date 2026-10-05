# Theatre Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Advance theatre beats on real completion events, prefetch the next beat, preload images, and retune the beat gap.

**Architecture:** The server emits a playback completion event over SSE; the theatre subscribes and advances, replacing the status poll; a one-beat-ahead prefetch fetches the next clip and preloads its art.

**Tech Stack:** Go standard library (SSE); React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-theatre-performance-design.md`

## Global Constraints

- `noUnusedLocals`/`noUnusedParameters` are on.
- Beat order and audio keys must be unchanged for a fixed turn.
- Prefetch at most one beat ahead.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The server completion event

**Files:**
- Modify: `pkg/media/playback` (completion signal), `pkg/gui/service.go`, `pkg/gui/server.go`, `pkg/gui/types.go`
- Test: `pkg/gui/playback_events_test.go`

**Interfaces:**
- Consumes: the playback player's completion signal.
- Produces: an SSE channel `playback` with `{turn, segment, state}` events.

- [ ] **Step 1: Write the failing test**

```go
func TestPlaybackCompletionEvent(t *testing.T) {
	// Drive the player to finish a clip and assert an event is emitted on the
	// subscription.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestPlaybackCompletionEvent -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Expose a completion callback from the playback player (it already knows when a clip ends) and emit
an SSE event through the existing subscription plumbing, mirroring the `audio_progress` channel.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestPlaybackCompletionEvent -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/playback pkg/gui
git commit -m "feat(gui): emit a playback completion event"
```

---

### Task 2: The client subscription and advance

**Files:**
- Modify: `frontend/src/components/StoryTheater.tsx`, `frontend/src/App.tsx`
- Test: `frontend/src/components/StoryTheater.test.tsx` (append)

**Interfaces:**
- Consumes: the completion event (Task 1).
- Produces: a beat advance on the event, with a safety timeout.

- [ ] **Step 1: Write the failing test**

```tsx
test("advances on a completion event", () => {
  // Render the theatre, emit a completion for the current beat, assert the next
  // beat becomes current.
});

test("advances after the safety timeout if no event arrives", () => { /* … */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- StoryTheater`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Subscribe to the playback SSE stream while the theatre is open; advance the current beat when the
event's segment matches it. Keep a safety timeout (a generous multiple of the clip's expected
duration) that advances a beat whose event never arrives.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- StoryTheater`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/StoryTheater.tsx frontend/src/App.tsx frontend/src/components/StoryTheater.test.tsx
git commit -m "feat(frontend): advance the theatre on playback events"
```

---

### Task 3: Remove the status poll

**Files:**
- Modify: `frontend/src/App.tsx`
- Test: `frontend/src/App.test.tsx` (append)

**Interfaces:**
- Consumes: Task 2.
- Produces: the `/api/audio/status` poll removed.

- [ ] **Step 1: Write the failing test**

```tsx
test("does not poll audio status", () => {
  // Assert no interval fetches /api/audio/status while the theatre is open.
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- App`
Expected: FAIL (the poll still runs).

- [ ] **Step 3: Write minimal implementation**

Delete the poll interval and its state; the completion event and safety timeout replace it. Remove
any now-unused imports and state (`noUnusedLocals` will catch them).

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- App`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/App.tsx frontend/src/App.test.tsx
git commit -m "refactor(frontend): drop the theatre status poll"
```

---

### Task 4: Prefetch and image preload

**Files:**
- Modify: `frontend/src/components/StoryTheater.tsx`
- Test: `frontend/src/components/StoryTheater.test.tsx` (append)

**Interfaces:**
- Consumes: the beat list and the clip/image URLs.
- Produces: a one-beat-ahead prefetch and image preload.

- [ ] **Step 1: Write the failing tests**

```tsx
test("prefetches the next beat's clip", () => { /* … */ });
test("preloads the next beat's image", () => { /* … */ });
test("does not prefetch beyond one beat", () => { /* … */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- StoryTheater`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

When beat N starts, fetch beat N+1's clip URL (a plain `fetch` the browser caches) and preload its
image with `new Image()` when it differs from the current beat's. Bound to one beat ahead.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- StoryTheater`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/StoryTheater.tsx frontend/src/components/StoryTheater.test.tsx
git commit -m "feat(frontend): prefetch the next theatre beat"
```

---

### Task 5: Retune the gap

**Files:**
- Modify: `frontend/src/components/StoryTheater.tsx`
- Test: `frontend/src/components/StoryTheater.test.tsx` (append)

**Interfaces:**
- Consumes: the existing speed control.
- Produces: a reduced `BEAT_GAP_MS` scaled by speed.

- [ ] **Step 1: Write the failing test**

```tsx
test("applies a small scaled inter-beat gap", () => {
  // Assert the gap constant is small and divides by the speed factor.
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- StoryTheater`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Reduce `BEAT_GAP_MS` (for example to 120) and scale it by the speed control, so faster playback
tightens it and slower playback widens it.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- StoryTheater`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/StoryTheater.tsx frontend/src/components/StoryTheater.test.tsx
git commit -m "feat(frontend): retune the theatre beat gap"
```

---

### Task 6: Verification

- [ ] **Step 1: Regression guard**

Add a test asserting the beat order and audio keys for a fixed turn are unchanged.

- [ ] **Step 2: Typecheck and full suite**

Run: `npx tsc --noEmit`, `npm run test` (in `frontend/`), and `mise run test:backend`
Expected: PASS.

- [ ] **Step 3: Confirm the acceptance criteria**

- A beat advances on the completion event.
- No status poll remains.
- The next beat is prefetched and its image preloaded.
- The gap is smaller and speed-scaled.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the theatre beat order"
```
