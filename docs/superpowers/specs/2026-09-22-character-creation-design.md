# Design Spec: Character Creation During Campaign Start

**Date:** 2026-09-22
**Status:** Draft
**Target:** `pkg/core`, `pkg/engine`, `pkg/gui`, `pkg/harness`, `frontend` (`LauncherHub`, `SystemsStudio`)

---

## 1. Executive Summary

Starting a campaign currently asks for a name, a rules system, a world, and a player name, then writes an almost empty player note. Character creation should be a first-class step: a set of prompts - appearance, voice, age, gender, pronouns, background, and any additional prompts the rules system defines - each of which the player may fill in or have generated as a starting value.

This spec adds a per-system character-creation schema, a wizard step that renders it, an optional generation endpoint, and persistence of the answers (including the player's voice) into the player note. It also provides the player voice that the player-speech playback spec depends on.

---

## 2. Findings

### 2.1 The create request carries almost nothing

`CreateGameRequestDTO` (`pkg/gui/types.go:132`) has `name`, `system_id`, `world_id`, `player_name`, `opening_prompt`. `engine.InitOptions` already has a `PlayerDetails string` (`pkg/engine/game.go:38`) but the GUI never sets it, and it is a single unstructured blob.

### 2.2 Player notes are near-empty and unvoiced

`ensurePlayerNote` (`pkg/engine/game.go:185`) writes:

```go
player := &entity.Entity{ ID: id, Name: playerName, Type: "character", Body: body }
```

so the player has no `appearance`, no `voice`, and no system-specific fields. Because it is not created through extraction, `AssignVoiceProfile` never runs for it, so the player is unvoiced - which blocks the player-speech playback feature.

### 2.3 Systems define no character-creation prompts

`core.SystemManifest` (`pkg/core/types.go:11`) holds only `id`, `name`, `version`, `description`. A system is a `system.yaml`, a `mechanics.js`, and a `prompts/rules.md`; `SystemDetailDTO` exposes only the script and the rules prompt. There is no place for a rules system to say "this game needs a Background and a Vice".

### 2.4 The entity model already supports the answers

`entity.Entity` has `Appearance`, `Voice *VoiceConfig`, `Aliases`, and `ExtraMeta map[string]interface{}` (inline YAML frontmatter). `SerializeMarkdown` already round-trips all of them. No memory-model change is needed; the player note simply needs to be populated.

### 2.5 A model is available for generation

`harness.Router` maps role to provider and already powers the gm and extractor roles. The `narrative-oracle` built-in lets generation work with no model at all. Generation must therefore degrade gracefully and never block campaign creation.

---

## 3. Design

### 3.1 A system-level character-creation schema

Extend `core.SystemManifest`:

```go
type CharacterCreationField struct {
	ID          string   `yaml:"id" json:"id"`
	Label       string   `yaml:"label" json:"label"`
	Prompt      string   `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Kind        string   `yaml:"kind,omitempty" json:"kind,omitempty"` // text | long | number | select | voice
	Required    bool     `yaml:"required,omitempty" json:"required,omitempty"`
	Generatable bool     `yaml:"generatable,omitempty" json:"generatable,omitempty"`
	Options     []string `yaml:"options,omitempty" json:"options,omitempty"`
	Default     string   `yaml:"default,omitempty" json:"default,omitempty"`
}

type CharacterCreationSpec struct {
	Preamble string                   `yaml:"preamble,omitempty" json:"preamble,omitempty"`
	Fields   []CharacterCreationField `yaml:"fields,omitempty" json:"fields,omitempty"`
}
```

Add `CharacterCreation CharacterCreationSpec` to `SystemManifest` with yaml key `character_creation`.

When a system defines no fields, the engine falls back to a built-in default set: `appearance` (long, required, generatable), `age` (text), `gender` (text), `pronouns` (text), `background` (long, generatable), and `voice` (kind `voice`). `name` is already collected by the wizard.

`kind: voice` is special: it is never a free-text model output. It renders as a picker over `config.Media.TTS.VoiceProfiles`, and when left blank the voice is chosen automatically from the description (Section 3.4).

### 3.2 Persisting the schema

- `SystemDetailDTO` gains `CharacterCreation CharacterCreationSpec`.
- `CreateSystemRequestDTO` gains `CharacterCreation CharacterCreationSpec`.
- `SaveSystem`/`GetSystem` marshal and return it.
- `SystemsStudio` gains an editor for fields (add/remove/reorder, id/label/prompt/kind/required/generatable/options).
- The shipped `systems/narrative_2d6/system.yaml` gains a small example so the feature is discoverable.

### 3.3 The wizard step

`LauncherHub`'s create wizard becomes three steps:

1. Campaign: name, system, world, opening prompt.
2. Character: name plus the system's fields, each with an input and a **Generate** button, plus a **Generate all** button. Generated values are editable; nothing is forced.
3. Review: a read-only summary, then **Begin Campaign**.

The wizard loads `GET /api/systems/{id}` when the system changes to render the fields. A missing system detail falls back to the built-in default fields.

### 3.4 Voice selection

- If the player chose a voice profile, use it verbatim.
- Otherwise auto-assign: create the player note, then call `harness.AssignVoiceProfile(player, cfg.Media.TTS.VoiceProfiles)` using a corpus of name, appearance, background, and the other answers, so a described voice determines the profile.
- If no profiles are configured, the player stays unvoiced and playback falls back to the narrator voice; this is not an error.

### 3.5 Generating starting values

New endpoint:

```
POST /api/character/generate
{
  "system_id": "...", "world_id": "...", "name": "...",
  "fields": [ { "id": "appearance", "label": "Appearance", "prompt": "...", "kind": "long" } ],
  "seed": { "gender": "female" }
}
→ { "values": { "appearance": "..." }, "generated_by": "gm|oracle|none" }
```

- Uses `harness.Router` role `character` if configured, else `gm`, via `RouterFromConfig`.
- The prompt asks for a JSON object keyed by field id and nothing else; the response is parsed leniently and unknown keys are dropped.
- With no usable model, the `narrative-oracle` built-in produces deterministic values; with TTS/LLM disabled entirely, the endpoint returns `generated_by: "none"` and an empty map so the UI can say generation is unavailable rather than fail.
- The endpoint is side-effect free: it creates no campaign and writes no files.

### 3.6 Creating the campaign with the character

`CreateGameRequestDTO` gains:

```go
type PlayerCharacterDTO struct {
	Appearance string            `json:"appearance,omitempty"`
	Age        string            `json:"age,omitempty"`
	Gender     string            `json:"gender,omitempty"`
	Pronouns   string            `json:"pronouns,omitempty"`
	Background string            `json:"background,omitempty"`
	Voice      *config.VoiceProfile `json:"voice,omitempty"`
	Extra      map[string]string `json:"extra,omitempty"`
}
```

`engine.InitOptions` gains `PlayerCharacter PlayerCharacter` (an `engine`-local struct mirroring the fields, with `Voice *entity.VoiceConfig`). `ensurePlayerNote` writes:

- `Appearance` and `Body` (background),
- `Voice`,
- system-specific answers into `ExtraMeta` under their field ids.

Validation: required fields (from the system spec, or the defaults) must be non-empty before `POST /api/games`. Voice, when supplied, must match a configured profile id.

### 3.7 Surfacing the character

`PlayerDTO` gains `Appearance string` and `Voice *config.VoiceProfile` so `CharacterSheetDrawer` can show how the character looks and sounds. No gameplay state is inferred from these fields; they are the player's own description.

---

## 4. Data Flow

```text
LauncherHub step 1: campaign + system/world
  └─ GET /api/systems/{id} ──► CharacterCreationSpec (or built-in defaults)
LauncherHub step 2: fields, optional per-field / generate-all
  └─ POST /api/character/generate ──► { values } ──► editable inputs
LauncherHub step 3: review
  └─ POST /api/games { ..., player: {...} }
       └─ engine.InitGame
            ├─ ensurePlayerNote(appearance, background, voice, extra)
            ├─ AssignVoiceProfile when no explicit voice
            └─ manifest written last
```

---

## 5. File Map

| Action | Path | Description |
| :--- | :--- | :--- |
| Modify | `pkg/core/types.go` | `CharacterCreationSpec`/`Field` on `SystemManifest` |
| Create | `pkg/engine/character.go` | Player character struct; default field set |
| Modify | `pkg/engine/game.go` | `InitOptions.PlayerCharacter`; `ensurePlayerNote` writes fields and voice |
| Modify | `pkg/gui/types.go` | `PlayerCharacterDTO`; system DTOs; `PlayerDTO` additions |
| Modify | `pkg/gui/service.go` | `CreateGame` validation and wiring; `GetSystem`/`SaveSystem` schema |
| Create | `pkg/gui/character_generate.go` | Generation endpoint logic with oracle fallback |
| Modify | `pkg/gui/server.go` | `POST /api/character/generate` route |
| Modify | `frontend/src/types.ts` | Character-creation types; create payload; player fields |
| Modify | `frontend/src/api/client.ts` | `generateCharacter` method |
| Modify | `frontend/src/components/LauncherHub.tsx` | Multi-step wizard and review |
| Modify | `frontend/src/components/SystemsStudio.tsx` | Character-creation field editor |
| Modify | `frontend/src/components/CharacterSheetDrawer.tsx` | Show appearance and voice |
| Modify | `systems/narrative_2d6/system.yaml` | Example character-creation spec |

---

## 6. Acceptance Criteria

1. A system can define character-creation fields, and they round-trip through save/get and the studio editor.
2. A system with no spec falls back to the built-in default fields.
3. The wizard collects the fields, and Generate fills values the player can still edit.
4. Creating a campaign writes appearance, background, voice, and system-specific fields into the player note, indexed and visible in the codex.
5. The player note has a voice when profiles are configured, chosen explicitly or auto-assigned.
6. Generation works with an LLM, with the built-in oracle, and reports unavailability when nothing is configured; it never blocks or fails creation.
7. `go test -count=1 ./...`, `go vet ./...`, and `npx tsc --noEmit` pass.
