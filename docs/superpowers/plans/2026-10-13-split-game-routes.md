# Split the game route handler Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Break `Server.handleGameRoutes` (`pkg/gui/server.go:371`, 676 lines) into a short dispatcher and one file per resource, with the path parse, the body decode and the turn/segment parse each written once.
**Architecture:** Six files in `pkg/gui` (`routes_game.go` plus five resource files), three shared helpers, and a guard test that pins the dispatch.
**Tech Stack:** Go 1.27, `net/http`, the standard test library and `go/parser`.
**Spec:** `docs/superpowers/specs/2026-10-13-split-game-routes-design.md`

## Global Constraints

- No route is added, removed, renamed or reordered. The mount table and the switched labels stay as they are.
- No status code changes. The only body change is the four malformed-body sites that move from a plain-text 400 to the structured envelope, and that is intentional.
- Each decode keeps the byte cap it has today. Do not unify `64 << 10`, `1024*1024` and `64*1024`.
- Whole case bodies move verbatim. The only edits inside a body are the three helper substitutions.
- Not a single new test double: the suite already routes every URL through the real `Service`.

---

### Task 1: Add the shared helpers

**Files:**
- Modify: `pkg/gui/server.go`

- [ ] **Step 1: Add `gameRoutePath`**

The preamble the function opens with, unchanged, as a helper that answers its own 400s:

```go
// gameRoutePath splits a /api/game/ path into the campaign id and the remaining
// segments, validating the id. It reports whether the caller should continue; a
// malformed path has already been answered.
func gameRoutePath(w http.ResponseWriter, r *http.Request) (string, []string, bool) {
	path := strings.TrimPrefix(r.URL.Path, "/api/game/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing game id", http.StatusBadRequest)
		return "", nil, false
	}
	if err := pathutil.ValidateID(parts[0]); err != nil {
		http.Error(w, "invalid game id", http.StatusBadRequest)
		return "", nil, false
	}
	return parts[0], parts, true
}
```

- [ ] **Step 2: Add `decodeJSONBody`**

```go
// decodeJSONBody decodes a JSON request body under a byte cap. It reports whether
// the caller should continue; a malformed body has already been answered.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, limit int64, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(dst); err != nil {
		writeInvalidRequest(w, "invalid request body")
		return false
	}
	return true
}
```

- [ ] **Step 3: Add `turnNumberFromPath` and `segmentIndexFromPath`**

```go
func turnNumberFromPath(w http.ResponseWriter, parts []string) (int, bool) {
	n, err := strconv.Atoi(parts[2])
	if err != nil {
		http.Error(w, "invalid turn number", http.StatusBadRequest)
		return 0, false
	}
	return n, true
}

func segmentIndexFromPath(w http.ResponseWriter, parts []string) (int, bool) {
	n, err := strconv.Atoi(parts[4])
	if err != nil {
		http.Error(w, "invalid segment index", http.StatusBadRequest)
		return 0, false
	}
	return n, true
}
```

- [ ] **Step 4: Build**

Run: `go build ./pkg/gui/`
Expected: exit 0. The helpers are unused so far, which Go permits for unexported functions only if something references them, so either land them in this task's commit with Task 2 or accept the `U1000` until it does; `staticcheck` runs at the end, not here.

---

### Task 2: Extract the campaign-level routes

**Files:**
- Create: `pkg/gui/routes_game_campaign.go`
- Modify: `pkg/gui/server.go`

- [ ] **Step 1: Move twelve cases**

`state`, `context`, `working-set`, `entities`, `recap`, `findings`, `graph`, `chronicle`, `usage`,
`restart`, `settings`, `advance` become `(s *Server) handleGameStateRoute`, `handleGameContextRoute`,
`handleGameWorkingSetRoute`, `handleGameEntitiesRoute`, `handleGameRecapRoute`,
`handleGameFindingsRoute`, `handleGameGraphRoute`, `handleGameChronicleRoute`,
`handleGameUsageRoute`, `handleGameRestartRoute`, `handleGameSettingsRoute`,
`handleGameAdvanceRoute`. None of the twelve reads a path segment, so each takes
`(w http.ResponseWriter, r *http.Request, gameID string)` and nothing else.

- [ ] **Step 2: Replace the decode sites**

`settings` and `findings` use `decodeJSONBody(w, r, maxTurnBody, &req)`; a false return means return.

- [ ] **Step 3: Build**

Run: `go build ./pkg/gui/`
Expected: exit 0 only once the dispatcher in Task 6 calls the new methods; the intermediate state
is not buildable, which is why the pass is one commit (spec section 7).

---

### Task 3: Extract the asset routes

**Files:**
- Create: `pkg/gui/routes_game_assets.go`
- Modify: `pkg/gui/server.go`

- [ ] **Step 1: Move the cases**

`banner`/`icon` become `handleGameAssetRoute`; `generate-asset` becomes
`handleGameGenerateAssetRoute`; `folders` becomes `handleGameFoldersRoute`.

- [ ] **Step 2: Replace the decode site**

The `generate-asset` body uses `decodeJSONBody(w, r, 1024*1024, &req)`.

- [ ] **Step 3: Keep the branch order**

`banner`/`icon` checks GET, then POST, then DELETE, and falls through to `405`. Leave the order alone.

---

### Task 4: Extract the audio routes

**Files:**
- Create: `pkg/gui/routes_game_audio.go`
- Modify: `pkg/gui/server.go`

- [ ] **Step 1: Move the cases**

`audio` becomes `handleGameAudioRoute` with the `ledger` sub-action inside it; `tts` becomes
`handleGameTTSRoute` with `uncached` and `batch` inside it.

- [ ] **Step 2: Replace the decode site**

The `ledger` POST body uses `decodeJSONBody(w, r, 1024*1024, &req)`.

- [ ] **Step 3: Keep the `case "audio"` fall-through**

A third segment that is not `ledger` is a `404`, and so is a method that is neither GET nor POST.
Leave both.

---

### Task 5: Extract the turn and entity routes

**Files:**
- Create: `pkg/gui/routes_game_turn.go`
- Create: `pkg/gui/routes_game_entity.go`
- Modify: `pkg/gui/server.go`

- [ ] **Step 1: Move the `turn` case whole**

`handleGameTurnRoute` keeps every sub-route in its current order: submit, `resolve-check`,
`renegotiate`, `ledger` (GET and POST), `scene-image` (GET and POST), `play`, `segment/{i}/play`,
then the `segment/{i}/audio` tail. The two-part guard, the six-part guard and the `switch` on the
audio error stay.

- [ ] **Step 2: Replace the eight numeric parses**

Each `strconv.Atoi` becomes `turnNumberFromPath(w, parts)` or `segmentIndexFromPath(w, parts)`; a
false return means return. The `ledger` POST body uses `decodeJSONBody(w, r, 1024*1024, &req)`.

- [ ] **Step 3: Move `location`, `character` and `entity`**

`handleGameLocationRoute`, `handleGameCharacterRoute` and `handleGameEntityRoute`. The `entity`
body decodes (PUT and merge) use `decodeJSONBody(w, r, maxTurnBody, …)`.

- [ ] **Step 4: Keep the entity fall-through order**

`entity` tries `turns`, then `memories`, then PUT, then `art`, then `merge`, then DELETE, then the
bare GET. The bare GET is last and is the only one that answers 404 rather than 400 on a missing
entity. Leave the order alone.

---

### Task 6: Reduce the dispatcher

**Files:**
- Create: `pkg/gui/routes_game.go`
- Modify: `pkg/gui/server.go`

- [ ] **Step 1: Move `handleGameRoutes`**

`handleGameRoutes` itself moves to `routes_game.go`, with `gameRoutePath`, `decodeJSONBody`,
`turnNumberFromPath` and `segmentIndexFromPath`. Its body becomes the preamble, the DELETE
special case, the two-part check and one call per case. The `default` stays `http.NotFound`.

- [ ] **Step 2: Verify the dispatcher is short**

Run: `awk '/^func \(s \*Server\) handleGameRoutes/,/^}/' pkg/gui/routes_game.go | wc -l`
Expected: under 80.

- [ ] **Step 3: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: both exit 0.

---

### Task 7: Add the dispatch guard

**Files:**
- Create: `pkg/gui/routes_game_test.go`

- [ ] **Step 1: Write the guard**

A table of `parts[1] -> handler method name`, and a test that parses `routes_game.go` with
`go/parser`, reads the `switch` inside `handleGameRoutes`, and asserts every case dispatches to the
method the table names, that no case is missing from the table, that each case body is that one
call and nothing else, and that each named method is declared. Modelled on `service_split_test.go`;
the point is that the dispatcher cannot grow a body without the table noticing.

- [ ] **Step 2: Write the length guard**

A second test that asserts `handleGameRoutes` is under eighty lines. The bound is loose on purpose:
it catches a body growing back rather than constraining the routing table.

- [ ] **Step 3: Prove it fails**

Point one case at the wrong handler and run
`go test -run TestGameRouteDispatchIsComplete ./pkg/gui/`. Expected: it fails, naming the action and
both method names. Then put it back.

---

### Task 8: Verify

**Files:** none

- [ ] **Step 1: Lint**

Run: `gofmt -l pkg/gui && mise run lint`
Expected: silent, exit 0.

- [ ] **Step 2: Test**

Run: `mise run test`
Expected: Go suite and frontend suite pass, coverage floor met.

- [ ] **Step 3: Check the routes are unchanged**

Run: `go test -run TestRouteManifestIsCurrent ./pkg/gui/`
Expected: pass. The manifest is checked in and no mount moved, so a diff here is a mistake.

- [ ] **Step 4: Commit**

Commit as `refactor(gui): split the game route handler by resource`.