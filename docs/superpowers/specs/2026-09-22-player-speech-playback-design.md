# Design Spec: Speaking the Player's Own Line

**Date:** 2026-09-22
**Status:** Draft
**Target:** `pkg/engine`, `pkg/entity`, `pkg/gui`, `pkg/export`, `frontend`

---

## 1. Executive Summary

When the player submits a `say` action, their words are shown today as a `[SAY] …` action block and are never spoken. They should instead be handled exactly like any other character's speech: rendered in the chronicle as a normal speech beat (speaker name plus quoted line) and synthesised in the **player character's** voice, played before the narration. The `/say` action block must not also print the same words.

The design promotes the player's line to a first-class speech segment, marks it as the player's own so the UI can avoid the duplicate action block, and relies on the existing speech rendering, playback queue, and cache.

---

## 2. Findings

### 2.1 The player's line is deliberately excluded from segments

`buildTurnSegments` documents the exclusion:

```go
// The player's own line is not a segment
// because the chronicle already shows their submitted action, and repeating it
// reads as a duplicate.
```
(`pkg/engine/segments.go:14`)

The pipeline only synthesises `turn.Segments` (`pkg/media/tts.go:102`), and playback only walks them (`pkg/gui/service.go:1426`), so the player's line is never heard.

### 2.2 The chronicle prints every turn's input as an action block

`ChronicleView` renders `turn.input_text` behind a mode badge for every turn (`frontend/src/components/ChronicleView.tsx:80-90`). For a `say` turn that is exactly the `/say "…"` block the player should not see once the line is shown as speech. Removing the segment exclusion without changing this would duplicate the line.

### 2.3 The turn already records the line and its mode

`Turn.Input` and `Turn.Mode` hold the submitted text and the console mode (`do`/`say`/`story`/`roll`) and are persisted in `history.jsonl`. The mode is exactly the signal needed to decide whether the line is speech.

### 2.4 The player has a voice only after character creation

`ensurePlayerNote` writes no `Voice` (`pkg/engine/game.go:185`). The companion spec `docs/superpowers/specs/2026-09-22-character-creation-design.md` gives the player a voice; this feature depends on it and must still behave when it is absent (fall back to the narrator voice).

### 2.5 Ordinary speech segments already do all the work

`entity.SegmentSpeech` segments render as `speaker` + quoted text, carry `audio_url` in the DTO, and are synthesised with the speaker's voice. The player's line needs no new rendering path, only a marker so the action block can be suppressed.

---

## 3. Design

### 3.1 The player's line is a speech segment

Add to `entity.TurnSegment`:

```go
// Player marks the utterance as the protagonist's own line. It renders and
// plays exactly like any other speech beat; the flag lets the chronicle skip
// the duplicate action block that would otherwise print the same words.
Player bool `json:"player,omitempty"`
```

`SegmentDTO` gains `Player bool `json:"player,omitempty"``, mirrored in `frontend/src/types.ts`.

### 3.2 Building the player beat

Add a helper in `pkg/engine`:

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

`ProcessActionStream` prepends it to `turn.Segments` after `buildTurnSegments`, so it is first in both the persisted record and the playback order. `mode`/`input`/`playerID`/player name are all in scope there (the orchestrator already holds `playerID`).

Decision: only `say` is spoken. `do`, `story`, and `roll` are actions or direction, not dialogue, and voicing them would have the narrator read out stage directions in the hero's voice. A later setting can extend this to quoted text inside `do`.

### 3.3 Rendering

- `frontend/src/components/ChronicleView.tsx` suppresses the `input_text` action block for a turn that has a player speech segment, so the line appears exactly once, as speech. Turns without a player beat keep the block unchanged.
- `frontend/src/components/TurnSegments.tsx` renders the player's beat with the existing speech treatment. It may add a subtle marker (for example a "You" chip or an amber tint) but the text, quoting, and layout match every other speech beat.
- `StoryTheater` and the export web/video renderers print the player's beat as ordinary speech.

### 3.4 Voice and cache

The beat is an ordinary speech segment, so `SynthesizeSegment` resolves `voiceFor(playerID)`. With character creation, that is the player's chosen or auto-assigned voice; without it, the narrator voice. The cache key already includes speaker, voice, prosody, and text (`pkg/media/tts.go:151`), so a change to the player's voice produces a new clip; the companion spec on voice-change propagation makes the URL reflect that.

### 3.5 Persistence and compatibility

`TurnSegment.Player` is `omitempty`, so existing `history.jsonl` records parse unchanged and older turns simply have no player beat. Exports and the TUI that iterate segments need no special case: the beat is speech.

---

## 4. Data Flow

```text
ActionConsole (mode=say, text)
  └─ POST turn ─► ProcessActionStream
       ├─ playerSegment(mode, input, playerID, playerName) ─► speech segment marked Player (prepended)
       ├─ buildTurnSegments(narration) ─► narration/speech beats
       ├─ Timeline.RecordTurn (entities voiced first)
       └─ turn emitted (segments incl. the player's speech)
            ├─ ChronicleView suppresses the input block and renders the beat as speech
            ├─ PlayTurnAudio synthesises the player beat first, then narration
            └─ Export prints the beat as ordinary speech and includes its audio
```

---

## 5. File Map

| Action | Path | Description |
| :--- | :--- | :--- |
| Modify | `pkg/entity/segment.go` | `Player` field |
| Modify | `pkg/engine/orchestrator.go` | Prepend the player beat for `say` turns |
| Create | `pkg/engine/player_segment.go` | `playerSegment` helper |
| Modify | `pkg/gui/types.go` | `Player` on `SegmentDTO` |
| Modify | `pkg/gui/service.go` | Propagate `Player` through `segmentDTOs` |
| Modify | `frontend/src/types.ts` | Optional `player` on `TurnSegment` |
| Modify | `frontend/src/components/ChronicleView.tsx` | Suppress the duplicate action block; pass through |
| Modify | `frontend/src/components/TurnSegments.tsx` | Render the player beat as speech (optional marker) |
| Modify | `frontend/src/components/StoryTheater.tsx` | Render the player beat as speech |

No change is needed in `pkg/scene` or the export renderers beyond what ordinary speech already does.

---

## 6. Acceptance Criteria

1. A `say` turn shows the player's line once, as a speech beat with the player's name and quoted text.
2. No `[SAY] …` action block is shown for that turn.
3. The line is spoken in the player character's voice, before the narration, in both server-side and browser playback.
4. `do`, `story`, and `roll` turns keep their existing action block and add no speech beat.
5. With no player voice configured, the line reads in the narrator voice rather than failing.
6. Old `history.jsonl` records without player beats continue to load and play.
7. Exports print the player's line as ordinary speech and include its audio.
8. `go test -count=1 ./...`, `go vet ./...`, and `npx tsc --noEmit` pass.
