# Design Specification: Canonical Game Database, Per-Turn Extraction, and the Campaign Timeline

**Date:** 2026-09-21  
**Status:** Draft — pending review  
**Topic:** Single canonical per-game SQLite database, entity extraction folded into every turn, and a numbered campaign timeline that records the player prompt, the narrator rewrite, and the entities involved

---

## 1. Problem Statement & Motivation

Three defects currently keep a campaign's history from being trustworthy:

1. **Two databases per game.** `engine.InitGame` and `localrpg play` open `games/<id>/cache/index.db`, while `gui.Service.GetEntity` and `gui.Service.SaveEntity` open `games/<id>/game.db` (`pkg/gui/service.go:136` and `:164`). The GUI therefore reads and writes a different index than the engine, which is why graph backlinks returned by the API are empty even after turns have been played.
2. **Extraction is dead code.** `harness.EntityExtractor.ExtractFromTurn` is never called outside its own tests, and when it is called it writes only to SQLite. Entity notes on disk never learn what the extractor discovered, and the flow contradicts the documented contract that Markdown is the source of truth and SQLite is a rebuildable index.
3. **No linked history.** `history.jsonl` records turns, but a turn does not record which entities it involved. Reconstructing "who was in turn 12" means re-reading prose and re-inferring mentions at read time. `TurnDTO.EntitiesHit` and `frontend/src/types.ts:26` already declare the field for exactly this purpose, and nothing ever populates it.
4. **Dialogue is attributed at read time, badly.** Two copies of the same regex (`pkg/media/tts.go:19` and `pkg/export/script.go:14`) guess who is speaking from raw prose when a turn is played back or exported. The guess has no idea which entities exist: the narrative oracle's line `As you declare: "I draw my blade"` matches `^([^:\n]+):\s*"([^"]+)"` and yields a speaker literally named **"As you declare"**, `[[wikilinks]]` pollute speaker names, and a speaker is never resolved to an entity, so playback cannot look up that character's `voice:` configuration. Speaker identity is only knowable when the turn is generated, which is when it must be recorded.

The fix establishes three invariants:

- **One database per game**, at `games/<id>/cache/index.db`, opened through a single code path.
- **Every turn extracts entities**, writes the affected Markdown notes, then syncs them into that database.
- **Every turn is a numbered timeline entry** carrying the player's prompt, the narrator's rewrite, the entities involved, and who spoke which line — recorded at write time, not inferred at read time.

---

## 2. Decisions & Non-Goals

Confirmed design decisions:

| Decision | Choice |
| --- | --- |
| Canonical database | `games/<id>/cache/index.db` |
| Timeline source of truth | `games/<id>/history.jsonl` (append-only), SQLite mirrors it for queries |
| Entity↔turn links | Entity frontmatter `history:` list **and** a SQLite `turn_entities` join table |
| Entity content source of truth | Entity Markdown notes in `games/<id>/entities/` |
| Extraction cadence | Once per turn, synchronously, as part of the turn pipeline |
| Dialogue attribution | Recorded per turn as ordered narration/speech segments with each speaker resolved to an entity ID; regex parsing survives only for pre-fix records |

Non-goals for this specification:

- Vector embeddings / semantic recall (the `sqlite-vec` idea in the core design doc stays unimplemented).
- Snapshot-based timeline branching that reverts entity state on undo.
- Asynchronous or background extraction. Extraction runs inline so the turn record is complete when it is written; a slow extractor slows the turn, and that is accepted.
- Dynamic location tracking. The orchestrator's `locationID` remains the campaign's start location (`engine.ResolveStartLocation`); a rules hook that moves the party is a future addition.
- A GUI turn-submission endpoint. The GUI still has no way to play a turn, so the timeline is exercised through `localrpg play` until that lands.
- New or changed TTS/STT/image providers. Dialogue attribution only decides which voice a line is rendered with; the provider layer is untouched.

---

## 3. Architecture

### 3.1 Layered data ownership

```text
games/<id>/
├── game.yaml              composition + pinned start_location        (authored)
├── entities/<id>.md       entity content, state, voice, history[]    (authored + extracted)
├── history.jsonl          numbered turn timeline with entity links   (canonical)
└── cache/index.db         queryable index: entities, edges, turns    (derived, rebuildable)
```

Each layer has exactly one writer:

| Layer | Written by | Rebuild rule |
| --- | --- | --- |
| Entity notes | GUI Codex drawer, timeline extraction | never rebuilt |
| `history.jsonl` | Timeline append / rewind | never rebuilt |
| `index.db` | Entity syncer + turn indexer | rebuildable from the two layers above |

`cache/index.db` stays disposable: deleting it loses nothing, because `Timeline.EnsureIndexed` re-derives entities from `entities/` and turns from `history.jsonl` on the next open.

### 3.2 Single canonical database access

`core.PathResolver` gains the only knowledge of the database location:

```go
// GameDBPath returns the canonical SQLite index for a campaign.
func (p *PathResolver) GameDBPath(gameID string) string {
	return filepath.Join(p.GameDir(gameID), "cache", "index.db")
}
```

Two hardening changes land with it:

1. **`storage.OpenGameStore(paths, gameID)`** — the single entry point every caller uses. It resolves `GameDBPath`, applies the legacy migration (§3.3), and returns a store from the process-wide pool below. Nothing else opens a game database.
2. **`storage.Pool`** — a path-keyed cache of open `*storage.Store` handles behind that entry point. `database/sql` already pools connections, so callers share one `Store` per game instead of opening and closing a connection per HTTP request (`gui.Service` does this today). `Pool.Close()` releases everything on shutdown.
3. **SQLite pragmas** in `storage.OpenDB`: `journal_mode=WAL`, `busy_timeout=5000`, `foreign_keys=ON`, `synchronous=NORMAL`. WAL is required once the GUI, the TUI, and an extraction pass can touch the same file.

`engine.InitGame` and every `gui.Service` method obtain their store through `OpenGameStore`; `cmd/localrpg/play.go` does the same, replacing its hand-built `filepath.Join(gameDir, "cache", "index.db")`.

### 3.3 Legacy `game.db` migration

A pre-fix campaign may contain `games/<id>/game.db`. It is derived data (the GUI always wrote the Markdown file before syncing the row), so migration is:

1. On first open of a game, if `games/<id>/game.db` exists, reindex `entities/` and `history.jsonl` into `cache/index.db`.
2. Rename the legacy file to `games/<id>/game.db.legacy` and log a one-line notice.

The rename keeps the old file available for inspection while guaranteeing only one live database. No read path ever consults `*.legacy`.

### 3.4 Turn record schema (`history.jsonl`)

`engine.Turn` becomes the timeline entry. `Output` is renamed to `Narration` because it holds the GM's rewrite for the narrator, and a read-only legacy field keeps pre-fix logs parseable:

```go
// entity package: shared by engine, media, export, and gui.
type SegmentKind = string

const (
	SegmentNarration SegmentKind = "narration"
	SegmentSpeech    SegmentKind = "speech"
)

// TurnSegment is one spoken or narrated span of a turn, in playback order.
type TurnSegment struct {
	Kind      SegmentKind `json:"kind"`
	Speaker   string      `json:"speaker,omitempty"`    // display name as written in the prose
	SpeakerID string      `json:"speaker_id,omitempty"` // resolved entity ID, "" when unresolved
	Text      string      `json:"text"`
}

// Mention records how one entity was involved in a turn.
type Mention struct {
	ID   string `json:"id"`
	Kind string `json:"mention"` // "player", "location", "wikilink", "extracted", "speech"
}

type Turn struct {
	Number    int               `json:"number"`
	Timestamp time.Time         `json:"timestamp"`
	Mode      string            `json:"mode"`      // "Do", "Say", "Story", "Roll", "GM", "System"
	Input     string            `json:"input"`     // exactly what the player typed, never rewritten
	Narration string              `json:"narration"` // the GM's verbatim rewrite
	Segments  []entity.TurnSegment `json:"segments,omitempty"` // ordered playback script
	Roll      *rules.RollResult    `json:"roll,omitempty"`
	Entities  []entity.Mention     `json:"entities,omitempty"` // involvement, ordered
	AudioRefs []string          `json:"audio_refs,omitempty"` // externally supplied audio only

	// LegacyOutput is populated only when reading records written before the
	// narration rename. New records must not set it.
	LegacyOutput string `json:"output,omitempty"`
}

// Prose returns the narrator-visible text regardless of which field holds it.
func (t Turn) Prose() string
```

`Narration` stays the GM's verbatim output for display, export, and debugging. `Segments` is the derived playback script: it carries the same prose split into ordered narration and speech spans, so playback reads prose in the narrator voice and speech in each character's voice without re-parsing text at read time. The duplication is deliberate — `Segments` is recomputable from `Narration`, and `Narration` must survive editing without loss.

`HistoryLogger.loadHistoryUnlocked` normalises on read: when `Narration` is empty and `LegacyOutput` is set, it promotes the value and clears the legacy field, so a rewind rewrite emits clean records.

Dialogue segments are deliberately **not** stored with audio references. Clips are content-addressed, so playback derives the file name from the same inputs that chose the voice (`media.ComputeAudioCacheKeyWithRate(speakerID, voiceID, pitch, speechRate, text)`) and checks `ContentCache.Exists`. Keeping audio out of the record is what lets `history.jsonl` stay append-only; `AudioRefs` remains for audio that arrives from outside the pipeline (imports, manual overrides).

On-disk example:

```json
{
  "number": 12,
  "timestamp": "2026-09-21T14:03:11Z",
  "mode": "Do",
  "input": "I ask Garrick about the ledger",
  "narration": "Garrick keeps his eyes on the crowd. Garrick: \"You didn't see me here.\"",
  "segments": [
    { "kind": "narration", "text": "Garrick keeps his eyes on the crowd." },
    { "kind": "speech", "speaker": "Garrick the Fence", "speaker_id": "garrick-the-fence", "text": "You didn't see me here." }
  ],
  "entities": [
    { "id": "hero", "mention": "player" },
    { "id": "aldon-harbour", "mention": "location" },
    { "id": "garrick-the-fence", "mention": "extracted" }
  ]
}
```

### 3.5 Entity history frontmatter

Entity notes gain their own view of involvement:

```yaml
---
id: garrick-the-fence
name: Garrick the Fence
type: character
history: [12, 14]
---
```

- `entity.EntityFrontmatter` and `entity.Entity` gain `History []int`; `ParseMarkdownEntity`/`SerializeMarkdown` round-trip it.
- It is also stored in `entities.frontmatter_json` so the index can answer "which turns touched this entity" without joining.
- Turn numbers are unique and ascending; adding a number already present is a no-op.
- `history:` records that a turn touched the note. It does not record what changed, and undo prunes numbers but does not revert body text.

### 3.6 Database schema

The schema stays additive, so existing indexes upgrade in place through the `CREATE TABLE IF NOT EXISTS` statements in `storage.OpenDB`:

```sql
CREATE TABLE IF NOT EXISTS turns (
    number       INTEGER PRIMARY KEY,
    timestamp    TIMESTAMP NOT NULL,
    mode         TEXT NOT NULL,
    input        TEXT NOT NULL,
    narration    TEXT NOT NULL,
    roll_json    TEXT,
    audio_refs_json TEXT
);

CREATE TABLE IF NOT EXISTS turn_entities (
    turn_number INTEGER NOT NULL,
    entity_id   TEXT NOT NULL,
    mention     TEXT NOT NULL,  -- "player" | "location" | "wikilink" | "extracted"
    PRIMARY KEY (turn_number, entity_id, mention)
);

CREATE INDEX IF NOT EXISTS idx_turn_entities_entity ON turn_entities(entity_id);
```

`turn_entities` is derived from the turn records, so a rebuilt index repopulates it. The `mention` column keeps provenance: a query can tell whether an entity drove a turn, hosted it, or was merely referenced.

`storage` gains row-level types (`TurnRecord`, `TurnEntityRef`) plus operations:

| Method | Purpose |
| --- | --- |
| `SaveTurn(rec TurnRecord) error` | Upsert one turn and replace its entity links in a single transaction |
| `GetTurn(number int) (*TurnRecord, error)` | Single timeline entry |
| `ListTurns(limit, offset int) ([]TurnRecord, error)` | Chronicle reads |
| `ListTurnsForEntity(entityID string) ([]int, error)` | Entity involvement, ascending |
| `ListEntitiesForTurn(number int) ([]TurnEntityRef, error)` | Who was in a turn |
| `DeleteTurnsFrom(number int) error` | Rewind support, cascades into `turn_entities` |
| `MaxTurnNumber() (int, error)` | Cheap `EnsureIndexed` check |
| `CountTurns() (int, error)` | Game summaries |

`engine.Turn` (JSONL shape) and `storage.TurnRecord` (row shape) stay separate types with an explicit mapper in `engine`; `storage` must not import `engine`, and a single mapper beats a shared type that forces one layer to know the other's JSON tags.

---

## 4. Turn Lifecycle

`TurnOrchestrator.ProcessAction` (`pkg/engine/orchestrator.go:74`) keeps its shape and gains one collaborator: an `engine.Timeline` built from the path resolver and the store.

```go
func NewTurnOrchestrator(
	paths *core.PathResolver,
	store *storage.Store,
	history *HistoryLogger,
	rulesEngine *rules.JSEngine,
	router *harness.Router,
	locationID, playerID string,
) *TurnOrchestrator
```

Per-turn pipeline:

1. Load history, derive `turnNum`, handle `/undo` (see §6).
2. Evaluate mechanics. The raw player input is preserved: `Turn.Input` keeps exactly what was typed, and roll/directive text lives in a local `generationPrompt` instead of overwriting the input field (today `orchestrator.go:106-125` mutates it).
3. Assemble context and generate the narration via `router.GenerateForRole(ctx, "gm", …)`.
4. Resolve **deterministic mentions**: the player, the current location, and every `[[wikilink]]` in the narration or input that matches an indexed entity (§5.1).
5. Run **extraction** through the `extractor` role (§5.2). Extracted records are matched, merged, written to `entities/<id>.md`, and synced.
6. Resolve **dialogue attribution** (§7) into `turn.Segments`, resolving every speaker to an entity ID.
7. Set `turn.Entities` to the deduplicated union of mentions, extracted IDs, and attributed speakers, ordered: player, location, wikilinks in encounter order, then extracted records, then speakers.
8. Append the turn record to `history.jsonl`.
9. Index the turn into `cache/index.db` via `SaveTurn`.

Failure handling is explicit and ordered so files never point at nothing:

| Failure | Behaviour |
| --- | --- |
| Entity note write fails | Abort the turn before appending; return the error. No timeline entry, no orphan links. |
| History append fails | Return the error; the index is not written. The retried turn reuses the same number. |
| Index write fails | Log and continue. `EnsureIndexed` repairs it on the next open — this is the whole point of a derived index. |
| Extraction provider fails | Log and continue with deterministic mentions only, appending the turn with `mention: "wikilink"`-sourced links. A failed extractor must never lose the turn. |
| Dialogue resolution fails | Record the segments with an empty `speaker_id`. Playback falls back to the narrator voice rather than dropping the segment. |

---

## 5. Entity Extraction & Reconciliation

### 5.1 Deterministic mentions (always on)

New `harness.ResolveEntityMentions(store, playerID, locationID, text...) []MentionedEntity`:

- The player ID and the orchestrator's location ID are always mentions, tagged `player` and `location`.
- `[[wikilinks]]` in the narration and the raw input are unwrapped with `entity.WikilinkTarget`, slugified with `entity.Slugify`, and matched against indexed entities by ID and by display name (the same normalisation `engine.findLocationByRef` and `harness.MatchExistingEntity` already use). Unknown link targets are skipped rather than created.
- Mentions carry the `mention` provenance used by `turn_entities`.

This runs with no model configured, so a zero-GPU game still gets an accurate involvement record.

### 5.2 Extractor role

`harness.EntityExtractor` is renamed and split into two responsibilities:

- `harness.Extractor.Extract(ctx, narrative string) ([]ExtractedEntity, error)` — model call only, no storage dependency, mockable with the existing `mockProvider`.
- `engine.Timeline` — match, merge, persist.

Router wiring follows the existing role pattern in `cmd/localrpg/play.go:68-85`:

| `agents.roles.extractor` | Behaviour |
| --- | --- |
| absent | Reuse the provider resolved for `gm`, so extraction happens every turn with no extra configuration |
| configured | Use that provider (`builtin`/`cli`/`http`) |
| `disabled` | Skip the model call; deterministic mentions still record involvement |

### 5.3 Matching, merging, and writing

For each extracted record:

1. `harness.MatchExistingEntity` resolves it against the index (exact ID → exact name → partial name tokens → same type plus shared location and role tag), unchanged from the current implementation.
2. A match is merged with `mergeExtractedEntity` (preserving ID, authored name, type, location, faction, voice, state) and the new detail is appended to the body unless already present.
3. No match creates a new note whose ID is `entity.Slugify(raw.Name)`, with a collision check against existing IDs.
4. The note's `history` gains `turnNum`.
5. The merged entity is serialized to `games/<id>/entities/<id>.md`.
6. After all touched notes are written, one `storage.Syncer.Sync(entitiesDir)` pass updates the index. Batched syncing replaces per-entity `SyncFile` calls, and the directory scan is trivially cheap at campaign scale.

Two constraints make this safe:

- **File names are entity IDs.** `gui.Service.GetGraph` derives node IDs from file names (`service.go:190`) and `GetEntity`/`SaveEntity` read `entityID + ".md"`. The timeline must therefore write `<id>.md`, and `engine.InitGame`'s copy of world template entities must be normalised to `<id>.md` instead of preserving the template's file name (a world note `Tavern.md` with `id: tavern` is currently unreachable through the GUI).
- **Synthetic hashes disappear.** Because every extracted entity now has a real file, `SyncFile`/`Sync` supplies its content hash. The `extracted-<id>` hash convention is retired; the matching work that motivated it stays.

---

## 6. Undo, Rewind & Index Integrity

`Timeline` owns a single rewind path used by `/undo`:

1. `HistoryLogger.RewindToTurn(n)` truncates `history.jsonl` (existing behaviour).
2. `storage.DeleteTurnsFrom(n + 1)` removes turn rows and their entity links.
3. For every entity that referenced the discarded turns, the turn numbers are pruned from `history:` and the note is rewritten, then the index is re-synced.

Rewind does **not** revert entity body text or state. `/undo` trims the record of what happened, not the world's memory of it; true rollback is the separate snapshot-branching feature.

`Timeline.EnsureIndexed()` runs whenever a game is opened (`engine.InitGame`, `localrpg play`, `gui.Service` first use of a game). It loads the timeline once through `HistoryLogger.LoadHistory` and reuses that read for every comparison below:

- Rebuild entities from `entities/` when the index has no rows, and always re-run `Syncer.Sync` to pick up hand edits.
- Compare `storage.CountTurns()` and `storage.MaxTurnNumber()` against the loaded timeline: replay the missing records through `Timeline.SyncTurns` (each loaded `Turn` mapped to `storage.SaveTurn`) when the index is behind, and delete rows above the highest real turn when it is ahead (a rewind performed while the index was unavailable).

Turn numbers after a rewind are reused (`len(turns) + 1`), which is safe because `SaveTurn` upserts and the rewind already deleted the stale rows.

This keeps "the index is disposable" honest without needing a new CLI command: opening the game repairs it.

---

## 7. Dialogue Attribution & Voice-Aware Playback

A turn such as "the player says something, an NPC answers" must record who said what, so playback can pick the right voice instead of guessing from prose.

### 7.1 One parser, not three

`pkg/media/tts.go:22` and `pkg/export/script.go:14` each carry the same inference regex; the new writer would make a third copy. Replace all of them with one dependency-light package:

```go
// pkg/dialogue
// Segment is one span of text: narration, or speech attributed to a speaker.
type Segment struct {
	Speaker   string // display name as written; "" for narration
	SpeakerID string // resolved entity ID; "" when the speaker is unknown
	Text      string
	IsSpeech  bool
}

// Parse splits text into ordered narration and speech segments, calling resolve to
// decide whether a candidate speaker is a known entity. resolve maps the candidate
// to an entity ID and reports whether the speaker is known; unknown candidates stay
// narration instead of becoming a bogus speaker.
func Parse(text string, resolve func(candidate string) (string, bool)) []Segment
```

`Parse` keeps the readable `Name: "…"` convention, unwraps `[[wikilink|label]]` speaker forms with `entity.WikilinkTarget`, and treats a resolved speaker as the gate for the whole match. That gate is what kills the false positive described in §1: `As you declare: "I draw my blade"` proposes the candidate speaker `As you declare`, which resolves to nothing, so the line stays narration instead of inventing a character.

### 7.2 Writing the segments

`engine.Timeline` builds `turn.Segments` from one ordered parse of the narration, with speaker identity coming from whichever source knows it:

1. **Extractor attributions.** The extractor's JSON response gains an optional `dialogue: [{ "speaker": "Garrick the Fence", "text": "You didn't see me here." }]` array alongside its entity list. Model-provided attributions win for a matching span because the model knows who was speaking — a narration span containing the quoted text is split so the speech is recorded even when the prose did not follow the `Name: "…"` convention — and each `speaker` is resolved to an entity ID, so `speaker_id` is populated whenever the character is known.
2. **Deterministic parse.** `dialogue.Parse(narration, resolve)` runs over the narration with `resolve` resolving a candidate speaker through `harness.ResolveSpeakerID`. In `Say` mode the player's raw input is prepended as a speech segment `{SpeakerID: playerID}`, because in that mode the input *is* the player's utterance.

Rules:

- Segments preserve narration order so playback interleaves prose and speech correctly instead of speaking the whole prose and then every line.
- Segments cover the whole narration: prose spans become `kind: "narration"`, so playback never re-reads speech inside prose and never drops prose.
- A speech segment whose speaker does not resolve keeps `speaker_id` empty. Playback reads it in the narrator voice; the segment is never dropped.
- If the extractor is `disabled` or returns no `dialogue` array, the deterministic parse still attributes every resolvable `Name: "…"` span.
- Speaker resolution uses `harness.ResolveSpeakerID(store, name)`, a name-only lookup over `storage.Store.ListEntities` reusing the same slug and token rules as `MatchExistingEntity`.

- Speakers of attributed segments are recorded as entity involvement with `mention: "speech"`, in addition to `player`, `location`, `wikilink`, and `extracted`.
- Authored GM prompts in `systems/*/prompts/rules.md` and `worlds/*/prompts/lore.md` are not modified; the dialogue request belongs to the extractor prompt, which the engine owns.

### 7.3 Playback

- `media.ParseDialogueSegments` and `media.UtteranceSegment` are replaced by `entity.TurnSegment`: playback consumes the recorded segments directly, and legacy turns (`turn.Segments` empty) are parsed once at playback time with a permissive resolve so old campaigns still play.
- Playback walks the segments in order. A `speech` segment with a `speaker_id` looks up that entity's `voice:` frontmatter (per-character override or the archetype assigned by `harness.AssignVoiceProfile`) and synthesizes with it; narration and unresolved speech use the narrator's configured voice.
- The existing cache key already isolates speaker, voice, pitch, rate, and text, so two characters sharing a voice choice still get distinct clips.
- `export.SceneBeat` replaces its single `Speaker`/`Dialogue` pair with `Segments []entity.TurnSegment`, compiled from the turn record. The embedded player template in `pkg/export/web.go` currently renders one `beat.speaker` plus `beat.dialogue` per beat (`web.go:129-130`) and becomes a per-segment list with the speaker label per segment.
- `pkg/export/video.go` stays a silent still-image renderer for now: it never reads audio, so per-segment audio and timed frame changes belong to the story-theater export effort. The segments now travel on the beat for that work. **[Erratum 2026-09-28: no longer true. `pkg/export/video.go` now animates frames and interleaves each beat's audio; see `docs/superpowers/plans/2026-09-21-animated-export-and-video.md`.]**
- The Story Theater component consumes the same segment data, so in-app replay, the standalone web bundle, and the rendered video all agree on who is speaking.

---

## 8. API & UI Surface

- `gui.Service.GetChronicle` populates the already-declared `TurnDTO.EntitiesHit` from `turn_entities`, giving the Chronicle view real involvement data, and exposes the ordered playback script via `TurnDTO.Segments`.
- `EntityDTO` gains `History []int` (turn numbers), sourced from the indexed frontmatter, so the Codex drawer can show which turns touched a note.
- `pkg/tui` renders `turn.Prose()`. The replay exporter keeps reading `history.jsonl`, which now also carries `entities` and `segments` for richer scene metadata.
- Frontend: `Turn.entities_hit` renders as chips in `ChronicleView` that open the Codex drawer for that entity, `Turn.segments` renders speaker-labelled speech spans with the speaker's voice label, and `EntityNote.history` renders as a turn list. This is the payoff for the whole design and is deliberately last so it can be dropped without affecting correctness.

---

## 9. Component & File Map

| File | Change |
| --- | --- |
| `pkg/core/types.go` | Add `PathResolver.GameDBPath` |
| `pkg/storage/db.go` | Add `turns`/`turn_entities` schema, WAL and busy-timeout pragmas |
| `pkg/storage/store.go` | Turn CRUD, `MaxTurnNumber`, `CountTurns`, `DeleteTurnsFrom` |
| `pkg/storage/pool.go` *(new)* | Path-keyed shared `Store` handles |
| `pkg/storage/game.go` *(new)* | `OpenGameStore(paths, gameID)`: pool lookup, legacy `game.db` migration |
| `pkg/entity/entity.go` | `History []int` on frontmatter and entity, parse/serialize round-trip |
| `pkg/dialogue/dialogue.go` *(new)* | Single `Parse` for ordered narration/speech spans, replacing the duplicated regexes |
| `pkg/entity/segment.go` *(new)* | `TurnSegment`, segment kinds, `Mention` and mention kinds |
| `pkg/engine/history.go` | `Narration` + `Segments` + `Entities` + legacy alias, `Prose()`, read normalisation |
| `pkg/engine/timeline.go` *(new)* | `Timeline`: `RecordTurn`, `Rewind`, `EnsureIndexed` |
| `pkg/engine/segments.go` *(new)* | Builds ordered narration/speech segments and applies extractor attributions |
| `pkg/engine/orchestrator.go` | Accept `paths`, build `Timeline`, preserve raw input, call `RecordTurn` |
| `pkg/engine/game.go` | Use `GameDBPath`, normalise template file names, `EnsureIndexed` |
| `pkg/harness/extractor.go` | Rename `EntityExtractor` to `Extractor`, split the model call from persistence, add `ResolveEntityMentions` and `ResolveSpeakerID`, extend the extractor prompt with `dialogue` |
| `pkg/media/tts.go` | `UtteranceSegment`/`ParseDialogueSegments` replaced by `entity.TurnSegment` playback |
| `pkg/export/script.go`, `web.go`, `video.go` | `SceneBeat.Lines`, per-line speaker/voice rendering and audio |
| `pkg/gui/service.go` | Canonical DB path via pool; populate `EntitiesHit` and `Segments`; expose `History` and `GetEntityTurns` |
| `pkg/gui/types.go`, `frontend/src/types.ts` | `TurnDTO.Segments`, `EntityDTO.History`, `EntityNote.history` |
| `cmd/localrpg/play.go` | Reuse the canonical store/`Timeline`; render `turn.Prose()` |
| `frontend/src/components/ChronicleView.tsx`, `CodexDrawer.tsx` | Entity chips and turn list |

---

## 10. Migration & Compatibility

- **Existing `game.db`**: renamed to `game.db.legacy` after reindexing; never read again.
- **Existing `history.jsonl`**: read through the legacy `output` alias and normalised in memory; rewritten clean on the next rewind.
- **Existing entity notes**: `history:` is absent until their next turn; nothing backfills historical involvement, because that would mean re-inferring the very thing this design stops inferring. Campaigns start recording links from the upgrade turn onwards.
- **Existing `index.db`**: upgraded in place by the additive schema; turns are replayed from JSONL on first open.
- **Missing `cache/` directory**: created on demand.

---

## 11. Verification & Testing Plan

**`pkg/storage`**
- Additive schema applies to a pre-existing database created with the old schema, preserving rows.
- `SaveTurn` upserts idempotently and replaces links; `DeleteTurnsFrom` cascades.
- `ListTurnsForEntity` returns ascending, deduplicated turn numbers.
- `Pool.Open` returns the same `*Store` for repeated calls and a distinct one per path.
- Legacy migration: a fixture `games/x/game.db` is renamed and `cache/index.db` becomes the live index.

**`pkg/entity`**
- `history:` round-trips through `ParseMarkdownEntity`/`SerializeMarkdown` and is omitted when empty.

**`pkg/engine`**
- A two-turn game writes both entity notes, both JSONL records, and both index rows; the second turn on the same entity does not duplicate the note.
- Entity notes carry ascending `history` numbers, and turn records list exactly the same IDs.
- Raw player input is preserved on `Roll` and `/gm` modes (regression test for the current mutation).
- A record written with the legacy `output` field loads as `Narration` and survives a rewind rewrite.
- `/undo` truncates JSONL, deletes indexed turns, and prunes entity `history` without reverting body text.
- A failing extractor still records the turn with deterministic mentions.
- Deleting `cache/index.db` and reopening the game restores entities and turns.

**`pkg/gui`**
- No request path creates `game.db`; `GetChronicle` returns populated `entities_hit`; `GetEntity` returns `history` and non-empty backlinks for a played turn.

**`pkg/dialogue`**
- `Name: "…"` lines resolve when `resolve` returns true and stay narration when it returns false (the `As you declare: "…"` regression).
- `[[Name|label]]` and `[[Name]]` speaker forms resolve to the plain name.
- Quoted speech with no attribution stays narration; prose without quotes is never marked as speech.

**`pkg/media`**
- Playback consumes `turn.Segments` when present and parses the prose once for pre-fix turns.
- Two speakers sharing a voice ID produce distinct cache keys; a line with no `speaker_id` uses the narrator voice.

**`pkg/export`**
- `SceneBeat.Lines` carries ordered speaker/text pairs from the turn record; the web template renders one speaker label per line.

**Project**
- `mise run test` (Go suite plus `tsc --noEmit`) and `go vet ./...` stay clean.

---

## 12. Phased Delivery

1. **Canonical database** — `GameDBPath`, pragmas, pool, `gui.Service` switch, legacy migration. Independently shippable and immediately fixes GUI backlinks.
2. **Timeline schema & persistence** — schema, turn CRUD, `SyncTurns` replay, `EnsureIndexed`.
3. **Turn record model** — `Narration` rename with legacy alias, raw-input preservation, `Entities` field, `Prose()` at the read sites.
4. **Per-turn extraction** — extractor split, deterministic mentions, `Timeline.RecordTurn` writing notes then syncing, extractor role wiring.
5. **Undo & integrity** — rewind across JSONL, index, and entity links.
6. **Dialogue attribution** — `pkg/dialogue`, `entity.TurnSegment`, extractor `dialogue` records, voice-aware segment playback, export per-segment rendering.
7. **Surface** — `EntitiesHit`, `EntityDTO.History`, Chronicle chips, Codex turn list.

---

## 13. Open Questions

- Should the extractor reuse the `gm` provider by default (assumed here), or default to `disabled` so players opt in to the extra per-turn model call once media and model costs matter?
- Should `turn_entities` also record the *roll* outcome per entity (e.g. who was involved in a failed check), or is turn-level involvement enough?
- Should the GM's authored system prompts gain a speech-formatting instruction so deterministic attribution matches as reliably as extractor-provided lines, or is extractor-only attribution enough when the role is enabled?
