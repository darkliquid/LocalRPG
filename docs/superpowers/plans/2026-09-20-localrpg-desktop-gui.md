# LocalRPG Wails v3 Desktop GUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the desktop graphical user interface for LocalRPG featuring the Option C immersive chronicle reader with flyout drawers (Character Sheet, Knowledge Graph, Codex Editor, Living World Arcs), an action console with dice rolls and audio playback, and a Go backend service bridging the engine to React 19.

**Architecture:** A decoupled frontend in `frontend/` (React 19 + TypeScript + Tailwind CSS) communicating via JSON RPC / REST with a Go backend service in `pkg/gui`. The backend exposes game state, turn history, entity markdown notes, and graph data, and can be packaged as a native Wails v3 desktop window or served locally via `localrpg gui`.

**Tech Stack:** Go 1.27, React 19, TypeScript, Tailwind CSS, Vite, HTML5 Canvas for interactive entity graphs, Go standard `net/http` and `embed`.

---

### File Structure Map

```text
LocalRPG/
├── cmd/
│   └── localrpg/
│       ├── main.go                     # Updated with "gui" subcommand dispatch
│       ├── gui.go                      # CLI handler for "localrpg gui"
│       └── gui_test.go                 # CLI GUI command integration test
├── pkg/
│   └── gui/
│       ├── types.go                    # DTOs: GameStateDTO, TurnDTO, EntityDTO, GraphDTO
│       ├── service.go                  # Engine & storage bridge service
│       ├── service_test.go             # Service unit tests
│       ├── server.go                   # HTTP API router & static asset server
│       ├── server_test.go              # API endpoint integration tests
│       └── assets.go                   # Embedded frontend distribution FS
└── frontend/
    ├── package.json                    # React 19, TypeScript, Vite, Tailwind CSS
    ├── tsconfig.json                   # TypeScript compiler configuration
    ├── vite.config.ts                  # Vite build config with proxy to backend
    ├── index.html                      # Entry HTML
    ├── src/
    │   ├── main.tsx                    # React DOM entrypoint
    │   ├── index.css                   # Tailwind directives & typography styling
    │   ├── types.ts                    # TypeScript contracts matching Go DTOs
    │   ├── api/
    │   │   └── client.ts               # HTTP client fetching backend endpoints
    │   ├── components/
    │   │   ├── ChronicleView.tsx       # Immersive reader: prose, dialogue, audio chips
    │   │   ├── ActionConsole.tsx       # Mode switcher, dice roll inputs, STT button
    │   │   ├── Drawers.tsx             # Flyout drawer container with top-layer transitions
    │   │   ├── CharacterSheetDrawer.tsx# Schema-driven stat dials and inventory slots
    │   │   ├── GraphDrawer.tsx         # 2D Canvas force-directed entity graph
    │   │   ├── CodexDrawer.tsx         # Entity Markdown editor and backlinks viewer
    │   │   └── LivingWorldDrawer.tsx   # Narrative arcs, faction clocks, and rumors
    │   └── App.tsx                     # Main layout coordinating Chronicle and Drawers
```

---

### Task 1: Go GUI Backend Service & Data Contracts

**Files:**
- Create: `pkg/gui/types.go`
- Create: `pkg/gui/service.go`
- Test: `pkg/gui/service_test.go`

- [ ] **Step 1: Write the failing test for GUI Service**

```go
// pkg/gui/service_test.go
package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func setupTestGame(t *testing.T) (string, *Service) {
	tempDir := t.TempDir()
	gamesDir := filepath.Join(tempDir, "games", "test-campaign")
	entitiesDir := filepath.Join(gamesDir, "entities")
	_ = os.MkdirAll(entitiesDir, 0755)

	// Create test game manifest
	manifestContent := `id: test-campaign
name: Test Campaign
system_id: core-d20
world_id: shadow-realm
player_entity: player-elena
`
	_ = os.WriteFile(filepath.Join(gamesDir, "game.yaml"), []byte(manifestContent), 0644)

	// Create player entity
	playerMD := `---
name: Elena Nightshade
type: character
state:
  hp: 24
  max_hp: 30
  level: 3
---
A cunning rogue in dark leather.`
	_ = os.WriteFile(filepath.Join(entitiesDir, "player-elena.md"), []byte(playerMD), 0644)

	// Create NPC entity
	npcMD := `---
name: Captain Kaelen
type: npc
voice:
  provider: kokoro
  voice_id: bm_george
state:
  attitude: neutral
---
The town watch captain. Speaks with [[player-elena]].`
	_ = os.WriteFile(filepath.Join(entitiesDir, "captain-kaelen.md"), []byte(npcMD), 0644)

	dbPath := filepath.Join(gamesDir, "game.db")
	db, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}

	syncer := storage.NewSyncer(db)
	_ = syncer.SyncDirectory(entitiesDir)

	service := NewService(tempDir)
	return "test-campaign", service
}

func TestGUIService_GetGameState(t *testing.T) {
	gameID, svc := setupTestGame(t)

	state, err := svc.GetGameState(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGameState failed: %v", err)
	}

	if state.Player.Name != "Elena Nightshade" {
		t.Errorf("expected player Elena Nightshade, got %s", state.Player.Name)
	}
	if state.Player.State["hp"] != 24 {
		t.Errorf("expected hp 24, got %v", state.Player.State["hp"])
	}
}

func TestGUIService_GetGraph(t *testing.T) {
	gameID, svc := setupTestGame(t)

	graph, err := svc.GetGraph(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGraph failed: %v", err)
	}

	if len(graph.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(graph.Nodes))
	}

	foundLink := false
	for _, link := range graph.Links {
		if link.Source == "captain-kaelen" && link.Target == "player-elena" {
			foundLink = true
			break
		}
	}
	if !foundLink {
		t.Errorf("expected wikilink edge between captain-kaelen and player-elena")
	}
}

func TestGUIService_EntityCRUD(t *testing.T) {
	gameID, svc := setupTestGame(t)

	ent, err := svc.GetEntity(context.Background(), gameID, "captain-kaelen")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if ent.Name != "Captain Kaelen" {
		t.Errorf("expected Captain Kaelen, got %s", ent.Name)
	}

	// Update entity
	updatedMD := `---
name: Captain Kaelen
type: npc
state:
  attitude: friendly
---
The town watch captain, now an ally.`
	err = svc.SaveEntity(context.Background(), gameID, "captain-kaelen", updatedMD)
	if err != nil {
		t.Fatalf("SaveEntity failed: %v", err)
	}

	updated, err := svc.GetEntity(context.Background(), gameID, "captain-kaelen")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if updated.State["attitude"] != "friendly" {
		t.Errorf("expected attitude friendly, got %v", updated.State["attitude"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/... -v`  
Expected: FAIL (package undefined)

- [ ] **Step 3: Implement GUI DTOs and Service**

Write `pkg/gui/types.go`:
```go
package gui

type PlayerDTO struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Type  string                 `json:"type"`
	State map[string]interface{} `json:"state"`
}

type NarrativeArcDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Progress    int    `json:"progress"`
	MaxProgress int    `json:"max_progress"`
	Status      string `json:"status"`
}

type FactionClockDTO struct {
	Faction  string `json:"faction"`
	Name     string `json:"name"`
	Ticks    int    `json:"ticks"`
	MaxTicks int    `json:"max_ticks"`
}

type GameStateDTO struct {
	GameID    string            `json:"game_id"`
	GameName  string            `json:"game_name"`
	Player    PlayerDTO         `json:"player"`
	Arcs      []NarrativeArcDTO `json:"arcs"`
	Clocks    []FactionClockDTO `json:"clocks"`
	Locations []string          `json:"locations"`
}

type TurnDTO struct {
	TurnNumber  int      `json:"turn_number"`
	InputText   string   `json:"input_text"`
	Mode        string   `json:"mode"`
	Prose       string   `json:"prose"`
	Speaker     string   `json:"speaker,omitempty"`
	Dialogue    string   `json:"dialogue,omitempty"`
	AudioURL    string   `json:"audio_url,omitempty"`
	ImageURL    string   `json:"image_url,omitempty"`
	EntitiesHit []string `json:"entities_hit,omitempty"`
}

type EntityDTO struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Markdown  string                 `json:"markdown"`
	State     map[string]interface{} `json:"state"`
	Backlinks []string               `json:"backlinks"`
}

type GraphNodeDTO struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

type GraphLinkDTO struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type GraphDTO struct {
	Nodes []GraphNodeDTO `json:"nodes"`
	Links []GraphLinkDTO `json:"links"`
}
```

Write `pkg/gui/service.go`:
```go
package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type Service struct {
	rootDir string
}

func NewService(rootDir string) *Service {
	return &Service{rootDir: rootDir}
}

func (s *Service) GetGameState(ctx context.Context, gameID string) (*GameStateDTO, error) {
	paths := core.ResolvePaths(s.rootDir, "", "", gameID)
	gameManifest, err := core.ParseGameManifest(paths.GameManifest)
	if err != nil {
		return nil, fmt.Errorf("read game manifest: %w", err)
	}

	playerFile := filepath.Join(paths.EntitiesDir, gameManifest.PlayerEntity+".md")
	data, err := os.ReadFile(playerFile)
	if err != nil {
		return nil, fmt.Errorf("read player entity: %w", err)
	}

	ent, err := entity.ParseMarkdownEntity(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse player entity: %w", err)
	}

	return &GameStateDTO{
		GameID:   gameID,
		GameName: gameManifest.Name,
		Player: PlayerDTO{
			ID:    gameManifest.PlayerEntity,
			Name:  ent.Frontmatter.Name,
			Type:  ent.Frontmatter.Type,
			State: ent.Frontmatter.State,
		},
		Arcs:      []NarrativeArcDTO{},
		Clocks:    []FactionClockDTO{},
		Locations: []string{},
	}, nil
}

func (s *Service) GetEntity(ctx context.Context, gameID, entityID string) (*EntityDTO, error) {
	paths := core.ResolvePaths(s.rootDir, "", "", gameID)
	path := filepath.Join(paths.EntitiesDir, entityID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read entity file: %w", err)
	}

	ent, err := entity.ParseMarkdownEntity(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse entity: %w", err)
	}

	dbPath := filepath.Join(paths.GameDir, "game.db")
	db, err := storage.NewSQLiteStore(dbPath)
	var backlinks []string
	if err == nil {
		defer db.Close()
		backlinks, _ = db.GetBacklinks(entityID)
	}

	return &EntityDTO{
		ID:        entityID,
		Name:      ent.Frontmatter.Name,
		Type:      ent.Frontmatter.Type,
		Markdown:  string(data),
		State:     ent.Frontmatter.State,
		Backlinks: backlinks,
	}, nil
}

func (s *Service) SaveEntity(ctx context.Context, gameID, entityID, rawMarkdown string) error {
	paths := core.ResolvePaths(s.rootDir, "", "", gameID)
	path := filepath.Join(paths.EntitiesDir, entityID+".md")
	if err := os.WriteFile(path, []byte(rawMarkdown), 0644); err != nil {
		return fmt.Errorf("write entity file: %w", err)
	}

	dbPath := filepath.Join(paths.GameDir, "game.db")
	db, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		return nil // Non-fatal if db sync fails temporarily
	}
	defer db.Close()

	syncer := storage.NewSyncer(db)
	return syncer.SyncFile(path)
}

func (s *Service) GetGraph(ctx context.Context, gameID string) (*GraphDTO, error) {
	paths := core.ResolvePaths(s.rootDir, "", "", gameID)
	entries, err := os.ReadDir(paths.EntitiesDir)
	if err != nil {
		return nil, fmt.Errorf("read entities dir: %w", err)
	}

	nodes := make([]GraphNodeDTO, 0, len(entries))
	links := make([]GraphLinkDTO, 0)

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".md")
		data, err := os.ReadFile(filepath.Join(paths.EntitiesDir, entry.Name()))
		if err != nil {
			continue
		}
		ent, err := entity.ParseMarkdownEntity(string(data))
		if err != nil {
			continue
		}

		nodes = append(nodes, GraphNodeDTO{
			ID:    id,
			Label: ent.Frontmatter.Name,
			Type:  ent.Frontmatter.Type,
		})

		for _, link := range ent.Links {
			links = append(links, GraphLinkDTO{
				Source: id,
				Target: link,
			})
		}
	}

	return &GraphDTO{
		Nodes: nodes,
		Links: links,
	}, nil
}

func (s *Service) GetChronicle(ctx context.Context, gameID string) ([]TurnDTO, error) {
	paths := core.ResolvePaths(s.rootDir, "", "", gameID)
	historyFile := filepath.Join(paths.GameDir, "history.jsonl")
	turns, err := engine.LoadTurnHistory(historyFile)
	if err != nil {
		return []TurnDTO{}, nil
	}

	dtos := make([]TurnDTO, len(turns))
	for i, turn := range turns {
		dtos[i] = TurnDTO{
			TurnNumber:  turn.TurnNumber,
			InputText:   turn.InputText,
			Mode:        turn.Mode,
			Prose:       turn.OutputProse,
			EntitiesHit: turn.EntitiesHit,
		}
	}
	return dtos, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/... -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/service_test.go
git commit -m "feat(gui): implement backend service and data transfer objects"
```

---

### Task 2: Go HTTP & Static Web Server

**Files:**
- Create: `pkg/gui/server.go`
- Test: `pkg/gui/server_test.go`

- [ ] **Step 1: Write failing test for HTTP Server**

```go
// pkg/gui/server_test.go
package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGUIServerRoutes(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	// Test GET /api/game/:id/state
	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/state", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var state GameStateDTO
	if err := json.NewDecoder(rec.Body).Decode(&state); err != nil {
		t.Fatalf("decode state failed: %v", err)
	}
	if state.Player.Name != "Elena Nightshade" {
		t.Errorf("expected Elena Nightshade, got %s", state.Player.Name)
	}

	// Test GET /api/game/:id/graph
	req = httptest.NewRequest("GET", "/api/game/"+gameID+"/graph", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var graph GraphDTO
	if err := json.NewDecoder(rec.Body).Decode(&graph); err != nil {
		t.Fatalf("decode graph failed: %v", err)
	}
	if len(graph.Nodes) < 2 {
		t.Errorf("expected nodes in graph, got %d", len(graph.Nodes))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/... -v -run TestGUIServerRoutes`  
Expected: FAIL (NewServer undefined)

- [ ] **Step 3: Implement HTTP Server**

Write `pkg/gui/server.go`:
```go
package gui

import (
	"encoding/json"
	"net/http"
	"strings"
)

type Server struct {
	service     *Service
	assetServer http.Handler
	mux         *http.ServeMux
}

func NewServer(service *Service, assetHandler http.Handler) *Server {
	s := &Server{
		service:     service,
		assetServer: assetHandler,
		mux:         http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/game/", s.handleGameRoutes)
	if s.assetServer != nil {
		s.mux.Handle("/", s.assetServer)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleGameRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/game/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	gameID := parts[0]
	action := parts[1]

	switch action {
	case "state":
		state, err := s.service.GetGameState(r.Context(), gameID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, state)

	case "graph":
		graph, err := s.service.GetGraph(r.Context(), gameID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, graph)

	case "chronicle":
		chronicle, err := s.service.GetChronicle(r.Context(), gameID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, chronicle)

	case "entity":
		if len(parts) < 3 {
			http.Error(w, "missing entity id", http.StatusBadRequest)
			return
		}
		entityID := parts[2]
		if r.Method == http.MethodPut {
			var body struct {
				Markdown string `json:"markdown"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if err := s.service.SaveEntity(r.Context(), gameID, entityID, body.Markdown); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		ent, err := s.service.GetEntity(r.Context(), gameID, entityID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, ent)

	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/... -v -run TestGUIServerRoutes`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/server.go pkg/gui/server_test.go
git commit -m "feat(gui): implement HTTP REST API server"
```

---

### Task 3: Frontend Project Setup & Design System

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/tsconfig.json`
- Create: `frontend/vite.config.ts`
- Create: `frontend/index.html`
- Create: `frontend/src/types.ts`
- Create: `frontend/src/index.css`
- Create: `frontend/src/api/client.ts`

- [ ] **Step 1: Create frontend configuration files**

Write `frontend/package.json`:
```json
{
  "name": "localrpg-frontend",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "lucide-react": "^0.475.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {
    "@tailwindcss/vite": "^4.0.0",
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "@vitejs/plugin-react": "^4.3.4",
    "tailwindcss": "^4.0.0",
    "typescript": "^5.7.0",
    "vite": "^6.2.0"
  }
}
```

Write `frontend/tsconfig.json`:
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": false,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src"]
}
```

Write `frontend/vite.config.ts`:
```typescript
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://localhost:8080'
    }
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true
  }
});
```

Write `frontend/index.html`:
```html
<!DOCTYPE html>
<html lang="en" class="dark">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>LocalRPG</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Cinzel:wght@600;800&family=EB+Garamond:ital,wght@0,400;0,600;1,400&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
  </head>
  <body class="bg-stone-950 text-stone-200 antialiased min-h-screen">
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

Write `frontend/src/types.ts`:
```typescript
export interface PlayerState {
  id: string;
  name: string;
  type: string;
  state: Record<string, any>;
}

export interface GameState {
  game_id: string;
  game_name: string;
  player: PlayerState;
  arcs: Array<{ id: string; name: string; progress: number; max_progress: number; status: string }>;
  clocks: Array<{ faction: string; name: string; ticks: number; max_ticks: number }>;
  locations: string[];
}

export interface Turn {
  turn_number: number;
  input_text: string;
  mode: string;
  prose: string;
  speaker?: string;
  dialogue?: string;
  audio_url?: string;
  image_url?: string;
  entities_hit?: string[];
}

export interface EntityNote {
  id: string;
  name: string;
  type: string;
  markdown: string;
  state: Record<string, any>;
  backlinks: string[];
}

export interface GraphNode {
  id: string;
  label: string;
  type: string;
}

export interface GraphLink {
  source: string;
  target: string;
}

export interface GraphData {
  nodes: GraphNode[];
  links: GraphLink[];
}
```

Write `frontend/src/api/client.ts`:
```typescript
import { GameState, Turn, EntityNote, GraphData } from '../types';

export class APIClient {
  private gameID: string;

  constructor(gameID: string) {
    this.gameID = gameID;
  }

  async getGameState(): Promise<GameState> {
    const res = await fetch(`/api/game/${this.gameID}/state`);
    if (!res.ok) throw new Error(`getGameState: ${res.statusText}`);
    return res.json();
  }

  async getChronicle(): Promise<Turn[]> {
    const res = await fetch(`/api/game/${this.gameID}/chronicle`);
    if (!res.ok) throw new Error(`getChronicle: ${res.statusText}`);
    return res.json();
  }

  async getGraph(): Promise<GraphData> {
    const res = await fetch(`/api/game/${this.gameID}/graph`);
    if (!res.ok) throw new Error(`getGraph: ${res.statusText}`);
    return res.json();
  }

  async getEntity(entityID: string): Promise<EntityNote> {
    const res = await fetch(`/api/game/${this.gameID}/entity/${entityID}`);
    if (!res.ok) throw new Error(`getEntity: ${res.statusText}`);
    return res.json();
  }

  async saveEntity(entityID: string, markdown: string): Promise<void> {
    const res = await fetch(`/api/game/${this.gameID}/entity/${entityID}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ markdown })
    });
    if (!res.ok) throw new Error(`saveEntity: ${res.statusText}`);
  }
}
```

Write `frontend/src/index.css`:
```css
@import "tailwindcss";

@layer base {
  body {
    font-family: 'EB Garamond', Georgia, serif;
    background-color: #0c0a09;
    color: #e7e5e4;
    overflow: hidden;
  }
  h1, h2, h3, h4, .font-cinzel {
    font-family: 'Cinzel', serif;
  }
  code, pre, .font-mono {
    font-family: 'JetBrains Mono', monospace;
  }
}

/* === Twintail Launcher Inspired Glassmorphic Styling & Animations === */

@keyframes bgFadeIn {
  0% {
    opacity: 0;
    transform: scale(1.02);
  }
  100% {
    opacity: 1;
    transform: scale(1);
  }
}

.animate-bg-fade-in {
  animation: bgFadeIn 450ms cubic-bezier(0.16, 1, 0.3, 1) forwards;
}

/* Translucent acrylic frosted glass panels */
.bg-glass {
  background: rgba(18, 15, 13, 0.70);
  backdrop-filter: blur(24px) saturate(140%);
  -webkit-backdrop-filter: blur(24px) saturate(140%);
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.bg-glass-card {
  background: rgba(22, 19, 17, 0.65);
  backdrop-filter: blur(20px) saturate(130%);
  -webkit-backdrop-filter: blur(20px) saturate(130%);
  border: 1px solid rgba(255, 255, 255, 0.07);
  box-shadow: 0 20px 40px -15px rgba(0, 0, 0, 0.7);
}

.bg-glass-drawer {
  background: rgba(12, 10, 9, 0.85);
  backdrop-filter: blur(32px) saturate(160%);
  -webkit-backdrop-filter: blur(32px) saturate(160%);
  border-left: 1px solid rgba(255, 255, 255, 0.1);
  box-shadow: -20px 0 50px -10px rgba(0, 0, 0, 0.85);
}

/* Subtle noise texture */
.bg-noise {
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noise'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.8' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noise)' opacity='0.035'/%3E%3C/svg%3E");
}

/* Custom scrollbars for translucent dark theme */
::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}
::-webkit-scrollbar-track {
  background: rgba(20, 17, 15, 0.4);
}
::-webkit-scrollbar-thumb {
  background: rgba(168, 162, 158, 0.3);
  border-radius: 3px;
}
::-webkit-scrollbar-thumb:hover {
  background: rgba(168, 162, 158, 0.6);
}
```

- [ ] **Step 2: Install dependencies and build test**

Run: `cd frontend && npm install && npm run build`  
Expected: PASS (generates `frontend/dist/`)

- [ ] **Step 3: Commit**

```bash
git add frontend/package.json frontend/tsconfig.json frontend/vite.config.ts frontend/index.html frontend/src/types.ts frontend/src/index.css frontend/src/api/client.ts
git commit -m "feat(gui): scaffold React 19 frontend project with Tailwind and API client"
```

---

### Task 4: Immersive Chronicle Reader & Action Console

**Files:**
- Create: `frontend/src/components/ChronicleView.tsx`
- Create: `frontend/src/components/ActionConsole.tsx`

- [ ] **Step 1: Implement ChronicleView component**

Write `frontend/src/components/ChronicleView.tsx`:
```tsx
import React from 'react';
import { Turn } from '../types';
import { Volume2, Sparkles } from 'lucide-react';

interface ChronicleViewProps {
  turns: Turn[];
  onWikilinkClick: (entityId: string) => void;
}

export const ChronicleView: React.FC<ChronicleViewProps> = ({ turns, onWikilinkClick }) => {
  const renderFormattedText = (text: string) => {
    // Replace [[wikilinks]] with clickable spans
    const parts = text.split(/(\[\[[^\]]+\]\])/g);
    return parts.map((part, i) => {
      if (part.startsWith('[[') && part.endsWith(']]')) {
        const link = part.slice(2, -2);
        return (
          <button
            key={i}
            onClick={() => onWikilinkClick(link)}
            className="text-amber-400 hover:text-amber-300 underline font-medium cursor-pointer mx-1 transition-colors"
          >
            {link}
          </button>
        );
      }
      return <span key={i}>{part}</span>;
    });
  };

  return (
    <div className="flex-1 overflow-y-auto px-6 py-8 max-w-4xl mx-auto space-y-8">
      {turns.map((turn) => (
        <div key={turn.turn_number} className="space-y-4 pb-6 border-b border-stone-800/60">
          {/* Player Input Block */}
          {turn.input_text && (
            <div className="flex items-start gap-3 text-stone-400 text-sm font-sans italic bg-stone-900/40 p-3 rounded-lg border border-stone-800">
              <span className="text-amber-500 font-semibold uppercase tracking-wider text-xs">[{turn.mode || 'Action'}]</span>
              <span>{turn.input_text}</span>
            </div>
          )}

          {/* Scene Illustration if available */}
          {turn.image_url && (
            <div className="my-4 rounded-xl overflow-hidden border border-stone-700 shadow-2xl">
              <img src={turn.image_url} alt="Scene illustration" className="w-full object-cover max-h-96" />
            </div>
          )}

          {/* Dialogue with Speaker Bubble */}
          {turn.dialogue && (
            <div className="bg-stone-900/80 border-l-4 border-amber-600 pl-4 py-3 pr-4 rounded-r-lg shadow-md my-3 space-y-2">
              <div className="flex items-center justify-between text-xs text-amber-500 font-cinzel font-bold tracking-widest">
                <span>{turn.speaker || 'UNKNOWN'}</span>
                {turn.audio_url && (
                  <button
                    onClick={() => new Audio(turn.audio_url).play()}
                    className="flex items-center gap-1 hover:text-amber-300 cursor-pointer"
                    title="Play voice clip"
                  >
                    <Volume2 className="w-3.5 h-3.5" />
                    <span>Play</span>
                  </button>
                )}
              </div>
              <p className="text-stone-200 text-lg leading-relaxed italic">
                "{renderFormattedText(turn.dialogue)}"
              </p>
            </div>
          )}

          {/* Narrator Prose */}
          {turn.prose && (
            <div className="text-stone-300 text-xl leading-relaxed tracking-wide font-serif">
              {renderFormattedText(turn.prose)}
            </div>
          )}
        </div>
      ))}
    </div>
  );
};
```

- [ ] **Step 2: Implement ActionConsole component**

Write `frontend/src/components/ActionConsole.tsx`:
```tsx
import React, { useState } from 'react';
import { Send, Mic, Dices, MessageSquare, Zap, Compass } from 'lucide-react';

interface ActionConsoleProps {
  onSubmit: (mode: string, text: string) => void;
  disabled?: boolean;
}

export const ActionConsole: React.FC<ActionConsoleProps> = ({ onSubmit, disabled }) => {
  const [text, setText] = useState('');
  const [mode, setMode] = useState<'do' | 'say' | 'story' | 'roll'>('do');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!text.trim() || disabled) return;
    onSubmit(mode, text.trim());
    setText('');
  };

  return (
    <div className="border-t border-stone-800 bg-stone-900/90 backdrop-blur-md p-4 max-w-4xl mx-auto w-full">
      {/* Mode Switcher Tabs */}
      <div className="flex items-center gap-2 mb-3">
        <button
          type="button"
          onClick={() => setMode('do')}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-cinzel tracking-wider transition-colors ${
            mode === 'do' ? 'bg-amber-600 text-stone-950 font-bold' : 'bg-stone-800 text-stone-400 hover:bg-stone-700'
          }`}
        >
          <Zap className="w-3 h-3" />
          <span>DO</span>
        </button>
        <button
          type="button"
          onClick={() => setMode('say')}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-cinzel tracking-wider transition-colors ${
            mode === 'say' ? 'bg-amber-600 text-stone-950 font-bold' : 'bg-stone-800 text-stone-400 hover:bg-stone-700'
          }`}
        >
          <MessageSquare className="w-3 h-3" />
          <span>SAY</span>
        </button>
        <button
          type="button"
          onClick={() => setMode('story')}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-cinzel tracking-wider transition-colors ${
            mode === 'story' ? 'bg-amber-600 text-stone-950 font-bold' : 'bg-stone-800 text-stone-400 hover:bg-stone-700'
          }`}
        >
          <Compass className="w-3 h-3" />
          <span>STORY</span>
        </button>
        <button
          type="button"
          onClick={() => setMode('roll')}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-cinzel tracking-wider transition-colors ${
            mode === 'roll' ? 'bg-amber-600 text-stone-950 font-bold' : 'bg-stone-800 text-stone-400 hover:bg-stone-700'
          }`}
        >
          <Dices className="w-3 h-3" />
          <span>ROLL</span>
        </button>
      </div>

      {/* Input Bar with STT and Submit */}
      <form onSubmit={handleSubmit} className="flex items-center gap-2">
        <input
          type="text"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={
            mode === 'do' ? 'Describe your action...' :
            mode === 'say' ? 'What do you speak aloud?' :
            mode === 'story' ? 'Director note / narrative steering...' : 'Enter dice expression (e.g. 1d20+5)...'
          }
          disabled={disabled}
          className="flex-1 bg-stone-950 border border-stone-800 rounded-lg px-4 py-2.5 text-stone-200 placeholder-stone-600 focus:outline-none focus:border-amber-600 font-sans text-base transition-colors"
        />
        <button
          type="button"
          className="p-2.5 rounded-lg bg-stone-800 hover:bg-stone-700 text-stone-400 hover:text-stone-200 transition-colors"
          title="Speech-to-Text Voice Input"
        >
          <Mic className="w-5 h-5" />
        </button>
        <button
          type="submit"
          disabled={disabled || !text.trim()}
          className="px-5 py-2.5 rounded-lg bg-amber-600 hover:bg-amber-500 disabled:opacity-50 disabled:cursor-not-allowed text-stone-950 font-cinzel font-bold flex items-center gap-1.5 transition-colors"
        >
          <span>Submit</span>
          <Send className="w-4 h-4" />
        </button>
      </form>
    </div>
  );
};
```

- [ ] **Step 3: Build verification**

Run: `cd frontend && npm run build`  
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/ChronicleView.tsx frontend/src/components/ActionConsole.tsx
git commit -m "feat(gui): implement Chronicle reader and action console components"
```

---

### Task 5: Flyout Drawers (Character Sheet, Knowledge Graph, Codex, Living World)

**Files:**
- Create: `frontend/src/components/Drawers.tsx`
- Create: `frontend/src/components/CharacterSheetDrawer.tsx`
- Create: `frontend/src/components/GraphDrawer.tsx`
- Create: `frontend/src/components/CodexDrawer.tsx`
- Create: `frontend/src/components/LivingWorldDrawer.tsx`
- Create: `frontend/src/App.tsx`
- Create: `frontend/src/main.tsx`

- [ ] **Step 1: Implement Drawers and individual panels**

Write `frontend/src/components/CharacterSheetDrawer.tsx`:
```tsx
import React from 'react';
import { PlayerState } from '../types';
import { Shield, Heart, Zap } from 'lucide-react';

interface CharacterSheetDrawerProps {
  player?: PlayerState;
}

export const CharacterSheetDrawer: React.FC<CharacterSheetDrawerProps> = ({ player }) => {
  if (!player) return <div className="p-6 text-stone-500">No character loaded.</div>;

  const hp = (player.state?.hp as number) ?? 20;
  const maxHp = (player.state?.max_hp as number) ?? 20;
  const level = (player.state?.level as number) ?? 1;

  return (
    <div className="space-y-6">
      <div className="border-b border-stone-800 pb-4">
        <h2 className="text-2xl font-cinzel text-amber-400 font-bold">{player.name}</h2>
        <span className="text-xs font-mono uppercase tracking-widest text-stone-400">Level {level} {player.type}</span>
      </div>

      {/* HP Bar */}
      <div className="space-y-1.5">
        <div className="flex justify-between text-xs font-cinzel text-stone-300">
          <span className="flex items-center gap-1"><Heart className="w-3.5 h-3.5 text-red-500" /> Health</span>
          <span>{hp} / {maxHp}</span>
        </div>
        <div className="w-full h-3 bg-stone-800 rounded-full overflow-hidden">
          <div
            className="h-full bg-red-600 transition-all duration-300"
            style={{ width: `${Math.min(100, (hp / maxHp) * 100)}%` }}
          />
        </div>
      </div>

      {/* Dynamic Attributes Grid */}
      <div className="grid grid-cols-2 gap-3">
        {Object.entries(player.state || {}).map(([key, val]) => {
          if (key === 'hp' || key === 'max_hp' || key === 'level') return null;
          return (
            <div key={key} className="bg-stone-900 p-3 rounded-lg border border-stone-800">
              <span className="text-xs uppercase text-stone-500 tracking-wider block font-cinzel">{key}</span>
              <span className="text-lg font-bold text-stone-200 font-mono">{String(val)}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
};
```

Write `frontend/src/components/GraphDrawer.tsx`:
```tsx
import React, { useEffect, useRef } from 'react';
import { GraphData } from '../types';

interface GraphDrawerProps {
  data?: GraphData;
  onSelectNode: (nodeId: string) => void;
}

export const GraphDrawer: React.FC<GraphDrawerProps> = ({ data, onSelectNode }) => {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !data) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    ctx.clearRect(0, 0, canvas.width, canvas.height);

    // Simple circular layout for nodes
    const centerX = canvas.width / 2;
    const centerY = canvas.height / 2;
    const radius = Math.min(centerX, centerY) - 40;
    const nodeCoords: Record<string, { x: number; y: number }> = {};

    data.nodes.forEach((node, i) => {
      const angle = (i / (data.nodes.length || 1)) * 2 * Math.PI;
      nodeCoords[node.id] = {
        x: centerX + radius * Math.cos(angle),
        y: centerY + radius * Math.sin(angle),
      };
    });

    // Draw links
    ctx.strokeStyle = 'rgba(120, 113, 108, 0.4)';
    ctx.lineWidth = 1.5;
    data.links.forEach((link) => {
      const src = nodeCoords[link.Source || link.source];
      const dst = nodeCoords[link.Target || link.target];
      if (src && dst) {
        ctx.beginPath();
        ctx.moveTo(src.x, src.y);
        ctx.lineTo(dst.x, dst.y);
        ctx.stroke();
      }
    });

    // Draw nodes
    data.nodes.forEach((node) => {
      const pos = nodeCoords[node.id];
      if (!pos) return;

      ctx.fillStyle = node.type === 'character' ? '#f59e0b' : node.type === 'npc' ? '#38bdf8' : '#a855f7';
      ctx.beginPath();
      ctx.arc(pos.x, pos.y, 8, 0, 2 * Math.PI);
      ctx.fill();

      ctx.fillStyle = '#e7e5e4';
      ctx.font = '10px Cinzel, serif';
      ctx.textAlign = 'center';
      ctx.fillText(node.label || node.id, pos.x, pos.y - 12);
    });
  }, [data]);

  return (
    <div className="space-y-4">
      <h3 className="text-lg font-cinzel text-amber-400 font-bold">Knowledge Graph</h3>
      <div className="bg-stone-900 rounded-lg p-2 border border-stone-800 flex justify-center">
        <canvas ref={canvasRef} width={340} height={340} className="rounded" />
      </div>
      <div className="text-xs text-stone-500 font-mono">
        Color legend: Gold (Player), Blue (NPC), Purple (Location/Item)
      </div>
    </div>
  );
};
```

Write `frontend/src/components/CodexDrawer.tsx`:
```tsx
import React, { useState, useEffect } from 'react';
import { EntityNote } from '../types';
import { Save } from 'lucide-react';

interface CodexDrawerProps {
  entity?: EntityNote;
  onSave: (entityId: string, markdown: string) => void;
}

export const CodexDrawer: React.FC<CodexDrawerProps> = ({ entity, onSave }) => {
  const [markdown, setMarkdown] = useState('');

  useEffect(() => {
    if (entity) setMarkdown(entity.markdown);
  }, [entity]);

  if (!entity) {
    return <div className="p-6 text-stone-500">Select an entity or [[wikilink]] to view notes.</div>;
  }

  return (
    <div className="space-y-4 flex flex-col h-full">
      <div className="flex items-center justify-between border-b border-stone-800 pb-3">
        <div>
          <h2 className="text-xl font-cinzel text-amber-400 font-bold">{entity.name}</h2>
          <span className="text-xs font-mono uppercase text-stone-500">{entity.type}</span>
        </div>
        <button
          onClick={() => onSave(entity.id, markdown)}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded bg-amber-600 hover:bg-amber-500 text-stone-950 font-cinzel font-bold text-xs"
        >
          <Save className="w-3.5 h-3.5" />
          <span>Save</span>
        </button>
      </div>

      <textarea
        value={markdown}
        onChange={(e) => setMarkdown(e.target.value)}
        className="w-full flex-1 min-h-[300px] bg-stone-900 border border-stone-800 rounded-lg p-3 font-mono text-xs text-stone-300 focus:outline-none focus:border-amber-600"
      />

      {entity.backlinks && entity.backlinks.length > 0 && (
        <div className="pt-2 border-t border-stone-800">
          <span className="text-xs font-cinzel text-stone-400 block mb-1">Referenced By (Backlinks):</span>
          <div className="flex flex-wrap gap-1.5">
            {entity.backlinks.map((link) => (
              <span key={link} className="text-xs bg-stone-900 border border-stone-800 px-2 py-0.5 rounded text-amber-400">
                [[{link}]]
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};
```

Write `frontend/src/components/LivingWorldDrawer.tsx`:
```tsx
import React from 'react';
import { GameState } from '../types';
import { Clock, BookOpen } from 'lucide-react';

interface LivingWorldDrawerProps {
  state?: GameState;
}

export const LivingWorldDrawer: React.FC<LivingWorldDrawerProps> = ({ state }) => {
  return (
    <div className="space-y-6">
      <div className="space-y-3">
        <h3 className="text-sm font-cinzel text-amber-400 font-bold uppercase tracking-wider flex items-center gap-1.5">
          <BookOpen className="w-4 h-4" />
          <span>Active Narrative Arcs</span>
        </h3>
        {state?.arcs?.length === 0 ? (
          <p className="text-stone-500 text-xs italic">No active world arcs registered.</p>
        ) : (
          state?.arcs?.map((arc) => (
            <div key={arc.id} className="bg-stone-900 p-3 rounded-lg border border-stone-800 space-y-1">
              <div className="flex justify-between text-xs font-cinzel">
                <span className="text-stone-300">{arc.name}</span>
                <span className="text-amber-500">{arc.progress}/{arc.max_progress}</span>
              </div>
              <div className="w-full h-1.5 bg-stone-800 rounded-full overflow-hidden">
                <div
                  className="h-full bg-amber-600"
                  style={{ width: `${(arc.progress / arc.max_progress) * 100}%` }}
                />
              </div>
            </div>
          ))
        )}
      </div>

      <div className="space-y-3">
        <h3 className="text-sm font-cinzel text-amber-400 font-bold uppercase tracking-wider flex items-center gap-1.5">
          <Clock className="w-4 h-4" />
          <span>Faction Clocks</span>
        </h3>
        {state?.clocks?.length === 0 ? (
          <p className="text-stone-500 text-xs italic">All factions are quiet in the shadows.</p>
        ) : (
          state?.clocks?.map((clock, i) => (
            <div key={i} className="bg-stone-900 p-3 rounded-lg border border-stone-800 space-y-1">
              <div className="flex justify-between text-xs font-cinzel">
                <span className="text-stone-300">{clock.name} ({clock.faction})</span>
                <span className="text-red-400">{clock.ticks}/{clock.max_ticks}</span>
              </div>
              <div className="w-full h-1.5 bg-stone-800 rounded-full overflow-hidden">
                <div
                  className="h-full bg-red-600"
                  style={{ width: `${(clock.ticks / clock.max_ticks) * 100}%` }}
                />
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
```

Write `frontend/src/components/Drawers.tsx`:
```tsx
import React from 'react';
import { X } from 'lucide-react';

interface DrawersProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
}

export const Drawers: React.FC<DrawersProps> = ({ isOpen, onClose, title, children }) => {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/60 backdrop-blur-xs transition-opacity duration-300">
      <div className="w-full max-w-md bg-glass-drawer h-full p-6 shadow-2xl flex flex-col transform transition-transform duration-300">
        <div className="flex items-center justify-between pb-4 border-b border-white/10 mb-4">
          <span className="font-cinzel text-amber-400 font-bold tracking-wider text-base">{title}</span>
          <button onClick={onClose} className="p-1 hover:text-amber-300 cursor-pointer text-stone-400 transition-colors">
            <X className="w-5 h-5" />
          </button>
        </div>
        <div className="flex-1 overflow-y-auto pr-1">
          {children}
        </div>
      </div>
    </div>
  );
};
```

Write `frontend/src/App.tsx`:
```tsx
import React, { useState, useEffect } from 'react';
import { APIClient } from './api/client';
import { GameState, Turn, EntityNote, GraphData } from './types';
import { ChronicleView } from './components/ChronicleView';
import { ActionConsole } from './components/ActionConsole';
import { Drawers } from './components/Drawers';
import { CharacterSheetDrawer } from './components/CharacterSheetDrawer';
import { GraphDrawer } from './components/GraphDrawer';
import { CodexDrawer } from './components/CodexDrawer';
import { LivingWorldDrawer } from './components/LivingWorldDrawer';
import { User, Network, BookOpen, Clock } from 'lucide-react';

export const App: React.FC = () => {
  const [client] = useState(() => new APIClient('test-campaign'));
  const [gameState, setGameState] = useState<GameState | null>(null);
  const [chronicle, setChronicle] = useState<Turn[]>([]);
  const [graph, setGraph] = useState<GraphData | null>(null);
  const [selectedEntity, setSelectedEntity] = useState<EntityNote | null>(null);

  // Active drawer tab: null, 'character', 'graph', 'codex', 'world'
  const [activeDrawer, setActiveDrawer] = useState<string | null>(null);

  useEffect(() => {
    client.getGameState().then(setGameState).catch(console.error);
    client.getChronicle().then(setChronicle).catch(console.error);
    client.getGraph().then(setGraph).catch(console.error);
  }, [client]);

  const handleOpenWikilink = async (entityId: string) => {
    try {
      const ent = await client.getEntity(entityId);
      setSelectedEntity(ent);
      setActiveDrawer('codex');
    } catch (err) {
      console.error(err);
    }
  };

  const handleActionSubmit = async (mode: string, text: string) => {
    // Add optimistic turn
    const nextTurn: Turn = {
      turn_number: chronicle.length + 1,
      input_text: text,
      mode: mode,
      prose: 'The storyteller ponders your directive...',
    };
    setChronicle((prev) => [...prev, nextTurn]);
  };

  const handleSaveEntity = async (entityId: string, markdown: string) => {
    await client.saveEntity(entityId, markdown);
    const updated = await client.getEntity(entityId);
    setSelectedEntity(updated);
  };

  // Find latest scene image for full-window atmospheric background
  const activeBgImage = chronicle.slice().reverse().find((t) => t.image_url)?.image_url;

  return (
    <div className="relative flex flex-col h-screen overflow-hidden text-stone-200">
      {/* Full-window atmospheric background layer (Twintail Launcher aesthetic) */}
      <div
        id="app-bg"
        key={activeBgImage || 'default'}
        className="fixed inset-0 bg-cover bg-center bg-no-repeat animate-bg-fade-in transition-all duration-700 pointer-events-none"
        style={{
          backgroundImage: activeBgImage ? `url(${activeBgImage})` : 'radial-gradient(ellipse at center, #261e1b 0%, #0c0a09 100%)',
          backgroundColor: '#0c0a09',
        }}
      />

      {/* Cinematic dark vignette and noise overlays */}
      <div className="fixed inset-0 bg-gradient-to-b from-black/70 via-black/45 to-black/85 pointer-events-none" />
      <div className="fixed inset-0 bg-radial-[circle_at_center] from-transparent via-black/30 to-black/90 pointer-events-none bg-noise" />

      {/* Floating Translucent Acrylic Header */}
      <header className="relative z-10 mx-6 mt-4 mb-2 h-14 bg-glass rounded-2xl px-6 flex items-center justify-between shadow-2xl">
        <div className="flex items-center gap-3">
          <div className="w-2.5 h-2.5 rounded-full bg-amber-500 shadow-[0_0_8px_rgba(245,158,11,0.8)]" />
          <h1 className="font-cinzel text-lg font-bold text-amber-400 tracking-wider">
            {gameState?.game_name || 'LocalRPG'}
          </h1>
        </div>

        {/* Floating Drawer Trigger Pills */}
        <div className="flex items-center gap-2">
          <button
            onClick={() => setActiveDrawer('character')}
            className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
              activeDrawer === 'character' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
            }`}
          >
            <User className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">Character</span>
          </button>
          <button
            onClick={() => setActiveDrawer('graph')}
            className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
              activeDrawer === 'graph' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
            }`}
          >
            <Network className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">Graph</span>
          </button>
          <button
            onClick={() => setActiveDrawer('codex')}
            className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
              activeDrawer === 'codex' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
            }`}
          >
            <BookOpen className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">Codex</span>
          </button>
          <button
            onClick={() => setActiveDrawer('world')}
            className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
              activeDrawer === 'world' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
            }`}
          >
            <Clock className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">World Arcs</span>
          </button>
        </div>
      </header>

      {/* Main Floating Translucent Chronicle & Action Container */}
      <main className="relative z-10 flex-1 overflow-hidden mx-6 mb-4 flex flex-col">
        <div className="flex-1 bg-glass-card rounded-2xl flex flex-col overflow-hidden shadow-2xl">
          <ChronicleView turns={chronicle} onWikilinkClick={handleOpenWikilink} />
          <ActionConsole onSubmit={handleActionSubmit} />
        </div>
      </main>

      {/* Flyout Drawer Modal */}
      <Drawers
        isOpen={activeDrawer !== null}
        onClose={() => setActiveDrawer(null)}
        title={
          activeDrawer === 'character' ? 'Character Sheet' :
          activeDrawer === 'graph' ? 'Lore Graph' :
          activeDrawer === 'codex' ? 'Codex Markdown Editor' : 'Living World Arcs & Clocks'
        }
      >
        {activeDrawer === 'character' && <CharacterSheetDrawer player={gameState?.player} />}
        {activeDrawer === 'graph' && <GraphDrawer data={graph || undefined} onSelectNode={handleOpenWikilink} />}
        {activeDrawer === 'codex' && <CodexDrawer entity={selectedEntity || undefined} onSave={handleSaveEntity} />}
        {activeDrawer === 'world' && <LivingWorldDrawer state={gameState || undefined} />}
      </Drawers>
    </div>
  );
};
```

Write `frontend/src/main.tsx`:
```tsx
import React from 'react';
import ReactDOM from 'react-dom/client';
import { App } from './App';
import './index.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
```

- [ ] **Step 2: Build frontend distribution**

Run: `cd frontend && npm run build`  
Expected: PASS (generates `frontend/dist/index.html` and assets)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/
git commit -m "feat(gui): implement Option C flyout drawers and complete React 19 UI"
```

---

### Task 6: CLI GUI Command & Asset Embedding

**Files:**
- Create: `pkg/gui/assets.go`
- Create: `cmd/localrpg/gui.go`
- Modify: `cmd/localrpg/main.go`
- Test: `cmd/localrpg/gui_test.go`

- [ ] **Step 1: Write integration test for CLI GUI command**

```go
// cmd/localrpg/gui_test.go
package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIGUICommandHelp(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "gui", "--help")
	out, err := cmd.CombinedOutput()
	// flag.ExitOnError or help returns 0 or 2 depending on flags
	output := string(out)
	if !strings.Contains(output, "Launch desktop GUI or browser app") && !strings.Contains(output, "Usage of gui") {
		t.Errorf("unexpected output: %s, err: %v", output, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/localrpg/... -v -run TestCLIGUICommandHelp`  
Expected: FAIL (gui command unknown or not handled)

- [ ] **Step 3: Implement Asset Embed and CLI GUI handler**

Write `pkg/gui/assets.go`:
```go
package gui

import (
	"embed"
	"io/fs"
	"net/http"
)

// In development or when built without bundled assets, returns a fallback handler
func FallbackAssetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!DOCTYPE html><html><body><h1>LocalRPG GUI Backend Running</h1><p>Vite dev server or frontend bundle active.</p></body></html>"))
	})
}

// FSAssetHandler returns an http.Handler serving an embedded filesystem
func FSAssetHandler(embeddedFS embed.FS, subDir string) http.Handler {
	sub, err := fs.Sub(embeddedFS, subDir)
	if err != nil {
		return FallbackAssetHandler()
	}
	return http.FileServer(http.FS(sub))
}
```

Write `cmd/localrpg/gui.go`:
```go
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func handleGUICommand(args []string) {
	fs := flag.NewFlagSet("gui", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: localrpg gui [flags]")
		fmt.Fprintln(os.Stderr, "Launch desktop GUI or browser app")
		fs.PrintDefaults()
	}
	port := fs.Int("port", 8080, "Port for GUI web server")
	dir := fs.String("dir", ".", "Project root directory")
	fs.Parse(args)

	svc := gui.NewService(*dir)
	server := gui.NewServer(svc, gui.FallbackAssetHandler())

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	fmt.Printf("Starting LocalRPG GUI on http://%s\n", addr)
	if err := http.ListenAndServe(addr, server); err != nil {
		fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
		os.Exit(1)
	}
}
```

Update `cmd/localrpg/main.go` to route `case "gui": handleGUICommand(args[1:])`.

- [ ] **Step 4: Run all package tests across workspace**

Run: `go test -count=1 ./... -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/assets.go cmd/localrpg/gui.go cmd/localrpg/main.go cmd/localrpg/gui_test.go
git commit -m "feat(cli): add 'gui' command connecting backend server and frontend assets"
```
