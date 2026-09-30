# Single-File Web Bundle Design

**Date:** 2026-09-30
**Status:** Proposed
**Scope:** Make an exported bundle one self-contained `.html` file — player, styles, story, art, portraits, and clips inlined — so it opens from `file://` with no flags, no server, and no sidecar directory
**Supersedes:** the directory layout and asset copying in `2026-09-29-export-player-parity-design.md` (§3.4, §3.5); its parity decisions (the theatre's own components, portraits, the typewriter) still stand
**Related:** `pkg/export` (`web.go`, `script.go`), `pkg/scene/compile.go`, `pkg/gui/export.go`, `cmd/localrpg/export.go`, `frontend/vite.player.config.ts`

## 1. Overview & Goals

A bundle exported as a directory of sidecars does not open. Chrome refuses both the
player's module script and its stylesheet over `file://` — *"Cross origin requests are
only supported for protocol schemes: … data, http, https"* — so the page loads, runs
nothing, and shows white. The user who reported it also found no audio in the folder,
which is a separate failure: a campaign whose clips could not be synthesized exports
silently, and the only word of it is a progress line.

**One file, and nothing to fetch.** Everything a bundle needs is inlined: the player as a
classic script, its styles as a `<style>` block, and every asset as a `data:` URI. A bundle
then opens by double-clicking it, and it can be moved, mailed, or archived as a single
artefact.

**Goals:**

- One `.html` file that plays from `file://` in a browser with no flags and no server.
- The page is compressed, because the player's own code is a fixed third of a megabyte and
  prose and art compress well; a bundle is smaller than the text it carries.
- The theatre's components still render it: the parity design is unchanged in substance.
- No reference to anything outside the file: no sidecars, no network, no module scripts.
- A bundle with no clips says so, and says why, instead of shipping silence.

**Non-Goals:**

- The video exporter, which is untouched.
- Deduplicating identical clips or portraits; they are carried as they are.
- Re-encoding clips to a smaller bitrate (see §8: it is the lever that matters for a long
  campaign, and it changes what the narration sounds like).
- Keeping the directory layout: a bundle is not read back by anything, so there is nothing
  to be compatible with.

**Success Criteria:**

- `localrpg export web` writes one file, and opening it in a browser plays the story.
- The page contains no `http(s)://`, no `src=`/`href=` pointing at a sidecar, and no
  `type="module"` script.
- Art, portraits, and clips are present as `data:` URIs, one per referenced asset.
- The browser test loads a bundle from `file://` with no permissive flags and asserts the
  theatre renders and advances.
- `go test ./...`, `npx tsc --noEmit`, and `npm run build` are green.

## 2. Investigation Findings

Evidence, from a headless Chrome loading an exported bundle with no flags:

```
log: Access to script at 'file:///…/assets/player-BJYVQWwx.js' from origin 'null' has been
     blocked by CORS policy: Cross origin requests are only supported for protocol
     schemes: chrome, …, data, http, https, isolated-app.
log: Access to CSS stylesheet at 'file:///…/assets/player-ILxkHeum.css' from origin 'null'
     has been blocked by CORS policy: …
```

- The player is built by Vite as an ES module, and a module script is *always* fetched with
  CORS, which an opaque `file://` origin can never satisfy. A classic script is not.
- `TestExportedBundlePlaysTheTheatre` passed only because it launched Chrome with
  `--allow-file-access-from-files`. That flag is what hid the defect; the test now runs
  without it.
- Audio is exported when clips exist: an export of a two-line campaign against a TTS
  provider wrote `audio/beat-0001.opus` and `audio/beat-0002.opus`. A campaign whose
  synthesis fails resolves no clips and exports silence, and `scene.Compile` reports only
  a count ("N beats have no audio clip and will play silently"), not the reason.
- `data:` is in the allowed scheme list, so a `data:` URI is fetchable from a `file://`
  page, and `<img>`/`<audio>` accept them.
- The exporter already resolves every asset to a local path before copying it, so inlining
  is a change of encoding rather than of resolution.
- Measured on a realistic mix — 60 beats of distinct prose, 20 SVG scenes, 60 distinct
  ~18 KB clips, and the built player — gzip does this:

  | Part | Raw | Gzipped |
  | --- | --- | --- |
  | Story (prose, art, base64 clips) | 1655 KB | 1104 KB |
  | Player script | 234 KB | 74 KB |
  | Player stylesheet | 101 KB | 15 KB |
  | **Whole page, base64 for the page to hold** | **1989 KB** | **1591 KB (20% smaller)** |

  The clips are Opus and already compressed, so the win is the text around them: a silent
  bundle — where the player's own build dominates — is roughly 70% smaller. Base64 costs a
  third of the *compressed* size, which is why the page is 1591 KB rather than 1193 KB.

## 3. Design

### 3.1 The player is a classic script

`vite.player.config.ts` builds the player as one classic IIFE with no code splitting and
one stylesheet:

```ts
build: {
  outDir: '../pkg/gui/dist/player',
  emptyOutDir: true,
  cssCodeSplit: false,
  rollupOptions: {
    input: { player: 'player.html' },
    output: { format: 'iife', inlineDynamicImports: true },
  },
}
```

A classic script needs no CORS and no module resolution, so inlining its text into the page
is enough for it to run.

### 3.2 The exporter inlines the page

`pkg/export` still takes the built player as an `fs.FS`, and now reads it rather than
copying it:

1. Read `player/player.html` — the page skeleton, so the markup still has one author.
2. Follow the page's own references to the player's assets (`href="./assets/*.css"`,
   `src="./assets/*.js"`), replacing each with the file's contents: a `<style>` block and a
   `<script>` block. The build's hashed names are never hardcoded.
3. Inline the story payload as `window.__LOCALRPG_STORY__`, with every asset already a
   `data:` URI.
4. Write one file, `<outDir>/<game-id>-web.html`, and report its path and size.

### 3.3 Assets are data URIs

The payload keeps its shape — `art`, `portrait`, `player_portrait`, `audio[]` — and each
value becomes `data:<mime>;base64,<bytes>`, mapped by extension:

| Extension | MIME |
| --- | --- |
| `.svg` | `image/svg+xml` |
| `.png` | `image/png` |
| `.jpg`, `.jpeg` | `image/jpeg` |
| `.webp` | `image/webp` |
| `.opus`, `.ogg` | `audio/ogg` |
| `.wav` | `audio/wav` |
| `.mp3` | `audio/mpeg` |
| `.flac` | `audio/flac` |
| anything else | `application/octet-stream` |

An asset that cannot be read is skipped, and the beat keeps the pacing it was compiled
with: a missing face or clip degrades a beat, never the bundle.

Base64 inflates audio by a third. A campaign's narration is Opus at a speech bitrate, so a
long campaign is tens of megabytes in one file — large, but a file that plays beats a
folder that does not. The export reports the written size so the cost is visible.

### 3.4 The page carries one compressed bundle

The built page's stylesheet and script references are replaced with a single gzipped,
base64 bundle and a small bootstrap:

```html
<script id="localrpg-bundle" type="application/octet-stream">H4sI…</script>
<script>
(async function () {
  var bytes = Uint8Array.from(atob(document.getElementById('localrpg-bundle').textContent.trim()), c => c.charCodeAt(0));
  var bundle = JSON.parse(await new Response(new Blob([bytes]).stream().pipeThrough(new DecompressionStream('gzip'))).text());
  // …append bundle.css as a <style>, window.__LOCALRPG_STORY__ = bundle.story, then
  // append bundle.js as a classic <script>, which starts the player.
})();
</script>
```

- The bundle is `{js, css, story}`: the player's script, its stylesheet, and the story with
  its assets as `data:` URIs.
- The player is unchanged: it still reads `window.__LOCALRPG_STORY__`, so compression lives
  entirely in the exporter and the bootstrap.
- `DecompressionStream` is the browser's own gzip decoder (Chrome 80+, Firefox 113+,
  Safari 16.4+), so no library and no tool is involved. A browser without it is told so
  rather than shown a blank page.
- The bootstrap is the one part that cannot be compressed, and it is about a kilobyte.

### 3.5 A silent bundle says why

`scene.Compile` keeps the first synthesis error and reports it with the count:

```
3 beats have no audio clip (last error: synthesize utterance: provider returned 429) and
will play silently
```

The GUI renders a progress event's message, and the CLI prints it, so an export that could
not speak says so rather than producing a quiet file with no explanation.

## 4. Interfaces

```go
// pkg/export
// Export writes one self-contained page and returns its path.
func (w *WebExporter) Export(ctx context.Context, script *scene.Script, outPath string) (string, error)
func (w *WebExporter) SetAssets(assets fs.FS)      // the built player, as before
func (w *WebExporter) SetDisplayMode(mode string)  // baked into the payload, as before

// pkg/gui
// exportArtifactPath names a bundle: <outDir>/<game-id>-web.html

// frontend
// window.__LOCALRPG_STORY__: Story, with data: URIs for art, portraits, and audio
```

`Export` takes the destination file rather than a directory, so a caller no longer creates
one for it.

## 5. Error Handling

| Situation | Behaviour |
| --- | --- |
| No player assets supplied | Refused, naming `mise run build:frontend`; nothing is written |
| The page references a player asset that is missing | Refused: a page that cannot boot is not written |
| An asset file cannot be read | Skipped; the beat keeps its compiled pacing |
| An asset's bytes are not a format this recognises | The extension decides; anything unknown is `application/octet-stream` |
| A clip is cached under a name its bytes contradict | The bytes decide the type, as the app's own serving does: a browser refuses a data URI whose type does not match its contents |
| A beat resolved no clip | Reported in the export's progress, with the reason |
| An empty script | Refused, as today |
| A very large campaign | Written anyway; the size is reported |
| The browser has no `DecompressionStream` | The page says the bundle cannot be decompressed here, and to open it in a current browser |
| The bundle's payload will not inflate or parse | The same message, with the reason |

## 6. Testing & Verification

Go (stdlib `testing`):

- One file is written, and no `assets/` or `audio/` directory exists beside it.
- The page inlines the player: the stub's script and stylesheet text appear in it, and no
  `src="./assets/` or `href="./assets/` remains, and no `type="module"`.
- The page references nothing outside itself: no `http://`, no `https://`, no `//`.
- The payload decodes; art, portraits, and every clip are `data:` URIs with the right MIME.
- Without assets, the export fails and writes nothing.
- The page carries one compressed bundle: the holder and the bootstrap are present, the
  story is not inlined uncompressed, and the blob is gzip (its magic bytes) and decodes to
  the player's script, its stylesheet, and the story.
- The page is smaller than the parts it holds, and the size is reported as the export
  finishes.
- An export whose beats resolved no clips reports the count and the first error.

Browser: `TestExportedBundlePlaysTheTheatre` exports, opens the file from `file://` with no
permissive flags, and asserts the theatre renders, the protagonist's portrait is present,
the dialogue panel shows the narrator and the prose, and the story advances to the speech
beat. Console output is captured so a failure says what the page refused to load.

Manual: export a played campaign, double-click the file, and watch it play.

## 7. Compatibility & Rollout

- The web artefact changes from a directory to a file. Nothing reads a bundle back, and no
  bundle is referenced by the app, so there is nothing to migrate.
- `Export`'s signature changes from a directory to a file path; the GUI and CLI are its
  only callers.
- The video exporter is untouched, and `pkg/scene`'s additions are unchanged apart from
  carrying the first synthesis error for the report.

## 8. Open Questions

- Should a large campaign be offered a sidecar variant, or a split across several files?
- Should clips be re-encoded at a lower bitrate for a bundle? Base64 pays a third more on
  top of Opus, and gzip cannot touch either, so this is the only lever left for a long
  campaign: halving the bitrate halves the bulk of the file.
- Should the export warn before writing a file above some size, the way a bulk synthesis
  warns before spending money?

## 9. References

- Code: `pkg/export/{web,script}.go`, `pkg/scene/compile.go`, `pkg/gui/export.go`,
  `cmd/localrpg/export.go`, `frontend/vite.player.config.ts`,
  `pkg/gui/export_player_e2e_test.go`
- Specs: `2026-09-29-export-player-parity-design.md` (the look this preserves),
  `2026-09-21-animated-export-and-video-design.md`
