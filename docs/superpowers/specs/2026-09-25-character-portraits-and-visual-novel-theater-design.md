# Design Spec: Character Portrait Generation, Backfill, and Visual Novel Theater

**Date:** 2026-09-25
**Status:** Approved
**Target:** `pkg/entity`, `pkg/engine`, `pkg/gui`, `pkg/media`, `frontend` (`TurnSegments`, `StoryTheater`, `types.ts`, `client.ts`)

---

## 1. Executive Summary

Characters currently have optional appearance descriptions and voice profiles, but lack visual identity across gameplay. Furthermore, characters invented dynamically by the Game Master or created with minimal notes often omit essential descriptive traits such as age, gender, and appearance.

This specification introduces:
1. **Automated Character Metadata Enrichment & Backfill**: Every character entity (protagonist and NPCs) is guaranteed at least baseline physical and demographic attributes (`name`, `appearance`, `gender`, `age`, `pronouns`). New characters receive these on creation; existing characters lacking them are backfilled automatically in the background using their accumulated history and narrative events.
2. **Standardized Portrait Generation**: When image generation is enabled, characters receive an AI-generated portrait. All portraits strictly adhere to a **3/4 bust facing right** framing with neutral backgrounds and world art-style inheritance. A deterministic procedural SVG bust serves as an immediate fallback when generation is disabled, offline, or pending.
3. **Dialogue & Chronicle Avatars**: Speech segments in the Chronicle and Turn drawers render a portrait avatar badge next to speaker names.
4. **Visual Novel Story Theater**: The Story Theater view transitions into a two-sided visual novel staging area with the player character anchored on the left (facing right) and active NPC speakers on the right (horizontally mirrored to face left), featuring dynamic speaker lighting and focus elevation.

---

## 2. Findings & Context

### 2.1 Entity Frontmatter Already Has Portrait & Appearance Fields
In `pkg/entity/entity.go`:
```go
type EntityFrontmatter struct {
    ID         string                 `yaml:"id"`
    Name       string                 `yaml:"name"`
    Type       string                 `yaml:"type"`
    Tags       []string               `yaml:"tags,omitempty"`
    Voice      *VoiceConfig           `yaml:"voice,omitempty"`
    Portrait   string                 `yaml:"portrait,omitempty"`
    Location   string                 `yaml:"location,omitempty"`
    Appearance string                 `yaml:"appearance,omitempty" json:"appearance,omitempty"`
    Aliases    []string               `yaml:"aliases,omitempty" json:"aliases,omitempty"`
    Faction    string                 `yaml:"faction,omitempty"`
    History    []int                  `yaml:"history,omitempty" json:"history,omitempty"`
    State      map[string]interface{} `yaml:"state,omitempty"`
    ExtraMeta  map[string]interface{} `yaml:",inline"`
}
```
`Portrait` and `Appearance` exist in the frontmatter struct and markdown serialization, but `Portrait` is currently unused by the UI and backend pipelines.

### 2.2 Turn Segments Carry Speaker Identity
`entity.TurnSegment` and `SegmentDTO` track `Speaker`, `SpeakerID`, and `Player: bool`. This information allows resolving the speaker's entity record and associated portrait asset without additional turn metadata.

### 2.3 Image Generation Infrastructure Exists
`pkg/media/image.go`, `pkg/provider/imagegemini`, `pkg/provider/imagehttp`, `pkg/provider/imagecli`, and `pkg/media/procedural_art.go` provide a uniform image client interface:
```go
type ImageClient interface {
    GenerateImage(ctx context.Context, prompt string) ([]byte, error)
}
```
The GUI service (`pkg/gui/image_generation.go`) already manages image spans, provider configuration, and byte validation.

---

## 3. Architecture & Data Flow

```
                      +-----------------------------+
                      | Character Lifecycle Events  |
                      | - Campaign Creation (Player)|
                      | - Turn Entity Discovery     |
                      | - Codex Manual Creation     |
                      | - Startup Backfill Scan     |
                      +--------------+--------------+
                                     |
                                     v
                      +-----------------------------+
                      | CharacterEnricher (LLM)     |
                      | If missing appearance/age/  |
                      | gender, generate via history|
                      +--------------+--------------+
                                     |
                                     v
                      +-----------------------------+
                      | PortraitWorker (Background) |
                      | If image generation enabled |
                      | generate 3/4 bust facing R  |
                      +--------------+--------------+
                                     |
                                     v
                      +-----------------------------+
                      | Storage & Serving           |
                      | games/<id>/assets/portraits/|
                      | GET /api/.../portrait       |
                      | (SVG fallback if disabled)  |
                      +--------------+--------------+
                                     |
                   +-----------------+-----------------+
                   |                                   |
                   v                                   v
      +-------------------------+         +-------------------------+
      | Chronicle Speech Badges |         | Story Theater (VN Stage)|
      | Small circular avatars  |         | Left: Player (facing R) |
      | alongside speaker names |         | Right: NPC (facing L)   |
      +-------------------------+         +-------------------------+
```

---

## 4. Detailed Design

### 4.1 Character Metadata Schema & Enrichment

#### Schema Standard
All character notes (`type: "character"`) must maintain the following core frontmatter fields:
* `name`: Display name.
* `appearance`: Physical description, clothing, noticeable features.
* `gender`: Demographic gender identity (e.g., female, male, non-binary, android).
* `age`: Approximate age or developmental stage (e.g., "28", "middle-aged", "ancient").
* `pronouns`: (Optional, e.g. "they/them", "she/her", "he/him").
* `portrait`: Relative path within campaign directory (e.g. `assets/portraits/<id>.png`).

#### Background Enrichment (`CharacterEnricher`)
When a character note is scanned or created with empty `appearance`, `gender`, or `age`:
1. **Context Formulation**:
   * Character ID and name.
   * Existing note body and tags.
   * Recent turn mentions via `store.GetEntityTurns(charID)`.
   * World genre and setting summary.
2. **LLM Structured Extraction**:
   * Uses the configured `extractor` (or `gm`) role with structured JSON schema:
     ```json
     {
       "appearance": "A scarred veteran pilot with weathered skin and cybernetic eyes...",
       "gender": "male",
       "age": "45",
       "pronouns": "he/him"
     }
     ```
3. **Safe Frontmatter Update**:
   * Populates empty frontmatter keys without overwriting user-authored prose or existing custom attributes.
   * Persists to `games/<game_id>/entities/<id>.md` and syncs with `storage.Store`.

---

### 4.2 Portrait Generation Pipeline

#### Portrait Prompt Template
To ensure visual consistency, modular staging, and seamless flipping:
```text
3/4 bust portrait, looking slightly to the right, head and upper torso centered, neutral plain studio backdrop, [world_art_style], [gender], [age] years old, [appearance], clean composition, high quality character portrait, no text, no borders
```
* **Angle Requirement**: Strictly `"3/4 bust portrait, looking slightly to the right"` so that the sprite can be flipped horizontally across dialogue turns without perspective distortion.
* **Art Style**: Sourced from the active campaign world (`manifest.ArtStyle`). Defaults to `"digital illustration, character concept art"` if unspecified.

#### Storage & Worker
* **Location**: `games/<game_id>/assets/portraits/<character_id>.<ext>`.
* **Worker**: `PortraitWorker` manages an in-memory deduplication set (`gameID:characterID`) to prevent redundant concurrent generations.
* **Non-blocking**: Generation runs asynchronously in the background. Gameplay, campaign loads, and turn streaming are never blocked.
* **Endpoint**:
  `GET /api/game/{game_id}/character/{character_id}/portrait`
  * If the image exists on disk: Serves bytes with appropriate MIME (`image/png`, `image/webp`).
  * If generating, missing, or image generation is disabled: Generates and serves a deterministic procedural SVG bust (seeded by character ID, name, and gender).

---

### 4.3 Chronicle & Turn Segment Presentation

#### Segment DTO Extension
In `pkg/gui/service.go`:
```go
type SegmentDTO struct {
    Kind        string  `json:"kind"`
    Speaker     string  `json:"speaker,omitempty"`
    SpeakerID   string  `json:"speaker_id,omitempty"`
    Text        string  `json:"text"`
    Player      bool    `json:"player,omitempty"`
    Duration    float64 `json:"duration,omitempty"`
    AudioURL    string  `json:"audio_url,omitempty"`
    PortraitURL string  `json:"portrait_url,omitempty"`
}
```
`segmentDTOs` populates `PortraitURL`:
* When `segment.SpeakerID` or resolved speaker name maps to a character entity, sets `PortraitURL = fmt.Sprintf("/api/game/%s/character/%s/portrait", gameID, speakerID)`.

#### Chronicle UI (`TurnSegments.tsx`)
* For speech segments (`kind === "speech"`):
  * Renders a 40×40px avatar badge next to the speaker's name.
  * Border accent reflects affiliation: sky-blue for the protagonist (`segment.player`), purple/amber for NPCs.
  * Clicking the avatar opens the character's note in the Codex drawer.
  * Native `img` tag with fallback to procedural SVG icon.

---

### 4.4 Visual Novel Story Theater (`StoryTheater.tsx`)

#### Stage Layout & Character Positioning
The Story Theater renders a two-sided visual novel dialogue stage:
* **Stage Left (Protagonist Anchor)**:
  * The player character's 3/4 bust is rendered on the left, unmirrored (facing right towards the conversation partner).
* **Stage Right (NPC / Interlocutor)**:
  * The current NPC speaker's 3/4 bust is rendered on the right, horizontally flipped using CSS (`scale-x-[-1]`) so they look left towards the protagonist.

#### Dynamic Focus & Lighting
* **Active Speaker**:
  * `opacity-100`, slight upward float / scale (`scale-105`), elevated drop shadow.
* **Listening Character**:
  * Subtly dimmed (`opacity-60 brightness-75`), slightly recessed (`scale-95`).
* **Narrative Segments**:
  * When narration plays, both character sprites are gently dimmed, directing visual focus to the narrative text box.

#### Visual Novel Dialogue Box
* Positioned across the bottom of the screen with translucent dark glass styling (`bg-stone-950/85 backdrop-blur-2xl border border-white/10`).
* Speaker name badge with distinct role/player colors.
* Smooth text pacing synchronized with speech audio duration.

---

## 5. File Map

| File | Change | Description |
|------|--------|-------------|
| `pkg/entity/entity.go` | Modify | Ensure `Portrait`, `Appearance`, `Gender`, `Age` frontmatter serialization and parsing. |
| `pkg/engine/character_enricher.go` | Create | LLM-based metadata enrichment for missing character fields. |
| `pkg/engine/portrait_worker.go` | Create | Background worker managing queued portrait generation and deduplication. |
| `pkg/gui/service.go` | Modify | Add character portrait endpoint and populate `PortraitURL` in `SegmentDTO`. |
| `pkg/gui/server.go` | Modify | Register `/api/game/{id}/character/{character_id}/portrait` route. |
| `frontend/src/types.ts` | Modify | Add `portrait_url` to `TurnSegment` and `PlayerState` / `EntitySummary`. |
| `frontend/src/components/TurnSegments.tsx` | Modify | Render speaker avatar badges next to speech segment headers. |
| `frontend/src/components/StoryTheater.tsx` | Modify | Implement two-sided visual novel character stage with dynamic focus and flipping. |
| `pkg/gui/service_test.go` | Modify | Unit tests for portrait endpoint and segment DTO resolution. |
| `pkg/engine/character_enricher_test.go` | Create | Unit tests for character backfilling and prompt compilation. |

---

## 6. Verification & Testing

1. **Unit Testing**:
   - `TestCharacterEnricher_BackfillsMissingFields`: Verify empty fields are populated while preserving existing note content.
   - `TestPortraitWorker_Generates34BustPrompt`: Verify prompt contains `"3/4 bust portrait, looking slightly to the right"`, world art style, and character attributes.
   - `TestPortraitEndpoint_FallbackProceduralSVG`: Verify request returns valid SVG when image provider is disabled.
2. **Integration & UI Testing**:
   - Build backend and frontend (`go test ./...`, `npm run build`).
   - Launch a campaign, trigger turns with speech segments, and verify avatar icons appear in Chronicle.
   - Open Story Theater and verify two-sided stage layout, CSS horizontal mirroring on stage right, and speaker highlighting.
