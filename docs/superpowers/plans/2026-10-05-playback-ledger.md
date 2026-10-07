# Playback Ledger Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One heard registry with resume offsets, shared by server and client, and one playback owner.

**Architecture:** A `Ledger` keyed by clip with offsets; the server owns the turn's ledger and merges the client's offsets by max; the client records offsets on pause and seeks on replay; an `owner` field disables the non-owning path.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-playback-ledger-design.md`
**Depends on:** RB-2.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- The merge is a max and idempotent.
- A client that sends no offsets still works.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The ledger

**Files:**
- Create: `pkg/gui/ledger.go`
- Test: `pkg/gui/ledger_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Ledger`, `Entry`, `func (l *Ledger) Record(key string, playedMS, totalMS int, complete bool)`, `func (l *Ledger) Merge(other map[string]Entry)`, `func (l *Ledger) Entry(key string) (Entry, bool)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestLedgerRecordAndMerge(t *testing.T) {
	l := NewLedger()
	l.Record("a", 500, 2000, false)
	l.Merge(map[string]Entry{"a": {PlayedMS: 900, TotalMS: 2000}})
	e, _ := l.Entry("a")
	if e.PlayedMS != 900 {
		t.Fatalf("played = %d, want the max 900", e.PlayedMS)
	}
}
func TestLedgerCompleteWins(t *testing.T) {
	l := NewLedger()
	l.Record("a", 2000, 2000, true)
	l.Merge(map[string]Entry{"a": {PlayedMS: 100}})
	e, _ := l.Entry("a")
	if !e.Complete {
		t.Fatal("a complete entry must not be downgraded")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestLedger -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the type and the record/merge/entry methods; merge takes the max offset and keeps `Complete`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestLedger -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/ledger.go pkg/gui/ledger_test.go
git commit -m "feat(gui): add the playback ledger"
```

---

### Task 2: The server uses offsets

**Files:**
- Modify: `pkg/gui/turn_audio.go`, `pkg/gui/service.go`
- Test: `pkg/gui/turn_audio_test.go` (append)

**Interfaces:**
- Consumes: `Ledger` (Task 1), RB-2's heard set.
- Produces: the finalise pass consulting offsets.

- [ ] **Step 1: Write the failing test**

```go
func TestFinaliseSkipsACompleteClip(t *testing.T) {
	// A clip recorded complete is not enqueued at finalise.
}
func TestFinaliseEmitsAPartialClipOnce(t *testing.T) {
	// A partially heard clip is not fully re-enqueued.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestFinalise -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Replace RB-2's heard-segment set with the ledger; at finalise, skip a complete clip and, for a
partial one, apply RB-2's suppression and record the offset for the client to resume.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestFinalise -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): use offsets in the finalise pass"
```

---

### Task 3: The client records offsets and resumes

**Files:**
- Modify: `frontend/src/hooks/useStreamedSpeech.ts`, `frontend/src/hooks/useSegmentPlayback.ts`, `frontend/src/lib/audio.ts`
- Test: `frontend/src/hooks/useStreamedSpeech.test.ts`

**Interfaces:**
- Consumes: the ledger (mirrored client-side).
- Produces: a partial play recorded and a replay seeking.

- [ ] **Step 1: Write the failing tests**

```tsx
test("records an offset on pause", () => { /* the ledger entry has playedMS > 0 */ });
test("resumes from the offset", () => { /* play sets currentTime to the offset */ });
test("marks complete on ended", () => { /* the entry is complete */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- useStreamedSpeech`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Replace the "only on ended" rule with an offset record on pause/abort; the play path reads the entry
and seeks before playing; `onended` marks complete.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- useStreamedSpeech`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): resume a partially heard clip"
```

---

### Task 4: The owner and reconciliation

**Files:**
- Modify: `pkg/gui/streaming_tts.go`, `pkg/gui/types.go`, `pkg/gui/service.go`
- Modify: `frontend/src/App.tsx`, `frontend/src/hooks/useStreamedSpeech.ts`
- Test: `pkg/gui/ledger_test.go` (append), `frontend/src/App.test.tsx` (append)

**Interfaces:**
- Consumes: the ledger, the playback events.
- Produces: an `owner` field and a client offset resend.

- [ ] **Step 1: Write the failing tests**

```go
func TestOwnerFlips(t *testing.T) { /* setting the owner to device disables the browser path */ }
```
```tsx
test("browser player is disabled when the device owns playback", () => { /* no browser play */ });
test("resends offsets at the handover", () => { /* the server receives the client ledger */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestOwner -v` and `npm run test -- App`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `owner` to the playback events; the server sets it when device playback begins and ends; the client
disables the non-owning path; at the handover the client posts its offsets and the server merges.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestOwner -v` and `npm run test -- App`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat: give playback one owner and reconcile offsets"
```

---

### Task 5: Verification

- [ ] **Step 1: End-to-end test**

Add a test that a streamed-then-finalised turn plays each clip once, resuming a partial clip.

- [ ] **Step 2: Regression guard**

Add a test that a simple turn's playback is unchanged.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- The ledger merges by max and keeps complete.
- A complete clip never replays; a partial clip resumes.
- One owner disables the other path.
- Offsets reconcile at the handover and on reconnect.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: guard the simple-turn playback"
```
