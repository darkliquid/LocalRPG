# Pure-Go Video Export: Theatre-Accurate WebM

**Date:** 2026-10-01
**Status:** Proposed
**Scope:** Replace the ffmpeg-based video exporter with a pure-Go one that composites
the story theatre's own look frame by frame and muxes VP8 video with the campaign's
Opus audio into a single `.webm`, with no external binary on any platform.
**Supersedes:** the frame-sequence + `ffmpeg` pipeline in `pkg/export/video.go`, the
`ffprobe` duration probe on the export path (`pkg/media/probe.go`), and the ffmpeg
capability gate in `pkg/gui/export.go`.
**Related:** `pkg/scene` (`scene.go`, `compile.go`, `render.go`, `timing.go`),
`pkg/media/opus`, `pkg/export/script.go`, `frontend/src/components/story/StoryPlayer.tsx`,
`frontend/src/components/theater/*`, `frontend/src/components/MarkdownProse.tsx`,
`docs/superpowers/specs/2026-09-29-export-player-parity-design.md`.
**Deferred:** injecting the bundled fonts into the web bundle (a natural follow-on,
noted in §9).

## 1. Overview & Goals

The web export ships a self-contained page that plays a campaign through the theatre's
own components. The video export does not: it composites frames in Go with a look of
its own (`pkg/scene/render.go`), writes them as PNGs, and shells out to `ffmpeg` to
encode H.264. The two renders have drifted, the export needs a binary that is not
present on every machine, and the 2026-09-29 parity spec already marks the video as the
one place parity is missing.

This design gives the video exporter the same relationship to the theatre that the web
bundle has: **one look, composited in Go from the theatre's own design, and encoded
entirely in Go.** No ffmpeg, no ffprobe, no browser, one code path on Windows, macOS,
and Linux.

**Goals:**

- A video that looks like the theatre: background art and scrim, the two portrait
  boxes with the active-speaker glow, the name plate, the rounded dialogue panel with
  markdown prose, and the scene card.
- A single `.webm` (VP8 video, Opus audio) that VLC plays, with a seekable index and a
  declared duration.
- No external binary. The exporter runs on a machine with nothing installed but the
  `localrpg` binary.
- The exporter reads `*scene.Script` directly. It does not require a prior web export.
- The web export is untouched except for the shared font assets it may later adopt.

**Non-Goals:**

- Pixel-exact parity with a browser's DOM. The video is a second renderer of the same
  design, not a recording of the page.
- H.264/AAC, MP4, or any format that needs a codec we cannot implement in Go. WebM is
  the only target.
- Subtitles, streaming, or progressive output. The exporter writes a complete file.
- A headless browser, a Node runtime, or any external rasteriser.

**Success Criteria:**

- `localrpg export video <game>` writes `dist/<game-id>.webm` with no ffmpeg present,
  and the file plays in VLC with picture and sound in sync.
- The frames show the theatre's stage, portraits, name plate, and dialogue panel, and
  render the same inline prose grammar the web export does.
- `go test ./...`, `npx tsc --noEmit`, `go vet ./...`, and `mise run build` are green,
  and no test needs ffmpeg or ffprobe.

## 2. Decisions & Non-Goals

Settled by review:

| Area | Decision |
| --- | --- |
| Frame source | A pure-Go compositor in `pkg/scene` that draws the theatre's layout; no browser, no external rasteriser |
| Look | The full theatre look: stage, portraits, name plate, dialogue panel, markdown prose, scene card |
| Animation | Optional, dual path: typewriter reveal, scene crossfade, and background drift when enabled; one fully revealed frame per beat when disabled |
| Encoder | `github.com/gen2brain/vpx` (`vp8.Encoder`), pure Go, keyframes and inter frames |
| Encoding mode | Inter-frame prediction (`Encode` + `EncodeInter`), not intra-only WebP keyframes; a heartbeat is a cheap predicted frame |
| Muxer | `github.com/at-wat/ebml-go/webm`, pure Go, VP8 + Opus, SeekHead and Cues |
| Container | WebM (`V_VP8` + `A_OPUS`). Playable in VLC; the acceptance bar |
| Audio | The campaign's existing Ogg/Opus clips, laid onto one 48 kHz timeline; no re-encode of speech |
| Fonts | Bundle the OFL fonts the theatre uses; `go:generate` fetches them once, they are committed and embedded |
| SVG art | Rasterized with `srwiley/oksvg` over `srwiley/rasterx`; raster art still decoded with `x/image` |
| ffmpeg | Removed from the export path entirely, including the `ffprobe` duration probe |

## 3. Architecture

Three pieces, each with one job:

```
pkg/scene          pkg/media/webm            pkg/export
┌───────────────┐  ┌──────────────────────┐  ┌───────────────────────────┐
│ theatre       │  │ vp8.Encoder          │  │ VideoPipeline.RenderVideo │
│ compositor    │─▶│  (RGBA → VP8)        │─▶│  drives beats → frames    │
│ Frame(req)    │  │ webm.Muxer           │  │  → muxer → temp file      │
│  → *image.RGBA│  │  (VP8 + Opus → WebM) │  │  → rename into place      │
└───────────────┘  └──────────────────────┘  └───────────────────────────┘
        ▲                     ▲                          ▲
   scene.Script          OpusTrack                  scene.Script
```

- `pkg/scene` gains a compositor that turns a beat into an `*image.RGBA`. It stays a
  pure model-plus-presentation package: no codec, no file, no ffmpeg.
- `pkg/media/webm` (new) owns everything codec-shaped: RGB→YUV420, VP8 encoding, Opus
  packet demux, WebM muxing, clusters, cues, duration. It imports `gen2brain/vpx`,
  `at-wat/ebml-go`, and `pion/opus/pkg/oggreader` (already a dependency).
- `pkg/export/video.go` becomes the orchestrator: it walks the script's beats, asks the
  compositor for frames, feeds the encoder and muxer, and writes the file atomically.

Data flow for one beat: the pipeline computes how many frames the beat occupies
(`scene.FramesFor`), asks the compositor for each frame's image, encodes each as a VP8
key or inter frame, and writes it with a millisecond timestamp. Audio is built once, up
front, as a single `OpusTrack`, and merged with the video by timestamp inside the muxer.

## 4. The Theatre Compositor (`pkg/scene`)

The compositor mirrors `StoryPlayer` → `TheaterStage` + `TheaterDialogue` +
`MarkdownProse`. It draws into an `*image.RGBA` at the export resolution using
`golang.org/x/image/font/opentype` for text and `image/draw` for the rest, plus small
helpers for rounded rectangles, linear/radial gradients, and soft shadows. No new
rendering dependency is needed.

### 4.1 Layout

Geometry follows the CSS, scaled to the frame (viewport units become frame fractions,
`rem` becomes a fraction of frame height so the design holds at any resolution):

| Element | Source | Geometry |
| --- | --- | --- |
| Background | `TheaterStage` | Cover-fit `beat.art \|\| scene.art \|\| script.Banner`; when absent, the radial gradient `#261e1b` → `#0c0a09` |
| Scrim | `TheaterStage` | Vertical gradient, bottom `black/90`, middle `black/45`, top `black/60` |
| Header | `StoryPlayer` | Top band: game name (uppercase, sans bold, purple), location pill, `Scene n of m` (mono) |
| Portrait band | `TheaterStage` | `top-20` to `bottom-[30vh]`, `px-[4vw]`, items aligned to the bottom, player left and NPC right |
| Portrait box | `TheaterStage` | Square, `clamp(140px, 24vw, 340px)`; radius `rounded-2xl`; `border-2`; image `object-cover object-top`; NPC mirrored horizontally |
| Active glow | `TheaterStage` | Player `border-sky-400` + sky shadow, NPC `border-purple-400` + purple shadow; inactive `white/30` |
| Label pill | `TheaterStage` | Below the box, `rounded-full`, sans bold, `text-xs`; active `bg-sky-600`/`bg-purple-600` on white, inactive `black/60` on stone |
| Dialogue panel | `TheaterDialogue` | `max-w-4xl` centred, `min-h-[20vh]`, `bg-stone-950/90`, `rounded-2xl`, border tinted by speaker (sky for the player, purple for speech, `white/15` for narration) |
| Name plate | `TheaterDialogue` | Absolute, above-left of the panel, `rounded-md`, sans extrabold, `text-sm`; `bg-sky-600`/`bg-purple-600`/`bg-stone-700` |
| Prose | `MarkdownProse` | Speech is wrapped in curly quotes, italic, `text-stone-50`; narration is `text-stone-200` |
| Scene card | `StoryPlayer` | Centred, `min-h-[20vh]`, serif uppercase, wide tracking, `text-amber-300` |

Two pieces of the page are interactive chrome and are **not** drawn: the transport bar
and the pulsing advance caret. A video has no controls, so reproducing them would add a
play button and a progress bar that mean nothing.

The compositor draws one beat, not the whole script, so it needs the script-level
constants the theatre keeps on stage: the protagonist's portrait and name, the banner,
the game name, the display mode, and the scene position. Those arrive through the frame
request (§7).

### 4.2 Inline prose grammar

`scene.Beat.Text` is the raw segment text, so the compositor applies the same grammar
`MarkdownProse.tsx` applies. The inline grammar is small and is reimplemented in Go with
the same pattern and the same rendering:

| Token | Rendering |
| --- | --- |
| `[[target\|label]]` | Purple link text (the label, or the target) |
| `[direction]` | A small sans, uppercase, purple performance pill; stripped when `display_mode` is `hidden`, shown literally when `raw` |
| `` `code` `` | JetBrains Mono, inset background |
| `**bold**` | Semibold, `stone-100` |
| `*italic*` / `_italic_` | Italic |

Block structure follows `MarkdownProse` too: blank lines separate paragraphs, single
newlines are kept, `---` is a rule, `#`/`##` are headings, `-`/`*` runs are lists, and
`>` is a blockquote. In practice a beat is a paragraph or two, so the block layer can be
as small as the grammar allows while still matching the page for the cases prose
actually produces.

### 4.3 Fonts

The theatre's typography resolves to three families (`frontend/src/index.css`):

- **EB Garamond** for body prose, narration, and speech (regular, semibold, italic).
- **JetBrains Mono** for code and monospace chrome.
- The OS UI sans for labels and the header (`font-sans`); the app uses
  `ui-sans-serif, system-ui, …`, which is platform-dependent, so the video bundles one
  neutral sans (Inter) for a consistent label.

Cinzel is loaded by the app page but the theatre never uses it (its class maps to the
sans stack), so the video does not need it. All of these families are SIL Open Font
License 1.1, which permits embedding in a binary provided the license and copyright
notice travel with it.

### 4.4 Animation

Animation is a single `Animate` flag on the frame request, giving the dual path:

- **Animated (default).** The typewriter reveal shows a prefix of the beat's text over
  `TypewriterFraction` of the beat (`scene.TypewriterFraction`, mirroring the player's
  `TYPEWRITER_FRACTION`); the first beat of a scene crossfades in over `crossfadeShare`;
  the background drifts in scale from `driftStart` to `driftEnd` across the beat. This
  is the current `revealText`/`crossfadeAlpha`/drift logic, redrawn over the new layout.
- **Static (`--still`).** One frame per beat at `Progress = 1`, fully revealed. Small
  and fast, and the honest fallback on a weak machine.

Frame count per beat is `scene.FramesFor(beat.Duration, fps)`; the frame rate defaults
to `scene.DefaultFPS` and is settable. During the static tail of an animated beat (after
the reveal completes) the compositor emits a cheap inter-frame heartbeat at about 1 fps,
which costs almost nothing once inter-frame prediction is in play and keeps players from
holding a single frame for many seconds.

### 4.5 Art loading

Every image the stage draws (scene art, portraits, banner) resolves to a file path from
the compiler. The compositor loads each path once per export and caches the decoded
image:

- Raster files (PNG, JPEG, WebP) decode with `image.Decode`, as today.
- SVG files rasterize with `srwiley/oksvg` over `srwiley/rasterx`. This is the default
  case, not an edge one: the built-in image provider emits SVG (`pkg/media/procedural_art.go`),
  and a character with no portrait always gets the procedural bust SVG
  (`pkg/media/procedural_bust.go`). oksvg handles the linear and radial gradients, paths,
  ellipses, circles, and rounded rects those generators use.
- The rasterized image is drawn with the same geometry the stage uses: cover-fit for
  scene art and the banner, and the square portrait box (`object-cover object-top`) for
  portraits.
- A file that is missing or will not parse degrades rather than failing the export:
  scene art falls back to the native radial gradient, and a missing portrait simply
  leaves its box empty, which is the state the stage already handles.

This replaces the current renderer's silent substitution of a Go gradient for any SVG
(`pkg/scene/render.go:136`), which is why a campaign on the default image provider
currently looks different in the video than in the web export, and why its portraits do
not appear at all.

## 5. The WebM Muxer (`pkg/media/webm`)

A new package with two responsibilities: encode VP8, and mux VP8 + Opus into WebM.

### 5.1 Video track

- Frames arrive as `*image.RGBA`. The package converts them to a `vp8.Picture` (4:2:0
  YUV, BT.601, the same colour space WebP and WebM VP8 use) with its own converter.
- `vpx.vp8.Encoder.Encode` writes a keyframe and refreshes every reference buffer;
  `EncodeInter` writes a frame predicted from the one before. The encoder never inserts
  a keyframe on its own, so the muxer's cadence drives it: a keyframe at the start and
  every 5 seconds (aligned to cluster starts), inter frames otherwise.
- The track is declared as `V_VP8`, `TrackType` 1, with the frame's pixel dimensions.
  WebM VP8 carries no `CodecPrivate`.
- Quality and method map to `vp8.EncodeOptions{Quality, Method, Threads}` and are
  exposed as a `--quality` flag with a sensible default.

### 5.2 Audio track

The campaign's clips are already Ogg/Opus files produced by `pkg/media/opus` with one
encoder configuration (mono, 48 kHz, pre-skip 312), which is what makes a lossless join
possible. The pipeline assembles one continuous track from the beats, without ever
decoding speech back to PCM: an Opus packet carries no timestamp of its own, so only the
container decides when it plays.

- `AppendClip` streams a clip with `pion/opus/pkg/oggreader` in O(1) memory, keeping each
  packet and skipping the `OpusHead`/`OpusTags` pages. It checks the clip's `OpusHead`
  against the track's and errors on a mismatch, which cannot happen with our own clips.
- Each packet's length comes from its own TOC byte (RFC 6716 §3.1): the config field
  gives the frame duration (2.5/5/10/20/40/60 ms) and the frame-count code gives how many
  frames it carries. No decoding, no granule arithmetic.
- Packets land on a single running sample position. `AppendSilence` advances the position
  by a gap: a short gap (under a second or two) is left to the container's timecodes, and
  a longer one is filled with the canonical 20 ms Opus silence packet, `f8 ff fe` for
  mono, so a beat with no clips never starves the decoder.
- The track's `CodecPrivate` is the standard `OpusHead` (RFC 7845), with
  `CodecDelay = preSkip` in nanoseconds, `SeekPreRoll = 80 ms`, `SamplingFrequency` 48000,
  and the clip channel count.

### 5.3 Interleaving, clusters, cues, and duration

- `webm.NewSimpleBlockWriter` returns one writer per track and merges them with its
  multi-track sorter. The pipeline keeps its own min-heap of timed video and audio
  packets (`container/heap`) and drains it in strict non-decreasing timestamp order, so
  neither track's blocks are ever dropped as out of date.
- `WithSeekHead(true)` and `WithCues` build the seek index, with the reserve sized
  generously from the expected cluster count so cues are never downsampled; the muxer
  therefore requires an `io.WriteSeeker`, which the pipeline supplies as the temp
  `*os.File`.
- `WithMaxKeyframeInterval` aligns cluster starts with video keyframes so every cluster
  begins on a keyframe and seeking lands on one.
- The total duration is declared through `mkvcore.WithSegmentInfo` (`webm.Info` with
  `SetDuration`), or written automatically once Cues are enabled, so a player knows the
  length without reading to EOF. Note that current `ebml-go` has no `webm.WithDuration`
  option; duration is a `SegmentInfo` field. `Close` flushes the last cluster and
  backfills the index.

## 6. The Pipeline (`pkg/export`)

`VideoPipeline.RenderVideo(ctx, script, outputFile)` is rewritten:

1. Assemble the `OpusTrack` once from the beats' clips and gaps (`AppendClip` and
   `AppendSilence`).
2. Open a temp file beside `outputFile`, wrapped as the muxer's seekable writer.
3. Walk the beats in order, computing each beat's frames, rendering them through the
   compositor, encoding them, and writing them with millisecond timestamps; the muxer
   interleaves the audio.
4. `Close` the muxer (finalizing cues and duration), then rename the temp file into
   place, so a failed or cancelled render leaves nothing that looks playable.
5. Report progress through `scene.ProgressFunc` in phases: `frames`, `encode`, `mux`.

Clip durations come from the Opus granule position rather than `ffprobe`
(`pkg/export/script.go:137`), which removes the last external process from the export
path. The public shape (`NewVideoPipeline`, `SetSize`, `SetFPS`, `SetStill`,
`SetProgress`, `RenderVideo`) is kept; `BuildCommand` and `FFmpegAvailable` are removed.

## 7. Interfaces

```go
// pkg/scene
type FrameRequest struct {
    Script      *Script
    SceneIndex  int
    Scene       Scene
    Beat        Beat
    Progress    float64 // 0 at the beat's start, 1 at its end
    PreviousArt string  // the outgoing scene's art, for the crossfade
    Animate     bool
}

type Renderer struct{ /* faces, size */ }
func NewRenderer(width, height int) (*Renderer, error)
func (r *Renderer) Frame(req FrameRequest) *image.RGBA
```

```go
// pkg/media/webm
type OpusPacket struct {
    Data     []byte
    Time     time.Duration
    Duration time.Duration
}
type OpusTrack struct {
    Head     []byte // OpusHead, the track's CodecPrivate
    Channels uint64
    Packets  []OpusPacket
}
func NewOpusTrack(channels uint64) *OpusTrack
// AppendClip demuxes one Ogg/Opus clip onto the track's timeline; AppendSilence
// advances it by a gap. The pipeline drives both from the script's beats.
func (t *OpusTrack) AppendClip(ogg []byte) error
func (t *OpusTrack) AppendSilence(d time.Duration) error

type Encoder struct{ /* vpx.Encoder, source picture */ }
func NewEncoder(width, height, quality int) *Encoder
func (e *Encoder) Encode(rgba *image.RGBA, keyframe bool) ([]byte, error)

type Muxer struct{ /* ebml-go writers */ }
func NewMuxer(w io.WriteSeeker, width, height int, track *OpusTrack) (*Muxer, error)
func (m *Muxer) WriteVideo(data []byte, keyframe bool, t time.Duration) error
func (m *Muxer) Close() error
```

```go
// pkg/media/opus
// Duration reads a clip's length from the final Ogg page's granule position, the
// pure-Go replacement for the ffprobe probe on the export path.
func Duration(data []byte) (time.Duration, error)
```

## 8. Fonts & Licensing

A new `pkg/scene/fonts` package embeds the families the theatre uses:

- `go:generate` runs a small program that downloads the OFL families and writes the
  `.ttf` files plus each family's `OFL.txt` and copyright notice into the package.
- The downloaded files are **committed**, so a build never needs the network and the
  binary is reproducible.
- The package exposes parsed faces and the license text; `go:embed` pulls the files in.
- If the embedded files are absent (a source checkout before `go generate`), the
  compositor falls back to `golang.org/x/image/font/gofont` rather than failing, so the
  build and the tests never depend on the fonts being present.

## 9. CLI & GUI Surface

- CLI: `localrpg export video` defaults to `dist/<game-id>.webm`; flags `--fps`,
  `--size`, `--quality`, and `--still` (disable animation). The ffmpeg check is gone.
- `pkg/gui/export.go`: drop `ErrExportNoFFmpeg`, the ffmpeg fields on
  `ExportCapabilitiesDTO`, and the up-front availability check; `exportArtifactPath`
  returns `.webm`. `pkg/gui/server.go` drops the now-unused error case.
- `frontend/src/components/ExportModal.tsx`: remove the ffmpeg-missing gate, since the
  video button is always available.
- Follow-on (not in this change): the same embedded fonts can be injected into the web
  bundle as `@font-face` data URIs, closing the player page's font gap that the
  2026-09-29 parity spec notes.

## 10. Testing

All tests are pure Go; none may invoke ffmpeg or ffprobe.

- `pkg/media/webm`: encode a handful of frames, mux them against a synthetic Opus track,
  then read the file back with `ebml-go`'s reader and assert the doc type, both track
  codec IDs, monotonic timestamps, a keyframe at each cluster start, a declared duration
  close to the timeline, and the presence of cues. Decode an encoded frame back with
  `vpx.vp8.Decoder` and assert its dimensions.
- `pkg/media/webm`: the RGB→YUV420 conversion round-trips within a tolerance, and the
  `OpusTrack` builder places packets and silence at the expected timestamps.
- `pkg/scene`: the compositor renders a narration beat, a speech beat, and a scene card
  at 1080p and asserts the frame is the right size and non-empty, that the active
  speaker's border colour appears in the portrait band, and that the inline grammar
  renders a `[[wikilink]]`, a `[direction]` under each `display_mode`, code, and
  emphasis.
- `pkg/scene`: the art loader rasterizes the procedural bust and procedural location SVGs
  and asserts the result is the right size and non-empty, and falls back to the gradient
  when a path is missing or unparseable.
- `pkg/export`: `RenderVideo` on a small script writes a file that reads back as a valid
  WebM, and a cancelled context leaves no output file.
- `pkg/media/opus`: `Duration` matches the granule position for a generated clip.

## 11. Risks & Open Questions

| Risk | Mitigation |
| --- | --- |
| `gen2brain/vpx` is young (single maintainer, low adoption) | It is a libwebp port with conformance tests against libvpx; pin a version and keep it behind the `pkg/media/webm` interface so it can be swapped |
| The inter-frame encoder is the least-proven path (libwebp has no inter-frame encoder; this is gen2brain's own, validated by libvpx decoding) | Pin the version, verify our own output by decoding it back with the same library, and keep the intra-only WebP-keyframe path (lossy WebP stripped to a VP8 keyframe) as the documented fallback if it proves faulty |
| `oksvg` implements only part of SVG | The procedural generators use a small subset (gradients, paths, ellipses, circles, rounded rects); test those specifically, and keep the native gradient fallback for anything that will not parse |
| 1080p encode time and memory | `--size`, `--fps`, `--still`, `--quality`, and `Method`/`Threads`; `--still` is the fast path |
| Safari's WebM support is partial | The acceptance bar is VLC; Chromium and Firefox play it. Noted, not blocking |
| VP8 4:2:0 BT.601 differs slightly from the browser's sRGB rendering | Expected; the goal is "very close", not pixel-exact |
| Embedding fonts grows the binary and adds a `go:generate` step | Files are committed; the fallback to Go fonts keeps builds working without them |
| A very long story produces a large file | Inter-frame VP8 keeps static spans small; `--still` and lower resolution are the escape hatches |
