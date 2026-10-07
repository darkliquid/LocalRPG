# Procedural Art Variety Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the built-in procedural image generator real variety: genre and mood palettes, eight structures, time of day and weather, all deterministic and driven by structured hints.

**Architecture:** A new optional `SceneHintProvider` capability carries a `SceneRequest` (genre, mood, time, weather, seed). The procedural provider implements it and composes an SVG from a palette table, a structure table, and modifier layers. The pipeline asserts the interface and falls back to the existing prompt path.

**Tech Stack:** Go standard library (`math/rand` seeded per request, `encoding/xml` or string building for SVG).

**Spec:** `docs/superpowers/specs/2026-10-05-procedural-art-variety-design.md`

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- No `math/rand` global source and no `time.Now()` in the generator; one seeded RNG per request.
- The output stays an 800×600 SVG; the art cache key must not change.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The scene request and capability interface

**Files:**
- Create: `pkg/media/scene_request.go`
- Test: `pkg/media/scene_request_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `SceneRequest`, `SceneHintProvider`.

- [ ] **Step 1: Write the failing test**

```go
package media

import (
	"context"
	"testing"
)

type fakeHint struct{ got SceneRequest }

func (f *fakeHint) Generate(context.Context, string) ([]byte, error) { return nil, nil }
func (f *fakeHint) GenerateScene(_ context.Context, req SceneRequest) ([]byte, error) {
	f.got = req
	return []byte("svg"), nil
}

func TestSceneHintProviderIsDiscoverable(t *testing.T) {
	var c ImageClient = &fakeHint{}
	hp, ok := c.(SceneHintProvider)
	if !ok {
		t.Fatal("expected the fake to implement SceneHintProvider")
	}
	if _, err := hp.GenerateScene(context.Background(), SceneRequest{Genre: "fantasy"}); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/ -run TestSceneHintProvider -v`
Expected: FAIL, `undefined: SceneHintProvider`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/media/scene_request.go`:

```go
package media

import "context"

// SceneRequest carries structured hints for a generated scene image.
type SceneRequest struct {
	Prompt    string
	Genre     string
	Mood      string
	TimeOfDay string
	Weather   string
	Seed      string
}

// SceneHintProvider is implemented by image providers that can use structured
// hints rather than only the prose prompt.
type SceneHintProvider interface {
	GenerateScene(ctx context.Context, req SceneRequest) ([]byte, error)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ -run TestSceneHintProvider -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/scene_request.go pkg/media/scene_request_test.go
git commit -m "feat(media): add a structured scene-image hint interface"
```

---

### Task 2: The palette table

**Files:**
- Create: `pkg/media/procedural_palettes.go`
- Test: `pkg/media/procedural_palettes_test.go`

**Interfaces:**
- Consumes: `SceneRequest`.
- Produces: `type palette struct { … }`, `func paletteFor(genre, mood, timeOfDay string, rng *rand.Rand) palette`.

- [ ] **Step 1: Write the failing test**

```go
package media

import (
	"math/rand"
	"testing"
)

func TestPaletteForIsDeterministicAndVaried(t *testing.T) {
	p1 := paletteFor("fantasy", "grim", "night", rand.New(rand.NewSource(1)))
	p2 := paletteFor("fantasy", "grim", "night", rand.New(rand.NewSource(1)))
	if p1 != p2 {
		t.Fatal("same inputs produced different palettes")
	}
	day := paletteFor("fantasy", "grim", "day", rand.New(rand.NewSource(1)))
	if p1 == day {
		t.Fatal("time of day did not change the palette")
	}
	if paletteFor("unknown", "", "", rand.New(rand.NewSource(1))) == (palette{}) {
		t.Fatal("an unknown genre should still yield a palette")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/ -run TestPaletteFor -v`
Expected: FAIL, `undefined: paletteFor`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/media/procedural_palettes.go` with a comparable `palette` struct (all fields are
comparable strings, so `==` works):

```go
package media

import "math/rand"

type palette struct {
	SkyTop, SkyBottom string
	Ground            string
	Ridge             string
	Fog               string
	Celestial         string
	AccentA, AccentB  string
}

// basePalettes maps a genre to a base palette. A mood desaturates or warms it,
// and time of day shifts the sky. Unknown genres fall back to fantasy.
var basePalettes = map[string]palette{
	"fantasy":   {SkyTop: "#1a2740", SkyBottom: "#3d5a80", Ground: "#20262e", Ridge: "#2b3440", Fog: "#9fb3c8", Celestial: "#ffe8a3", AccentA: "#7fb069", AccentB: "#8d99ae"},
	"cyberpunk": {SkyTop: "#0b0f1a", SkyBottom: "#2b1b4d", Ground: "#12121c", Ridge: "#1c1c2b", Fog: "#7a5cff", Celestial: "#ff4fd8", AccentA: "#00e5ff", AccentB: "#ff2e88"},
	"horror":    {SkyTop: "#0a0a0a", SkyBottom: "#1b1b1b", Ground: "#101010", Ridge: "#181818", Fog: "#4a4a4a", Celestial: "#c9c9c9", AccentA: "#6b1f1f", AccentB: "#3a3a3a"},
	"scifi":     {SkyTop: "#050914", SkyBottom: "#123055", Ground: "#0d1520", Ridge: "#16283c", Fog: "#7fd4ff", Celestial: "#d6f0ff", AccentA: "#39a0ed", AccentB: "#9fd8ff"},
	"wildwest":  {SkyTop: "#3a2a1a", SkyBottom: "#c98a4b", Ground: "#2a1f14", Ridge: "#4a3524", Fog: "#d9b382", Celestial: "#ffd9a0", AccentA: "#a34a28", AccentB: "#7a5a3a"},
}

// paletteFor resolves a palette from the hints, defaulting gracefully.
func paletteFor(genre, mood, timeOfDay string, rng *rand.Rand) palette {
	p, ok := basePalettes[genre]
	if !ok {
		p = basePalettes["fantasy"]
	}
	p = applyMood(p, mood)
	p = applyTimeOfDay(p, timeOfDay)
	return p
}
```

Implement `applyMood` (darken/desaturate for `grim`/`tense`, warm for `serene`) and `applyTimeOfDay`
(shift `SkyTop`/`SkyBottom`, dim `Celestial` at night) as small, pure colour-string transforms. Add
at least three more genres so the table covers the presets.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ -run TestPaletteFor -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/procedural_palettes.go pkg/media/procedural_palettes_test.go
git commit -m "feat(media): add a genre and mood palette table"
```

---

### Task 3: The structure table

**Files:**
- Create: `pkg/media/procedural_structures.go`
- Test: `pkg/media/procedural_structures_test.go`

**Interfaces:**
- Consumes: `palette`.
- Produces: `type structure func(p palette, rng *rand.Rand, w, h int) string`, `func structureFor(tags []string, genre string, rng *rand.Rand) structure`.

- [ ] **Step 1: Write the failing test**

```go
package media

import (
	"math/rand"
	"strings"
	"testing"
)

func TestStructureForSelectsByTagThenGenre(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	if s := structureFor([]string{"forest"}, "fantasy", rng); s == nil {
		t.Fatal("no structure for a forest tag")
	}
	if !strings.Contains(structureFor([]string{"city"}, "fantasy", rng)(paletteFor("fantasy", "", "day", rng), rng, 800, 600), "svg") &&
		!strings.Contains(structureFor([]string{"city"}, "fantasy", rng)(paletteFor("fantasy", "", "day", rng), rng, 800, 600), "<") {
		t.Fatal("a structure should emit SVG fragments")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/ -run TestStructureFor -v`
Expected: FAIL, `undefined: structureFor`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/media/procedural_structures.go` with eight structure functions (`ridge`, `forest`,
`city`, `coast`, `interior`, `dungeon`, `ruins`, `sky`), each returning an SVG fragment drawn
relative to the horizon, and:

```go
// structureTags maps a location tag to a structure name.
var structureTags = map[string]string{
	"forest": "forest", "wood": "forest", "jungle": "forest",
	"city": "city", "town": "city", "street": "city",
	"coast": "coast", "shore": "coast", "sea": "coast", "harbour": "coast",
	"interior": "interior", "room": "interior", "hall": "interior",
	"dungeon": "dungeon", "crypt": "dungeon", "catacomb": "dungeon",
	"ruin": "ruins", "ruins": "ruins",
	"sky": "sky", "mountain": "sky", "ridge": "ridge",
}

// structureFor picks a structure from tags, then the genre default, then a
// seeded choice.
func structureFor(tags []string, genre string, rng *rand.Rand) structure {
	for _, tag := range tags {
		if name, ok := structureTags[strings.ToLower(tag)]; ok {
			return structures[name]
		}
	}
	if name, ok := genreStructures[genre]; ok {
		return structures[name]
	}
	return structures[structureOrder[rng.Intn(len(structureOrder))]]
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ -run TestStructureFor -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/procedural_structures.go pkg/media/procedural_structures_test.go
git commit -m "feat(media): add eight procedural scene structures"
```

---

### Task 4: The composer and `GenerateScene`

**Files:**
- Modify: `pkg/media/procedural_art.go`
- Create: `pkg/media/procedural_scene.go`
- Test: `pkg/media/procedural_scene_test.go`

**Interfaces:**
- Consumes: `paletteFor`, `structureFor`, `SceneRequest`.
- Produces: `func GenerateSceneSVG(req SceneRequest) []byte`; the procedural provider's `GenerateScene` method.

- [ ] **Step 1: Write the failing test**

```go
package media

import (
	"bytes"
	"context"
	"testing"
)

func TestGenerateSceneIsDeterministicAndSeeded(t *testing.T) {
	req := SceneRequest{Genre: "fantasy", Mood: "grim", TimeOfDay: "night", Weather: "rain", Seed: "loc-1"}
	a := GenerateSceneSVG(req)
	b := GenerateSceneSVG(req)
	if !bytes.Equal(a, b) {
		t.Fatal("same request produced different SVG")
	}
	req.Seed = "loc-2"
	if bytes.Equal(a, GenerateSceneSVG(req)) {
		t.Fatal("a changed seed did not change the SVG")
	}
	if !bytes.Contains(a, []byte("<svg")) {
		t.Fatal("output is not SVG")
	}
}

func TestProceduralProviderImplementsSceneHint(t *testing.T) {
	var c ImageClient = NewProceduralImageProvider()
	hp, ok := c.(SceneHintProvider)
	if !ok {
		t.Fatal("procedural provider must implement SceneHintProvider")
	}
	if _, err := hp.GenerateScene(context.Background(), SceneRequest{Genre: "horror", Seed: "x"}); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/ -run 'TestGenerateScene|TestProceduralProviderImplementsSceneHint' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/media/procedural_scene.go`:

```go
package media

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"strings"
)

// GenerateSceneSVG composes a scene from structured hints. It is deterministic:
// the seed and hints fully determine the bytes.
func GenerateSceneSVG(req SceneRequest) []byte {
	rng := rand.New(rand.NewSource(seedFor(req)))
	p := paletteFor(req.Genre, req.Mood, req.TimeOfDay, rng)
	st := structureFor(strings.Fields(req.Prompt), req.Genre, rng)
	const w, h = 800, 600
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, w, h, w, h)
	writeSky(&b, p, w, h)
	b.WriteString(st(p, rng, w, h))
	writeWeather(&b, req.Weather, p, rng, w, h)
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

func seedFor(req SceneRequest) int64 {
	h := fnv.New64a()
	h.Write([]byte(req.Seed + "|" + req.Genre + "|" + req.Mood + "|" + req.TimeOfDay + "|" + req.Weather))
	return int64(h.Sum64())
}

// GenerateScene implements SceneHintProvider.
func (c *proceduralImageProvider) GenerateScene(_ context.Context, req SceneRequest) ([]byte, error) {
	return GenerateSceneSVG(req), nil
}
```

Refactor `procedural_art.go`: keep `GenerateImage(prompt)` as a thin wrapper that derives a
`SceneRequest` from the prose (genre from keywords, everything else defaulted) and calls
`GenerateSceneSVG`, so existing callers are unchanged. Move `writeSky`, `writeWeather`, and the
existing ridge/fog drawing into `procedural_scene.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ -run 'TestGenerateScene|TestProcedural' -v`
Expected: PASS, and the existing `GenerateImage` tests still pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/procedural_art.go pkg/media/procedural_scene.go pkg/media/procedural_scene_test.go
git commit -m "feat(media): compose procedural scenes from hints"
```

---

### Task 5: Resolve hints in the pipeline

**Files:**
- Modify: `pkg/media/image.go` (the `ImagePipeline` scene path)
- Test: `pkg/media/image_test.go` (append)

**Interfaces:**
- Consumes: `SceneHintProvider`, `SceneRequest`.
- Produces: `func (p *ImagePipeline) sceneRequest(location *entity.Entity, appearanceHash string) SceneRequest`.

- [ ] **Step 1: Write the failing test**

```go
func TestSceneRequestDerivesHints(t *testing.T) {
	loc := testLocation("forest", "grim", map[string]interface{}{"weather": "rain"})
	req := sceneRequestForTest(loc, "hash1")
	if req.Mood != "grim" || req.Weather != "rain" || req.Seed == "" {
		t.Fatalf("hints = %+v", req)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/ -run TestSceneRequestDerives -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add a resolver that fills `Genre` from the world style, `Mood` from location tags/state, `TimeOfDay`
and `Weather` from location state (keys `time_of_day` and `weather`), and `Seed` from the location
id plus the appearance hash. When the provider implements `SceneHintProvider`, call `GenerateScene`;
otherwise call `Generate(prompt)`. Leave the cache key computation untouched.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/image.go pkg/media/image_test.go
git commit -m "feat(media): feed scene hints to hint-aware image providers"
```

---

### Task 6: Golden matrix and verification

**Files:**
- Create: `pkg/media/procedural_golden_test.go`

**Interfaces:**
- Consumes: `GenerateSceneSVG`.
- Produces: no production symbols.

- [ ] **Step 1: Write the golden test**

```go
func TestProceduralGoldenMatrix(t *testing.T) {
	genres := []string{"fantasy", "cyberpunk", "horror", "scifi", "wildwest", "unknown"}
	times := []string{"dawn", "day", "dusk", "night"}
	weathers := []string{"clear", "rain", "fog", "snow"}
	for _, g := range genres {
		for _, tod := range times {
			for _, wx := range weathers {
				svg := GenerateSceneSVG(SceneRequest{Genre: g, TimeOfDay: tod, Weather: wx, Seed: "golden"})
				if len(svg) == 0 || !bytes.Contains(svg, []byte("<svg")) {
					t.Fatalf("%s/%s/%s produced no SVG", g, tod, wx)
				}
			}
		}
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./pkg/media/ -run TestProceduralGoldenMatrix -v`
Expected: PASS.

- [ ] **Step 3: Confirm the rasteriser still consumes the art**

Run: `go test ./pkg/scene/ -v`
Expected: PASS.

- [ ] **Step 4: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/procedural_golden_test.go
git commit -m "test: cover the procedural scene matrix"
```
