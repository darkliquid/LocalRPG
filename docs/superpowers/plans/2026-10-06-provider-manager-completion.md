# Provider Manager Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish MP-5: let the manager select and edit any entry (default or named), make set-default visible, select a new entry on add/duplicate, and give the LLM roles the same list.

**Architecture:** A pure `lib/mediaProviders.ts` owns the singleton/named address arithmetic; `ProviderManager` gains a selection and a `renderEditor` render prop; `SettingsStudio` passes its existing per-family forms as the editor; set-default refreshes the inspect and moves the dot.

**Tech Stack:** React 19 + Tailwind v4; a `frontend/scripts/check*.mjs` node script for the pure helpers (esbuild, the `checkEntityScaffold.mjs` pattern).

**Spec:** `docs/superpowers/specs/2026-10-06-provider-manager-completion-design.md`
**Completes:** `docs/superpowers/plans/2026-10-05-provider-manager-ui.md` (Task 6 and the set-default behaviour)

## Global Constraints

- `tsconfig.json` has `noUnusedLocals` and `noUnusedParameters`: no unused imports or params.
- The manager must not change the config shape; an untouched config stays byte-identical.
- The frontend has no JS test runner; the gate is `tsc --noEmit`, `npm run build`, and the `check*.mjs` scripts.
- Conventional Commits with a scope, subject under 72 chars.

## File Map

- Create `frontend/src/lib/mediaProviders.ts` - the pure address helpers.
- Create `frontend/scripts/checkProviderManager.mjs` - the helper assertions.
- Modify `frontend/package.json` - add the `check:provider-manager` script.
- Modify `mise.toml` - add the check to `test:frontend`.
- Modify `frontend/src/components/ProviderManager.tsx` - selection, `renderEditor`, set-default feedback, select-on-add.
- Modify `frontend/src/components/SettingsStudio.tsx` - mount the manager with each family's form as `renderEditor`, re-bound to the selected entry.

---

### Task 1: The pure entry helpers

**Files:**
- Create: `frontend/src/lib/mediaProviders.ts`
- Create: `frontend/scripts/checkProviderManager.mjs`
- Modify: `frontend/package.json`, `mise.toml`

**Interfaces:**
- Consumes: `AppConfig`, `TTSConfig`, `STTConfig`, `ImageConfig` (`frontend/src/types.ts`).
- Produces: `ProviderFamily`, `providersKey`, `mediaEntryValue`, `setMediaEntry`, `entryNames`, `uniqueName`, `purposesFor`.

- [ ] **Step 1: Write the failing check**

`frontend/scripts/checkProviderManager.mjs`, bundling the helper with esbuild the way
`checkEntityScaffold.mjs` does, then asserting:

```js
// A default read is the singleton; a named read is the map entry.
// A named write touches only the map; a default write touches only the singleton.
// An untouched config round-trips byte-identically.
// entryNames puts "default" first; uniqueName suffixes until free.
```

- [ ] **Step 2: Run it to verify it fails**

Run: `node frontend/scripts/checkProviderManager.mjs`
Expected: FAIL, the module does not exist.

- [ ] **Step 3: Write the helpers**

Move `providersKey`, `uniqueName`, and `purposesFor` out of `ProviderManager.tsx` into
`mediaProviders.ts`, and add `mediaEntryValue`, `setMediaEntry`, and `entryNames` as in the spec §4.1.
`setMediaEntry` returns `{ ...config, media: { ...config.media, [slot]: { ...entry, [name]: value } } }`
for a named entry and `{ ...config, media: { ...config.media, [family]: value } }` for `default`, and
must not add an empty `*_providers` map when writing the default.

- [ ] **Step 4: Run it to verify it passes**

Run: `node frontend/scripts/checkProviderManager.mjs`
Expected: PASS. Then `cd frontend && npx tsc --noEmit`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/mediaProviders.ts frontend/scripts/checkProviderManager.mjs frontend/package.json mise.toml
git commit -m "feat(frontend): add media provider entry helpers"
```

---

### Task 2: Selection and the editor render prop

**Files:**
- Modify: `frontend/src/components/ProviderManager.tsx`

**Interfaces:**
- Consumes: the Task 1 helpers.
- Produces: `ProviderManagerProps.renderEditor`, the `selected` state, and an **Edit** row action.

- [ ] **Step 1: Add the selection**

Add `renderEditor?: (name: string, value: unknown, onChange: (value: unknown) => void) => React.ReactNode`
to the props, a `selected` state defaulting to `"default"`, an **Edit** action on each row that sets
it, and below the list:

```tsx
{renderEditor && (
  <div className="rounded-lg border border-stone-800 bg-stone-950/40 p-3">
    {renderEditor(selected, mediaEntryValue(config, family, selected), (value) =>
      emit(setMediaEntry(config, family, selected, value))
    )}
  </div>
)}
```

When `renderEditor` is nil the manager renders only the list, so it stays self-contained.

- [ ] **Step 2: Typecheck and build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/ProviderManager.tsx
git commit -m "feat(frontend): select and edit a provider entry"
```

---

### Task 3: Bind the family forms to the selected entry

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`

**Interfaces:**
- Consumes: `ProviderManager` (Task 2), the existing TTS/STT/Image form bodies.
- Produces: each media tab renders a `ProviderManager` whose `renderEditor` is that tab's form, bound to the selected entry.

- [ ] **Step 1: Re-bind the TTS form**

Extract the TTS tab's form body into a local `TTSProviderForm({ value, onChange, ... })` that reads
`value` and calls `onChange(next)` instead of touching `config.media.tts`, and pass it as
`renderEditor` to `<ProviderManager family="tts" ... />`. Preserve the existing inspect, preset, and
voice controls by feeding them `value` and threading the other props through.

- [ ] **Step 2: Re-bind the STT and Image forms**

Do the same for STT and Image.

- [ ] **Step 3: Verify no-op save and the default path**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Then confirm by hand that editing the default writes `media.<family>`, editing a named entry writes
`media.<family>_providers[name]`, and opening and saving untouched changes nothing.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): edit a named provider from the manager"
```

---

### Task 4: Set-default reflects and confirms

**Files:**
- Modify: `frontend/src/components/ProviderManager.tsx`

**Interfaces:**
- Consumes: `setMediaEntry` (Task 1).
- Produces: a `refreshToken` the inspect effect depends on, a `defaultSource` marker, and a confirmation.

- [ ] **Step 1: Make set-default refresh**

Add a `refreshToken` state and include it in the inspect effect's dependency list. In `setDefault`,
after `emit(setMediaEntry(config, family, "default", entry))`, set `defaultSource` to the chosen name
and bump `refreshToken`. Render the filled dot on `defaultSource` instead of always on `default`, and
show a transient "Default is now <name>." line that clears on the next interaction.

- [ ] **Step 2: Typecheck and build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/ProviderManager.tsx
git commit -m "fix(frontend): reflect and confirm the default provider"
```

---

### Task 5: Select on add and duplicate

**Files:**
- Modify: `frontend/src/components/ProviderManager.tsx`

**Interfaces:**
- Consumes: Task 2's `selected`.
- Produces: `addProvider` and `duplicateProvider` set `selected` to the new name.

- [ ] **Step 1: Select the new entry**

Set `selected` to the name created by `addProvider` and `duplicateProvider`, so the editor opens on it.

- [ ] **Step 2: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/ProviderManager.tsx
git commit -m "feat(frontend): open the editor on a new provider entry"
```

---

### Task 6: The LLM role list

**Files:**
- Modify: `frontend/src/components/ProviderManager.tsx`
- Modify: `frontend/src/components/SettingsStudio.tsx`

**Interfaces:**
- Consumes: the catalogue (`useProviderCatalog`), `config.agents.roles`.
- Produces: an `llm` mode rendering a row per role with its provider label, `TierBadge`, and key state; the existing role editor as its `renderEditor`.

- [ ] **Step 1: Add the mode**

Allow `family: 'llm'`. In that mode the list is `Object.keys(config.agents.roles)`; each row shows the
role's label, the catalogue descriptor for its resolved adapter (reusing the label and tier mapping),
and its key state. Selecting a role sets `selected`; `renderEditor` supplies the existing role editor
bound to `config.agents.roles[selected]`.

- [ ] **Step 2: Mount it**

Render the `llm` manager in the Agents tab above the existing role editor.

- [ ] **Step 3: Typecheck and build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/ProviderManager.tsx frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): list LLM roles like media providers"
```

---

### Task 7: Verification

- [ ] **Step 1: The check and the type checks**

Run: `node frontend/scripts/checkProviderManager.mjs && cd frontend && npx tsc --noEmit && npm run build`
Expected: PASS.

- [ ] **Step 2: The full gates**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Every entry, the default included, can be selected and edited.
- Setting the default changes the singleton, moves the dot, refreshes the badges, and confirms.
- Add and duplicate open the editor on the new entry.
- The LLM roles list mirrors the media families.
- Opening and saving untouched produces no config diff.
