# Image Trigger Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A configurable policy (off, scene_break, significant, every_turn, manual) deciding when a turn image is generated, with a deterministic significance heuristic.

**Architecture:** A trigger value resolved from campaign → config → default; a pure `ShouldIllustrate` function with logged reasons; the orchestrator gate consults it; a manual request endpoint covers on-demand generation.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-image-trigger-policy-design.md`
**Depends on:** IMG-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- With `scene_break`, behaviour must be identical to today.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The trigger configuration

**Files:**
- Modify: `pkg/config/types.go`, `pkg/config/manager.go`
- Test: `pkg/config/types_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces: `ImageConfig.Trigger`, `func (c *Config) ImageTrigger() string` returning `"significant"` by default.

- [ ] **Step 1: Write the failing test**

```go
func TestImageTriggerDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ImageTrigger() != "significant" {
		t.Fatalf("default = %q", cfg.ImageTrigger())
	}
	cfg.Media.Image.Trigger = "every_turn"
	if cfg.ImageTrigger() != "every_turn" {
		t.Fatalf("override = %q", cfg.ImageTrigger())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/config/ -run TestImageTriggerDefault -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `Trigger string \`yaml:"trigger,omitempty"\`` to `ImageConfig` and an accessor normalising an
empty or unknown value to `"significant"`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/config/ -run TestImageTriggerDefault -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/manager.go pkg/config/types_test.go
git commit -m "feat(config): add the image trigger policy"
```

---

### Task 2: `ShouldIllustrate`

**Files:**
- Create: `pkg/engine/image_trigger.go`
- Test: `pkg/engine/image_trigger_test.go`

**Interfaces:**
- Consumes: `Turn`, the previous turn, the trigger config.
- Produces: `TriggerConfig`, `func ShouldIllustrate(turn, prev Turn, cfg TriggerConfig) (bool, string)`.

- [ ] **Step 1: Write the failing test**

```go
func TestShouldIllustrate(t *testing.T) {
	cfg := DefaultTriggerConfig()
	if ok, _ := ShouldIllustrate(Turn{SceneBreak: true}, Turn{}, cfg); !ok {
		t.Fatal("a scene break is significant")
	}
	if ok, _ := ShouldIllustrate(Turn{Narration: "You wait."}, Turn{}, cfg); ok {
		t.Fatal("a quiet one-liner is not significant")
	}
	failed := Turn{Checks: []harness.CheckResult{{Outcome: "miss"}}, Narration: "The lock holds."}
	if ok, reason := ShouldIllustrate(failed, Turn{}, cfg); !ok || reason == "" {
		t.Fatal("a failed check is significant and should carry a reason")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestShouldIllustrate -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement the heuristic from the spec §4.2: scene break, decisive outcome, location change, entity
entrance/exit, or long narration, returning the first matching reason.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -run TestShouldIllustrate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/image_trigger.go pkg/engine/image_trigger_test.go
git commit -m "feat(engine): decide when a turn is worth illustrating"
```

---

### Task 3: The gate

**Files:**
- Modify: `pkg/engine/orchestrator.go:1462-1473`
- Test: `pkg/engine/image_trigger_test.go` (append)

**Interfaces:**
- Consumes: `ShouldIllustrate`, the trigger policy.
- Produces: the enqueue consults the policy.

- [ ] **Step 1: Write the failing tests**

```go
func TestGateHonoursPolicies(t *testing.T) {
	// off -> never; scene_break -> only on a break; every_turn -> always;
	// significant -> per the heuristic; manual -> never automatically.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/engine/ -run TestGateHonoursPolicies -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Replace the scene-break condition with a policy switch: `off` never, `manual` never, `every_turn`
always, `scene_break` on the existing condition, `significant` on `ShouldIllustrate`. Log the reason
with the job.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/engine/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/image_trigger_test.go
git commit -m "feat(engine): gate scene images on the trigger policy"
```

---

### Task 4: The manual request

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`
- Modify: `frontend/src/components/TurnSegments.tsx`, `frontend/src/api/client.ts`
- Test: `pkg/gui/scene_image_test.go`

**Interfaces:**
- Consumes: the scene worker.
- Produces: `POST /api/game/{id}/turn/{n}/scene-image` and a "Generate image" action.

- [ ] **Step 1: Write the failing test**

```go
func TestManualSceneImageRequest(t *testing.T) {
	// A request generates an image for a turn that has none.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestManualSceneImageRequest -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `Service.GenerateTurnSceneImage` that enqueues a scene image for a turn (using IMG-1's context)
regardless of policy, mount the route, and add a "Generate image" action to the turn view when the
turn has no image.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestManualSceneImageRequest -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat: generate a turn image on request"
```

---

### Task 5: Verification

- [ ] **Step 1: Regression guard**

Add a test asserting `scene_break` produces the same set of images as before this change.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Each policy behaves per the table.
- The heuristic returns a reason.
- A manual request generates an image regardless of policy.
- `scene_break` is unchanged.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the scene-break trigger"
```
