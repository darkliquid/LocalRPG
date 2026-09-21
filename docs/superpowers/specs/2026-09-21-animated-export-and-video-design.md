# Design Specification: Animated Story Export and Video Rendering

**Date:** 2026-09-21  
**Status:** Draft — pending review  
**Topic:** Turn the replay export into an animated visual-novel player that runs a campaign in order, and make the video renderer produce that same experience, with location-keyed scenes, real imagery, per-speaker audio, and reading-time pacing

---

## 1. Problem Statement & Motivation

`localrpg export web|video` exists, is documented in the README as an in-app player, a standalone HTML5 bundle, and headless FFmpeg rendering, and delivers almost none of that.

1. **The bundle is a click-through slideshow.** `pkg/export/web.go` writes one `index.html` with inline CSS/JS and the marshalled script embedded, and the player shows one beat at a time behind Previous/Next/Pause buttons. It does not run by itself, has no scene structure, and shows one speaker label per beat.
2. **The video is a silent still image.** `pkg/export/video.go` builds `ffmpeg -f lavfi -i color=…#0c0a09 -f lavfi -i anullsrc … -shortest out.mp4`: no text, no imagery, no audio, ignoring everything the script carries. It is the README's "headless FFmpeg video rendering" claim in name only.
3. **Nothing associates imagery with a scene.** `SceneBeat` has no image field and nothing populates one. Location art resolution arrives with the attribution and location work (`media.AppearanceHash`, `media.GenerateLocationImage`), but the export does not consume it.
4. **Nothing materialises audio.** Clips are content-addressed in the media cache and served on demand by the GUI; a standalone bundle cannot call the backend, so clips and art must be written beside it.
5. **Pacing is four hard-coded numbers.** `ScriptCompiler` assigns 3.5 seconds per narrated span and 4.0 per spoken line. On a page of dense prose that is far too fast, and on a three-word line it is far too slow.
6. **Two renderers would invent two structures.** The web player, the video renderer, and the in-app Story Theater all need the same answer to "what are the beats, how long is each, and what art belongs to it". Without one model they drift, which is how the current web player ended up reading `beat.speaker`/`beat.dialogue` while the API never sent them.
7. **The audio cache mislabels its own contents.** `TTSPipeline.SynthesizeUtterance` names every clip `<key>.wav` whatever the provider returned, and the segment audio route serves it as `audio/wav`.

---

## 2. Decisions & Non-Goals

Settled by review:

| Area | Decision |
| --- | --- |
| Capture strategy | Go-side composite frames sharing one scene-data model with the web player; no headless browser |
| Web player | The animated player replaces the static click-through one |
| Scene key | Scenes are locations; consecutive turns in one location form a scene, and a location change is a scene boundary |
| Scene transition | A full-frame scene card (art plus location name) held for one reading-time floor, with a short crossfade into the scene's art |
| Reading-time pacing | 180 words per minute, a 2.0s minimum, a 0.4s gap, audio duration plus that gap when a clip exists, and a character-per-minute fallback for unspaced scripts. Named constants in the shared model, not configuration |
| Bundle layout | Sidecar directories: `index.html`, `assets/`, `audio/`, referenced by relative path |
| Fonts | No font binaries in the repo: Go fonts (`golang.org/x/image/font/gofont`) in video, system serif on the web |
| Video output | 1920×1080 at 15 fps by default, with `--still` falling back to one frame per beat |
| Export audio | Synthesised at export time reusing the media cache, skippable with `--no-audio`, and silent with a warning when TTS is unconfigured |
| Export art | Resolved through the location appearance hash with the built-in generator as the offline fallback, with `--no-art` for a plain background |
| Ordering | This spec follows the attribution, location, and playback work, which supplies scene keys, art resolution, and per-segment clips |

Non-goals:

- Pixel-exact parity with the browser's DOM. The video is a second renderer of the same scene data, not a recording of the first.
- A headless browser, a Node runtime, or any external rasteriser. `--out` bundles are self-contained.
- Progressive or streamed video. The renderer writes a complete file.
- Separate subtitle files (`.srt`/`.vtt`). Beats are timed for playback, not for sidecar captions.
- Bundling a webfont. The app's own Google Fonts dependency is a pre-existing wart and is not addressed here.
- GUI turn submission, which remains its own spec.

---

## 3. The Scene Model (`pkg/scene`)

One package owns the shape every renderer reads. It imports `entity`, `media`, and the timeline types, and nothing imports it except the exporters.

```go
package scene

// BeatKind distinguishes what a beat is presenting.
type BeatKind string

const (
	// BeatSceneCard introduces a location: full-frame art and its name.
	BeatSceneCard BeatKind = "scene_card"
	// BeatNarration is prose in the narrator's voice.
	BeatNarration BeatKind = "narration"
	// BeatSpeech is an attributed line.
	BeatSpeech BeatKind = "speech"
)

// Beat is one unit of playback: a span of text, its imagery, and its audio.
type Beat struct {
	Kind          BeatKind
	TurnNumber    int
	Speaker       string
	SpeakerID     string
	Text          string
	ArtPath       string
	AudioPath     string
	AudioDuration time.Duration
	Duration      time.Duration
}

// Scene groups the beats that happened in one place.
type Scene struct {
	LocationID   string
	LocationName string
	ArtPath      string
	Beats        []Beat
	Duration     time.Duration
}

// Script is the whole export.
type Script struct {
	GameID        string
	GameName      string
	WorldStyle    string
	Scenes        []Scene
	TotalDuration time.Duration
}

// Beats flattens the scenes for renderers that walk a single sequence.
func (s Script) Beats() []Beat
```

`SceneCard` produces the introductory beat for a scene, so both renderers agree on its text and duration:

```go
// SceneCard is the beat that introduces a location.
func SceneCard(s Scene) Beat {
	text := strings.TrimSpace(s.LocationName)
	if text == "" {
		text = "Somewhere new"
	}
	return Beat{
		Kind:          BeatSceneCard,
		Text:          text,
		ArtPath:       s.ArtPath,
		AudioDuration: 0,
		Duration:      BeatDuration(Beat{Text: text}),
	}
}
```

### 3.1 Reading-time pacing

```go
const (
	// ReadingWordsPerMinute sits below the ~200wpm adult average deliberately: a
	// viewer cannot scroll back to re-read a beat that has passed.
	ReadingWordsPerMinute = 180
	// ReadingCharactersPerMinute covers scripts that do not separate words, where
	// counting fields under-measures badly.
	ReadingCharactersPerMinute = 600
	// MinimumBeatDuration keeps a three-word line on screen long enough to register.
	MinimumBeatDuration = 2 * time.Second
	// BeatGap separates consecutive beats perceptually.
	BeatGap = 400 * time.Millisecond
	// TypewriterFraction is the share of a beat spent revealing its text; the rest
	// is the viewer's.
	TypewriterFraction = 0.6
)

// ReadingDuration estimates the time a viewer needs for text, never below the floor.
func ReadingDuration(text string) time.Duration {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return MinimumBeatDuration
	}

	var minutes float64
	if fields := strings.Fields(trimmed); len(fields) >= 3 {
		minutes = float64(len(fields)) / ReadingWordsPerMinute
	} else {
		minutes = float64(utf8.RuneCountInString(trimmed)) / ReadingCharactersPerMinute
	}

	if duration := time.Duration(minutes * float64(time.Minute)); duration > MinimumBeatDuration {
		return duration
	}
	return MinimumBeatDuration
}

// BeatDuration is how long a beat is shown: a clip's real length when one exists,
// otherwise the reading estimate, plus the gap. Pacing therefore follows the audio
// whenever there is audio and follows the text whenever there is not.
func BeatDuration(beat Beat) time.Duration {
	if beat.AudioDuration > 0 {
		return beat.AudioDuration + BeatGap
	}
	return ReadingDuration(beat.Text) + BeatGap
}
```

Frames per beat follow from the same value, which is what keeps picture and sound in step: `frames = max(1, round(beat.Duration.Seconds() * fps))`.

---

## 4. Compilation

`script.go`'s `ScriptCompiler` is replaced by `scene.Compile`, which reads the timeline, groups it by location, and resolves art and audio per beat. `export.ReplayScript`, `export.SceneBeat`, and the old player's `beat.speaker`/`beat.dialogue` shape are removed with it. Nothing outside `pkg/export` and its tests consumes those types.

`export.NewScriptCompiler(rootDir)` stays as the CLI's entry point and becomes the adapter: it builds the path resolver, store, media cache, and pipelines from `rootDir` and the loaded config, runs `scene.Compile`, and returns a `*scene.Script`. `cmd/localrpg/export.go` therefore keeps its current shape and only gains flags.

```go
// Options controls what compilation resolves.
type Options struct {
	Art           bool   // resolve scene art through the image pipeline
	Audio         bool   // synthesise missing clips
	WorldStyle    string // the world's art_style and genre
	ProviderParams string // image provider and model, part of the art cache key
	OnProgress    func(format string, args ...interface{})
}

// Compiler turns a campaign's timeline into a playable script.
type Compiler struct {
	resolver *core.PathResolver
	store    *storage.Store
	imager   ArtResolver
	speech   SpeechResolver
}

// ArtResolver returns a scene's image path, generating it when needed.
type ArtResolver interface {
	SceneArt(location *entity.Entity, force bool) (string, error)
}

// SpeechResolver returns a clip's path and duration, or ErrAudioUnavailable.
type SpeechResolver interface {
	SegmentAudio(segment entity.TurnSegment) (string, time.Duration, error)
}

// ErrAudioUnavailable means no TTS provider is configured. It lives here rather
// than in pkg/gui because every consumer of a script needs it; pkg/gui keeps its
// exported alias so its route and tests read unchanged.
var ErrAudioUnavailable = errors.New("audio unavailable")

func (c *Compiler) Compile(ctx context.Context, gameID string, opts Options) (*Script, error)
```

Compilation rules:

- **Grouping.** Walk turns in order; a turn's `Location` key opens a new scene when it differs from the previous turn's. A turn with no location becomes a scene of its own with no art and no scene card, never merged with its neighbours, because "unknown" is not a place.
- **Art per scene.** Resolved once per scene, not once per beat, so a long conversation reuses one image. A scene whose art cannot be resolved keeps playing: art is decoration, never a prerequisite.
- **Audio per beat.** A segment's clip is read from the cache when present; when absent and `Audio` is set, it is synthesised. `ErrAudioUnavailable` (TTS unconfigured or `disabled`) leaves the beat silent and counts it, and compilation reports the total once so the export is not silently mute.
- **Audio durations.** A clip's real duration is probed with `ffprobe` (already required alongside `ffmpeg`), falling back to the reading estimate when probing fails, so one unreadable clip cannot fail an export.
- **Scene cards.** `SceneCard` is prepended to every scene that has a location.
- **Empty campaigns.** A campaign with no turns compiles to a script with no scenes, and the export refuses with a clear message rather than writing an empty player.

---

## 5. The Animated Web Player

The bundle changes shape and behaviour.

```text
dist/<game>-web/
├── index.html          inline CSS and JS, plus the script JSON
├── assets/             one file per scene image (svg or raster)
└── audio/              one file per beat that has a clip
```

- **Playback.** The player starts on load and walks the flattened beats. Each beat sets the background (crossfading over 300ms when the image changes), sets the speaker label for speech, reveals its text, and plays its clip if it has one. A scene card holds for its duration before its scene's first beat.
- **Autoplay and the first gesture.** Browsers refuse to start audio without a user gesture, so the player attempts playback on load and, if the first clip's `play()` rejects, pauses on that beat with a prominent Play control instead of running silently ahead. One click starts normal playback, and the same control covers a viewer who prefers to drive it.
- **Typewriter.** Text reveals over `Duration × TypewriterFraction`, giving a character rate of `runes / (duration × 0.6)`; the remaining time is the viewer's. `prefers-reduced-motion` disables the reveal and shows each beat whole.
- **Controls.** Play/pause, previous and next beat, a scene indicator showing the location name, and a progress bar over the whole campaign. Reaching the end offers a replay rather than stopping dead.
- **Offline and self-contained.** No external requests, no webfonts, no CDN. Assets are relative paths; the CSS uses a system serif stack. Opening `index.html` from disk works with no server.
- **Failure tolerance.** A missing asset or unreadable clip must not stall playback: the beat shows its text and the player continues.

`WebExporter.Export` keeps its signature (`(ctx, script, outDir) (string, error)`) and returns the bundle directory rather than a file path, writing assets and audio alongside `index.html`.

---

## 6. Video Rendering

The renderer draws frames in Go and lets FFmpeg encode them.

### 6.1 The rasteriser

```go
// Renderer draws frames for a script.
type Renderer struct {
	width  int
	height int
	fps    int
	beat   *BeatRenderer
}

// Frames renders one image per output frame for a beat, in order.
func (r *Renderer) Frames(beat scene.Beat, scene scene.Scene) ([]*image.RGBA, error)
```

- **Fonts and text.** `golang.org/x/image/font/gofont/goregular` and `gobold`, parsed with `font/opentype`, laid out with `font.MeasureString`, wrapped to the frame's width minus margins, capped at a readable number of lines with an ellipsis beyond it.
- **Backgrounds.** A raster art file is decoded and used directly (`image/png`, `image/jpeg`, and `golang.org/x/image/webp`). SVG art cannot be decoded by `x/image`, so a scene whose art is SVG gets a **raster background drawn in Go** from the same inputs: a deterministic palette and structure derived from the location ID and its appearance hash. This is the one place the video differs visibly from the web player for procedurally generated art, and it is deliberate: the alternative is an external rasteriser or a browser, both rejected.
- **Motion.** Each beat drifts its background slightly (a 2% to 6% scale over the beat) and reveals text at the same character rate as the player, so the video feels like the player rather than a slideshow.
- **Scene cards.** Rendered as a full-frame image with the location name centred, fading in over the first 300ms and crossfading to the first beat's art, matching the player's crossfade.

### 6.2 Audio assembly

Clips are heterogeneous bytes (a WAV from one provider, MP3 or Opus from another), so FFmpeg's `concat` filter assembles the track by decoding each input rather than by concatenating files:

```text
ffmpeg -y \
  -framerate 15 -i frames/%06d.png \
  -i clip-0001.wav -i clip-0002.ogg \
  -f lavfi -t 2.4 -i anullsrc=r=44100:cl=stereo \
  -filter_complex "[1:a][2:a][3:a]concat=n=3:v=0:a=1[a]" \
  -map 0:v -map "[a]" -c:v libx264 -pix_fmt yuv420p -c:a aac -b:a 192k -shortest \
  out.mp4
```

Inputs are added in beat order, so the `concat` filter's stream indices follow the beats rather than being grouped by kind; the example above has two clips followed by one silent beat, and a silent beat between two clips appears between them in the input list and in the filter. One `anullsrc` input per silent beat, each with its own `-t`, keeps the audio track's length equal to the sum of beat durations, which the frame count already matches.

`BuildCommand` keeps its separate role so the invocation stays testable without running FFmpeg; it gains the frame-pattern and per-clip inputs.

### 6.3 Output and failure

- Frames go to a temporary directory and are removed after encoding; the muxed file is written to `out.mp4.part` and renamed on success, so a failed export never leaves a playable-looking half-file.
- `--still` renders one frame per beat instead of an animated sequence, for a fast export on a weak machine.
- Progress goes to stderr (`scene 2/5, beat 7/12`) so it does not pollute stdout, and one unrenderable beat is skipped rather than failing the run.
- `ffmpeg`/`ffprobe` absence is detected up front with `exec.LookPath` and reported with the install hint, instead of failing mid-render.

---

## 7. Pacing Parity

The in-app Story Theater is React and cannot import Go constants, so it would otherwise re-implement the estimator and drift. `SegmentDTO` gains `duration` in seconds, computed server-side with `scene.ReadingDuration`, and the Chronicle and Story Theater use it for typewriter rate and dwell.

For a segment that has a clip, the browser knows the clip's real length the moment the audio element loads it, so the client prefers that over the estimate once it is known — the same preference the video renderer applies using the probed length, without the API having to run `ffprobe` on every chronicle request. One estimator plus one rule ("a clip's length wins when there is a clip") therefore paces the in-app player, the exported player, and the video.

## 8. Fixing the Audio Cache's Extension

`TTSPipeline.SynthesizeUtterance` names every clip `.wav` regardless of the bytes, and the segment audio route serves `audio/wav` for whatever it finds. Both are corrected by sniffing the content: a RIFF/WAVE magic yields `.wav`/`audio/wav`, `ID3` or an MP3 frame sync yields `.mp3`/`audio/mpeg`, `OggS` yields `.ogg`/`audio/ogg`, `fLaC` yields `.flac`/`audio/flac`, and an unrecognised body falls back to `.wav`/`application/octet-stream`.

Clips written before this change keep being reused rather than regenerated: the pipeline looks for a hit under the sniffed extension first, then under a legacy `.wav` name, and the *served* content type always comes from the bytes. New clips are named honestly, existing caches stay warm, and no one has to delete a cache directory to get correct behaviour.

---

## 9. CLI Surface

```text
localrpg export web   <game-id> [--out dir]  [--dir root] [--no-art] [--no-audio]
localrpg export video <game-id> [--out file] [--dir root] [--no-art] [--no-audio]
                                [--still] [--fps N] [--size WxH]
```

Defaults stay where they are (`dist/<game>-web`, `dist/<game>.mp4`, root `.`), so existing invocations keep working. Progress and warnings go to stderr; the final path goes to stdout, which the existing CLI tests already parse.

---

## 10. Component & File Map

| File | Change |
| --- | --- |
| `pkg/scene/scene.go` *(new)* | `Script`, `Scene`, `Beat`, `BeatKind`, `SceneCard`, `Script.Beats` |
| `pkg/scene/timing.go` *(new)* | Reading-time constants, `ReadingDuration`, `BeatDuration` |
| `pkg/scene/compile.go` *(new)* | `Compiler`, `Options`, `ArtResolver`, `SpeechResolver`, `ErrAudioUnavailable`, grouping and resolution |
| `pkg/scene/render.go` *(new)* | The Go rasteriser: text layout, backgrounds, drift, scene cards |
| `pkg/scene/*_test.go` *(new)* | Timing, grouping, art and audio resolution, frame determinism |
| `pkg/export/script.go` | `ScriptCompiler` becomes the adapter that builds a `scene.Compiler` and calls `scene.Compile`; `ReplayScript`/`SceneBeat` removed |
| `pkg/export/web.go` | Animated player, sidecar `assets/` and `audio/`, scene JSON, crossfades, controls |
| `pkg/export/video.go` | Frame rendering, per-clip audio inputs, `concat` filter, temp-file-then-rename, `--still` |
| `pkg/export/types.go` | Removed with the old script shape |
| `pkg/media/tts.go` | `AudioExtension` and format-aware clip naming |
| `pkg/gui/service.go` | `SegmentDTO.Duration`; audio route serves the sniffed content type; `ErrAudioUnavailable` becomes an alias of the `pkg/scene` sentinel |
| `pkg/gui/types.go` | `SegmentDTO.Duration` |
| `frontend/src/types.ts`, `StoryTheater.tsx` | Consume `duration` for pacing |
| `cmd/localrpg/export.go` | `--no-art`, `--no-audio`, `--still`, `--fps`, `--size`, progress to stderr |
| `README.md` | Describe what the export actually produces |
| `go.mod` | Adds `golang.org/x/image` |

---

## 11. Migration & Compatibility

- **Existing bundles are regenerated.** The player's structure changes completely; there is no compatibility mode for the old static player, and `localrpg export web` overwrites the directory.
- **Existing video invocations keep working** with the same defaults; the output is simply real now.
- **Existing audio caches keep working** after the extension fix, because the sniff reads bytes rather than filenames.
- **Campaigns with no location data** (recorded before the location work) export as a sequence of single-turn scenes with no art and no scene cards, which is a complete, if plainer, result rather than an error.
- **`pkg/export`'s public types change**, so any downstream consumer of `ReplayScript` must move to `scene.Script`. Within this repository the only consumers are the two exporters and their tests.

---

## 12. Verification & Testing Plan

**`pkg/scene` timing**
- A 100-word narration and a 3-word line land where the formula says, with the short one clamped to the 2.0s floor.
- Unspaced text (for example 40 CJK characters) uses the character rate rather than reading one field.
- A beat with a known audio duration takes that duration plus the gap, ignoring the text estimate; a beat without audio uses the estimate.
- Frame counts for a beat equal `round(duration × fps)` for several fps values, and never zero.

**`pkg/scene` grouping and resolution**
- Consecutive turns in one location form one scene with one scene card; a location change opens a new scene.
- Turns with no location become single-turn scenes without art and without scene cards.
- Art is resolved once per scene: a five-turn conversation calls the resolver once.
- A scene whose art resolution fails still compiles, with beats and no art.
- Audio resolves from the cache without calling the synthesiser when a clip exists; when TTS is unavailable, beats are silent and compilation reports how many.

**`pkg/scene` rendering**
- A frame is exactly the requested size, is not a single flat colour, and contains visibly different pixels where text was drawn.
- Rendering the same beat twice produces byte-identical frames, which is what makes exports reproducible.
- A long beat wraps to a bounded number of lines and truncates the remainder rather than overflowing the frame.
- A beat with audio produces more frames than one without, since its duration is longer.

**`pkg/export` web**
- The bundle contains `index.html`, an `assets/` entry per scene, and an `audio/` entry per clip-bearing beat.
- The HTML references no external URL (`http://`, `https://`, `//cdn`) and no font file, and every asset path it uses exists in the bundle.
- The embedded JSON carries every beat's duration and art path.
- A transcript with no audio produces a bundle with an empty `audio/` and a working player.

**`pkg/export` video**
- `BuildCommand` output includes the frame pattern, one input per clip, one `anullsrc` per silent beat with the right `-t`, the `concat` filter, and the output path.
- Each beat's frame count is `round(duration × fps)`, at least one, and the rendered sequence length equals the sum of those per-beat counts (asserted per beat, since rounding per beat does not sum to rounding the total).
- `--still` produces exactly one frame per beat.
- With `ffmpeg` present (`exec.LookPath`), an end-to-end render of a two-beat script produces a file of non-trivial size; skipped when it is absent.
- A failed render leaves no output file behind.

**`pkg/media` / `pkg/gui`**
- `AudioExtension` recognises RIFF, MP3, Ogg, and FLAC magics, and falls back for unknown bytes.
- The segment audio route serves the sniffed content type, and an existing `.wav`-named file with MP3 bytes is served as `audio/mpeg`.
- `SegmentDTO.Duration` matches `scene.BeatDuration` for the same segment.

**Project**
- `go vet ./...`, `mise run test` (Go suite plus `tsc --noEmit`), and every commit builds standalone in a scratch worktree.

---

## 13. Phased Delivery

1. **Scene model and pacing** — types, the reading-time estimator, frame-count arithmetic, and its tests. Nothing renders yet; everything else depends on it.
2. **Compilation** — grouping by location, per-scene art, per-segment audio, duration probing, progress and warnings.
3. **Animated web player** — the sidecar bundle, playback loop, typewriter, controls, offline guarantee.
4. **Rasteriser** — fonts, wrapping, backgrounds, drift, scene cards, determinism.
5. **Video rendering** — frame sequence, audio assembly, `concat` command, temp-and-rename, `--still`.
6. **Surface** — CLI flags, `SegmentDTO.Duration` and the in-app player's pacing, the audio extension fix, README.

---

## 14. Open Questions

None outstanding. The one deliberate visual divergence — procedurally generated SVG art is redrawn as a Go raster background for video rather than rasterised — follows from rejecting external tools and browser capture, and is documented in Section 6.1 rather than left implicit.
