# World Generation Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate a draft world from a brief in steps, with progress, producing cross-linked entities and never committing a live world.

**Architecture:** `pkg/worldgen` orchestrates structured model calls (outline, places/factions, characters, cross-link) through a `Generator` seam; an NDJSON endpoint streams progress; the studio collects the brief and hands the draft to review.

**Tech Stack:** Go standard library; the existing provider router; `jsonrepair` for recovery.

**Spec:** `docs/superpowers/specs/2026-10-05-world-generation-pipeline-design.md`
**Depends on:** PKG-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- A draft is never written into `worlds/<id>/`; it lives in `worlds/.drafts/` or memory.
- Every wikilink in a draft resolves to a draft entity, or is dropped.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The types and the generator seam

**Files:**
- Create: `pkg/worldgen/worldgen.go`
- Test: `pkg/worldgen/worldgen_test.go`

**Interfaces:**
- Consumes: `core.WorldManifest`.
- Produces: `Brief`, `Counts`, `Draft`, `DraftEntity`, `Step`, `Generator`, `func Generate(ctx, Generator, Brief, func(Step)) (Draft, error)`.

- [x] **Step 1: Write the failing test**

```go
package worldgen

import (
	"context"
	"testing"
)

type stubGen struct{ calls int }

func (s *stubGen) GenerateJSON(_ context.Context, prompt, schema string) ([]byte, error) {
	s.calls++
	return []byte(`{}`), nil
}

func TestGenerateRunsEveryStep(t *testing.T) {
	g := &stubGen{}
	var steps []Step
	_, err := Generate(context.Background(), g, Brief{Premise: "a drowned kingdom"}, func(s Step) {
		steps = append(steps, s)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 4 {
		t.Fatalf("steps = %d, want 4", len(steps))
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/worldgen/ -run TestGenerateRunsEveryStep -v`
Expected: FAIL, `undefined: Generate`.

- [x] **Step 3: Write minimal implementation**

Add the types and a `Generate` that calls `Generator.GenerateJSON` once per step, emitting a `Step`
per call. The stub returns `{}`, so parsing must tolerate empty structures at this stage.

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/worldgen/ -run TestGenerateRunsEveryStep -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/worldgen/worldgen.go pkg/worldgen/worldgen_test.go
git commit -m "feat(worldgen): add the pipeline skeleton"
```

---

### Task 2: The outline and places/factions steps

**Files:**
- Create: `pkg/worldgen/steps.go`
- Test: `pkg/worldgen/steps_test.go`

**Interfaces:**
- Consumes: `Generator`.
- Produces: `outlineSchema`, `placesSchema`, `func runOutline`, `func runPlaces`.

- [x] **Step 1: Write the failing test**

```go
func TestPlacesStepParsesLocationsAndFactions(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"name":"Ashen Reach","genre":"dark fantasy","premise":"a dying frontier"}`,
		`{"locations":[{"name":"Saltmarch","tags":["coast"]}],"factions":[{"name":"The Tidewatch"}]}`,
	}}
	draft, err := Generate(context.Background(), g, Brief{Premise: "x", Counts: Counts{Locations: 1, Factions: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if draft.World.Name != "Ashen Reach" || len(draft.Entities) < 2 {
		t.Fatalf("draft = %+v", draft)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/worldgen/ -run TestPlacesStep -v`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

Define the JSON schemas and the parse of each step's response into the draft's world manifest and
entities. Use `jsonrepair.Repair` before unmarshalling so a fenced or malformed response recovers.

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/worldgen/ -run TestPlacesStep -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/worldgen/steps.go pkg/worldgen/steps_test.go
git commit -m "feat(worldgen): generate the outline and places"
```

---

### Task 3: The characters step and cross-linking

**Files:**
- Modify: `pkg/worldgen/steps.go`
- Test: `pkg/worldgen/steps_test.go` (append)

**Interfaces:**
- Consumes: `entity.ParseMarkdownEntity`.
- Produces: `func runCharacters`, `func linkDraft(Draft) Draft`.

- [x] **Step 1: Write the failing test**

```go
func TestLinkDraftResolvesAndDropsUnknownLinks(t *testing.T) {
	d := Draft{Entities: []DraftEntity{
		{ID: "saltmarch", Body: "Near [[The Tidewatch]] and [[Nowhere]]."},
		{ID: "the-tidewatch", Body: "A faction."},
	}}
	got := linkDraft(d)
	if !strings.Contains(got.Entities[0].Body, "[[The Tidewatch]]") {
		t.Fatal("a known link should survive")
	}
	if strings.Contains(got.Entities[0].Body, "[[Nowhere]]") {
		t.Fatal("an unknown link should be dropped")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/worldgen/ -run TestLinkDraft -v`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

Generate characters seeded with the outline and places, then validate every `[[wikilink]]` in every
entity body against the draft's ids (via `entity.WikilinkTarget` and `entity.Slugify`) and drop an
unresolved one.

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/worldgen/ -run TestLinkDraft -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/worldgen/steps.go pkg/worldgen/steps_test.go
git commit -m "feat(worldgen): generate characters and cross-link the draft"
```

---

### Task 4: The endpoint and draft persistence

**Files:**
- Create: `pkg/gui/worldgen.go`
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/types.go`
- Test: `pkg/gui/worldgen_test.go`

**Interfaces:**
- Consumes: `worldgen.Generate`, the provider router.
- Produces: `POST /api/world/generate` (NDJSON), `Service.GenerateWorld(ctx, brief, emit) (DraftDTO, error)`, and a draft store under `worlds/.drafts/`.

- [x] **Step 1: Write the failing test**

```go
func TestGenerateWorldStreamsStepsAndDraft(t *testing.T) {
	svc := newTestServiceWithStubGenerator(t)
	var events []TurnEvent
	draft, err := svc.GenerateWorld(context.Background(), worldgen.Brief{Premise: "x"}, func(e TurnEvent) error {
		events = append(events, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft.World.Name == "" || len(events) == 0 {
		t.Fatalf("draft %+v events %d", draft, len(events))
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestGenerateWorld -v`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

Build a `worldgen.Generator` from the router's `generator` role (falling back to `gm`), run the
pipeline, stream a `step` event per step and a final `draft` event, and persist the draft to
`worlds/.drafts/<id>.yaml` so a reload can resume review.

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestGenerateWorld -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): stream world generation"
```

---

### Task 5: The studio flow

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `frontend/src/components/WorldGenerateDialog.test.tsx`

**Interfaces:**
- Consumes: the endpoint (Task 4).
- Produces: a brief form, step progress, and a hand-off to review (WG-5).

- [x] **Step 1: Write the failing test**

```tsx
test("collects a brief and shows step progress", () => {
  render(<WorldGenerateDialog onCancel={() => {}} onDraft={() => {}} />);
  fireEvent.change(screen.getByLabelText(/premise/i), { target: { value: "a drowned kingdom" } });
  fireEvent.click(screen.getByRole("button", { name: /generate/i }));
  // Assert a step indicator appears.
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `npm run test -- WorldGenerateDialog`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

Add a dialog that collects the brief (premise, optional genre/name, counts), calls the endpoint,
renders step progress, and passes the draft to the review surface (WG-5). Add a cancel that aborts
the stream.

- [x] **Step 4: Run test to verify it passes**

Run: `npm run test -- WorldGenerateDialog`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): add the world generation flow"
```

---

### Task 6: Verification

- [x] **Step 1: Regression guard**

Add a test that a malformed step response is repaired by `jsonrepair` and still yields a draft.

- [x] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [x] **Step 3: Confirm the acceptance criteria**

- The pipeline runs four steps and emits progress.
- The draft has the requested counts and cross-linked entities.
- A step failure keeps earlier steps; cancellation aborts.
- No live world is written.

- [x] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the world generation pipeline"
```
