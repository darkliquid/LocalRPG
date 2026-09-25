# Design Spec: Turn Audio Feedback, Regeneration, and Codex Voice & Portrait Enhancements

**Date:** 2026-09-25  
**Status:** Approved  
**Target:** `frontend` (`TurnSegments.tsx`, `StoryTheater.tsx`, `CodexDrawer.tsx`, `types.ts`, `client.ts`), `pkg/gui` (`service.go`, `server.go`), `pkg/media` (`pipeline.go`, `player.go`)

---

## 1. Executive Summary

During gameplay, audio playback and text-to-speech (TTS) synthesis can take several seconds depending on the selected TTS provider and sentence length. Currently:
1. Play and Stop buttons do not indicate whether a TTS synthesis request is currently in flight, whether audio was loaded instantly from disk/cache, or if a provider error occurred.
2. The Stop button remains displayed and enabled even when no audio is playing, while the Play button can be clicked during playback.
3. There is no mechanism to force regenerate speech when voice tuning or settings change.
4. In the Codex, the voice provider catalogue and presets picker duplicate settings functionality and clutter the screen; the "Apply Voice Archetype" dropdown overflows off the edge of the panel; and there is no preview button to audition voices for a character.
5. Character portraits (generated for visual novel mode and speech segments) are not visible on the character's Codex sheet.

This specification designs:
1. **Interactive Audio Status & Control Bar**: A stateful turn audio control bar featuring spinners during synthesis, dynamic button states (Stop enabled only during playback, Play disabled during playback), clear disk-cache vs provider-generation status indicators, and a Force Regenerate button.
2. **Audio State Polling & Synchronization**: Active playback status tracking (`GET /api/audio/status`) for desktop/server audio device playback.
3. **Unified High-Resolution Portrait Display**: The same high-resolution 3/4 bust portrait asset (`/api/game/{id}/character/{character_id}/portrait`) displayed in full in Story Theater is cleanly scaled down for Codex note headers and Chronicle speech avatars.
4. **Streamlined Codex Voice UI**: Removal of the redundant voice catalogue from Codex, addition of gender/demographic voice tags to the Archetype dropdown with overflow constraints, and an inline Voice Preview audition button.

---

## 2. Architecture & Data Flow

```
                      +-----------------------------+
                      | User clicks Play / Regen    |
                      +--------------+--------------+
                                     |
             +-----------------------+-----------------------+
             |                                               |
             v                                               v
  [Server / Native Device]                         [Browser HTML5 Audio]
  - State: 'generating'                            - State: 'generating'
  - Spinners on Play/Stop                          - Spinners on Play/Stop
  - POST /api/.../play?force=0|1                   - useSegmentPlayback fetch
             |                                               |
             v                                               v
  - Detects cached clips vs                        - Audio buffered & ready
    fresh synthesis                                - Transition: 'playing'
  - Return HTTP 200/204                            - Stop button enabled
  - Transition: 'playing'                          - Play button disabled
  - Poll GET /api/audio/status                     - Audio ended -> 'idle'
  - On finish -> 'idle'
```

---

## 3. Detailed Component Designs

### 3.1 Turn Audio Controls & Status Presentation (`TurnSegments.tsx`)

#### Playback States
The component tracks local audio state:
```typescript
type TurnAudioState = 'idle' | 'generating' | 'playing' | 'error';
```

#### Button Behavior & Icons
- **Play Turn Button**:
  - `idle`: Enabled, displays `<Play />` icon with `"Play turn"`. Clicking transitions to `generating` and triggers playback.
  - `generating`: Disabled, displays `<Loader2 className="animate-spin" />` with `"Generating speech..."`.
  - `playing`: Disabled (`opacity-40 cursor-not-allowed`).
  - `error`: Enabled, displays `<Play />` with `"Retry speech"`.
- **Stop Button**:
  - `playing`: Enabled, displays `<Square />` with red/rose highlight (`text-rose-400 border-rose-500/50 hover:bg-rose-500/20 cursor-pointer`).
  - `idle`, `generating`, `error`: Disabled (`opacity-30 cursor-not-allowed pointer-events-none`).
- **Regenerate Button**:
  - Displays `<RotateCw />` icon button with tooltip `"Force regenerate speech"`.
  - Enabled when `idle` or `error`; disabled during `generating` or `playing`.
  - Clicking triggers turn synthesis with `force=true`.

#### Status Indicator
Adjacent to the buttons, an inline status chip provides clear visibility:
- **Generating**: Pulsing purple dot + `"Rendering speech (calling TTS provider)..."`.
- **Loaded from Cache**: Brief green indicator + `"Audio loaded from cache"`.
- **Error**: Red alert icon + `"Speech error: <error_message>"`.

---

### 3.2 Backend Force Regeneration Support (`pkg/gui/service.go`, `server.go`)

#### Endpoint Parameter
- `POST /api/game/{id}/turn/{n}/play?force=1`
- `GET /api/game/{id}/turn/{n}/segment/{i}/audio?force=1`

#### Cache Invalidation Flow
When `force=1` (or `force=true`) is requested:
1. `Service.PlayTurnAudio` and `Service.GetSegmentAudio` accept a `force bool` parameter.
2. When `force` is true, the `media.TTSPipeline` bypasses the existing disk cache file, calls the TTS provider to generate fresh audio bytes, and overwrites the content-addressed cache file.
3. The newly generated clip durations are measured and returned to the player and client.

---

### 3.3 Server Playback Polling & State Sync (`App.tsx` & `useAudioStatus`)

When `serverPlayback` is active:
1. When `playTurnAudio` or `playSegmentAudio` is invoked, `App.tsx` marks playback as `generating`.
2. Upon HTTP 200/204 completion from the backend, playback state transitions to `playing`.
3. While `playing`, `App.tsx` polls `APIClient.audioStatus()` every 500ms.
4. When `res.playing === false`, state reverts to `idle` and polling stops.
5. If `stopAudio()` is clicked, state immediately transitions to `idle`.

---

### 3.4 Codex Drawer Voice & Portrait Enhancements (`CodexDrawer.tsx`)

#### 1. Character Portrait Display
- In `CodexDrawer`, when `entity.type === 'character'`:
  - Render an avatar card (56×56px, rounded-xl, border border-white/10 shadow-lg overflow-hidden) in the header next to `entity.name`.
  - Source: `/api/game/${gameID}/character/${entity.id}/portrait`.
  - Displays the single high-resolution portrait generated for visual novel mode and speech segments, scaled down with crisp object-cover styling.
  - Falls back seamlessly to the procedural SVG bust if pending or disabled.

#### 2. Cleaned Voice UI & Catalog Removal
- Remove `<VoiceCatalogPicker ttsConfig={ttsConfig} onAddProfile={onAddProfile} />` and associated imports from `CodexDrawer`.
- The Codex is for reading/editing character lore and assigning presets, not browsing/adding external voice providers.

#### 3. Voice Archetype Dropdown with Demographic Tags & Overflow Protection
- Modify option text in `applyVoiceArchetype`:
  - Format: `"{p.name} ({p.voice_id}){tagsStr}"` where `tagsStr` formats `tags` if present (e.g. `p.tags: ["female", "calm"]` -> `" [female, calm]"`).
- Styling:
  - Add `max-w-[280px] truncate` to the `<select>` element to prevent overflowing outside the drawer on narrow screens or long voice IDs.

#### 4. Voice Preview Audition Button
- Add a preview button (`<Volume2 />`) beside the archetype dropdown.
- When clicked:
  - Takes the currently selected voice profile from the dropdown (or the character's currently assigned voice in frontmatter).
  - Calls `APIClient.testProvider` with category `'tts'` and prompt `"Greetings. I am {characterName}, ready for the journey."` using the voice profile's ID, pitch, and speech rate.
  - Plays the returned audio data URI via `audioPreview.ts` with loading spinner during synthesis.

---

## 4. Test Strategy

1. **Backend Integration Tests (`pkg/gui`)**:
   - Verify `POST /api/game/{id}/turn/{n}/play?force=1` passes force flag through `Service.PlayTurnAudio`.
   - Verify `GET /api/game/{id}/turn/{n}/segment/{i}/audio?force=1` re-synthesizes audio even when cached on disk.
2. **Frontend Type Checking & Build**:
   - Verify `types.ts`, `client.ts`, `TurnSegments.tsx`, and `CodexDrawer.tsx` compile cleanly with strict TypeScript checks (`npm --prefix frontend run build`).
3. **Manual Flow Verification**:
   - Verify Play turns into spinner while generating; Stop button disabled until speech plays.
   - Verify Stop button enables and Play button disables during speech.
   - Verify Regenerate button re-synthesizes speech.
   - Verify Codex character header renders the high-res portrait scaled down.
   - Verify Codex voice dropdown displays tags and preview button speaks the character greeting without layout overflow.
