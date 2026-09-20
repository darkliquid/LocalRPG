# Systems Workshop & Worlds Studio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide dedicated in-app authoring environments for Rule Systems (`mechanics.js` and manifests) and Worlds Studio (world lore, genres, art style prompts, and starter Markdown entity templates), enabling players to build custom games from scratch and play them in the Launcher Hub.

**Architecture:**
- Backend (`pkg/gui`): Add REST endpoints for fetching and saving systems (`/api/system/:id`), worlds (`/api/world/:id`), and starter entities (`/api/world/:id/entity/:name`).
- Frontend API Client (`frontend/src/api/client.ts`): Add static methods for system, world, and entity template CRUD.
- UI Studios (`frontend/src/components/SystemsStudio.tsx` & `frontend/src/components/WorldsStudio.tsx`): Master-detail authoring studios with code editing and markdown entity drafting.
- Navigation (`frontend/src/components/LauncherHub.tsx`): Add tab navigation between "Campaigns", "Rule Systems", and "Worlds Studio".

**Tech Stack:** Go 1.27.1, React 19, TypeScript, Tailwind CSS v4, Lucide React, Wails v3.

---

### Task 1: Backend System & World Studio CRUD Endpoints

**Files:**
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/server.go`
- Test: `pkg/gui/server_test.go`

- [ ] **Step 1: Write failing tests for System and World CRUD in `pkg/gui/server_test.go`**

Add `TestSystemAndWorldStudioCRUD` to `pkg/gui/server_test.go`:
```go
func TestSystemAndWorldStudioCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)
	server := NewServer(svc, AssetHandler())

	// 1. Create a new system via POST /api/systems
	sysPayload := `{
		"name": "Custom 2d6",
		"version": "1.0.0",
		"description": "Narrative two-dice resolution",
		"script": "function evaluateRoll(stats, dice) { return { total: 12 }; }"
	}`
	reqSys := httptest.NewRequest("POST", "/api/systems", strings.NewReader(sysPayload))
	recSys := httptest.NewRecorder()
	server.ServeHTTP(recSys, reqSys)
	if recSys.Code != http.StatusCreated {
		t.Fatalf("POST /api/systems failed (%d): %s", recSys.Code, recSys.Body.String())
	}

	var createdSys SystemDetailDTO
	_ = json.NewDecoder(recSys.Body).Decode(&createdSys)
	if createdSys.ID != "custom-2d6" || createdSys.Name != "Custom 2d6" {
		t.Errorf("unexpected created system: %+v", createdSys)
	}

	// 2. Fetch system detail via GET /api/system/custom-2d6
	reqGetSys := httptest.NewRequest("GET", "/api/system/custom-2d6", nil)
	recGetSys := httptest.NewRecorder()
	server.ServeHTTP(recGetSys, reqGetSys)
	if recGetSys.Code != http.StatusOK {
		t.Fatalf("GET /api/system/custom-2d6 failed (%d): %s", recGetSys.Code, recGetSys.Body.String())
	}
	var fetchedSys SystemDetailDTO
	_ = json.NewDecoder(recGetSys.Body).Decode(&fetchedSys)
	if !strings.Contains(fetchedSys.Script, "evaluateRoll") {
		t.Errorf("expected script in system detail, got: %s", fetchedSys.Script)
	}

	// 3. Create a new world via POST /api/worlds
	worldPayload := `{
		"name": "The Sunken Bastion",
		"description": "An underwater gothic citadel",
		"genre": "Aquatic Gothic",
		"default_system": "custom-2d6",
		"art_style": "Moody oil painting with deep teal and amber lighting",
		"tags": ["gothic", "ocean"]
	}`
	reqWorld := httptest.NewRequest("POST", "/api/worlds", strings.NewReader(worldPayload))
	recWorld := httptest.NewRecorder()
	server.ServeHTTP(recWorld, reqWorld)
	if recWorld.Code != http.StatusCreated {
		t.Fatalf("POST /api/worlds failed (%d): %s", recWorld.Code, recWorld.Body.String())
	}

	var createdWorld WorldDetailDTO
	_ = json.NewDecoder(recWorld.Body).Decode(&createdWorld)
	if createdWorld.ID != "the-sunken-bastion" || createdWorld.DefaultSystem != "custom-2d6" {
		t.Errorf("unexpected created world: %+v", createdWorld)
	}

	// 4. Create starter entity in world via PUT /api/world/the-sunken-bastion/entity/sunken_throne
	entityMD := "---\nname: The Sunken Throne\ntype: location\n---\nAncient seat of forgotten sea kings."
	reqEnt := httptest.NewRequest("PUT", "/api/world/the-sunken-bastion/entity/sunken_throne", strings.NewReader(entityMD))
	recEnt := httptest.NewRecorder()
	server.ServeHTTP(recEnt, reqEnt)
	if recEnt.Code != http.StatusOK {
		t.Fatalf("PUT world entity failed (%d): %s", recEnt.Code, recEnt.Body.String())
	}

	// 5. Fetch world detail via GET /api/world/the-sunken-bastion
	reqGetWorld := httptest.NewRequest("GET", "/api/world/the-sunken-bastion", nil)
	recGetWorld := httptest.NewRecorder()
	server.ServeHTTP(recGetWorld, reqGetWorld)
	if recGetWorld.Code != http.StatusOK {
		t.Fatalf("GET /api/world failed: %d", recGetWorld.Code)
	}
	var fetchedWorld WorldDetailDTO
	_ = json.NewDecoder(recGetWorld.Body).Decode(&fetchedWorld)
	if len(fetchedWorld.Entities) != 1 || fetchedWorld.Entities[0].ID != "sunken_throne" {
		t.Errorf("expected 1 entity in world detail, got %+v", fetchedWorld.Entities)
	}

	// 6. Fetch entity markdown via GET /api/world/the-sunken-bastion/entity/sunken_throne
	reqGetEnt := httptest.NewRequest("GET", "/api/world/the-sunken-bastion/entity/sunken_throne", nil)
	recGetEnt := httptest.NewRecorder()
	server.ServeHTTP(recGetEnt, reqGetEnt)
	if recGetEnt.Code != http.StatusOK {
		t.Fatalf("GET world entity failed (%d)", recGetEnt.Code)
	}
	var fetchedEnt WorldEntityDetailDTO
	_ = json.NewDecoder(recGetEnt.Body).Decode(&fetchedEnt)
	if !strings.Contains(fetchedEnt.Markdown, "Ancient seat of forgotten sea kings") {
		t.Errorf("unexpected entity markdown: %s", fetchedEnt.Markdown)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/gui -run TestSystemAndWorldStudioCRUD`  
Expected: FAIL with compilation errors (undefined DTOs or methods)

- [ ] **Step 3: Define DTOs in `pkg/gui/types.go`**

Add to `pkg/gui/types.go`:
```go
type SystemDetailDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Script      string `json:"script"`
}

type CreateSystemRequestDTO struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Script      string `json:"script,omitempty"`
}

type WorldEntitySummaryDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type WorldDetailDTO struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description"`
	Genre         string                  `json:"genre"`
	DefaultSystem string                  `json:"default_system"`
	ArtStyle      string                  `json:"art_style"`
	Tags          []string                `json:"tags"`
	Entities      []WorldEntitySummaryDTO `json:"entities"`
}

type CreateWorldRequestDTO struct {
	ID            string   `json:"id,omitempty"`
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Genre         string   `json:"genre,omitempty"`
	DefaultSystem string   `json:"default_system,omitempty"`
	ArtStyle      string   `json:"art_style,omitempty"`
	Tags          []string `json:"tags,omitempty"`
}

type WorldEntityDetailDTO struct {
	ID       string `json:"id"`
	Markdown string `json:"markdown"`
}
```

- [ ] **Step 4: Implement Service methods in `pkg/gui/service.go`**

Add:
- `GetSystem(ctx context.Context, id string) (*SystemDetailDTO, error)`
- `SaveSystem(ctx context.Context, req CreateSystemRequestDTO) (*SystemDetailDTO, error)`
- `GetWorld(ctx context.Context, id string) (*WorldDetailDTO, error)`
- `SaveWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error)`
- `GetWorldEntity(ctx context.Context, worldID, entityID string) (*WorldEntityDetailDTO, error)`
- `SaveWorldEntity(ctx context.Context, worldID, entityID, markdown string) error`
- `DeleteWorldEntity(ctx context.Context, worldID, entityID string) error`

- [ ] **Step 5: Register routes and handlers in `pkg/gui/server.go`**

Register routes:
- `/api/system/`: `handleSystemRoutes` (`GET /api/system/:id`, `PUT /api/system/:id`)
- `/api/systems`: `handleSystemsRoutes` (`GET`, `POST`)
- `/api/world/`: `handleWorldRoutes` (`GET /api/world/:id`, `PUT /api/world/:id`, `/api/world/:id/entity/:name`)
- `/api/worlds`: `handleWorldsRoutes` (`GET`, `POST`)

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test -v ./pkg/gui -run TestSystemAndWorldStudioCRUD`  
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/server.go pkg/gui/server_test.go
git commit -m "feat(gui): implement backend CRUD endpoints for system and world authoring"
```

---

### Task 2: Frontend Types & API Client Studio Methods

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: Add studio types in `frontend/src/types.ts`**

Add `SystemDetail`, `CreateSystemRequest`, `WorldDetail`, `WorldEntitySummary`, `CreateWorldRequest`, `WorldEntityDetail`.

- [ ] **Step 2: Add studio methods to `APIClient` in `frontend/src/api/client.ts`**

Add:
- `APIClient.getSystem(id)`
- `APIClient.saveSystem(req)`
- `APIClient.getWorld(id)`
- `APIClient.saveWorld(req)`
- `APIClient.getWorldEntity(worldId, entityId)`
- `APIClient.saveWorldEntity(worldId, entityId, markdown)`
- `APIClient.deleteWorldEntity(worldId, entityId)`

- [ ] **Step 3: Run TypeScript compiler check**

Run: `npx --prefix frontend tsc --noEmit`  
Expected: PASS with 0 errors

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): add studio API client methods for systems and worlds"
```

---

### Task 3: Systems Workshop UI Component

**Files:**
- Create: `frontend/src/components/SystemsStudio.tsx`

- [ ] **Step 1: Implement `SystemsStudio.tsx`**

Features:
- Left Master Pane:
  - List of systems with version tag and description.
  - "+ New System" button (resets editor).
- Right Detail Pane:
  - Sub-tabs: "Manifest Info" and "Mechanics Script (`mechanics.js`)".
  - Manifest Info inputs: Name, Slug ID, Version, Description.
  - Mechanics Script editor: Monospace dark textarea with starter boilerplate for `evaluateRoll`.
  - Save button with toast notification.
  - Callback `onSystemSaved` to refresh parent state.

- [ ] **Step 2: Run TypeScript check**

Run: `npx --prefix frontend tsc --noEmit`  
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/SystemsStudio.tsx
git commit -m "feat(frontend): implement systems workshop studio component"
```

---

### Task 4: Worlds Studio UI Component

**Files:**
- Create: `frontend/src/components/WorldsStudio.tsx`

- [ ] **Step 1: Implement `WorldsStudio.tsx`**

Features:
- Left Master Pane:
  - List of worlds with genre badge and entity count.
  - "+ New World" button.
- Right Detail Pane:
  - Sub-tabs: "Setting Lore & Atmosphere" and "Starter Entities (`entities/*.md`)".
  - Lore & Atmosphere inputs: Name, Slug ID, Genre, Default System dropdown, Visual Art Style prompt guide, Lore Synopsis.
  - Starter Entities Manager:
    - List of template entities with "+ Add Starter Entity" button.
    - Markdown editor with frontmatter scaffolding (`name`, `type`, `wikilinks`).
    - Save entity and delete entity actions.
  - Save World button with toast notification.
  - Callback `onWorldSaved` to refresh parent state.

- [ ] **Step 2: Run TypeScript check**

Run: `npx --prefix frontend tsc --noEmit`  
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/WorldsStudio.tsx
git commit -m "feat(frontend): implement worlds studio component with entity template editor"
```

---

### Task 5: Launcher Hub Navigation Integration & End-to-End Verification

**Files:**
- Modify: `frontend/src/components/LauncherHub.tsx`
- Verify: `mise run test`
- Verify: `mise run build`

- [ ] **Step 1: Integrate Navigation Tabs into `LauncherHub.tsx`**

Features:
- Add top-level tabs in header:
  - **Campaigns** (Saved games, Resume hero card, New Game modal)
  - **Rule Systems** (`<SystemsStudio />`)
  - **Worlds Studio** (`<WorldsStudio />`)
- When switching tabs, automatically refresh system and world lists so newly authored items appear in wizard dropdowns immediately.

- [ ] **Step 2: Test and build frontend bundle**

Run: `mise run test:frontend && mise run build:frontend`  
Expected: PASS, builds cleanly into `pkg/gui/dist`.

- [ ] **Step 3: Run full automated tests across Go and TypeScript**

Run: `mise run test`  
Expected: PASS across all 12 Go packages and TypeScript checks.

- [ ] **Step 4: End-to-End Verification over Unix Domain Socket**

Test creating a system, world with an entity, and a game using `localrpg gui --socket`:
```bash
mise run build
./bin/localrpg gui --socket /tmp/test-studio.sock &
PID=$!
sleep 1
# 1. Create system
curl -s --unix-socket /tmp/test-studio.sock -X POST -H "Content-Type: application/json" \
  -d '{"name":"Iron Realm","version":"1.0.0","description":"Gritty d20"}' http://localhost/api/systems
# 2. Create world
curl -s --unix-socket /tmp/test-studio.sock -X POST -H "Content-Type: application/json" \
  -d '{"name":"Black Marsh","genre":"Grimdark","default_system":"iron-realm"}' http://localhost/api/worlds
# 3. Create game using new system and world
curl -s --unix-socket /tmp/test-studio.sock -X POST -H "Content-Type: application/json" \
  -d '{"name":"Escape from Black Marsh","system_id":"iron-realm","world_id":"black-marsh","player_name":"Garrick"}' http://localhost/api/games
kill -SIGTERM $PID
rm -f /tmp/test-studio.sock
```
Expected: All requests succeed, game is created from scratch with the newly authored system and world!

- [ ] **Step 5: Commit and merge**

```bash
git add frontend/src/components/LauncherHub.tsx
git commit -m "feat(gui): integrate systems and worlds studio into launcher hub navigation"
```
