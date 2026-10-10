# Systems Studio Authoring Design

**Date:** 2026-10-10
**Status:** Proposed
**Issue:** [#127](https://github.com/darkliquid/LocalRPG/issues/127)
**Epic:** [#122 Studio UX and authoring](https://github.com/darkliquid/LocalRPG/issues/122)
**Depends on:** [System Test Harness Design](2026-10-05-system-test-harness-design.md), [System Enhancement Design](2026-10-05-system-enhancement-design.md), [System Generation Design](2026-10-05-system-generation-design.md)
**Scope:** `frontend`, `pkg/gui`, `pkg/systemtest`, `pkg/gui/docs`

---

## 1. Problem

Five defects in the Systems Studio make authoring harder than it needs to be. They are
one spec because they are one screen and one review surface.

1. **Starting points are redundant and destructive.** `SystemsStudio.tsx:517-533` renders
   a "Starting points" row whose buttons call `applyReference` (`:260-275`), which
   overwrites the fields of whatever system is currently open and does not change the
   selection. "From a Base" (`:484-492`) opens `BaseSystemCatalogue`, whose Clone button
   calls the *same* `applyReference` (`:1295-1299`). The two entry points are the same
   action under two names, and neither is clearly "start something new". A third entry,
   the "Reset Template" button (`:688-696`), also calls `applyReference`.
2. **The mechanics form has prose help but no examples.** `MechanicsEditor.tsx` attaches
   a `HelpTip` to sections and fields (`Sections`/`Field help=…`), and the help is
   decent, but there is no worked example anywhere. Two profile fields the backend
   supports, `Opposed` and `Ties` (`pkg/core/profile.go:28,31`), are missing from the
   frontend `ResolutionProfile` type (`frontend/src/types.ts:973-982`) and so cannot be
   set at all.
3. **The mechanics.js API reference has no examples.** `mechanics/ScriptReference.tsx`
   lists eight globals and six hooks as one-line signatures, hardcoded in two arrays
   (`GLOBALS`, `HOOKS`) plus one whole-page `EXAMPLE`. There is no per-entry example and
   nothing is individually expandable.
4. **Tests can be run but not defined.** `SystemsStudio.tsx` has a "Run Tests" button
   (`:698-707` → `handleRunTests` `:367-392`) that fetches `systems/<id>/tests/*.yaml`
   via `GET /api/system/tests/{id}` and posts them to `POST /api/system/test`. There is
   no scenario editor anywhere in the frontend, no scenario count before a run, and no
   empty state: with no scenarios the button reports "All 0 scenario(s) passed". The only
   way to define a test is to hand-write YAML on disk. The guide even advertises a "Dice
   & Rules Tester" at the bottom of the studio (`pkg/gui/docs/09-systems-studio.md:107`)
   that does not exist.
5. **Character creation prompts are opaque.** The editor (`:954-1093`) exposes `id`,
   `label`, `prompt`, `kind`, `required`, and `generatable`, but not `options` or
   `default`, which `core.CharacterCreationField` supports (`pkg/core/types.go:27-43`).
   Choosing `kind: select` therefore offers no way to enter options. No help explains
   what each field does or how `kind` is interpreted; in fact nothing interprets `kind`
   except that `voice` is excluded from generation and from the required set, and the
   campaign form (`launcher/NewCampaignModal.tsx`) ignores the authored fields entirely
   and renders a hard-coded six.

## 2. Goals

- One "start from a base" entry point, and it does not overwrite the open system.
- The mechanics form carries per-section and per-field examples, and exposes every
  field the backend supports.
- Every global and hook in the sandbox reference has an expandable example.
- A scenario editor: list, create, edit, and delete tests in the studio, with a count
  and an empty state.
- Character creation fields are documented per kind, `select` can define its options,
  and the authored fields are what the campaign form renders.

## 3. Non-goals

- Changing the scenario format or the deterministic runner. The YAML stays as
  `pkg/systemtest` defines it.
- A YAML source editor for scenarios. The editor is a structured builder over the same
  types; hand-editing the file remains possible and is still honoured.
- Changing how mechanics are evaluated or how the smoke gate works.
- Signing or sharing tests; they stay per-system files.

## 4. Design

### 4.1 Starting points

- **Delete** the "Starting points" row (`SystemsStudio.tsx:517-533`) and its now-unused
  `applyReference` call site. `referenceSystems` is still loaded for the catalogue.
- **Make Clone non-destructive.** `BaseSystemCatalogue`'s `onClone` currently calls
  `applyReference` (`:1295-1299`). It instead calls a new `handleStartFromBase(base)`
  that seeds a *new* unsaved system draft, never touching the open selection:

  ```ts
  const handleStartFromBase = (base: ReferenceSystem) => {
    setShowBaseCatalogue(false);
    setSelection({ kind: 'draft' });
    setActiveDraftID(null);
    setDraft({ localId: crypto.randomUUID(), dirty: true });
    setName(`${base.name} (copy)`);
    setSlugID(`${base.id}-copy`);
    setVersion(base.version);
    setDescription(base.description);
    setRulesPrompt(base.rules_prompt);
    setScript(base.script);
    setMechanics(base.mechanics ?? {});
    setCreationPreamble('');
    setCreationFields([]);
    setVerificationResult(null);
    setDraftNotes([]);
    setActiveTab('manifest');
    setToast({ type: 'success', message: `Started a new system from ${base.name}.` });
  };
  ```

  The draft's id is a suggestion the author edits before the first save; a clash is
  reported by the existing save path.
- **Relabel for clarity.** The catalogue buttons read "Start from this base" (clone) and
  "Derive a variant" (AI), with one line each explaining the difference: a clone copies
  the base into a new editable system; a derive asks the model for a changed variant.
- **"Reset Template" stays, renamed and guarded.** `handleResetToReference`
  (`:277-285`) still replaces the *current* editor's contents, which is a different
  action. Rename the button to "Replace with 2d6" and require a confirmation, so the
  destructive in-place reset is distinct from "start from a base". `applyReference`
  remains only for this path.

### 4.2 Mechanics form help and examples

- Add `components/ui/Example.tsx`: a compact, titled, monospace block (`label`,
  `children`), visually like `HelpTip`'s tooltip but inline. Add an optional `example`
  prop to `Section` and `Field` so a section or field can carry one.
- Attach examples to every section of `MechanicsEditor.tsx`: stats, skills, health,
  checks (including one worked `difficulty_class` profile and one worked `dice_pool`
  profile), advancement, and policy. Examples are short YAML fragments, for example for
  Checks:

  ```yaml
  checks:
    default_notation: "2d6+{modifier}"
    outcome_vocabulary: [strong_hit, weak_hit, miss]
    difficulties:
      - { name: routine, target: 7 }
    profiles:
      - name: standard
        shape: difficulty_class
        notation: "2d6+{modifier}"
  ```

- **Expose the missing profile fields.** Add `opposed?: boolean` and `ties?: string` to
  the frontend `ResolutionProfile` type and render them in `ProfilesEditor`, with help
  describing when a profile is opposed and what a tie resolves to. This closes the
  backend/frontend type gap.
- Improve the existing section help where it only restates the label; each section's help
  gains a sentence on when to use it, and the example shows the shape.

### 4.3 mechanics.js API reference

`ScriptReference.tsx` becomes a list of expandable entries. Each entry in `GLOBALS` and
`HOOKS` gains `example` (a short script) and keeps `signature` and `description`:

```ts
interface ApiEntry {
  signature: string;   // roll(notation)
  description: string;  // Rolls dice and returns the total and faces.
  example: string;      // const r = roll('2d6+3'); log(r.total);
}
```

Entries render as a `<details>` whose summary is the signature plus a one-line
description, and whose body is the example in a monospace block with a copy button.
`Globals` and `Hooks` are two such lists; the whole-page worked example stays below
them. A small test asserts every entry has a non-empty `example`, so a new global cannot
ship undocumented. The doc `pkg/gui/docs/09-systems-studio.md:79-97` already points at
the reference and needs no structural change; add one sentence that each entry expands
to an example.

### 4.4 Defining tests in the studio

**Backend.** `handleSystemTestsRoute` (`pkg/gui/server.go`, mounted at
`/api/system/tests/`) currently serves `GET` only. It gains `POST` (upsert) and `DELETE`:

```go
// SaveSystemScenario writes one scenario under systems/<id>/tests/<slug>.yaml.
// It re-parses what it wrote with systemtest.LoadScenario before the write lands.
func (s *Service) SaveSystemScenario(id string, sc systemtest.Scenario) error
// DeleteSystemScenario removes one scenario file by its scenario name.
func (s *Service) DeleteSystemScenario(id, name string) error
```

The filename is `entity.Slugify(sc.Name)` plus `.yaml`; the scenario's declared `Name`
is authoritative and must be non-empty. The write is validated by marshalling to YAML,
re-reading through `LoadScenario`, and only then writing the file, so a step-less or
malformed scenario is rejected with `400` rather than stored. `DELETE` takes
`?name=<name>`, resolves the slug, and returns `404` when absent.

**Frontend.** The studio gains a fifth tab, `tests`, beside manifest/rules/mechanics/
script. It contains:

- A header showing the scenario count and a "New scenario" button; with zero scenarios an
  empty state reads "No scenarios yet. Add one to test this system's mechanics without a
  provider."
- A list of scenarios (name, step count, seed) with Edit and Delete.
- `components/ScenarioEditor.tsx`, a structured builder over the `systemtest.Scenario`
  shape: `name`, `seed`, the player's `stats` (key/value rows) and `tags`, and a list of
  steps. Each step has `action`, optional `input`, and an expectations group: `outcome`,
  `outcome_one_of` (comma-separated), `total` min/max, `state` (key/value rows), and
  `message_contains`. An empty expectations group is labelled "setup step (asserts
  nothing)".
- The client gains `saveSystemScenario(id, scenario)` and
  `deleteSystemScenario(id, name)`; `listSystemScenarios` and `runSystemTest` already
  exist.

"Run Tests" stays where it is; before a run the button now has a count beside it, and
after a run the existing inline failure list is shown. The `09-systems-studio.md` "Dice
& Rules Tester" sentence is replaced with a description of the Tests tab, and
`22-editing-content.md` gains a short worked scenario example.

### 4.5 Character creation prompts

**Editor changes** (`SystemsStudio.tsx:954-1093`):

- Add a `default` text input per field.
- When `kind === 'select'`, render an options editor: a list of strings with add/remove,
  writing `field.options`.
- Add a `HelpTip` to every field: `id` (stable key the campaign records), `label`
  (shown to the player), `prompt` (guidance for the AI generator), `kind` (see below),
  `required` (the player must answer it; a `voice` field is never required),
  `generatable` (the AI may fill it; disabled for `voice`), `options` (the choices a
  `select` offers), `default` (prefilled value).
- Add a section-level example showing a `select` field with options:

  ```yaml
  character_creation:
    preamble: Answer in the tone of a wandering chronicle.
    fields:
      - id: calling
        label: Calling
        kind: select
        prompt: The character's trade or vocation.
        required: true
        options: [sellsword, scholar, thief, priest]
        default: scholar
  ```

**Make `kind` mean something.** Today only `voice` is interpreted. The design fixes this
by rendering the authored fields in campaign creation:

- `launcher/NewCampaignModal.tsx` stops hard-coding its six fields and renders
  `CharacterFields(manifest)` (`pkg/engine/character.go:26-55` already selects the
  authored fields, or the default six when none are authored).
- Mapping: `text` and `long` to a text input and textarea; `number` to a number input;
  `select` to a `<select>` over `options` with `default` selected; `voice` to the
  existing voice picker. `required` marks the field; `generatable` shows the
  `AIGenerateButton` for that field, reusing `APIClient.generateText` with
  `form_type: 'character'` and the field's id.
- The system manifest must reach the modal. `NewCampaignModal` already loads the chosen
  system to display it; it reads `character_creation.fields` from that same detail.
- Generation is unchanged: `characterGeneratorPrompt` (`pkg/gui/character_generate.go`)
  already lists every generatable, non-voice field with its prompt, so an authored
  `select` is still one JSON key among the others.

If a migration concern arises, `DefaultCharacterFields()` is the fallback and keeps
existing campaigns working; the change only affects the form for *new* campaigns using a
system that authors fields.

## 5. Behaviour

| Action | Before | After |
| --- | --- | --- |
| Click a starting point | open system's fields overwritten | (removed) |
| From a Base, Clone | open system's fields overwritten | a new unsaved system, base untouched |
| From a Base, Derive | new draft | unchanged |
| Reset Template | replaces current editor silently | "Replace with 2d6", confirmed |
| Read the mechanics form | prose help only | help plus a worked example per section |
| Set a profile's opposed/tie rule | impossible | editable |
| Read a sandbox global | one-line signature | expandable example |
| Define a test | hand-edit YAML on disk | Tests tab, structured editor |
| Add a `select` prompt | no options field | options list, default, help |
| Start a campaign | six hard-coded fields | the system's authored fields |

## 6. Testing

- `pkg/gui`: `SaveSystemScenario` round-trips a scenario, rejects one with no steps, and
  writes under `tests/<slug>.yaml`; `DeleteSystemScenario` removes it and 404s when
  absent; the `POST`/`DELETE` methods dispatch on `handleSystemTestsRoute` (extend
  `system_test_test.go`).
- `frontend`: vitest for the scenario editor (add step, assert an outcome, save), the
  empty "no scenarios" state, the mechanics `Example` rendering, `ScriptReference`'s
  "every entry has an example" guard, the select-options editor, and the campaign form
  rendering authored fields (including a `select`).
- `tsc --noEmit` and `npm run build` are the gate; the new `Example`/`ScenarioEditor`
  files must be free of unused imports.
- `mise run lint:docs` after the guide edits (`09-systems-studio.md`,
  `22-editing-content.md`).

## 7. Rollout

Frontend-heavy, with one small backend addition (two service methods and two handler
branches). Ship in two reviewable halves: the authoring polish (4.1 to 4.3 and 4.5) and
the Tests tab (4.4), which is the only part that touches the filesystem.

## 8. Risks

- **Non-destructive clone changes muscle memory.** A user who expects Clone to overwrite
  the open system will instead get a new draft. The toast and the relabelled buttons say
  so; the undesired "replace in place" action still exists as the confirmed "Replace with
  2d6".
- **Writing scenario files must stay inside the system directory.** `SaveSystemScenario`
  resolves `tests/` under `s.resolver.SystemDir(id)` and slugifies the name, so a name
  cannot escape the directory. It validates the id with `pathutil.ValidateID` like the
  other system operations.
- **Rendering authored fields in the campaign form is a behaviour change.** A system that
  authors fields will now show them instead of the default six. This is the point, but it
  is the change most likely to surprise; the default-six fallback keeps systems that
  author nothing exactly as they are.
- **Doc drift.** The "Dice & Rules Tester" sentence is already wrong; the Tests tab edit
  must replace it rather than add to it.
