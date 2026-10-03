# Grouped Live Audio Design

**Date:** 2026-10-04
**Status:** Proposed
**Supersedes:** the A3 deferral in
`2026-10-04-progressive-turn-stream-followups.md`
**Scope:** Live TTS grouping, the turn's clip plan, `pkg/media`, `pkg/gui`
**Related:** `2026-10-03-progressive-turn-stream-design.md` §5,
`pkg/media/group.go`, `pkg/media/groupstream.go`, `pkg/gui/streaming_tts.go`,
`pkg/gui/service.go`

---

## 1. Problem

Live audio is one request per sentence, because a streamed sentence was a cache
miss for a group. That is why `sentenceStreamerFor` refuses to run when
`TTSGrouping() == "always"` and why `clipPlanFor` only groups when streaming is
off. Grouping consecutive same-speaker sentences into one request would cut the
number of provider calls, but only if the streamed clips are the clips the turn
records; otherwise the finalise pass synthesizes the group again and the streamed
audio is wasted.

## 2. The insight: the plan need not be shared, only reproducible

The stream and the finalise see the same segments in the same order. The grouping
algorithm is a deterministic left-to-right fold: its output depends only on the
sequence and the capabilities, not on when the items arrive. So if **both sides
run the same fold with the same inputs**, their groups coincide, and the
finalise's cache lookups hit the clips the stream already wrote. No plan needs to
be carried from the streamer to the DTO; the DTO recomputes it.

This is what makes the wiring small: the two sides must agree on three things.

1. **The fold.** `GroupFolder` (`pkg/media/groupstream.go`) must reproduce
   `planGroups` (`pkg/media/group.go`) exactly, including splitting a single
   oversized segment at sentence boundaries. With a zero latency budget they are
   the same function; the budget is the only intentional divergence, and both
   sides use the same budget.
2. **The lines.** A streamed event and the segment it becomes must build the same
   `SpeakerLine`: the same reduced text, the same label, the same voice. Both go
   through one method, `TTSPipeline.SegmentLine`, extracted from `GroupPlan`.
3. **The capabilities.** A live group is single-speaker even when the provider
   could render several, because a multi-speaker request delays the first
   speaker's audio until the second speaker's text exists. Both sides therefore
   fold under the same single-speaker capability set when the stream ran.

## 3. Design

### 3.1 One fold

`GroupFolder.Add(line) [][]SpeakerLine` returns the groups a line closed:

- a line that cannot join the pending group (speaker change, speaker budget,
  request limit) flushes the pending group first;
- a line too large for one request is split with `splitLineToFit` and emitted as
  its own groups, mirroring `planGroups`;
- a positive budget flushes the pending group once it holds that many speakable
  characters, split at a sentence boundary.

`planGroups` stays the batch entry point; a parity test asserts that folding a
segment list through `GroupFolder` (budget 0) yields the same lines as
`planGroups` over the same segments.

### 3.2 One line builder

```go
// SegmentLine builds the line a segment is grouped and voiced as, so a streamed
// group and a finalised group share a cache key.
func (p *TTSPipeline) SegmentLine(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) (SpeakerLine, bool)
```

`GroupPlan` is refactored to call it, so the two cannot drift.

### 3.3 The streamer groups

`sentenceStreamer` gains a grouping mode. In it:

- `FeedSegment` builds the event's `SpeakerLine` and `folder.Add`s it, enqueuing
  each returned group;
- the worker renders a queued group with `SynthesizeGroups`, so the clip is keyed
  by the group;
- `Close` flushes the folder;
- a record or a speaker change is a natural boundary, because a record produces
  no line and a speaker change fails `canJoinGroup`.

The per-utterance mode is unchanged: it is what `TTSGrouping() == "off"` selects.

### 3.4 The plan matches

`clipPlanFor` groups under the same capability set the streamer used. When
sentence streaming ran, that set is single-speaker; otherwise the provider's own
caps apply. The turn's `GroupClipKeys` then names the clips the stream wrote.

### 3.5 Capabilities and the budget

- **Live caps.** `liveGroupCaps(caps)` clamps `MaxSpeakers` to 1 and keeps the
  request limits.
- **Budget.** A configured latency budget (default: flush on a boundary only) is
  passed to both folds. A zero budget is the user's own model: submit a group
  once a different segment type, a record, or the end of the response arrives.

## 4. Non-Goals

- Continuous-streaming providers (`TTSCapabilities.SupportsStreaming`). They take
  one long request per speaker run and are a separate change.
- Multi-speaker live groups. They remain available to the offline batch path.
- Migrating stored turns. Only new turns use the fold.

## 5. Success Criteria

- A turn streamed with grouping on produces one TTS request per speaker run, not
  per sentence.
- The finalise pass synthesizes nothing the stream already wrote: the turn's clip
  keys are the streamed group keys.
- With grouping off, behaviour is unchanged: one request per sentence.
- `GroupFolder` and `planGroups` agree on the same segment list.
