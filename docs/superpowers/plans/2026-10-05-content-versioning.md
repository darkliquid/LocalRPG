# Content Versioning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce semver on content, let a world require a system version, and lock the versions a campaign was played against.

**Architecture:** `golang.org/x/mod/semver` validates versions; `WorldManifest.Requires` declares a constraint; a per-campaign `content.lock.yaml` records versions and behavioural-file digests; opening resolves, refusing an unsatisfied constraint and warning on a changed digest.

**Tech Stack:** Go standard library; `golang.org/x/mod/semver`.

**Spec:** `docs/superpowers/specs/2026-10-05-content-versioning-design.md`
**Depends on:** PKG-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- The lock hashes behavioural files only (`system.yaml`, `mechanics.js`), not prose.
- Conventional Commits, subject under 72 chars.

---

### Task 1: Semver validation

**Files:**
- Modify: `pkg/core/types.go`
- Test: `pkg/core/types_test.go` (append)

**Interfaces:**
- Consumes: `golang.org/x/mod/semver`.
- Produces: version validation in `LoadSystemManifest`/`LoadWorldManifest` (or a `Validate` on the manifests).

- [ ] **Step 1: Write the failing test**

```go
func TestManifestVersionMustBeSemver(t *testing.T) {
	if err := (SystemManifest{ID: "s", Version: "latest"}).Validate(); err == nil {
		t.Fatal("an invalid version should be rejected")
	}
	if err := (SystemManifest{ID: "s", Version: "1.2.0"}).Validate(); err != nil {
		t.Fatalf("a valid version was rejected: %v", err)
	}
	if err := (SystemManifest{ID: "s", Version: ""}).Validate(); err != nil {
		t.Fatalf("an unversioned manifest should be allowed: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/core/ -run TestManifestVersionMustBeSemver -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `Validate() error` to `SystemManifest` and `WorldManifest` that rejects a non-empty version that
is not valid semver (`semver.IsValid`, accepting a leading `v`). Call it from the loaders so a load
reports the problem.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/core/ -run TestManifestVersionMustBeSemver -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/core/types.go pkg/core/types_test.go go.mod go.sum
git commit -m "feat(core): require semver on content versions"
```

---

### Task 2: `Requires` on a world

**Files:**
- Modify: `pkg/core/types.go`
- Test: `pkg/core/types_test.go` (append)

**Interfaces:**
- Consumes: `ContentRequirement`.
- Produces: `WorldManifest.Requires []ContentRequirement`, `ContentRequirement`, and a
  `Satisfies(version string) bool` helper.

- [ ] **Step 1: Write the failing test**

```go
func TestRequirementSatisfies(t *testing.T) {
	r := ContentRequirement{Type: "system", ID: "narrative_2d6", Version: ">=1.0.0 <2.0.0"}
	if !r.Satisfies("1.4.0") {
		t.Fatal("1.4.0 should satisfy the range")
	}
	if r.Satisfies("2.0.0") {
		t.Fatal("2.0.0 should not satisfy the range")
	}
	any := ContentRequirement{Type: "system", ID: "x"}
	if !any.Satisfies("0.0.0") {
		t.Fatal("an empty constraint should accept any version")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/core/ -run TestRequirementSatisfies -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the type and a `Satisfies` that returns true for an empty constraint and otherwise evaluates a
simple `>=`/`<`/`>`/`<=` range list against `semver.Compare`. Keep the grammar small and documented.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/core/ -run TestRequirementSatisfies -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/core/types.go pkg/core/types_test.go
git commit -m "feat(core): let a world require a system version"
```

---

### Task 3: The lockfile and the digest

**Files:**
- Create: `pkg/content/lock.go`
- Test: `pkg/content/lock_test.go`

**Interfaces:**
- Consumes: the directory walk from PKG-1.
- Produces: `ContentLock`, `LockEntry`, `func BehaviouralDigest(dir string) (string, error)`, `func LoadLock(path string) (ContentLock, error)`, `func (l ContentLock) Save(path string) error`.

- [ ] **Step 1: Write the failing test**

```go
func TestBehaviouralDigestIgnoresProse(t *testing.T) {
	dir := writeFixtureSystem(t) // system.yaml + mechanics.js + prompts/rules.md
	a, err := BehaviouralDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	appendTo(t, filepath.Join(dir, "prompts", "rules.md"), "\nmore prose\n")
	b, _ := BehaviouralDigest(dir)
	if a != b {
		t.Fatal("a prose edit should not change the behavioural digest")
	}
	appendTo(t, filepath.Join(dir, "mechanics.js"), "\n// x\n")
	c, _ := BehaviouralDigest(dir)
	if a == c {
		t.Fatal("a mechanics.js edit should change the digest")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/content/ -run TestBehaviouralDigest -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement the lock types, `BehaviouralDigest` (hash `system.yaml` and `mechanics.js`, or for a world
`world.yaml` and any `system_overrides/*/hooks.js`), and the load/save helpers.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/content/ -run TestBehaviouralDigest -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/content/lock.go pkg/content/lock_test.go
git commit -m "feat(content): add a behavioural digest and lockfile"
```

---

### Task 4: Write and resolve the lock

**Files:**
- Modify: `pkg/engine/game.go` (`InitGame`), `pkg/engine/startlocation.go` or a new `pkg/engine/content.go`
- Test: `pkg/engine/content_lock_test.go`

**Interfaces:**
- Consumes: `content.ContentLock`, `core.ContentRequirement`.
- Produces: `func (g *Game) LockContent() error`, `func ResolveContentLock(paths, gameID) (ContentLock, []string, error)` returning warnings.

- [ ] **Step 1: Write the failing tests**

```go
func TestInitGameWritesLock(t *testing.T) { /* a new campaign has content.lock.yaml */ }
func TestOpenWarnsOnChangedDigest(t *testing.T) { /* a compatible change warns */ }
func TestOpenRefusesUnsatisfiedRequirement(t *testing.T) { /* a 2.0 system vs <2.0.0 refuses */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/engine/ -run 'TestInitGameWritesLock|TestOpenWarnsOnChangedDigest|TestOpenRefusesUnsatisfiedRequirement' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

`InitGame` writes the lock after resolving the system and world. A resolve function reads the lock,
compares versions and digests, checks the world's `Requires` against the system version, and returns
warnings or a refusal error.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/engine/ -run 'TestInitGameWritesLock|TestOpenWarns|TestOpenRefuses' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine
git commit -m "feat(engine): lock and resolve a campaign's content"
```

---

### Task 5: Surface the warnings

**Files:**
- Modify: `pkg/gui/service.go` (open path), `frontend/src/components/` (a warning surface)
- Test: `pkg/gui/content_lock_test.go`

**Interfaces:**
- Consumes: the resolve warnings (Task 4).
- Produces: a warning in the open response and the UI.

- [ ] **Step 1: Write the failing test**

```go
func TestOpenSurfacesContentWarnings(t *testing.T) { /* the open response carries the warnings */ }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestOpenSurfacesContentWarnings -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Carry the warnings on the game-open DTO and render them as a dismissible notice; a refusal returns
an error the UI shows with the constraint detail.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestOpenSurfacesContentWarnings -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat(gui): surface content-lock warnings"
```

---

### Task 6: Verification

- [ ] **Step 1: Regression guard**

Add a test that a campaign with no lock opens, locks, and plays.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Invalid versions are rejected.
- A world's `Requires` is enforced.
- A campaign writes and resolves a lock.
- A compatible change warns; an incompatible one refuses.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the unversioned-campaign path"
```
