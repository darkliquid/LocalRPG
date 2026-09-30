# Single-File Web Bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make an exported bundle one self-contained `.html` file that plays from `file://`, by building the player as a classic script and inlining the page, the story, and every asset.

**Architecture:** `vite.player.config.ts` emits one classic IIFE and one stylesheet. `pkg/export` reads the built page, replaces its stylesheet and script references with inline `<style>`/`<script>` blocks, encodes art, portraits, and clips as `data:` URIs in the payload, and writes a single file. `scene.Compile` reports why a bundle is silent.

**Tech Stack:** Go 1.27 (stdlib `testing`, `testing/fstest`), React 19 + TypeScript, Vite 6, chromedp.

**Spec:** `docs/superpowers/specs/2026-09-30-single-file-web-bundle-design.md`

## Global Constraints

- Module path `github.com/darkliquid/localrpg`; `interface{}`, never `any`; `go vet` clean.
- Tests use the standard library only; no testify.
- The page must reference nothing outside itself: no sidecars, no network, no module scripts.
- The player's look stays the theatre's components; only delivery changes.
- The video exporter is not touched.
- `frontend/src` is `strict` with `noUnusedLocals`/`noUnusedParameters`: `npx tsc --noEmit` is the gate.
- Conventional Commits with a scope; subject under 72 chars.

---

## File Map

| File | Responsibility after this change |
| --- | --- |
| `frontend/vite.player.config.ts` | builds the player as one classic IIFE and one stylesheet |
| `pkg/export/web.go` | inlines the page, the story, and every asset; writes one file |
| `pkg/export/web_test.go` | asserts the single-file shape and the data URIs |
| `pkg/export/script.go` | unchanged resolution; the reason for silence travels with the script |
| `pkg/scene/compile.go` | reports the first synthesis error beside the silent-beat count |
| `pkg/gui/export.go` | names the artefact `<game-id>-web.html` |
| `cmd/localrpg/export.go` | the same default path |
| `pkg/gui/export_player_e2e_test.go` | loads a bundle from `file://` with no permissive flags |

---

### Task 1: The player becomes a classic script

**Files:**
- Modify: `frontend/vite.player.config.ts`
- Test: `npm run build`

- [x] **Step 1: Build the player as an IIFE**

```ts
  build: {
    outDir: path.resolve(__dirname, '../pkg/gui/dist/player'),
    emptyOutDir: true,
    // A bundle inlines this file into a page opened from file://, where a module
    // script is refused by CORS and dynamic imports cannot resolve.
    cssCodeSplit: false,
    rollupOptions: {
      input: { player: path.resolve(__dirname, 'player.html') },
      output: { format: 'iife', inlineDynamicImports: true }
    }
  }
```

- [x] **Step 2: Confirm one script and one stylesheet**

Run: `cd frontend && npm run build && ls ../pkg/gui/dist/player/assets/`
Expected: exactly one `.js` and one `.css`.

---

### Task 2: The exporter writes one file

**Files:**
- Modify: `pkg/export/web.go`
- Test: `pkg/export/web_test.go`

**Interfaces:**
- Consumes: the player `fs.FS` (a page plus `assets/*.js` and `assets/*.css`).
- Produces: `Export` writing `outPath` as a file and returning it.

- [x] **Step 1: Write the failing tests**

```go
func TestWebExportWritesOneSelfContainedFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	dir := t.TempDir()

	if _, err := testExporter().Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	page, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("expected the bundle at %q: %v", out, err)
	}
	body := string(page)
	for _, want := range []string{
		"window.__LOCALRPG_STORY__",
		"console.log('player')",   // the built script, inlined
		"body{background:#000}",   // the built stylesheet, inlined
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q inlined in the bundle", want)
		}
	}
	for _, unwanted := range []string{`src="./assets/`, `href="./assets/`, `type="module"`, "http://", "https://"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the bundle still refers outside itself: %q", unwanted)
		}
	}
	for _, sidecar := range []string{"assets", "audio"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(out), sidecar)); err == nil {
			t.Errorf("a bundle must not leave a %s directory beside it", sidecar)
		}
	}
}

func TestWebExportInlinesEveryAsset(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	dir := t.TempDir()

	if _, err := testExporter().Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	payload := readWebPayload(t, out)
	if !strings.HasPrefix(payload.PlayerPortrait, "data:image/svg+xml;base64,") {
		t.Errorf("player portrait = %q, want a data URI", payload.PlayerPortrait)
	}
	if !strings.HasPrefix(payload.Scenes[0].Art, "data:image/svg+xml;base64,") {
		t.Errorf("scene art = %q, want a data URI", payload.Scenes[0].Art)
	}
	beats := payload.Scenes[0].Beats
	if !strings.HasPrefix(beats[2].Portrait, "data:image/svg+xml;base64,") {
		t.Errorf("speech portrait = %q, want a data URI", beats[2].Portrait)
	}
	if len(beats[2].Audio) != 1 || !strings.HasPrefix(beats[2].Audio[0], "data:audio/wav;base64,") {
		t.Errorf("audio = %#v, want a data URI per clip", beats[2].Audio)
	}
}
```

- [x] **Step 2: Run them to watch them fail**

Run: `go test ./pkg/export/ -run 'SelfContained|InlinesEveryAsset' -v`
Expected: FAIL, the export writes a directory and leaves sidecars.

- [x] **Step 3: Implement the inlining**

```go
// inlinePlayer replaces the built page's own asset references with their contents, so
// the page needs nothing beside it. The build's hashed names are read from the page
// rather than assumed.
func (w *WebExporter) inlinePlayer(page string) (string, error)

// dataURI encodes a file for the page to carry, mapping its extension to a MIME type.
func dataURI(path string) (string, error)
```

- [x] **Step 4: Run the export tests**

Run: `go test ./pkg/export/ -count=1`
Expected: PASS.

---

### Task 3: Name the artefact, and stop creating a directory

**Files:**
- Modify: `pkg/gui/export.go`, `cmd/localrpg/export.go`
- Test: `pkg/gui/export_test.go`

- [x] **Step 1: Point the callers at a file**

```go
// exportArtifactPath names the artifact inside a chosen directory: a single
// self-contained page for the web player, a video file otherwise.
func exportArtifactPath(outDir, gameID, format string) string {
	if format == "web" {
		return filepath.Join(outDir, gameID+"-web.html")
	}
	return filepath.Join(outDir, gameID+".mp4")
}
```

`cmd/localrpg/export.go`'s default target becomes `dist/<game-id>-web.html`.

- [x] **Step 2: Verify**

Run: `go build ./... && go test ./pkg/gui/ -run Export -count=1`
Expected: builds; the export tests pass with the new path.

---

### Task 4: A silent bundle says why

**Files:**
- Modify: `pkg/scene/compile.go`
- Test: `pkg/scene/compile_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestCompileReportsWhyBeatsAreSilent(t *testing.T) {
	compiler := NewCompiler(twoLocationSource())
	compiler.SetSpeechResolver(&failingSpeech{err: errors.New("provider returned 429")})

	var warnings []string
	if _, err := compiler.Compile(context.Background(), "campaign-01", Options{
		Audio:      true,
		OnProgress: func(format string, args ...interface{}) { warnings = append(warnings, fmt.Sprintf(format, args...)) },
	}); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one", warnings)
	}
	for _, want := range []string{"no audio clip", "provider returned 429"} {
		if !strings.Contains(warnings[0], want) {
			t.Errorf("warning %q does not mention %q", warnings[0], want)
		}
	}
}
```

- [x] **Step 2: Run it to watch it fail**

Run: `go test ./pkg/scene/ -run WhyBeatsAreSilent -v`
Expected: FAIL, the warning has a count and no reason.

- [x] **Step 3: Carry the reason**

`resolveAudio` keeps the first error it saw; `Compile` appends it to the message it already
reports.

- [x] **Step 4: Run the scene tests**

Run: `go test ./pkg/scene/ -count=1`
Expected: PASS.

---

### Task 5: The browser test loads a bundle like a user

**Files:**
- Modify: `pkg/gui/export_player_e2e_test.go`

- [x] **Step 1: Drop the permissive flag and assert self-containment**

The test already navigates to the file over `file://`; it must not pass
`--allow-file-access-from-files`, and it should assert that the page made no failed
requests: the console capture it now has reports them.

- [x] **Step 2: Run it**

Run: `CHROME_EXEC=/opt/google/chrome/chrome go test ./pkg/gui/ -run ExportedBundle -count=1 -v`
Expected: PASS with no CORS entries in the captured console.

---

### Task 6: Full verification

- [x] **Step 1: Gates**

Run: `mise run build:frontend && mise run test && mise run lint`
Expected: all green.

- [x] **Step 2: A real bundle, opened the way a user opens it**

Run: `localrpg export web <game-id> --dir <root> --out /tmp/bundle.html`, then check that
the file is the only artefact, that it contains `data:audio/`, and that no `assets/` or
`audio/` directory exists.

- [x] **Step 3: Commit**

```bash
git add frontend pkg cmd docs
git commit -m "fix(export): make a web bundle one file that opens from disk"
```

---


---

## Implementation Notes (added during execution)

**Follow-up (2026-09-30):** the player no longer starts itself, and a beat is held for
`max(compiled pace, reading time + buffer)` so a clip that cannot play (a browser wanting a
gesture) can never shorten a line. The payload carries each beat's reading estimate for
this. With no auto-start the play button is also the gesture that enables audio, so the
player no longer shows a separate enable-audio control.

The page is compressed as one bundle, which the plan's tasks implied but did not spell out.
The measured effect, on a realistic mix:

| Part | Raw | Gzipped |
| --- | --- | --- |
| Story (60 beats of prose, 20 scenes, 60 clips) | 1655 KB | 1104 KB |
| Player script | 234 KB | 74 KB |
| Player stylesheet | 101 KB | 15 KB |
| Whole page | 1989 KB | 1591 KB (20% smaller) |

A silent bundle, where the player's own build dominates, is roughly 70% smaller. The clips
are Opus and already compressed, so gzip cannot help them, and base64 costs a third of the
compressed size; re-encoding the clips is the remaining lever (spec §8).

Two other things the implementation added: the export reports the written size (the GUI as
a progress message, the CLI on its line), and the browser test now decodes the bundle it
loaded so it can assert the story the browser was given, not only what it rendered.

## Self-Review

**1. Spec coverage**

| Spec requirement | Task |
| --- | --- |
| §3.1 classic IIFE player | 1 |
| §3.2 inlined page, one file | 2 |
| §3.3 assets as data URIs | 2 |
| §3.4 silent bundles say why | 4 |
| §4 artefact naming | 3 |
| §6 tests, including the no-flags browser load | 2, 5, 6 |

**2. Placeholder scan:** the two helpers in Task 2 are named with their contract; their
bodies are small and specified by the tests above them.

**3. Type consistency:** `Export(ctx, script, outPath)` writes `outPath` (Task 2) which
Task 3's `exportArtifactPath` names; the payload's asset fields keep their names and change
only their encoding (Task 2), which Task 5's browser test exercises end to end.
