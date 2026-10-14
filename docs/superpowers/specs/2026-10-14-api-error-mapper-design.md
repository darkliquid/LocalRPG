# Route every API error through one mapper Design

**Date:** 2026-10-14
**Status:** Proposed
**Issue:** [#212](https://github.com/darkliquid/LocalRPG/issues/212)
**Epic:** [#154](https://github.com/darkliquid/LocalRPG/issues/154)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (SEC-1, finding S-2)
**Scope:** `pkg/gui`'s HTTP surface: 97 `http.Error(w, err.Error(), status)` sites, one string match on an error's text, and ten wrong-method fallbacks.

## 1. Problem

Ninety-seven handler sites answer a failure by writing the error's own text:

```go
http.Error(w, err.Error(), http.StatusInternalServerError)
```

Three things follow from that, and the count is the least of them.

- **Internal detail reaches the client.** Those errors are wrapped on the way up (`fmt.Errorf("open campaign: %w", err)`), so a filesystem path, a package name or a provider's response lands in a browser or on a `0600` socket.
- **The same failure is answered differently.** Because the status is chosen at the call site, each handler guesses: a missing entity is `404` while a failed `GetGameState` is `500`, `CreateGame`'s failures are `400` while `ListGames`' are `500`. Every one of those guesses is a *fallback* after a chain of `errors.Is` checks, so the guess is what the client gets for anything the chain did not name.
- **The client cannot tell what to do.** With no stable code in the body it cannot separate "not found" from "try again" from "stop asking", which is the cause of F-5 as well as its own problem.

`handleContentExportRoute` is the shape of the whole problem in one place: it decides `404` from `strings.Contains(err.Error(), "not found")`.

## 2. Goals

- One place decides the status, the code and the message a failure produces.
- An error this side caused is answered with a fixed message and a stable code, and its detail is recorded rather than sent.
- An error the client caused is answered with the message that says what to change.
- Wrong methods are `405` with an `Allow` header everywhere, not `404` in ten places and `405` in sixty-three.
- No failure is decided by matching an error's text.

## 3. Non-goals

- **Rewriting each handler's `errors.Is` chain.** The chains that name a sentinel give the same answer the mapper's table does, so collapsing them changes no behaviour and is a separate, mechanical sweep. This proposal makes the *fallback* the mapper's decision, which is where the guesses were.
- **Touching the generation-failure envelope.** `writeGenerationError` already carries a `code` and a message from `harness.GenerationFailure`, and the client already parses it.
- **What the service returns.** This is about the HTTP surface; the errors' types are as they are.

## 4. Design

A fault is a status, a code and a message, and one function decides all three from the error:

```go
// apiFaultFor decides what a client is told about err. A failure the client
// caused carries its own message, because that names what to change. Anything
// else is this side's fault, and is answered with a fixed message and a stable
// code, because a wrapped error names paths and package structure that are not
// the client's business.
func apiFaultFor(err error) apiFault
```

The table is built from the sentinels the handlers already distinguish, so no handler's decision is lost:

| The error | Status | Code |
|---|---|---|
| `ErrTurnInFlight` | 409, with `Retry-After` | `turn_in_flight` |
| A conflict: `ErrDuplicateEntityID`, `ErrContentConflict`, `ErrRegistrySourceExists`, `ErrWorldEntityExists`, `ErrWorldExists`, `ErrSystemInUse`, `ErrWorldInUse`, `ErrDirectoryChoiceInFlight`, `ErrSaveFileChoiceInFlight`, `models.ErrDownloadActive`, `ErrExportInFlight` | 409 | `conflict` |
| A request the client got wrong: `ErrInvalidRequest`, the `pathutil` identifier sentinels, `ErrInvalidFolderPath`, `ErrAdvancementRefused`, `ErrEmptyDraftSelection`, `ErrInvalidCounter`, `ErrNegotiationLimit`, `ErrNoPendingCheck`, `ErrPendingCheckMismatch`, `ErrSourceTooLarge`, `ErrExportDirRequired` | 400 | `invalid_request` |
| Not found: `ErrWorldNotFound`, `ErrSystemNotFound`, `ErrReferenceSystemNotFound`, `ErrContentNotFound`, `ErrDocNotFound`, `ErrRegistrySourceNotFound`, `fs.ErrNotExist`, `os.ErrNotExist` | 404 | `not_found` |
| Not available on this build: `ErrNoNativeDialog`, `ErrNoNativeSaveFileDialog`, `errNoURLOpener` | 501 | `not_implemented` |
| Rate limited: `harness.ErrRateLimitedUntil` (by `errors.As`) | 429, with `Retry-After` | `rate_limited` |
| Ours, and explainable: `ErrCampaignNotPlayable`, `ErrNoBatchAPI` (a provider with no batch endpoint) | 503, carrying the message | `unavailable` |
| The client went away: `context.Canceled` | 499 | `cancelled` |
| **Anything else** | 500 | `internal` |

Two things make the last row the point of the proposal. Its message is fixed (`the request could not be completed`), so no wrapped error is echoed; and its detail goes to `Service.logger` as an `api.error` event, so the diagnosis is not lost with the leak. The mapper takes the logger it records through, because the logger belongs to the service. Both types here serve routes, so `*Server` and `*Service` each carry a one-line method onto it; the registry routes are service methods.

`invalid(err)` marks an error as the request's fault where a handler knows it and the service's error carries no sentinel. It exists so that a handler's own judgement is written down as a judgement rather than encoded as a status number.

Every one of the 97 sites becomes `s.writeAPIError(w, err)`, or `s.writeAPIError(w, invalid(err))` where the handler is stating that the request was wrong. The envelope is the one the client already parses, with the code added:

```json
{"error": {"code": "not_found", "message": "document not found"}}
```

Wrong methods get one helper, `methodNotAllowed(w, http.MethodGet, ...)`, which answers `405` with an `Allow` header. Eight sites whose method check answered `404` become it. The sixty-three that already answered `405` keep their status; giving each of them the right `Allow` list is a per-route judgement that deserves its own pass and is not guessed at here.

## 5. Behaviour

| | Before | After |
|---|---|---|
| An internal failure | 500, the wrapped error's text | 500, `{"code":"internal","message":"the request could not be completed"}`, detail in the log |
| A request the client got wrong | 400, usually, sometimes 404 or 500 | 400, its own message, `code: invalid_request` |
| A failure no handler named | whatever the handler guessed | 500 generic, unless the error is one the table knows |
| A wrong method | 404 here, 405 there | 405 with `Allow` |
| Content export of a missing id | 404 from a substring match | 404 from `errors.Is` |

## 6. Testing

- **A table test over `apiFaultFor`**: one case per row, asserting the status and the code, including that an unknown wrapped error is `500`/`internal`.
- **The leak, pinned**: a handler is driven with an error whose message contains a path, and the test asserts the path is not in the body while the status is 500.
- **A wrong method**: a request with the wrong method gets 405 and an `Allow` header naming the right one.
- **The string match is gone**: the export route answers 404 through the sentinel, tested by the type rather than the text.
- The existing route tests exercise every converted site; `mise run test` and `mise run lint`.

## 7. Rollout

One commit, in this order: the mapper and its table with the table test first, then the 97 sites, then the method helper, then the string match. The paragraphs that follow are what the compiler cannot check, so each stage ends with its own verification.

## 8. Risks

- **A 400 fallback that becomes a 500.** A validation failure with no sentinel is, by this table, this side's fault. Where a handler knows better it says so with `invalid(err)`, which is the audit this proposal forces; the spec's table records every case that was decided.
- **The chains are left in place**, so a reader sees two places that appear to decide. They agree by construction, and collapsing them is recorded as follow-up work rather than pretended here.
- **The message for a 4xx is still the error's own**, so a wrapped client error could carry a path. In practice the client-fault errors are the `pathutil` sentinels and the handlers' own text; the alternative, a generic message for 4xx too, would take away the client's ability to fix the request.
