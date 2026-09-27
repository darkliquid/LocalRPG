# Mechanics engagement, visibility, and advancement research

Date: 2026-09-26
Status: research + recommendations (no code changed)

## Question

Mechanics still feel under-engaged, and whatever engagement happens is not
visible enough. How should the system prompt, response processing, and the
chronicle change to make rolls frequent, visible, and tunable — and how do we
add player advancement when different systems advance in wildly different ways?

## TL;DR

- Engagement is **constant but unstructured**: the mechanics instruction is
  always in the prompt and never varies, there is no cadence knob, and nothing
  in response processing forces a check. The GM is free to answer in prose every
  turn (`pkg/harness/mechanics_instructions.go:14-41`; no `mechanic`/`check`/
  `roll` config exists).
- Visibility exists but is **conditional**: checks render inline only when a
  segment carries `check_ref`, and unattached checks are appended after the
  prose (`frontend/src/components/TurnSegments.tsx:94-109`). There is no
  summary of what mechanics ran, and no "pending roll" affordance.
- There is **no advancement system at all** (`pkg/` has no XP/level/progression;
  the character sheet's `level` is just an arbitrary state value,
  `frontend/src/components/CharacterSheetDrawer.tsx:14`). The materials to build
  one already exist: entity `state` frontmatter, declared stats, state changes,
  `mechanics.js` hooks, and the character drawer.
- The engagement ladder the user described maps cleanly onto a policy setting
  (off / automatic / ask) with three prompt variants, an enforcement rule per
  level, and a two-phase turn for "ask".
- Advancement decomposes into a small, schema-agnostic model — an earned
  currency plus a catalog of unlocks — that covers PbtA, Ironsworn, Blades, and
  5e without hardcoding any of them.

## Method

- Traced the current mechanics path end to end (prompt injection, the check
  pipeline, submission validation, persistence, and rendering) with file:line
  citations.
- Read primary advancement rules: Dungeon World/PbtA, Ironsworn, Blades in the
  Dark, D&D 5e (SRDs).
- Read provider primary docs on forcing tool calls (OpenAI `tool_choice`,
  Gemini `FunctionCallingConfig`, Anthropic `tool_choice`).

## Part A: What exists today

**Prompt.** `harness.FormatMechanicsInstructions` emits a fixed block:
"Call `request_check` when an action is uncertain and failure would change the
story… NPCs do not roll…" (`pkg/harness/mechanics_instructions.go:16-20`), and
appends the system's declared notation, outcomes, and difficulties
(`:25-41`). `orchestrator.LoadPrompts` builds it whenever a system ships
`mechanics.js` or a `mechanics` block (`pkg/engine/orchestrator.go:388-409`),
and it is injected as the `mechanics` section (`pkg/harness/context.go:89-91,
265,322-327`). The system's `rules.md` is also sent as the provider system
message (`orchestrator.go:1304,1358`).

**Pipeline.** `request_check` and `submit_turn` are the turn tools
(`pkg/harness/turn_tools.go:21-78`). A check is resolved by the rules engine or
`defaultCheckResolver` (`pkg/engine/check_resolver.go:16-41`, pass at total ≥ 8
on `2d6`), or by a system `onCheck` resolver over `SchemaResolver`
(`pkg/rules/js_engine.go:263-293`, `pkg/rules/resolver.go:23-64`).
`validateSubmission` enforces: `uncertain` ⇒ at least one resolved check;
`impossible` ⇒ none; every `check_ref` must name a check; state changes must be
declared stats; a player `ProposedCheck` must be resolved or dismissed
(`pkg/engine/submission.go:101-161`). Two validation failures fall back to
prose (`orchestrator.go:1543-1551`).

**Visibility.** `Turn.Checks` persists (`pkg/engine/history.go:56`,
`timeline.go:510-513`), reaches the DTO (`pkg/gui/types.go:89`,
`service.go:977`), and renders inline before the matching segment or after the
prose (`TurnSegments.tsx:94-115`, `DiceCheckCard.tsx`).

**Tunables that exist:** `tool_rounds`, `tool_result_chars`, `action_echo`,
`continuity_checks`, `completion.*`, and system-level `mechanics` (stats,
skills, health, checks, `allow_freeform_state`). **Hardcoded:** the instruction
text, its ship-gating, the pass threshold and `2d6` fallback, and the fact that
mechanics are always on with no cadence knob.

## Part B: Why engagement still reads as low

1. **The instruction is generic and static.** It says when to roll but gives the
   model no reason to prefer a roll over narration, no turn-level pressure, and
   no cadence. A model that labels an action `automatic` never rolls
   (`submission.go:110-117` only forces a check when it *chooses* `uncertain`).
2. **Nothing forces a check structurally.** The only hard requirements are
   conditional on the model's own verdict. There is no "at least one check per N
   turns", no stakes heuristic, and no tool-call forcing.
3. **Prose fallback is a silent off-ramp.** Two validation failures drop to
   prose with no checks at all (`orchestrator.go:1546-1551`).
4. **Visibility is easy to miss.** A check with no `check_ref` is appended after
   all narration, so the cause/effect ordering is lost, and there is no turn
   summary of what mechanics ran.
5. **No reward loop.** Rolls resolve outcomes but grant nothing, so the player
   never feels a roll "count".

## Part C: The engagement ladder

Three levels, matching the user's ask, expressed as one policy:

| Level | Meaning | Prompt | Enforcement |
|---|---|---|---|
| `off` | Never use mechanics | "Do not roll or call `request_check`; narrate consequences directly." | `request_check` disabled; a submitted check is rejected |
| `auto` (default) | Roll automatically when consequences are strong | Current block, sharpened: "resolve with `request_check` *before* narrating whenever an outcome could cost or grant something the player would care about" | `uncertain` ⇒ check required (existing); `off`/`ask` modes gate |
| `ask` | Ask the player to roll, then wait | "When an action has a chance of consequences, `propose_check` and stop; do not resolve it." | A turn may end with a pending check; the next turn is the player's roll |

**Where the policy lives.** A system declares its default in `system.yaml`; a
campaign overrides it in `game.yaml`; a global default in config. Resolution
order: campaign → system → config default (`auto`). Concretely
`mechanics.engagement: off|auto|ask` in `MechanicsSpec`, plus `game.yaml`
`settings.mechanics_engagement`, plus a config accessor mirroring `ToolRounds()`.

**Prompt crafting.** Replace the fixed text with a policy-specific block built
from the same inputs. `auto` cites the system's outcomes/difficulties and adds
the sharpened "consequences" wording; `ask` describes the propose-then-wait
protocol and its UI; `off` says mechanics are disabled.

**Response structure.** For `auto`, keep the existing enforcement and add a
deterministic floor: after N consecutive turns with no check (default 3, itself
tunable), the engine nudges the prompt and, if tools are available, requires a
check via provider tool-call forcing — `tool_choice: required` (OpenAI),
`mode: ANY` (Gemini), `tool_choice: {type:"tool",...}` (Anthropic), which all
guarantee a call rather than leaving it optional. This is the mechanism the
provider docs describe for exactly this "model under-calls" problem
(sources in §9).

**Two-phase turn for `ask`.** `propose_check` (new tool) records a pending
check on the turn instead of resolving it. The turn is persisted with a pending
state and the chronicle renders a **Roll** button. Pressing it runs a second,
cheap generation that resolves the roll and hands the result back to the GM to
adjudicate (the existing `Roll` mode path is the seed: `orchestrator.go:565-574`
already builds a `ProposedCheck`). This is the largest new piece of structure
and should be scoped as its own design.

## Part D: Making mechanics visible

- **Always render a check with its narration.** Keep the inline placement when a
  `check_ref` exists; when it does not, place checks *before* the prose block
  that follows the action rather than after all narration, so cause precedes
  effect.
- **Show the outcome, not just the dice.** `DiceCheckCard` already tone-colours
  pass/partial/fail; add the stakes line and the system's outcome label
  (`CheckResult.Outcome`) so a "weak hit" reads as a fiction consequence.
- **A per-turn mechanics strip.** A one-line chip above the action console:
  "Checks this turn: 1 (2d6 → 8, weak hit)" with a count when zero, and the
  engagement level, so the player can see the system at work even on quiet
  turns.
- **Pending rolls** (in `ask`) are a distinct, prominent card with the stakes
  and a Roll button, plus a dot on the Character/Chronicle affordance.
- **A header indicator** for "mechanics on / off / asking", extending the
  existing status-dot pattern (`frontend/src/App.tsx:495`).

## Part E: Advancement

**Common data model (from the SRDs).** Every system is an earned currency plus a
catalog of unlocks, differing in earn triggers, cost model, and timing:

| System | Earned | Spent on | Cost model |
|---|---|---|---|
| Dungeon World | 6− roll; end-of-session questions | advanced moves, +1 stat (cap 18) | lump sum (level+7), gated by downtime |
| Ironsworn | fulfilling a vow (1-5 by rank) | new asset (3) / upgrade (2) | fixed per item |
| Blades | desperate rolls; XP triggers; downtime training | special ability / +1 action dot (crew too) | fill a 6-box track then advance |
| D&D 5e | encounter XP or milestones | the level itself (features, ASI, HP) | cumulative threshold table |

**Proposed schema-agnostic model.** A declarative `advancement` block in
`system.yaml`, plus a JS hook for anything exotic:

```yaml
advancement:
  currency: { stat: xp, label: Experience }
  earn:
    - { on: miss,              amount: 1 }        # engine-recognised trigger
    - { on: check_outcome, outcome: strong, amount: 2 }
    - { on: hook }                                 # mechanics.js grantXP(amount)
  mode: spend        # spend | track | threshold
  unlocks:
    - { id: stat-increase, label: "Increase a stat", cost: 5,
        effect: { type: stat_increase, amount: 1, max: 18 } }
    - { id: new-asset, label: "New asset", cost: 3,
        effect: { type: hook, hook: onAcquireAsset } }
  levels:            # mode: threshold only
    - { at: 300, label: "Level 2" }
```

- **Earn triggers** are engine-recognised events (`miss`, `check_outcome`,
  `turn_end`, `scene_end`) so a system does not need JS for the common cases;
  `mechanics.js` gains a `grantXP(amount)` host call mapped to the currency stat.
- **Spend effects** are a tiny closed set (`stat_increase`, `set_stat`,
  `grant_tag`, `hook`), enough for DW/Ironsworn/Blades, with a hook escape hatch.
- **Modes** cover the cost-model spread: `spend` (fixed-cost list), `track`
  (fill N then clear and choose), `threshold` (auto-apply on a table).
- **Timing gates** (DW's downtime) and caps are fields on the unlock; the engine
  refuses a spend whose gate is not met.

**Applying it.** A spend is just an authenticated state change: reuse
`ApplyStateChanges`/`StateChangeDecl` for the currency and the stat effects, run
the unlock's hook for anything else, and record an `advanced` memory so the
chronicle can mention it. A new route `POST /api/game/{id}/advance` takes an
unlock id.

**Exposing it.** List affordable unlocks in `CharacterSheetDrawer` with a Spend
button; put a notification dot on the Character trigger button
(`frontend/src/App.tsx:504-512`) when anything is affordable — the refresh hook
`refreshCorpus()` (`App.tsx:237`) already runs after each turn, so the
affordability signal rides the existing game-state payload.

**Auto vs ask.** In `auto`, XP is granted silently and the dot appears when it
can be spent; the GM may also narrate a milestone. In `ask`, the GM proposes a
spend and the player confirms. In all modes the engine never invents a cost:
no catalog, no spend.

## Recommendations (phased)

1. **Add the engagement policy** (`off|auto|ask`) with resolution order campaign
   → system → config, and make the prompt policy-specific. (Smallest change
   that makes engagement tunable.)
2. **Force the floor in `auto`**: a configurable cadence nudge plus provider
   tool-call forcing so a check truly happens when the model would otherwise
   skip it.
3. **Improve rendering**: stakes and outcome labels on `DiceCheckCard`, checks
   placed before their narration, a per-turn mechanics strip, and an
   engagement indicator in the header.
4. **`ask` mode**: `propose_check`, pending checks persisted on a turn, a Roll
   affordance, and the two-phase adjudication generation. Scope as its own
   design.
5. **Advancement**: the declarative block, `grantXP`, the spend/advance API, the
   character-drawer spend list, and the notification dot. Scope as its own
   design; it is independent of 1-4.

## Open questions

- Should the engagement policy be per campaign only, or also per role (e.g. the
  narrator never rolls, the GM does)?
- For `ask`, does the player's roll run the system's `onCheck` resolver, or a
  plain `EvaluateRoll`? The resolver path keeps system semantics.
- Does `auto` cadence forcing risk railroading stakes the fiction does not
  support? A cap ("at most one forced check per turn") and a fiction-quality
  prompt may be needed.
- Advancement currency as an entity `state` stat keeps everything in one place,
  but a currency that must not be spent by the GM suggests a separate
  `advancement` frontmatter section.
- Blades' crew track and Ironsworn's shared vows are non-character ledgers — do
  we model them now or defer?

## Sources

Repo (primary): `pkg/harness/mechanics_instructions.go:14-41`;
`pkg/engine/orchestrator.go:388-409,565-588,1304,1358,1512-1536,1543-1551`;
`pkg/harness/context.go:89-91,265,322-327`; `pkg/harness/turn_tools.go:21-78`;
`pkg/harness/turn.go:29,54-60,63-72,84-119`; `pkg/engine/submission.go:101-161`;
`pkg/engine/check_resolver.go:16-41`; `pkg/rules/js_engine.go:263-293`;
`pkg/rules/resolver.go:23-64`; `pkg/rules/state_changes.go:14-55`;
`pkg/entity/entity.go:41,62,236-237`; `pkg/engine/history.go:56`;
`pkg/engine/timeline.go:510-513,560-598`; `pkg/gui/types.go:16-23,61,89`;
`pkg/gui/service.go:977,1262-1285,1630-1635`;
`frontend/src/components/TurnSegments.tsx:94-115`;
`frontend/src/components/DiceCheckCard.tsx`;
`frontend/src/components/CharacterSheetDrawer.tsx:12-61`;
`frontend/src/App.tsx:237,483-569,673`; `pkg/config/types.go:51-98,562-627,646-659`.

Advancement (primary rules texts):
- Dungeon World SRD — https://www.dungeonworldsrd.com/playing-the-game/ and https://www.dungeonworldsrd.com/moves/
- Ironsworn SRD (Experience/Advance/Fulfill Your Vow/Assets) — https://github.com/Obsidian-TTRPG-Community/Ironsworn-SRD-Markdown
- Blades in the Dark advancement — https://bladesinthedark.com/advancement
- D&D 5e SRD leveling — https://5thsrd.org/rules/leveling_up/

Tool-call forcing (primary vendor docs):
- OpenAI function calling / `tool_choice` — https://platform.openai.com/docs/guides/function-calling
- Gemini function calling / `FunctionCallingConfig` — https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/models/function-calling
- Anthropic tool use / `tool_choice` — https://platform.claude.com/docs/en/agents-and-tools/tool-use/define-tools and https://platform.claude.com/docs/en/agents-and-tools/tool-use/overview
