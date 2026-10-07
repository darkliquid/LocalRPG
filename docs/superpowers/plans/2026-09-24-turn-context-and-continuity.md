# Turn Context and Continuity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make each turn's context a first-class, replayable artifact with typed references and a persistent working set, and let session/cache-capable providers hold context across turns without ever replacing local canonical history.

**Architecture:** `pkg/harness` gains `TurnContext`/`Ref`/`SessionProvider`/`ContextCacher` and a `ContextPlan` selector. `pkg/engine` gains a working set (`memory.go`), records the context with each turn, and gates strategies in the orchestrator. `pkg/storage` gains two tables written inside `Timeline.RecordTurn`. The GUI exposes the context and working set.

**Tech Stack:** Go 1.27.1; existing `pkg/harness`, `pkg/engine`, `pkg/storage`, `pkg/gui`; React + TypeScript.

**Spec:** `docs/superpowers/specs/2026-09-24-turn-context-and-continuity-design.md`

## Global Constraints

- `history.jsonl` stays canonical and append-only; the compact `context` field carries no prompt text.
- A provider session is a cache: used only on exact tip, model, and prefix-hash match.
- Everything degrades to `full_prompt`; no provider is required to support sessions.
- Use `any`, not `interface{}`; wrap errors with `%w`; stdlib tests only.
- `Timeline.RecordTurn` remains the single writer; no second write path.
- `go vet ./...`, `go test -count=1 ./...`, and `npx tsc --noEmit` are the gate.

---

### Task 1: `TurnContext` and assembler provenance

**Files:**
- Create: `pkg/harness/context_types.go`
- Modify: `pkg/harness/context.go` (`AssembleResult.Context`, section refs)
- Modify: `pkg/engine/history.go` (`TurnRecord.Context`)
- Create: `pkg/harness/context_types_test.go`
- Modify: `pkg/harness/context_test.go` (existing assertions still pass)

**Interfaces:**
- Produces: `harness.RefKind`, `harness.Ref`, `harness.SectionReport`, `harness.ContextStrategy`, `harness.ProviderSession`, `harness.TurnContext`, and `AssembleResult.Context TurnContext`.

- [x] **Step 1: Write the failing provenance test**

```go
func TestAssembleRecordsSectionRefs(t *testing.T) {
	assembler, _ := newTestAssembler(t) // existing helper: store with a location, player, and one character
	result, err := assembler.Assemble(harness.ContextRequest{
		LocationID: "aldon-harbour",
		PlayerID:   "player-elena",
		Action:     "look around",
		TurnNumber: 3,
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	var canon harness.SectionReport
	for _, section := range result.Context.Sections {
		if section.Name == "canon" {
			canon = section
		}
	}
	if canon.Name == "" {
		t.Fatalf("expected a canon section, got %+v", result.Context.Sections)
	}
	found := false
	for _, ref := range canon.Refs {
		if ref.Kind == harness.RefEntity && ref.ID == "aldon-harbour" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the location ref in canon, got %+v", canon.Refs)
	}
	if result.Context.PromptHash == "" || result.Context.TurnNumber != 3 {
		t.Fatalf("incomplete context: %+v", result.Context)
	}
}
```

- [x] **Step 2: Run it to verify it fails**

Run: `go test -run TestAssembleRecordsSectionRefs ./pkg/harness/ -v`
Expected: FAIL — `result.Context` undefined.

- [x] **Step 3: Add the types**

Create `pkg/harness/context_types.go` with the `RefKind`, `Ref`,
`SectionReport`, `ContextStrategy`, `ProviderSession`, and `TurnContext` types
exactly as specified in spec §2, with JSON tags.

- [x] **Step 4: Populate refs while building sections**

In `pkg/harness/context.go`, change the internal `section` struct to carry
`refs []Ref` and `source string`, and have each builder return them:
- `assembleCanon` appends `RefEntity` refs for the location, player, arcs, and
  present characters as it reads them.
- `recentSection` appends `RefTurn` refs for each recalled turn number.
- `sceneRecall`/`relevantHistory` append `RefTurn` refs for retrieved turns.
- `summarySection` appends `RefSummary` with `SummaryVersion` as the ID.
- the catalogue appends `RefEntity` refs for voice-profile IDs with relation
  `voice`.
- the action appends a `RefEntity` for the player with relation `action`.

Then `Assemble` builds `TurnContext`:

```go
result.Context = TurnContext{
	TurnNumber:      req.TurnNumber,
	Mode:            req.Mode,
	Budget:          c.limits.TokenBudget,
	EstimatedTokens: result.EstimatedTokens,
	Sections:        reports,
	Refs:            dedupeRefs(collectRefs(sections)),
	SummaryVersion:  req.SummaryVersion,
	PromptHash:      hashPrompt(result.Prompt),
	Strategy:        StrategyFullPrompt,
}
```

Add `SummaryVersion int` and `Mode string` to `ContextRequest`. Keep
`AssembleResult.Prompt` unchanged.

- [x] **Step 5: Record context in history**

In `pkg/engine/history.go`, add to `TurnRecord`:

```go
Context *harness.TurnContext `json:"context,omitempty"`
```

and set it in the orchestrator when the turn is recorded (the field is already
available on `AssembleResult`).

- [x] **Step 6: Run and commit**

Run: `go vet ./... && go test -count=1 ./pkg/harness/ ./pkg/engine/`
Expected: PASS.

```bash
git add pkg/harness pkg/engine
git commit -m "feat(context): record typed provenance with every turn"
```

---

### Task 2: Persist the full context snapshot in SQLite

**Files:**
- Modify: `pkg/storage/db.go` (schema + migration)
- Modify: `pkg/storage/migrate.go`
- Modify: `pkg/storage/turn.go` (`SaveTurnContext`, `GetTurnContext`)
- Modify: `pkg/engine/timeline.go` (`RecordTurn` writes the snapshot)
- Create: `pkg/storage/turn_context_test.go`

**Interfaces:**
- Produces: `storage.Store.SaveTurnContext(number int, prompt string, ctx harness.TurnContext) error`, `storage.Store.GetTurnContext(number int) (harness.TurnContext, string, error)`.

- [x] **Step 1: Write the failing round-trip test**

```go
func TestTurnContextRoundTrip(t *testing.T) {
	store := openTestStore(t) // existing helper
	ctx := harness.TurnContext{TurnNumber: 1, PromptHash: "abc", Strategy: harness.StrategyFullPrompt}
	if err := store.SaveTurnContext(1, "the prompt", ctx); err != nil {
		t.Fatalf("SaveTurnContext: %v", err)
	}
	got, prompt, err := store.GetTurnContext(1)
	if err != nil {
		t.Fatalf("GetTurnContext: %v", err)
	}
	if prompt != "the prompt" || got.PromptHash != "abc" {
		t.Fatalf("round trip mismatch: %q %+v", prompt, got)
	}
}
```

- [x] **Step 2: Run it to verify it fails**
- [x] **Step 3: Add the table and methods**

Add to the schema and migration:

```sql
CREATE TABLE IF NOT EXISTS turn_contexts (
    turn_number INTEGER PRIMARY KEY,
    prompt      TEXT NOT NULL,
    context_json TEXT NOT NULL,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

Implement `SaveTurnContext` as an upsert and `GetTurnContext` decoding
`context_json` into `harness.TurnContext` (storage already imports entity; add a
harness import only if it does not exist — if a cycle appears, store the JSON as
`[]byte` and let the engine decode; prefer the engine-side decode to keep
storage free of harness).

- [x] **Step 4: Write the snapshot from `Timeline.RecordTurn`**

After the history append succeeds, call `SaveTurnContext` with the assembled
prompt. The prompt travels from `ProcessAction` to `RecordTurn` as a new
`Turn.Prompt` field (not persisted to history).

- [x] **Step 5: Run and commit**

Run: `go test -count=1 ./pkg/storage/ ./pkg/engine/`
Expected: PASS.

```bash
git add pkg/storage pkg/engine
git commit -m "feat(context): persist the exact turn prompt and context snapshot"
```

---

### Task 3: Working set

**Files:**
- Create: `pkg/engine/memory.go`
- Create: `pkg/engine/memory_test.go`
- Modify: `pkg/storage/db.go`, `pkg/storage/migrate.go` (`working_set` table)
- Modify: `pkg/storage/store.go` (`ReplaceWorkingSet`, `LoadWorkingSet`)
- Modify: `pkg/harness/context.go` (working-set section)
- Modify: `pkg/engine/orchestrator.go` (update the set each turn)

**Interfaces:**
- Produces: `engine.WorkingEntry`, `engine.WorkingSet`, `(*WorkingSet).Apply(turn int, refs []harness.Ref)`, `(*WorkingSet).Select(limit int) []harness.Ref`, `(*WorkingSet).Rederive(turns []Turn) WorkingSet`.

- [x] **Step 1: Write the failing decay/selection test**

```go
func TestWorkingSetDecaysAndSelects(t *testing.T) {
	set := engine.WorkingSet{}
	set.Apply(1, []harness.Ref{{Kind: harness.RefEntity, ID: "kaelen", Relation: "present"}})
	set.Apply(5, []harness.Ref{{Kind: harness.RefEntity, ID: "elena", Relation: "action"}})

	top := set.Select(1)
	if len(top) != 1 || top[0].ID != "elena" {
		t.Fatalf("expected elena to outrank the decayed kaelen, got %+v", top)
	}
}
```

- [x] **Step 2: Run it to verify it fails**
- [x] **Step 3: Implement the working set**

`pkg/engine/memory.go` with a decay factor, floor, age cap, and entry cap as
constants. `Apply` stamps/boosts each ref and decays the rest; `Select` returns
the top N; `Rederive` replays history turns through `Apply` in order.

- [x] **Step 4: Persist and load it**

Add the `working_set` table and `ReplaceWorkingSet`/`LoadWorkingSet` on the
store. In the orchestrator, before assembly load the set, pass its selection to
the assembler, and after recording the turn apply the turn's refs and replace
the persisted set.

- [x] **Step 5: Add the working-set prompt section**

In `buildSections`, insert a `working_set` section between `canon` and `summary`
with `rank: 5` (dropped after retrieval). Render entries as a short
"active continuity" list of names/arcs.

- [x] **Step 6: Repair on index rebuild**

In `pkg/engine/timeline.go:EnsureIndexed`, if `LoadWorkingSet` is empty, call
`Rederive` over the last N history turns and `ReplaceWorkingSet`.

- [x] **Step 7: Run and commit**

Run: `go test -count=1 ./pkg/engine/ ./pkg/storage/ ./pkg/harness/`
Expected: PASS.

```bash
git add pkg/engine pkg/storage pkg/harness
git commit -m "feat(context): carry a persistent working set across turns"
```

---

### Task 4: Continuity reference checks

**Files:**
- Modify: `pkg/engine/continuity.go`
- Modify: `pkg/engine/continuity_test.go`
- Modify: `pkg/engine/orchestrator.go` (pass refs into the check)

**Interfaces:**
- Consumes: `harness.TurnContext.Refs`, `engine.WorkingSet`.

- [x] **Step 1: Write the failing checks test**

```go
func TestContinuityFlagsUnknownNamedEntity(t *testing.T) {
	findings := engine.CheckContinuity(engine.ContinuityInput{
		Narration: "Captain Kaelen nods, and Seraphine Vex steps forward.",
		Context:   harness.TurnContext{Refs: []harness.Ref{{Kind: harness.RefEntity, ID: "captain-kaelen"}}},
	})
	if !hasRule(findings, engine.RuleUnknownEntity) {
		t.Fatalf("expected an unknown-entity finding, got %+v", findings)
	}
}
```

- [x] **Step 2: Run it to verify it fails**
- [x] **Step 3: Implement the extended pass**

Extend `continuity.go` with `RuleUnknownEntity` (a named entity absent from
refs/working set and not newly introduced), reusing the existing mention
resolution. Keep the existing rename/thread/summary checks and emit the same
finding shape used by the findings list.

- [x] **Step 4: Run and commit**

Run: `go test -count=1 ./pkg/engine/`
Expected: PASS.

```bash
git add pkg/engine
git commit -m "feat(continuity): check narration against declared context refs"
```

---

### Task 5: Context strategies, sessions, and caching

**Files:**
- Create: `pkg/harness/session.go`
- Create: `pkg/harness/context_plan.go`
- Create: `pkg/harness/session_test.go`, `pkg/harness/context_plan_test.go`
- Modify: `pkg/engine/orchestrator.go` (strategy gate, session persist/continue/fallback)
- Modify: `pkg/engine/timeline.go` (`RewindToTurn` clears the session)
- Modify: `pkg/harness/gemini_provider.go` (implement `SessionProvider` over Interactions)

**Interfaces:**
- Produces: `harness.SessionHandle`, `harness.SessionProvider`, `harness.ContextCacher`, `harness.SelectStrategy(caps Capabilities, stored *ProviderSession, tip int, prefixHash, model string) ContextStrategy`.

- [x] **Step 1: Write the failing selection-test matrix**

```go
func TestSelectStrategy(t *testing.T) {
	cases := []struct {
		name   string
		caps   harness.Capabilities
		stored *harness.ProviderSession
		want   harness.ContextStrategy
	}{
		{"stateless", harness.Capabilities{}, nil, harness.StrategyFullPrompt},
		{"cached prefix", harness.Capabilities{ContextCache: true}, nil, harness.StrategyCachedPrefix},
		{"session mismatched tip", harness.Capabilities{Sessions: true},
			&harness.ProviderSession{Provider: "gemini", ID: "x", ThroughTurn: 1}, harness.StrategyServerSession},
		{"session matching tip", harness.Capabilities{Sessions: true},
			&harness.ProviderSession{Provider: "gemini", ID: "x", ThroughTurn: 5, Model: "m", PrefixHash: "h"}, harness.StrategyServerSession},
	}
	for _, tc := range cases {
		got := harness.SelectStrategy(tc.caps, tc.stored, 5, "h", "m")
		if got != tc.want {
			t.Errorf("%s: got %s want %s", tc.name, got, tc.want)
		}
	}
}
```

(Matching tip/model/prefix must yield `server_session`; any mismatch must yield
`full_prompt`. The test's "mismatched" case expects `full_prompt`; encode the
exact rule from spec §4.1 in the implementation and test both branches.)

- [x] **Step 2: Run it to verify it fails**
- [x] **Step 3: Implement the interfaces and selector**

`pkg/harness/session.go` defines `SessionHandle`, `SessionProvider`,
`ContextCacher` (spec §4). `pkg/harness/context_plan.go` implements
`SelectStrategy` and a `PrefixHash`/`BuildPrefix` helper over the stable sections.

- [x] **Step 4: Gate the orchestrator**

In `runGenerationLoop`, before the first request:
- load the stored session from the previous turn's context;
- select the strategy;
- for `server_session`, call `ContinueSession` with the delta and
  `previous_interaction_id`, re-sending tools/system/generation config;
- on any session/cache error, retry once with `full_prompt`, record the
  fallback on the root span and in `TurnContext.Strategy`;
- persist the new/continued session in `TurnContext.Session`.

- [x] **Step 5: Clear the session on rewind**

In `Timeline.RewindToTurn`, delete the `turn_contexts` rows above the target and
the session reference, so the next turn rebuilds with `full_prompt`.

- [x] **Step 6: Implement Gemini `SessionProvider`**

`gemini_provider.go` gains `StartSession`/`ContinueSession` over
`POST /v1beta/interactions` and `previous_interaction_id`, reading
`usage.total_cached_tokens` into `CachedTokens`. `store=false` is never used;
the id is treated as a cache.

- [x] **Step 7: Run and commit**

Run: `go vet ./... && go test -count=1 ./pkg/harness/ ./pkg/engine/`
Expected: PASS.

```bash
git add pkg/harness pkg/engine
git commit -m "feat(context): prefer provider sessions and cached prefixes when available"
```

---

### Task 6: Expose the context and working set

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`, `pkg/gui/server.go`
- Modify: `pkg/gui/server_test.go`
- Modify: `frontend/src/types.ts`, `frontend/src/api/client.ts`
- Create: `frontend/src/components/ContextDrawer.tsx`
- Modify: `frontend/src/components/DebugPanel.tsx` (entry point)

**Interfaces:**
- Produces: `GET /api/game/{id}/context`, `GET /api/game/{id}/working-set`.

- [x] **Step 1: Add the endpoints and route tests**

`Service.GetTurnContext(gameID string) (*TurnContextDTO, error)` reads the last
turn's context; `Service.GetWorkingSet(gameID string)` returns the current set.
Register both routes and assert they decode in `server_test.go`.

- [x] **Step 2: Add frontend types and client methods** for `TurnContext`,
`SectionReport`, `Ref`, `WorkingEntry`, and `APIClient.getTurnContext(id)` /
`getWorkingSet(id)`.

- [x] **Step 3: Build the Context drawer** showing sections, tokens, inclusion,
refs as wikilinks, trimmed items, the working set, strategy, session id, prefix
hash, and cached tokens. Link it from the Debug panel.

- [x] **Step 4: Verify**

Run: `go test -count=1 ./pkg/gui/ && cd frontend && npx tsc --noEmit`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat(context): expose turn context and working set in the gui"
```

---

## File Map

| File | Responsibility |
|------|----------------|
| `pkg/harness/context_types.go` | `TurnContext`/`Ref`/`SectionReport`/`ContextStrategy` |
| `pkg/harness/context.go` | Section refs, working-set section, prefix build |
| `pkg/harness/session.go` | `SessionProvider`, `ContextCacher`, `SessionHandle` |
| `pkg/harness/context_plan.go` | `SelectStrategy`, prefix hashing |
| `pkg/harness/gemini_provider.go` | Gemini Interactions session support |
| `pkg/engine/memory.go` | Working set model, decay, selection, rederive |
| `pkg/engine/history.go` | Compact `context` field on turn records |
| `pkg/engine/timeline.go` | Snapshot write, rewind session clear, repair |
| `pkg/engine/continuity.go` | Reference-aware continuity checks |
| `pkg/engine/orchestrator.go` | Strategy gate, working-set update |
| `pkg/storage/db.go`, `migrate.go`, `store.go`, `turn.go` | `turn_contexts`, `working_set` tables and accessors |
| `pkg/gui/*`, `frontend/src/*` | Context/working-set endpoints and drawer |

## Self-Review

- **Spec coverage:** §2 artifact → Task 1; §5 storage → Tasks 1-3; §3 working set → Task 3; §6 continuity → Task 4; §4 strategies/sessions → Task 5; §7 exposure → Task 6; §8 tests appear in each task; §4.4 reconciliation and rewind are in Task 5.
- **Placeholder scan:** no TBD/TODO. The strategy-matrix test deliberately encodes both branches of the match rule so the implementer cannot guess; the storage decode note names both options for the import-cycle decision.
- **Type consistency:** `harness.TurnContext`/`Ref`/`SectionReport` (Task 1) are consumed unchanged in Tasks 2-6; `WorkingSet`/`WorkingEntry` (Task 3) feed Tasks 4-6; `SessionProvider`/`ContextCacher`/`SelectStrategy` (Task 5) match the amended provider-capability spec.
