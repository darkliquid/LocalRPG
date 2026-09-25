# Entity Memories & Memory Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every entity searchable memories: GM-declared narrative memories and engine-derived mechanical memories, stored and FTS-indexed in SQLite and reachable through `search_memories` and `get_entity_timeline`.

**Architecture:** A new `memories` table family (memories, memory_entities, memory_tags, memories_fts) is added by migration v5. A `Timeline` step stages the structured turn's memory declarations and writes one engine memory per resolved check. Query tools are added to the single `harness.ToolSpecs()` surface and dispatched in `pkg/tools`. Memories are canonical in the database but re-derivable from `history.jsonl`, which embeds the accepted declarations on the `Turn`.

**Tech Stack:** Go 1.27.1 (`pkg/entity`, `pkg/storage`, `pkg/harness`, `pkg/tools`, `pkg/engine`), `modernc.org/sqlite` (no CGO), React 19 + TypeScript.

**Spec:** `docs/superpowers/specs/2026-09-25-entity-memories-and-memory-tools-design.md`
**Depends on:** Structured Turn Protocol plan (the `TurnSubmission.Memories` field and `CheckResult`).

## Global Constraints

- Go 1.27.1. Standard library only for tests; no testify.
- Use `interface{}`, not `any`; `go vet ./...` clean.
- `cache/index.db` stays disposable; a dropped database is repaired from `history.jsonl` by `EnsureIndexed`.
- Memories never auto-inject into the context prompt; they are tool-reachable only.
- A memory write failure fails the turn, because the turn and its memories must stay consistent.
- FTS5 with the porter tokenizer, mirroring `entities_fts`/`turns_fts`; migrations are versioned via `PRAGMA user_version`.
- TypeScript: `strict`; `npx tsc --noEmit` is the gate.
- Conventional Commits with a scope; subject under 72 characters.

---

## File Map

**Create**
- `pkg/entity/memory.go` — `Memory`, `MemorySource`, kind constants.
- `pkg/entity/memory_test.go`.
- `pkg/storage/memory.go` — `SaveMemory`, `ListMemoriesForEntity`, `SearchMemories`, `MemoryHit`, rebuild helpers.
- `pkg/storage/memory_test.go`.
- `pkg/tools/memory.go` — `searchMemories`, `getEntityTimeline` implementations.
- `pkg/tools/memory_test.go`.

**Modify**
- `pkg/storage/migrate.go` — migration v5 (tables + FTS + triggers).
- `pkg/storage/fts.go` — `EnsureFTS` gains a memories backfill.
- `pkg/harness/tools.go` — two new `ToolSpec`s.
- `pkg/tools/tools.go` — dispatch the two names.
- `pkg/engine/history.go` — `Turn.Memories`.
- `pkg/engine/timeline.go` — memory staging + mechanical memory.
- `pkg/engine/orchestrator.go` — pass declarations through.
- `pkg/gui/types.go`, `pkg/gui/service.go` — entity timeline endpoint.
- `frontend/src/components/Codex.tsx` (or equivalent) — read-only timeline.

---

### Task 1: Memory type

**Files:**
- Create: `pkg/entity/memory.go`
- Test: `pkg/entity/memory_test.go`

**Interfaces:**
- Produces: `Memory{ID,Turn,Kind,EntityRefs,Text,Importance,Tags,Source,CheckID,CreatedAt}`; `MemorySource`; kind constants `MemoryEvent`, `MemoryRelationship`, `MemoryDiscovery`, `MemoryDialogue`, `MemoryMechanical`; `ValidateMemory(m *Memory) error`.

- [ ] **Step 1: Write the failing test**

```go
package entity

import "testing"

func TestValidateMemory(t *testing.T) {
	ok := &Memory{Turn: 3, Kind: MemoryEvent, EntityRefs: []string{"kae"}, Text: "Crossed the bridge.", Importance: 3}
	if err := ValidateMemory(ok); err != nil {
		t.Fatalf("valid memory rejected: %v", err)
	}
	empty := &Memory{Turn: 3, Kind: MemoryEvent, Text: "x", Importance: 3}
	if err := ValidateMemory(empty); err == nil {
		t.Fatal("memory with no entity refs should be invalid")
	}
	noText := &Memory{Turn: 3, Kind: MemoryEvent, EntityRefs: []string{"kae"}, Importance: 3}
	if err := ValidateMemory(noText); err == nil {
		t.Fatal("memory with no text should be invalid")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestValidateMemory ./pkg/entity/`
Expected: FAIL.

- [ ] **Step 3: Implement**

`pkg/entity/memory.go` with the struct from spec §3, the kind/source constants, and `ValidateMemory` rejecting empty text, no entity refs, and importance outside 1–5.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestValidateMemory ./pkg/entity/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/entity/memory.go pkg/entity/memory_test.go
git commit -m "feat(entity): add the memory record type"
```

---

### Task 2: Storage schema and writes

**Files:**
- Modify: `pkg/storage/migrate.go`
- Create: `pkg/storage/memory.go`
- Test: `pkg/storage/memory_test.go`

**Interfaces:**
- Produces: migration v5; `(*Store).SaveMemory(m *entity.Memory) (int64, error)`; `(*Store).ListMemoriesForEntity(entityID string, limit int) ([]entity.Memory, error)`.

- [ ] **Step 1: Write the failing test**

```go
package storage

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestSaveAndListMemories(t *testing.T) {
	store := openTestStore(t)
	id, err := store.SaveMemory(&entity.Memory{Turn: 2, Kind: entity.MemoryEvent, EntityRefs: []string{"kae", "player"}, Text: "Crossed the rope bridge.", Importance: 3, Tags: []string{"travel"}})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if id == 0 {
		t.Fatal("SaveMemory returned id 0")
	}
	got, err := store.ListMemoriesForEntity("kae", 10)
	if err != nil {
		t.Fatalf("ListMemoriesForEntity: %v", err)
	}
	if len(got) != 1 || got[0].Text != "Crossed the rope bridge." {
		t.Fatalf("memories = %+v", got)
	}
	if len(got[0].EntityRefs) != 2 {
		t.Fatalf("entity refs = %v", got[0].EntityRefs)
	}
}
```

`openTestStore(t)` opens a `storage.Store` on `t.TempDir()` (reuse the helper from existing storage tests).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestSaveAndListMemories ./pkg/storage/`
Expected: FAIL.

- [ ] **Step 3: Implement**

- `migrate.go`: append `{version: 5, apply: addMemoriesTables}`; the function runs the DDL and triggers from spec §4.1 and creates `memories_fts`.
- `pkg/storage/memory.go`: `SaveMemory` inserts `memories`, `memory_entities`, `memory_tags`, and the FTS row in one transaction; `ListMemoriesForEntity` joins `memory_entities`, ordered by turn then id.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestSaveAndListMemories ./pkg/storage/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/storage/migrate.go pkg/storage/memory.go pkg/storage/memory_test.go
git commit -m "feat(storage): add the memories tables and writes"
```

---

### Task 3: Memory search and ranking

**Files:**
- Modify: `pkg/storage/memory.go`, `pkg/storage/fts.go`
- Test: `pkg/storage/memory_search_test.go`

**Interfaces:**
- Produces: `type MemoryHit struct { ID int64; Turn int; Kind string; Snippet string; Importance int }`; `(*Store).SearchMemories(match, entityID, kind string, minImportance, limit int) ([]MemoryHit, error)`; `rankMemoryHits(hits []MemoryHit, currentTurn int, halfLife int) []MemoryHit`.

- [ ] **Step 1: Write the failing test**

```go
func TestSearchMemoriesAndRank(t *testing.T) {
	store := openTestStore(t)
	must := func(m *entity.Memory) { if _, err := store.SaveMemory(m); err != nil { t.Fatal(err) } }
	must(&entity.Memory{Turn: 1, Kind: entity.MemoryEvent, EntityRefs: []string{"kae"}, Text: "Found a silver locket.", Importance: 2})
	must(&entity.Memory{Turn: 40, Kind: entity.MemoryEvent, EntityRefs: []string{"kae"}, Text: "The silver locket is cursed.", Importance: 5})

	hits, err := store.SearchMemories("silver locket", "kae", "", 0, 10)
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestRankPrefersImportantRecent(t *testing.T) {
	hits := []MemoryHit{{ID: 1, Turn: 1, Importance: 2}, {ID: 2, Turn: 40, Importance: 5}}
	ranked := rankMemoryHits(hits, 41, 20)
	if ranked[0].ID != 2 {
		t.Fatalf("ranked = %+v, want id 2 first", ranked)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestSearchMemoriesAndRank|TestRankPrefersImportantRecent' ./pkg/storage/`
Expected: FAIL.

- [ ] **Step 3: Implement**

- `SearchMemories` uses `BuildMatch`-style sanitisation for `match` and queries `memories_fts` joined to `memories`, with optional `entityID` (via `memory_entities`), `kind`, and `minImportance` filters; returns `bm25`-ordered hits.
- `rankMemoryHits` re-ranks by `importance * 0.5^((currentTurn-turn)/halfLife)` (with `halfLife` guarded ≥ 1); used by the tool layer.
- `EnsureFTS` backfills `memories_fts` for rows missing an index entry.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestSearchMemoriesAndRank|TestRankPrefersImportantRecent' ./pkg/storage/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/storage/memory.go pkg/storage/fts.go pkg/storage/memory_search_test.go
git commit -m "feat(storage): search and rank entity memories"
```

---

### Task 4: Memory tools

**Files:**
- Modify: `pkg/harness/tools.go`, `pkg/tools/tools.go`
- Create: `pkg/tools/memory.go`
- Test: `pkg/tools/memory_test.go`

**Interfaces:**
- Consumes: `store.SearchMemories`, `store.ListMemoriesForEntity`, `rankMemoryHits`.
- Produces: `searchMemories(args map[string]interface{}) (string, bool)`; `getEntityTimeline(args map[string]interface{}) (string, bool)`; the two `ToolSpec`s.

- [ ] **Step 1: Write the failing test**

```go
func TestSearchMemoriesTool(t *testing.T) {
	store := newToolTestStore(t) // existing helper
	if _, err := store.SaveMemory(&entity.Memory{Turn: 2, Kind: entity.MemoryEvent, EntityRefs: []string{"kae"}, Text: "Found a silver locket.", Importance: 4}); err != nil {
		t.Fatal(err)
	}
	exec := NewExecutor(store, 4000)
	out, ok := exec.Execute(harness.ToolCall{Name: "search_memories", Arguments: `{"query":"silver locket"}`})
	if !ok || !strings.Contains(out, "silver locket") {
		t.Fatalf("search_memories = %q, ok=%v", out, ok)
	}
}

func TestGetEntityTimelineTool(t *testing.T) {
	store := newToolTestStore(t)
	if _, err := store.SaveMemory(&entity.Memory{Turn: 5, Kind: entity.MemoryRelationship, EntityRefs: []string{"kae"}, Text: "Distrusts the warden.", Importance: 3}); err != nil {
		t.Fatal(err)
	}
	exec := NewExecutor(store, 4000)
	out, ok := exec.Execute(harness.ToolCall{Name: "get_entity_timeline", Arguments: `{"entity":"kae"}`})
	if !ok || !strings.Contains(out, "Distrusts the warden") {
		t.Fatalf("get_entity_timeline = %q, ok=%v", out, ok)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestSearchMemoriesTool|TestGetEntityTimelineTool' ./pkg/tools/`
Expected: FAIL.

- [ ] **Step 3: Implement**

- Add two `ToolSpec`s to `harness.ToolSpecs()` (`search_memories` with `query` required, `entity`/`kind`/`min_importance`/`limit` optional; `get_entity_timeline` with `entity` required, `limit` optional default 20).
- Add cases to the dispatch switch and implement in `pkg/tools/memory.go`, reusing `BuildMatch`, `cap`, and the existing argument helpers.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestSearchMemoriesTool|TestGetEntityTimelineTool' ./pkg/tools/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/tools.go pkg/tools/tools.go pkg/tools/memory.go pkg/tools/memory_test.go
git commit -m "feat(tools): add memory search and entity timeline tools"
```

---

### Task 5: Turn integration and mechanical memories

**Files:**
- Modify: `pkg/engine/history.go`, `pkg/engine/timeline.go`, `pkg/engine/orchestrator.go`
- Test: `pkg/engine/memories_test.go`

**Interfaces:**
- Consumes: `harness.TurnSubmission.Memories`, `harness.CheckResult`, `entity.Memory`, `store.SaveMemory`.
- Produces: `Turn.Memories []entity.Memory`; `Timeline.stageMemories(turn *Turn, decls []harness.MemoryDecl) error`; `Timeline.writeMechanicalMemories(turn *Turn, checks []harness.CheckResult) error`.

- [ ] **Step 1: Write the failing test**

```go
func TestTurnWritesDeclaredAndMechanicalMemories(t *testing.T) {
	// Build a submission with one memory declaration and one resolved check,
	// then run ProcessActionStream against a scripted provider that submits both.
	// Assert two memories exist and one is kind "mechanical" linked to the check.
}
```

Use `toolLoopOrchestrator(t, provider)`; assert via `store.ListMemoriesForEntity`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestTurnWritesDeclaredAndMechanicalMemories ./pkg/engine/`
Expected: FAIL.

- [ ] **Step 3: Implement**

- `Turn.Memories []entity.Memory json:"memories,omitempty"`.
- `RecordTurnContext` gains, before the history append: resolve each `MemoryDecl`'s entity refs through `MatchExistingEntity`, `ValidateMemory`, `SaveMemory`, and attach the stored memory to `turn.Memories` (so `history.jsonl` carries it).
- After checks are known, `writeMechanicalMemories` writes one `mechanical` memory per `CheckResult` with `entity_refs = [actor] (+ target)`, `check_id`, mapped importance, and tags `[check_kind, stat]`.
- A memory that fails validation is dropped and logged `memory.dropped`; a storage failure fails the turn.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestTurnWritesDeclaredAndMechanicalMemories ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/history.go pkg/engine/timeline.go pkg/engine/orchestrator.go pkg/engine/memories_test.go
git commit -m "feat(engine): persist declared and mechanical memories per turn"
```

---

### Task 6: Rebuild from history

**Files:**
- Modify: `pkg/engine/timeline.go` (`EnsureIndexed`), `pkg/storage/memory.go`
- Test: `pkg/engine/memories_rebuild_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestEnsureIndexedRebuildsMemories(t *testing.T) {
	// Record a turn with memories, delete the database, run EnsureIndexed,
	// then assert the memories are present again by search.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestEnsureIndexedRebuildsMemories ./pkg/engine/`
Expected: FAIL.

- [ ] **Step 3: Implement**

- `EnsureIndexed`'s replay, when it reprocesses a `Turn`, re-saves `turn.Memories` and re-derives mechanical memories from `turn.Checks` if a `mechanical` memory for that `check_id` is absent.
- Rebuild keys on `(turn, kind, text, check_id)`, since memory ids are volatile.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestEnsureIndexedRebuildsMemories ./pkg/engine/` and `go test ./pkg/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/timeline.go pkg/storage/memory.go pkg/engine/memories_rebuild_test.go
git commit -m "feat(engine): rebuild memories from the turn history"
```

---

### Task 7: Read-only entity timeline in the GUI

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/server.go`, `pkg/gui/service.go`, `frontend/src/types.ts`, `frontend/src/api/client.ts`, `frontend/src/components/Codex.tsx`
- Test: `go test ./pkg/gui/`, `mise run test:frontend`

- [ ] **Step 1: Backend**

Add `GET /api/game/{id}/entity/{entityID}/memories` returning `[]MemoryDTO{Turn, Kind, Text, Importance, Tags}` via `store.ListMemoriesForEntity`. Map the route in `routePattern` and `handleGameRoutes`.

- [ ] **Step 2: Frontend**

Fetch and render a "Memories" timeline in the codex for the selected entity: newest-first, kind + importance, turn link. Read-only.

- [ ] **Step 3: Verify and commit**

```bash
go test ./pkg/gui/ && mise run test:frontend
git add -A
git commit -m "feat(gui): show an entity memory timeline"
```

---

### Task 8: Full verification

- [ ] **Step 1: Run everything**

Run: `mise run test` and `mise run lint`
Expected: PASS.

- [ ] **Step 2: Manual checks**

1. A declared memory is findable by `search_memories` on a distinctive word.
2. One resolved check produces exactly one `mechanical` memory on the actor's timeline.
3. Deleting `cache/index.db` and restarting restores memories from `history.jsonl` (rebuild covers declared + mechanical).
4. A campaign with no memories renders the codex exactly as before.

- [ ] **Step 3: Commit fixups**

```bash
git add -A
git commit -m "test: verify entity memories and memory tools"
```
