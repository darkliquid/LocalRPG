# Roll Presentation Integrity Design

**Date:** 2026-10-10
**Status:** Proposed
**Issue:** [#131](https://github.com/darkliquid/LocalRPG/issues/131)
**Epic:** [#123 Generation, playback, and roll resilience](https://github.com/darkliquid/LocalRPG/issues/123)
**Depends on:** [Roll Card Design](2026-10-05-roll-card-design.md), [Roll Mode Interactive Design](2026-10-05-roll-mode-interactive-design.md)
**Scope:** `pkg/engine`, `pkg/harness`, `frontend`

---

## 1. Problem

Roll mechanics sometimes appear as narrative prose instead of (or beside) the structured
roll card. The card is chosen by exactly one link: `TurnSegment.CheckRef` names the
`TurnCheck.check_id` whose roll the segment narrates (`pkg/entity/segment.go:18`,
`frontend/src/components/TurnSegments.tsx:130-145`). A check with a matching `check_ref`
renders a `DiceCheckCard` immediately before that segment; a check with **no** matching
segment is unshifted to the top of the turn (`TurnSegments.tsx:143-146`), detached from
the prose that describes it.

Three of the four resolution paths fail to set that link:

| Path | Sets `CheckRef`? | Where |
| --- | --- | --- |
| `@roll` record (auto) | **yes** | `orchestrator.go:1143-1148`, stamped at `:1379-1387` |
| `request_check` tool call (auto, tool-calling provider) | **no** | `orchestrator.go:2188-2210` |
| pending check resolution (ask policy) | **no** | `orchestrator.go:959-980`, appended `:2005-2006` |

So a tool-calling model's roll and an ask-policy roll both produce an **unattached**
card floating at the top of the turn, while the model's own prose about the same roll
sits in the narration.

Nothing forbids that prose. The mechanics prompt (`pkg/harness/mechanics_instructions.go:25-45`)
says not to *invent* results, and `context.go:299` says "Never invent dice results", but
none of them says "do not restate the dice". The tool descriptions
(`pkg/harness/turn_tools.go:37,72`) say "Call it, then keep narrating" and "Do not resolve
it yourself", which leaves the arithmetic unmentioned. The model therefore writes
"You roll a 4 and a 3 for a total of 7" next to the card that already says `2d6 → 7`.

The outcome text is model-authored too: `RollOutcome` is `req.Outcomes[resolved.Outcome]`
(`orchestrator.go:1133`) and is appended as a narration segment (`:1229-1235`,
`:1362-1368`). It is attached on the `@roll` path and unattached elsewhere.

Finally, two display surfaces bypass the card entirely:

- **Story Theater never renders one.** `StoryTheater.tsx:51-54` reads only `segments`
  and does not import `DiceCheckCard`, so in theater mode a check's outcome is narration
  and nothing else.
- **The fallback is unfiltered.** `ChronicleView.tsx:148` passes `turn.prose` as the
  fallback, and `TurnSegments.tsx:67` renders it verbatim when `segments` is empty.
  The live stream filter drops only lines beginning with `@`
  (`frontend/src/lib/turnStreamProcessor.ts:43-49,101-105`).

## 2. Goals

- Every resolved check is attached to the segment that narrates it, on all four paths.
- The model is told not to restate dice, totals, or modifiers in prose.
- Any dice telegraphy that still arrives is hidden from the narration, not shown beside
  the card, and the card is the one place the mechanism appears.
- The theater renders the roll, not only its prose.

## 3. Non-goals

- Changing how a check is resolved, what a card contains, or the pending-check flow.
- Rewriting stored history: `history.jsonl` keeps the model's text for provenance; the
  filtering is presentational.
- Removing the `MechanicsStrip`, which is a structured readout, not narration.

## 4. Design

### 4.1 Attach the check on every path

Reuse the anchor mechanism the `@roll` path already uses. In `orchestrator.go`, an anchor
records the segment index a check should attach to:

```go
type checkAnchor struct {
	segmentIndex int
	checkID      string
}
```

- **`request_check` tool path** (`:2188-2210`): when the tool resolves a check, append a
  `checkAnchor{segmentIndex: len(segmentsFromEvents(collected)), checkID: resolved.CheckID}`
  before continuing the loop, exactly as the `@roll` path does. After the loop, stamp the
  anchor onto the first segment at or after its index that has no `CheckRef`, guarded by
  the existing bounds check (`:1379-1387`).
- **Pending resolution** (`:959-980`, `:2005-2006`): the resolved check carries
  `RollCheckRef` on the stream result (`pkg/engine/orchestrator.go:1652-1661`). When the
  outcome segment is appended it is given `CheckRef: result.RollCheckRef`, matching the
  `@roll` path at `:1360-1368`. On the proposing turn, the same ref is written so the
  resolved card replaces the pending card where `ContinuationOf` groups them, as the roll
  card spec already requires.
- The existing unattached-checks unshift (`TurnSegments.tsx:143-146`) stays as a fallback
  for a genuinely unlinked check, but becomes the rare case rather than the default for
  tool and pending rolls.

### 4.2 Tell the model not to restate the roll

Add one rule to the mechanics instructions (`pkg/harness/mechanics_instructions.go`), in
the enabled-mechanics branch:

```text
Do not restate the dice. Never write notation, totals, modifiers, or the arithmetic of a
roll in your prose; the engine renders the roll beside your words. Narrate only what the
outcome means for the fiction.
```

Strengthen the `context.go:299` line from "Never invent dice results" to
"Never invent or restate dice results", and add a sentence to both `request_check` and
`@roll` tool descriptions (`pkg/harness/turn_tools.go:37,72`) that the result is rendered
for the player and need not be repeated. This is a prompt change; it reduces the leak but
does not guarantee its absence, which is why 4.3 exists.

### 4.3 Hide remaining telegraphy

Add `frontend/src/lib/rollText.ts`:

```ts
// stripDiceTelegraphy removes dice arithmetic from a narration string, so the
// roll card is the only place a number appears. Conservative: it removes a
// fragment only when it is unambiguously a roll report.
export function stripDiceTelegraphy(text: string): string;
```

It removes, and only removes:

- a parenthesised or bracketed fragment that is dice notation or an arithmetic result,
  for example `(2d6+3 = 9)`, `[rolled 4 and 3 for 7]`, `(d20: 14)`;
- a sentence or line that is nothing but a roll report, for example
  `Roll: 2d6+3 → 9` or `Total: 7`;
- a trailing appositive introduced by "for a total of" or "which totals" when the
  measured pattern is a roll.

It leaves a number in ordinary prose (`you have 4 rations`, `a 7-foot wall`) untouched.
The pattern set is documented in the module and covered by positive and negative tests;
when in doubt it leaves the text alone, because a leaked sentence is better than a
mangled one.

Apply it at the presentation boundary so history is untouched:

- `TurnSegments.tsx` filters `segment.text` for `kind: 'narration'` before rendering, and
  filters the `turn.prose` fallback (`:67`) the same way.
- `turnStreamProcessor.ts` applies it as live narration arrives, so the streamed view
  matches the settled one.
- `DiceCheckCard` and `MechanicsStrip` are **not** filtered; they are the structured
  surface.

Speech is never filtered: a character saying "a seven!" is dialogue, not a roll report.

### 4.4 Render the roll in the theater

`StoryTheater.tsx` gains the same check-to-segment grouping `TurnSegments` uses: build a
`checkByID` map from `turn.checks`, splice a card before the segment whose `check_ref`
matches, and lead with any unattached check. It renders the existing `DiceCheckCard`
(a compact theater variant if the full card is too tall for the performance layout),
so a theater turn shows the mechanism as a card and the prose shows its consequence. The
grouping helper is extracted from `TurnSegments.tsx` into `frontend/src/lib/checks.ts`
(`groupChecksWithSegments(segments, checks)`) and shared, so the two surfaces cannot
drift.

### 4.5 The strip

`MechanicsStrip` keeps its one-line structured readout. It is not narration and is the
fallback surface when the theater layout has no room for a card. Its label is the only
change: it is titled "Rolls" so it reads as the structured record, not prose.

## 5. Behaviour

| Situation | Before | After |
| --- | --- | --- |
| `request_check` resolves a roll | card at the top, prose narrates dice | card attached to the narrating segment |
| ask-policy roll resolves | card at the top | card attached, pending card replaced |
| The model writes "(2d6+3 = 9)" in prose | shown as narration | stripped from narration, shown on the card |
| A sentence "you have 4 rations" | shown | unchanged |
| Story Theater plays a roll | outcome only as prose | card plus prose |
| A turn with no check | unchanged | unchanged |

## 6. Testing

- `pkg/engine`: a `request_check` tool resolution stamps `CheckRef` on the following
  segment (extend `mechanics_turn_test.go`); a pending resolution stamps it and links the
  continuation; an `@roll` turn is unchanged.
- `frontend`: `stripDiceTelegraphy` table tests, including the negatives above and an
  empty string; `groupChecksWithSegments` places an attached card before its segment and
  leads with an unattached one; `StoryTheater` renders a `DiceCheckCard` for a turn with
  a check.
- A regression test: a narration segment whose `check_ref` matches still yields exactly
  one card (no duplicate from grouping).
- Prompt assertions: the mechanics instructions contain the "Do not restate the dice"
  rule when engagement is on (extend the existing instruction tests).

## 7. Rollout

Engine first (the anchor and the pending link), then the prompt, then the frontend
filter and theater card. The engine change is a bug fix and can ship alone; the prompt
and filter improve the result but neither is required for the card to be attached.

## 8. Risks

- **The filter can eat legitimate prose.** It is conservative and only removes
  unambiguous roll reports; the negative tests are the guard. The worst case is a
  sentence that still leaks, which is today's behaviour.
- **Anchor drift.** The segment-index anchor is already used by `@roll` and is bounds
  checked; the tool path adds the same discipline. A segment shift between the anchor and
  the stamp is handled by the first-unset-search rather than a fixed index.
- **The prompt is not a guarantee.** A model can ignore it; the filter and the attachment
  are what make the card authoritative regardless.
- **History is unfiltered by design.** A reader of `history.jsonl` sees the model's raw
  text, including any telegraphy; that is correct for provenance and is stated here so it
  is not mistaken for a bug.
