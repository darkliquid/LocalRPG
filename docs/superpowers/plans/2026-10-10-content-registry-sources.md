# Content Registry Sources Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Let a user see and manage the registry sources the Content Registry fetches from, and replace the misleading empty-search message with truthful states.

**Architecture:** A `NormalizeSource` in `pkg/registry` shared by the CLI and the GUI; a `Client.Sources` that reports each configured URL's fetch state (including a per-source error, which `Registries` drops); three service methods that load, mutate, and `Save` `registries.urls`; and a Sources panel plus four explicit empty states in `RegistryModal`.

**Tech Stack:** Go 1.27, `net/http`, `pkg/config`; React 19, TypeScript (strict, `noUnusedLocals`), Tailwind v4, Vitest + React Testing Library.

**Spec:** `docs/superpowers/specs/2026-10-10-content-registry-sources-design.md`
**Issue:** [#133](https://github.com/darkliquid/LocalRPG/issues/133)

## Global Constraints

- Persist through `config.ConfigManager.Save`, never by writing YAML directly, so the local-override rule holds.
- No default registries and no config schema change; `registries.urls` already exists.
- The exact user-facing copy is in the spec §4.4; use it verbatim.
- Errors are wrapped with `fmt.Errorf("...: %w", err)`; sentinels use `errors.New`; no testify.

## File Map

| File | Change |
| --- | --- |
| `pkg/registry/source.go`, `source_test.go` | new: `NormalizeSource` |
| `pkg/registry/client.go`, `client_test.go` | `SourceStatus`, `Sources` |
| `cmd/localrpg/registry.go` | `runRegistryAdd` uses `NormalizeSource` |
| `pkg/gui/service.go` | `RegistrySources`, `AddRegistrySource`, `RemoveRegistrySource`, sentinels |
| `pkg/gui/types.go` | `RegistrySourceDTO` |
| `pkg/gui/routes.go`, `pkg/gui/server.go` | `/api/registry/sources` route and handler |
| `pkg/gui/registry_test.go` | service and route tests |
| `frontend/src/types.ts`, `frontend/src/api/client.ts` | DTO and source methods |
| `frontend/src/components/RegistryModal.tsx`, `RegistryModal.test.tsx` | panel, empty states, debounce |
| `pkg/gui/docs/22-editing-content.md` | document the Sources panel |

---

### Task 1: `NormalizeSource` and `Client.Sources`

**Files:**
- Create: `pkg/registry/source.go`, `pkg/registry/source_test.go`
- Modify: `pkg/registry/client.go`, `pkg/registry/client_test.go`

- [x] **Step 1: Write the failing tests**

`NormalizeSource` table: `https://example.org/index.json` and `git+https://example.org/repo` pass through trimmed; `""`, `"ftp://x"`, and `"example.org"` fail. `Sources` returns one entry per configured URL, with the name and package count for a reachable one and a non-empty `Error` for one whose server 500s, while the other still resolves.

- [x] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestNormalizeSource|TestSources' ./pkg/registry/`
Expected: FAIL, undefined.

- [x] **Step 3: Write the minimal implementation**

```go
// NormalizeSource validates a registry URL and returns its canonical string.
// Accepted forms: http:// and https:// for a static index, and git+<scheme>://
// (https, http, ssh, file) for a git repository.
func NormalizeSource(raw string) (string, error)
```

```go
// SourceStatus reports one configured registry's fetch state. Unlike Registries,
// which drops a failed source, this keeps it so the UI can say why a registry
// shows nothing.
type SourceStatus struct {
	URL          string `json:"url"`
	Name         string `json:"name,omitempty"`
	PackageCount int    `json:"package_count"`
	Error        string `json:"error,omitempty"`
}

// Sources reports the fetch state of every configured registry.
func (c *Client) Sources(ctx context.Context) []SourceStatus
```

`Sources` iterates `c.cfg.URLs`, calling the existing `fetchOrCachedIndex` and recording either the name/count or the error.

- [x] **Step 4: Run them to verify they pass**

Run: `go test -run 'TestNormalizeSource|TestSources' ./pkg/registry/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/registry/source.go pkg/registry/source_test.go pkg/registry/client.go pkg/registry/client_test.go
git commit -m "feat(registry): validate and report registry sources"
```

### Task 2: The CLI uses the shared normaliser

**Files:**
- Modify: `cmd/localrpg/registry.go`

- [x] **Step 1: Write the failing test** in `cmd/localrpg/registry_test.go`: `registry add ftp://x` reports an error and adds nothing; `registry add https://example.org/index.json` succeeds.

- [x] **Step 2: Run it to verify it fails**

Run: `go test -run TestRegistry - ./cmd/localrpg/`
Expected: FAIL on the invalid URL case.

- [x] **Step 3: Write the minimal implementation**

In `runRegistryAdd`, after `rawURL := strings.TrimSpace(args[0])`, call `registry.NormalizeSource` and print `Invalid registry URL: %v` on error.

- [x] **Step 4: Run it to verify it passes**

Run: `go test -run TestRegistry - ./cmd/localrpg/`
Expected: PASS, including the existing add/list/remove test.

- [x] **Step 5: Commit**

```bash
git add cmd/localrpg/registry.go cmd/localrpg/registry_test.go
git commit -m "feat(cli): reject an invalid registry URL"
```

### Task 3: The service methods, DTO, and route

**Files:**
- Modify: `pkg/gui/service.go`, `pkg/gui/types.go`, `pkg/gui/routes.go`, `pkg/gui/server.go`
- Test: `pkg/gui/registry_test.go`

- [x] **Step 1: Write the failing tests**

```go
func TestRegistrySourcesListsConfiguredURLs(t *testing.T)   // a configured URL appears with its name
func TestAddRegistrySourcePersistsAndRejectsDuplicate(t *testing.T) // duplicate wraps ErrRegistrySourceExists
func TestRemoveRegistrySourcePersistsAndReportsAbsent(t *testing.T) // absent wraps ErrRegistrySourceNotFound
func TestRegistrySourcesRouteDispatchesByMethod(t *testing.T)        // GET 200, POST 201, DELETE 204
```

- [x] **Step 2: Run them to verify they fail**

Run: `go test -run TestRegistrySources ./pkg/gui/`
Expected: FAIL, undefined.

- [x] **Step 3: Write the minimal implementation**

```go
// RegistrySourceDTO reports one configured registry and its fetch state.
type RegistrySourceDTO struct {
	URL          string `json:"url"`
	Name         string `json:"name,omitempty"`
	PackageCount int    `json:"package_count"`
	Error        string `json:"error,omitempty"`
}

var (
	ErrRegistrySourceExists   = errors.New("registry source already configured")
	ErrRegistrySourceNotFound = errors.New("registry source not configured")
)

func (s *Service) RegistrySources(ctx context.Context) ([]RegistrySourceDTO, error)
func (s *Service) AddRegistrySource(ctx context.Context, rawURL string) (RegistrySourceDTO, error)
func (s *Service) RemoveRegistrySource(_ context.Context, rawURL string) error
func (s *Service) HandleRegistrySources(w http.ResponseWriter, r *http.Request)
```

`Add` normalises, rejects a duplicate with `ErrRegistrySourceExists`, appends to a copy of `cfg.Registries.URLs`, `Save`s, and returns the resolved `RegistrySourceDTO`. `Remove` normalises, rebuilds the slice without the URL, `Save`s, and reports `ErrRegistrySourceNotFound` when absent. The handler maps 409/404/400 and dispatches GET/POST/DELETE.

Add the mount `{"/api/registry/sources", "handleRegistrySourcesRoute", (*Server).handleRegistrySourcesRoute}` and its delegating handler.

- [x] **Step 4: Run them to verify they pass**

Run: `go test -run TestRegistrySources ./pkg/gui/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/types.go pkg/gui/routes.go pkg/gui/server.go pkg/gui/registry_test.go
git commit -m "feat(api): manage registry sources"
```

### Task 4: The frontend client and types

**Files:**
- Modify: `frontend/src/types.ts`, `frontend/src/api/client.ts`
- Test: `frontend/src/api/client.test.ts`

- [x] **Step 1: Write the failing test** that `listRegistrySources`/`addRegistrySource`/`removeRegistrySource` hit the right methods and paths and throw an `HTTPError` on failure.

- [x] **Step 2: Run it to verify it fails**

Run: `cd frontend && npx vitest run src/api/client.test.ts`
Expected: FAIL, not a function.

- [x] **Step 3: Write the minimal implementation**

Add `RegistrySourceDTO` to `types.ts` and the three static methods to `APIClient`, matching the existing registry method style.

- [x] **Step 4: Run it to verify it passes**

Run: `cd frontend && npx vitest run src/api/client.test.ts`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts frontend/src/api/client.test.ts
git commit -m "feat(frontend): registry source client methods"
```

### Task 5: The Sources panel, empty states, and debounce

**Files:**
- Modify: `frontend/src/components/RegistryModal.tsx`
- Test: `frontend/src/components/RegistryModal.test.tsx`

- [x] **Step 1: Write the failing tests** for the four empty states (spec §4.4), the Sources panel add/remove, and a debounced search effect.

- [x] **Step 2: Run them to verify they fail**

Run: `cd frontend && npx vitest run src/components/RegistryModal.test.tsx`
Expected: FAIL.

- [x] **Step 3: Write the minimal implementation**

- Fetch sources on open; keep `sources` in state.
- Add a Sources disclosure above the search: a summary line, one row per source (name or URL, package count, per-source error, Remove), and a URL input + Add.
- The empty-state render keys on `sources.length` and the query as the spec §4.4 table; the query is never interpolated into a "matching" sentence for an empty box.
- A results heading reads "All packages" for an empty query, `Results for "<query>"` otherwise.
- Debounce the query 250 ms and key the fetch effect on the debounced value.

- [x] **Step 4: Run them to verify they pass**

Run: `cd frontend && npx vitest run src/components/RegistryModal.test.tsx`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/RegistryModal.tsx frontend/src/components/RegistryModal.test.tsx
git commit -m "feat(frontend): show and manage registry sources"
```

### Task 6: Docs

**Files:**
- Modify: `pkg/gui/docs/22-editing-content.md`

- [x] **Step 1: Add the GUI path (the Sources panel) beside the CLI verbs.**
- [x] **Step 2: Run `mise run lint:docs`.**
- [x] **Step 3: Commit**

## Verification

- `go test ./pkg/registry/ ./pkg/gui/ ./cmd/localrpg/`, then `mise run test:backend`.
- `cd frontend && npx vitest run && npx tsc --noEmit`.
- `mise run lint`.
