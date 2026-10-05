# Guaranteed Single-Play Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Guarantee a clip plays at most once per turn by tracking heard segments and skipping fully-heard groups at finalise.

**Architecture:** `turnAudioPlan` gains a heard-segment set fed by the streamer and the plan; `emitTurnClips` skips a group whose segments are all heard; a parity assertion traces a streamed-vs-plan key mismatch; the unframed `Feed` path folds paragraphs so it cannot diverge.

**Tech Stack:** Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-guaranteed-single-play-design.md`

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- The framed grouping path must be behaviourally unchanged.
- A partially heard group is suppressed, not replayed, and traced.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The heard-segment ledger

**Files:**
- Modify: `pkg/gui/turn_audio.go`
- Test: `pkg/gui/turn_audio_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces: `turnAudioPlan.heard`, `func (a *turnAudioPlan) markHeard(indexes []int)`, `func (a *turnAudioPlan) heardAll(indexes []int) bool`.

- [ ] **Step 1: Write the failing test**

```go
func TestHeardAll(t *testing.T) {
	a := newTurnAudioPlan(nil)
	a.markHeard([]int{0, 1})
	if !a.heardAll([]int{0, 1}) {
		t.Fatal("both indexes were heard")
	}
	if a.heardAll([]int{0, 2}) {
		t.Fatal("index 2 was not heard")
	}
	if !a.heardAll(nil) {
		t.Fatal("an empty set is trivially heard")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestHeardAll -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the heard map (mutex-guarded) and the two methods; `enqueueClip` marks the segments it was given.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestHeardAll -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/turn_audio.go pkg/gui/turn_audio_test.go
git commit -m "feat(gui): track heard segments on the turn plan"
```

---

### Task 2: The streamer reports its segments

**Files:**
- Modify: `pkg/gui/streaming_tts.go`
- Test: `pkg/gui/streaming_tts_test.go` (append)

**Interfaces:**
- Consumes: `turnstream.Event`.
- Produces: `provisionalSpeech.Segments []int`, and the streamer tracking a segment index per line.

- [ ] **Step 1: Write the failing test**

```go
func TestStreamedGroupReportsSegments(t *testing.T) {
	// Feed two segments; the emitted speech reports the segment indexes it covers.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestStreamedGroupReportsSegments -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Give `FeedSegment` a caller-supplied segment index; in grouping mode track the indexes folded into
the pending group alongside the folder's lines, and put them on the emitted `provisionalSpeech`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestStreamedGroupReportsSegments -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/streaming_tts.go pkg/gui/streaming_tts_test.go
git commit -m "feat(gui): report the segments a streamed group covers"
```

---

### Task 3: Finalise skips fully-heard groups

**Files:**
- Modify: `pkg/gui/service.go` (`emitTurnClips`, `finishTurnAudio`, `TurnSession.Run`)
- Test: `pkg/gui/turn_audio_test.go` (append)

**Interfaces:**
- Consumes: `heardAll` (Task 1), the plan's group segment indexes.
- Produces: a finalise skip and a `turn.audio_partial` trace.

- [ ] **Step 1: Write the failing tests**

```go
func TestFinaliseSkipsAFullyHeardGroup(t *testing.T) { /* the group is not enqueued */ }
func TestFinaliseSuppressesAPartialGroup(t *testing.T) { /* skipped, traced */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestFinalise -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

`TurnSession.Run` marks the segments heard as the streamer emits (in the emit callback); `emitTurnClips`
takes a `heardAll` predicate and skips a group whose `SegmentIndexes` are all heard; a partially heard
group is skipped and `turn.audio_partial` traced.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestFinalise -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/turn_audio_test.go
git commit -m "feat(gui): skip a fully-heard group at finalise"
```

---

### Task 4: The parity assertion and the unframed path

**Files:**
- Modify: `pkg/gui/service.go` (a parity check), `pkg/gui/streaming_tts.go` (`Feed`)
- Test: `pkg/gui/streaming_tts_test.go` (append)

**Interfaces:**
- Consumes: the streamed keys and the plan's keys.
- Produces: a `turn.audio_parity_mismatch` trace; a paragraph-folding `Feed`.

- [ ] **Step 1: Write the failing tests**

```go
func TestParityMismatchIsTraced(t *testing.T) { /* a streamed key absent from the plan traces */ }
func TestUnframedFeedFoldsParagraphs(t *testing.T) {
	// Feed a two-sentence paragraph when grouping; one line is folded, not two.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run 'TestParityMismatch|TestUnframedFeed' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

After finalise, compare the streamed keys with the plan's; log a mismatch with both sets. Change `Feed`
so, when grouping, it buffers to a paragraph boundary and feeds one line.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run 'TestParityMismatch|TestUnframedFeed' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): assert stream/plan parity and fold unframed paragraphs"
```

---

### Task 5: Verification

- [ ] **Step 1: Property test**

Extend `pkg/media/groupstream_test.go` so the streamed fold equals the plan and their segment-index
sets match for arbitrary segment sequences.

- [ ] **Step 2: End-to-end guard**

Add a test that a streamed-then-finalised turn emits each clip key at most once.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- A fully-heard group is not re-enqueued.
- A partially heard group is suppressed and traced.
- A streamed/plan key mismatch is traced.
- The framed path is behaviourally unchanged.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: guard the framed single-play path"
```
