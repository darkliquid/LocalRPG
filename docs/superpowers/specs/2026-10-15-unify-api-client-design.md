# Unify the API Client Design

**Date:** 2026-10-15
**Status:** Proposed
**Issue:** gh-188 (the bead LocalRPG-14a.3)
**Epic:** [#152](https://github.com/darkliquid/LocalRPG/issues/152)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (FE-7)
**Scope:** `frontend/src/api/client.ts` and the two components that fetch beside it. No Go change.

## 1. Problem

The frontend has one fetch layer by convention — `frontend/src/api/client.ts` — and three ways it is
not, all measured today:

| The fault | What is there |
|---|---|
| **Two error types for one thing** | 77 `throw new Error` against 43 `HTTPError` mentions in `client.ts`. `getGameState` throws the typed one while its neighbours throw a bare `Error`, and many of the bare ones build a message from `res.statusText` |
| **Five methods with two faces** | `advanceUnlock`, `getGameUsage`, `getTurnContext`, `getWorkingSet` and `updateGameSettings` each exist as both a static and an instance method |
| **Two fetches beside the client** | `components/editor/frontmatterSchema.ts:15` fetches `/api/schema/entity-frontmatter` raw, and `components/StoryTheater.tsx:267` fetches a media URL |

Each is small; together they are why a caller cannot answer "what does a failed call look like". That
matters more than it did a week ago: **FE-9 now shows failures to the player**, and the sentence it
shows is whatever the error carries. A bare `Error("getGameState: Not Found")` is not a sentence, and
the Go side already sends a structured one — `{"error":{"code","message"}}` — that is being thrown
away in favour of `statusText`.

## 2. Goals

- **One error type for a failed request**, carrying the status and the server's own code and message
  when it sent one.
- **One face per method.** A static variant either goes or becomes a one-line delegate to the
  instance method, whichever the call sites justify; five methods having two signatures is a decision
  nobody made.
- **Every `/api` call goes through the client.** The one fetch that is not an API call — a media URL
  the server already handed the app — stays, with a comment saying that is what it is.
- **No caller changes its behaviour** other than getting a better error out of a failure.

## 3. Non-goals

- **The routes or the Go handlers**, which already return the structured error this proposal starts
  using.
- **Retries, timeouts or caching**, which are their own decision and are not a unification.
- **Splitting `client.ts`** (1575 lines). One file with one error type and one face per method is a
  real improvement; whether it should be several files is a later question, and answering it now
  would bury this diff.
- **The Go/TypeScript contract check** (FE-5), which is a different fault: this proposal makes the
  errors agree with each other, not with the server's schema.

## 4. Design

### One error, built from what the server said

```ts
// APIError is every failed request. The status is the response's, and Code and
// Message are the server's own error object when it sent one: a route that failed
// for a reason the player can act on says so, rather than the client inventing a
// sentence from a status line.
export class APIError extends Error {
  readonly status: number;
  readonly code?: string;
}
```

- A response is parsed once, in one helper: on `!res.ok` it reads the JSON body, takes `error.code`
  and `error.message` where they are there, and throws. Every method calls that helper, which is what
  makes "what does a failure look like" answerable in one place.
- `HTTPError` is either that type renamed or deleted in its favour; the plan measures its users first,
  because a caller that switches on it should keep working.
- A network failure (no response at all) is the same type with `status: 0` and a sentence naming the
  connection, so callers do not need a second branch.

### One face per method

For each of the five pairs, count the call sites of each variant and keep the one they use. Where both
are used, the static becomes a one-line delegate — which is not a second implementation, and the
difference becomes visible rather than incidental.

### Every `/api` call through the client

- `frontmatterSchema.ts`'s raw fetch becomes a method on the client, which is where the URL, the error
  handling and the cache live for every other route.
- `StoryTheater.tsx`'s `fetch(url)` is not an API call: the URL is a media file the server already
  named, so a failure there is a playback problem. It stays, with a comment saying which boundary it
  is — the same rule FE-8 applied to casts.

## 5. Behaviour

- A failed request now carries the server's own message where there is one, instead of `statusText`,
  so the failure region FE-9 added shows something a player can read.
- A caller that switched on `HTTPError` keeps working, whether it is renamed or kept.
- Nothing else changes: the same requests, in the same order, with the same bodies.

## 6. Testing

- **The error shape**: a 4xx carrying the Go error object yields one `APIError` with the code and
  message; a bare 500 yields one with a status and no code; a network failure yields one with
  `status: 0`. These are the three cases a caller can see, and they are cheap to drive through a
  mocked `fetch`.
- **One face**: a test per pair that the static and instance forms now agree, or are gone.
- **The stray calls**: a test that the frontmatter schema comes through the client.
- `npx tsc --noEmit`, `npx vitest run --coverage`, `mise run lint:frontend`, then `mise run test`.

## 7. Rollout

Batches, one pull request each, the bead open across them: the error type and the helper first (it
touches every method), then the five method pairs, then the two stray fetches. The first batch is the
one worth reviewing closely; the rest are mechanical.

## 8. Risks

- **A better error can be a louder one.** A route that used to fail quietly with a `statusText`
  message now carries the server's message into FE-9's region; if a route's message is written for a
  developer, it becomes developer prose on screen. The batch that wires them re-reads the messages.
- **77 call sites is a wide diff.** They all converge on one helper, so the diff is repetitive; it is
  still a diff to read, and it is batched.
- **Removing a static method breaks an import somewhere.** The count of call sites per variant is the
  control, and `tsc` catches the rest.
