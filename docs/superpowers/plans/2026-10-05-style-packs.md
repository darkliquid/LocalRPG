# Style Packs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A user-supplied style pack that overrides the procedural palettes, structures, portraits, and genre chrome, merging over the built-in look.

**Architecture:** `pkg/media.StylePack` with a loader, a merge over the built-in tables, and validation that degrades to built-in; a `styles.pack` config selects one; the generators take the active pack; the art cache key includes the pack id.

**Tech Stack:** Go standard library; `gopkg.in/yaml.v3`; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-style-packs-design.md`
**Depends on:** PH-1, PH-2.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- No pack yields the built-in output byte-for-byte.
- An invalid pack is reported and ignored, never fatal.
- The art cache key includes the pack id.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The pack type, loader, and merge

**Files:**
- Create: `pkg/media/style_pack.go`
- Test: `pkg/media/style_pack_test.go`

**Interfaces:**
- Consumes: the PH-1 palette, the PH-2 species/archetype, the PH-4 genre palette.
- Produces: `StylePack`, `func LoadStylePack(path string) (StylePack, error)`, `func (p StylePack) Merge() Tables`.

- [ ] **Step 1: Write the failing tests**

```go
func TestPackMergesOverBuiltin(t *testing.T) {
	p := StylePack{Genres: map[string]GenrePalette{"fantasy": {From: "#000000"}}}
	tables := p.Merge()
	if tables.Genres["fantasy"].From != "#000000" {
		t.Fatal("the pack should override the genre")
	}
	if tables.Genres["horror"].From == "" {
		t.Fatal("an absent genre should inherit the built-in")
	}
}
func TestLoadStylePack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.yaml")
	os.WriteFile(path, []byte("id: p\ngenres:\n  fantasy:\n    from: \"#111111\"\n"), 0o644)
	p, err := LoadStylePack(path)
	if err != nil || p.ID != "p" {
		t.Fatalf("pack %+v err %v", p, err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/media/ -run 'TestPackMerges|TestLoadStylePack' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the type, the YAML loader, and `Merge` that starts from the built-in tables and overlays the
pack's entries by key.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ -run 'TestPackMerges|TestLoadStylePack' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/style_pack.go pkg/media/style_pack_test.go
git commit -m "feat(media): add the style pack loader and merge"
```

---

### Task 2: Validation

**Files:**
- Modify: `pkg/media/style_pack.go`
- Test: `pkg/media/style_pack_test.go` (append)

**Interfaces:**
- Consumes: Task 1.
- Produces: `func (p StylePack) Validate() []string`.

- [ ] **Step 1: Write the failing test**

```go
func TestValidateStylePack(t *testing.T) {
	bad := StylePack{Genres: map[string]GenrePalette{"fantasy": {From: "not-a-colour"}}}
	if len(bad.Validate()) == 0 {
		t.Fatal("a bad colour should be reported")
	}
	good := StylePack{Genres: map[string]GenrePalette{"fantasy": {From: "#112233"}}}
	if len(good.Validate()) != 0 {
		t.Fatalf("a good pack reported %v", good.Validate())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/ -run TestValidateStylePack -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Check colours (`#rrggbb`), scene palette keys, structure names, and species/archetype shapes; return
human-readable problems.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ -run TestValidateStylePack -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/style_pack.go pkg/media/style_pack_test.go
git commit -m "feat(media): validate a style pack"
```

---

### Task 3: The config and the active pack

**Files:**
- Modify: `pkg/config/types.go`, `pkg/config/manager.go`
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/style_pack_test.go`

**Interfaces:**
- Consumes: `LoadStylePack`, `Validate`.
- Produces: `Config.Styles.Pack`, an accessor, and a load-once active pack in the GUI.

- [ ] **Step 1: Write the failing tests**

```go
func TestStylePackConfig(t *testing.T) { /* styles.pack round-trips */ }
func TestInvalidPackWarnsAndIgnores(t *testing.T) {
	// An invalid pack yields a warning and the built-in look.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/config/ -run TestStylePackConfig -v` and `go test ./pkg/gui/ -run TestInvalidPack -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `Styles{ Pack string }` to the config and load the selected pack under the config directory,
caching it on the config revision and warning on an invalid pack.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/config/ -run TestStylePackConfig -v` and `go test ./pkg/gui/ -run TestInvalidPack -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config pkg/gui
git commit -m "feat: select and load a style pack"
```

---

### Task 4: The generators take the pack

**Files:**
- Modify: `pkg/media/procedural_scene.go` (PH-1), `pkg/media/procedural_portrait.go` (PH-2)
- Test: `pkg/media/style_pack_test.go` (append)

**Interfaces:**
- Consumes: the merged tables (Task 1).
- Produces: the generators reading the active pack's tables.

- [ ] **Step 1: Write the failing test**

```go
func TestPackChangesTheScene(t *testing.T) {
	a := GenerateSceneSVG(SceneRequest{Genre: "fantasy", Seed: "x"}) // built-in
	b := generateSceneWithPack(SceneRequest{Genre: "fantasy", Seed: "x"}, packOverridingFantasy())
	if bytes.Equal(a, b) {
		t.Fatal("a pack override should change the scene")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/ -run TestPackChangesTheScene -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Thread the merged tables into `paletteFor`, `speciesFor`, `archetypeFor`, and the composer; keep the
built-in path when no pack is active.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ -run TestPackChangesTheScene -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media
git commit -m "feat(media): render with the active style pack"
```

---

### Task 5: The cache key, the picker, and the CLI

**Files:**
- Modify: `pkg/media/image.go` (the art cache key)
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`
- Create: `cmd/localrpg/styles.go`
- Modify: `frontend/src/components/SettingsStudio.tsx`
- Test: `pkg/media/style_pack_test.go` (append), `pkg/gui/style_pack_test.go` (append)

**Interfaces:**
- Consumes: the active pack id.
- Produces: the pack id in the cache key; a picker and CLI verbs.

- [ ] **Step 1: Write the failing tests**

```go
func TestCacheKeyIncludesPack(t *testing.T) { /* a different pack id changes the art cache key */ }
func TestStylesListEndpoint(t *testing.T) { /* the packs under the config dir are listed */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/media/ -run TestCacheKeyIncludesPack -v` and `go test ./pkg/gui/ -run TestStylesList -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the pack id to `ComputeArtCacheKey`; add a `GET /api/styles` listing the packs with validation
status; add `localrpg styles list|validate`; add a picker in the settings.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ -run TestCacheKeyIncludesPack -v` and `go test ./pkg/gui/ -run TestStylesList -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media pkg/gui cmd/localrpg frontend/src
git commit -m "feat: expose and apply style packs"
```

---

### Task 6: Verification

- [ ] **Step 1: Regression guard**

Add a test that no pack yields the built-in output byte-for-byte.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- A pack merges over the built-in tables by key.
- An invalid pack warns and is ignored.
- The pack changes the scene, portrait, and chrome.
- The cache key includes the pack id.
- No pack is byte-identical to today.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the no-pack output"
```
