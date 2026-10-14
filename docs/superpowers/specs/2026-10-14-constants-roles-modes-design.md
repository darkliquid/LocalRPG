# Replace Stringly-Typed Role and Mode Literals Design

**Date:** 2026-10-14
**Status:** Proposed
**Issue:** [#174](https://github.com/darkliquid/LocalRPG/issues/174)
**Epic:** [#151](https://github.com/darkliquid/LocalRPG/issues/151)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (RD-8, finding R-9)
**Scope:** `pkg/engine`, `pkg/config`, `pkg/core`, `pkg/harness`, `pkg/rules`. No behaviour change.

## 1. Problem

Finding R-9 highlighted multiple instances where bare string and magic number literals are used instead of existing or dedicated constants:
1. **Bare `"gm"` role literals:** The literal `"gm"` appears 18 times in `pkg/engine/orchestrator.go` and in `character_enricher.go`, despite `config.RoleGM` already existing in `pkg/config/types.go`.
2. **Engagement mode literals:** The strings `"off"`, `"auto"`, and `"ask"` are repeated across `pkg/config`, `pkg/engine/engagement.go`, and `pkg/engine/orchestrator.go` without named constants.
3. **Pending check proposer literals:** `PendingCheck.ProposedBy` uses bare strings `"player"` and `"gm"` in `pkg/engine/orchestrator.go` without named constants.
4. **Default check notation:** The default dice notation `"2d6"` is duplicated as a bare literal in both `pkg/engine/orchestrator.go:607` and `pkg/rules/resolver.go:69`.
5. **Magic turn upper bound:** `1 << 30` is hardcoded twice in `pkg/harness/context.go` (`:583` and `:679`) to represent an unconstrained upper turn limit for query windows.
6. **Token heuristic divisor:** `runes / 4` in `pkg/engine/orchestrator.go:2360` duplicates the named ratio `runesPerToken = 4` in `pkg/harness/context.go:56`.

## 2. Goals

- Replace all bare role literals with `config.RoleGM`.
- Define and use constants for engagement policies (`config.EngagementOff`, `config.EngagementAuto`, `config.EngagementAsk`).
- Define and use constants for check proposer (`harness.ProposedByPlayer`, `harness.ProposedByGM`).
- Define and use `core.DefaultCheckNotation = "2d6"` in `pkg/core/mechanics.go`.
- Name the unconstrained turn upper bound (`unconstrainedTurnUpperLimit = 1 << 30`) in `pkg/harness/context.go`.
- Export `harness.RunesPerToken = 4` in `pkg/harness/context.go` and use it in `pkg/engine/orchestrator.go`.
- Zero behaviour changes across tests and runtime.

## 3. Non-goals

- Refactoring `ProcessActionStream` control flow (covered by RD-3).
- Altering CLI flag names or user-facing serialization schemas.

## 4. Design

### Constants

1. **`pkg/config/types.go`:**
   ```go
   // Engagement policies for game mechanics.
   const (
       EngagementOff  = "off"
       EngagementAuto = "auto"
       EngagementAsk  = "ask"
   )
   ```

2. **`pkg/core/mechanics.go`:**
   ```go
   // DefaultCheckNotation is the fallback dice expression used when neither the check request,
   // resolution profile, nor system conventions declare one.
   const DefaultCheckNotation = "2d6"
   ```

3. **`pkg/harness/turn.go`:**
   ```go
   // Proposer of a pending check.
   const (
       ProposedByPlayer = "player"
       ProposedByGM     = "gm"
   )
   ```

4. **`pkg/harness/context.go`:**
   ```go
   // RunesPerToken is the estimated ratio of characters to tokens for English prose.
   const RunesPerToken = 4

   // unconstrainedTurnUpperLimit represents an unbounded upper turn number for queries.
   const unconstrainedTurnUpperLimit = 1 << 30
   ```

### Call-Site Migration

- **`pkg/engine/orchestrator.go`:**
  - Replace `"gm"` role lookups, trace attributes, and role fields with `config.RoleGM`.
  - Replace `ProposedBy: "player"` with `harness.ProposedByPlayer`.
  - Replace `ProposedBy: "gm"` with `harness.ProposedByGM`.
  - Replace `"2d6"` fallback with `core.DefaultCheckNotation`.
  - Replace `runes / 4` with `runes / harness.RunesPerToken`.
- **`pkg/engine/character_enricher.go`:**
  - Replace `"gm"` with `config.RoleGM`.
- **`pkg/engine/engagement.go`:**
  - Replace `"off"`, `"auto"`, `"ask"` with `config.EngagementOff`, `config.EngagementAuto`, `config.EngagementAsk`.
- **`pkg/rules/resolver.go`:**
  - Replace `"2d6"` fallback with `core.DefaultCheckNotation`.
- **`pkg/harness/context.go`:**
  - Replace `1 << 30` with `unconstrainedTurnUpperLimit`.

## 5. Testing

- Existing tests continue to pass without regression.
- Table tests verifying engagement defaults and resolution continue to pass.
- Function length limits ($\le 80$ lines) remain satisfied.
