# Exported Player Parity with the Story Theatre Design

**Date:** 2026-09-29
**Status:** Proposed
**Scope:** Make the exported web bundle play the story through the same presentational components the in-app theatre uses, so a bundle looks and sounds like watching theatre mode, keeping the typewriter reveal
**Supersedes:** the hand-written player in `pkg/export/web.go` (the `playerHTML` template and the `const SCRIPT = …` payload)
**Related:** `pkg/export` (`web.go`, `script.go`), `pkg/scene` (`scene.go`, `compile.go`), `pkg/gui` (`assets.go`, `export.go`), `frontend/src/components/theater/*`, `frontend/src/components/MarkdownProse.tsx`, `frontend/vite.config.ts`
**Deferred:** video parity. The video exporter still composites frames in Go (`pkg/scene/render.go`); when it is revisited it should capture this same player page rather than a second implementation.

## 1. Overview & Goals

An exported bundle is meant to be the campaign, watchable anywhere. Today it is a
hand-written page: art, a scrim, centred plain text, a transport, and `Georgia` for
everything. The in-app theatre is a different design entirely — portrait boxes with an
active-speaker glow, a name plate, a rounded dialogue panel, markdown prose, and the
system's own fonts — and it was redesigned after the export was written. So the two
drifted, and a bundle looks nothing like the app it came from.

**A look must have one implementation.** The bundle player becomes a second build entry
that renders the same `TheaterStage`, `TheaterDialogue`, `TheaterTransport`, and
`MarkdownProse` components the app's theatre renders. Parity then holds by construction,
and the video capture (later) has one page to point a browser at.

**Goals:**

- A bundle's stage, portraits, name plate, dialogue panel, prose styling, and transport
  are the app's, not a lookalike.
- Keep the bundle's typewriter reveal, and keep its audio: the same clip files, in the
  same order, per beat.
- The bundle stays self-contained: no network, no API, opens from `file://`.
- One place to change the look, so the theatre and the export cannot drift again.

**Non-Goals:**

- Video export parity (a later change captures this page; the Go frame renderer stays as
  it is until then).
- Changing the in-app theatre's behaviour.
- Changing the bundled audio: the same `ComputeAudioCacheKeyForVoice` clips, one file per
  sentence, muxed in beat order.

**Success Criteria:**

- The bundle's markup and CSS come from the theatre's components; `pkg/export/web.go`
  contains no presentation of its own.
- A bundle shows portraits for the protagonist and the current speaker, glows the active
  one, and renders prose through the same inline grammar (emphasis, code, wikilinks,
  performance tags) and `display_mode` the app does.
- The typewriter reveal is preserved and pauses with playback.
- `go test ./...`, `npx tsc --noEmit`, and `npm run build` are green, and a manually
  exported bundle of a played campaign matches the theatre.

## 2. Investigation Findings

| Area | Today |
| --- | --- |
| Bundle player | `playerHTML` in `pkg/export/web.go`: `#stage` background, scrim, `#scene`, `#speaker`, `#text` with `textContent` slicing, play/prev/next, progress bar; `Georgia, serif` |
| Bundle payload | `const SCRIPT = {game_name, scenes[{location, art, beats[{kind, speaker, text, art, audio[], duration}]}]}` |
| Theatre | `StoryTheater` composes `TheaterStage` (background, scrim, two portrait boxes, name pills, active glow), `TheaterDialogue` (name plate, quoted speech, `MarkdownProse`), `TheaterTransport` (progress, play/pause, prev/next, speed, audio chips); system sans/serif |
| Portraits in the app | `/api/game/{id}/character/{char}/portrait`; `ent.Portrait` file when generated, else `media.GenerateProceduralBustSVG` |
| Portraits in an export | not exported at all: `scene.Beat` has no portrait field and `pkg/export` never resolves one |
| Player flag | `entity.TurnSegment.Player` marks the protagonist's own line; `scene.Compile` drops it |
| Frontend build | single Vite entry; `outDir` is `pkg/gui/dist`, `emptyOutDir: true`, embedded via `//go:embed all:dist` |
| The SPA itself | not reusable as a bundle: it fetches `/api/…` on load |

- `TheaterStage`, `TheaterDialogue`, and `TheaterTransport` take props only, so a
  non-app runtime can drive them; nothing in them reads the API.
- `TheaterTransport` already covers the bundle's controls (progress, play/pause,
  prev/next, speed); only its tooltips say "turn".
- The export runs inside the app (`gui.Service`, which has the embedded `dist` on hand)
  and from the CLI (`cmd/localrpg/export.go`, which can resolve the same directory).

## 3. Design

### 3.1 One player page, built by Vite

The bundle ships a compiled player instead of a template string:

- `frontend/player.html` plus `frontend/src/player/main.tsx` are the player's entry,
  built by `vite.player.config.ts` into `pkg/gui/dist/player`.
- The entry reads an inlined payload from `window.__LOCALRPG_STORY__` and mounts
  `StoryPlayer`, which composes the theatre's components.
- The exporter copies that whole directory into the bundle and makes its page the
  bundle's `index.html` with the payload inlined.

The player is a build of its own rather than a second entry beside the app: Vite hoists
modules two entries share into common chunks and one stylesheet, which would force a
bundle to ship the app's bundle too. Its config sets `base: './'`, because a bundle is
opened from a file rather than served from a web root.

`StoryPlayer` owns the bundle's runtime, which the theatre cannot supply because a bundle
has no API, no live turns, and no audio device:

```tsx
interface Story {
  game_name: string;
  display_mode?: 'stage_directions' | 'hidden' | 'raw';
  player_portrait?: string;
  scenes: Array<{ location?: string; art?: string; beats: StoryBeat[] }>;
}

interface StoryBeat {
  kind: 'narration' | 'speech' | 'scene_card';
  speaker?: string;
  text: string;
  art?: string;
  portrait?: string;
  audio?: string[];
  duration: number;
  player?: boolean;
}
```

### 3.2 A beat is paced the way the app paces it

```
show beat: background, portraits, name plate, panel
  reveal the text over beat.duration * TypewriterFraction / speed
  start the beat's clips in order, if it has any
advance when: the clips end (plus BEAT_GAP_MS)  or, without clips, the reveal settles
pause: reveal and audio pause together
```

- With clips, the beat waits for them, exactly as the theatre does when the browser or the
  device owns playback: the voice leads, the text follows.
- Without clips (a scene card, a silent beat), the reading-time dwell paces it, and the
  speed control scales that dwell only. Speed never changes an audio clip's rate, so a
  bundle's narration sounds the same at every speed.
- The reveal is the same idea the bundle already has: a prefix of the beat's text through
  `MarkdownProse`, which tolerates an unclosed marker mid-reveal exactly as the app's
  streamed prose does.

A scene card is the one beat the theatre has no equivalent for: it presents the location
name centred over the art, with no dialogue panel, so a scene change reads as a title
rather than as a line of narration. The location also appears in the header pill, as it
does in the theatre.

### 3.3 Portraits travel with the bundle

- `scene.Beat` gains `PortraitPath` and `Player`, and `scene.Script` gains
  `PlayerPortrait`; `scene.Compile` fills them from the segment (`SpeakerID`, `Player`).
- A new `scene.PortraitResolver` (mirroring `ArtResolver`) resolves a character's portrait
  file. `pkg/export` implements it: the entity's own portrait file when it has one,
  otherwise `media.GenerateProceduralBustSVG` written into the cache, which is the same
  fallback the app serves.
- The bundle copies each referenced portrait into `assets/` as
  `portrait-<entityID>.<ext>` and the payload references it relatively, so a bundle opens
  from `file://` with no API.
- Compiling an export reindexes the campaign's notes from Markdown first, as playing one
  does. An export must see the notes as they are on disk rather than as an earlier
  process happened to index them: a portrait the index has not seen is a face missing
  from every bundle.

### 3.4 The exporter supplies the built player

`pkg/export` must not embed or import the frontend build, and the build directory is
gitignored, so the caller passes it:

```go
// NewWebExporter builds an exporter rooted at a campaign directory.
func NewWebExporter(rootDir string) *WebExporter
// SetAssets supplies the built player. Without it, Export fails with a message naming
// the build task rather than writing a bundle nobody can open.
func (w *WebExporter) SetAssets(assets fs.FS)
// SetDisplayMode bakes the campaign's speech-cue display setting into the bundle, so a
// bundle renders performance tags the way the app does.
func (w *WebExporter) SetDisplayMode(mode string)
```

- `pkg/gui` exposes `AssetFS() (fs.FS, error)`, resolving the embedded `dist` with the
  same disk fallbacks `AssetHandler` already uses, and passes it in `gui.Service`.
- `cmd/localrpg/export.go` passes the same filesystem.

### 3.5 Bundle layout

```
index.html            the player page, with the story inlined
assets/player-*.js    the compiled player
assets/player-*.css   its styles
assets/scene-001.svg  scene art
assets/portrait-*.svg portraits (generated PNG, or the procedural bust)
audio/beat-0001.opus  one file per clip, in play order
```

Nothing reads a bundle back, so the layout change breaks nothing.

## 4. Interfaces

```go
// pkg/scene
type Beat struct {
    // …existing fields…
    PortraitPath string
    Player       bool
}
type Script struct {
    // …existing fields…
    PlayerPortrait string
}
type PortraitResolver interface {
    Portrait(ctx context.Context, characterID string) (string, error)
}
// Compiler.SetPortraitResolver enables per-beat portraits. Without one, the
// player falls back to the procedural bust it draws for a missing portrait.

// pkg/export
func (w *WebExporter) SetAssets(assets fs.FS)
func (w *WebExporter) SetDisplayMode(mode string)

// pkg/gui
func AssetFS() (fs.FS, error)

// frontend: window.__LOCALRPG_STORY__: Story (see §3.1)
```

## 5. Error Handling

| Situation | Behaviour |
| --- | --- |
| No player assets supplied | `Export` fails naming `mise run build:frontend`; no bundle is written |
| A portrait cannot be resolved | The beat keeps no portrait, as it does without a voice; the export continues |
| A beat's clip is missing from disk | That clip is not copied and not referenced; the beat keeps its reading-time pacing |
| Art is missing | Unchanged: the payload references no art and the player shows the theatre's fallback gradient |
| A bundle is opened from `file://` | Works: no fetch, no API, all references relative |
| Audio cannot autoplay without a gesture | The transport's existing "Enable audio" affordance, unchanged |
| An empty script | Refused, as today |

## 6. Testing & Verification

Go (stdlib `testing`):

- `pkg/scene`: compiling a two-location source fills `PortraitPath` for speech beats and
  `Player` for the protagonist's own line, and leaves narration without a portrait.
- `pkg/export`: the bundle copies the player assets it was given, names them in
  `index.html`, copies one portrait per distinct character, and references every copied
  file relatively.
- `pkg/export`: a bundle never references `http://` or `https://`, and the payload decodes
  into the `Story` shape the player expects.
- `pkg/export`: with no assets supplied, `Export` fails with the build-task message.

Frontend: `npx tsc --noEmit`, `npm run build`, and the built player's own directory under
`pkg/gui/dist/player`.

Browser: `TestExportedBundlePlaysTheTheatre` exports a played campaign, opens the bundle
over `file://` in headless Chrome, and asserts the theatre's own furniture renders, that
the protagonist's portrait is on the stage, that the dialogue panel shows the narrator and
the prose, and that the story advances to the speech beat with its speaker and portrait.
It skips when no browser or no built player is present, as the launcher's browser test does.

Manual: export a played campaign, open `index.html`, and compare against the theatre:
same portraits and glow, same panel and prose, same audio, typewriter intact.

## 7. Compatibility & Rollout

- Bundles change shape (`window.__LOCALRPG_STORY__`, a shipped player build). Nothing reads
  a bundle back, and a bundle is not persisted anywhere the app consumes.
- `pkg/scene` gains fields; the video pipeline ignores them, so the video's output is
  unchanged by this design.
- The CLI gains a dependency on `pkg/gui`'s asset resolution, which is already built into
  the same binary.
- A machine that has never run `mise run build:frontend` cannot export a bundle. The
  failure names the task instead of writing a broken page.

## 8. Open Questions

- Should a bundle ship the theatre's header (game name, location pill, scene counter), or
  a slimmer one with just the game name?
- Should the scene card become an optional export setting, so a bundle can present the
  story with no title cards at all?
- When the video is revisited: capture this page in a browser (exact, needs a browser
  binary) or keep the Go renderer and accept a near-match?

## 9. References

- Code: `pkg/export/{web,script}.go`, `pkg/scene/{scene,compile}.go`, `pkg/gui/{assets,export}.go`,
  `frontend/vite.config.ts`, `frontend/src/components/theater/*`,
  `frontend/src/components/MarkdownProse.tsx`, `frontend/src/components/StoryTheater.tsx`,
  `pkg/media/procedural_bust.go`
- Specs: `2026-09-21-animated-export-and-video-design.md` (its "no headless browser" and
  font decisions are what this design revisits for the bundle),
  `2026-09-26-theater-native-audio-and-visual-novel-layout-design.md`
