# Mechanics Trigger & Cadence Design

**Date:** 2026-09-26
**Status:** Proposed
**Scope:** Restore per-turn mechanics hooks, widen mode coverage, add a trigger/adjudication layer and check enforcement, and give systems an authoring contract
**Related:** Mechanics & dice integration research (`docs/proposals/2026-09-26-mechanics-integration-research.md`), Mechanics Engagement & Declarative Schema Design (2026-09-25), Structured Turn Protocol Design (2026-09-25), `pkg/rules`, `pkg/engine`, `pkg/harness`, `pkg/gui/service.go`, `cmd/localrpg/play.go`

## 1. Overview & Goals

The declarative-schema work (2026-09-25) is in place: `MechanicsSpec`,
`CheckConventions`, `onCheck`, `onTurnBegin`, `state_changes`, declared-stat
validation, and the GUI's `LoadRules` call all exist. The engine can resolve
and apply mechanics.

It just rarely runs them. `prepareTurn` builds a **new** `rules.JSEngine` every
turn (`pkg/gui/service.go:1190`) but loads rules only once per campaign
(`:1194-1205`, flag never cleared), so from turn two the VM is empty and every
hook is dead. Beyond that, only `Do` reaches an action hook, `Roll` is an
advisory string the model may ignore, checks require a tool-calling provider,
and nothing in the prompt asks the GM to look for risk.

This specification closes the frequency gap. It does not change the schema; it
changes the lifecycle, the mode contract, the trigger layer, and enforcement.

**Goals:**

- A campaign's `mechanics.js` is loaded into the engine that actually runs each
  turn, so `onAction`/`onCheck`/`onTurnBegin`/`onTurnEnd` fire on every turn.
- `Do`, `Say`, and `Story` all reach the rules engine; a system decides what
  each does by registering hooks for it.
- The GM is instructed, in engine-generated context, to propose a check when an
  action is uncertain and failure would matter, and enforce that a proposed
  check is resolved or dismissed.
- A check is possible regardless of whether the provider supports tool calls,
  via the system's action hook.
- System authors have a documented contract for which modes to hook and how to
  describe resolution.
- Optional: a cadence control that nudges a check after N turns without one.

**Non-Goals:**

- Hardcoding any rule system; the engine stays schema-agnostic.
- Position/effect as first-class data (Blades-style). Noted as a follow-up.
- A GUI editor for mechanics YAML.
- Replacing `EvaluateRoll` or the dice library.
- Persisting `mechanics.js` VM state across turns (see §4.1 for why).

**Success Criteria:**

- Playing two consecutive `Do` turns in the GUI produces a `[MECHANICS RESULT]`
  on both, and `onTurnEnd` fires on both.
- A system that registers `onAction("say", …)` rolls on a `Say` turn; a system
  that registers nothing for `say` behaves exactly as today.
- The assembled GM prompt contains an engine-generated mechanics instruction
  section when the loaded system declares mechanics.
- A `Roll` turn's proposed check is either resolved by `request_check` or
  explicitly dismissed; a submission that does neither is rejected.
- A non-tool provider playing a `Do` turn still gets a system action hook and
  its `[MECHANICS RESULT]`.
- `systems/narrative_2d6` no longer advertises an unreachable `attack` mode and
  hooks the modes a client actually sends.

## 2. Investigation Findings

See the research doc for the full citation list. Load-bearing facts:

- `prepareTurn` creates `rules.NewJSEngine(...)` each turn
  (`pkg/gui/service.go:1190`); `LoadRules` is guarded by
  `s.rulesLoaded[manifest.ID]` (`:1194-1205`), set once (`:1203`) and never
  cleared (`:59-62,120`).
- Mode dispatch (`pkg/engine/orchestrator.go:538-574`): `Opening`, `/gm`,
  `Roll` short-circuit; only the remainder calls
  `ExecuteAction(strings.ToLower(mode), …)` (`:561-574`). `rollRes` is assigned
  only at `:568`.
- `ExecuteAction` is a no-op when no handler matches (`pkg/rules/js_engine.go:184-187`).
- Clients send `do`, `say`, `story`, `roll`, `gm`
  (`pkg/gui/types.go:422-426`, `frontend/src/components/ActionConsole.tsx:21`,
  `pkg/tui/app.go:20,36`). The reference and `narrative_2d6` hook only `do` and
  `attack`.
- `Roll` becomes `[PROPOSED CHECK: …]` at `pkg/engine/orchestrator.go:560` and
  is never validated.
- `request_check` exists only when tools are offered
  (`orchestrator.go:201-213,1253,1363-1373`); tool support defaults to the
  provider's `auto` capability (`pkg/config/types.go:638-647`).
- No prompt section instructs the GM to propose checks; sections are listed at
  `pkg/harness/context.go:260-273`.
- Checks live on `result.Submission` (`orchestrator.go:807,1527`); a prose turn
  has none.
- `validateSubmission` (`pkg/engine/submission.go:100-143`) rejects an
  `uncertain` verdict with no check (`:108-112`) but never checks a proposed
  check, and `automatic` needs nothing.
- `ExecuteTurnEnd` receives a context dict (`orchestrator.go:960-978`);
  `ExecuteTurnBegin` receives turn and location (`:662-669`).

## 3. Design

### 3.1 D1 — Rules engine lifecycle (correctness fix, do first)

The engine the orchestrator uses must hold the loaded rules. Two options:

- **Option A (recommended): load every turn.** Delete the `rulesLoaded` guard
  and call `loader.LoadRules(manifest.SystemID, manifest.WorldID)` on each new
  `JSEngine` in `prepareTurn`. Cost is re-parsing one small script per turn.
  Keeps the deliberately-fresh-orchestrator property
  (`pkg/gui/service.go:1145-1147`).
- **Option B (future): cache the engine, not the orchestrator.** Keep one
  `*rules.JSEngine` per campaign in the service, invalidated when
  `system.yaml`, `mechanics.js`, or world `hooks.js` change. Avoids re-eval and
  would preserve module-level VM state, at the cost of more invalidation code.

Choose A now. Document that hooks must treat each turn as stateless and keep
durable state in entity frontmatter (the model already used by
`setStat`/`EntityWriter`, `pkg/rules/host_api.go`), because the VM is rebuilt
per turn.

`rulesLoaded` is removed, not merely cleared; nothing else reads it.

### 3.2 D2 — Mode contract

Define which modes reach the rules engine and stop treating unreachable ones as
real:

- `Do`, `Say`, `Story` each call `ExecuteAction(lower(mode))`. A system opts in
  per mode by registering `onAction`.
- `Roll` remains the player's explicit dice request (see D4), not an action
  hook.
- `GM`/`Opening` keep their special handling.
- `attack` is not a client mode. It stays callable from JS (a system may invoke
  it internally) but documentation and templates must stop presenting it as a
  player mode; sub-actions are modelled with a check's `stat`/`skill` instead.

No client changes are required; this only widens what the engine already
receives.

### 3.3 D3 — Trigger layer and GM guidance

The prompt is the only place the model learns to look for risk. Add an
engine-generated section, built when a rules engine is loaded:

`pkg/engine/orchestrator.go` composes a `mechanics_instructions` block from the
system's `mechanics.checks` (notation, outcome vocabulary, difficulty ladder)
and passes it to `harness.AssembleContext…` as a new field on the request
(alongside `RulesPrompt`/`LorePrompt`, `pkg/harness/context.go:75-100`), then
`context.go` renders it as a section next to `rulesSection`
(`context.go:260-273,311-316`).

Fixed instruction text (system-agnostic, schema cited at runtime):

> Call `request_check` when an action is uncertain and failure would change the
> story. State the stakes and the possible outcomes first. Do not roll for
> safe or trivial actions. NPCs do not roll; resolve opposition through the
> protagonist's check. Honour the result the engine returns.

This is the engine's expression of the common rules-text principle (only roll
when the outcome is uncertain and failure matters) and fits systems with no
declared mechanics too, because it is generated only when rules are loaded.

### 3.4 D4 — Enforce proposed checks

Replace the magic string for player rolls with structured data so the engine
can enforce it:

- Add `ProposedCheck *harness.ProposedCheck` (actor, description) to the
  generation request instead of embedding `[PROPOSED CHECK: …]` in a directive
  string (`orchestrator.go:560`). The prompt still renders a readable line.
- Extend `validateSubmission` (`pkg/engine/submission.go`): when a
  `ProposedCheck` is present and feasibility is not `impossible`, the submission
  must either resolve a check or carry a `dismissed_checks` entry whose ref
  matches. Today only a non-empty dismissal reason is required
  (`submission.go:137-141`); require the ref to correspond.
- Keep behaviour for `uncertain` (must resolve a check) and `impossible` (must
  not) unchanged.

This makes the explicit Roll button authoritative without bypassing the model:
the player asks, the GM must answer with a roll or a reason.

### 3.5 D5 — Provider-independent checks

Two distinct paths, both restored by D1/D2:

- **Non-tool providers** cannot emit `submit_turn`/`request_check`; their only
  mechanics come from the system action hook, which D1/D2 make run on every
  turn. This is the primary fix for local CLI providers.
- **Tool providers** use `request_check`; D3/D4 make them actually do it.

A deeper fallback (engine synthesises a check from a prose turn's uncertainty)
is out of scope and noted in Open Questions.

### 3.6 D6 — Optional cadence control (optional)

A config key under `agents`/mechanics — e.g. `mechanics_cadence_turns` — that,
when > 0, adds a "raise the stakes" nudge to the prompt after N consecutive
turns with no check. Default 0 (off). Mirrors Ironsworn's "if you've made the
same move three times, make something happen" and Blades' pacing guidance
without forcing rolls. Not required for the success criteria.

### 3.7 D7 — Authoring contract

Document in the systems-studio guidance and update bundled content:

- Register `onAction` for `do`, `say`, `story`; keep `attack` internal-only.
- Declare `mechanics.checks` in `system.yaml`; add `onCheck(kind, fn)` for
  kinds whose outcomes differ from the schema convention.
- Put trigger language in `prompts/rules.md`, but rely on D3 for the general
  instruction.
- Update `frontend/src/templates/referenceTemplates.ts` and
  `systems/narrative_2d6/mechanics.js` + `prompts/rules.md` accordingly.

## 4. Interfaces & Data Flow

### 4.1 Lifecycle

```
prepareTurn
  └─ NewJSEngine(host bridge)            // fresh each turn
     └─ RuleLoader.LoadRules(system, world)   // ALWAYS (was: once)
  └─ NewTurnOrchestrator(store, timeline, jsEngine, …)
```

Per turn: `ExecuteTurnBegin` → assemble context (incl. mechanics instructions)
→ `ExecuteAction(lower(mode))` for Do/Say/Story → generation
(`request_check` when tools; structured turn) → `validateSubmission` (incl.
proposed-check enforcement) → `ExecuteTurnEnd`.

### 4.2 New/changed types

- `pkg/harness` (context request): `MechanicsPrompt string` (engine-rendered
  instruction block).
- `pkg/harness`: `ProposedCheck{ Actor, Description string }`, carried on the
  generation request and mirrored in the rendered prompt.
- `pkg/rules`: no interface change (the schema and hooks already exist).
- `pkg/gui/service.go`: remove the `rulesLoaded` map and guard.

## 5. Error Handling

- `LoadRules` failure: log `rules.load_error` and continue with an empty VM, as
  today (`pkg/gui/service.go:1199-1201`). A missing system file must not fail a
  turn.
- Hook errors remain non-fatal and logged (`turn.begin_hook_error`,
  `turn.end_hook_error`, `orchestrator.go:660-668,974-976`).
- Proposed-check enforcement failure is a `submissionError`, which already
  feeds the existing retry/repair path.

## 6. Testing & Verification

Go (stdlib `testing`, `t.TempDir()`):

- `pkg/gui`: a fixture that registers an `onAction("do")` hook must produce a
  mechanics result on turn 2 as well as turn 1 (regression for D1).
- `pkg/engine`: after `LoadRules` on a fresh engine, `ExecuteAction("do", …)`
  returns a roll; on an unloaded engine it returns nil (documents the bug).
- `pkg/engine`: `ExecuteAction` is reached for `say` and `story` when hooked.
- `pkg/engine`: a submission with a `ProposedCheck` and neither a resolved nor
  a dismissed matching check is rejected; one with a matching dismissal passes.
- `pkg/harness`: the assembled prompt contains the mechanics instruction when a
  rules prompt/schema is supplied, and omits it otherwise.
- `systems/narrative_2d6`: a `Do`, `Say`, and `Story` turn each run their hook.

Frontend: no test runner; verify `ActionConsole` modes still map and `tsc`
passes.

Manual: three `Do` turns against `narrative_2d6` show a check each turn; a
`Roll` turn produces a check or a stated dismissal.

## 7. Compatibility & Rollout

- Systems that register no `say`/`story` hooks are unchanged.
- Removing `rulesLoaded` is behaviour-restoring; no data migration.
- `ProposedCheck` replaces a directive string; the rendered prompt text stays
  equivalent so model behaviour should hold.
- `attack` remains callable from JS; only its documentation changes.

## 8. Open Questions

- Should an engine-synthesised check be added for prose-only turns, or is the
  action-hook path sufficient?
- Should position/effect be promoted to `CheckRequest` so the GM must negotiate
  stakes before a roll?
- Is a cadence control (D6) wanted, and should it live in config or in
  `system.yaml`?
- If a system wants durable counters in JS, do we implement engine caching
  (D1 Option B) or a snapshot mechanism?

## 9. References

- Research: `docs/proposals/2026-09-26-mechanics-integration-research.md`
- Prior spec: `docs/superpowers/specs/2026-09-25-mechanics-engagement-and-declarative-schema-design.md`
- Structured turns: `docs/superpowers/specs/2026-09-25-structured-turn-protocol-design.md`
- Code: `pkg/gui/service.go:1145-1205`, `pkg/engine/orchestrator.go:538-574,662-672,960-978,1221-1530`,
  `pkg/engine/submission.go:100-143`, `pkg/harness/context.go:75-100,260-273,311-316`,
  `pkg/harness/turn_tools.go:23-78`, `pkg/rules/js_engine.go:45-158,184-187`,
  `pkg/rules/loader.go:23-49`, `pkg/core/mechanics.go:6-52`
