# Mechanics Editor Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#41 SYS-5](https://github.com/darkliquid/LocalRPG/issues/41)
**Epic:** [#18 Systems depth (mechanics and rolls)](https://github.com/darkliquid/LocalRPG/issues/18)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §3 (SYS-5)
**Depends on:** [#38 SYS-2](https://github.com/darkliquid/LocalRPG/issues/38)
**Scope:** `pkg/gui`, `frontend`

---

## 1. Problem

A system's declarative mechanics are unauthorable through the app. `MechanicsSpec` covers stats,
skills, health, checks, `allow_freeform_state`, engagement, and advancement
(`pkg/core/mechanics.go:6-20`), but the studio DTOs do not expose it:

- `SystemDetailDTO` carries `ID, Name, Version, Description, Script, RulesPrompt, CharacterCreation`
  (`pkg/gui/types.go:410-418`);
- `CreateSystemRequestDTO` mirrors it (`pkg/gui/types.go:420-428`).

So a system with real mechanics must be hand-edited in `system.yaml`. The Systems Studio edits
`mechanics.js` and `rules.md` but cannot declare a stat, a skill, a difficulty, a resolution
profile, or an advancement track. Every downstream feature that wants to reason about mechanics
(profile selection, skill modifiers, advancement) therefore depends on YAML the user wrote blind.

## 2. Goals

- The studio reads and writes the whole `mechanics` block.
- The user can add, edit, and remove stats, skills, health, check conventions (including profiles),
  and advancement, and toggle freeform state.
- Validation catches an invalid id, a duplicate id, a broken profile, and a dangling reference
  (a skill naming a stat that does not exist, an advancement unlock naming an unknown stat).
- The saved `system.yaml` is unchanged in shape: the editor writes the same fields the manifest
  already carries.

## 3. Non-goals

- A visual rules builder or a `mechanics.js` editor beyond what exists.
- Generating a system (SG-1).
- Editing a world's `system_overrides`; that is out of scope here.

## 4. Design

### 4.1 DTOs

Both DTOs gain the block, so a GET returns it and a PUT accepts it:

```go
type SystemDetailDTO struct {
	// … existing …
	Mechanics *core.MechanicsSpec `json:"mechanics,omitempty"`
}

type CreateSystemRequestDTO struct {
	// … existing …
	Mechanics *core.MechanicsSpec `json:"mechanics,omitempty"`
}
```

`GetSystem` (`pkg/gui/service.go:4011-4041`) sets `Mechanics` from the loaded manifest.
`SaveSystem` (`pkg/gui/service.go:4043-4097`) writes it back into the `system.yaml` it marshals. A
nil `Mechanics` writes no `mechanics` key, so a system that never had one is unchanged.

### 4.2 The editor surface

A **Mechanics** tab in `SystemsStudio.tsx`, beside the existing Script and Rules tabs:

- **Stats** — a table of `{id, label, type, default, min, max}` with add/remove rows.
- **Skills** — a table of `{id, label, stat}` where `stat` is a dropdown of declared stats.
- **Health** — `stat`, `max_stat` (a stat dropdown), `zero_effect`.
- **Checks** — notation, the outcome vocabulary (an ordered list, best first, which drives SYS-3's
  tone), difficulties (a list of `{id, label, target}`), and **profiles** (a sub-editor per profile
  choosing the ladder, DC, pool, or position/effect shape from SYS-2).
- **Advancement** — currency, mode, earn rules, unlocks, levels (the schema already exists at
  `pkg/core/mechanics.go:22-73`).
- **Freeform state** — a toggle for `allow_freeform_state`.
- **Engagement** — a `off`/`auto`/`ask` selector for the system default.

Each list uses the same small add/remove row pattern as the provider manager (MP-5), so the studio
gains no new interaction vocabulary.

### 4.3 Validation

The editor validates before save and the backend is the backstop:

- every id matches `^[a-z0-9][a-z0-9_-]*$` and is unique within its list;
- a skill's `stat` names a declared stat (or is empty);
- a health `stat`/`max_stat` names a declared stat;
- a difficulty's `target` is an integer;
- a profile passes `CheckConventions.Validate` (SYS-2);
- an advancement unlock's effects name declared stats.

`SaveSystem` runs the same checks server-side and returns them in `Warnings` (the existing pattern),
so a client that bypasses the editor cannot persist an invalid system.

### 4.4 Round-trip

The editor must not reorder or drop fields it does not understand. The DTO carries the whole
`MechanicsSpec`, and the TS type mirrors it exactly, so an unedited field survives a load/save. A
test asserts a system with every field set round-trips byte-identically.

## 5. Behaviour

| Action | Result |
| --- | --- |
| Open a system with mechanics | the tab shows every declared element |
| Open a system without mechanics | the tab shows empty lists and the editor can create them |
| Add a stat, save, reload | the stat is present |
| A skill naming an unknown stat | blocked client-side; server returns a warning |
| A malformed profile | blocked; server returns a warning |
| A system with no mechanics, saved unedited | `system.yaml` unchanged |

## 6. Testing

- `pkg/gui`: `GetSystem` returns the mechanics block; `SaveSystem` writes it; a nil block writes no
  key; an invalid block returns warnings; a full block round-trips.
- `frontend`: the editor renders the declared elements; add/remove updates the draft; validation
  messages appear; a system with no mechanics renders empty lists.
- A regression guard: a system created before this change loads and saves without a `mechanics` key.

## 7. Rollout

Additive DTO fields and a new studio tab. Existing systems have no `mechanics` and are unaffected.

## 8. Risks

- **Studio growth.** `SystemsStudio.tsx` is already large. The mechanics editor should be its own
  component (`MechanicsEditor.tsx`) mounted by the studio, with small list-row components, matching
  the review's advice to keep units focused.
- **Schema drift.** The TS mirror of `MechanicsSpec` must track the Go struct. A test that loads a
  fixture with every field set catches a drift.
- **Over-engineering the advancement editor.** Advancement is the most complex block; scope its
  editor to the schema's actual fields and defer a visual track builder.
