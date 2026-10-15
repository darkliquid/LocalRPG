# Unify the API Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One error type for a failed request, one face per method, and every `/api` call through the client.
**Architecture:** One response-checking helper throws `APIError` built from the server's own error object; the five duplicated methods converge on one variant; the two stray fetches move into the client or are documented as a media boundary.
**Tech Stack:** TypeScript 5.9, Vitest, the existing `client.ts`.
**Spec:** `docs/superpowers/specs/2026-10-15-unify-api-client-design.md`

## File Map

| File | Why |
|---|---|
| `frontend/src/api/client.ts` | the error type, the helper, and the 77 throws |
| `frontend/src/api/client.test.ts` | the three error shapes and the method pairs |
| `frontend/src/components/editor/frontmatterSchema.ts` | the raw `/api` fetch |
| `frontend/src/components/StoryTheater.tsx` | the media fetch and its comment |

## Global Constraints

- **The same requests, in the same order, with the same bodies.** This changes what a failure looks
  like, not what is sent.
- **The server's message wins** where it sent one; `statusText` is the fallback, not the default.
- **A caller switching on `HTTPError` keeps working**, whether it is renamed or kept.
- **`tsc` is the proof** for the method pairs, and the mocked-`fetch` tests for the error shapes.

---

### Task 1: Measure

- [ ] **Step 1: The counts, recorded in the batch's pull request**

77 `throw new Error`, 43 `HTTPError` mentions, two stray fetches, and for each of the five pairs the
static and instance call-site counts (`advanceUnlock`, `getGameUsage`, `getTurnContext`,
`getWorkingSet`, `updateGameSettings`).

---

### Task 2: The error type and the helper (its own pull request)

- [ ] **Step 1: `APIError`** with `status` and an optional `code`, and the message from the server.
- [ ] **Step 2: One helper** that every method's response goes through.
- [ ] **Step 3: The three shapes tested** through a mocked `fetch`.
- [ ] **Step 4: The remaining 77 throws converted**, in the same change, so no method still throws a
      bare `Error` for a failed request.

---

### Task 3: The five method pairs

- [ ] **Step 1: Count the call sites per variant.**
- [ ] **Step 2: Keep the one used**, or make the static a one-line delegate.
- [ ] **Step 3: A test per pair.**

---

### Task 4: The two stray fetches

- [ ] **Step 1: The frontmatter schema** becomes a client method, with a test.
- [ ] **Step 2: The media fetch in `StoryTheater`** keeps its comment naming the boundary, or moves if
      it turns out to be an API call after all.

---

### Task 5: Verification

- [ ] **Step 1: `grep -rn "fetch(" frontend/src | grep -v api/client.ts`** returns only the media boundary, with its comment.
- [ ] **Step 2: `npx tsc --noEmit && npx vitest run --coverage && mise run lint:frontend`**
- [ ] **Step 3: `mise run test && mise run lint`**
