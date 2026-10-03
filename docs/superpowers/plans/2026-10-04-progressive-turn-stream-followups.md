# Progressive Turn Stream — Follow-ups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the progressive turn stream: voice attributed speech live, make a roll continue the reply, and retire `submit_turn`.

**Architecture:** The streamer is generalised from "narration sentences in the narrator voice" to "any segment's sentences in its speaker's voice", fed from the parser's segment events instead of raw text. The generation loop is wrapped so a `@roll` resolves and the reply continues. The terminal structured path is deleted.

**Tech Stack:** Go standard library, `modernc.org/sqlite`, React 19 + TypeScript.

**Spec:** `docs/superpowers/specs/2026-10-03-progressive-turn-stream-design.md`

**Depends on:** `feat/progressive-turn-stream` (parser, engine wiring, GUI segments, records, single-shot rolls, framing prompt).

## Status (2026-10-04)

Done:

- Stream A, Tasks A1-A2 — the streamer takes parsed segment events and voices
  speech in the speaker's profile; the session drives it from the segment
  observer instead of raw chunks.
- Stream B, Task B1 — a `@roll` under auto resolves and the model continues,
  with the continuation's segments and prose appended to the same turn, bounded
  by `maxRollContinuations`.

Remaining:

- Stream B, Task B0 — a pending-check turn records empty prose; recover it.
- Stream B, Task B2 — persist a resolved roll so a retry cannot re-roll.
- Stream B, Task B3 — mark an ask continuation as continuing the same turn.
- Stream C, Tasks C1-C3 — retire `submit_turn` and the turn-tool surface.
- Stream A, Task A3 — grouped live audio, sharing the turn's clip plan.

## Global Constraints

- Go standard library only for tests; no testify. `interface{}`, not `any`. `go vet` clean.
- TypeScript `strict` with `noUnusedLocals`/`noUnusedParameters`; `npx tsc --noEmit` is the frontend gate.
- Commits are Conventional Commits with a scope, subject under 72 chars.
- A streamed sentence must key the same clip the finaliser would, or the turn pays twice. `groupingEnabled` (`pkg/gui/service.go:519`) is the invariant that keeps them equal: while sentence streaming runs, grouping is off, so the finalise keys are per-utterance.

---

### File Map

- **`pkg/gui/streaming_tts.go`** — the streamer: accept segment events, resolve a per-speaker voice.
- **`pkg/gui/streaming_tts_test.go`** — streamer unit tests.
- **`pkg/gui/service.go`** — wire the streamer to the segment observer; stop feeding it raw chunks.
- **`pkg/engine/orchestrator.go`** — the roll continuation loop.
- **`pkg/engine/history.go`** — `Turn.ContinuationOf`.
- **`pkg/harness/turn_tools.go`**, **`pkg/harness/turn.go`**, **`pkg/engine/submission.go`** — deleted in Stream C.

---

## Stream A — Voice attributed speech live

### Task A1: The streamer accepts segment events

**Files:**
- Modify: `pkg/gui/streaming_tts.go`
- Test: `pkg/gui/streaming_tts_test.go`

**Interfaces:**
- Consumes: `turnstream.Event`, `media.SplitCompleteSentences`, `media.TTSPipeline.SynthesizeProvisional`.
- Produces: `func (s *sentenceStreamer) FeedSegment(event turnstream.Event)`.

- [ ] **Step 1: Write the failing test**

```go
package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/turnstream"
)

// recordingPipeline is a stand-in that records each provisional request.
type recordingUnit struct {
	kind      string
	speakerID string
	text      string
}

func TestStreamerAttributesSpeechToItsSpeaker(t *testing.T) {
	var got []recordingUnit
	streamer := &sentenceStreamer{
		ctx: context.Background(),
		synthesize: func(_ context.Context, kind, speakerID, text string, _ *entity.VoiceConfig) (string, error) {
			got = append(got, recordingUnit{kind: kind, speakerID: speakerID, text: text})
			return "", nil
		},
		queue: make(chan speechUnit, 8),
	}
	streamer.FeedSegment(turnstream.Event{Kind: turnstream.KindSpeech, SpeakerID: "kaelen", Text: "Keep walking."})
	close(streamer.queue)
	streamer.drain()

	if len(got) != 1 || got[0].kind != entity.SegmentSpeech || got[0].speakerID != "kaelen" {
		t.Fatalf("synthesized %#v", got)
	}
}
```

This test needs the streamer to expose an injectable `synthesize` and a `speechUnit`
queue; both are introduced in Step 3.

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestStreamerAttributesSpeech ./pkg/gui/`
Expected: build failure, `speechUnit`/`FeedSegment` undefined.

- [ ] **Step 3: Generalise the streamer**

Replace the sentence-only buffer with a unit queue. A unit carries the kind and
speaker so the worker can resolve a voice:

```go
// speechUnit is one sentence to speak, with the attribution its voice needs.
type speechUnit struct {
	Kind      string
	SpeakerID string
	Text      string
}

// FeedSegment queues every complete sentence of one parsed segment, in its
// speaker's voice. Narration is read by the narrator; speech by its speaker.
func (s *sentenceStreamer) FeedSegment(event turnstream.Event) {
	if s == nil || s.synthesize == nil {
		return
	}
	kind := entity.SegmentNarration
	if event.Kind == turnstream.KindSpeech {
		kind = entity.SegmentSpeech
	}
	text := strings.TrimSpace(event.Text)
	if text == "" {
		return
	}
	complete, remainder := media.SplitCompleteSentences(text)
	if strings.TrimSpace(remainder) != "" {
		complete = append(complete, remainder)
	}
	for _, sentence := range complete {
		select {
		case s.queue <- speechUnit{Kind: kind, SpeakerID: event.SpeakerID, Text: sentence}:
		default:
		}
	}
}
```

The worker takes a `speechUnit`, resolves the voice (`voiceFor(unit.SpeakerID)`
for speech, the narrator voice otherwise), and calls `s.synthesize`. Replace the
old `Feed` and the single `voice` field with a `voiceFor` function and an
injected `synthesize func(ctx, kind, speakerID, text, voice) (string, error)`
defaulting to `pipeline.SynthesizeProvisional`. Keep `StopEmitting`, `Close`, and
the `provisionalSpeech` emission unchanged.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test ./pkg/gui/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/streaming_tts.go pkg/gui/streaming_tts_test.go
git commit -m "feat(gui): voice streamed speech with the speaker's profile"
```

### Task A2: Feed the streamer from the segment observer

**Files:**
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/service_test.go`

**Interfaces:**
- Consumes: `sentenceStreamer.FeedSegment`, `TurnOrchestrator.SetSegmentObserver`.

- [ ] **Step 1: Rewire the session**

In `TurnSession.Run`, move the `SetSegmentObserver` call to after the streamer is
built, and have it feed the streamer:

```go
	t.orchestrator.SetSegmentObserver(func(event turnstream.Event) {
		if segment, ok := liveSegmentDTO(event); ok {
			_ = announce(TurnEvent{Type: "segment", Segment: &segment})
		}
		streamer.FeedSegment(event)
	})
```

Stop feeding the streamer from the raw chunk listener: the `onChunk` passed to
`ProcessActionStream` becomes only `announce(TurnEvent{Type: "chunk", Text: text})`.

- [ ] **Step 2: Run the tests**

Run: `go test ./pkg/gui/`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add pkg/gui/service.go pkg/gui/service_test.go
git commit -m "feat(gui): drive live audio from parsed segments, not raw chunks"
```

### Task A3: Grouped live audio shares the turn's clip plan

**Files:**
- Create: `pkg/media/groupstream.go`
- Test: `pkg/media/groupstream_test.go`
- Modify: `pkg/gui/streaming_tts.go`, `pkg/gui/service.go`

**Interfaces:**
- Consumes: `TTSCapabilities`, `SpeakerLine`, `canJoinGroup`, `ComputeGroupCacheKey`, `SynthesizeGroups`.
- Produces: `func (s *sentenceStreamer) GroupPlan() []media.ClipGroup`, so `clipPlanFor` reuses the stream's groups instead of re-planning.

- [ ] **Step 1: Write the failing test**

```go
func TestGroupFolderMatchesPlanGroupsWithoutABudget(t *testing.T) {
	lines := []SpeakerLine{
		{SpeakerID: "narrator", Label: "Narrator", Text: "The hall is quiet."},
		{SpeakerID: "narrator", Label: "Narrator", Text: "Cold air rushes in."},
		{SpeakerID: "garrick", Label: "Garrick", Text: "Keep walking."},
	}
	caps := TTSCapabilities{MaxSpeakers: 1}

	folder := NewGroupFolder(caps, 0)
	var flushed [][]SpeakerLine
	for _, line := range lines {
		if out := folder.Add(line); out != nil {
			flushed = append(flushed, out)
		}
	}
	if out := folder.Flush(); out != nil {
		flushed = append(flushed, out)
	}
	if len(flushed) != 2 || len(flushed[0]) != 2 || len(flushed[1]) != 1 {
		t.Fatalf("folded %#v, want 2/1", flushed)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestGroupFolder ./pkg/media/`
Expected: build failure, `NewGroupFolder` undefined.

- [ ] **Step 3: Implement the fold**

```go
// GroupFolder folds a turn's speaker lines into groups incrementally, applying
// the same rules as planGroups: a group ends when the speaker changes, when the
// speaker budget is reached, or when the request limits would be exceeded. A
// positive budget additionally forces a sentence-aligned flush so live audio is
// not held until a long block ends.
type GroupFolder struct {
	caps    TTSCapabilities
	budget  int
	pending []SpeakerLine
	chars   int
}

func NewGroupFolder(caps TTSCapabilities, budget int) *GroupFolder {
	return &GroupFolder{caps: normalizeCaps(caps), budget: budget}
}

func (f *GroupFolder) Add(line SpeakerLine) []SpeakerLine {
	if len(f.pending) > 0 && !canJoinGroup(ClipGroup{Lines: f.pending}, line, f.caps) {
		out := f.Flush()
		f.append(line)
		return out
	}
	f.append(line)
	if f.budget > 0 && f.chars >= f.budget {
		return f.Flush()
	}
	return nil
}

func (f *GroupFolder) Flush() []SpeakerLine {
	if len(f.pending) == 0 {
		return nil
	}
	out := f.pending
	f.pending = nil
	f.chars = 0
	return out
}

func (f *GroupFolder) append(line SpeakerLine) {
	f.pending = append(f.pending, line)
	f.chars += len([]rune(strings.TrimSpace(line.Text))) + 1
}
```

- [ ] **Step 4: Run it and watch it pass**

Run: `go test ./pkg/media/`
Expected: PASS.

- [ ] **Step 5: Record the stream's plan and reuse it**

Give the streamer a `GroupFolder`, synthesize each flushed group through
`SynthesizeGroups` so the clip key is the group key, and expose the plan:

```go
// GroupPlan returns the groups the stream produced, so the turn records the same
// clips the player already heard rather than re-planning them.
func (s *sentenceStreamer) GroupPlan() []media.ClipGroup
```

In `clipPlanFor`, when a plan is supplied for the turn, build `plan.segmentKeys`
and `plan.groups` from it instead of calling `GroupClipKeys`. `TurnSession.Run`
passes `streamer.GroupPlan()` to `turnDTO`. Remove the `TTSGrouping() == "always"`
early return in `sentenceStreamerFor`: grouping and streaming now cooperate.

- [ ] **Step 6: Run the tests and commit**

```bash
go test ./pkg/media/ ./pkg/gui/
git add -A && git commit -m "feat(gui): group streamed audio under the turn's own clip plan"
```

---

## Stream B — A roll continues the reply

### Task B0: Recover prose on a pending-check turn

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/pending_check_test.go`

A turn whose reply ended on a check records empty narration, because the
recovery step is gated on `result.Submission == nil && result.PendingCheck == nil`.
The prose the model wrote before the check is the setup the player reads while
deciding, so it must survive.

- [ ] **Step 1: Write the failing test**

```go
func TestAPendingCheckTurnKeepsItsProse(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{
		"Kaelen steps onto the bridge, the planks swaying.\n" +
			"@roll {\"actor\":\"kaelen\",\"check_kind\":\"skill\",\"stakes\":\"the bridge\"}\n",
	}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	orchestrator.SetMechanicsEngagement("ask")

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I follow.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.PendingCheck == nil {
		t.Fatalf("expected a pending check, got %+v", turn)
	}
	if !strings.Contains(turn.Narration, "planks swaying") {
		t.Fatalf("narration = %q, want the prose before the check", turn.Narration)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestAPendingCheckTurnKeepsItsProse ./pkg/engine/`
Expected: FAIL, narration is empty.

- [ ] **Step 3: Recover whenever there is text**

Change the gate to `if result.Submission == nil {`, and only treat an empty
narration as a failure when there is no pending check:

```go
	if result.Submission == nil {
		if endedOnRoll {
			cause = cutNone
		} else {
			cause = o.classifyCut(result)
		}
		narration, recovery, stillIncomplete = o.recoverReply(ctx, strings.Join(narrationParts, "\n\n"), cause, onChunk)
		if strings.TrimSpace(narration) == "" && result.PendingCheck == nil {
			// existing empty-narration failure handling
		}
	}
```

- [ ] **Step 4: Run the tests and commit**

```bash
go test ./pkg/engine/
git add pkg/engine/
git commit -m "fix(engine): keep the prose a pending-check turn was written with"
```

### Task B1: Wrap generation in a roll loop

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/turnstream_integration_test.go`

**Interfaces:**
- Consumes: `pendingRoll`, `resolveCheck`, `rollRef`.
- Produces: a bounded loop in `ProcessActionStream` that re-runs `runGenerationLoop` after a roll.

- [ ] **Step 1: Write the failing test**

```go
func TestRollContinuationAppendsToTheSameTurn(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{
		"Kaelen crosses the bridge.\n@roll {\"actor\":\"kaelen\",\"check_kind\":\"skill\",\"stakes\":\"the bridge\",\"outcomes\":{\"pass\":\"He crosses.\",\"fail\":\"He falls.\"}}\n",
		"Kaelen reaches the far side.\n",
	}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I follow.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if !strings.Contains(turn.Narration, "far side") {
		t.Fatalf("the continuation must be recorded: %q", turn.Narration)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestRollContinuation ./pkg/engine/`
Expected: FAIL, the continuation text is absent.

- [ ] **Step 3: Implement the loop**

Wrap the `runGenerationLoop` call and its error handling in a loop:

```go
	const maxRollContinuations = 3
	var collected []turnstream.Event
	var rollResults []harness.CheckResult
	for attempt := 0; ; attempt++ {
		result, err = o.runGenerationLoop(ctx, &assembly, gmDirective, proposedCheck, resolvedPending, validationEngagement, onChunk)
		if err != nil {
			// existing error handling, unchanged
			return nil, fmt.Errorf("gm generation failed: %w", err)
		}
		if o.parser != nil {
			o.parser.Flush()
			collected = append(collected, o.parser.Events()...)
		}
		req, ok := o.pendingRoll()
		if !ok || o.mechanicsEngagement == "off" || o.mechanicsEngagement == "ask" || attempt >= maxRollContinuations {
			break
		}
		resolved, resolveErr := o.resolveCheck(ctx, req, nil)
		if resolveErr != nil {
			o.logger.Event("roll.resolve_error", map[string]interface{}{"error": resolveErr.Error()})
			break
		}
		resolved.CheckID = rollRef(turnNum, len(rollResults))
		rollResults = append(rollResults, *resolved)
		gmDirective = rollContinuationDirective(*resolved, req)
		resolvedPending = nil
		o.parser.Reset()
	}
	result.Checks = append(result.Checks, rollResults...)
```

`collected` becomes the events the finalise step builds segments from, so the
continuation's segments are appended to the same turn. `narration` becomes the
join of every continuation's recovered text. `rollRef` gains a disambiguating
index. `rollContinuationDirective` names the outcome and its pre-committed text,
reusing the `[PLAYER ROLL: …]` shape already in `orchestrator.go:805`.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/
git commit -m "feat(engine): continue the turn after a resolved roll"
```

### Task B2: Persist a resolved roll so a retry cannot re-roll

**Files:**
- Modify: `pkg/engine/orchestrator.go`, `pkg/engine/history.go`

- [ ] **Step 1: Write the failing test**

```go
func TestResolvedRollIsNotRerolled(t *testing.T) {
	// Record a turn whose pending check resolved to a fixed outcome, then run the
	// continuation twice with the same pending_check_ref and assert the outcome is
	// identical and the roll happened once.
}
```

- [ ] **Step 2: Store the result on the turn**

When `pendingCheckRef` resolves, write the `CheckResult` onto the turn that
carried the `PendingCheck` (a `ResolvedChecks []harness.CheckResult` field on
`Turn`), and reuse it when the same ref is seen again instead of calling
`resolveCheck`.

- [ ] **Step 3: Run the tests**

Run: `go test ./pkg/engine/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/engine/
git commit -m "fix(engine): resolve a pending check once and persist the result"
```

### Task B3: An ask continuation continues the same logical turn

**Files:**
- Modify: `pkg/engine/history.go`, `pkg/engine/timeline.go`, `pkg/gui/types.go`

- [ ] **Step 1: Add `ContinuationOf`**

Add `ContinuationOf int \`json:"continuation_of,omitempty"\`` to `Turn`, set when
`pendingCheckRef` is present, and surface it on the DTO so the chronicle groups
the halves. Stitching the halves into one stored record is out of scope; the
field lets a client present them as one.

- [ ] **Step 2: Run the tests and commit**

```bash
go test ./pkg/engine/ ./pkg/gui/
git add -A && git commit -m "feat(engine): mark a roll continuation as continuing a turn"
```

---

## Stream C — Retire `submit_turn`

### Task C1: Remove the turn-tool surface

**Files:**
- Modify: `pkg/harness/turn_tools.go`, `pkg/engine/orchestrator.go`

- [ ] **Step 1: Delete the tools**

Delete `TurnToolSpecsFor`, `TurnToolSpecs`, `submitTurnSpec`, `requestCheckSpec`,
`proposeCheckSpec`, `TurnToolNames`, `IsTurnTool`, and `ParseCheckRequest`. Delete
the turn-tool dispatch block in `runGenerationLoop`
(`pkg/engine/orchestrator.go:1754-1801`). Rolls and declarations now arrive as
stream records.

- [ ] **Step 2: Run the tests**

Run: `go test ./pkg/engine/ ./pkg/harness/`
Expected: failures only in tests that referenced the deleted symbols.

### Task C2: Remove the structured submission path

**Files:**
- Delete: `pkg/harness/turn.go`, `pkg/engine/submission.go`, `pkg/engine/structured_turn_test.go`, `pkg/engine/structured_stream_test.go`, `pkg/engine/submission_segments_test.go`
- Modify: `pkg/engine/orchestrator.go`

- [ ] **Step 1: Delete the types and the branch**

Delete `TurnSubmission`, `TurnSubmissionSchema`, `ParseSubmission`,
`SegmentSpec`, `buildSegments`, `validateSubmission`, `speakerResolver`, and the
`structured` branch in `ProcessActionStream`. The `result.Submission` and
`FallbackReason` fields on `streamResult` go too.

- [ ] **Step 2: Run the tests**

Run: `go test ./...`
Expected: PASS after deleting the tests that covered the removed path.

### Task C3: Update the prompt and docs

**Files:**
- Modify: `pkg/harness/context.go`, `pkg/gui/docs/*.md`

- [ ] **Step 1: Regenerate the embedded docs**

Run: `go test ./pkg/gui -update-docs`
Expected: the provider catalogue and config reference regenerate.

- [ ] **Step 2: Run the full gate and commit**

```bash
mise run test && mise run lint
git add -A && git commit -m "refactor(engine): retire submit_turn for the turn stream"
```
