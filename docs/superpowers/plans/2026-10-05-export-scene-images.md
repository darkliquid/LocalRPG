# Export Scene Illustrations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An export shows a turn's scene illustration where one exists, and the location backdrop otherwise, in both web and video.

**Architecture:** `scene.Compile`'s `ArtResolver` prefers `assets/scenes/turn-<N><ext>` per beat, falling back to the location; both exports share the compiled script.

**Tech Stack:** Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-export-scene-images-design.md`
**Depends on:** IMG-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- An export of a campaign with no illustrations is unchanged.
- A missing asset is skipped, never a failure.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The per-beat art resolver

**Files:**
- Modify: `pkg/scene/compile.go` (the `ArtResolver` and `SceneArt` call)
- Test: `pkg/scene/compile_test.go` (append)

**Interfaces:**
- Consumes: the game's assets dir, `media.ArtExtension`.
- Produces: `func TurnArt(assetsDir string, turn int) (string, bool)` and a per-beat preference in the compiler.

- [ ] **Step 1: Write the failing tests**

```go
func TestTurnArtFindsAnIllustration(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "scenes"), 0o755)
	os.WriteFile(filepath.Join(dir, "scenes", "turn-3.png"), []byte("x"), 0o644)
	got, ok := TurnArt(dir, 3)
	if !ok || !strings.HasSuffix(got, "turn-3.png") {
		t.Fatalf("art = %q ok %v", got, ok)
	}
}
func TestTurnArtMissing(t *testing.T) {
	if _, ok := TurnArt(t.TempDir(), 3); ok {
		t.Fatal("a missing illustration should not resolve")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/scene/ -run TestTurnArt -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Probe `assets/scenes/turn-<N>` across the known extensions (`png`, `webp`, `jpg`, `jpeg`, `svg`),
mirroring `GetTurnSceneImage`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/scene/ -run TestTurnArt -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/scene/compile.go pkg/scene/compile_test.go
git commit -m "feat(scene): resolve a turn's illustration for an export"
```

---

### Task 2: The compiler prefers it

**Files:**
- Modify: `pkg/scene/compile.go` (`SceneArt`/the per-beat art)
- Test: `pkg/scene/compile_test.go` (append)

**Interfaces:**
- Consumes: `TurnArt` (Task 1).
- Produces: a beat whose art is the illustration when present.

- [ ] **Step 1: Write the failing test**

```go
func TestCompilePrefersTurnArt(t *testing.T) {
	// A turn with an illustration compiles to a beat whose art is that file;
	// a turn without one falls back to the scene's location art.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/scene/ -run TestCompilePrefersTurnArt -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

When resolving a beat's art, prefer `TurnArt(assetsDir, beat.Turn)`; otherwise use the scene's
location art as today. Keep the crossfade logic unchanged.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/scene/ -run TestCompilePrefersTurnArt -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/scene/compile.go pkg/scene/compile_test.go
git commit -m "feat(scene): prefer a turn illustration when compiling"
```

---

### Task 3: The web and video exports

**Files:**
- Modify: `pkg/export/web.go` (embed the chosen image)
- Modify: `pkg/export/video.go` (render it)
- Modify: `pkg/export/script.go` (the art store wiring)
- Test: `pkg/export/scene_image_test.go`

**Interfaces:**
- Consumes: the compiled beats (Task 2).
- Produces: both exports using the beat's chosen art.

- [ ] **Step 1: Write the failing tests**

```go
func TestWebExportEmbedsTurnIllustration(t *testing.T) { /* the bundle contains the turn's image */ }
func TestVideoPlanReferencesTurnIllustration(t *testing.T) { /* the render plan uses it */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/export/ -run 'TestWebExportEmbedsTurn|TestVideoPlanReferencesTurn' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Both exports already consume the compiled beats; ensure the web embedder and the video renderer read
`beat.Art` (the chosen image) rather than a scene-level image, and that the assets dir is passed so
`TurnArt` can resolve.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/export/ -run 'TestWebExportEmbedsTurn|TestVideoPlanReferencesTurn' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/export
git commit -m "feat(export): include turn illustrations in web and video"
```

---

### Task 4: Verification

- [ ] **Step 1: Parity guard**

Add a test that `GetTurnSceneImage` and the compiler resolve the same file for a turn.

- [ ] **Step 2: Regression guard**

Add a test that an export of a campaign with no illustrations is byte-identical to before.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- A beat with an illustration uses it; otherwise the location backdrop.
- Both exports agree.
- A missing asset is skipped.
- A campaign with no illustrations is unchanged.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: guard the illustration-free export"
```
