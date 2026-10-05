# Guaranteed Single-Play Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#67 RB-2](https://github.com/darkliquid/LocalRPG/issues/67)
**Epic:** [#23 Malformed output and playback integrity](https://github.com/darkliquid/LocalRPG/issues/23)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §5 (RB-2)
**Scope:** `pkg/gui` (`streaming_tts.go`, `turn_audio.go`, `service.go`), `pkg/media`

---

## 1. Problem

A turn's audio is produced twice: live, by the sentence pre-synthesiser as prose streams
(`sentenceStreamer`, `pkg/gui/streaming_tts.go`), and at finalise, by the turn's clip plan
(`emitTurnClips`, `pkg/gui/service.go:2787`). The two must name the **same clips** or a line is
heard twice.

Today the only thing preventing a repeat is **key equality**: the streamer's clip key and the
plan's group key are content-addressed hashes, and the played set skips a key it has already sent
(`turnAudioPlan.enqueueClip`, `pkg/gui/turn_audio.go:73-88`). That is exact *only while the two
paths fold the same lines under the same capabilities*.

The design intends parity:

- Both fold through `media.GroupFolder` with budget 0 (`pkg/gui/streaming_tts.go:180`), so the
  streaming fold and `planGroups` are identical for the same input.
- Both use the same line builder, `pipeline.SegmentLine`, and the same caps,
  `pipeline.GroupCaps()` (`pkg/gui/streaming_tts.go:470`, `pkg/gui/service.go:2852`), resolved
  through `media.TurnGroupCaps` (`pkg/media/caps.go:47`).
- The framed path feeds **whole segments** when grouping
  (`FeedSegment`, `pkg/gui/streaming_tts.go:336-343`), which is what the plan folds.

So parity holds on the framed path, and there is a unit test for the fold
(`TestGroupFolderMatchesPlanGroups`, `pkg/media/groupstream_test.go:62`). But three holes remain:

1. **The unframed path diverges.** `sentenceStreamer.Feed`
   (`pkg/gui/streaming_tts.go:304-319`) splits raw narration into **sentences** and feeds each as
   its own line, so its groups differ from the plan's whole-segment groups and their keys differ.
   Any caller using it double-plays.
2. **Parity is a property, not a check.** Nothing verifies at runtime that the keys the streamer
   emitted are the keys the plan contains. A future change to either fold reintroduces the bug
   silently.
3. **A partially heard group is replayed.** When the streamer emitted some but not all of a group's
   segments before the turn became authoritative, finalise emits the group clip and the heard
   segments play again.

## 2. Goals

- A clip for a given unit of text plays **at most once per turn**.
- The guarantee does not depend on the two folds happening to agree: it is enforced from a shared
  ledger of what the player has heard.
- A divergence between the streamed keys and the plan keys is **detected and traced**, not silent.
- The common case (framed provider, grouping on) keeps its current early-audio behaviour.

## 3. Non-goals

- Client-side resume offsets for a partially heard clip. That is RB-3; this spec makes the server
  side safe and leaves a hook for RB-3.
- Cross-turn deduplication. A clip may legitimately play in two turns.
- Changing when audio is synthesized, only which clips are enqueued for playback.

## 4. Design

### 4.1 A shared heard ledger keyed by segment

`turnAudioPlan` (`pkg/gui/turn_audio.go:67-70`) gains a second, authoritative record of what the
player has heard, expressed in **segment indexes** rather than clip keys:

```go
type turnAudioPlan struct {
	queue  chan string
	played *clipSet
	// heard tracks the segment indexes the player has been sent audio for. It is
	// the authority for the finalise skip: a group whose segments are all heard is
	// not re-enqueued, whatever its key.
	heard map[int]bool
	mu    sync.Mutex
}
```

`heard` is populated by both producers:

- The streamer, when it emits a unit, reports the segment indexes that unit covers (§4.2).
- `enqueueClip` marks the plan's own emission too, so a clip enqueued by the plan counts as heard
  for any later pass in the same turn.

A helper on the plan answers the finalise question:

```go
// heardAll reports whether every index has been heard.
func (a *turnAudioPlan) heardAll(indexes []int) bool
```

### 4.2 The streamer reports the segments it covered

`provisionalSpeech` (`pkg/gui/streaming_tts.go`) gains `Segments []int`, and the streamer tracks
which segment each line belongs to:

- `FeedSegment` already receives one `turnstream.Event` per segment. It gains a monotonically
  increasing `segmentIndex` supplied by the caller (`TurnSession.Run`), and passes it into `feed`.
- In grouping mode, `GroupFolder` accumulates lines; the streamer keeps a parallel slice of the
  segment indexes folded into the pending group, so when the folder returns a closed group the
  streamer knows its segment set. `Flush` does the same for the trailing group.
- `synthesizeGroup`/`synthesizeUnit` carry the segment set through `jobResult`, and
  `completeJobLocked` puts it on the emitted `provisionalSpeech`.

`TurnSession.Run` (`pkg/gui/service.go:2034`) maintains the counter and, in the emit callback
(`pkg/gui/service.go:1996-1997`), marks the segments heard on the plan before enqueuing.

The segment index is the position of the event in the turn's segment order, which is the same order
`clipPlanFor` uses to build `plan.groups[].SegmentIndexes` (`pkg/gui/service.go:559-570`). Both
therefore speak the same index space.

### 4.3 Finalise skips fully heard groups

`emitTurnClips` (`pkg/gui/service.go:2787`) is called by `finishTurnAudio` with an emit callback
(`pkg/gui/service.go:2103-2107`). It gains an optional "already heard" predicate, supplied by the
plan:

```go
func (t *TurnSession) finishTurnAudio(ctx context.Context, turn engine.Turn, plan *turnAudioPlan) {
	t.service.emitTurnClips(ctx, t.gameID, turn, false, func(clip string) {
		plan.enqueueClip(media.ClipKeyForPath(clip), clip)
	}, plan.heardAll)
}
```

Inside `emitTurnClips`, for each group the plan computes the group's segment indexes (it already
has them: `plan.groups[].SegmentIndexes`) and, when `heardAll` reports them all heard, skips the
group without enqueuing it. The key check in `enqueueClip` stays as a second line of defence.

For a **partially heard** group the clip cannot be split, so the turn chooses the guarantee over
completeness: the group is skipped and a `turn.audio_partial` trace event records the segment
indexes that were missed. RB-3 replaces this suppression with a resume offset so the unheard tail
is played without replaying the head.

### 4.4 Parity is asserted, not assumed

After finalise, `TurnSession.Run` compares the set of keys the streamer emitted with the set of
keys the plan contains:

```go
// plan.parityMissing returns streamed keys absent from the plan.
// plan.parityExtra returns plan keys never streamed.
```

A non-empty result is logged as `turn.audio_parity_mismatch` with both key sets and their segment
indexes. This turns a silent regression into a visible trace, and it is the signal RB-5 renders.
The assertion is diagnostic only; it never changes what plays.

### 4.5 The unframed path is made safe

`sentenceStreamer.Feed` must not produce lines that differ from the plan. Two options; the spec
chooses the first:

1. **Fold whole buffered paragraphs.** When grouping, `Feed` buffers text until a paragraph
   boundary (a blank line, or a `Flush`) and feeds the whole paragraph as one line, matching the
   plan's paragraph segment. When not grouping, per-sentence feeding is already consistent because
   the plan uses `SegmentClipKeys`, which is sentence-scoped for that mode.
2. If `Feed` proves unused in production (a grep shows only tests call it), delete it and the
   branch that selects it, so the divergence cannot exist.

The plan resolves which applies by checking the production call sites; the spec requires that,
after this change, no code path feeds the streamer lines that differ from the plan's fold.

## 5. Behaviour

| Situation | Result |
| --- | --- |
| Framed provider, grouping on, every group streamed | finalise skips all groups; nothing repeats |
| Framed provider, grouping on, last group not streamed | only the last group is enqueued at finalise |
| Group partially streamed | group skipped, `turn.audio_partial` traced (RB-3 will resume) |
| Unframed provider, grouping on | `Feed` folds paragraphs, so keys match the plan |
| Streamed keys differ from plan keys | `turn.audio_parity_mismatch` traced, no double play |
| No audio enabled | plan is a no-op; `heardAll` is never consulted |

## 6. Testing

- `pkg/gui/turn_audio_test.go`:
  - `heardAll` is true only when every index is present;
  - a fully heard group is not enqueued at finalise;
  - a partially heard group is skipped and traced.
- `pkg/gui/streaming_tts_test.go`:
  - a streamed group reports the segment indexes it covered;
  - the unframed path folds a two-sentence paragraph into one line when grouping.
- `pkg/media/groupstream_test.go`: extend the property test to assert that, for arbitrary segment
  sequences, the streamer's fold equals `planGroups` **and** the segment-index sets match.
- `pkg/gui` end-to-end (the existing driver harness): a turn whose audio is streamed emits each
  clip key at most once across the `speech` events and the final `turn` DTO.

## 7. Rollout

No configuration. The change is internal and only removes repeats; the framed path is unchanged in
behaviour. `turn.audio_parity_mismatch` and `turn.audio_partial` are additive trace events.

## 8. Risks

- **Segment index drift.** The streamer's index and the plan's index must be the same ordering. The
  plan uses the event order the streamer is fed in, so they agree by construction; the parity
  assertion catches a regression.
- **Suppressing a partial group loses a tail.** This is a deliberate trade of completeness for the
  no-repeat guarantee. RB-3 restores the tail. The trade is documented in the trace event.
- **`Feed` removal.** If `Feed` is reachable from a provider adapter, deleting it would break that
  path; the plan checks call sites before deleting.
