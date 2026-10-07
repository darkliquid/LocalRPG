# Reference Systems Corpus Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship three runnable systems (PbtA 2d6, d20+DC, d10 pool) as Go-embedded fixtures, serve them to the studio, and exercise them in tests.

**Architecture:** A new `pkg/refsystems` leaf embeds the systems and exposes `List`/`Get`; the GUI serves them; the studio fetches them instead of a hardcoded constant; an integration test loads each into a `JSEngine` and resolves a check.

**Tech Stack:** Go standard library (`embed`, `gopkg.in/yaml.v3`); React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-reference-systems-corpus-design.md`
**Depends on:** SYS-1, SYS-2.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- One source of truth: the studio must not keep a hardcoded system template.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The `refsystems` package

**Files:**
- Create: `pkg/refsystems/refsystems.go`
- Test: `pkg/refsystems/refsystems_test.go`

**Interfaces:**
- Consumes: `core.SystemManifest`, `core.MechanicsSpec`.
- Produces: `type ReferenceSystem`, `func List() []ReferenceSystem`, `func Get(id string) (ReferenceSystem, bool)`.

- [ ] **Step 1: Write the failing test**

```go
package refsystems

import "testing"

func TestListHasTheThreeSystems(t *testing.T) {
	got := List()
	ids := map[string]bool{}
	for _, s := range got {
		ids[s.ID] = true
		if s.Script == "" || s.RulesPrompt == "" || s.Mechanics == nil {
			t.Errorf("%s is incomplete", s.ID)
		}
	}
	for _, want := range []string{"narrative_2d6", "d20_dc", "dice_pool"} {
		if !ids[want] {
			t.Errorf("missing %s", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/refsystems/ -run TestListHasTheThreeSystems -v`
Expected: FAIL, `undefined: List`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/refsystems/refsystems.go`:

```go
// Package refsystems ships complete, runnable systems as starting points. They
// are embedded so the studio, the docs, and the tests share one source.
package refsystems

import (
	"embed"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"gopkg.in/yaml.v3"
)

//go:embed systems
var files embed.FS

// ReferenceSystem is a complete, runnable system.
type ReferenceSystem struct {
	ID          string
	Name        string
	Version     string
	Description string
	RulesPrompt string
	Script      string
	Mechanics   *core.MechanicsSpec
}

// List returns every reference system, ordered by ID.
func List() []ReferenceSystem {
	entries, err := fs.ReadDir(files, "systems")
	if err != nil {
		return nil
	}
	out := make([]ReferenceSystem, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if sys, ok := load(e.Name()); ok {
			out = append(out, sys)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns one reference system by id.
func Get(id string) (ReferenceSystem, bool) {
	for _, s := range List() {
		if s.ID == id {
			return s, true
		}
	}
	return ReferenceSystem{}, false
}

func load(dir string) (ReferenceSystem, bool) {
	manifest, err := files.ReadFile(path.Join("systems", dir, "system.yaml"))
	if err != nil {
		return ReferenceSystem{}, false
	}
	var m core.SystemManifest
	if err := yaml.Unmarshal(manifest, &m); err != nil {
		return ReferenceSystem{}, false
	}
	script, _ := files.ReadFile(path.Join("systems", dir, "mechanics.js"))
	rules, _ := files.ReadFile(path.Join("systems", dir, "prompts", "rules.md"))
	return ReferenceSystem{
		ID: m.ID, Name: m.Name, Version: m.Version, Description: m.Description,
		RulesPrompt: strings.TrimSpace(string(rules)),
		Script:      string(script),
		Mechanics:   m.Mechanics,
	}, true
}
```

Adjust the manifest field names to the real `core.SystemManifest` (`pkg/core/types.go:12-23`).

- [ ] **Step 4: Run test to verify it passes once Task 2 lands**

Run: `go test ./pkg/refsystems/ -run TestListHasTheThreeSystems -v`
Expected: FAIL until Task 2 adds the files; PASS after.

- [ ] **Step 5: Commit**

```bash
git add pkg/refsystems/refsystems.go pkg/refsystems/refsystems_test.go
git commit -m "feat(refsystems): add the embedded reference systems loader"
```

---

### Task 2: The three systems

**Files:**
- Create: `pkg/refsystems/systems/narrative_2d6/{system.yaml,mechanics.js,prompts/rules.md}`
- Create: `pkg/refsystems/systems/d20_dc/{system.yaml,mechanics.js,prompts/rules.md}`
- Create: `pkg/refsystems/systems/dice_pool/{system.yaml,mechanics.js,prompts/rules.md}`

**Interfaces:**
- Consumes: the schema from SYS-1/SYS-2 and the host API.
- Produces: three complete systems.

- [ ] **Step 1: Write `narrative_2d6`**

Use the spec §4.2 manifest verbatim. Move the existing script from
`frontend/src/templates/referenceTemplates.ts:53-78` into `mechanics.js` unchanged, and the rules
prompt (`:34-52`) into `prompts/rules.md`.

- [ ] **Step 2: Write `d20_dc`**

```yaml
id: d20_dc
name: d20 + DC
version: "1.0"
mechanics:
  stats:
    - { id: strength, label: Strength, type: number, default: 10 }
    - { id: dexterity, label: Dexterity, type: number, default: 10 }
  skills:
    - { id: athletics, label: Athletics, stat: strength }
  checks:
    notation: 1d20
    outcome: [success, fail]
    profiles:
      d20:
        notation: 1d20
        dc: 15
  engagement: auto
```

The script registers `onAction("do", …)` and `onTurnEnd(…)`, rolling `1d20` and returning an
outcome.

- [ ] **Step 3: Write `dice_pool`**

```yaml
id: dice_pool
name: d10 Dice Pool
version: "1.0"
mechanics:
  stats:
    - { id: dice, label: Pool Size, type: number, default: 5 }
    - { id: grit, label: Grit, type: number, default: 2 }
  checks:
    notation: 5d10
    outcome: [strong, weak, miss]
    profiles:
      pool:
        notation: 5d10
        success_on: ">=8"
        outcomes:
          - { min: 3, max: -1, outcome: strong }
          - { min: 1, max: 2,  outcome: weak }
          - { min: 0, max: 0,  outcome: miss }
  engagement: auto
```

The script spends a point of `grit` on a strong outcome via `setStat`, exercising state writes.

- [ ] **Step 4: Run the loader test**

Run: `go test ./pkg/refsystems/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/refsystems/systems
git commit -m "feat(refsystems): ship three reference systems"
```

---

### Task 3: Validation and integration tests

**Files:**
- Test: `pkg/refsystems/refsystems_test.go` (append), `pkg/engine/refsystems_test.go`

**Interfaces:**
- Consumes: `List`, `rules.NewJSEngine`, `rules.SchemaResolver`.
- Produces: per-system assertions that each loads and resolves.

- [ ] **Step 1: Write the failing tests**

```go
func TestEverySystemMechanicsValidate(t *testing.T) {
	for _, s := range List() {
		if problems := s.Mechanics.Checks.Validate(); len(problems) > 0 {
			t.Errorf("%s: %v", s.ID, problems)
		}
	}
}

func TestEverySystemResolvesACheck(t *testing.T) {
	for _, s := range List() {
		engine := rules.NewJSEngine(newTestBridge())
		if err := engine.SetManifest(manifestFor(s)); err != nil {
			t.Fatal(err)
		}
		if err := engine.LoadScript(s.Script); err != nil {
			t.Errorf("%s script: %v", s.ID, err)
		}
		res, err := engine.Resolve(context.Background(), harness.CheckRequest{}, testActor())
		if err != nil || res.Outcome == "" {
			t.Errorf("%s resolve: %v %+v", s.ID, err, res)
		}
	}
}
```

Adapt to the real `JSEngine`/`SchemaResolver` constructors and test helpers.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/refsystems/ ./pkg/engine/ -run 'TestEverySystem' -v`
Expected: FAIL until the helpers exist.

- [ ] **Step 3: Write the test helpers**

Add the bridge/actor/manifest helpers the tests need in the test files.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/refsystems/ ./pkg/engine/ -run 'TestEverySystem' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/refsystems/refsystems_test.go pkg/engine/refsystems_test.go
git commit -m "test(refsystems): load and resolve every reference system"
```

---

### Task 4: The endpoint

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/types.go`, `pkg/gui/service.go`
- Test: `pkg/gui/reference_systems_test.go`

**Interfaces:**
- Consumes: `refsystems.List`.
- Produces: `GET /api/reference-systems` → `ReferenceSystemsDTO`.

- [ ] **Step 1: Write the failing test**

```go
func TestReferenceSystemsEndpoint(t *testing.T) {
	svc := newTestService(t)
	got, err := svc.ListReferenceSystems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Systems) != 3 {
		t.Fatalf("systems = %d, want 3", len(got.Systems))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestReferenceSystemsEndpoint -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `Service.ListReferenceSystems` mapping `refsystems.List()` to a DTO, mount
`GET /api/reference-systems` in `routes.go`/`server.go`, and mirror the type in
`frontend/src/types.ts` and `client.ts`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestReferenceSystemsEndpoint -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/routes.go pkg/gui/server.go pkg/gui/types.go pkg/gui/service.go pkg/gui/reference_systems_test.go frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(gui): serve the reference systems"
```

---

### Task 5: The studio uses them

**Files:**
- Modify: `frontend/src/components/SystemsStudio.tsx`, `frontend/src/templates/referenceTemplates.ts`

**Interfaces:**
- Consumes: the endpoint (Task 4).
- Produces: three starting points; "reset to reference" loads `narrative_2d6`.

- [ ] **Step 1: Fetch the systems**

On mount, fetch `/api/reference-systems` and list them as starting points. Remove the hardcoded
`REFERENCE_SYSTEM_TEMPLATE` system constant (keep the world template). If the fetch fails, show no
templates rather than a stale constant.

- [ ] **Step 2: Wire the reset action**

"Reset to reference" loads `narrative_2d6` from the fetched list.

- [ ] **Step 3: Typecheck and test**

Run: `npx tsc --noEmit` and `npm run test` (in `frontend/`)
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/SystemsStudio.tsx frontend/src/templates/referenceTemplates.ts
git commit -m "feat(frontend): offer the reference systems as starting points"
```

---

### Task 6: Verification

- [ ] **Step 1: Parity guard**

Add a test that the served list equals `refsystems.List()`.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Three complete systems ship and load.
- Each resolves a check through its profile.
- The studio offers them and the endpoint matches the embed.
- No hardcoded system template remains in the frontend.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the reference systems parity"
```
