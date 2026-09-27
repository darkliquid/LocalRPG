# Mechanics Engagement & Visibility Design

**Date:** 2026-09-26
**Status:** Proposed
**Scope:** A tunable mechanics-engagement policy, policy-specific prompt crafting, a deterministic forcing floor, and making checks visible in the chronicle
**Related:** Mechanics engagement, visibility, and advancement research (`docs/proposals/2026-09-26-mechanics-engagement-and-advancement-research.md`), Mechanics Engagement & Declarative Schema Design (2026-09-25), Mechanics Trigger & Cadence Design (2026-09-26), `pkg/harness`, `pkg/engine`, `pkg/rules`, `pkg/gui`, `frontend/`

## 1. Overview & Goals

Mechanics are always on but unstructured. The instruction to the GM is a fixed
constant (`pkg/harness/mechanics_instructions.go:16-20`), there is no knob for
how often mechanics run, and nothing forces a check: `validateSubmission` only
requires one when the model itself marks the action `uncertain`
(`pkg/engine/submission.go:110-117`). Visibility is conditional too — a check
renders inline only when a segment carries `check_ref`, and unattached checks
are appended after all narration (`frontend/src/components/TurnSegments.tsx:94-115`).

This specification makes engagement **tunable** (`off` / `auto` / `ask`), makes
the prompt **reflect the policy**, adds a deterministic floor so `auto` really
rolls, and makes what happened **legible** in the chronicle.

**Goals:**

- One engagement policy with a resolution order (campaign → system → config),
  default `auto`.
- Prompt text generated per policy, not a constant.
- In `auto`, a configurable cadence that forces a check when the model has gone
  quiet, using provider tool-call forcing where available.
- In `ask`, the GM proposes a check and the turn ends pending; the player rolls
  and the GM adjudicates.
- In `off`, no checks are offered or accepted.
- Every resolved check is visible with its stakes and outcome, placed next to
  the narration it caused, and the turn says how much mechanics ran.

**Non-Goals:**

- Advancement/XP (separate design).
- Changing how a check is resolved (the system's `onCheck` or schema resolver
  keeps that).
- A GUI editor for the policy (config/system/campaign only for now).

**Success Criteria:**

- Switching the policy changes the prompt, the offered tools, and whether
  checks are accepted, with no other change.
- With `auto` and a model that answers in prose, a check still occurs at least
  once per `cadence_turns` turns.
- With `ask`, a turn can end with a pending check; the chronicle shows a Roll
  affordance; rolling it produces an adjudicated continuation.
- With `off`, a submitted check is rejected and `request_check` is not offered.
- A resolved check always appears with its stakes and outcome label, in causal
  position relative to the narration.

## 2. Investigation Findings

See the research note for the full map. Load-bearing facts:

- Instruction text and ship-gating are hardcoded
  (`pkg/harness/mechanics_instructions.go:14-41`;
  `pkg/engine/orchestrator.go:401-404`).
- No mechanics/check/roll cadence setting exists in `pkg/config`; only
  `continuity_checks` and `action_echo` relate to mechanics
  (`pkg/config/types.go:84-90`).
- `request_check`/`submit_turn` are the turn tools
  (`pkg/harness/turn_tools.go:21-78`); the loop resolves checks and feeds them
  back (`pkg/engine/orchestrator.go:1512-1536`).
- `validateSubmission` requires a check only for a self-declared `uncertain`
  verdict, and two failures fall back to prose (`submission.go:101-161`;
  `orchestrator.go:1546-1551`).
- Checks render inline by `check_ref`, else after the prose
  (`TurnSegments.tsx:94-115`; `DiceCheckCard.tsx`).
- Provider tool forcing is available: OpenAI `tool_choice: "required"`, Gemini
  `FunctionCallingConfig.mode: ANY`, Anthropic `tool_choice` (`any`/`tool`);
  our `ModelProvider.Stream`/`GenerateRequest` has no such field today
  (`pkg/harness/types.go:8-15,107-111`).

## 3. Design

### 3.1 The policy

```go
// pkg/core
type MechanicsSpec struct {
    // ...existing...
    // Engagement is the system's default mechanics policy.
    Engagement string `yaml:"engagement,omitempty"` // "off" | "auto" | "ask"
}
```

Resolution, highest first:

1. Campaign: `game.yaml` `settings.mechanics_engagement`.
2. System: `system.yaml` `mechanics.engagement`.
3. Config: `mechanics.engagement` (new block), default `auto`.

```go
// pkg/config
type MechanicsConfig struct {
    Engagement   string `yaml:"engagement,omitempty" json:"engagement,omitempty"`
    CadenceTurns int    `yaml:"cadence_turns,omitempty" json:"cadence_turns,omitempty"`
}
func (c *Config) MechanicsEngagement() string // default "auto"
func (c *Config) MechanicsCadenceTurns() int  // default 3
```

A single resolver, `engine.ResolveEngagement(manifest, systemManifest, cfg)`,
is used by the orchestrator so prompt, tools, and validation agree.

### 3.2 Prompt crafting

`FormatMechanicsInstructions` becomes policy-aware:

```go
func FormatMechanicsInstructions(spec *core.MechanicsSpec, engagement string) string
```

- **off**: "Mechanics are disabled for this campaign. Do not roll; decide
  outcomes from the fiction and narrate consequences directly."
- **auto**: today's block, sharpened around consequences: "Resolve with
  `request_check` *before* narrating whenever an outcome could cost or grant
  something the player would care about — harm, resources, standing, or a
  lasting change. Do not roll for safe or trivial actions." Then the declared
  notation/outcomes/difficulties as today.
- **ask**: "When an action has a chance of consequences, call `propose_check`
  with the stakes and the possible outcomes, then stop. Do not resolve it
  yourself; the player rolls." Then the same declared facts.

The section stays where it is injected (`pkg/harness/context.go:265`).

### 3.3 Tool surface per policy

- **off**: `request_check` and `propose_check` are not offered; a submitted
  check is rejected by validation (`no_check` becomes `checks_disabled`).
- **auto**: `request_check` as today; no `propose_check`.
- **ask**: `propose_check` offered; `request_check` rejected by validation
  (the model must not resolve its own checks). `submit_turn` remains terminal,
  but a turn may also end *pending* on a proposal.

Add a `ToolChoice` to the generation request for the forcing floor:

```go
// pkg/harness
type GenerateRequest struct {
    // ...existing...
    // ToolChoice is a hint the provider may honour: "" (default), "required",
    // or "none". A provider that cannot force ignores it.
    ToolChoice string
}
```

Providers map it: `openaichat` sets `tool_choice: "required"` / `"none"`;
`geminillm` sets `FunctionCallingConfig.mode: ANY` / `NONE`; others ignore it
and rely on the prompt and the retry below.

### 3.4 The forcing floor (`auto` only)

Cadence is counted from history: consecutive turns whose `Turn.Checks` is empty
(`pkg/engine/history.go:56`). When the count reaches `cadence_turns`:

1. If the provider reports tool-calling and honours `ToolChoice`, set
   `ToolChoice: "required"` for the turn's first assistant round, so the model
   must call a tool (it will pick `request_check` when the schema is the only
   fit, or `submit_turn`, which validation will then reject because the action
   is not `automatic` with no check — acceptable, it is a nudge).
2. If the provider cannot force, append a one-line instruction to the prompt:
   "The last {n} turns resolved without a check; if this action carries any
   consequence, call `request_check` before narrating."
3. Cap: at most one forced check per turn, so the fiction is never railroaded.

Both the cadence and the cap are tunable (`mechanics.cadence_turns`; cap fixed
at one).

### 3.5 `ask`: two-phase turns

New tool:

```
propose_check(actor, check_kind, stat, stakes, outcomes, notation?, difficulty?)
  -> { "pending": true }   // does not resolve
```

When the model calls it:

- The orchestrator records a `PendingCheck` on the turn and ends generation for
  that turn (like `submit_turn` at `orchestrator.go:1537-1557`), persisting the
  turn with `PendingCheck != nil` and no resolved checks.
- The chronicle renders a **Roll** card with stakes and the Roll button.

```go
// pkg/harness
type PendingCheck struct {
    Ref        string `json:"ref"`
    Request    CheckRequest `json:"request"`
    ProposedBy string `json:"proposed_by"` // "gm"
}
```

```go
// pkg/engine/history.go
type Turn struct {
    // ...existing...
    PendingCheck *harness.PendingCheck `json:"pending_check,omitempty"`
}
```

The player rolls by submitting a turn with the pending reference:

```go
// pkg/gui/types.go
type TurnRequest struct {
    // ...existing...
    // PendingCheckRef continues a turn whose GM proposed a check.
    PendingCheckRef string `json:"pending_check_ref,omitempty"`
}
```

The orchestrator resolves the stored request through the same `resolveCheck`
path (system `onCheck` or schema resolver), then generates the adjudication with
the result supplied to the GM, so the narration honours the rolled outcome. The
continuation is the existing `Roll` mode generation with a resolved check
attached rather than a proposal (`orchestrator.go:565-574` is the seed).

Edge cases: a pending check survives a restart (it is in `history.jsonl`); a
later player action before rolling supersedes it (the next turn clears it).

### 3.6 Visibility

- **Causal placement.** In `TurnSegments`, place unattached checks *before* the
  first narration segment of the turn instead of after all prose, so cause
  precedes effect (`TurnSegments.tsx:94-115`).
- **Outcome, not just dice.** `DiceCheckCard` shows the stakes line, the
  system's outcome label (`CheckResult.Outcome`), and the notation
  (`CheckResult.Roll.Notation`), keeping the existing tone colours.
- **Per-turn mechanics strip.** Above the action console, a one-line chip:
  "Mechanics: auto · 1 check (2d6 → 8, weak hit)" or "no checks this turn", so
  quiet turns still show the policy.
- **Pending roll card.** A distinct card with stakes and a Roll button; a dot on
  the Chronicle/Character affordance while a roll is pending.
- **Header indicator.** The game-state DTO gains `mechanics_engagement`, and the
  header shows a chip/dot for the active policy (extending the existing dot at
  `frontend/src/App.tsx:495`).

## 4. Interfaces

```go
// pkg/core
type MechanicsSpec struct { /* …; Engagement string */ }

// pkg/config
type MechanicsConfig struct{ Engagement string; CadenceTurns int }
func (c *Config) MechanicsEngagement() string
func (c *Config) MechanicsCadenceTurns() int

// pkg/engine
func ResolveEngagement(manifest *core.GameManifest, system *core.SystemManifest, cfg *config.Config) string

// pkg/harness
func FormatMechanicsInstructions(spec *core.MechanicsSpec, engagement string) string
type PendingCheck struct{ Ref string; Request CheckRequest; ProposedBy string }
type GenerateRequest struct{ /* …; ToolChoice string */ }

// pkg/gui
type TurnRequest struct{ /* …; PendingCheckRef string */ }
type GameStateDTO struct{ /* …; MechanicsEngagement string */ }
```

## 5. Error Handling

- An unknown engagement value falls back to `auto`.
- A `propose_check` with an empty stakes field is rejected with a tool error the
  model can fix, as `request_check` parsing is today.
- A roll for an unknown or already-resolved pending ref is refused with a clear
  message, and the client drops the card.
- If a provider cannot force tools, the floor degrades to the prompt nudge
  rather than failing the turn.
- A turn that ends pending is still recorded and indexed; it is simply missing
  a resolved check.

## 6. Testing & Verification

Go (stdlib `testing`, `t.TempDir()`):

- `pkg/engine`: engagement resolution order (campaign beats system beats
  config); `off` rejects `request_check`; `ask` rejects `request_check` and
  accepts `propose_check`, persisting a `PendingCheck`; rolling a pending ref
  resolves it and generates a continuation.
- `pkg/harness`: `FormatMechanicsInstructions` differs per policy and always
  cites declared notation/outcomes.
- `pkg/engine`: cadence — after N empty-check turns the first assistant round
  carries `ToolChoice: "required"`; the cap prevents a second forced check in
  one turn.
- `pkg/provider/openaichat`: `ToolChoice: "required"` marshals
  `tool_choice: "required"`; `geminillm`: maps to `mode: ANY`.

Frontend: `tsc`; the strip, card, and dot are checked by inspection.

## 7. Compatibility & Rollout

- Default `auto` preserves today's behaviour, plus the cadence floor (which can
  be disabled with `cadence_turns: 0`).
- `ProposedCheck` (existing) and `PendingCheck` (new) are distinct: the former
  is the player pre-empting a roll, the latter a GM proposal awaiting one.
- `GenerateRequest.ToolChoice` is optional; providers that ignore it are
  unchanged.
- `Turn.PendingCheck` is additive to `history.jsonl`; older turns have none.

## 8. Open Questions

- Should the cadence floor be per campaign or also per role?
- In `ask`, should the walk-away path (ignore the prompt and act) auto-resolve
  the pending check, or discard it?
- Does the mechanics strip belong above the console or in the turn header?
- Should `off` also disable `onTurnBegin`/`mechanics.js` execution, or only the
  check loop?

## 9. References

- Research: `docs/proposals/2026-09-26-mechanics-engagement-and-advancement-research.md`
- Code: `pkg/harness/mechanics_instructions.go:14-41`;
  `pkg/engine/orchestrator.go:388-409,565-588,1512-1557`;
  `pkg/engine/submission.go:101-161`; `pkg/engine/check_resolver.go:16-41`;
  `pkg/harness/turn_tools.go:21-78`; `pkg/harness/turn.go:63-119`;
  `pkg/harness/context.go:265,322-327`; `pkg/engine/history.go:56`;
  `pkg/gui/types.go:61,89`; `frontend/src/components/TurnSegments.tsx:94-115`;
  `frontend/src/components/DiceCheckCard.tsx`; `frontend/src/App.tsx:495`
- Provider forcing: OpenAI `tool_choice` — https://platform.openai.com/docs/guides/function-calling;
  Gemini `FunctionCallingConfig` — https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/models/function-calling;
  Anthropic `tool_choice` — https://platform.claude.com/docs/en/agents-and-tools/tool-use/define-tools
