# Provider Manager UI Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#31 MP-5](https://github.com/darkliquid/LocalRPG/issues/31)
**Epic:** [#16 Multiple providers of the same type](https://github.com/darkliquid/LocalRPG/issues/16)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §1 (MP-5)
**Depends on:** [#27 MP-1](https://github.com/darkliquid/LocalRPG/issues/27), [#29 MP-3](https://github.com/darkliquid/LocalRPG/issues/29), [#34 LF-3](https://github.com/darkliquid/LocalRPG/issues/34)
**Scope:** `pkg/gui`, `frontend`

---

## 1. Problem

The Settings Studio edits one provider per family. `config.media.tts`, `.stt`, and `.image` are each
a single form (`frontend/src/components/SettingsStudio.tsx`), and the LLM side edits one role at a
time (`config.agents.roles[selectedRole]`). MP-1 and MP-3 add named providers and purposes to the
config, but nothing in the UI can create a second entry, name it, point a purpose at it, or see
which providers are actually in use.

Without a manager, the multi-provider feature is reachable only by hand-editing `config.yaml`, which
is exactly the barrier the settings UI exists to remove.

## 2. Goals

- See every provider of a family in one place, with its name and its capability tier (LF-3).
- Add, duplicate, rename, and remove an entry.
- Choose the default entry per family.
- See and set which **purpose** resolves to which entry (MP-3).
- See, per entry, whether its credential is present and whether it is usable.
- Nothing is saved until the user saves; a rename or remove that would strand a purpose is caught
  before the save.

## 3. Non-goals

- Selection rules (cheapest, first-that-works). That is MP-4.
- A new backend provider registry; the catalogue already exists (`/api/providers`).
- Editing the raw YAML.

## 4. Design

### 4.1 Structure

A **Providers** section in the Settings Studio, organised by family. Each family shows a list of
entries and a default marker:

```
TTS
  ● default   ElevenLabs · Cloud        key ✓    [uses: narrator]   [Edit] [Duplicate] [Remove]
  ○ npc       Kokoro (sherpa-onnx) · Offline · small model  —  [uses: npc]  [Edit] [Duplicate] [Remove]
  + Add TTS provider
```

- The **filled dot** marks the default; clicking another row's dot makes it the default.
- The **tier badge** is the LF-3 `TierBadge`.
- The **key indicator** is `key ✓` / `key ✗` / `no key needed`, from the catalogue descriptor and
  `/api/tts/inspect`.
- The **uses** list shows every purpose (MP-3) that resolves to this entry; editing it is a
  dropdown per purpose.
- **Edit** expands the existing per-family form bound to that entry.
- **Add** appends `providers["new-provider"]` with the family's current defaults.
- **Duplicate** copies the entry to a new name (the common "same provider, different key" case).
- **Remove** deletes the entry, after checking for references (§4.4).

### 4.2 Purposes panel

A compact **Uses** panel per family lists the purposes for that family with a provider dropdown:

```
Uses
  narrator  →  [default ▾]
  npc       →  [npc ▾]
```

The dropdown offers `default` plus every named entry. A purpose with no explicit choice shows
`default` and writes nothing to `media.purposes`, so the config stays minimal.

### 4.3 Data flow

The studio already loads the whole `config` and saves it whole. The manager needs no new
read endpoint:

- The entry list is `config.media.tts_providers` (and `stt_providers`, `image_providers`) plus the
  singleton as `default`.
- The tier and capability badges come from `/api/providers` (the catalogue, keyed by adapter) and
  `/api/tts/inspect` for a specific entry.
- Key presence comes from the inspect response's `KeyPresent`/`KeyRequired`
  (`TTSInspectResponseDTO`, `pkg/gui/types.go:509-524`).

One small backend addition: a **bulk inspect** so the list can show key state for every entry in one
call rather than N. `POST /api/media/inspect` takes the family's entries and returns their
`{name, providerKey, keyPresent, keyRequired, metered, tier}`. It reuses the existing inspect
builder (`pkg/gui/tts_inspect.go:15-85`), so no provider code changes.

### 4.4 Referential integrity

A rename or remove can strand a `media.purposes` entry. The UI handles it client-side and the
backend is the backstop:

- **Rename**: offer to update every purpose that referenced the old name. Default to yes.
- **Remove**: if a purpose references the entry, block the removal until the user reassigns it, or
  offer to reset that purpose to `default`.
- **Save**: `SaveSettings` already returns `Warnings`; MP-3's validation turns a dangling reference
  into an error, so a client that bypasses the UI still cannot persist an invalid config.

### 4.5 LLM roles

The LLM side keeps its role-based editor (roles already give it several providers), but the same
list treatment applies: a row per role with its provider, tier badge, and key state, so the LLM and
media views read alike. No structural change to `agents.roles`.

## 5. Behaviour

| Action | Result |
| --- | --- |
| Add | a new entry named `new-provider`, editing it immediately |
| Duplicate | a copy under `<name>-copy`, editing it immediately |
| Rename | the entry key changes; referencing purposes are updated (or the rename is blocked) |
| Remove | the entry is deleted; referencing purposes must be reassigned first |
| Set default | the entry's name is written as the family's default |
| Set a purpose | `media.purposes[use]` is written, or removed when set back to `default` |
| Save with a dangling purpose | blocked client-side; the backend validation is the backstop |
| No extra providers configured | the family shows one row, `default`, exactly as today |

## 6. Testing

- `pkg/gui`: the bulk-inspect endpoint returns a row per entry with the expected key state; it
  reuses the single inspect path.
- `frontend`: the manager renders one row per entry; add/duplicate/rename/remove update the draft
  config; rename updates referencing purposes; remove is blocked when referenced; the purpose
  dropdown writes and clears `media.purposes`; a family with only the default shows one row.
- A regression guard: opening the manager and saving without changes produces no config diff.

## 7. Rollout

Frontend plus one additive endpoint. A config with no named providers shows the current single form
in the new list chrome. No migration.

## 8. Risks

- **Studio size.** `SettingsStudio.tsx` is already the largest component. The manager should be a
  new component (`ProviderManager.tsx`) mounted by the studio, not more code in the studio file,
  following the review's recommendation to keep units focused.
- **Stale key state.** The bulk inspect is a network call; a provider whose server is down reports
  `keyPresent` but not reachable. The indicator says "key present", not "working"; a separate
  "Test" action (the existing `test-provider` endpoint) is the way to check reachability.
- **Rename churn.** Renaming changes cache namespaces if MP-2's cache-key follow-up lands. The UI
  should note that a rename may re-synthesize audio.
