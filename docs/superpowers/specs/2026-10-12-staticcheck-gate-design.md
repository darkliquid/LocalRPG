# Add static analysis to lint and CI Design

**Date:** 2026-10-12
**Status:** Implemented
**Issue:** [#159](https://github.com/darkliquid/LocalRPG/issues/159)
**Epic:** [#150](https://github.com/darkliquid/LocalRPG/issues/150)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (TQ-4, finding C-1)
**Scope:** `mise.toml`, `.github/workflows/ci.yml`, and the 25 findings the first run
reported.

## 1. Problem

Neither suite has a static analyser. The `go` job runs `go vet ./...` and the test suite;
`mise run lint` adds `gofmt`, rumdl, goreleaser and actionlint. `go vet` catches a real
but narrow class of mistakes, and nothing else looks at the Go source.

The cost of that gap is not hypothetical, and this proposal's own findings are the
evidence: the first `staticcheck` run reported 25 issues, and they included a provider
that computed a model's tool calls and then threw them away, three tests comparing a
function against itself, and ten symbols that turned out to be a feature wired to
nothing. `go vet` reports none of them.

Two decisions have to be made before a check can be added: what to do about a tree that
does not yet pass, and what to do about findings that are not defects. Both are answered
below from the measurement rather than in advance.

## 2. Goals

- `staticcheck ./...` runs in `mise run lint`, so a contributor runs exactly what CI runs.
- It fails the build on any finding, with no baseline, allowance list or suppression.
- The 25 findings the first run reported are resolved rather than tolerated.

## 3. Non-goals

- **`golangci-lint`.** It orchestrates several linters behind one binary, which is
  attractive, but it is another pinned dependency and another configuration surface. If a
  specific check is wanted later it can be added directly, as `staticcheck` is here.
- **`revive`.** RD-5 introduces it for exported-symbol documentation once that work makes
  the tree pass it. Adding it now would mean gating on a condition the tree does not meet.
- **`gofumpt` or a stricter formatter.** Decided in TQ-1; `gofmt` is what every editor
  does by default.
- Suppressing any finding. A check that needs an exclusion list on day one teaches the
  team to add exemptions.

## 4. Design

One pinned tool, one task, one step, and the findings resolved.

`staticcheck 2026.2.1` joins the other pinned tools in `mise.toml` (`gitleaks`,
`actionlint`, `vale`, `rumdl`), so `mise-action` installs it in CI from the same file that
developers use locally. Then:

```toml
[tasks."lint:staticcheck"]
description = "Run staticcheck over the Go packages"
run = "staticcheck ./..."
```

added to the `lint` task's `depends`, and a "Staticcheck" step in the CI `go` job beside
"Vet". The run uses the **default** check set rather than `-checks=all`: `U1000` is the
one that found the scene worker's guardrails wired to nothing, and it is in the default
set, so the stricter set would add preferences rather than defects. Test files are
analysed, because the three tautological assertions were found in them.

**The gate lands clean, which was the open question.** The expectation was hundreds of
findings and therefore a decision about how to introduce a check without gating on a
failing tree. Measured, there were 25, so the honest answer is to fix all of them and gate
on zero. No baseline file, no exclusions.

**Ten of the findings were not defect-free cleanup, and the plan changed because of it.**
`configureSceneBudget`, the scene worker's setter cluster, `imageProviderMetered` and
`imagePrice`, the STT registry, the ComfyUI usage recorder, `TurnOrchestrator.generate`,
`ScriptCompiler.rootDir` and `speechFormattingInstruction` are all `U1000`, but only some
are obsolete. Deleting the scene cluster would have removed the only implementation of a
feature that is simply not wired up. Those became **RD-13**, which landed first, and this
proposal depends on it rather than resolving them by deletion.

## 5. Behaviour

- `mise run lint` runs `staticcheck` and fails on any finding.
- The CI `go` job fails on any finding, after the formatting check and vet, before tests.
- A contributor sees the same result locally as CI shows.

## 6. Testing

The gate's failure path is exercised by adding a `func unusedProbe() {}` to a file in
`pkg/`, running `mise run lint:staticcheck`, and confirming a non-zero exit that names the
symbol (`U1000`), then deleting it. Both directions were checked.

The findings themselves are verified by `staticcheck ./...` exiting 0 with no output,
alongside `go build`, `go vet`, `gofmt -l`, `actionlint` and the full suite (63 packages).

## 7. Rollout

One commit, no migration. The first CI run after it is the real proof, and because the
tree was fixed first that run passes.

## 8. Risks

- **A `staticcheck` upgrade adds checks that a pinned tree fails.** Real, and the same
  exposure every pinned linter in this repository carries. The version is pinned, so it
  cannot happen without a deliberate bump, and the bump is then the change that has to
  deal with it. This is the argument for pinning rather than floating.
- **The default check set is narrower than `-checks=all`**, so some style findings go
  unreported. Accepted: the review's TQ-3 already tracks the `gopls` modernisation hints,
  which cover the same ground, and a gate that fails on preferences is a gate people learn
  to route around.
- **`U1000` fires on code that is complete but not yet called**, which is exactly the
  ambiguity this proposal hit. The precedent set here is RD-13's: work out which it is,
  and if the code is wanted, wire it up rather than deleting it to quiet the check.
