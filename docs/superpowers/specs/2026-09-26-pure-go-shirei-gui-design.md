# Design Spec: Pure-Go GUI with go-shirei (retiring Wails, the React SPA, the TUI, the HTTP daemon, and the in-app debugger)

**Date:** 2026-09-26
**Status:** Draft for review (rev 2)
**Topic:** Replace the Wails window and the React/Vite SPA with an in-process,
pure-Go immediate-mode GUI built on `go.hasen.dev/shirei`, unify the in-app
Story Theater with video export, drop the unused TUI and the HTTP/socket daemon,
and remove the Node toolchain.
**Research:** `docs/proposals/2026-09-26-wails-to-shirei-feasibility.md`

---

## 1. Executive summary

Wails is imported in exactly one file (`cmd/localrpg/gui.go:16`) and is used
only to open a webview window; every runtime feature of Wails is unused, and
the real GUI backend is already a plain `net/http` handler (`pkg/gui`). The
expensive part of this project is therefore **not** removing Wails, it is
replacing the ~14,700-line React SPA and re-cutting the `gui.Service` boundary
so the desktop app calls the application core in-process.

Because the SPA is the only first-party consumer of the HTTP API, the daemon
that serves it (`pkg/gui/server.go`, `socket.go`, `assets.go`) and its debug
surfaces go with it. The TUI, which is unused, goes too. What remains is one
pure-Go application core, one shirei GUI, and the CLI/export tooling.

Shirei gives us an immediate-mode, pure-Go, flexbox-layout GUI that
cross-compiles for desktop without CGO, plus deterministic software-rendered
headless output (`RenderToImage`/`RenderToPNG`, golden-snapshot tooling) that
lets one set of theatre view functions serve both the live window and the
ffmpeg video exporter.

## 2. Decisions settled by grilling and review

| # | Decision | Choice |
| --- | --- | --- |
| 1 | End state | shirei is the only GUI; Wails, React, Vite, Node, TUI, HTTP daemon, and `pkg/debugger` deleted |
| 2 | Platforms | Desktop only: Linux, Windows, macOS. No mobile, no web in v1 |
| 3 | Audio | Keep `oto` untouched for now; adopt shirei audio (mono) later |
| 4 | Dependency posture | Pin exact shirei version, `vendor/`, thin internal adapter |
| 5 | Theatre goal | One implementation for in-app **and** video-encoded theatre (ultimate goal) |
| 6 | Web/HTTP layer | Delete SPA, assets, daemon, `--port`/`--socket`/`--headless`, Node |
| 7 | Scenario tests | Re-platform onto shirei's UDP drive harness; delete the chromedp driver |
| 8 | Markdown | Only emphasis/italics are needed; parse with goldmark, render to spans |
| 9 | Theatre trade | Export drives `t` explicitly; implicit shirei animation is live-only |
| 10 | Service boundary | Core is transport-free; no HTTP adapter survives |
| 11 | Voice input | **Drop in v1** (STT has never worked properly) |
| 12 | Cutover | Build shirei to parity screen-by-screen; SPA is reference-only; delete at the end |
| 13 | Packaging | `pkg/desktop` for views; mise stays; cross-compile now; defer `shirei_bundle` |
| 14 | Debug surfaces | Drop `pkg/debugger`, the dashboard, and `debug server`; use external OTel collectors via the existing OTLP export + docs |
| 15 | Accessibility | Non-goal for v1; keyboard/focus kept; reduced-motion is a setting |
| 16 | TUI | Dropped entirely; remove `pkg/tui` and `localrpg play` |
| 17 | Go style | Prefer `any` over `interface{}` and current-Go idioms; update `AGENTS.md` |
| 18 | Fidelity | Follow SPA layouts/styling closely; 1:1 pixel parity is **not** required |
| 19 | CLI surface | No `gui` subcommand: the root command boots straight into the shirei GUI |

## 3. Goals and non-goals

**Goals**

- One pure-Go GUI (`pkg/desktop`) over a transport-free application core.
- No CGO on desktop; cross-compiled Linux/Windows/macOS artifacts from one host.
- One shared theatre view used by both the live window and video export.
- Node/Vite/npm gone from the build and from `mise.toml`; TUI and daemon gone.
- Headless, deterministic UI verification (`--png`, golden snapshots).
- Layouts and styling that track the old SPA closely enough to feel like the
  same product, without chasing pixel-for-pixel fidelity.

**Non-goals (v1)**

- Mobile (iOS/Android) and web/WASM targets, even though shirei supports them.
- Replacing `oto`; audio stays exactly as it is until a later, separate step.
- Voice input / microphone capture; the browser STT path is deleted with the SPA.
- Full screen-reader support.
- A daemon/API server of any kind: with the SPA gone there is no first-party
  consumer. External automation moves to shirei's drive harness, which is
  in-process and needs no socket.
- The standalone web export player (`pkg/export/web.go`) stays as-is.

## 4. Architecture

### 4.1 Layer split

```
cmd/localrpg
  gui      -> constructs the core and runs pkg/desktop (shirei window only).
              No Wails, no HTTP, no socket, no SPA, no TUI.
  roll/prompt/tts/image/export/debug  -> unchanged in shape
  play     -> DELETED (TUI)

pkg/gui    -> TRANSPORT-FREE CORE (today's Service, minus HTTP and assets).
              Owns storage, orchestrator, TTS, media, model management.
              Exposes BeginTurn(ctx, ...); no JSON DTOs anywhere.

pkg/desktop-> shirei VIEWS + entry point. Root view, screens, drawers.

pkg/ui     -> THIN ADAPTER over go.hasen.dev/shirei. Re-exports the parts we
              use, our theme, icon glyph aliases, snapshot helpers. Contains
              pre-1.0 churn to one package so an upstream break is a one-file fix.

pkg/theater-> SHARED theatre scene model + pure view functions parameterised
              by (script, t). Consumers:
                pkg/desktop/theater  (live: t from a clock, implicit anim ok)
                pkg/export/video     (offline: t stepped, animations off)

pkg/scene  -> retained: the script/beat model the exporters already use.
pkg/desktop/model (new) -> view-model structs replacing the HTTP DTO ceiling
              of pkg/gui/types.go for the GUI path.
```

Rationale for not renaming `pkg/gui`: it avoids an import churn pass now. The
core is defined by what leaves it (HTTP, DTOs, assets) rather than by a new
name. A later mechanical rename to `pkg/session` is optional and out of scope.

### 4.2 Turn lifecycle in-process

- `BeginTurn(ctx, gameID, request, progress)` returns a `TurnResult` or an
  error. `progress` is the in-process replacement for the NDJSON stream; the
  desktop GUI publishes progress into its app state under `WithFrameLock` and
  calls `RequestNextFrame()`.
- Per-campaign serialisation is preserved as an error (`ErrTurnInFlight`),
  replacing the HTTP 409.
- Nothing is persisted on `ctx` cancellation, matching today's behaviour.
- Action correlation currently arrives as the `X-LocalRPG-Action-ID` request
  header (`pkg/gui/middleware.go:34-53`); in-process it becomes an explicit
  argument/context value on the core call, and `otelhttp` disappears from the
  GUI path. OTel spans stay, created explicitly in the core.

### 4.3 Wails removal

Delete the `application.New`/`app.Run` block in `cmd/localrpg/gui.go:137-158`;
the shirei entry (`pkg/desktop.Run`) takes its place. Delete the
`wails://wails` / `http://wails.localhost` trusted-origin allow-list in
`pkg/gui/middleware.go:15-18`. Drop `github.com/wailsapp/wails/v3` from
`go.mod` and roll `cmd/localrpg/gui.go` back to a single mode: the shirei
window, plus a `--png <path>` snapshot flag for tests.

### 4.4 TUI removal

`pkg/tui` (318 lines) is only consumed by `cmd/localrpg/play.go`, which is
only reached by the `play` subcommand (`cmd/localrpg/main.go:39,67`). Delete
both, and remove `bubbletea`, `lipgloss`, `glamour` (and transitive `chroma`,
`go-osc52`, etc.) from `go.mod`.

**Migration landmine:** `cmd/localrpg/play.go:64-97` reindexes the campaign
and re-resolves the start location at session start. That behaviour must move
into the desktop game-open path (the GUI's `ensureIndexed` covers the index but
not the re-resolve) before the TUI is deleted, or hand-edited campaign content
will stop being picked up.

### 4.5 Daemon removal

Delete `pkg/gui/server.go`, `pkg/gui/socket.go`, `pkg/gui/assets.go`,
`pkg/gui/middleware.go`, `pkg/gui/dist/`, `cmd/localrpg`'s `debug server`
subcommand, and `pkg/driver` (chromedp). The `--port`, `--socket`, and
`--headless` flags go with them. The 29 `pkg/gui` handler tests are either
rewritten against the transport-free core or deleted with the handlers they
test; the underlying `Service` behaviour must keep its coverage.

## 5. Shared theatre and export

The payoff decision (grilling #5) is that the live theatre and the video
exporter share one implementation. Because shirei animation is wall-clock
driven and `RenderToImage` forces `NoAnimate`, "one implementation" means
**shared view functions plus an explicit time model**, not shared implicit
animation:

```go
// pkg/theater
type Frame struct { Script *scene.Script; T time.Duration }
func View(f Frame) // pure shirei view; every animated value is f(T)
```

- **Live** (`pkg/desktop/theater`): `t` advances from a real clock; the view may
  additionally rely on shirei's implicit animation for polish.
- **Offline** (`pkg/export/video.go`): for each output frame, set `t`, call
  shirei `RenderToImage(w, h, func(){ theater.View(f) })`, encode PNG, and feed
  the existing ffmpeg mux (`pkg/export/video.go:123-175`). The existing
  `pkg/scene.Renderer` (`pkg/scene/render.go`) is retired as the rasteriser;
  `pkg/scene` stays as the beat/script model.
- **Accepted trade:** anything the live view gets "for free" from shirei's
  implicit animation must be expressed as a function of `t` to appear in the
  export. This is the explicit cost of the fix and is deliberately accepted.
- The standalone web player (`pkg/export/web.go`) is out of scope and keeps its
  own HTML5 player.

## 6. Markdown and typography

The requirement is narrow: emphasis and italics in prose (codex, narration,
rules). Plan: `pkg/desktop/markdown` parses with **goldmark** into an AST and
emits shirei `Text` + `Span` runs for the inline styles we support.
Headings/tables are not a v1 requirement; unknown nodes render as plain text.
Reuse the existing markdown-stripping logic that feeds TTS
(`pkg/media/speakable.go`) as the reference for what "markdown" means to us, so
the two paths stay aligned. `charmbracelet/glamour` leaves with the TUI.

## 7. Voice input

Dropped in v1 (grilling #11). On SPA deletion:

- Delete `frontend/src/hooks/useVoiceInput.ts` with the rest of the SPA.
- Remove the `/api/stt` route (`pkg/gui/server.go:98,985-1038`) from the active
  surface.
- Retire `pkg/provider/sttwebspeech`; leave `sttwhisperhttp`/`sttwhispercli`
  dormant for a future mic-capture subsystem.
- Note in the changelog that voice input is removed, not silently lost.

## 8. Observability and debugging

- Keep `pkg/telemetry` (OTLP/gRPC export; endpoint from
  `OTEL_EXPORTER_OTLP_ENDPOINT` or config, default `localhost:4317`) and
  `pkg/trace` (file/stderr JSONL, sanitized). Both are transport-free and
  orthogonal to the GUI.
- **Delete `pkg/debugger` entirely** (collector, correlator, report, HTTP
  dashboard, HTML report), the `debug server` subcommand, and the SPA
  `DebugPanel`. In-app debugging is replaced by external OTel tooling: point the
  app at a collector such as `otel-desktop-viewer` (or `otel-tui`, Jaeger, etc.)
  and document how in `docs/debugging.md`.
- The scenario DSL re-platforms onto shirei's UDP drive harness (grilling #7).
  Until then, headless PNG snapshots (`SHIREI_SNAP_REPORT`) provide the
  regression safety net.

## 9. Go style

Adopt current-Go idioms project-wide, ahead of and during this work:

- `any` in preference to `interface{}` (reverses the old `AGENTS.md` rule; the
  file must be updated so agents do not reintroduce `interface{}`).
- Use the standard-library helpers the pinned toolchain provides: `min`/`max`,
  `slices`/`maps`, `for i := range n`, typed `sync/atomic`, `errors.Join`,
  `log/slog`, `clear`. `go vet` stays clean.
- This is a mechanical, repo-wide change; make it its own task/phase so it is
  not entangled with GUI work.

## 10. Build, tooling, and packaging

- **Pin + vendor** shirei at an exact version; commit `vendor/`.
- **`.gitignore` fix required:** line 23 `localrpg` also ignores the
  `cmd/localrpg/` directory, so new files added there are silently untracked.
  Change it to `/localrpg` (root binary only) before adding entry-point code.
- **mise tasks:** remove `build:frontend`, `dev:frontend`, `test:frontend`;
  `build:backend` no longer depends on the frontend. Add `desktop:build`
  (linux/amd64+arm64, windows/amd64, darwin/universal) and a `desktop:png`
  snapshot helper.
- **CLI:** the root command boots the GUI directly; drop the `gui`
  subcommand, `--port`, `--socket`, `--headless`, and the `play`
  subcommand. Root flags: `--dir` (project root) and `--png <path>`
  (headless snapshot). `roll`, `prompt`, `tts`, `image`, `export`, `debug`,
  `version`, and `help` remain subcommands.
- **Packaging:** defer `cmd/shirei_bundle` until there is a release to ship.

## 11. Phasing and screen inventory

The SPA is frozen at the start of Phase 2 and used only as a visual reference.

| Phase | Deliverable | Screens / pieces |
| --- | --- | --- |
| 0 | Foundations, no user-visible change | vendor+pin shirei; `.gitignore` fix; `pkg/ui` adapter; `pkg/desktop` shell with a `--png` golden test; cross-compile mise tasks; Go-style sweep |
| 1 | Core split + removals | `BeginTurn(ctx, …)`; move `play.go` reindex/re-resolve into the desktop open path; delete TUI (`pkg/tui`, `play.go`, `play` cmd); delete daemon/HTTP/driver/`debug server`; rewrite or drop handler tests |
| 2 | First shirei window | root command boots the GUI (`gui` subcommand removed); launcher/hub (`LauncherHub`, `launcher/*`: campaign gallery, hero stage, new-campaign modal, world gallery); drawer shell; app frame; Wails block removed here |
| 3 | Core loop | Chronicle (`ChronicleView`, `TurnHistoryList`, `TurnSegments`, `SegmentAudioControls`, `DiceCheckCard`, `ActionConsole`, `ProloguePanel`); Codex/`CodexDrawer`; `ContextDrawer`, `GraphDrawer`, `LivingWorldDrawer`, `CharacterSheetDrawer`; `MarkdownProse` |
| 4 | Settings | `SettingsStudio`, provider/model catalogue, `ModelDownloadModal`, `VoiceCatalogModal`, `VoiceCatalogPicker`, `VoiceCombobox`, `VoiceOptionsControl`, TTS inspect |
| 5 | Theatre + export | `pkg/theater` shared view; `pkg/desktop/theater` live; `pkg/export/video.go` re-rastered; `ImageLightbox` |
| 6 | Studios & remaining | `SystemsStudio`, `WorldsStudio`, character creation, asset generation; rewrite `docs/debugging.md` for external OTel collectors |
| 7 | Teardown | delete `frontend/`, `pkg/gui/assets.go` remnants, Node from mise; drop Wails/TUI/chromedp deps from `go.mod`; final Node-free `go build` |

## 12. Testing strategy

- **Golden PNG snapshots** per screen via shirei `RenderToPNG`/`Snapshot`,
  committed under `pkg/desktop/<screen>/testdata/snapshots/`.
- **Headless interaction tests** by driving frames directly
  (`ResetInputSession` + `RunFrameFn`), the pattern shirei's own widget tests use.
- **Core tests** (`go test ./pkg/...`) keep covering `Service` behaviour; tests
  that were written against HTTP handlers are rewritten against the core API
  rather than dropped wholesale.
- **Scenario DSL** re-platformed onto shirei's UDP drive harness (grilling #7);
  `SHIREI_SNAP_REPORT` can feed the same review tooling as goldens.
- **`go vet` stays clean**; `any` is now the house style (§9).

## 13. Risks

| Risk | Severity | Mitigation |
| --- | --- | --- |
| shirei pre-1.0 churn (v0.8.0, days old) | High | pin + vendor + `pkg/ui` adapter |
| Rewriting every screen is large | High | phase by screen; SPA stays as reference until parity |
| Shared theatre trade-off leaks implicit animation | Medium | explicit `t` model; golden video frames as the test |
| Shirei a11y weaker than the browser | Medium | accepted non-goal; opportunistic access names |
| `cmd/localrpg` ignored by `.gitignore` | Medium | fix to `/localrpg` before Phase 0 code |
| TUI deletion loses play-start reindex/re-resolve | Medium | port `play.go:64-97` into the desktop open path in Phase 1 |
| Handler-test coverage lost with the daemon | Medium | rewrite `Service` tests against the core, not delete them |
| Loss of in-app debug dashboard | Low | external OTel collectors (otel-desktop-viewer) + `docs/debugging.md` (Phase 6) |
| Loss of voice input | Low | explicit removal note; dormant whisper providers |
| No repo CI | Low | out of scope; cross-compile tasks are manual for now |

## 14. File map (rough)

Create:

- `pkg/ui/` — shirei adapter: re-exports, theme, icon aliases, snapshot helpers.
- `pkg/desktop/` — `Run`, root view, `testdata/snapshots/`, per-screen packages.
- `pkg/desktop/markdown/` — goldmark AST → shirei spans.
- `pkg/desktop/model/` — GUI view-model structs.
- `pkg/theater/` — shared scene/time view.

Modify:

- `cmd/localrpg/gui.go` — root GUI boot; drop Wails block, `--port`/`--socket`/`--headless`; add `--png`.
- `cmd/localrpg/main.go` — no-args boots the GUI instead of printing usage; remove the `gui` case, the `play` subcommand, and their help lines.
- `cmd/localrpg/debug.go` — remove `server`; re-platform `test-run` off chromedp.
- `pkg/gui/service.go` — `BeginTurn(ctx, …)`, transport-free; remove NDJSON.
- `pkg/export/video.go` — rasterise via `pkg/theater` + `RenderToImage`.
- `pkg/scene/render.go` — retired as rasteriser (kept as model).
- `docs/debugging.md` — rewritten for external OTel collectors; scenario/CDP docs removed.
- `.gitignore` — `localrpg` → `/localrpg`.
- `mise.toml` — Node-free tasks, cross-compile, snapshot helper.
- `AGENTS.md` — `any`/modern-Go convention; remove TUI/daemon from the description.
- `go.mod` / `go.sum` / `vendor/` — pin + vendor shirei; drop Wails, TUI, chromedp.

Delete:

- `frontend/`, `pkg/gui/assets.go`, `pkg/gui/server.go`, `pkg/gui/socket.go`,
  `pkg/gui/middleware.go`, `pkg/gui/dist/`, `pkg/gui/types.go` (HTTP DTOs),
  `pkg/tui/`, `cmd/localrpg/play.go`, `pkg/driver/`, `pkg/debugger/`,
  `pkg/provider/sttwebspeech`, `/api/stt` route.

## 15. Open questions

1. `shirei_bundle` packaging: adopt when the first release is cut, or keep a
   plain tarball/zip?
2. Do we ever want the WASM target as the "web again" route, or is web
   permanently out of scope?
3. Should `pkg/gui` be renamed to `pkg/session` once the split settles?
4. Audio follow-up: shirei's mono device and process-global `StartAudio` mean
   `playback.Player`'s lazy `Open`/`Available`/`Stop` semantics must be
   redesigned; schedule as a separate spec after the GUI lands.
5. Go-style sweep: fold the `interface{}` → `any` change into Phase 0, or land
   it as its own standalone commit/PR first?
