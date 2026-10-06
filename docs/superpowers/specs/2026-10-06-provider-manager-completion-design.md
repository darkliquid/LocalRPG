# Provider Manager Completion Design

**Date:** 2026-10-06
**Status:** Proposed
**Issue:** [#31 MP-5](https://github.com/darkliquid/LocalRPG/issues/31)
**Epic:** [#16 Multiple providers of the same type](https://github.com/darkliquid/LocalRPG/issues/16)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §1 (MP-5)
**Depends on:** [#27 MP-1](https://github.com/darkliquid/LocalRPG/issues/27), [#29 MP-3](https://github.com/darkliquid/LocalRPG/issues/29), [#34 LF-3](https://github.com/darkliquid/LocalRPG/issues/34)
**Completes:** `docs/superpowers/specs/2026-10-05-provider-manager-ui-design.md`
**Scope:** `frontend`

---

## 1. Problem

MP-5 shipped the manager's list, add/duplicate/rename/remove, the purposes panel, and the bulk
inspect endpoint, but three parts of the approved spec were not implemented:

1. **No per-entry editing.** The spec (§4.1) and the plan's Task 6 require "the existing per-family
   form bound to that entry". The shipped `ProviderManager` (`frontend/src/components/ProviderManager.tsx`)
   was mounted **additively**: the TTS/STT/Image forms in `SettingsStudio.tsx` still read and write
   the singleton `config.media.<family>`, and nothing selects a named entry. A named provider can be
   created but never configured, so multi-provider is still unreachable from the UI.

2. **Set default looks like a no-op.** `ProviderManager.setDefault` copies the chosen entry into the
   singleton (correct per MP-1: the singleton *is* the default), but the inspect effect keys on
   `[family, providers]`, so changing the singleton never re-inspects. The `default` row's tier and
   key badge stay stale, no confirmation is shown, and the default marker never moves.

3. **Add and duplicate do not select the new entry.** The spec's behaviour table says an added or
   duplicated entry is "editing it immediately"; the shipped handlers only write the config.

A fourth, smaller gap: §4.5's LLM role list (a row per `agents.roles` entry with its tier and key
state) was not built.

## 2. Goals

- Every entry of a family, the singleton included, can be selected and edited from the manager.
- Setting the default visibly changes the default, refreshes its badges, and confirms it.
- Adding or duplicating an entry selects it, so it can be configured straight away.
- The LLM roles get the same list treatment as the media families.
- The config shape is unchanged: the singleton remains the default (MP-1), and an untouched config
  still serializes byte-identically.

## 3. Non-goals

- Selection rules (cheapest, first-that-works). That is MP-4.
- A new backend endpoint or a config schema change; the bulk inspect from MP-5 already serves the
  list.
- Replacing the existing per-family forms' fields; they are reused, only re-bound.

## 4. Design

### 4.1 Reading and writing one entry

A small pure module, `frontend/src/lib/mediaProviders.ts`, owns the address arithmetic so the list
and the forms agree on it:

```ts
export type ProviderFamily = 'tts' | 'stt' | 'image';

// providersKey is the config field holding a family's named providers.
export function providersKey(family: ProviderFamily): 'tts_providers' | 'stt_providers' | 'image_providers';

// mediaEntryValue returns the configuration for a name: the singleton for
// "default", the named map entry otherwise.
export function mediaEntryValue(config: AppConfig, family: ProviderFamily, name: string): TTSConfig | STTConfig | ImageConfig;

// setMediaEntry returns a new config with one entry's configuration replaced.
export function setMediaEntry(config: AppConfig, family: ProviderFamily, name: string, value: unknown): AppConfig;

// entryNames lists "default" first, then the named entries sorted.
export function entryNames(config: AppConfig, family: ProviderFamily): string[];

// uniqueName appends a numeric suffix until the name is free.
export function uniqueName(base: string, taken: Record<string, unknown>): string;
```

`mediaEntryValue` and `setMediaEntry` are the only place the singleton/named distinction is encoded,
so a form bound through them cannot write the wrong slot.

### 4.2 Selection and the per-entry editor

`ProviderManager` gains a selection and hosts the editor through a render prop, so `SettingsStudio`
keeps owning the forms it already has:

```tsx
<ProviderManager
  family="tts"
  config={config}
  onChange={setConfig}
  renderEditor={(name, value, onValue) => (
    <TTSProviderForm value={value} onChange={onValue} ... />
  )}
/>
```

- `ProviderManager` tracks `selected` (default `"default"`).
- Each row gains an **Edit** action that sets `selected`.
- Below the list it renders `renderEditor(selected, mediaEntryValue(config, family, selected), v => setMediaEntry(config, family, selected, v))`.
- When `renderEditor` is nil the manager renders the list alone, so the manager stays usable on its
  own (and in tests).

`SettingsStudio`'s TTS, STT, and Image tabs replace their bare form with a `ProviderManager` that
passes the tab's existing form as `renderEditor`. The forms are re-bound from `config.media.<family>`
to the `value`/`onChange` the manager supplies; no field changes. This is the plan's Task 6, done in
a way that reuses the forms rather than rewriting them.

### 4.3 Set default reflects and confirms

`setDefault(name)` copies the entry into the singleton via `setMediaEntry(config, family, "default", entry)`,
then:

- bumps a `refreshToken` that the inspect effect depends on, so the `default` row's tier and key
  badge re-read the new singleton;
- sets `defaultSource = name` in component state, and renders the filled dot on that row, so the
  marker visibly moves;
- shows a transient confirmation, "Default is now <name>.".

The dot is a session view of the copy the user just made; on reload it falls back to `default`,
because MP-1 stores the default as the singleton, not as a name. Persisting a default *name* would
be a schema change and is out of scope.

### 4.4 Add and duplicate select the new entry

`addProvider` and `duplicateProvider` set `selected` to the name they created, so the editor opens on
it immediately, matching the spec's behaviour table.

### 4.5 The LLM role list

An `llm` mode of `ProviderManager` renders one row per `config.agents.roles` entry: the role name,
the provider label and `TierBadge` from the catalogue (keyed by the role's resolved adapter), and its
key state. Selecting a role sets `selected`, and `renderEditor` supplies the existing role editor.
No structural change to `agents.roles`.

## 5. Behaviour

| Action | Result |
| --- | --- |
| Select an entry | the editor shows that entry's configuration |
| Edit a named entry, save | `media.<family>_providers[name]` is updated |
| Edit the default, save | `media.<family>` (the singleton) is updated |
| Set default | the singleton is replaced; the dot moves; the badges refresh; a confirmation shows |
| Add | a new entry named `new-provider`, selected |
| Duplicate | a copy under `<name>-copy`, selected |
| Rename / remove | unchanged from MP-5 (purposes updated / removal blocked when referenced) |
| A family with only the default | one row, and its editor is the existing form |
| Open and save with no interaction | no config diff |

## 6. Testing

The frontend has no JS test runner; the repo's gate is `tsc --noEmit`, the Vite build, and the
`frontend/scripts/check*.mjs` node scripts wired into `mise run test:frontend`.

- `frontend/scripts/checkProviderManager.mjs`: bundles `src/lib/mediaProviders.ts` with esbuild
  (the `checkEntityScaffold.mjs` pattern) and asserts `mediaEntryValue`/`setMediaEntry`/`entryNames`/
  `uniqueName` for the default and named cases, including a round trip that leaves an untouched
  config byte-identical.
- `tsc --noEmit` and `npm run build` for the component and the re-bound forms.
- Manual: open Settings, select a named entry, edit and save, and confirm the named map changed and
  the singleton did not; set default and confirm the dot and badges move.

## 7. Rollout

Frontend only. A config with no named providers shows one row whose editor is the existing form, so
the tab reads as it does today. No migration and no schema change.

## 8. Risks

- **Re-binding the forms.** The TTS/STT/Image forms are large and reference `config.media.<family>`
  in many handlers. The helpers centralize the read/write, and a no-op-save test guards the default
  path; the risk is mechanical, not semantic.
- **The default dot is session-only.** A reload shows the dot on `default` even when the singleton
  came from a named entry. That is honest to the MP-1 model (the default has no stored name) and is
  noted in the UI copy.
- **LLM key state.** A role's key state comes from the catalogue adapter, which for some roles is
  `inherit`; an inherited role shows its source's state or "inherited".
