# Character Creation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make character creation part of starting a campaign: per-system prompts, fillable by the player or generated, persisted into a fully described and voiced player note.

**Architecture:** Add a `character_creation` schema to `system.yaml`, render it as a wizard step, add a side-effect-free generation endpoint with an oracle fallback, and extend campaign creation to write appearance, background, voice, and system-specific fields into the player note.

**Tech Stack:** Go 1.27.1, `harness.Router`, React 19, TypeScript, Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-09-22-character-creation-design.md`

---

## File Structure Map

| File Path | Responsibility |
| :--- | :--- |
| `pkg/core/types.go` | `CharacterCreationSpec`/`Field` and manifest field |
| `pkg/engine/character.go` | `PlayerCharacter`, default field set |
| `pkg/engine/game.go` | `InitOptions.PlayerCharacter`; populated player note |
| `pkg/gui/types.go` | Player character DTO; system and player DTO additions |
| `pkg/gui/character_generate.go` | Generation logic and fallback |
| `pkg/gui/server.go` | `/api/character/generate` route |
| `pkg/gui/service.go` | `CreateGame` wiring and validation; system schema |
| `frontend/src/types.ts`, `api/client.ts` | Types and `generateCharacter` |
| `frontend/src/components/LauncherHub.tsx` | Multi-step wizard |
| `frontend/src/components/SystemsStudio.tsx` | Field editor |
| `frontend/src/components/CharacterSheetDrawer.tsx` | Appearance and voice display |
| `systems/narrative_2d6/system.yaml` | Example spec |

---

### Task 1: Add the character-creation schema to the system manifest

**Files:**
- Modify: `pkg/core/types.go`
- Modify: `pkg/core/types_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestSystemManifestCharacterCreationRoundTrip(t *testing.T) {
	in := &SystemManifest{
		ID: "test", Name: "Test", Version: "1.0.0",
		CharacterCreation: CharacterCreationSpec{
			Preamble: "Create a hero.",
			Fields: []CharacterCreationField{
				{ID: "appearance", Label: "Appearance", Kind: "long", Required: true, Generatable: true},
				{ID: "voice", Label: "Voice", Kind: "voice"},
			},
		},
	}
	data, err := yaml.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out SystemManifest
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.CharacterCreation.Fields) != 2 || out.CharacterCreation.Fields[0].ID != "appearance" {
		t.Fatalf("round trip lost fields: %+v", out.CharacterCreation)
	}
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test -run TestSystemManifestCharacterCreationRoundTrip ./pkg/core/`
Expected: FAIL.

- [ ] **Step 3: Implement**

Add the types from the spec and the field:

```go
type SystemManifest struct {
	ID                string               `yaml:"id"`
	Name              string               `yaml:"name"`
	Version           string               `yaml:"version"`
	Description       string               `yaml:"description,omitempty"`
	CharacterCreation CharacterCreationSpec `yaml:"character_creation,omitempty"`
}
```

- [ ] **Step 4: Run the test**

Run: `go test -run TestSystemManifestCharacterCreationRoundTrip ./pkg/core/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/core/types.go pkg/core/types_test.go
git commit -m "feat(core): let a rules system define character creation prompts"
```

---

### Task 2: Persist a described, voiced player note

**Files:**
- Create: `pkg/engine/character.go`
- Modify: `pkg/engine/game.go`
- Modify: `pkg/engine/game_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestInitGameWritesPlayerCharacter(t *testing.T) {
	// InitGame with PlayerCharacter{Appearance:"Tall", Background:"Exile", Age:"34", Voice:&entity.VoiceConfig{VoiceID:"bf_emma"}}
	// read entities/<player>.md and assert appearance/age/voice are present
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test -run TestInitGameWritesPlayerCharacter ./pkg/engine/`
Expected: FAIL.

- [ ] **Step 3: Implement `pkg/engine/character.go`**

```go
package engine

import "github.com/darkliquid/localrpg/pkg/entity"

type PlayerCharacter struct {
	Appearance string
	Age        string
	Gender     string
	Pronouns   string
	Background string
	Voice      *entity.VoiceConfig
	Extra      map[string]string
}

// DefaultCharacterFields is the fallback creation spec for a system that defines
// none.
func DefaultCharacterFields() []string { return []string{"appearance", "age", "gender", "pronouns", "background", "voice"} }
```

- [ ] **Step 4: Populate the note**

Add `PlayerCharacter PlayerCharacter` to `InitOptions`, change `ensurePlayerNote` to accept it, and set:

```go
player := &entity.Entity{
	ID:         id,
	Name:       playerName,
	Type:       "character",
	Appearance: pc.Appearance,
	Body:       strings.TrimSpace(pc.Background),
	Voice:      pc.Voice,
	ExtraMeta:  map[string]interface{}{},
}
for key, value := range pc.Extra {
	player.ExtraMeta[key] = value
}
if pc.Age != "" {
	player.ExtraMeta["age"] = pc.Age
}
if pc.Gender != "" {
	player.ExtraMeta["gender"] = pc.Gender
}
if pc.Pronouns != "" {
	player.ExtraMeta["pronouns"] = pc.Pronouns
}
if player.Body == "" {
	player.Body = "The player character."
}
```

Do not auto-assign a voice here; the caller (Task 5) does that with the configured profiles.

- [ ] **Step 5: Run the engine tests**

Run: `go test -count=1 ./pkg/engine/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/engine/character.go pkg/engine/game.go pkg/engine/game_test.go
git commit -m "feat(engine): write a described player character at campaign start"
```

---

### Task 3: Expose the schema and accept the character in `CreateGame`

**Files:**
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/service_test.go`

- [ ] **Step 1: Add the DTOs**

```go
type PlayerCharacterDTO struct {
	Appearance string                `json:"appearance,omitempty"`
	Age        string                `json:"age,omitempty"`
	Gender     string                `json:"gender,omitempty"`
	Pronouns   string                `json:"pronouns,omitempty"`
	Background string                `json:"background,omitempty"`
	Voice      *config.VoiceProfile  `json:"voice,omitempty"`
	Extra      map[string]string     `json:"extra,omitempty"`
}
```

Add `Player PlayerCharacterDTO `json:"player,omitempty"`` to `CreateGameRequestDTO`; add `CharacterCreation core.CharacterCreationSpec` to `SystemDetailDTO` and `CreateSystemRequestDTO`; add `Appearance string` and `Voice *config.VoiceProfile` to `PlayerDTO`.

- [ ] **Step 2: Write the failing test**

```go
func TestCreateGamePersistsPlayerCharacter(t *testing.T) {
	// CreateGame with Player{Appearance:"...", Voice:&config.VoiceProfile{ID:"x", VoiceID:"bf_emma"}}
	// read the player note and assert appearance and voice
	// assert required fields missing returns an error
}
```

- [ ] **Step 3: Implement**

In `CreateGame`:
- convert `PlayerCharacterDTO.Voice` to `*entity.VoiceConfig`;
- assemble `engine.PlayerCharacter`;
- validate required fields from the system spec (`GetSystem(...).CharacterCreation`), falling back to `engine.DefaultCharacterFields()`;
- pass it through `engine.InitOptions`.

In `GetSystem`/`SaveSystem`, carry `CharacterCreation` through.

- [ ] **Step 4: Run tests**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/service_test.go
git commit -m "feat(gui): accept a player character when creating a campaign"
```

---

### Task 4: Add the generation endpoint

**Files:**
- Create: `pkg/gui/character_generate.go`
- Modify: `pkg/gui/server.go`
- Create: `pkg/gui/character_generate_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestGenerateCharacterValues(t *testing.T) {
	// service with a mock router returning a JSON object for the requested fields
	// POST /api/character/generate, assert 200 and values keyed by field id
}

func TestGenerateCharacterFallsBackWhenDisabled(t *testing.T) {
	// no model configured: assert 200 with generated_by "none" and no error
}
```

- [ ] **Step 2: Implement `character_generate.go`**

```go
type GenerateCharacterRequest struct {
	SystemID string   `json:"system_id"`
	WorldID  string   `json:"world_id"`
	Name     string   `json:"name"`
	Fields   []core.CharacterCreationField `json:"fields"`
	Seed     map[string]string `json:"seed,omitempty"`
}

type GenerateCharacterResponse struct {
	Values      map[string]string `json:"values"`
	GeneratedBy string            `json:"generated_by"`
}
```

Build a JSON-only system prompt from the fields, resolve the router role `character` falling back to `gm` (`harness.RouterFromConfig`), call `GenerateForRole`, and parse the returned object leniently: keep only known field ids, coerce numbers to strings, and never fail the request on a model error - return `generated_by: "none"` instead. `kind: voice` fields are excluded from generation.

- [ ] **Step 3: Route it**

In `pkg/gui/server.go`, map `POST /api/character/generate` (a top-level `/api/character/...` route, outside `/api/game/{id}`). Use `http.MaxBytesReader` as the other write routes do.

- [ ] **Step 4: Run tests**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/character_generate.go pkg/gui/character_generate_test.go pkg/gui/server.go
git commit -m "feat(gui): generate starting character values with a safe fallback"
```

---

### Task 5: Auto-assign the player's voice

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/service_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestCreateGameAutoAssignsPlayerVoice(t *testing.T) {
	// config with KokoroVoiceProfiles and a description of "a gruff old man"
	// create the campaign, read the player note, assert Voice != nil
}
```

- [ ] **Step 2: Implement**

In `CreateGame`, when `req.Player.Voice == nil`:

1. create the campaign via `InitGame`;
2. open the game store, load the player note, run `harness.AssignVoiceProfile(player, cfg.Media.TTS.VoiceProfiles)`, and write it back through `Timeline.SaveEntity`.

Reuse the existing `harness.AssignVoiceProfile` rather than duplicating matching logic.

- [ ] **Step 3: Run tests**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/gui/service.go pkg/gui/service_test.go
git commit -m "feat(gui): give the player a voice chosen from their description"
```

---

### Task 6: Frontend wizard and editors

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/api/client.ts`
- Modify: `frontend/src/components/LauncherHub.tsx`
- Modify: `frontend/src/components/SystemsStudio.tsx`
- Modify: `frontend/src/components/CharacterSheetDrawer.tsx`

- [ ] **Step 1: Types and client**

Add `CharacterCreationField`, `CharacterCreationSpec`, `PlayerCharacter`, extend `SystemDetail`, `CreateGameRequest`, and `Player`; add:

```ts
static async generateCharacter(payload: GenerateCharacterRequest): Promise<GenerateCharacterResponse> {
  const res = await fetch('/api/character/generate', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
  if (!res.ok) throw new HTTPError(res.status, `generateCharacter: ${res.statusText}`);
  return res.json();
}
```

- [ ] **Step 2: Wizard**

Restructure the create wizard into campaign → character → review, loading the system spec when the system changes, rendering each field by kind (`voice` renders the configured `voice_profiles` picker), wiring per-field and generate-all buttons, and sending `player` on create.

- [ ] **Step 3: Studio editor**

Add a character-creation section to `SystemsStudio`: list fields with add/remove, editing id/label/prompt/kind/required/generatable/options.

- [ ] **Step 4: Sheet**

Show `player.appearance` and `player.voice` in `CharacterSheetDrawer`.

- [ ] **Step 5: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts frontend/src/components/LauncherHub.tsx frontend/src/components/SystemsStudio.tsx frontend/src/components/CharacterSheetDrawer.tsx
git commit -m "feat(frontend): add a character creation step to the campaign wizard"
```

---

### Task 7: Ship an example and verify

**Files:**
- Modify: `systems/narrative_2d6/system.yaml`

- [ ] **Step 1: Add an example spec**

Add a `character_creation` block with `appearance`, `age`, `gender`, `pronouns`, `background`, and a `voice` field so the feature is discoverable.

- [ ] **Step 2: Full verification**

Run: `mise run test:backend`, `mise run lint`, `cd frontend && npx tsc --noEmit`
Expected: all pass.

- [ ] **Step 3: Manual smoke**

Create a campaign with a generated character, confirm the player note has appearance, background, voice, and the extra fields, and confirm they appear in the codex.
