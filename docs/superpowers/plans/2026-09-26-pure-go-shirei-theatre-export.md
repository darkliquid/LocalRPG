# Pure-Go shirei GUI — Shared Theatre and Video Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `pkg/scene`'s bespoke rasteriser with song-shared shirei view functions parameterised by an explicit time/progress model, so the live window and the ffmpeg exporter render the same theatre.

**Architecture:** `pkg/theater` owns `Frame{Script, SceneIdx, BeatIdx, Progress}` and pure view functions that read it. The live screen (`pkg/desktop/theater.go`) drives `Progress`/beat index from audio state and a timer. The exporter drives them per output frame and rasterises with shirei's `RunFrameFn` + `SoftRenderer` at scale 1 (not `RenderToImage`, which hardcodes 2×). `pkg/scene` keeps `Script/Scene/Beat`; `pkg/export/web.go` is untouched.

**Tech Stack:** Go 1.27.1, `go.hasen.dev/shirei` (`RunFrameFn`, `SoftRenderer`, `Framebuffer.ToRGBA`, `ContainerWithKey`), `pkg/scene`, `pkg/export`, `pkg/gui` audio service, `image/png`, ffmpeg (external).

**Spec:** `docs/superpowers/specs/2026-09-26-pure-go-shirei-gui-design.md` (Phase 7, §5)

## Global Constraints

- Desktop only. Do not modify `pkg/gui/server.go`, `socket.go`, `assets.go`, `middleware.go`, or `pkg/export/web.go`.
- `pkg/scene` stays as the `Script/Scene/Beat` model and its pacing helpers; only `pkg/scene/render.go`'s rasteriser is retired from use (keep the file until nothing imports it, then delete).
- The exporter must render at scale 1: `RenderToImage` hardcodes `HeadlessScale = 2`, so 1080p would rasterise 3840×2160. Use `RunFrameFn` + `SoftRenderer.Render(...)` directly.
- Time is explicit data: all animated values are functions of `Frame.Progress`; never rely on shirei's implicit animation (forced off in the offline path).
- Go style: `any`; `go vet ./...` clean. Tests standard-library only.
- Commits: Conventional Commits with a scope; end with the attribution block shown in Task 1 Step 4.
- `mise run test` must pass (`pkg/gui` is flaky ~1 in 6; re-run before calling it a regression).

---

### Task 1: The shared theatre view

**Files:**
- Create: `pkg/theater/theater.go`
- Create: `pkg/theater/view.go`
- Test: `pkg/theater/theater_test.go`
- Create: `pkg/desktop/theater_fixture_test.go` (a shared fixture, in `desktop`, used for the golden)

**Interfaces:**
- Consumes: `scene.Script`, `scene.Scene`, `scene.Beat`, `scene.BeatKind` (`pkg/scene/scene.go:23-52`), `scene.BeatDuration` (`timing.go:50`), `scene.FramesFor` (`timing.go:60`).
- Produces:
  - `type theater.Frame struct { Script *scene.Script; SceneIdx, BeatIdx int; Progress float64 }`
  - `func theater.View(f Frame)` — pure shirei view, no state reads
  - `func theater.BeatAt(f Frame) (scene.Scene, scene.Beat, bool)`
  - `func theater.BeatProgress(script *scene.Script, frame int, fps int) Frame` — maps an exporter frame index to a `Frame`

- [ ] **Step 1: Write the failing test**

Create `pkg/theater/theater_test.go`:

```go
package theater

import (
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/scene"
)

func testScript() *scene.Script {
	return &scene.Script{
		GameName: "Demo",
		Scenes: []scene.Scene{{
			LocationID: "hall", LocationName: "The Hall", ArtPath: "/tmp/hall.png",
			Beats: []scene.Beat{
				{Kind: scene.BeatSceneCard, Text: "The Hall", Duration: 2 * time.Second},
				{Kind: scene.BeatNarration, Text: "The door opens.", Duration: 3 * time.Second},
				{Kind: scene.BeatSpeech, Speaker: "Vance", Text: "Hello?", Duration: 2 * time.Second},
			},
		}},
	}
}

func TestBeatAtIndexesFlattenedBeats(t *testing.T) {
	s := testScript()
	sc, beat, ok := BeatAt(Frame{Script: s, SceneIdx: 0, BeatIdx: 2})
	if !ok {
		t.Fatal("expected a beat")
	}
	if beat.Speaker != "Vance" || sc.LocationName != "The Hall" {
		t.Fatalf("beat = %+v scene = %+v", beat, sc)
	}
}

func TestBeatProgressWalksFrames(t *testing.T) {
	s := testScript()
	// 2s + 3s + 2s = 7s at 15fps = 105 frames.
	f0 := BeatProgress(s, 0, 15)
	if f0.BeatIdx != 0 || f0.Progress != 0 {
		t.Fatalf("first frame = %+v", f0)
	}
	fLast := BeatProgress(s, 104, 15)
	if fLast.BeatIdx != 2 {
		t.Fatalf("last frame beat = %+v", fLast)
	}
	fMid := BeatProgress(s, 45, 15)
	if fMid.SceneIdx != 0 || fMid.Progress <= 0 || fMid.Progress >= 1 {
		t.Fatalf("mid frame = %+v", fMid)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/theater/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement the model and view**

Create `pkg/theater/theater.go` with `Frame`, `BeatAt`, and `BeatProgress`. `BeatProgress` accumulates `scene.FramesFor(beat.Duration, fps)` across `script.Scenes`/`Beats` (mirroring `pkg/export/video.go:65-91`), picks the beat containing `frame`, and computes `Progress = float64(frameInBeat)/float64(frames-1)` (or `1` when one frame).

Create `pkg/theater/view.go` with `View(f Frame)`. Port the SPA layout from `frontend/src/components/theater/*`:

- Full-bleed background `Image(artPath, Vec2{w, h})` over a dark fill, then a vertical scrim via layered `Element`s with `Background` alpha (mirroring the Tailwind gradient).
- A character row: player sprite bottom-left, NPC sprite bottom-right mirrored (`scale-x-[-1]` has no shirei equivalent; document that as a known deviation), each in a borderless/rounded box with an accent border when active.
- Bottom dialogue box: speaker plate tinted by kind (player/NPC/narrator), the beat text via `proseBlocks`-style rendering — **but `proseBlocks` lives in `pkg/desktop`; do not import it here**. Instead accept a text-render callback: `type TextFunc func(text string)`, set by the caller via `ViewWith(f, textFn)`, defaulting to a plain `Label`. This keeps `pkg/theater` free of the desktop package.
- A transport bar: progress track/fill from `f.Progress`, and (in the live screen only) buttons. For the shared view, render the progress bar; buttons are added by the live screen wrapper.

Keep `View` free of hooks and package state so repeated offline renders cannot leak identity-keyed state.

- [ ] **Step 4: Run the test and commit**

Run: `go test ./pkg/theater/ -v`
Expected: PASS.

```bash
git add pkg/theater
git commit -m "$(cat <<'EOF'
feat(theater): add the shared theatre view and time model

The live window and the video exporter will render the same view from
an explicit (beat, progress) frame, so a change to the stage shows up
in both.

💘 Generated with Crush

Assisted-by: Crush:deepseek-v4.1-flash
EOF
)"
```

---

### Task 2: Offline frame rasteriser at scale 1

**Files:**
- Create: `pkg/theater/render.go`
- Test: `pkg/theater/render_test.go`

**Interfaces:**
- Consumes: shirei `RunFrameFn(frameFn FrameFn) FrameOutputData` (`shirei.go:205`), `SoftRenderer`/`Render`/`Framebuffer.ToRGBA` (`softrender.go:158/227/70`), `ResetInputSession` (`renderpng.go:12`), `ContainerWithKey` (`shirei.go:966`).
- Produces:
  - `func theater.RenderFrame(f Frame, w, h int) *image.RGBA`
  - `func theater.WriteFramePNG(path string, f Frame, w, h int) error`

- [ ] **Step 1: Write the failing test**

Create `pkg/theater/render_test.go`:

```go
package theater

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderFrameSize(t *testing.T) {
	// A minimal script with no art so the render needs no external files.
	s := testScript()
	s.Scenes[0].ArtPath = ""
	f := Frame{Script: s, SceneIdx: 0, BeatIdx: 1, Progress: 0.5}
	img := RenderFrame(f, 640, 360)
	if img.Bounds().Dx() != 640 || img.Bounds().Dy() != 360 {
		t.Fatalf("bounds = %v", img.Bounds())
	}
}

func TestWriteFramePNG(t *testing.T) {
	s := testScript()
	s.Scenes[0].ArtPath = ""
	path := filepath.Join(t.TempDir(), "frame.png")
	if err := WriteFramePNG(path, Frame{Script: s, SceneIdx: 0, BeatIdx: 0, Progress: 1}, 320, 180); err != nil {
		t.Fatalf("WriteFramePNG: %v", err)
	}
	fh, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer fh.Close()
	img, err := png.Decode(fh)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if img.Bounds().Dx() != 320 {
		t.Fatalf("decoded bounds = %v", img.Bounds())
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/theater/ -run 'TestRenderFrame|TestWriteFramePNG' -v`
Expected: FAIL.

- [ ] **Step 3: Implement the rasteriser**

Create `pkg/theater/render.go`:

```go
package theater

import (
	"image"
	"image/png"
	"os"

	"go.hasen.dev/shirei"
)

// theaterScope keeps the offline view's identity stable across the settle
// passes of one frame. Each RenderFrame call reuses it; because View reads all
// state from the Frame argument, retained identity state cannot change output.
var theaterScope = new(int)

// RenderFrame software-rasterises one theatre frame at scale 1.
//
// It deliberately does not use shirei.RenderToImage, which hardcodes a 2x
// headless scale and would turn a 1920x1080 export frame into a 3840x2160
// rasterisation.
func RenderFrame(f Frame, w, h int) *image.RGBA {
	shirei.ResetInputSession()
	host := shirei.GetHost()
	host.WindowSize = shirei.Vec2{float32(w), float32(h)}
	host.WindowScale = 1
	host.HeadlessRender = true
	defer func() { host.HeadlessRender = false }()
	if host.GlyphCacheBudgetBytes == 0 {
		host.GlyphCacheBudgetBytes = 16 << 20
	}

	var out shirei.FrameOutputData
	for i := 0; i < 8; i++ {
		out = shirei.RunFrameFn(func() {
			shirei.ModAttrs(func(a *shirei.AttrSet) { a.Animations = 0 })
			shirei.ContainerWithKey(theaterScope, shirei.Attrs(shirei.Viewport), func() {
				View(f)
			})
		})
		if i >= 1 && !out.NextFrameRequested {
			break
		}
	}

	var rend shirei.SoftRenderer
	fb := rend.Render(out.Surfaces, out.GlyphRuns, w, h, 1)
	return fb.ToRGBA()
}

// WriteFramePNG renders f and writes it as a PNG with fast compression
// (1080p size is irrelevant next to the x264 encode).
func WriteFramePNG(path string, f Frame, w, h int) error {
	img := RenderFrame(f, w, h)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	return encoder.Encode(file, img)
}
```

Confirm `SoftRenderer.Render`'s parameter order and `Framebuffer.ToRGBA` against `softrender.go`; adjust the call if the vendored signature differs.

- [ ] **Step 4: Run tests and commit**

Run: `go test ./pkg/theater/ -v`
Expected: PASS.

```bash
git add pkg/theater
git commit -m "feat(theater): rasterise theatre frames offscreen at scale 1"
```

---

### Task 3: Rewire the video exporter

**Files:**
- Modify: `pkg/export/video.go` (frame producer only)
- Test: `pkg/export/video_test.go`

**Interfaces:**
- Consumes: `theater.BeatProgress`, `theater.WriteFramePNG`.
- Produces: `frameWriter.write` now renders via `pkg/theater`; `NewVideoPipeline`, `SetSize`, `SetFPS`, `SetStill`, `BuildCommand`, `RenderVideo` keep their signatures.

- [ ] **Step 1: Write the failing test**

Add to `pkg/export/video_test.go`:

```go
func TestFrameWriterUsesTheatreFrames(t *testing.T) {
	dir := t.TempDir()
	w := &frameWriter{dir: dir, fps: 15, still: true, progress: func(string, ...any) {}}
	script := &scene.Script{Scenes: []scene.Scene{{LocationName: "Hall", Beats: []scene.Beat{
		{Kind: scene.BeatSceneCard, Text: "Hall", Duration: 2 * time.Second},
		{Kind: scene.BeatNarration, Text: "It opens.", Duration: 3 * time.Second},
	}}}}
	count, err := w.write(script)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if count != 2 { // --still = one frame per beat
		t.Fatalf("count = %d, want 2", count)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("frames on disk = %d, want 2", len(entries))
	}
}
```

If `frameWriter` uses a `*scene.Renderer` field, the test will fail to compile until Step 3 removes it; that is the intended red state.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/export/ -run TestFrameWriterUsesTheatreFrames -v`
Expected: FAIL.

- [ ] **Step 3: Swap the rasteriser**

In `pkg/export/video.go`:
- Remove the `renderer *scene.Renderer` field from `frameWriter` and the `scene.NewRenderer` call in `RenderVideo`.
- Rework `write` to iterate frame indices the way `theater.BeatProgress` expects: compute total frames from `scene.FramesFor(beat.Duration, fps)` (or `1` when `still`), keep a running `number`, and for each frame call `theater.WriteFramePNG(filepath.Join(dir, fmt.Sprintf("frame-%06d.png", number)), theater.BeatProgress(script, number, fps), w.width, w.height)`.
- Because `BeatProgress` needs the pixel size only for rendering, store `width`/`height` on `frameWriter`.
- Leave `BuildCommand` and the ffmpeg/audio concat logic untouched.

- [ ] **Step 4: Run tests and commit**

Run: `go test ./pkg/export/ -v`
Expected: PASS.

```bash
git add pkg/export
git commit -m "refactor(export): render video frames with the shared theatre view"
```

---

### Task 4: The live theatre screen

**Files:**
- Create: `pkg/desktop/theater.go`
- Modify: `pkg/desktop/state.go` (`ScreenTheater`, playback fields)
- Modify: `pkg/desktop/root.go` (dispatch)
- Test: `pkg/desktop/theater_test.go`
- Create: `pkg/desktop/testdata/snapshots/theater.png`

**Interfaces:**
- Consumes: the shared `theater.View`, `(*gui.Service).PlayTurnAudio`/`PlaySegmentAudio`/`GetSegmentAudio`/`StopAudio`/`AudioAvailable`/`AudioPlaying` (`pkg/gui/service.go:1830/1872/1683/1775/1761/1766`), `gui.TurnDTO`/`SegmentDTO`.
- Produces:
  - `State.TheaterTurn int`, `State.TheaterBeat int`, `State.TheaterPlaying bool`, `State.TheaterSpeed float64`
  - `func theaterScreen()`, `func advanceBeat()`, `func changeTurn(delta int)`, `func togglePlay()`

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/theater_test.go`:

```go
package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestAdvanceBeatWalksSegmentsThenTurns(t *testing.T) {
	appState = &State{
		Loaded: true,
		Screen: ScreenTheater,
		Turns: []gui.TurnDTO{
			{TurnNumber: 1, Segments: []gui.SegmentDTO{{Kind: "narration", Text: "a"}, {Kind: "speech", Speaker: "V", Text: "b"}}},
			{TurnNumber: 2, Segments: []gui.SegmentDTO{{Kind: "narration", Text: "c"}}},
		},
		TheaterTurn: 0, TheaterBeat: 0,
	}
	advanceBeat()
	if appState.TheaterBeat != 1 {
		t.Fatalf("beat = %d, want 1", appState.TheaterBeat)
	}
	advanceBeat()
	if appState.TheaterTurn != 1 || appState.TheaterBeat != 0 {
		t.Fatalf("turn/beat = %d/%d, want 1/0", appState.TheaterTurn, appState.TheaterBeat)
	}
}

func TestChangeTurnClamps(t *testing.T) {
	appState = &State{Loaded: true, Turns: []gui.TurnDTO{{TurnNumber: 1}, {TurnNumber: 2}}, TheaterTurn: 0}
	changeTurn(-1)
	if appState.TheaterTurn != 0 {
		t.Fatalf("turn = %d, want clamp at 0", appState.TheaterTurn)
	}
	changeTurn(1)
	if appState.TheaterTurn != 1 {
		t.Fatalf("turn = %d, want 1", appState.TheaterTurn)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run 'TestAdvanceBeat|TestChangeTurn' -v`
Expected: FAIL.

- [ ] **Step 3: Implement the screen**

`theaterScreen()` builds a `theater.Script` from `appState.Turns` (one scene, beats from segments; use the segment text and portrait art paths) and renders `theater.View` for the current `(turn, beat)`, plus `Transport` buttons (prev/play-pause/next/speed) over the shared view. Pass a text-render callback so dialogue uses `proseBlocks`.

Playback state machine (mirrors `StoryTheater.tsx`): on entering a beat, if audio is available call `PlaySegmentAudio` once (guarded by a beat-key ref); on a `playing → idle` transition wait 300 ms then `advanceBeat`; with no audio, advance after `max(1200, duration*1000)/speed` ms. `togglePlay` calls `StopAudio` and resets; `changeTurn` stops audio and resets the beat. Speed cycles 1 → 1.5 → 2.

Audio availability comes from `AudioAvailable()`; segment durations from `SegmentDTO.Duration`.

- [ ] **Step 4: Run tests, goldens, commit**

Run: `go test ./pkg/desktop/ -v && mise run desktop:snapshots`
Expected: PASS.

```bash
git add pkg/desktop
git commit -m "feat(gui): add the live theatre screen"
```

---

### Task 5: Retire the old rasteriser and verify parity

**Files:**
- Delete: `pkg/scene/render.go`, `pkg/scene/render_test.go` (only after nothing imports them)
- Test: none new.

**Interfaces:**
- Consumes: Tasks 1-4.
- Produces: no remaining user of `scene.Renderer`.

- [ ] **Step 1: Confirm nothing uses the old renderer**

Run: `rg -n 'scene\.(NewRenderer|Renderer|FrameRequest)' --glob '*.go'`
Expected: no matches outside `pkg/scene/render.go` itself.

- [ ] **Step 2: Delete and verify**

```bash
git rm pkg/scene/render.go pkg/scene/render_test.go
go build ./... && go vet ./...
```

Expected: clean.

- [ ] **Step 3: Compare a video frame to the live golden**

Run: `mise run test`
Expected: PASS (re-run if the `pkg/gui` flake triggers).

Run: `go run ./cmd/localrpg export video --dir <dir> <game-id> --out /tmp/out.mp4 --still`
Expected: ffmpeg produces an MP4 whose still frames match the theatre view's layout for the same beat. Eyeball one frame against `pkg/desktop/testdata/snapshots/theater.png`.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(scene): retire the bespoke rasteriser"
```

---

## Self-Review

**Spec coverage (Phase 7, §5):**

| Spec item | Task |
| --- | --- |
| `pkg/theater` shared scene/time view | Task 1 |
| live `pkg/desktop/theater` | Task 4 |
| `pkg/export/video.go` re-rastered | Task 3 |
| `RenderToImage` avoided for export scale | Task 2 |
| `pkg/scene` kept as model, rasteriser retired | Task 5 |
| `ImageLightbox` | already delivered in Plan 5 |

**Accepted trade (§5):** anything the live view gets from shirei's implicit animation must be expressed as a function of `Progress` to appear in the export. The shared view reads only `Frame`, so this is enforced structurally.

**Placeholder scan:** No TBDs. Task 1 notes the mirrored-sprite deviation explicitly rather than pretending parity. Task 2 instructs the implementer to confirm the `SoftRenderer.Render` signature; factual verification.

**Type consistency:** `Frame`, `View`, `BeatAt`, `BeatProgress`, `RenderFrame`, `WriteFramePNG`, `theaterScope`, `theaterScreen`, `advanceBeat`, `changeTurn`, `togglePlay` are each defined once and used consistently. `frameWriter` keeps its name; its `renderer` field is removed.

**Known deferrals (not gaps):** portrait mirroring, per-speaker sprite selection beyond the active segment, and cross-fades are simplified in v1; teardown is the final plan.
