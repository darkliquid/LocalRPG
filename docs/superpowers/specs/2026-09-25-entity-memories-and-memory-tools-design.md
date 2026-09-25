# Entity Memories & Memory Tools Design

**Date:** 2026-09-25
**Status:** Proposed
**Scope:** Per-entity memory records, storage and full-text index, mechanical-memory derivation, retrieval tools, entity timeline
**Related:** Structured Turn Protocol Design (2026-09-25), Mechanics Engagement & Declarative Schema Design (2026-09-25), `pkg/entity`, `pkg/storage`, `pkg/tools`, `pkg/engine/memory.go`

## 1. Overview & Goals

Entities currently carry a body of prose, a `history []int` of turn numbers, and
wikilink edges. There is no searchable record of *what happened* to a character,
location, faction, or item: recall means grepping turn narration via
`search_timeline`. A GM enriching its context before a turn has no way to ask
"what does this character remember" or "what happened here".

This specification adds first-class memory records attached to entities, written
by the GM as part of the structured turn and derived by the engine for mechanical
events, stored and indexed in SQLite, and reached through retrieval tools. The
GM's per-entity "key events" timeline becomes a query over memories rather than
something reconstructed from turn prose.

**Goals:**

- A memory record: `{turn, kind, entity_refs[], text, importance 1-5, tags[],
  source, check_id?}`.
- Shared `event` memories (one record, many participants) and entity-scoped
  memories (`relationship`, `discovery`, `dialogue`).
- Engine-authored `mechanical` memories, one per resolved check, linked to the
  check and its actor/target; the GM cannot author these.
- Storage in a new `memories` table with an FTS index, indexed alongside the
  existing entity and turn indexes.
- Retrieval tools: `search_memories` and `get_entity_timeline` (plus the existing
  four query tools).
- Ranking by FTS relevance × importance × recency decay; no automatic prompt
  injection — memories reach the GM only through tools.
- No eviction from storage; any cap applies only to a tool result or an injected
  excerpt.

**Non-Goals:**

- Embedding-based semantic search (FTS5 only for now).
- A memory-editing GUI beyond the read-only entity timeline view.
- Migrating existing turn prose into memories; memories begin empty.
- Auto-injection of memories into the context prompt.
- Changing `entity.History` or `turn_entities`, which remain the turn link.

**Success Criteria:**

- A `submit_turn` memory declaration is persisted with its entity links and is
  findable via `search_memories` on a distinctive word.
- A resolved check writes exactly one `mechanical` memory linking the check, its
  actor, and its target, and it appears in `get_entity_timeline` for the actor.
- Old campaigns with no memories behave exactly as today.
- The SQLite `cache/index.db` remains disposable: dropping it and resyncing
  rebuilds entity/turn data, and memories are documented as non-rebuildable from
  Markdown (they are canonical in the database, like turn history).

## 2. Investigation Findings

- Existing primitives: `entity.History []int` (`pkg/entity/entity.go:38`),
  `turn_entities` with `mention` and per-turn `outcome` (`pkg/storage/db.go:42`),
  `turn_contexts` (`:52`), `working_set` (`:59`, decay `0.8^dt` in
  `pkg/engine/memory.go:39`).
- FTS5 indexes are `entities_fts(name, body, tags)` and
  `turns_fts(input, narration)`, porter tokenizer, kept by triggers
  (`pkg/storage/fts.go:14-52`); `EnsureFTS` backfills idempotently (`:58`).
- Query tools are read-only and dispatched by name in `pkg/tools/tools.go:38`;
  `BuildMatch` sanitises a query into a safe FTS5 MATCH (`pkg/tools/query.go:10`).
- Migrations are versioned via `PRAGMA user_version` (`pkg/storage/migrate.go:8`).
- `history.jsonl` is canonical for turns; the DB is rebuildable from it. Memories
  are new state that is *not* in `history.jsonl` unless embedded in the `Turn`
  record (see §7).

## 3. Data Model

```go
// pkg/entity/memory.go
type MemorySource string // "gm" | "engine"

type Memory struct {
    ID         int64    `json:"id"`
    Turn       int      `json:"turn"`
    Kind       string   `json:"kind"`          // event|relationship|discovery|dialogue|mechanical
    EntityRefs []string `json:"entity_refs"`
    Text       string   `json:"text"`
    Importance int      `json:"importance"`    // 1-5
    Tags       []string `json:"tags,omitempty"`
    Source     MemorySource `json:"source"`
    CheckID    string   `json:"check_id,omitempty"`
    CreatedAt  time.Time `json:"created_at"`
}
```

Kinds:

- `event` — a thing that happened; `entity_refs` names everyone present.
- `relationship` — how one entity now regards another; `entity_refs` = subject(s).
- `discovery` — something learned; `entity_refs` names who learned it.
- `dialogue` — a line or exchange worth remembering; `entity_refs` names the
  speakers.
- `mechanical` — engine-only, one per resolved check; `entity_refs` = `[actor]`
  plus `target` when present; `check_id` links the `CheckResult`.

Importance is a 1–5 scale the GM assigns; the engine derives it for mechanical
memories from the check's stakes (configurable mapping, default 3).

## 4. Storage

### 4.1 Schema (migration v5)

```sql
CREATE TABLE IF NOT EXISTS memories (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    turn        INTEGER NOT NULL,
    kind        TEXT NOT NULL,
    text        TEXT NOT NULL,
    importance  INTEGER NOT NULL DEFAULT 3,
    source      TEXT NOT NULL,
    check_id    TEXT,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS memory_entities (
    memory_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    PRIMARY KEY (memory_id, entity_id)
);
CREATE INDEX IF NOT EXISTS idx_memory_entities_entity ON memory_entities(entity_id);
CREATE INDEX IF NOT EXISTS idx_memories_turn ON memories(turn);
CREATE TABLE IF NOT EXISTS memory_tags (
    memory_id INTEGER NOT NULL,
    tag       TEXT NOT NULL,
    PRIMARY KEY (memory_id, tag)
);

CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(text, tags, tokenize='porter');
```

Triggers keep `memories_fts` in sync on insert/update/delete, joining `memory_tags`
for the tags column, mirroring `entities_fts`.

### 4.2 Store methods

- `SaveMemory(m *entity.Memory) (int64, error)` — inserts the memory, its entity
  links, its tags, and the FTS row in one transaction.
- `ListMemoriesForEntity(entityID string, limit int) ([]entity.Memory, error)` —
  the entity timeline, newest first, ordered by turn then id.
- `SearchMemories(match, entityID, kind string, minImportance, limit int)
  ([]MemoryHit, error)` — FTS `MATCH` with optional entity/kind/importance filters.
- `MemoryHit{ID, Turn, Kind, Snippet, Importance}`.

### 4.3 Canonicality and rebuild

`entity.History` and `turn_entities` are rebuildable from Markdown and
`history.jsonl`. Memories are not rebuildable from Markdown; they are canonical in
`cache/index.db` for the same reason `turn_contexts` is. To survive a lost
`index.db`, every accepted turn's declarations are embedded in the `Turn` record
(§7), so a rebuild can re-derive memories from `history.jsonl`. `EnsureIndexed`
runs that replay.

## 5. Retrieval Tools

Two new tools, added to `harness.ToolSpecs()` and `pkg/tools` dispatch:

- **`search_memories`** — `{query, entity?, kind?, min_importance?, limit=10}`.
  Returns `- turn N (kind, importance): snippet`. No query and no entity is an
  error, matching `search_entities`.
- **`get_entity_timeline`** — `{entity, limit=20}`. Returns the entity's memories
  ordered newest-first with turn, kind, importance, and text (capped by the
  existing tool result cap).

Ranking for `search_memories`: SQL returns FTS `bm25` hits; the Go layer re-ranks
by `bm25_score × importance × recencyFactor`, where
`recencyFactor = 0.5 ^ (currentTurn - memory.Turn) / halfLife` with a configurable
half-life (default 20 turns), reusing the working-set decay idea.

No memory is auto-injected into the context prompt; the GM retrieves what it needs
with these tools. Tool results are bounded by `ToolResultChars` as today.

## 6. Derivation of Mechanical Memories

When the engine resolves a check (spec 3), it writes one `mechanical` memory:

```
kind: mechanical
turn: <turn>
entity_refs: [actor] (+ target if present)
text: "Rolled <notation> for <stat/skill> against <difficulty>: <total>, <outcome>."
importance: <mapped from stakes>
tags: [<check_kind>, <stat/skill>]
source: engine
check_id: <check id>
```

This is written by `Timeline` in the same transaction region as the turn record,
after segment/personae staging and before/with the history append, so a record
never points at a missing memory. The GM may attach narrative memories in the
same turn; the engine writes exactly one mechanical memory per resolved check.

## 7. Turn Integration

The accepted `TurnSubmission` is embedded in the `Turn` record so `history.jsonl`
stays the canonical timeline:

```go
type Turn struct {
    // ...existing...
    Memories   []entity.Memory `json:"memories,omitempty"`   // accepted declarations (ids assigned on index)
    Checks     []harness.CheckResult `json:"checks,omitempty"`
    Verdict    *harness.ActionVerdict `json:"verdict,omitempty"`
    Rejected   bool `json:"rejected,omitempty"`
    Personae   []string `json:"personae,omitempty"`
}
```

`Timeline.RecordTurnContext` gains a memory step: stage declarations, attach
entity links resolved through `MatchExistingEntity` (spec 1), write memories, then
append the turn and index. `EnsureIndexed` replays `Turn.Memories` and the
engine-written mechanical memories into the tables when the database disagrees
with `history.jsonl`.

## 8. Frontend

- The entity/codex view gains a "Memories" timeline: `get_entity_timeline` rendered
  newest-first with kind, importance, and turn link. Read-only.
- Memory search is available to the GM through tools only; the GUI exposes a
  read-only search box in the codex if cheap, otherwise it is a follow-up.
- No chronicle change (spec 1 owns segment rendering).

## 9. Error Handling

- A memory with empty text or no `entity_refs` is dropped and logged
  (`memory.dropped`), never failing the turn.
- A memory referencing an entity that cannot be resolved is kept with the raw ref
  and flagged (`memory.unresolved_ref`); resolution is best-effort, matching the
  extractor's tolerance.
- A storage failure while writing memories fails the turn the same way a turn
  record failure does, because the turn and its memories must stay consistent.

## 10. Testing

- `SaveMemory` round-trips entity links, tags, and FTS findability by a
  distinctive word.
- `ListMemoriesForEntity` orders by turn then id and honours the limit.
- `SearchMemories` filters by entity, kind, and minimum importance.
- Ranking test: a higher-importance recent memory outranks a lower-importance old
  one for the same query.
- Engine test: resolving one check writes exactly one mechanical memory with the
  actor linked; two checks write two.
- `EnsureIndexed` rebuild from `history.jsonl` recreates memories and mechanical
  memories after the database is deleted.
- A memory with an empty body is dropped without failing the turn.

## 11. Compatibility & Migration

- Migration v5 is additive; existing campaigns get empty memory tables.
- The index remains disposable; a lost database replays memories from
  `history.jsonl`.
- No change to `entity.History` or `turn_entities`; they stay the turn link.

## 12. Open Questions

- Should `dialogue` memories be synthesised automatically from speech segments, or
  only when the GM declares them? (Current proposal: GM declares; speech already
  lives in the turn segments.)
- Is a stable memory id needed in `history.jsonl`, or is `(turn, kind, text)` a
  sufficient key for the rebuild? (Current proposal: volatile ids, rebuild matches
  on turn + kind + text.)
