# Voice Change Propagation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a voice edited in the codex take effect for the character's next line, in both server-side and browser playback.

**Architecture:** Route every speaker-to-voice lookup through `harness.ResolveSpeakerVoice` (defined by the GM-invented-characters plan), version each segment's audio URL with the content cache key so a changed voice changes the URL, and mark audio responses `no-store`.

**Tech Stack:** Go 1.27.1, React 19, TypeScript.

**Spec:** `docs/superpowers/specs/2026-09-22-voice-change-propagation-design.md`

**Dependency:** Task 1 requires `harness.ResolveSpeakerVoice` from `docs/superpowers/plans/2026-09-22-gm-invented-character-voices.md` Task 4. Land that first, or land the shared helper as the first step here.

---

## File Structure Map

| File Path | Responsibility |
| :--- | :--- |
| `pkg/gui/service.go` | `voiceFor`; `segmentDTOs` versioned audio key/URL |
| `pkg/gui/server.go` | `Cache-Control: no-store` on audio responses |
| `pkg/gui/types.go` | `AudioKey` on `SegmentDTO` |
| `pkg/harness/extractor.go` | Shared `ResolveSpeakerVoice` |
| `pkg/export/script.go` | Export resolver uses the shared helper |
| `frontend/src/types.ts` | Optional `audio_key` on the segment type |

---

### Task 1: Resolve the voice through the shared helper

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/service_test.go`

- [x] **Step 1: Write the failing test**

Add a test that saves a character with a display name different from its ID, then resolves the voice by display name.

```go
func TestVoiceForResolvesDisplayName(t *testing.T) {
	gameID, svc := setupTestGame(t)
	if err := svc.SaveEntity(context.Background(), gameID, "captain-kaelen",
		"---\nid: captain-kaelen\nname: Captain Kaelen\ntype: npc\nvoice:\n  voice_id: af_bella\n---\nWatch.\n"); err != nil {
		t.Fatal(err)
	}
	got := svc.voiceFor(gameID)("Captain Kaelen")
	if got == nil || got.VoiceID != "af_bella" {
		t.Fatalf("expected Captain Kaelen's voice, got %+v", got)
	}
}
```

- [x] **Step 2: Run and confirm failure**

Run: `go test -run TestVoiceForResolvesDisplayName ./pkg/gui/`
Expected: FAIL (nil voice).

- [x] **Step 3: Implement**

Replace the closure body in `Service.voiceFor`:

```go
return func(speakerID string) *entity.VoiceConfig {
    return harness.ResolveSpeakerVoice(store, speakerID)
}
```

- [x] **Step 4: Run the test**

Run: `go test -run TestVoiceForResolvesDisplayName ./pkg/gui/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/service_test.go
git commit -m "fix(gui): resolve a speaker's voice by display name as well as id"
```

---

### Task 2: Version the segment audio URL by its content key

**Files:**
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/service_test.go`
- Modify: `frontend/src/types.ts`

- [x] **Step 1: Add the DTO field**

In `pkg/gui/types.go`:

```go
type SegmentDTO struct {
	Kind      string  `json:"kind"`
	Speaker   string  `json:"speaker,omitempty"`
	SpeakerID string  `json:"speaker_id,omitempty"`
	Text      string  `json:"text"`
	AudioURL  string  `json:"audio_url,omitempty"`
	AudioKey  string  `json:"audio_key,omitempty"`
	Duration  float64 `json:"duration"`
}
```

Mirror `audio_key?: string` on `TurnSegment` in `frontend/src/types.ts`.

- [x] **Step 2: Write the failing test**

```go
func TestSegmentAudioURLChangesWithVoice(t *testing.T) {
	gameID, svc := setupTestGame(t)
	// save a character with voice af_bella, then change it to am_adam,
	// building the DTO each time; assert the audio_url differs.
}
```

- [x] **Step 3: Run and confirm failure**

Run: `go test -run TestSegmentAudioURLChangesWithVoice ./pkg/gui/`
Expected: FAIL (URLs identical).

- [x] **Step 4: Implement**

Extend `segmentDTOs` with a resolver that returns the segment's current voice:

```go
func segmentDTOs(segments []entity.TurnSegment, gameID string, turnNumber int, audioAvailable bool, resolve func(string) string, voiceFor func(string) *entity.VoiceConfig) []SegmentDTO
```

When `audioAvailable`, compute the key and append a short version:

```go
voiceID, pitch, rate := "", 0.0, 0.0
if v := voiceFor(speakerRef); v != nil {
    voiceID, pitch, rate = v.VoiceID, v.Pitch, v.SpeechRate
}
key := media.ComputeAudioCacheKeyWithRate(speakerRef, voiceID, pitch, rate, segment.Text)
dto.AudioKey = key
dto.AudioURL = fmt.Sprintf("/api/game/%s/turn/%d/segment/%d/audio?v=%s", gameID, turnNumber, i, key[:12])
```

where `speakerRef` is `segment.SpeakerID` when set, else `segment.Speaker`. Update the `turnDTO` caller to pass `s.voiceFor(gameID)` (it already has a store-backed lookup available). Keep `ComputeAudioCacheKeyWithRate` as-is; it already hashes speaker, voice, prosody, and text.

- [x] **Step 5: Run tests**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS.

- [x] **Step 6: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [x] **Step 7: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/service_test.go frontend/src/types.ts
git commit -m "fix(gui): give a segment's audio url a voice-sensitive version"
```

---

### Task 3: Stop the browser reusing stale clips

**Files:**
- Modify: `pkg/gui/server.go`
- Modify: `pkg/gui/server_test.go`

- [x] **Step 1: Write the failing test**

Add a test that requests a segment audio route and asserts `Cache-Control: no-store` is present.

- [x] **Step 2: Run and confirm failure**

Run: `go test -run TestSegmentAudioNoStore ./pkg/gui/`
Expected: FAIL.

- [x] **Step 3: Implement**

Before serving the segment or turn audio, set the header:

```go
w.Header().Set("Cache-Control", "no-store")
```

Apply it to both the `turn/{n}/audio` and `segment/{i}/audio` responses in `pkg/gui/server.go` (see the `GetSegmentAudio`/`PlayTurnAudio` branches around lines 214 and 258).

- [x] **Step 4: Run tests**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/server.go pkg/gui/server_test.go
git commit -m "fix(gui): never cache turn audio in the browser"
```

---

### Task 4: Use the shared resolver in export

**Files:**
- Modify: `pkg/export/script.go`
- Modify: `pkg/export/script_test.go`

- [x] **Step 1: Implement**

Replace `speechResolver.voiceFor`'s body with `return harness.ResolveSpeakerVoice(r.store, speakerID)` (import `harness` if needed and keep the store field it already holds).

- [x] **Step 2: Run tests**

Run: `go test -count=1 ./pkg/export/`
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add pkg/export/script.go pkg/export/script_test.go
git commit -m "fix(export): resolve speaker voices by name when rendering audio"
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

Open a campaign, change a character's voice in the codex, submit a turn in which the character speaks, and confirm both the auto-play and a replay use the new voice.
