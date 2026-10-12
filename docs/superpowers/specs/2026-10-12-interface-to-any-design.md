# Rewrite interface{} as any Design

**Date:** 2026-10-12
**Status:** Implemented
**Issue:** [#157](https://github.com/darkliquid/LocalRPG/issues/157)
**Epic:** [#150](https://github.com/darkliquid/LocalRPG/issues/150)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (TQ-2, finding R-15)
**Scope:** Every `.go` file under `cmd/`, `pkg/` and `tools/`, tests included.

## 1. Problem

`AGENTS.md` states the convention: "prefer `any` over `interface{}`". The tree contains
636 occurrences of `interface{}`, 471 of them in non-test code across 109 files, with the
heaviest concentrations in `pkg/provider/imagehttp/client.go` (39), `pkg/gui/service.go`
(34), `pkg/engine/orchestrator.go` (27) and `pkg/harness/turn_tools.go` (19).

Because the stated convention and the code disagree, a contributor cannot tell which to
follow, so both keep being written. Every site also draws a `gopls` hint, which buries
the hints that matter.

## 2. Goals

- `interface{}` appears nowhere in the tree.
- The convention and the code agree, so the next contributor has one answer.

## 3. Non-goals

- Replacing `any` with a concrete type anywhere. That is a design change per site and
  belongs to whichever task owns that code; this one is a rename.
- Adding a linter gate. `gopls`'s `any` hint already reports it, and RD-5 introduces
  `revive` when the exported-symbol work lands; wiring a rule here would gate on a
  condition the tree only just satisfies.

## 4. Design

One mechanical rewrite with `gofmt`'s own rewriter, which parses rather than
text-substitutes:

```bash
gofmt -r 'interface{} -> any' -w cmd pkg tools
```

`any` is a type alias for `interface{}`, introduced in Go 1.18, so the rewrite is
semantically inert: every type identity, method set and interface satisfaction is
unchanged. `gofmt -r` operating on the AST is what makes this safe to do in one pass
rather than by search and replace.

## 5. Behaviour

No runtime behaviour changes at all. The only observable difference is that the tree
matches the documented convention.

## 6. Testing

The compiler is the test: `go build ./...` and `go vet ./...` pass, and the full suite
(`go test -count=1 ./...`, 63 packages) passes unchanged. A pure alias rewrite cannot
alter behaviour, so the suite is a regression guard rather than a semantic check.

One real defect was found and fixed during the rewrite, which is why the sweep is not
purely cosmetic: `pkg/gui/turn_audio.go`'s `heardState` returned a named boolean result
called `any`, which shadowed the builtin. Once the call site asked for `map[string]any`,
the shadowing broke the build. The result is now named `some`, which is also the honest
name for "at least one of these", and the call sites in `pkg/gui/service.go` and
`pkg/gui/turn_audio_test.go` were updated with it.

## 7. Rollout

Single commit, no migration. If any downstream consumer copied a signature from this
code, the alias makes the change invisible to them too.

## 8. Risks

- **A blind rewrite hits a shadowed identifier.** Demonstrated and fixed here, by the
  compiler, at the one site where it existed. This is the reason the rewrite is done with
  `gofmt -r` and not `sed`.
- **Reviewers skim a 224-file diff.** The diff is provably uniform: 927 insertions
  against 929 deletions, so two lines were net-removed by the `heardState` fix, and
  nothing else moved.
