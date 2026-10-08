# Batch Entity Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate a previewed batch of entities for an existing world, seeded with its lore and linked to its existing entities.

**Architecture:** `pkg/worldgen.GenerateEntities` reuses WG-1's generator and link step with an added existing-entity set; endpoints return a preview and accept writes; the studio shows the batch.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-batch-entity-generation-design.md`
**Depends on:** WG-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- The preview writes nothing; only accept writes.
- `count` is clamped to a cap (10).
- Conventional Commits, subject under 72 chars.

---

### Task 1: The request, context, and pipeline

**Files:**
- Create: `pkg/worldgen/entities.go`
- Test: `pkg/worldgen/entities_test.go`

**Interfaces:**
- Consumes: `Generator` (WG-1), `entity`.
- Produces: `EntityRequest`, `WorldContext`, `func GenerateEntities(ctx, Generator, WorldContext, EntityRequest) ([]DraftEntity, error)`.

- [x] **Step 1: Write the failing test**

```go
func TestGenerateEntitiesClampsAndGenerates(t *testing.T) {
	g := &jsonGen{responses: []string{`{"entities":[{"name":"A","type":"faction"},{"name":"B","type":"faction"}]}`}}
	got, err := GenerateEntities(context.Background(), g, WorldContext{ID: "w"}, EntityRequest{Instruction: "x", Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("entities = %d", len(got))
	}
	if _, err := GenerateEntities(context.Background(), g, WorldContext{ID: "w"}, EntityRequest{Count: 99}); err != nil {
		t.Fatal(err)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/worldgen/ -run TestGenerateEntities -v`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

Add the types and a `GenerateEntities` that clamps `Count`, builds the prompt, calls the generator,
and parses the response into `[]DraftEntity` (repairing JSON first).

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/worldgen/ -run TestGenerateEntities -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/worldgen/entities.go pkg/worldgen/entities_test.go
git commit -m "feat(worldgen): generate an entity batch"
```

---

### Task 2: Context assembly

**Files:**
- Modify: `pkg/worldgen/entities.go`
- Test: `pkg/worldgen/entities_test.go` (append)

**Interfaces:**
- Consumes: `core.WorldManifest`, existing entities.
- Produces: `func buildEntityPrompt(world WorldContext, req EntityRequest) string`.

- [x] **Step 1: Write the failing test**

```go
func TestEntityPromptIsBounded(t *testing.T) {
	w := WorldContext{ID: "w", Lore: strings.Repeat("lore ", 5000)}
	for i := 0; i < 500; i++ {
		w.Entities = append(w.Entities, EntitySummary{ID: fmt.Sprintf("e%d", i), Name: "X", Type: "character"})
	}
	p := buildEntityPrompt(w, EntityRequest{Instruction: "add a faction"})
	if len(p) > 8000 {
		t.Fatalf("prompt too long: %d", len(p))
	}
	if !strings.Contains(p, "add a faction") {
		t.Fatal("the instruction must be present")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/worldgen/ -run TestEntityPromptIsBounded -v`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

Assemble the manifest, a truncated lore, and a capped entity list (id, name, type, summary), then the
instruction, with hard length caps.

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/worldgen/ -run TestEntityPromptIsBounded -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/worldgen/entities.go pkg/worldgen/entities_test.go
git commit -m "feat(worldgen): bound the entity-generation context"
```

---

### Task 3: Linking against existing entities

**Files:**
- Modify: `pkg/worldgen/entities.go`
- Test: `pkg/worldgen/entities_test.go` (append)

**Interfaces:**
- Consumes: WG-1's `linkDraft`.
- Produces: `func linkBatch(batch []DraftEntity, existing []EntitySummary) []DraftEntity` and a dropped-link note.

- [x] **Step 1: Write the failing test**

```go
func TestLinkBatchResolvesExisting(t *testing.T) {
	batch := []DraftEntity{{ID: "new-crew", Body: "Rivals of [[The Tidewatch]]."}}
	existing := []EntitySummary{{ID: "the-tidewatch", Name: "The Tidewatch"}}
	got := linkBatch(batch, existing)
	if !strings.Contains(got[0].Body, "[[The Tidewatch]]") {
		t.Fatal("a link to an existing entity should survive")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/worldgen/ -run TestLinkBatch -v`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

Union the batch ids with the existing ids and validate every wikilink, dropping and noting an
unresolved one.

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/worldgen/ -run TestLinkBatch -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/worldgen/entities.go pkg/worldgen/entities_test.go
git commit -m "feat(worldgen): link a batch to existing entities"
```

---

### Task 4: The endpoints

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`, `pkg/gui/types.go`
- Test: `pkg/gui/worldgen_entities_test.go`

**Interfaces:**
- Consumes: `GenerateEntities` (Task 1).
- Produces: `POST /api/world/{id}/generate-entities` (preview) and `POST /api/world/{id}/entities/accept`.

- [x] **Step 1: Write the failing tests**

```go
func TestEntityPreviewWritesNothing(t *testing.T) { /* the world dir is unchanged */ }
func TestEntityAcceptWritesAndIndexes(t *testing.T) { /* the entities appear and are indexed */ }
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestEntity -v`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

The preview endpoint builds the `WorldContext` from the world and its entities, runs the pipeline, and
returns the batch plus dropped-link notes. The accept endpoint writes each via the existing
`SaveWorldEntity` path, refusing or renaming an id clash.

- [x] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestEntity -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): preview and accept an entity batch"
```

---

### Task 5: The studio action

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `frontend/src/components/EntityBatchDialog.test.tsx`

**Interfaces:**
- Consumes: the endpoints (Task 4).
- Produces: a batch generation action with a preview.

- [x] **Step 1: Write the failing test**

```tsx
test("previews a batch and accepts it", () => {
  render(<EntityBatchDialog worldId="w" onClose={() => {}} />);
  // Enter an instruction, generate, assert the preview lists the entities, click accept.
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `npm run test -- EntityBatchDialog`
Expected: FAIL.

- [x] **Step 3: Write minimal implementation**

Add an action on a world that opens the dialog, collects the instruction and counts, shows the
preview (entities and their links, with dropped-link notes), and offers accept/discard.

- [x] **Step 4: Run test to verify it passes**

Run: `npm run test -- EntityBatchDialog`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): add the entity batch action"
```

---

### Task 6: Verification

- [x] **Step 1: Duplicate guard**

Add a test that accepting a batch with an existing id refuses or renames.

- [x] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [x] **Step 3: Confirm the acceptance criteria**

- The batch matches the requested kind and count (clamped).
- Links resolve against existing entities; unresolved ones are dropped and noted.
- The preview writes nothing; accept writes and indexes.
- No existing entity is modified.

- [x] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the entity batch accept path"
```
