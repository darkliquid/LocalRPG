# Content Organisation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an entity note live in a nested folder without its identity becoming its path, so a campaign can be organised into places, factions and acts while every `[[wikilink]]` keeps working.

**Architecture:** The frontmatter `id` becomes the stable identity and the file path becomes location only. `Syncer.Sync` walks `entities/` recursively and records each note's folder in a new `entities.folder` column (migration 12). Listing and graph reads walk the tree too, folder CRUD is exposed as routes for the GUI tree, and moving a note is a save with a new `folder` because the id does not change.

**Tech Stack:** Go standard library, `modernc.org/sqlite` (no CGO), React 19 + TypeScript + Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-10-04-content-organisation-design.md`

**Depends on:** nothing. This is the first of three plans on `feat/content-authoring`.

## Global Constraints

- Go standard library only for tests (`testing`, `t.TempDir()`); no testify. Use `any`, not `interface{}`. `go vet ./...` must stay clean.
- Errors wrapped with `fmt.Errorf("...: %w", err)`. Identifiers go through `entity.Slugify` and `entity.WikilinkTarget`; do not write local slug or link parsing.
- Migration versions are contiguous and idempotent; the next free version is **12**.
- `entities.id` stays `TEXT PRIMARY KEY`: the id is globally unique per collection, and a duplicate is a `409`, never an overwrite.
- A folder is organisational only. Do not touch the context assembler, scene scope, `MatchExistingEntity`, `history.jsonl` or the timeline.
- Never call `storage.NewStore` for a game; go through `storage.OpenGameStore`.
- Frontend: `tsconfig.json` sets `strict`, `noUnusedLocals`, `noUnusedParameters`, so `npm run build` fails on an unused import. Components live in `frontend/src/components/`, icons come from `lucide-react`.
- When adding an endpoint, update `Service`, `pkg/gui/server.go`, `frontend/src/types.ts` and `frontend/src/api/client.ts` together.
- Commits are Conventional Commits with a scope; subject under 72 characters.

---

### File Map

- **`pkg/entity/entity.go`** — add `Entity.Folder`; add `WikilinkBasename`. `Folder` is never serialized.
- **`pkg/entity/entity_test.go`** — folder is not emitted; basename helper.
- **`pkg/storage/db.go`** — `entities` table gains `folder` for fresh databases.
- **`pkg/storage/migrate.go`** — migration 12 adds the column to existing databases.
- **`pkg/storage/migrate_test.go`** — migration 12 is idempotent.
- **`pkg/storage/store.go`** — `SaveEntity` writes `folder`; `GetEntity` reads it.
- **`pkg/storage/store_test.go`** — folder round-trip.
- **`pkg/storage/sync.go`** — recursive walk, folder per note.
- **`pkg/storage/sync_test.go`** — nested notes, hidden dirs, `assets/`.
- **`pkg/gui/folders.go`** (new) — path validation and the folder tree builder.
- **`pkg/gui/folders_test.go`** (new) — validation and tree.
- **`pkg/gui/service.go`** — `eachEntityNote` walker; recursive `ListEntities`, `GetGraph`, `rewriteInboundLinks`; folder CRUD; entity move; duplicate-id guard.
- **`pkg/gui/service_test.go`** — recursion, move, 409, folder CRUD.
- **`pkg/gui/types.go`** — `folder` on the entity DTOs, folder DTOs, request bodies.
- **`pkg/gui/server.go`** — folder routes, `folder` on entity save, `409` mapping.
- **`frontend/src/types.ts`** — `folder`, folder DTOs.
- **`frontend/src/api/client.ts`** — folder methods.
- **`frontend/src/components/EntityTree.tsx`** (new) — the tree.
- **`frontend/src/components/CodexDrawer.tsx`** — use the tree for the note list.
- **`frontend/src/components/WorldsStudio.tsx`** — use the tree for the template list.

---

### Task 1: `Folder` on the entity, never serialized

**Files:**
- Modify: `pkg/entity/entity.go`
- Test: `pkg/entity/entity_test.go`

**Interfaces:**
- Produces: `Entity.Folder string`; `func WikilinkBasename(target string) string`

- [ ] **Step 1: Write the failing test**

Append to `pkg/entity/entity_test.go`:

```go
func TestSerializeMarkdownOmitsFolder(t *testing.T) {
	ent := &Entity{
		ID:      "silver-hand",
		Name:    "Silver Hand",
		Type:    "faction",
		Folder:  "factions/orders",
		Body:    "A guild of smiths.\n",
		Aliases: []string{"The Hand"},
	}

	data, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown: %v", err)
	}
	if strings.Contains(string(data), "folder") {
		t.Fatalf("folder is a location, not frontmatter; got:\n%s", data)
	}
	if !strings.Contains(string(data), "id: silver-hand") {
		t.Fatalf("frontmatter lost the id:\n%s", data)
	}
}

func TestWikilinkBasename(t *testing.T) {
	cases := map[string]string{
		"silver-hand":            "silver-hand",
		"guilds/silver-hand":     "silver-hand",
		"guilds/orders/the-hand": "the-hand",
		"  guilds/silver-hand  ": "silver-hand",
		"":                       "",
	}
	for input, want := range cases {
		if got := WikilinkBasename(input); got != want {
			t.Errorf("WikilinkBasename(%q) = %q, want %q", input, got, want)
		}
	}
}
```

Add `"strings"` to that file's imports if it is not already there.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/entity/ -run 'TestSerializeMarkdownOmitsFolder|TestWikilinkBasename' -v`
Expected: FAIL, `ent.Folder` and `WikilinkBasename` undefined.

- [ ] **Step 3: Implement**

In `pkg/entity/entity.go`, add the field to `Entity`, after `Hash string`:

```go
	// Folder is where the note sits under entities/, relative and slash-separated,
	// with "" for the root. It is a location, not frontmatter: the same note in two
	// folders is the same note, so SerializeMarkdown must never emit it.
	Folder string
```

Add below `WikilinkTarget`:

```go
// WikilinkBasename returns the final path segment of a link target, so a
// hand-written [[guilds/silver-hand]] can still resolve to the note whose id is
// silver-hand. The app never generates the path-qualified form.
func WikilinkBasename(target string) string {
	cleaned := strings.TrimSpace(target)
	if idx := strings.LastIndex(cleaned, "/"); idx >= 0 {
		cleaned = cleaned[idx+1:]
	}
	return strings.TrimSpace(cleaned)
}
```

Do not add `Folder` to `EntityFrontmatter` or to `SerializeMarkdown`; the test is the guard.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/entity/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/entity/entity.go pkg/entity/entity_test.go
git commit -m "feat(entity): give a note a folder that is not its identity"
```

---

### Task 2: The `folder` column and its migration

**Files:**
- Modify: `pkg/storage/db.go`, `pkg/storage/migrate.go`, `pkg/storage/store.go`
- Test: `pkg/storage/migrate_test.go`, `pkg/storage/store_test.go`

**Interfaces:**
- Consumes: `entity.Entity.Folder` (Task 1)
- Produces: `entities.folder`; `Store.SaveEntity` persists `Entity.Folder`; `Store.GetEntity` populates it.

- [ ] **Step 1: Write the failing test**

Append to `pkg/storage/store_test.go`:

```go
func TestSaveEntityRecordsFolder(t *testing.T) {
	store := newTestStore(t)

	ent := &entity.Entity{
		ID:     "silver-hand",
		Name:   "Silver Hand",
		Type:   "faction",
		Folder: "factions/orders",
		Body:   "A guild of smiths.\n",
	}
	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}

	got, err := store.GetEntity("silver-hand")
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if got.Folder != "factions/orders" {
		t.Errorf("Folder = %q, want %q", got.Folder, "factions/orders")
	}
}

func TestSaveEntityRootFolderIsEmpty(t *testing.T) {
	store := newTestStore(t)

	ent := &entity.Entity{ID: "loose-note", Name: "Loose Note", Type: "item", Body: "x\n"}
	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}
	got, err := store.GetEntity("loose-note")
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if got.Folder != "" {
		t.Errorf("Folder = %q, want the empty root folder", got.Folder)
	}
}
```

If `newTestStore` does not already exist in that package's tests, use the existing helper the file already uses to open a temp store and keep its name.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/storage/ -run 'TestSaveEntityRecordsFolder|TestSaveEntityRootFolderIsEmpty' -v`
Expected: FAIL, `no such column: folder`.

- [ ] **Step 3: Add the column to the fresh-database schema**

In `pkg/storage/db.go`, in the `entities` table inside the schema constant:

```sql
CREATE TABLE IF NOT EXISTS entities (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    frontmatter_json TEXT NOT NULL,
    body TEXT NOT NULL,
    file_hash TEXT NOT NULL,
    folder TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

- [ ] **Step 4: Add migration 12**

In `pkg/storage/migrate.go`, append to the `migrations` slice:

```go
	{version: 12, apply: addEntityFolderColumn},
```

and add the function beside the other migration helpers:

```go
// addEntityFolderColumn records where a note sits under entities/, so the codex
// can render a tree from the index instead of walking the filesystem on every
// list. The folder is a location: the note's id is still its identity.
func addEntityFolderColumn(db *sql.DB) error {
	exists, err := columnExists(db, "entities", "folder")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := db.Exec("ALTER TABLE entities ADD COLUMN folder TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("add entities.folder: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Persist and read the folder**

In `pkg/storage/store.go`, in `SaveEntity`, extend the statement and its arguments:

```go
	query := `
	INSERT INTO entities (id, name, type, frontmatter_json, body, file_hash, folder, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		name = excluded.name,
		type = excluded.type,
		frontmatter_json = excluded.frontmatter_json,
		body = excluded.body,
		file_hash = excluded.file_hash,
		folder = excluded.folder,
		updated_at = CURRENT_TIMESTAMP
	`
	if _, err := tx.Exec(query, e.ID, e.Name, e.Type, string(fmJSON), e.Body, e.Hash, e.Folder); err != nil {
		return fmt.Errorf("upsert entity: %w", err)
	}
```

In `GetEntity`, extend the select and the scan:

```go
	query := `SELECT id, name, type, frontmatter_json, body, file_hash, folder FROM entities WHERE id = ?`
	row := s.db.QueryRow(query, id)

	var ent entity.Entity
	var fmJSON string
	if err := row.Scan(&ent.ID, &ent.Name, &ent.Type, &fmJSON, &ent.Body, &ent.Hash, &ent.Folder); err != nil {
		return nil, err
	}
```

- [ ] **Step 6: Write the migration idempotency test**

Append to `pkg/storage/migrate_test.go`:

```go
func TestMigrationTwelveIsIdempotent(t *testing.T) {
	db := newTestDB(t)

	if err := addEntityFolderColumn(db); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := addEntityFolderColumn(db); err != nil {
		t.Fatalf("second apply must be a no-op, got: %v", err)
	}

	exists, err := columnExists(db, "entities", "folder")
	if err != nil {
		t.Fatalf("columnExists: %v", err)
	}
	if !exists {
		t.Fatal("entities.folder missing after migration")
	}
}
```

Use whichever helper the file already uses to open a bare `*sql.DB`; if none exists, call the same open path the other migration tests use.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./pkg/storage/ -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add pkg/storage/db.go pkg/storage/migrate.go pkg/storage/store.go pkg/storage/store_test.go pkg/storage/migrate_test.go
git commit -m "feat(storage): record a note's folder in the index"
```

---

### Task 3: Sync walks the tree

**Files:**
- Modify: `pkg/storage/sync.go`
- Test: `pkg/storage/sync_test.go`

**Interfaces:**
- Consumes: `Entity.Folder` (Task 1), `entities.folder` (Task 2)
- Produces: `Syncer.Sync` indexes nested notes; `Syncer.SyncFile` records the folder of the file it is given.

- [ ] **Step 1: Write the failing test**

Append to `pkg/storage/sync_test.go`:

```go
func TestSyncWalksNestedFolders(t *testing.T) {
	dir := t.TempDir()
	store := newTestStore(t)

	mustWriteNote(t, filepath.Join(dir, "silver-hand.md"), "silver-hand", "Silver Hand")
	mustWriteNote(t, filepath.Join(dir, "factions", "orders", "ashen-order.md"), "ashen-order", "Ashen Order")
	// Hidden directories and non-markdown files are not notes.
	mustWriteNote(t, filepath.Join(dir, ".obsidian", "hidden.md"), "hidden", "Hidden")
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	mustWriteNote(t, filepath.Join(dir, "assets", "art.md"), "art", "Art")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write notes.txt: %v", err)
	}

	res, err := NewSyncer(store).Sync(dir)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Added != 2 {
		t.Fatalf("Added = %d, want 2", res.Added)
	}

	root, err := store.GetEntity("silver-hand")
	if err != nil {
		t.Fatalf("GetEntity(silver-hand): %v", err)
	}
	if root.Folder != "" {
		t.Errorf("root Folder = %q, want %q", root.Folder, "")
	}

	nested, err := store.GetEntity("ashen-order")
	if err != nil {
		t.Fatalf("GetEntity(ashen-order): %v", err)
	}
	if nested.Folder != "factions/orders" {
		t.Errorf("nested Folder = %q, want %q", nested.Folder, "factions/orders")
	}

	if _, err := store.GetEntity("hidden"); err == nil {
		t.Error("a hidden directory must not be indexed")
	}
	if _, err := store.GetEntity("art"); err == nil {
		t.Error("assets/ must not be indexed")
	}
}

func TestSyncFileRecordsFolder(t *testing.T) {
	dir := t.TempDir()
	store := newTestStore(t)

	path := filepath.Join(dir, "places", "port-vel.md")
	mustWriteNote(t, path, "port-vel", "Port Vel")

	if err := NewSyncer(store).SyncFile(path); err != nil {
		t.Fatalf("SyncFile: %v", err)
	}

	got, err := store.GetEntity("port-vel")
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if got.Folder != "places" {
		t.Errorf("Folder = %q, want %q", got.Folder, "places")
	}
}
```

Add the shared helper if the file does not have one:

```go
// mustWriteNote writes a minimal, valid entity note.
func mustWriteNote(t *testing.T, path, id, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	body := fmt.Sprintf("---\nid: %s\nname: %s\ntype: character\n---\n\nA note.\n", id, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/storage/ -run 'TestSyncWalksNestedFolders|TestSyncFileRecordsFolder' -v`
Expected: FAIL, `Added = 1, want 2` and `Folder = "", want "places"`.

- [ ] **Step 3: Rewrite `Sync` as a walk**

In `pkg/storage/sync.go`, replace `Sync` with:

```go
// Sync indexes every note under dir. The walk is recursive, because a note's
// folder is a location the author chose; hidden directories and assets/ are not
// note storage, so they are skipped whole.
func (s *Syncer) Sync(dir string) (*SyncResult, error) {
	res := &SyncResult{}

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return fmt.Errorf("walk %q: %w", path, walkErr)
		}
		if entry.IsDir() {
			if path == dir {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "assets" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			return nil
		}
		if err := s.syncNote(dir, path); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// syncNote parses one note and upserts it, so the counters and the skip-on-parse-
// failure rule live in one place.
func (s *Syncer) syncNote(root, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}

	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		// A note that will not parse is skipped rather than fatal: a missing
		// entity in the codex is almost always a frontmatter typo, and one bad
		// file must not stop the rest of the campaign from indexing.
		return nil
	}
	if ent.ID == "" {
		ent.ID = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	ent.Folder = folderFromPath(root, path)

	existing, err := s.store.GetEntity(ent.ID)
	if err == nil && existing.Hash == ent.Hash && existing.Folder == ent.Folder {
		return nil
	}
	if err := s.store.SaveEntity(ent); err != nil {
		return fmt.Errorf("save entity %q: %w", ent.ID, err)
	}
	return nil
}

// folderFromPath is the stored folder form: the directory relative to the
// entities root, slash-separated, with "" for the root itself.
func folderFromPath(root, path string) string {
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil || rel == "." || rel == "" {
		return ""
	}
	return filepath.ToSlash(rel)
}
```

Note: `SyncResult`'s counters are no longer filled by the walk. Keep the struct for callers, but since the walk above does not distinguish added from updated, either drop the counters from the return or compute them inside `syncNote` by returning a small enum. Use the enum:

```go
// syncOutcome tells the walk whether a note was new, changed, or already indexed.
type syncOutcome int

const (
	syncUnchanged syncOutcome = iota
	syncAdded
	syncUpdated
)
```

Make `syncNote` return `(syncOutcome, error)`, return `syncUnchanged` for a parse failure and for an unchanged hash, and have `Sync` increment the matching counter:

```go
		outcome, err := s.syncNote(dir, path)
		if err != nil {
			return err
		}
		switch outcome {
		case syncAdded:
			res.Added++
		case syncUpdated:
			res.Updated++
		default:
			res.Unchanged++
		}
		return nil
```

- [ ] **Step 4: Record the folder in `SyncFile`**

Replace `SyncFile` with:

```go
// SyncFile indexes one note, which is what a save uses so the index never waits
// for a full resync. The collection root is found by walking up to the entities/
// directory, so no caller can pass the wrong one and the signature stays a single
// path.
func (s *Syncer) SyncFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}

	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return fmt.Errorf("parse %q: %w", path, err)
	}

	ent.Folder = folderFromPath(entitiesRootFor(path), path)
	return s.store.SaveEntity(ent)
}

// entitiesRootFor walks up from a note's path to the entities/ directory that
// contains it. A note always lives under one, so the root needs no argument.
func entitiesRootFor(path string) string {
	dir := filepath.Dir(path)
	for {
		if filepath.Base(dir) == "entities" {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(path)
		}
		dir = parent
	}
}
```

The signature is deliberately unchanged. `SyncFile` has eight call sites across `pkg/engine` and `pkg/gui`, and every one of them passes a path under a directory named `entities`, so deriving the root removes eight chances to pass the wrong one.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/storage/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/storage/sync.go pkg/storage/sync_test.go
git commit -m "feat(storage): index notes nested in folders"
```

---

### Task 4: Listing and the graph see nested notes

**Files:**
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/service_test.go`

**Interfaces:**
- Consumes: `Syncer` recursion (Task 3), `Entity.Folder` (Task 1)
- Produces: `func eachEntityNote(entitiesDir string, fn func(path, folder string, data []byte) error) error`; `EntitySummaryDTO.Folder`; `EntityDTO.Folder`

- [ ] **Step 1: Write the failing test**

Append to `pkg/gui/service_test.go`:

```go
func TestListEntitiesIncludesNestedNotes(t *testing.T) {
	svc, gameID := newTestServiceWithGame(t)
	dir := filepath.Join(svc.resolver.GameDir(gameID), "entities")

	mustWriteEntityNote(t, filepath.Join(dir, "silver-hand.md"), "silver-hand", "Silver Hand")
	mustWriteEntityNote(t, filepath.Join(dir, "factions", "ashen-order.md"), "ashen-order", "Ashen Order")

	got, err := svc.ListEntities(context.Background(), gameID)
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	folders := map[string]string{}
	for _, summary := range got {
		folders[summary.ID] = summary.Folder
	}
	if folders["silver-hand"] != "" {
		t.Errorf("silver-hand folder = %q, want the root", folders["silver-hand"])
	}
	if folders["ashen-order"] != "factions" {
		t.Errorf("ashen-order folder = %q, want %q", folders["ashen-order"], "factions")
	}
}

func TestGetGraphIncludesNestedNotes(t *testing.T) {
	svc, gameID := newTestServiceWithGame(t)
	dir := filepath.Join(svc.resolver.GameDir(gameID), "entities")

	mustWriteEntityNote(t, filepath.Join(dir, "places", "port-vel.md"), "port-vel", "Port Vel")

	graph, err := svc.GetGraph(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}
	for _, node := range graph.Nodes {
		if node.ID == "port-vel" {
			return
		}
	}
	t.Fatalf("nested note missing from the graph: %+v", graph.Nodes)
}
```

Use the helper the file already uses to build a service and a game; if the names differ, keep the existing names and only add `mustWriteEntityNote`:

```go
func mustWriteEntityNote(t *testing.T, path, id, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	note := fmt.Sprintf("---\nid: %s\nname: %s\ntype: character\n---\n\nA note.\n", id, name)
	if err := os.WriteFile(path, []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/gui/ -run 'TestListEntitiesIncludesNestedNotes|TestGetGraphIncludesNestedNotes' -v`
Expected: FAIL, the nested note is absent.

- [ ] **Step 3: Add the shared walker**

In `pkg/gui/service.go`, add:

```go
// eachEntityNote walks a collection's entities/ tree in name order, calling fn
// with each note's path, its folder relative to the root, and its bytes. Every
// reader of the tree goes through this, so "what counts as a note" is decided
// once: markdown files, not hidden, not under assets/.
func eachEntityNote(entitiesDir string, fn func(path, folder string, data []byte) error) error {
	err := filepath.WalkDir(entitiesDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return fmt.Errorf("walk %q: %w", path, walkErr)
		}
		if entry.IsDir() {
			if path == entitiesDir {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "assets" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(entitiesDir, filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("relative path for %q: %w", path, err)
		}
		folder := ""
		if rel != "." && rel != "" {
			folder = filepath.ToSlash(rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil // a note that vanished mid-walk is not an error
		}
		return fn(path, folder, data)
	})
	if err != nil {
		return fmt.Errorf("walk entities dir: %w", err)
	}
	return nil
}
```

Add `"io/fs"` to the imports if it is not already there.

- [ ] **Step 4: Rewrite `ListEntities` on the walker**

Replace the body of `Service.ListEntities` so it walks, keeps the id from the frontmatter with the filename as the fallback, and reports the folder and the filename mismatch:

```go
func (s *Service) ListEntities(ctx context.Context, gameID string) ([]EntitySummaryDTO, error) {
	entitiesDir := filepath.Join(s.resolver.GameDir(gameID), "entities")

	summaries := make([]EntitySummaryDTO, 0)
	err := eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		filenameID := strings.TrimSuffix(filepath.Base(path), ".md")
		parsed, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			// A note that fails to parse is still a note the player wrote. Show it
			// so it can be repaired instead of silently vanishing.
			summaries = append(summaries, EntitySummaryDTO{
				ID:         filenameID,
				Name:       filenameID,
				Folder:     folder,
				ParseError: true,
			})
			return nil
		}

		id := parsed.ID
		if id == "" {
			id = filenameID
		}
		name := parsed.Name
		if name == "" {
			name = id
		}

		hasPortrait := parsed.Portrait != "" && s.hasCustomPortrait(gameID, id)
		portraitURL := ""
		if parsed.Type == "character" {
			portraitURL = fmt.Sprintf("/api/game/%s/character/%s/portrait", gameID, id)
		}

		summaries = append(summaries, EntitySummaryDTO{
			ID:               id,
			Name:             name,
			Type:             parsed.Type,
			Location:         parsed.Location,
			Tags:             parsed.Tags,
			Aliases:          parsed.Aliases,
			Folder:           folder,
			HasPortrait:      hasPortrait,
			PortraitURL:      portraitURL,
			FilenameMismatch: id != filenameID,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		return strings.ToLower(summaries[i].Name) < strings.ToLower(summaries[j].Name)
	})
	return summaries, nil
}
```

`Aliases`, `Folder` and `FilenameMismatch` do not exist on `EntitySummaryDTO` yet. Add them now, in `pkg/gui/types.go`, so this task compiles and its test can run:

```go
	Aliases          []string `json:"aliases,omitempty"`
	Folder           string   `json:"folder,omitempty"`
	FilenameMismatch bool     `json:"filename_mismatch,omitempty"`
```

- [ ] **Step 5: Rewrite `GetGraph` on the walker**

```go
func (s *Service) GetGraph(ctx context.Context, gameID string) (*GraphDTO, error) {
	entitiesDir := filepath.Join(s.resolver.GameDir(gameID), "entities")

	nodes := make([]GraphNodeDTO, 0)
	links := make([]GraphLinkDTO, 0)
	known := make(map[string]struct{})

	err := eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		ent, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			return nil
		}
		id := ent.ID
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(path), ".md")
		}
		known[id] = struct{}{}
		nodes = append(nodes, GraphNodeDTO{ID: id, Label: ent.Name, Type: ent.Type})
		for _, target := range ent.Wikilinks {
			links = append(links, GraphLinkDTO{Source: id, Target: target})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// A hand-written path-qualified link lands on the note whose id is its final
	// segment, so the graph and the mention resolver agree.
	for i := range links {
		if _, ok := known[links[i].Target]; ok {
			continue
		}
		if base := entity.WikilinkBasename(links[i].Target); base != links[i].Target {
			if _, ok := known[base]; ok {
				links[i].Target = base
			}
		}
	}

	return &GraphDTO{Nodes: nodes, Links: links}, nil
}
```

Also update `rewriteInboundLinks` to walk with `eachEntityNote` instead of `os.ReadDir`, so a link inside a nested note is rewritten too. Keep its existing regex and rewrite logic, only replacing the iteration.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./pkg/gui/ -run 'TestListEntities|TestGetGraph|TestMerge' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/gui/service.go pkg/gui/service_test.go
git commit -m "feat(gui): list and graph notes nested in folders"
```

---

### Task 5: Folder path validation and the tree

**Files:**
- Create: `pkg/gui/folders.go`
- Create: `pkg/gui/folders_test.go`
- Modify: `pkg/gui/types.go`

**Interfaces:**
- Produces: `func ValidateFolderPath(raw string) (string, error)`; `var ErrInvalidFolderPath error`; `func BuildFolderTree(paths []string) []FolderDTO`; `type FolderDTO struct{ Path, Name string; Children []FolderDTO }`

- [ ] **Step 1: Write the failing test**

Create `pkg/gui/folders_test.go`:

```go
package gui

import (
	"errors"
	"testing"
)

func TestValidateFolderPath(t *testing.T) {
	valid := map[string]string{
		"":                       "",
		"/":                      "",
		"factions":               "factions",
		"factions/orders":        "factions/orders",
		"Guilds & Orders":        "Guilds & Orders",
		"  factions/orders  ":    "factions/orders",
		"places/port-vel/taverns": "places/port-vel/taverns",
	}
	for input, want := range valid {
		got, err := ValidateFolderPath(input)
		if err != nil {
			t.Errorf("ValidateFolderPath(%q) errored: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ValidateFolderPath(%q) = %q, want %q", input, got, want)
		}
	}

	invalid := []string{
		"../escape",
		"factions/../../etc",
		"/absolute",
		"factions\\orders",
		"factions//orders",
		"factions/",
		".hidden",
		"factions/.hidden",
		"factions/./orders",
	}
	for _, input := range invalid {
		if _, err := ValidateFolderPath(input); !errors.Is(err, ErrInvalidFolderPath) {
			t.Errorf("ValidateFolderPath(%q) error = %v, want ErrInvalidFolderPath", input, err)
		}
	}

	long := ""
	for i := 0; i < 65; i++ {
		long += "a"
	}
	if _, err := ValidateFolderPath(long); !errors.Is(err, ErrInvalidFolderPath) {
		t.Errorf("an over-long segment must be rejected, got %v", err)
	}
}

func TestBuildFolderTree(t *testing.T) {
	tree := BuildFolderTree([]string{"factions/orders", "factions", "places", "factions/orders/inner"})
	if len(tree) != 2 {
		t.Fatalf("root children = %d, want 2", len(tree))
	}
	if tree[0].Path != "factions" || tree[1].Path != "places" {
		t.Fatalf("children not sorted: %q, %q", tree[0].Path, tree[1].Path)
	}
	if tree[0].Name != "factions" {
		t.Errorf("Name = %q, want the directory name", tree[0].Name)
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].Path != "factions/orders" {
		t.Fatalf("factions children = %+v, want one orders child", tree[0].Children)
	}
	if len(tree[0].Children[0].Children) != 1 {
		t.Fatalf("orders children = %+v, want one inner child", tree[0].Children[0].Children)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/gui/ -run 'TestValidateFolderPath|TestBuildFolderTree' -v`
Expected: FAIL, undefined.

- [ ] **Step 3: Implement the validator and the tree**

Create `pkg/gui/folders.go`:

```go
package gui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrInvalidFolderPath reports a folder path a client sent that could escape the
// entities directory or name something the walk would then skip.
var ErrInvalidFolderPath = errors.New("invalid folder path")

// maxFolderSegment bounds one directory name, so a path cannot be used to create
// a name no filesystem will accept.
const maxFolderSegment = 64

// ValidateFolderPath normalises and checks a folder path supplied by a client and
// returns its canonical form: forward slashes, no leading or trailing slash, and
// "" for the root. Nothing reaches the filesystem until this has passed.
func ValidateFolderPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "/" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, "/") || strings.Contains(trimmed, "\\") || strings.ContainsRune(trimmed, 0) {
		return "", fmt.Errorf("%w: %q", ErrInvalidFolderPath, raw)
	}
	if strings.HasSuffix(trimmed, "/") {
		return "", fmt.Errorf("%w: trailing slash in %q", ErrInvalidFolderPath, raw)
	}

	segments := strings.Split(trimmed, "/")
	for _, segment := range segments {
		switch {
		case segment == "":
			return "", fmt.Errorf("%w: empty segment in %q", ErrInvalidFolderPath, raw)
		case segment == "." || segment == "..":
			return "", fmt.Errorf("%w: relative segment %q", ErrInvalidFolderPath, segment)
		case strings.HasPrefix(segment, "."):
			return "", fmt.Errorf("%w: hidden segment %q", ErrInvalidFolderPath, segment)
		case len(segment) > maxFolderSegment:
			return "", fmt.Errorf("%w: segment longer than %d characters", ErrInvalidFolderPath, maxFolderSegment)
		}
	}
	return strings.Join(segments, "/"), nil
}

// FolderDTO is one directory in a collection's entities tree. Folders are pure
// directories: the name is the label and there is no metadata file.
type FolderDTO struct {
	Path     string      `json:"path"`
	Name     string      `json:"name"`
	Children []FolderDTO `json:"children,omitempty"`
}

// BuildFolderTree turns a flat list of folder paths into a sorted tree. Every
// ancestor of a listed path is present, so an empty parent is never hidden by a
// child that exists.
func BuildFolderTree(paths []string) []FolderDTO {
	type node struct {
		name     string
		path     string
		children map[string]*node
	}

	root := &node{children: map[string]*node{}}
	for _, path := range paths {
		if path == "" {
			continue
		}
		current := root
		prefix := ""
		for _, segment := range strings.Split(path, "/") {
			if prefix == "" {
				prefix = segment
			} else {
				prefix = prefix + "/" + segment
			}
			child, ok := current.children[segment]
			if !ok {
				child = &node{name: segment, path: prefix, children: map[string]*node{}}
				current.children[segment] = child
			}
			current = child
		}
	}

	var build func(*node) []FolderDTO
	build = func(n *node) []FolderDTO {
		names := make([]string, 0, len(n.children))
		for name := range n.children {
			names = append(names, name)
		}
		sort.Strings(names)

		out := make([]FolderDTO, 0, len(names))
		for _, name := range names {
			child := n.children[name]
			out = append(out, FolderDTO{
				Path:     child.path,
				Name:     child.name,
				Children: build(child),
			})
		}
		return out
	}
	return build(root)
}
```

- [ ] **Step 4: Extend the DTOs**

`EntitySummaryDTO` gained `Aliases`, `Folder` and `FilenameMismatch` in Task 4. Add the folder to `EntityDTO` in `pkg/gui/types.go`:

```go
	Folder string `json:"folder,omitempty"`
```

Add the folder request bodies:

```go
// FolderRequestDTO creates or moves a folder. Path is the target; From is only
// set when moving.
type FolderRequestDTO struct {
	Path string `json:"path"`
	From string `json:"from,omitempty"`
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/gui/ -run 'TestValidateFolderPath|TestBuildFolderTree' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/folders.go pkg/gui/folders_test.go pkg/gui/types.go
git commit -m "feat(gui): validate folder paths and build the tree"
```

---

### Task 6: Folder CRUD, service and routes

**Files:**
- Modify: `pkg/gui/service.go`, `pkg/gui/server.go`
- Test: `pkg/gui/service_test.go`

**Interfaces:**
- Consumes: `ValidateFolderPath`, `BuildFolderTree` (Task 5), `eachEntityNote` (Task 4)
- Produces: `Service.ListFolders(entitiesDir string) ([]FolderDTO, error)`; `Service.CreateFolder(entitiesDir, path string) error`; `Service.MoveFolder(entitiesDir, from, to string) error`; `Service.DeleteFolder(entitiesDir, path string, recursive bool) error`; `Service.GameFolders/WorldFolders` wrappers; routes `GET|POST|PUT|DELETE /api/game/{id}/folders` and `/api/world/{id}/folders`

- [ ] **Step 1: Write the failing test**

Append to `pkg/gui/service_test.go`:

```go
func TestFolderCRUD(t *testing.T) {
	svc, gameID := newTestServiceWithGame(t)
	entitiesDir := filepath.Join(svc.resolver.GameDir(gameID), "entities")
	if err := os.MkdirAll(entitiesDir, 0o755); err != nil {
		t.Fatalf("mkdir entities: %v", err)
	}

	if err := svc.CreateFolder(entitiesDir, "factions/orders"); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	tree, err := svc.ListFolders(entitiesDir)
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(tree) != 1 || tree[0].Path != "factions" {
		t.Fatalf("tree = %+v, want one factions root", tree)
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].Path != "factions/orders" {
		t.Fatalf("children = %+v, want factions/orders", tree[0].Children)
	}

	if err := svc.MoveFolder(entitiesDir, "factions/orders", "factions/ashen-order"); err != nil {
		t.Fatalf("MoveFolder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(entitiesDir, "factions", "ashen-order")); err != nil {
		t.Fatalf("moved folder missing: %v", err)
	}

	if err := svc.DeleteFolder(entitiesDir, "factions", false); err == nil {
		t.Fatal("deleting a non-empty folder must be refused without recursive")
	}
	if err := svc.DeleteFolder(entitiesDir, "factions", true); err != nil {
		t.Fatalf("DeleteFolder recursive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(entitiesDir, "factions")); !os.IsNotExist(err) {
		t.Fatalf("factions still on disk: %v", err)
	}
}

func TestFolderCRUDRejectsTraversal(t *testing.T) {
	svc, gameID := newTestServiceWithGame(t)
	entitiesDir := filepath.Join(svc.resolver.GameDir(gameID), "entities")
	if err := os.MkdirAll(entitiesDir, 0o755); err != nil {
		t.Fatalf("mkdir entities: %v", err)
	}
	if err := svc.CreateFolder(entitiesDir, "../escape"); err == nil {
		t.Fatal("a traversing folder path must be rejected")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestFolderCRUD -v`
Expected: FAIL, undefined.

- [ ] **Step 3: Implement the folder operations**

Append to `pkg/gui/service.go`:

```go
// ListFolders returns the folder tree under an entities directory. Folders are
// read from disk rather than from the index, so a folder with no notes in it is
// still visible.
func (s *Service) ListFolders(entitiesDir string) ([]FolderDTO, error) {
	paths := make([]string, 0)
	err := filepath.WalkDir(entitiesDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return fmt.Errorf("walk %q: %w", path, walkErr)
		}
		if !entry.IsDir() || path == entitiesDir {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "assets" {
			return fs.SkipDir
		}
		rel, err := filepath.Rel(entitiesDir, path)
		if err != nil {
			return fmt.Errorf("relative path for %q: %w", path, err)
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return BuildFolderTree(paths), nil
}

// CreateFolder creates a folder and any missing parents.
func (s *Service) CreateFolder(entitiesDir, path string) error {
	clean, err := ValidateFolderPath(path)
	if err != nil {
		return err
	}
	if clean == "" {
		return fmt.Errorf("%w: a folder needs a name", ErrInvalidFolderPath)
	}
	return os.MkdirAll(filepath.Join(entitiesDir, filepath.FromSlash(clean)), 0o755)
}

// MoveFolder renames a folder, taking its notes with it. Links are untouched,
// because a note is linked by id and not by path.
func (s *Service) MoveFolder(entitiesDir, from, to string) error {
	cleanFrom, err := ValidateFolderPath(from)
	if err != nil {
		return err
	}
	cleanTo, err := ValidateFolderPath(to)
	if err != nil {
		return err
	}
	if cleanFrom == "" || cleanTo == "" {
		return fmt.Errorf("%w: cannot move the entities root", ErrInvalidFolderPath)
	}
	if _, err := os.Stat(filepath.Join(entitiesDir, filepath.FromSlash(cleanFrom))); err != nil {
		return fmt.Errorf("move folder %q: %w", cleanFrom, err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(entitiesDir, filepath.FromSlash(cleanTo))), 0o755); err != nil {
		return fmt.Errorf("create parent of %q: %w", cleanTo, err)
	}
	return os.Rename(
		filepath.Join(entitiesDir, filepath.FromSlash(cleanFrom)),
		filepath.Join(entitiesDir, filepath.FromSlash(cleanTo)),
	)
}

// DeleteFolder removes a folder. A folder with notes in it is refused unless
// recursive is set, so a stray click cannot delete a campaign's lore.
func (s *Service) DeleteFolder(entitiesDir, path string, recursive bool) error {
	clean, err := ValidateFolderPath(path)
	if err != nil {
		return err
	}
	if clean == "" {
		return fmt.Errorf("%w: cannot delete the entities root", ErrInvalidFolderPath)
	}
	target := filepath.Join(entitiesDir, filepath.FromSlash(clean))

	empty, err := folderIsEmpty(target)
	if err != nil {
		return err
	}
	if !empty && !recursive {
		return fmt.Errorf("folder %q is not empty; pass recursive=true to delete its notes", clean)
	}
	return os.RemoveAll(target)
}

// folderIsEmpty reports whether a directory holds anything, so a non-recursive
// delete can refuse.
func folderIsEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read folder %q: %w", dir, err)
	}
	return len(entries) == 0, nil
}

// GameFolders lists a campaign's folder tree.
func (s *Service) GameFolders(gameID string) ([]FolderDTO, error) {
	return s.ListFolders(filepath.Join(s.resolver.GameDir(gameID), "entities"))
}

// WorldFolders lists a world's folder tree.
func (s *Service) WorldFolders(worldID string) ([]FolderDTO, error) {
	return s.ListFolders(filepath.Join(s.resolver.WorldDir(worldID), "entities"))
}
```

- [ ] **Step 4: Add the routes**

In `pkg/gui/server.go`, inside `handleGameRoutes`'s `switch action`, add a case:

```go
	case "folders":
		s.handleFolderRoutes(w, r, filepath.Join(s.service.resolver.GameDir(gameID), "entities"))
		return
```

and in `handleWorldRoutes`, the same case with `filepath.Join(s.service.resolver.WorldDir(worldID), "entities")`. Use the existing world-id variable name in that function.

Add the shared handler:

```go
// handleFolderRoutes serves the folder CRUD a tree UI needs. It takes the
// entities directory rather than a collection id, because the game and world
// trees are the same shape.
func (s *Server) handleFolderRoutes(w http.ResponseWriter, r *http.Request, entitiesDir string) {
	switch r.Method {
	case http.MethodGet:
		tree, err := s.service.ListFolders(entitiesDir)
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, tree)

	case http.MethodPost:
		var body FolderRequestDTO
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := s.service.CreateFolder(entitiesDir, body.Path); err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, map[string]string{"path": strings.TrimSpace(body.Path)})

	case http.MethodPut:
		var body FolderRequestDTO
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := s.service.MoveFolder(entitiesDir, body.From, body.Path); err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, map[string]string{"path": strings.TrimSpace(body.Path)})

	case http.MethodDelete:
		recursive := r.URL.Query().Get("recursive") == "true"
		if err := s.service.DeleteFolder(entitiesDir, r.URL.Query().Get("path"), recursive); err != nil {
			writeGameError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
```

Map the validation error to a `400` in `writeGameError`:

```go
	case errors.Is(err, ErrInvalidFolderPath):
		http.Error(w, err.Error(), http.StatusBadRequest)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/gui/ -run 'TestFolder' -v && go vet ./pkg/gui/`
Expected: PASS, vet clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/service.go pkg/gui/server.go pkg/gui/service_test.go
git commit -m "feat(gui): add folder create, move and delete routes"
```

---

### Task 7: Moving a note, and refusing a duplicate id

**Files:**
- Modify: `pkg/gui/service.go`, `pkg/gui/server.go`
- Test: `pkg/gui/service_test.go`

**Interfaces:**
- Consumes: `eachEntityNote` (Task 4), `ValidateFolderPath` (Task 5), `Syncer.SyncFile(root, path)` (Task 3)
- Produces: `func (s *Service) SaveEntityInFolder(ctx, gameID, entityID, folder, markdown string) error`; `var ErrDuplicateEntityID error`; `409` on collision; `EntityDTO.Folder` populated by `GetEntity`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/gui/service_test.go`:

```go
func TestSaveEntityMovesBetweenFolders(t *testing.T) {
	svc, gameID := newTestServiceWithGame(t)
	ctx := context.Background()

	note := "---\nid: silver-hand\nname: Silver Hand\ntype: faction\n---\n\nSee [[port-vel]].\n"
	if err := svc.SaveEntityInFolder(ctx, gameID, "silver-hand", "", note); err != nil {
		t.Fatalf("initial save: %v", err)
	}

	// A second note links to the first, and the move must not touch it.
	linker := "---\nid: port-vel\nname: Port Vel\ntype: location\n---\n\nHome of the [[silver-hand]].\n"
	if err := svc.SaveEntityInFolder(ctx, gameID, "port-vel", "", linker); err != nil {
		t.Fatalf("save linker: %v", err)
	}

	if err := svc.SaveEntityInFolder(ctx, gameID, "silver-hand", "factions/orders", note); err != nil {
		t.Fatalf("move: %v", err)
	}

	entitiesDir := filepath.Join(svc.resolver.GameDir(gameID), "entities")
	if _, err := os.Stat(filepath.Join(entitiesDir, "silver-hand.md")); !os.IsNotExist(err) {
		t.Fatalf("the old file is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(entitiesDir, "factions", "orders", "silver-hand.md")); err != nil {
		t.Fatalf("the moved file is missing: %v", err)
	}

	linkerBytes, err := os.ReadFile(filepath.Join(entitiesDir, "port-vel.md"))
	if err != nil {
		t.Fatalf("read linker: %v", err)
	}
	if !strings.Contains(string(linkerBytes), "[[silver-hand]]") {
		t.Fatalf("the move rewrote an inbound link, which it must not do:\n%s", linkerBytes)
	}

	got, err := svc.GetEntity(ctx, gameID, "silver-hand")
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if got.Folder != "factions/orders" {
		t.Errorf("Folder = %q, want %q", got.Folder, "factions/orders")
	}
}

func TestSaveEntityRejectsDuplicateID(t *testing.T) {
	svc, gameID := newTestServiceWithGame(t)
	ctx := context.Background()

	first := "---\nid: silver-hand\nname: Silver Hand\ntype: faction\n---\n\nFirst.\n"
	if err := svc.SaveEntityInFolder(ctx, gameID, "silver-hand", "", first); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// A different file claims an id that is already taken.
	second := "---\nid: silver-hand\nname: Impostor\ntype: faction\n---\n\nSecond.\n"
	err := svc.SaveEntityInFolder(ctx, gameID, "impostor", "", second)
	if !errors.Is(err, ErrDuplicateEntityID) {
		t.Fatalf("error = %v, want ErrDuplicateEntityID", err)
	}
	if !strings.Contains(err.Error(), "silver-hand") {
		t.Errorf("the error should name the id it collided with, got: %v", err)
	}

	// The original note is intact.
	got, err := svc.GetEntity(ctx, gameID, "silver-hand")
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if !strings.Contains(got.Markdown, "First.") {
		t.Errorf("the duplicate overwrote the original:\n%s", got.Markdown)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/gui/ -run 'TestSaveEntityMoves|TestSaveEntityRejectsDuplicate' -v`
Expected: FAIL, undefined.

- [ ] **Step 3: Implement the move and the guard**

In `pkg/gui/service.go`, replace `SaveEntity` with a thin wrapper and add the folder-aware implementation:

```go
// ErrDuplicateEntityID reports a save whose frontmatter id already belongs to a
// different note. The id is the identity, so the second claim is refused rather
// than silently overwriting a note in another folder.
var ErrDuplicateEntityID = errors.New("entity id already in use")

// SaveEntity writes a note at the collection root.
func (s *Service) SaveEntity(ctx context.Context, gameID, entityID, rawMarkdown string) error {
	return s.SaveEntityInFolder(ctx, gameID, entityID, "", rawMarkdown)
}

// SaveEntityInFolder writes a note at folder, moving it when it already lives
// somewhere else. The file name stays the id, because the id is the identity and
// the folder is only location: no inbound link is rewritten, which is the point
// of decoupling the two.
func (s *Service) SaveEntityInFolder(ctx context.Context, gameID, entityID, folder, rawMarkdown string) error {
	cleanFolder, err := ValidateFolderPath(folder)
	if err != nil {
		return err
	}

	ent, err := entity.ParseMarkdownEntity([]byte(rawMarkdown))
	if err != nil {
		return fmt.Errorf("save entity %q: %w", entityID, err)
	}
	// The file name is the note's identity, so a hand-edited or copied id can
	// never index a note under another note's key.
	ent.ID = entityID
	ent.Folder = cleanFolder

	gameDir := s.resolver.GameDir(gameID)
	entitiesDir := filepath.Join(gameDir, "entities")

	existingPath, err := s.findEntityNote(entitiesDir, entityID)
	if err != nil {
		return err
	}

	targetDir := entitiesDir
	if cleanFolder != "" {
		targetDir = filepath.Join(entitiesDir, filepath.FromSlash(cleanFolder))
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create folder %q: %w", cleanFolder, err)
	}

	if err := s.assertIDIsFree(entitiesDir, entityID, rawMarkdown); err != nil {
		return err
	}

	normalised, err := ent.SerializeMarkdown()
	if err != nil {
		return fmt.Errorf("normalise entity %q: %w", entityID, err)
	}

	targetPath := filepath.Join(targetDir, entityID+".md")
	if err := os.WriteFile(targetPath, normalised, 0o644); err != nil {
		return fmt.Errorf("write entity file: %w", err)
	}
	if existingPath != "" && existingPath != targetPath {
		if err := os.Remove(existingPath); err != nil {
			return fmt.Errorf("remove old note %q: %w", existingPath, err)
		}
	}

	store, err := s.store(gameID)
	if err != nil {
		return nil // Non-fatal if db sync fails temporarily
	}
	return storage.NewSyncer(store).SyncFile(targetPath)
}

// findEntityNote returns the path a note currently occupies, or "" when it is new.
func (s *Service) findEntityNote(entitiesDir, entityID string) (string, error) {
	found := ""
	err := eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		parsed, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			if strings.TrimSuffix(filepath.Base(path), ".md") == entityID {
				found = path
			}
			return nil
		}
		id := parsed.ID
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(path), ".md")
		}
		if id == entityID {
			found = path
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return found, nil
}

// assertIDIsFree refuses a save whose frontmatter id already names another note.
// The file name is the requested id, so a note whose body declares a different
// id is a collision with that other note.
func (s *Service) assertIDIsFree(entitiesDir, entityID, rawMarkdown string) error {
	declared := entity.DeclaredID([]byte(rawMarkdown))
	if declared == "" || declared == entityID {
		return nil
	}
	if _, err := os.Stat(filepath.Join(entitiesDir, declared+".md")); err == nil {
		return fmt.Errorf("%w: %q; use a different id", ErrDuplicateEntityID, declared)
	}
	existing, err := s.findEntityNote(entitiesDir, declared)
	if err == nil && existing != "" {
		return fmt.Errorf("%w: %q; use a different id", ErrDuplicateEntityID, declared)
	}
	return nil
}
```

Add `DeclaredID` to `pkg/entity/entity.go` so the collision check does not re-implement frontmatter parsing:

```go
// DeclaredID returns the id a document's frontmatter declares, or "" when it has
// none. It exists so a writer can detect a collision without parsing the whole
// note.
func DeclaredID(data []byte) string {
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return ""
	}
	endIdx := strings.Index(content[4:], "\n---\n")
	if endIdx == -1 {
		return ""
	}
	var fm struct {
		ID string `yaml:"id"`
	}
	if err := yaml.Unmarshal([]byte(content[4:4+endIdx]), &fm); err != nil {
		return ""
	}
	return strings.TrimSpace(fm.ID)
}
```

Add a test for it in `pkg/entity/entity_test.go`:

```go
func TestDeclaredID(t *testing.T) {
	cases := map[string]string{
		"---\nid: silver-hand\nname: X\n---\n\nbody\n": "silver-hand",
		"---\nname: X\n---\n\nbody\n":                 "",
		"no frontmatter at all":                       "",
		"---\nid: broken\n":                           "",
	}
	for input, want := range cases {
		if got := DeclaredID([]byte(input)); got != want {
			t.Errorf("DeclaredID(%q) = %q, want %q", input, got, want)
		}
	}
}
```

- [ ] **Step 4: Populate `Folder` in `GetEntity` and add the folder to the save route**

In `Service.GetEntity`, set `Folder` on the DTO from the note's location. Use `findEntityNote` and derive the folder from the path relative to `entities/`:

```go
	entitiesDir := filepath.Join(s.resolver.GameDir(gameID), "entities")
	notePath, err := s.findEntityNote(entitiesDir, entityID)
	if err != nil {
		return nil, err
	}
	if notePath != "" {
		if rel, err := filepath.Rel(entitiesDir, filepath.Dir(notePath)); err == nil && rel != "." {
			dto.Folder = filepath.ToSlash(rel)
		}
	}
```

In `pkg/gui/server.go`, the `PUT` entity handler accepts a folder:

```go
		if r.Method == http.MethodPut {
			var body struct {
				Markdown string `json:"markdown"`
				Folder   string `json:"folder"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if err := s.service.SaveEntityInFolder(r.Context(), gameID, entityID, body.Folder, body.Markdown); err != nil {
				writeGameError(w, err)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
```

Map the collision in `writeGameError`:

```go
	case errors.Is(err, ErrDuplicateEntityID):
		http.Error(w, err.Error(), http.StatusConflict)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./... && go vet ./...`
Expected: PASS, vet clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/service.go pkg/gui/server.go pkg/gui/service_test.go pkg/entity/entity.go pkg/entity/entity_test.go
git commit -m "feat(gui): move a note between folders without breaking links"
```

---

### Task 8: The frontend types and client

**Files:**
- Modify: `frontend/src/types.ts`, `frontend/src/api/client.ts`

**Interfaces:**
- Consumes: the routes from Task 6 and the `folder` field from Task 7
- Produces: `EntitySummary.folder`, `EntitySummary.aliases`, `EntitySummary.filename_mismatch`, `EntityNote.folder`, `FolderNode`; `APIClient.listFolders`, `createFolder`, `moveFolder`, `deleteFolder`, and `saveEntity(id, markdown, folder?)`

- [ ] **Step 1: Extend the types**

In `frontend/src/types.ts`, replace `EntitySummary` and `EntityNote` and add `FolderNode`:

```ts
export interface EntitySummary {
  id: string;
  name: string;
  type: string;
  location?: string;
  tags?: string[];
  aliases?: string[];
  folder?: string;
  filename_mismatch?: boolean;
  parse_error?: boolean;
  has_portrait?: boolean;
  portrait_url?: string;
}

export interface EntityNote {
  id: string;
  name: string;
  type: string;
  markdown: string;
  state: Record<string, any>;
  backlinks: string[];
  history?: number[];
  folder?: string;
  parse_error?: boolean;
}

export interface FolderNode {
  path: string;
  name: string;
  children?: FolderNode[];
}
```

- [ ] **Step 2: Add the client methods**

In `frontend/src/api/client.ts`, beside the existing entity methods:

```ts
  async saveEntity(entityID: string, markdown: string, folder?: string): Promise<void> {
    const res = await fetch(
      `/api/game/${this.gameID}/entity/${entityID}`,
      {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ markdown, folder: folder ?? '' }),
      },
    );
    if (!res.ok) {
      // The server names the line for a frontmatter parse failure; surface it
      // rather than a bare status, so the author can fix it.
      throw new Error((await res.text()).trim() || `saveEntity: ${res.statusText}`);
    }
  }

  async listFolders(): Promise<FolderNode[]> {
    const res = await fetch(`/api/game/${this.gameID}/folders`);
    if (!res.ok) throw new Error(`listFolders: ${res.statusText}`);
    return res.json();
  }

  async createFolder(path: string): Promise<void> {
    const res = await fetch(`/api/game/${this.gameID}/folders`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    });
    if (!res.ok) throw new Error((await res.text()).trim() || `createFolder: ${res.statusText}`);
  }

  async moveFolder(from: string, to: string): Promise<void> {
    const res = await fetch(`/api/game/${this.gameID}/folders`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ from, path: to }),
    });
    if (!res.ok) throw new Error((await res.text()).trim() || `moveFolder: ${res.statusText}`);
  }

  async deleteFolder(path: string, recursive = false): Promise<void> {
    const params = new URLSearchParams({ path, recursive: String(recursive) });
    const res = await fetch(
      `/api/game/${this.gameID}/folders?${params.toString()}`,
      { method: 'DELETE' },
    );
    if (!res.ok) throw new Error((await res.text()).trim() || `deleteFolder: ${res.statusText}`);
  }
```

Match the existing class's field name for the campaign id (it is `gameID` in the current methods; keep whatever that file uses). Add `FolderNode` to the type import at the top.

- [ ] **Step 3: Verify the type gate**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): carry folders in the entity API"
```

---

### Task 9: The `EntityTree` component

**Files:**
- Create: `frontend/src/components/EntityTree.tsx`

**Interfaces:**
- Consumes: `FolderNode`, `EntitySummary` (Task 8)
- Produces: `<EntityTree>` with props `{ folders, entities, selectedId, onSelect, onMoveEntity, onMoveFolder, onCreateFolder, onDeleteFolder }`

- [ ] **Step 1: Write the pure helpers and their component**

Create `frontend/src/components/EntityTree.tsx`:

```tsx
import { useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, FileText, Folder, FolderPlus, Trash2 } from 'lucide-react';
import type { EntitySummary, FolderNode } from '../types';

interface EntityTreeProps {
  folders: FolderNode[];
  entities: EntitySummary[];
  selectedId?: string;
  onSelect: (entityId: string) => void;
  onMoveEntity: (entityId: string, folder: string) => void;
  onMoveFolder: (from: string, to: string) => void;
  onCreateFolder: (path: string) => void;
  onDeleteFolder: (path: string) => void;
}

// groupByFolder is pure so the shape of the tree is reviewable without a browser.
export function groupByFolder(entities: EntitySummary[]): Map<string, EntitySummary[]> {
  const byFolder = new Map<string, EntitySummary[]>();
  for (const entity of entities) {
    const folder = entity.folder ?? '';
    const bucket = byFolder.get(folder);
    if (bucket) {
      bucket.push(entity);
    } else {
      byFolder.set(folder, [entity]);
    }
  }
  for (const bucket of byFolder.values()) {
    bucket.sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }));
  }
  return byFolder;
}

// noteIdFor resolves a wikilink target to a note id, falling back to the final
// path segment so a hand-written [[guilds/silver-hand]] still lands.
export function noteIdFor(target: string, known: Set<string>): string | undefined {
  const trimmed = target.trim();
  if (known.has(trimmed)) return trimmed;
  const base = trimmed.slice(trimmed.lastIndexOf('/') + 1).trim();
  return known.has(base) ? base : undefined;
}

export default function EntityTree({
  folders,
  entities,
  selectedId,
  onSelect,
  onMoveEntity,
  onMoveFolder,
  onCreateFolder,
  onDeleteFolder,
}: EntityTreeProps) {
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [filter, setFilter] = useState('');
  const [dragEntity, setDragEntity] = useState<string | null>(null);
  const [dragFolder, setDragFolder] = useState<string | null>(null);

  const byFolder = useMemo(() => groupByFolder(entities), [entities]);
  const needle = filter.trim().toLowerCase();
  const matches = (entity: EntitySummary) =>
    needle === '' ||
    entity.name.toLowerCase().includes(needle) ||
    entity.id.toLowerCase().includes(needle) ||
    (entity.aliases ?? []).some((alias) => alias.toLowerCase().includes(needle));

  const toggle = (path: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });

  const renderNotes = (folder: string) =>
    (byFolder.get(folder) ?? [])
      .filter(matches)
      .map((entity) => (
        <button
          key={entity.id}
          draggable
          onDragStart={() => setDragEntity(entity.id)}
          onDragEnd={() => setDragEntity(null)}
          onClick={() => onSelect(entity.id)}
          className={`w-full text-left px-2 py-1.5 rounded-lg text-xs transition-colors cursor-pointer ${
            selectedId === entity.id
              ? 'bg-purple-600/20 border border-purple-500/40 text-purple-100'
              : 'border border-transparent hover:bg-black/30 text-stone-200'
          }`}
        >
          <span className="flex items-center gap-1.5 truncate">
            <FileText className="w-3 h-3 shrink-0 text-stone-500" />
            <span className="truncate">{entity.name}</span>
          </span>
          {entity.filename_mismatch && (
            <span className="block pl-4 text-[10px] text-amber-400">
              file name differs from id
            </span>
          )}
        </button>
      ));

  const renderFolder = (node: FolderNode, depth: number) => {
    const isCollapsed = collapsed.has(node.path);
    return (
      <div key={node.path} style={{ paddingLeft: depth * 8 }}>
        <div
          className="flex items-center gap-1 rounded-lg px-1 py-1 hover:bg-black/30"
          onDragOver={(ev) => ev.preventDefault()}
          onDrop={() => {
            if (dragEntity) onMoveEntity(dragEntity, node.path);
            if (dragFolder && dragFolder !== node.path) onMoveFolder(dragFolder, `${node.path}/${dragFolder.split('/').pop()}`);
            setDragEntity(null);
            setDragFolder(null);
          }}
        >
          <button onClick={() => toggle(node.path)} className="cursor-pointer text-stone-400">
            {isCollapsed ? <ChevronRight className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />}
          </button>
          <Folder className="w-3.5 h-3.5 text-purple-400/70" />
          <span
            draggable
            onDragStart={() => setDragFolder(node.path)}
            onDragEnd={() => setDragFolder(null)}
            className="flex-1 text-xs text-stone-300 truncate cursor-grab"
          >
            {node.name}
          </span>
          <button
            onClick={() => onDeleteFolder(node.path)}
            title="Delete folder"
            className="cursor-pointer text-stone-500 hover:text-red-400"
          >
            <Trash2 className="w-3 h-3" />
          </button>
        </div>
        {!isCollapsed && (
          <div>
            {renderNotes(node.path)}
            {(node.children ?? []).map((child) => renderFolder(child, depth + 1))}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="flex flex-col gap-2 min-h-0">
      <div className="flex items-center gap-1.5">
        <input
          value={filter}
          onChange={(ev) => setFilter(ev.target.value)}
          placeholder="Filter notes..."
          className="flex-1 bg-black/50 border border-white/10 rounded-lg px-2 py-1.5 text-xs text-stone-200 placeholder-stone-500 focus:outline-none focus:border-purple-500/60"
        />
        <button
          onClick={() => {
            const name = window.prompt('New folder name');
            if (name) onCreateFolder(name.trim());
          }}
          title="New folder"
          className="cursor-pointer rounded-lg border border-white/10 bg-black/40 p-1.5 text-stone-300 hover:text-purple-300"
        >
          <FolderPlus className="w-3.5 h-3.5" />
        </button>
      </div>

      <div
        className="flex-1 min-h-0 overflow-y-auto space-y-0.5 pr-1"
        onDragOver={(ev) => ev.preventDefault()}
        onDrop={() => {
          if (dragEntity) onMoveEntity(dragEntity, '');
          setDragEntity(null);
          setDragFolder(null);
        }}
      >
        {renderNotes('')}
        {folders.map((node) => renderFolder(node, 0))}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Verify the type gate**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors. `noUnusedLocals` will catch an unused import, so remove any you did not use.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/EntityTree.tsx
git commit -m "feat(frontend): add a folder tree for entity notes"
```

---

### Task 10: Wire the tree into the codex and the world studio

**Files:**
- Modify: `frontend/src/components/CodexDrawer.tsx`, `frontend/src/components/WorldsStudio.tsx`

**Interfaces:**
- Consumes: `EntityTree` (Task 9), `APIClient.listFolders/createFolder/moveFolder/deleteFolder` and `saveEntity(id, markdown, folder)` (Task 8)
- Produces: the note list in the codex is the tree; the world template list is the tree.

- [ ] **Step 1: Load folders and render the tree in the codex**

In `CodexDrawer.tsx`, replace the `filtered.map(...)` list (`CodexDrawer.tsx:261-283`) with the tree. Add beside the existing entity load:

```tsx
  const [folders, setFolders] = useState<FolderNode[]>([]);

  useEffect(() => {
    let cancelled = false;
    APIClient.listFolders()
      .then((tree) => {
        if (!cancelled) setFolders(tree);
      })
      .catch(() => {
        // A campaign with no folders yet is not an error.
      });
    return () => {
      cancelled = true;
    };
  }, [gameID]);
```

Render in place of the flat list:

```tsx
              <EntityTree
                folders={folders}
                entities={entities ?? []}
                selectedId={entity?.id}
                onSelect={onSelect}
                onMoveEntity={async (id, folder) => {
                  const note = await APIClient.getEntity(id);
                  await APIClient.saveEntity(id, note.markdown, folder);
                  await onReload?.();
                }}
                onMoveFolder={async (from, to) => {
                  await APIClient.moveFolder(from, to);
                  setFolders(await APIClient.listFolders());
                }}
                onCreateFolder={async (path) => {
                  await APIClient.createFolder(path);
                  setFolders(await APIClient.listFolders());
                }}
                onDeleteFolder={async (path) => {
                  await APIClient.deleteFolder(path, false);
                  setFolders(await APIClient.listFolders());
                }}
              />
```

Use the component's existing reload callback name in place of `onReload`; if the drawer reloads entities through a prop or a local `load()` function, call that instead.

- [ ] **Step 2: Do the same for the world template list**

In `WorldsStudio.tsx`, replace the `entities.map(...)` list (`WorldsStudio.tsx:1016-1036`) with `EntityTree`, loading the world's folders from the same client methods, and saving a moved template with its folder so the world entity file moves too.

- [ ] **Step 3: Verify the build**

Run: `cd frontend && npm run build`
Expected: `tsc` clean and vite writes the bundle. The `build` script touches `pkg/gui/dist/.gitkeep`, so `git status` stays clean.

- [ ] **Step 4: Run the whole suite**

Run: `mise run test`
Expected: Go tests pass and `tsc --noEmit` is clean.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/CodexDrawer.tsx frontend/src/components/WorldsStudio.tsx
git commit -m "feat(frontend): organise codex and world notes into folders"
```

---

## Self-Review

**Spec coverage.** §2.1 model and invariants → Tasks 1, 7 (stable id, duplicate refusal), 4 (filename mismatch). §2.2 wikilinks → Task 1 (`WikilinkBasename`) and Task 4 (applied in `GetGraph`). §2.3 storage and index → Tasks 2, 3. §2.4 API surface → Tasks 5, 6, 7, 8. §2.5 UI surface → Tasks 9, 10. §3 migration → Task 2 (migration 12, root notes default to `''`). §6 testing → each task's tests.

**Placeholder scan.** No `TBD`, no "add error handling", no "similar to Task N". Two steps deliberately say "use the existing helper name" where a test helper's name cannot be known without opening the file; each names exactly what to do if the name differs.

**Type consistency.** `Entity.Folder` (Task 1) is the only folder carrier; `folderFromPath` (Task 3) and `eachEntityNote`'s `folder` (Task 4) both produce the same canonical form. `ValidateFolderPath` (Task 5) is the single validator, used by `CreateFolder`, `MoveFolder`, `DeleteFolder` (Task 6) and `SaveEntityInFolder` (Task 7). `ErrDuplicateEntityID` is declared in Task 7 and mapped in Task 7's `writeGameError` change. `FolderNode` (Task 8) matches `FolderDTO`'s JSON (`path`, `name`, `children`).

## Execution Notes

Recorded after the plan was executed, so the plan matches what shipped.

- **Task 3** kept `SyncFile(path string)` rather than adding a `root` argument. The
  plan assumed one caller; there are eight, and every one passes a path under a
  directory named `entities`, so `entitiesRootFor` derives the root and removes
  eight chances to pass the wrong one.
- **Task 4** moved the `EntitySummaryDTO` field additions forward from Task 5, so
  the task compiles and its test runs on its own.
- **Task 10** needed backend work the File Map did not list. World templates are a
  separate store from campaign notes, so nesting them required
  `WorldEntitySummaryDTO.Folder`, a recursive `GetWorld`, a `folder` parameter on
  `SaveWorldEntity`, `findWorldEntityNote`, and world folder client methods. It
  also kept the world studio's per-template delete, which the tree does not offer,
  by moving it into the editor header.
- **World template identity stays the file name**, unlike a campaign note. A world
  template is not indexed and is not linked by id, so taking the id from the
  frontmatter would rename every existing template the first time it was saved.
  `findWorldEntityNote` still matches either, so a hand-edited template is
  reachable.
