# Grouped Live Audio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the live streamer group consecutive same-speaker segments into one TTS request, with the turn's clip plan naming the clips the stream wrote.

**Architecture:** The stream and the finalise run the same deterministic grouping fold over the same lines under the same capabilities, so their groups coincide and the finalise is a cache hit. `GroupFolder` becomes a faithful incremental `planGroups`; `TTSPipeline.SegmentLine` is the one line builder; the streamer gains a grouping mode.

**Tech Stack:** Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-04-grouped-live-audio-design.md`

**Depends on:** `pkg/media/groupstream.go` (`GroupFolder`), `pkg/gui/streaming_tts.go`, `pkg/gui/service.go`.

## Global Constraints

- Go standard library only for tests; no testify. `interface{}`, not `any`. `go vet` clean.
- A streamed group and a finalised group must share a cache key, or the turn pays twice. `ComputeGroupCacheKey` hashes the `SpeakerLine`s, so both sides must build identical lines.
- Commits are Conventional Commits with a scope, subject under 72 chars.

---

### File Map

- **`pkg/media/groupstream.go`** — `GroupFolder` returns `[][]SpeakerLine`, handles an oversized line, keeps the budget.
- **`pkg/media/groupstream_test.go`** — fold parity with `planGroups`, oversized split, budget, speaker change.
- **`pkg/media/group.go`** — extract `SegmentLine`; `GroupPlan` uses it.
- **`pkg/media/caps.go`** — `liveGroupCaps`.
- **`pkg/gui/streaming_tts.go`** — the streamer's grouping mode.
- **`pkg/gui/service.go`** — `clipPlanFor` folds under the live caps.

---

### Task 1: `GroupFolder` reproduces `planGroups`

**Files:**
- Modify: `pkg/media/groupstream.go`
- Test: `pkg/media/groupstream_test.go`

**Interfaces:**
- Produces: `func (f *GroupFolder) Add(line SpeakerLine) [][]SpeakerLine`

- [ ] **Step 1: Write the failing parity test**

```go
func TestGroupFolderMatchesPlanGroups(t *testing.T) {
	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The hall is quiet."},
		{Kind: entity.SegmentNarration, Text: "Cold air rushes in."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Keep walking."},
		{Kind: entity.SegmentNarration, Text: strings.Repeat("Long. ", 200)},
	}
	caps := TTSCapabilities{MaxSpeakers: 1, MaxCharsPerRequest: 40}

	// The batch plan.
	want := planGroups(segments, caps, func(segment entity.TurnSegment) (SpeakerLine, bool) {
		label := narratorLabel
		if segment.Kind == entity.SegmentSpeech {
			label = segment.Speaker
		}
		return SpeakerLine{SpeakerID: segment.SpeakerID, Label: label, Text: segment.Text}, true
	})

	// The same list folded incrementally.
	folder := NewGroupFolder(caps, 0)
	var got []ClipGroup
	for _, segment := range segments {
		label := narratorLabel
		if segment.Kind == entity.SegmentSpeech {
			label = segment.Speaker
		}
		line := SpeakerLine{SpeakerID: segment.SpeakerID, Label: label, Text: segment.Text}
		for _, group := range folder.Add(line) {
			got = append(got, ClipGroup{Lines: group})
		}
	}
	if group := folder.Flush(); group != nil {
		got = append(got, ClipGroup{Lines: group})
	}

	if len(got) != len(want) {
		t.Fatalf("folded %d groups, want %d", len(got), len(want))
	}
	for i := range want {
		if len(got[i].Lines) != len(want[i].Lines) {
			t.Fatalf("group %d has %d lines, want %d", i, len(got[i].Lines), len(want[i].Lines))
		}
		for j := range want[i].Lines {
			if got[i].Lines[j].Text != want[i].Lines[j].Text {
				t.Fatalf("group %d line %d = %q, want %q", i, j, got[i].Lines[j].Text, want[i].Lines[j].Text)
			}
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestGroupFolderMatchesPlanGroups ./pkg/media/`
Expected: build failure, `Add` returns the wrong type.

- [ ] **Step 3: Make `Add` return groups and split an oversized line**

```go
// Add appends a line and returns every group it closed. A line that cannot join
// the pending group flushes it first, and a line too large for one request is
// split at sentence boundaries, exactly as planGroups does.
func (f *GroupFolder) Add(line SpeakerLine) [][]SpeakerLine {
	if !linesFit([]SpeakerLine{line}, f.caps) {
		out := f.flushGroups()
		for _, part := range splitLineToFit(line, f.caps) {
			out = append(out, []SpeakerLine{part})
		}
		return out
	}
	if len(f.pending) > 0 && !canJoinGroup(ClipGroup{Lines: f.pending}, line, f.caps) {
		out := f.flushGroups()
		f.append(line)
		if f.budget > 0 && f.chars >= f.budget {
			out = append(out, f.flushGroups()...)
		}
		return out
	}
	f.append(line)
	if f.budget > 0 && f.chars >= f.budget {
		return f.flushGroups()
	}
	return nil
}

// Flush returns the pending group, or nil.
func (f *GroupFolder) Flush() []SpeakerLine { ... unchanged ... }

// flushGroups returns the pending group as a one-element slice, or nil.
func (f *GroupFolder) flushGroups() [][]SpeakerLine {
	if group := f.Flush(); group != nil {
		return [][]SpeakerLine{group}
	}
	return nil
}
```

Update the existing `GroupFolder` tests to the new return type.

- [ ] **Step 4: Run the tests and watch them pass**

Run: `go test ./pkg/media/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/groupstream.go pkg/media/groupstream_test.go
git commit -m "feat(media): make the streaming fold reproduce the batch grouping"
```

### Task 2: One line builder

**Files:**
- Modify: `pkg/media/group.go`
- Test: `pkg/media/group_test.go`

**Interfaces:**
- Produces: `func (p *TTSPipeline) SegmentLine(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) (SpeakerLine, bool)`

- [ ] **Step 1: Write the failing test**

```go
func TestSegmentLineMatchesGroupPlan(t *testing.T) {
	// Build a pipeline with a fake client, then assert the line SegmentLine
	// returns for a segment equals the line GroupPlan grouped it as.
}
```

- [ ] **Step 2: Extract the builder**

Move the closure body of `GroupPlan` into `SegmentLine` and call it from both.

- [ ] **Step 3: Run the tests and commit**

```bash
go test ./pkg/media/
git add pkg/media/
git commit -m "refactor(media): one line builder for grouping and streaming"
```

### Task 3: Live group capabilities

**Files:**
- Create: `pkg/media/caps.go`
- Test: `pkg/media/caps_test.go`

**Interfaces:**
- Produces: `func liveGroupCaps(caps TTSCapabilities) TTSCapabilities`

- [ ] **Step 1: Write the failing test**

```go
func TestLiveGroupCapsClampToSingleSpeaker(t *testing.T) {
	got := liveGroupCaps(TTSCapabilities{MaxSpeakers: 3, MaxCharsPerRequest: 500})
	if got.MaxSpeakers != 1 || got.MaxCharsPerRequest != 500 {
		t.Fatalf("liveGroupCaps = %+v", got)
	}
}
```

- [ ] **Step 2: Implement**

```go
// liveGroupCaps clamps a capability set to what live audio can use: one speaker
// per request, because a multi-speaker request delays the first speaker's audio
// until the second speaker's text exists. The request limits are kept.
func liveGroupCaps(caps TTSCapabilities) TTSCapabilities {
	caps = normalizeCaps(caps)
	caps.MaxSpeakers = 1
	return caps
}
```

- [ ] **Step 3: Run the tests and commit**

```bash
go test ./pkg/media/
git add pkg/media/caps.go pkg/media/caps_test.go
git commit -m "feat(media): clamp live grouping to one speaker per request"
```

### Task 4: The streamer groups

**Files:**
- Modify: `pkg/gui/streaming_tts.go`
- Test: `pkg/gui/streaming_tts_test.go`

**Interfaces:**
- Consumes: `media.GroupFolder`, `media.SegmentLine`, `media.SynthesizeGroups`.
- Produces: a grouping mode on `sentenceStreamer`, selected by `sentenceStreamerFor`.

- [ ] **Step 1: Write the failing test**

```go
func TestStreamerGroupsConsecutiveSameSpeakerSegments(t *testing.T) {
	// A fake client counts requests. Feed two narration events of the same
	// speaker, then close, and assert one request rather than two.
}
```

- [ ] **Step 2: Implement the grouping mode**

Add `grouping bool`, `folder *media.GroupFolder`, and a queue of `[]media.SpeakerLine`.
In `FeedSegment`, when grouping: build the line with `pipeline.SegmentLine`, `folder.Add`,
and enqueue each returned group. The worker renders a group with
`pipeline.SynthesizeGroups` and emits its clip. `Close` flushes the folder.
When not grouping, the existing per-sentence path is unchanged.

- [ ] **Step 3: Run the tests and commit**

```bash
go test ./pkg/gui/
git add pkg/gui/streaming_tts.go pkg/gui/streaming_tts_test.go
git commit -m "feat(gui): group consecutive same-speaker segments in the streamer"
```

### Task 5: The plan matches

**Files:**
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/service_test.go`

- [ ] **Step 1: Fold under the live caps**

In `clipPlanFor`, when sentence streaming ran for the turn, group with
`pipeline.GroupPlan(segments, narrator, voiceFor, media.LiveGroupCaps(caps))` so the
groups are the single-speaker ones the streamer wrote. `TurnSession.Run` passes the
same capability set to the streamer. Remove the `TTSGrouping() == "always"` early
return in `sentenceStreamerFor`: grouping and streaming now cooperate.

- [ ] **Step 2: Run the tests and commit**

```bash
go test ./pkg/gui/
git add pkg/gui/service.go pkg/gui/service_test.go
git commit -m "feat(gui): name the turn's clips from the stream's own grouping"
```

### Task 6: Full gate

- [ ] **Step 1: Run everything**

```bash
mise run test && mise run lint
```

- [ ] **Step 2: Commit any fixes**

```bash
git add -A && git commit -m "chore: verify grouped live audio"
```
