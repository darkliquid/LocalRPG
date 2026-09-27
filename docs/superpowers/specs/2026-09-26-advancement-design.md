# Advancement & XP Design

**Date:** 2026-09-26
**Status:** Proposed
**Scope:** A schema-agnostic advancement system: an earned currency, declarative earn triggers, a catalog of unlocks, three cost modes, a spend API, and the character-drawer surface
**Related:** Mechanics engagement, visibility, and advancement research (`docs/proposals/2026-09-26-mechanics-engagement-and-advancement-research.md`), Mechanics Engagement & Declarative Schema Design (2026-09-25), Mechanics Trigger & Cadence Design (2026-09-26), Mechanics Engagement & Visibility Design (2026-09-26), `pkg/core`, `pkg/rules`, `pkg/engine`, `pkg/gui`, `frontend/`

## 1. Overview & Goals

There is no advancement system. A repo search for `XP|level|experience|
progression` in `pkg/` finds nothing; the character sheet's `level` is an
arbitrary `state` value the GM could set like any other
(`frontend/src/components/CharacterSheetDrawer.tsx:14,20`). The building blocks
exist: entity `state` frontmatter (`pkg/entity/entity.go:41,62,236-237`),
declared stats (`core.StatSpec`), validated state changes
(`pkg/engine/submission.go:130-136`, `pkg/rules/state_changes.go:14-55`),
`mechanics.js` hooks, and the character drawer.

The four systems surveyed (Dungeon World, Ironsworn, Blades in the Dark, D&D
5e) differ in what is earned, what it buys, and how it is bought, but share one
shape: an earned currency plus a catalog of unlocks, with a cost model and an
optional timing gate. This specification encodes that shape declaratively so no
system is hardcoded.

**Goals:**

- A system declares its advancement: currency, earn triggers, mode, unlocks,
  levels, gates.
- The engine recognises common earn events (a miss, a strong/weak outcome, a
  turn end) without JS, and exposes `grantXP` to `mechanics.js` for everything
  else.
- Three cost modes cover the surveyed systems: fixed-cost `spend`, `track`
  (fill N, clear, choose), and `threshold` (auto-apply on a table).
- A spend applies through the existing state-change path, so Markdown stays
  canonical and the index mirrors it.
- The player sees affordable unlocks in the character drawer, with a dot on the
  Character button when something can be spent.

**Non-Goals:**

- Inventing a cost, a currency, or an unlock for a system that declares none.
- Modelling shared/crew ledgers (Blades' crew track, Ironsworn's shared vows) —
  recorded as an open question; the model is per-character first.
- A full character-builder; unlocks apply to existing stats/tags/hooks.

**Success Criteria:**

- A campaign with no `advancement` block behaves exactly as today.
- A `spend`-mode system awards the currency on its declared earn events and
  lets the player buy an unlock; the currency is deducted and the effect is
  visible in the character sheet and the entity's frontmatter.
- A `threshold`-mode system auto-applies the next level when the currency
  crosses it, and records the event.
- A `track`-mode system fills a track and, when full, clears it and offers the
  advancement.
- The Character button shows a dot exactly when at least one unlock is
  affordable.
- Every award and spend is recorded in the campaign's memory trail.

## 2. Investigation Findings

- No advancement concept anywhere (`pkg/` has no XP/level/progression).
- `state` is the durable home for character data: `EntityFrontmatter.State`
  round-trips to frontmatter (`pkg/entity/entity.go:41,236-237`), and the sheet
  already renders arbitrary `player.state` keys
  (`CharacterSheetDrawer.tsx:51-61`).
- State changes are proposed by the GM (`StateChangeDecl`,
  `pkg/harness/turn.go:54-60`), validated against declared stats unless
  freeform (`submission.go:130-136`), and applied by `rules.ApplyStateChanges`
  (`pkg/rules/state_changes.go:14-55`).
- `mechanics.js` has `getStat`/`setStat`, `roll`, `injectGMDirection`, and
  lifecycle hooks (`pkg/rules/js_engine.go:45-158`) — the place a `grantXP` call
  belongs.
- Checks leave a mechanical memory trail already
  (`pkg/engine/timeline.go:560-598`), so an advancement record has a precedent.
- The game-state payload already refreshes after each turn via `refreshCorpus()`
  (`frontend/src/App.tsx:237`), which is where an affordability signal rides.

## 3. Design

### 3.1 The declarative block

```yaml
# system.yaml
mechanics:
  # ...existing stats/skills/health/checks...
  advancement:
    currency: { stat: xp, label: Experience }
    mode: spend            # spend | track | threshold
    earn:
      - { on: miss, amount: 1 }
      - { on: check_outcome, outcome: strong, amount: 2 }
      - { on: turn_end, amount: 0 }            # present but inert
      - { on: hook }                            # mechanics.js grantXP(amount)
    track_size: 6          # mode: track
    gate: ""               # "" | "downtime"
    unlocks:
      - id: stat-increase
        label: Increase a stat
        cost: 5
        requires: [level_2]                     # optional unlock ids or tags
        effects:
          - { type: stat_increase, amount: 1, max: 18 }
      - id: new-asset
        label: New asset
        cost: 3
        effects:
          - { type: hook, hook: onAcquireAsset }
    levels:                 # mode: threshold
      - { at: 300, label: "Level 2", effects: [{ type: stat_increase, amount: 1 }] }
```

Go types in `pkg/core`:

```go
type AdvancementSpec struct {
    Currency  CurrencySpec  `yaml:"currency"`
    Mode      string        `yaml:"mode,omitempty"`      // spend|track|threshold
    Earn      []EarnRule    `yaml:"earn,omitempty"`
    TrackSize int           `yaml:"track_size,omitempty"`
    Gate      string        `yaml:"gate,omitempty"`
    Unlocks   []UnlockSpec  `yaml:"unlocks,omitempty"`
    Levels    []LevelSpec   `yaml:"levels,omitempty"`
}
type CurrencySpec struct{ Stat, Label string }
type EarnRule struct{ On, Outcome, Rank string; Amount int }
type UnlockSpec struct{ ID, Label, Description string; Cost int; Requires []string; Effects []EffectSpec }
type EffectSpec struct{ Type, Stat, Tag, Hook string; Amount, Max int }
type LevelSpec struct{ At int; Label string; Effects []EffectSpec }
```

`MechanicsSpec` gains `Advancement *AdvancementSpec` (`pkg/core/mechanics.go`).
The currency `stat` is also declared as a `StatSpec` so state-change validation
accepts it.

### 3.2 Earning

The engine evaluates earn rules **after** a turn's checks resolve, in
`RecordTurnContextStructured` (`pkg/engine/timeline.go`) or immediately before
it, so the award is part of the same record:

```go
// pkg/rules
func EarnFromTurn(spec *core.AdvancementSpec, turn *harness.Turn) int
```

- `on: miss` — a check whose outcome is the system's failure outcome (the last
  declared outcome, or a `miss`/`fail` label).
- `on: check_outcome` — matches the system's vocabulary (`outcome: strong`).
- `on: turn_end` — once per turn.
- `on: hook` — the engine calls nothing; `mechanics.js` calls `grantXP(amount)`.

`grantXP` is a new host binding alongside `setStat`
(`pkg/rules/js_engine.go`), writing the currency through the same state path:
`player.state.xp += amount`.

Awarding applies through `rules.ApplyStateChanges` (op `add` on the currency
stat), so the entity's frontmatter and the index stay consistent, and a memory
of kind `advancement` is recorded (reusing `writeMechanicalMemories`' shape,
`timeline.go:560-598`).

### 3.3 Spending

Modes:

- **spend** — the player picks an unlock whose `cost <= currency`, whose
  `requires` are met (owned unlock ids or entity tags), and whose gate is open.
- **track** — each earn ticks the track; when it reaches `track_size` the track
  clears and the mode behaves like `spend` for one advancement.
- **threshold** — when the currency crosses `levels[i].at`, the level's effects
  auto-apply and the event is recorded; no player choice.

Applying an unlock:

```go
// pkg/rules
func ApplyUnlock(spec *core.AdvancementSpec, unlock core.UnlockSpec, store *storage.Store, playerID string) error
```

- Deduct `cost` (op `sub`) from the currency stat.
- Apply each effect: `stat_increase` (add, clamped by `max`), `set_stat`,
  `grant_tag` (append to the entity's `tags`), `hook` (call the system's
  `onAdvance(unlockID)`), so exotic systems have an escape hatch.
- Record an `advancement` memory naming the unlock.

The gate (`downtime`) is checked against a campaign setting the GM can set
(`settings.downtime: true`), so DW's "have downtime" is expressible without new
machinery.

### 3.4 API and DTO

```go
// pkg/gui
type AdvancementDTO struct {
    Currency    string      `json:"currency"`
    Label       string      `json:"label"`
    Value       int         `json:"value"`
    Mode        string      `json:"mode"`
    Track       *TrackDTO   `json:"track,omitempty"`   // filled, size
    Unlocks     []UnlockDTO `json:"unlocks,omitempty"` // id,label,cost,affordable,requires_met
    Pending     bool        `json:"pending"`           // a threshold level is reached
}
type UnlockDTO struct{ ID, Label string; Cost int; Affordable, RequiresMet bool }
```

`GameStateDTO` gains `Advancement *AdvancementDTO` (`pkg/gui/types.go`), so the
existing per-turn refresh carries it.

Route:

```
POST /api/game/{id}/advance  { "unlock_id": "stat-increase" }
  -> { advancement: AdvancementDTO, player: PlayerDTO }
```

A spend that is unaffordable, gated, or missing a requirement returns 400 with a
clear message and does not alter state.

### 3.5 UI

- **CharacterSheetDrawer** gains an "Advancement" section: the currency, the
  track (if any), and the unlock list with cost, a disabled state and reason
  when not affordable/meeting requirements, and a Spend button.
- **Notification dot** on the Character trigger (`frontend/src/App.tsx:504-512`)
  when `Advancement.Unlocks` has any affordable entry or `Pending` is true,
  using the existing dot style (`App.tsx:495`). Clicking opens the drawer.
- **Interaction with the engagement policy.** In `auto`, XP is granted silently
  and the dot appears when spendable; the GM may narrate a milestone. In `ask`,
  the GM (or a prompt nudge) proposes the spend and the player confirms in the
  drawer. `off` leaves advancement dormant.

## 4. Data Flow

```
turn resolves
  → checks
  → EarnFromTurn(spec, turn) → ApplyStateChanges(add currency) → advancement memory
  → grantXP(amount) from mechanics.js → same path
  → track tick or threshold check
  → GameStateDTO.Advancement computed on read (affordability, requirements)
  → player POSTs /advance → ApplyUnlock (deduct + effects) → memory → refreshed DTO
```

## 5. Error Handling

- A system with no `advancement` block awards nothing and shows no UI.
- An earn rule naming an unknown `on` value is ignored (and logged once).
- A spend with insufficient currency, a closed gate, or an unmet requirement is
  refused with a specific reason; nothing is written.
- A `hook` effect whose JS hook is missing is refused, not skipped silently.
- A currency stat that is not declared is treated as freeform and allowed, so a
  schema-light system still works.

## 6. Testing & Verification

Go (stdlib `testing`, `t.TempDir()`):

- `pkg/rules`: `EarnFromTurn` for `miss`, `check_outcome`, and `turn_end`;
  `ApplyUnlock` deducts and applies each effect type; a `stat_increase` honours
  `max`; a `grant_tag` appends; a missing hook errors.
- `pkg/rules`: `grantXP` from a `mechanics.js` fixture increments the currency.
- `pkg/engine`: a turn with a failure outcome awards XP and records an
  `advancement` memory; a threshold level auto-applies on crossing.
- `pkg/gui`: `GET game state` computes `Advancement` affordability and
  requirements; `POST /advance` applies a valid unlock and rejects an
  unaffordable/gated one with 400; the entity frontmatter reflects the change.
- `pkg/config`/`core`: an `advancement` block round-trips; a system without it
  yields a nil spec and no behaviour change.

Frontend: `tsc`; the drawer section and dot are checked by inspection.

## 7. Compatibility & Rollout

- Purely additive: `MechanicsSpec.Advancement` is nil by default.
- The currency rides entity `state`, so it is visible in the existing sheet's
  attributes grid even before the new section lands.
- `GameStateDTO.Advancement` is optional; older clients ignore it.
- `grantXP` is a new host binding; existing `mechanics.js` scripts are
  unaffected.

## 8. Open Questions

- Currency as a `state` stat versus a dedicated `advancement:` frontmatter
  section (so the GM's `state_changes` cannot spend it).
- Shared ledgers: Blades' crew track and Ironsworn's shared vows — separate
  entity, separate currency, or defer?
- Should `threshold` levels also be spendable choices (some 5e variants offer
  options at level-up) or always auto-apply?
- Does `ask` mode need a GM-proposed spend as a first-class turn artifact (like
  a pending check), or is a prompt nudge plus the drawer enough?
- How should refunds/respecs work, if at all?

## 9. References

- Research: `docs/proposals/2026-09-26-mechanics-engagement-and-advancement-research.md`
- Code: `pkg/core/mechanics.go:6-52`; `pkg/entity/entity.go:41,62,236-237`;
  `pkg/harness/turn.go:54-60`; `pkg/engine/submission.go:130-136`;
  `pkg/rules/state_changes.go:14-55`; `pkg/rules/js_engine.go:45-158`;
  `pkg/engine/timeline.go:560-598`; `pkg/gui/types.go:16-23,89`;
  `pkg/gui/service.go:977,1630-1635`;
  `frontend/src/components/CharacterSheetDrawer.tsx:12-61`;
  `frontend/src/App.tsx:237,495,504-512`
- Advancement rules (primary): Dungeon World SRD — https://www.dungeonworldsrd.com/moves/ ;
  Ironsworn SRD (Experience/Advance) — https://github.com/Obsidian-TTRPG-Community/Ironsworn-SRD-Markdown ;
  Blades in the Dark — https://bladesinthedark.com/advancement ;
  D&D 5e SRD — https://5thsrd.org/rules/leveling_up/
