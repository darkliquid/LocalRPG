# Feasibility: replacing Wails (and React) with go-shirei

**Date:** 2026-09-26
**Status:** Research memo (pre-spec). Superseded in places by the approved direction in `docs/superpowers/specs/2026-09-26-pure-go-shirei-gui-design.md`: the TUI and the HTTP/socket daemon are now also being dropped, and the Go style moves to `any`.

**Question:** Can LocalRPG drop Wails entirely for `go.hasen.dev/shirei`, and
can shirei also replace `oto` for audio output?

Sources are primary (the shirei source tree at `master`, LocalRPG source).
Line references are for the versions checked on 2026-09-26.

---

## 1. How much Wails do we actually use?

Wails is imported in **exactly one Go file**: `cmd/localrpg/gui.go:16`. Symbols
used: `application.New`, `application.Options`, `application.AssetOptions`,
`app.Window.NewWithOptions`, `application.WebviewWindowOptions`,
`application.BackgroundTypeTranslucent`, `app.Run` (`cmd/localrpg/gui.go:137-158`).

- **No runtime features at all**: no events, dialogs, clipboard, tray, menus,
  notifications, screen info, single-instance, or generated JS bindings.
- The frontend never imports `@wailsio/runtime`; it talks to the backend purely
  by `fetch` (`frontend/src/api/client.ts`).
- The Wails app is handed a plain `http.Handler` built at
  `cmd/localrpg/gui.go:85` (`gui.ProtectCrossOrigin(gui.NewServer(svc, gui.AssetHandler()))`)
  and does nothing with it except serve the SPA inside its webview.
- The only Wails leak into non-Wails code is a trusted-origin string allow-list
  in `pkg/gui/middleware.go:15-18` (`wails://wails`, `http://wails.localhost`).

Meanwhile the whole application already runs **without** Wails through two
other branches of the same function: `--port N` TCP (`cmd/localrpg/gui.go:87-96`)
and the Unix-socket/headless daemon (`cmd/localrpg/gui.go:98-134`,
`pkg/gui/socket.go`). The real GUI backend is 100% `net/http` plus
`pkg/gui/service.go` (3,245 lines), `server.go` (1,245), `types.go` (531) and 29
`_test.go` files that never touch Wails.

**Conclusion:** removing Wails is low-risk and mostly mechanical. The hard part
is not Wails; it is deciding what replaces the **React SPA** (54 files, ~14,700
lines of TS/TSX) and the HTTP DTO boundary.

## 2. What shirei is

`go.hasen.dev/shirei`, zlib licence, 627 stars, latest release **v0.8.0 on
2026-09-24** (pre-1.0, released two days before this memo; a
`docs/migration-v0.8.0.md` exists, so API churn is real). Active (pushed
2026-09-24). 42 non-test Go files in the root package; `go 1.25.0`.

- Immediate-mode, pure-Go, flexbox-like layout. UI is a function of state; no
  HTML/CSS/JS, no callbacks, no virtual DOM.
- Platforms: Windows, macOS, Linux (Wayland + X11), iOS, Android, and web
  (WASM/WebGL2 via `app/app_js.go`).
- **Desktop cross-compiles without CGO, including macOS**; iOS still needs CGO.
- `cmd/shirei_bundle` produces IPA, release APK, macOS `.app`/zip/pkg, Linux
  `.tar.gz`, Windows `.zip`; `cmd/shirei_mobilerun` for dev device installs.
- Widgets: Button, CheckBox, OptionButton, ToggleSwitch, Slider, TextInput,
  PasswordInput, Table (sortable + virtualized), VirtualListView, LogView,
  LargeText, FileSelector/DirectoryBrowse, Modal, Menu, PopupPanel, Toast,
  Tabs, segmented controls, drag-and-drop, progress, `Link`/`OpenURL`.
- Text: bundled Typicons + Microns icon fonts, system fonts with per-rune
  fallback, complex shaping, bidi, IME.
- Images: `images.go`, `scaledimage`, image-list/image-viewer/lightbox demos.
- Verified-good tooling for AI/humans: `--png` headless snapshots baked into
  examples, `RenderToImage`/`RenderToPNG`, golden snapshot infra (`snap.go`),
  `SHIREI_LAYOUT_WARN=1`, an `ext/window` extension, a UDP "drive" harness for
  windowed behavior tests (`docs/drive.md`, `docs/drive-tutorial.md`),
  `examples/markdown_viewer`, and a strong tutorial corpus in `docs/`.
- **Limitations it states itself:** one window with standard decorations; no
  app-owned GPU surfaces (no video/custom 3D); screen-reader support is basic
  and only on macOS / Linux Wayland / 64-bit Windows, covering static text,
  buttons, toggles, sliders.

## 3. Headless rendering and theatre mode

This is the strongest technical argument for shirei, but with a caveat.

- `RenderToImage(w,h,fn)` (`renderpng.go`) runs several frames until layout
  settles, **from a neutral input state, with animations forced off**
  (`NoAnimate`), and software-rasterises the result via `SoftRenderer` at
  `HeadlessScale = 2`. `RenderToPNG` wraps it.
- Output is **deterministic** (caret blink and wall-clock visuals suppressed by
  `HeadlessRender`), which is exactly what offline video frame generation wants.
- `RunFrameFn` is exported and input is data (`InputState`/`FrameInput`), so a
  renderer can step frames itself with no window and no GPU.

Caveat: **the SPA is not what renders video today.** `localrpg export video`
already uses its own pure-Go renderer (`pkg/scene`, 1,428 lines) plus
`ffmpeg` (`pkg/export/video.go`), and `export web` writes a standalone static
HTML+JS player (`pkg/export/web.go`). So shirei does not "unlock" video export;
it offers a path to *unify* the in-app GUI and the export renderer on one
immediate-mode scene model. That is a nice end state, not a v1 requirement.

## 4. Audio: can shirei replace oto?

Current stack: `github.com/ebitengine/oto/v3` + `github.com/gopxl/beep` for
device playback (`pkg/media/playback/player.go`); decode is separate
(`pkg/media/audiodecode.go` go-mp3, `pkg/media/opus/` pion/opus Ogg).
Playback is lazy (`playback.Open` at `pkg/gui/service.go:1750`), exposes
`Available`/`Playing`/`SetVolume`/`Stop`/`Close`, and the browser is the
fallback when no device exists.

Shirei's audio:

- `app.StartAudio(sampleRate int, fill func([]float32))` opens the default
  device and pulls **mono float32** forever. Per-OS: ALSA (Linux), AudioQueue
  (macOS), waveOut (Windows). Linux path is `purego`-dlopened `libasound` —
  **no CGO**, consistent with the current CGO-free build.
- `shirei/audio` is a pure-Go `Mixer` of `Voice`s. `StreamVoice` is a blocking
  ring writer with declicked underrun handling and free pause; **decoding
  formats is explicitly out of scope** — the app's business, which we already
  own.
- Offline rendering: `Mixer.Fill` works with no device, and `audio.WriteWAV`
  writes the result — a headless audio path matching the headless video path.
- Latency targets ~17ms macOS / ~30ms Linux / ~46ms Windows; darwin+windows
  self-heal after sleep, Linux recovers in-loop.

Friction points:

1. **Mono only.** The docs state stereo "is a platform-boundary change, not a
   mixer one". We currently run 48 kHz stereo s16 (Opus). Downmix is easy for
   speech; stereo music would be lost.
2. **Process-global singleton, no `StopAudio`.** `StartAudio` is once per
   process and there is no stop. Our player is lazy, reopenable, and has a
   real `Stop`/`Close` plus an `Available` probe. The seam must be redesigned
   (start at GUI launch; `Stop` becomes "silence the mixer", not "close the
   device").
3. **Coupled to the `app` package.** Audio lives beside the windowing layer.
   If we keep a TUI/CLI that wants sound without a GUI, this pulls in `app`'s
   platform backends (lazy `dlopen`, so linking is fine, but it is a
   structural coupling to weigh).
4. `StartAudio` is startup-time, matching `SetupWindow`; it is not a thing to
   call repeatedly.

**Verdict:** shirei audio is viable for the GUI, especially once the GUI itself
is shirei. It is *not* a drop-in for the existing `playback.Player` semantics,
and mono is a real regression to accept or route around.

## 5. Cost inventory for the GUI rewrite

- Delete/retire `frontend/` (~14,700 LOC TS/TSX) or keep it as the browser/web
  target.
- Re-home the DTOs in `pkg/gui/types.go` (531 lines) into UI-facing model
  structs; `server.go` (1,245) + 29 handler tests either stay (serving web /
  daemon) or become dead weight.
- `service.go` (3,245) stays; the shirei views call it in-process instead of
  over HTTP. That removes the NDJSON turn-streaming transport for the desktop
  app (turns update shared state + `RequestNextFrame()` instead).
- Build: `pkg/gui/assets.go`'s `//go:embed all:dist` and the whole
  `mise build:frontend`/Node toolchain become optional if the SPA is retired.
  `mise.toml` would grow shirei build/bundle tasks.
- Markdown prose: shirei has **no markdown widget**; `examples/markdown_viewer`
  is a complete example app with its own parser (`document.go`) and view
  (`view.go`, `theme.go`). Codex notes, rules, lore, and narration all render
  Markdown today, so this is a non-trivial port or a new renderer over `Text` +
  `Span` ranges.
- `middleware.go`'s webview-origin special case disappears. `ProtectCrossOrigin`
  stays useful only for the browser/daemon modes.
- VN theatre with sprites, cross-fades, a background image, and a dialogue box
  is straightforward in shirei (images + `PopupPanel`/z-order + animation), but
  it is a from-scratch rebuild of every screen, not a port.

## 6. Risk register

| Risk | Severity | Note |
| --- | --- | --- |
| Pre-1.0 API churn (v0.8.0, 2 days old) | High | pin + vendor; wrap in an internal adapter |
| Mono-only audio | Medium | downmix speech; stereo music lost |
| Audio singleton vs lazy/reopenable player | Medium | redesign the playback seam |
| No app-owned GPU surfaces | Low | we have no video/3D in the GUI today |
| One window only | Low | matches current behaviour |
| Markdown-rendering port | Medium | 14k-LOC frontend already solves it in TSX |
| Screen-reader support limited | Low/Medium | browser SPA is currently the accessible path |
| No repo CI / no cross-compile today | Low | shirei makes cross-compile *easier*, not harder |
| `app` package coupling for audio | Low/Medium | only if TUI/CLI needs device audio |

## 7. What is genuinely easy

- Dropping the Wails window (`cmd/localrpg/gui.go`) in favour of shirei's
  `app.SetupWindow`/`app.Run`, or even of just launching the system browser.
- Deleting the `wails://` origin allow-list.
- Keeping the HTTP API and Unix-socket daemon exactly as they are.
- Keeping the TUI and CLI untouched (they do not use Wails or the SPA).
- Wiring shirei's headless `RenderToImage` into a future unified export.

## 8. What is genuinely hard

- Rebuilding all GUI screens in immediate mode (settings/providers, systems and
  worlds studios, codex, chronicle, theatre, launcher, character creation,
  asset generation) — the bulk of the work.
- Markdown/prose rendering for codex and narration.
- Redesigning audio around shirei's singleton, mono device.
- Deciding the fate of the 14,700-LOC SPA and its type-safety/build gate, and
  of the 29 HTTP handler tests.
- Absorbing pre-1.0 churn from a very young dependency.
