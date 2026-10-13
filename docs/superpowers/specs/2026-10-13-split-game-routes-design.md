# Split the game route handler Design

**Date:** 2026-10-13
**Status:** Proposed
**Issue:** [#168](https://github.com/darkliquid/LocalRPG/issues/168)
**Epic:** [#151](https://github.com/darkliquid/LocalRPG/issues/151)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (RD-2, finding R-2)
**Scope:** `pkg/gui/server.go`, and the six files `handleGameRoutes` becomes. No route, status or
body changes.

## 1. Problem

`Server.handleGameRoutes` (`pkg/gui/server.go:371`) is **676 lines** and serves every URL under
`/api/game/`. It parses the game id, special-cases the one route without a sub-action, and then
hands a single `switch` twenty-one `case` labels: audio, folders, banner/icon, generate-asset, tts,
restart, settings, state, context, working-set, entities, recap, findings, graph, chronicle, usage,
advance, turn, location, character and entity.

Three things are wrong with it, and only the first is about length.

- **Length.** It is the second-longest function in the repository after
  `ProcessActionStream` (RD-3, `pkg/engine/orchestrator.go`). Finding the handler for one URL means
  scrolling past twenty others.
- **Repeated parsing.** `strconv.Atoi` is called on a path segment ten times, each with its own
  four-line error block. The `turn` case alone accounts for all ten.
- **Repeated decode.** The `json.NewDecoder(http.MaxBytesReader(w, r.Body, N)).Decode(&x)` idiom
  appears eight times with a byte cap that is one of three different values, and two different
  ways of answering the failure.

The routing is also unevenly weighted: the `turn` case (line 699 to 871, 172 lines) and the
`entity` case (936 to 1041, 105 lines) are each larger than every other group combined, so the
function's shape does not match the resources it serves.

## 2. Goals

- `handleGameRoutes` becomes a dispatcher: parse the path once, then one call per resource.
- Every resource gets a named handler in a file named for it, so the file list explains the
  routing.
- The path parse, the body decode and the turn/segment parse each exist once.
- No route is added, removed, renamed or reordered, and no response status or body changes,
  except the one decode-failure body normalisation stated in section 5.

## 3. Non-goals

- **Rewriting the routing as a table or a router type.** `registerRoutes` already iterates
  `mounts`; this proposal is about the body of the one handler that mux entry points at, not
  about replacing `http.ServeMux`.
- **`server.go` itself.** The file keeps `Server`, the mux, the error mappers, the shared
  response helpers and the non-`/api/game/` handlers. Only the `handleGameRoutes` subtree moves.
- **The `writeGameError` mapping.** RD-6 and the security epic own error handling. This proposal
  moves call sites; it does not change what they map to.
- **Changing a byte cap.** Each decode keeps the limit it has today, even where two limits look
  interchangeable, so the move is provably behaviour-preserving.

## 4. Design

Six files, one of which is the dispatcher. Every `case` label in the current switch appears below
exactly once, and a test asserts that.

| File | Contents |
|---|---|
| `routes_game.go` | `handleGameRoutes`, the shared path/decode/turn-number helpers |
| `routes_game_campaign.go` | state, context, working-set, entities, recap, findings, graph, chronicle, usage, restart, settings, advance |
| `routes_game_assets.go` | banner/icon, generate-asset, folders |
| `routes_game_audio.go` | audio/ledger, tts (uncached, batch) |
| `routes_game_turn.go` | turn (submit, resolve-check, renegotiate, ledger, scene-image, play, segment audio/play) |
| `routes_game_entity.go` | location/art, character/portrait, entity (turns, memories, put, art, merge, delete, get) |

**The rule is "what does this URL address".** A route that reads or writes the campaign as a whole
is a campaign route; a route that addresses one entity by id is an entity route, even when the
service method behind it reads campaign state.

`handleGameRoutes` keeps the three things that are true of every game route, in one place:

1. the `DELETE /api/game/{id}` special case, which is the only route without a sub-action and so
   cannot wait for the two-part check;
2. the two-part check itself;
3. the `switch` on `parts[1]`.

Everything else is a call. A handler takes the arguments it reads: `(w, r, gameID)`, plus `parts`
where it needs a path segment. Taking `parts` everywhere would be uniform but would leave fourteen
handlers holding a parameter they never touch, so each declares only what it uses. A handler
returns nothing.

### The shared helpers

Three helpers replace the three repeated idioms:

- `gameRoutePath(w, r) (gameID string, parts []string, ok bool)` trims the mount prefix, splits
  the path, validates the id and answers 400 itself. It is the preamble the function opens with
  today, unchanged.
- `decodeJSONBody(w, r, limit, dst) bool` decodes a JSON body under `http.MaxBytesReader` and
  answers 400 on failure, replacing the eight decoded sites. It reports whether the caller should
  continue.
- `turnNumberFromPath(w, parts) (int, bool)` and `segmentIndexFromPath(w, parts) (int, bool)` parse
  the numeric path segments and answer 400 with the message each site uses today
  (`"invalid turn number"`, `"invalid segment index"`).

The two numeric helpers live in `routes_game.go` with the rest because they parse a path and belong
beside `gameRoutePath`, rather than buried in the two-hundred-line turn handler they are used by.

## 5. Behaviour

None that a request can observe, with one deliberate exception.

- **Routes.** The mount table, the `switch` labels and the order of the method checks inside each
  case are unchanged, so every URL reaches the same branch and the same response.
- **Statuses and bodies.** Every error written today is written by the same call with the same
  arguments. The `writeInvalidRequest` / `http.Error` split for a malformed body is the exception:
  four sites (`settings` PATCH, `findings` POST, `entity` PUT and `entity` merge) answer with a
  plain-text 400 today, while the other four answer with the structured generation-failure
  envelope. `decodeJSONBody` gives all eight the structured envelope. That is one error shape for
  one failure, which is the convention `writeInvalidRequest` already documents ("clients parse one
  error type"), and it removes the only place in this function where the same fault had two
  bodies.

The compiler proves the rest: a case left behind, or a helper left with a caller that cannot see
it, does not build.

## 6. Testing

- `go build ./...` and `go vet ./...`, which catch a missed case or a stray import.
- The existing `pkg/gui` suite, unchanged. It exercises every route through
  `httptest`-backed tests, and `routes_test.go` checks the mount table against the checked-in
  manifest, so a lost mount is a failure with a diff.
- **A test that the dispatch is exhaustive**, in the shape of the split guard from RD-1
  (`service_split_test.go`): a table of `step -> handler method`, and a test that parses the
  `switch` in `handleGameRoutes` and asserts every step dispatches to the method the table names,
  that every case body is that one call and nothing else, and that every named method exists. This
  is what stops the dispatcher growing a second body.
- **A test that the dispatcher stays small**, asserting `handleGameRoutes` is under eighty lines.
  The bound is loose on purpose: the switch is two lines per action and there are twenty-one, so
  the test catches a body growing back rather than constraining the routing table.
- `gofmt -l`, `staticcheck`, `mise run lint`.

## 7. Rollout

One commit. The move is a permutation, so it is reviewable as one, and an intermediate commit that
does not build would be worse than a large diff that does.

The helpers land first (they change no behaviour, and the compiler proves each rewritten call
site equivalent), then the case bodies move one file at a time, then the dispatcher is reduced to
calls in the same commit the last body leaves.

## 8. Risks

- **A large move-shaped diff**, which is hard to review for behaviour. Mitigated by moving whole
  case bodies verbatim, by one resource per file, and by the fact that the only edits inside a
  moved body are the three helper substitutions.
- **The file assignment is a judgement call.** `location` and `character` are entity-shaped, so
  they go to `routes_game_entity.go`; a reader could argue for their own files. The guard test
  makes the current choice explicit and cheap to change.
- **`routes_game_campaign.go` holds twelve handlers**, more than any other file. They are short and
  uniform, and the alternative (a file per action) trades one 676-line function for twelve
  five-line files. If the file grows, the guard test is what makes splitting it again safe.
- **Churn with SEC-1.** That proposal routes every handler through one error mapper. This one moves
  call sites without changing them, so SEC-1 rebases onto the same text, in smaller files.
