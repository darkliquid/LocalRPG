# Route and DTO Manifest Implementation Plan

> **Status:** Implemented (routes) on 2026-09-28; the DTO-name guard is deferred.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the API route table data, check it in, and fail a test when the frontend or the span-naming switch drifts from it.

**Architecture:** A `mounts` table of `routeMount{Pattern, Handler, serve}` is the single list; `registerRoutes` iterates it with method expressions. A checked-in `testdata/routes.json` is regenerated with `-update-routes`, and Go source-text guards cover `routePattern` and the frontend client.

**Tech Stack:** Go 1.27 stdlib tests.

**Spec:** `docs/superpowers/specs/2026-09-28-route-and-dto-manifest-design.md`

## Global Constraints

- Use `interface{}`, never `any`; `go vet` clean.
- Standard library only in tests.
- No route, DTO, or response-shape change; the bound patterns are identical.

---

### Task 1: The route table

**Files:** `pkg/gui/routes.go`, `pkg/gui/server.go`.

- [x] **Step 1:** `routeMount{Pattern, Handler string; serve func(*Server, http.ResponseWriter, *http.Request)}`
  and the `mounts` table, using method expressions so the pattern appears once.
- [x] **Step 2:** `Routes() []string` returns the sorted patterns; `mountCovers`
  resolves a concrete path against a trailing-slash mount.
- [x] **Step 3:** `registerRoutes` iterates `mounts`; the asset mount is
  unchanged.

### Task 2: Name every mount in `routePattern`

**Files:** `pkg/gui/server.go`.

- [x] **Step 1:** Add `/api/usage` and `/api/limits`, which were falling back to
  `"http.request"`.

### Task 3: Guards

**Files:** `pkg/gui/routes_test.go`, `pkg/gui/testdata/routes.json`.

- [x] **Step 1:** `TestRouteManifestIsCurrent` with `-update-routes` regeneration.
- [x] **Step 2:** `TestRoutePatternNamesEveryMount`.
- [x] **Step 3:** `TestEveryMountHasAHandler` (non-empty handler, no duplicate
  pattern).
- [x] **Step 4:** `TestFrontendPathsResolveToMounts`: every `/api/...` literal in
  `frontend/src/api/client.ts` resolves to a mount, with a seeded negative case.
- [x] **Step 5:** Generate `testdata/routes.json` and re-run without the flag.

### Task 4: DTO name guard — rejected

- [x] **Step 1:** Until the DTO inventory was taken, the guard looked cheap. It is
  not: only 3 of 60 Go type names match `types.ts` exactly, so a real guard needs
  a rotting alias map to catch the weaker drift. Rejected; the reason is recorded
  in the spec. Deriving `types.ts` from Go is the only version worth doing.

### Task 5: Full verification

- [x] **Step 1:** `go build ./...`, `go test ./...`, `go vet ./...`.
