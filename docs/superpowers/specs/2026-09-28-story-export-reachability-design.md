# Story Export Reachability & Cinematic Effects Design

**Date:** 2026-09-28
**Status:** Proposed
**Scope:** Make the finished web and video export reachable from inside the app with progress and cancellation, give export its own output directory, detect ffmpeg up front, and make `preferences.cinematic_effects` do what it says
**Related:** `pkg/export` (`script.go`, `web.go`, `video.go`), `pkg/scene` (`compile.go`), `pkg/gui` (`service.go`, `server.go`, `types.go`), `pkg/core` (`PathResolver`), `pkg/paths`, `cmd/localrpg/export.go`, `frontend/src`; builds on `docs/superpowers/specs/2026-09-21-animated-export-and-video-design.md`, `docs/superpowers/specs/2026-09-26-theater-native-audio-and-visual-novel-layout-design.md`, and `docs/superpowers/specs/2026-09-20-global-settings-panel-design.md`

## 1. Overview & Goals

The export pipeline is complete and tested, but it is only reachable from the
command line: `localrpg export <web|video>` (`cmd/localrpg/main.go:47-48`,
`cmd/localrpg/export.go`). Nothing in `pkg/gui` exposes it and the frontend has
zero references to export. A user who plays a campaign in the desktop app has no
way to produce the story replay the app is capable of.

Two smaller inconsistencies sit alongside it:

- `preferences.cinematic_effects` is a no-op. It is defined, defaulted to `true`
  (`pkg/config/types.go:212,458`), typed (`frontend/src/types.ts:603`), and
  toggled in Settings (`SettingsStudio.tsx:2815-2837`), but no renderer reads it.
- Export progress is unstructured. `scene.Options.OnProgress` and the video
  pipeline's callback take `(format string, args ...interface{})`
  (`pkg/export/script.go:164`, `pkg/export/video.go:83`), which cannot be turned
  into a progress bar or an event stream.

**Goals:**

- Start, observe, and cancel a web or video export from the running app.
- Give export a dedicated output location that does not collide with the SPA
  build output in `dist/`.
- Detect a missing `ffmpeg` before the user starts a video export and explain it.
- Replace free-form progress strings with a small structured progress type.
- Make `cinematic_effects` either functional or removed, not decorative.

**Non-Goals:**

- Changing what web/video export produces, or the timeline-driven content model.
- Cloud upload, sharing links, or a non-`ffmpeg` video encoder.
- A generic job framework for the whole service; this spec reuses the existing
  background-work primitives.
- Editing a previously exported artifact.

**Success Criteria:**

- From the theater (and launcher), a user can choose Web or Video, watch
  progress, cancel mid-run, and be told where the artifact landed.
- A second export for the same campaign while one is running is rejected with a
  clear 409, matching the turn-serialisation convention.
- Video export is unavailable, with an explanation, when `ffmpeg` is not found.
- Export output defaults under a per-campaign exports directory, never `dist/`.
- Toggling `cinematic_effects` in Settings visibly changes the in-game/theater
  rendering, and honours `prefers-reduced-motion`.

## 2. Investigation Findings

- **Public export API.** `export.NewScriptCompiler(rootDir)`
  (`pkg/export/script.go:88`), `SetMedia(art, audio bool)` (`:105`),
  `Compile(ctx, gameID) (*scene.Script, error)` (`:111`) builds the media stores
  itself (`:130-157`). `export.NewWebExporter(rootDir).Export(ctx, script, outDir)`
  (`pkg/export/web.go:20,48`) writes a self-contained HTML bundle. `export.NewVideoPipeline(rootDir)`
  (`pkg/export/video.go:28`) with `SetSize/SetFPS/SetStill` (`:33,40,48`) and
  `RenderVideo(ctx, script, outputFile)` (`:180`) checks `exec.LookPath("ffmpeg")`
  and returns `"ffmpeg is required for video export: …"` (`:185-187`), renders to
  a temp frame dir, and atomically renames a `.part` file (`:218-234`).
- **CLI wiring and defaults.** `cmd/localrpg/export.go:15-92`. Web default target
  `dist/<gameID>-web` (`:57`); video default `dist/<gameID>.mp4` (`:70`);
  `--no-art/--no-audio/--still/--fps/--size` map to the setters. `/dist/` is
  gitignored (`.gitignore:5`) and is also where Vite writes the embedded SPA
  (`frontend/vite.config.ts` → `pkg/gui/dist`), so reusing it for user artifacts
  mixes build output with user data.
- **No route, no client, no button.** `pkg/gui/server.go:91-108` is the full
  route table; there is no export path. `frontend/src/api/client.ts` has no
  export method and `App.tsx` has no export trigger. `grep -ri export frontend/src`
  returns only ES-module keywords.
- **Established long-running patterns.** Model download: `POST` returns 202 and
  runs via `Service.goBackground` (`pkg/gui/service.go:192-200`), with
  `SubscribeModelEvents`/`UnsubscribeModelEvents` (`:202-208`) feeding an SSE
  route `GET /api/models/events` (`pkg/gui/server.go:1249-1276`). Turn streaming:
  NDJSON via `handleTurnSubmit` (`server.go:1171-1236`). There is no generic job
  framework. `goBackground` tracks work with a `sync.WaitGroup` and `Close()`
  waits (`service.go:143-186`). New routes must be added to `routePattern`
  (`server.go:50-82`) for span naming.
- **Unstructured progress.** `scene.Options.OnProgress` is the only hook
  (`pkg/export/script.go:164-166`) and it formats to stderr; the video pipeline's
  `w.progress` is the same shape (`pkg/export/video.go:83,205`). There is no phase
  or percentage.
- **`cinematic_effects` is dead.** References are exactly: struct
  (`pkg/config/types.go:212`), default (`:458`), TS type (`types.ts:603`), toggle
  (`SettingsStudio.tsx:2815-2837`), doc (`pkg/gui/docs/13-configuration-reference.md:173`).
  No consumer. Existing reusable pieces: `.bg-noise` SVG turbulence at 3.5%
  opacity, defined but unreferenced (`frontend/src/index.css:89-91`); a fixed
  dark scrim in `App.tsx:555-556`; vignette gradients in
  `launcher/CampaignHeroStage.tsx:72-74` and `launcher/ProceduralAsset.tsx:83-84`;
  the theater backdrop `theater/TheaterStage.tsx:13`.

## 3. Design

### 3.1 Export as a serialised background job

Mirror the model-download pattern, per campaign:

```go
// pkg/gui
type ExportRequestDTO struct {
    GameID  string `json:"game_id"`
    Format  string `json:"format"`            // "web" | "video"
    Art     bool   `json:"art"`
    Audio   bool   `json:"audio"`
    Still   bool   `json:"still"`
    FPS     int    `json:"fps,omitempty"`
    Size    string `json:"size,omitempty"`    // "1920x1080"
    OutDir  string `json:"out_dir,omitempty"` // optional override
}

type ExportJobDTO struct {
    GameID   string `json:"game_id"`
    Format   string `json:"format"`
    OutputPath string `json:"output_path,omitempty"`
    Running  bool   `json:"running"`
}

type ExportEvent struct {
    GameID   string `json:"game_id"`
    Format   string `json:"format"`
    Phase    string `json:"phase"`   // "compile","art","audio","frames","encode","done","error","cancelled"
    Done     int    `json:"done"`
    Total    int    `json:"total"`
    Message  string `json:"message,omitempty"`
    OutputPath string `json:"output_path,omitempty"`
    Error    string `json:"error,omitempty"`
}
```

- `Service.StartExport(ctx, req) (*ExportJobDTO, error)` validates the request
  (known format, art/audio at least one of them if both disabled is allowed but
  warned), takes a per-campaign export lock (separate from the turn lock so a
  turn can still be played while exporting, or shared if simpler — decide in the
  plan), and starts work with `goBackground`. A second start while running for
  the same campaign returns a conflict.
- `Service.CancelExport(gameID)` cancels the job's context; `RenderVideo` and
  `WebExporter.Export` already accept `ctx`.
- `Service.SubscribeExportEvents()/UnsubscribeExportEvents(ch)` fan out
  `ExportEvent`s, matching the models pattern.
- Routes:
  - `POST /api/export` → 202 `ExportJobDTO` or 409 when running.
  - `DELETE /api/export/{gameID}` → cancel.
  - `GET /api/export/events` → SSE `ExportEvent` stream.
  - `GET /api/export/capabilities` → `{ "ffmpeg": bool, "ffmpeg_path": string }`.
  Add each to `routePattern`.

### 3.2 Structured progress

Add a small progress type to `pkg/scene` and thread it through:

```go
// pkg/scene
type Progress struct {
    Phase string // "compile","art","audio","frames","encode"
    Done  int
    Total int
}
type ProgressFunc func(Progress)
```

- `scene.Options` gains `Progress ProgressFunc`; keep `OnProgress` as a
  deprecated adapter that formats `Progress` to the existing string callback, so
  the CLI keeps working unchanged.
- `ScriptCompiler.SetProgress(fn ProgressFunc)` passes it into `Compile`.
- `VideoPipeline.SetProgress(fn ProgressFunc)` replaces the internal
  `w.progress` string callback; emit `frames` progress per scene and `encode`
  once ffmpeg starts.
- `StartExport` converts each `Progress` into an `ExportEvent`.

### 3.3 Output directory

Add an exports location resolved like every other path:

- `pkg/paths`: an `Exports` category (default under the games/data home, e.g.
  `<DataHome>/localrpg/exports`), with the same absolute/relative rules as the
  other categories (`pkg/paths/resolve.go`).
- `core.PathResolver`: `ExportsDir() string` and
  `ExportDir(gameID, format string) string` returning
  `<ExportsDir>/<gameID>/<web|video>-<UTC timestamp>` so repeated exports do not
  overwrite each other. Honor `ExportRequestDTO.OutDir` only when it is absolute
  and inside `ExportsDir` (or explicitly allow any absolute path, documented).
- Web export writes a directory; the GUI shows the path and, for the desktop
  app, offers a "Reveal" action through the existing Wails shell if available.
  A zip of the bundle is a possible follow-up, not required.

### 3.4 ffmpeg detection

`Service.ExportCapabilities()` runs `exec.LookPath("ffmpeg")` once (cached) and
returns the result. The frontend disables the Video option with the returned
explanation when `ffmpeg` is absent, instead of failing after a long render.

### 3.5 Cinematic effects

Implement the setting rather than removing it, using assets already present:

- New `frontend/src/components/CinematicOverlay.tsx`: a fixed, pointer-events-none
  layer combining a radial vignette and the existing `.bg-noise` texture, with
  opacity driven by a prop.
- Render it in the in-game shell and in `TheaterStage`, gated by
  `config.preferences.cinematic_effects`.
- Respect `prefers-reduced-motion`: static overlay only, no animated grain.
- Keep the Settings copy accurate; if the implementation ships without film
  noise, update the description to say vignette only.

If the team prefers not to maintain an overlay, the alternative is to remove the
setting, its default, its DTO field, the toggle, and the doc entry. This spec
recommends implementing it, because the setting is already documented and the
building blocks exist.

### 3.6 Frontend

- New `ExportModal.tsx` following the existing modal pattern
  (`useMountTransition` + `z-50` overlay, as in `NewCampaignModal.tsx:228`).
  Trigger from the in-game header beside the Theater button
  (`App.tsx:647-654`) and optionally from the launcher dock
  (`launcher/LauncherDock.tsx:125-182`).
- Controls: format toggle (Web/Video), include art/audio, still/fps/size for
  video, Start/Cancel, a progress bar from `ExportEvent`, and the final path with
  a reveal action.
- `APIClient` gains `startExport`, `cancelExport`, `subscribeExportEvents`,
  `exportCapabilities`; `types.ts` mirrors the DTOs and `ExportEvent`.
- The Video option is disabled with an explanation when capabilities report no
  `ffmpeg`.

## 4. Interfaces

```go
// pkg/scene
type Progress struct { Phase string; Done, Total int }
type ProgressFunc func(Progress)
// scene.Options gains: Progress ProgressFunc

// pkg/paths
// Resolve gains an Exports category.

// pkg/core
func (p *PathResolver) ExportsDir() string
func (p *PathResolver) ExportDir(gameID, format string) string

// pkg/export
func (c *ScriptCompiler) SetProgress(fn scene.ProgressFunc)
func (v *VideoPipeline) SetProgress(fn scene.ProgressFunc)

// pkg/gui
func (s *Service) StartExport(ctx context.Context, req ExportRequestDTO) (*ExportJobDTO, error)
func (s *Service) CancelExport(gameID string)
func (s *Service) SubscribeExportEvents() chan ExportEvent
func (s *Service) UnsubscribeExportEvents(ch chan ExportEvent)
func (s *Service) ExportCapabilities() ExportCapabilitiesDTO
```

HTTP: `POST /api/export`, `DELETE /api/export/{gameID}`,
`GET /api/export/events`, `GET /api/export/capabilities`.

## 5. Error Handling

| Situation | Behaviour |
| --- | --- |
| Unknown format | 400 with the accepted values (`web`, `video`) |
| Export already running (same campaign) | 409 `ExportJobDTO` describing the running job |
| `ffmpeg` missing | Video export rejected before rendering; `capabilities` reports it |
| Art/audio resolved to nothing | Export still succeeds; event `message` notes the degradation (matches `scene`'s silent audio degradation) |
| Provider failure during art/audio | Export continues where `scene` degrades; a hard compile error reports an `error` event and no partial artifact is promoted |
| Cancel mid-run | Job context cancelled; temp frames dir is removed; no `.part` rename; `cancelled` event |
| Server shutdown mid-export | `goBackground`/`Close` waits; the render is cancelled and temp files cleaned |

The export writes to a staging path and promotes to the final path only on
success, matching `video.go`'s `.part` convention; the web bundle writes into a
temp directory and renames it into place.

## 6. Testing & Verification

Go (stdlib `testing`, `t.TempDir()`):

- `pkg/export`: `SetProgress` emits `frames` progress with monotonic `Done` and a
  final `encode` phase; `OnProgress` (legacy) still receives formatted strings.
- `pkg/gui`: `StartExport` twice for one campaign returns a conflict; a cancelled
  context yields no promoted artifact; `ExportCapabilities` reports absent
  `ffmpeg` when `PATH` is empty.
- `pkg/gui`: SSE handler emits `done` with the output path on success and
  `error` on failure; the route is registered in `routePattern`.
- `pkg/core`/`pkg/paths`: `ExportDir` is under the resolved exports category and
  never under `dist/`; two exports get distinct directories.
- `pkg/scene`: the deprecated `OnProgress` adapter formats `Progress` as before
  (protects the CLI's output).

Frontend: `tsc` only. Verify the modal by inspection and a manual run: start a
web export, cancel a video export, and toggle cinematic effects in both the
in-game shell and the theater.

Manual/CLI parity: run `localrpg export web <id>` and the in-app web export for
the same campaign and compare bundle contents.

## 7. Compatibility & Rollout

- The CLI is unchanged: `SetProgress`/`OnProgress` both remain, and the CLI keeps
  using the string callback.
- The new exports directory is additive; no migration of existing `dist/` output.
- `cinematic_effects` becomes functional without a config version change.
- Adding routes updates `routePattern`, the route table docs, and the appendix in
  `docs/architecture/review-2026-09-24.md` if it is kept current.

## 8. Open Questions

- Should export share the per-campaign turn lock (simple, blocks play during a
  long video render) or take its own lock (allows play, more concurrency)?
- Should the web bundle be zipped for portability, and should the app serve it
  over HTTP for immediate viewing?
- Does the desktop Wails build expose a "reveal in file manager" primitive, or is
  copying the path enough?
- Should export run from the launcher (campaign card) as well as the theater?
- Is `cinematic_effects` worth maintaining, or should it be removed instead?

## 9. References

- Code: `pkg/export/script.go:88-174`, `pkg/export/web.go:20-116`,
  `pkg/export/video.go:19-236`, `pkg/scene/compile.go:22`,
  `cmd/localrpg/export.go:15-114`, `pkg/gui/server.go:50-108,1238-1295`,
  `pkg/gui/service.go:143-208`, `pkg/core/types.go:83-148`,
  `frontend/src/index.css:89-91`, `frontend/src/App.tsx:555-556,647-654`,
  `frontend/src/components/SettingsStudio.tsx:2815-2837`.
- Specs: `2026-09-21-animated-export-and-video-design.md`,
  `2026-09-26-theater-native-audio-and-visual-novel-layout-design.md`,
  `2026-09-20-global-settings-panel-design.md`.
