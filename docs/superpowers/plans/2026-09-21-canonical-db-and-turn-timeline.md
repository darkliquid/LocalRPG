# Canonical Game Database, Turn Timeline, and Dialogue Attribution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Give every campaign exactly one canonical SQLite index, record a numbered entity-linked timeline for every turn, extract entities into Markdown then sync them on every turn, and record ordered narration/speech segments so playback uses each character's own voice.

**Architecture:** Entity notes and `history.jsonl` stay the sources of truth; `games/<id>/cache/index.db` becomes a derived, rebuildable index reached only through one entry point (`storage.OpenGameStore`). A new `engine.Timeline` owns every write across those three layers (entity notes, turn records, index rows) so files never point at nothing. Dialogue attribution is resolved once at write time into ordered `entity.TurnSegment` spans, replacing three duplicated read-time regexes.

**Tech Stack:** Go 1.27.1, `modernc.org/sqlite` (no CGO), Bubbletea/Lipgloss TUI, React 19 + TypeScript frontend, `mise` tasks.

**Spec:** `docs/superpowers/specs/2026-09-21-canonical-db-and-turn-timeline-design.md`

## Global Constraints

- Canonical game database path is `games/<id>/cache/index.db`. No other SQLite file may be created for a game; `game.db` is retired on first open and must never be read again.
- `history.jsonl` is the timeline source of truth and stays append-only; `cache/index.db` must be rebuildable from `entities/` plus `history.jsonl` alone.
- Entity notes are Markdown named `<id>.md`; the note is written to disk before the index is synced.
- Tests use the standard library only (`testing`, `t.TempDir()`); no testify, no mocks frameworks.
- Use `interface{}`, never `any`.
- `go vet ./...` and `mise run test` (Go suite plus `tsc --noEmit`) must pass before the work is considered done.
- Test commands, from the repo root: all `go test -count=1 ./...`; package `go test -count=1 ./pkg/storage/`; single test `go test -run TestName -count=1 ./pkg/engine/`.
- Do not add comments beyond doc comments on exported identifiers; do not add dependencies.

## Scope & Splitting

Phases 1 and 2 are independently shippable and reviewable on their own (Phase 1 alone fixes empty graph backlinks by making the GUI read the engine's database). Phases 3-5 share the `Turn` record shape and should land as one unit. Phases 6-7 are additive: Phase 6 can be dropped without breaking the timeline, and Phase 7 can be dropped without breaking the backend.

---

## Phase 1: One Canonical Database

### Task 1: Canonical path, shared handle pool, and SQLite pragmas

**Files:**
- Modify: `pkg/core/types.go` (add `GameDBPath` next to `GameDir`)
- Modify: `pkg/storage/db.go` (DSN pragmas)
- Create: `pkg/storage/pool.go`
- Test: `pkg/core/gamedbpath_test.go`, `pkg/storage/db_test.go`, `pkg/storage/pool_test.go`

**Interfaces:**
- Consumes: nothing
- Produces: `(*core.PathResolver).GameDBPath(gameID string) string`; `storage.OpenDB(path string) (*sql.DB, error)` now opens with `journal_mode=WAL`; `storage.Pool`, `storage.NewPool() *Pool`, `(*Pool).Store(path string) (*Store, error)`, `(*Pool).Close() error`

- [x] **Step 1: Write the failing tests**

`pkg/core/gamedbpath_test.go`:

```go
package core

import (
	"path/filepath"
	"testing"
)

func TestGameDBPath(t *testing.T) {
	paths := NewPathResolver("/srv/localrpg")
	want := filepath.Join("/srv/localrpg", "games", "campaign-01", "cache", "index.db")
	if got := paths.GameDBPath("campaign-01"); got != want {
		t.Errorf("GameDBPath() = %q, want %q", got, want)
	}

	custom := NewCustomPathResolver("s", "w", filepath.Join("/data", "games"), "c")
	wantCustom := filepath.Join("/data", "games", "x", "cache", "index.db")
	if got := custom.GameDBPath("x"); got != wantCustom {
		t.Errorf("GameDBPath() = %q, want %q", got, wantCustom)
	}
}
```

`pkg/storage/db_test.go`:

```go
package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenDBAppliesPragmas(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	assertStringPragma(t, db, "PRAGMA journal_mode", "wal")
	assertIntPragma(t, db, "PRAGMA busy_timeout", 5000)
	assertIntPragma(t, db, "PRAGMA foreign_keys", 1)
}

func assertStringPragma(t *testing.T, db *sql.DB, query, want string) {
	t.Helper()

	var got string
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	if got != want {
		t.Errorf("%s = %q, want %q", query, got, want)
	}
}

func assertIntPragma(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()

	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	if got != want {
		t.Errorf("%s = %d, want %d", query, got, want)
	}
}
```

The driver returns `PRAGMA busy_timeout` and `PRAGMA foreign_keys` as `int64`, so those two must be scanned into an `int`; only `PRAGMA journal_mode` is text.

`pkg/storage/pool_test.go`:

```go
package storage

import (
	"path/filepath"
	"testing"
)

func TestPoolSharesStorePerPath(t *testing.T) {
	pool := NewPool()
	dir := t.TempDir()

	first, err := pool.Store(filepath.Join(dir, "a.db"))
	if err != nil {
		t.Fatalf("Pool.Store failed: %v", err)
	}

	again, err := pool.Store(filepath.Join(dir, "a.db"))
	if err != nil {
		t.Fatalf("Pool.Store failed: %v", err)
	}
	if first != again {
		t.Errorf("expected the same store for the same path")
	}

	other, err := pool.Store(filepath.Join(dir, "b.db"))
	if err != nil {
		t.Fatalf("Pool.Store failed: %v", err)
	}
	if other == first {
		t.Errorf("expected a distinct store per path")
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("Pool.Close failed: %v", err)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -count=1 ./pkg/core/ ./pkg/storage/`
Expected: FAIL — `paths.GameDBPath undefined`, `undefined: Pool`, and the pragma test reporting `journal_mode = "delete"`.

- [x] **Step 3: Implement**

In `pkg/core/types.go`, directly after `GameDir`:

```go
// GameDBPath returns the canonical SQLite index for a campaign.
func (p *PathResolver) GameDBPath(gameID string) string {
	return filepath.Join(p.GameDir(gameID), "cache", "index.db")
}
```

In `pkg/storage/db.go`, replace `OpenDB` and add the pragma DSN:

```go
// pragmas are applied on every connection: WAL lets the GUI, the TUI, and a
// turn's extraction pass share one file, and the busy timeout turns lock
// contention into a short wait instead of an immediate failure.
const pragmas = "_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=NORMAL"

func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?"+pragmas)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return db, nil
}
```

Create `pkg/storage/pool.go`:

```go
package storage

import (
	"path/filepath"
	"sync"
)

// Pool hands out one shared Store per database path so callers stop opening and
// closing connections per request.
type Pool struct {
	mu     sync.Mutex
	stores map[string]*Store
}

func NewPool() *Pool {
	return &Pool{stores: make(map[string]*Store)}
}

// Store returns the shared Store for path, opening it on first use.
func (p *Pool) Store(path string) (*Store, error) {
	key := filepath.Clean(path)

	p.mu.Lock()
	defer p.mu.Unlock()

	if store, ok := p.stores[key]; ok {
		return store, nil
	}

	store, err := NewStore(key)
	if err != nil {
		return nil, err
	}
	store.shared = true
	p.stores[key] = store
	return store, nil
}

// Close releases every handle the pool opened.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var firstErr error
	for key, store := range p.stores {
		if err := store.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(p.stores, key)
	}
	return firstErr
}
```

A pooled store's lifetime belongs to the pool, so `Store.Close` must become a no-op for shared handles. Without this, `engine.Session.Close` and `gui.Service.CreateGame` (which calls it) close the handle out from under the pool and every later request fails with "sql: database is closed". In `pkg/storage/store.go`:

```go
type Store struct {
	db     *sql.DB
	shared bool
}

// Close releases the underlying handle. A store handed out by a Pool is shared,
// so its lifetime belongs to the pool and Close is a no-op.
func (s *Store) Close() error {
	if s.shared {
		return nil
	}
	return s.db.Close()
}
```

Add to `pkg/storage/pool_test.go`:

```go
func TestPooledStoreIgnoresClose(t *testing.T) {
	pool := NewPool()
	store, err := pool.Store(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("Pool.Store failed: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("closing a pooled store failed: %v", err)
	}

	ent := &entity.Entity{ID: "hero", Name: "Hero", Type: "character", Hash: "hash-hero"}
	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("a pooled store must stay usable after a deferred Close: %v", err)
	}
	if _, err := store.GetEntity("hero"); err != nil {
		t.Fatalf("pooled store lookup failed: %v", err)
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("Pool.Close failed: %v", err)
	}
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/core/ ./pkg/storage/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/core/types.go pkg/core/gamedbpath_test.go pkg/storage/db.go pkg/storage/db_test.go pkg/storage/pool.go pkg/storage/pool_test.go
git commit -m "feat(storage): add a canonical database path and shared handle pool"
```

### Task 2: Single game-store entry point with legacy migration

**Files:**
- Create: `pkg/storage/game.go`
- Test: `pkg/storage/game_test.go`

**Interfaces:**
- Consumes: `core.PathResolver.GameDBPath`, `storage.Pool`, `storage.NewStore`
- Produces: `storage.OpenGameStore(paths *core.PathResolver, gameID string) (*Store, error)`; `storage.CloseGameStores() error`

- [x] **Step 1: Write the failing test**

`pkg/storage/game_test.go`:

```go
package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestOpenGameStoreUsesCanonicalPathAndRetiresLegacyDB(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	t.Cleanup(func() { _ = CloseGameStores() })

	gameDir := paths.GameDir("campaign-01")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}

	legacy := filepath.Join(gameDir, "game.db")
	if err := os.WriteFile(legacy, []byte("legacy"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy+"-wal", []byte("legacy-wal"), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := OpenGameStore(paths, "campaign-01")
	if err != nil {
		t.Fatalf("OpenGameStore failed: %v", err)
	}

	if _, err := os.Stat(paths.GameDBPath("campaign-01")); err != nil {
		t.Errorf("expected canonical database at %s: %v", paths.GameDBPath("campaign-01"), err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("expected legacy game.db to be retired, stat err = %v", err)
	}
	if _, err := os.Stat(legacy + ".legacy"); err != nil {
		t.Errorf("expected legacy database to be renamed: %v", err)
	}
	if _, err := os.Stat(legacy + "-wal.legacy"); err != nil {
		t.Errorf("expected legacy WAL sidecar to be renamed: %v", err)
	}

	again, err := OpenGameStore(paths, "campaign-01")
	if err != nil {
		t.Fatalf("second OpenGameStore failed: %v", err)
	}
	if again != store {
		t.Errorf("expected the shared store for the canonical path")
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestOpenGameStoreUsesCanonicalPathAndRetiresLegacyDB -count=1 ./pkg/storage/`
Expected: FAIL — `undefined: OpenGameStore`

- [x] **Step 3: Implement**

Create `pkg/storage/game.go`:

```go
package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/core"
)

// legacyGameDB is the pre-timeline database name. It is retired on first open.
const legacyGameDB = "game.db"

var gameStores = NewPool()

// OpenGameStore returns the shared canonical store for a campaign, retiring a
// legacy game.db first. It is the only way a game database is opened.
func OpenGameStore(paths *core.PathResolver, gameID string) (*Store, error) {
	if paths == nil {
		return nil, fmt.Errorf("open game store %q: missing path resolver", gameID)
	}

	dbPath := paths.GameDBPath(gameID)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create game cache dir: %w", err)
	}

	if err := retireLegacyGameDB(paths, gameID); err != nil {
		return nil, err
	}

	store, err := gameStores.Store(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open game store %q: %w", gameID, err)
	}
	return store, nil
}

// CloseGameStores releases every pooled game store.
func CloseGameStores() error {
	return gameStores.Close()
}

func retireLegacyGameDB(paths *core.PathResolver, gameID string) error {
	legacy := filepath.Join(paths.GameDir(gameID), legacyGameDB)

	if _, err := os.Stat(legacy); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect legacy game database: %w", err)
	}

	// Walk the WAL sidecars too, so no stale bytes are left behind.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		from := legacy + suffix
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if err := os.Rename(from, from+".legacy"); err != nil {
			return fmt.Errorf("retire legacy game database: %w", err)
		}
	}
	return nil
}
```

- [x] **Step 4: Run the test to verify it passes**

Run: `go test -run TestOpenGameStoreUsesCanonicalPathAndRetiresLegacyDB -count=1 ./pkg/storage/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/storage/game.go pkg/storage/game_test.go
git commit -m "feat(storage): open every game through one canonical store entry point"
```

### Task 3: Engine and CLI use the canonical store

**Files:**
- Modify: `pkg/engine/game.go` (`InitGame` store creation)
- Modify: `cmd/localrpg/play.go` (store creation)
- Test: `pkg/engine/game_test.go`

**Interfaces:**
- Consumes: `storage.OpenGameStore`, `core.PathResolver.GameDBPath`
- Produces: no new exported API

- [x] **Step 1: Write the failing assertions**

Append to `TestGameInitAndLoad` in `pkg/engine/game_test.go`, before the closing brace:

```go
	if _, err := os.Stat(paths.GameDBPath("campaign-01")); err != nil {
		t.Errorf("expected the canonical game database: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.GameDir("campaign-01"), "game.db")); !os.IsNotExist(err) {
		t.Errorf("expected no legacy game.db beside the game, stat err = %v", err)
	}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestGameInitAndLoad -count=1 ./pkg/engine/`
Expected: PASS if the current code already writes `cache/index.db`; the `game.db` assertion is the guard that no second database appears. If both assertions pass here, keep them as regression guards.

- [x] **Step 3: Implement**

In `pkg/engine/game.go`, replace the store-opening block:

```go
	// Open store and run initial sync
	dbPath := filepath.Join(gameCacheDir, "index.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("init game store: %w", err)
	}
```

with:

```go
	// Open the canonical store and run initial sync
	store, err := storage.OpenGameStore(paths, gameID)
	if err != nil {
		return nil, fmt.Errorf("init game store: %w", err)
	}
```

In `cmd/localrpg/play.go`, replace:

```go
	dbPath := filepath.Join(gameDir, "cache", "index.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening game database %q: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer store.Close()
```

with:

```go
	store, err := storage.OpenGameStore(paths, gameID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening game database: %v\n", err)
		os.Exit(1)
	}
```

Note the removed `defer store.Close()`: the store is pooled and outlives the command body. `gameCacheDir` stays in the `MkdirAll` loop in `InitGame`; `OpenGameStore` also creates the directory, so the loop entry is harmless.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/ ./cmd/localrpg/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine/game.go pkg/engine/game_test.go cmd/localrpg/play.go
git commit -m "refactor(engine): open the canonical database from the engine and CLI"
```

### Task 4: GUI service reads and writes the canonical database

**Files:**
- Modify: `pkg/gui/service.go` (`GetEntity`, `SaveEntity`, add `store` helper)
- Modify: `pkg/gui/service_test.go` (fixture writes the canonical path)
- Modify: `cmd/localrpg/gui.go` (release pooled stores on shutdown)
- Test: `pkg/gui/service_test.go`

**Interfaces:**
- Consumes: `storage.OpenGameStore`
- Produces: `(*Service).store(gameID string) (*storage.Store, error)` (unexported)

- [x] **Step 1: Write the failing test**

In `pkg/gui/service_test.go`, add:

```go
func TestGetEntityReadsCanonicalDatabase(t *testing.T) {
	gameID, svc := setupTestGame(t)

	ent, err := svc.GetEntity(context.Background(), gameID, "player-elena")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if len(ent.Backlinks) == 0 {
		t.Errorf("expected backlinks from the canonical index, got none")
	}

	gameDir := svc.GetResolver().GameDir(gameID)
	if _, err := os.Stat(filepath.Join(gameDir, "game.db")); !os.IsNotExist(err) {
		t.Errorf("expected no legacy game.db, stat err = %v", err)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestGetEntityReadsCanonicalDatabase -count=1 ./pkg/gui/`
Expected: FAIL — the fixture still seeds `game.db`, so after the service switches to `cache/index.db` the backlinks are empty.

- [x] **Step 3: Implement**

In `pkg/gui/service.go`, add the helper next to `GetResolver`:

```go
// store returns the campaign's canonical index. The store is pooled and must not
// be closed by callers.
func (s *Service) store(gameID string) (*storage.Store, error) {
	return storage.OpenGameStore(s.resolver, gameID)
}
```

In `GetEntity`, replace:

```go
	dbPath := filepath.Join(gameDir, "game.db")
	store, err := storage.NewStore(dbPath)
	var backlinks []string
	if err == nil {
		defer store.Close()
		edges, _ := store.GetEdgesTo(entityID)
```

with:

```go
	var backlinks []string
	if store, err := s.store(gameID); err == nil {
		edges, _ := store.GetEdgesTo(entityID)
```

In `SaveEntity`, replace:

```go
	dbPath := filepath.Join(gameDir, "game.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		return nil // Non-fatal if db sync fails temporarily
	}
	defer store.Close()

	syncer := storage.NewSyncer(store)
	return syncer.SyncFile(path)
```

with:

```go
	store, err := s.store(gameID)
	if err != nil {
		return nil // Non-fatal if db sync fails temporarily
	}

	syncer := storage.NewSyncer(store)
	return syncer.SyncFile(path)
```

In `pkg/gui/service_test.go`'s `setupTestGame`, replace:

```go
	dbPath := filepath.Join(gamesDir, "game.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
```

with:

```go
	dbPath := filepath.Join(gamesDir, "cache", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
```

In `cmd/localrpg/gui.go`, after `svc := gui.NewService(cfg.Dir)`:

```go
	defer func() { _ = storage.CloseGameStores() }()
```

and add `github.com/darkliquid/localrpg/pkg/storage` to that file's imports. `CloseGameStores` lives in `storage` (the pool's home), not in `gui`.

Also give the two fixture entities an explicit `id:` in the frontmatter (`id: player-elena`, `id: captain-kaelen`) so their indexed IDs match their filenames. Without it they are indexed under an empty ID and every edge lookup misses, which only stayed invisible while reads went through filenames.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/gui/ ./cmd/localrpg/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/service_test.go cmd/localrpg/gui.go
git commit -m "fix(gui): read and write the same database the engine indexes"
```

## Phase 2: Timeline Persistence in the Index

### Task 5: Turn tables, row types, and save/load

**Files:**
- Modify: `pkg/storage/db.go` (append to the `schema` constant)
- Create: `pkg/storage/turn.go`
- Test: `pkg/storage/turn_test.go`

**Interfaces:**
- Consumes: `Store.db`
- Produces: `storage.TurnRecord{Number int; Timestamp time.Time; Mode, Input, Narration, RollJSON, AudioRefsJSON string; Entities []TurnEntityRef}`, `storage.TurnEntityRef{EntityID, Mention string}`, `(*Store).SaveTurn(rec TurnRecord) error`, `(*Store).GetTurn(number int) (*TurnRecord, error)`, `(*Store).ListEntitiesForTurn(number int) ([]TurnEntityRef, error)`

- [x] **Step 1: Write the failing test**

`pkg/storage/turn_test.go`:

```go
package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadTurn(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	rec := TurnRecord{
		Number:        1,
		Timestamp:     time.Date(2026, 9, 21, 14, 3, 11, 0, time.UTC),
		Mode:          "Do",
		Input:         "I search the harbour",
		Narration:     "The docks reek of brine.",
		RollJSON:      `{"notation":"1d20","total":15}`,
		AudioRefsJSON: `["assets/audio/abc.wav"]`,
		Entities: []TurnEntityRef{
			{EntityID: "hero", Mention: "player"},
			{EntityID: "aldon-harbour", Mention: "location"},
		},
	}

	if err := store.SaveTurn(rec); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}

	loaded, err := store.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn failed: %v", err)
	}
	if loaded.Narration != rec.Narration || loaded.Input != rec.Input || loaded.Mode != rec.Mode {
		t.Errorf("loaded turn mismatch: %+v", loaded)
	}
	if !loaded.Timestamp.Equal(rec.Timestamp) {
		t.Errorf("timestamp = %v, want %v", loaded.Timestamp, rec.Timestamp)
	}
	if loaded.RollJSON != rec.RollJSON || loaded.AudioRefsJSON != rec.AudioRefsJSON {
		t.Errorf("json columns mismatch: %+v", loaded)
	}
	if len(loaded.Entities) != 2 {
		t.Fatalf("expected 2 entity links, got %+v", loaded.Entities)
	}

	// Re-saving replaces links rather than accumulating them.
	rec.Entities = []TurnEntityRef{{EntityID: "hero", Mention: "player"}}
	if err := store.SaveTurn(rec); err != nil {
		t.Fatalf("second SaveTurn failed: %v", err)
	}
	reloaded, err := store.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn failed: %v", err)
	}
	if len(reloaded.Entities) != 1 || reloaded.Entities[0].EntityID != "hero" {
		t.Errorf("expected links to be replaced, got %+v", reloaded.Entities)
	}
}

func TestGetTurnMissing(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	if _, err := store.GetTurn(7); err == nil {
		t.Errorf("expected an error for a missing turn")
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestSaveAndLoadTurn -count=1 ./pkg/storage/`
Expected: FAIL — `undefined: TurnRecord`

- [x] **Step 3: Implement**

Append to the `schema` constant in `pkg/storage/db.go`, after the edge indexes:

```sql
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
);

CREATE INDEX IF NOT EXISTS idx_turn_entities_entity ON turn_entities(entity_id);
```

Create `pkg/storage/turn.go`:

```go
package storage

import (
	"fmt"
	"time"
)

// TurnEntityRef records how one entity was involved in a turn.
type TurnEntityRef struct {
	EntityID string
	Mention  string
}

// TurnRecord is the row shape of one timeline entry. Timestamps are stored as
// RFC3339 text so the value does not depend on driver time handling.
type TurnRecord struct {
	Number        int
	Timestamp     time.Time
	Mode          string
	Input         string
	Narration     string
	RollJSON      string
	AudioRefsJSON string
	Entities      []TurnEntityRef
}

// SaveTurn upserts one turn and replaces its entity links in a single transaction.
func (s *Store) SaveTurn(rec TurnRecord) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const upsert = `
	INSERT INTO turns (number, timestamp, mode, input, narration, roll_json, audio_refs_json)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(number) DO UPDATE SET
		timestamp = excluded.timestamp,
		mode = excluded.mode,
		input = excluded.input,
		narration = excluded.narration,
		roll_json = excluded.roll_json,
		audio_refs_json = excluded.audio_refs_json
	`
	stamp := rec.Timestamp.UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(upsert, rec.Number, stamp, rec.Mode, rec.Input, rec.Narration,
		emptyToNull(rec.RollJSON), emptyToNull(rec.AudioRefsJSON)); err != nil {
		return fmt.Errorf("upsert turn %d: %w", rec.Number, err)
	}

	if _, err := tx.Exec(`DELETE FROM turn_entities WHERE turn_number = ?`, rec.Number); err != nil {
		return fmt.Errorf("clear turn %d links: %w", rec.Number, err)
	}
	const link = `INSERT OR IGNORE INTO turn_entities (turn_number, entity_id, mention) VALUES (?, ?, ?)`
	for _, ref := range rec.Entities {
		if ref.EntityID == "" || ref.Mention == "" {
			continue
		}
		if _, err := tx.Exec(link, rec.Number, ref.EntityID, ref.Mention); err != nil {
			return fmt.Errorf("link entity %q to turn %d: %w", ref.EntityID, rec.Number, err)
		}
	}

	return tx.Commit()
}

// GetTurn returns one turn with its entity links.
func (s *Store) GetTurn(number int) (*TurnRecord, error) {
	const query = `
	SELECT number, timestamp, mode, input, narration,
	       COALESCE(roll_json, ''), COALESCE(audio_refs_json, '')
	FROM turns WHERE number = ?`

	var rec TurnRecord
	var stamp string
	if err := s.db.QueryRow(query, number).Scan(&rec.Number, &stamp, &rec.Mode, &rec.Input,
		&rec.Narration, &rec.RollJSON, &rec.AudioRefsJSON); err != nil {
		return nil, fmt.Errorf("get turn %d: %w", number, err)
	}

	parsed, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return nil, fmt.Errorf("parse turn %d timestamp: %w", number, err)
	}
	rec.Timestamp = parsed

	refs, err := s.ListEntitiesForTurn(number)
	if err != nil {
		return nil, err
	}
	rec.Entities = refs
	return &rec, nil
}

// ListEntitiesForTurn returns the entity links recorded for a turn.
func (s *Store) ListEntitiesForTurn(number int) ([]TurnEntityRef, error) {
	const query = `SELECT entity_id, mention FROM turn_entities WHERE turn_number = ? ORDER BY entity_id, mention`
	rows, err := s.db.Query(query, number)
	if err != nil {
		return nil, fmt.Errorf("list turn %d entities: %w", number, err)
	}
	defer rows.Close()

	refs := make([]TurnEntityRef, 0)
	for rows.Next() {
		var ref TurnEntityRef
		if err := rows.Scan(&ref.EntityID, &ref.Mention); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

func emptyToNull(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/storage/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/storage/db.go pkg/storage/turn.go pkg/storage/turn_test.go
git commit -m "feat(storage): persist turns and their entity links"
```

### Task 6: Timeline queries

**Files:**
- Modify: `pkg/storage/turn.go`
- Test: `pkg/storage/turn_test.go`

**Interfaces:**
- Consumes: `SaveTurn`
- Produces: `(*Store).ListTurns(limit, offset int) ([]TurnRecord, error)`, `(*Store).ListTurnsForEntity(entityID string) ([]int, error)`, `(*Store).MaxTurnNumber() (int, error)`, `(*Store).CountTurns() (int, error)`

- [x] **Step 1: Write the failing test**

Append to `pkg/storage/turn_test.go`:

```go
func TestTurnQueries(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	for n := 1; n <= 3; n++ {
		rec := TurnRecord{
			Number:    n,
			Timestamp: time.Date(2026, 9, 21, 10, n, 0, 0, time.UTC),
			Mode:      "Do",
			Input:     "input",
			Narration: "narration",
			Entities: []TurnEntityRef{
				{EntityID: "hero", Mention: "player"},
				{EntityID: "garrick", Mention: "extracted"},
			},
		}
		if n == 3 {
			rec.Entities = []TurnEntityRef{{EntityID: "hero", Mention: "player"}}
		}
		if err := store.SaveTurn(rec); err != nil {
			t.Fatalf("SaveTurn(%d) failed: %v", n, err)
		}
	}

	turns, err := store.ListTurns(0, 0)
	if err != nil {
		t.Fatalf("ListTurns failed: %v", err)
	}
	if len(turns) != 3 || turns[0].Number != 1 || turns[2].Number != 3 {
		t.Fatalf("expected turns 1..3 ascending, got %+v", turns)
	}

	limited, err := store.ListTurns(2, 1)
	if err != nil {
		t.Fatalf("ListTurns failed: %v", err)
	}
	if len(limited) != 2 || limited[0].Number != 2 {
		t.Errorf("expected turns 2..3, got %+v", limited)
	}

	heroTurns, err := store.ListTurnsForEntity("hero")
	if err != nil {
		t.Fatalf("ListTurnsForEntity failed: %v", err)
	}
	if len(heroTurns) != 3 || heroTurns[0] != 1 || heroTurns[2] != 3 {
		t.Errorf("expected hero in turns 1,2,3, got %v", heroTurns)
	}

	garrickTurns, err := store.ListTurnsForEntity("garrick")
	if err != nil {
		t.Fatalf("ListTurnsForEntity failed: %v", err)
	}
	if len(garrickTurns) != 2 {
		t.Errorf("expected garrick in turns 1,2, got %v", garrickTurns)
	}

	if max, err := store.MaxTurnNumber(); err != nil || max != 3 {
		t.Errorf("MaxTurnNumber = %d, %v; want 3", max, err)
	}
	if count, err := store.CountTurns(); err != nil || count != 3 {
		t.Errorf("CountTurns = %d, %v; want 3", count, err)
	}
}

func TestTurnQueriesOnEmptyStore(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	max, err := store.MaxTurnNumber()
	if err != nil {
		t.Fatalf("MaxTurnNumber failed: %v", err)
	}
	if max != 0 {
		t.Errorf("MaxTurnNumber = %d, want 0", max)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestTurnQueries -count=1 ./pkg/storage/`
Expected: FAIL — `store.ListTurns undefined`

- [x] **Step 3: Implement**

Append to `pkg/storage/turn.go`:

```go
// ListTurns returns timeline entries in ascending order. A limit of 0 returns all.
func (s *Store) ListTurns(limit, offset int) ([]TurnRecord, error) {
	query := `
	SELECT number, timestamp, mode, input, narration,
	       COALESCE(roll_json, ''), COALESCE(audio_refs_json, '')
	FROM turns ORDER BY number`
	args := make([]interface{}, 0, 2)
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list turns: %w", err)
	}
	defer rows.Close()

	turns := make([]TurnRecord, 0)
	for rows.Next() {
		var rec TurnRecord
		var stamp string
		if err := rows.Scan(&rec.Number, &stamp, &rec.Mode, &rec.Input, &rec.Narration,
			&rec.RollJSON, &rec.AudioRefsJSON); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			return nil, fmt.Errorf("parse turn %d timestamp: %w", rec.Number, err)
		}
		rec.Timestamp = parsed
		turns = append(turns, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return turns, nil
}

// ListTurnsForEntity returns the ascending turn numbers an entity was involved in.
func (s *Store) ListTurnsForEntity(entityID string) ([]int, error) {
	const query = `SELECT DISTINCT turn_number FROM turn_entities WHERE entity_id = ? ORDER BY turn_number`
	rows, err := s.db.Query(query, entityID)
	if err != nil {
		return nil, fmt.Errorf("list turns for entity %q: %w", entityID, err)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return numbers, nil
}

// MaxTurnNumber returns the highest indexed turn number, or 0 when empty.
func (s *Store) MaxTurnNumber() (int, error) {
	var max int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(number), 0) FROM turns`).Scan(&max); err != nil {
		return 0, fmt.Errorf("read max turn number: %w", err)
	}
	return max, nil
}

// CountTurns returns the number of indexed turns.
func (s *Store) CountTurns() (int, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM turns`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count turns: %w", err)
	}
	return count, nil
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/storage/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/storage/turn.go pkg/storage/turn_test.go
git commit -m "feat(storage): query turns by number, range, and entity"
```

### Task 7: Delete indexed turns for rewinds

**Files:**
- Modify: `pkg/storage/turn.go`
- Test: `pkg/storage/turn_test.go`

**Interfaces:**
- Consumes: `SaveTurn`
- Produces: `(*Store).DeleteTurnsFrom(number int) error`

- [x] **Step 1: Write the failing test**

Append to `pkg/storage/turn_test.go`:

```go
func TestDeleteTurnsFrom(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	for n := 1; n <= 3; n++ {
		rec := TurnRecord{
			Number:    n,
			Timestamp: time.Date(2026, 9, 21, 10, n, 0, 0, time.UTC),
			Mode:      "Do",
			Input:     "input",
			Narration: "narration",
			Entities:  []TurnEntityRef{{EntityID: "hero", Mention: "player"}},
		}
		if err := store.SaveTurn(rec); err != nil {
			t.Fatalf("SaveTurn(%d) failed: %v", n, err)
		}
	}

	if err := store.DeleteTurnsFrom(3); err != nil {
		t.Fatalf("DeleteTurnsFrom failed: %v", err)
	}

	count, err := store.CountTurns()
	if err != nil {
		t.Fatalf("CountTurns failed: %v", err)
	}
	if count != 2 {
		t.Errorf("CountTurns = %d, want 2", count)
	}

	heroTurns, err := store.ListTurnsForEntity("hero")
	if err != nil {
		t.Fatalf("ListTurnsForEntity failed: %v", err)
	}
	if len(heroTurns) != 2 {
		t.Errorf("expected links for turns 1,2 only, got %v", heroTurns)
	}

	// Deleting beyond the highest turn is a no-op, not an error.
	if err := store.DeleteTurnsFrom(99); err != nil {
		t.Fatalf("DeleteTurnsFrom(99) failed: %v", err)
	}
}

func TestDeleteTurnsFromMissingTableRowsIsSafe(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	if err := store.DeleteTurnsFrom(1); err != nil {
		t.Fatalf("DeleteTurnsFrom on an empty store failed: %v", err)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestDeleteTurnsFrom -count=1 ./pkg/storage/`
Expected: FAIL — `store.DeleteTurnsFrom undefined`

- [x] **Step 3: Implement**

Append to `pkg/storage/turn.go`:

```go
// DeleteTurnsFrom discards a turn and everything after it, including their entity
// links. It backs /undo, where the JSONL log and the index must agree.
func (s *Store) DeleteTurnsFrom(number int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM turn_entities WHERE turn_number >= ?`, number); err != nil {
		return fmt.Errorf("delete turn links from %d: %w", number, err)
	}
	if _, err := tx.Exec(`DELETE FROM turns WHERE number >= ?`, number); err != nil {
		return fmt.Errorf("delete turns from %d: %w", number, err)
	}

	return tx.Commit()
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/storage/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/storage/turn.go pkg/storage/turn_test.go
git commit -m "feat(storage): delete indexed turns when the timeline is rewound"
```

## Phase 3: The Turn Record Model

### Task 8: `Narration`, `Entities`, and the legacy `output` alias

**Files:**
- Create: `pkg/entity/mention.go`
- Modify: `pkg/engine/history.go` (`Turn` struct, `loadHistoryUnlocked`)
- Modify: `pkg/engine/history_test.go` (existing literals use `Output`)
- Test: `pkg/engine/history_test.go`

**Interfaces:**
- Consumes: nothing
- Produces: `entity.Mention{ID, Kind string}`, `entity.MentionPlayer`, `entity.MentionLocation`, `entity.MentionWikilink`, `entity.MentionExtracted`; `engine.Turn.Narration string`, `engine.Turn.Entities []entity.Mention`, `engine.Turn.LegacyOutput string`, `(Turn).Prose() string`

- [x] **Step 1: Write the failing test**

Append to `pkg/engine/history_test.go`:

```go
func TestLoadHistoryNormalisesLegacyOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	legacy := `{"number":1,"timestamp":"2026-09-20T10:00:00Z","mode":"Do","input":"look","output":"You look around.","entities":[{"id":"hero","mention":"player"}]}` + "\n"
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	logger := NewHistoryLogger(path)
	turns, err := logger.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if got := turns[0].Prose(); got != "You look around." {
		t.Errorf("Prose() = %q, want the legacy output text", got)
	}
	if turns[0].LegacyOutput != "" {
		t.Errorf("expected the legacy field to be cleared after normalisation")
	}
	if len(turns[0].Entities) != 1 || turns[0].Entities[0].Kind != entity.MentionPlayer {
		t.Errorf("expected entity involvement to survive loading, got %+v", turns[0].Entities)
	}

	if err := logger.RewindToTurn(1); err != nil {
		t.Fatalf("RewindToTurn failed: %v", err)
	}
	rewritten, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rewritten), `"output"`) {
		t.Errorf("expected rewrites to drop the legacy field, got %s", rewritten)
	}
	if !strings.Contains(string(rewritten), `"narration":"You look around."`) {
		t.Errorf("expected rewrites to emit narration, got %s", rewritten)
	}
}
```

Replace `Output:` with `Narration:` in the existing `TestHistoryLogger` literals in the same file so the package still compiles.

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestLoadHistoryNormalisesLegacyOutput -count=1 ./pkg/engine/`
Expected: FAIL — `turns[0].Prose undefined` and `undefined: entity.MentionPlayer`

- [x] **Step 3: Implement**

Create `pkg/entity/mention.go`:

```go
package entity

// Mention kinds describe how an entity was involved in a turn.
const (
	MentionPlayer    = "player"
	MentionLocation  = "location"
	MentionWikilink  = "wikilink"
	MentionExtracted = "extracted"
)

// Mention records how one entity was involved in a turn.
type Mention struct {
	ID   string `json:"id"`
	Kind string `json:"mention"`
}
```

In `pkg/engine/history.go`, replace the `Turn` struct:

```go
type Turn struct {
	Number    int               `json:"number"`
	Timestamp time.Time         `json:"timestamp"`
	Mode      string            `json:"mode"` // "Do", "Say", "Story", "Roll", "GM", "System"
	Input     string            `json:"input"`
	Narration string            `json:"narration"`
	Roll      *rules.RollResult `json:"roll,omitempty"`
	Entities  []entity.Mention  `json:"entities,omitempty"`
	AudioRefs []string          `json:"audio_refs,omitempty"`

	// LegacyOutput is only populated when reading records written before the
	// narration rename. New records must not set it.
	LegacyOutput string `json:"output,omitempty"`
}

// Prose returns the narrator-visible text regardless of which field holds it.
func (t Turn) Prose() string {
	if strings.TrimSpace(t.Narration) != "" {
		return t.Narration
	}
	return t.LegacyOutput
}
```

In `loadHistoryUnlocked`, replace the unmarshal block with:

```go
		var t Turn
		if err := json.Unmarshal(line, &t); err == nil {
			if strings.TrimSpace(t.Narration) == "" && t.LegacyOutput != "" {
				t.Narration = t.LegacyOutput
				t.LegacyOutput = ""
			}
			turns = append(turns, t)
		}
```

Add `"strings"` and `"github.com/darkliquid/localrpg/pkg/entity"` to the file's imports.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/ ./pkg/entity/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/entity/mention.go pkg/engine/history.go pkg/engine/history_test.go
git commit -m "feat(engine): record entity involvement and the narrator rewrite per turn"
```

### Task 9: Preserve the player's raw prompt in the turn record

**Files:**
- Modify: `pkg/engine/orchestrator.go` (`ProcessAction`)
- Test: `pkg/engine/orchestrator_test.go`

**Interfaces:**
- Consumes: `Turn.Input`, `rules.EvaluateRoll`
- Produces: no new exported API; `Turn.Input` is now the untouched player entry and `generationPrompt` is a local

- [x] **Step 1: Write the failing test**

Append to `pkg/engine/orchestrator_test.go`:

```go
func TestProcessActionPreservesRawInput(t *testing.T) {
	tempDir := t.TempDir()

	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.SaveEntity(&entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy tavern."})
	store.SaveEntity(&entity.Entity{ID: "player", Name: "Sean", Type: "character"})

	history := NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	model := &mockOrchestratorModel{response: "The dice settle."}
	router := harness.NewRouter()
	router.RegisterProvider(model)
	router.AssignRole("gm", "mock-gm")

	orchestrator := NewTurnOrchestrator(store, history, nil, router, "tavern", "player")

	turn, err := orchestrator.ProcessAction(context.Background(), "Roll", "1d20+5")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	if turn.Input != "1d20+5" {
		t.Errorf("Input = %q, want the raw player entry", turn.Input)
	}
	if turn.Roll == nil {
		t.Errorf("expected the roll to be evaluated")
	}
	if !strings.Contains(model.lastPrompt, "I rolled 1d20+5") {
		t.Errorf("expected the resolved roll in the generation prompt, got %q", model.lastPrompt)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestProcessActionPreservesRawInput -count=1 ./pkg/engine/`
Expected: FAIL — `Input = "I rolled 1d20+5 with result N"`, the raw entry is lost.

- [x] **Step 3: Implement**

In `pkg/engine/orchestrator.go`, introduce a local prompt and stop mutating the input. Add after `var rollRes *rules.RollResult`:

```go
	generationPrompt := actionInput
```

Replace the roll branch:

```go
	} else if strings.EqualFold(mode, "Roll") {
		// Evaluate dice roll
		r, err := rules.EvaluateRoll(actionInput)
		if err == nil {
			rollRes = r
			actionInput = fmt.Sprintf("I rolled %s with result %d", r.Notation, r.Total)
		}
	}
```

with:

```go
	} else if strings.EqualFold(mode, "Roll") {
		if r, err := rules.EvaluateRoll(actionInput); err == nil {
			rollRes = r
			generationPrompt = fmt.Sprintf("I rolled %s with result %d", r.Notation, r.Total)
		}
	}
```

Replace the context assembly call:

```go
	contextPrompt, err := o.assembler.AssembleContextWithProfiles(o.locationID, o.playerID, actionInput, o.rulesPrompt, o.lorePrompt, o.voiceProfiles)
```

with:

```go
	contextPrompt, err := o.assembler.AssembleContextWithProfiles(o.locationID, o.playerID, generationPrompt, o.rulesPrompt, o.lorePrompt, o.voiceProfiles)
```

Finally, in the constructed `Turn`, set both fields:

```go
	turn := Turn{
		Number:    turnNum,
		Timestamp: time.Now(),
		Mode:      mode,
		Input:     actionInput,
		Narration: resp.Text,
		Roll:      rollRes,
	}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/orchestrator_test.go
git commit -m "fix(engine): keep the player's own words in the turn record"
```

### Task 10: `Timeline` replay and index repair

**Files:**
- Create: `pkg/engine/timeline.go`
- Modify: `pkg/engine/game.go` (`InitGame` repairs the index on open)
- Modify: `cmd/localrpg/play.go` (repair on open)
- Test: `pkg/engine/timeline_test.go`

**Interfaces:**
- Consumes: `HistoryLogger.LoadHistory`, `storage.SaveTurn/DeleteTurnsFrom/CountTurns/MaxTurnNumber`
- Produces: `engine.NewTimeline(paths *core.PathResolver, store *storage.Store, history *HistoryLogger, gameID string) *Timeline`, `(*Timeline).SyncTurns() (int, error)`, `(*Timeline).EnsureIndexed() error`

- [x] **Step 1: Write the failing test**

`pkg/engine/timeline_test.go`:

```go
package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestTimelineEnsureIndexedReplaysHistory(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	store := newTestStore(t)
	history := NewHistoryLogger(filepath.Join(t.TempDir(), "history.jsonl"))
	timeline := NewTimeline(paths, store, history, "campaign-01")

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"You look around.","entities":[{"id":"hero","mention":"player"},{"id":"garrick","mention":"extracted"}]}` + "\n"
	record += `{"number":2,"timestamp":"2026-09-21T10:05:00Z","mode":"Say","input":"hello","narration":"Garrick nods.","entities":[{"id":"garrick","mention":"wikilink"}]}` + "\n"

	if err := os.WriteFile(history.path, []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	if err := timeline.EnsureIndexed(); err != nil {
		t.Fatalf("EnsureIndexed failed: %v", err)
	}

	count, err := store.CountTurns()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("CountTurns = %d, want 2", count)
	}

	garrick, err := store.ListTurnsForEntity("garrick")
	if err != nil {
		t.Fatal(err)
	}
	if len(garrick) != 2 || garrick[0] != 1 || garrick[1] != 2 {
		t.Errorf("expected garrick in turns 1,2, got %v", garrick)
	}

	// Second run is a no-op.
	if err := timeline.EnsureIndexed(); err != nil {
		t.Fatalf("second EnsureIndexed failed: %v", err)
	}
	if count, _ := store.CountTurns(); count != 2 {
		t.Errorf("CountTurns = %d after a no-op reindex, want 2", count)
	}

	// A rewind that only touched the log is repaired on the next open.
	truncated := strings.SplitAfter(record, "\n")[0]
	if err := os.WriteFile(history.path, []byte(truncated), 0644); err != nil {
		t.Fatal(err)
	}
	if err := timeline.EnsureIndexed(); err != nil {
		t.Fatalf("third EnsureIndexed failed: %v", err)
	}
	if count, _ := store.CountTurns(); count != 1 {
		t.Errorf("CountTurns = %d after truncating the log, want 1", count)
	}
}

func TestTimelineSyncTurnsRejectsUnreadableHistory(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	store := newTestStore(t)

	historyPath := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(historyPath, []byte("{\"number\":1,\"mode\":\"Do\"}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	timeline := NewTimeline(paths, store, NewHistoryLogger(historyPath), "campaign-01")
	indexed, err := timeline.SyncTurns()
	if err != nil {
		t.Fatalf("SyncTurns failed: %v", err)
	}
	if indexed != 1 {
		t.Errorf("indexed = %d, want 1", indexed)
	}

	if _, err := store.GetTurn(1); err != nil {
		t.Errorf("expected turn 1 in the index: %v", err)
	}
}

func TestTimelineMapsEntityMentions(t *testing.T) {
	store := newTestStore(t)
	history := NewHistoryLogger(filepath.Join(t.TempDir(), "history.jsonl"))
	timeline := NewTimeline(core.NewPathResolver(t.TempDir()), store, history, "campaign-01")

	turn := Turn{
		Number:    1,
		Mode:      "Do",
		Input:     "look",
		Narration: "You look around.",
		Entities: []entity.Mention{
			{ID: "hero", Kind: entity.MentionPlayer},
			{ID: "garrick", Kind: entity.MentionExtracted},
		},
	}
	if err := timeline.indexTurn(turn); err != nil {
		t.Fatalf("indexTurn failed: %v", err)
	}

	rec, err := store.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn failed: %v", err)
	}
	if len(rec.Entities) != 2 {
		t.Fatalf("expected 2 links, got %+v", rec.Entities)
	}
	if rec.Narration != turn.Narration {
		t.Errorf("Narration = %q, want %q", rec.Narration, turn.Narration)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestTimeline -count=1 ./pkg/engine/`
Expected: FAIL — `undefined: NewTimeline`

- [x] **Step 3: Implement**

Create `pkg/engine/timeline.go`:

```go
package engine

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// Timeline owns every write a turn makes across the campaign's entity notes,
// history.jsonl, and the SQLite index.
type Timeline struct {
	paths   *core.PathResolver
	store   *storage.Store
	history *HistoryLogger
	gameID  string
}

func NewTimeline(paths *core.PathResolver, store *storage.Store, history *HistoryLogger, gameID string) *Timeline {
	return &Timeline{paths: paths, store: store, history: history, gameID: gameID}
}

// EntitiesDir returns the campaign directory holding entity notes.
func (t *Timeline) EntitiesDir() string {
	return filepath.Join(t.paths.GameDir(t.gameID), "entities")
}

// SyncTurns replays history.jsonl into the index and removes rows for turns the
// log no longer contains. It returns the number of replayed turns.
func (t *Timeline) SyncTurns() (int, error) {
	turns, err := t.history.LoadHistory()
	if err != nil {
		return 0, fmt.Errorf("load history: %w", err)
	}
	if err := t.indexTurns(turns); err != nil {
		return 0, err
	}
	return len(turns), nil
}

// EnsureIndexed repairs the turn index when it disagrees with history.jsonl,
// which is the only durable record of the timeline.
func (t *Timeline) EnsureIndexed() error {
	turns, err := t.history.LoadHistory()
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}

	count, err := t.store.CountTurns()
	if err != nil {
		return fmt.Errorf("count indexed turns: %w", err)
	}
	max, err := t.store.MaxTurnNumber()
	if err != nil {
		return fmt.Errorf("read max indexed turn: %w", err)
	}

	if count == len(turns) && max == lastTurnNumber(turns) {
		return nil
	}
	return t.indexTurns(turns)
}

func (t *Timeline) indexTurns(turns []Turn) error {
	for _, turn := range turns {
		if err := t.indexTurn(turn); err != nil {
			return err
		}
	}

	highest := lastTurnNumber(turns)
	indexed, err := t.store.MaxTurnNumber()
	if err != nil {
		return fmt.Errorf("read max indexed turn: %w", err)
	}
	if indexed > highest {
		if err := t.store.DeleteTurnsFrom(highest + 1); err != nil {
			return fmt.Errorf("prune indexed turns: %w", err)
		}
	}
	return nil
}

func (t *Timeline) indexTurn(turn Turn) error {
	if err := t.store.SaveTurn(turnRecord(turn)); err != nil {
		return fmt.Errorf("index turn %d: %w", turn.Number, err)
	}
	return nil
}

func lastTurnNumber(turns []Turn) int {
	if len(turns) == 0 {
		return 0
	}
	return turns[len(turns)-1].Number
}

func turnRecord(turn Turn) storage.TurnRecord {
	rec := storage.TurnRecord{
		Number:    turn.Number,
		Timestamp: turn.Timestamp,
		Mode:      turn.Mode,
		Input:     turn.Input,
		Narration: turn.Prose(),
	}

	if turn.Roll != nil {
		if data, err := json.Marshal(turn.Roll); err == nil {
			rec.RollJSON = string(data)
		}
	}
	if len(turn.AudioRefs) > 0 {
		if data, err := json.Marshal(turn.AudioRefs); err == nil {
			rec.AudioRefsJSON = string(data)
		}
	}
	for _, mention := range turn.Entities {
		rec.Entities = append(rec.Entities, storage.TurnEntityRef{
			EntityID: mention.ID,
			Mention:  mention.Kind,
		})
	}
	return rec
}

var _ = entity.Mention{}
```

Drop the trailing `var _ = entity.Mention{}` line and the `entity` import if the compiler reports it unused — it is only needed once Task 14 adds entity writes.

In `pkg/engine/game.go`, after the initial `syncer.Sync(gameEntitiesDir)` call and before resolving the start location, add:

```go
	// Repair the derived index if the log and the index disagree
	timeline := NewTimeline(paths, store, NewHistoryLogger(filepath.Join(gameDir, "history.jsonl")), gameID)
	if err := timeline.EnsureIndexed(); err != nil {
		store.Close()
		return nil, fmt.Errorf("index turns: %w", err)
	}
```

In `cmd/localrpg/play.go`, after the entity sync and before resolving the start location, add:

```go
	timeline := engine.NewTimeline(paths, store, engine.NewHistoryLogger(filepath.Join(gameDir, "history.jsonl")), gameID)
	if err := timeline.EnsureIndexed(); err != nil {
		fmt.Fprintf(os.Stderr, "Error indexing turns: %v\n", err)
		os.Exit(1)
	}
```

and move the existing `history := engine.NewHistoryLogger(historyPath)` line above it so both use one logger.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/ ./cmd/localrpg/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine/timeline.go pkg/engine/timeline_test.go pkg/engine/game.go cmd/localrpg/play.go
git commit -m "feat(engine): rebuild the turn index from the timeline on open"
```

### Task 11: Read sites use `Prose()`

**Files:**
- Modify: `pkg/tui/app.go` (two `turn.Output` reads)
- Modify: `pkg/gui/service.go` (`GetChronicle`)
- Modify: `pkg/export/script.go` (`Compile`)
- Modify: `pkg/export/script_test.go` (literals use `Output`)
- Test: `pkg/export/script_test.go`

**Interfaces:**
- Consumes: `engine.Turn.Prose()`
- Produces: no new exported API

- [x] **Step 1: Write the failing test**

Append to `pkg/export/script_test.go`:

```go
func TestCompileReadsLegacyTurns(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "legacy-campaign")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "id: legacy-campaign\nname: Legacy Campaign\n"
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	legacy := `{"number":1,"timestamp":"2026-09-20T10:00:00Z","mode":"Do","input":"look","output":"The hall is quiet."}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "legacy-campaign")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if len(script.Beats) != 1 || script.Beats[0].Prose != "The hall is quiet." {
		t.Errorf("expected the legacy output to compile as prose, got %+v", script.Beats)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestCompileReadsLegacyTurns -count=1 ./pkg/export/`
Expected: FAIL — `script.Beats[0].Prose` is empty because `turn.Output` is now always empty.

- [x] **Step 3: Implement**

- `pkg/export/script.go`: replace both `turn.Output` reads with `turn.Prose()`.
- `pkg/gui/service.go` in `GetChronicle`: `Prose: turn.Prose()`.
- `pkg/tui/app.go`: replace `turn.Output` with `turn.Prose()` in both the markdown render and the fallback write.
- `pkg/export/script_test.go`: rename `Output:` literals to `Narration:` in the existing tests.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/export/ ./pkg/gui/ ./pkg/tui/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/tui/app.go pkg/gui/service.go pkg/export/script.go pkg/export/script_test.go
git commit -m "refactor(export,tui,gui): read turn prose through one accessor"
```

## Phase 4: Extraction On Every Turn

### Task 12: Entity notes carry their turn numbers

**Files:**
- Modify: `pkg/entity/entity.go` (`EntityFrontmatter`, `Entity`, parse, serialize)
- Modify: `pkg/storage/store.go` (`SaveEntity`, `GetEntity`)
- Test: `pkg/entity/entity_test.go`, `pkg/storage/store_test.go`

**Interfaces:**
- Consumes: nothing
- Produces: `entity.EntityFrontmatter.History []int`, `entity.Entity.History []int`, round-tripped through `ParseMarkdownEntity`/`SerializeMarkdown` and persisted in `entities.frontmatter_json`

- [x] **Step 1: Write the failing tests**

Append to `pkg/entity/entity_test.go`:

```go
func TestEntityHistoryRoundTrip(t *testing.T) {
	doc := `---
id: garrick-the-fence
name: Garrick the Fence
type: character
history: [1, 4, 9]
---
A shadowy broker.
`
	ent, err := ParseMarkdownEntity([]byte(doc))
	if err != nil {
		t.Fatalf("ParseMarkdownEntity failed: %v", err)
	}
	if !reflect.DeepEqual(ent.History, []int{1, 4, 9}) {
		t.Fatalf("History = %v, want [1 4 9]", ent.History)
	}

	serialized, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown failed: %v", err)
	}
	reparsed, err := ParseMarkdownEntity(serialized)
	if err != nil {
		t.Fatalf("reparse failed: %v", err)
	}
	if !reflect.DeepEqual(reparsed.History, ent.History) {
		t.Errorf("History after round trip = %v, want %v", reparsed.History, ent.History)
	}
}

func TestEntityWithoutHistoryOmitsFrontmatter(t *testing.T) {
	ent := &Entity{ID: "hero", Name: "Sean", Type: "character", Body: "A traveller."}
	serialized, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown failed: %v", err)
	}
	if strings.Contains(string(serialized), "history:") {
		t.Errorf("expected no history frontmatter when empty, got %s", serialized)
	}
}
```

Append to `pkg/storage/store_test.go`:

```go
func TestStoreRoundTripsEntityHistory(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ent := &entity.Entity{ID: "garrick", Name: "Garrick", Type: "character", Hash: "hash-garrick", History: []int{2, 5}}
	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("SaveEntity failed: %v", err)
	}

	loaded, err := store.GetEntity("garrick")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if len(loaded.History) != 2 || loaded.History[0] != 2 || loaded.History[1] != 5 {
		t.Errorf("History = %v, want [2 5]", loaded.History)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestEntityHistoryRoundTrip|TestStoreRoundTripsEntityHistory" -count=1 ./pkg/entity/ ./pkg/storage/`
Expected: FAIL — `ent.History undefined`

- [x] **Step 3: Implement**

In `pkg/entity/entity.go`: add `History []int \`yaml:"history,omitempty" json:"history,omitempty"\`` to `EntityFrontmatter`, add `History []int` to `Entity`, set `History: fm.History` in `ParseMarkdownEntity`, and `History: e.History` in `SerializeMarkdown`'s `EntityFrontmatter` literal. Add `"strings"` to `pkg/entity/entity_test.go` imports if missing.

In `pkg/storage/store.go`: add `"history": e.History` to the `fmMeta` map in `SaveEntity`, and in `GetEntity` read it back:

```go
		if history, ok := meta["history"].([]interface{}); ok {
			for _, value := range history {
				if number, ok := value.(float64); ok {
					ent.History = append(ent.History, int(number))
				}
			}
		}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/entity/ ./pkg/storage/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/entity/entity.go pkg/entity/entity_test.go pkg/storage/store.go pkg/storage/store_test.go
git commit -m "feat(entity): let notes record the turns they took part in"
```

### Task 13: Extraction returns records; mentions and speakers resolve separately

**Files:**
- Modify: `pkg/harness/extractor.go`
- Modify: `pkg/harness/extractor_test.go` (existing tests call `NewEntityExtractor`/`ExtractFromTurn`)
- Modify: `pkg/entity/entity.go` (add `WikilinkTargets`)
- Test: `pkg/entity/entity_test.go`, `pkg/harness/extractor_test.go`

**Interfaces:**
- Consumes: `storage.Store.ListEntities`, `storage.Store.GetEntity`, `entity.Slugify`, `entity.WikilinkTarget`
- Produces: `harness.Extractor`, `harness.NewExtractor(model ModelProvider) *Extractor`, `(*Extractor).SetVoiceProfiles([]config.VoiceProfile)`, `(*Extractor).Extract(ctx context.Context, narrative string) ([]ExtractedEntity, error)`, `harness.MergeExtractedEntity(existing *entity.Entity, raw *ExtractedEntity) *entity.Entity`, `harness.ResolveEntityMentions(store *storage.Store, playerID, locationID string, texts ...string) []entity.Mention`, `harness.ResolveSpeakerID(store *storage.Store, name string) string`, `entity.WikilinkTargets(text string) []string`

- [x] **Step 1: Write the failing tests**

Append to `pkg/harness/extractor_test.go`:

```go
func TestResolveEntityMentions(t *testing.T) {
	store := newTestEntityStore(t)
	saveExtractorEntity(t, store, &entity.Entity{ID: "hero", Name: "Sean", Type: "character", Hash: "h1"})
	saveExtractorEntity(t, store, &entity.Entity{ID: "aldon-tavern", Name: "Alden Tavern", Type: "location", Hash: "h2"})
	saveExtractorEntity(t, store, &entity.Entity{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Hash: "h3"})

	mentions := ResolveEntityMentions(store, "hero", "aldon-tavern", "Garrick nods at [[Garrick the Fence]] and [[Nobody At All]].")
	if len(mentions) != 3 {
		t.Fatalf("expected 3 mentions, got %+v", mentions)
	}
	if mentions[0].ID != "hero" || mentions[0].Kind != entity.MentionPlayer {
		t.Errorf("expected the player first, got %+v", mentions[0])
	}
	if mentions[1].ID != "aldon-tavern" || mentions[1].Kind != entity.MentionLocation {
		t.Errorf("expected the location second, got %+v", mentions[1])
	}
	if mentions[2].ID != "garrick-the-fence" || mentions[2].Kind != entity.MentionWikilink {
		t.Errorf("expected the wikilink third, got %+v", mentions[2])
	}
}

func TestResolveEntityMentionsSkipsMissingEntities(t *testing.T) {
	store := newTestEntityStore(t)

	mentions := ResolveEntityMentions(store, "ghost-player", "", "[[Nobody]]")
	if len(mentions) != 0 {
		t.Errorf("expected no mentions for unknown entities, got %+v", mentions)
	}
}

func TestResolveSpeakerID(t *testing.T) {
	store := newTestEntityStore(t)
	saveExtractorEntity(t, store, &entity.Entity{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Hash: "h1"})
	saveExtractorEntity(t, store, &entity.Entity{ID: "lady-evelyn", Name: "Lady Evelyn Vance", Type: "character", Hash: "h2"})

	cases := map[string]string{
		"Garrick the Fence":            "garrick-the-fence",
		"[[Garrick the Fence|Garrick]]": "garrick-the-fence",
		"Evelyn":                        "lady-evelyn",
		"As you declare":                "",
		"":                              "",
	}
	for name, want := range cases {
		if got := ResolveSpeakerID(store, name); got != want {
			t.Errorf("ResolveSpeakerID(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestExtractorReturnsRecordsWithoutPersisting(t *testing.T) {
	store := newTestEntityStore(t)

	model := &mockProvider{
		id: "extractor-model",
		output: `[
			{"id": "garrick-the-fence", "name": "Garrick the Fence", "type": "character", "location": "[[alden-tavern]]", "body": "A shadowy broker."}
		]`,
	}

	records, err := NewExtractor(model).Extract(context.Background(), "You meet Garrick.")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if len(records) != 1 || records[0].Name != "Garrick the Fence" {
		t.Fatalf("unexpected records: %+v", records)
	}
	if _, err := store.GetEntity("garrick-the-fence"); err == nil {
		t.Errorf("expected extraction to leave persistence to the caller")
	}
}
```

Add to `pkg/entity/entity_test.go`:

```go
func TestWikilinkTargets(t *testing.T) {
	got := WikilinkTargets("Garrick nods at [[Garrick the Fence]] and [[Vorpal-Dagger|the dagger]].")
	want := []string{"Garrick the Fence", "Vorpal-Dagger"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WikilinkTargets() = %v, want %v", got, want)
	}
	if len(WikilinkTargets("no links here")) != 0 {
		t.Errorf("expected no targets for plain prose")
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestResolve|TestExtractorReturns|TestWikilinkTargets" -count=1 ./pkg/harness/ ./pkg/entity/`
Expected: FAIL — `undefined: ResolveEntityMentions`, `undefined: NewExtractor`, `undefined: WikilinkTargets`

- [x] **Step 3: Implement**

In `pkg/entity/entity.go`, add next to `WikilinkTarget`:

```go
// WikilinkTargets returns every link target found in text.
func WikilinkTargets(text string) []string {
	matches := wikilinkRegex.FindAllStringSubmatch(text, -1)
	targets := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			targets = append(targets, strings.TrimSpace(m[1]))
		}
	}
	return targets
}
```

In `pkg/harness/extractor.go`:

- Rename `EntityExtractor` to `Extractor` and `NewEntityExtractor(model ModelProvider, store *storage.Store)` to `NewExtractor(model ModelProvider)`; drop the `store` field.
- Replace `ExtractFromTurn(ctx, narrative) (int, error)` with:

```go
// Extract asks the model for the entities a turn introduced or changed. Persisting
// them is the caller's job.
func (e *Extractor) Extract(ctx context.Context, narrative string) ([]ExtractedEntity, error) {
	req := GenerateRequest{
		System: extractorSystemPrompt,
		Prompt: narrative,
	}

	res, err := e.model.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("extractor model failed: %w", err)
	}

	cleaned := strings.TrimSpace(res.Text)
	if idx := strings.Index(cleaned, "["); idx != -1 {
		cleaned = cleaned[idx:]
	}
	if idx := strings.LastIndex(cleaned, "]"); idx != -1 {
		cleaned = cleaned[:idx+1]
	}

	var extracted []ExtractedEntity
	if err := json.Unmarshal([]byte(cleaned), &extracted); err != nil {
		return nil, fmt.Errorf("parse extracted json %q: %w", cleaned, err)
	}
	return extracted, nil
}
```

- Export the merge helper by renaming `mergeExtractedEntity` to `MergeExtractedEntity`.
- Delete `ExtractEntitiesWithProfiles` and its `wikilinkExtractorRegex`, and delete `TestExtractEntitiesWithProfiles_TagMatching` from `pkg/harness/extractor_test.go` (mentions now replace it).
- Update the remaining tests in `pkg/harness/extractor_test.go`: `TestExtractAndSyncEntities` and `TestEntityExtractor_AutoAssignsVoiceProfile` keep their mock providers but assert on the returned `[]ExtractedEntity` (name, type, and voice profile after `AssignVoiceProfile`), not on store contents, since persistence moved to `pkg/engine`.
- Add the new resolution helpers:

```go
// ResolveEntityMentions returns the entities a turn touched, tagged by how. It
// needs no model, so a zero-GPU game still records involvement.
func ResolveEntityMentions(store *storage.Store, playerID, locationID string, texts ...string) []entity.Mention {
	if store == nil {
		return nil
	}

	mentions := make([]entity.Mention, 0)
	seen := make(map[string]bool, 4)

	add := func(id, kind string) {
		if id == "" || seen[id] {
			return
		}
		if ent, err := store.GetEntity(id); err != nil || ent == nil {
			return
		}
		seen[id] = true
		mentions = append(mentions, entity.Mention{ID: id, Kind: kind})
	}

	add(playerID, entity.MentionPlayer)
	add(locationID, entity.MentionLocation)

	for _, text := range texts {
		for _, target := range entity.WikilinkTargets(text) {
			add(ResolveSpeakerID(store, target), entity.MentionWikilink)
		}
	}
	return mentions
}

// ResolveSpeakerID maps a written speaker name to an entity ID using identity
// signals only: exact ID, exact name, then partial name tokens.
func ResolveSpeakerID(store *storage.Store, name string) string {
	cleaned := entity.WikilinkTarget(name)
	if store == nil || cleaned == "" {
		return ""
	}

	slug := entity.Slugify(cleaned)
	if slug == "" {
		return ""
	}

	if ent, err := store.GetEntity(slug); err == nil && ent != nil {
		return ent.ID
	}

	summaries, err := store.ListEntities()
	if err != nil {
		return ""
	}

	for _, summary := range summaries {
		if entity.Slugify(summary.Name) == slug {
			return summary.ID
		}
	}
	for _, summary := range summaries {
		if sharesNameTokens(slug, summary.ID) || sharesNameTokens(slug, entity.Slugify(summary.Name)) {
			return summary.ID
		}
	}
	return ""
}
```

`EntityExtractor` is renamed, so update `pkg/harness/extractor_test.go` call sites and check nothing else references the old names: `grep -rn "EntityExtractor\|ExtractFromTurn\|ExtractEntitiesWithProfiles" --include=*.go pkg cmd` must return nothing outside this doc.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/harness/ ./pkg/entity/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/entity/entity.go pkg/entity/entity_test.go pkg/harness/extractor.go pkg/harness/extractor_test.go
git commit -m "refactor(harness): separate extraction from persistence and resolve mentions"
```

### Task 14: `Timeline.RecordTurn` writes notes, then the log, then the index

**Files:**
- Modify: `pkg/engine/timeline.go`
- Test: `pkg/engine/timeline_test.go`

**Interfaces:**
- Consumes: `harness.MatchExistingEntity`, `harness.MergeExtractedEntity`, `harness.AssignVoiceProfile`, `storage.Syncer.Sync`
- Produces: `(*Timeline).SetVoiceProfiles([]config.VoiceProfile)`, `(*Timeline).RecordTurn(turn *Turn, extracted []harness.ExtractedEntity) error`

- [x] **Step 1: Write the failing test**

Append to `pkg/engine/timeline_test.go`:

```go
func writeTestEntityNote(t *testing.T, dir string, ent *entity.Entity) {
	t.Helper()

	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ent.ID+".md"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestTimelineRecordTurnWritesNotesThenIndexes(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	store := newTestStore(t)
	history := NewHistoryLogger(filepath.Join(t.TempDir(), "history.jsonl"))
	timeline := NewTimeline(paths, store, history, "campaign-01")
	timeline.SetVoiceProfiles([]config.VoiceProfile{
		{ID: "elder_sage", VoiceID: "bm_george", Pitch: 0.85, SpeechRate: 0.9, Tags: []string{"elder", "veteran"}},
	})

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "hero", Name: "Sean", Type: "character", Body: "A traveller."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "aldon-tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}

	narration := "An elder veteran introduces himself."
	turn := Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I ask around",
		Narration: narration,
		Entities:  harness.ResolveEntityMentions(store, "hero", "aldon-tavern", narration),
	}
	extracted := []harness.ExtractedEntity{
		{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Location: "[[aldon-tavern]]", Body: "An elder veteran broker."},
	}

	if err := timeline.RecordTurn(&turn, extracted); err != nil {
		t.Fatalf("RecordTurn failed: %v", err)
	}

	garrickPath := filepath.Join(entitiesDir, "garrick-the-fence.md")
	data, err := os.ReadFile(garrickPath)
	if err != nil {
		t.Fatalf("expected a note for the extracted entity: %v", err)
	}
	garrick, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("parse extracted note: %v", err)
	}
	if !reflect.DeepEqual(garrick.History, []int{1}) {
		t.Errorf("History = %v, want [1]", garrick.History)
	}
	if garrick.Voice == nil || garrick.Voice.VoiceID != "bm_george" {
		t.Errorf("expected the elder voice profile to be assigned, got %+v", garrick.Voice)
	}

	refs, err := store.ListEntitiesForTurn(1)
	if err != nil {
		t.Fatalf("ListEntitiesForTurn failed: %v", err)
	}
	if len(refs) != 3 {
		t.Errorf("expected player, location, and extracted entity links, got %+v", refs)
	}

	turns, err := history.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(turns) != 1 || turns[0].Prose() != narration {
		t.Fatalf("expected the turn in history.jsonl, got %+v", turns)
	}
	if len(turns[0].Entities) != 3 {
		t.Errorf("expected involvement in the record, got %+v", turns[0].Entities)
	}

	// A later turn about the same entity accumulates history instead of duplicating.
	second := Turn{
		Number:    2,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I press him",
		Narration: "Garrick the Fence says nothing.",
		Entities:  harness.ResolveEntityMentions(store, "hero", "aldon-tavern", "Garrick the Fence says nothing."),
	}
	if err := timeline.RecordTurn(&second, []harness.ExtractedEntity{
		{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Body: "A broker who keeps his mouth shut."},
	}); err != nil {
		t.Fatalf("second RecordTurn failed: %v", err)
	}

	data, err = os.ReadFile(garrickPath)
	if err != nil {
		t.Fatal(err)
	}
	garrick, err = entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(garrick.History, []int{1, 2}) {
		t.Errorf("History = %v, want [1 2]", garrick.History)
	}
	if !strings.Contains(garrick.Body, "elder veteran broker") || !strings.Contains(garrick.Body, "keeps his mouth shut") {
		t.Errorf("expected accumulated body detail, got %q", garrick.Body)
	}
	if garrick.Location != "[[aldon-tavern]]" {
		t.Errorf("expected the authored location to survive merging, got %q", garrick.Location)
	}

	indexed, err := store.ListTurnsForEntity("garrick-the-fence")
	if err != nil {
		t.Fatalf("ListTurnsForEntity failed: %v", err)
	}
	if !reflect.DeepEqual(indexed, []int{1, 2}) {
		t.Errorf("indexed turns = %v, want [1 2]", indexed)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestTimelineRecordTurn -count=1 ./pkg/engine/`
Expected: FAIL — `timeline.RecordTurn undefined`

- [x] **Step 3: Implement**

Add to `pkg/engine/timeline.go` (imports gain `os`, `sort`, `strings`, `github.com/darkliquid/localrpg/pkg/config`, `github.com/darkliquid/localrpg/pkg/harness`):

```go
// SetVoiceProfiles provides the archetypes assigned to newly discovered characters.
func (t *Timeline) SetVoiceProfiles(profiles []config.VoiceProfile) {
	t.voiceProfiles = profiles
}

// RecordTurn persists everything a turn changed: the affected entity notes, the
// turn record, and the index row. Notes are written before the index is synced,
// and a failed note write aborts before the turn is appended, so the record never
// points at a note that does not exist.
func (t *Timeline) RecordTurn(turn *Turn, extracted []harness.ExtractedEntity) error {
	pending, err := t.stageEntities(turn, extracted)
	if err != nil {
		return err
	}

	if len(pending) > 0 {
		if err := t.writeEntities(pending); err != nil {
			return err
		}
	}

	if err := t.history.AppendTurn(*turn); err != nil {
		return fmt.Errorf("append turn: %w", err)
	}

	return t.indexTurn(*turn)
}

func (t *Timeline) stageEntities(turn *Turn, extracted []harness.ExtractedEntity) (map[string]*entity.Entity, error) {
	pending := make(map[string]*entity.Entity, len(turn.Entities)+len(extracted))

	for _, mention := range turn.Entities {
		existing, err := t.store.GetEntity(mention.ID)
		if err != nil || existing == nil {
			continue
		}
		pending[existing.ID] = existing
	}

	for _, raw := range extracted {
		if strings.TrimSpace(raw.Name) == "" {
			continue
		}

		ent := harness.MatchExistingEntity(t.store, &raw)
		if ent != nil {
			ent = harness.MergeExtractedEntity(ent, &raw)
		} else {
			id := entity.Slugify(raw.Name)
			if id == "" {
				continue
			}
			if existing, err := t.store.GetEntity(id); err == nil && existing != nil {
				ent = harness.MergeExtractedEntity(existing, &raw)
			} else {
				ent = &entity.Entity{
					ID:        id,
					Name:      raw.Name,
					Type:      raw.Type,
					Location:  raw.Location,
					Faction:   raw.Faction,
					Body:      raw.Body,
					Wikilinks: make([]string, 0),
				}
			}
		}

		if ent.Type == "character" {
			harness.AssignVoiceProfile(ent, t.voiceProfiles)
		}

		pending[ent.ID] = ent
		if !containsMention(turn.Entities, ent.ID) {
			turn.Entities = append(turn.Entities, entity.Mention{ID: ent.ID, Kind: entity.MentionExtracted})
		}
	}

	return pending, nil
}

func (t *Timeline) writeEntities(pending map[string]*entity.Entity) error {
	dir := t.EntitiesDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create entities dir: %w", err)
	}

	ids := make([]string, 0, len(pending))
	for id := range pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		ent := pending[id]
		data, err := ent.SerializeMarkdown()
		if err != nil {
			return fmt.Errorf("serialize entity %q: %w", id, err)
		}
		if err := os.WriteFile(filepath.Join(dir, id+".md"), data, 0644); err != nil {
			return fmt.Errorf("write entity %q: %w", id, err)
		}
	}

	if _, err := storage.NewSyncer(t.store).Sync(dir); err != nil {
		return fmt.Errorf("sync entities: %w", err)
	}
	return nil
}

func containsMention(mentions []entity.Mention, id string) bool {
	for _, mention := range mentions {
		if mention.ID == id {
			return true
		}
	}
	return false
}
```

`writeEntities` only writes files and syncs the index; history stamping happens in `RecordTurn` so prune paths can reuse it unchanged. In `RecordTurn`, replace the write call with:

```go
	if len(pending) > 0 {
		for id, ent := range pending {
			ent.History = appendTurnNumber(ent.History, turn.Number)
			pending[id] = ent
		}
		if err := t.writeEntities(pending); err != nil {
			return err
		}
	}
```

and add:

```go
func appendTurnNumber(history []int, turnNumber int) []int {
	for _, number := range history {
		if number == turnNumber {
			return history
		}
	}
	return append(history, turnNumber)
}
```

Add the `voiceProfiles []config.VoiceProfile` field to the `Timeline` struct.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine/timeline.go pkg/engine/timeline_test.go
git commit -m "feat(engine): extract entities into notes and link them to each turn"
```

### Task 15: Wire extraction and the timeline into the turn loop

**Files:**
- Modify: `pkg/engine/orchestrator.go` (constructor, `ProcessAction`, drop the unused `SetVoiceProfiles`)
- Modify: `pkg/engine/game.go` (normalise template note names when instantiating a game)
- Modify: `pkg/engine/game_test.go`, `pkg/engine/orchestrator_test.go`, `pkg/tui/app_test.go` (constructor signature)
- Modify: `pkg/config/types.go` (role name constants)
- Modify: `cmd/localrpg/play.go` (build the timeline, the extractor, and the orchestrator)
- Test: `pkg/engine/orchestrator_test.go`, `pkg/engine/game_test.go`

**Interfaces:**
- Consumes: `Timeline.RecordTurn`, `harness.ResolveEntityMentions`, `harness.NewExtractor`
- Produces: `engine.NewTurnOrchestrator(store *storage.Store, timeline *Timeline, rulesEngine *rules.JSEngine, router *harness.Router, locationID, playerID string) *TurnOrchestrator`, `(*TurnOrchestrator).SetExtractor(*harness.Extractor)`, `config.RoleGM`, `config.RoleNarrator`, `config.RoleExtractor`

- [x] **Step 1: Write the failing tests**

Append to `pkg/engine/orchestrator_test.go`:

```go
type mockTimelineModel struct {
	response string
}

func (m *mockTimelineModel) ID() string { return "mock-timeline" }

func (m *mockTimelineModel) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: m.response}, nil
}

func (m *mockTimelineModel) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	out <- harness.StreamChunk{Text: m.response, Done: true}
	return nil
}

func TestProcessActionRecordsEntitiesAndTimeline(t *testing.T) {
	tempDir := t.TempDir()
	paths := core.NewPathResolver(tempDir)
	store := newTestStore(t)
	timeline := NewTimeline(paths, store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(&mockOrchestratorModel{response: "Garrick the Fence glances up."})
	router.AssignRole("gm", "mock-gm")

	extractorModel := &mockTimelineModel{response: `[{"id":"garrick-the-fence","name":"Garrick the Fence","type":"character","body":"A shadowy broker."}]`}

	orchestrator := NewTurnOrchestrator(store, timeline, nil, router, "tavern", "player")
	orchestrator.SetExtractor(harness.NewExtractor(extractorModel))

	turn, err := orchestrator.ProcessAction(context.Background(), "Do", "I look for Garrick")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(entitiesDir, "garrick-the-fence.md")); err != nil {
		t.Fatalf("expected the extracted entity note on disk: %v", err)
	}

	indexed, err := store.ListTurnsForEntity("garrick-the-fence")
	if err != nil {
		t.Fatalf("ListTurnsForEntity failed: %v", err)
	}
	if len(indexed) != 1 || indexed[0] != turn.Number {
		t.Errorf("indexed turns = %v, want [%d]", indexed, turn.Number)
	}

	if len(turn.Entities) != 3 {
		t.Errorf("expected player, location, and extracted involvement, got %+v", turn.Entities)
	}
}

func TestProcessActionSurvivesExtractorFailure(t *testing.T) {
	tempDir := t.TempDir()
	paths := core.NewPathResolver(tempDir)
	store := newTestStore(t)
	timeline := NewTimeline(paths, store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(&mockOrchestratorModel{response: "Nothing stirs."})
	router.AssignRole("gm", "mock-gm")

	orchestrator := NewTurnOrchestrator(store, timeline, nil, router, "tavern", "player")
	orchestrator.SetExtractor(harness.NewExtractor(&mockTimelineModel{response: "not json at all"}))

	turn, err := orchestrator.ProcessAction(context.Background(), "Do", "I listen")
	if err != nil {
		t.Fatalf("a failed extractor must not fail the turn: %v", err)
	}
	if turn.Number != 1 {
		t.Errorf("expected the turn to be recorded, got %+v", turn)
	}
	if len(turn.Entities) != 2 {
		t.Errorf("expected player and location involvement only, got %+v", turn.Entities)
	}
}
```

Add to `TestGameInitAndLoad` in `pkg/engine/game_test.go`:

```go
	if _, err := os.Stat(filepath.Join(paths.GameDir("campaign-01"), "entities", "tavern.md")); err != nil {
		t.Errorf("expected the world template to be copied as <id>.md: %v", err)
	}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestProcessActionRecordsEntitiesAndTimeline|TestProcessActionSurvivesExtractorFailure|TestGameInitAndLoad" -count=1 ./pkg/engine/`
Expected: FAIL — `too many arguments in call to NewTurnOrchestrator`, `orchestrator.SetExtractor undefined`, and the `tavern.md` assertion.

- [x] **Step 3: Implement**

In `pkg/config/types.go`, add:

```go
// Agent role names routed by the harness router.
const (
	RoleGM        = "gm"
	RoleNarrator  = "narrator"
	RoleExtractor = "extractor"
)
```

In `pkg/engine/orchestrator.go`:

- Replace the `history *HistoryLogger` field with `timeline *Timeline` and add `extractor *harness.Extractor`.
- New constructor:

```go
func NewTurnOrchestrator(
	store *storage.Store,
	timeline *Timeline,
	rulesEngine *rules.JSEngine,
	router *harness.Router,
	locationID string,
	playerID string,
) *TurnOrchestrator {
	return &TurnOrchestrator{
		store:       store,
		timeline:    timeline,
		rulesEngine: rulesEngine,
		router:      router,
		locationID:  locationID,
		playerID:    playerID,
		assembler:   harness.NewContextAssembler(store),
	}
}

// SetExtractor enables per-turn entity extraction. A nil extractor records
// deterministic mentions only.
func (o *TurnOrchestrator) SetExtractor(extractor *harness.Extractor) {
	o.extractor = extractor
}
```

- Delete `SetVoiceProfiles` and the `voiceProfiles` field (voice archetypes are now applied by `Timeline.RecordTurn`).
- Replace `o.history.LoadHistory()` with `o.timeline.history.LoadHistory()` and keep `/undo` on `o.timeline.history.RewindToTurn(...)` for now (Task 16 routes it through the timeline).
- Replace the append at the end of `ProcessAction` with:

```go
	turn.Entities = harness.ResolveEntityMentions(o.store, o.playerID, o.locationID, resp.Text, actionInput)

	var extracted []harness.ExtractedEntity
	if o.extractor != nil {
		// A failed extractor must not lose the turn; mentions above still stand.
		if records, err := o.extractor.Extract(ctx, resp.Text); err == nil {
			extracted = records
		}
	}

	if err := o.timeline.RecordTurn(&turn, extracted); err != nil {
		return nil, fmt.Errorf("record turn: %w", err)
	}
```

In `pkg/engine/game.go`, replace the world-template copy loop with one that renames notes to `<id>.md`:

```go
	// Copy initial template entities from world into game, named <id>.md
	worldEntitiesDir := filepath.Join(paths.WorldDir(worldID), "entities")
	if entries, err := os.ReadDir(worldEntitiesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(worldEntitiesDir, e.Name()))
			if err != nil {
				return nil, fmt.Errorf("read entity template %q: %w", e.Name(), err)
			}
			template, err := entity.ParseMarkdownEntity(data)
			if err != nil {
				return nil, fmt.Errorf("parse entity template %q: %w", e.Name(), err)
			}
			if err := os.WriteFile(filepath.Join(gameEntitiesDir, template.ID+".md"), data, 0644); err != nil {
				return nil, fmt.Errorf("write entity template %q: %w", e.Name(), err)
			}
		}
	}
```

`copyFile` becomes unused once this lands; delete it. Add the `entity` import to `pkg/engine/game.go`.

In `cmd/localrpg/play.go`:

```go
	timeline := engine.NewTimeline(paths, store, history, gameID)
	timeline.SetVoiceProfiles(cfg.Media.TTS.VoiceProfiles)
	if err := timeline.EnsureIndexed(); err != nil {
		fmt.Fprintf(os.Stderr, "Error indexing turns: %v\n", err)
		os.Exit(1)
	}

	orchestrator := engine.NewTurnOrchestrator(store, timeline, jsEngine, router, startLocation, manifest.Player)
	orchestrator.SetExtractor(resolveExtractor(cfg, router))
	orchestrator.LoadPrompts(paths, manifest.SystemID, manifest.WorldID)
```

with the helper in the same file:

```go
// resolveExtractor picks the provider used for per-turn entity extraction: an
// explicitly configured extractor role, otherwise the gm provider, and nil when
// the role is disabled.
func resolveExtractor(cfg *config.Config, router *harness.Router) *harness.Extractor {
	roleCfg, configured := cfg.Agents.Roles[config.RoleExtractor]
	if !configured {
		provider, err := router.GetProviderForRole(config.RoleGM)
		if err != nil {
			return nil
		}
		return harness.NewExtractor(provider)
	}
	if roleCfg.Type == "disabled" {
		return nil
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

Update the remaining constructor call sites: `pkg/tui/app_test.go:42` and the existing tests in `pkg/engine/orchestrator_test.go` now build a `Timeline` first, e.g.

```go
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, history, "test-campaign")
	orchestrator := NewTurnOrchestrator(store, timeline, jsEngine, router, "tavern", "player")
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./... && go vet ./...`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/engine/orchestrator.go pkg/engine/game.go pkg/engine/game_test.go pkg/engine/orchestrator_test.go pkg/tui/app_test.go cmd/localrpg/play.go
git commit -m "feat(engine): extract and link entities on every turn"
```

## Phase 5: Undo Across All Three Layers

### Task 16: Rewind prunes the log, the index, and entity links

**Files:**
- Modify: `pkg/engine/timeline.go`
- Modify: `pkg/engine/orchestrator.go` (`/undo` branch)
- Test: `pkg/engine/timeline_test.go`, `pkg/engine/orchestrator_test.go`

**Interfaces:**
- Consumes: `storage.DeleteTurnsFrom`, `HistoryLogger.RewindToTurn`, `writeEntities`
- Produces: `(*Timeline).RewindToTurn(target int) error`

- [x] **Step 1: Write the failing test**

Append to `pkg/engine/timeline_test.go`:

```go
func TestTimelineRewindPrunesLinksButKeepsNotes(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	store := newTestStore(t)
	history := NewHistoryLogger(filepath.Join(t.TempDir(), "history.jsonl"))
	timeline := NewTimeline(paths, store, history, "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "hero", Name: "Sean", Type: "character", Body: "A traveller."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	first := Turn{Number: 1, Timestamp: time.Now(), Mode: "Do", Input: "I ask", Narration: "Garrick the Fence frowns."}
	first.Entities = harness.ResolveEntityMentions(store, "hero", "", first.Narration)
	if err := timeline.RecordTurn(&first, []harness.ExtractedEntity{
		{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Body: "A broker."},
	}); err != nil {
		t.Fatal(err)
	}

	second := Turn{Number: 2, Timestamp: time.Now(), Mode: "Do", Input: "I push", Narration: "Garrick the Fence relents."}
	second.Entities = harness.ResolveEntityMentions(store, "hero", "", second.Narration)
	if err := timeline.RecordTurn(&second, []harness.ExtractedEntity{
		{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Body: "He admits the debt is real."},
	}); err != nil {
		t.Fatal(err)
	}

	if err := timeline.RewindToTurn(1); err != nil {
		t.Fatalf("RewindToTurn failed: %v", err)
	}

	turns, err := history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Number != 1 {
		t.Errorf("expected only turn 1 in the log, got %+v", turns)
	}

	count, err := store.CountTurns()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("CountTurns = %d, want 1", count)
	}

	indexed, err := store.ListTurnsForEntity("garrick-the-fence")
	if err != nil {
		t.Fatal(err)
	}
	if len(indexed) != 1 || indexed[0] != 1 {
		t.Errorf("indexed turns = %v, want [1]", indexed)
	}

	data, err := os.ReadFile(filepath.Join(entitiesDir, "garrick-the-fence.md"))
	if err != nil {
		t.Fatal(err)
	}
	garrick, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(garrick.History, []int{1}) {
		t.Errorf("History = %v, want [1]", garrick.History)
	}
	if !strings.Contains(garrick.Body, "admits the debt is real") {
		t.Errorf("expected the world's memory to survive an undo, got %q", garrick.Body)
	}
}
```

Append to `pkg/engine/orchestrator_test.go`:

```go
func TestProcessActionUndoRewindsIndex(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "test-campaign")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(&mockOrchestratorModel{response: "You step inside."})
	router.AssignRole("gm", "mock-gm")

	orchestrator := NewTurnOrchestrator(store, timeline, nil, router, "tavern", "player")

	if _, err := orchestrator.ProcessAction(context.Background(), "Do", "I open the door"); err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.ProcessAction(context.Background(), "Do", "I sit down"); err != nil {
		t.Fatal(err)
	}

	undo, err := orchestrator.ProcessAction(context.Background(), "System", "/undo")
	if err != nil {
		t.Fatalf("undo failed: %v", err)
	}
	if undo.Mode != "System" {
		t.Errorf("expected a system turn for the undo, got %+v", undo)
	}

	count, err := store.CountTurns()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("CountTurns = %d, want 1 after undo", count)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestTimelineRewind|TestProcessActionUndoRewindsIndex" -count=1 ./pkg/engine/`
Expected: FAIL — `timeline.RewindToTurn undefined`; the index still holds the discarded turn.

- [x] **Step 3: Implement**

Append to `pkg/engine/timeline.go`:

```go
// RewindToTurn discards every turn after target from the log, the index, and each
// entity's history list. Entity prose and state are deliberately left alone:
// undo trims the record of what happened, not the world's memory of it.
func (t *Timeline) RewindToTurn(target int) error {
	if target < 0 {
		target = 0
	}

	turns, err := t.history.LoadHistory()
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}

	affected := make(map[string]bool)
	for _, turn := range turns {
		if turn.Number <= target {
			continue
		}
		for _, mention := range turn.Entities {
			affected[mention.ID] = true
		}
	}

	if len(affected) > 0 {
		if err := t.pruneEntityHistory(affected, target); err != nil {
			return err
		}
	}

	if err := t.history.RewindToTurn(target); err != nil {
		return fmt.Errorf("rewind history: %w", err)
	}
	if err := t.store.DeleteTurnsFrom(target + 1); err != nil {
		return fmt.Errorf("delete indexed turns: %w", err)
	}
	return nil
}

func (t *Timeline) pruneEntityHistory(affected map[string]bool, target int) error {
	pending := make(map[string]*entity.Entity, len(affected))

	for id := range affected {
		ent, err := t.store.GetEntity(id)
		if err != nil || ent == nil {
			continue
		}

		pruned := make([]int, 0, len(ent.History))
		for _, number := range ent.History {
			if number <= target {
				pruned = append(pruned, number)
			}
		}
		if len(pruned) == len(ent.History) {
			continue
		}
		ent.History = pruned
		pending[id] = ent
	}

	if len(pending) == 0 {
		return nil
	}
	return t.writeEntities(pending)
}
```

In `pkg/engine/orchestrator.go`, replace `o.timeline.history.RewindToTurn(len(pastTurns) - 1)` with `o.timeline.RewindToTurn(len(pastTurns) - 1)`.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine/timeline.go pkg/engine/timeline_test.go pkg/engine/orchestrator.go pkg/engine/orchestrator_test.go
git commit -m "feat(engine): make undo rewind the log, the index, and entity links"
```

---

## Phase 6: Dialogue Attribution

### Task 17: One parser for attributed speech

**Files:**
- Create: `pkg/dialogue/dialogue.go`
- Test: `pkg/dialogue/dialogue_test.go`

**Interfaces:**
- Consumes: `entity.WikilinkTarget`
- Produces: `dialogue.Segment{Speaker, SpeakerID, Text string; IsSpeech bool}`, `dialogue.Parse(text string, resolve func(candidate string) (string, bool)) []Segment` (the callback returns the resolved entity ID)

- [x] **Step 1: Write the failing test**

`pkg/dialogue/dialogue_test.go`:

```go
package dialogue

import (
	"reflect"
	"testing"
)

func TestParseAttributesResolvedSpeakers(t *testing.T) {
	known := map[string]string{
		"Garrick the Fence": "garrick-the-fence",
		"Garrick":           "garrick-the-fence",
		"Lady Evelyn Vance": "lady-evelyn",
	}
	resolve := func(candidate string) (string, bool) {
		id, ok := known[candidate]
		return id, ok
	}

	text := "The docks are quiet.\nGarrick the Fence: \"You didn't see me here.\"\nAs you declare: \"I draw my blade.\"\n[[Lady Evelyn Vance|Evelyn]]: \"Later.\""
	segments := Parse(text, resolve)

	want := []Segment{
		{Text: "The docks are quiet."},
		{Speaker: "Garrick the Fence", SpeakerID: "garrick-the-fence", Text: "You didn't see me here.", IsSpeech: true},
		{Text: `As you declare: "I draw my blade."`},
		{Speaker: "Lady Evelyn Vance", SpeakerID: "lady-evelyn", Text: "Later.", IsSpeech: true},
	}
	if !reflect.DeepEqual(segments, want) {
		t.Fatalf("Parse() = %#v, want %#v", segments, want)
	}
}

func TestParseKeepsUnresolvedAndUnattributedProse(t *testing.T) {
	segments := Parse("Someone whispers: \"not me\".\n\nA plain line.", func(string) (string, bool) { return "", false })

	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %#v", segments)
	}
	for _, segment := range segments {
		if segment.IsSpeech {
			t.Errorf("expected no speech segments, got %#v", segment)
		}
	}
}

func TestParseEmptyText(t *testing.T) {
	if segments := Parse("   \n\n", func(string) (string, bool) { return "", false }); len(segments) != 0 {
		t.Errorf("expected no segments for blank text, got %#v", segments)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -count=1 ./pkg/dialogue/`
Expected: FAIL — package does not exist

- [x] **Step 3: Implement**

Create `pkg/dialogue/dialogue.go`:

```go
// Package dialogue splits narration into ordered narration and speech spans,
// attributing speech to speakers the caller can resolve.
package dialogue

import (
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// Segment is one ordered span of a turn.
type Segment struct {
	Speaker   string
	SpeakerID string
	Text      string
	IsSpeech  bool
}

var attributedSpeakerRegex = regexp.MustCompile(`^([^:\n]+):\s*["“]([^"”]+)["”]`)

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

		if match := attributedSpeakerRegex.FindStringSubmatch(line); len(match) == 3 {
			candidate := entity.WikilinkTarget(strings.TrimSpace(match[1]))
			if id, ok := resolve(candidate); ok {
				segments = append(segments, Segment{
					Speaker:   candidate,
					SpeakerID: id,
					Text:      strings.TrimSpace(match[2]),
					IsSpeech:  true,
				})
				continue
			}
		}

		segments = append(segments, Segment{Text: line})
	}

	return segments
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/dialogue/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/dialogue/dialogue.go pkg/dialogue/dialogue_test.go
git commit -m "feat(dialogue): parse attributed speech behind an entity resolution gate"
```

### Task 18: Record ordered narration and speech segments

**Files:**
- Create: `pkg/entity/segment.go`, `pkg/engine/segments.go`
- Modify: `pkg/entity/mention.go` (add `MentionSpeech`)
- Modify: `pkg/engine/history.go` (`Turn.Segments`)
- Modify: `pkg/engine/orchestrator.go` (build segments, add speakers to involvement)
- Modify: `pkg/harness/extractor.go` (`Extraction`, `ExtractedDialogue`, object-or-array parsing)
- Modify: `pkg/harness/extractor_test.go`, `pkg/engine/orchestrator_test.go` (updated call sites)
- Test: `pkg/engine/segments_test.go`, `pkg/harness/extractor_test.go`

**Interfaces:**
- Consumes: `dialogue.Parse`, `harness.ResolveSpeakerID`
- Produces: `entity.TurnSegment{Kind, Speaker, SpeakerID, Text}`, `entity.SegmentNarration`, `entity.SegmentSpeech`, `entity.MentionSpeech`, `harness.Extraction{Entities, Dialogue}`, `harness.ExtractedDialogue{Speaker, Text}`, `(*Extractor).Extract(ctx, narrative) (*Extraction, error)`, `engine.Turn.Segments []entity.TurnSegment`

- [x] **Step 1: Write the failing tests**

`pkg/engine/segments_test.go`:

```go
package engine

import (
	"reflect"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestBuildTurnSegmentsAttributesSayModePlayerAndResolvedSpeakers(t *testing.T) {
	store := newTestStore(t)
	saveTestEntity(t, store, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Hash: "h1"})
	saveTestEntity(t, store, &entity.Entity{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Hash: "h2"})

	narration := "The docks are quiet.\nGarrick the Fence: \"You didn't see me here.\"\nAs you declare: \"I draw my blade.\""
	segments := buildTurnSegments(store, "Say", "player", "Where is the ledger?", narration, nil)

	want := []entity.TurnSegment{
		{Kind: entity.SegmentSpeech, Speaker: "Sean", SpeakerID: "player", Text: "Where is the ledger?"},
		{Kind: entity.SegmentNarration, Text: "The docks are quiet."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick the Fence", SpeakerID: "garrick-the-fence", Text: "You didn't see me here."},
		{Kind: entity.SegmentNarration, Text: `As you declare: "I draw my blade."`},
	}
	if !reflect.DeepEqual(segments, want) {
		t.Fatalf("buildTurnSegments() = %#v, want %#v", segments, want)
	}
}

func TestBuildTurnSegmentsAppliesExtractorAttribution(t *testing.T) {
	store := newTestStore(t)
	saveTestEntity(t, store, &entity.Entity{ID: "lady-evelyn", Name: "Lady Evelyn Vance", Type: "character", Hash: "h1"})

	narration := `Vance looks up. She says "You made it back in one piece."`
	attributions := []harness.ExtractedDialogue{
		{Speaker: "Lady Evelyn Vance", Text: "You made it back in one piece."},
	}

	segments := buildTurnSegments(store, "Do", "player", "I enter", narration, attributions)

	want := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "Vance looks up. She says"},
		{Kind: entity.SegmentSpeech, Speaker: "Lady Evelyn Vance", SpeakerID: "lady-evelyn", Text: "You made it back in one piece."},
	}
	if !reflect.DeepEqual(segments, want) {
		t.Fatalf("buildTurnSegments() = %#v, want %#v", segments, want)
	}
}

func TestSpeechMentionsDeduplicatesSpeakers(t *testing.T) {
	segments := []entity.TurnSegment{
		{Kind: entity.SegmentSpeech, SpeakerID: "garrick"},
		{Kind: entity.SegmentNarration, Text: "silence"},
		{Kind: entity.SegmentSpeech, SpeakerID: "garrick"},
		{Kind: entity.SegmentSpeech, SpeakerID: ""},
	}

	mentions := speechMentions(segments)
	if len(mentions) != 1 || mentions[0].ID != "garrick" || mentions[0].Kind != entity.MentionSpeech {
		t.Errorf("speechMentions() = %+v, want one garrick speech mention", mentions)
	}
}
```

Append to `pkg/harness/extractor_test.go`:

```go
func TestExtractorParsesObjectAndArrayResponses(t *testing.T) {
	cases := []struct {
		name     string
		output   string
		wantEnts int
		wantDial int
	}{
		{
			name:     "object",
			output:   `{"entities":[{"id":"garrick","name":"Garrick","type":"character","body":"A broker."}],"dialogue":[{"speaker":"Garrick","text":"Keep walking."}]}`,
			wantEnts: 1,
			wantDial: 1,
		},
		{
			name:     "bare array from older prompts",
			output:   `[{"id":"garrick","name":"Garrick","type":"character","body":"A broker."}]`,
			wantEnts: 1,
			wantDial: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewExtractor(&mockProvider{id: "extractor-model", output: tc.output}).Extract(context.Background(), "narrative")
			if err != nil {
				t.Fatalf("Extract failed: %v", err)
			}
			if len(result.Entities) != tc.wantEnts || len(result.Dialogue) != tc.wantDial {
				t.Errorf("got %d entities and %d dialogue lines, want %d and %d",
					len(result.Entities), len(result.Dialogue), tc.wantEnts, tc.wantDial)
			}
		})
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestBuildTurnSegments|TestSpeechMentions|TestExtractorParses" -count=1 ./pkg/engine/ ./pkg/harness/`
Expected: FAIL — `undefined: buildTurnSegments`, `undefined: entity.SegmentSpeech`, `result.Entities undefined`

- [x] **Step 3: Implement**

Create `pkg/entity/segment.go`:

```go
package entity

// Segment kinds for a turn's ordered playback script.
const (
	SegmentNarration = "narration"
	SegmentSpeech    = "speech"
)

// TurnSegment is one spoken or narrated span of a turn, in playback order.
type TurnSegment struct {
	Kind      string `json:"kind"`
	Speaker   string `json:"speaker,omitempty"`
	SpeakerID string `json:"speaker_id,omitempty"`
	Text      string `json:"text"`
}
```

Add to `pkg/entity/mention.go`:

```go
// MentionSpeech marks a mention that came from an attributed spoken line.
const MentionSpeech = "speech"
```

Add to the `Turn` struct in `pkg/engine/history.go`:

```go
	Segments []entity.TurnSegment `json:"segments,omitempty"`
```

Create `pkg/engine/segments.go`:

```go
package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/dialogue"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// buildTurnSegments resolves the ordered playback script for a turn: the player's
// own utterance in Say mode, then the narration split into narration and speech,
// with extractor attributions applied where the prose parse found none.
func buildTurnSegments(store *storage.Store, mode, playerID, input, narration string, attributions []harness.ExtractedDialogue) []entity.TurnSegment {
	segments := make([]entity.TurnSegment, 0)

	if strings.EqualFold(mode, "Say") && strings.TrimSpace(input) != "" {
		segments = append(segments, entity.TurnSegment{
			Kind:      entity.SegmentSpeech,
			Speaker:   entityName(store, playerID),
			SpeakerID: playerID,
			Text:      strings.TrimSpace(input),
		})
	}

	resolve := func(candidate string) (string, bool) {
		id := harness.ResolveSpeakerID(store, candidate)
		return id, id != ""
	}

	for _, segment := range dialogue.Parse(narration, resolve) {
		if !segment.IsSpeech {
			segments = append(segments, entity.TurnSegment{Kind: entity.SegmentNarration, Text: segment.Text})
			continue
		}
		segments = append(segments, entity.TurnSegment{
			Kind:      entity.SegmentSpeech,
			Speaker:   segment.Speaker,
			SpeakerID: segment.SpeakerID,
			Text:      segment.Text,
		})
	}

	return mergeAttributions(segments, attributions, resolve)
}

// mergeAttributions splits narration spans so model-attributed speech is recorded
// even when the prose did not follow the `Name: "…"` convention.
func mergeAttributions(segments []entity.TurnSegment, attributions []harness.ExtractedDialogue, resolve func(string) (string, bool)) []entity.TurnSegment {
	for _, attribution := range attributions {
		text := strings.TrimSpace(attribution.Text)
		if text == "" || attribution.Speaker == "" {
			continue
		}

		speakerID, ok := resolve(attribution.Speaker)
		if !ok {
			continue
		}

		merged := false
		for i, segment := range segments {
			if segment.Kind != entity.SegmentNarration || !strings.Contains(segment.Text, text) {
				continue
			}

			idx := strings.Index(segment.Text, text)
			before := strings.TrimSpace(segment.Text[:idx])
			after := strings.TrimSpace(segment.Text[idx+len(text):])

			replacement := make([]entity.TurnSegment, 0, 3)
			if before != "" {
				replacement = append(replacement, entity.TurnSegment{Kind: entity.SegmentNarration, Text: before})
			}
			replacement = append(replacement, entity.TurnSegment{
				Kind:      entity.SegmentSpeech,
				Speaker:   attribution.Speaker,
				SpeakerID: speakerID,
				Text:      text,
			})
			if after != "" {
				replacement = append(replacement, entity.TurnSegment{Kind: entity.SegmentNarration, Text: after})
			}

			segments = append(segments[:i:i], append(replacement, segments[i+1:]...)...)
			merged = true
			break
		}

		if !merged {
			continue
		}
	}
	return segments
}

// speechMentions returns the speakers of a segment list, deduplicated.
func speechMentions(segments []entity.TurnSegment) []entity.Mention {
	mentions := make([]entity.Mention, 0)
	seen := make(map[string]bool)
	for _, segment := range segments {
		if segment.SpeakerID == "" || seen[segment.SpeakerID] {
			continue
		}
		seen[segment.SpeakerID] = true
		mentions = append(mentions, entity.Mention{ID: segment.SpeakerID, Kind: entity.MentionSpeech})
	}
	return mentions
}

func entityName(store *storage.Store, id string) string {
	if id == "" || store == nil {
		return ""
	}
	if ent, err := store.GetEntity(id); err == nil && ent != nil {
		return ent.Name
	}
	return id
}
```

The `strings.TrimSpace` body of a quoted line may already be trimmed by the parser, so `mergeAttributions` only fires for prose spans; that is the intended path.

In `pkg/harness/extractor.go`, replace `Extract`'s return type and add the result types:

```go
// Extraction is everything one extraction pass returned for a turn.
type Extraction struct {
	Entities []ExtractedEntity   `json:"entities"`
	Dialogue []ExtractedDialogue `json:"dialogue,omitempty"`
}

// ExtractedDialogue is one utterance the model attributed to a speaker.
type ExtractedDialogue struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

// Extract asks the model for the entities and attributed speech in a turn.
// Persisting them is the caller's job.
func (e *Extractor) Extract(ctx context.Context, narrative string) (*Extraction, error) {
	res, err := e.model.Generate(ctx, GenerateRequest{System: extractorSystemPrompt, Prompt: narrative})
	if err != nil {
		return nil, fmt.Errorf("extractor model failed: %w", err)
	}

	cleaned := strings.TrimSpace(res.Text)
	if start := strings.IndexAny(cleaned, "[{"); start != -1 {
		cleaned = cleaned[start:]
	}
	if end := strings.LastIndexAny(cleaned, "]}"); end != -1 {
		cleaned = cleaned[:end+1]
	}

	var result Extraction
	if err := json.Unmarshal([]byte(cleaned), &result); err == nil && (len(result.Entities) > 0 || len(result.Dialogue) > 0) {
		return &result, nil
	}

	// Older prompts asked for a bare array of entities.
	var entities []ExtractedEntity
	if err := json.Unmarshal([]byte(cleaned), &entities); err != nil {
		return nil, fmt.Errorf("parse extracted json %q: %w", cleaned, err)
	}
	return &Extraction{Entities: entities}, nil
}
```

Extend `extractorSystemPrompt` to ask for the object shape, keeping the entity fields unchanged:

```go
const extractorSystemPrompt = `You are a world-state extractor. Read the narrative turn and return a JSON object. Format:
{
  "entities": [
    {
      "id": "kebab-case-id",
      "name": "Full Name",
      "type": "character|location|item|faction|arc",
      "location": "[[Optional-Location]]",
      "body": "Description and known facts."
    }
  ],
  "dialogue": [
    { "speaker": "Full Name", "text": "Exactly what they said." }
  ]
}
List every line of direct speech in "dialogue", attributed to the speaker, using the same names as the entity list. Return empty arrays when nothing new is discovered.`
```

Update the tests that Task 13 introduced: `TestExtractorReturnsRecordsWithoutPersisting` now reads `result.Entities`, and any `Extract` call in `pkg/engine/orchestrator_test.go` is unaffected because it goes through the orchestrator.

In `pkg/engine/orchestrator.go`, replace the extraction block from Task 15 with:

```go
	extraction := harness.Extraction{}
	if o.extractor != nil {
		// A failed extractor must not lose the turn; mentions below still stand.
		if result, err := o.extractor.Extract(ctx, resp.Text); err == nil {
			extraction = *result
		}
	}

	turn.Segments = buildTurnSegments(o.store, mode, o.playerID, actionInput, resp.Text, extraction.Dialogue)

	turn.Entities = harness.ResolveEntityMentions(o.store, o.playerID, o.locationID, resp.Text, actionInput)
	for _, mention := range speechMentions(turn.Segments) {
		if !containsMention(turn.Entities, mention.ID) {
			turn.Entities = append(turn.Entities, mention)
		}
	}

	if err := o.timeline.RecordTurn(&turn, extraction.Entities); err != nil {
		return nil, fmt.Errorf("record turn: %w", err)
	}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./... && go vet ./...`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/entity/segment.go pkg/entity/mention.go pkg/engine/segments.go pkg/engine/segments_test.go pkg/engine/history.go pkg/engine/orchestrator.go pkg/harness/extractor.go pkg/harness/extractor_test.go
git commit -m "feat(engine): record ordered narration and speech segments per turn"
```

### Task 19: Playback consumes recorded segments

**Files:**
- Modify: `pkg/media/tts.go` (replace `UtteranceSegment` and `ParseDialogueSegments`)
- Modify: `pkg/media/tts_test.go`
- Test: `pkg/media/tts_test.go`

**Interfaces:**
- Consumes: `entity.TurnSegment`, `dialogue.Parse`, `ContentCache`
- Produces: `(*TTSPipeline).SynthesizeSegments(ctx context.Context, segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) ([]string, error)`, `media.LegacySegments(narration string) []entity.TurnSegment`

- [x] **Step 1: Write the failing test**

Replace the `ParseDialogueSegments` test in `pkg/media/tts_test.go` with:

```go
func TestLegacySegmentsKeepProseAndAttributeObviousSpeakers(t *testing.T) {
	segments := LegacySegments("The hall is quiet.\nGarrick: \"Keep walking.\"")

	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %#v", segments)
	}
	if segments[0].Kind != entity.SegmentNarration || segments[0].Text != "The hall is quiet." {
		t.Errorf("unexpected first segment %#v", segments[0])
	}
	if segments[1].Kind != entity.SegmentSpeech || segments[1].Speaker != "Garrick" {
		t.Errorf("unexpected second segment %#v", segments[1])
	}
}

func TestSynthesizeSegmentsUsesPerSpeakerVoicesAndCaches(t *testing.T) {
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))

	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The hall is quiet."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Keep walking."},
	}

	voices := map[string]*entity.VoiceConfig{
		"garrick": {VoiceID: "bm_george", Pitch: 0.85, SpeechRate: 0.9},
	}
	voiceFor := func(speakerID string) *entity.VoiceConfig { return voices[speakerID] }

	first, err := pipeline.SynthesizeSegments(context.Background(), segments, nil, voiceFor)
	if err != nil {
		t.Fatalf("SynthesizeSegments failed: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("expected 2 clips, got %d", len(first))
	}

	second, err := pipeline.SynthesizeSegments(context.Background(), segments, nil, voiceFor)
	if err != nil {
		t.Fatalf("second SynthesizeSegments failed: %v", err)
	}
	if first[1] != second[1] {
		t.Errorf("expected the cached clip to be reused, got %q then %q", first[1], second[1])
	}
	if client.calls != 2 {
		t.Errorf("expected 2 synthesis calls across both runs, got %d", client.calls)
	}
	if client.lastVoice == nil || client.lastVoice.VoiceID != "bm_george" {
		t.Errorf("expected the speaker's voice to be used, got %+v", client.lastVoice)
	}
}
```

with the recording client in the same file:

```go
type recordingTTSClient struct {
	calls     int
	lastVoice *entity.VoiceConfig
}

func (c *recordingTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	c.calls++
	c.lastVoice = voice
	return []byte("RIFF" + text), nil
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run "TestLegacySegments|TestSynthesizeSegments" -count=1 ./pkg/media/`
Expected: FAIL — `undefined: LegacySegments`, `pipeline.SynthesizeSegments undefined`

- [x] **Step 3: Implement**

In `pkg/media/tts.go`: delete `UtteranceSegment` and `ParseDialogueSegments` (both regexes go with them), then add:

```go
// LegacySegments parses pre-segment turns at playback time. Speaker names are
// resolved permissively: legacy records never carried entity IDs, so any speaker
// the prose names is accepted.
func LegacySegments(narration string) []entity.TurnSegment {
	segments := dialogue.Parse(narration, func(candidate string) (string, bool) {
		if strings.TrimSpace(candidate) == "" {
			return "", false
		}
		return "", true
	})

	result := make([]entity.TurnSegment, 0, len(segments))
	for _, segment := range segments {
		kind := entity.SegmentNarration
		if segment.IsSpeech {
			kind = entity.SegmentSpeech
		}
		result = append(result, entity.TurnSegment{
			Kind:    kind,
			Speaker: segment.Speaker,
			Text:    segment.Text,
		})
	}
	return result
}

// SynthesizeSegments renders every segment with its speaker's voice, falling back
// to the narrator voice for narration and unresolved speech. Cached clips are
// reused; audio references stay out of the turn record because the cache key is
// a pure function of speaker, voice, prosody, and text.
func (p *TTSPipeline) SynthesizeSegments(ctx context.Context, segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) ([]string, error) {
	clips := make([]string, 0, len(segments))

	for _, segment := range segments {
		voice := narratorVoice
		speakerID := narratorSpeaker
		if segment.Kind == entity.SegmentSpeech && segment.SpeakerID != "" {
			speakerID = segment.SpeakerID
			if voiceFor != nil {
				if resolved := voiceFor(segment.SpeakerID); resolved != nil {
					voice = resolved
				}
			}
		}

		clip, err := p.SynthesizeUtterance(ctx, speakerID, voice, segment.Text)
		if err != nil {
			return nil, err
		}
		clips = append(clips, clip)
	}

	return clips, nil
}
```

Add `"github.com/darkliquid/localrpg/pkg/dialogue"` to the imports and define the narrator fallback speaker id used by the pipeline:

```go
// narratorSpeaker is the cache-namespace for lines read in the narrator voice.
const narratorSpeaker = "narrator"
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/media/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/media/tts.go pkg/media/tts_test.go
git commit -m "feat(media): render playback from recorded segments and per-speaker voices"
```

### Task 20: Export per-segment speakers and audio

**Files:**
- Modify: `pkg/export/types.go` (`SceneBeat`)
- Modify: `pkg/export/script.go` (`Compile`)
- Modify: `pkg/export/web.go` (embedded player renders segments)
- Modify: `pkg/export/script_test.go`
- Test: `pkg/export/script_test.go`

**Interfaces:**
- Consumes: `engine.Turn.Segments`, `media.LegacySegments`
- Produces: `ReplayScript.Beats[].Segments []entity.TurnSegment`

- [x] **Step 1: Write the failing test**

Replace the speaker/dialogue assertions in `pkg/export/script_test.go` with:

```go
func TestCompileKeepsAttributedSegments(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "segmented")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte("id: segmented\nname: Segmented\n"), 0644); err != nil {
		t.Fatal(err)
	}

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Say","input":"Where is the ledger?","narration":"He does not look up. Garrick: \"Keep walking.\"","segments":[{"kind":"speech","speaker":"Sean","speaker_id":"player","text":"Where is the ledger?"},{"kind":"narration","text":"He does not look up."},{"kind":"speech","speaker":"Garrick","speaker_id":"garrick","text":"Keep walking."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "segmented")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if len(script.Beats) != 1 {
		t.Fatalf("expected 1 beat, got %d", len(script.Beats))
	}

	segments := script.Beats[0].Segments
	if len(segments) != 3 {
		t.Fatalf("expected 3 segments, got %#v", segments)
	}
	if segments[0].SpeakerID != "player" || segments[2].SpeakerID != "garrick" {
		t.Errorf("expected speaker IDs to survive compilation, got %#v", segments)
	}
}
```

Keep a variant of the existing legacy test: a record with only `narration` compiles into segments with the narration kind.

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestCompileKeepsAttributedSegments -count=1 ./pkg/export/`
Expected: FAIL — `script.Beats[0].Segments undefined`

- [x] **Step 3: Implement**

In `pkg/export/types.go`, replace `Speaker` and `Dialogue` on `SceneBeat` with the recorded segments, keeping `AudioPath` as it is:

```go
	// Segments is the ordered playback script: narration and attributed speech.
	Segments []entity.TurnSegment `json:"segments,omitempty"`
```

In `pkg/export/script.go`, add `github.com/darkliquid/localrpg/pkg/media` to the imports, add `github.com/darkliquid/localrpg/pkg/entity` to `pkg/export/types.go`, and drop `regexp`/`strings` from `script.go` if they become unused. Then drop the `dialogueSpeakerRegex` and build the beat from the record:

```go
		segments := turn.Segments
		if len(segments) == 0 {
			segments = media.LegacySegments(turn.Prose())
		}

		audioPath := ""
		if len(turn.AudioRefs) > 0 {
			audioPath = turn.AudioRefs[0]
		}

		beats[i] = SceneBeat{
			TurnNumber:  turn.Number,
			Timestamp:   turn.Timestamp,
			Mode:        turn.Mode,
			PlayerInput: turn.Input,
			Prose:       turn.Prose(),
			Segments:    segments,
			AudioPath:   audioPath,
			DurationSec: duration,
		}
```

Duration is now the sum over segments: 4 seconds per speech segment, 3.5 seconds per narration segment, accumulating into `totalDuration`.

In `pkg/export/web.go`, replace the single speaker/dialogue elements in the embedded template with a container rendered from `beat.segments`:

```javascript
    const container = document.getElementById('content');
    container.innerHTML = '';
    (beat.segments || [{ kind: 'narration', text: beat.prose }]).forEach((segment) => {
      if (segment.kind === 'speech') {
        const speaker = document.createElement('div');
        speaker.className = 'speaker';
        speaker.textContent = segment.speaker || 'Unknown';
        container.appendChild(speaker);
      }
      const line = document.createElement('div');
      line.textContent = segment.kind === 'speech' ? '"' + segment.text + '"' : segment.text;
      container.appendChild(line);
    });
```

and remove the now-unused `#speaker` element from the markup.

In `pkg/export/video.go`, leave the renderer as it is. It currently produces a silent still-image render (`-f lavfi color=...` plus `anullsrc`) and never reads `AudioPath`, so per-segment audio and timed frame changes are a separate piece of work owned by the story-theater export effort. Do not add speculative audio plumbing here; the segments are now on the beat for whoever picks that up.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/export/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/export/types.go pkg/export/script.go pkg/export/script_test.go pkg/export/web.go pkg/export/video.go
git commit -m "feat(export): replay and render turns from recorded segments"
```

## Phase 7: Surface the Timeline

### Task 21: Chronicle and entity APIs expose involvement and segments

**Files:**
- Modify: `pkg/gui/types.go` (`SegmentDTO`, `TurnDTO.Segments`, `EntityDTO.History`)
- Modify: `pkg/gui/service.go` (index-on-first-use, `GetChronicle`, `GetEntity`, new `GetEntityTurns`)
- Modify: `pkg/gui/server.go` (entity turns route)
- Test: `pkg/gui/service_test.go`, `pkg/gui/server_test.go`

**Interfaces:**
- Consumes: `storage.ListTurnsForEntity`, `engine.Timeline.EnsureIndexed`, `HistoryLogger.LoadHistory`
- Produces: `gui.SegmentDTO{Kind, Speaker, SpeakerID, Text}`, `TurnDTO.EntitiesHit []string`, `TurnDTO.Segments []SegmentDTO`, `EntityDTO.History []int`, `(*Service).GetEntityTurns(ctx context.Context, gameID, entityID string) ([]TurnDTO, error)`, route `GET /api/game/{id}/entity/{eid}/turns`

- [x] **Step 1: Write the failing tests**

Append to `pkg/gui/service_test.go`:

```go
func TestGetChronicleAndEntityTurnsReportInvolvement(t *testing.T) {
	gameID, svc := setupTestGame(t)
	gameDir := svc.GetResolver().GameDir(gameID)

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"I ask the captain","narration":"The captain nods.","segments":[{"kind":"narration","text":"The captain nods."}],"entities":[{"id":"player-elena","mention":"player"},{"id":"captain-kaelen","mention":"wikilink"}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	turns, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetChronicle failed: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if len(turns[0].EntitiesHit) != 2 {
		t.Errorf("expected 2 entities hit, got %+v", turns[0].EntitiesHit)
	}
	if len(turns[0].Segments) != 1 || turns[0].Segments[0].Kind != "narration" {
		t.Errorf("expected segments to reach the DTO, got %+v", turns[0].Segments)
	}

	involved, err := svc.GetEntityTurns(context.Background(), gameID, "captain-kaelen")
	if err != nil {
		t.Fatalf("GetEntityTurns failed: %v", err)
	}
	if len(involved) != 1 || involved[0].TurnNumber != 1 {
		t.Fatalf("expected turn 1 for captain-kaelen, got %+v", involved)
	}

	uninvolved, err := svc.GetEntityTurns(context.Background(), gameID, "nobody-at-all")
	if err != nil {
		t.Fatalf("GetEntityTurns failed: %v", err)
	}
	if len(uninvolved) != 0 {
		t.Errorf("expected no turns for an uninvolved entity, got %+v", uninvolved)
	}
}
```

Append to `pkg/gui/server_test.go`:

```go
func TestEntityTurnsRoute(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/entity/captain-kaelen/turns", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestGetChronicleAndEntityTurns|TestEntityTurnsRoute" -count=1 ./pkg/gui/`
Expected: FAIL — `turns[0].EntitiesHit` is empty, `svc.GetEntityTurns undefined`

- [x] **Step 3: Implement**

In `pkg/gui/types.go`:

```go
type SegmentDTO struct {
	Kind      string `json:"kind"`
	Speaker   string `json:"speaker,omitempty"`
	SpeakerID string `json:"speaker_id,omitempty"`
	Text      string `json:"text"`
}
```

Add to `TurnDTO`:

```go
	Segments []SegmentDTO `json:"segments,omitempty"`
```

Add to `EntityDTO`:

```go
	History []int `json:"history,omitempty"`
```

In `pkg/gui/service.go`:

- Add `indexed map[string]bool` to `Service` and initialise it in `NewService`.
- Add the once-per-game repair:

```go
// ensureIndexed repairs the campaign index the first time this process serves it,
// so timeline queries answer from the database rather than re-reading files.
func (s *Service) ensureIndexed(gameID string) {
	s.mu.Lock()
	if s.indexed[gameID] {
		s.mu.Unlock()
		return
	}
	s.indexed[gameID] = true
	s.mu.Unlock()

	gameDir := s.resolver.GameDir(gameID)
	store, err := s.store(gameID)
	if err != nil {
		return
	}

	_, _ = storage.NewSyncer(store).Sync(filepath.Join(gameDir, "entities"))
	history := engine.NewHistoryLogger(filepath.Join(gameDir, "history.jsonl"))
	_ = engine.NewTimeline(s.resolver, store, history, gameID).EnsureIndexed()
}
```

- `GetChronicle` keeps reading `history.jsonl` and maps the new fields:

```go
		dtos[i] = TurnDTO{
			TurnNumber:  turn.Number,
			InputText:   turn.Input,
			Mode:        turn.Mode,
			Prose:       turn.Prose(),
			EntitiesHit: mentionIDs(turn.Entities),
			Segments:    segmentDTOs(turn.Segments),
		}
```

- `GetEntity` sets `History: ent.History` on the returned DTO.
- Add the query:

```go
// GetEntityTurns returns the turns an entity took part in, newest last.
func (s *Service) GetEntityTurns(ctx context.Context, gameID, entityID string) ([]TurnDTO, error) {
	s.ensureIndexed(gameID)

	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	numbers, err := store.ListTurnsForEntity(entityID)
	if err != nil {
		return nil, err
	}
	if len(numbers) == 0 {
		return []TurnDTO{}, nil
	}

	wanted := make(map[int]bool, len(numbers))
	for _, number := range numbers {
		wanted[number] = true
	}

	all, err := s.GetChronicle(ctx, gameID)
	if err != nil {
		return nil, err
	}

	turns := make([]TurnDTO, 0, len(numbers))
	for _, turn := range all {
		if wanted[turn.TurnNumber] {
			turns = append(turns, turn)
		}
	}
	return turns, nil
}
```

plus the two mapping helpers:

```go
func mentionIDs(mentions []entity.Mention) []string {
	ids := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		ids = append(ids, mention.ID)
	}
	return ids
}

func segmentDTOs(segments []entity.TurnSegment) []SegmentDTO {
	dtos := make([]SegmentDTO, 0, len(segments))
	for _, segment := range segments {
		dtos = append(dtos, SegmentDTO{
			Kind:      segment.Kind,
			Speaker:   segment.Speaker,
			SpeakerID: segment.SpeakerID,
			Text:      segment.Text,
		})
	}
	return dtos
}
```

In `pkg/gui/server.go`'s `handleGameRoutes` `case "entity":`, add the nested route before the GET fallback:

```go
		if len(parts) >= 4 && parts[3] == "turns" && r.Method == http.MethodGet {
			turns, err := s.service.GetEntityTurns(r.Context(), gameID, entityID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, turns)
			return
		}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/service_test.go pkg/gui/server.go pkg/gui/server_test.go
git commit -m "feat(gui): expose timeline involvement and attributed segments"
```

### Task 22: Frontend renders segments, entity chips, and turn history

**Files:**
- Modify: `frontend/src/types.ts`
- Create: `frontend/src/components/TurnSegments.tsx`, `frontend/src/components/TurnHistoryList.tsx`
- Modify: `frontend/src/components/ChronicleView.tsx`, `frontend/src/components/CodexDrawer.tsx`, `frontend/src/components/StoryTheater.tsx`
- Test: `frontend/src` type-checks via `npx tsc --noEmit`

**Interfaces:**
- Consumes: `Turn.segments`, `Turn.entities_hit`, `EntityNote.history`
- Produces: `TurnSegments({ segments, fallback, onEntityClick })`, `TurnHistoryList({ turns })`

- [x] **Step 1: Update the shared types**

In `frontend/src/types.ts`:

```typescript
export interface TurnSegment {
  kind: 'narration' | 'speech';
  speaker?: string;
  speaker_id?: string;
  text: string;
}

export interface Turn {
  turn_number: number;
  input_text: string;
  mode: string;
  prose: string;
  segments?: TurnSegment[];
  entities_hit?: string[];
  image_url?: string;
}

export interface EntityNote {
  id: string;
  name: string;
  type: string;
  markdown: string;
  state: Record<string, any>;
  backlinks: string[];
  history?: number[];
}
```

`speaker`, `dialogue`, and `audio_url` are dropped from `Turn` because the API no longer sends them; remove their usages in the same commit.

- [x] **Step 2: Run the type check to verify it fails**

Run: `cd frontend && npx tsc --noEmit`
Expected: FAIL — `ChronicleView.tsx` reads `turn.speaker`, `turn.dialogue`, and `turn.audio_url`, which no longer exist on `Turn`.

- [x] **Step 3: Implement**

Create `frontend/src/components/TurnSegments.tsx`:

```tsx
import React from 'react';
import { TurnSegment } from '../types';

interface TurnSegmentsProps {
  segments?: TurnSegment[];
  fallback: string;
  onEntityClick?: (name: string) => void;
}

const renderWithLinks = (text: string, onEntityClick?: (name: string) => void) =>
  text.split(/(\[\[[^\]]+\]\])/g).map((part, i) => {
    if (part.startsWith('[[') && part.endsWith(']]')) {
      const link = part.slice(2, -2);
      return (
        <button
          key={i}
          onClick={() => onEntityClick?.(link)}
          className="text-amber-400 hover:text-amber-300 underline font-medium cursor-pointer mx-1 transition-colors"
        >
          {link}
        </button>
      );
    }
    return <span key={i}>{part}</span>;
  });

export const TurnSegments: React.FC<TurnSegmentsProps> = ({ segments, fallback, onEntityClick }) => {
  const ordered = segments && segments.length > 0 ? segments : [{ kind: 'narration' as const, text: fallback }];

  return (
    <div className="space-y-3">
      {ordered.map((segment, i) =>
        segment.kind === 'speech' ? (
          <div
            key={i}
            className="bg-glass-card border-l-4 border-amber-500/90 pl-4 py-3 pr-4 rounded-r-xl shadow-lg space-y-2"
          >
            <div className="text-xs text-amber-400 font-cinzel font-bold tracking-widest">
              {segment.speaker || 'UNKNOWN'}
            </div>
            <p className="text-stone-100 text-lg leading-relaxed italic">
              &ldquo;{renderWithLinks(segment.text, onEntityClick)}&rdquo;
            </p>
          </div>
        ) : (
          <div key={i} className="text-stone-200 text-xl leading-relaxed tracking-wide font-serif">
            {renderWithLinks(segment.text, onEntityClick)}
          </div>
        )
      )}
    </div>
  );
};
```

Create `frontend/src/components/TurnHistoryList.tsx`:

```tsx
import React from 'react';

interface TurnHistoryListProps {
  turns?: number[];
  onTurnClick?: (turnNumber: number) => void;
}

export const TurnHistoryList: React.FC<TurnHistoryListProps> = ({ turns, onTurnClick }) => {
  if (!turns || turns.length === 0) {
    return null;
  }

  return (
    <div className="flex flex-wrap items-center gap-2 text-xs font-cinzel tracking-wider text-stone-400">
      <span className="uppercase">Appears in</span>
      {turns.map((turn) => (
        <button
          key={turn}
          onClick={() => onTurnClick?.(turn)}
          className="px-2 py-0.5 rounded border border-white/10 hover:border-amber-500/60 hover:text-amber-300 cursor-pointer transition-colors"
        >
          Turn {turn}
        </button>
      ))}
    </div>
  );
};
```

In `frontend/src/components/ChronicleView.tsx`:

- Import `TurnSegments` and delete `renderFormattedText` (it moves into `TurnSegments`).
- Replace the dialogue block, the prose block, and the `Volume2` import with:

```tsx
            <TurnSegments
              segments={turn.segments}
              fallback={turn.prose}
              onEntityClick={onWikilinkClick}
            />

            {turn.entities_hit && turn.entities_hit.length > 0 && (
              <div className="flex flex-wrap items-center gap-2 pt-1">
                {turn.entities_hit.map((entityId) => (
                  <button
                    key={entityId}
                    onClick={() => onWikilinkClick(entityId)}
                    className="px-2 py-0.5 text-xs font-cinzel tracking-wider rounded-full bg-white/5 border border-white/10 text-stone-300 hover:text-amber-300 hover:border-amber-500/60 cursor-pointer transition-colors"
                  >
                    {entityId}
                  </button>
                ))}
              </div>
            )}
```

In `frontend/src/components/CodexDrawer.tsx`, render the entity's turn list directly above the markdown editor:

```tsx
        <TurnHistoryList turns={entity.history} />
```

In `frontend/src/components/StoryTheater.tsx`, replace the single speaker/dialogue block (lines 64-78, which read `currentTurn.speaker`, `currentTurn.audio_url`, and `currentTurn.dialogue`) with:

```tsx
          <TurnSegments segments={currentTurn?.segments} fallback={currentTurn?.prose ?? ''} />
```

and drop the now-unused `Volume2` import. Story Theater is read-only playback, so it passes no click handler.

- [x] **Step 4: Run the type check to verify it passes**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS (no unused imports, since `noUnusedLocals` is enabled)

- [x] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/components/TurnSegments.tsx frontend/src/components/TurnHistoryList.tsx frontend/src/components/ChronicleView.tsx frontend/src/components/CodexDrawer.tsx
git commit -m "feat(frontend): show attributed dialogue, entity involvement, and turn history"
```

---

## Spec Coverage

| Spec section | Tasks |
| --- | --- |
| §3.1 layered ownership, §3.2 canonical access | 1, 2, 3, 4 |
| §3.3 legacy `game.db` migration | 2 |
| §3.4 turn record schema | 5, 8, 18 |
| §3.5 entity history frontmatter | 12, 14 |
| §3.6 database schema | 5, 6, 7 |
| §4 turn lifecycle and failure handling | 9, 14, 15 |
| §5.1 deterministic mentions | 13, 18 |
| §5.2 extractor role | 15 |
| §5.3 matching, merging, writing | 13, 14, 15 |
| §6 undo, rewind, index integrity | 10, 16 |
| §7.1 one parser | 17 |
| §7.2 writing the segments | 18 |
| §7.3 playback | 19, 20 |
| §8 API and UI surface | 21, 22 |
| §9 migration and compatibility | 8, 10, 19, 20 |
| §11 verification plan | every task's Step 1 |

### Deferred

- `AddInsecureBypassPattern`-style CORS changes: out of scope, `gui.ProtectCrossOrigin` already covers it.
- Telling authored GM prompts to use the `Name: "…"` convention (spec §13, third open question). The extractor's `dialogue` array covers it for now; if deterministic attribution proves too weak, add the instruction to `prompt` files in a later change.
- Per-entity roll outcomes in `turn_entities` (spec §13, second open question).
- A Codex drawer that lists turns for an entity read straight from the API has the endpoint (`GetEntityTurns`, Task 21) but Task 22 renders the cheaper `history` list from the note itself. Wire the endpoint when the drawer needs full turn text.

---

## Execution Notes

Implemented on top of the spec/plan and AGENTS.md doc commits: `b278da4` storage, `51f23a6` engine and playback, `e6ebf9c` frontend, then the docs alignment carrying this section. Every commit was checked out into a scratch worktree and built on its own, so each one is a usable bisect point.

The first attempt at slicing this got the dependency order wrong: `pkg/export` calls `media.LegacySegments`, so committing the export changes before the media changes produced a commit that did not build. The playback work therefore ships inside the engine commit. When splitting a change like this, check each commit with `git worktree add --detach /tmp/verify <sha> && (cd /tmp/verify && go build ./...)` rather than assuming the layers are independent.

Deviations from the task steps as written:

- **Task 8 absorbed Task 11's read-site renames.** Renaming `Turn.Output` to `Narration` leaves `pkg/tui`, `pkg/gui`, and `pkg/export` uncompilable until they call `Prose()`, so the renames shipped with the rename itself. Task 11 kept its legacy-export test.
- **Task 16 also routed `/undo` through `Timeline.RewindToTurn`.** Task 15 left the orchestrator calling `history.RewindToTurn`; rewinding the index is the whole point of the task, so the call moved with it.
- **`Turn.Entities` is `[]entity.Mention`, not `[]string`.** Provenance (`turn_entities.mention`) is only rebuildable from the log if the log records it.
- **`DialogueLine` became `entity.TurnSegment`.** A list of speech lines cannot say where in the prose each line sits, so playback would either double-read dialogue or drop prose. Segments cover narration *and* speech, in order.
- **`media.UtteranceSegment` was deleted, not deprecated.** `entity.TurnSegment` replaces it outright; `LegacySegments` covers pre-`segments` records.
- **`pkg/export/video.go` was left unchanged**, as the task predicted: it renders a silent still image and never reads audio. Per-segment audio remains with the story-theater export effort. **[Erratum 2026-09-28: the follow-up landed. `pkg/export/video.go` now animates and muxes per-beat audio; see `2026-09-21-animated-export-and-video.md`.]**

Test files landed under the names below rather than the plan's single-file predictions: `pkg/engine/{timeline,record_turn,rewind,segments,startlocation}_test.go`, `pkg/engine/orchestrator_{input,timeline}_test.go`, `pkg/entity/history_test.go`, `pkg/storage/{db,pool,game,turn,entity_history}_test.go`, `pkg/export/legacy_script_test.go`, `pkg/gui/middleware_test.go`, `pkg/dialogue/dialogue_test.go`.

Two defects surfaced while executing and were fixed in-flight: a pooled store that ignored `Close` was needed so `Session.Close()` could not leave a dead handle behind, and `strings.Trim(TrimSpace(x), quotes)` left a trailing space when splitting narration around attributed speech.
