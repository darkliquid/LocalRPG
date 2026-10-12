# Format the Go tree and gate on it Design

**Date:** 2026-10-12
**Status:** Implemented
**Issue:** [#156](https://github.com/darkliquid/LocalRPG/issues/156)
**Epic:** [#150](https://github.com/darkliquid/LocalRPG/issues/150)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (TQ-1, finding R-14)
**Scope:** The Go sources under `cmd/`, `pkg/` and `tools/`, and the two places that
decide whether a change is acceptable (`mise.toml`, `.github/workflows/ci.yml`).

## 1. Problem

65 files fail `gofmt -l`, and 30 of them differ for reasons other than blank-line
normalisation. Nothing checks formatting: `mise run lint` runs `go vet` for Go, and CI
runs `go vet` and the test suite. Two consequences follow.

A contributor whose editor formats on save produces a diff that mixes formatting churn
with their actual change, which makes review harder and buries the real edit. And
because the tree is already unformatted, nobody can tell whether a given inconsistency
is deliberate or drift, so the state is self-perpetuating.

The blank-line differences are the tell: `gofmt` collapses runs of blank lines between
top-level declarations, and 35 files carry more than one. That is not a style anyone
chose; it is the residue of editing without a formatter.

## 2. Goals

- Every Go file in the repository is `gofmt`-clean.
- A change that is not `gofmt`-clean fails locally and in CI, with a message that names
  the files to fix.
- The gate is one task, reachable from `mise run lint`, so it runs where a contributor
  already runs everything else.

## 3. Non-goals

- `gofumpt` or any stricter superset. `gofmt` is the tool every Go editor uses by
  default, and a stricter formatter would reformat on save for fewer people.
- Rewriting the blank-line style by hand. `gofmt -w` is the definition of correct here.
- Formatting Markdown, YAML or TypeScript. Those have their own tools (rumdl, Vale, and
  the ESLint task tracked as FE-1).

## 4. Design

One sweep, then one gate.

```bash
gofmt -w cmd pkg tools          # 65 files, formatting only
mise run lint:gofmt             # fails if any file is still unformatted
```

`lint:gofmt` is added to `mise.toml` as a task whose `run` captures `gofmt -l` and exits
non-zero when the capture is non-empty, printing the list. `mise run lint` gains it as a
dependency beside `lint:docs`, `lint:goreleaser` and `lint:actions`. The CI `go` job
gains a "Check formatting" step before "Vet", so the cheapest failure is reported first.

The gate lives in `mise.toml` rather than inline in CI so that a contributor runs the
identical check locally. That is the pattern the other linters already follow.

## 5. Behaviour

- On a clean tree: silent, exit 0.
- On an unformatted file: prints `gofmt: these files are not formatted (run gofmt -w on
  them):` followed by one path per line, then exits non-zero.

## 6. Testing

The gate's failure path is exercised by creating a deliberately misformatted file in
`pkg/`, running the task, and confirming a non-zero exit that names the file, then
deleting it. Both directions were checked.

The sweep is verified by `gofmt -l cmd pkg tools` returning no output, and by the full
suite (`go test ./...`, 63 packages), `go vet ./...` and `actionlint` passing unchanged.

## 7. Rollout

Single change, no migration. Because it is formatting only, the diff cannot alter
behaviour, and the compiler plus the test suite confirm that.

## 8. Risks

- **The whitespace-only diff obscures history.** Accepted: the alternative is that every
  future diff is likewise obscured, forever. `git blame --ignore-rev` is available for
  the sweep commit if it ever matters.
- **A contributor's editor disagrees with `gofmt`.** Not a real risk; nearly every Go
  editor formats with `gofmt` itself.
- **Generated Go source gets reformatted.** There is none that is not already clean: the
  generated artefacts here are Markdown, and `pkg/scene/fonts/gen` is `gofmt`-clean.
