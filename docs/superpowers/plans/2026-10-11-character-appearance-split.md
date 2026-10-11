# Character Appearance Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every note a baseline appearance and a current one, so gameplay writes only the current, a reset clears it, an undo restores it, and every prompt, cache key, and sheet shows the baseline with the current layered on it.

**Architecture:** `entity.CurrentAppearance` joins the existing `Appearance` field, and `entity.DescribeAppearance` is the one rule that composes them. The extractor's merge routes a new note's description into the baseline and a changed note's into the current, the timeline records what each turn rewrote so a rewind can put it back, and the prompt builders and art cache key read the composed text.

**Tech Stack:** Go 1.27 (`pkg/entity`, `pkg/harness`, `pkg/engine`, `pkg/media`, `pkg/config`, `pkg/gui`), React 19 + TypeScript strict, Vitest + React Testing Library.

**Spec:** `docs/superpowers/specs/2026-10-11-character-appearance-split-design.md`
**Issue:** [#147](https://github.com/darkliquid/LocalRPG/issues/147)

## Global Constraints

- The frontmatter a note serializes to stays byte-identical for a note with no current appearance.
- `appearance` keeps its name and meaning. Nothing is renamed and nothing is migrated.
- Only a field play owns is cleared by a reset or restored by a rewind. `appearance`, `voice`, `portrait`, `gender`, `age`, and the body are never touched by either.
- The location-art option defaults to the behaviour that exists today, so a campaign that sets nothing sees no change.
- Go tests use the standard library only (`testing`, `t.TempDir()`); no testify. Errors are wrapped with `fmt.Errorf("...: %w", err)`.
- `mise run lint` must stay clean, `mise run lint:docs` must pass, and `STRICT=1 mise run lint:prose` must report no errors.
- Changing `pkg/config` means regenerating the embedded reference: `go test ./pkg/gui -update-docs`, and the regenerated file is committed with the change.

## File Map

| File | Change |
| --- | --- |
| `pkg/entity/entity.go` | `CurrentAppearance` on the frontmatter and the entity; parse; serialize; the `ExtraMeta` skip list |
| `pkg/entity/appearance.go` | new: `DescribeAppearance` |
| `pkg/storage/store.go` | the metadata map and its rehydration carry the new field |
| `pkg/harness/extractor.go` | the merge routes the description; one sentence of prompt guidance |
| `pkg/engine/timeline.go` | capture the revert in `stageEntities`; apply it in the rewind pass |
| `pkg/engine/history.go` | `Turn.CurrentAppearances` and `EntityAppearance` |
| `pkg/engine/restart.go` | `clearRuntimeFields` clears the current appearance |
| `pkg/engine/scene_worker.go` | the cast composes both; `ScenePromptContext.CurrentAppearance` |
| `pkg/engine/orchestrator.go` | set the location's current appearance for the scene |
| `pkg/engine/portrait_worker.go` | the portrait prompt composes both |
| `pkg/media/image.go` | `AppearanceHash` and `BuildLocationPrompt` take the layer decision |
| `pkg/media/location_art.go` | `ArtStore` carries the decision |
| `pkg/config/types.go` | `LocationArtAppearance` and its accessor |
| `pkg/gui/service.go` | pass the option; the player DTO carries the current appearance |
| `pkg/gui/types.go` | the DTO fields |
| `pkg/gui/schema.go`, `pkg/gui/entitycatalog.go` | the field is described and offered |
| `pkg/gui/docs/10-codex-yaml.md`, `pkg/gui/docs/13-configuration-reference.md` | documented, the second regenerated |
| `frontend/src/types.ts` | the DTO and `ImageConfig` types |
| `frontend/src/components/CharacterSheetDrawer.tsx` | show the current appearance |
| `frontend/src/components/providers/ImageProviderEditor.tsx` | the new option |

---

### Task 1: The field and the composition rule

**Files:**
- Modify: `pkg/entity/entity.go`
- Create: `pkg/entity/appearance.go`, `pkg/entity/appearance_test.go`
- Modify: `pkg/storage/store.go`
- Modify: `pkg/entity/entity_test.go`

**Interfaces:**
- Produces: `entity.Entity.CurrentAppearance string`, `entity.EntityFrontmatter.CurrentAppearance string`, `entity.DescribeAppearance(ent *Entity, limit int) string` (a limit of zero or less means no truncation).

- [ ] **Step 1: Write the failing test** for the composition rule.

```go
// pkg/entity/appearance_test.go
package entity

import "testing"

func TestDescribeAppearance(t *testing.T) {
	cases := []struct {
		name   string
		ent    *Entity
		limit  int
		want   string
	}{
		{name: "neither", ent: &Entity{}, want: ""},
		{name: "baseline only", ent: &Entity{Appearance: "Silver hair."}, want: "Silver hair."},
		{name: "current only", ent: &Entity{CurrentAppearance: "Soaked."}, want: "Soaked."},
		{
			name: "layered",
			ent:  &Entity{Appearance: "Silver hair.", CurrentAppearance: "Soaked."},
			want: "Silver hair.; now Soaked.",
		},
		{
			name: "truncated",
			ent:  &Entity{Appearance: "Silver hair.", CurrentAppearance: "Soaked."},
			limit: 10,
			want: "Silver hai",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DescribeAppearance(tc.ent, tc.limit); got != tc.want {
				t.Errorf("DescribeAppearance = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDescribeAppearanceNilEntity(t *testing.T) {
	if got := DescribeAppearance(nil, 0); got != "" {
		t.Errorf("DescribeAppearance(nil) = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run TestDescribeAppearance ./pkg/entity/`
Expected: FAIL with `undefined: DescribeAppearance`.

- [ ] **Step 3: Implement the rule and the field**

`pkg/entity/appearance.go`:

```go
package entity

import "strings"

// appearanceSeparator introduces the current appearance inside a composed
// description. The baseline comes first so a note reads as itself, and the
// current comes last, where a model weights it as the qualifier.
const appearanceSeparator = "; now "

// DescribeAppearance composes a note's baseline appearance with the current one
// for its circumstances. It is the single rule for every consumer: an image
// prompt, an art cache key, and a sheet all show what this returns, so something
// authored and something play wrote can never be read apart. A limit of zero or
// less returns the whole text.
func DescribeAppearance(ent *Entity, limit int) string {
	if ent == nil {
		return ""
	}
	baseline := strings.TrimSpace(ent.Appearance)
	current := strings.TrimSpace(ent.CurrentAppearance)

	composed := baseline
	if current != "" {
		if baseline == "" {
			// Nothing authored a baseline, so there is no context to layer the
			// current one onto and it stands alone.
			composed = current
		} else {
			composed = baseline + appearanceSeparator + current
		}
	}
	if limit > 0 && len(composed) > limit {
		composed = strings.TrimSpace(composed[:limit])
	}
	return composed
}
```

In `pkg/entity/entity.go` add the field to `EntityFrontmatter` after `Appearance`, and to `Entity` in the same position:

```go
	Appearance      string                 `yaml:"appearance,omitempty" json:"appearance,omitempty"`
	// CurrentAppearance is how the note looks right now, layered on Appearance by
	// DescribeAppearance. Play writes it and describes the present rather than the
	// identity, so a reset clears it and a baseline is never rewritten.
	CurrentAppearance string               `yaml:"current_appearance,omitempty" json:"current_appearance,omitempty"`
```

Add `CurrentAppearance: fm.CurrentAppearance,` to the parse return value (next to `Appearance`), `CurrentAppearance: e.CurrentAppearance,` to `SerializeMarkdown`'s `EntityFrontmatter` literal, and `"current_appearance"` to the `ExtraMeta` skip list's `switch` case at `pkg/entity/entity.go:353`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/entity/`
Expected: PASS.

- [ ] **Step 5: Write the failing round-trip and metadata tests**

```go
// appended to pkg/entity/entity_test.go
func TestCurrentAppearanceRoundTripsWithoutBecomingExtraMeta(t *testing.T) {
	note := []byte("---\nid: kaelen\nname: Kaelen\ntype: character\n" +
		"appearance: Silver hair.\ncurrent_appearance: Soaked through.\n---\nA traveller.\n")

	ent, err := ParseMarkdownEntity(note)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ent.CurrentAppearance != "Soaked through." {
		t.Errorf("CurrentAppearance = %q, want the parsed value", ent.CurrentAppearance)
	}
	if _, ok := ent.ExtraMeta["current_appearance"]; ok {
		t.Error("current_appearance must not also land in ExtraMeta")
	}

	out, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if back, err := ParseMarkdownEntity(out); err != nil {
		t.Fatalf("reparse: %v", err)
	} else if back.CurrentAppearance != "Soaked through." {
		t.Errorf("round trip lost the current appearance: %q", back.CurrentAppearance)
	}
}

func TestSerializeKeepsANoteWithoutACurrentAppearanceUnchanged(t *testing.T) {
	note := []byte("---\nid: kaelen\nname: Kaelen\ntype: character\n" +
		"appearance: Silver hair.\n---\nA traveller.\n")
	out, err := (&Entity{ID: "kaelen", Name: "Kaelen", Type: "character", Appearance: "Silver hair.", Body: "A traveller.\n"}).SerializeMarkdown()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	_ = note
	if strings.Contains(string(out), "current_appearance") {
		t.Errorf("an unset field must not be emitted:\n%s", out)
	}
}
```

- [ ] **Step 6: Run the tests to verify they pass, then commit**

Run: `go test ./pkg/entity/ ./pkg/storage/`
Expected: PASS. Add `"current_appearance"` to the metadata map and its rehydration in `pkg/storage/store.go:54` and `:129` first (the map is what an entity read from the index is rebuilt from, so the field would be lost without it).

```bash
git add pkg/entity pkg/storage && git commit -m "feat(entity): carry a current appearance beside the baseline"
```

---

### Task 2: Work out which appearance a prompt or a cache key uses

**Files:**
- Modify: `pkg/engine/scene_worker.go`
- Modify: `pkg/engine/portrait_worker.go`
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/engine/scene_prompt_test.go`

**Interfaces:**
- Consumes: `entity.DescribeAppearance` (Task 1).
- Produces: `ScenePromptContext.CurrentAppearance string`.

- [ ] **Step 1: Write the failing test** for the cast and the scene moment.

```go
// appended to pkg/engine/scene_prompt_test.go
func TestSceneCastLayersTheCurrentAppearance(t *testing.T) {
	f := newResetFixture(t)

	// Write circumstances to the protagonist's note the way a turn would, so the
	// cast has something to layer on the authored description.
	notePath := filepath.Join(f.gameDir, "entities", "sean.md")
	note, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	player, err := entity.ParseMarkdownEntity(note)
	if err != nil {
		t.Fatal(err)
	}
	player.CurrentAppearance = "Soaked through."
	out, err := player.SerializeMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, out, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.NewSyncer(f.store).Sync(filepath.Join(f.gameDir, "entities")); err != nil {
		t.Fatal(err)
	}

	turn := &Turn{
		Number:   1,
		Location: OpeningSceneEntityID,
		Entities: []entity.Mention{{ID: "sean", Kind: entity.MentionPlayer}},
	}
	cast := SceneCast(f.store, turn)
	if len(cast) == 0 {
		t.Fatal("expected the player in the cast")
	}
	if !strings.Contains(cast[0].Appearance, "Weathered.") {
		t.Errorf("cast appearance = %q, want the baseline", cast[0].Appearance)
	}
	if !strings.Contains(cast[0].Appearance, "Soaked through.") {
		t.Errorf("cast appearance = %q, want the current appearance layered on", cast[0].Appearance)
	}
}

func TestScenePromptCarriesThePlaceAsItIsNow(t *testing.T) {
	ctx := ScenePromptContext{
		Location:          "The Drowned Hall",
		Appearance:        "A vaulted stone hall.",
		CurrentAppearance: "The floor is flooded and the torches are out.",
		Style:             "oil painting",
	}
	got := BuildScenePrompt(ctx)
	if !strings.Contains(got, "The floor is flooded and the torches are out.") {
		t.Errorf("the moment should carry the place's current look: %s", got)
	}
	// The stable prefix is the scene's shared look, so it must not move with it.
	if strings.Contains(ScenePromptPrefix(ctx), "flooded") {
		t.Error("the current appearance belongs to the moment, not the shared prefix")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run 'TestScenePromptCarriesThePlaceAsItIsNow' ./pkg/engine/`
Expected: FAIL, the prompt has no `CurrentAppearance` field to compile against.

- [ ] **Step 3: Implement**

`ScenePromptContext` gains `CurrentAppearance string` documented as the place's look right now, appended by `BuildScenePrompt` after `Appearance` as `"now: "+current`. `ScenePromptPrefix` does not read it, which is what keeps `TestPromptPrefixIsStable` true.

`sceneCastAppearance` returns `entity.DescribeAppearance(ent, sceneCastCap)` when the baseline is set, and the bounded current alone otherwise, so the cast carries the wound as well as the face.

`portrait_worker.go:130` passes `entity.DescribeAppearance(ent, 0)` to `BuildPortraitPrompt`.

`orchestrator.go:1600` and `pkg/gui/service.go:2758` set `sceneCtx.CurrentAppearance = locEntity.CurrentAppearance` beside the existing `sceneCtx.Appearance = locEntity.Appearance`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -run 'TestScene|TestBuildScenePrompt|TestPromptPrefix' ./pkg/engine/`
Expected: PASS, including the existing prefix-stability tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine pkg/gui && git commit -m "feat(engine): compose a character's current appearance into its prompts"
```

---

### Task 3: Route the writes, so the baseline stops moving

**Files:**
- Modify: `pkg/harness/extractor.go`
- Modify: `pkg/harness/extractor_test.go`

**Interfaces:**
- Consumes: `entity.CurrentAppearance` (Task 1).
- Produces: `MergeExtractedEntity` sets `CurrentAppearance` instead of `Appearance` on an existing entity.

- [ ] **Step 1: Write the failing test**

```go
// appended to pkg/harness/extractor_test.go
func TestMergeKeepsTheBaselineAndWritesTheCurrentAppearance(t *testing.T) {
	existing := &entity.Entity{
		ID:         "kaelen",
		Name:       "Kaelen",
		Type:       "character",
		Appearance: "Silver hair and blast goggles.",
	}
	raw := &ExtractedEntity{
		Name:              "Kaelen",
		Appearance:        "A scar across one cheek.",
		AppearanceChanged: true,
	}

	merged := MergeExtractedEntity(existing, raw)
	if merged.Appearance != "Silver hair and blast goggles." {
		t.Errorf("Appearance = %q, want the baseline untouched", merged.Appearance)
	}
	if merged.CurrentAppearance != "A scar across one cheek." {
		t.Errorf("CurrentAppearance = %q, want the change", merged.CurrentAppearance)
	}
}

func TestMergeGivesANewNoteItsBaseline(t *testing.T) {
	merged := MergeExtractedEntity(&entity.Entity{ID: "kaelen", Name: "Kaelen", Type: "character"}, &ExtractedEntity{
		Name: "Kaelen", Appearance: "Silver hair and blast goggles.",
	})
	if merged.Appearance != "Silver hair and blast goggles." {
		t.Errorf("Appearance = %q, want the first description to become the baseline", merged.Appearance)
	}
	if merged.CurrentAppearance != "" {
		t.Errorf("CurrentAppearance = %q, want it untouched for a new note", merged.CurrentAppearance)
	}
}

func TestMergeLeavesAnUnchangedAppearanceAlone(t *testing.T) {
	existing := &entity.Entity{ID: "kaelen", Name: "Kaelen", Appearance: "Silver hair.", CurrentAppearance: "Soaked."}
	merged := MergeExtractedEntity(existing, &ExtractedEntity{Name: "Kaelen", Appearance: "Something else entirely."})
	if merged.Appearance != "Silver hair." || merged.CurrentAppearance != "Soaked." {
		t.Errorf("an unchanged extraction moved something: %q / %q", merged.Appearance, merged.CurrentAppearance)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run TestMerge ./pkg/harness/`
Expected: the first and third FAIL, because the merge currently overwrites `Appearance`.

- [ ] **Step 3: Implement**

Replace the merge's appearance block (`pkg/harness/extractor.go:393`) with the two rules, and extend the comment above `MergeExtractedEntity` to say the baseline is written at most once:

```go
	// A description becomes the baseline only while there is none: the first
	// writer describes who the note is. After that the baseline is authored
	// content, and a change the model reports is the current appearance.
	if strings.TrimSpace(raw.Appearance) != "" {
		if strings.TrimSpace(merged.Appearance) == "" {
			merged.Appearance = raw.Appearance
		} else if raw.AppearanceChanged {
			merged.CurrentAppearance = raw.Appearance
		}
	}
```

The enricher (`pkg/engine/character_enricher.go:99`) needs no change: it fills a
description for a character that has none, which is exactly what a baseline is,
and it never writes the current appearance.

In `extractorSystemPrompt`, the line that defines `appearance_changed` gains the instruction that keeps the current read as a change:

> When "appearance_changed" is true, describe only what is different now, not the character's enduring traits.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/harness/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness && git commit -m "feat(harness): stop gameplay rewriting a note's baseline appearance"
```

---

### Task 4: Clear the current appearance on a reset

**Files:**
- Modify: `pkg/engine/restart.go`
- Modify: `pkg/engine/restart_test.go`

**Interfaces:**
- Consumes: `entity.CurrentAppearance` (Task 1).

- [ ] **Step 1: Write the failing test**

```go
// appended to pkg/engine/restart_test.go
func TestResetCampaignClearsTheCurrentAppearanceAndKeepsTheBaseline(t *testing.T) {
	f := newResetFixture(t)

	notePath := filepath.Join(f.gameDir, "entities", "sean.md")
	note, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	ent, err := entity.ParseMarkdownEntity(note)
	if err != nil {
		t.Fatal(err)
	}
	ent.CurrentAppearance = "Soaked through and limping."
	out, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, out, 0644); err != nil {
		t.Fatal(err)
	}

	if err := ResetCampaign(f.paths, f.store, f.manifest); err != nil {
		t.Fatalf("ResetCampaign: %v", err)
	}

	reset, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	player, err := entity.ParseMarkdownEntity(reset)
	if err != nil {
		t.Fatal(err)
	}
	if player.CurrentAppearance != "" {
		t.Errorf("CurrentAppearance = %q, want the circumstances cleared", player.CurrentAppearance)
	}
	if player.Appearance != "Weathered." {
		t.Errorf("Appearance = %q, want the authored description kept", player.Appearance)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run TestResetCampaignClearsTheCurrentAppearance ./pkg/engine/`
Expected: FAIL with `CurrentAppearance = "Soaked through and limping."`.

- [ ] **Step 3: Implement**

`clearRuntimeFields` also clears the field, and its comment gains it:

```go
	if len(ent.History) == 0 && ent.State == nil && strings.TrimSpace(ent.CurrentAppearance) == "" {
		return nil
	}
	ent.History = nil
	ent.State = nil
	ent.CurrentAppearance = ""
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -run TestResetCampaign ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine && git commit -m "feat(engine): a reset clears the circumstances, not just the history"
```

---

### Task 5: Record what a turn rewrote, so an undo can put it back

**Files:**
- Modify: `pkg/engine/history.go`
- Modify: `pkg/engine/timeline.go`
- Modify: `pkg/engine/rewind_test.go`

**Interfaces:**
- Consumes: `entity.CurrentAppearance` (Task 1).
- Produces: `Turn.CurrentAppearances []EntityAppearance`, `EntityAppearance{EntityID, Before, After string}`.

- [ ] **Step 1: Write the failing test**

```go
// appended to pkg/engine/rewind_test.go
func TestRewindRestoresTheAppearanceTheDiscardedTurnsReplaced(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	store := newTestStore(t)
	history := NewHistoryLogger(filepath.Join(t.TempDir(), "history.jsonl"))
	timeline := NewTimeline(paths, store, history, "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID: "kaelen", Name: "Kaelen", Type: "character", Appearance: "Silver hair.", Body: "A traveller.",
	})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	scar := func(n int, appearance string) {
		turn := Turn{Number: n, Timestamp: time.Now(), Mode: "Do", Input: "I press on"}
		turn.Entities = []entity.Mention{{ID: "kaelen", Kind: entity.MentionPlayer}}
		turn.CurrentAppearances = []EntityAppearance{{EntityID: "kaelen", Before: "", After: appearance}}
		if err := timeline.RecordTurn(&turn, nil); err != nil {
			t.Fatalf("RecordTurn(%d): %v", n, err)
		}
	}
	scar(1, "A cut above one eye.")
	scar(2, "The cut has closed to a scar.")

	if err := timeline.RewindToTurn(0); err != nil {
		t.Fatalf("RewindToTurn: %v", err)
	}

	ent, err := store.GetEntity("kaelen")
	if err != nil || ent == nil {
		t.Fatalf("read the note: %v", err)
	}
	if ent.CurrentAppearance != "" {
		t.Errorf("CurrentAppearance = %q, want the value from before the oldest discarded turn", ent.CurrentAppearance)
	}
	if ent.Appearance != "Silver hair." {
		t.Errorf("Appearance = %q, want the baseline untouched", ent.Appearance)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run TestRewindRestoresTheAppearance ./pkg/engine/`
Expected: FAIL, `undefined: EntityAppearance`.

- [ ] **Step 3: Implement**

`pkg/engine/history.go`, next to the other advisory records:

```go
// EntityAppearance records one entity's current appearance before and after a
// turn. The value before it is what a rewind puts back, because a note on its own
// cannot say what a character looked like three turns ago.
type EntityAppearance struct {
	EntityID string `json:"id"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
}
```

`Turn` gains `CurrentAppearances []EntityAppearance `json:"current_appearances,omitempty"``.

`stageEntities` records one entry per entity whose current appearance it is about to change, capturing `ent.CurrentAppearance` before the merge and the merged value after, and only when the two differ.

`RewindToTurn` builds the restore map before it prunes, then applies it in the pass that already writes the notes:

```go
	// Newest first, so an entity several discarded turns touched settles on the
	// value from before the oldest of them.
	restore := make(map[string]string)
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Number <= target {
			continue
		}
		for _, change := range turns[i].CurrentAppearances {
			restore[change.EntityID] = change.Before
		}
		for _, mention := range turns[i].Entities {
			affected[mention.ID] = true
		}
	}
	for id := range restore {
		affected[id] = true
	}
```

`pruneEntityHistory` becomes `undoDiscardedChanges(affected map[string]bool, restore map[string]string, target int)`: it applies `ent.CurrentAppearance = restore[id]` for a restored id and keeps the history pruning, skipping a note only when neither changes, so each note is still written once.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/engine/`
Expected: PASS, including the existing rewind tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine && git commit -m "feat(engine): an undo restores the appearance the turn replaced"
```

---

### Task 6: The location-art option

**Files:**
- Modify: `pkg/config/types.go`
- Modify: `pkg/media/image.go`, `pkg/media/location_art.go`
- Modify: `pkg/gui/service.go`, `pkg/export/script.go`
- Modify: `pkg/config/types_test.go`, `pkg/media/image_test.go`
- Modify: `pkg/gui/docs/13-configuration-reference.md` (regenerated)

**Interfaces:**
- Consumes: `entity.DescribeAppearance` (Task 1).
- Produces: `config.Config.LocationArtAppearance() string` returning `"original"` or `"original_and_current"`; `media.AppearanceHash(ent, providerParams string, layerCurrent bool) string`; `media.BuildLocationPrompt(location, worldStyle string, layerCurrent bool) string`; `media.NewArtStore(client, cache, worldStyle, providerParams string, layerCurrent bool) *ArtStore`.

- [ ] **Step 1: Write the failing tests**

```go
// appended to pkg/config/types_test.go
func TestLocationArtAppearanceDefaultsToLayering(t *testing.T) {
	cfg := &Config{}
	if got := cfg.LocationArtAppearance(); got != LocationArtAppearanceLayered {
		t.Errorf("LocationArtAppearance = %q, want %q", got, LocationArtAppearanceLayered)
	}

	cfg.Media.Image.LocationArtAppearance = "original"
	if got := cfg.LocationArtAppearance(); got != LocationArtAppearanceOriginal {
		t.Errorf("LocationArtAppearance = %q, want %q", got, LocationArtAppearanceOriginal)
	}

	cfg.Media.Image.LocationArtAppearance = "nonsense"
	if got := cfg.LocationArtAppearance(); got != LocationArtAppearanceLayered {
		t.Errorf("an unknown value = %q, want the default", got)
	}
}
```

```go
// appended to pkg/media/image_test.go
func TestAppearanceHashFollowsTheLayerDecision(t *testing.T) {
	loc := &entity.Entity{
		ID:                "hall",
		Name:              "The Hall",
		Appearance:        "A vaulted stone hall.",
		CurrentAppearance: "Flooded.",
	}

	layered := AppearanceHash(loc, "builtin:", true)
	if layered == AppearanceHash(loc, "builtin:", false) {
		t.Error("layering the current appearance should change the key")
	}
	if layered != AppearanceHash(loc, "builtin:", true) {
		t.Error("the same decision should produce the same key")
	}

	original := AppearanceHash(loc, "builtin:", false)
	loc.CurrentAppearance = "On fire."
	if AppearanceHash(loc, "builtin:", false) != original {
		t.Error("with the current appearance ignored, a change to it must not move the key")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestLocationArtAppearance|TestAppearanceHash' ./pkg/config/ ./pkg/media/`
Expected: FAIL to compile.

- [ ] **Step 3: Implement**

`pkg/config/types.go`, beside the `Trigger` field:

```go
	// LocationArtAppearance decides what a place's own picture is drawn from and
	// keyed by: the authored appearance alone ("original"), or that appearance
	// with the current one layered on it ("original_and_current", the default).
	LocationArtAppearance string `yaml:"location_art_appearance,omitempty" json:"location_art_appearance,omitempty"`
```

with the two constants and the accessor next to `ImageTrigger()`:

```go
const (
	LocationArtAppearanceOriginal = "original"
	LocationArtAppearanceLayered  = "original_and_current"
)

// LocationArtAppearance resolves the location-art option, treating an empty or
// unknown value as the layered default.
func (c *Config) LocationArtAppearance() string {
	if c != nil && c.Media.Image.LocationArtAppearance == LocationArtAppearanceOriginal {
		return LocationArtAppearanceOriginal
	}
	return LocationArtAppearanceLayered
}
```

`pkg/media/image.go`: `AppearanceHash` and `BuildLocationPrompt` take `layerCurrent bool`. The hash uses `entity.DescribeAppearance(ent, 0)` when layering and either field is set, and keeps the tags-and-state fallback otherwise. `GenerateLocationImage` and `LocationLayers` pass the flag down from `ArtStore`, which gains the field through `NewArtStore`.

`pkg/gui/service.go:3006` and `pkg/export/script.go:542` pass `cfg.LocationArtAppearance() != config.LocationArtAppearanceOriginal`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/config/ ./pkg/media/ ./pkg/gui/ ./pkg/export/`
Expected: PASS. Fix any test that constructs an `ArtStore` or calls `AppearanceHash` by passing `true` for the layered default.

- [ ] **Step 5: Regenerate the config reference and commit**

Run: `go test ./pkg/gui -update-docs` and check the diff names the new key.

```bash
git add pkg/config pkg/media pkg/gui pkg/export && git commit -m "feat(media): let a campaign pin a place's art to its authored look"
```

---

### Task 7: The editing surface, the sheet, and the guides

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`, `pkg/gui/schema.go`, `pkg/gui/entitycatalog.go`
- Modify: `frontend/src/types.ts`, `frontend/src/components/CharacterSheetDrawer.tsx`, `frontend/src/components/providers/ImageProviderEditor.tsx`
- Modify: `frontend/src/components/CharacterSheetDrawer.test.tsx`
- Create: `frontend/src/components/providers/ImageProviderEditor.test.tsx`
- Modify: `pkg/gui/docs/10-codex-yaml.md`

**Interfaces:**
- Consumes: `config.LocationArtAppearance()` (Task 6), `entity.DescribeAppearance` (Task 1).

- [ ] **Step 1: Write the failing frontend test**

```tsx
// frontend/src/components/providers/ImageProviderEditor.test.tsx
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ImageProviderEditor } from './ImageProviderEditor';
import type { ImageConfig } from '../../types';

const entry = (): ImageConfig => ({ type: 'gemini', auto_generate: true });

describe('ImageProviderEditor location art', () => {
  it('offers the authored look and the layered look, and defaults to layered', () => {
    const onChange = vi.fn();
    render(
      <ImageProviderEditor
        entry={entry()}
        onChange={onChange}
        presets={{}}
        geminiSharedKey={false}
        onTest={() => {}}
        testingCategory={null}
        testResult={null}
      />,
    );

    const select = screen.getByLabelText(/location art/i) as HTMLSelectElement;
    expect(select.value || 'original_and_current').toBe('original_and_current');

    fireEvent.change(select, { target: { value: 'original' } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ location_art_appearance: 'original' }));
  });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd frontend && npx vitest run src/components/providers/ImageProviderEditor.test.tsx`
Expected: FAIL, no such control.

- [ ] **Step 3: Implement**

`frontend/src/types.ts`: `ImageConfig` gains `location_art_appearance?: 'original_and_current' | 'original';`. The `PlayerDTO` type gains `current_appearance?: string`.

`ImageProviderEditor.tsx`: a labelled select beside **Auto-generate Scene Art**:

```tsx
<label htmlFor="location-art-appearance" className="text-xs font-sans uppercase text-stone-300">
  Location Art Appearance
</label>
<select
  id="location-art-appearance"
  value={entry.location_art_appearance ?? 'original_and_current'}
  title="The authored look alone, or that look with what play has done to the place layered on top of it."
  onChange={(e) =>
    onChange({ ...entry, location_art_appearance: e.target.value as 'original_and_current' | 'original' })
  }
  className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
>
  <option value="original_and_current">The authored look with what happened to it</option>
  <option value="original">The authored look alone</option>
</select>
```

`CharacterSheetDrawer.tsx`: under the appearance line, render the current appearance when it is set, labelled so it reads as the present state rather than the identity.

`pkg/gui/types.go`: `PlayerDTO` and `PlayerCharacterDTO` gain `CurrentAppearance string `json:"current_appearance,omitempty"``; `pkg/gui/service.go:826` fills it from `ent.CurrentAppearance`; the entity DTO's markdown already carries the key.

`pkg/gui/schema.go`: a description for `current_appearance` ("How the note looks right now, layered on `appearance`. Written by play."), and the `appearance` description says it is the baseline.

`pkg/gui/entitycatalog.go`: `entityKeys("appearance", "current_appearance", "gender", "age", "voice")` for `character`, and `entityKeys("appearance", "current_appearance")` for `location`.

`pkg/gui/docs/10-codex-yaml.md`: the full schema block gains `current_appearance` with a comment saying play writes it, the per-type list names it, and the reset section says a restart clears it.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd frontend && npx vitest run && npx tsc --noEmit`, then `go test ./pkg/gui/`.
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend && git commit -m "feat(studio): show and edit a note's current appearance"
```

---

## Verification

- `go test ./...`
- `cd frontend && npx vitest run && npx tsc --noEmit`
- `mise run lint`, `mise run lint:docs`, `STRICT=1 mise run lint:prose`
- `mise run test:e2e` (the settings studio and play views render)
- A manual pass: scar a character over two turns, undo one, and confirm the scene prompt carries the earlier description; reset the campaign and confirm the current appearance is gone and the baseline remains.