# The Go and TypeScript Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A JSON tag, a property name, an added field or a changed `omitempty` fails a test that names the type and the field, on either side of the contract.
**Architecture:** A Go test generates `pkg/gui/testdata/dto.json` from the package's own source; a Vitest test parses `frontend/src/types.ts` with the TypeScript compiler, resolves each DTO's mirror as the interface whose property set is exactly its fields, and compares names and optionality.
**Tech Stack:** Go (go/parser, go/ast, reflect), TypeScript 5.9, Vitest, the TypeScript compiler API.
**Spec:** `docs/superpowers/specs/2026-10-16-dto-contract-design.md`
**Bead:** LocalRPG-14a.5 (FE-5)
**Branch:** `feat/fe-5-contract`

## File Map

| File | Why |
|---|---|
| `pkg/gui/dto_manifest_test.go` | new: generate and check the manifest |
| `pkg/gui/testdata/dto.json` | new: the generated manifest, the artefact the TypeScript side reads |
| `frontend/src/test/dtoContract.test.ts` | new: the comparison and the two exception lists |
| `pkg/gui/types.go` | `omitempty` on the request fields the client may omit |
| `frontend/src/types.ts` | the fields the client cannot see, and the optionality it gets wrong |
| `AGENTS.md` | the rule, its generator, and where the exceptions live |

## Global Constraints

- **The manifest is generated, never edited.** `go test ./pkg/gui -update-dto` rewrites it; the failure message says so in the same words as the route manifest.
- **No hand-written pairing table.** A mirror is the interface whose property set is exactly the DTO's fields; the pairing is proven, not declared.
- **An exception carries a reason**, and the test fails if the type or field it names is gone.
- **No wire field is renamed or removed.** The only Go change is `omitempty` where the client may genuinely omit a field.
- **Both sides run in the default suites**: `go test ./pkg/gui` and `npx vitest run`.

## Task 1: The Go manifest

- [x] `pkg/gui/dto_manifest_test.go`: walk `pkg/gui`'s own source with `go/parser`, collect every exported struct with at least one JSON tag, and emit `{json, go, optional}` per field.
- [x] `optional` is true when the tag carries `omitempty` or the field is a pointer.
- [x] Write `pkg/gui/testdata/dto.json` under `-update-dto`, with a trailing newline, and fail on a stale manifest naming the flag.
- [x] A test that the walk finds `GameStateDTO` and `PlayerDTO.state`, so a broken parser fails rather than writing an empty manifest.
- [x] A test that a second run over unchanged source produces an identical manifest.

## Task 2: The TypeScript comparison

- [x] `frontend/src/test/dtoContract.test.ts`: read `../../../pkg/gui/testdata/dto.json`, parse `frontend/src/types.ts` with the TypeScript compiler into `{name -> properties}`.
- [x] For each manifest entry not in `NOT_MIRRORED`, resolve the mirror as the interface whose property set is exactly the JSON names, and fail when there is none.
- [x] Fail on a JSON name the interface does not declare; on a property with no JSON name unless it is in `TS_ONLY`; on TypeScript required where Go is optional; on TypeScript optional where Go is required unless it is in `WIRE_REQUIRED`.
- [x] Both exception maps carry a reason per entry, and a test fails on an entry naming a type or field absent from the manifest.
- [x] A test that a seeded difference is reported, so the comparator is exercised against a fixture and not only against a green file.

## Task 3: Resolve what the check reports

- [x] The thirteen optionality disagreements: `omitempty` in Go where the client may omit the field (the request DTOs), otherwise the `?` in TypeScript goes.
- [x] The seven Go fields absent from TypeScript are declared.
- [x] The four TypeScript-only properties: three are decided, one (`Turn.negotiations`) is recorded in `TS_ONLY` with its reason.
- [x] The eighteen DTOs with no counterpart are listed in `NOT_MIRRORED` with a reason each.
- [x] Re-run until the comparator is green, and record the count of each kind of difference in the bead.

## Task 4: Documentation and gates

- [x] `AGENTS.md`: the manifest, its flag, the two exception lists, and why the TypeScript file is not generated.
- [x] `gofmt`, `go vet ./...`, `npx tsc --noEmit`, `npx prettier --write`, `npx eslint src` (0 errors).
- [x] `mise run test` and `mise run lint`.
- [x] Open the pull request, wait for CI, close the bead with the evidence.
