# Captions and Subtitles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A theatre caption overlay and a WebVTT subtitle track in both exports, from the same segments.

**Architecture:** `pkg/scene.Captions` renders the compiled beats as WebVTT; the web export embeds it, the video export writes a sidecar; the theatre shows the current beat's caption behind a toggle.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-captions-and-subtitles-design.md`
**Depends on:** TH-3.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- Captions are off by default.
- Captions and subtitles come from the same segments and timings.
- Conventional Commits, subject under 72 chars.

---

### Task 1: `Captions`

**Files:**
- Create: `pkg/scene/captions.go`
- Test: `pkg/scene/captions_test.go`

**Interfaces:**
- Consumes: the compiled `Beat`s.
- Produces: `func Captions(beats []Beat) string`.

- [ ] **Step 1: Write the failing tests**

```go
func TestCaptionsEmitSpokenCuesOnly(t *testing.T) {
	beats := []Beat{
		{Kind: SegmentSpeech, Speaker: "Garrick", Text: "Keep walking.", Start: 0, Duration: 2 * time.Second},
		{Kind: SegmentNarration, Text: "The hall is quiet.", Start: 2 * time.Second, Duration: 2 * time.Second},
	}
	got := Captions(beats)
	if !strings.Contains(got, "WEBVTT") || !strings.Contains(got, "<v Garrick>Keep walking.") {
		t.Fatalf("vtt = %q", got)
	}
	if strings.Contains(got, "hall is quiet") {
		t.Fatal("narration should not be a caption")
	}
}
func TestCaptionsAreWellFormed(t *testing.T) { /* timestamps are hh:mm:ss.mmm --> ... */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/scene/ -run TestCaptions -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Emit the WebVTT header and one cue per spoken beat, using the beat's start and duration and a
`<v Speaker>` voice tag.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/scene/ -run TestCaptions -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/scene/captions.go pkg/scene/captions_test.go
git commit -m "feat(scene): render captions as WebVTT"
```

---

### Task 2: The web export

**Files:**
- Modify: `pkg/export/web.go`, `pkg/export/script.go`
- Modify: `frontend/src/components/story/StoryPlayer.tsx`
- Test: `pkg/export/captions_test.go`

**Interfaces:**
- Consumes: `scene.Captions` (Task 1).
- Produces: the bundle carrying the VTT and the player offering the track.

- [ ] **Step 1: Write the failing test**

```go
func TestWebBundleCarriesCaptions(t *testing.T) {
	// The exported bundle contains a WEBVTT document and the player references it.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/export/ -run TestWebBundleCarriesCaptions -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Embed the VTT in the bundle (a data URI or a blob the player loads) and add a `<track>` to the
player, defaulting off.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/export/ -run TestWebBundleCarriesCaptions -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/export frontend/src/components/story
git commit -m "feat(export): carry captions in the web bundle"
```

---

### Task 3: The video export

**Files:**
- Modify: `pkg/export/video.go`, `pkg/gui/export.go`
- Test: `pkg/export/captions_test.go` (append)

**Interfaces:**
- Consumes: `scene.Captions` (Task 1).
- Produces: a `.vtt` sidecar beside the `.webm`.

- [ ] **Step 1: Write the failing test**

```go
func TestVideoExportWritesASidecar(t *testing.T) { /* a .vtt exists beside the .webm */ }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/export/ -run TestVideoExportWritesASidecar -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Write the VTT beside the video with the same base name, and name it in the export summary/artifact
list.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/export/ -run TestVideoExportWritesASidecar -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/export pkg/gui
git commit -m "feat(export): write a subtitle sidecar for video"
```

---

### Task 4: The theatre caption

**Files:**
- Modify: `frontend/src/components/theater/TheaterDialogue.tsx`, `StoryTheater.tsx`, `TheaterTransport.tsx`
- Test: `frontend/src/components/theater/TheaterDialogue.test.tsx` (append)

**Interfaces:**
- Consumes: the current beat's spoken line.
- Produces: a caption overlay and a toggle.

- [ ] **Step 1: Write the failing tests**

```tsx
test("shows a caption when enabled", () => { /* the spoken line appears in the caption layer */ });
test("hides the caption when disabled", () => { /* no caption layer */ });
test("shows a caption for a silent spoken beat", () => { /* the caption is present without audio */ });
test("the toggle is keyboard reachable", () => { /* the control has a label */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- TheaterDialogue`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the caption overlay (with a solid backdrop for contrast), a `captions` preference, and a toggle in
the transport with an accessible label.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- TheaterDialogue`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components
git commit -m "feat(frontend): add theatre captions"
```

---

### Task 5: Verification

- [ ] **Step 1: Parity test**

Add a test that the WebVTT cue for a beat matches the theatre's caption text and timing.

- [ ] **Step 2: Regression guard**

Add a test that captions off changes nothing.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- Spoken beats produce cues; narration does not.
- The web bundle and the video sidecar carry the track.
- The theatre shows a caption behind a toggle.
- A silent beat still reads.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: guard the captions-off path"
```
