# Image Cost Guardrails Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bound a campaign's image generation by count and spend, with an approve mode and a procedural fallback.

**Architecture:** An `ImageBudget` in `game.yaml` with used/spent counters; the scene worker checks it before calling the provider and falls back to procedural art; an approval mode emits a pending event for a metered provider.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-image-cost-guardrails-design.md`
**Depends on:** IMG-2.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- Unlimited by default; an unconfigured campaign is unchanged.
- The budget check is in one place.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The budget type and its storage

**Files:**
- Modify: `pkg/core/types.go` (`GameManifest.Settings`), `pkg/config/types.go`
- Create: `pkg/engine/image_budget.go`
- Test: `pkg/engine/image_budget_test.go`

**Interfaces:**
- Consumes: the game settings, the pricing ledger.
- Produces: `ImageBudget`, `func (b *ImageBudget) Exhausted() bool`, `func (b *ImageBudget) Charge(images int, micros int64)`, `func LoadImageBudget(paths, gameID) (ImageBudget, error)`, `func SaveImageBudget(paths, gameID, b) error`.

- [ ] **Step 1: Write the failing tests**

```go
func TestBudgetExhausted(t *testing.T) {
	b := ImageBudget{MaxImages: 2, Used: 2}
	if !b.Exhausted() {
		t.Fatal("at the limit should be exhausted")
	}
	b = ImageBudget{MaxMicros: 100, Spent: 99}
	if b.Exhausted() {
		t.Fatal("under the spend limit is not exhausted")
	}
	if (ImageBudget{}).Exhausted() {
		t.Fatal("unlimited is never exhausted")
	}
}
func TestBudgetRoundTrips(t *testing.T) { /* save then load yields the same counters */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/engine/ -run TestBudget -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the type, `Exhausted`, `Charge`, and load/save in the game settings.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/engine/ -run TestBudget -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/core pkg/config pkg/engine/image_budget.go pkg/engine/image_budget_test.go
git commit -m "feat(engine): add a per-campaign image budget"
```

---

### Task 2: Enforcement in the scene worker

**Files:**
- Modify: `pkg/engine/scene_worker.go`, `pkg/engine/orchestrator.go`
- Test: `pkg/engine/image_budget_test.go` (append)

**Interfaces:**
- Consumes: `ImageBudget`.
- Produces: the worker checks the budget and charges on success.

- [ ] **Step 1: Write the failing tests**

```go
func TestBudgetBlocksGeneration(t *testing.T) { /* at the limit, the provider is not called */ }
func TestBudgetChargesOnSuccess(t *testing.T) { /* Used increments */ }
func TestBudgetFallsBackToProcedural(t *testing.T) { /* an image still exists */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/engine/ -run TestBudgetBlocks -v`
Expected: FAIL.

- **Step 3: Write minimal implementation**

Check `Exhausted()` before enqueuing; trace `turn.image_budget_exhausted` and use the procedural
fallback when exhausted; charge on success using the ledger's price.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/engine/ -run TestBudget -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine
git commit -m "feat(engine): enforce the image budget"
```

---

### Task 3: The approval flow

**Files:**
- Modify: `pkg/engine/scene_worker.go`, `pkg/gui/types.go`, `pkg/gui/service.go`
- Modify: `frontend/src/components/TurnSegments.tsx`
- Test: `pkg/gui/image_approval_test.go`

**Interfaces:**
- Consumes: `image_approval: ask`.
- Produces: a `scene_image_pending` event and an approve action.

- [ ] **Step 1: Write the failing tests**

```go
func TestAskModeEmitsPending(t *testing.T) { /* no provider call; a pending event */ }
func TestApproveGeneratesAndCharges(t *testing.T) { /* the generation runs on approve */ }
func TestLocalProviderDoesNotAsk(t *testing.T) { /* a non-metered provider generates directly */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestAskMode -v`
Expected: FAIL.

- **Step 3: Write minimal implementation**

When `image_approval` is `ask` and the provider is metered, emit a pending event with the prompt and
estimate instead of generating; add an approve endpoint and a client prompt.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestAskMode -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine pkg/gui frontend/src
git commit -m "feat: approve a metered image before it is generated"
```

---

### Task 4: Visibility

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`
- Modify: `frontend/src/components/launcher/CampaignSettingsModal.tsx`, `frontend/src/components/TurnSegments.tsx`
- Test: `frontend/src/components/CampaignSettingsModal.test.tsx` (append)

**Interfaces:**
- Consumes: the budget.
- Produces: the remaining allowance in the settings and a near-limit indicator on the turn.

- [ ] **Step 1: Write the failing test**

```tsx
test("shows the image budget", () => {
  render(<CampaignSettingsModal budget={{ max_images: 200, used_images: 195 }} /* … */ />);
  expect(screen.getByText(/195/)).toBeInTheDocument();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- CampaignSettingsModal`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the budget fields to the settings modal and a small indicator when the budget is nearly spent.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- CampaignSettingsModal`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat(frontend): show the image budget"
```

---

### Task 5: Verification

- [ ] **Step 1: Regression guard**

Add a test that an unlimited budget behaves exactly as IMG-2.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- The budget blocks at the limit and falls back to procedural art.
- Counters increment and persist.
- `ask` emits a pending event for a metered provider.
- Unlimited is unchanged.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the unlimited-budget path"
```
