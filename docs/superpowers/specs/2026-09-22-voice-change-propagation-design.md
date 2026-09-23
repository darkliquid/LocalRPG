# Design Spec: Propagating a Codex Voice Change to New Speech

**Date:** 2026-09-22
**Status:** Draft
**Target:** `pkg/gui`, `pkg/harness`, `pkg/export`, `pkg/media`, `frontend`

---

## 1. Executive Summary

Editing a character's voice in the codex does not take effect for that character's later lines: new dialogue is still read in the previous voice. The persisted note is correct, so the defect is in how the voice is *looked up* at synthesis time and in how the client *caches* the resulting clip.

Two fixes close it:

1. resolve a speaker reference to an entity (and therefore to its current voice) by ID **and** display name, and
2. stop the browser reusing a clip for a segment URL whose voice has since changed.

Because the audio cache is content-addressed by speaker, voice, prosody, and text, a changed voice already produces a new clip on disk; the fix makes sure the new clip is what actually plays.

---

## 2. Findings

### 2.1 The voice lookup is by exact ID only

`Service.voiceFor` (`pkg/gui/service.go:1460`) does:

```go
ent, err := store.GetEntity(speakerID)
if err != nil || ent == nil {
    return nil
}
return ent.Voice
```

`speakerID` is taken from `segment.SpeakerID`, falling back to `segment.Speaker` (a display name) (`pkg/media/tts.go:127`). When it is a display name such as `Captain Kaelen`, `GetEntity("Captain Kaelen")` fails and the utterance falls back to the narrator voice. If it is an ID it works, which is why the bug is intermittent: it depends on whether the prose used the `Name: "…"` convention and whether the name matched.

`pkg/export/script.go:70` has the same exact-ID lookup.

### 2.2 The browser can serve a stale clip for a stable URL

`segmentDTOs` builds a **stable** URL per segment:

```go
dto.AudioURL = fmt.Sprintf("/api/game/%s/turn/%d/segment/%d/audio", gameID, turnNumber, i)
```
(`pkg/gui/service.go:249`)

`GetSegmentAudio` synthesises a new, content-addressed file when the voice changes, and the route serves it with `http.ServeFile` (`pkg/gui/server.go:258`). Because the URL is unchanged, a browser that already fetched that URL can reuse its cached body within the session, so replaying a turn after a voice edit can still play the old voice. The server-side content cache is correct; the HTTP cache is the leak.

### 2.3 The content cache itself is sound

`SynthesizeUtterance` hashes `speakerID`, the voice hash (`provider:voiceID:pitch:rate`), and the text (`pkg/media/tts.go:151`). A changed voice yields a different key and a different file. No fix is needed here; the spec keeps it as the reason the fix is cheap.

### 2.4 Authored voices are not overwritten

`AssignVoiceProfile` returns early when `ent.Voice != nil` (`pkg/harness/extractor.go:61`), and `MergeExtractedEntity` copies the existing entity, so a codex-edited voice survives re-extraction. This should be locked down by a test in the companion spec; it is not the cause here.

---

## 3. Design

### 3.1 Resolve speakers through the shared helper

Replace the exact-ID lookups in `Service.voiceFor` and the export `speechResolver` with `harness.ResolveSpeakerVoice(store, speakerRef)` (defined in the companion spec, `docs/superpowers/specs/2026-09-22-gm-invented-character-voices-design.md`). It tries the exact ID first, then `harness.ResolveSpeakerID` (slug, exact name, alias, shared name tokens), so both an ID and a display name reach the character's current voice.

This is the single most likely cause of the reported staleness, and it is a strict widening: an entity that resolved before still resolves.

### 3.2 Version the clip URL by its content key

Give `SegmentDTO` an `AudioKey` and append it as a query parameter:

```go
dto.AudioKey = media.ComputeAudioCacheKeyWithRate(segment.SpeakerID, voiceID, pitch, rate, segment.Text)
dto.AudioURL = fmt.Sprintf("/api/game/%s/turn/%d/segment/%d/audio?v=%s", gameID, turnNumber, i, shortKey(dto.AudioKey))
```

`shortKey` is the first 12 hex characters. The route ignores the query parameter for resolution and only uses it to make the URL content-specific, so a voice change changes the URL and the browser fetches the new clip.

Then, belt-and-braces, set `Cache-Control: no-store` on the segment and turn audio responses. The app is local; the server's content cache still makes repeat synthesis instant, and disabling browser caching removes an entire class of staleness.

`segmentDTOs` needs the resolved voice to compute the key. Extend its signature with a `voiceKey func(speakerID string) (voiceID string, pitch, rate float64, ok bool)` (or pass the `harness.ResolveSpeakerVoice` result) supplied by `Service`, which already has the store.

### 3.3 Make the auto-play path use live voices

`PlayTurnAudio` already calls `GetSegmentAudio`, which is live. With 3.1 and 3.2 in place, both server-side and browser playback use the current voice. No change is needed beyond the shared resolver.

### 3.4 Invalidate nothing

Because clips are content-addressed, a voice edit does not require deleting old clips; they simply become unreferenced. A future cache-eviction task can garbage-collect unreferenced audio, but that is out of scope.

---

## 4. Data Flow

```text
Codex save ─► SaveEntity (file + index)
                  │
later turn ─► segment(speaker display name or id)
                  └─ harness.ResolveSpeakerVoice(store, ref)
                       ├─ exact ID ─► current voice
                       └─ ResolveSpeakerID(ref) ─► current voice
                            └─ ContentCache key(speaker, voice, prosody, text) ─► NEW clip
                                 └─ SegmentDTO.AudioURL?v=<shortkey> ─► browser fetches NEW clip
```

---

## 5. File Map

| Action | Path | Description |
| :--- | :--- | :--- |
| Modify | `pkg/harness/extractor.go` | `ResolveSpeakerVoice` (shared with the invented-characters spec) |
| Modify | `pkg/gui/service.go` | `voiceFor` and `segmentDTOs` use it; versioned `audio_url`; `no-store` |
| Modify | `pkg/gui/server.go` | `Cache-Control: no-store` on audio routes |
| Modify | `pkg/gui/types.go` | `AudioKey` on `SegmentDTO` |
| Modify | `pkg/export/script.go` | Export resolver uses the shared helper |
| Modify | `frontend/src/types.ts` | Optional `audio_key` on `TurnSegment` |

---

## 6. Acceptance Criteria

1. Saving a new voice for a character, then playing a later line by that character, uses the new voice for both server-side and browser playback.
2. A segment that carries only a display name resolves to that character's voice.
3. The segment audio URL changes when the resolved voice changes.
4. Audio responses are not reused from the browser cache across a voice change.
5. `go test -count=1 ./...`, `go vet ./...`, and `npx tsc --noEmit` pass.
