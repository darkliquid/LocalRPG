# Scene Consistency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make successive images of one scene share a look, via a stable seed and prompt prefix, and an optional reference image.

**Architecture:** A `SceneStyle` derived from the location and world style gives a stable seed, palette, and lighting; IMG-1's prompt splits into a stable prefix and a variable suffix; an optional `SceneConditioner` takes the previous scene image.

**Tech Stack:** Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-scene-consistency-design.md`
**Depends on:** IMG-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- A scene's first image is unchanged from IMG-1's output.
- Conditioning is optional; the seed-and-prefix path stands alone.
- Conventional Commits, subject under 72 chars.

---

### Task 1: `SceneStyle`

**Files:**
- Create: `pkg/media/scene_style.go`
- Test: `pkg/media/scene_style_test.go`

**Interfaces:**
- Consumes: the location entity, the world style.
- Produces: `SceneStyle`, `func NewSceneStyle(sceneID, worldStyle, appearance string) SceneStyle`.

- [ ] **Step 1: Write the failing tests**

```go
func TestSceneStyleIsStable(t *testing.T) {
	a := NewSceneStyle("saltmarch", "grim fantasy", "A salt-crusted port.")
	b := NewSceneStyle("saltmarch", "grim fantasy", "A salt-crusted port.")
	if a != b {
		t.Fatalf("style differs: %+v %+v", a, b)
	}
}
func TestSceneStyleChangesWithScene(t *testing.T) {
	if NewSceneStyle("a", "s", "").Seed == NewSceneStyle("b", "s", "").Seed {
		t.Fatal("a different scene should change the seed")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/media/ -run TestSceneStyle -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Derive the seed from the scene id and the world style (an FNV hash), and the palette and lighting
from the style and the appearance with small deterministic tables.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ -run TestSceneStyle -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/scene_style.go pkg/media/scene_style_test.go
git commit -m "feat(media): derive a scene's stable style"
```

---

### Task 2: The stable prompt prefix

**Files:**
- Modify: `pkg/engine/scene_worker.go` (IMG-1's `BuildScenePrompt`)
- Test: `pkg/engine/scene_prompt_test.go` (append)

**Interfaces:**
- Consumes: `SceneStyle`, `ScenePromptContext` (IMG-1).
- Produces: `BuildScenePrompt` splitting into a prefix and a suffix.

- [ ] **Step 1: Write the failing tests**

```go
func TestPromptPrefixIsStable(t *testing.T) {
	style := media.SceneStyle{SceneID: "s", Palette: "muted ochre", Lighting: "overcast", Described: "A port."}
	a := BuildScenePrompt(ScenePromptContext{Style: style, Action: "climb"})
	b := BuildScenePrompt(ScenePromptContext{Style: style, Action: "wait"})
	if promptPrefix(a) != promptPrefix(b) {
		t.Fatal("the prefix should be identical for one scene")
	}
	if a == b {
		t.Fatal("the suffix should differ")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/engine/ -run TestPromptPrefix -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Compose the prefix from the scene style (place, palette, lighting, appearance) and the suffix from
the turn (action, outcome tone, cast). Expose the prefix for the test.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/engine/ -run TestPromptPrefix -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/scene_worker.go pkg/engine/scene_prompt_test.go
git commit -m "feat(engine): keep a scene's prompt prefix stable"
```

---

### Task 3: The reference lookup and conditioning

**Files:**
- Modify: `pkg/media/scene_request.go` (the `SceneConditioner` interface)
- Modify: `pkg/media/image.go` (the pipeline asserts it)
- Modify: `pkg/engine/scene_worker.go` (find the previous scene image)
- Test: `pkg/media/scene_conditioner_test.go`

**Interfaces:**
- Consumes: `assets/scenes/*`, the timeline's turn→location index.
- Produces: `SceneConditioner`, and a reference path on `SceneStyle`.

- [ ] **Step 1: Write the failing tests**

```go
func TestConditionerIsUsedWhenAvailable(t *testing.T) {
	// A provider implementing SceneConditioner receives the reference bytes.
}
func TestPlainProviderIsUsedOtherwise(t *testing.T) {
	// A provider without the interface gets the plain request.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/media/ -run TestConditioner -v`
Expected: FAIL.

- **Step 3: Write minimal implementation**

Add the interface; in the pipeline, assert it and, when a reference exists for the same scene, call
`GenerateSceneWithReference`. In the scene worker, find the most recent scene image whose turn is in
the same location.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ -run TestConditioner -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media pkg/engine/scene_worker.go
git commit -m "feat(media): condition a scene on its previous image"
```

---

### Task 4: Verification

- [ ] **Step 1: Regression guard**

Add a test that a scene's first image is unchanged from IMG-1's output for the same context.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- The scene style is stable per location and varies across locations.
- The prompt prefix is identical within a scene; the suffix varies.
- A conditioner is used when available; otherwise the plain path.
- A first image is unchanged.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the first-scene-image path"
```
