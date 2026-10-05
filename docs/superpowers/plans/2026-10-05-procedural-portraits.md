# Procedural Portraits Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A procedural portrait that reflects a character's species, archetype, palette, and expression, deterministically.

**Architecture:** A `PortraitRequest` carries tags and state; species and archetype tables choose a silhouette and accessories; a palette derives from both; an expression derives from state; one seeded RNG keeps it deterministic.

**Tech Stack:** Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-procedural-portraits-design.md`

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- The same request yields byte-identical SVG; no clock, no global RNG.
- The old `GenerateProceduralBustSVG` wrapper is unchanged for its inputs.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The request and the wrapper

**Files:**
- Create: `pkg/media/procedural_portrait.go`
- Test: `pkg/media/procedural_portrait_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `PortraitRequest`, `func GenerateProceduralPortrait(req PortraitRequest) []byte`.

- [ ] **Step 1: Write the failing tests**

```go
func TestPortraitIsDeterministic(t *testing.T) {
	req := PortraitRequest{ID: "garrick", Name: "Garrick", Tags: []string{"human", "warrior"}}
	if !bytes.Equal(GenerateProceduralPortrait(req), GenerateProceduralPortrait(req)) {
		t.Fatal("the same request produced different portraits")
	}
}
func TestPortraitIsSVG(t *testing.T) {
	got := GenerateProceduralPortrait(PortraitRequest{ID: "x"})
	if !bytes.Contains(got, []byte("<svg")) {
		t.Fatalf("not svg: %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/media/ -run TestPortrait -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the request and a generator that draws the existing bust shape with the new derivation hooks
(stubbed to the current behaviour at first), plus the wrapper `GenerateProceduralBustSVG` delegating
to it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ -run TestPortrait -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/procedural_portrait.go pkg/media/procedural_portrait_test.go
git commit -m "feat(media): add the structured portrait request"
```

---

### Task 2: Species

**Files:**
- Create: `pkg/media/portrait_species.go`
- Test: `pkg/media/portrait_species_test.go`

**Interfaces:**
- Consumes: `PortraitRequest.Tags`.
- Produces: `type species struct { ID string; EarShape, Brow, Jaw string; Skin [2]string }`, `func speciesFor(tags []string, rng *rand.Rand) species`.

- [ ] **Step 1: Write the failing tests**

```go
func TestSpeciesFromTags(t *testing.T) {
	if speciesFor([]string{"orc"}, rand.New(rand.NewSource(1))).ID != "orc" {
		t.Fatal("an orc tag should select the orc species")
	}
	if speciesFor(nil, rand.New(rand.NewSource(1))).ID != "human" {
		t.Fatal("no tag should default to human")
	}
}
func TestSpeciesChangesTheSilhouette(t *testing.T) {
	orc := speciesFor([]string{"orc"}, rand.New(rand.NewSource(1)))
	elf := speciesFor([]string{"elf"}, rand.New(rand.NewSource(1)))
	if orc.EarShape == elf.EarShape && orc.Jaw == elf.Jaw {
		t.Fatal("species should differ in silhouette")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/media/ -run TestSpecies -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the species table (human, elf, dwarf, orc, halfling, beastfolk, undead, construct) with ear shape,
brow, jaw, and a skin hue range; match tags case-insensitively; default to human.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ -run TestSpecies -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/portrait_species.go pkg/media/portrait_species_test.go
git commit -m "feat(media): derive a portrait's species from tags"
```

---

### Task 3: Archetype, hair, and accessory

**Files:**
- Create: `pkg/media/portrait_archetype.go`
- Test: `pkg/media/portrait_archetype_test.go`

**Interfaces:**
- Consumes: `PortraitRequest.Tags`.
- Produces: `type archetype struct { ID, Hair, Collar, Accessory string; Garment [2]string }`, `func archetypeFor(tags []string, rng *rand.Rand) archetype`.

- [ ] **Step 1: Write the failing tests**

```go
func TestArchetypeFromTags(t *testing.T) {
	if archetypeFor([]string{"warrior"}, rand.New(rand.NewSource(1))).ID != "warrior" {
		t.Fatal("a warrior tag should select the warrior archetype")
	}
}
func TestArchetypeAccessoryDiffers(t *testing.T) {
	w := archetypeFor([]string{"warrior"}, rand.New(rand.NewSource(1)))
	m := archetypeFor([]string{"mage"}, rand.New(rand.NewSource(1)))
	if w.Accessory == m.Accessory {
		t.Fatal("archetypes should differ in accessory")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/media/ -run TestArchetype -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the archetype table (warrior, mage, rogue, scholar, priest, ranger, noble, labourer) with hair,
collar, an accessory, and a garment hue range; default to a seeded archetype when no tag matches.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ -run TestArchetype -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/portrait_archetype.go pkg/media/portrait_archetype_test.go
git commit -m "feat(media): derive a portrait's archetype from tags"
```

---

### Task 4: The palette and the drawing

**Files:**
- Modify: `pkg/media/procedural_portrait.go`
- Test: `pkg/media/procedural_portrait_test.go` (append)

**Interfaces:**
- Consumes: species and archetype.
- Produces: the composed SVG.

- [ ] **Step 1: Write the failing tests**

```go
func TestPortraitSpeciesAndArchetypeShow(t *testing.T) {
	orc := GenerateProceduralPortrait(PortraitRequest{ID: "a", Tags: []string{"orc", "warrior"}})
	elf := GenerateProceduralPortrait(PortraitRequest{ID: "a", Tags: []string{"elf", "mage"}})
	if bytes.Equal(orc, elf) {
		t.Fatal("species and archetype should change the portrait")
	}
}
func TestPortraitWrapperUnchanged(t *testing.T) {
	// GenerateProceduralBustSVG for the same inputs matches the previous output.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/media/ -run TestPortraitSpecies -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Compose the palette from the species skin range and the archetype garment range with the seeded RNG,
and draw the silhouette with the species' ear/brow/jaw, the archetype's hair and collar, and its
accessory.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ -run TestPortrait -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/procedural_portrait.go pkg/media/procedural_portrait_test.go
git commit -m "feat(media): draw a species and archetype portrait"
```

---

### Task 5: Expression, the GUI, and the export

**Files:**
- Modify: `pkg/media/procedural_portrait.go` (expression)
- Modify: `pkg/gui/service.go` (`GetCharacterPortrait`)
- Test: `pkg/media/procedural_portrait_test.go` (append), `pkg/gui/portrait_test.go` (append)

**Interfaces:**
- Consumes: `PortraitRequest.State`, the system's `HealthSpec`.
- Produces: an expression derived from state; the GUI passing tags and state.

- [ ] **Step 1: Write the failing tests**

```go
func TestExpressionFromLowHealth(t *testing.T) {
	healthy := GenerateProceduralPortrait(PortraitRequest{ID: "a", State: map[string]interface{}{"health": 10}})
	hurt := GenerateProceduralPortrait(PortraitRequest{ID: "a", State: map[string]interface{}{"health": 1}})
	if bytes.Equal(healthy, hurt) {
		t.Fatal("a low health state should change the expression")
	}
}
func TestGUIUsesTagsAndState(t *testing.T) { /* a portrait-less orc gets a tag-aware portrait */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/media/ -run TestExpression -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the state→expression mapping (low health, `mood`/`expression` state, else neutral) and have
`GetCharacterPortrait` pass the entity's tags and state.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ -run TestExpression -v` and `go test ./pkg/gui/ -run TestGUIUsesTagsAndState -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media pkg/gui
git commit -m "feat: make the procedural portrait reflect state"
```

---

### Task 6: Verification

- [ ] **Step 1: Golden matrix**

Add a test rendering (species × archetype) without panic and asserting distinct hashes.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Species and archetype derive from tags; the palette and accessory follow.
- Expression derives from state.
- Deterministic; the old wrapper is unchanged.
- The GUI and the export use the same function.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: cover the procedural portrait matrix"
```
