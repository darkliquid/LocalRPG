# The CodeQL alert cluster Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Account for every alert in the CodeQL cluster as contained, by design or fixed, record each reason where the next reader meets it, and configure the scanner so the accepted classes stop being re-reported.
**Architecture:** No production behaviour changes. Inline guards where the scanner can see them, dismissals with reasons where the flow is contained or accepted, and a query exclusion in a CodeQL configuration the repository owns.
**Tech Stack:** CodeQL for Go, `gh api` for the alert lifecycle, GitHub Actions for the analysis.
**Spec:** `docs/superpowers/specs/2026-10-13-codeql-alert-cluster-design.md`

## Global Constraints

- No check is added at a sink that a `pkg/pathutil` helper already performs. The spec's table is the rule.
- Every dismissal names its reason, because a dismissal is the historical record and `won't fix` with an explanation is this repository's convention for a correct-but-invisible guard (`false positive` is rejected).
- No other query is excluded. `go/zipslip` in particular stays, because it covers the imported-package class.
- A green pull request check is not evidence. The alerts API and the analysis log are.

## File Map

| File | Why |
|---|---|
| `pkg/gui/server.go`, `pkg/gui/worlds.go`, `pkg/gui/games.go`, `pkg/gui/service.go` | the sinks the cluster names |
| `pkg/pathutil/pathutil.go` | the helpers the scanner cannot see through; unchanged, and the control |
| `pkg/content/unpack.go` | the archive extractors, where the guard is inlined at the sink |
| `.github/codeql/codeql-config.yml` | the exclusion and its reasoning |
| `.github/workflows/codeql.yml` | the analysis that carries the configuration |
| `AGENTS.md` | the guard rules and the setup, for whoever meets the next alert |

---

### Task 1: Read the cluster instead of patching it

**Files:** none

- [x] **Step 1: Get the alerts, with their state**

`gh api 'repos/darkliquid/LocalRPG/code-scanning/alerts?state=open'` returned an empty list, and `state=dismissed` returned the cluster with its reasons. That is the finding this task exists for: the alerts had been dismissed with reasoning as they appeared, and nothing was open.

- [x] **Step 2: Confirm the guard shape at each sink**

Every `go/path-injection` alert resolves to a path that came through `pathutil.ValidateID` or `pathutil.ResolveSafeChild`, or to a destination the user chose. The two `go/request-forgery` alerts resolve to the registry's configured URL.

- [x] **Step 3: Decide the rule**

The distinction in the spec's table: an identifier is validated, a user-chosen path is not contained by design, and an archive is contained inline where the scanner can see it.

---

### Task 2: Make the guard visible where the query was right

**Files:**
- Modify: `pkg/gui/server.go`, `pkg/content/unpack.go`

- [x] **Step 1: Inline the containment checks the scanner could not see**

The archive extractors were given an inline `filepath.Rel` containment check at the sink. `go/zipslip` reports that class and the alerts cleared, which is what proved the shape works: a call to a correct helper is invisible, and the same check written out is not.

- [x] **Step 2: Inline the id guard in the API surface**

The same treatment where the API surface validated an id only through the helper.

- [x] **Step 3: Verify**

The alerts at those sites cleared. `GetLocationArt` kept calling the helper, and its alert stayed open, which is the control case for the rule.

---

### Task 3: Record the accepted risks where the next reader will find them

**Files:** none (the alert records)

- [x] **Step 1: Dismiss with the reason, not with "false positive"**

Each accepted flow was dismissed as `won't fix` with the reasoning in the comment: contained by `ResolveSafeChild`, contained by `ValidateID`, or named as the feature. `false positive` is rejected by this repository, because the guard is correct and the scanner's model is what is missing.

- [x] **Step 2: Keep the comments inside the cap and check the write**

A dismissal comment is capped at **280 characters** and the API rejects a longer one outright rather than truncating it.

---

### Task 4: Configure the scanner to match the decision

**Files:**
- Create: `.github/codeql/codeql-config.yml`
- Create: `.github/workflows/codeql.yml`
- Modify: `AGENTS.md`

- [x] **Step 1: Establish what the scanner can and cannot be told**

Default setup reads a config file only through the `github-codeql-config-file` repository property, and custom properties are an organisation feature, so a repository owned by a user cannot use it. Hence the workflow: same languages, same analysis, same weekly cadence, configuration under the repository's control.

- [x] **Step 2: Measure the better fix before settling for the exclusion**

A Go model pack with `pathutil.ValidateID` as a `barrierGuardModel` and `SanitizeID` and `ResolveSafeChild` as `barrierModel`s, run with CodeQL CLI 2.27.2 against a database of this repository: **127 results to 113**, clearing exactly the helper-guarded flows and not touching the by-design ones. The CLI honours Go barrier models; code scanning's model packs cover C/C++, C#, Java/Kotlin, Python, Ruby and Rust, and not Go. A pack under `.github/codeql/extensions` would have been inert, so it was removed rather than shipped.

- [x] **Step 3: Exclude the query and say why**

`query-filters: - exclude: id: go/path-injection`, with the spec's reasoning in the file.

- [x] **Step 4: Prove it on the pull request**

The Go job's log carries `Using configuration file input from workflow: ./.github/codeql/codeql-config.yml` and the parsed `query-filters`; 27 queries were interpreted and `go/path-injection` is not one of them, while `go/xml/xpath-injection` still is; and the alerts API reports zero `go/path-injection` alerts on the pull request ref.

- [x] **Step 5: Record the guard rules for the next alert**

`AGENTS.md` keeps the two rules (a guard must be visible, and on the value the sink receives), the setup change, the model-pack finding with its measurements, and the exclusion with its revisit condition.

---

### Task 5: Verify and close

**Files:** none

- [x] **Step 1: Confirm nothing is open**

`gh api repos/darkliquid/LocalRPG/code-scanning/alerts?state=open` is empty.

- [x] **Step 2: Lint the workflow and the tree**

Run: `actionlint && mise run lint`
Expected: both silent, exit 0.

- [x] **Step 3: Check the analysis is actually running**

The workflow's three language jobs pass on the pull request, so the switch out of default setup did not leave the repository unanalysed.

- [x] **Step 4: Re-run the suite**

Run: `mise run test`
Expected: unchanged, because no production file changed.
