# The CodeQL alert cluster Design

**Date:** 2026-10-13
**Status:** Accepted
**Issue:** [#228](https://github.com/darkliquid/LocalRPG/issues/228)
**Epic:** [#154](https://github.com/darkliquid/LocalRPG/issues/154)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (the CodeQL discussion in finding C-1, section 2.7)
**Scope:** No production behaviour. The decisions about the alerts, the dismissals that record them, and the scanner configuration that follows from them.

## 1. Problem

Code scanning reported **1 critical and 13 high alerts** that nobody had actioned: `go/request-forgery` at `pkg/registry/client.go`, and `go/path-injection` in `pkg/registry/client.go` and across `pkg/gui`. They were found, rather than fixed, because RD-1's split moved the code and the scanner re-attributed the alerts to that pull request.

They are pre-existing and they sit in the API surface, and that is what makes them a decision rather than a patch. Every one is a flow from a value that entered through a request to a filesystem or network sink, and in each case the thing standing between the two is one of:

- a **helper** that validates the value, which the scanner cannot see through: `pathutil.ValidateID` rejects separators, traversal tokens and null bytes, and `pathutil.ResolveSafeChild` verifies with `filepath.Rel` that a joined path stays in its base;
- **nothing**, because the path is the user's to choose.

Adding a check at each sink would repeat work the helpers already do correctly, and it would not make the scanner quieter, because the scanner is complaining about the helper rather than about the sink. The costs are real in both directions: the query is noise for the accepted classes, and its alerts are attached to *lines*, so any refactor that moves the code re-raises them. RD-1 and then RD-2 each paid that twice over.

## 2. Goals

- One stated rule for where a path is validated, so a new sink knows where its guard belongs.
- Every alert in the cluster accounted for, as contained, by design, or fixed, with the reason recorded on the alert itself, where the next reader meets it.
- The accepted classes stop being re-reported, so a refactor is not a CodeQL incident.
- Nothing a request can observe changes.

## 3. Non-goals

- **A validation pass at each sink.** The helpers are the control; a second check at the sink is a second place to get it wrong.
- **Modelling the sanitisers as CodeQL data extensions.** This is the right shape and it is unavailable: GitHub code scanning's model packs are in public preview for C/C++, C#, Java/Kotlin, Python, Ruby and Rust, and not for Go. Section 6 records what was measured before this was set aside.
- **Changing the threat model.** This is a local-first application with one user, served over loopback or a `0600` socket.
- **Every other query.** This is about one cluster in two rules.

## 4. Design

### Where a path is validated

A value reaches a filesystem sink only after it has entered through `pkg/gui`'s routes, and it is one of two kinds:

- **An identifier** - a campaign, world, system, entity, asset kind or similar. It is validated by `pathutil.ValidateID` at the boundary of the service method that will use it, and again by `pathutil.ResolveSafeChild` where it becomes a path component.
- **A path the user chose** - an export destination, an import folder. It is normalised by `pathutil.ValidateUserPath` (null bytes and control characters rejected) and otherwise passed through, because writing where the user asked is the feature.

That distinction is the whole rule, and it is what separates a contained flow from an accepted risk.

| The flow | What stands between the request and the sink | Verdict |
|---|---|---|
| An identifier to a filesystem sink (most of `pkg/gui`, `pkg/storage`, `pkg/worldgen`) | `pathutil.ValidateID` at the service boundary, and `ResolveSafeChild` where the identifier becomes a component | Contained. The alert is dismissed, not fixed, because the code is already correct |
| Archive extraction from an imported package | An inline containment check at the sink, which the scanner can see | Fixed inline. `go/zipslip` reports this class and stays enabled, because an imported package is the one genuinely untrusted input here |
| An export destination, an import folder, a draft directory | Nothing, by design: the user named it | Accepted risk, recorded as a design tradeoff on the alert |
| A registry URL (`go/request-forgery`) | The scheme is restricted to http and https; the host is deliberately open, because a registry on localhost is supported | Accepted risk. The hardening is named in the dismissal, and the residual depends on the user having added a hostile registry |

### What the scanner is told

`go/path-injection` is **excluded**, in `.github/codeql/codeql-config.yml`, with the reasoning beside it. Exclusion rather than forty dismissals because a dismissal is attached to a line and the alerts move; the query reports the shape of an application that reads paths from its user, and every alert it had raised had been read and found to be one of the rows above.

The exclusion is a coverage loss, and it is taken deliberately and narrowly:

- `go/zipslip` stays, and it is the rule that covers archive extraction from an imported package.
- `go/request-forgery`, `go/xml/xpath-injection` and every other rule stay.
- `pathutil.ValidateID`, `SanitizeID`, `ResolveSafeChild` and their tests remain the actual control, and they are what a reviewer should read to satisfy themselves about a path.

The configuration needs a workflow, because default setup reads a config file only through the `github-codeql-config-file` repository property and custom properties are an organisation feature. `.github/workflows/codeql.yml` runs the same three languages over the same analysis on the same weekly cadence.

## 5. Behaviour

None that a request can observe. No production file changes; the difference is which findings the scanner reports and which are recorded as decisions.

## 6. Verification

- **No open alerts on the default branch.** `gh api repos/darkliquid/LocalRPG/code-scanning/alerts?state=open` returns an empty list, which is the endpoint that matters: a green pull request check only answers "did this change add an alert".
- **The exclusion is applied, not merely committed.** On the pull request that introduced it, the Go job's log shows `Using configuration file input from workflow: ./.github/codeql/codeql-config.yml` and the parsed `query-filters`, 27 queries were interpreted with `go/path-injection` not among them, and the alerts API reported zero `go/path-injection` alerts on the pull request ref.
- **The better fix was measured rather than assumed.** A Go model pack declaring `pathutil.ValidateID` as a `barrierGuardModel` and `SanitizeID` and `ResolveSafeChild` as `barrierModel`s was built and run with CodeQL CLI 2.27.2 against a database of this repository: `go/path-injection` falls from **127 results to 113**, clearing exactly the helper-guarded flows, including the campaign and world asset sinks at `pkg/gui/server.go:442` and `:1394`. The CLI honours Go barrier models; code scanning does not yet. That is the whole reason for the exclusion, and the first thing to re-check before excluding another query.

## 7. Rollout

Retrospective, in three parts that already landed: the inline containment checks and the id guards in the `fix/codeql-*` pull requests; the alert dismissals, each carrying its reason; and the configuration, in #237 and #238, with the model-pack finding recorded in `AGENTS.md`.

## 8. Risks

- **A real path-injection defect would now go unreported.** The mitigation is that the helpers are the control and are tested against the traversal cases, and that `go/zipslip` covers the untrusted-input class. The revisit condition is a remote or multi-user surface, or Go model packs leaving preview, and both are written into the config file where the next person will be looking.
- **The exclusion could be read as "path handling is unchecked here".** Section 4 of this document and the config's comment exist so that it cannot be: the rule is stated, and the accepted risks are named individually.
- **The cluster may not be finished.** The registry's request-forgery site is an accepted risk rather than a fix, and that acceptance should be revisited if the registry client ever follows a user-supplied host that is not the registry the user configured.
