# Content Registry Sources Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user see and manage the registry sources the Content Registry fetches from, and replace the misleading empty-search message with truthful states.

**Architecture:** A URL normaliser in `pkg/registry` shared by the CLI and the GUI; three service methods and one route in `pkg/gui` that load, mutate, and save `registries.urls` through the config manager; a Sources panel and four explicit empty states in `RegistryModal`.

**Tech Stack:** Go 1.27, `net/http`, Wails v3 GUI service; React 19, TypeScript, Tailwind v4, Vitest.

**Spec:** `docs/superpowers/specs/2026-10-10-content-registry-sources-design.md`
**Issue:** [#133](https://github.com/darkliquid/LocalRPG/issues/133)

## Global Constraints

- Persist through `config.Manager.Save`, never by writing YAML directly, so the local-override rule holds.
- Reuse `registry.Client.Registries` for fetch state; do not add a second fetch path.
- No default registries and no config schema change.
- Exact user-facing copy is in the spec §4.4; use it verbatim.

## File Map

| File | Change |
| --- | --- |
| `pkg/registry/source.go` | new: `NormalizeSource` |
| `pkg/registry/source_test.go` | new: normaliser table tests |
| `cmd/localrpg/registry.go` | `runRegistryAdd` uses `NormalizeSource` |
| `pkg/gui/service.go` | `RegistrySources`, `AddRegistrySource`, `RemoveRegistrySource` |
| `pkg/gui/types.go` | `RegistrySourceDTO` |
| `pkg/gui/routes.go`, `pkg/gui/server.go` | `/api/registry/sources` route and handler |
| `pkg/gui/registry_test.go` | service and route tests |
| `frontend/src/api/client.ts`, `types.ts` | source methods and DTO |
| `frontend/src/components/RegistryModal.tsx`, `RegistryModal.test.tsx` | Sources panel, empty states, debounce |
| `pkg/gui/docs/22-editing-content.md` | document the Sources panel |

---

### Task 1: `NormalizeSource` and the CLI

**Files:**
- Create: `pkg/registry/source.go`, `pkg/registry/source_test.go`
- Modify: `cmd/localrpg/registry.go`

- [ ] **Step 1: Write the failing test** for the accepted and rejected forms in spec §4.1.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Implement `NormalizeSource`.**
- [ ] **Step 4: Run it to verify it passes.**
- [ ] **Step 5: Commit.**

### Task 2: Service methods, DTO, and route

**Files:**
- Modify: `pkg/gui/service.go`, `pkg/gui/types.go`, `pkg/gui/routes.go`, `pkg/gui/server.go`
- Test: `pkg/gui/registry_test.go`

- [ ] **Step 1: Write the failing tests** for list (name plus per-source error), add (persists, rejects duplicate with 409, invalid with 400), and remove (persists, 404 when absent).
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement the methods, DTO, route, and handler.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 3: Frontend client and types

**Files:**
- Modify: `frontend/src/api/client.ts`, `frontend/src/types.ts`

- [ ] **Step 1: Write a failing test** that `listRegistrySources`/`addRegistrySource`/`removeRegistrySource` hit the right methods and paths.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Implement the methods and the `RegistrySourceDTO` type.**
- [ ] **Step 4: Run it to verify it passes.**
- [ ] **Step 5: Commit.**

### Task 4: The Sources panel, empty states, and debounce

**Files:**
- Modify: `frontend/src/components/RegistryModal.tsx`
- Test: `frontend/src/components/RegistryModal.test.tsx`

- [ ] **Step 1: Write the failing tests** for the four empty states (spec §4.4), the Sources panel add/remove, and a debounced search effect.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement the panel and the keyed render decision.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 5: Docs

**Files:**
- Modify: `pkg/gui/docs/22-editing-content.md`

- [ ] **Step 1: Add the GUI path beside the CLI verbs.**
- [ ] **Step 2: Run `mise run lint:docs`.**
- [ ] **Step 3: Commit.**
