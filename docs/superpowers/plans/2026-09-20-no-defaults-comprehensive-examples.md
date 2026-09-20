# Zero-Default Systems & Worlds with Comprehensive Reference Examples Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove all hardcoded default systems and worlds, provide a comprehensive reference implementation (*Narrative 2d6 Engine* & *The Ashen Reach* with rules/lore prompts, action hooks, and starter entities) in the Studios, and inject these prompts into the AI storytelling agent's context.

**Architecture:**
- Backend (`pkg/gui`, `pkg/engine`, `pkg/harness`): Add `RulesPrompt` to system DTOs and `LorePrompt` to world DTOs. Update `ContextAssembler` and `TurnOrchestrator` to inject `prompts/rules.md` and `prompts/lore.md` into the agent's prompt layers.
- Frontend Templates (`frontend/src/templates/referenceTemplates.ts`): Define production-grade reference templates for *Narrative 2d6* and *The Ashen Reach*.
- UI Studios (`SystemsStudio.tsx` & `WorldsStudio.tsx`): Add sub-tabs for authoring `rules.md` and `lore.md`, pre-fill new items with reference templates, and add "Reset to Reference Template" buttons.
- Launcher Hub (`LauncherHub.tsx`): Strip all hardcoded `'daggerheart'` and `'solitary_defiance'` references, handling zero-item states gracefully.

**Tech Stack:** Go 1.27.1, React 19, TypeScript, Tailwind CSS v4, Lucide React, Wails v3.

---

### Task 1: Backend DTOs & Service Layer Support for Rules & Lore Prompts

**Files:**
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/server_test.go`

- [x] **Step 1: Write failing tests for RulesPrompt and LorePrompt in `pkg/gui/server_test.go`**

Update `TestSystemAndWorldStudioCRUD` in `pkg/gui/server_test.go` to assert `rules_prompt` on system creation/retrieval and `lore_prompt` on world creation/retrieval:
```go
	// 1. Create a new system via POST /api/systems with rules_prompt
	sysPayload := `{
		"name": "Custom 2d6",
		"version": "1.0.0",
		"description": "Narrative two-dice resolution",
		"rules_prompt": "Evaluate rolls on a 2d6 ladder. 10+ is full success, 7-9 is partial success, 6- is failure.",
		"script": "function evaluateRoll(stats, dice) { return { total: 12 }; }"
	}`
...
	if fetchedSys.RulesPrompt == "" || !strings.Contains(fetchedSys.RulesPrompt, "2d6 ladder") {
		t.Errorf("expected rules_prompt in system detail, got: %s", fetchedSys.RulesPrompt)
	}
...
	// 3. Create a new world via POST /api/worlds with lore_prompt
	worldPayload := `{
		"name": "The Sunken Bastion",
		"description": "An underwater gothic citadel",
		"genre": "Aquatic Gothic",
		"default_system": "custom-2d6",
		"art_style": "Moody oil painting with deep teal and amber lighting",
		"lore_prompt": "The sunken citadel smells of brine and ancient kelp.",
		"tags": ["gothic", "ocean"]
	}`
...
	if fetchedWorld.LorePrompt == "" || !strings.Contains(fetchedWorld.LorePrompt, "sunken citadel") {
		t.Errorf("expected lore_prompt in world detail, got: %s", fetchedWorld.LorePrompt)
	}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/gui -run TestSystemAndWorldStudioCRUD`  
Expected: FAIL (compilation error: unknown field `RulesPrompt` or `LorePrompt`)

- [x] **Step 3: Update DTOs in `pkg/gui/types.go`**

Add `RulesPrompt` to `SystemDetailDTO` and `CreateSystemRequestDTO`:
```go
type SystemDetailDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Script      string `json:"script"`
	RulesPrompt string `json:"rules_prompt"`
}

type CreateSystemRequestDTO struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Script      string `json:"script,omitempty"`
	RulesPrompt string `json:"rules_prompt,omitempty"`
}
```

Add `LorePrompt` to `WorldDetailDTO` and `CreateWorldRequestDTO`:
```go
type WorldDetailDTO struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description"`
	Genre         string                  `json:"genre"`
	DefaultSystem string                  `json:"default_system"`
	ArtStyle      string                  `json:"art_style"`
	Tags          []string                `json:"tags"`
	LorePrompt    string                  `json:"lore_prompt"`
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
	LorePrompt    string   `json:"lore_prompt,omitempty"`
}
```

- [x] **Step 4: Update `GetSystem`, `SaveSystem`, `GetWorld`, and `SaveWorld` in `pkg/gui/service.go`**

In `GetSystem`:
Read `filepath.Join(sysDir, "prompts", "rules.md")` if it exists and populate `RulesPrompt`.

In `SaveSystem`:
If `req.RulesPrompt != ""`, create `filepath.Join(sysDir, "prompts")` and write to `rules.md`.

In `GetWorld`:
Read `filepath.Join(worldDir, "prompts", "lore.md")` if it exists and populate `LorePrompt`.

In `SaveWorld`:
If `req.LorePrompt != ""`, create `filepath.Join(worldDir, "prompts")` and write to `lore.md`.

- [x] **Step 5: Run tests to verify they pass**

Run: `go test -v ./pkg/gui -run TestSystemAndWorldStudioCRUD`  
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/server_test.go
git commit -m "feat(gui): support rules_prompt and lore_prompt in backend studio service"
```

---

### Task 2: Engine Context Assembler & Turn Orchestrator Prompt Injection

**Files:**
- Modify: `pkg/harness/context.go`
- Modify: `pkg/harness/context_test.go`
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/engine/orchestrator_test.go`

- [x] **Step 1: Write failing tests in `pkg/harness/context_test.go`**

Add test checking `rulesPrompt` and `lorePrompt` in assembled context:
```go
func TestContextAssemblerWithRulesAndLore(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := storage.NewStore(filepath.Join(tmpDir, "test.db"))
	defer store.Close()

	assembler := NewContextAssembler(store)
	rulesPrompt := "Resolution: 10+ Success, 7-9 Mixed, 6- Failure."
	lorePrompt := "Atmosphere: Cold mist and distant bells."

	prompt, err := assembler.AssembleContextWithRules("loc1", "p1", "I inspect the door", rulesPrompt, lorePrompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "## SYSTEM RULES & RESOLUTION MECHANICS") || !strings.Contains(prompt, rulesPrompt) {
		t.Errorf("expected rules prompt in context, got: %s", prompt)
	}
	if !strings.Contains(prompt, "## WORLD LORE & ATMOSPHERE") || !strings.Contains(prompt, lorePrompt) {
		t.Errorf("expected lore prompt in context, got: %s", prompt)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/harness -run TestContextAssemblerWithRulesAndLore`  
Expected: FAIL (`AssembleContextWithRules` undefined)

- [x] **Step 3: Implement `AssembleContextWithRules` in `pkg/harness/context.go`**

In `pkg/harness/context.go`:
```go
func (c *ContextAssembler) AssembleContextWithRules(locationID, playerID, playerAction, rulesPrompt, lorePrompt string) (string, error) {
	var sb strings.Builder

	if strings.TrimSpace(rulesPrompt) != "" {
		sb.WriteString("## SYSTEM RULES & RESOLUTION MECHANICS\n")
		sb.WriteString(strings.TrimSpace(rulesPrompt) + "\n\n")
	}

	if strings.TrimSpace(lorePrompt) != "" {
		sb.WriteString("## WORLD LORE & ATMOSPHERE\n")
		sb.WriteString(strings.TrimSpace(lorePrompt) + "\n\n")
	}

	baseContext, err := c.AssembleContext(locationID, playerID, playerAction)
	if err != nil {
		return "", err
	}
	sb.WriteString(baseContext)
	return sb.String(), nil
}
```

- [x] **Step 4: Update `pkg/engine/orchestrator.go` to load prompts and pass them to assembler**

In `pkg/engine/orchestrator.go`:
Store `systemDir` and `worldDir` or `paths *core.PathResolver` in `TurnOrchestrator`.
In `ProcessAction`:
Load `filepath.Join(o.systemDir, "prompts", "rules.md")` if existing.
Load `filepath.Join(o.worldDir, "prompts", "lore.md")` if existing.
Call `o.assembler.AssembleContextWithRules(...)`.

- [x] **Step 5: Run tests across harness and engine**

Run: `go test -v ./pkg/harness ./pkg/engine`  
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add pkg/harness/context.go pkg/harness/context_test.go pkg/engine/orchestrator.go pkg/engine/orchestrator_test.go
git commit -m "feat(engine): inject system rules prompt and world lore prompt into GM agent context"
```

---

### Task 3: Comprehensive Reference Template Definitions & Frontend API Types

**Files:**
- Modify: `frontend/src/types.ts`
- Create: `frontend/src/templates/referenceTemplates.ts`

- [x] **Step 1: Update frontend types in `frontend/src/types.ts`**

Add `rules_prompt` to `SystemDetail` & `CreateSystemRequest`:
```typescript
export interface SystemDetail {
  id: string;
  name: string;
  version: string;
  description: string;
  script: string;
  rules_prompt?: string;
}

export interface CreateSystemRequest {
  id?: string;
  name: string;
  version?: string;
  description?: string;
  script?: string;
  rules_prompt?: string;
}
```

Add `lore_prompt` to `WorldDetail` & `CreateWorldRequest`:
```typescript
export interface WorldDetail {
  id: string;
  name: string;
  description: string;
  genre: string;
  default_system: string;
  art_style: string;
  tags: string[];
  lore_prompt?: string;
  entities: WorldEntitySummary[];
}

export interface CreateWorldRequest {
  id?: string;
  name: string;
  description?: string;
  genre?: string;
  default_system?: string;
  art_style?: string;
  tags?: string[];
  lore_prompt?: string;
}
```

- [x] **Step 2: Create `frontend/src/templates/referenceTemplates.ts`**

Export:
- `REFERENCE_SYSTEM_TEMPLATE`:
  - `id`: `"narrative_2d6"`
  - `name`: `"Narrative 2d6 Engine"`
  - `version`: `"1.0.0"`
  - `description`: `"Versatile 2d6 resolution with partial success, dynamic stress, and event hooks."`
  - `rules_prompt`: Complete Markdown instructions covering resolution ladder (10+ Strong Hit, 7-9 Weak Hit, 6- Miss), action modes (`do`, `attack`, `say`, `roll`), and instructions on how to adjudicate and reflect `[MECHANICS RESULT: ...]`.
  - `script`: Complete JavaScript implementation of `onAction("do")`, `onAction("attack")`, `roll("2d6")`, `getStat`, `setStat`, `injectGMDirection`, and `onTurnEnd`.
- `REFERENCE_WORLD_TEMPLATE`:
  - `id`: `"the_ashen_reach"`
  - `name`: `"The Ashen Reach"`
  - `genre`: `"Dark Fantasy / Gothic Exploration"`
  - `default_system`: `"narrative_2d6"`
  - `art_style`: `"Oil on textured canvas, chiaroscuro lighting, deep umber and lantern gold, atmospheric fog, classical dark fantasy illustration"`
  - `tags`: `["gothic", "dark_fantasy", "mystery", "ruins"]`
  - `description`: `"A mist-veiled frontier of shattered cathedral keeps, lingering ember magic, and sunken peat bogs."`
  - `lore_prompt`: Setting sensory guidelines, tone cues, and background factions.
  - `entities`: Array of 3 starter entity templates:
    1. `the_ashen_bastion` (type: location)
    2. `wardens_of_the_ember` (type: character)
    3. `the_creeping_miasma` (type: arc with 6-tick clock)

- [x] **Step 3: Run TypeScript compiler check**

Run: `mise run test:frontend`  
Expected: PASS with 0 errors

- [x] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/templates/referenceTemplates.ts
git commit -m "feat(frontend): define comprehensive reference templates for narrative 2d6 system and ashen reach world"
```

---

### Task 4: Systems Workshop UI with Rules Prompt Tab & Template Reset

**Files:**
- Modify: `frontend/src/components/SystemsStudio.tsx`

- [x] **Step 1: Add Rules Prompt Tab & Reference Template Integration to `SystemsStudio.tsx`**

- Add state for `rulesPrompt` (`string`).
- Add 3rd sub-tab button: "Agent Rules Prompt (`rules.md`)" next to "Manifest Info" and "Mechanics Script".
- Sub-tab render: Markdown editor textarea for `rulesPrompt` with syntax and styling.
- Update `handleNewSystem`: Pre-populate state from `REFERENCE_SYSTEM_TEMPLATE`.
- Add `handleResetToReference`: A button with `RotateCcw` or `Sparkles` icon labeled "Reset to Reference Template" that reloads `REFERENCE_SYSTEM_TEMPLATE`.
- Update `handleSaveSystem`: Include `rules_prompt: rulesPrompt` in `APIClient.saveSystem(...)`.
- Update `loadSystemDetail`: Set `rulesPrompt` from `detail.rules_prompt || ''`.

- [x] **Step 2: Run TypeScript compiler check**

Run: `mise run test:frontend`  
Expected: PASS with 0 errors

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/SystemsStudio.tsx
git commit -m "feat(frontend): add rules prompt tab and reference template actions to systems studio"
```

---

### Task 5: Worlds Studio UI with Lore Prompt Tab & Template Reset

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`

- [x] **Step 1: Add Lore Prompt Tab & Reference Template Integration to `WorldsStudio.tsx`**

- Add state for `lorePrompt` (`string`).
- Add 3rd sub-tab button: "Agent Lore Prompt (`lore.md`)".
- Sub-tab render: Markdown editor textarea for `lorePrompt`.
- Update `handleNewWorld`: Pre-populate state from `REFERENCE_WORLD_TEMPLATE`.
- Add `handleResetToReference`: Button labeled "Reset to Reference Template" that reloads `REFERENCE_WORLD_TEMPLATE` (including the 3 starter entities).
- Update `handleSaveWorld`: Include `lore_prompt: lorePrompt` in `APIClient.saveWorld(...)`.
- Update `loadWorldDetail`: Set `lorePrompt` from `detail.lore_prompt || ''`.

- [x] **Step 2: Run TypeScript compiler check**

Run: `mise run test:frontend`  
Expected: PASS with 0 errors

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/WorldsStudio.tsx
git commit -m "feat(frontend): add lore prompt tab and reference template actions to worlds studio"
```

---

### Task 6: Launcher Hub Clean-Up & Zero-State Guidance

**Files:**
- Modify: `frontend/src/components/LauncherHub.tsx`

- [x] **Step 1: Strip hardcoded defaults and implement zero-state guidance in `LauncherHub.tsx`**

- Remove `'daggerheart'` and `'solitary_defiance'` hardcoded fallbacks from:
  - `payload.system_id`
  - `payload.world_id`
  - Select dropdown `<option>` tags
- If `systems.length === 0 || worlds.length === 0`:
  - Disable the "Embark on Adventure" button in the wizard.
  - Render an atmospheric warning box inside the wizard:
    `"Before embarking on an adventure, you need at least one Rule System and one World Setting."`
  - Render two button links inside the wizard:
    - `<button onClick={() => { setIsWizardOpen(false); setActiveTab('systems'); }}>Craft Rule System</button>`
    - `<button onClick={() => { setIsWizardOpen(false); setActiveTab('worlds'); }}>Craft World Setting</button>`
- In the main empty state (when 0 games exist):
  - If `systems.length === 0 || worlds.length === 0`, display a prominent prompt inviting the player to visit the Rule Systems and Worlds Studio tabs to create or load the reference templates.

- [x] **Step 2: Test and build frontend bundle**

Run: `mise run test:frontend && mise run build:frontend`  
Expected: PASS, builds cleanly into `pkg/gui/dist`.

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/LauncherHub.tsx
git commit -m "feat(frontend): remove hardcoded defaults and add studio onboarding guidance to launcher hub"
```

---

### Task 7: Full Automated Tests & End-to-End Verification

**Files:**
- Verify: `mise run test`
- Verify: `mise run build`

- [x] **Step 1: Run complete test suite across all 12 Go packages and TypeScript**

Run: `mise run test`  
Expected: PASS across all packages.

- [x] **Step 2: Build binary and run E2E socket verification**

Run `localrpg gui --socket /tmp/test-ref.sock` in the background and verify:
1. Fresh start: `GET /api/systems` and `GET /api/worlds` return empty arrays `[]` (0 defaults).
2. Save system with `rules_prompt` and verify `GET /api/system/narrative_2d6` returns `rules_prompt` and script.
3. Save world with `lore_prompt` and 3 starter entities.
4. Create game `ashen-chronicle` using `narrative_2d6` and `the_ashen_reach`.
5. Verify game initializes with starter entities copied from `the_ashen_reach`.

- [x] **Step 3: Commit and merge**

```bash
git commit --allow-empty -m "chore: verify zero-defaults and reference templates end-to-end"
```
