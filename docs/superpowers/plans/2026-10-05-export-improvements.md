# Export Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Chapters, smaller exports, and scene transitions, with the bundle size measured.

**Architecture:** Chapters derive from the compiled scenes; embedded images are downscaled to the display size; a distinct scene transition uses TH-2's renderer; a benchmark pins the size.

**Tech Stack:** Go standard library; `golang.org/x/image/draw`; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-export-improvements-design.md`
**Depends on:** IMG-5, TH-4.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- The campaign's originals are never modified; only the export's copies.
- A story with no illustrations and one scene is unchanged apart from chapters.
- Conventional Commits, subject under 72 chars.

---

### Task 1: Chapters

**Files:**
- Modify: `pkg/scene/compile.go` (a chapter list on the script)
- Test: `pkg/scene/chapters_test.go`

**Interfaces:**
- Consumes: the compiled scenes.
- Produces: `type Chapter struct { Title string; Start time.Duration }`, `Script.Chapters`.

- [ ] **Step 1: Write the failing test**

```go
func TestChaptersFromScenes(t *testing.T) {
	s := compileFixture(t) // two scenes
	if len(s.Chapters) != 2 || s.Chapters[0].Title == "" {
		t.Fatalf("chapters = %+v", s.Chapters)
	}
	if s.Chapters[1].Start <= s.Chapters[0].Start {
		t.Fatal("chapter starts must increase")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/scene/ -run TestChaptersFromScenes -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Populate `Script.Chapters` from the scenes: the title from the scene's location, the start from its
first beat.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/scene/ -run TestChaptersFromScenes -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/scene/compile.go pkg/scene/chapters_test.go
git commit -m "feat(scene): derive export chapters from scenes"
```

---

### Task 2: The web player chapters

**Files:**
- Modify: `pkg/export/web.go`, `frontend/src/components/story/StoryPlayer.tsx`
- Test: `frontend/src/components/story/StoryPlayer.test.tsx` (append)

**Interfaces:**
- Consumes: `Script.Chapters` (Task 1).
- Produces: a chapter list and a `chapters` VTT track.

- [ ] **Step 1: Write the failing test**

```tsx
test("lists chapters and seeks on click", () => { /* a chapter button seeks the player */ });
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- StoryPlayer`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Render the chapter list beside the transport, mark the scrubber, and emit a `chapters` VTT track for
the native controls.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- StoryPlayer`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/export frontend/src/components/story
git commit -m "feat(export): add chapters to the web player"
```

---

### Task 3: Video chapters

**Files:**
- Modify: `pkg/export/video.go`
- Test: `pkg/export/chapters_test.go`

**Interfaces:**
- Consumes: `Script.Chapters` (Task 1).
- Produces: WebM chapter metadata or a sidecar.

- [ ] **Step 1: Write the failing test**

```go
func TestVideoWritesChapters(t *testing.T) {
	// Either the WebM carries a chapters element or a sidecar exists.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/export/ -run TestVideoWritesChapters -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Write the chapters into the WebM if the muxer supports it; otherwise write a sidecar in the ffmpeg
chapters format beside the video.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/export/ -run TestVideoWritesChapters -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/export
git commit -m "feat(export): add chapters to the video"
```

---

### Task 4: Image downscale

**Files:**
- Modify: `pkg/export/web.go` (the embed path)
- Test: `pkg/export/downscale_test.go`

**Interfaces:**
- Consumes: `golang.org/x/image/draw`.
- Produces: an embedded image scaled to the display size.

- [ ] **Step 1: Write the failing test**

```go
func TestEmbeddedImageIsDownscaled(t *testing.T) {
	// A 4000x3000 source embeds at no larger than the display size.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/export/ -run TestEmbeddedImageIsDownscaled -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Before embedding, decode the image and, when larger than the display size (for example 1280×720),
scale it down with `draw.CatmullRom`; re-encode as JPEG or WebP where the source is an oversized PNG.
Never touch the campaign's file.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/export/ -run TestEmbeddedImageIsDownscaled -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/export
git commit -m "feat(export): downscale embedded images"
```

---

### Task 5: Scene transitions and the benchmark

**Files:**
- Modify: `pkg/scene/render.go`, `pkg/export/video.go`
- Create: `pkg/export/size_bench_test.go`
- Test: `pkg/export/video_test.go` (append)

**Interfaces:**
- Consumes: TH-2's effects.
- Produces: a distinct scene transition; a size benchmark.

- [ ] **Step 1: Write the failing test**

```go
func TestSceneBoundaryTransitionDiffersFromBeatTransition(t *testing.T) {
	// A scene boundary's transition duration exceeds a beat's.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/export/ -run TestSceneBoundaryTransition -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

At a scene boundary, use a longer transition (a slower fade or a dip to black); elsewhere keep TH-2's
beat transition. Force a keyframe at the boundary as the renderer already does.

- [ ] **Step 4: Write the benchmark**

```go
func BenchmarkDemoBundleSize(b *testing.B) {
	// Export the website/demo fixture and record the bundle bytes; fail above the
	// pinned target so a regression is visible.
}
```

Run it once to record the baseline, pin the target, and re-run.

- [ ] **Step 5: Commit**

```bash
git add pkg/scene pkg/export
git commit -m "feat(export): scene transitions and a size benchmark"
```

---

### Task 6: Verification

- [ ] **Step 1: Regression guard**

Add a test that a single-scene story has one chapter and no boundary transition.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Chapters derive from scenes and appear in both exports.
- Embedded images are downscaled; the campaign's originals are untouched.
- A scene boundary transitions distinctly.
- The bundle size is measured and pinned.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the single-scene export"
```
