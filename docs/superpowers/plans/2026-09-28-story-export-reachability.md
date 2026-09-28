# Story Export Reachability Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-28. `go build ./...`, `go test ./...`, `go vet ./...`, and `frontend npx tsc --noEmit` all pass. Commits are left uncreated per the session rule.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the finished web and video export startable, observable, and cancellable from inside the app, give export its own per-campaign output directory, detect missing ffmpeg up front, and make `preferences.cinematic_effects` do what it says.

**Architecture:** A structured `scene.Progress` value replaces free-form progress strings and threads through `ScriptCompiler` and `VideoPipeline`. A new `Service.StartExport` runs a serialised per-campaign background job, publishing `ExportEvent`s over an SSE stream that mirrors the existing model-download events. Export artifacts land in `<game>/exports/` via `PathResolver`. A small `CinematicOverlay` component honours the existing preference.

**Tech Stack:** Go 1.27 (stdlib `testing`, `t.TempDir()`, `net/http/httptest`, no testify), React 19 + TypeScript.

**Spec:** `docs/superpowers/specs/2026-09-28-story-export-reachability-design.md`

## Global Constraints

- Use `interface{}`, never `any`; `go vet` clean.
- Standard library only in tests; no testify.
- Routes must be added to `routePattern` (`pkg/gui/server.go:50-82`).
- `frontend/` `tsc --noEmit` must pass (`strict`, `noUnusedLocals`).
- No new dependencies.
- **Deviation from the spec:** the export destination is **required and user-chosen**, not a server-computed `<game>/exports/` directory. The request carries `out_dir`; the UI defaults it to the XDG Videos folder (then Documents, then home) and offers a native Wails directory dialog where one exists, falling back to a text field in browser/socket mode. An existing target gets a timestamp suffix so an export never overwrites a previous one. `core.PathResolver.ExportsDir` was removed as dead code.

---

### Task 1: Structured progress in `pkg/scene` and `pkg/export`

**Files:**
- Create: `pkg/scene/progress.go`
- Modify: `pkg/scene/compile.go` (`Options`, `Compile`)
- Modify: `pkg/export/script.go`, `pkg/export/video.go`
- Test: `pkg/scene/progress_test.go`, `pkg/export/progress_test.go`

**Interfaces:**
- Produces: `scene.Progress{Phase string; Done, Total int; Message string}`,
  `scene.ProgressFunc func(Progress)`, `(*ScriptCompiler).SetProgress`,
  `(*VideoPipeline).SetProgress`, `export.FFmpegAvailable() (string, bool)`.

- [x] **Step 1: Add the progress type and emit it from `Compile`**

```go
// pkg/scene/progress.go
package scene

type Progress struct {
	Phase   string // "compile","frames","encode","done"
	Done    int
	Total   int
	Message string
}

type ProgressFunc func(Progress)

func emitProgress(fn ProgressFunc, p Progress) {
	if fn != nil {
		fn(p)
	}
}
```

In `Options` add `Progress ProgressFunc`. In `Compile`, after each turn emit
`Progress{Phase: "compile", Done: i + 1, Total: len(turns)}`; before returning,
emit the silent-beat note as `Progress{Phase: "compile", Message: ...}` (keep the
existing `OnProgress` call too).

- [x] **Step 2: Thread progress through `ScriptCompiler` and `VideoPipeline`**

`ScriptCompiler` gains a `progress scene.ProgressFunc` field and
`SetProgress(fn scene.ProgressFunc)`. In `Compile`, build `scene.Options` with
`Progress: c.progress` and set `OnProgress` to the stderr adapter **only when
`c.progress == nil`**, so GUI exports do not write to stderr.

`VideoPipeline` gains `progress scene.ProgressFunc` and
`SetProgress(fn scene.ProgressFunc)`; `frameWriter.progress` becomes a
`scene.ProgressFunc`. Emit `frames` with `Done: i+1, Total: len(sc.Scenes)` per
scene, `encode` before `cmd.Run`, and `done` after the rename. When `progress`
is nil, keep the stderr fallback.

Add to `pkg/export/video.go`:

```go
// FFmpegAvailable reports the ffmpeg binary path, or ok=false when it is absent.
func FFmpegAvailable() (string, bool) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", false
	}
	return path, true
}
```

- [x] **Step 3: Tests**

`pkg/scene`: a fake `Source` with three turns and an `Options.Progress` recorder
asserts three monotonic `compile` updates with `Total == 3`.

`pkg/export`: a script with two scenes rendered with `SetProgress` records
`frames` updates ending at `Done == Total`; the legacy stderr path is unchanged
when `SetProgress` is not called.

- [x] **Step 4: Verify**

Run: `go test ./pkg/scene/ ./pkg/export/`
Expected: PASS.

---

### Task 2: Per-campaign exports directory

**Files:**
- Modify: `pkg/core/types.go` (`PathResolver`)
- Test: `pkg/core/paths_test.go` (extend) or a new test

**Interfaces:**
- Produces: `(*PathResolver).ExportsDir(gameID string) string`.

- [x] **Step 1: Add the accessor**

```go
// ExportsDir is where a campaign's generated exports live. It sits inside the
// campaign directory so an export travels with the campaign it came from.
func (p *PathResolver) ExportsDir(gameID string) string {
	return filepath.Join(p.GameDir(gameID), "exports")
}
```

- [x] **Step 2: Test** that `ExportsDir("g")` is `GameDir("g")/exports` and is
  not under any `dist` path.

- [x] **Step 3: Verify** `go test ./pkg/core/`.

---

### Task 3: Export service

**Files:**
- Create: `pkg/gui/export.go` (DTOs, manager)
- Modify: `pkg/gui/service.go` (struct field + wiring)
- Test: `pkg/gui/export_test.go`

**Interfaces:**
- Produces: `ExportRequestDTO`, `ExportJobDTO`, `ExportEvent`,
  `ExportCapabilitiesDTO`, `Service.StartExport`, `CancelExport`,
  `SubscribeExportEvents`, `UnsubscribeExportEvents`, `ExportCapabilities`.

- [x] **Step 1: DTOs**

```go
type ExportRequestDTO struct {
	GameID string `json:"game_id"`
	Format string `json:"format"` // "web" | "video"
	Art    bool   `json:"art"`
	Audio  bool   `json:"audio"`
	Still  bool   `json:"still,omitempty"`
	FPS    int    `json:"fps,omitempty"`
	Size   string `json:"size,omitempty"`
}

type ExportJobDTO struct {
	GameID     string `json:"game_id"`
	Format     string `json:"format"`
	OutputPath string `json:"output_path,omitempty"`
	Running    bool   `json:"running"`
}

type ExportEvent struct {
	GameID     string `json:"game_id"`
	Format     string `json:"format"`
	Phase      string `json:"phase"`
	Done       int    `json:"done"`
	Total      int    `json:"total"`
	Message    string `json:"message,omitempty"`
	OutputPath string `json:"output_path,omitempty"`
	Error      string `json:"error,omitempty"`
}

type ExportCapabilitiesDTO struct {
	FFmpeg     bool   `json:"ffmpeg"`
	FFmpegPath string `json:"ffmpeg_path,omitempty"`
}
```

- [x] **Step 2: Manager**

An `exportManager` on `Service` (field `exports *exportManager`) holds:
- `mu sync.Mutex`, `running map[string]*exportJob`
- `subMu sync.Mutex`, `subs map[chan ExportEvent]struct{}`

`StartExport` validates `Format` (`web`/`video`), rejects a concurrent export for
the same game (`ErrExportInFlight`), creates a context with cancel, records the
job, and `goBackground`s the run:

1. emit `{Phase:"compile"}`; build `export.NewScriptCompiler(s.rootDir)`,
   `SetMedia(req.Art, req.Audio)`, `SetProgress` → fan-out event.
2. output path: `<ExportsDir>/web-<ts>` or `<ExportsDir>/<gameID>-<ts>.mp4`
   (`ts = time.Now().UTC().Format("20060102-150405")`).
3. web: `export.NewWebExporter(s.rootDir).Export(ctx, script, out)`.
   video: `export.NewVideoPipeline(s.rootDir)` with `SetStill/SetFPS/SetSize`,
   `SetProgress`, `RenderVideo`.
4. emit `done` with `OutputPath`, or `error`/`cancelled`.
5. clear the running job.

`CancelExport(gameID)` cancels the job context. `ExportCapabilities` calls
`export.FFmpegAvailable`.

- [x] **Step 3: Tests**

- `StartExport` twice for one game returns `ErrExportInFlight` (use a slow format
  or a hook; simplest is to start a video export with a fake ffmpeg check).
- A cancelled context publishes a `cancelled` event and promots no artifact.
- `ExportCapabilities` reports absent ffmpeg when `PATH` is empty.
- Events fan out to a subscriber channel.

- [x] **Step 4: Verify** `go test ./pkg/gui/`.

---

### Task 4: HTTP routes

**Files:**
- Modify: `pkg/gui/server.go` (`registerRoutes`, `routePattern`,
  `handleExportRoutes`)

**Interfaces:**
- Consumes: Task 3 service methods.

- [x] **Step 1: Register and route**

```go
s.mux.HandleFunc("/api/export", s.handleExportRoutes)
s.mux.HandleFunc("/api/export/", s.handleExportRoutes)
```

`routePattern`: add `/api/export` and map `/api/export/...` to
`"/api/export/{id}"` / `"/api/export/events"`.

- [x] **Step 2: Handler**

- `GET /api/export/capabilities` → `ExportCapabilitiesDTO`.
- `GET /api/export/events` → SSE, mirroring `handleModelsRoutes`'s events loop.
- `POST /api/export` → decode `ExportRequestDTO`, `StartExport`; 202 with
  `ExportJobDTO`, 409 on `ErrExportInFlight`, 400 on a bad format.
- `DELETE /api/export/{gameID}` → `CancelExport`; 202.

- [x] **Step 3: Test** the routes with `httptest`: capabilities JSON; a POST
  starts a job; a second POST returns 409; DELETE cancels; the events endpoint
  emits an event.

- [x] **Step 4: Verify** `go test ./pkg/gui/`.

---

### Task 5: Frontend export modal

**Files:**
- Modify: `frontend/src/types.ts`, `frontend/src/api/client.ts`
- Create: `frontend/src/components/ExportModal.tsx`
- Modify: `frontend/src/App.tsx` (header button + modal)

- [x] **Step 1: Types and client**

Add `ExportRequest`, `ExportJob`, `ExportEvent`, `ExportCapabilities` to
`types.ts`. Add to `APIClient`: `exportCapabilities()`, `startExport(req)`,
  `cancelExport(gameID)`, `subscribeExportEvents(onEvent)` returning a close fn
  (mirror `subscribeModelEvents`).

- [x] **Step 2: `ExportModal.tsx`**

Format toggle (Web/Video), include art/audio, still/fps/size for video,
Start/Cancel, a progress bar from `ExportEvent`, and the final path. Follow the
existing modal pattern (`useMountTransition`, `z-50` overlay). Disable Video with
an explanation when `capabilities.ffmpeg` is false.

- [x] **Step 3: Wire the button**

Add an Export button beside the Theater button in `App.tsx` and render the modal
lazily.

- [x] **Step 4: Verify** `cd frontend && npx tsc --noEmit`.

---

### Task 6: Cinematic effects

**Files:**
- Create: `frontend/src/components/CinematicOverlay.tsx`
- Modify: `frontend/src/App.tsx`, `frontend/src/components/theater/TheaterStage.tsx`

- [x] **Step 1: Overlay component**

A fixed, `pointer-events-none` layer combining a radial vignette and the existing
`.bg-noise` (`index.css:89-91`), gated by a `enabled` prop, honouring
`prefers-reduced-motion` (static grain when reduced).

- [x] **Step 2: Wire the preference**

Render `<CinematicOverlay enabled={config?.preferences?.cinematic_effects ?? false} />`
in the in-game shell and over `TheaterStage`.

- [x] **Step 3: Verify** `cd frontend && npx tsc --noEmit`.

---

### Task 7: Full verification

- [x] **Step 1:** `go build ./...`, `go test ./...`, `go vet ./...`.
- [x] **Step 2:** `cd frontend && npx tsc --noEmit`.
- [x] **Step 3:** `go test ./pkg/gui -update-docs` if any generated docs changed
  (none expected; export routes are not in the config reference).

## Self-Review

- **Spec coverage:** progress §3.2 Task 1; output dir §3.3 Task 2; service and
  events §3.1/§3.4 Task 3; routes §3.1 Task 4; frontend §3.6 Task 5; cinematic
  §3.5 Task 6.
- **Placeholders:** none.
- **Type consistency:** `ExportEvent` fields are used identically in the service,
  the SSE handler, and `types.ts`.
