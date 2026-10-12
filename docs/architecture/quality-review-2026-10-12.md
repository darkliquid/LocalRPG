# LocalRPG Code Quality Review

- **Date:** 2026-10-12
- **Baseline:** commit `78898e3e` (branch `main`, clean tree)
- **Status:** Delivered. This document is the input to a task-synthesis pass; it is
  not itself a plan of record.
- **Companion:** supersedes nothing, and complements
  [`review-2026-09-24.md`](review-2026-09-24.md), which analyses system shape and
  package coupling. This review covers readability, test coverage, documentation,
  dependencies, and CI/CD tooling.
- **Method:** direct reads of the source, plus measured runs of the project's own
  toolchain (`go test -cover`, `vitest --coverage`, `gofmt -l`, `vale`,
  `npm audit`, `go list -m -u`, `npm outdated`). Every number below was produced in
  this session and is reproducible with the commands in
  [Appendix A](#appendix-a-metric-baseline).

---

## Progress

The baseline above is a snapshot and is left as one: the numbers in the body and in
[Appendix A](#appendix-a-metric-baseline) describe commit `78898e3e` and are not
rewritten as work lands. What follows records the movement, re-measured at each merge.

**Wave 6 so far.** Ten of the 57 tasks in section 4 are done, plus RD-13, which the review
itself did not anticipate; all at commit `e3ca988b`:

| Task | Closed finding | What changed |
|---|---|---|
| TQ-1 | R-14, and the gofmt half of C-1 | 65 files formatted; `lint:gofmt` gates `mise run lint` and CI |
| TQ-2 | R-15 | 636 uses of `interface{}` rewritten to `any`; surfaced and fixed a boolean named `any` that shadowed the builtin |
| TQ-4 | the rest of C-1 | `staticcheck` pinned and gated; 25 findings fixed, and one of them, a provider that dropped a model's tool calls, was a real bug |
| TQ-5 | C-4, C-5 | `project.sh` no longer clears a single-select field when a wave or area is added, and reads an option name containing a comma correctly |
| TQ-11 | S-4 | `frontend/coverage/` ignored |
| RD-13 | R-10, and ten `U1000` findings | The scene worker's budget, approval, price, fallback and skip callback were configured by a function nothing called, so a campaign's image allowance was never enforced |
| TC-1 | T-1, T-3 | Coverage floors on both suites: Go 72.0%, frontend 33/34/33/30 |
| FE-1 | F-1 | ESLint and Prettier, the tree formatted, and the six hook-dependency findings they immediately exposed |
| DOC-9 | D-9 | The stale counts in `AGENTS.md` corrected |
| DOC-5 | D-5 | Six articles shared two `order` values, so the sidebar sorted arbitrarily |
| DOC-4 | D-6 | A generated `00-start-here.md` index, which is what the viewer opens, and a pointer to the manual from the README |

**Re-measured after wave 6.**

| Metric | Baseline | Now |
|---|---|---|
| Go files failing `gofmt` | 65 | 0 |
| `interface{}` in Go (non-test) | 471 | 0 |
| `mise run lint` code gates | `go vet` | `go vet`, `gofmt -l`, `staticcheck`, ESLint, Prettier |
| Go statement coverage | 73.3% | 73.3% |
| Go functions at 0% coverage | 422 | 422 |
| Guide articles | 24 | 25 |
| Vale alerts / files | 290 / 26 | 314 / 27 |
| Stale figures in `AGENTS.md` (D-9) | 4 | 0 |

Two corrections were made to this review from evidence found while acting on it, both
of which narrowed a claim: **CodeQL is running** (C-1), and the `interface{}` count in
Appendix A was 636 across the whole tree rather than 471 outside tests alone. Two more
were forced by the work itself: F-1's "five dead suppressions" were four working
suppressions and one dead one, and TQ-3's premise, that its hint list would come from the
gate, does not hold, because the gate is `staticcheck` and it reports none of those hints.
The lesson is recorded rather than quietly fixed, because a review that overstates is
worth less than one that is wrong in a way you can check.

---

## 1. Executive summary

The project is well above average for a codebase of its size in the areas that are
hardest to retrofit: it has almost no TODO debt, no commented-out code, no lint
suppressions hiding problems, a genuine end-to-end browser harness, fuzzing in CI,
secret scanning over full history, and a documentation set that is generated where
generation is possible. The engineering discipline is real.

The weaknesses cluster in four places:

1. **A handful of files have grown far past the size where a human can hold them in
   their head.** `pkg/gui/service.go` is 6,579 lines with 165 exported methods on one
   struct; `handleGameRoutes` is a single 676-line function; `ProcessActionStream` is
   980 lines. These are the first things a new contributor will read, and they are
   the least readable things in the repository.
2. **The frontend is the least-tested and least-tooled part of the system.** Statement
   coverage is 33%, no ESLint or Prettier is configured, 56 of 95 components have no
   test, and only 3 of roughly 22 modal surfaces declare `role="dialog"`.
3. **Neither the Go nor the frontend test suite is gated on coverage, and neither has
   a static analyser beyond `go vet` and `tsc`.** 422 Go functions sit at 0% coverage,
   including every CLI entry point.
4. **The Go/TypeScript contract is maintained by hand in two places** (`pkg/gui/types.go`
   and `frontend/src/types.ts`) with no codegen and no drift check, while a route
   manifest already proves the team knows how to guard this class of problem.

### Scorecard

| Area | Rating | Headline |
|---|---|---|
| Go readability and structure | **Fair** | Excellent conventions; three god-files and a 165-method service struct |
| Frontend readability and structure | **Fair** | Two idioms, no state layer, no shared modal primitive, no linter |
| Go test coverage | **Good** | 73.3% statements, 0.76 test:code ratio, 2,012 tests |
| Frontend test coverage | **Poor** | 33.4% statements, 39 of 95 components covered |
| UI/UX flow coverage | **Poor** | 10 e2e tests for the whole application |
| Documentation | **Good** | 24 strong articles; no CLI reference, no GUI guide, no contributor on-ramp |
| Dependencies | **Fair** | 21 of 38 direct Go deps behind; four frontend majors behind |
| CI/CD | **Good** | Strong job structure and SAST via CodeQL; no Go linter, no coverage gate, ubuntu-only |
| Housekeeping | **Fair** | 65 files fail `gofmt`; `AGENTS.md` counts are stale; no community files |

---

## 2. Findings

Severity: **High** means it materially impedes comprehension, correctness confidence,
or contribution. **Medium** means it is a real cost that compounds. **Low** is polish.

### 2.1 Go readability and structure

#### R-1. `pkg/gui/service.go` is 6,579 lines and `Service` has 165 exported methods

**Severity: High.**
Evidence: `wc -l pkg/gui/service.go` → 6579; `rg -c '^func \(s \*Service\) [A-Z]' pkg/gui/*.go`
→ 165; the struct at `pkg/gui/service.go:52` carries roughly 55 fields.

`pkg/gui` holds 45 non-test files, and it is not one concern; it is at least nine:
HTTP transport (`server.go`, `routes.go`, `middleware.go`, `socket.go`, `assets.go`),
wire types (`types.go`), games/systems/worlds/entities CRUD, AI generation
(`sysgen*.go`, `worldgen*.go`, `draft_review.go`, `character_generate.go`), audio and
speech (`streaming_tts.go`, `turn_audio.go`, `playback_session.go`), usage and cost
(`usage.go`, `limits.go`), export (`export.go`), docs and schema (`docs.go`,
`schema.go`), and the content registry.

Impact: a newcomer looking for "where a turn is handled" must scan a 6,579-line file
of unrelated methods first. Every change to any feature touches the same file, so
merge conflicts are structural rather than incidental.

Recommendation: split by domain into `service.go` (struct plus lifecycle only),
`games.go`, `worlds.go`, `systems.go`, `entities.go`, `audio_service.go`,
`assets_service.go`, and `registry_service.go`. This is a mechanical move and can be
done one file at a time without changing a single signature.

#### R-2. `handleGameRoutes` is one 676-line function that hand-parses the URL eight times

**Severity: High.**
Evidence: `pkg/gui/server.go:371`; the function re-parses turn and segment numbers
with `strconv.Atoi` in at least eight separate blocks, each repeating the same
`MaxBytesReader` plus `Decode` idiom (e.g. `server.go:711`, `:745`, `:802`, `:848`).

Impact: the routing table in `routes.go:20` names this handler, so the reader is sent
to a function whose length is close to the size of the whole `pkg/gui` route surface.

Recommendation: adopt Go 1.22+ `http.ServeMux` patterns (`{id}`, `{n}`) for the
sub-routes, and extract a `parseTurnNumber` helper plus a `decodeJSONBody` helper.
That removes most of the duplicated blocks and drops the function to a dispatch.

#### R-3. `ProcessActionStream` is a 980-line function spanning every abstraction in the system

**Severity: High.**
Evidence: `pkg/engine/orchestrator.go:648`; `runGenerationLoop` at `:1872` is a further
415 lines.

In one body it handles `/undo`, `/recap` and `/go`; branches on Opening/GM/Roll mode;
assembles context; runs world ticks and JS hooks; wraps the chunk callback twice in
closures; drives a roll-continuation loop; extracts entities; builds segments,
portraits and scene prompts; applies advancement; and records the turn. It mixes
string prefix matching, OpenTelemetry span attributes, and entity slugging at the same
indentation level.

Recommendation: extract the slash-command handling, the roll-continuation loop, and
the post-generation assembly into named methods on `TurnOrchestrator`. `Timeline` was
factored out along the "single writer" axis already; do the same along "phases of a
turn".

#### R-4. `pkg/engine` and `pkg/storage` are named for what they are not

**Severity: Medium.**
Evidence: `pkg/storage` contains only the SQLite **index**, which is derived and
disposable, while the durable writes (`history.jsonl`, entity notes) live in
`pkg/engine/timeline.go:26`. `pkg/engine` holds turn orchestration
(`orchestrator.go`), durable persistence (`timeline.go`), and campaign bootstrap
(`game.go:45`), with nothing in the names to say which is which.

Impact: a newcomer looking for "where notes are saved" goes to `pkg/storage` and finds
a cache. This is the single most likely first-day misdirection in the repository.

Recommendation: at minimum add package doc comments that state the distinction
(see R-5). A rename (`pkg/index` for the SQLite layer) would be better but is
disruptive; the doc comment is the cheap 80%.

#### R-5. Only 2 of roughly 30 packages have a package doc comment

**Severity: Medium.**
Evidence: `rg '^// Package '` matches `pkg/turnstream/parser.go` and
`pkg/jsonrepair/scan.go` only. No package-level documentation exists for `engine`,
`harness`, `storage`, `entity`, `state`, `rules`, `media`, `scene`, `config`, `gui`,
`export`, or `ingest`.

Recommendation: a one-paragraph `// Package x ...` comment above `package x` in each of
the 12 core packages, saying what the package is responsible for and what it
explicitly is not. This is the highest readability return per line of effort in the
whole review.

#### R-6. Exported symbols are widely undocumented in the lower layers

**Severity: Medium.**
Evidence: `pkg/state/state.go` has no doc comment on any exported symbol (`State`:9,
`NewState`:14, `Get`:21, `Set`:42, `Raw`:73), so nothing states that paths are
dot-separated, that `Set` auto-creates intermediate maps, or that `Raw` returns a
shallow copy. Same pattern in `pkg/entity/entity.go` (`EntityFrontmatter`:28,
`Entity`:52, `ParseMarkdownEntity`:89, `SerializeMarkdown`:339), `pkg/storage/store.go`
(`NewStore`:22, `SaveEntity`:39, `GetEntity`:102), `pkg/rules` (`JSEngine`:16,
`LoadScript`:187, `ExecuteAction`:198), and `pkg/engine/game.go` (`Session`:16,
`InitGame`:45).

Contrast: `pkg/gui/types.go` is well commented (93 doc lines), which shows the standard
is attainable in this codebase.

Recommendation: enable `revive`'s `exported` rule in report-only mode first, then work
the core packages down to zero. Do not gate CI until the count is near zero.

#### R-7. Errors are discarded in the write path, where losing them is invisible

**Severity: High.**
Evidence: 249 `_ =` discards and 116 `, _ :=` results in non-test Go code. The
concerning ones are load-bearing:

- `pkg/storage/store.go:113`: `if err := json.Unmarshal([]byte(fmJSON), &meta); err == nil {`
  means corrupt frontmatter silently yields an entity with no state, voice or aliases.
- `pkg/engine/timeline.go:292`: `_ = os.Remove(prevPath)` and
  `_ = t.store.DeleteEntity(prevEnt.ID)`, so a failed de-merge is invisible.
- `pkg/engine/orchestrator.go:1558`: `_ = o.timeline.SaveEntity(ent)`, so an assigned
  voice can be silently lost; `:2214`: `encoded, _ := json.Marshal(resolved)` turns a
  marshal failure into an empty tool message that is then sent to the model.
- `pkg/storage/sync.go:105` uses `existingErr == nil` as a proxy for "already indexed",
  making a real database error indistinguishable from a new entity.

Many other discards are benign (closing files, shutting down telemetry), but they are
indistinguishable from the harmful ones without reading each site.

Recommendation: introduce `errcheck` and triage. Fix R-7's cited sites unconditionally;
for the benign sites, prefer an explicit `_ =` immediately preceded by a comment
explaining why the error is ignorable, so the pattern becomes self-documenting.

#### R-8. Twelve `panic` calls inside the JavaScript host bindings

**Severity: Medium.**
Evidence: `pkg/rules/js_engine.go:51, 61, 71, 79, 87, 114, 123, 132, 141, 150, 160, 169`.
These are the goja idiom for throwing a JS exception, which is correct, but nothing in
the file says so.

Recommendation: one doc comment at the top of `bindHostAPI` (or a named helper such as
`throwJS(vm, msg)`) explaining that a panic is the mechanism by which goja surfaces a
JS-level throw. That converts twelve alarming lines into one explained pattern.

#### R-9. Stringly-typed role and mode literals where constants already exist

**Severity: Medium.**
Evidence: the literal `"gm"` appears about 18 times in `pkg/engine/orchestrator.go`
(including `:884`, `:1131`, `:1840`, `:2079`), even though `config.RoleGM` exists and is
used at `pkg/harness/factory.go:166`. Engagement modes `"player"`, `"auto"`, `"ask"`
and `"off"` are likewise bare literals. Other magic values: `"2d6"` is a default in
both `orchestrator.go:607` and `resolver.go:37`; `1 << 30` means "no limit" in
`harness/context.go:583` and `:706`; `runes / 4` at `orchestrator.go:2314` duplicates
the named `runesPerToken = 4` at `context.go:56`.

Recommendation: replace with the existing constants, and name the remainder.

#### R-10. Dead code and unreachable branches

**Severity: Medium.**
Evidence:

- `pkg/harness/context.go:1006`: `speechFormattingInstruction` is an unused constant
  whose own comment says it is "kept for backwards compatibility". In a 0.x project
  with no compatibility contract (see the 0.x zero-debt rule), both the constant and
  the comment are debt. Its replacement, `FormatSpeechFormattingInstructions`
  (`context.go:1029`), carries near-identical text.
- `pkg/state/state.go:47`: `if len(parts) == 0 { return fmt.Errorf("empty path") }` is
  unreachable, because `strings.Split("", ".")` returns a one-element slice. `Set("")`
  therefore silently writes an empty key instead of erroring.
- `pkg/gui/routes.go:13`: the `routeMount.Handler` string field is read only by
  `routes_test.go:63`; it duplicates the `serve` function and can drift from it.
- `pkg/harness/context_types.go:11-14`: `RefWorld`, `RefSystem`, `RefLocation` and
  `RefSession` have no references anywhere.
- `pkg/harness/factory.go:95`: `BuildModel` is a pure alias of `BuildModelFor`, called
  only from a test.
- `pkg/harness/context.go:646`: `TurnRecordView` is exported but used only inside
  `relevantHistory`.

Also found in the same area: `appendUnique` (`timeline.go:839`) and
`appendUniqueInts` (`:858`) are the same algorithm twice, and `containsMention` (`:412`)
reimplements the membership test `appendUnique` already encodes.

#### R-11. `EntityFrontmatter` and `Entity` are hand-mirrored in four places

**Severity: Medium.**
Evidence: the same ~18 fields are enumerated independently in `pkg/entity/entity.go:145`
(parse), `:340` (serialize), `pkg/storage/store.go:46` (index write) and `:114` (index
read). Adding one field requires four coordinated edits.

Recommendation: derive the index round-trip from a single struct via an explicit field
map, or at minimum add a test that fails when the four lists diverge.

#### R-12. Long functions elsewhere worth targeting after R-1 to R-3

**Severity: Medium.**
Evidence, worst first: `ImportContentWithOptions` 205 lines (`pkg/gui/service.go`),
`handleGUICommand` 198 lines (`cmd/localrpg/gui.go:29`), `writeFrames` 197 lines
(`pkg/export/video.go`), `TestProvider` 190 lines (`pkg/gui/service.go`), `InitGame`
160 lines (`pkg/engine/game.go:45`), `stageEntities` 172 lines
(`pkg/engine/timeline.go:203`), `Unpack` 221 lines (`pkg/content/unpack.go`),
`GenerateImage` 184 lines (`pkg/provider/imagehttp/client.go`), `streamOnce` 174 lines
(`pkg/provider/openaichat/http.go`).

#### R-13. Inconsistent command shapes make the CLI half-testable and half-not

**Severity: Medium.**
Evidence: `registry.go`, `styles.go`, `content.go`, `provenance.go` and `offline.go`
use a testable `handleXCommand(args) -> runXCommand(args, stdout, stderr) int` shape.
`roll.go:10`, `prompt.go:14`, `play.go:27`, `media.go`, `export.go`, `gui.go:54` and
`debug.go:62` call `os.Exit` directly. Flag handling diverges too (`prompt.go:15` uses
`flag.ExitOnError`, `gui.go:29` uses `ContinueOnError`), and `play.go:34` discards a
config load error (`cfg, _ := cfgMgr.Load()`) that `gui.go:69` handles properly.

This finding is also the cause of T-4 below.

#### R-14. Dozens of files fail `gofmt`

**Severity: Medium.**
Evidence: `gofmt -l cmd pkg tools` lists **65 files**, of which **30 have real
differences** (not merely blank-line normalisation), including `pkg/gui/service.go`,
`pkg/engine/roster.go`, `pkg/storage/store.go`, `pkg/entity/entity.go`,
`pkg/media/tts.go` and `pkg/models/manager.go`. `mise run lint` and CI both run
`go vet` but never `gofmt -l`.

Impact: a contributor running gofmt-on-save produces a diff unrelated to their change,
and `go vet` will never catch it.

Recommendation: run `gofmt -w` once across `cmd pkg tools`, then add `gofmt -l` (or
`gofumpt -l`) as a CI gate. If a blank-line style is genuinely preferred, express it in
a committed `.gofumpt`-style config rather than by leaving the tree unformatted.

#### R-15. 471 uses of `interface{}` contradict the project's own stated convention

**Severity: Medium.**

Evidence: `AGENTS.md` states "Use modern Go idioms: prefer `any` over `interface{}`".
Measured: **471** occurrences of `interface{}` in non-test Go across **109 files** (plus
165 in tests). The heaviest files are `pkg/provider/imagehttp/client.go` (39),
`pkg/gui/service.go` (34), `pkg/engine/orchestrator.go` (27),
`pkg/harness/turn_tools.go` (19), `pkg/harness/tools.go` (14),
`pkg/engine/advancement.go` (12) and `pkg/state/state.go` (11).

Impact: the stated convention and the code disagree, so a contributor cannot tell which
to follow, and `gopls` emits an "any" hint on every site. Since `interface{}` and `any`
are identical types, the change is a pure rename and is safe under `gofmt -r`.

Recommendation: mechanical rewrite (`gofmt -r 'interface{} -> any' -w` over the tree),
then add `gopls`' `any` analyser or `revive`'s `use-any` rule to keep it clean. Truthful
either way: if `interface{}` is actually preferred, change `AGENTS.md` instead, but do
not leave both.

#### R-16. 322 modernisation hints, not the two this review first reported

**Severity: Low, with three exceptions that are not style at all.**

The first draft of this finding named two sites, because the editor reports `gopls`
diagnostics with a truncated tail ("and 247 more") and that tail was taken at face value.
Enumerated properly with
`gopls check -severity=hint $(find cmd pkg tools -name '*.go' ! -name '*_test.go')`, the
tree carries **322 hints**, and the two named were never the interesting part:

| Hint | Count |
|---|---|
| Inefficient string concatenation in call to `WriteString` | 90 |
| `for` loop can be modernised using range over int | 39 |
| `omitempty` has no effect on nested struct fields | 25 |
| `fmt.Fprintf` instead of `WriteString(fmt.Sprintf(...))` | 16 |
| Ranging over `SplitSeq` is more efficient | 15 |
| Backward loop over slice can be modernised using `slices.Backward` | 13 |
| Replace `m[k]=v` loop with `maps.Copy` | 12 |
| `unused parameter` | 8 |
| `intPtr(x)` can be simplified to `new(x)` | 8 |
| `if` statement can be modernised using `min` / `max` | 6 / 4 |
| `HasPrefix`+`TrimPrefix` can be simplified to `CutPrefix` (and `CutSuffix`) | 6 / 4 |
| `strings.Index` can be simplified using `strings.Cut` | 5 |
| unused function / method / const | 5 / 4 / 2 |
| `errors.As` can be simplified using `AsType[...]` | 4 |
| `could use tagged switch` | 5 |
| `string += string` in a loop | 2 |
| **`sql.Rows` used in a `Next` loop without a final `Err()` check** | **3** |
| Deprecated API in use (`kv.Value.Emit`, `tar.TypeRegA`, `oteltrace.NewNoopTracerProvider`) | **3** |

The last two rows are defects rather than preferences, and the third row is the one that
matters: three `Rows.Next()` loops never check `rows.Err()`, which is the same omission
R-7 and RD-6 describe, and it means a query that fails halfway is reported as a
completed one. Those belong to RD-6 and are cross-referenced there rather than counted
twice here.

The scale changes what TQ-3 should be. Ninety `WriteString` concatenations and thirty-nine
range-over-int loops are a real sweep, not a tidy-up, and `staticcheck` (TQ-4) will
overlap with most of the list. TQ-4 therefore goes first, and TQ-3 becomes "work the
findings the gate reports" rather than a hand-enumerated one.

`AGENTS.md` already states the intended idioms (`slices`/`maps` helpers, `min`/`max`,
range-over-int), so the rest is drift rather than disagreement.

### 2.2 Frontend readability and structure

#### F-1. No linter and no formatter for TypeScript at all

**Severity: High.**
Evidence: `frontend/package.json` has no `eslint`, no `prettier`, no `lint` script.
`mise run lint` (`mise.toml:86`) runs `lint:docs`, `lint:goreleaser`, `lint:actions`
and `go vet`; it never touches `frontend/`. The only gate is `tsc --noEmit`, which
checks types and unused locals and nothing else.

Two consequences visible in the tree: `frontend/src/components/RegistryModal.tsx:86`,
`EntityTree.tsx:162`, `EntityTree.tsx:168`, `editor/MarkdownEditor.tsx:180` and `:211`
carry `// eslint-disable-next-line react-hooks/exhaustive-deps` comments for a linter
that is not installed, so five real exhaustive-deps warnings are being suppressed
against nothing; and there is no enforcement of formatting, hook rules, or a11y rules.

Recommendation: add ESLint (flat config) with `typescript-eslint`, `react-hooks` and
`jsx-a11y`, plus Prettier, and a `lint` script wired into `mise run lint` and the CI
`web` job. Then remove the five dead suppressions or replace them with a note
explaining why the dependency array is intentionally incomplete.

#### F-2. No state layer; `App.tsx` holds about 45 `useState` hooks

**Severity: High.**
Evidence: `frontend/src/App.tsx:45-116` and `:404-410` declare roughly 45 `useState`,
3 `useRef` and 6 polling `useEffect`s, owning turn streaming, audio, limits, and every
drawer's open flag. `createContext`/`useContext`/`useReducer` do not appear anywhere in
`src/` (0 occurrences). `SystemsStudio.tsx:35-85` adds 33 more hooks and
`WorldsStudio.tsx:36-96` another 30.

Prop drilling is the visible cost: `App.tsx:1074` passes **30 props** to
`ChronicleView`, `:1280` passes 20 to `StoryTheater`, and
`providers/TTSProviderEditor.tsx:17` accepts about 30, including seven derived booleans
(`isBuiltinKokoro`, `isKokoro`, `isGeminiTTS`, `isElevenLabsTTS`, `isCartesiaTTS`,
`geminiSharedKey`, `cartesiaSharedKey`) that the parent computes.

`characterPortraits` is derived twice by two near-identical effects
(`App.tsx:284-311` and `:313-337`), and config is held twice: `App.tsx:160` fetches
settings while `SettingsStudio` fetches its own copy and `App.tsx:1266` re-fetches on
save.

Recommendation: introduce a small number of context providers (campaign, audio,
settings) with `useReducer`, and split `App.tsx` into a shell plus feature views. Extract
the duplicated portrait effect. Aim for a prop count under 8 per component.

#### F-3. Three god-files hold most of the UI

**Severity: High.**
Evidence: `src/components/WorldsStudio.tsx` 1,552 lines, `src/components/SystemsStudio.tsx`
1,540 lines, `src/App.tsx` 1,407 lines. The JSX returns alone run to about 1,000 lines in
each studio (`SystemsStudio.tsx:532-1540`).

#### F-4. The Go/TypeScript contract is hand-maintained with no codegen and no drift check

**Severity: High.**
Evidence: `pkg/gui/types.go` is 1,279 lines with 114 exported `*DTO` structs;
`frontend/src/types.ts` is 1,846 lines. Nothing generates one from the other
(`go.mod` and `package.json` contain no `tygo`, `oapi-codegen`, OpenAPI generator or
zod). Concrete drift surfaces: `GameStateDTO` (`types.go:111`) versus `GameState`
(`types.ts:36`); `TurnDTO` (`types.go:204`) versus `Turn` (`types.ts:161`);
`SegmentDTO` (`types.go:137`) versus `TurnSegment` (`types.ts:62`); `PendingCheckDTO`
(`types.go:251`) versus `PendingCheck` (`types.ts:123`); `TTSBatchJobDTO` (`types.go:165`)
versus `TTSBatchJob` (`types.ts:96`).

Four distinct failure modes are unguarded: JSON tag versus property-name drift produces
a silent `undefined`; optionality drift (Go `omitempty` versus TS `?`) is independent on
each side; enum drift is real, since `SegmentDTO.Kind` is a bare `string`
(`types.go:138`) while `TurnSegment.kind` is a union (`types.ts:63`); and a field rename
type-checks on both sides, as with `SpeakerPortrait`/`HasCustomPortrait` (`types.go:149`)
versus `speaker_portrait`/`has_custom_portrait` (`types.ts:77`).

The team has already solved this exact problem for routes: `pkg/gui/testdata/routes.json`
is a checked-in manifest and `routes_test.go:42` fails with "route manifest is stale;
regenerate with `go test ./pkg/gui -update-routes`". The DTO layer needs the same
treatment: generate `frontend/src/types.ts` from the Go DTOs (tygo is a good fit), or
add a test that reflects over the DTO structs and compares JSON tags against a
checked-in manifest that the TypeScript side imports.

#### F-5. About 140 client endpoints return unvalidated JSON behind a type assertion

**Severity: Medium.**
Evidence: `frontend/src/api/client.ts` (1,563 lines) ends every call at `res.json()`
typed only by assertion, for example `client.ts:200`, `:304`, `:1265`. There is no
runtime schema (no zod, no io-ts), so a backend field rename surfaces as a runtime
`undefined` rather than a type error.

Related inconsistencies in the same file: `getGameState` throws `HTTPError`
(`:1264`) while `getChronicle`, `getRecap` and `getGraph` throw plain `Error`
(`:1270`, `:1276`, `:1300`), so callers cannot distinguish a 404 from a transient
failure; and static and instance variants coexist for `updateGameSettings` (`:574`
versus `:593`), `advanceUnlock` (`:578` versus `:582`), `getTurnContext` (`:1394`
versus `:1406`), `getWorkingSet` and `getGameUsage`.

Recommendation: pick one error type and one calling convention, and validate responses
at the boundary for the handful of endpoints where drift would be silent.

#### F-6. Type-safety escape hatches are concentrated and countable

**Severity: Medium.**
Evidence: 20 `any` occurrences (including two public DTO fields,
`Record<string, any>` at `src/types.ts:5` and `:596`), 10 `catch (err: any)` sites
(`SettingsStudio.tsx:148, 172, 214`, `RegistryModal.tsx:42, 139`, `WorldGallery.tsx:157`,
`LauncherHub.tsx:113`, `DebugPanel.tsx:37, 61`, `ContextDrawer.tsx:76`), 7 `as any`
casts (`ImageProviderEditor.tsx:102`, `AgentsPanel.tsx:221`, `TTSProviderEditor.tsx:255,
641`, `STTProviderEditor.tsx:83`, `LLMRoleEditor.tsx:69`), and 91 `as Type` assertions.
The riskiest is the double cast `preset.config as unknown as T`
(`SettingsStudio.tsx:36`).

Recommendation: `catch (err: unknown)` everywhere with a shared `errorMessage(err)`
helper (which is currently duplicated: `WorldsStudio.tsx:99` and `SystemsStudio.tsx:93`),
and remove the `as unknown as` chain.

#### F-7. No shared modal or overlay primitive, so chrome is copy-pasted

**Severity: Medium.**
Evidence: the overlay shell class `fixed inset-0 z-50 flex items-center justify-center
p-4 bg-black/70 backdrop-blur-sm` appears **31 times** across `src/`. The animation
style `style={{ '--anim-dur': '200ms' } as React.CSSProperties}` appears in 17 files.
`Drawers.tsx` is the only extracted overlay component, which shows the pattern works.

Recommendation: extract a `<Modal>` (and reuse `Drawers`) and migrate. This is a
prerequisite for burning down F-8 and U-1 to U-3, because the a11y fixes belong in the
primitive rather than in 22 copies.

#### F-8. Accessibility is largely unaddressed and untested

**Severity: High.**
Evidence:

- `role="dialog"` plus `aria-modal` appears on only 3 surfaces: `DocsModal.tsx:153`,
  `AboutModal.tsx:37`, `StoryTheater.tsx:363`. The other roughly 19 overlays are bare
  `div`s, for example `SystemsStudio.tsx:1404`, `:1466`, `:1520`,
  `WorldsStudio.tsx:1490`, `EntityBatchDialog.tsx:174`, `WorldGenerateDialog.tsx:228`,
  `WorldDraftReview.tsx:115`, `ExportModal.tsx:142`, `NewEntityWizard.tsx:141`,
  `AddEntityModal.tsx:28`, `TreeDialogs.tsx:40`, `CodexDrawer.tsx:539`, `App.tsx:1247`.
- **No focus management anywhere.** There is no focus trap and no `inert` usage; the
  only `.focus()` calls are `App.tsx:658` and `:799` (the action console). No modal
  moves focus on open or restores it on close; `autoFocus` appears twice
  (`TreeDialogs.tsx:119`, `:174`).
- Escape handling is ad hoc, present in 8 places (`DocsModal.tsx:104`,
  `ImageLightbox.tsx:21`, `RowMenu.tsx:44`, `MenuBar.tsx:89`, `CampaignGallery.tsx:158`,
  `VoiceProfileSelect.tsx:133`, `VoiceCombobox.tsx:152`, `VoiceCatalogModal.tsx:44`)
  and absent from the delete, confirm and generate modals.
- Progress is exposed to assistive technology in exactly one place
  (`ImportProgressPanel.tsx:55`). Plain `<div style={{width}}>` bars exist in
  `TTSBatchPanel.tsx:378`, `ExportModal.tsx:278`, `ModelDownloadModal.tsx:125`,
  `LivingWorldDrawer.tsx:103` and `:130`, `CharacterSheetDrawer.tsx:51` and `:71`,
  `TheaterTransport.tsx:53`.
- `aria-live` is used 3 times (`TheaterDialogue.tsx:55`, `WorldDraftReview.tsx:132`,
  `ImportProgressPanel.tsx:41`); the streaming turn area and the error banners
  (`App.tsx:1118-1159`) are not announced.
- There are 90 `aria-label` attributes, so the practice is understood; but many
  icon-only buttons are named by `title` alone (`App.tsx:904, 973, 981, 989, 999, 1009`;
  `SystemsStudio.tsx:552, 562, 570`), which is not a reliable accessible name.

Recommendation: the a11y work becomes cheap once F-7's `<Modal>` exists. Add
`eslint-plugin-jsx-a11y`, and add `vitest-axe` (or `jest-axe`) so the tests in T-5 can
assert a clean axe run.

#### F-9. Failures are swallowed to the console in 55 places

**Severity: Medium.**
Evidence: 55 `console.*` call sites in `src/`, 22 of them in `App.tsx` alone, mostly
`.catch(console.error)` (for example `App.tsx:166, 209, 210, 211, 277`). Users see
nothing, and `noUnusedLocals` cannot detect a dropped failure.

Recommendation: route errors into a visible state (a toast or inline banner) or through
a small logger that also surfaces them in the debug panel.

#### F-10. Hardcoded routes, storage keys and defaults outside the API client

**Severity: Medium.**
Evidence:

- `/api/...` strings re-typed outside `client.ts`: `App.tsx:319`
  (`/api/game/${...}/character/${...}/portrait`), `App.tsx:690`
  (`new EventSource('/api/audio/events')`), `WorldsStudio.tsx:240, 241, 374, 376`,
  `StoryTheater.tsx:113`, `CodexDrawer.tsx:239`, and a raw `fetch` in
  `editor/frontmatterSchema.ts:15` that duplicates
  `APIClient.getEntityFrontmatterSchema` (`client.ts:1286`). Since `client.ts` is
  documented as "the only fetch layer", these are contract violations.
- `localStorage` keys `'localrpg_active_game'` and `'localrpg_last_played_game'` are
  hardcoded in `App.tsx:187, 188, 247, 248, 380, 381` and again in
  `LauncherHub.tsx:52, 141`; `'localrpg.settings.advanced'` lives only in
  `useAdvanced.ts:10`. No key registry exists.
- Extension strings `.lrpgsystem` / `.lrpgpack` repeat in `SystemsStudio.tsx:478, 479,
  548, 570`.
- Magic defaults: model id `'kokoro-tts'` and `90177536` bytes at `App.tsx:496`;
  a hardcoded `GEMINI_AGENT_MODELS` list at `LLMRoleEditor.tsx:9-24`; the display-mode
  union `'stage_directions' | 'hidden' | 'raw'` re-declared in `types.ts`, `App.tsx`,
  `StoryTheater.tsx` and `CodexDrawer.tsx`.

#### F-11. Duplicated helpers and two competing component idioms

**Severity: Low.**
Evidence: `errorMessage()` is defined identically in `WorldsStudio.tsx:99` and
`SystemsStudio.tsx:93`; `slugify` exists in `lib/slug.ts` yet is reimplemented inline at
`SystemsStudio.tsx:948` and `:962`; `typeFromMarkdown` (`WorldsStudio.tsx:30`) re-parses
frontmatter that `editor/frontmatter.ts` already parses. Declarations mix
`export const X: React.FC<Props>` (about 90 components) with
`export default function X(...)` (`EntityTree.tsx:64`, `ContentStudio.tsx:21`,
`editor/MarkdownEditor.tsx:55`, `RowMenu.tsx:23`). Component file-header comments exist
(`EntityTree.tsx:60`, `ContentStudio.tsx:18`, `App.tsx:34`) but are applied
inconsistently.

Recommendation: pick the `function` declaration form (React 19 has no reason to prefer
`React.FC`), delete the local helpers, and adopt the existing header comment convention
across `components/`.

### 2.3 Test coverage

#### T-1. Frontend statement coverage is 33.4%

**Severity: High.**
Evidence: `npx vitest run --coverage` reports Statements 33.36% (2,801/8,394),
Branches 30.09% (2,299/7,638), Functions 33.17% (771/2,324), Lines 34.98%
(2,597/7,424). 69 test files, 339 tests, 4,523 lines of test code against 33,103 lines
of source (a ratio of 0.14, against Go's 0.76).

Recommendation: add coverage thresholds to `frontend/vitest.config.ts` set just above
today's numbers so the floor can only rise, then raise it deliberately as the untested
components below are covered.

#### T-2. 56 of 95 components have no test

**Severity: High.**
Evidence: 39 components are covered (ActionConsole, BaseSystemCatalogue,
CharacterSheetDrawer, ChronicleView, ContentImportDialog, DiceCheckCard, EntityAvatar,
EntityBatchDialog, EntityTree, GenerationLimitNotice, ImportProgressPanel, LimitChip,
MarkdownProse, MechanicsEditor, MechanicsStrip, OfflinePreset, PendingCheckCard,
ProviderManager, RegistryModal, ScenarioEditor, SegmentAudioControls, SettingsStudio,
StoryTheater, SystemEnhanceDialog, SystemGenerateDialog, WorldDraftReview,
WorldEnhanceDialog, WorldGenerateDialog, WorldsStudio (delete path only), StoryPlayer,
TheaterDialogue, TheaterStage, ScriptReference, NewCampaignModal,
`ui/{AdvancedSection, Example, HelpTip, SubTabs}`, `providers/ImageProviderEditor`,
`treeModel`, `editor/frontmatter{,Lint,Schema}`).

Untested: `App.tsx`, `AddEntityModal`, `AboutModal`, `CinematicOverlay`,
`ContentStudio`, `ContextDrawer`, `DebugPanel`, `Drawers`, `EmbeddingProviderEditor`,
`ExportModal`, `GraphDrawer`, `LauncherHub`, `LivingWorldDrawer`, `MarkdownDocViewer`,
`MenuBar`, `ModelDownloadModal`, `NewEntityWizard`, `ProloguePanel`, `RecordNotice`,
`RowMenu`, `TurnHistoryList`, `TurnSegments`, `UsagePanel`, `VoiceCatalogModal`,
`VoiceCombobox`, `VoiceOptionsControl`, `VoiceProfileSelect`, `TreeDialogs`,
`editor/MarkdownEditor`, `editor/{entityIndex, wikilink, languages, theme}`,
`launcher/{CampaignGallery, CampaignHeroStage, CampaignSettingsModal,
DiscardDraftConfirm, LauncherDock, ProceduralAsset, WorldFlyout, WorldGallery}`,
`settings/{AgentsPanel, ContextLimitsPanel, GenerationLimitsPanel, MediaPanel,
PathsPanel, PreferencesPanel, ProvidersPanel}`, `providers/{CloudKeysPanel,
EmbeddingsPanel, LLMRoleEditor, STTProviderEditor, TTSProviderEditor, TierBadge}`,
`theater/TheaterTransport`, `mechanics/RowList`.

#### T-3. Neither suite is gated on coverage, and neither has a static analyser

**Severity: High.**
Evidence: `.github/workflows/ci.yml` runs `go vet`, `mise run test:backend`,
`mise run test:frontend` (which is `tsc --noEmit` plus `vitest run`), the fuzz targets,
`mise run lint:docs` and `STRICT=1 mise run lint:prose`. There is no `go test -cover`
gate, no `--coverage` gate, no `staticcheck`, no `golangci-lint`, no `revive`, no
`errcheck`, and no `gofmt -l`. `mise run lint` runs `go vet` only for code.

Recommendation: add `gofmt -l` and `staticcheck ./...` to `mise run lint` and to CI, and
add coverage floors to both suites. Note that `staticcheck` will very likely surface
R-7's discarded errors and R-10's dead code, which makes it a good forcing function.

#### T-4. 422 Go functions are at 0% coverage, including the entire CLI surface

**Severity: High.**
Evidence: `go tool cover -func` reports 422 functions at 0.0%. Package totals:
`cmd/localrpg` 29.8%, `pkg/tui` 44.0%, `pkg/telemetry` 47.3%, `pkg/scene/fonts` 45.2%,
`pkg/config` 66.2%, `pkg/gui` 69.1% (on 32,008 lines, so about 9,900 uncovered
statements), and `pkg/provider/geminiembedding` 22.5%.

Every user-facing command handler reads 0.0%: `handleRollCommand`
(`roll.go:10`), `handlePromptCommand` (`prompt.go:14`), `handleTTSCommand`
(`media.go:16`), `handleImageCommand` (`media.go:77`), `handlePlayCommand`
(`play.go:27`), `handleGUICommand` (`gui.go:54`), `handleExportCommand`
(`export.go:19`), `handleExportInspect` (`export.go:301`), `handleTTSBatchCommand`
(`ttsbatch.go:25`), `handleDebugCommand` (`debug.go:62`), `handleConfigCommand`
(`offline.go:12`), `handleRegistryCommand` (`registry.go:20`), `handleStylesCommand`
(`styles.go:15`), `handleContentCommand` (`content.go:14`), `handlePublisherCommand`
(`provenance.go:20`), plus `printUsage` (`main.go:75`).

Root cause: R-13. The `os.Exit`-calling command shapes cannot be driven from a test.

Recommendation: convert the remaining commands to the `runXCommand(args, stdout,
stderr) int` shape already used by `registry.go` and `styles.go`, then table-test each
one: no arguments, unknown flag, and a happy path against a `t.TempDir()` root.

#### T-5. Seven packages have no test file at all

**Severity: Medium.**
Evidence: `pkg/provider/imagecli` (2 files), `pkg/provider/imageprocedural` (1),
`pkg/provider/inworldshared` (1), `pkg/provider/sttwebspeech` (1),
`pkg/provider/sttwhispercli` (2), `pkg/provider/ttspiper` (2), `pkg/scene/fonts/gen`
(1). `pkg/e2e` is excluded from the default run by build tag, which is intended, but it
also reports 0.0% in the untagged profile.

The provider packages are the notable ones: they are small adapters, which is exactly
the shape that is cheap to test with a fake command or a stub HTTP server, and
`pkg/provider/ttssherpa`, `inworldstt` and `inworldtts` already do this.

#### T-6. No Go test runs in parallel, and 16 tests sleep

**Severity: Medium.**
Evidence: `t.Parallel()` appears **zero** times across 475 test files. `time.Sleep`
appears 16 times. `mise run test:backend` runs 2,012 test functions serially, in about
40 seconds locally, which is tolerable now but scales linearly.

Recommendation: add `t.Parallel()` to table-driven tests that hold no shared state, and
replace the 16 sleeps with polling helpers (`require.Eventually`-style loops) to remove
timing flakiness.

#### T-7. No race detector or cross-platform test job

**Severity: Medium.**
Evidence: `mise run test:backend` is `go test -v -count=1 ./...`, and CI runs exactly
that. There is no `-race`, no `-shuffle=on`. CI tests only on `ubuntu-latest`, while the
release matrix ships `linux/amd64`, `linux/arm64`, `darwin/arm64`, `darwin/amd64` and
`windows/amd64`.

Impact: the desktop app is cross-platform by contract, and its path handling
(`pkg/paths`, `pkg/pathutil`), native TTS (`spd-say`/`say`/PowerShell), audio, and
socket code are exercised only on Linux before a tag. A release runner will find those
bugs only at packaging time.

Recommendation: add a `-race` job on Linux, and a lightweight `go test ./...` job on
`windows-latest` and `macos-latest` for the packages that build without the GTK headers.
At minimum, add `-shuffle=on` to catch inter-test ordering dependencies.

### 2.4 UI/UX flow coverage

#### U-1. Ten end-to-end tests cover the whole application

**Severity: High.**
Evidence: `pkg/e2e` contains exactly ten tests:
`TestDeletingAWorldEntityKeepsTheStudioUsable`, `TestDeletingTheLastWorldEntityKeepsTheStudioUsable`,
`TestMirroredPortraitRendersInsideItsRoundedBox`, `TestSettingsStudioRendersEveryTab`,
`TestSettingsRoundTripAPath`, `TestSettingsEmbeddingsSectionRenders`,
`TestLauncherHubRenders`, `TestExportedBundlePlaysTheTheatre`,
`TestDeletingTheLastCampaignResetsLauncherStats`,
`TestCreatingACampaignFromTheLauncher`.

The harness itself is good (`pkg/e2e/harness.go` wraps chromedp with `Click`, `Type`,
`WaitFor`, `Poll`, `InstrumentAudio`; `fixture.go` starts the real `gui.Service` behind
an `httptest` server; failures write `body.txt`, `dom.json` and `failure.png`; the suite
skips rather than fails without a browser, and CI installs Chrome and uploads
artifacts). The investment is in the scaffolding, not in the flows.

#### U-2. The primary user journey has no end-to-end test

**Severity: High.**
Evidence: the central flow is: launch, create a campaign, play a turn, watch it stream,
hear it, undo it. There is no test that plays a turn. `POST /api/game/{id}/turn` streams
newline-delimited JSON and is serialised per campaign by `Service.BeginTurn` (409 while
one is in flight); the frontend consumes it in `App.tsx`, which has no unit test either
(T-2). So the streaming path, the turn segments, the audio controls and the undo
affordance are covered nowhere.

Recommended flows to add, in priority order:

1. Create a campaign from the launcher, submit a turn, assert the narration renders and
   the chronicle gains a turn.
2. Undo (`/undo`) and assert the chronicle loses the turn and the entity state reverts.
3. A turn that returns a rate-limit or funds error, asserting the visible error path
   (`App.tsx:1118-1159`) rather than a console message.
4. Open every drawer (Codex, Graph, Context, Living World) and assert it opens, closes
   on Escape, and returns focus to the trigger. This one doubles as the F-8 regression
   test.
5. Import a content package, assert the progress panel reports completion, and assert
   the imported world appears in the studio list.

#### U-3. Loading, empty and error states are untested almost everywhere

**Severity: Medium.**
Evidence: `RegistryModal.test.tsx:83-121` is the only test file with deliberate
empty-state coverage (four cases). Loading and error states are untested in
`UsagePanel`, `TTSBatchPanel`, `ExportModal`, `ModelDownloadModal`,
`LivingWorldDrawer` and every `settings/*` panel.

#### U-4. Implemented keyboard interactions are untested

**Severity: Medium.**
Evidence: `MenuBar.tsx:78-116` implements an Alt-key menu and `VoiceCombobox.tsx:152`
and `VoiceProfileSelect.tsx:133` implement keyboard navigation, with no tests. Escape
handling exists on 8 surfaces (F-8) and is asserted nowhere.

### 2.5 Documentation

The user-facing documentation is genuinely strong: 24 articles totalling about 19,800
words, covering architecture, worlds, systems, campaigns, providers, agents, storage,
both studios, the codex schema, pricing, a generated provider catalogue, a generated
config reference, six local-stack guides, embeddings, local-first play, the content
package format, AI world generation, and style packs. Two of them are generated from
the code and guarded against staleness
(`pkg/gui/docs_catalogue_test.go:261`). This is better than most projects of this size.

The gaps are coverage of surfaces and the absence of an on-ramp.

#### D-1. No CLI reference, and 9 of 13 commands are undocumented

**Severity: High.**
Evidence: `localrpg` exposes `roll`, `prompt`, `play`, `tts` (and `tts batch`),
`image`, `gui`, `export` (and `export inspect`), `debug` (and `test-run` and
`test-system`), `config`, `content`, `publisher`, `registry`, `styles`, and `version`.
Documented anywhere: `export web`/`video` (README:112, `04-campaigns.md:115`),
`debug server`/`test-run`/`test-system` (`docs/debugging.md`, `09-systems-studio.md:101`),
`content`/`publisher`/`registry` (`22-editing-content.md:288-315`), `config`
(`05-providers.md:55`), `styles` (`24-style-packs.md:79`).

Undocumented: `roll`, `prompt`, `play`, `tts`, `tts batch`, `image`, `gui`, `export
inspect`, and `version`. Users must read `main.go:75` (`printUsage`) to discover them.

Recommendation: write `pkg/gui/docs/25-cli-reference.md` with every command, its flags,
and its exit codes. It needs no registration: both the site generator and the lint
script read the directory.

#### D-2. The TUI is undocumented as a mode

**Severity: High.**
Evidence: `localrpg play` runs a full Bubbletea client (`pkg/tui`, `cmd/localrpg/play.go`),
but it appears in the documentation only as a reindex side note
(`19-local-stack-docker-compose.md:301`, `22-editing-content.md:321`). Keybindings,
`/undo`, `/gm`, the inspection views and how it differs from the GUI are all absent.
`pkg/tui` is also the second-lowest-covered Go package at 44.0% (T-4).

#### D-3. No GUI navigation guide; only the two Studios are documented

**Severity: High.**
Evidence: `08-worlds-studio.md` and `09-systems-studio.md` cover the studios. Nothing
documents the launcher hub, campaign gallery, hero stage, Codex Drawer, Chronicle view,
character sheet, graph view, Living World drawer, prologue panel, or the eight settings
panels.

#### D-4. Story Theatre, audio playback and portrait generation are undocumented flows

**Severity: Medium.**
Evidence: Story Theatre (the in-app visual-novel replay) has no guide, only the export
side (`04-campaigns.md:115`). Segment audio controls
(`SegmentAudioControls.tsx`) have no UI documentation, only config keys
(`13-configuration-reference.md:120`). Character portrait generation is scattered
across four articles with no end-to-end workflow (`10-codex-yaml.md:61`,
`18-local-image-comfyui.md:99`, `22-editing-content.md:217`, `24-style-packs.md:18`).

#### D-5. Duplicate `order` frontmatter breaks the documentation sidebar sort

**Severity: Medium, and it is a live UI bug.**
Evidence: `07-storage-paths.md`, `11-usage-and-pricing.md`, `12-provider-catalogue.md`
and `13-configuration-reference.md` all declare `order: 7`; `08-worlds-studio.md` and
`24-style-packs.md` both declare `order: 8`. `DocsModal.tsx:124` sorts the sidebar by
`order`, so the Configuration & Providers category renders in an arbitrary order.

Note that articles 11, 12 and 13 are generated, so their `order` values are produced by
`renderProviderCatalogue` (`pkg/gui/docs_catalogue_test.go:26`) and its siblings; fix
the generators, not just the files.

#### D-6. No documentation landing page and no link from the README

**Severity: Medium.**
Evidence: the 24 articles carry `category` and `order` frontmatter and are numbered
`01` to `24`, but there is no index article. `DocsModal.tsx:56` opens whatever sorts
first. `README.md` (1,418 words) never references `pkg/gui/docs` at all, so a new user
reading the repository has no path into the manual.

#### D-7. Cross-links do not resolve outside the in-app viewer

**Severity: Low.**
Evidence: article cross-links omit the `.md` extension, for example
`[Editing Content Outside the App](22-editing-content)` (`02-worlds.md:47`,
`05-providers.md:50`, `23-ai-world-generation.md:154`), while the README uses real paths
(`README.md:131`). The extension-less form renders correctly only in
`MarkdownDocViewer`; it is dead on GitHub, in the showcase site, and in any editor.

#### D-8. No contributor on-ramp

**Severity: High.**
Evidence: there is no `CONTRIBUTING.md`, no `SECURITY.md`, no `CODE_OF_CONDUCT.md`, no
`CODEOWNERS`, no pull-request template and no issue template; `.github/` contains only
`workflows/`. The only architecture document is `docs/architecture/review-2026-09-24.md`,
a dated review rather than a living guide, and its own header says it is "intended to
become `docs/architecture/overview.md` as the refactors it references land". `AGENTS.md`
is the closest thing to an onboarding document but is written for agents.

Recommendation: add `CONTRIBUTING.md` (build, test, lint, branch and commit conventions,
the project board workflow), promote the architecture review into a living
`docs/architecture/overview.md`, and add issue and PR templates.
`THIRD_PARTY_NOTICES.md` and `LICENSE` already exist, so the license side is covered.

#### D-9. Stale counts and command lists in `AGENTS.md`

**Severity: Low.**
Evidence: `AGENTS.md` states "23 guide articles" and a Vale scope of "25 files"
reporting "234 alerts ... 0 errors, 100 warnings and 134 suggestions". Measured today:
**24** articles, **26** files linted, and **290 alerts (0 errors, 112 warnings, 178
suggestions)**. `AGENTS.md:42` and the README also omit `content`, `publisher`,
`registry` and `styles` from the documented CLI surface.

Recommendation: either re-measure these numbers as part of the release checklist, or
remove them and point at `mise run lint:prose` for the current figure.

#### D-10. Vale is configured for American spelling in a British-English repository

**Severity: Low.**
Evidence: `mise run lint:prose` reports `Google.Spelling` warnings for "colour" and
"Colour" (`24-style-packs.md:22`, `:86`, `:88`), while the documentation and the code
comments consistently use British English ("behavioural digest",
`22-editing-content.md:340`). Nineteen `Google.Semicolons` and 66 `Google.Passive`
alerts remain, all documented in `AGENTS.md` as deliberate.

Recommendation: add the British spellings in use to
`styles/config/vocabularies/LocalRPG/accept.txt`, which is the file the project already
uses for exactly this purpose, and consider whether `Google.Spelling` should be disabled
outright as a house-style decision.

#### D-11. Long articles without internal structure

**Severity: Low.**
Evidence: `22-editing-content.md` is 343 lines and `19-local-stack-docker-compose.md` is
369 lines. `23-ai-world-generation.md:63-79` is five dense paragraphs under one heading.
The README's feature bullets (`README.md:13-18`) and engine bullets (`:85-96`) are dense
blocks. Jargon appears on first use without a gloss: "Goja runtime"
(`03-systems.md:117`), "span waterfall" (`docs/debugging.md:50`), "capability tier"
(`05-providers.md:41`), "behavioural digest" (`22-editing-content.md:340`), "ledger key"
(`12-provider-catalogue.md:15`). Articles 11, 12 and 13 also reuse identical section
names ("Ledger key rules", "Built-in default prices"), which makes search results
ambiguous.

#### D-12. No telemetry or observability guide

**Severity: Low.**
Evidence: `trace_level` and the OTLP exporter settings exist as config keys
(`13-configuration-reference.md:201-214`) and `docs/debugging.md` describes the debug
server, but nothing explains what a trace level does, where traces are written, or how
to read a span tree. `pkg/telemetry` is at 47.3% coverage (T-4).
**Security note:** `pkg/trace` carries redaction logic and the test that exercises it is
one of only two gitleaks allowlist entries, so this subsystem is load-bearing for the
"never leak a credential" property; it deserves documentation more than most.

### 2.6 Dependencies

#### Z-1. 21 of 38 direct Go modules are behind, including two beta and one pseudo-version pin

**Severity: Medium.**
Evidence (`go list -m -u`, direct dependencies only):

| Module | Current | Latest |
|---|---|---|
| `github.com/wailsapp/wails/v3` | v3.0.0-beta.24 | v3.0.0-beta.28 |
| `github.com/chromedp/chromedp` | v0.16.0 | v0.21.0 |
| `github.com/chromedp/cdproto` | v0.0.0-20260922... | v0.157.10 |
| `github.com/yalue/onnxruntime_go` | v1.28.0 | v1.36.0 |
| `github.com/ebitengine/oto/v3` | v3.1.0 | v3.5.1 |
| `github.com/dop251/goja` | v0.0.0-20260917... | v0.0.0-20261012... |
| `github.com/yuin/goldmark` | v1.7.16 | v1.8.6 |
| `google.golang.org/genai` | v1.72.0 | v1.73.0 |
| `modernc.org/sqlite` | v1.59.0 | v1.60.1 |
| `golang.org/x/image` | v0.46.0 | v0.47.0 |
| all 10 `go.opentelemetry.io/otel*` modules | v1.46.0 / v0.22.0 | v1.47.0 |

The whole OpenTelemetry stack moving in lockstep means it should be bumped as one unit,
and `go.opentelemetry.io/otel/log` jumping `v0.22.0` to `v1.47.0` is a real stability
promotion worth reading the release notes for.

#### Z-2. Four frontend dependencies are a full major version behind

**Severity: Medium.**
Evidence (`npm outdated`):

| Package | Current | Latest | Note |
|---|---|---|---|
| `typescript` | 5.9.3 | 7.0.2 | two majors |
| `vite` | 6.4.3 | 8.3.4 | two majors |
| `@vitejs/plugin-react` | 4.7.0 | 6.1.2 | two majors |
| `lucide-react` | 0.475.0 | 1.55.0 | pre-1.0 to stable |
| `vitest`, `@vitest/coverage-v8` | 5.0.3 | 5.0.4 | patch |

`package.json` declares `"vite": "^6.2.0"` and `"lucide-react": "^0.475.0"`, so a caret
range cannot reach the new majors; this needs a deliberate bump. The Vite and
TypeScript majors should be done together with `vitest`, `@vitest/coverage-v8` and
`@vitejs/plugin-react`, since the Vitest transform pipeline depends on all of them.

#### Z-3. No automated dependency update mechanism

**Severity: Medium.**
Evidence: `.github/` contains only `workflows/`. There is no `dependabot.yml` and no
Renovate config. `go.mod` has 38 direct and about 60 indirect modules;
`frontend/package.json` has 21 dependencies and 17 devDependencies. `npm audit` reports
0 vulnerabilities today, and gitleaks runs weekly, but nothing opens a pull request when
a module publishes a security release.

Recommendation: add a Dependabot config covering `gomod`, `npm` (directory `/frontend`)
and `github-actions`, grouped so the ten-module OpenTelemetry set arrives as one PR.

#### Z-4. No Go vulnerability scanning

**Severity: Medium.**
Evidence: CI runs `gitleaks` over git history, which finds committed credentials, but
nothing scans module code for known vulnerabilities. `govulncheck` is not installed in
`mise.toml` and is not a CI step. The project takes CGO dependencies with a large
surface (`sherpa-onnx`, `onnxruntime_go`, `wails`, `webkits`, `oto`, `beego`-class
transitive trees) plus a 60-module indirect graph.

Recommendation: pin `govulncheck` in `mise.toml` alongside `gitleaks` and `actionlint`,
add a `mise run vuln:scan` task, and run it on push, on pull request, and weekly, which
mirrors the existing secrets workflow exactly. Note that `npm audit` is deliberately
not in CI; if it is added, it should start from today's clean tree.

### 2.7 CI/CD and tooling

#### C-1. CI has no Go linter, no formatting check and no coverage gate

**Severity: High.**
Evidence: `.github/workflows/ci.yml`. The `go` job runs `go vet ./...` and
`mise run test:backend`. The `web` job runs `mise run test:frontend` (`tsc --noEmit`
plus `vitest run`) and `mise run lint:docs`. `mise run lint` adds only
`lint:goreleaser` and `lint:actions`.

Absent: `gofmt -l` (65 files currently fail it, R-14), `staticcheck`/`golangci-lint`,
`revive`, `errcheck`, ESLint, Prettier, `-race`, coverage thresholds, and any frontend
formatting check.

One correction to a first reading of this section, worth recording because the mistake
is easy to repeat: **CodeQL is running.** `Analyze (go)`,
`Analyze (javascript-typescript)` and `Analyze (actions)` all report on every pull
request, so the repository does have security static analysis (SAST). It is invisible
from the checkout, because it is GitHub's *default setup* rather than a workflow file:
`.github/workflows/` contains only `ci.yml`, `pages.yml`, `release.yml` and
`secrets.yml`, and nothing in the repository mentions CodeQL at all. The claim being
made here is narrower than "no static analysis", and it is about correctness linters
rather than security scanners: nothing enforces Go idioms, dead code, unused symbols or
discarded errors, and `go vet` alone will not.

Recommendation, in order of return: `gofmt -l` (one line, immediately green after R-14),
ESLint plus Prettier, `staticcheck`, then coverage floors.

#### C-2. CI tests only Linux while shipping five platform targets

**Severity: Medium.**
Evidence: every job in `ci.yml` is `runs-on: ubuntu-latest`. `release.yml` packages
`linux/amd64`, `linux/arm64`, `darwin/arm64`, `darwin/amd64` and `windows/amd64`, and
`.goreleaser.yaml` has a `skip` template per target. Native-OS TTS paths
(`spd-say`/`say`/PowerShell), path resolution (`pkg/paths`, `pkg/pathutil`) and the
Windows socket handling are therefore first exercised at tag time.

Recommendation: add a matrix `go test` job for `windows-latest` and `macos-latest`
restricted to the packages that build without the GTK/WebKit headers, so path and
platform logic is tested before a release, not during one.

#### C-3. Release packaging compiles each target but never runs its tests on that target

**Severity: Low.**
Evidence: `release.yml` re-runs CI via `verify: uses: ./.github/workflows/ci.yml`,
which is excellent practice, but that reused workflow is ubuntu-only.

#### C-4. `project.sh add-wave` and `add-area` silently clear board fields

**Severity: High, and it is a data-loss bug.**
Found while registering this review's proposals, and it was triggered in the process.

`ensure_option` (`scripts/project.sh:169`) rebuilds a single-select field's option list
and sends it back to `updateProjectV2Field`. It sent each existing option as
`{name, description, color}` with **no `id`**. `ProjectV2SingleSelectFieldOptionInput`
does have an `id` field (confirmed by schema introspection), and without it GitHub treats
every option as new, deletes the originals, and clears that field on every project item
that referenced one.

Blast radius when triggered: adding five waves and six areas wiped the Wave and Area
values from all twelve items that had them (issues #122 to #133). They were restored by
hand from a listing taken beforehand, and the bug is fixed in the same commit as the
option-id preservation plus the C-5 parser fix. The lesson is general: the tooling that
manages the board is itself untested code with write access to the board.

#### C-5. `project.sh` cannot parse an option whose name contains a comma

**Severity: Medium.**

`resolve()` packed each field's options into one comma-joined column
(`map("\(.name)=\(.id)") | join(",")`), and `option_names`, `has_option`, `option_id`
and `ensure_option` all re-split that column on `,`. Any option name containing a comma
was therefore truncated at the comma.

This is not hypothetical: the Wave 10 option added for this phase is
`Wave 10 - Quality: dependencies, docs and polish`, and `wave_option 10` returned
`Wave 10 - Quality: dependencies`. The epic that used it failed to create with
`could not add to milestone 'Wave 10 - Quality: dependencies': not found`.

Fixed by emitting one row per option instead of a packed column
(`option <field> <id> <name> <colour> <description>`), which is unambiguous for any name
containing no tab, and which is also what lets C-4's fix carry each option's id.

#### C-6. Owned strengths in CI worth keeping

For balance, these are genuinely good and should not be lost in a refactor:

- `ci.yml` declares `workflow_call`, so `release.yml` re-runs the full check suite on a
  tag rather than trusting the tagger.
- The `frontend` job builds `pkg/gui/dist` once and shares it as an artifact, with a
  documented reason (the GUI embeds it and the e2e suite drives the same bundle).
- Go module and build caches are keyed differently and correctly: modules on `go.sum`,
  the build cache on the commit SHA, with the reasoning written down.
- Fuzzing runs on every CI pass with a short budget (`10s`) so a committed seed cannot
  regress silently; a longer budget is available via `mise run test:fuzz`.
- The e2e job asserts Chrome is present (`google-chrome --version`) specifically so the
  suite cannot silently skip for want of a browser, and uploads `test-results/` on
  failure with `if: always()`.
- `secrets.yml` scans full history on push, on pull request, and weekly, with the
  rationale ("newly published detection rules ... applied to history that has not
  moved") written in the file.
- CodeQL runs on every pull request as GitHub's default setup, covering Go, the
  JavaScript/TypeScript frontend and the workflow files themselves.

The last one carries a documentation gap rather than a configuration one. Nothing in the
repository records that CodeQL is enabled, because default setup lives in repository
settings rather than in a workflow file, and a green analysis is easy to mistake for an
absent one. That is exactly what happened during this review. A line in `AGENTS.md`
naming the checks a pull request gets, and which of them are configured outside the
repository, would fix it.

### 2.8 Security and configuration hygiene

This is the strongest area of the project, and the findings here are minor.

#### S-1. Secrets discipline is sound

Evidence: `.gitignore:23` ignores `*.key` and `.gitignore:25` ignores `*-session.txt`;
`11labs.key` and `crush-session.txt` are present in the working tree and both are
ignored. `api_key: "env:OPENAI_API_KEY"` is the documented form. `.gitleaks.toml`
extends the default ruleset and allowlists only the two fake credentials the `pkg/trace`
redaction tests need. `mise run secrets:scan` covers full history and staged changes.
No hardcoded credential was found in tracked files during this review.

#### S-2. Internal error strings are returned to HTTP clients

**Severity: Medium.**
Evidence: roughly 90 call sites use `http.Error(w, err.Error(), ...)` directly rather
than the sentinel-aware `writeGameError` (`server.go:141`). Examples include
`server.go:886`, `:949`, `:964`, `:990`, and `handleContentExportRoute` branches on
`strings.Contains(err.Error(), "not found")` instead of `errors.Is`
(`server.go:2272`). The same logical failure maps to different codes: a missing entity
is 404 (`server.go:1038`) while a failed `GetGameState` is 500 (`server.go:579`), and
`CreateGame` failures are 400 (`server.go:1078`) while `ListGames` failures are 500
(`:1066`). Wrong HTTP methods return 404 in some branches (`:427`, `:505`, `:532`) and
405 in others (`:424`, `:473`).

Impact: wrapped internal errors leak file paths and package structure to the browser or
socket client, and the frontend cannot reliably distinguish not-found from a transient
failure (which is also F-5's cause).

Recommendation: route every handler through `writeGameError`, return a stable error code
alongside a generic message, and log the detail. Add `errors.Is` matching.

#### S-3. Response encoding errors are discarded, producing truncated 200 responses

**Severity: Medium.**
Evidence: `writeJSON` ignores the encoder result (`server.go:1050`), as do the inline
`_ = json.NewEncoder(w).Encode(...)` sites (`server.go:177`, `:1083`, `:1111`, `:1295`,
`:2060`, `:2105`, `:2170`, `:2177`). A mid-stream marshal failure returns 200 with a
partial body.

#### S-4. The coverage output directory is not gitignored

**Severity: Low.**
Evidence: `npm run test:coverage` writes `frontend/coverage/`, which `.gitignore` does
not cover (it ignores `test-results/` and `pkg/e2e/test-results/` but not
`frontend/coverage/`). Running the coverage script leaves an untracked directory that
`git status` reports. (The directory created during this review was removed.)

#### S-5. `http.CrossOriginProtection` is the right choice and should be protected

Evidence: `pkg/gui/middleware.go` wraps handlers in the standard library's
`http.CrossOriginProtection` rather than hand-rolled CORS, cross-origin writes get 403,
the native Wails webview (`wails://wails`) is allowed, and `Server.ServeHTTP` is a bare
mux delegation. This is correct and should not regress; the advice in `AGENTS.md` ("do
not add `Access-Control-*` headers back") is worth keeping in `CONTRIBUTING.md`.

---

## 3. What the project already does well

Recorded so the task synthesis does not "fix" these:

1. **No TODO debt.** `rg 'TODO|FIXME|HACK|XXX'` over all Go and TypeScript returns
   **zero** matches. No commented-out code blocks were found either.
2. **No lint suppressions hiding problems in Go.** Zero `//nolint` directives. The five
   `eslint-disable` comments in the frontend are dead rather than masking, since ESLint
   is not installed (F-1).
3. **Generated artefacts cannot drift.** `pkg/gui/docs_catalogue_test.go:261` fails with
   "is stale; regenerate with `go test ./pkg/gui -update-docs`", and `routes_test.go:42`
   does the same for the route manifest. This is exactly the pattern the DTO layer needs
   (F-4).
4. **A real end-to-end harness.** chromedp plus a real `gui.Service` behind `httptest`,
   with audio instrumentation, capability probing (`driver.Available`), and failure
   artifacts. Rare and valuable.
5. **Fuzzing in CI** for the two parsers that most need it (`pkg/turnstream`,
   `pkg/jsonrepair`), with a short budget on every run.
6. **Secret scanning over full history**, on push, on pull request, and weekly, with
   push protection recommended in CI documentation.
7. **A disciplined release pipeline** that re-runs CI on the tag, builds each target on a
   native runner because CGO forbids cross-compilation, and explains each constraint in
   comments.
8. **Standard-library-only Go tests** with no testify, table-driven tests in 77 files,
   and a test:code ratio of 0.76.
9. **`tsc` with `strict`, `noUnusedLocals` and `noUnusedParameters`**, so unused imports
   fail the build.
10. **A documented on-disk contract.** `AGENTS.md` and `22-editing-content.md` both
    state the three-tier layout and the interchange format, so the system is
    approachable from the files as well as the code.
11. **Deliberate, explained configuration.** Nearly every non-obvious setting in
    `mise.toml`, `.vale.ini`, `.rumdl.toml`, `.gitleaks.toml`, `.goreleaser.yaml` and
    the workflows carries a comment giving the reason. This is unusually good and makes
    the repository's decisions auditable.

---

## 4. Prioritised backlog

Each item is written so it can become one task. Effort is `S` (hours), `M` (a day or
two), `L` (a week or more). Impact is the finding severity.

### Workstream A: make the code readable

| # | Task | Findings | Effort | Impact |
|---|---|---|---|---|
| A1 | Split `pkg/gui/service.go` (6,579 lines) by domain into `games.go`, `worlds.go`, `systems.go`, `entities.go`, `audio_service.go`, `assets_service.go`, `registry_service.go` | R-1 | L | High |
| A2 | Break `handleGameRoutes` (server.go:371, 676 lines) into per-resource route files and a shared body-decode helper | R-2 | M | High |
| A3 | Extract the phases of `ProcessActionStream` (980 lines) into named methods | R-3 | L | High |
| A4 | Add `// Package x` doc comments to the 12 core packages, starting with `engine`, `harness`, `storage`, `entity` | R-4, R-5 | M | Medium |
| A5 | Document exported symbols in `pkg/state`, `pkg/entity`, `pkg/storage`, `pkg/rules`, `pkg/engine/game.go`; add `revive` in report-only mode | R-6 | M | Medium |
| A6 | Fix the cited load-bearing error discards in `storage/store.go:113`, `engine/timeline.go:292`, `engine/orchestrator.go:1558` and `:2214`, `storage/sync.go:105`; then triage with `errcheck` | R-7 | M | High |
| A7 | Explain or encapsulate the 12 panics in `pkg/rules/js_engine.go` bindings | R-8 | S | Medium |
| A8 | Replace `"gm"` and engagement-mode literals with the existing constants; name the remaining magic values | R-9 | S | Medium |
| A9 | Delete dead code: `speechFormattingInstruction`, the unreachable `pkg/state` branch, `routeMount.Handler`, the four unused `RefKind`s, `BuildModel`, the unexported-but-exported `TurnRecordView`; merge the duplicate `appendUnique` helpers | R-10 | S | Medium |
| A10 | Add a test that fails when the four `EntityFrontmatter`/`Entity` field lists diverge | R-11 | M | Medium |
| A11 | Work down the remaining 100+ line functions (R-12 list) | R-12 | L | Medium |
| A12 | Unify `cmd/localrpg` command shapes on `runXCommand(args, stdout, stderr) int`; align flag error modes; stop discarding the config load error in `play.go:34` | R-13 | M | Medium |
| A13 | Run `gofmt -w cmd pkg tools`, then gate on `gofmt -l` in CI | R-14, C-1 | S | Medium |
| A14 | Rewrite the 471 `interface{}` uses as `any` (or change the convention), then enable a rule that keeps it clean | R-15 | S | Medium |
| A15 | Work the 322 `gopls` hints, after TQ-4 has put `staticcheck` in place so the list comes from the gate rather than from a sample | R-16 | M | Low |

### Workstream B: make the frontend readable and accessible

| # | Task | Findings | Effort | Impact |
|---|---|---|---|---|
| B1 | Add ESLint (flat config: `typescript-eslint`, `react-hooks`, `jsx-a11y`) plus Prettier, a `lint` script, and wire both into `mise run lint` and CI; then resolve or re-justify the five dead suppressions | F-1, C-1 | M | High |
| B2 | Extract a `<Modal>` primitive (a11y, focus trap, focus restore, Escape, `aria-modal`) and migrate the 31 overlay shells to it | F-7, F-8 | M | High |
| B3 | Introduce context providers for campaign, audio and settings; split `App.tsx`; extract the duplicated portrait effect; bring prop counts under 8 | F-2 | L | High |
| B4 | Split `WorldsStudio.tsx` and `SystemsStudio.tsx` into container plus feature views | F-3 | L | High |
| B5 | Decide and implement the DTO contract: generate `frontend/src/types.ts` from the Go DTOs, or add a manifest drift test modelled on `routes_test.go` | F-4 | L | High |
| B6 | Add progress-bar semantics, `aria-live` on the streaming turn and error banners, `aria-label` on icon-only buttons, Escape on the remaining modals | F-8 | M | High |
| B7 | Unify the API client: one error type, one calling convention, remove static/instance duplicates; move the stray `/api` fetches in `App.tsx`, `WorldsStudio.tsx`, `StoryTheater.tsx`, `CodexDrawer.tsx` and `editor/frontmatterSchema.ts` into `client.ts` | F-5, F-10 | M | Medium |
| B8 | Replace `any` and the `as unknown as` cast with `unknown` plus a shared `errorMessage(err)`; delete the duplicate helpers and the inline `slugify`; standardise on `function` declarations | F-6, F-11 | M | Medium |
| B9 | Introduce a small error-surfacing path so failures are visible rather than logged to the console | F-9 | M | Medium |
| B10 | Add a shared constants module for `localStorage` keys, file extensions and the display-mode union | F-10 | S | Low |

### Workstream C: close the test gaps

| # | Task | Findings | Effort | Impact |
|---|---|---|---|---|
| C1 | Add coverage floors to both suites, set just above the current values | T-1, T-3 | S | High |
| C2 | Add `gofmt -l` and `staticcheck` to `mise run lint` and CI | T-3, R-14 | S | High |
| C3 | Table-test every `cmd/localrpg` entry point after A12 | T-4, R-13 | M | High |
| C4 | Cover the 10 highest-value untested frontend components: `LauncherHub`, `App.tsx` turn flow, `CodexDrawer`, `SettingsStudio` panels, `UsagePanel`, `TTSBatchPanel`, `ExportModal`, `MenuBar`, `TurnSegments`, `NewEntityWizard` | T-2 | L | High |
| C5 | Cover loading, empty and error states, using `RegistryModal.test.tsx:83-121` as the pattern | U-3 | M | Medium |
| C6 | Add `vitest-axe` and assert a clean run in every component test, starting with the `<Modal>` primitive from B2 | F-8 | M | High |
| C7 | Add a `-race` CI job and a `windows-latest` / `macos-latest` matrix for platform-agnostic packages; add `-shuffle=on` | T-7, C-2 | M | Medium |
| C8 | Add `t.Parallel()` to stateless table tests and replace the 16 `time.Sleep` calls with polling | T-6 | M | Medium |
| C9 | Add test files for the seven untested packages, starting with the provider adapters | T-5 | M | Medium |

### Workstream D: UI/UX flow coverage

| # | Task | Findings | Effort | Impact |
|---|---|---|---|---|
| D1 | Add an e2e test for the primary journey: create a campaign, play a turn, assert narration and the chronicle | U-2 | M | High |
| D2 | Add an e2e test for `/undo`, asserting both the chronicle and the entity state revert | U-2 | M | High |
| D3 | Add an e2e test for the visible error path (rate limit or funds error), not the console path | U-2, F-9 | M | High |
| D4 | Add an e2e test that every drawer opens, closes on Escape, and restores focus | U-2, F-8 | M | High |
| D5 | Add an e2e test for content package import, asserting the progress panel and the resulting studio entry | U-2 | M | Medium |
| D6 | Add keyboard-interaction tests for `MenuBar` and the voice selectors | U-4 | S | Medium |

### Workstream E: documentation

| # | Task | Findings | Effort | Impact |
|---|---|---|---|---|
| E1 | Write `pkg/gui/docs/25-cli-reference.md`: every command, its flags, its exit codes | D-1 | M | High |
| E2 | Write a TUI guide for `localrpg play`: keybindings, `/undo`, `/gm`, inspection views | D-2 | M | High |
| E3 | Write a GUI navigation guide covering the launcher, campaign views and all drawers and settings panels | D-3 | M | High |
| E4 | Add a documentation index article and link `pkg/gui/docs` from the README | D-6 | S | Medium |
| E5 | Fix the duplicate `order` frontmatter in `07`, `11`, `12`, `13`, `08` and `24`, remembering that 11 to 13 are generated | D-5 | S | Medium |
| E6 | Add `CONTRIBUTING.md` and promote the architecture review into a living `docs/architecture/overview.md` | D-8 | M | High |
| E7 | Document Story Theatre, segment audio controls, and the portrait-generation workflow end to end | D-4 | M | Medium |
| E8 | Write the telemetry and observability guide | D-12 | S | Low |
| E9 | Re-measure or remove the stale Vale and article counts in `AGENTS.md`; add the missing commands to the CLI surface list | D-9 | S | Low |
| E10 | Emit `.md` cross-links, add subheadings to `19`/`22`/`23`, gloss jargon on first use | D-7, D-11 | S | Low |

### Workstream F: dependencies, CI and security

| # | Task | Findings | Effort | Impact |
|---|---|---|---|---|
| F1 | Add `govulncheck` to `mise.toml` and a `mise run vuln:scan` task, run on push, pull request and weekly | Z-4 | S | Medium |
| F2 | Add a Dependabot config for `gomod`, `npm` (`/frontend`) and `github-actions`, grouping the OpenTelemetry set | Z-3 | S | Medium |
| F3 | Bump the frontend quartet (`vite`, `typescript`, `@vitejs/plugin-react`, `vitest` plus coverage) together, then `lucide-react` | Z-2 | M | Medium |
| F4 | Bump Go dependencies in reviewable batches: OpenTelemetry as one, then the rest | Z-1 | M | Medium |
| F5 | Route every handler through `writeGameError`, return stable generic messages, log detail, use `errors.Is` | S-2 | M | Medium |
| F6 | Handle response-encoding errors instead of discarding them | S-3 | S | Medium |
| F7 | Add `frontend/coverage/` to `.gitignore` | S-4 | S | Low |

---

## 5. Suggested sequencing

**Wave 1: cheap, high-signal, unblocks the rest (about a day).**
A13 (`gofmt`), A14 (`interface{}` to `any`), A15 (the `gopls` hints), C2 (`gofmt -l` plus
`staticcheck` in CI), B1 (ESLint and Prettier), F7 (`.gitignore`), C1 (coverage floors),
E5 (docs order), E9 (stale counts). Every one of these is small, and together they turn
the treadmill on: the linters that arrive here are what keep the later refactors honest.

**Wave 2: comprehension (the largest single win).**
A1, A2, A3, A4, A5, A6, A9, A12. This is the work that makes a new contributor
productive: the god-files shrink, the packages explain themselves, the CLI becomes
testable, and the silent failure paths stop being silent.

**Wave 3: frontend quality and the contract.**
B2 (the `<Modal>` primitive, which unlocks the a11y work), B5 (the DTO contract), B1's
findings worked down, B7, B8. Then B3 and B4, which are large but straightforward once
the primitive and the contract exist.

**Wave 4: test coverage breadth.**
C3 (CLI tests, now possible after A12), C4, C6, D1 to D4. The e2e tests in D are the
only way the primary journey is ever verified, and they are worth prioritising over
further unit coverage.

**Wave 5: dependencies, docs and polish.**
Z-1 to Z-4, E1 to E4, E6 to E8, remaining R-12 functions, remaining low findings.

---

## Appendix A: metric baseline

All figures were produced on commit `78898e3e`, 2026-10-12, on Linux amd64 with Go
1.27.1 and the toolchain pinned in `mise.toml`.

```bash
# Go suite and coverage
go test -count=1 -covermode=atomic -coverprofile=/tmp/localrpg-cover.out ./...
go tool cover -func=/tmp/localrpg-cover.out | tail -1     # total: 73.3%
go tool cover -func=/tmp/localrpg-cover.out | awk '$3=="0.0%"' | wc -l  # 422

# Frontend suite and coverage
cd frontend && npx vitest run --coverage

# Go formatting
gofmt -l cmd pkg tools | wc -l                            # 65

# Idiom drift
rg -o 'interface\{\}' -g '*.go' --glob '!*_test.go' | wc -l   # 471

# Lint and static-hint counts
mise run lint

# Prose lint
mise run lint:prose                                       # 0 errors, 112 warnings, 178 suggestions, 26 files

# Dependency currency
go list -m -u -f '{{if and .Update (not .Indirect)}}{{.Path}} {{.Version}} -> {{.Update.Version}}{{end}}' all
cd frontend && npm outdated && npm audit
```

| Metric | Value |
|---|---|
| Go non-test files / lines | 373 / 76,248 |
| Go test files / lines | 475 / 57,629 |
| Go test:code ratio | 0.76 |
| Go test functions | 2,012 |
| Go statement coverage | 73.3% |
| Go functions at 0% coverage | 422 |
| Go files failing `gofmt` | 65 (30 with real diffs) |
| Go packages with no test file | 7 |
| `t.Parallel()` uses | 0 |
| `time.Sleep` in tests | 16 |
| Fuzz targets / benchmarks | 2 / 6 |
| Fuzz budget in CI | 10s per target |
| Frontend non-test files / lines | 215 / 33,103 |
| Frontend test files / lines | 69 / 4,523 |
| Frontend test:code ratio | 0.14 |
| Frontend tests | 339 |
| Frontend statement coverage | 33.36% |
| Frontend branch coverage | 30.09% |
| Frontend components / tested | 95 / 39 (41%) |
| e2e tests | 10 |
| `role="dialog"` surfaces / overlay shells | 3 / 31 |
| `console.*` call sites | 55 |
| `any` / `as` casts in frontend | 20 / 98 |
| `_ =` discards in Go (non-test) | 249 |
| `interface{}` in Go (non-test) | 471 across 109 files |
| `panic(` in Go libraries | 14 |
| `TODO`/`FIXME`/`HACK` | 0 |
| `//nolint` | 0 |
| Direct Go modules behind | 21 of 38 |
| Frontend majors behind | 4 (`typescript`, `vite`, `@vitejs/plugin-react`, `lucide-react`) |
| `npm audit` vulnerabilities | 0 |
| User-facing docs | 24 articles, about 19,800 words |
| Vale scope / alerts | 26 files / 290 (0 errors) |

## Appendix B: finding index

| ID | Severity | Title |
|---|---|---|
| R-1 | High | `pkg/gui/service.go` is 6,579 lines; `Service` has 165 exported methods |
| R-2 | High | `handleGameRoutes` is a 676-line function parsing the URL eight times |
| R-3 | High | `ProcessActionStream` is 980 lines across every abstraction |
| R-4 | Medium | `pkg/engine` and `pkg/storage` are named for what they are not |
| R-5 | Medium | Only 2 of about 30 packages have a package doc comment |
| R-6 | Medium | Exported symbols widely undocumented in the lower layers |
| R-7 | High | Errors discarded in the write path |
| R-8 | Medium | 12 unexplained `panic` calls in the JS host bindings |
| R-9 | Medium | Stringly-typed role and mode literals where constants exist |
| R-10 | Medium | Dead code and an unreachable branch |
| R-11 | Medium | `EntityFrontmatter`/`Entity` mirrored in four places |
| R-12 | Medium | Other long functions |
| R-13 | Medium | Inconsistent CLI command shapes |
| R-14 | Medium | 65 files fail `gofmt` |
| R-15 | Medium | 471 `interface{}` uses contradict the stated `any` convention |
| R-16 | Low | 322 modernisation hints, three of them real defects |
| F-1 | High | No TypeScript linter or formatter |
| F-2 | High | No state layer; about 45 `useState` in `App.tsx` |
| F-3 | High | Three god-files hold most of the UI |
| F-4 | High | Go/TS contract hand-maintained with no drift check |
| F-5 | Medium | About 140 endpoints return unvalidated JSON |
| F-6 | Medium | 20 `any`, 98 `as` casts |
| F-7 | Medium | No shared modal primitive; chrome copy-pasted 31 times |
| F-8 | High | Accessibility largely unaddressed and untested |
| F-9 | Medium | Failures swallowed to the console in 55 places |
| F-10 | Medium | Hardcoded routes, storage keys and defaults outside the client |
| F-11 | Low | Duplicated helpers and two component idioms |
| T-1 | High | Frontend statement coverage is 33.4% |
| T-2 | High | 56 of 95 components untested |
| T-3 | High | No coverage gate and no Go linter in either suite |
| T-4 | High | 422 Go functions at 0%, including every CLI entry point |
| T-5 | Medium | Seven packages with no test file |
| T-6 | Medium | No parallelism; 16 sleeps |
| T-7 | Medium | No race detector; ubuntu-only CI |
| U-1 | High | Ten e2e tests for the whole application |
| U-2 | High | The primary user journey has no e2e test |
| U-3 | Medium | Loading, empty and error states untested |
| U-4 | Medium | Implemented keyboard interactions untested |
| D-1 | High | No CLI reference; 9 of 13 commands undocumented |
| D-2 | High | The TUI is undocumented as a mode |
| D-3 | High | No GUI navigation guide |
| D-4 | Medium | Story Theatre, audio controls and portrait generation undocumented |
| D-5 | Medium | Duplicate `order` frontmatter breaks the docs sidebar sort |
| D-6 | Medium | No docs landing page and no README link |
| D-7 | Low | Cross-links do not resolve outside the app |
| D-8 | High | No contributor on-ramp |
| D-9 | Low | Stale counts and command lists in `AGENTS.md` |
| D-10 | Low | Vale configured for American spelling in a British-English repo |
| D-11 | Low | Long articles without internal structure |
| D-12 | Low | No telemetry or observability guide |
| Z-1 | Medium | 21 of 38 direct Go modules behind |
| Z-2 | Medium | Four frontend majors behind |
| Z-3 | Medium | No automated dependency updates |
| Z-4 | Medium | No Go vulnerability scanning |
| C-1 | High | CI has no Go linter, formatting check or coverage gate (CodeQL does run) |
| C-2 | Medium | CI tests only Linux while shipping five targets |
| C-3 | Low | Packaging runners do not run their target's tests |
| C-4 | High | `project.sh` clears board fields when a wave or area is added |
| C-5 | Medium | `project.sh` truncates an option name containing a comma |
| C-6 | n/a | CI strengths worth keeping (informational) |
| S-1 | n/a | Secrets discipline is sound (informational) |
| S-2 | Medium | Internal error strings returned to HTTP clients |
| S-3 | Medium | Response encoding errors discarded, producing truncated 200s |
| S-4 | Low | `frontend/coverage/` not gitignored |
| S-5 | n/a | `http.CrossOriginProtection` is the right choice (informational) |
