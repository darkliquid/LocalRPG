# Exported Player Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make an exported web bundle play the story through the same components the in-app theatre uses, so a bundle looks and sounds like theatre mode, with the typewriter reveal kept.

**Architecture:** A second Vite entry (`player.html` + `src/player/main.tsx`) renders `StoryPlayer`, which composes the theatre's `TheaterStage`, `TheaterDialogue`, `TheaterTransport`, and `MarkdownProse` against an inlined `window.__LOCALRPG_STORY__` payload. `pkg/scene` carries portraits and the protagonist flag; `pkg/export` resolves portraits (entity file, else the procedural bust), copies the built player and the portraits into the bundle, and writes an `index.html` that owns no presentation. The video exporter is untouched.

**Tech Stack:** Go 1.27 (stdlib `testing`, `testing/fstest`), React 19 + TypeScript (strict), Vite 6 multi-entry, Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-09-29-export-player-parity-design.md`

## Global Constraints

- Module path `github.com/darkliquid/localrpg`; `interface{}`, never `any`; `go vet` clean.
- Tests use the standard library only (`testing`, `t.TempDir()`, `testing/fstest`); no testify.
- The bundle must reference nothing outside itself: no `http://`, no fetch, relative paths only.
- The player's look comes from the theatre's components; `pkg/export` must not carry its own CSS or markup beyond the payload and asset tags.
- Audio stays identical: the same clips, one file per sentence, copied in play order.
- `frontend/src` has `strict`, `noUnusedLocals`, `noUnusedParameters`: `npx tsc --noEmit` is the frontend gate.
- Conventional Commits with a scope; subject under 72 chars.

---

## File Map

| File | Responsibility after this change |
| --- | --- |
| `frontend/src/player/main.tsx` | (new) second entry: reads `window.__LOCALRPG_STORY__`, mounts `StoryPlayer` |
| `frontend/player.html` | (new) the player page a bundle ships |
| `frontend/src/components/story/StoryPlayer.tsx` | (new) scripted runtime: beat pacing, typewriter, clips, transport |
| `frontend/src/components/theater/TheaterDialogue.tsx` | gains a `reveal` prop so a prefix of a line can be shown |
| `frontend/src/components/theater/TheaterTransport.tsx` | gains labels so its tooltips fit a bundle's scenes |
| `frontend/vite.config.ts` | two entries, pinned output names for the player |
| `pkg/scene/scene.go` | `Beat.PortraitPath`, `Beat.Player`, `Script.PlayerPortrait` |
| `pkg/scene/compile.go` | fills them; gains `PortraitResolver` |
| `pkg/export/script.go` | resolves a character's portrait file and the protagonist's |
| `pkg/export/web.go` | payload + asset tags; copies player assets and portraits; no presentation |
| `pkg/gui/assets.go` | `AssetFS()`: the embedded `dist` with the existing disk fallbacks |
| `pkg/gui/export.go` | passes `AssetFS()` to the exporter |
| `cmd/localrpg/export.go` | same, so the CLI writes the same bundle |

---

### Task 1: The player runtime and its page

**Files:**
- Create: `frontend/player.html`, `frontend/src/player/main.tsx`, `frontend/src/components/story/StoryPlayer.tsx`, `frontend/src/components/story/types.ts`
- Modify: `frontend/src/components/theater/TheaterDialogue.tsx`, `frontend/src/components/theater/TheaterTransport.tsx`
- Test: `npx tsc --noEmit`

**Interfaces:**
- Produces: `Story`, `StoryBeat` types; `StoryPlayer: React.FC<{ story: Story }>`; `TheaterDialogue` accepts `reveal?: number`; `TheaterTransport` accepts `labels?: { prev?: string; next?: string }`.

- [x] **Step 1: Add the reveal and the labels**

`TheaterDialogue` gains `reveal = 1` and shows `text.slice(0, Math.max(1, Math.round(text.length * reveal)))` when `reveal < 1`, so the typewriter is a property of the panel rather than of a second renderer:

```tsx
const text = segment?.text ?? fallback;
const shown = reveal >= 1 ? text : text.slice(0, Math.max(1, Math.round(text.length * reveal)));
// …and the panel renders {shown}, with the caret hidden while revealing:
{reveal < 1 ? null : <span className="absolute bottom-2 right-3 text-purple-300/70 animate-pulse text-xs" aria-hidden="true">&#9662;</span>}
```

`TheaterTransport` gains `labels?: { prev?: string; next?: string }`, defaulting to the existing "Previous turn"/"Next turn".

- [x] **Step 2: Write the story types**

`frontend/src/components/story/types.ts`:

```ts
export interface StoryBeat {
  kind: 'narration' | 'speech' | 'scene_card';
  speaker?: string;
  text: string;
  art?: string;
  portrait?: string;
  audio?: string[];
  duration: number;
  player?: boolean;
}

export interface Story {
  game_name: string;
  display_mode?: 'stage_directions' | 'hidden' | 'raw';
  player_portrait?: string;
  scenes: Array<{ location?: string; art?: string; beats: StoryBeat[] }>;
}

declare global {
  interface Window {
    __LOCALRPG_STORY__?: Story;
  }
}
```

- [x] **Step 3: Write the player**

`StoryPlayer` flattens scenes into `{scene, beat}[]`, then for each beat: shows it, reveals its text over `duration * TypewriterFraction / speed`, plays its clips in order, and advances when the clips end (plus `BEAT_GAP_MS`) or, without clips, once the reveal settles. It reuses the theatre's components and its colour rules (`player` beats glow sky, others purple; the protagonist's portrait sits left, the speaker's right, mirrored as the theatre mirrors it).

Key parts:

```tsx
const TYPEWRITER_FRACTION = 0.6;
const BEAT_GAP_MS = 300;

const StoryPlayer: React.FC<{ story: Story }> = ({ story }) => {
  const beats = useMemo(
    () => story.scenes.flatMap((scene, sceneIndex) => scene.beats.map((beat, beatIndex) => ({ scene, sceneIndex, beat, beatIndex }))),
    [story]
  );
  const [index, setIndex] = useState(0);
  const [reveal, setReveal] = useState(1);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState(1);

  // …reveal timer, clip queue, pause/stop, prev/next, progress, header…
};
```

- [x] **Step 4: Write the entry and the page**

`frontend/player.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Story Theater</title>
<link rel="stylesheet" href="/assets/player.css">
</head>
<body class="bg-stone-950">
<div id="story-player" class="fixed inset-0"></div>
<script>window.__LOCALRPG_STORY__ = window.__LOCALRPG_STORY__ || undefined;</script>
<script type="module" src="/src/player/main.tsx"></script>
</body>
</html>
```

`main.tsx` mounts `StoryPlayer` when a payload is present and renders a short "no story" note otherwise.

- [x] **Step 5: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

---

### Task 2: Build the player as a second entry with pinned names

**Files:**
- Modify: `frontend/vite.config.ts`

**Interfaces:**
- Produces: `pkg/gui/dist/player.html`, `pkg/gui/dist/assets/player.js`, `pkg/gui/dist/assets/player.css`.

- [x] **Step 1: Add the entry**

```ts
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: { port: 3000, proxy: { '/api': 'http://localhost:8080' } },
  build: {
    outDir: path.resolve(__dirname, '../pkg/gui/dist'),
    emptyOutDir: true,
    rollupOptions: {
      input: {
        index: path.resolve(__dirname, 'index.html'),
        player: path.resolve(__dirname, 'player.html'),
      },
      output: {
        entryFileNames: (chunk) => (chunk.name === 'player' ? 'assets/player.js' : 'assets/[name]-[hash].js'),
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: (asset) =>
          asset.names?.includes('player.css') ? 'assets/player.css' : 'assets/[name]-[hash][extname]',
      },
    },
  },
});
```

- [x] **Step 2: Build and confirm the names**

Run: `cd frontend && npm run build && ls ../pkg/gui/dist/assets/player.js ../pkg/gui/dist/assets/player.css ../pkg/gui/dist/player.html`
Expected: all three exist. If Vite hashes the player CSS anyway, name the CSS by importing it from the entry and keep `assetFileNames` handling the emitted CSS asset, then re-run until the three pinned names exist.

- [x] **Step 3: Confirm the app still builds**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

---

### Task 3: Scene carries portraits and the protagonist flag

**Files:**
- Modify: `pkg/scene/scene.go`, `pkg/scene/compile.go`
- Test: `pkg/scene/compile_test.go`

**Interfaces:**
- Produces: `Beat.PortraitPath string`, `Beat.Player bool`, `Script.PlayerPortrait string`, `PortraitResolver`, `Compiler.SetPortraitResolver`, `Options.PlayerID string`.

- [x] **Step 1: Write the failing test**

```go
type fakePortraits struct{ paths map[string]string }

func (f *fakePortraits) Portrait(_ context.Context, characterID string) (string, error) {
	if path, ok := f.paths[characterID]; ok {
		return path, nil
	}
	return "", ErrAudioUnavailable
}

func TestCompileCarriesPortraitsAndThePlayerFlag(t *testing.T) {
	source := twoLocationSource()
	compiler := NewCompiler(source)
	compiler.SetPortraitResolver(&fakePortraits{paths: map[string]string{
		"sean": "/cache/portrait-sean.svg",
		"elen": "/cache/portrait-elen.svg",
	}})

	script, err := compiler.Compile(context.Background(), "campaign-01", Options{PlayerID: "sean"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if script.PlayerPortrait != "/cache/portrait-sean.svg" {
		t.Errorf("PlayerPortrait = %q, want the protagonist's", script.PlayerPortrait)
	}

	var speech *Beat
	for _, beat := range script.Beats() {
		if beat.Kind == BeatSpeech {
			speech = &beat
		}
		if beat.Kind == BeatNarration && beat.PortraitPath != "" {
			t.Errorf("narration gained a portrait: %+v", beat)
		}
	}
	if speech == nil {
		t.Fatal("the fixture has no speech beat")
	}
	if speech.PortraitPath != "/cache/portrait-elen.svg" {
		t.Errorf("speech portrait = %q, want the speaker's", speech.PortraitPath)
	}
}
```

Use the fixture's existing speaker (`twoLocationSource` speaks as "Evelyn"/`evelyn`; adjust the fake's keys to the fixture's speaker ID so the assertion is real).

- [x] **Step 2: Run it to watch it fail**

Run: `go test ./pkg/scene/ -run CarriesPortraits -v`
Expected: compile failure, `SetPortraitResolver undefined`.

- [x] **Step 3: Implement**

`pkg/scene/scene.go`: add `PortraitPath string` and `Player bool` to `Beat`, and `PlayerPortrait string` to `Script`.

`pkg/scene/compile.go`:

```go
// PortraitResolver returns a character's portrait file, or ErrAudioUnavailable when
// the character has none.
type PortraitResolver interface {
	Portrait(ctx context.Context, characterID string) (string, error)
}
```
`Options` gains `PlayerID string`; the compiler resolves `opts.PlayerID` once into `Script.PlayerPortrait`, and per speech beat resolves `segment.SpeakerID` (falling back to the slug of `segment.Speaker`), ignoring failures so a portrait is decoration and never a failure. Copy `segment.Player` onto the beat.

- [x] **Step 4: Run the scene tests**

Run: `go test ./pkg/scene/ -count=1`
Expected: PASS.

---

### Task 4: The exporter resolves portraits

**Files:**
- Modify: `pkg/export/script.go`
- Test: `pkg/export/script_test.go`

**Interfaces:**
- Consumes: `scene.PortraitResolver`, `media.GenerateProceduralBustSVG`.
- Produces: `ScriptCompiler` sets a portrait resolver, and `Options.PlayerID` is the manifest's player.

- [x] **Step 1: Write the failing test**

```go
func TestCompileResolvesPortraits(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "portraits")
	if err := os.MkdirAll(filepath.Join(gameDir, "entities"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(gameDir, "game.yaml"), "id: portraits\nname: Portraits\nsystem: freeform\nworld: harbour\nplayer: sean\n")
	writeFile(t, filepath.Join(gameDir, "entities", "sean.md"),
		"---\nid: sean\nname: Sean\ntype: character\nportrait: assets/portraits/sean.png\n---\nA traveller.\n")
	writeFile(t, filepath.Join(gameDir, "assets", "portraits", "sean.png"), "png-bytes")
	// …a history.jsonl with one narration turn, so the script has a beat…

	script, err := NewScriptCompiler(root).Compile(context.Background(), "portraits")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if script.PlayerPortrait == "" {
		t.Fatal("expected the protagonist's portrait to resolve")
	}
}
```

Seed the store the compiler opens (`storage.Syncer` over `entities/`) the way `TestCompileReplayScript` does, so the entity is indexed.

- [x] **Step 2: Run it to watch it fail**

Run: `go test ./pkg/export/ -run ResolvesPortraits -v`
Expected: FAIL, `PlayerPortrait` is empty.

- [x] **Step 3: Implement**

In `pkg/export/script.go`, alongside `speechResolver`:

```go
// portraitResolver serves a character's portrait: the note's own file when it has
// one, otherwise the procedural bust the app serves for a character without art.
type portraitResolver struct {
	resolver *core.PathResolver
	store    *storage.Store
	gameID   string
	cache    *media.ContentCache
}

func (r *portraitResolver) Portrait(ctx context.Context, characterID string) (string, error) {
	if characterID == "" {
		return "", scene.ErrAudioUnavailable
	}
	ent, err := r.store.GetEntity(characterID)
	if err != nil || ent == nil {
		return "", scene.ErrAudioUnavailable
	}
	if strings.TrimSpace(ent.Portrait) != "" {
		path := filepath.Join(r.resolver.GameDir(r.gameID), ent.Portrait)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	name := ent.Name
	if strings.TrimSpace(name) == "" {
		name = characterID
	}
	return r.cache.Put("export-portraits", characterID+".svg", media.GenerateProceduralBustSVG(ent.ID, name, ent.Gender))
}
```

`Compile` sets it (`compiler.SetPortraitResolver(...)`) and passes `Options.PlayerID = manifest.Player`. Add `media.GenerateProceduralBustSVG`'s third argument from `entity.Entity`'s gender field (check its name in `pkg/entity`).

- [x] **Step 4: Run the export tests**

Run: `go test ./pkg/export/ -count=1`
Expected: PASS.

---

### Task 5: The bundle ships the built player

**Files:**
- Modify: `pkg/export/web.go`
- Test: `pkg/export/web_test.go`

**Interfaces:**
- Consumes: `scene.Script.PlayerPortrait`, `Beat.PortraitPath`, `Beat.Player`, `fs.FS`.
- Produces: `func (w *WebExporter) SetAssets(assets fs.FS)`; bundle layout `index.html`, `assets/player.js`, `assets/player.css`, `assets/scene-*.svg`, `assets/portrait-*.svg`, `audio/beat-*.opus`.

- [x] **Step 1: Write the failing tests**

```go
func playerAssets() fs.FS {
	return fstest.MapFS{
		"player.html":        &fstest.MapFile{Data: []byte("<html></html>")},
		"assets/player.js":   &fstest.MapFile{Data: []byte("console.log('player')")},
		"assets/player.css":  &fstest.MapFile{Data: []byte("body{background:#000}")},
	}
}

func TestWebExportShipsTheTheatrePlayer(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	dir := t.TempDir()

	exporter := NewWebExporter(".")
	exporter.SetAssets(playerAssets())
	if _, err := exporter.Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	for _, want := range []string{"index.html", "assets/player.js", "assets/player.css"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(want))); err != nil {
			t.Errorf("expected %s in the bundle: %v", want, err)
		}
	}

	page, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !strings.Contains(string(page), `src="assets/player.js"`) || !strings.Contains(string(page), `href="assets/player.css"`) {
		t.Errorf("index.html does not reference the player assets:\n%s", page)
	}
	if !strings.Contains(string(page), "__LOCALRPG_STORY__") {
		t.Errorf("index.html does not inline the payload:\n%s", page)
	}
	if strings.Contains(string(page), "http://") || strings.Contains(string(page), "https://") {
		t.Error("a bundle must not reach out to the network")
	}
}

func TestWebExportWithoutPlayerAssetsFails(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "bundle")
	_, err := NewWebExporter(".").Export(context.Background(), fixtureScript(t, dir), out)
	if err == nil || !strings.Contains(err.Error(), "build:frontend") {
		t.Fatalf("err = %v, want a message naming the frontend build", err)
	}
}
```

- [x] **Step 2: Run them to watch them fail**

Run: `go test ./pkg/export/ -run 'TheatrePlayer|PlayerAssetsFails' -v`
Expected: compile failure, `SetAssets undefined`.

- [x] **Step 3: Implement**

`pkg/export/web.go`:

```go
func (w *WebExporter) SetAssets(assets fs.FS) { w.assets = assets }

func (w *WebExporter) Export(ctx context.Context, script *scene.Script, outDir string) (string, error) {
	if script == nil || len(script.Scenes) == 0 {
		return "", fmt.Errorf("script has no scenes to export")
	}
	if w.assets == nil {
		return "", fmt.Errorf("web export needs the built player: run `mise run build:frontend`")
	}
	if err := w.copyPlayerAssets(outDir); err != nil {
		return "", err
	}
	// …existing scene/beat loop, now also copying portraits and recording them…
	// payload gains display_mode and player_portrait; the page is:
	// <link rel="stylesheet" href="assets/player.css">
	// <script>window.__LOCALRPG_STORY__ = {…};</script>
	// <script type="module" src="assets/player.js"></script>
}
```

Copy a portrait per distinct character, named `portrait-<id><ext>`, and reference it from the beat. Beat `audio` handling is unchanged; art handling is unchanged apart from naming.

- [x] **Step 4: Run the export tests**

Run: `go test ./pkg/export/ -count=1`
Expected: PASS.

---

### Task 6: Wire the callers

**Files:**
- Modify: `pkg/gui/assets.go`, `pkg/gui/export.go`, `cmd/localrpg/export.go`
- Test: `pkg/gui/export_test.go`, `cmd/localrpg` (build)

**Interfaces:**
- Produces: `func AssetFS() (fs.FS, error)`.

- [x] **Step 1: Add the asset filesystem**

`pkg/gui/assets.go` exposes the same resolution `AssetHandler` uses, so the exporter and the SPA serve one build:

```go
// AssetFS returns the built frontend: the embedded copy, or a local build during
// development. It is what an export ships as its player.
func AssetFS() (fs.FS, error) {
	if distFS, err := fs.Sub(embeddedDist, "dist"); err == nil {
		if _, err := fs.Stat(distFS, "player.html"); err == nil {
			return distFS, nil
		}
	}
	for _, candidate := range []string{"frontend/dist", "pkg/gui/dist"} {
		if _, err := os.Stat(filepath.Join(candidate, "player.html")); err == nil {
			return os.DirFS(candidate), nil
		}
	}
	return nil, fmt.Errorf("no built player found: run `mise run build:frontend`")
}
```

- [x] **Step 2: Pass it from both callers**

`pkg/gui/export.go`: resolve `AssetFS()` when the export starts and call `SetAssets`, reporting the error through the existing export failure path. `cmd/localrpg/export.go`: same, exiting with the message when it fails.

- [x] **Step 3: Verify**

Run: `go build ./... && go test ./pkg/gui/ -run Export -count=1`
Expected: builds; export tests pass. If a GUI export test constructs an exporter without assets, give it `fstest.MapFS` assets.

---

### Task 7: Full verification

- [x] **Step 1: Gates**

Run: `mise run build:frontend && mise run test && mise run lint`
Expected: the bundle builds, Go tests and `tsc` pass, markdownlint and `go vet` are clean.

- [x] **Step 2: An exported bundle is self-contained**

Run: `rg -n "http://|https://|fetch\(" pkg/gui/dist/assets/player.js | head`
Expected: no network access in the player. (React's dev warnings may mention URLs in comments; only real requests matter.)

- [x] **Step 3: Manual parity check**

Export a played campaign from the app, open `index.html`, and compare against the theatre: the same portraits and glow, the same dialogue panel and prose, the same clips, the typewriter intact. Note any difference in the plan's follow-ups.

- [x] **Step 4: Commit**

```bash
git add frontend pkg/scene pkg/export pkg/gui cmd docs
git commit -m "feat(export): play a bundle through the theatre's own components"
```

---

---

## Implementation Notes (added during execution)

**Follow-up (2026-09-30):** an export now uses the app's own pipelines — the campaign's
narrator voice, the shared scene-art resolver, and the shared speech pipeline — because
building its own clients made every clip a cache miss (silent bundles) and left the
campaign banner out entirely. The banner travels with the bundle, and the portrait images
carry their own rounding so a mirrored portrait is clipped in every engine. See the spec's
§10 for the symptoms and their causes.

Three deviations from the tasks above, all recorded in the spec as well:

1. **The player is its own build, not a second entry.** A second `input` beside the app
   made Vite hoist everything the two share into common chunks and one stylesheet named
   after the app, so a bundle would have had to ship the app's bundle to get the player
   running. `vite.player.config.ts` builds `player.html` alone into `pkg/gui/dist/player`
   with `base: './'`, and the export copies that directory and makes its page the
   bundle's `index.html`.
2. **Compiling an export reindexes the campaign first.** Portraits and location names come
   from the index, and an export of a campaign no process had indexed yet resolved no
   portraits at all. `ScriptCompiler.Compile` now syncs `entities/` from Markdown, exactly
   as `localrpg play` and the GUI do.
3. **The transport's icon buttons carry `data-transport` hooks.** The bundle's browser test
   has to press Next, and selecting an icon-only button by its tooltip couples the test to
   copy. The hooks are inert markup.

The payload also carries `display_mode`, which the plan's Task 5 said to include and the
spec now lists under `SetDisplayMode`.

## Self-Review

**1. Spec coverage**

| Spec requirement | Task |
| --- | --- |
| §3.1 second entry, payload, `StoryPlayer` | 1, 2 |
| §3.2 beat pacing, typewriter, speed, scene card | 1 |
| §3.3 portraits on beats/script, resolver, procedural fallback | 3, 4 |
| §3.4 assets filesystem, named failure without it | 5, 6 |
| §3.5 bundle layout | 5, 6 |
| §6 Go tests, tsc, manual parity | 3, 4, 5, 7 |

**2. Placeholder scan:** no TBDs; the two code steps that say "…existing scene/beat loop" are inside `pkg/export/web.go`, whose current shape is quoted in Task 5's interface list, and the payload keys are enumerated in the spec's §3.1.

**3. Type consistency:** `Story`/`StoryBeat` (Task 1) match the payload keys the exporter writes (Task 5); `PortraitPath`/`Player`/`PlayerPortrait` (Task 3) are the fields Task 4 fills and Task 5 exports; `SetAssets(fs.FS)` (Task 5) is what Task 6 supplies.
