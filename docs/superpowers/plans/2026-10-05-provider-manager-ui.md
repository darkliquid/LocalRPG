# Provider Manager UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Settings Studio section that lists every provider of a family, lets the user add, duplicate, rename, remove, and choose defaults and purposes, and shows key state and tier.

**Architecture:** A new `ProviderManager.tsx` component driven by the whole `config` plus the existing catalogue; one additive backend endpoint (`POST /api/media/inspect`) for bulk key state; referential-integrity checks client-side with backend validation as the backstop.

**Tech Stack:** React 19 + Tailwind v4; Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-provider-manager-ui-design.md`
**Depends on:** MP-1, MP-3, LF-3.

## Global Constraints

- Go tests use the standard library only; no testify.
- `noUnusedLocals`/`noUnusedParameters` are on: no unused imports or params.
- The manager must not change `SettingsStudio.tsx` beyond mounting the new component.
- Opening the manager and saving with no changes must produce no config diff.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The bulk-inspect endpoint

**Files:**
- Create: `pkg/gui/media_inspect.go`
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`
- Test: `pkg/gui/media_inspect_test.go`

**Interfaces:**
- Consumes: the single inspect builder (`pkg/gui/tts_inspect.go:15-85`), `config.MediaConfig`.
- Produces: `POST /api/media/inspect` → `MediaInspectResponseDTO{Entries []MediaInspectEntryDTO}`, `MediaInspectEntryDTO{Name, Family, ProviderKey, KeyPresent, KeyRequired, Metered, Tier}`.

- [ ] **Step 1: Write the failing test**

```go
func TestMediaInspectListsEveryEntry(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Media.TTSProviders = map[string]config.TTSConfig{"npc": {Type: "builtin", BuiltinName: "echo"}}
	svc := newTestServiceWithConfig(t, cfg)
	resp, err := svc.InspectMedia(context.Background(), MediaInspectRequestDTO{Family: "tts"})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, e := range resp.Entries {
		names[e.Name] = true
	}
	if !names["default"] || !names["npc"] {
		t.Fatalf("entries = %v, want default and npc", names)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestMediaInspect -v`
Expected: FAIL, `undefined: InspectMedia`.

- [ ] **Step 3: Write minimal implementation**

Add `Service.InspectMedia` that, for each named entry of the requested family, builds the client
through the MP-1 registry and returns its provider key, key presence, key requirement, metered flag,
and tier (from the catalogue descriptor). Mount `POST /api/media/inspect` in `routes.go` and
`server.go`, mirroring the existing `handleTTSInspectRoute`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestMediaInspect -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/media_inspect.go pkg/gui/media_inspect_test.go pkg/gui/routes.go pkg/gui/server.go
git commit -m "feat(gui): add a bulk media inspect endpoint"
```

---

### Task 2: The client and DTO types

**Files:**
- Modify: `pkg/gui/types.go`, `frontend/src/types.ts`, `frontend/src/api/client.ts`

**Interfaces:**
- Consumes: Task 1.
- Produces: `MediaInspectRequestDTO`, `MediaInspectResponseDTO`, `MediaInspectEntryDTO`; the matching TS types and `inspectMedia()` client call.

- [ ] **Step 1: Add the DTOs**

In `pkg/gui/types.go`:

```go
type MediaInspectRequestDTO struct {
	Family string `json:"family"` // "tts" | "stt" | "image"
}

type MediaInspectEntryDTO struct {
	Name        string `json:"name"`
	ProviderKey string `json:"provider_key"`
	KeyPresent  bool   `json:"key_present"`
	KeyRequired bool   `json:"key_required"`
	Metered     bool   `json:"metered"`
	Tier        string `json:"tier"`
}

type MediaInspectResponseDTO struct {
	Entries []MediaInspectEntryDTO `json:"entries"`
}
```

Mirror in `frontend/src/types.ts` and add `inspectMedia(family)` to `frontend/src/api/client.ts`,
following the existing fetch pattern.

- [ ] **Step 2: Typecheck and build**

Run: `npx tsc --noEmit` (in `frontend/`) and `go build ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add pkg/gui/types.go frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(gui): type the bulk media inspect endpoint"
```

---

### Task 3: The provider list component

**Files:**
- Create: `frontend/src/components/ProviderManager.tsx`
- Test: `frontend/src/components/ProviderManager.test.tsx`

**Interfaces:**
- Consumes: the `config` prop, `inspectMedia` (Task 2), `TierBadge` (LF-3).
- Produces: `<ProviderManager family config onChange />`.

- [ ] **Step 1: Write the failing test**

```tsx
test("lists the default and named entries", () => {
  const config = { media: { tts: { type: "builtin" }, stt: {}, image: {},
    tts_providers: { npc: { type: "builtin" } } } };
  render(<ProviderManager family="tts" config={config} onChange={() => {}} />);
  expect(screen.getByText("default")).toBeInTheDocument();
  expect(screen.getByText("npc")).toBeInTheDocument();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- ProviderManager`
Expected: FAIL, module not found.

- [ ] **Step 3: Write minimal implementation**

Render one row per entry (the singleton as `default`, then the map's names sorted). Each row shows
the name, the `TierBadge`, and a key indicator from `inspectMedia`. A filled/empty dot marks the
default. The family's provider map is `config.media.<family>_providers`.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- ProviderManager`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ProviderManager.tsx frontend/src/components/ProviderManager.test.tsx
git commit -m "feat(frontend): add the provider manager list"
```

---

### Task 4: Entry actions

**Files:**
- Modify: `frontend/src/components/ProviderManager.tsx`
- Test: `frontend/src/components/ProviderManager.test.tsx` (append)

**Interfaces:**
- Consumes: Task 3.
- Produces: add, duplicate, rename, remove, and set-default handlers that emit a new config via `onChange`.

- [ ] **Step 1: Write the failing tests**

```tsx
test("add creates a new-provider entry", () => {
  const onChange = vi.fn();
  render(<ProviderManager family="tts" config={emptyConfig()} onChange={onChange} />);
  fireEvent.click(screen.getByText(/add tts provider/i));
  const next = onChange.mock.calls[0][0];
  expect(next.media.tts_providers["new-provider"]).toBeDefined();
});

test("duplicate copies under a -copy name", () => { /* … */ });
test("rename updates referencing purposes", () => { /* … */ });
test("remove is blocked when a purpose references the entry", () => { /* … */ });
test("set default writes the name", () => { /* … */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- ProviderManager`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement the handlers as pure functions over the config draft:

- `add`: `providers["new-provider"] = clone(providers[default])` (or the family's zero config).
- `duplicate`: `providers[uniqueName(name + "-copy")] = clone(providers[name])`.
- `rename`: move the key; rewrite `config.media.purposes` values equal to the old name.
- `remove`: if a purpose references the name, block and prompt to reassign; otherwise delete.
- `setDefault`: the family's default is the singleton, so "set default" copies the chosen entry into
  `config.media.<family>` (the singleton) and leaves the named entry in place, or swaps the two.

The set-default semantics follow the spec: the singleton field *is* the default (MP-1), so making an
entry the default means copying it into the singleton field. Show a note that the previous default's
settings remain available if it was also a named entry.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- ProviderManager`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ProviderManager.tsx frontend/src/components/ProviderManager.test.tsx
git commit -m "feat(frontend): manage provider entries"
```

---

### Task 5: The purposes panel

**Files:**
- Modify: `frontend/src/components/ProviderManager.tsx`
- Test: `frontend/src/components/ProviderManager.test.tsx` (append)

**Interfaces:**
- Consumes: Task 3, the `Purpose` set (MP-3).
- Produces: a per-family uses panel with a provider dropdown per purpose.

- [ ] **Step 1: Write the failing test**

```tsx
test("setting a purpose writes media.purposes", () => {
  const onChange = vi.fn();
  render(<ProviderManager family="tts" config={withNpc()} onChange={onChange} />);
  fireEvent.change(screen.getByLabelText("npc"), { target: { value: "npc" } });
  const next = onChange.mock.calls.at(-1)[0];
  expect(next.media.purposes.npc).toBe("npc");
});

test("choosing default clears the purpose", () => {
  const onChange = vi.fn();
  render(<ProviderManager family="tts" config={withNpcPurpose()} onChange={onChange} />);
  fireEvent.change(screen.getByLabelText("npc"), { target: { value: "default" } });
  const next = onChange.mock.calls.at(-1)[0];
  expect(next.media.purposes?.npc).toBeUndefined();
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- ProviderManager`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Render a dropdown per purpose of the family (`narrator`, `npc` for TTS; `scene`, `portrait`,
`placeholder` for image), offering `default` plus every named entry. Writing a non-default value
sets `media.purposes[use]`; choosing `default` deletes the key.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- ProviderManager`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ProviderManager.tsx frontend/src/components/ProviderManager.test.tsx
git commit -m "feat(frontend): assign purposes in the provider manager"
```

---

### Task 6: Mount the manager and add the LLM role list

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx` (mount `ProviderManager`)
- Modify: `frontend/src/components/ProviderManager.tsx` (an LLM-roles mode)

**Interfaces:**
- Consumes: Tasks 3-5.
- Produces: the manager visible in the studio; an LLM role list with the same chrome.

- [ ] **Step 1: Mount the manager**

Replace the single per-family forms' surrounding chrome with `<ProviderManager>` for the TTS, STT,
and Image tabs. The per-entry Edit form remains the existing form, now bound to the selected entry.

- [ ] **Step 2: Add the LLM role list**

In an `llm` mode, render one row per `config.agents.roles` entry with its provider label, tier
badge, and key state. Editing a role keeps the existing role editor. No structural config change.

- [ ] **Step 3: Typecheck and test**

Run: `npx tsc --noEmit` and `npm run test -- ProviderManager`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx frontend/src/components/ProviderManager.tsx
git commit -m "feat(frontend): mount the provider manager in settings"
```

---

### Task 7: Verification

- [ ] **Step 1: No-diff guard**

Add a component test: load the manager with a default config, save without interacting, and assert
the serialized config is unchanged.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Every entry of a family is listed with its tier and key state.
- Add, duplicate, rename, remove, and set-default work on the draft config.
- A rename updates referencing purposes; a remove is blocked when referenced.
- Setting a purpose writes `media.purposes`; choosing default clears it.
- A config with no named providers shows one row.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the provider manager against a no-op save"
```
