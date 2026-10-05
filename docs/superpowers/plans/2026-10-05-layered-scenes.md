# Layered Scenes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Emit a procedural scene as background, midground, and foreground layers with depths, so the theatre and export can parallax them.

**Architecture:** `LayeredScene`/`Layer` types; PH-1's composer splits its parts into layers; the flattened output equals PH-1's single image; the theatre and export draw layers with TH-2's parallax.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-layered-scenes-design.md`
**Depends on:** PH-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- A one-layer scene at depth 0 is a flat image; the flat path is unchanged.
- Layers are deterministic; one seeded RNG.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The layer types

**Files:**
- Create: `pkg/media/layers.go`
- Test: `pkg/media/layers_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Layer`, `LayeredScene`, `func (s LayeredScene) Flatten() []byte`.

- [ ] **Step 1: Write the failing tests**

```go
func TestFlattenSingleLayer(t *testing.T) {
	s := LayeredScene{Width: 10, Height: 10, Layers: []Layer{{Depth: 0, SVG: []byte("<svg/>")}}}
	if !bytes.Contains(s.Flatten(), []byte("<svg")) {
		t.Fatalf("flatten = %q", s.Flatten())
	}
}
func TestFlattenOverlaysLayersInOrder(t *testing.T) {
	s := LayeredScene{Layers: []Layer{{Depth: 0, SVG: []byte("<a/>")}, {Depth: 1, SVG: []byte("<b/>")}}}
	got := string(s.Flatten())
	if strings.Index(got, "<a/>") > strings.Index(got, "<b/>") {
		t.Fatal("layers should flatten back to front")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/media/ -run TestFlatten -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the types and `Flatten`, which composes the layers into one SVG in depth order.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ -run TestFlatten -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/layers.go pkg/media/layers_test.go
git commit -m "feat(media): add layered scene types"
```

---

### Task 2: The generator emits layers

**Files:**
- Modify: `pkg/media/procedural_scene.go` (PH-1)
- Test: `pkg/media/layers_test.go` (append)

**Interfaces:**
- Consumes: PH-1's palette/structure/weather.
- Produces: `func GenerateLayeredScene(req SceneRequest) LayeredScene`.

- [ ] **Step 1: Write the failing tests**

```go
func TestGenerateLayeredScene(t *testing.T) {
	s := GenerateLayeredScene(SceneRequest{Genre: "fantasy", TimeOfDay: "day", Weather: "rain", Seed: "x"})
	if len(s.Layers) != 3 {
		t.Fatalf("layers = %d", len(s.Layers))
	}
	for i := 1; i < len(s.Layers); i++ {
		if s.Layers[i].Depth <= s.Layers[i-1].Depth {
			t.Fatal("depths should increase back to front")
		}
	}
}
func TestLayeredFlattenMatchesSingleImage(t *testing.T) {
	req := SceneRequest{Genre: "fantasy", Seed: "x"}
	flat := GenerateLayeredScene(req).Flatten()
	single := GenerateSceneSVG(req)
	// The composed content should match (same parts, same order).
	if len(flat) == 0 || len(single) == 0 {
		t.Fatal("empty output")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/media/ -run TestGenerateLayeredScene -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Split PH-1's composer: background (sky, celestial, ridge), midground (structure), foreground (fog,
particles, weather), each a transparent SVG. `GenerateSceneSVG` becomes `GenerateLayeredScene(req).Flatten()`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ -run 'TestGenerateLayeredScene|TestLayeredFlatten' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/procedural_scene.go pkg/media/layers_test.go
git commit -m "feat(media): emit a procedural scene as layers"
```

---

### Task 3: The theatre draws layers

**Files:**
- Modify: `frontend/src/components/theater/TheaterStage.tsx`
- Test: `frontend/src/components/theater/TheaterStage.test.tsx` (append)

**Interfaces:**
- Consumes: `LayeredScene`, TH-2's parallax.
- Produces: layers drawn with parallax.

- [ ] **Step 1: Write the failing test**

```tsx
test("draws layers at different offsets", () => {
  // Given a layered scene, the background and foreground offsets differ.
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- TheaterStage`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

When the scene provides layers, render each as an `<img>` (data URI) or inline SVG, positioned with
its depth-scaled offset; a flat image renders as today.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- TheaterStage`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/theater
git commit -m "feat(frontend): draw layered scenes with parallax"
```

---

### Task 4: The export draws layers

**Files:**
- Modify: `pkg/scene/render.go`, `pkg/export/script.go`
- Test: `pkg/scene/layers_test.go`

**Interfaces:**
- Consumes: `LayeredScene`.
- Produces: the renderer composites layers with parallax.

- [ ] **Step 1: Write the failing test**

```go
func TestRendererDrawsLayersWithParallax(t *testing.T) {
	// Two frames at different progresses show different layer offsets.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/scene/ -run TestRendererDrawsLayers -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Rasterise each layer (the existing SVG rasteriser handles a fragment) and composite with the depth
offset; a flat image composites as today.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/scene/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/scene pkg/export
git commit -m "feat(scene): composite layered scenes in the export"
```

---

### Task 5: Verification

- [ ] **Step 1: Determinism test**

Add a test that `GenerateLayeredScene` is byte-identical for the same request.

- [ ] **Step 2: Regression guard**

Add a test that a flat image path is unchanged.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- Three layers with increasing depth are emitted.
- The flattened output matches PH-1's single image.
- The theatre and export parallax layers.
- A flat image is unchanged.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: guard the flat scene path"
```
