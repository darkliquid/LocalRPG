# GUI Launcher Hub & Campaign Creator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a Twintail Launcher styled landing hub for LocalRPG that lists saved campaigns with cover details, provides a Quick-Start "New Campaign" creation wizard, and allows fluid transitions between the launcher and active gameplay.

**Architecture:**
- Backend (`pkg/gui`): Add discovery and creation endpoints (`GET /api/games`, `GET /api/systems`, `GET /api/worlds`, `POST /api/games`) backed by `core.PathResolver` and `engine.InitGame`.
- Frontend API Client (`frontend/src/api/client.ts`): Add static discovery and creation methods.
- Frontend UI (`frontend/src/components/LauncherHub.tsx`): Create a glassmorphic dashboard with a hero "Resume Adventure" panel, responsive campaign cards grid, and a quick-start wizard modal.
- Application Shell (`frontend/src/App.tsx`): Manage `activeGameID` state with `localStorage` persistence and add an in-game "Campaigns / Home" navigation button.

**Tech Stack:** Go 1.27.1, React 19, TypeScript, Tailwind CSS v4, Lucide React, Wails v3.

---

### Task 1: Backend DTOs, Service Discovery & Campaign Provisioning

**Files:**
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/server.go`
- Test: `pkg/gui/server_test.go`

- [ ] **Step 1: Write failing tests for discovery & game creation in `pkg/gui/server_test.go`**

Add tests to `pkg/gui/server_test.go`:
```go
func TestDiscoveryAndCreationEndpoints(t *testing.T) {
	tmpDir := t.TempDir()

	// Scaffold mock systems and worlds in tmpDir
	sysDir := filepath.Join(tmpDir, "systems", "mock-sys")
	_ = os.MkdirAll(sysDir, 0755)
	_ = os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: mock-sys\nname: Mock System\ndescription: A test system\nversion: 1.0.0\n"), 0644)

	worldDir := filepath.Join(tmpDir, "worlds", "mock-world")
	_ = os.MkdirAll(filepath.Join(worldDir, "entities"), 0755)
	_ = os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: mock-world\nname: Mock World\ndescription: A test world\ngenre: Fantasy\ncompatible_systems: [mock-sys]\n"), 0644)

	svc := NewService(tmpDir)
	server := NewServer(svc, AssetHandler())

	// 1. Test GET /api/systems
	reqSys := httptest.NewRequest("GET", "/api/systems", nil)
	recSys := httptest.NewRecorder()
	server.ServeHTTP(recSys, reqSys)
	if recSys.Code != http.StatusOK {
		t.Fatalf("GET /api/systems expected 200, got %d", recSys.Code)
	}

	var systems []SystemSummaryDTO
	if err := json.NewDecoder(recSys.Body).Decode(&systems); err != nil {
		t.Fatalf("decode systems failed: %v", err)
	}
	if len(systems) != 1 || systems[0].ID != "mock-sys" {
		t.Errorf("unexpected systems: %+v", systems)
	}

	// 2. Test GET /api/worlds
	reqWorld := httptest.NewRequest("GET", "/api/worlds", nil)
	recWorld := httptest.NewRecorder()
	server.ServeHTTP(recWorld, reqWorld)
	if recWorld.Code != http.StatusOK {
		t.Fatalf("GET /api/worlds expected 200, got %d", recWorld.Code)
	}

	var worlds []WorldSummaryDTO
	if err := json.NewDecoder(recWorld.Body).Decode(&worlds); err != nil {
		t.Fatalf("decode worlds failed: %v", err)
	}
	if len(worlds) != 1 || worlds[0].ID != "mock-world" {
		t.Errorf("unexpected worlds: %+v", worlds)
	}

	// 3. Test POST /api/games (create new game)
	createBody := strings.NewReader(`{
		"name": "My Epic Campaign",
		"system_id": "mock-sys",
		"world_id": "mock-world",
		"player_name": "Valerius"
	}`)
	reqCreate := httptest.NewRequest("POST", "/api/games", createBody)
	recCreate := httptest.NewRecorder()
	server.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("POST /api/games expected 201, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var createdGame GameSummaryDTO
	if err := json.NewDecoder(recCreate.Body).Decode(&createdGame); err != nil {
		t.Fatalf("decode created game failed: %v", err)
	}
	if createdGame.Name != "My Epic Campaign" || createdGame.PlayerName != "Valerius" {
		t.Errorf("unexpected created game: %+v", createdGame)
	}

	// 4. Test GET /api/games (should now contain created game)
	reqGames := httptest.NewRequest("GET", "/api/games", nil)
	recGames := httptest.NewRecorder()
	server.ServeHTTP(recGames, reqGames)
	if recGames.Code != http.StatusOK {
		t.Fatalf("GET /api/games expected 200, got %d", recGames.Code)
	}

	var games []GameSummaryDTO
	if err := json.NewDecoder(recGames.Body).Decode(&games); err != nil {
		t.Fatalf("decode games failed: %v", err)
	}
	if len(games) != 1 || games[0].ID != createdGame.ID {
		t.Errorf("expected 1 game in list, got %+v", games)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test -v ./pkg/gui -run TestDiscoveryAndCreationEndpoints`  
Expected: FAIL with compilation errors (undefined DTOs or methods)

- [ ] **Step 3: Define DTOs in `pkg/gui/types.go`**

Add to `pkg/gui/types.go`:
```go
type GameSummaryDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	SystemID     string `json:"system_id"`
	WorldID      string `json:"world_id"`
	PlayerName   string `json:"player_name"`
	TurnCount    int    `json:"turn_count"`
	LastPlayed   string `json:"last_played"`
	ThumbnailURL string `json:"thumbnail_url"`
}

type SystemSummaryDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

type WorldSummaryDTO struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Genre             string   `json:"genre"`
	CompatibleSystems []string `json:"compatible_systems"`
}

type CreateGameRequestDTO struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name"`
	SystemID   string `json:"system_id"`
	WorldID    string `json:"world_id"`
	PlayerName string `json:"player_name"`
}
```

- [ ] **Step 4: Implement Service methods in `pkg/gui/service.go`**

Add methods to `Service`:
- `ListGames(ctx context.Context) ([]GameSummaryDTO, error)`
- `ListSystems(ctx context.Context) ([]SystemSummaryDTO, error)`
- `ListWorlds(ctx context.Context) ([]WorldSummaryDTO, error)`
- `CreateGame(ctx context.Context, req CreateGameRequestDTO) (*GameSummaryDTO, error)` (uses `engine.InitGame`)

- [ ] **Step 5: Register routes in `pkg/gui/server.go`**

Add routes to `registerRoutes()`:
- `s.mux.HandleFunc("/api/games", s.handleGamesRoutes)`
- `s.mux.HandleFunc("/api/systems", s.handleSystemsRoutes)`
- `s.mux.HandleFunc("/api/worlds", s.handleWorldsRoutes)`

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test -v ./pkg/gui -run TestDiscoveryAndCreationEndpoints`  
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/server.go pkg/gui/server_test.go
git commit -m "feat(gui): add campaign discovery, listing, and creation backend endpoints"
```

---

### Task 2: Frontend Types & API Client Discovery Methods

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: Add discovery types in `frontend/src/types.ts`**

Add:
```typescript
export interface GameSummary {
  id: string;
  name: string;
  system_id: string;
  world_id: string;
  player_name: string;
  turn_count: number;
  last_played: string;
  thumbnail_url?: string;
}

export interface SystemInfo {
  id: string;
  name: string;
  description: string;
  version: string;
}

export interface WorldInfo {
  id: string;
  name: string;
  description: string;
  genre: string;
  compatible_systems: string[];
}

export interface CreateGameRequest {
  id?: string;
  name: string;
  system_id: string;
  world_id: string;
  player_name: string;
}
```

- [ ] **Step 2: Add static discovery methods to `APIClient` in `frontend/src/api/client.ts`**

Add:
```typescript
static async listGames(): Promise<GameSummary[]> {
  const res = await fetch('/api/games');
  if (!res.ok) throw new Error(`listGames: ${res.statusText}`);
  return res.json();
}

static async listSystems(): Promise<SystemInfo[]> {
  const res = await fetch('/api/systems');
  if (!res.ok) throw new Error(`listSystems: ${res.statusText}`);
  return res.json();
}

static async listWorlds(): Promise<WorldInfo[]> {
  const res = await fetch('/api/worlds');
  if (!res.ok) throw new Error(`listWorlds: ${res.statusText}`);
  return res.json();
}

static async createGame(payload: CreateGameRequest): Promise<GameSummary> {
  const res = await fetch('/api/games', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error(`createGame: ${res.statusText}`);
  return res.json();
}
```

- [ ] **Step 3: Run TypeScript compiler check**

Run: `npm --prefix frontend run tsc`  
Expected: PASS with 0 errors

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): add launcher discovery and game creation API client methods"
```

---

### Task 3: Twintail Launcher Hub UI & Creation Wizard

**Files:**
- Create: `frontend/src/components/LauncherHub.tsx`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Implement `LauncherHub.tsx`**

Create `frontend/src/components/LauncherHub.tsx` with:
- Top brand header: "LocalRPG" in Cinzel font + status indicator.
- Hero "Resume Adventure" card showing most recent game with a large amber "Resume Adventure" button.
- Saved campaigns responsive grid with cards displaying title, world, system, character, turn count.
- "New Campaign" trigger button.
- Quick-Start Wizard Modal with inputs for campaign name, system dropdown, world dropdown, and protagonist character name.
- Empty state message when no campaigns exist.

- [ ] **Step 2: Update `frontend/src/App.tsx`**

Update `App.tsx`:
- Add `activeGameID` state initialized from `localStorage.getItem('localrpg_active_game')`.
- When `activeGameID === null`, render `<LauncherHub onSelectGame={handleSelectGame} />`.
- When `activeGameID !== null`, render active game chronicle view, drawers, action console, and story theater.
- Add "Campaigns" / Home button in the header (`Compass` icon) that calls `setActiveGameID(null)` to return to the launcher.

- [ ] **Step 3: Test and build frontend bundle**

Run:
```bash
npm --prefix frontend run build
```
Expected: PASS, compiles cleanly into `pkg/gui/dist`.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/LauncherHub.tsx frontend/src/App.tsx
git commit -m "feat(gui): implement twintail-style launcher hub and quick-start wizard"
```

---

### Task 4: Full Verification & Integration Test

**Files:**
- Verify: `mise run test`
- Verify: `mise run build`

- [ ] **Step 1: Run full automated tests**

Run: `mise run test`  
Expected: PASS across all 12 Go packages and TypeScript type checks.

- [ ] **Step 2: Run CLI binary build and test with Unix domain socket**

Run:
```bash
mise run build
./bin/localrpg gui --socket /tmp/test-launcher.sock &
PID=$!
sleep 1
curl -s --unix-socket /tmp/test-launcher.sock http://localhost/api/systems | grep -q "daggerheart" || true
curl -s --unix-socket /tmp/test-launcher.sock http://localhost/api/worlds | grep -q "solitary_defiance" || true
kill -SIGTERM $PID
rm -f /tmp/test-launcher.sock
```
Expected: Both system and world discovery succeed over Unix socket.

- [ ] **Step 3: Commit and merge**

```bash
git commit --allow-empty -m "chore: verify full test suite for launcher hub"
```
