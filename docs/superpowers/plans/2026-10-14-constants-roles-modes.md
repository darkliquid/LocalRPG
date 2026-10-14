# Replace Stringly-Typed Role and Mode Literals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace bare strings (`"gm"`, `"player"`, `"off"`, `"auto"`, `"ask"`, `"2d6"`) and magic numbers (`1 << 30`, `4`) with named constants.
**Architecture:** Define constants in `pkg/config`, `pkg/core`, and `pkg/harness`; replace call sites across `pkg/engine`, `pkg/rules`, and `pkg/harness`.
**Tech Stack:** Go 1.27.
**Spec:** `docs/superpowers/specs/2026-10-14-constants-roles-modes-design.md`

## File Map

| File | Why |
|---|---|
| `pkg/config/types.go` | add `EngagementOff`, `EngagementAuto`, `EngagementAsk` constants |
| `pkg/core/mechanics.go` | add `DefaultCheckNotation` constant |
| `pkg/harness/turn.go` | add `ProposedByPlayer`, `ProposedByGM` constants |
| `pkg/harness/context.go` | export `RunesPerToken`, name `unconstrainedTurnUpperLimit` |
| `pkg/engine/engagement.go` | use `config.Engagement*` constants |
| `pkg/engine/orchestrator.go` | use `config.RoleGM`, `harness.ProposedBy*`, `core.DefaultCheckNotation`, `harness.RunesPerToken` |
| `pkg/engine/character_enricher.go` | use `config.RoleGM` |
| `pkg/rules/resolver.go` | use `core.DefaultCheckNotation` |

---

### Task 1: Declare the constants in foundational packages

**Files:**
- Modify: `pkg/config/types.go`
- Modify: `pkg/core/mechanics.go`
- Modify: `pkg/harness/turn.go`
- Modify: `pkg/harness/context.go`

- [ ] **Step 1: Add engagement constants to `pkg/config/types.go`**
- [ ] **Step 2: Add `DefaultCheckNotation` to `pkg/core/mechanics.go`**
- [ ] **Step 3: Add `ProposedByPlayer` and `ProposedByGM` to `pkg/harness/turn.go`**
- [ ] **Step 4: Export `RunesPerToken` and name `unconstrainedTurnUpperLimit` in `pkg/harness/context.go`**
- [ ] **Step 5: Verify packages compile**

---

### Task 2: Replace call sites in consumers

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/engine/character_enricher.go`
- Modify: `pkg/engine/engagement.go`
- Modify: `pkg/rules/resolver.go`

- [ ] **Step 1: Update `pkg/engine/engagement.go` to use `config.Engagement*`**
- [ ] **Step 2: Update `pkg/engine/character_enricher.go` to use `config.RoleGM`**
- [ ] **Step 3: Update `pkg/rules/resolver.go` to use `core.DefaultCheckNotation`**
- [ ] **Step 4: Update `pkg/engine/orchestrator.go` to use all named constants**
- [ ] **Step 5: Run unit tests across `pkg/engine`, `pkg/rules`, `pkg/harness`, and `pkg/config`**

---

### Task 3: Verification & Integration

- [ ] **Step 1: Run function length gate and linters**

Run: `go test -v ./pkg/systemtest -run "TestFunctionLength.*"`
Run: `mise run lint:gofmt && mise run lint:staticcheck`

- [ ] **Step 2: Run test suite with coverage check**

Run: `mise run test:backend`

- [ ] **Step 3: Commit, PR, merge, close bead**

Create PR, verify CI checks pass, merge PR, close bead `LocalRPG-92l.6`.
