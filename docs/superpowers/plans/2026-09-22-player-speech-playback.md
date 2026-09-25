# Player Speech Playback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show the player's `say` line as an ordinary speech beat and speak it in the player character's voice before the narration, without also printing the `/say` action block.

**Architecture:** Add a `Player` marker to `TurnSegment`, prepend a normal speech segment for `say` turns, propagate the marker through the DTOs, suppress the duplicate action block for turns that carry one, and let the existing speech rendering, playback queue, and cache do the rest.

**Tech Stack:** Go 1.27.1, React 19, TypeScript.

**Spec:** `docs/superpowers/specs/2026-09-22-player-speech-playback-design.md`

**Dependency:** The player voice comes from `docs/superpowers/plans/2026-09-22-character-creation.md`. Voice-change propagation (`docs/superpowers/specs/2026-09-22-voice-change-propagation-design.md`) keeps a changed player voice from being served stale.

---

## File Structure Map

| File Path | Responsibility |
| :--- | :--- |
| `pkg/entity/segment.go` | `Player` marker |
| `pkg/engine/player_segment.go` | `playerSegment` helper |
| `pkg/engine/orchestrator.go` | Prepend the beat; keep order |
| `pkg/gui/types.go` | `Player` on `SegmentDTO` |
| `pkg/gui/service.go` | Propagate `Player` |
| `frontend/src/types.ts` | Optional `player` |
| `frontend/src/components/ChronicleView.tsx` | Suppress the duplicate action block |
| `frontend/src/components/TurnSegments.tsx` | Render the player beat as speech |
| `frontend/src/components/StoryTheater.tsx` | Render the player beat as speech |

---

### Task 1: Add the player marker to the segment

**Files:**
- Modify: `pkg/entity/segment.go`
- Modify: `pkg/entity/history_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestTurnSegmentPlayerRoundTrip(t *testing.T) {
	segment := TurnSegment{Kind: SegmentSpeech, Speaker: "Sean", SpeakerID: "sean", Text: "Hello.", Player: true}
	data, err := json.Marshal(segment)
	if err != nil {
		t.Fatal(err)
	}
	var out TurnSegment
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !out.Player {
		t.Fatal("player marker lost in round trip")
	}
}
```

- [x] **Step 2: Run and confirm failure**

Run: `go test -run TestTurnSegmentPlayerRoundTrip ./pkg/entity/`
Expected: FAIL, unknown field `Player`.

- [x] **Step 3: Implement**

```go
// Player marks the utterance as the protagonist's own line. It renders and
// plays exactly like any other speech beat; the flag lets the chronicle skip
// the duplicate action block that would otherwise print the same words.
Player bool `json:"player,omitempty"`
```

- [x] **Step 4: Run the test**

Run: `go test -count=1 ./pkg/entity/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/entity/segment.go pkg/entity/history_test.go
git commit -m "feat(entity): mark the player's own line as a speech beat"
```

---

### Task 2: Build and prepend the player beat

**Files:**
- Create: `pkg/engine/player_segment.go`
- Modify: `pkg/engine/orchestrator.go`
- Create: `pkg/engine/player_segment_test.go`

- [x] **Step 1: Write the failing tests**

```go
func TestPlayerSegmentForSay(t *testing.T) {
	got := playerSegment("say", "  I draw my blade. ", "sean", "Sean")
	if got == nil || !got.Player || got.SpeakerID != "sean" || got.Kind != entity.SegmentSpeech {
		t.Fatalf("unexpected segment: %+v", got)
	}
}

func TestPlayerSegmentIgnoredForNonSpeechModes(t *testing.T) {
	for _, mode := range []string{"do", "story", "roll", "opening", ""} {
		if got := playerSegment(mode, "text", "sean", "Sean"); got != nil {
			t.Errorf("mode %q produced a player beat: %+v", mode, got)
		}
	}
}
```

- [x] **Step 2: Run and confirm failure**

Run: `go test -run TestPlayerSegment ./pkg/engine/`
Expected: FAIL, undefined `playerSegment`.

- [x] **Step 3: Implement `pkg/engine/player_segment.go`**

```go
// playerSegment returns the speech beat for the player's own utterance, or nil
// when the input is an action rather than speech.
func playerSegment(mode, input, playerID, playerName string) *entity.TurnSegment {
	if !strings.EqualFold(strings.TrimSpace(mode), "say") {
		return nil
	}
	text := strings.TrimSpace(input)
	if text == "" {
		return nil
	}
	return &entity.TurnSegment{
		Kind:      entity.SegmentSpeech,
		Speaker:   playerName,
		SpeakerID: playerID,
		Text:      text,
		Player:    true,
	}
}
```

- [x] **Step 4: Prepend it**

In `ProcessActionStream`, immediately after `turn.Segments = buildTurnSegments(...)`:

```go
if beat := playerSegment(mode, actionInput, o.playerID, playerName); beat != nil {
	turn.Segments = append([]entity.TurnSegment{*beat}, turn.Segments...)
}
```

Resolve `playerName` from the player entity (the orchestrator stores `playerID`; a small store lookup or an existing accessor provides the display name). If no name is available, fall back to `playerID`.

- [x] **Step 5: Run the engine tests**

Run: `go test -count=1 ./pkg/engine/`
Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add pkg/engine/player_segment.go pkg/engine/player_segment_test.go pkg/engine/orchestrator.go
git commit -m "feat(engine): record the player's own line as speech"
```

---

### Task 3: Propagate the marker through the DTOs

**Files:**
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/turn_test.go`

- [x] **Step 1: Write the failing test**

Add a test that builds a `TurnDTO` for a turn whose first segment is marked `Player` and asserts `Segments[0].Player` is true. Use the existing DTO test helpers.

- [x] **Step 2: Implement**

Add `Player bool `json:"player,omitempty"`` to `SegmentDTO` and set `Player: segment.Player` in `segmentDTOs`. Audio URL generation already runs for every segment, so the player beat gets one.

- [x] **Step 3: Run the GUI tests**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS.

- [x] **Step 4: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/turn_test.go
git commit -m "feat(gui): expose the player's speech beat to the client"
```

---

### Task 4: Display the player's line as speech, once

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/components/ChronicleView.tsx`
- Modify: `frontend/src/components/TurnSegments.tsx`
- Modify: `frontend/src/components/StoryTheater.tsx`

- [x] **Step 1: Types**

Add `player?: boolean` to `TurnSegment`.

- [x] **Step 2: Suppress the duplicate action block**

In `ChronicleView`, only render the `turn.input_text` action block when the turn has no player speech beat:

```tsx
const hasPlayerBeat = (turn.segments ?? []).some((segment) => segment.player);
{turn.input_text && !hasPlayerBeat && (
  <div className="flex items-start gap-3 ...">
    <span className="text-amber-400 ...">[{turn.mode || 'Action'}]</span>
    <span>{turn.input_text}</span>
    {turn.outcome && <span className="ml-auto ...">{turn.outcome}</span>}
  </div>
)}
```

Keep the block for `do`/`story`/`roll` turns, which have no player beat.

- [x] **Step 3: Render the beat as ordinary speech**

In `TurnSegments`, the existing `kind === 'speech'` branch already renders `speaker` + quoted text; the player beat needs no new branch. Optionally add a subtle marker (a "You" chip or an amber tint) driven by `segment.player`, but keep text, quoting, and layout identical to other speakers. Ensure the beat still participates in `useSegmentPlayback` so it plays first.

- [x] **Step 4: Story theater**

In `StoryTheater`, confirm the player beat renders through the same speech path as other speakers and is not filtered or treated as narration.

- [x] **Step 5: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [x] **Step 6: Commit**

```bash
git add frontend/src/types.ts frontend/src/components/ChronicleView.tsx frontend/src/components/TurnSegments.tsx frontend/src/components/StoryTheater.tsx
git commit -m "feat(frontend): show the player's line as speech instead of a say block"
```

---

### Task 5: Verification

- [x] **Step 1: Backend gate**

Run: `mise run test:backend` and `mise run lint`
Expected: all tests pass, `go vet` clean.

- [x] **Step 2: Frontend gate**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [x] **Step 3: Manual smoke**

1. Submit a `say` action and confirm the line appears once, as a speech beat with the player's name and quoted text.
2. Confirm the line is spoken in the player's voice first, then the narration.
3. Submit a `do` action and confirm the action block is unchanged and no speech beat is added.
4. Replay the turn and confirm the player beat plays in order.
5. Export the campaign and confirm the player's line prints as ordinary speech and is audible.
