# The long functions Design

**Date:** 2026-10-14
**Status:** Proposed
**Issue:** [#177](https://github.com/darkliquid/LocalRPG/issues/177)
**Epic:** [#151](https://github.com/darkliquid/LocalRPG/issues/151)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (RD-11, finding R-12)
**Scope:** 71 non-test functions over eighty lines, worked down in batches by package. No behaviour change.

## 1. Problem

**71 non-test functions are longer than eighty lines**, and the count is the parser's rather than an impression:

```bash
go run /tmp/lf/main.go .   # the measurement program, kept in this proposal's plan
```

The review estimated "about forty" from a sample; the real list is longer, and it starts with `ProcessActionStream` at **981 lines** and `runGenerationLoop` at **416**. The distribution matters more than the total:

| Package | Functions over eighty lines |
|---|---|
| `pkg/gui` | 17 |
| `pkg/provider` | 9 |
| `pkg/engine` | 5 (plus two in `orchestrator.go`) |
| `cmd/localrpg` | 6 |
| `pkg/content`, `pkg/export`, `pkg/harness`, `pkg/config` | 5, 4, 4, 3 |
| the rest | 22, one or two each |

Length is a proxy, and it is worth being precise about what it proxies. A long function has phases, the phases are its real structure, and they are invisible: `Unpack` validates a destination, stages, reads a manifest, indexes what it declares, extracts and verifies each member, checks nothing is missing, and promotes the staging directory, all at one indentation level. Finding the containment check that protects the extraction means reading 230 lines to discover it is on line 200. The same is true of every entry in the list, and the review's own evidence for the cost is that a reader asking "where does a turn start" gets a 981-line answer.

## 2. Goals

- Every function in the list is under eighty lines, or carries a written reason why not.
- The phases of each are named functions with one job, so the caller reads as a table of contents.
- No behaviour change: the same errors, in the same order, with the same deferred work.

## 3. Non-goals

- **`ProcessActionStream`.** It is its own proposal (RD-3), because splitting it is a decision about what a turn's phases *are* rather than about length. This proposal's list excludes it.
- **`runGenerationLoop` before RD-3 lands.** It is 416 lines in the same file, and both proposals would rewrite its boundaries. It is in the list and lands after RD-3.
- **A blanket eighty-line lint, for now.** A gate that fails the tree for the next few batches would be turned off rather than obeyed. The last batch adds the check once the list is empty, which is the shape the `interface{}` proposal used.
- **Extracting for its own sake.** A three-line helper called once is noise, and a function that reads well at ninety lines is left alone with the reason recorded.

## 4. Design

### How a function is broken up

1. **Name the phases first.** The comment blocks and the blank-line-separated runs in a long function usually already name them; where they do not, the names come from what the code is *for*.
2. **Extract each phase into a function with one job and a doc comment.** The original becomes a sequence of calls, which is what a reader wanted in the first place.
3. **Prefer a small state value over six out-parameters.** `Unpack`'s extraction loop carries a declared-file index, an extracted set, the signature bytes, a running size and a member count; that is a struct, not five pointers.
4. **Keep a guard where the scanner sees it.** The CodeQL lesson from SEC-3 applies to every extraction that touches a path: a containment check that becomes a helper call is one that stops counting, so the check stays inlined at the sink and the helper stays behind it.
5. **Move, do not rewrite.** The same error strings, the same order, the same `defer`s. Where a phase turns out to be wrong, that is a different proposal.

### The batches

One batch per package group, each its own pull request, because a single diff touching seventy functions across twenty packages is not reviewable and not bisectable.

| Batch | Package | Functions |
|---|---|---|
| 1 | `pkg/content` | 5 |
| 2 | `cmd/localrpg` | 6 |
| 3 | `pkg/gui` route handlers | 5 |
| 4 | `pkg/gui` service methods | 12 |
| 5 | `pkg/engine` | 5 |
| 6 | `pkg/harness` | 4 |
| 7 | `pkg/provider` | 9 |
| 8 | `pkg/export` | 4 |
| 9 | `pkg/config`, `pkg/rules`, `pkg/sysgen`, `pkg/tools`, `pkg/media`, `pkg/models`, `pkg/registry`, `pkg/scene`, `pkg/storage`, `pkg/ingest`, `pkg/driver`, `pkg/entity`, `tools/sitegen` | 20 |
| 10 | the gate | a check that keeps the list empty |

Batches are independent, so their order is about the size of the diff rather than about correctness. Batch 9 is a list of small ones rather than one group, and can be split again when it is picked up.

## 5. Behaviour

None. Every extraction is a move inside one package, and the compiler plus the existing suite is the proof: a phase that was left behind, or a helper that captured the wrong variable, does not compile or does not pass. The one visible difference is `git log`, which stops blaming a hundred-line function for a one-line change.

## 6. Testing

- **The existing tests are the test.** `pkg/content` has `unpack_test.go`, `provenance_test.go`, `pack_test.go` and `determinism_test.go`; the extraction sites in this list are the largest and best-covered functions in the repository. An extraction that changes behaviour shows up there.
- **The measurement, before and after.** The parser program's count is the deliverable's number: batch 1 takes `pkg/content` from five over the line to zero.
- **A test where a phase had none**, added with the extraction rather than after it, if the phase turns out to be one the suite never reached.
- `go vet`, `mise run test`, `mise run lint`.

## 7. Rollout

One pull request per batch, each verified on its own. The bead stays open across them, and its comment records which batches have landed and which remain.

## 8. Risks

- **A mechanical extraction is where behaviour hides.** The mitigation is that each phase moves whole, that the error strings are compared rather than retyped, and that the suite covers the functions this starts with.
- **Seventy-one functions is a long road**, and the list will grow while it is walked. The gate in the last batch is what makes the end of the road mean something.
- **`pkg/gui` is seventeen of the seventy-one**, and four of those are route handlers that RD-2 has just split. They are in batch 3, after this lands.
- **Over-extraction.** A function that reads well at ninety lines stays, with the reason in the batch's pull request rather than in silence.