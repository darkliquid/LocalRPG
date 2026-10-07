# Content Registry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A registry client that searches, installs, and updates content from configured indexes, with caching and offline search.

**Architecture:** `pkg/registry` parses a static index, caches it, and reuses PKG-2's install; a `registries` config lists URLs; CLI verbs and a launcher view expose it.

**Tech Stack:** Go standard library (`net/http`, `encoding/json`); React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-content-registry-design.md`
**Depends on:** PKG-1, PKG-2, PKG-3.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- Install goes through PKG-2; the registry never writes content directly.
- The index cache is used when the network is unavailable.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The configuration and the index

**Files:**
- Modify: `pkg/config/types.go`, `pkg/config/manager.go`
- Create: `pkg/registry/index.go`
- Test: `pkg/registry/index_test.go`

**Interfaces:**
- Consumes: `gopkg.in/yaml.v3` (config), `encoding/json`.
- Produces: `config.RegistriesConfig{URLs []string}`, `registry.Index`, `registry.PackageRef`, `registry.ParseIndex([]byte) (Index, error)`.

- [ ] **Step 1: Write the failing test**

```go
package registry

import "testing"

func TestParseIndex(t *testing.T) {
	in := []byte(`{"name":"R","packages":[{"type":"world","id":"w","version":"1.0.0","download":"https://x/w.lrpgpack","sha256":"ab"}]}`)
	idx, err := ParseIndex(in)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Name != "R" || len(idx.Packages) != 1 || idx.Packages[0].ID != "w" {
		t.Fatalf("index = %+v", idx)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/registry/ -run TestParseIndex -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `RegistriesConfig{URLs []string \`yaml:"urls,omitempty"\`}` to `Config`, and the index types plus
`ParseIndex` in `pkg/registry`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/registry/ -run TestParseIndex -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config pkg/registry
git commit -m "feat(registry): add the registries config and index format"
```

---

### Task 2: The client and its cache

**Files:**
- Create: `pkg/registry/client.go`
- Test: `pkg/registry/client_test.go`

**Interfaces:**
- Consumes: `Index` (Task 1), an HTTP client, the cache dir.
- Produces: `Client`, `NewClient(cfg, cacheDir, logger) *Client`, `(*Client).Indexes(ctx) ([]Index, error)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestClientCachesIndexes(t *testing.T) {
	// A stub server returns an index; the second call is served from cache when the
	// server is unreachable.
}
func TestClientSurvivesOneBadRegistry(t *testing.T) { /* one failure does not fail the rest */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/registry/ -run TestClient -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Fetch each configured index with an `http.Client`, cache it (JSON plus ETag/Last-Modified) under the
cache dir, revalidate on demand, and fall back to the cache when a fetch fails. Log a per-registry
error without failing the others.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/registry/ -run TestClient -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/registry/client.go pkg/registry/client_test.go
git commit -m "feat(registry): fetch and cache indexes"
```

---

### Task 3: Search

**Files:**
- Modify: `pkg/registry/client.go`
- Test: `pkg/registry/client_test.go` (append)

**Interfaces:**
- Consumes: `Indexes` (Task 2).
- Produces: `(*Client).Search(ctx, query) ([]PackageRef, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestSearchMatchesAcrossIndexes(t *testing.T) { /* a query matches id, name, description */ }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/registry/ -run TestSearch -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Case-insensitive substring match on id, name, description, and author across every cached index,
returning `PackageRef{Registry, Package}`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/registry/ -run TestSearch -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/registry/client.go pkg/registry/client_test.go
git commit -m "feat(registry): search across indexes"
```

---

### Task 4: Install

**Files:**
- Modify: `pkg/registry/client.go`
- Test: `pkg/registry/client_test.go` (append)

**Interfaces:**
- Consumes: PKG-2's import, PKG-1's verification.
- Produces: `(*Client).Install(ctx, ref PackageRef, onConflict string) (content.Manifest, error)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestInstallVerifiesChecksum(t *testing.T) {
	// A package whose bytes do not match the index sha256 is refused.
}
func TestInstallDelegatesToImport(t *testing.T) {
	// A valid package installs through PKG-2 and reports the manifest.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/registry/ -run TestInstall -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Resolve the download URL, stream to a temp file, verify the SHA-256 against the index, then hand the
file to the GUI's/service's import path (staging, validate, conflict) or to `pkg/content` plus the
install logic shared with PKG-2.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/registry/ -run TestInstall -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/registry/client.go pkg/registry/client_test.go
git commit -m "feat(registry): install a package with checksum verification"
```

---

### Task 5: Update and git registries

**Files:**
- Modify: `pkg/registry/client.go`
- Test: `pkg/registry/client_test.go` (append)

**Interfaces:**
- Consumes: PKG-3's semver and the content lock.
- Produces: `(*Client).Update(ctx) ([]PackageRef, error)`; a `git+` scheme handled in `Indexes`.

- [ ] **Step 1: Write the failing tests**

```go
func TestUpdateFindsNewerVersions(t *testing.T) { /* a higher version than installed is reported */ }
func TestGitRegistryScheme(t *testing.T) { /* a git+ URL is recognised and cloned */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/registry/ -run 'TestUpdate|TestGitRegistry' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

`Update` compares each installed content package's version against the highest in the indexes using
`semver.Compare`. For a `git+https://…` URL, shallow-clone into the cache (shelling out to `git`, as
the repo already does for other tooling) and read `index.json`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/registry/ -run 'TestUpdate|TestGitRegistry' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/registry/client.go pkg/registry/client_test.go
git commit -m "feat(registry): update detection and git indexes"
```

---

### Task 6: The CLI verbs and the launcher view

**Files:**
- Create: `cmd/localrpg/registry.go`
- Modify: `cmd/localrpg/main.go`
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`
- Modify: `frontend/src/components/LauncherHub.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `cmd/localrpg/registry_test.go`, `pkg/gui/registry_test.go`

**Interfaces:**
- Consumes: the client (Tasks 2-5).
- Produces: `localrpg registry add|list|remove|search|install|update` and a Registry view.

- [ ] **Step 1: Write the failing tests**

```go
func TestRegistrySearchCLI(t *testing.T) { /* prints matching packages */ }
func TestRegistryListEndpoint(t *testing.T) { /* returns the search results */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/localrpg/ -run TestRegistry -v` and `go test ./pkg/gui/ -run TestRegistryList -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement the verbs and a `GET /api/registry/search` endpoint, and add a Registry view to the
launcher that lists, searches, and installs (reusing the PKG-2 confirmation).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/localrpg/ -run TestRegistry -v` and `go test ./pkg/gui/ -run TestRegistryList -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/localrpg pkg/gui frontend/src
git commit -m "feat: browse and install from registries"
```

---

### Task 7: Verification

- [ ] **Step 1: Offline guard**

Add a test that search works from the cache with the network unavailable.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Search spans every configured index.
- Install verifies the checksum and delegates to PKG-2.
- Update reports newer versions.
- A git index works.
- No registries configured is an empty list, not an error.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the offline registry path"
```
