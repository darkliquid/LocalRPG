# Image Generation Triggers and History Design

**Date**: 2026-10-03  
**Status**: Approved  
**Topic**: Scene break image triggers, character appearance evolution, and historical portrait versioning  

---

## 1. Context and Goals

During extended playthroughs in LocalRPG, two key image generation limitations were observed:
1. **Scene breaks are visually lost**: Dramatic temporal leaps (such as *"10 years later..."*) or narrative scene shifts currently reuse the existing location backdrop without generating a new visual. Even within the same physical location, a time skip or scene transition represents a distinct moment that warrants a fresh scene illustration.
2. **Character evolution clobbers visual history**: Characters like Sabon and Vera undergo major narrative transformations (such as aging 10 years, gaining scars, or transforming). Currently, the extractor only populates empty appearances, the portrait worker only generates a portrait once, and when regenerated, it overwrites the original portrait file. Furthermore, the frontend uses a single global portrait state per character, causing historical turns to display the character's new appearance instead of how they looked at that point in time.

### Core Objectives
- **Hybrid Scene Break Detection**: Trigger scene image generation when an explicit `---` markdown horizontal rule occurs in narration or when the extractor identifies a temporal/setting shift.
- **Turn Scene Illustrations**: Asynchronously generate dramatic scene illustrations tied to the specific turn (`turn.image_url`), saved under `assets/scenes/turn-<N>.<ext>`, separate from general location art.
- **Character Evolution Detection**: Enable the extractor to detect significant physical changes (`appearance_changed: true`, `age`, and updated `appearance`), allowing `MergeExtractedEntity` to record evolving appearances.
- **Versioned Portraits**: Generate versioned portrait assets (`assets/portraits/<id>-v<N>.<ext>`), incrementing version numbers without deleting historical versions.
- **Historical Turn Anchoring**: Persist the active portrait version in `history.jsonl` on spoken turn segments (`TurnSegment.SpeakerPortrait`), allowing historical turns to permanently display the era-appropriate portrait.

---

## 2. Architecture & Data Flow

```
                      +-----------------------------+
                      |     GM Narration Output     |
                      +--------------+--------------+
                                     |
             +-----------------------+-----------------------+
             |                                               |
             v                                               v
     Markdown `---` Rule                     Extractor Analysis
     (Explicit divider)                      (Entity extraction + scene_break)
             |                                               |
             +-----------------------+-----------------------+
                                     |
                                     v
                        +--------------------------+
                        |  Turn Processing Flow    |
                        +------------+-------------+
                                     |
        +----------------------------+----------------------------+
        |                                                         |
        v                                                         v
 [Scene Break Triggered]                                 [Character Changed]
 - Mark Turn: SceneBreak = true                          - raw.AppearanceChanged = true
 - Construct Scene Prompt                                - MergeExtractedEntity updates
   (cue/excerpt + location + style)                        appearance & age
 - Enqueue SceneWorker                                   - Enqueue PortraitWorker(v+1)
        |                                                         |
        v                                                         v
 Writes assets/scenes/turn-<N>.<ext>                     Writes assets/portraits/<id>-v<N>.<ext>
 Broadcasts TurnEvent{"scene_image"}                     Broadcasts TurnEvent{"portrait"}
        |                                                         |
        v                                                         v
 ChronicleView displays turn.image_url                   TurnSegment records ?v=<N>
                                                         TurnSegments renders era portrait
```

---

## 3. Detailed Component Specifications

### 3.1 Scene Break Detection & Prompt Construction

1. **Detection Strategy**:
   - **Prose Rule**: Check narration for a standalone markdown horizontal rule (`---` on its own line surrounded by blank lines or whitespace).
   - **Extractor Cue**: Update `Extraction` in `pkg/harness/extractor.go`:
     ```go
     type ExtractedSceneBreak struct {
         Occurred   bool   `json:"occurred"`
         VisualCue  string `json:"visual_cue,omitempty"`
     }

     type Extraction struct {
         Entities       []ExtractedEntity    `json:"entities"`
         Dialogue       []ExtractedDialogue  `json:"dialogue,omitempty"`
         PlayerLocation string               `json:"player_location,omitempty"`
         SceneBreak     *ExtractedSceneBreak `json:"scene_break,omitempty"`
     }
     ```
   - **Prompt Instruction**: The extractor system prompt instructs:
     > "If the narration features a significant scene break, time jump (e.g. '10 years later'), or dramatic shift in setting, set 'scene_break' to {\"occurred\": true, \"visual_cue\": \"Concise visual description of the new scene moment\"}."
   - **Turn Flag**: If `Narration` contains `---` or `Extraction.SceneBreak.Occurred` is true, the turn orchestrator marks the turn as a scene break.

2. **Prompt Composition**:
   - Helper `BuildScenePrompt(cue string, location *entity.Entity, worldStyle string) string`:
     - If `cue` is present: uses `cue`.
     - Otherwise, extracts the paragraph immediately following `---` (up to 200 characters) as the moment's context.
     - Appends the location's name and appearance (if available) and the world's `art_style`.

### 3.2 Asynchronous Scene Worker & Asset Storage

1. **`SceneWorker` Implementation** (`pkg/engine/scene_worker.go`):
   - Mirrored from `PortraitWorker`.
   - Thread-safe deduplication with in-memory `inFlight` map keyed by `gameID:turnNumber`.
   - `Enqueue(gameID string, turnNumber int, prompt string)` triggers generation in a background goroutine.
   - Saves generated image to:
     `assets/scenes/turn-<N>.<ext>`
     within `resolver.GameDir(gameID)`.
   - Extension determined by `media.ArtExtension(imgBytes)`.
2. **API Endpoint & Service**:
   - Route: `GET /api/game/{gameID}/turn/{turnNumber}/scene-image`
   - Service method: `GetTurnSceneImage(ctx, gameID string, turnNumber int) ([]byte, string, error)`
     - Checks `assets/scenes/turn-<turnNumber>.*` on disk.
     - Returns image bytes and content type (`image/png`, `image/webp`, `image/jpeg`, `image/svg+xml`).
     - Returns 404 if not found or generation not completed.
3. **Live Turn Event**:
   - `broadcastSceneImageReady(gameID string, turnNumber int, relPath string)` emits:
     ```json
     {
       "type": "scene_image",
       "turn_number": 10,
       "image_url": "/api/game/<id>/turn/10/scene-image"
     }
     ```
   - Sent via existing SSE `turnstream` or WebSocket listeners.

### 3.3 Character Evolution Detection & Entity Merging

1. **Extractor Evolution Schema**:
   - `ExtractedEntity` receives:
     ```go
     type ExtractedEntity struct {
         ID                string `json:"id"`
         Name              string `json:"name"`
         Type              string `json:"type"`
         Location          string `json:"location,omitempty"`
         Faction           string `json:"faction,omitempty"`
         Appearance        string `json:"appearance,omitempty"`
         Age               string `json:"age,omitempty"`
         AppearanceChanged bool   `json:"appearance_changed,omitempty"`
         Body              string `json:"body"`
     }
     ```
   - Extractor prompt instructs:
     > "When an existing character's physical traits or age visibly alter significantly (e.g. dramatic aging, scars, transformations, haircuts, new cybernetics), set 'appearance_changed': true and populate 'appearance' and 'age' with their new physical state."
2. **Entity Merge Logic (`MergeExtractedEntity`)**:
   ```go
   if raw.AppearanceChanged || merged.Appearance == "" {
       if raw.Appearance != "" {
           merged.Appearance = raw.Appearance
       }
   }
   if raw.Age != "" {
       merged.Age = raw.Age
   }
   ```
   - When `raw.AppearanceChanged` is true on a `character`, the timeline/orchestrator queues the character for portrait version increment.

### 3.4 Versioned Portrait System & Asset Immutability

1. **Entity Frontmatter**:
   - In `pkg/entity/entity.go`:
     ```go
     type Entity struct {
         // ...
         Portrait        string   `yaml:"portrait,omitempty" json:"portrait,omitempty"`
         PortraitVersion int      `yaml:"portrait_version,omitempty" json:"portrait_version,omitempty"`
         PortraitHistory []string `yaml:"portrait_history,omitempty" json:"portrait_history,omitempty"`
     }
     ```
2. **File Structure**:
   - Versioned portrait path: `assets/portraits/<character-id>-v<version>.<ext>`.
   - Example:
     - `assets/portraits/sabon-v1.png`
     - `assets/portraits/sabon-v2.png`
   - **Retention Guarantee**: Historical files (`v1`, `v2`, etc.) are NEVER removed during regeneration. The stale file cleanup only removes mismatched extensions for the *active version* (e.g. removing `sabon-v2.svg` if `sabon-v2.png` was just written).
3. **Portrait Worker Versioning**:
   - When generating the initial portrait (`ent.Portrait == ""`):
     - `version := 1`
     - File: `assets/portraits/<id>-v1.<ext>`
     - Updates frontmatter: `Portrait: "assets/portraits/<id>-v1.<ext>"`, `PortraitVersion: 1`.
   - When generating an updated portrait (`AppearanceChanged` or user `Regenerate`):
     - `version := ent.PortraitVersion + 1` (defaults to 2 if `PortraitVersion` was 0)
     - File: `assets/portraits/<id>-v<version>.<ext>`
     - Appends previous `ent.Portrait` to `ent.PortraitHistory`.
     - Updates frontmatter: `Portrait: newPath`, `PortraitVersion: version`.
   - `Enqueue` condition: Allowed if `ent.Portrait == ""` OR `forceVersion` is set by appearance evolution.

### 3.5 Historical Turn Anchoring in History

1. **Turn Segment Portrait Reference**:
   - In `pkg/entity/segment.go`:
     ```go
     type TurnSegment struct {
         Kind            string `json:"kind"`
         Speaker         string `json:"speaker,omitempty"`
         SpeakerID       string `json:"speaker_id,omitempty"`
         SpeakerPortrait string `json:"speaker_portrait,omitempty"`
         Text            string `json:"text"`
         CheckRef        string `json:"check_ref,omitempty"`
         Player          bool   `json:"player,omitempty"`
     }
     ```
   - When speech segments are constructed in `orchestrator.go` / `segmentDTOs`:
     - Look up speaker's active `ent.PortraitVersion`.
     - Set `SpeakerPortrait = fmt.Sprintf("/api/game/%s/character/%s/portrait?v=%d", gameID, speakerID, version)`.
   - Since `history.jsonl` is append-only, turns 1–9 retain `?v=1`, while turn 10+ records `?v=2`.
2. **Service Portrait Endpoint**:
   - `GET /api/game/{gameID}/character/{characterID}/portrait?v={version}`:
     - If `v` is provided (e.g. `v=1`):
       - Searches for `assets/portraits/<characterID>-v1.<ext>`.
       - Falls back to `portrait_history` entries matching `v1`.
       - Falls back to legacy unversioned `assets/portraits/<characterID>.<ext>`.
     - If `v` is omitted:
       - Returns `ent.Portrait` (the latest version).
     - If no custom file exists:
       - Returns procedural SVG fallback via `GenerateProceduralBustSVG`.

---

## 4. Frontend Integration

1. **Turn Portrait Resolution in `TurnSegments.tsx`**:
   - Update portrait URL selection:
     ```tsx
     const portraitURL = segment.speaker_portrait || segment.portrait_url ||
       (charId && characterPortraits?.[charId]?.url);
     ```
   - Segment-anchored portrait URLs (with `?v=<N>`) take precedence over the global active portrait in `characterPortraits`.
   - Global `characterPortraits` is only used for live streaming segments before turn finalization or legacy turns missing explicit versioning.
2. **Scene Break & Illustration in `ChronicleView.tsx`**:
   - When `turn.image_url` is present:
     - Render scene illustration using the existing lightbox card.
   - Listen for incoming `scene_image` events:
     - Match `turn_number` in `chronicle` state and assign `turn.image_url = event.image_url`.
3. **App State Handling in `App.tsx`**:
   - In `TurnEvent` handler:
     - For `type === "scene_image"`:
       ```ts
       setChronicle(prev => prev.map(t =>
         t.turn_number === event.turn_number
           ? { ...t, image_url: event.image_url }
           : t
       ));
       ```
     - For `type === "portrait"`:
       - Updates `characterPortraits[event.character_id]` with the latest URL (`?v=<N>`).

---

## 5. Error Handling & Backward Compatibility

- **Provider Failure or Disabled**:
  - Image generation runs purely in background workers. If the image provider is disabled or errors, turns finish without blocking.
  - Scene illustration: `image_url` remains empty; narration and audio play as normal without error popups.
  - Character portrait: Falls back to procedural SVG bust without crashing.
- **Legacy Campaigns**:
  - Old games with `assets/portraits/<id>.<ext>` continue to serve the unversioned image directly.
  - Existing turns without `speaker_portrait` fall back to the unversioned `/api/game/{id}/character/{id}/portrait` route.
  - No database migration or file renaming is required.

---

## 6. Testing Strategy

### 6.1 Backend Tests
1. **Extractor Tests (`pkg/harness/extractor_test.go`)**:
   - Test extraction of `scene_break: { occurred: true, visual_cue: "..." }`.
   - Test extraction of `appearance_changed: true` and `age: "38"`.
   - Verify `MergeExtractedEntity` updates appearance and age when `appearance_changed` is true, but leaves existing appearance intact on routine mentions.
2. **Portrait Worker Versioning Tests (`pkg/engine/portrait_worker_test.go`)**:
   - Test initial portrait generation creates `assets/portraits/<id>-v1.<ext>` and sets `portrait_version: 1`.
   - Test subsequent evolution creates `assets/portraits/<id>-v2.<ext>`, increments version, appends to `portrait_history`, and preserves `v1` on disk.
3. **Scene Worker Tests (`pkg/engine/scene_worker_test.go`)**:
   - Test scene prompt construction combining cue, location name, and world style.
   - Test generation writes `assets/scenes/turn-<N>.<ext>` and triggers ready callback.
4. **Service & HTTP Endpoint Tests (`pkg/gui/service_test.go`)**:
   - Test `GetCharacterPortrait` with `?v=1` returns version 1 asset; `?v=2` returns version 2 asset.
   - Test `GetTurnSceneImage` returns 200 with image bytes for a valid turn, and 404 for an ungenerated turn.
5. **Timeline History Tests (`pkg/engine/history_test.go`)**:
   - Test serialization and deserialization of `TurnSegment` containing `SpeakerPortrait`.

### 6.2 Frontend Verification
- Run `mise run test:frontend` (`npx tsc --noEmit`) to verify all TypeScript types (`TurnDTO`, `TurnSegment`, `TurnEvent`).
- Verify `TurnSegments` retains historical portrait URLs when new versions arrive in live state.
- Full suite verification: `mise run test` (Go tests + TypeScript check).
