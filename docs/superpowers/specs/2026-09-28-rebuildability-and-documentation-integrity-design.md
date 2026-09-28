# Rebuildability Invariant & Documentation Integrity Design

**Date:** 2026-09-28
**Status:** Proposed
**Scope:** Prove the "index is disposable, Markdown and history are canonical" invariant with a full delete-and-replay equivalence test, and remove the documentation drift that points contributors at symbols and behaviours that no longer exist
**Related:** `pkg/engine` (`timeline.go`, `history.go`), `pkg/storage` (`sync.go`, `store.go`), `pkg/gui` (`service.go` `ensureIndexed`), `AGENTS.md`, `docs/debugging.md`, `docs/architecture/review-2026-09-24.md`; implements finding P1.6 of `docs/architecture/review-2026-09-24.md` and complements `docs/superpowers/specs/2026-09-21-canonical-db-and-turn-timeline-design.md`

## 1. Overview & Goals

The architecture rests on one promise: `games/<id>/cache/index.db` is
disposable and rebuildable from `entities/*.md` plus `history.jsonl`, and
`storage.OpenGameStore` plus `Timeline.EnsureIndexed` restore it. The review
flagged this as the invariant most likely to be broken silently by the P0 work
and asked for a test that mutates a campaign, deletes the index, replays
history, and asserts the index and working set match
(`docs/architecture/review-2026-09-24.md` finding P1.6).

Today the coverage is partial: `TestTimelineEnsureIndexedReplaysHistory`
(`pkg/engine/timeline_test.go:17`) writes two synthetic log lines and checks turn
counts and a couple of entity mappings, with a prune case; and
`TestEnsureIndexedRederivesWorkingSetWhenEmpty` (`:209`) covers the working set.
Neither exercises a real turn pipeline (entity notes, memories, checks,
advancement), a deleted `index.db`, or full derived-state equivalence.

Separately, contributor-facing docs point at removed code. `AGENTS.md:50` and
`docs/debugging.md:191` both name `harness.AssembleContextWithProfiles`, which no
longer exists; the current entry point is `ContextAssembler.Assemble(ContextRequest)`
(`pkg/harness/context.go:145,173`). Other still-referenced documents describe
video export as a silent still renderer, which the shipped pipeline is not.

**Goals:**

- One test that a full campaign survives deleting `index.db` and rebuilding, with
  derived state equal before and after.
- Cover the paths that make the invariant non-trivial: entity wikilinks and
  frontmatter edges, turn segments and mentions, memories, checks, advancement,
  and (once shipped) health effects and world ticks.
- Cover idempotence (rebuilding twice is a no-op) and `/undo` (a rewound log
  rebuilds to a trimmed index).
- Remove the removed-symbol references from live docs and guard against their
  return with a cheap test.

**Non-Goals:**

- Changing the persistence model or the index schema.
- Migrating or validating legacy `game.db` beyond the existing retirement path.
- Generating the route/DTO manifest (recorded as an optional phase in §3.5).
- Rewriting historical, dated specs and plans; those are records, not docs.

**Success Criteria:**

- A test fails if `EnsureIndexed` stops replaying any derived table, or if a
  derived row no longer matches the canonical source.
- `grep AssembleContextWithProfiles AGENTS.md docs/debugging.md` returns nothing.
- A test fails if a live doc references a symbol on the removed-symbol denylist.
- The two still-referenced documents that call video export a silent renderer are
  corrected; the dated plan gets an erratum note.

## 2. Investigation Findings

- **Single writer and rebuild path.** `Timeline.RecordTurn` stages entity notes
  (`pkg/engine/timeline.go:103`), mechanical memories (`:113`), appends to
  `history.jsonl` (`:118`), saves turn context (`:130`), then indexes the turn
  (`:136`). `EnsureIndexed` (`:402`) loads history, compares `CountTurns()` and
  `MaxTurnNumber()`, replays via `indexTurns` when they disagree (`:417-421`),
  re-derives turn memories (`:425-431`) and the working set when empty
  (`:433-445`). `SyncTurns` (`:389`) is the explicit replay entry point.
  `indexTurns` prunes index rows above the log's highest via `DeleteTurnsFrom`
  (`:460-476`).
- **Sync is file-hash based.** `Syncer.Sync(dir)` parses every `*.md` and upserts
  new/changed entities by hash (`pkg/storage/sync.go:26`); `SyncFile` does one
  file (`:81`). The GUI repair path `Service.ensureIndexed` (`service.go:224`)
  runs once per process per campaign: `Sync(entities)` (`:239`), build a
  `Timeline` (`:241`), `EnsureIndexed()` (`:246`), then enqueue embeddings
  (`:247-258`).
- **Existing tests.** `pkg/engine/timeline_test.go:17` (two hand-written log
  lines, counts and mappings, prune on truncate) and `:209` (working set
  re-derivation). No test deletes `index.db`, and none drives a real turn.
- **Doc drift, live.** `AGENTS.md:50` says *"Context prompt layering lives in
  `pkg/harness/context.go:AssembleContextWithProfiles`"*; `docs/debugging.md:191`
  says *"the prompt assembled by `harness.AssembleContextWithProfiles`"*. Neither
  symbol exists. The current API is `type ContextAssembler` (`context.go:145`)
  with `func (c *ContextAssembler) Assemble(req ContextRequest) (AssembleResult, error)`
  (`:173`), called as `o.assembler.Assemble(...)` (`orchestrator.go:712`).
- **Doc drift, video.** `docs/superpowers/specs/2026-09-21-canonical-db-and-turn-timeline-design.md:392`
  and its plan (`:3959,4455`) still describe `pkg/export/video.go` as a silent
  still-image renderer; `video.go:137-150` now interleaves real audio.
- **Historical references.** Many dated plans/specs mention
  `AssembleContextWithProfiles` (e.g.
  `plans/2026-09-23-voice-speech-cues-and-steering.md:438`); these are records of
  what was true then and should not be rewritten.
- **Manual route/DTO mirroring.** `pkg/gui/server.go` registers routes
  (`:91-108`) and names spans in `routePattern` (`:50-82`);
  `frontend/src/api/client.ts` and `frontend/src/types.ts` mirror them by hand.
  The review lists a generated manifest as P2.8.

## 3. Design

### 3.1 Define the invariant precisely

- **Canonical, must never be lost:** `games/<id>/entities/*.md`,
  `games/<id>/history.jsonl`, `game.yaml`; indirectly `system.yaml`/`world.yaml`.
- **Derived, must be reconstructible:** `cache/index.db` — entities, edges,
  `turns`, `turn_entities`, memories, working set, and embeddings.
- **Invariant:** for any campaign, deleting `cache/index.db`, reopening via
  `storage.OpenGameStore`, running `Syncer.Sync(entities)` and
  `Timeline.EnsureIndexed()`, yields derived state equal to the pre-deletion
  state (embeddings excepted, which are recomputed asynchronously).

### 3.2 The delete-and-replay equivalence test

Add `pkg/engine/rebuild_invariant_test.go`:

1. Build a campaign through the real pipeline with `t.TempDir()`:
   - three or four turns via `Timeline.RecordTurn`, including narration and
     speech segments, entity mentions, and at least one check result;
   - entity notes whose bodies contain `[[wikilinks]]` and whose frontmatter
     carries `location`/`faction`, so edges are non-trivial;
   - a memory and an advancement event;
   - once the mechanics spec lands, a turn with `HealthEffects` and `WorldTick`.
2. Snapshot derived state through `storage.Store` queries: entity rows and
   hashes, edges, `turns`, `turn_entities`, memories, and the working set.
3. Close the store, delete `cache/index.db` (including any `-wal`/`-shm`), reopen
   via `storage.OpenGameStore`, run `Syncer.Sync(entities)` and
   `Timeline.EnsureIndexed()`, and re-snapshot.
4. Assert deep equality (order-normalised for query results).
5. Assert **idempotence**: a second `EnsureIndexed()` changes nothing.
6. Assert **undo**: after `Timeline.RewindToTurn(n)`, deleting and rebuilding the
   index yields the index matching the trimmed log.

Also add a GUI-level test that `Service.ensureIndexed` performs the same repair
for a campaign whose `index.db` was deleted before the service first served it,
and that the legacy `game.db` → `game.db.legacy` retirement still applies.

The test must fail loudly if a future writer stops indexing a table: compare
against a **structural** list of tables/columns, not only the rows, so a new
derived table added to `index.db` and forgotten in `EnsureIndexed` is caught.

### 3.3 Keep the invariant honest

- `EnsureIndexed` should report what it repaired (counts per table) so the test
  and the logs can assert it did work, and a no-op rebuild is distinguishable.
- Where a derived table has no canonical counterpart (e.g. embeddings), the test
  documents that and asserts it is queued for recomputation rather than left
  stale.

### 3.4 Documentation integrity

- Fix `AGENTS.md:50` and `docs/debugging.md:191` to name
  `ContextAssembler.Assemble(ContextRequest)` and describe the sections as they
  are (`pkg/harness/context.go:263-277`).
- Correct the still-referenced video claims in
  `2026-09-21-canonical-db-and-turn-timeline-design.md` (and add a dated erratum
  to its plan rather than rewriting the historical checkboxes).
- Add a small guard test, `pkg/gui/docs_symbols_test.go` or a `scripts/` task,
  that scans **live** docs only (`AGENTS.md`, `README.md`, `docs/debugging.md`,
  `docs/architecture/*.md`, `pkg/gui/docs/*.md`) for a denylist of removed or
  renamed exported symbols (`AssembleContextWithProfiles`, and any others the
  test seeds) and fails with the offending file and line. Dated
  `docs/superpowers/**` files are exempt.
- Run the existing docs regeneration after any provider/config change:
  `go test ./pkg/gui -update-docs`.

### 3.5 Optional phase — route/DTO drift guard

The review's P2.8 asks for a generated route/DTO manifest so `server.go`,
`types.ts`, and `client.ts` cannot drift. A lightweight version: emit the route
table from `server.go` into a checked-in manifest and assert every route has a
`client.ts` method and every DTO referenced in `types.ts`. This is a larger change
and is recorded here as an optional follow-on, not part of the invariant fix.

## 4. Interfaces

No production API changes are required. `EnsureIndexed` may additionally return
or log repair counts (a struct, not a signature-breaking change). The docs guard
is a test, not an interface.

## 5. Error Handling

| Situation | Behaviour |
| --- | --- |
| `index.db` missing entirely | `OpenGameStore` creates it; `Sync` + `EnsureIndexed` repopulate (today's behaviour, now tested) |
| `index.db` corrupt | Same as missing once deleted; the test covers deletion, not corruption (a corrupt-DB test is optional) |
| `history.jsonl` malformed line | `LoadHistory` behaviour is unchanged; the invariant assumes a well-formed log |
| Entity file malformed frontmatter | `Sync` skips it (existing); the test asserts the skip is stable, not silent data loss in the derived tables for the files that did parse |
| Legacy `game.db` present | Retired to `game.db.legacy` as today; assert it in the test |
| Live doc references a removed symbol | docs guard fails with file:line |

## 6. Testing & Verification

- The new `rebuild_invariant_test.go` cases above (delete/replay equivalence,
  idempotence, undo, structural table coverage, GUI repair, legacy retirement).
- `pkg/gui/docs_symbols_test.go` (or the scripts task) fails on a seeded
  removed-symbol reference and passes on the live docs.
- `mise run lint` (`markdownlint` + `go vet`) stays clean; `mise run test`
  (`go test ./...` and `tsc --noEmit`) stays green.

## 7. Compatibility & Rollout

- Tests only; no runtime behaviour changes, so the risk is low and the change can
  ship independently of the other specs.
- The docs fixes and the guard are independent; if the guard is deferred, still
  fix the two live references now.
- Historical superpowers documents are left intact by design; the guard's
  exemption is explicit and tested.

## 8. Open Questions

- Should the invariant test also cover index corruption (truncated DB) and not
  only deletion?
- Should `EnsureIndexed` self-report repairs to telemetry, so a rebuild in
  production is observable?
- Is the structural table list worth maintaining, or should it be derived from
  the schema constants in `pkg/storage`?
- Should the docs guard also check route tables, or is that the separate optional
  phase?
- Do we promote `docs/architecture/review-2026-09-24.md` to
  `docs/architecture/overview.md` once these and the earlier findings land?

## 9. References

- Code: `pkg/engine/timeline.go:63-136,389-480`,
  `pkg/engine/history.go:17-60`, `pkg/storage/sync.go:22-81`,
  `pkg/gui/service.go:224-258`, `pkg/harness/context.go:145,173,263-277`,
  `pkg/gui/server.go:50-108`, `frontend/src/api/client.ts`,
  `frontend/src/types.ts`.
- Tests: `pkg/engine/timeline_test.go:17,209`.
- Docs: `AGENTS.md:50`, `docs/debugging.md:191`,
  `docs/architecture/review-2026-09-24.md` (§13 P1.6, P2.8),
  `docs/superpowers/specs/2026-09-21-canonical-db-and-turn-timeline-design.md:392`.
