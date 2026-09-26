# Pure-Go shirei GUI — Studios and Character Creation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the Systems Studio (manifest, rules.md, mechanics.js), the Worlds Studio (lore, lore.md, starter entities, artwork), and system-driven character creation in the new-campaign flow, with AI field generation.

**Architecture:** A shared master/detail studio shell holds a local draft (create vs update) and a saved selection; drafts are frontend-only concepts today, so they stay in `desktop.State`. Systems and worlds are pure filesystem CRUD through the service; only field generation, asset generation, and asset previews reach providers, behind injected callbacks. Shirei has no code editor, so `mechanics.js`/`rules.md` edit in a monospace `TextArea` and can be previewed read-only with `LargeText`.

**Tech Stack:** Go 1.27.1, `go.hasen.dev/shirei` widgets, `pkg/core` (`SystemManifest`, `CharacterCreationSpec`), `pkg/gui` service + DTOs, standard-library tests.

**Spec:** `docs/superpowers/specs/2026-09-26-pure-go-shirei-gui-design.md` (Phase 6)

## Global Constraints

- Desktop only. Do not modify `pkg/gui/server.go`, `socket.go`, `assets.go`, or `middleware.go`.
- No code editor exists in shirei: `mechanics.js` and `rules.md` edit in a monospace `TextArea`; there is no syntax highlighting or gutter.
- The service has **no** delete for systems or worlds; do not add one. Delete only world entities (`DeleteWorldEntity`).
- Go style: `any`; `go vet ./...` clean. Tests standard-library only.
- Every interactive container gets `NextAccessName(...)` + `AssignAccess()`.
- Commits: Conventional Commits with a scope; end with the attribution block shown in Task 1 Step 4.
- `mise run test` must pass (`pkg/gui` is flaky ~1 in 6; re-run before calling it a regression).

---

### Task 1: Shared studio shell and draft model

**Files:**
- Modify: `pkg/desktop/state.go` (studio fields)
- Create: `pkg/desktop/studio.go`
- Modify: `pkg/desktop/root.go` (add `ScreenSystemsStudio`, `ScreenWorldsStudio`)
- Test: `pkg/desktop/studio_test.go`

**Interfaces:**
- Produces:
  - `type desktop.Selection struct { Kind string; ID string }` where `Kind` is `"saved"`, `"draft"`, or `""`
  - `State.Studio Selection`, `State.StudioDirty bool`
  - `func applySelection(next Selection, dirty func() bool)` — sets or discards
  - `func studioHeader(title string, onNew, onBack func())` — a shared top bar
  - `func confirmDiscardModal()`

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/studio_test.go`:

```go
package desktop

import "testing"

func TestApplySelectionKeepsDirtyDraft(t *testing.T) {
	appState = &State{Studio: Selection{Kind: "draft"}, StudioDirty: true}
	applySelection(Selection{Kind: "saved", ID: "existing"}, func() bool { return true })
	// A dirty draft must not be silently replaced.
	if appState.Studio.Kind != "draft" || !appState.ConfirmDiscard {
		t.Fatalf("studio = %+v confirm = %v", appState.Studio, appState.ConfirmDiscard)
	}
}

func TestApplySelectionSwitchesWhenClean(t *testing.T) {
	appState = &State{Studio: Selection{Kind: "draft"}, StudioDirty: false}
	applySelection(Selection{Kind: "saved", ID: "existing"}, func() bool { return false })
	if appState.Studio.ID != "existing" {
		t.Fatalf("studio = %+v", appState.Studio)
	}
}
```

Add `State.ConfirmDiscard bool` to the state fields.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestApplySelection -v`
Expected: FAIL — `Selection`/`applySelection` undefined.

- [ ] **Step 3: Implement the shell**

Add the fields and implement `applySelection` (replace unless the current selection is a dirty draft, in which case set `ConfirmDiscard` and stash the pending selection in `State.PendingStudio`). `confirmDiscardModal()` renders a `Modal` when `ConfirmDiscard` is set, offering Discard (apply pending) or Cancel (clear).

`studioHeader` renders a title, a New button, and a Back button that returns to the launcher.

- [ ] **Step 4: Run tests and commit**

Run: `go test ./pkg/desktop/ -v`
Expected: PASS.

```bash
git add pkg/desktop
git commit -m "$(cat <<'EOF'
feat(gui): add the studio shell and draft handling

Studios share a master/detail shell with a local draft that must be
discarded explicitly, matching how the SPA guards unsaved work.

💘 Generated with Crush

Assisted-by: Crush:deepseek-v4.1-flash
EOF
)"
```

---

### Task 2: Systems Studio

**Files:**
- Create: `pkg/desktop/systems_studio.go`
- Modify: `pkg/desktop/state.go` (system form fields)
- Modify: `pkg/desktop/data.go`
- Test: `pkg/desktop/systems_studio_test.go`

**Interfaces:**
- Consumes: `ListSystems(ctx) ([]gui.SystemSummaryDTO, error)` (`pkg/gui/service.go:2024`), `GetSystem(ctx, id) (*gui.SystemDetailDTO, error)` (`:2414`), `SaveSystem(ctx, req gui.CreateSystemRequestDTO) (*gui.SystemDetailDTO, error)` (`:2442`), `SystemDetailDTO`/`CreateSystemRequestDTO` (`types.go:243/253`), `core.CharacterCreationSpec`/`CharacterCreationField` (`pkg/core/types.go:24-40`).
- Produces:
  - `State.Systems []gui.SystemSummaryDTO`, `State.System *gui.SystemDetailDTO`
  - `State.SystemName`, `SystemVersion`, `SystemSlug`, `SystemDescription`, `SystemRules`, `SystemScript`, `SystemPreamble`, `SystemFields []core.CharacterCreationField`
  - `func loadSystems(ctx, svc)`, `func loadSystemDetail(ctx, svc, id)`, `func systemPayload() gui.CreateSystemRequestDTO`
  - injected `saveSystem func(ctx, svc, req gui.CreateSystemRequestDTO) error`, `generateText func(ctx, svc, req gui.GenerateTextRequest) (*gui.GenerateTextResponse, error)`

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/systems_studio_test.go`:

```go
package desktop

import "testing"

func TestSystemPayloadDefaultsSlugFromName(t *testing.T) {
	appState = &State{SystemName: "Blades in the Dark", SystemSlug: "", SystemVersion: "1.0.0"}
	req := systemPayload()
	if req.Name != "Blades in the Dark" {
		t.Fatalf("name = %q", req.Name)
	}
	if req.ID != "" {
		t.Fatalf("create must omit an explicit ID, got %q", req.ID)
	}
}

func TestSystemPayloadKeepsSavedSlug(t *testing.T) {
	appState = &State{SystemName: "Blades in the Dark", SystemSlug: "blades", SystemSaved: true}
	req := systemPayload()
	if req.ID != "blades" {
		t.Fatalf("id = %q, want blades", req.ID)
	}
}
```

Add `State.SystemSaved bool`.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestSystemPayload -v`
Expected: FAIL.

- [ ] **Step 3: Implement the studio**

`systemsStudioView()`:
- Left: the saved list (`VirtualListView`) plus a "New System" tile that resets the form to the reference defaults.
- Tabs (`TabStrip`/`TabItem`): Manifest, rules.md, mechanics.js.
- **Manifest:** `TextInput`s for name/version/slug (slug disabled when saved), a `TextArea` for description, a `TextInput` for the creation preamble, and a repeatable `CharacterCreationField` editor (id, label, prompt, `OptionGroup` for kind `text|long|number|select|voice`, `CheckBox` for required, `CheckBox` for generatable — disabled when kind is `voice` — plus Add/Remove).
- **rules.md / mechanics.js:** monospace `TextArea`s (use `TextInputExt` with a monospace `TextInputAttrs` style), plus a read-only `LargeText` preview toggle.
- Per-field and "generate all" buttons call the injected `generateText` with `gui.GenerateTextRequest{FormType: "system", FieldName: ...}` and apply `res.Fields`; surface `res.Warning` via `formatGenerationError`-equivalent text.
- Save calls injected `saveSystem(systemPayload())`; on success reload the list and set `SystemSaved`.

`systemPayload()` builds `gui.CreateSystemRequestDTO{ID: slug if saved else "", Name, Version, Description, RulesPrompt, Script, CharacterCreation: core.CharacterCreationSpec{Preamble, Fields}}`. If the slug is empty on a draft, leave `ID` empty so `SaveSystem` slugifies the name.

- [ ] **Step 4: Run tests and commit**

```bash
go test ./pkg/desktop/ -v
git add pkg/desktop
git commit -m "feat(gui): add the systems studio"
```

---

### Task 3: Worlds Studio

**Files:**
- Create: `pkg/desktop/worlds_studio.go`
- Modify: `pkg/desktop/state.go` (world form + entity fields)
- Modify: `pkg/desktop/data.go`
- Test: `pkg/desktop/worlds_studio_test.go`

**Interfaces:**
- Consumes: `ListWorlds(ctx) ([]gui.WorldSummaryDTO, error)` (`service.go:2054`), `GetWorld(ctx, id) (*gui.WorldDetailDTO, error)` (`:2495`), `CreateWorld(ctx, req gui.CreateWorldRequestDTO) (*gui.WorldDetailDTO, error)` (`:2608`), `UpdateWorld(ctx, req gui.CreateWorldRequestDTO) (*gui.WorldDetailDTO, error)` (`:2624`), `GetWorldEntity(ctx, worldID, entityID) (*gui.WorldEntityDetailDTO, error)` (`:2631`), `SaveWorldEntity(ctx, worldID, entityID, markdown string) error` (`:2643`), `DeleteWorldEntity(ctx, worldID, entityID) error` (`:2652`), `SaveWorldAsset(worldID, kind string, data []byte, ext string) (string, error)` (`:2980`), `GenerateWorldAsset(ctx, worldID, req gui.GenerateAssetRequestDTO) (string, error)` (`:3086`), `GenerateAssetPreview(ctx, req gui.GenerateAssetPreviewRequestDTO) ([]byte, string, error)` (`:3051`), `gui.ErrWorldExists` (confirm at `service.go:2552`).
- Produces: world form fields, `State.WorldEntities []gui.WorldEntitySummaryDTO`, `State.WorldEntityMarkdown string`, `func worldPayload() gui.CreateWorldRequestDTO`, injected `saveWorld`, `saveWorldEntity`, `deleteWorldEntity`, `generateWorldAsset`, `saveWorldAsset`.

- [ ] **Step 1: Write the failing test**

```go
package desktop

import "testing"

func TestWorldPayloadSplitsTags(t *testing.T) {
	appState = &State{WorldName: "The Sundered Realm", WorldTags: "fantasy, dark , ,low-magic"}
	req := worldPayload()
	if len(req.Tags) != 3 {
		t.Fatalf("tags = %#v", req.Tags)
	}
	if req.Tags[1] != "dark" {
		t.Fatalf("tags trimmed wrong: %#v", req.Tags)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestWorldPayloadSplitsTags -v`
Expected: FAIL.

- [ ] **Step 3: Implement the studio**

`worldsStudioView()` with tabs Lore, lore.md, Starter Entities:
- **Lore:** name, genre, slug (disabled when saved), a `MenuButton` default-system picker over `appState.Systems`, art-style prompt, comma-separated tags, a `TextArea` description, and banner/icon sections. Artwork: a `Modal` + `FileSelector` to pick a file, then the injected `saveWorldAsset`; an "AI Gen" button calling the injected `generateWorldAsset` when saved, or `GenerateAssetPreview` into a temp file when drafting (mirroring the SPA).
- **lore.md:** monospace `TextArea`.
- **Starter Entities:** list with add/delete, and a markdown `TextArea` per entity saving through injected `saveWorldEntity`/`deleteWorldEntity`. New entities start from the SPA's frontmatter template (port `STARTER_ENTITY_TEMPLATE`).
- `worldPayload()` splits tags on commas, trims, and drops empties.

- [ ] **Step 4: Run tests and commit**

```bash
go test ./pkg/desktop/ -v
git add pkg/desktop
git commit -m "feat(gui): add the worlds studio with entities and artwork"
```

---

### Task 4: System-driven character creation

**Files:**
- Modify: `pkg/desktop/newcampaign.go`
- Modify: `pkg/desktop/state.go` (`CharacterAnswers map[string]string`)
- Test: `pkg/desktop/newcampaign_test.go` (extend)

**Interfaces:**
- Consumes: `GenerateText(ctx, req gui.GenerateTextRequest) (*gui.GenerateTextResponse, error)` (`pkg/gui/text_generate.go:122`), `GenerateCharacter(ctx, req gui.GenerateCharacterRequest) (*gui.GenerateCharacterResponse, error)` (`pkg/gui/character_generate.go:62`), `core.CharacterCreationField`/`Spec`, `engine.CharacterFields` (server-side; the client uses the system detail's spec).
- Produces: `State.CharacterFields []core.CharacterCreationField`, `State.CharacterAnswers map[string]string`, `func characterForm()`, `func characterSubmitFields() map[string]string`.

- [ ] **Step 1: Write the failing test**

Add to `newcampaign_test.go`:

```go
func TestCharacterSubmitFieldsUsesSystemFields(t *testing.T) {
	appState = &State{
		CharacterFields: []core.CharacterCreationField{
			{ID: "name", Label: "Name"},
			{ID: "class", Label: "Class", Kind: "select"},
			{ID: "voice", Label: "Voice", Kind: "voice"},
		},
		CharacterAnswers: map[string]string{"name": "Vance", "class": "Rogue"},
	}
	got := characterSubmitFields()
	if got["name"] != "Vance" || got["class"] != "Rogue" {
		t.Fatalf("answers = %#v", got)
	}
	if _, ok := got["voice"]; ok {
		t.Fatal("voice fields must not be submitted as character text")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestCharacterSubmitFields -v`
Expected: FAIL.

- [ ] **Step 3: Implement the form**

When a system is selected in the new-campaign flow, load its `CharacterCreationSpec` (from `ListSystems` + `GetSystem`, or from the already-loaded `SystemDetailDTO`) and render one control per field: `TextInput`/`TextArea` for `text`/`long`, a numeric `TextInput` for `number`, a `MenuButton`+`MenuItem` for `select` (its `Options`), and a voice picker for `voice` (options from `appState.VoiceProfiles`). `characterSubmitFields()` returns all answers except `voice`-kind fields, which are carried as the player's voice profile id.

Extend `submitCreate` to populate `CreateGameRequestDTO.Player` from these answers (the `PlayerCharacterDTO.Extra` map for system-specific fields, or the named fields for the six standard ones — match `pkg/gui/types.go:196`).

Per-field AI: call injected `generateText` with `FormType: "character"`, `FieldName: <field id>`, `Context` seeded from the current answers; apply `res.Fields[field id]`.

- [ ] **Step 4: Run tests, goldens, full gate, commit**

Run: `go test ./pkg/desktop/ -v && mise run desktop:snapshots`
Expected: PASS.

Run: `mise run test`
Expected: PASS (re-run if the `pkg/gui` flake triggers).

```bash
git add -A
git commit -m "feat(gui): add system-driven character creation"
```

---

## Self-Review

**Spec coverage (Phase 6):**

| Spec item | Task |
| --- | --- |
| `SystemsStudio` (manifest, rules, mechanics.js) | Task 2 |
| `WorldsStudio` (lore, lore.md, entities, artwork) | Task 3 |
| character creation | Task 4 (extends the Plan 4/5 new-campaign flow) |
| asset generation/preview | Tasks 3 (world) and reuses Plan 5's preview for campaigns |

**Known gaps carried from the SPA (documented):** no system or world delete; no code editor (monospace `TextArea`); `GenerateCharacter` exists server-side but the SPA drives character fields through `GenerateText`, which this plan also does.

**Placeholder scan:** No TBDs. Tasks 2-4 instruct the implementer to confirm `PlayerCharacterDTO` field names and `gui.ErrWorldExists` against the source; factual verification.

**Type consistency:** `Selection`, `applySelection`, `studioHeader`, `confirmDiscardModal`, `systemPayload`, `worldPayload`, `characterForm`, `characterSubmitFields`, and the injected callbacks are each defined once and used consistently. Service signatures come from the cited `pkg/gui` lines; manifest types from `pkg/core/types.go`.

**Known deferrals (not gaps):** the shared theatre and teardown are the remaining plans.
