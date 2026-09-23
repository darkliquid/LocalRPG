# Design Spec: Voicing Characters the GM Invents

**Date:** 2026-09-22
**Status:** Draft
**Target:** `pkg/engine`, `pkg/harness`, `pkg/gui`, `pkg/entity`

---

## 1. Executive Summary

When the GM invents a character mid-turn (a guard, a shopkeeper, a villain), the character should:

1. be added to the codex as a note,
2. be given a voice chosen from the character's description and attributes, and
3. have that voice used for the character's own lines **in the same turn**, before any audio is synthesised.

Today the note is created, but the voice is frequently never assigned, and even when it is, the character's speech can still be read in the narrator's voice. This spec fixes the type gate that blocks assignment, broadens the matching corpus to include the extracted description, and makes speaker-to-entity resolution robust at playback time so a line attributed to a newly invented character always reaches that character's voice.

---

## 2. Findings

### 2.1 Only `type: character` gets a voice, but the extractor and content use `npc`

`Timeline.recordEntities` assigns a voice only when the entity type is exactly `character`:

```go
if ent.Type == "character" {
    harness.AssignVoiceProfile(ent, t.voiceProfiles)
}
```
(`pkg/engine/timeline.go:122`)

`harness.AssignVoiceProfile` repeats the same gate (`pkg/harness/extractor.go:61`). However:

- the extractor prompt permits `character|location|item|faction|arc`, but hand-authored and test content use `type: npc` (`pkg/gui/service_test.go:62`, `pkg/gui/service_test.go:823`), and the graph legend treats `npc` as a distinct character type (`frontend/src/components/GraphDrawer.tsx:67`);
- models routinely return `npc`, `person`, or a bare name for an invented character.

Any of those types disables voice assignment, so the invented character's speech falls back to the narrator voice.

### 2.2 Matching ignores the extracted description

`AssignVoiceProfile` scores profiles against `ent.Name + " " + ent.Body` (`pkg/harness/extractor.go:65`). It never considers:

- `ent.Appearance`, which is exactly where extraction records how a character looks and sounds, and
- `ent.Aliases`, the alternative names the extractor or narrator may use.

A character introduced with a vivid appearance but a thin body therefore only ever reaches the deterministic hash fallback, which can pick a voice that contradicts the description.

### 2.3 Segment attribution is good but not guaranteed

`buildTurnSegments` (`pkg/engine/segments.go:17`) deliberately resolves speakers against entities the extractor "is about to create" (`proposedSpeakerID`), so a new NPC's first line can carry a `SpeakerID`. That is the right design. Two gaps remain:

- if the extractor is disabled, fails, or does not list the speaker, the segment carries only a display name;
- at playback, `Service.voiceFor` looks up `store.GetEntity(speakerID)` by the **exact** ID (`pkg/gui/service.go:1462`). A segment whose `SpeakerID` is empty falls back to `segment.Speaker` (a display name such as `Lady Evelyn`) and the lookup fails, so the line reads in the narrator voice.

### 2.4 Ordering is already correct, and should be made explicit

`ProcessActionStream` builds segments, then calls `Timeline.RecordTurn`, which creates and voices entities (`pkg/engine/orchestrator.go:496`, `:553`). `Service.Run` only then emits the turn and (optionally) starts `PlayTurnAudio` (`pkg/gui/service.go:1128`). Synthesis is therefore after persistence. This ordering is an invariant worth stating and testing, because moving synthesis earlier would silently reintroduce the bug.

---

## 3. Design

### 3.1 A single character-type predicate

Add to `pkg/entity`:

```go
// IsCharacterType reports whether a type names a speaking being. Content authors,
// extractor models, and the graph all use different spellings for the same idea.
func IsCharacterType(t string) bool {
    switch strings.ToLower(strings.TrimSpace(t)) {
    case "character", "npc", "person", "creature":
        return true
    }
    return false
}
```

Use it in both `Timeline.recordEntities` and `AssignVoiceProfile`, replacing the `== "character"` gates. This is the smallest change that makes invented NPCs voiceable and keeps a single definition of "a speaking being".

### 3.2 Match against everything the turn knew

Change `AssignVoiceProfile` to search `Name + " " + Body + " " + Appearance + " " + strings.Join(Aliases, " ")`. Keep the existing precedence (literal profile ID, then tag score, then deterministic hash) so no existing voice assignment changes except for the better.

Add descriptive tags to the Kokoro profiles where they are missing so tags like `male`, `female`, `british`, `american`, `elder`, `young`, `deep`, `soft` are reliably matched. The profiles already carry these; audit them during implementation and add any obvious omissions.

### 3.3 Voice every unvoiced character-like entity

`recordEntities` currently assigns only for entities it is about to write. Extend the loop so that:

- a newly created character-like entity gets a voice, and
- a character-like entity that was **matched** to an existing note and has no voice gets one the next time it is extracted.

`AssignVoiceProfile` already refuses to overwrite an existing `Voice`, so an authored or codex-chosen voice is never clobbered. This is important for the companion spec on codex voice edits.

### 3.4 Resolve speakers by name as a last resort

Make speaker resolution shared and layered. Add a resolver used by both `Service.voiceFor` and the export `speechResolver`:

1. exact entity ID;
2. `entity.Slugify(display name)` as an ID;
3. `entity.Slugify(display name)` against the entity **name** and **aliases** (reusing the existing `harness` name/alias matching helpers if suitable, otherwise a small store query).

This means a segment that carries only a display name still reaches the right voice, and it also fixes the propagation bug in the companion spec. The resolver belongs in one place, not duplicated in `pkg/gui` and `pkg/export`.

### 3.5 State and test the ordering invariant

Document in `Timeline.RecordTurn` and `Service.Run` that **no segment is synthesised until the turn's entities are written and voiced**. Add tests that fail if a new character's segment is synthesised before its note exists.

### 3.6 Fallback when there is no extractor

When extraction is disabled or fails, invented speakers cannot be created automatically. In that case:

- resolve what can be resolved (existing entities by name),
- leave the rest in the narrator voice, and
- surface the unresolved speakers in the existing `segment.build` trace event (`unresolved` is already logged at `pkg/engine/orchestrator.go:501`) so the continuity findings can nudge the player to add the note.

No new automatic entity creation without a model: that would be guesswork.

---

## 4. Data Flow

```text
GM narration
  └─ dialogue.Parse + extractor dialogue ─► segments (speaker name, maybe SpeakerID via proposed entities)
       └─ resolve(store, candidate) ─► SpeakerID
            └─ Timeline.RecordTurn
                 ├─ create/match entities
                 ├─ IsCharacterType → AssignVoiceProfile(Name+Body+Appearance+Aliases)
                 └─ write notes + index
                      └─ (after RecordTurn returns) emit turn
                           └─ PlayTurnAudio → voiceFor(speaker) resolves ID → persisted voice
```

---

## 5. File Map

| Action | Path | Description |
| :--- | :--- | :--- |
| Modify | `pkg/entity/entity.go` | Add `IsCharacterType` |
| Modify | `pkg/engine/timeline.go` | Use the predicate; voice matched-but-unvoiced characters |
| Modify | `pkg/harness/extractor.go` | Use the predicate; widen the matching corpus |
| Modify | `pkg/harness/*_test.go` | Tests for `npc` types, appearance matching, no-overwrite |
| Create | `pkg/gui/speaker_resolver.go` | Shared ID → slug → name/alias resolution |
| Modify | `pkg/gui/service.go` | `voiceFor` uses the shared resolver |
| Modify | `pkg/export/script.go` | `speechResolver.voiceFor` uses the shared resolver |
| Modify | `pkg/engine/orchestrator.go` | Document the ordering invariant; keep unresolved-speaker trace |

---

## 6. Acceptance Criteria

1. A turn that invents an `npc`-typed character persists a note with a `voice` chosen from its name/body/appearance.
2. The invented character's speech segment is synthesised with that voice, not the narrator's.
3. A codex-authored voice is never overwritten by later extraction.
4. A segment carrying only a display name resolves to the character's voice.
5. `go test -count=1 ./...` and `go vet ./...` pass.
