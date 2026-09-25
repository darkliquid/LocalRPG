# Turn Attribution, Location Tracking, and Playback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the open questions from the timeline work: make extraction a first-class configurable role, give systems a way to report their own check outcomes, tell the GM how to write speech and parse it more widely, create the missing player note, track and record the player's location per turn, and make the recorded dialogue actually play.

**Architecture:** Storage gains a real migration path (`PRAGMA user_version`) before it gains any new columns. The player note becomes the source of truth for location, read at the start of each turn and recorded on the turn, with three movement paths of decreasing authority (rules host call, `/go` command, gated extractor proposal). Imagery and audio stay content-addressed in the media cache and are generated lazily on request; nothing about a clip or an image is ever stored on the turn.

**Tech Stack:** Go 1.27.1, `modernc.org/sqlite` (no CGO), Bubbletea TUI, React 19 + TypeScript frontend, `mise` tasks.

**Spec:** `docs/superpowers/specs/2026-09-21-turn-attribution-location-and-playback-design.md`

## Global Constraints

- The player note's frontmatter `location:` field is the single source of truth for where a turn happened. Nothing caches it in session state.
- `engine.Timeline` remains the only writer of entity notes. New callers (`DefaultHostBridge`) get a writer rather than reaching for `storage.Store.SaveEntity`.
- Storage migrations must be idempotent and must run on every open, before any query that needs the new columns.
- Media output is content-addressed: identical inputs must reuse cached bytes. No clip or image path is stored on the turn record.
- TTS or image generation being unconfigured is never an error: audio returns `204`, imagery falls back per `media.image.builtin_fallback`.
- Tests use the standard library only (`testing`, `t.TempDir()`); no testify. Use `interface{}`, never `any`.
- `go vet ./...`, `go test -count=1 ./...`, and `cd frontend && npx tsc --noEmit` must pass. Every commit must build standalone: `git worktree add --detach /tmp/verify <sha> && (cd /tmp/verify && go build ./...)`.
- Test commands, from the repo root: package `go test -count=1 ./pkg/storage/`; single test `go test -run TestName -count=1 ./pkg/engine/`.

## Scope & Splitting

Phase 1 is a prerequisite for every later phase, because without it the new columns silently fail to appear on an existing campaign database. Phases 2-3 (location, outcomes) are the timeline-completeness work and stand alone as a shippable slice. Phases 4-5 (extractor role, speech attribution) are independent of each other. Phases 6-7 (imagery, playback) are the consumer-facing half and depend on 2 and 5 respectively: imagery needs a per-turn location, playback needs trustworthy segments.

---

## Phase 1: Migrations and Column Changes

### Task 1: A real migration path, with the new columns

**Files:**
- Modify: `pkg/storage/db.go` (`schema` const, `OpenDB`, new migration machinery)
- Create: `pkg/storage/migrate.go`, `pkg/storage/migrate_test.go`

**Interfaces:**
- Consumes: `storage.OpenDB` (existing callers unchanged)
- Produces: `storage.migrations`, `storage.migrate(db *sql.DB) error`, `storage.columnExists(db *sql.DB, table, column string) (bool, error)`

- [x] **Step 1: Write the failing test**

`pkg/storage/migrate_test.go`:

```go
package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// legacyDB builds a database with the pre-migration shape: turn tables without
// the location and outcome columns, and the vestigial audio_refs_json column.
func legacyDB(t *testing.T, path string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path+"?"+pragmas)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}

	const legacySchema = `
CREATE TABLE IF NOT EXISTS turns (
    number       INTEGER PRIMARY KEY,
    timestamp    TEXT NOT NULL,
    mode         TEXT NOT NULL,
    input        TEXT NOT NULL,
    narration    TEXT NOT NULL,
    roll_json    TEXT,
    audio_refs_json TEXT
);
CREATE TABLE IF NOT EXISTS turn_entities (
    turn_number INTEGER NOT NULL,
    entity_id   TEXT NOT NULL,
    mention     TEXT NOT NULL,
    PRIMARY KEY (turn_number, entity_id, mention)
);`
	if _, err := db.Exec(legacySchema); err != nil {
		t.Fatalf("apply legacy schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO turns (number, timestamp, mode, input, narration) VALUES (1, '2026-09-21T10:00:00Z', 'Do', 'look', 'You look around.')`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	return db
}

func TestMigrationsUpgradeALegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")

	legacy := legacyDB(t, path)
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore failed on a legacy database: %v", err)
	}
	defer store.Close()

	for _, column := range []string{"location", "outcome"} {
		exists, err := columnExists(store.db, "turns", column)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("expected turns.%s to exist after migration", column)
		}
	}

	exists, err := columnExists(store.db, "turn_entities", "outcome")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Errorf("expected turn_entities.outcome to exist after migration")
	}

	// Existing rows survive.
	rec, err := store.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn on a migrated database failed: %v", err)
	}
	if rec.Narration != "You look around." {
		t.Errorf("Narration = %q after migration", rec.Narration)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")

	first, err := OpenDB(path)
	if err != nil {
		t.Fatalf("first OpenDB failed: %v", err)
	}
	var version int
	if err := first.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := OpenDB(path)
	if err != nil {
		t.Fatalf("second OpenDB failed: %v", err)
	}
	defer second.Close()

	var again int
	if err := second.QueryRow("PRAGMA user_version").Scan(&again); err != nil {
		t.Fatal(err)
	}
	if again != version {
		t.Errorf("user_version moved from %d to %d on reopen", version, again)
	}
	if version != len(migrations) {
		t.Errorf("user_version = %d, want %d (one per migration)", version, len(migrations))
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestMigrations -count=1 ./pkg/storage/`
Expected: FAIL — `undefined: columnExists`, and `turns.location` missing after open.

- [x] **Step 3: Implement**

Create `pkg/storage/migrate.go`:

```go
package storage

import (
	"database/sql"
	"fmt"
)

// migration is one ordered, idempotent step. Versions must be contiguous
// starting at 1: user_version records the highest applied version.
type migration struct {
	version int
	apply   func(*sql.DB) error
}

var migrations = []migration{
	{version: 1, apply: addTimelineColumns},
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, m := range migrations {
		if m.version <= version {
			continue
		}
		if err := m.apply(db); err != nil {
			return fmt.Errorf("migration %d: %w", m.version, err)
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
	}
	return nil
}

func addTimelineColumns(db *sql.DB) error {
	columns := []struct{ table, column, definition string }{
		{"turns", "location", "TEXT"},
		{"turns", "outcome", "TEXT"},
		{"turn_entities", "outcome", "TEXT"},
	}

	for _, c := range columns {
		exists, err := columnExists(db, c.table, c.column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", c.table, c.column, c.definition)); err != nil {
			return fmt.Errorf("add %s.%s: %w", c.table, c.column, err)
		}
	}
	return nil
}

// columnExists reports whether a table already has a column, which is what makes
// a partially applied migration converge instead of failing on the second run.
func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid          int
			name, ctype  string
			notNull, pk  int
			defaultValue interface{}
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan %s columns: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
```

In `pkg/storage/db.go`, update the `schema` const's `turns` table for fresh databases — the new columns, with `audio_refs_json` still present because Task 2 removes both the column and its only reader:

```sql
CREATE TABLE IF NOT EXISTS turns (
    number       INTEGER PRIMARY KEY,
    timestamp    TEXT NOT NULL,
    mode         TEXT NOT NULL,
    input        TEXT NOT NULL,
    narration    TEXT NOT NULL,
    roll_json    TEXT,
    audio_refs_json TEXT,
    location     TEXT,
    outcome      TEXT
);
```

and `turn_entities` gains `outcome TEXT`, while the `turn_entities` primary key stays `(turn_number, entity_id, mention)`. Call the migrator after the schema so a legacy database converges and a fresh one records its version:

```go
func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?"+pragmas)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return db, nil
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/storage/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/storage/db.go pkg/storage/migrate.go pkg/storage/migrate_test.go
git commit -m "feat(storage): add a schema migration path before the timeline grows"
```

### Task 2: Record location and outcome, and drop the dead audio fields

**Files:**
- Modify: `pkg/storage/migrate.go` (migration 2 drops the vestigial column)
- Modify: `pkg/storage/db.go` (remove `audio_refs_json` from the schema)
- Modify: `pkg/storage/turn.go` (`TurnRecord`, `TurnEntityRef`, save, load, list)
- Modify: `pkg/engine/history.go` (`Turn`: `Location`, `Outcome`; remove `AudioRefs`)
- Modify: `pkg/engine/timeline.go` (`turnRecord`)
- Modify: `pkg/export/types.go`, `pkg/export/script.go` (remove the dead `AudioPath`)
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go` (remove the dead `TurnDTO.AudioURL`)
- Test: `pkg/storage/turn_test.go`, `pkg/export/script_test.go`

**Interfaces:**
- Consumes: `migrations` from Task 1
- Produces: `storage.TurnRecord{Location, Outcome string}`, `storage.TurnEntityRef{EntityID, Mention, Outcome string}`, `engine.Turn.Location` (JSON `location`), `engine.Turn.Outcome` (JSON `outcome`)

- [x] **Step 1: Write the failing test**

Append to `pkg/storage/turn_test.go`:

```go
func TestTurnRecordCarriesLocationAndOutcome(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	rec := TurnRecord{
		Number:    1,
		Timestamp: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC),
		Mode:      "Do",
		Input:     "I swing at the cultist",
		Narration: "Steel rings.",
		Location:  "alden-tavern",
		Outcome:   "glancing_blow",
		Entities: []TurnEntityRef{
			{EntityID: "player", Mention: "player", Outcome: "glancing_blow"},
			{EntityID: "alden-tavern", Mention: "location", Outcome: "glancing_blow"},
		},
	}

	if err := store.SaveTurn(rec); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}

	loaded, err := store.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn failed: %v", err)
	}
	if loaded.Location != "alden-tavern" {
		t.Errorf("Location = %q, want alden-tavern", loaded.Location)
	}
	if loaded.Outcome != "glancing_blow" {
		t.Errorf("Outcome = %q, want glancing_blow", loaded.Outcome)
	}
	if len(loaded.Entities) != 2 {
		t.Fatalf("expected 2 links, got %+v", loaded.Entities)
	}

	// The per-entity copy is what makes an outcome query single-table.
	outcomes, err := store.ListTurnEntitiesByOutcome("player", "glancing_blow")
	if err != nil {
		t.Fatalf("ListTurnEntitiesByOutcome failed: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0] != 1 {
		t.Errorf("expected turn 1 for a glancing_blow involving player, got %v", outcomes)
	}

	none, err := store.ListTurnEntitiesByOutcome("player", "clean_hit")
	if err != nil {
		t.Fatalf("ListTurnEntitiesByOutcome failed: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected no turns for a clean_hit, got %v", none)
	}
}
```

In `pkg/export/script_test.go`, remove the `AudioRefs:` line from the two `engine.Turn` literals it builds (`AudioRefs` is gone), keeping the rest of the fixture intact.

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestTurnRecordCarriesLocationAndOutcome -count=1 ./pkg/storage/`
Expected: FAIL — `unknown field Location in struct literal of type TurnRecord`

- [x] **Step 3: Implement**

First, retire the vestigial column in the same change that removes its only reader:

```go
var migrations = []migration{
	{version: 1, apply: addTimelineColumns},
	{version: 2, apply: dropAudioRefsColumn},
}

// dropAudioRefsColumn removes the column nothing has ever written. It is a
// separate step from the timeline columns because it has to land with the change
// that removes its reader, or GetTurn would select a column that is gone.
func dropAudioRefsColumn(db *sql.DB) error {
	exists, err := columnExists(db, "turns", "audio_refs_json")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, err := db.Exec("ALTER TABLE turns DROP COLUMN audio_refs_json"); err != nil {
		return fmt.Errorf("drop turns.audio_refs_json: %w", err)
	}
	return nil
}
```

remove `audio_refs_json` from the `schema` const's `turns` table, and assert the drop in the test:

```go
	dropped, err := columnExists(store.db, "turns", "audio_refs_json")
	if err != nil {
		t.Fatal(err)
	}
	if dropped {
		t.Errorf("expected turns.audio_refs_json to be dropped")
	}
```

Then in `pkg/storage/turn.go`:

```go
// TurnEntityRef records how one entity was involved in a turn. Outcome is the
// turn's system-reported outcome, copied for single-table per-entity queries.
type TurnEntityRef struct {
	EntityID string
	Mention  string
	Outcome  string
}

// TurnRecord is the row shape of one timeline entry.
type TurnRecord struct {
	Number    int
	Timestamp time.Time
	Mode      string
	Input     string
	Narration string
	Location  string
	Outcome   string
	RollJSON  string
	Entities  []TurnEntityRef
}
```

`SaveTurn` writes the two new columns and the link outcome:

```go
	const upsert = `
	INSERT INTO turns (number, timestamp, mode, input, narration, roll_json, location, outcome)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(number) DO UPDATE SET
		timestamp = excluded.timestamp,
		mode = excluded.mode,
		input = excluded.input,
		narration = excluded.narration,
		roll_json = excluded.roll_json,
		location = excluded.location,
		outcome = excluded.outcome
	`
	// … exec with emptyToNull(rec.RollJSON), emptyToNull(rec.Location), emptyToNull(rec.Outcome) …

	const link = `INSERT OR IGNORE INTO turn_entities (turn_number, entity_id, mention, outcome) VALUES (?, ?, ?, ?)`
	// … exec with ref.EntityID, ref.Mention, emptyToNull(ref.Outcome) …
```

`GetTurn` and `ListTurns` select `COALESCE(location, '')` and `COALESCE(outcome, '')` into the new fields, and `ListEntitiesForTurn` selects `COALESCE(outcome, '')` into `TurnEntityRef.Outcome`.

Add the query the per-entity test needs:

```go
// ListTurnEntitiesByOutcome returns the turns where an entity's link carries a
// given system-reported outcome.
func (s *Store) ListTurnEntitiesByOutcome(entityID, outcome string) ([]int, error) {
	const query = `
	SELECT DISTINCT turn_number FROM turn_entities
	WHERE entity_id = ? AND outcome = ?
	ORDER BY turn_number`

	rows, err := s.db.Query(query, entityID, outcome)
	if err != nil {
		return nil, fmt.Errorf("list turns for entity %q with outcome %q: %w", entityID, outcome, err)
	}
	defer rows.Close()

	numbers := make([]int, 0)
	for rows.Next() {
		var number int
		if err := rows.Scan(&number); err != nil {
			return nil, err
		}
		numbers = append(numbers, number)
	}
	return numbers, rows.Err()
}
```

In `pkg/engine/history.go`, `Turn` gains the fields and loses `AudioRefs`:

```go
	Location  string            `json:"location,omitempty"`
	Outcome   string            `json:"outcome,omitempty"`
```

In `pkg/engine/timeline.go`, `turnRecord` maps them, and every link carries the turn's outcome:

```go
	rec := storage.TurnRecord{
		Number:    turn.Number,
		Timestamp: turn.Timestamp,
		Mode:      turn.Mode,
		Input:     turn.Input,
		Narration: turn.Prose(),
		Location:  turn.Location,
		Outcome:   turn.Outcome,
	}
	// … roll marshalling unchanged, AudioRefs block deleted …

	for _, mention := range turn.Entities {
		rec.Entities = append(rec.Entities, storage.TurnEntityRef{
			EntityID: mention.ID,
			Mention:  mention.Kind,
			Outcome:  turn.Outcome,
		})
	}
```

In `pkg/export/types.go`, delete `SceneBeat.AudioPath`. In `pkg/export/script.go`, delete the block that populated it. In `pkg/gui/types.go`, delete `TurnDTO.AudioURL`, and in `pkg/gui/service.go` delete the `audioURL` local and the field assignment in `GetChronicle`.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go build ./... && go test -count=1 ./...`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/storage pkg/engine pkg/export pkg/gui
git commit -m "feat(storage): record where a turn happened and how its check resolved"
```

---

## Phase 2: Player Location

### Task 3: Create the player note

**Files:**
- Modify: `pkg/engine/game.go` (`InitGame`)
- Test: `pkg/engine/game_test.go`

**Interfaces:**
- Consumes: `entity.SerializeMarkdown`, `ResolveStartLocation` (existing)
- Produces: `engine.playerNotePath(paths *core.PathResolver, gameID, player string) string`; the note itself

- [x] **Step 1: Write the failing test**

Append to `pkg/engine/game_test.go`:

```go
func TestInitGameCreatesThePlayerNote(t *testing.T) {
	tempDir := t.TempDir()
	paths := core.NewPathResolver(tempDir)

	sysDir := paths.SystemDir("d20-test")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: d20-test\nname: D20 Test\nversion: 1.0\n"), 0644)

	worldDir := paths.WorldDir("fantasy-realm")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: fantasy-realm\nname: Fantasy Realm\n"), 0644)

	session, err := InitGame(paths, "campaign-02", "d20-test", "fantasy-realm", "Sean O'Neill")
	if err != nil {
		t.Fatalf("InitGame failed: %v", err)
	}
	defer session.Close()

	// The slugified player name becomes the note's ID, so the path is predictable.
	// Punctuation is dropped rather than transliterated: the apostrophe in
	// "O'Neill" leaves "sean-oneill".
	notePath := filepath.Join(paths.GameDir("campaign-02"), "entities", "sean-oneill.md")
	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("expected the player note to exist: %v", err)
	}

	player, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("parse player note: %v", err)
	}
	if player.Name != "Sean O'Neill" || player.Type != "character" {
		t.Errorf("unexpected player note: %+v", player)
	}
	if player.Location == "" {
		t.Errorf("expected the player note to link the opening location")
	}

	indexed, err := session.Store.GetEntity(player.ID)
	if err != nil {
		t.Fatalf("expected the player in the index: %v", err)
	}
	if indexed.Location != player.Location {
		t.Errorf("index location = %q, note location = %q", indexed.Location, player.Location)
	}
}

func TestInitGameLeavesAnAuthoredPlayerNoteAlone(t *testing.T) {
	tempDir := t.TempDir()
	paths := core.NewPathResolver(tempDir)

	sysDir := paths.SystemDir("d20-test")
	os.MkdirAll(sysDir, 0755)
	os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: d20-test\nname: D20 Test\nversion: 1.0\n"), 0644)

	worldDir := paths.WorldDir("fantasy-realm")
	worldEntities := filepath.Join(worldDir, "entities")
	os.MkdirAll(worldEntities, 0755)
	os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: fantasy-realm\nname: Fantasy Realm\n"), 0644)
	authored := "---\nid: sean\nname: Sean\n type: character\n---\nHand written.\n"
	os.WriteFile(filepath.Join(worldEntities, "sean.md"), []byte(authored), 0644)

	session, err := InitGame(paths, "campaign-03", "d20-test", "fantasy-realm", "Sean")
	if err != nil {
		t.Fatalf("InitGame failed: %v", err)
	}
	defer session.Close()

	data, err := os.ReadFile(filepath.Join(paths.GameDir("campaign-03"), "entities", "sean.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Hand written.") {
		t.Errorf("expected the authored note to survive, got %s", data)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestInitGameCreatesThePlayerNote|TestInitGameLeavesAnAuthored" -count=1 ./pkg/engine/`
Expected: FAIL — the player note does not exist.

- [x] **Step 3: Implement**

In `pkg/engine/game.go`, after the start location is resolved and `manifest.Settings[StartLocationSetting]` is set, create the note when the file is absent:

```go
	if err := ensurePlayerNote(paths, store, gameID, playerName, startLocation); err != nil {
		return nil, fmt.Errorf("create player note: %w", err)
	}
```

and the helper:

```go
// ensurePlayerNote writes the campaign's player note when the file is absent,
// linking it to the opening location. An authored note is never touched.
func ensurePlayerNote(paths *core.PathResolver, store *storage.Store, gameID, playerName, locationID string) error {
	id := entity.Slugify(playerName)
	if id == "" {
		id = "player"
	}

	path := filepath.Join(paths.GameDir(gameID), "entities", id+".md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	player := &entity.Entity{
		ID:   id,
		Name: playerName,
		Type: "character",
		Body: "The player character.",
	}
	if locationID != "" {
		player.Location = "[[" + locationID + "]]"
	}

	data, err := player.SerializeMarkdown()
	if err != nil {
		return fmt.Errorf("serialize player note: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write player note: %w", err)
	}

	if _, err := storage.NewSyncer(store).SyncFile(path); err != nil {
		return fmt.Errorf("index player note: %w", err)
	}
	return nil
}
```

Pass `store` into the helper rather than reaching for the session, and keep it a plain function of `(paths, store, gameID, playerName, locationID)`. `InitGame` calls it before `ResolveStartLocation`? No — after, because the location is the point of the link. The note is written before the manifest is written so a failure leaves no half-built campaign manifest.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine/game.go pkg/engine/game_test.go
git commit -m "feat(engine): give every campaign a player note"
```

### Task 4: Read the location each turn and record it

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/orchestrator_location_test.go`

**Interfaces:**
- Consumes: `engine.findLocationByRef` (existing, `pkg/engine/startlocation.go`), `Timeline.history`
- Produces: `(*TurnOrchestrator).currentLocation() string`, `(*TurnOrchestrator).previousLocation() string`

- [x] **Step 1: Write the failing test**

`pkg/engine/orchestrator_location_test.go`:

```go
package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestProcessActionRecordsThePlayersLocation(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt air."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Warm."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID: "player", Name: "Sean", Type: "character", Body: "A traveller.",
		Location: "[[alden-tavern]]",
	})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(&mockOrchestratorModel{response: "The tavern hums."})
	router.AssignRole("gm", "mock-gm")

	o := NewTurnOrchestrator(store, timeline, nil, router, "aldon-harbour", "player")

	turn, err := o.ProcessAction(context.Background(), "Do", "I look around")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	if turn.Location != "alden-tavern" {
		t.Errorf("Location = %q, want the player's own note location", turn.Location)
	}

	indexed, err := store.GetTurn(turn.Number)
	if err != nil {
		t.Fatal(err)
	}
	if indexed.Location != "alden-tavern" {
		t.Errorf("indexed Location = %q, want alden-tavern", indexed.Location)
	}
}

func TestLocationFallsBackToThePinnedStartThenThePreviousTurn(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	history := NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, history, "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt air."})
	// A player whose location points at a note that does not exist is exactly the
	// typo case the fallback exists for.
	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID: "player", Name: "Sean", Type: "character", Body: "A traveller.",
		Location: "[[somewhere-that-was-deleted]]",
	})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(&mockOrchestratorModel{response: "Silence."})
	router.AssignRole("gm", "mock-gm")

	o := NewTurnOrchestrator(store, timeline, nil, router, "aldon-harbour", "player")

	first, err := o.ProcessAction(context.Background(), "Do", "I wait")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	if first.Location != "aldon-harbour" {
		t.Errorf("Location = %q, want the pinned start location", first.Location)
	}

	// Now give the campaign a history in a different place, break the note, and
	// confirm continuity wins over the pinned start: the party stays where the last
	// turn happened rather than teleporting back to where the campaign opened.
	if err := timeline.SetPlayerLocation("player", "alden-tavern"); err != nil {
		t.Fatalf("SetPlayerLocation failed: %v", err)
	}
	if _, err := o.ProcessAction(context.Background(), "Do", "I settle in"); err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	if err := timeline.SetPlayerLocation("player", "somewhere-that-was-deleted"); err != nil {
		t.Fatalf("SetPlayerLocation failed: %v", err)
	}

	third, err := o.ProcessAction(context.Background(), "Do", "I wait again")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	if third.Location != "alden-tavern" {
		t.Errorf("Location = %q, want the previous turn's location, not the pinned start", third.Location)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestProcessActionRecordsThePlayersLocation|TestLocationFallsBack" -count=1 ./pkg/engine/`
Expected: FAIL — `turn.Location` is empty and `timeline.SetPlayerLocation undefined`.

- [x] **Step 3: Implement**

Add to `pkg/engine/timeline.go` the durable location write:

```go
// SetPlayerLocation points a player note at a location, writing the note before
// the index so the Markdown stays the source of truth.
func (t *Timeline) SetPlayerLocation(playerID, locationID string) error {
	player, err := t.store.GetEntity(playerID)
	if err != nil || player == nil {
		return fmt.Errorf("player %q not found: %w", playerID, err)
	}

	player.Location = "[[" + locationID + "]]"
	return t.writeEntities(map[string]*entity.Entity{player.ID: player})
}

// SaveEntity persists one entity note and reindexes it. It lets collaborators
// outside the engine (the rules host bridge) write through the same path.
func (t *Timeline) SaveEntity(ent *entity.Entity) error {
	return t.writeEntities(map[string]*entity.Entity{ent.ID: ent})
}
```

In `pkg/engine/orchestrator.go`, replace the fixed location with a resolution chain and record it:

```go
// currentLocation resolves where this turn is happening. The player note wins
// because it is the single source of truth; the last recorded turn follows, so a
// note pointing at a deleted location cannot teleport the party back to where the
// campaign opened; the pinned start location is the bootstrap for a campaign with
// no history at all.
func (o *TurnOrchestrator) currentLocation() string {
	if o.playerID != "" {
		if player, err := o.store.GetEntity(o.playerID); err == nil && player != nil && player.Location != "" {
			if ent := findLocationByRef(o.store, player.Location); ent != nil {
				return ent.ID
			}
		}
	}

	if previous := o.previousLocation(); previous != "" {
		return previous
	}

	if o.startLocation != "" {
		if ent := findLocationByRef(o.store, o.startLocation); ent != nil {
			return ent.ID
		}
	}
	return ""
}

// previousLocation is the most recent location a turn recorded, scanning back past
// turns that predate location tracking.
func (o *TurnOrchestrator) previousLocation() string {
	turns, err := o.timeline.history.LoadHistory()
	if err != nil {
		return ""
	}

	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Location != "" {
			return turns[i].Location
		}
	}
	return ""
}
```

`ProcessAction` uses it once, before generation, and writes it onto the turn:

```go
	locationID := o.currentLocation()
	contextPrompt, err := o.assembler.AssembleContextWithProfiles(locationID, o.playerID, generationPrompt, o.rulesPrompt, o.lorePrompt, o.timeline.VoiceProfiles())
	// …
	turn := Turn{
		Number:    turnNum,
		Timestamp: time.Now(),
		Mode:      mode,
		Input:     actionInput,
		Roll:      rollRes,
		Narration: resp.Text,
		Location:  locationID,
	}

	turn.Entities = harness.ResolveEntityMentions(o.store, o.playerID, locationID, turn.Narration, actionInput)
```

The orchestrator's field is renamed `startLocation` and is only ever the bootstrap
value: it is consulted after the player note and after the last recorded turn, so a
campaign that has played a turn never snaps back to where it opened.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/orchestrator_location_test.go pkg/engine/timeline.go cmd/localrpg/play.go
git commit -m "feat(engine): record where each turn happens, not where the campaign began"
```

### Task 5: Move with `/go`

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/orchestrator_location_test.go`

**Interfaces:**
- Consumes: `Timeline.SetPlayerLocation`, `Timeline.RecordTurn`, `findLocationByRef`
- Produces: `/go <location>` handling in `ProcessAction`

- [x] **Step 1: Write the failing test**

Append to `pkg/engine/orchestrator_location_test.go`:

```go
func TestGoMovesThePlayerAndRecordsASystemTurn(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Warm."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "alden-harbour", Name: "Alden Harbour", Type: "location", Body: "Salt air."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller.", Location: "[[alden-tavern]]"})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(&mockOrchestratorModel{response: "unused"})
	router.AssignRole("gm", "mock-gm")

	o := NewTurnOrchestrator(store, timeline, nil, router, "alden-tavern", "player")

	move, err := o.ProcessAction(context.Background(), "System", "/go Alden Harbour")
	if err != nil {
		t.Fatalf("/go failed: %v", err)
	}
	if move.Mode != "System" || move.Location != "alden-harbour" {
		t.Errorf("unexpected move turn: %+v", move)
	}

	player, err := store.GetEntity("player")
	if err != nil {
		t.Fatal(err)
	}
	if player.Location != "[[alden-harbour]]" {
		t.Errorf("player location = %q, want [[alden-harbour]]", player.Location)
	}

	// The move is part of the timeline, not a side effect.
	recorded, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 || recorded[0].Location != "alden-harbour" {
		t.Errorf("expected the move recorded with its destination, got %+v", recorded)
	}

	if _, err := o.ProcessAction(context.Background(), "System", "/go Nowhere At All"); err == nil {
		t.Errorf("expected an error for an unknown location")
	}
	if _, err := o.ProcessAction(context.Background(), "System", "/go player"); err == nil {
		t.Errorf("expected an error when the target is not a location")
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestGoMovesThePlayer -count=1 ./pkg/engine/`
Expected: FAIL — `/go Alden Harbour` is treated as an action and no move is recorded.

- [x] **Step 3: Implement**

In `ProcessAction`, next to the `/undo` branch:

```go
	// Handle /go command
	if strings.HasPrefix(strings.TrimSpace(actionInput), "/go ") {
		target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(actionInput), "/go "))
		ent := findLocationByRef(o.store, target)
		if ent == nil {
			return nil, fmt.Errorf("unknown location %q", target)
		}

		if err := o.timeline.SetPlayerLocation(o.playerID, ent.ID); err != nil {
			return nil, fmt.Errorf("move failed: %w", err)
		}

		// The player is a party to their own move, so the move records them the way
		// every generated turn does; without this the move would be the one turn
		// whose player involvement is missing from the timeline.
		moveEntities := make([]entity.Mention, 0, 2)
		if o.playerID != "" {
			moveEntities = append(moveEntities, entity.Mention{ID: o.playerID, Kind: entity.MentionPlayer})
		}
		moveEntities = append(moveEntities, entity.Mention{ID: ent.ID, Kind: entity.MentionLocation})

		move := Turn{
			Number:    turnNum,
			Timestamp: time.Now(),
			Mode:      "System",
			Input:     actionInput,
			Narration: fmt.Sprintf("You make your way to %s.", ent.Name),
			Location:  ent.ID,
			Entities:  moveEntities,
		}
		if err := o.timeline.RecordTurn(&move, nil); err != nil {
			return nil, fmt.Errorf("record move: %w", err)
		}
		return &move, nil
	}
```

The unresolvable-target and non-location cases are both covered by `findLocationByRef`, which only ever returns entities of type `location`. This adds `github.com/darkliquid/localrpg/pkg/entity` to `pkg/engine/orchestrator.go`'s imports, which it does not currently need.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/orchestrator_location_test.go
git commit -m "feat(engine): add a /go command that moves the player and records it"
```

### Task 6: Let rules move the player

**Files:**
- Modify: `pkg/rules/host_api.go` (`EntityWriter`, `SetLocation`, `GetLocation`, constructor)
- Modify: `pkg/rules/js_engine.go` (`setLocation`, `getLocation` bindings)
- Modify: `pkg/engine/game.go` or `cmd/localrpg/play.go` (construct the bridge with the writer and player)
- Test: `pkg/rules/host_api_test.go`

**Interfaces:**
- Consumes: `Timeline.SaveEntity` (Task 4)
- Produces: `rules.EntityWriter`, `rules.NewHostBridge(store *storage.Store, writer EntityWriter, playerID string) *DefaultHostBridge`, `(*DefaultHostBridge).SetLocation(string) error`, `(*DefaultHostBridge).GetLocation() (string, error)`; JS `setLocation(id)`, `getLocation()`

- [x] **Step 1: Write the failing test**

Append to `pkg/rules/host_api_test.go`:

```go
type recordingWriter struct {
	saved []*entity.Entity
}

func (w *recordingWriter) SaveEntity(ent *entity.Entity) error {
	w.saved = append(w.saved, ent)
	return nil
}

func TestHostBridgeMovesThePlayerThroughItsWriter(t *testing.T) {
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.SaveEntity(&entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Hash: "h1"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "alden-harbour", Name: "Alden Harbour", Type: "location", Hash: "h2"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "player", Name: "Sean", Type: "character", Hash: "h3", Location: "[[alden-tavern]]"}); err != nil {
		t.Fatal(err)
	}

	writer := &recordingWriter{}
	bridge := NewHostBridge(store, writer, "player")

	current, err := bridge.GetLocation()
	if err != nil {
		t.Fatalf("GetLocation failed: %v", err)
	}
	if current != "alden-tavern" {
		t.Errorf("GetLocation = %q, want alden-tavern", current)
	}

	if err := bridge.SetLocation("alden-harbour"); err != nil {
		t.Fatalf("SetLocation failed: %v", err)
	}
	if len(writer.saved) != 1 || writer.saved[0].Location != "[[alden-harbour]]" {
		t.Fatalf("expected the move to go through the writer, got %+v", writer.saved)
	}

	if err := bridge.SetLocation("nowhere"); err == nil {
		t.Errorf("expected an error for an unknown location")
	}
	if err := bridge.SetLocation("player"); err == nil {
		t.Errorf("expected an error when the target is not a location")
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestHostBridgeMovesThePlayer -count=1 ./pkg/rules/`
Expected: FAIL — `too many arguments in call to NewHostBridge`

- [x] **Step 3: Implement**

In `pkg/rules/host_api.go`:

```go
// EntityWriter persists an entity to its note and the index. It is how the host
// bridge writes without duplicating the engine's write path.
type EntityWriter interface {
	SaveEntity(ent *entity.Entity) error
}

type GameHostAPI interface {
	// … existing methods …
	SetLocation(locationID string) error
	GetLocation() (string, error)
}

type DefaultHostBridge struct {
	store      *storage.Store
	writer     EntityWriter
	playerID   string
	directives []string
	logs       []string
}

func NewHostBridge(store *storage.Store, writer EntityWriter, playerID string) *DefaultHostBridge {
	return &DefaultHostBridge{
		store:      store,
		writer:     writer,
		playerID:   playerID,
		directives: make([]string, 0),
		logs:       make([]string, 0),
	}
}

// SetLocation moves the player, writing through the same path the engine uses.
func (h *DefaultHostBridge) SetLocation(locationID string) error {
	if h.playerID == "" {
		return fmt.Errorf("set location: no player configured")
	}

	target, err := h.store.GetEntity(locationID)
	if err != nil || target == nil {
		return fmt.Errorf("set location: unknown location %q", locationID)
	}
	if target.Type != "location" {
		return fmt.Errorf("set location: %q is a %s, not a location", locationID, target.Type)
	}

	player, err := h.store.GetEntity(h.playerID)
	if err != nil || player == nil {
		return fmt.Errorf("set location: player %q not found", h.playerID)
	}

	player.Location = "[[" + locationID + "]]"
	if h.writer != nil {
		return h.writer.SaveEntity(player)
	}
	return h.store.SaveEntity(player)
}

// GetLocation returns the player's current location ID.
func (h *DefaultHostBridge) GetLocation() (string, error) {
	if h.playerID == "" {
		return "", fmt.Errorf("get location: no player configured")
	}

	player, err := h.store.GetEntity(h.playerID)
	if err != nil || player == nil {
		return "", fmt.Errorf("get location: player %q not found", h.playerID)
	}
	if player.Location == "" {
		return "", nil
	}
	return entity.Slugify(entity.WikilinkTarget(player.Location)), nil
}
```

`SetStat` is switched to the writer as well, which fixes a pre-existing divergence: writing state through `store.SaveEntity` never reached the Markdown note, so the next file-driven sync could revert it.

In `js_engine.go`'s `bindHostAPI`, following the `getStat`/`setStat` pattern:

```go
	j.vm.Set("setLocation", func(call goja.FunctionCall) goja.Value {
		locationID := call.Argument(0).String()
		if err := j.bridge.SetLocation(locationID); err != nil {
			panic(j.vm.ToValue(fmt.Sprintf("setLocation error: %v", err)))
		}
		return goja.Undefined()
	})

	j.vm.Set("getLocation", func(call goja.FunctionCall) goja.Value {
		locationID, err := j.bridge.GetLocation()
		if err != nil {
			panic(j.vm.ToValue(fmt.Sprintf("getLocation error: %v", err)))
		}
		return j.vm.ToValue(locationID)
	})
```

Update every construction site to pass the writer and player: `cmd/localrpg/play.go`, `pkg/tui/app_test.go`, the existing `pkg/rules/*_test.go` cases that build a bridge for the JS engine, and any other fixture the compiler flags. `play.go` builds the bridge after the timeline exists:

```go
	bridge := rules.NewHostBridge(store, timeline, manifest.Player)
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/rules/ ./pkg/engine/ ./pkg/tui/ ./cmd/localrpg/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/rules pkg/engine cmd/localrpg pkg/tui
git commit -m "feat(rules): let a system move the player and read their location"
```

### Task 7: Accept a gated location proposed by extraction

**Files:**
- Modify: `pkg/harness/extractor.go` (`Extraction.PlayerLocation`, prompt)
- Modify: `pkg/engine/orchestrator.go` (apply the proposal)
- Test: `pkg/harness/extractor_test.go`, `pkg/engine/orchestrator_location_test.go`

**Interfaces:**
- Consumes: `Timeline.SetPlayerLocation`, `findLocationByRef`
- Produces: `harness.Extraction.PlayerLocation string` (JSON `player_location`)

- [x] **Step 1: Write the failing tests**

Append to `pkg/harness/extractor_test.go`:

```go
func TestExtractorReturnsAProposedPlayerLocation(t *testing.T) {
	model := &mockProvider{
		id: "extractor-model",
		output: `{
			"entities": [],
			"dialogue": [],
			"player_location": "[[Alden Harbour]]"
		}`,
	}

	result, err := NewExtractor(model).Extract(context.Background(), "You walk down to the harbour.")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if result.PlayerLocation != "[[Alden Harbour]]" {
		t.Errorf("PlayerLocation = %q, want the proposed reference", result.PlayerLocation)
	}

	// The bare-array shape stays valid and carries no proposal.
	legacy := &mockProvider{id: "extractor-model", output: `[]`}
	result, err = NewExtractor(legacy).Extract(context.Background(), "Nothing happens.")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if result.PlayerLocation != "" {
		t.Errorf("PlayerLocation = %q, want empty for the legacy shape", result.PlayerLocation)
	}
}
```

Append to `pkg/engine/orchestrator_location_test.go`:

```go
func TestExtractorProposedLocationAppliesOnlyWhenItResolves(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Warm."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "alden-harbour", Name: "Alden Harbour", Type: "location", Body: "Salt air."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller.", Location: "[[alden-tavern]]"})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(&mockOrchestratorModel{response: "You reach the water's edge."})
	router.AssignRole("gm", "mock-gm")

	o := NewTurnOrchestrator(store, timeline, nil, router, "alden-tavern", "player")
	o.SetExtractor(harness.NewExtractor(&mockTimelineModel{
		response: `{"entities":[],"dialogue":[],"player_location":"[[Alden Harbour]]"}`,
	}))

	turn, err := o.ProcessAction(context.Background(), "Do", "I walk to the harbour")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	if turn.Location != "alden-tavern" {
		t.Errorf("the turn happened where it started, got %q", turn.Location)
	}

	player, err := store.GetEntity("player")
	if err != nil {
		t.Fatal(err)
	}
	if player.Location != "[[alden-harbour]]" {
		t.Errorf("player location = %q, want the applied move", player.Location)
	}

	// A proposal that does not resolve is ignored rather than teleporting anyone.
	o.SetExtractor(harness.NewExtractor(&mockTimelineModel{
		response: `{"entities":[],"dialogue":[],"player_location":"[[Place That Does Not Exist]]"}`,
	}))
	if _, err := o.ProcessAction(context.Background(), "Do", "I walk somewhere odd"); err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	player, err = store.GetEntity("player")
	if err != nil {
		t.Fatal(err)
	}
	if player.Location != "[[alden-harbour]]" {
		t.Errorf("an unresolvable proposal must be ignored, got %q", player.Location)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestExtractorReturnsAProposedPlayerLocation|TestExtractorProposedLocationApplies" -count=1 ./pkg/harness/ ./pkg/engine/`
Expected: FAIL — `result.PlayerLocation undefined`

- [x] **Step 3: Implement**

In `pkg/harness/extractor.go`:

```go
// Extraction is everything one extraction pass returned for a turn.
type Extraction struct {
	Entities       []ExtractedEntity   `json:"entities"`
	Dialogue       []ExtractedDialogue `json:"dialogue,omitempty"`
	PlayerLocation string              `json:"player_location,omitempty"`
}
```

and the prompt gains one line, after the dialogue instruction:

```
If the narration moves the player to a different place, set "player_location" to a [[wikilink]] of that location; otherwise omit it.
```

In `pkg/engine/orchestrator.go`, apply the proposal after extraction and before recording, taking effect from the next turn:

```go
	if ref := strings.TrimSpace(extraction.PlayerLocation); ref != "" {
		if ent := findLocationByRef(o.store, ref); ent != nil && ent.ID != locationID {
			if err := o.timeline.SetPlayerLocation(o.playerID, ent.ID); err != nil {
				return nil, fmt.Errorf("apply proposed location: %w", err)
			}
		}
	}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/harness/ ./pkg/engine/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness/extractor.go pkg/harness/extractor_test.go pkg/engine/orchestrator.go pkg/engine/orchestrator_location_test.go
git commit -m "feat(engine): let extraction propose a move the engine then verifies"
```

<!-- PLAN-CONTINUES -->

## Phase 3: Check Outcomes

### Task 8: Let a system report its own outcome

**Files:**
- Modify: `pkg/rules/host_api.go` (`ActionResult`)
- Modify: `pkg/rules/js_engine.go` (`ExecuteAction` reserved key)
- Test: `pkg/rules/js_engine_test.go`

**Interfaces:**
- Consumes: nothing new
- Produces: `rules.ActionResult.Outcome string` (JSON `outcome`)

- [x] **Step 1: Write the failing test**

Append to `pkg/rules/js_engine_test.go`:

```go
func TestExecuteActionReadsTheOutcomeLabel(t *testing.T) {
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	engine := NewJSEngine(NewHostBridge(store, nil, ""))
	if err := engine.LoadScript(`
		onAction("attack", (ctx) => ({ success: false, outcome: "glancing_blow", message: "A glancing blow." }));
		onAction("parley", (ctx) => ({ outcome: "uneasy_truce" }));
	`); err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	res, err := engine.ExecuteAction("attack", map[string]interface{}{"action": "swing"})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}
	if res.Outcome != "glancing_blow" {
		t.Errorf("Outcome = %q, want glancing_blow", res.Outcome)
	}
	if res.Success {
		t.Errorf("expected success to stay false")
	}

	// A label alone does not imply success: the engine never invents semantics.
	truce, err := engine.ExecuteAction("parley", map[string]interface{}{"action": "talk"})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}
	if truce.Outcome != "uneasy_truce" || truce.Success {
		t.Errorf("expected the label with success false, got %+v", truce)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestExecuteActionReadsTheOutcomeLabel -count=1 ./pkg/rules/`
Expected: FAIL — `res.Outcome undefined`

- [x] **Step 3: Implement**

In `pkg/rules/host_api.go`:

```go
type ActionResult struct {
	Success bool                   `json:"success"`
	Outcome string                 `json:"outcome,omitempty"`
	Message string                 `json:"message"`
	Roll    *RollResult            `json:"roll,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}
```

In `pkg/rules/js_engine.go`'s `ExecuteAction`, after the `success` key is read and before the `Data` sweep:

```go
	if outcome, ok := m["outcome"].(string); ok {
		res.Outcome = outcome
	}
```

The `Data` sweep already excludes reserved keys; add `outcome` to that exclusion list so it does not appear twice.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/rules/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/rules/host_api.go pkg/rules/js_engine.go pkg/rules/js_engine_test.go
git commit -m "feat(rules): let a system name its own check outcome"
```

### Task 9: Carry the outcome through the turn and out to the API

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`
- Modify: `frontend/src/types.ts`, `frontend/src/components/ChronicleView.tsx`
- Test: `pkg/engine/orchestrator_outcome_test.go`

**Interfaces:**
- Consumes: `rules.ActionResult.Outcome` (Task 8), `storage.ListTurnEntitiesByOutcome` (Task 2)
- Produces: `Turn.Outcome`, `TurnDTO.Outcome`, `Turn.outcome` in the frontend types

- [x] **Step 1: Write the failing test**

`pkg/engine/orchestrator_outcome_test.go`:

```go
package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestProcessActionRecordsTheSystemsOutcomeLabel(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt air."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller.", Location: "[[aldon-harbour]]"})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	jsEngine := rules.NewJSEngine(rules.NewHostBridge(store, timeline, "player"))
	if err := jsEngine.LoadScript(`
		onAction("attack", (ctx) => ({ success: false, outcome: "glancing_blow", message: "A glancing blow." }));
	`); err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(&mockOrchestratorModel{response: "Steel skitters off the wall."})
	router.AssignRole("gm", "mock-gm")

	o := NewTurnOrchestrator(store, timeline, jsEngine, router, "aldon-harbour", "player")

	turn, err := o.ProcessAction(context.Background(), "Attack", "I swing at the cultist")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	if turn.Outcome != "glancing_blow" {
		t.Fatalf("Outcome = %q, want glancing_blow", turn.Outcome)
	}

	// The label reaches the index both on the turn and on every link.
	rec, err := store.GetTurn(turn.Number)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Outcome != "glancing_blow" {
		t.Errorf("indexed Outcome = %q", rec.Outcome)
	}

	turns, err := store.ListTurnEntitiesByOutcome("player", "glancing_blow")
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0] != turn.Number {
		t.Errorf("expected the player's link to carry the outcome, got %v", turns)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestProcessActionRecordsTheSystemsOutcomeLabel -count=1 ./pkg/engine/`
Expected: FAIL — `turn.Outcome` empty.

- [x] **Step 3: Implement**

In `ProcessAction`, alongside `var rollRes *rules.RollResult`:

```go
	var outcome string
```

in the mechanics branch:

```go
		if err == nil && res != nil {
			rollRes = res.Roll
			outcome = res.Outcome
			if res.Message != "" {
				gmDirective = fmt.Sprintf("[MECHANICS RESULT: %s]", res.Message)
			}
		}
```

and on the turn:

```go
		Narration: resp.Text,
		Location:  locationID,
		Outcome:   outcome,
```

In `pkg/gui/types.go`, `TurnDTO` gains `Outcome string \`json:"outcome,omitempty"\``; in `pkg/gui/service.go`'s `GetChronicle`, map `Outcome: turn.Outcome`.

In `frontend/src/types.ts`, `Turn` gains `outcome?: string`. In `ChronicleView.tsx`, render the label beside the mode tag when present:

```tsx
                {turn.outcome && (
                  <span className="text-xs font-mono text-stone-400">{turn.outcome}</span>
                )}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/ ./pkg/gui/ && cd frontend && npx tsc --noEmit`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine pkg/gui frontend/src
git commit -m "feat(engine): record how each check resolved, per turn and per entity"
```

---

## Phase 4: The Extraction Role

### Task 10: A first-class extractor role that inherits until overridden

**Files:**
- Modify: `pkg/config/types.go` (`InheritFrom`, `inherit`, default role)
- Modify: `cmd/localrpg/play.go` (`resolveExtractor`)
- Test: `cmd/localrpg/play_resolver_test.go`

**Interfaces:**
- Consumes: `harness.NewModelProvider`, `harness.Router.GetProviderForRole`
- Produces: `config.AgentRoleConfig.InheritFrom string`; `resolveExtractor` honouring `inherit` and `disabled`

- [x] **Step 1: Write the failing test**

`cmd/localrpg/play_resolver_test.go`:

```go
package main

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func routerWithGM(t *testing.T) *harness.Router {
	t.Helper()

	router := harness.NewRouter()
	router.RegisterProvider(harness.NewCLIProvider("gm", "echo", []string{}))
	router.AssignRole(config.RoleGM, "gm")
	return router
}

func TestResolveExtractorInheritsTheNamedRole(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{
		Type:        "inherit",
		InheritFrom: config.RoleGM,
	}

	if resolveExtractor(cfg, routerWithGM(t)) == nil {
		t.Fatalf("expected an extractor inheriting gm")
	}
}

func TestResolveExtractorDefaultsToInheritingGMWhenUnset(t *testing.T) {
	cfg := config.DefaultConfig()
	delete(cfg.Agents.Roles, config.RoleExtractor)

	if resolveExtractor(cfg, routerWithGM(t)) == nil {
		t.Fatalf("an unset extractor role must keep working as it did before")
	}
}

func TestResolveExtractorDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{Type: "disabled"}

	if resolveExtractor(cfg, routerWithGM(t)) != nil {
		t.Errorf("expected a disabled role to produce no extractor")
	}
}

func TestResolveExtractorWithAMissingInheritTarget(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{
		Type:        "inherit",
		InheritFrom: "narrator",
	}
	cfg.Agents.Roles["narrator"] = config.AgentRoleConfig{Type: "disabled"}

	if resolveExtractor(cfg, routerWithGM(t)) != nil {
		t.Errorf("expected a missing inherit target to disable extraction")
	}
}

func TestResolveExtractorUsesAConcreteProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{Type: "builtin"}

	if resolveExtractor(cfg, routerWithGM(t)) == nil {
		t.Errorf("expected a configured builtin extractor")
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestResolveExtractor -count=1 ./cmd/localrpg/`
Expected: FAIL — `cfg.Agents.Roles[config.RoleExtractor]` has no `InheritFrom`, and `inherit` falls through to the provider factory.

- [x] **Step 3: Implement**

In `pkg/config/types.go`:

```go
type AgentRoleConfig struct {
	Type        string   `yaml:"type" json:"type"` // "builtin", "http", "cli", "inherit", "disabled"
	InheritFrom string   `yaml:"inherit_from,omitempty" json:"inherit_from,omitempty"`
	// … unchanged fields …
}
```

and `DefaultConfig` seeds the role so it is visible and editable:

```go
			Roles: map[string]AgentRoleConfig{
				RoleGM: {
					Type:        "cli",
					Command:     "echo",
					Args:        []string{},
					Temperature: 0.7,
					MaxTokens:   1024,
				},
				RoleNarrator: {
					Type: "disabled",
				},
				RoleExtractor: {
					Type:        "inherit",
					InheritFrom: RoleGM,
				},
			},
```

In `cmd/localrpg/play.go`, `resolveExtractor` becomes a switch on the resolved role:

```go
// resolveExtractor picks the provider used for per-turn entity extraction. An
// absent role still inherits gm, so configuration written before this role
// existed keeps working; `disabled` opts out; `inherit` follows the named role,
// which is what stops extraction silently pointing at a stale copy of gm.
func resolveExtractor(cfg *config.Config, router *harness.Router) *harness.Extractor {
	roleCfg, configured := cfg.Agents.Roles[config.RoleExtractor]
	if !configured {
		roleCfg = config.AgentRoleConfig{Type: "inherit", InheritFrom: config.RoleGM}
	}

	switch roleCfg.Type {
	case "disabled":
		return nil
	case "inherit", "":
		source := roleCfg.InheritFrom
		if source == "" {
			source = config.RoleGM
		}
		provider, err := router.GetProviderForRole(source)
		if err != nil {
			return nil
		}
		return harness.NewExtractor(provider)
	}

	provider, err := harness.NewModelProvider(config.RoleExtractor, harness.ProviderConfig{
		Type:        roleCfg.Type,
		BuiltinName: roleCfg.BuiltinName,
		Command:     roleCfg.Command,
		Args:        roleCfg.Args,
		Endpoint:    roleCfg.Endpoint,
		Model:       roleCfg.Model,
		APIKey:      roleCfg.APIKey,
		Temperature: roleCfg.Temperature,
		MaxTokens:   roleCfg.MaxTokens,
	})
	if err != nil {
		return nil
	}
	return harness.NewExtractor(provider)
}
```

The router loop in `play.go` must not try to build a provider for an `inherit` role itself; skip `inherit` there, since it is resolved through the router by name.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./cmd/localrpg/ ./pkg/config/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/config/types.go cmd/localrpg/play.go cmd/localrpg/play_resolver_test.go
git commit -m "feat(config): make the extractor a role you can see and aim"
```

### Task 11: Settings UI for roles, including the new one

**Files:**
- Modify: `frontend/src/types.ts` (`AgentRoleConfig.inherit_from`)
- Modify: `frontend/src/components/SettingsStudio.tsx`
- Test: `cd frontend && npx tsc --noEmit`

**Interfaces:**
- Consumes: `AgentsConfig.roles` from the API (already sent)
- Produces: a role list derived from config; an `inherit` provider type with an `inherit_from` selector

- [x] **Step 1: Write the failing check**

There is no frontend test runner, so the check is the type checker plus the existing manual path. Introduce the change and let the compiler find the fallout: replace the hardcoded union at `SettingsStudio.tsx:48`

```tsx
  const [selectedRole, setSelectedRole] = useState<'gm' | 'narrator' | 'evaluator'>('gm');
```

with

```tsx
  const [selectedRole, setSelectedRole] = useState<string>('gm');
```

Then run the type check:

Run: `cd frontend && npx tsc --noEmit`
Expected: FAIL if any code depended on the union, which is the point of starting here.

- [x] **Step 2: Implement the role list**

Replace the hardcoded options at `SettingsStudio.tsx:288-290` with a derived list, add the labels and the per-role default, near the top of the component:

```tsx
const ROLE_LABELS: Record<string, string> = {
  gm: 'Game Master (GM / Storyteller)',
  narrator: 'Atmospheric Narrator',
  extractor: 'Entity Extractor (per-turn world state)',
};

// A missing role falls back to inheriting gm for the extractor, which is what
// makes extraction work out of the box without a second configuration step.
const defaultRoleConfig = (role: string): AgentRoleConfig =>
  role === 'extractor' ? { type: 'inherit', inherit_from: 'gm' } : { type: 'disabled' };
```

```tsx
  const roleNames = Array.from(new Set([...Object.keys(config.agents.roles), 'extractor']));
```

and the selector body:

```tsx
                  {roleNames.map((role) => (
                    <option key={role} value={role}>
                      {ROLE_LABELS[role] ?? role}
                    </option>
                  ))}
```

`evaluator` disappears with the hardcoded list: no backend code has ever routed it.

- [x] **Step 3: Implement `inherit` and the cost hint**

At `SettingsStudio.tsx:114`, default a missing role through the helper:

```tsx
  const currentRoleConfig: AgentRoleConfig = config.agents.roles[selectedRole] || defaultRoleConfig(selectedRole);
```

Add to the provider-type select (beside the `disabled`/`http`/`cli`/`builtin` options):

```tsx
                    <option value="inherit">Inherit from another role</option>
```

and, immediately after that select, a conditional block:

```tsx
                {currentRoleConfig.type === 'inherit' && (
                  <div className="space-y-1.5">
                    <label className="text-xs font-cinzel uppercase text-stone-300">Inherit From</label>
                    <select
                      value={currentRoleConfig.inherit_from || 'gm'}
                      onChange={(e) => updateRole({ type: 'inherit', inherit_from: e.target.value })}
                      className="w-full bg-stone-950 border border-stone-800 rounded-lg px-2.5 py-1 text-xs text-amber-300 font-mono focus:outline-none"
                    >
                      {roleNames.map((role) => (
                        <option key={role} value={role}>
                          {ROLE_LABELS[role] ?? role}
                        </option>
                      ))}
                    </select>
                    <p className="text-[11px] text-stone-500">
                      Resolves to {config.agents.roles[currentRoleConfig.inherit_from || 'gm']?.model ||
                        config.agents.roles[currentRoleConfig.inherit_from || 'gm']?.command ||
                        'the default provider'}
                      , so changing that role changes this one until you override it here.
                    </p>
                  </div>
                )}
```

where `updateRole` is the existing pattern already used by the other fields:

```tsx
  const updateRole = (updated: AgentRoleConfig) => {
    setConfig({
      ...config,
      agents: {
        ...config.agents,
        roles: { ...config.agents.roles, [selectedRole]: updated },
      },
    });
  };
```

In `frontend/src/types.ts`, `AgentRoleConfig` gains `inherit_from?: string;`.

- [x] **Step 4: Verify the type check and the build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): configure every agent role, including the extractor"
```

<!-- PLAN-CONTINUES -->

## Phase 5: Speech Attribution

### Task 12: Parse the forms a GM actually writes

**Files:**
- Modify: `pkg/dialogue/dialogue.go`
- Test: `pkg/dialogue/dialogue_test.go`

**Interfaces:**
- Consumes: `entity.WikilinkTarget`
- Produces: `dialogue.Parse` accepting emphasis and wikilink speaker wrappers, splitting trailing prose

- [x] **Step 1: Write the failing test**

Append to `pkg/dialogue/dialogue_test.go`:

```go
func TestParseAcceptsTheFormsAGMWrites(t *testing.T) {
	resolve := func(candidate string) (string, bool) {
		if candidate == "Garrick the Fence" {
			return "garrick-the-fence", true
		}
		return "", false
	}

	cases := map[string]string{
		`Garrick the Fence: "Plain."`:                        "Plain.",
		`**Garrick the Fence:** "Bold."`:                      "Bold.",
		`*Garrick the Fence:* "Italic."`:                      "Italic.",
		`[[Garrick the Fence]]: "Wikilink."`:                  "Wikilink.",
		`[[Garrick the Fence|Garrick]]: "Labelled."`:          "Labelled.",
		"\u201cGarrick the Fence\u201d: \u201cSmart quotes.\u201d": "Smart quotes.",
	}

	for line, want := range cases {
		segments := Parse(line, resolve)
		if len(segments) != 1 || !segments[0].IsSpeech {
			t.Errorf("%q produced %#v, want one speech segment", line, segments)
			continue
		}
		if segments[0].Text != want || segments[0].SpeakerID != "garrick-the-fence" {
			t.Errorf("%q produced %#v, want %q", line, segments[0], want)
		}
	}
}

func TestParseKeepsProseAfterASpokenLine(t *testing.T) {
	resolve := func(candidate string) (string, bool) { return "garrick-the-fence", true }

	segments := Parse(`Garrick the Fence: "Keep walking." He turns away.`, resolve)

	if len(segments) != 2 {
		t.Fatalf("expected a speech segment and a narration segment, got %#v", segments)
	}
	if !segments[0].IsSpeech || segments[0].Text != "Keep walking." {
		t.Errorf("unexpected speech segment %#v", segments[0])
	}
	if segments[1].IsSpeech || segments[1].Text != "He turns away." {
		t.Errorf("unexpected trailing segment %#v", segments[1])
	}
}

func TestParseRejectsImpossibleSpeakers(t *testing.T) {
	resolve := func(candidate string) (string, bool) { return "", false }

	long := strings.Repeat("a", 70)
	for _, line := range []string{
		long + `: "Too long to be a name."`,
		`: "No name at all."`,
		`Garrick the Fence: "Unterminated.`,
	} {
		segments := Parse(line, resolve)
		for _, segment := range segments {
			if segment.IsSpeech {
				t.Errorf("%q produced a speech segment %#v", line, segment)
			}
		}
	}
}
```

Add `"strings"` to the test file's imports.

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestParseAcceptsTheForms|TestParseKeepsProseAfter|TestParseRejectsImpossible" -count=1 ./pkg/dialogue/`
Expected: FAIL — bold, italic, and trailing-prose cases are not recognised.

- [x] **Step 3: Implement**

Replace the parser in `pkg/dialogue/dialogue.go`:

```go
// Emphasis can sit between the colon and the quote, as in `**Name:** "…"`, so the
// separator allows whitespace and emphasis characters.
var attributedSpeakerRegex = regexp.MustCompile(`^([^:\n]+):[\s*_]*["“]([^"”]+)["”](.*)$`)

// maxSpeakerLength bounds a candidate so a sentence cannot masquerade as a name.
const maxSpeakerLength = 64

// Parse splits text into ordered narration and speech segments. resolve maps a
// candidate speaker to an entity ID, reporting whether the speaker is known. A
// candidate that does not resolve stays narration, so prose such as
// `As you declare: "I draw my blade"` never invents a character.
func Parse(text string, resolve func(candidate string) (string, bool)) []Segment {
	segments := make([]Segment, 0)

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if match := attributedSpeakerRegex.FindStringSubmatch(line); len(match) == 4 {
			candidate := cleanSpeaker(match[1])
			if candidate != "" {
				if id, ok := resolve(candidate); ok {
					segments = append(segments, Segment{
						Speaker:   candidate,
						SpeakerID: id,
						Text:      strings.TrimSpace(match[2]),
						IsSpeech:  true,
					})
					if rest := strings.TrimSpace(match[3]); rest != "" {
						segments = append(segments, Segment{Text: rest})
					}
					continue
				}
			}
		}

		segments = append(segments, Segment{Text: line})
	}

	return segments
}

// cleanSpeaker strips markdown emphasis and a wikilink label from a candidate,
// returning "" when what remains cannot be a speaker name. Resolution, not
// punctuation, is what rejects sentence fragments, so titles such as
// "Mr. Garrick" survive.
func cleanSpeaker(raw string) string {
	// Wrapping emphasis and quotes are formatting, not part of the name.
	candidate := entity.WikilinkTarget(strings.Trim(strings.TrimSpace(raw), "*_\"'“”‘’"))
	if candidate == "" || len(candidate) > maxSpeakerLength {
		return ""
	}
	return candidate
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/dialogue/ ./pkg/engine/`
Expected: PASS, including the retained `As you declare` regression in the engine's segment tests.

- [x] **Step 5: Commit**

```bash
git add pkg/dialogue
git commit -m "feat(dialogue): parse the speaker forms a GM actually writes"
```

### Task 13: Tell the GM how to write speech

**Files:**
- Modify: `pkg/harness/context.go`
- Test: `pkg/harness/context_test.go`

**Interfaces:**
- Consumes: `AssembleContextWithProfiles` (existing)
- Produces: the instruction appears in every assembled prompt

- [x] **Step 1: Write the failing test**

Append to `pkg/harness/context_test.go`:

```go
func TestAssembleContextAlwaysAsksForAttributableSpeech(t *testing.T) {
	store := newTestEntityStore(t)
	assembler := NewContextAssembler(store)

	// No rules prompt, no lore prompt: the instruction must not depend on a
	// system or world shipping anything.
	prompt, err := assembler.AssembleContextWithProfiles("", "", "I listen", "", "", nil)
	if err != nil {
		t.Fatalf("AssembleContextWithProfiles failed: %v", err)
	}

	if !strings.Contains(prompt, "## SPEECH FORMATTING") {
		t.Errorf("expected the speech formatting section, got %q", prompt)
	}
	if !strings.Contains(prompt, `Name: "the words spoken"`) {
		t.Errorf("expected the instruction to show the shape it wants, got %q", prompt)
	}
	if !strings.Contains(prompt, "leave the words in the narration") {
		t.Errorf("expected guidance for the case the model cannot name a speaker, got %q", prompt)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestAssembleContextAlwaysAsksForAttributableSpeech -count=1 ./pkg/harness/`
Expected: FAIL — no such section.

- [x] **Step 3: Implement**

In `pkg/harness/context.go`, add the block as a package constant:

```go
// speechFormattingInstruction is injected for every game. The parser needs one
// shape it can resolve deterministically, and the last line tells the model what
// to do when it cannot name a speaker, which is where attribution usually fails.
const speechFormattingInstruction = `## SPEECH FORMATTING
Write each spoken line on its own line, formatted as  Name: "the words spoken"
Use a character's established name, or [[their note name]] to link them.
Keep narration on its own lines with no leading name. If you cannot name the
speaker, leave the words in the narration instead of inventing a name.`
```

and write it in `AssembleContextWithProfiles`, after the lore block and before the voice-profile catalog:

```go
	sb.WriteString(speechFormattingInstruction + "\n\n")
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/harness/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness/context.go pkg/harness/context_test.go
git commit -m "feat(harness): tell the GM how to mark up spoken lines"
```

---

## Phase 6: Location Imagery

### Task 14: Make the art pipeline correct and location-aware

**Files:**
- Modify: `pkg/media/image.go` (`GenerateLocationImage`, `AppearanceHash`, `BuildLocationPrompt`, extension handling; `GenerateSceneImage` is replaced)
- Modify: `pkg/media/procedural_art.go` (seed)
- Test: `pkg/media/image_test.go`, `pkg/media/procedural_art_test.go`

**Ordering:** this task uses `entity.Entity.Appearance`, which Task 15 adds. Land Task 15's entity change (the frontmatter field, its parse/serialize round trip, and the storage round trip) first, or fold it into this task; the extractor half of Task 15 is independent and can follow.

**Interfaces:**
- Consumes: `entity.Entity.Appearance`, `ContentCache`, `ComputeArtCacheKey`
- Produces: `media.AppearanceHash(ent *entity.Entity, providerParams string) string`, `media.BuildLocationPrompt(ent *entity.Entity, worldStyle string) string`, `(*ImagePipeline).GenerateLocationImage(ctx context.Context, location *entity.Entity, worldStyle, providerParams string, force bool) (string, error)`, `media.artExtension(data []byte) string`

The hash and the prompt are deliberately separate inputs. The hash decides whether cached art still applies and covers only authored intent (the `appearance` field, else tags and state); the prompt is free to include a body excerpt because it only runs when generation actually happens. Collapsing the two would regenerate art on every turn, since extraction appends to the body almost every turn.

- [x] **Step 1: Write the failing tests**

Append to `pkg/media/image_test.go`:

```go
type stubImageClient struct {
	calls int
	body  []byte
}

func (c *stubImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	c.calls++
	return c.body, nil
}

func locationFixture() *entity.Entity {
	return &entity.Entity{
		ID:       "alden-tavern",
		Name:     "Alden Tavern",
		Type:     "location",
		Tags:     []string{"tavern", "safehouse"},
		Body:     "A quiet tavern at the edge of the woods.",
		Location: "[[aldor]]",
	}
}

func TestLocationImageExtensionFollowsTheBytes(t *testing.T) {
	svg := &stubImageClient{body: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)}
	pipeline := NewImagePipeline(svg, NewContentCache(t.TempDir()))

	path, err := pipeline.GenerateLocationImage(context.Background(), locationFixture(), "dark fantasy", "builtin:", false)
	if err != nil {
		t.Fatalf("GenerateLocationImage failed: %v", err)
	}
	if filepath.Ext(path) != ".svg" {
		t.Errorf("path = %q, want an .svg extension for SVG bytes", path)
	}

	again, err := pipeline.GenerateLocationImage(context.Background(), locationFixture(), "dark fantasy", "builtin:", false)
	if err != nil {
		t.Fatalf("second GenerateLocationImage failed: %v", err)
	}
	if again != path {
		t.Errorf("expected a cache hit at %q, got %q", path, again)
	}
	if svg.calls != 1 {
		t.Errorf("expected 1 provider call, got %d", svg.calls)
	}
}

func TestLocationImageRasterAndForce(t *testing.T) {
	raster := &stubImageClient{body: []byte("\x89PNG\r\n\x1a\nbinary")}
	pipeline := NewImagePipeline(raster, NewContentCache(t.TempDir()))

	path, err := pipeline.GenerateLocationImage(context.Background(), locationFixture(), "dark fantasy", "builtin:", false)
	if err != nil {
		t.Fatalf("GenerateLocationImage failed: %v", err)
	}
	if filepath.Ext(path) != ".webp" {
		t.Errorf("path = %q, want a .webp extension for raster bytes", path)
	}

	if _, err := pipeline.GenerateLocationImage(context.Background(), locationFixture(), "dark fantasy", "builtin:", true); err != nil {
		t.Fatalf("forced GenerateLocationImage failed: %v", err)
	}
	if raster.calls != 2 {
		t.Errorf("expected the force flag to bypass the cache, got %d calls", raster.calls)
	}
}

func TestAppearanceHashIgnoresProseAndTracksState(t *testing.T) {
	location := locationFixture()
	providerParams := "builtin:"

	original := AppearanceHash(location, providerParams)

	// Extraction appends to the body nearly every turn; that must not move the key.
	location.Body += " The floorboards creak."
	if AppearanceHash(location, providerParams) != original {
		t.Errorf("the body must not affect the cache key")
	}

	// Neither does tag order.
	location.Tags = []string{"safehouse", "tavern"}
	if AppearanceHash(location, providerParams) != original {
		t.Errorf("tag order must not affect the cache key")
	}

	// Structured state does.
	location.InitState(map[string]interface{}{"burned": true})
	if AppearanceHash(location, providerParams) == original {
		t.Errorf("state changes must move the cache key")
	}

	// An authored appearance overrides both.
	location.Appearance = "gutted by fire"
	withAppearance := AppearanceHash(location, providerParams)
	if withAppearance == original {
		t.Errorf("an appearance change must move the cache key")
	}
	location.State = nil
	if AppearanceHash(location, providerParams) != withAppearance {
		t.Errorf("an authored appearance must be the only input")
	}

	// A different provider model must not reuse another model's art.
	if AppearanceHash(location, "comfyui:sd-xl") == withAppearance {
		t.Errorf("provider parameters must move the cache key")
	}
}

func TestBuildLocationPromptPrefersTheAppearanceField(t *testing.T) {
	location := locationFixture()
	location.Body = strings.Repeat("prose ", 100)
	location.Appearance = "gutted by fire"

	prompt := BuildLocationPrompt(location, "dark fantasy")
	if !strings.Contains(prompt, "gutted by fire") {
		t.Errorf("expected the appearance field, got %q", prompt)
	}
	if strings.Contains(prompt, "prose") {
		t.Errorf("expected the body to be left out when an appearance exists, got %q", prompt)
	}

	location.Appearance = ""
	prompt = BuildLocationPrompt(location, "dark fantasy")
	if !strings.Contains(prompt, "Alden Tavern") || !strings.Contains(prompt, "dark fantasy") || !strings.Contains(prompt, "tavern") {
		t.Errorf("expected name, tags, and style, got %q", prompt)
	}
	if len(prompt) > 512 {
		t.Errorf("expected the body excerpt to be bounded, got %d characters", len(prompt))
	}
}
```

Append to `pkg/media/procedural_art_test.go`:

```go
func TestProceduralArtVariesByContentNotLength(t *testing.T) {
	client := NewProceduralArtClient()

	first, err := client.GenerateImage(context.Background(), "market square")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	repeat, err := client.GenerateImage(context.Background(), "market square")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if !bytes.Equal(first, repeat) {
		t.Errorf("identical prompts must produce identical art")
	}

	// Same length, different words: the old length-based seed collided here.
	other, err := client.GenerateImage(context.Background(), "square market")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if bytes.Equal(first, other) {
		t.Errorf("equal-length prompts must not share a layout")
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestLocationImage|TestAppearanceHash|TestBuildLocationPrompt|TestProceduralArtVaries" -count=1 ./pkg/media/`
Expected: FAIL — `undefined: GenerateLocationImage`, `undefined: AppearanceHash`.

- [x] **Step 3: Implement**

In `pkg/media/image.go`, replace `GenerateSceneImage` with the location-aware pair:

```go
// GenerateLocationImage returns a cached scene image path, generating it when the
// appearance has changed or force is set. The key covers authored intent only;
// the prompt may include prose because it runs only on a miss.
func (p *ImagePipeline) GenerateLocationImage(ctx context.Context, location *entity.Entity, worldStyle, providerParams string, force bool) (string, error) {
	base := ComputeArtCacheKey(location.ID, AppearanceHash(location, providerParams), worldStyle)

	if !force {
		for _, ext := range []string{".svg", ".webp"} {
			if p.cache.Exists("images", base+ext) {
				return filepath.Join(p.cache.Subdir("images"), base+ext), nil
			}
		}
	}

	prompt := BuildLocationPrompt(location, worldStyle)
	imgBytes, err := p.client.GenerateImage(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("generate image for %q: %w", location.ID, err)
	}

	return p.cache.Put("images", base+artExtension(imgBytes), imgBytes)
}

// AppearanceHash is the variation input for a scene's art: the authored
// appearance when set, otherwise sorted tags and canonical state, plus the
// provider parameters so one model's art is never served for another.
func AppearanceHash(ent *entity.Entity, providerParams string) string {
	parts := make([]string, 0, 3)

	if appearance := strings.TrimSpace(ent.Appearance); appearance != "" {
		parts = append(parts, appearance)
	} else {
		tags := append([]string(nil), ent.Tags...)
		sort.Strings(tags)

		state := ""
		if ent.State != nil {
			if data, err := json.Marshal(ent.State.Raw()); err == nil {
				state = string(data)
			}
		}
		parts = append(parts, strings.Join(tags, ","), state)
	}

	parts = append(parts, providerParams)
	sum := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(sum[:])
}

// BuildLocationPrompt composes the generation prompt: authored appearance when
// present, otherwise the name, a bounded body excerpt, and the tags, plus the
// world's art style.
func BuildLocationPrompt(ent *entity.Entity, worldStyle string) string {
	parts := make([]string, 0, 4)

	if appearance := strings.TrimSpace(ent.Appearance); appearance != "" {
		parts = append(parts, appearance)
	} else {
		parts = append(parts, ent.Name)
		body := strings.Join(strings.Fields(ent.Body), " ")
		if len(body) > 200 {
			body = body[:200] + "\u2026"
		}
		if body != "" {
			parts = append(parts, body)
		}
		if len(ent.Tags) > 0 {
			parts = append(parts, strings.Join(ent.Tags, ", "))
		}
	}

	if style := strings.TrimSpace(worldStyle); style != "" {
		parts = append(parts, style)
	}
	return strings.Join(parts, ", ")
}

// artExtension picks the cache file's extension from the bytes, because the
// built-in generator returns SVG while providers return raster data.
func artExtension(data []byte) string {
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	if bytes.Contains(bytes.ToLower(head), []byte("<svg")) {
		return ".svg"
	}
	return ".webp"
}
```

`BuildPrompt` stays for direct callers, and `GenerateSceneImage` is deleted: nothing outside tests called it, and the location-aware form is what every remaining caller wants.

In `pkg/media/procedural_art.go`, replace the length-based seed:

```go
	// Deterministic variation keyed on the prompt's content, so identical prompts
	// produce identical art and prompts of equal length do not collide.
	hasher := fnv.New64a()
	hasher.Write([]byte(prompt))
	rng := rand.New(rand.NewSource(int64(hasher.Sum64())))
```

with `"hash/fnv"` imported. Rewrite the `GenerateSceneImage` cases in `pkg/media/image_test.go` that this replaces, and update any other call site the compiler flags.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/media/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/media
git commit -m "feat(media): key scene art on authored appearance, not on prose"
```

### Task 15: Let extraction describe how a place looks

**Files:**
- Modify: `pkg/entity/entity.go` (`Appearance` on frontmatter and entity)
- Modify: `pkg/storage/store.go` (frontmatter JSON round-trip)
- Modify: `pkg/harness/extractor.go` (`ExtractedEntity.Appearance`, merge, prompt)
- Test: `pkg/entity/history_test.go`, `pkg/harness/extractor_test.go`

**Interfaces:**
- Consumes: nothing new
- Produces: `entity.Entity.Appearance` (frontmatter `appearance`), `harness.ExtractedEntity.Appearance` (JSON `appearance`)

- [x] **Step 1: Write the failing tests**

Append to `pkg/entity/history_test.go`:

```go
func TestEntityAppearanceRoundTrip(t *testing.T) {
	doc := `---
id: alden-tavern
name: Alden Tavern
type: location
appearance: gutted by fire, roof collapsed
---
A ruin now.
`
	ent, err := ParseMarkdownEntity([]byte(doc))
	if err != nil {
		t.Fatalf("ParseMarkdownEntity failed: %v", err)
	}
	if ent.Appearance != "gutted by fire, roof collapsed" {
		t.Fatalf("Appearance = %q", ent.Appearance)
	}

	serialized, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := ParseMarkdownEntity(serialized)
	if err != nil {
		t.Fatal(err)
	}
	if reparsed.Appearance != ent.Appearance {
		t.Errorf("Appearance after round trip = %q, want %q", reparsed.Appearance, ent.Appearance)
	}
}
```

Append to `pkg/harness/extractor_test.go`:

```go
func TestMergeOnlyFillsAnEmptyAppearance(t *testing.T) {
	existing := &entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Appearance: "warm and lamp-lit"}
	raw := &ExtractedEntity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Appearance: "gutted by fire"}

	merged := MergeExtractedEntity(existing, raw)
	if merged.Appearance != "warm and lamp-lit" {
		t.Errorf("Appearance = %q, want the authored value kept", merged.Appearance)
	}

	empty := &entity.Entity{ID: "alden-harbour", Name: "Aldon Harbour", Type: "location"}
	merged = MergeExtractedEntity(empty, raw)
	if merged.Appearance != "gutted by fire" {
		t.Errorf("Appearance = %q, want the extracted value applied to an empty field", merged.Appearance)
	}
}

func TestExtractorReadsAnEntityAppearance(t *testing.T) {
	model := &mockProvider{
		id: "extractor-model",
		output: `{"entities":[{"id":"alden-tavern","name":"Alden Tavern","type":"location","appearance":"roof collapsed","body":"A ruin."}]}`,
	}

	result, err := NewExtractor(model).Extract(context.Background(), "The tavern is a burnt shell.")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if len(result.Entities) != 1 || result.Entities[0].Appearance != "roof collapsed" {
		t.Errorf("expected an extracted appearance, got %+v", result.Entities)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestEntityAppearanceRoundTrip|TestMergeOnlyFills|TestExtractorReadsAnEntityAppearance" -count=1 ./pkg/entity/ ./pkg/harness/`
Expected: FAIL — `ent.Appearance undefined`

- [x] **Step 3: Implement**

In `pkg/entity/entity.go`, add to both types and to parse and serialize:

```go
	Appearance string `yaml:"appearance,omitempty" json:"appearance,omitempty"`
```

In `pkg/storage/store.go`, include it in `SaveEntity`'s `fmMeta` (`"appearance": e.Appearance`) and read it back in `GetEntity` (`if appearance, ok := meta["appearance"].(string); ok { ent.Appearance = appearance }`).

In `pkg/harness/extractor.go`:

```go
type ExtractedEntity struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Location   string `json:"location,omitempty"`
	Faction    string `json:"faction,omitempty"`
	Appearance string `json:"appearance,omitempty"`
	Body       string `json:"body"`
}
```

`MergeExtractedEntity` gains the same fill-only-when-empty treatment as `Location`:

```go
	if merged.Appearance == "" {
		merged.Appearance = raw.Appearance
	}
```

and the prompt's entity schema documents the field:

```
"appearance": "How this place or person looks right now, when it has visibly changed."
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/entity/ ./pkg/storage/ ./pkg/harness/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/entity pkg/storage pkg/harness
git commit -m "feat(entity): let a note describe how it currently looks"
```

### Task 16: Guarantee imagery offline

**Files:**
- Modify: `pkg/config/types.go` (`ImageConfig.BuiltinFallback`)
- Modify: `pkg/media/providers.go` (fallback client, `NewSceneImageClient`)
- Test: `pkg/media/providers_test.go`

**Interfaces:**
- Consumes: `NewImageClient`, the builtin `procedural-art` provider
- Produces: `config.ImageConfig.BuiltinFallback bool`, `media.NewSceneImageClient(cfg config.ImageConfig) (ImageClient, error)`

- [x] **Step 1: Write the failing test**

Append to `pkg/media/providers_test.go`:

```go
func TestSceneImageClientAlwaysProducesArtByDefault(t *testing.T) {
	// No provider configured, fallback on: the built-in generator stands in.
	client, err := NewSceneImageClient(config.ImageConfig{Type: "disabled", BuiltinFallback: true})
	if err != nil {
		t.Fatalf("NewSceneImageClient failed: %v", err)
	}

	data, err := client.GenerateImage(context.Background(), "moonlit harbour")
	if err != nil {
		t.Fatalf("expected the built-in generator to stand in: %v", err)
	}
	if !bytes.Contains(bytes.ToLower(data), []byte("<svg")) {
		t.Errorf("expected SVG art from the built-in generator")
	}
}

func TestSceneImageClientRespectsAFailedProvider(t *testing.T) {
	client, err := NewSceneImageClient(config.ImageConfig{
		Type:            "http",
		Endpoint:        "http://127.0.0.1:1/unreachable",
		BuiltinFallback: true,
	})
	if err != nil {
		t.Fatalf("NewSceneImageClient failed: %v", err)
	}

	data, err := client.GenerateImage(context.Background(), "moonlit harbour")
	if err != nil {
		t.Fatalf("expected the built-in generator to cover a provider failure: %v", err)
	}
	if !bytes.Contains(bytes.ToLower(data), []byte("<svg")) {
		t.Errorf("expected fallback art")
	}
}

func TestSceneImageClientWithoutFallbackStaysDisabled(t *testing.T) {
	client, err := NewSceneImageClient(config.ImageConfig{Type: "disabled", BuiltinFallback: false})
	if err != nil {
		t.Fatalf("NewSceneImageClient failed: %v", err)
	}

	if _, err := client.GenerateImage(context.Background(), "moonlit harbour"); !errors.Is(err, ErrProviderDisabled) {
		t.Errorf("expected ErrProviderDisabled, got %v", err)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestSceneImageClient -count=1 ./pkg/media/`
Expected: FAIL — `undefined: NewSceneImageClient`

- [x] **Step 3: Implement**

In `pkg/config/types.go`, `ImageConfig` gains:

```go
	// BuiltinFallback lets the built-in procedural generator stand in when no
	// provider is configured or a provider call fails, so imagery always exists.
	BuiltinFallback bool `yaml:"builtin_fallback" json:"builtin_fallback"`
```

and `DefaultConfig` sets `BuiltinFallback: true` alongside `AutoGenerate: false`.

In `pkg/media/providers.go`:

```go
// fallbackImageClient covers a disabled or failing provider with the built-in
// generator, so a scene always has art offline.
type fallbackImageClient struct {
	primary  ImageClient
	fallback ImageClient
}

func (c *fallbackImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	data, err := c.primary.GenerateImage(ctx, prompt)
	if err == nil {
		return data, nil
	}
	return c.fallback.GenerateImage(ctx, prompt)
}

// NewSceneImageClient builds the image client used for scene art: the configured
// provider, wrapping the built-in generator when the fallback is enabled.
func NewSceneImageClient(cfg config.ImageConfig) (ImageClient, error) {
	primary, err := NewImageClient(cfg)
	if err != nil {
		return nil, err
	}
	if !cfg.BuiltinFallback {
		return primary, nil
	}

	fallback, err := NewImageClient(config.ImageConfig{Type: "builtin", BuiltinName: "procedural-art"})
	if err != nil {
		return nil, err
	}
	return &fallbackImageClient{primary: primary, fallback: fallback}, nil
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/media/ ./pkg/config/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/media/providers.go pkg/media/providers_test.go
git commit -m "feat(media): keep scene art available with no provider configured"
```

### Task 17: Serve location art and show it when the scene changes

**Files:**
- Modify: `pkg/gui/service.go` (`GetLocationArt`, DTO fields in `GetChronicle`)
- Modify: `pkg/gui/server.go` (route)
- Modify: `pkg/gui/types.go` (`TurnDTO.LocationID`, `LocationName`, `LocationArtURL`)
- Modify: `pkg/gui/service_test.go` (`setupTestGame` gains a location the art tests can use)
- Modify: `frontend/src/types.ts`, `frontend/src/components/ChronicleView.tsx`
- Test: `pkg/gui/service_test.go`, `pkg/gui/server_test.go`

**Interfaces:**
- Consumes: `media.NewSceneImageClient`, `media.NewImagePipeline`, `media.GenerateLocationImage`, `media.AppearanceHash` (all inside the pipeline call)
- Produces: `(*Service).GetLocationArt(ctx context.Context, gameID, locationID string, force bool) (string, string, error)` returning a path and content type; route `GET /api/game/{id}/location/{eid}/art`

- [x] **Step 1: Write the failing test**

In `pkg/gui/service_test.go`, add a location to `setupTestGame` and point the player at it, so art and location tests have something real to work with:

```go
	locationMD := `---
id: aldon-harbour
name: Aldon Harbour
type: location
tags: [harbour, docks]
---
Salt air and gull cries.`
	_ = os.WriteFile(filepath.Join(entitiesDir, "aldon-harbour.md"), []byte(locationMD), 0644)
```

and give the player note a `location: "[[aldon-harbour]]"` line. Then append:

```go
func TestGetLocationArtUsesTheBuiltinGeneratorAndCaches(t *testing.T) {
	gameID, svc := setupTestGame(t)

	path, contentType, err := svc.GetLocationArt(context.Background(), gameID, "aldon-harbour", false)
	if err != nil {
		t.Fatalf("GetLocationArt failed: %v", err)
	}
	if contentType != "image/svg+xml" {
		t.Errorf("contentType = %q, want image/svg+xml from the built-in generator", contentType)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected the art on disk: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(data)), "<svg") {
		t.Errorf("expected SVG art, got %q", string(data[:min(len(data), 40)]))
	}

	// A second request for the same appearance is a cache hit at the same path.
	again, _, err := svc.GetLocationArt(context.Background(), gameID, "aldon-harbour", false)
	if err != nil {
		t.Fatalf("second GetLocationArt failed: %v", err)
	}
	if again != path {
		t.Errorf("expected a cache hit at %q, got %q", path, again)
	}

	// Appearance changes move the key, so the image changes with it.
	entityPath := filepath.Join(svc.GetResolver().GameDir(gameID), "entities", "aldon-harbour.md")
	updated := strings.Replace(string(mustRead(t, entityPath)), "type: location", "type: location\nappearance: burned and abandoned", 1)
	if err := os.WriteFile(entityPath, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}

	moved, _, err := svc.GetLocationArt(context.Background(), gameID, "aldon-harbour", false)
	if err != nil {
		t.Fatalf("GetLocationArt after a change failed: %v", err)
	}
	if moved == path {
		t.Errorf("expected a new image after the appearance changed")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
```

Append to `pkg/gui/server_test.go`:

```go
func TestLocationArtRoute(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/location/aldon-harbour/art", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("Content-Type = %q, want image/svg+xml", ct)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestGetLocationArt|TestLocationArtRoute" -count=1 ./pkg/gui/`
Expected: FAIL — `svc.GetLocationArt undefined`

- [x] **Step 3: Implement**

In `pkg/gui/service.go`:

```go
// GetLocationArt returns a location's scene image and its content type, drawing
// it on first request and reusing it until the appearance changes.
func (s *Service) GetLocationArt(ctx context.Context, gameID, locationID string, force bool) (string, string, error) {
	// Read the note from disk rather than the index: appearance, tags, and state are
	// authored content, and the index is derived from them. Reading the index here
	// would make a hand-edited appearance invisible until the next sync.
	notePath := filepath.Join(s.resolver.GameDir(gameID), "entities", locationID+".md")
	data, err := os.ReadFile(notePath)
	if err != nil {
		return "", "", fmt.Errorf("location %q not found: %w", locationID, err)
	}

	location, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return "", "", fmt.Errorf("parse location %q: %w", locationID, err)
	}

	cfg := s.configMgr.Get()
	client, err := media.NewSceneImageClient(cfg.Media.Image)
	if err != nil {
		return "", "", fmt.Errorf("build image client: %w", err)
	}

	pipeline := media.NewImagePipeline(client, media.NewContentCache(s.resolver.CacheDir()))
	providerParams := cfg.Media.Image.Type + ":" + cfg.Media.Image.Model

	path, err := pipeline.GenerateLocationImage(ctx, location, s.worldArtStyle(gameID), providerParams, force)
	if err != nil {
		return "", "", err
	}
	return path, contentTypeForArt(path), nil
}

// worldArtStyle reads the art style and genre of the campaign's world, which is
// what keeps a setting's imagery visually consistent.
func (s *Service) worldArtStyle(gameID string) string {
	manifest, err := core.LoadGameManifest(filepath.Join(s.resolver.GameDir(gameID), "game.yaml"))
	if err != nil {
		return ""
	}
	world, err := core.LoadWorldManifest(filepath.Join(s.resolver.WorldDir(manifest.WorldID), "world.yaml"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.Join([]string{world.ArtStyle, world.Genre}, ", "))
}

func contentTypeForArt(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".svg") {
		return "image/svg+xml"
	}
	return "image/webp"
}
```

`GetChronicle` fills the location fields per turn, and only offers an art URL when art can actually be produced:

```go
		artAvailable := cfg.Media.Image.BuiltinFallback || cfg.Media.Image.Type != "disabled"
		// …
		if turn.Location != "" {
			dto.LocationID = turn.Location
			if location, err := store.GetEntity(turn.Location); err == nil && location != nil {
				dto.LocationName = location.Name
			}
			if artAvailable {
				dto.LocationArtURL = "/api/game/" + gameID + "/location/" + turn.Location + "/art"
			}
		}
```

In `pkg/gui/server.go`, extend `handleGameRoutes` with a `location` action:

```go
	case "location":
		if len(parts) < 4 || parts[3] != "art" {
			http.NotFound(w, r)
			return
		}
		locationID := parts[2]
		force := r.URL.Query().Get("force") == "1"

		path, contentType, err := s.service.GetLocationArt(r.Context(), gameID, locationID, force)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		http.ServeFile(w, r, path)
```

In `pkg/gui/types.go`, `TurnDTO` gains `LocationID`, `LocationName`, and `LocationArtURL` (all `omitempty`). In `frontend/src/types.ts`, mirror them, and in `ChronicleView.tsx` render the art when the scene changes:

```tsx
            {turn.location_art_url && turn.location_id !== previousLocationID && (
              <div className="my-4 rounded-xl overflow-hidden border border-white/10 shadow-2xl">
                <img src={turn.location_art_url} alt={turn.location_name || 'Scene'} className="w-full object-cover max-h-96" />
                {turn.location_name && (
                  <div className="px-3 py-2 text-xs font-cinzel tracking-widest text-stone-400 uppercase">
                    {turn.location_name}
                  </div>
                )}
              </div>
            )}
```

with `let previousLocationID: string | undefined;` reset on each render pass inside the `turns.map` callback, assigning `previousLocationID = turn.location_id;` after the comparison so consecutive turns in one place show the image once.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/gui/ && cd frontend && npx tsc --noEmit`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat(gui): show a location's art when the scene changes"
```

<!-- PLAN-CONTINUES -->

---

## Phase 7: Playback and Surface

### Task 18: One segment at a time

**Files:**
- Modify: `pkg/media/tts.go`
- Test: `pkg/media/tts_test.go`

**Interfaces:**
- Consumes: `SynthesizeUtterance` (existing)
- Produces: `(*TTSPipeline).SynthesizeSegment(ctx, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (string, error)`

- [x] **Step 1: Write the failing test**

Append to `pkg/media/tts_test.go`:

```go
func TestSynthesizeSegmentPicksTheRightVoice(t *testing.T) {
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))

	narrator := &entity.VoiceConfig{VoiceID: "narrator-voice"}
	voices := map[string]*entity.VoiceConfig{
		"garrick": {VoiceID: "bm_george"},
		"Sean":    {VoiceID: "player-voice"},
	}
	voiceFor := func(key string) *entity.VoiceConfig { return voices[key] }

	if _, err := pipeline.SynthesizeSegment(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentNarration, Text: "The hall is quiet."}, narrator, voiceFor); err != nil {
		t.Fatalf("SynthesizeSegment failed: %v", err)
	}
	if client.lastVoice == nil || client.lastVoice.VoiceID != "narrator-voice" {
		t.Errorf("narration should use the narrator voice, got %+v", client.lastVoice)
	}

	if _, err := pipeline.SynthesizeSegment(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentSpeech, SpeakerID: "garrick", Text: "Keep walking."}, narrator, voiceFor); err != nil {
		t.Fatalf("SynthesizeSegment failed: %v", err)
	}
	if client.lastVoice == nil || client.lastVoice.VoiceID != "bm_george" {
		t.Errorf("speech should use the speaker's voice, got %+v", client.lastVoice)
	}

	// A legacy record has a name but no ID; the name still resolves a voice.
	if _, err := pipeline.SynthesizeSegment(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentSpeech, Speaker: "Sean", Text: "Hello."}, narrator, voiceFor); err != nil {
		t.Fatalf("SynthesizeSegment failed: %v", err)
	}
	if client.lastVoice == nil || client.lastVoice.VoiceID != "player-voice" {
		t.Errorf("legacy speech should resolve by name, got %+v", client.lastVoice)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestSynthesizeSegmentPicksTheRightVoice -count=1 ./pkg/media/`
Expected: FAIL — `pipeline.SynthesizeSegment undefined`

- [x] **Step 3: Implement**

In `pkg/media/tts.go`, extract the existing per-segment body out of `SynthesizeSegments`:

```go
// SynthesizeSegment renders one segment: narration and unresolved speech read in
// the narrator voice, resolved speech in the speaker's own. Legacy records carry
// a speaker name but no entity ID, so the name is tried as a voice key too.
func (p *TTSPipeline) SynthesizeSegment(ctx context.Context, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (string, error) {
	voice := narratorVoice
	speakerID := narratorSpeaker

	if segment.Kind == entity.SegmentSpeech {
		speakerID = segment.SpeakerID
		if speakerID == "" {
			speakerID = segment.Speaker
		}
		if speakerID == "" {
			speakerID = narratorSpeaker
		}
		if voiceFor != nil {
			if resolved := voiceFor(speakerID); resolved != nil {
				voice = resolved
			}
		}
	}

	return p.SynthesizeUtterance(ctx, speakerID, voice, segment.Text)
}
```

`SynthesizeSegments` becomes:

```go
	clips := make([]string, 0, len(segments))
	for _, segment := range segments {
		clip, err := p.SynthesizeSegment(ctx, segment, narratorVoice, voiceFor)
		if err != nil {
			return nil, err
		}
		clips = append(clips, clip)
	}
	return clips, nil
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/media/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/media/tts.go pkg/media/tts_test.go
git commit -m "feat(media): synthesize a single segment for on-demand playback"
```

### Task 19: Serve a segment's audio on demand

**Files:**
- Modify: `pkg/gui/service.go` (`GetSegmentAudio`, `voiceFor`, `SegmentDTO.AudioURL` in `GetChronicle`)
- Modify: `pkg/gui/server.go` (route)
- Modify: `pkg/gui/types.go` (`SegmentDTO.AudioURL`)
- Modify: `pkg/gui/service_test.go` (`setupTestGame` enables builtin TTS)
- Test: `pkg/gui/service_test.go`, `pkg/gui/server_test.go`

**Interfaces:**
- Consumes: `media.NewTTSClient`, `media.NewTTSPipeline`, `media.NewContentCache`, `TTSPipeline.SynthesizeSegment`
- Produces: `(*Service).GetSegmentAudio(ctx context.Context, gameID string, turnNumber, segmentIndex int) (string, error)`; `gui.ErrAudioUnavailable`; route `GET /api/game/{id}/turn/{n}/segment/{i}/audio`

- [x] **Step 1: Write the failing tests**

In `pkg/gui/service_test.go`, extend `setupTestGame` so media is configured for playback tests:

```go
	// Media is enabled so playback and art routes have something to serve. The
	// built-in TTS client returns synthetic bytes and needs no external process.
	configYAML := "media:\n  tts:\n    type: builtin\n    default_voice: narrator\n"
	if err := os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}
```

Append the tests:

```go
func writeSegmentTurn(t *testing.T, svc *Service, gameID string) {
	t.Helper()

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Say","input":"Where is the ledger?","narration":"He does not look up.","segments":[{"kind":"narration","text":"He does not look up."},{"kind":"speech","speaker":"Captain Kaelen","speaker_id":"captain-kaelen","text":"Keep walking."}]}` + "\n"
	path := filepath.Join(svc.GetResolver().GameDir(gameID), "history.jsonl")
	if err := os.WriteFile(path, []byte(record), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestGetSegmentAudioSynthesizesAndCaches(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	path, err := svc.GetSegmentAudio(context.Background(), gameID, 1, 1)
	if err != nil {
		t.Fatalf("GetSegmentAudio failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected a clip on disk: %v", err)
	}

	again, err := svc.GetSegmentAudio(context.Background(), gameID, 1, 1)
	if err != nil {
		t.Fatalf("second GetSegmentAudio failed: %v", err)
	}
	if again != path {
		t.Errorf("expected the cached clip %q, got %q", path, again)
	}

	if _, err := svc.GetSegmentAudio(context.Background(), gameID, 1, 9); err == nil {
		t.Errorf("expected an error for an out-of-range segment")
	}
	if _, err := svc.GetSegmentAudio(context.Background(), gameID, 42, 0); err == nil {
		t.Errorf("expected an error for an unknown turn")
	}
}

func TestGetSegmentAudioWithoutTTSIsUnavailable(t *testing.T) {
	// A service whose config never enabled TTS.
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	quiet := NewService(t.TempDir())
	if _, err := os.MkdirAll(quiet.GetResolver().GameDir(gameID), 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := quiet.GetSegmentAudio(context.Background(), gameID, 1, 1); !errors.Is(err, ErrAudioUnavailable) {
		t.Errorf("expected ErrAudioUnavailable, got %v", err)
	}
}

func TestChronicleOffersAudioURLsWhenTTSIsConfigured(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	turns, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || len(turns[0].Segments) != 2 {
		t.Fatalf("unexpected chronicle: %+v", turns)
	}
	if turns[0].Segments[0].AudioURL == "" || turns[0].Segments[1].AudioURL == "" {
		t.Errorf("expected audio URLs on both segments, got %+v", turns[0].Segments)
	}
}
```

Append to `pkg/gui/server_test.go`:

```go
func TestSegmentAudioRoute(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/turn/1/segment/1/audio", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		t.Errorf("expected audio bytes")
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestGetSegmentAudio|TestChronicleOffersAudioURLs|TestSegmentAudioRoute" -count=1 ./pkg/gui/`
Expected: FAIL — `svc.GetSegmentAudio undefined`

- [x] **Step 3: Implement**

In `pkg/gui/service.go`:

```go
// ErrAudioUnavailable means no TTS provider is configured, which is a normal
// state rather than a failure: the client stays silent.
var ErrAudioUnavailable = errors.New("audio unavailable")

// GetSegmentAudio synthesizes one segment on demand and returns the cached clip,
// reusing it for every later request.
func (s *Service) GetSegmentAudio(ctx context.Context, gameID string, turnNumber, segmentIndex int) (string, error) {
	turns, err := engine.NewHistoryLogger(filepath.Join(s.resolver.GameDir(gameID), "history.jsonl")).LoadHistory()
	if err != nil {
		return "", fmt.Errorf("load history: %w", err)
	}

	var turn *engine.Turn
	for i := range turns {
		if turns[i].Number == turnNumber {
			turn = &turns[i]
			break
		}
	}
	if turn == nil {
		return "", fmt.Errorf("turn %d not found", turnNumber)
	}
	if segmentIndex < 0 || segmentIndex >= len(turn.Segments) {
		return "", fmt.Errorf("segment %d out of range for turn %d", segmentIndex, turnNumber)
	}

	cfg := s.configMgr.Get()
	if cfg.Media.TTS.Type == "" || cfg.Media.TTS.Type == "disabled" {
		return "", ErrAudioUnavailable
	}

	client, err := media.NewTTSClient(cfg.Media.TTS)
	if err != nil {
		return "", fmt.Errorf("build tts client: %w", err)
	}

	narratorVoice := &entity.VoiceConfig{
		VoiceID:    cfg.Media.TTS.DefaultVoice,
		Pitch:      cfg.Media.TTS.Pitch,
		SpeechRate: cfg.Media.TTS.SpeechRate,
	}

	pipeline := media.NewTTSPipeline(client, media.NewContentCache(s.resolver.CacheDir()))
	return pipeline.SynthesizeSegment(ctx, turn.Segments[segmentIndex], narratorVoice, s.voiceFor(gameID))
}

// voiceFor resolves a speaker entity's configured voice, if it has one.
func (s *Service) voiceFor(gameID string) func(speakerID string) *entity.VoiceConfig {
	store, err := s.store(gameID)
	if err != nil {
		return nil
	}

	return func(speakerID string) *entity.VoiceConfig {
		if speakerID == "" {
			return nil
		}
		ent, err := store.GetEntity(speakerID)
		if err != nil || ent == nil {
			return nil
		}
		return ent.Voice
	}
}
```

`GetChronicle` fills `SegmentDTO.AudioURL` when TTS is configured, which means `segmentDTOs` needs the turn it is describing:

```go
func segmentDTOs(segments []entity.TurnSegment, gameID string, turnNumber int, audioAvailable bool) []SegmentDTO {
	dtos := make([]SegmentDTO, 0, len(segments))
	for i, segment := range segments {
		dto := SegmentDTO{
			Kind:      segment.Kind,
			Speaker:   segment.Speaker,
			SpeakerID: segment.SpeakerID,
			Text:      segment.Text,
		}
		if audioAvailable {
			dto.AudioURL = fmt.Sprintf("/api/game/%s/turn/%d/segment/%d/audio", gameID, turnNumber, i)
		}
		dtos = append(dtos, dto)
	}
	return dtos
}
```

with the call site reading `Segments: segmentDTOs(turn.Segments, gameID, turn.Number, audioAvailable)`.

In `pkg/gui/server.go`, add a `turn` action to `handleGameRoutes`:

```go
	case "turn":
		// /api/game/{id}/turn/{n}/segment/{i}/audio
		if len(parts) < 6 || parts[1] != "segment" || parts[3] != "audio" {
			http.NotFound(w, r)
			return
		}
		turnNumber, err := strconv.Atoi(parts[0])
		if err != nil {
			http.Error(w, "invalid turn number", http.StatusBadRequest)
			return
		}
		segmentIndex, err := strconv.Atoi(parts[2])
		if err != nil {
			http.Error(w, "invalid segment index", http.StatusBadRequest)
			return
		}

		path, err := s.service.GetSegmentAudio(r.Context(), gameID, turnNumber, segmentIndex)
		switch {
		case errors.Is(err, ErrAudioUnavailable):
			w.WriteHeader(http.StatusNoContent)
		case err != nil:
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			w.Header().Set("Content-Type", "audio/wav")
			http.ServeFile(w, r, path)
		}
```

(Index arithmetic: with `parts = [gameID, "turn", n, "segment", i, "audio"]`, the turn number is `parts[2]` and the segment index is `parts[4]`; use those. Add `strconv` and `errors` to the file's imports.)

In `pkg/gui/types.go`, `SegmentDTO` gains `AudioURL string \`json:"audio_url,omitempty"\``, and `frontend/src/types.ts` mirrors it as `audio_url?: string`.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/gui frontend/src/types.ts
git commit -m "feat(gui): synthesize a turn's dialogue on demand"
```

### Task 20: Play a turn's segments in the browser

**Files:**
- Create: `frontend/src/hooks/useSegmentPlayback.ts`
- Modify: `frontend/src/components/TurnSegments.tsx`, `frontend/src/components/App.tsx` or wherever `auto_play` reaches the chronicle
- Test: `cd frontend && npx tsc --noEmit && npm run build`

**Interfaces:**
- Consumes: `TurnSegment.audio_url` from the API
- Produces: `useSegmentPlayback(segments, autoPlay, volume)` returning `{ playing, play, stop }`

- [x] **Step 1: Write the failing check**

Create the hook and use it, then let the type checker find what does not line up:

Run: `cd frontend && npx tsc --noEmit`
Expected: FAIL until the props are threaded through.

- [x] **Step 2: Implement the hook**

`frontend/src/hooks/useSegmentPlayback.ts`:

```typescript
import { useCallback, useEffect, useRef, useState } from 'react';
import { TurnSegment } from '../types';

// Plays a turn's segments in order, skipping any without a clip. It is the one
// playback implementation, shared by the chronicle and the story theater so
// pacing and controls cannot drift between them.
export const useSegmentPlayback = (
  segments: TurnSegment[] | undefined,
  autoPlay: boolean,
  volume: number
) => {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [playing, setPlaying] = useState(false);

  const stop = useCallback(() => {
    audioRef.current?.pause();
    audioRef.current = null;
    setPlaying(false);
  }, []);

  const playFrom = useCallback(
    (index: number) => {
      const urls = (segments ?? []).map((segment) => segment.audio_url);
      const next = urls.findIndex((url, i) => i >= index && !!url);
      if (next === -1) {
        stop();
        return;
      }

      const audio = new Audio(urls[next] as string);
      audio.volume = volume;
      audio.onended = () => playFrom(next + 1);
      audioRef.current = audio;
      setPlaying(true);
      void audio.play();
    },
    [segments, stop, volume]
  );

  useEffect(() => {
    if (!autoPlay) return;
    playFrom(0);
    return stop;
  }, [autoPlay, playFrom, stop]);

  return { playing, play: () => playFrom(0), stop };
};
```

- [x] **Step 3: Wire it into the segment renderer**

`TurnSegments.tsx` gains `autoPlay` and `volume` props, uses the hook, and renders a transport when any segment has a clip:

```tsx
  const hasAudio = (segments ?? []).some((segment) => !!segment.audio_url);
  const { playing, play, stop } = useSegmentPlayback(segments, autoPlay && hasAudio, volume);

  // …
      {hasAudio && (
        <div className="flex items-center gap-2 text-xs text-stone-400">
          <button onClick={playing ? stop : play} className="hover:text-amber-300 cursor-pointer">
            {playing ? 'Pause' : 'Play turn'}
          </button>
        </div>
      )}
```

and each speech segment's label becomes a control that plays from that segment:

```tsx
              <button onClick={() => playFrom(index)} className="hover:text-amber-300 cursor-pointer">
                {segment.speaker || 'UNKNOWN'}
              </button>
```

The chronicle and the story theater read `preferences.tts.auto_play` and `preferences.tts.master_volume` from the settings they already fetch and pass them down, so the two existing switches finally do something.

- [x] **Step 4: Verify the type check and the build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): play a turn's dialogue in each character's voice"
```

### Task 21: Make the `tts` command do what it says

**Files:**
- Modify: `cmd/localrpg/media.go`
- Modify: `cmd/localrpg/media_test.go`
- Test: `cmd/localrpg/media_test.go`

**Interfaces:**
- Consumes: `media.NewTTSClient`, `media.NewTTSPipeline`, `media.NewContentCache`, `config.NewConfigManager`
- Produces: `localrpg tts [--voice id] [--pitch n] [--rate n] -- <text>` printing the written clip path

- [x] **Step 1: Write the failing test**

Replace the TTS assertion in `cmd/localrpg/media_test.go` with one that proves a clip was written, using a throwaway config so the test does not depend on the developer's own settings:

```go
func TestCLITTSWritesAClip(t *testing.T) {
	configDir := t.TempDir()
	configYAML := "media:\n  tts:\n    type: builtin\n    default_voice: narrator\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", ".", "tts", "hello there")
	cmd.Env = append(os.Environ(), "LOCALRPG_CONFIG_DIR="+configDir)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tts failed: %v: %s", err, out)
	}

	line := strings.TrimSpace(strings.Split(strings.TrimSpace(string(out)), "\n")[len(strings.Split(strings.TrimSpace(string(out)), "\n"))-1])
	if _, err := os.Stat(line); err != nil {
		t.Errorf("expected %q to be a written clip path: %v", line, err)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestCLITTSWritesAClip -count=1 ./cmd/localrpg/`
Expected: FAIL — the command prints a synthetic message and writes nothing.

- [x] **Step 3: Implement**

In `cmd/localrpg/media.go`, `handleTTSCommand` gains flags and a real synthesis path:

```go
func handleTTSCommand(args []string) {
	fs := flag.NewFlagSet("tts", flag.ContinueOnError)
	voice := fs.String("voice", "", "Voice ID override")
	pitch := fs.Float64("pitch", 0, "Pitch override")
	rate := fs.Float64("rate", 0, "Speech rate override")
	if err := fs.Parse(args); err != nil {
		return
	}

	text := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if text == "" {
		fmt.Fprintln(os.Stderr, "Usage: localrpg tts [--voice id] [--pitch n] [--rate n] <text>")
		os.Exit(1)
	}

	cfgMgr := config.NewConfigManager()
	cfg, _ := cfgMgr.Load()

	client, err := media.NewTTSClient(cfg.Media.TTS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building TTS client: %v\n", err)
		os.Exit(1)
	}

	voiceCfg := &entity.VoiceConfig{
		VoiceID:    cfg.Media.TTS.DefaultVoice,
		Pitch:      cfg.Media.TTS.Pitch,
		SpeechRate: cfg.Media.TTS.SpeechRate,
	}
	if *voice != "" {
		voiceCfg.VoiceID = *voice
	}
	if *pitch != 0 {
		voiceCfg.Pitch = *pitch
	}
	if *rate != 0 {
		voiceCfg.SpeechRate = *rate
	}

	pipeline := media.NewTTSPipeline(client, media.NewContentCache(cfg.Paths.Cache))
	path, err := pipeline.SynthesizeUtterance(context.Background(), "cli", voiceCfg, text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error synthesizing: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(path)
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./cmd/localrpg/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add cmd/localrpg/media.go cmd/localrpg/media_test.go
git commit -m "feat(cli): make the tts command produce a file"
```

### Task 22: Show where the party is

**Files:**
- Modify: `pkg/engine/orchestrator.go` (`CurrentLocationName`)
- Modify: `pkg/tui/app.go` (`View` status line)
- Test: `pkg/engine/orchestrator_location_test.go`, `pkg/tui/app_test.go`

**Interfaces:**
- Consumes: `(*TurnOrchestrator).currentLocation` (Task 4)
- Produces: `(*TurnOrchestrator).CurrentLocationName() string`

- [x] **Step 1: Write the failing tests**

Append to `pkg/engine/orchestrator_location_test.go`:

```go
func TestCurrentLocationName(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Warm."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller.", Location: "[[alden-tavern]]"})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	o := NewTurnOrchestrator(store, timeline, nil, harness.NewRouter(), "alden-tavern", "player")
	if name := o.CurrentLocationName(); name != "Alden Tavern" {
		t.Errorf("CurrentLocationName = %q, want Alden Tavern", name)
	}
}
```

Append to `pkg/tui/app_test.go`:

```go
func TestTUIViewShowsTheLocation(t *testing.T) {
	tempDir := t.TempDir()
	store, _ := storage.NewStore(filepath.Join(tempDir, "index.db"))
	defer store.Close()

	store.SaveEntity(&entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Quiet place.", Hash: "h1"})
	store.SaveEntity(&entity.Entity{ID: "player", Name: "Sean", Type: "character", Location: "[[alden-tavern]]", Hash: "h2"})

	history := engine.NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	timeline := engine.NewTimeline(core.NewPathResolver(tempDir), store, history, "test-campaign")
	router := harness.NewRouter()
	router.RegisterProvider(&mockTUIModel{output: "hi"})
	router.AssignRole("gm", "tui-mock")

	orch := engine.NewTurnOrchestrator(store, timeline, nil, router, "alden-tavern", "player")
	app := NewAppModel(orch, 80, 24)

	if !strings.Contains(app.View(), "Alden Tavern") {
		t.Errorf("expected the view to name the current location, got %q", app.View())
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestCurrentLocationName|TestTUIViewShowsTheLocation" -count=1 ./pkg/engine/ ./pkg/tui/`
Expected: FAIL — `o.CurrentLocationName undefined`

- [x] **Step 3: Implement**

In `pkg/engine/orchestrator.go`:

```go
// CurrentLocationName returns the display name of where the party is, for
// clients that show it without loading the entity themselves.
func (o *TurnOrchestrator) CurrentLocationName() string {
	id := o.currentLocation()
	if id == "" {
		return ""
	}
	if ent, err := o.store.GetEntity(id); err == nil && ent != nil {
		return ent.Name
	}
	return id
}
```

In `pkg/tui/app.go`'s `View`, prefix the status line:

```go
	status := m.statusMsg
	if name := m.orchestrator.CurrentLocationName(); name != "" {
		status = fmt.Sprintf("[%s] %s", name, status)
	}
	sb.WriteString(StatusStyle.Render(status) + "\n")
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/ ./pkg/tui/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/tui/app.go pkg/engine/orchestrator_location_test.go pkg/tui/app_test.go
git commit -m "feat(tui): show where the party is"
```

---

## Spec Coverage

| Spec section | Tasks |
| --- | --- |
| §3 extraction role (config, resolution, UI, cost visibility) | 10, 11 |
| §4.1 speech instruction | 13 |
| §4.2 parser rules | 12 |
| §4.3 extracted schema (`appearance`, `player_location`) | 7, 15 |
| §5 check outcomes (reporting, storage, surface) | 8, 9 |
| §6.1 location source of truth | 4 |
| §6.2 per-turn recording | 4 |
| §6.3 movement (rules, `/go`, extraction) | 5, 6, 7 |
| §6.4 the player note | 3 |
| §7.1 variation inputs | 14, 15 |
| §7.2 pipeline fixes | 14 |
| §7.3 generation policy | 16 |
| §7.4 surface | 17 |
| §8.1 segment audio | 18, 19 |
| §8.2 client behaviour | 20 |
| §8.3 CLI | 21 |
| §9 schema migrations | 1, 2 |
| §10 file map | all |
| §12 verification plan | every task's Step 1 |

### Deviations

- **Task 22 (TUI location) rides in Phase 7** rather than Phase 2. It is a display-only change that depends on Task 4's accessor, and grouping it with the other surface work keeps Phase 2 to the data model.
- **Task 6 also switches `DefaultHostBridge.SetStat` to the entity writer.** Writing state through `storage.Store.SaveEntity` alone never reached the Markdown note, so a file-driven sync could revert a scripted state change. The writer was being added for `SetLocation` anyway, and leaving the pre-existing divergence in place would have left two write paths in the same file.
- **Task 19's route arithmetic** is spelled out because `handleGameRoutes` strips the `/api/game/` prefix before splitting: for `/api/game/{id}/turn/{n}/segment/{i}/audio`, `parts` is `[gameID, "turn", n, "segment", i, "audio"]`, so the turn number is `parts[2]` and the segment index is `parts[4]`; the `case "turn"` body indexes `parts[0]`/`parts[2]` because it re-slices after `gameID`.

### Deferred to the export spec

Reading-time constants and pacing, scene transitions in the animated player, the standalone bundle layout, raster typography for video frames, and export-time audio synthesis with its skip-and-warn behaviour. GUI turn submission remains its own spec.
