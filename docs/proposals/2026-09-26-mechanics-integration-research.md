# Mechanics & dice integration research

Date: 2026-09-26
Status: research + recommendations (no code changed)

## Question

Dice rolls, checks, and system mechanics appear far more rarely during play
than the fiction seems to call for. Why, and how can a system's mechanics be
introduced and integrated better?

## TL;DR

The dominant cause is a **bug**, not a design choice: the GUI loads a
campaign's `mechanics.js` into a fresh per-turn JS VM only once, so from turn
two onward the hooks never run. On top of that, only the `do` mode reaches an
action hook, `Roll` mode is an advisory suggestion the model can ignore,
`request_check` needs a tool-calling provider and is never asked for by the
prompt, and a prose-only turn produces no checks at all.

Every rules text consulted agrees on one design rule the engine is missing: a
roll is a **triggered move**. When the fictional action matches a trigger, the
move happens and dice are rolled; the mechanic is not opt-in per turn.

## Method

- Read the engine's turn pipeline, rules runtime, prompt assembly, submission
  validation, and GUI turn preparation in this repo (paths and lines below).
- Read the local (gitignored) `systems/narrative_2d6/` definition directly;
  note that `glob`/`grep` skip it because `/systems/` is in `.gitignore`
  (`AGENTS.md` says `systems/` is empty, which is only true of the checkout).
- Consulted primary rules texts for PbtA/Dungeon World, Ironsworn, Blades in
  the Dark, and D&D 5e (2014 and 2024) on when a roll is called for.

## How a roll happens today

Decision points, in order:

1. **`ProcessActionStream`** is the pipeline (`pkg/engine/orchestrator.go:407`);
   `ProcessAction` wraps it (`:984`). `rollRes` is assigned exactly once, at
   `:568`.
2. **Mode dispatch** (`pkg/engine/orchestrator.go:538-574`):
   - `Opening` — no mechanics (`:538-545`).
   - `/gm` correction — directive only (`:548-551`).
   - `Roll` — becomes a string, `[PROPOSED CHECK: %s by %s]`, and is **not
     executed** (`:552-560`).
   - Everything else with a rules engine — `o.rulesEngine.ExecuteAction(
     strings.ToLower(mode), …)` (`:561-574`). This is the only path that runs a
     JS action hook and produces a roll.
3. **Model-requested checks** — the `request_check` tool is handled in the
   generation loop (`pkg/engine/orchestrator.go:1484-1506`), resolved by
   `o.resolveCheck` (`:165-181`; the resolver is the rules engine, `:237-239`),
   and only reach the turn through a successful `submit_turn`
   (`:1507-1530`, `:807`). The tool is described at
   `pkg/harness/turn_tools.go:66-78`.
4. **Tools are optional.** `offersTools` returns `isCaller` for the default
   `auto` capability (`pkg/engine/orchestrator.go:201-213`); `canCallTools`
   gates attachment (`:1253`, `:1363-1373`). Only `openaichat`
   (`pkg/provider/openaichat/http.go:79`) and conditionally Gemini
   (`pkg/provider/geminillm/provider.go:251`) report tool-calling capability.
5. **Validation** (`pkg/engine/submission.go:100-143`): an `uncertain`
   feasibility verdict with no resolved check is rejected (`:108-112`), but an
   `automatic` verdict needs no check, a `dismissed_checks` entry only needs a
   non-empty reason (`:137-141`), and a player's `[PROPOSED CHECK]` is never
   checked for resolution.

### What a system may hook

`mechanics.js` runs through `bindHostAPI` (`pkg/rules/js_engine.go:45-158`):
`onAction(actionType, fn)` (`:103-111`), `onTurnBegin` (`:131-138`),
`onTurnEnd` (`:113-120`), `onWorldTick` (`:122-129`), `onCheck` (`:140-148`),
`onHealthZero` (`:150-157`), plus `roll`, `getStat`/`setStat`,
`getLocation`/`setLocation`, `injectGMDirection`, and `log`. `ExecuteAction`
returns `nil, nil` when no handler matches a mode (`:184-187`), so an untouched
mode silently does nothing. Check resolution prefers a system `onCheck`
resolver for the kind and otherwise falls back to `SchemaResolver`
(`:263-293`; `pkg/rules/resolver.go:23-64`). Loading order is manifest, then
`systems/<id>/mechanics.js`, then world `hooks.js`
(`pkg/rules/loader.go:23-49`).

## Root causes of the low frequency

### 1. The per-turn JS VM is never re-loaded (the big one)

`prepareTurn` builds a **new** `JSEngine` every turn (`pkg/gui/service.go:1190`)
because a cached orchestrator would miss settings and note edits — a deliberate
choice documented at `pkg/gui/service.go:1145-1147`. But `LoadRules` is guarded
by `s.rulesLoaded[manifest.ID]` (`:1194-1205`), a flag set to `true` and never
cleared (`pkg/gui/service.go:59-62,120,1195,1203`).

So: turn one loads hooks into an engine that then has them; every later turn
gets an empty VM. `ExecuteAction` finds no handler (`js_engine.go:184-187`),
`onCheck` resolvers are absent (checks fall back to the default resolver), and
`onTurnBegin`/`onTurnEnd` never fire again. This is why mechanics "work" on the
first turn and then effectively disappear. The TUI does not have this bug: it
builds one engine for the whole session and loads once
(`cmd/localrpg/play.go:104-107`).

### 2. Mode coverage is narrow

`ExecuteAction` is called with the lowercased client mode
(`orchestrator.go:563`). Clients send `do`, `say`, `story`, `roll`, `gm`
(`pkg/gui/types.go:422-426`, `frontend/src/components/ActionConsole.tsx:21`,
`pkg/tui/app.go:20,36`). The bundled reference registers only `"do"` and
`"attack"` (`frontend/src/templates/referenceTemplates.ts:60,81`) and the local
`systems/narrative_2d6/mechanics.js` registers `"do"` and `"attack"`.
`attack` is not reachable from any client mode, and `say`/`story` have no hook.
Only `Do` turns can ever roll.

### 3. `Roll` mode is advice, not execution

`orchestrator.go:552-560` turns a player roll into `[PROPOSED CHECK: …]` and
nothing enforces it. `validateSubmission` never verifies that a proposed check
was resolved or dismissed, so the model may drop it silently.

### 4. Checks depend on a tool-calling provider

`request_check` only exists when tools are offered, which for the default
`auto` capability means the provider must report tool-calling support
(`orchestrator.go:201-213`, `config/types.go:638-647`). Local CLI providers
cannot call tools and therefore can never emit a check, no matter how risky the
fiction is.

### 5. The model is never told to propose checks

Other than the `request_check` description (`turn_tools.go:66-78`), no section
of the assembled prompt asks the GM to look for risk and call a check. Context
sections are listed in `pkg/harness/context.go:260-273`; `rulesSection`
(`:311-316`) only wraps the author's `rules.md`. The local `rules.md` even tells
the GM that hooks run automatically and only explains `[MECHANICS RESULT]`, so
there is no instruction that would drive `request_check`.

### 6. Prose turns produce nothing

Checks live on `result.Submission` (`orchestrator.go:807,1527`). If the model
answers in prose without `submit_turn`, `result.Submission` is nil (`:737`) and
there are no checks; the fallback after repeated validation failure also nulls
the submission (`:1516-1521`).

## What the rules texts require (primary sources)

- **Apocalypse World / Dungeon World — "to do it, do it":** every move has an
  explicit fictional trigger ("when you…"); when the player does that thing,
  the move happens and dice are rolled. "A character can't take the fictional
  action that triggers a move without that move occurring." Moves are named by
  the GM from the fiction, and many moves need no dice.
- **Ironsworn — fiction first, then move:** moves are triggered by
  "When you…" phrasing and you roll only when a move says so. "Make Moves
  Matter": every roll, hit or miss, must change the situation. NPCs never make
  moves or roll. *Ask the Oracle* covers questions, NPC reactions, and events
  (odds table: Almost Certain 11+, Likely 26+, 50/50 51+, Unlikely 76+, Small
  Chance 91+).
- **Blades in the Dark:** roll when the PC is "put to the test" and the action
  is challenging; set **position** (Controlled/Risky/Desperate, default Risky)
  and **effect** (Limited/Standard/Great) before rolling. NPCs do not roll —
  a single PC action roll does "double duty". Partial success (4/5) is success
  *with* a consequence, so the fiction never stalls. Consequence economy
  (stress, resistance) keeps every roll a decision.
- **D&D 5e (2024):** call for a test only when success and failure are both
  possible and failure has **meaningful consequences**; skip trivial or
  impossible tasks and tasks where retrying is free.

The common rule: **a roll is triggered by a clearly defined fictional
condition, and only when failure matters.** The current engine has neither the
trigger layer nor the "failure matters" enforcement; it has hooks the model
must opt into.

## Recommendations

### A. Fix the per-turn rules load (do this first)

The VM is new each turn, so the fix is to load rules into it each turn rather
than once per campaign. Either:

- remove the `rulesLoaded` guard and call `LoadRules` every turn
  (`pkg/gui/service.go:1194-1205`) — cheap, and it preserves the "fresh
  orchestrator each turn" property, or
- cache the `JSEngine` (and only the engine) per campaign and invalidate it
  when `system.yaml`/`mechanics.js`/`hooks.js` change.

Either way, add a regression test that a **second** GUI turn still fires
`onAction`/`onTurnEnd`.

### B. Add a trigger/adjudication layer in the engine

- Add an explicit prompt section (alongside `rulesSection`,
  `pkg/harness/context.go:260-273`) instructing the GM to call `request_check`
  when an action is uncertain or consequential, and to state stakes first.
- Fix mode coverage so a `do` action always reaches the system hook, and map
  `say`/`story` to hooks where the system defines them (the reference and
  `narrative_2d6` should register `say`/`story` too).
- Enforce outcomes in `validateSubmission`: require that a submitted turn with
  `verdict.feasibility == uncertain` resolves a check (already true) **and**
  that any player `[PROPOSED CHECK]` is resolved or explicitly dismissed.
- Consider a deterministic fallback: if the model marks an action `uncertain`
  but emits no `request_check` (e.g. a non-tool provider), the engine resolves
  a check itself using the system's declared convention.

### C. Make checks provider-independent

Tool-calling capability should not be a prerequisite for mechanics. Options:
allow the engine to synthesize a check from the submission's verdict, add a
`propose_check` tool to the query tool set (`pkg/harness/tools.go:31-106`), or
relax the default `gm` `supports_tools` value (`pkg/config/types.go:34-36,
638-647`).

### D. System-author guidance (no Go changes)

- Register `onAction` for the modes clients actually send (`do`, `say`,
  `story`) and give each a 2d6/position-effect style outcome; `attack` is
  unreachable today (`systems/narrative_2d6/mechanics.js`).
- Declare `mechanics.checks` notation/outcome/difficulty in `system.yaml`
  (`pkg/core/mechanics.go:40-52`) and add `onCheck` resolvers per kind
  (`pkg/rules/js_engine.go:140-148`).
- Put explicit "call `request_check` when…" language in `prompts/rules.md`;
  it is injected as the first context section (`context.go:261,311-316`).
- Use `onTurnBegin`/`onTurnEnd` (`js_engine.go:113-138`) and
  `injectGMDirection` (`:91-95`) to push periodic stakes or complications.

### E. Pacing controls

To raise frequency deliberately: a config cadence (for example "at least one
check every N turns of `do` action"), and NPC "double duty" so obstacles are
resolved by the protagonist's roll rather than narrated away. Both mirror
Blades and Ironsworn, and both keep rolls meaningful instead of automatic.

## How to validate

- Regression test: create a campaign, run turn 1 and turn 2 in `Do`; assert
  both turns carry a mechanics result and that `onTurnEnd` ran on turn 2.
- Unit test in `pkg/engine`: build a fresh engine, `LoadRules`, then assert
  `ExecuteAction("do", …)` returns a roll; and assert `ExecuteAction` on an
  unloaded engine returns nil (documents the bug).
- Integration: a submission with `verdict.feasibility == uncertain` and no
  check is already rejected; extend the test to cover a dropped
  `[PROPOSED CHECK]`.
- Manual: play three `Do` turns against `systems/narrative_2d6` and confirm
  mechanics results appear each turn.

## Open questions

- Should a `do` turn always roll, or only when the system's hook decides the
  action is risky? PbtA says only on a trigger; a always-roll default trades
  fidelity for frequency.
- Do we want position/effect as first-class data on `CheckRequest` so the GM
  must negotiate stakes before a roll?
- Should `Roll` mode bypass the model and execute immediately, then hand the
  result to the GM (making the explicit roll button authoritative)?

## Sources

Repo (primary):

- `pkg/engine/orchestrator.go:407,431,538-574,165-181,201-213,807,984,
  1484-1530`
- `pkg/engine/submission.go:100-143`
- `pkg/harness/turn_tools.go:23-78`; `pkg/harness/tools.go:31-106`
- `pkg/harness/context.go:260-273,311-316`
- `pkg/rules/js_engine.go:45-158,184-187,263-293`;
  `pkg/rules/host_api.go:25-41`; `pkg/rules/resolver.go:23-64`;
  `pkg/rules/loader.go:23-49`; `pkg/rules/dice.go:16-33`
- `pkg/core/mechanics.go:6-52`; `pkg/core/types.go:11-40`
- `pkg/config/types.go:34-36,84-97,638-663`
- `pkg/gui/service.go:59-62,1145-1205`; `pkg/gui/types.go:422-457`
- `pkg/tui/app.go:20,36`; `cmd/localrpg/play.go:104-107`
- `systems/narrative_2d6/mechanics.js`, `systems/narrative_2d6/system.yaml`,
  `systems/narrative_2d6/prompts/rules.md` (local, gitignored)
- `frontend/src/templates/referenceTemplates.ts:29-109`;
  `frontend/src/components/ActionConsole.tsx:21`

External (primary rules texts):

- Ironsworn SRD — https://tedtschopp.github.io/Ironsworn-SRD/Ironsworn%20SRD.html
- Dungeon World SRD, Playing the Game — https://www.dungeonworldsrd.com/playing-the-game/
- Dungeon World GM chapter — https://exposit.github.io/dw-srd/dw_gm.html
- Blades in the Dark action roll — https://bladesinthedark.com/action-roll
- Blades position & effect — https://bladesinthedark.com/setting-position-effect
- D&D 5e 2014 ability checks — https://www.dndbeyond.com/sources/dnd/basic-rules-2014/using-ability-scores
- D&D 5e 2024 D20 tests — https://5e24srd.com/playing-the-game/d20-tests.html
