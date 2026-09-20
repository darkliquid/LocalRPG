# LocalRPG Core Engine & Data Storage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the foundational headless core engine for LocalRPG: the three-tier directory structure (Systems, Worlds, Games), Markdown entity parser with wikilink extraction, schema-agnostic state engine, pure Go SQLite graph indexer, and game composition loader.

**Architecture:** A decoupled, pure Go module with zero CGO dependencies (using `modernc.org/sqlite`). Entities are stored as human-readable Markdown files with YAML frontmatter on disk (source of truth) and synchronized into a local SQLite database for graph traversals and rapid querying.

**Tech Stack:** Go 1.27, `modernc.org/sqlite`, `gopkg.in/yaml.v3`.

---

### File Structure Map

```text
LocalRPG/
├── go.mod
├── go.sum
├── cmd/
│   └── localrpg/
│       ├── main.go               # Entry point CLI (flags, subcommands)
│       └── main_test.go          # CLI integration tests
├── pkg/
│   ├── core/
│   │   ├── types.go              # Manifest structs (System, World, Game) & paths
│   │   └── types_test.go         # Manifest parsing & path resolution tests
│   ├── state/
│   │   ├── state.go              # Schema-agnostic dot-path getter/setter
│   │   └── state_test.go         # Property traversal and mutation tests
│   ├── entity/
│   │   ├── entity.go             # Markdown frontmatter & wikilink parser
│   │   └── entity_test.go        # Entity serialization/deserialization tests
│   ├── storage/
│   │   ├── db.go                 # SQLite schema migration & connection
│   │   ├── store.go              # Entity & graph edge CRUD operations
│   │   ├── sync.go               # Filesystem-to-SQLite incremental indexer
│   │   └── store_test.go         # Storage and sync tests
│   └── engine/
│       ├── game.go               # Game session loader & 3-tier composition
│       └── game_test.go          # Session lifecycle & override merge tests
```

---

### Task 1: Go Module Initialization & Core Manifest Types

**Files:**
- Create: `go.mod`
- Create: `pkg/core/types.go`
- Test: `pkg/core/types_test.go`

- [ ] **Step 1: Write the failing test for Core Manifests**

```go
// pkg/core/types_test.go
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifests(t *testing.T) {
	tempDir := t.TempDir()

	systemYAML := `
id: "d20-classic"
name: "D20 Classic"
version: "1.0.0"
`
	systemPath := filepath.Join(tempDir, "system.yaml")
	if err := os.WriteFile(systemPath, []byte(systemYAML), 0644); err != nil {
		t.Fatalf("failed to write system.yaml: %v", err)
	}

	sys, err := LoadSystemManifest(systemPath)
	if err != nil {
		t.Fatalf("LoadSystemManifest failed: %v", err)
	}
	if sys.ID != "d20-classic" || sys.Name != "D20 Classic" || sys.Version != "1.0.0" {
		t.Errorf("unexpected system manifest: %+v", sys)
	}
}

func TestResolvePaths(t *testing.T) {
	baseDir := "/test/rpg"
	paths := NewPathResolver(baseDir)

	if paths.SystemDir("d20-classic") != "/test/rpg/systems/d20-classic" {
		t.Errorf("unexpected system dir: %s", paths.SystemDir("d20-classic"))
	}
	if paths.WorldDir("forgotten-reach") != "/test/rpg/worlds/forgotten-reach" {
		t.Errorf("unexpected world dir: %s", paths.WorldDir("forgotten-reach"))
	}
	if paths.GameDir("campaign-01") != "/test/rpg/games/campaign-01" {
		t.Errorf("unexpected game dir: %s", paths.GameDir("campaign-01"))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/core/... -v`  
Expected: FAIL (package/types not defined)

- [ ] **Step 3: Implement go.mod and core manifest types**

Initialize `go.mod`:
```bash
go mod init github.com/darkliquid/localrpg
go get gopkg.in/yaml.v3
```

Write `pkg/core/types.go`:
```go
package core

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type SystemManifest struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description,omitempty"`
}

type WorldManifest struct {
	ID              string   `yaml:"id"`
	Name            string   `yaml:"name"`
	Genre           string   `yaml:"genre,omitempty"`
	DefaultSystem   string   `yaml:"default_system,omitempty"`
	ArtStyle        string   `yaml:"art_style,omitempty"`
	Tags            []string `yaml:"tags,omitempty"`
}

type GameManifest struct {
	ID       string                 `yaml:"id"`
	Name     string                 `yaml:"name"`
	SystemID string                 `yaml:"system"`
	WorldID  string                 `yaml:"world"`
	Player   string                 `yaml:"player"`
	Settings map[string]interface{} `yaml:"settings,omitempty"`
}

type PathResolver struct {
	BaseDir string
}

func NewPathResolver(baseDir string) *PathResolver {
	return &PathResolver{BaseDir: baseDir}
}

func (p *PathResolver) SystemsDir() string {
	return filepath.Join(p.BaseDir, "systems")
}

func (p *PathResolver) SystemDir(id string) string {
	return filepath.Join(p.SystemsDir(), id)
}

func (p *PathResolver) WorldsDir() string {
	return filepath.Join(p.BaseDir, "worlds")
}

func (p *PathResolver) WorldDir(id string) string {
	return filepath.Join(p.WorldsDir(), id)
}

func (p *PathResolver) GamesDir() string {
	return filepath.Join(p.BaseDir, "games")
}

func (p *PathResolver) GameDir(id string) string {
	return filepath.Join(p.GamesDir(), id)
}

func LoadSystemManifest(path string) (*SystemManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read system manifest: %w", err)
	}
	var manifest SystemManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal system manifest: %w", err)
	}
	return &manifest, nil
}

func LoadWorldManifest(path string) (*WorldManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read world manifest: %w", err)
	}
	var manifest WorldManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal world manifest: %w", err)
	}
	return &manifest, nil
}

func LoadGameManifest(path string) (*GameManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read game manifest: %w", err)
	}
	var manifest GameManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal game manifest: %w", err)
	}
	return &manifest, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/core/... -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum pkg/core/
git commit -m "feat(core): initialize Go module and core manifest types"
```

---

### Task 2: Schema-Agnostic State Engine

**Files:**
- Create: `pkg/state/state.go`
- Test: `pkg/state/state_test.go`

- [ ] **Step 1: Write the failing test for State Get/Set**

```go
// pkg/state/state_test.go
package state

import (
	"reflect"
	"testing"
)

func TestStateGetSet(t *testing.T) {
	s := NewState(map[string]interface{}{
		"hp": 100,
		"stats": map[string]interface{}{
			"strength": 14,
			"agility":  12,
		},
		"tags": []interface{}{"warrior", "veteran"},
	})

	// Direct get
	val, ok := s.Get("hp")
	if !ok || val != 100 {
		t.Errorf("expected hp=100, got %v (ok=%v)", val, ok)
	}

	// Nested dot-path get
	str, ok := s.Get("stats.strength")
	if !ok || str != 14 {
		t.Errorf("expected stats.strength=14, got %v (ok=%v)", str, ok)
	}

	// Set nested path
	if err := s.Set("stats.intelligence", 16); err != nil {
		t.Fatalf("failed to set stats.intelligence: %v", err)
	}
	intel, ok := s.Get("stats.intelligence")
	if !ok || intel != 16 {
		t.Errorf("expected stats.intelligence=16, got %v (ok=%v)", intel, ok)
	}

	// Set new root path
	if err := s.Set("mana", 50); err != nil {
		t.Fatalf("failed to set mana: %v", err)
	}
	mana, ok := s.Get("mana")
	if !ok || mana != 50 {
		t.Errorf("expected mana=50, got %v (ok=%v)", mana, ok)
	}

	// Snapshot
	raw := s.Raw()
	if !reflect.DeepEqual(raw["mana"], 50) {
		t.Errorf("expected raw state to reflect changes, got %+v", raw)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/state/... -v`  
Expected: FAIL (package/state not defined)

- [ ] **Step 3: Implement Schema-Agnostic State Container**

Write `pkg/state/state.go`:
```go
package state

import (
	"fmt"
	"strings"
	"sync"
)

type State struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

func NewState(initial map[string]interface{}) *State {
	if initial == nil {
		initial = make(map[string]interface{})
	}
	return &State{data: initial}
}

func (s *State) Get(path string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	parts := strings.Split(path, ".")
	var current interface{} = s.data

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		val, exists := m[part]
		if !exists {
			return nil, false
		}
		current = val
	}
	return current, true
}

func (s *State) Set(path string, val interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return fmt.Errorf("empty path")
	}

	current := s.data
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		next, exists := current[part]
		if !exists {
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
			continue
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return fmt.Errorf("path component %q is not a map", part)
		}
		current = nextMap
	}

	lastPart := parts[len(parts)-1]
	current[lastPart] = val
	return nil
}

func (s *State) Raw() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clone := make(map[string]interface{}, len(s.data))
	for k, v := range s.data {
		clone[k] = v
	}
	return clone
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/state/... -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/state/
git commit -m "feat(state): implement schema-agnostic generic state container"
```

---

### Task 3: Markdown Entity Document Parser & Wikilink Extractor

**Files:**
- Create: `pkg/entity/entity.go`
- Test: `pkg/entity/entity_test.go`

- [ ] **Step 1: Write the failing test for Entity Markdown parsing**

```go
// pkg/entity/entity_test.go
package entity

import (
	"reflect"
	"testing"
)

func TestParseMarkdownEntity(t *testing.T) {
	doc := `---
id: lady-evelyn
name: Lady Evelyn Vance
type: character
tags: [npc, rogue]
voice:
  provider: kokoro
  voice_id: bf_emma
location: "[[Alden-Tavern]]"
state:
  hp: 35
  armor: 14
---

# Lady Evelyn Vance

A former lieutenant in the Iron Guard, Evelyn hides at [[Alden-Tavern]].
She wields the [[Vorpal-Dagger|dread dagger]] and answers to [[The-Iron-Pact]].
`

	entity, err := ParseMarkdownEntity([]byte(doc))
	if err != nil {
		t.Fatalf("ParseMarkdownEntity failed: %v", err)
	}

	if entity.ID != "lady-evelyn" {
		t.Errorf("expected ID 'lady-evelyn', got %q", entity.ID)
	}
	if entity.Name != "Lady Evelyn Vance" {
		t.Errorf("expected Name 'Lady Evelyn Vance', got %q", entity.Name)
	}
	if entity.Type != "character" {
		t.Errorf("expected Type 'character', got %q", entity.Type)
	}

	expectedLinks := []string{"Alden-Tavern", "Vorpal-Dagger", "The-Iron-Pact"}
	if !reflect.DeepEqual(entity.Wikilinks, expectedLinks) {
		t.Errorf("expected wikilinks %v, got %v", expectedLinks, entity.Wikilinks)
	}

	hp, ok := entity.State.Get("hp")
	if !ok || hp != 35 {
		t.Errorf("expected state hp=35, got %v", hp)
	}
}

func TestSerializeMarkdownEntity(t *testing.T) {
	e := &Entity{
		ID:   "alden-tavern",
		Name: "Alden Tavern",
		Type: "location",
		Tags: []string{"tavern", "safehouse"},
		Body: "A quiet tavern at the edge of the woods.\n",
	}
	e.InitState(map[string]interface{}{"capacity": 40})

	data, err := e.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown failed: %v", err)
	}

	reparsed, err := ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("reparse failed: %v", err)
	}
	if reparsed.ID != e.ID || reparsed.Name != e.Name || reparsed.Type != e.Type {
		t.Errorf("mismatch after serialization: got %+v", reparsed)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/entity/... -v`  
Expected: FAIL (package/entity not defined)

- [ ] **Step 3: Implement Entity Parser & Wikilink Extractor**

Write `pkg/entity/entity.go`:
```go
package entity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/state"
	"gopkg.in/yaml.v3"
)

var wikilinkRegex = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|[^\]]+)?\]\]`)

type VoiceConfig struct {
	Provider string  `yaml:"provider,omitempty"`
	VoiceID  string  `yaml:"voice_id,omitempty"`
	Pitch    float64 `yaml:"pitch,omitempty"`
}

type EntityFrontmatter struct {
	ID        string                 `yaml:"id"`
	Name      string                 `yaml:"name"`
	Type      string                 `yaml:"type"`
	Tags      []string               `yaml:"tags,omitempty"`
	Voice     *VoiceConfig           `yaml:"voice,omitempty"`
	Portrait  string                 `yaml:"portrait,omitempty"`
	Location  string                 `yaml:"location,omitempty"`
	Faction   string                 `yaml:"faction,omitempty"`
	State     map[string]interface{} `yaml:"state,omitempty"`
	ExtraMeta map[string]interface{} `yaml:",inline"`
}

type Entity struct {
	ID        string
	Name      string
	Type      string
	Tags      []string
	Voice     *VoiceConfig
	Portrait  string
	Location  string
	Faction   string
	State     *state.State
	ExtraMeta map[string]interface{}
	Body      string
	Wikilinks []string
	Hash      string
}

func (e *Entity) InitState(data map[string]interface{}) {
	e.State = state.NewState(data)
}

func ParseMarkdownEntity(data []byte) (*Entity, error) {
	hashBytes := sha256.Sum256(data)
	fileHash := hex.EncodeToString(hashBytes[:])

	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("entity markdown missing frontmatter header (---)")
	}

	endIdx := strings.Index(content[4:], "\n---\n")
	if endIdx == -1 {
		return nil, fmt.Errorf("entity markdown missing frontmatter closing delimiter")
	}

	frontmatterRaw := content[4 : 4+endIdx]
	bodyRaw := content[4+endIdx+5:]

	var fm EntityFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatterRaw), &fm); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	linksSet := make(map[string]struct{})
	var links []string

	// Scan frontmatter location/faction for wikilinks
	for _, rawRef := range []string{fm.Location, fm.Faction} {
		matches := wikilinkRegex.FindAllStringSubmatch(rawRef, -1)
		for _, m := range matches {
			if len(m) > 1 {
				target := strings.TrimSpace(m[1])
				if _, seen := linksSet[target]; !seen {
					linksSet[target] = struct{}{}
					links = append(links, target)
				}
			}
		}
	}

	// Scan body for wikilinks
	bodyMatches := wikilinkRegex.FindAllStringSubmatch(bodyRaw, -1)
	for _, m := range bodyMatches {
		if len(m) > 1 {
			target := strings.TrimSpace(m[1])
			if _, seen := linksSet[target]; !seen {
				linksSet[target] = struct{}{}
				links = append(links, target)
			}
		}
	}

	entity := &Entity{
		ID:        fm.ID,
		Name:      fm.Name,
		Type:      fm.Type,
		Tags:      fm.Tags,
		Voice:     fm.Voice,
		Portrait:  fm.Portrait,
		Location:  fm.Location,
		Faction:   fm.Faction,
		State:     state.NewState(fm.State),
		ExtraMeta: fm.ExtraMeta,
		Body:      bodyRaw,
		Wikilinks: links,
		Hash:      fileHash,
	}

	return entity, nil
}

func (e *Entity) SerializeMarkdown() ([]byte, error) {
	fm := EntityFrontmatter{
		ID:        e.ID,
		Name:      e.Name,
		Type:      e.Type,
		Tags:      e.Tags,
		Voice:     e.Voice,
		Portrait:  e.Portrait,
		Location:  e.Location,
		Faction:   e.Faction,
		ExtraMeta: e.ExtraMeta,
	}
	if e.State != nil {
		fm.State = e.State.Raw()
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(fm); err != nil {
		return nil, fmt.Errorf("serialize frontmatter: %w", err)
	}
	buf.WriteString("---\n\n")
	buf.WriteString(e.Body)

	return buf.Bytes(), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/entity/... -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/entity/
git commit -m "feat(entity): implement markdown entity parser with wikilink extraction"
```

---

### Task 4: Pure Go SQLite Graph & Entity Storage

**Files:**
- Create: `pkg/storage/db.go`
- Create: `pkg/storage/store.go`
- Test: `pkg/storage/store_test.go`

- [ ] **Step 1: Install pure Go SQLite driver & write failing test**

```bash
go get modernc.org/sqlite
```

Write `pkg/storage/store_test.go`:
```go
// pkg/storage/store_test.go
package storage

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestStorageOperations(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "index.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ent := &entity.Entity{
		ID:        "alden-tavern",
		Name:      "Alden Tavern",
		Type:      "location",
		Body:      "A warm tavern.",
		Wikilinks: []string{"Eldoria"},
		Hash:      "hash-123",
	}
	ent.InitState(map[string]interface{}{"cozy": true})

	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("SaveEntity failed: %v", err)
	}

	loaded, err := store.GetEntity("alden-tavern")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if loaded.ID != ent.ID || loaded.Name != ent.Name {
		t.Errorf("loaded entity mismatch: %+v", loaded)
	}

	edges, err := store.GetEdgesFrom("alden-tavern")
	if err != nil {
		t.Fatalf("GetEdgesFrom failed: %v", err)
	}
	if len(edges) != 1 || edges[0].TargetID != "Eldoria" {
		t.Errorf("expected edge to Eldoria, got: %+v", edges)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/storage/... -v`  
Expected: FAIL (store types not defined)

- [ ] **Step 3: Implement SQLite Schema & Store**

Write `pkg/storage/db.go`:
```go
package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS entities (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    frontmatter_json TEXT NOT NULL,
    body TEXT NOT NULL,
    file_hash TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS edges (
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    relation TEXT NOT NULL,
    PRIMARY KEY (source_id, target_id, relation)
);

CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id);
`

func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
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

Write `pkg/storage/store.go`:
```go
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type Edge struct {
	SourceID string
	TargetID string
	Relation string
}

type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
	db, err := OpenDB(path)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SaveEntity(e *entity.Entity) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	fmMeta := map[string]interface{}{
		"tags":     e.Tags,
		"voice":    e.Voice,
		"portrait": e.Portrait,
		"location": e.Location,
		"faction":  e.Faction,
		"extra":    e.ExtraMeta,
	}
	if e.State != nil {
		fmMeta["state"] = e.State.Raw()
	}

	fmJSON, err := json.Marshal(fmMeta)
	if err != nil {
		return fmt.Errorf("marshal frontmatter: %w", err)
	}

	query := `
	INSERT INTO entities (id, name, type, frontmatter_json, body, file_hash, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		name = excluded.name,
		type = excluded.type,
		frontmatter_json = excluded.frontmatter_json,
		body = excluded.body,
		file_hash = excluded.file_hash,
		updated_at = CURRENT_TIMESTAMP
	`
	if _, err := tx.Exec(query, e.ID, e.Name, e.Type, string(fmJSON), e.Body, e.Hash); err != nil {
		return fmt.Errorf("upsert entity: %w", err)
	}

	// Recreate outgoing edges
	if _, err := tx.Exec(`DELETE FROM edges WHERE source_id = ?`, e.ID); err != nil {
		return fmt.Errorf("clear edges: %w", err)
	}

	for _, target := range e.Wikilinks {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO edges (source_id, target_id, relation) VALUES (?, ?, ?)`,
			e.ID, target, "references"); err != nil {
			return fmt.Errorf("insert edge: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) GetEntity(id string) (*entity.Entity, error) {
	query := `SELECT id, name, type, frontmatter_json, body, file_hash FROM entities WHERE id = ?`
	row := s.db.QueryRow(query, id)

	var ent entity.Entity
	var fmJSON string
	if err := row.Scan(&ent.ID, &ent.Name, &ent.Type, &fmJSON, &ent.Body, &ent.Hash); err != nil {
		return nil, err
	}

	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(fmJSON), &meta); err == nil {
		if stateData, ok := meta["state"].(map[string]interface{}); ok {
			ent.InitState(stateData)
		}
	}

	return &ent, nil
}

func (s *Store) GetEdgesFrom(sourceID string) ([]Edge, error) {
	rows, err := s.db.Query(`SELECT source_id, target_id, relation FROM edges WHERE source_id = ?`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.SourceID, &e.TargetID, &e.Relation); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/storage/... -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum pkg/storage/
git commit -m "feat(storage): implement SQLite entity and graph edge store"
```

---

### Task 5: Filesystem Incremental Sync & Indexer

**Files:**
- Create: `pkg/storage/sync.go`
- Test: `pkg/storage/sync_test.go`

- [ ] **Step 1: Write the failing test for SyncDirectory**

```go
// pkg/storage/sync_test.go
package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncDirectory(t *testing.T) {
	tempDir := t.TempDir()
	entitiesDir := filepath.Join(tempDir, "entities")
	dbPath := filepath.Join(tempDir, "index.db")

	if err := os.MkdirAll(entitiesDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	doc1 := `---
id: tavern
name: Alden Tavern
type: location
---
A rustic [[Tavern]] in [[Eldoria]].
`
	if err := os.WriteFile(filepath.Join(entitiesDir, "Tavern.md"), []byte(doc1), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	syncer := NewSyncer(store)
	result, err := syncer.Sync(entitiesDir)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if result.Added != 1 {
		t.Errorf("expected 1 added, got %d", result.Added)
	}

	// Sync again without changes - should be 0 modified
	result2, err := syncer.Sync(entitiesDir)
	if err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	if result2.Unchanged != 1 {
		t.Errorf("expected 1 unchanged, got %d", result2.Unchanged)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/storage/... -v -run TestSyncDirectory`  
Expected: FAIL (Syncer not defined)

- [ ] **Step 3: Implement Directory Syncer**

Write `pkg/storage/sync.go`:
```go
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type SyncResult struct {
	Added     int
	Updated   int
	Unchanged int
}

type Syncer struct {
	store *Store
}

func NewSyncer(store *Store) *Syncer {
	return &Syncer{store: store}
}

func (s *Syncer) Sync(dir string) (*SyncResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return &SyncResult{}, nil
		}
		return nil, fmt.Errorf("read entities dir: %w", err)
	}

	res := &SyncResult{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", fullPath, err)
		}

		ent, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			// Skip or log malformed markdown
			continue
		}

		existing, err := s.store.GetEntity(ent.ID)
		if err != nil {
			// New entity
			if err := s.store.SaveEntity(ent); err != nil {
				return nil, fmt.Errorf("save entity %q: %w", ent.ID, err)
			}
			res.Added++
			continue
		}

		if existing.Hash == ent.Hash {
			res.Unchanged++
			continue
		}

		// Updated entity
		if err := s.store.SaveEntity(ent); err != nil {
			return nil, fmt.Errorf("update entity %q: %w", ent.ID, err)
		}
		res.Updated++
	}

	return res, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/storage/... -v -run TestSyncDirectory`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/storage/sync.go pkg/storage/sync_test.go
git commit -m "feat(storage): implement filesystem incremental entity syncer"
```

---

### Task 6: Game Instance Loader & Layer Composition

**Files:**
- Create: `pkg/engine/game.go`
- Test: `pkg/engine/game_test.go`

- [ ] **Step 1: Write the failing test for Game Session Loader**

```go
// pkg/engine/game_test.go
package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestGameInitAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	paths := core.NewPathResolver(tempDir)

	// 1. Setup system
	sysDir := paths.SystemDir("d20-test")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: d20-test\nname: D20 Test\nversion: 1.0\n"), 0644)

	// 2. Setup world
	worldDir := paths.WorldDir("fantasy-realm")
	worldEntities := filepath.Join(worldDir, "entities")
	if err := os.MkdirAll(worldEntities, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: fantasy-realm\nname: Fantasy Realm\n"), 0644)
	tavernDoc := "---\nid: tavern\nname: Oakhaven Tavern\ntype: location\n---\nStarting tavern."
	os.WriteFile(filepath.Join(worldEntities, "Tavern.md"), []byte(tavernDoc), 0644)

	// 3. Initialize game
	session, err := InitGame(paths, "campaign-01", "d20-test", "fantasy-realm", "Sean")
	if err != nil {
		t.Fatalf("InitGame failed: %v", err)
	}
	defer session.Close()

	if session.Manifest.Player != "Sean" {
		t.Errorf("expected player Sean, got %q", session.Manifest.Player)
	}

	// Verify base entity was copied and indexed into game
	loaded, err := session.Store.GetEntity("tavern")
	if err != nil {
		t.Fatalf("failed to query tavern from game index: %v", err)
	}
	if loaded.Name != "Oakhaven Tavern" {
		t.Errorf("expected Oakhaven Tavern, got %q", loaded.Name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/... -v`  
Expected: FAIL (package/engine not defined)

- [ ] **Step 3: Implement Game Session & Composition Loader**

Write `pkg/engine/game.go`:
```go
package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/storage"
	"gopkg.in/yaml.v3"
)

type Session struct {
	Paths    *core.PathResolver
	Manifest *core.GameManifest
	System   *core.SystemManifest
	World    *core.WorldManifest
	Store    *storage.Store
}

func (s *Session) Close() error {
	if s.Store != nil {
		return s.Store.Close()
	}
	return nil
}

func InitGame(paths *core.PathResolver, gameID, systemID, worldID, playerName string) (*Session, error) {
	// Verify system & world exist
	sysManifest, err := core.LoadSystemManifest(filepath.Join(paths.SystemDir(systemID), "system.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load system %q: %w", systemID, err)
	}

	worldManifest, err := core.LoadWorldManifest(filepath.Join(paths.WorldDir(worldID), "world.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load world %q: %w", worldID, err)
	}

	gameDir := paths.GameDir(gameID)
	gameEntitiesDir := filepath.Join(gameDir, "entities")
	gameCacheDir := filepath.Join(gameDir, "cache")
	gameAssetsDir := filepath.Join(gameDir, "assets")

	for _, d := range []string{gameDir, gameEntitiesDir, gameCacheDir, gameAssetsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, fmt.Errorf("create dir %q: %w", d, err)
		}
	}

	// Create game manifest
	manifest := &core.GameManifest{
		ID:       gameID,
		Name:     gameID,
		SystemID: systemID,
		WorldID:  worldID,
		Player:   playerName,
	}

	manifestBytes, err := yaml.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal game manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), manifestBytes, 0644); err != nil {
		return nil, fmt.Errorf("write game.yaml: %w", err)
	}

	// Copy initial template entities from world into game
	worldEntitiesDir := filepath.Join(paths.WorldDir(worldID), "entities")
	if entries, err := os.ReadDir(worldEntitiesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			src := filepath.Join(worldEntitiesDir, e.Name())
			dst := filepath.Join(gameEntitiesDir, e.Name())
			if err := copyFile(src, dst); err != nil {
				return nil, fmt.Errorf("copy entity template %q: %w", e.Name(), err)
			}
		}
	}

	// Open store and run initial sync
	dbPath := filepath.Join(gameCacheDir, "index.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("init game store: %w", err)
	}

	syncer := storage.NewSyncer(store)
	if _, err := syncer.Sync(gameEntitiesDir); err != nil {
		store.Close()
		return nil, fmt.Errorf("initial sync: %w", err)
	}

	return &Session{
		Paths:    paths,
		Manifest: manifest,
		System:   sysManifest,
		World:    worldManifest,
		Store:    store,
	}, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/... -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/
git commit -m "feat(engine): implement 3-tier game composition and session loader"
```

---

### Task 7: CLI Entry Point & Sanity Verification

**Files:**
- Create: `cmd/localrpg/main.go`
- Test: `cmd/localrpg/main_test.go`

- [ ] **Step 1: Write integration test for CLI**

```go
// cmd/localrpg/main_test.go
package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIVersionAndHelp(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v, output: %s", err, string(out))
	}

	if !strings.Contains(string(out), "LocalRPG v0.1.0") {
		t.Errorf("expected version output, got: %s", string(out))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/localrpg/... -v`  
Expected: FAIL (main.go not implemented)

- [ ] **Step 3: Implement CLI Entry Point**

Write `cmd/localrpg/main.go`:
```go
package main

import (
	"flag"
	"fmt"
	"os"
)

const Version = "0.1.0"

func main() {
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("LocalRPG v%s\n", Version)
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	switch args[0] {
	case "version":
		fmt.Printf("LocalRPG v%s\n", Version)
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: localrpg <command> [arguments]")
	fmt.Println("\nCommands:")
	fmt.Println("  play <game-id>     Launch terminal TUI play mode")
	fmt.Println("  gui                Launch desktop application (Wails v3)")
	fmt.Println("  export <format>    Export story replay (web, video)")
	fmt.Println("  version            Print version information")
}
```

- [ ] **Step 4: Run all package tests**

Run: `go test ./... -v`  
Expected: All package tests PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/localrpg/
git commit -m "feat(cli): add initial entry point and CLI commands"
```
