# Turn Context and Continuity Specification

- **Date:** 2026-09-24
- **Status:** Approved (design); spec pending review
- **Scope:** A first-class, replayable turn-context artifact with typed
  references, a persistent working set, and provider-aware context strategies
  (full prompt, explicit cached prefix, server-held session) so turns accumulate
  a coherent timeline rather than executing in isolation.
- **Related:** `pkg/harness/context.go` (`ContextAssembler`, `ContextRequest`,
  `AssembleResult`), `pkg/engine/orchestrator.go`, `pkg/engine/chronicler.go`,
  `pkg/engine/timeline.go`, `pkg/engine/history.go`, `pkg/engine/continuity.go`,
  `pkg/storage/turn.go`, `pkg/storage/store.go`,
  `docs/superpowers/specs/2026-09-24-provider-capability-model-design.md`,
  `docs/superpowers/specs/2026-09-24-opentelemetry-instrumentation-design.md`.

---

## 1. Overview & Goals

Each turn assembles a ten-section prompt from the index, the recent window, a
lossy summary, and FTS retrieval, then trims it to a token budget. The resulting
coherence depends on heuristics firing well: nothing records which entities,
turns, threads, or summary version a reply actually leaned on, no working set
survives between turns, and providers that can hold a conversation server-side
are driven as if they were stateless.

This specification makes context a first-class artifact.

### 1.1 Goals

1. **Typed provenance.** Every section declares the references it read; the turn
   records the deduped union so a reply's dependencies are inspectable.
2. **Replayable context.** The exact prompt and its provenance persist per turn.
3. **Persistent working set.** Recently active entities and open threads carry
   across turns with decayed weight, so continuity does not hinge on retrieval.
4. **Provider-aware strategies.** Prefer a server-held session or a cached prefix
   when the provider supports it; fall back deterministically to a full prompt.
5. **Local canon stays canonical.** Server sessions are a cache; `history.jsonl`
   remains the append-only truth and `/undo` invalidates any chain.
6. **Observable.** Strategy, refs, trimming, and cached-token usage are traced and
   exposed to the GUI.

### 1.2 Non-Goals

1. Event-sourcing the narrative or replacing the summary/chronicle model.
2. Changing provider prompts beyond adding the working-set section and the
   strategy-specific framing.
3. Replacing FTS retrieval; the working set biases it, it does not remove it.
4. Requiring any provider to support sessions; everything degrades to the current
   full-prompt path.

### 1.3 Success Criteria

- A turn's `TurnContext` lists every section with its tokens, inclusion, and refs,
  and the refs resolve to real entities/turns/threads.
- The working set is deterministic, bounded, persisted, and rederived correctly
  after index loss.
- With a session-capable provider, a multi-turn conversation chains via
  `previous_interaction_id`; a rewind or an expired session rebuilds from local
  history without losing a turn.
- With a stateless provider, behaviour is byte-for-byte the current full prompt.
- `go vet ./...` and `go test ./...` remain the gate.

---

## 2. The `TurnContext` Artifact

Lives in `pkg/harness` alongside the assembler.

```go
type RefKind string

const (
	RefEntity   RefKind = "entity"
	RefTurn     RefKind = "turn"
	RefSummary  RefKind = "summary"
	RefThread   RefKind = "thread"
	RefWorld    RefKind = "world"
	RefSystem   RefKind = "system"
	RefLocation RefKind = "location"
	RefSession  RefKind = "session"
)

// Ref is one thing a section read. Relation names why: present, mention,
// retrieved, arc, voice, recall, action.
type Ref struct {
	Kind     RefKind `json:"kind"`
	ID       string  `json:"id"`
	Relation string  `json:"relation,omitempty"`
}

// SectionReport describes one prompt section and what it referenced.
type SectionReport struct {
	Name     string `json:"name"`
	Tokens   int    `json:"tokens"`
	Included bool   `json:"included"`
	Source   string `json:"source,omitempty"`
	Refs     []Ref  `json:"refs,omitempty"`
}

// ContextStrategy is how the assembled context reaches the provider.
type ContextStrategy string

const (
	StrategyFullPrompt   ContextStrategy = "full_prompt"
	StrategyCachedPrefix ContextStrategy = "cached_prefix"
	StrategyServerSession ContextStrategy = "server_session"
)

// ProviderSession identifies a server-held conversation. It is a cache keyed by
// the local tip it covers, never the source of truth.
type ProviderSession struct {
	Provider    string `json:"provider"`
	ID          string `json:"id"`
	ThroughTurn int    `json:"through_turn"`
	Model       string `json:"model,omitempty"`
	PrefixHash  string `json:"prefix_hash,omitempty"`
}

// TurnContext is the durable description of one turn's context.
type TurnContext struct {
	TurnNumber      int               `json:"turn_number"`
	Mode            string            `json:"mode"`
	Budget          int               `json:"budget"`
	EstimatedTokens int               `json:"estimated_tokens"`
	Sections        []SectionReport   `json:"sections"`
	Refs            []Ref             `json:"refs"`
	WorkingSet      []Ref             `json:"working_set"`
	Threads         []string          `json:"threads,omitempty"`
	SummaryVersion  int               `json:"summary_version"`
	WorldHash       string            `json:"world_hash,omitempty"`
	SystemHash      string            `json:"system_hash,omitempty"`
	PromptHash      string            `json:"prompt_hash"`
	Strategy        ContextStrategy   `json:"strategy"`
	PrefixHash      string            `json:"prefix_hash,omitempty"`
	Session         *ProviderSession  `json:"session,omitempty"`
	CachedTokens    int               `json:"cached_tokens,omitempty"`
}
```

`AssembleResult` gains `Context TurnContext` while retaining `Prompt`, so every
existing caller and test compiles unchanged.

---

## 3. Working Set

A bounded, weighted set of active entities and threads persisted per campaign and
maintained in `pkg/engine/memory.go`.

```go
type WorkingEntry struct {
	Kind     RefKind
	ID       string
	Weight   float64
	LastTurn int
	Role     string
}

type WorkingSet struct {
	Entries []WorkingEntry
}
```

Rules (deterministic; no model involved):

1. **On each turn**: stamp every `RefEntity`/`RefThread` from `TurnContext.Refs`
   as `LastTurn = turn`, and raise its weight by a fixed increment.
2. **Decay**: every other entry's weight is multiplied by a decay factor per
   elapsed turn; entries below a floor, or past an age cap, are evicted.
3. **Cap**: the set is capped at N entries; ties break by `LastTurn` then ID.
4. **Role**: `present` for entities in the canon scene, `mentioned` for
   narration mentions, `thread` for arcs, `extracted` for new entities.
5. **Selection for the prompt**: the top entries within a token allowance form
   the working-set section, ranked so it is dropped only after retrieval.
6. **Repair**: on `EnsureIndexed`, if the persisted set is missing, rederive it
   from the last N history turns using the same rules.

The working set also seeds recall and retrieval: retrieval dedupes against it, so
a recently-referenced entity is never crowded out by an FTS neighbour.

---

## 4. Context Strategies and Provider Sessions

### 4.1 Selection

`pkg/harness/context_plan.go` decides, in order:

1. `server_session` when the provider implements `SessionProvider` **and** a
   stored session matches the current tip, model, and prefix hash.
2. `cached_prefix` when the provider implements `ContextCacher` and the stable
   prefix is cacheable.
3. `full_prompt` otherwise.

The selection is deterministic from `(capabilities, stored state, prefix hash)`
and is recorded in `TurnContext.Strategy`.

### 4.2 Session lifecycle

- **Start**: with no valid session, assemble a full prompt, call
  `StartSession`, and persist `ProviderSession{provider, id, through_turn, model,
  prefix_hash}` with the turn.
- **Continue**: with a valid session, send only the per-turn delta (scene,
  working set, recent, action) plus `previous_interaction_id`. Because
  server-side tools, system instruction, and generation config are
  interaction-scoped, they are re-sent every turn.
- **Invalidate**: `/undo` (`RewindToTurn`) clears the session; the next turn
  rebuilds via `full_prompt` and starts a fresh chain. An expired or missing
  session (retention, deleted) is detected by the provider error and handled the
  same way.
- **Fallback**: a session or cache failure never loses a turn; the turn retries
  once with `full_prompt`, records the fallback, and starts a new session/cache.

### 4.3 Cached-prefix lifecycle

- Prefix = rules + lore + voice catalogue + world arcs. `PrefixHash` is a hash of
  the rendered prefix.
- `cached_prefix` calls `EnsureCache(prefix, ttl)` and sends the suffix each
  turn. The cache is recreated when `PrefixHash` changes or TTL expires.
- Gemini's Interactions surface relies on **implicit** caching through chaining
  (it has no explicit `cached_content`); explicit caching is used only for
  `generateContent`-style providers that implement `ContextCacher`.

### 4.4 Reconciliation with local canon

`history.jsonl` remains canonical. A session is used only when:
`through_turn == current tip` **and** `prefix_hash == current PrefixHash`
**and** `model` matches. Any mismatch discards the session. This makes a session
purely a cache that may be rebuilt at any time without changing the narrative.

### 4.5 Telemetry

`TurnContext.CachedTokens` is taken from provider usage (Interactions
`usage.total_cached_tokens`), stored with the turn, and exported as
`localrpg.provider.cached_tokens`. `Strategy`, `Session`, and `PrefixHash` are
span attributes (spec #1).

---

## 5. Storage

### 5.1 `history.jsonl` (canonical, compact)

Each turn record gains:

```json
"context": {
  "prompt_hash": "...", "budget": 8000, "estimated_tokens": 1234,
  "strategy": "server_session", "prefix_hash": "...", "cached_tokens": 900,
  "summary_version": 12,
  "sections": [{"name": "canon", "tokens": 300, "included": true, "refs": 4}],
  "refs": [{"kind": "entity", "id": "captain-kaelen", "relation": "present"}],
  "working_set": [{"kind": "entity", "id": "captain-kaelen"}],
  "session": {"provider": "gemini", "id": "v1_...", "through_turn": 42}
}
```

No prompt text is stored in the log, so it stays small; refs and hashes are
enough to know exactly what the turn leaned on.

### 5.2 SQLite (disposable)

- `turn_contexts(turn_number INTEGER PRIMARY KEY, prompt TEXT NOT NULL, context_json TEXT NOT NULL, created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`.
- `working_set(entity_id TEXT PRIMARY KEY, kind TEXT NOT NULL, weight REAL NOT NULL, last_turn INTEGER NOT NULL, role TEXT)`.
- Both ship in the schema and in `migrate`, and both are written by
  `Timeline.RecordTurn` inside the existing single-writer path.

### 5.3 Rebuildability

- `working_set` is rederived from the last N history turns when the database is
  rebuilt, so losing `index.db` costs at most the current set's precision.
- `turn_contexts` is exact where present and absent after a rebuild; replay then
  reconstructs the prompt from the compact history provenance and current
  entities, and marks the reconstruction in the trace.

---

## 6. Reference Model and Continuity

### 6.1 Sources declare refs

Each of the ten sections declares the refs it read. The assembler dedupes and
records the union in `TurnContext.Refs`, and each section's own refs in
`SectionReport.Refs`.

### 6.2 Continuity checks

`pkg/engine/continuity.go` extends its deterministic pass to assert:

1. Every entity named in the narration is either in canon/working set or is
   recorded as newly introduced.
2. An established name is never renamed.
3. Every unresolved thread appears in the prompt (existing behaviour).
4. The summary used does not contradict canon (existing deterministic pass).

Findings continue to feed `localrpg.continuity.findings` and the GUI findings
list; the new checks add `localrpg.continuity.rule` values.

---

## 7. Exposure

- `GET /api/game/{id}/context` returns the last turn's `TurnContext`.
- `GET /api/game/{id}/working-set` returns the current working set.
- A **Context drawer** in the GUI shows sections, tokens, inclusion, refs (as
  wikilinks), trimmed items, the working set, the strategy, session id, prefix
  hash, and cached tokens.
- The existing Debug panel links to the drawer.

---

## 8. Testing

1. Assembler tests asserting each section's refs for a scripted store.
2. Working-set decay, eviction, cap, and selection tests.
3. A replay test proving the working set rederives identically from history.
4. Strategy-selection matrix tests: session-capable, cache-capable, stateless.
5. Session-reuse tests: only on exact tip/prefix/model match; rewind clears it;
   expired session falls back and starts fresh.
6. Cached-prefix tests: recreation on prefix-hash change.
7. Budget tests proving trimming still holds with the working-set section.
8. Endpoint and `tsc --noEmit` gates.

---

## 9. Rollout

One plan, six tasks:

1. `TurnContext` types + assembler provenance + compact history record.
2. `turn_contexts` migration and read path.
3. Working set model, persistence, repair, and recall/retrieval integration.
4. Continuity reference checks.
5. Context strategies (`SessionProvider`/`ContextCacher`, selection, fallback,
   rewind invalidation).
6. GUI endpoints + Context drawer.

---

## 10. Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Server retention expiry silently breaking sessions | Treat the session as a cache; fall back to `full_prompt` and start fresh; log the id and created turn. |
| Two sources of history (local vs server) | Local canon is authoritative; a session is used only on exact tip/prefix/model match. |
| Prompt-behaviour change from the working set | Bounded, ranked, token-allowanced; budget tests; falls back to current ordering. |
| Index loss losing the working set | Rederived from the last N turns on `EnsureIndexed`. |
| Provider variance in session semantics | Capability-gated; `full_prompt` is always valid; fallback is one retry. |
| `history.jsonl` growth | Only compact provenance is stored; prompt text stays in SQLite and trace. |
| Cache invalidation bugs | Prefix hash check before reuse; a changed prefix always recreates. |
