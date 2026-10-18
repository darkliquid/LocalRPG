# AGENTS.md

LocalRPG is a local-first, turn-based tabletop RPG client: a single Go binary that runs a Wails v3 desktop GUI, a Bubbletea terminal TUI, an HTTP/Unix-socket API daemon, plus media generation and story export. Module path: `github.com/darkliquid/localrpg`. The engine is **schema-agnostic** — no HP/Mana/classes are hardcoded anywhere; all RPG state is opaque YAML frontmatter plus sandboxed JavaScript hooks.

## Commands

Toolchain is pinned by `mise.toml` (Go 1.27.2, Node 26.9.0, Aube 2.7.2, GoReleaser 2.18.2). Prefer `mise run`, but plain `go`/`aube`/`npm` work if the toolchain is already active.

```bash
mise run setup          # go mod download + cd frontend && aube install
mise run install:vale-styles # vale sync: download the style packages when .vale.ini changes
mise run build          # frontend bundle -> pkg/gui/dist, then bin/localrpg
mise run build:frontend # npm run build in frontend/ (tsc + vite)
mise run build:backend  # depends on build:frontend
mise run test           # coverage-checked Go tests  AND  mise run test:frontend  AND  mise run test:e2e
mise run test:backend   # go test with a coverage floor and test cache reuse (scripts/check-coverage.sh)
mise run test:backend:fast # go test without the coverage check
mise run test:backend:clean # go test bypassing the test cache (-count=1)
mise run test:backend:race # go test with the race detector enabled
mise run test:backend:cross # go test across cross-platform packages (scripts/test-cross-platform.sh)
mise run coverage:diff  # compare two profiles by function, not by their statement totals
mise run test:frontend  # npx tsc --noEmit and npx vitest run --coverage (in frontend/)
mise run test:e2e       # go test -tags e2e ./pkg/e2e/... (skips without a browser)
mise run lint           # gofmt, staticcheck, go vet, Prettier, Oxlint, rumdl, goreleaser, actionlint
mise run lint:gofmt     # fail if any Go file is not gofmt-formatted
mise run lint:staticcheck # staticcheck over every Go package, tests included
mise run lint:frontend  # prettier --check and oxlint over frontend/
mise run lint:docs      # rumdl over the embedded help articles (pkg/gui/docs, .rumdl.toml)
mise run lint:prose     # Vale over tracked prose and source comments (report only; STRICT=1 to gate)
mise run lint:revive    # revive over the Go packages: undocumented exported symbols (report only)
mise run lint:goreleaser # goreleaser check
mise run lint:actions   # actionlint over .github/workflows
mise run secrets:scan   # gitleaks over the full git history and staged changes
mise run vuln:scan      # govulncheck over Go dependencies for known vulnerabilities
mise run site:build     # render the showcase site into website/dist
mise run site:serve     # build it and preview at http://localhost:4173
mise run site:screenshots # capture website/screenshots from a running build
mise run release:snapshot # local snapshot build for the current OS
mise run release:package # package archives for RELEASE_GOOS (one platform)
mise run release:cut     # bump (or set) the version, commit, tag and push with tags
mise run dev:gui        # go run ./cmd/localrpg gui --port 8080
mise run dev:frontend   # vite dev server on :3000, proxies /api -> localhost:8080
mise run clean
```

Run a single Go test: `go test -run TestTurnOrchestrator ./pkg/engine/`.

Regenerate the generated embedded docs (the index, provider catalogue, config reference)
after changing a provider, preset or config struct, and after adding, renaming or
re-describing any guide article, since the index lists them all:
`go test ./pkg/gui -update-docs`.

CLI surface (`localrpg <cmd>`): `roll <notation>`, `prompt`, `play <game-id>`, `tts`, `image`, `gui`, `export <web|video>`, `debug <test-run|server>`, `config <cmd>`, `version`.

## Working the project

Development work is tracked in **Beads** (`bd`), a local, dependency-aware issue tracker backed by
Dolt. Its database lives in `.beads/`, and its id prefix is `LocalRPG`. GitHub hosts the repository
and reviews pull requests, and is used for nothing else: no issue tracker, no milestones, no project
board.

`bd` is deliberately not pinned in `mise.toml`: `jdx/mise-action` installs every pinned tool in all
five CI jobs, and no build should download a 150 MB tracker it never runs. Install it once, with
`mise use -g aqua:gastownhall/beads` or `go install github.com/gastownhall/beads/cmd/bd@latest`.

The tracker keeps the three levels the work has always had:

- **Wave** - an ordering, carried as a `wave/<n>` label. Waves are dependency-ordered; work in the
  earliest incomplete wave first.
- **Epic** - a bead of type `epic`, one per theme, labelled `area/<theme>`, with the theme's
  proposals as its children.
- **Proposal** - one bead per unit of work, labelled `area/*`, `wave/*`, `effort/*` and the `type/*`
  label the proposal was written under, with its dependencies as graph edges rather than prose.

A bead moves through `open`, `in_progress`, `blocked`, `deferred` and `closed`, and the movement is
the command rather than a field you edit by hand:

| Command | Means |
|---|---|
| `bd ready` | the claimable frontier: open, unblocked, in priority order |
| `bd update <id> --claim` | taken, and `in_progress` |
| `bd dep add <id> <blocker>` | this work cannot start until that one is done |
| `bd close <id> --reason "..."` | complete; the reason is the historical record |

Only `blocks` edges affect `bd ready`, so a real ordering constraint is a `blocks` dependency and
nothing else. Provenance and soft links (`discovered-from`, `related`) are recorded without holding
work back.

**Spec-first.** A proposal is specced and planned before it is implemented: the spec is a design doc
in `docs/superpowers/specs/<date>-<slug>-design.md`, and the plan is a task-by-task implementation
plan in `docs/superpowers/plans/<date>-<slug>.md`, following the superpowers workflow. Read the
relevant spec before changing a subsystem.

A proposal that decides an interface, a behaviour, or a tradeoff is specced and planned. Work that is
mechanical decides nothing and carries no design step, because the spec would only restate the issue
and nobody would read it: a formatting sweep, a rename, a dependency bump, a test that mirrors an
existing one. The bead's type says which it is, so `feature` and `spike`, and a `task` that is a
refactor, usually decide something, while `chore` and a test usually do not. When a bead does decide
something its description names the spec and plan; when it does not, its checklist is implemented,
tests and lint, and user-facing docs.

### The `bd` loop

```bash
bd prime                       # workflow context, at the start of a session
bd ready                       # what can be started now
bd show <id>                   # the full description, dependencies and acceptance criteria
bd update <id> --claim         # take it
# write the spec and plan, implement, run the gates, then:
bd close <id> --reason "Done: <summary>"
```

Creating work:

```bash
bd create "Repair the X path" -t task -p 1 -l "area/architecture,wave/7,effort/m" \
    -d "..." --acceptance "Implemented; mise run test and mise run lint pass" \
    --external-ref gh-216
bd q "A bug found while working" -t bug -p 2      # capture now, triage later
bd dep add <new-id> <current-id> --type discovered-from
```

`bd create` takes `--parent <epic-id>` for a child, `--deps blocks:<id>` for a blocker, and
`--external-ref gh-<n>` where a bead came from a GitHub issue. Use `--parent` and `bd dep add` rather
than writing "part of" or "depends on" in the description, so `bd ready` can compute the frontier.
`bd q` returns a bare id for scripting.

Waves, areas and efforts are open-ended: a new wave is a new `wave/<n>` label and a new theme a new
`area/<slug>` label. Nothing enumerates them, so there is no registry to update.

The beads in this database were migrated from the GitHub Project in October 2026, which is why each
one carries an `--external-ref gh-<n>` pointing at the closed GitHub issue it came from, and why the
`wave/*` and `area/*` labels match the old board's. The board itself, and the `mise run project:*`
tasks that drove it, are gone.

### Syncing

Beads data is Dolt, kept under the git ref `refs/dolt/data`, independent of code commits, so
`bd dolt push` and `bd dolt pull` move issues without touching the working tree. Normal writes
auto-commit; a dirty working set complains until `bd dolt commit -m "..."` clears it.

## CI, releases and the showcase site

Three workflows live in `.github/workflows/`. `ci.yml` uses `dorny/paths-filter` to inspect changes at the start of a pull request, skipping unchanged test suites while running the complete matrix unconditionally on `main` and release tags. Frontend setup uses `jdx/aube-action` with content-addressable `node_modules` caching, and Go test caching is preserved across runs. Jobs include Go (native headers, frontend build, `go vet`, `go test`), race detector, cross-platform tests (macOS and Windows), browser e2e tests, a web job (`tsc --noEmit`, vitest, oxlint, rumdl, Vale), and a packaging job (`goreleaser check`, `mise run site:build`, plus the built site as an artifact). It also declares `workflow_call`, so `release.yml` reuses it as a `verify` job and a tag can only ship what passed.

`release.yml` runs on `v*` tags and published releases. Every platform needs CGO, so no runner can cross-compile the matrix: `.goreleaser.yaml` defines one build per target (OS plus architecture), each with a `skip` template that disables it unless `RELEASE_TARGET` names its target. The matrix runs one native runner per target — `ubuntu-latest` and `ubuntu-24.04-arm` for the two Linux architectures (a CGO build for arm64 cannot come from an amd64 host), `macos-latest` twice for both macOS slices, and `windows-latest` — uploads archives, and a final job combines the checksums and publishes or enriches the release with `gh`. GoReleaser's own split/merge is a Pro feature, which is why publishing is done with `gh`. The arm64 Linux runner is a hosted runner, free only for public repositories; on a private repo, drop that matrix entry. `Version` in `cmd/localrpg/main.go` is a `var` so `-X main.Version` can stamp the tag.

`pages.yml` builds the showcase site and deploys it to GitHub Pages. The site is generated by `tools/sitegen`, a small Go program that renders `README.md` and `pkg/gui/docs/*.md` into `website/dist` using the same glassmorphic styling as the app (the palette and blur values are copied from `frontend/src/index.css`). Nothing about the site is hand-maintained prose: it reads the documentation the binary embeds, so the two cannot drift. `mise run site:serve` builds it and previews it at `http://localhost:4173`. `website/screenshots/` holds the gallery images (see its README); `website/demo/` is the fixture campaign the capture task plays against. GitHub Pages must be enabled with "GitHub Actions" as the source.

`secrets.yml` scans for credentials on every push and pull request, and weekly so newly published detection rules are applied to history that has not moved. `vuln.yml` scans Go module dependencies for known vulnerabilities on the same schedule using `govulncheck`.

## Secrets

`mise run secrets:scan` runs gitleaks over the full git history and over staged changes; `.github/workflows/secrets.yml` runs the same task in CI. `.gitleaks.toml` *extends* the default ruleset (a config without `useDefault` silently disables every shipped rule) and allowlists only the two fake credentials the `pkg/trace` redaction tests need.

Anything that reaches a commit is public the moment it is pushed, so scanning is a backstop, not the control. The real protections are:

- **Enable secret scanning and push protection** in the repository's security settings. Push protection rejects a push containing a recognised credential before it ever lands in history. Both are free on public repositories.
- **Never commit provider credentials.** `/localrpg.yaml`, `/config.yaml` and `.env` are gitignored because the app writes API keys into a workspace override when it runs in project mode. The documented form is `api_key: "env:OPENAI_API_KEY"`, which keeps the value out of the file entirely.
- **Screenshots leak too.** `website/screenshots/` is committed, and a shot of the Settings Studio or a provider panel can show a key. Look at them before committing.

If a key does reach a commit, rotate it first: rewriting history does not un-publish it.

## Vulnerabilities

`mise run vuln:scan` runs `govulncheck ./...` over Go module dependencies and standard library usage; `.github/workflows/vuln.yml` runs the same check in CI on push, on pull request, and weekly so new advisories are caught even if code has not changed.

## Known advisories

`npm audit` in `frontend/` reports **nothing**. Two things keep it that way: the
documentation linter is a Rust binary rather than a Node package, and one
transitive dependency is pinned by an override.

**The docs lint is `rumdl`, not a Node linter.** `mise run lint:docs` runs
`rumdl check pkg/gui/docs`, with rumdl pinned in `mise.toml` like `gitleaks`,
`actionlint` and `vale`. rumdl is a Rust reimplementation of markdownlint's rules
that keeps the same numbering, so `.rumdl.toml` reads like the markdownlint config
it replaced. This is deliberate: every Node-based markdown linter dragged an
advisory in with it. `markdownlint-cli2` brought `braces` (stack-exhaustion denial
of service through deeply nested patterns, GHSA-vfj7-8cjw-p6xm, CVE-2026-93687)
via `globby` → `micromatch` → `braces`, with no patched release and a suggested
remedy of downgrading twenty-three minor versions; `markdownlint-cli` avoided
`braces` but pinned `js-yaml ~5.2.1`, inside a different advisory's range. The
engine itself then turned out to reach a vulnerable `katex` through
`micromark-extension-math`. A native binary has none of that surface, and
`rumdl check` reports the same result on the corpus: no issues in 25 files.

**Do not add a Node markdown linter back.** If the docs ever need a rule the config
cannot express, add it to `.rumdl.toml`, which is where the three deviations from
the default ruleset live with their explanations. rumdl adds rules of its own
(MD057, MD061-MD094) and is still 0.x, so re-check the corpus after a version bump.

One advisory does remain in the tree, held down by an `overrides` block in
`frontend/package.json`:

- **`source-map-js`** (GHSA-68fv-2mgg-jv7q, event-loop denial of service through
  indexed source-map section offsets), reached through `postcss`,
  `@tailwindcss/node`, `css-tree` and `magicast`. The override forces `^1.2.2`, a
  patch-level fix that is API-compatible with every consumer, so it is a version
  floor rather than a fork.

No CI job runs `npm audit`, so a new advisory would not gate a build. If one is
added, it should start from a clean tree.

## Automated dependency updates (Dependabot)

`.github/dependabot.yml` configures Dependabot to check weekly on Mondays at 06:00 UTC across three ecosystems:

- **Go modules (`gomod`)**: checks root `go.mod` dependencies. The OpenTelemetry suite (`go.opentelemetry.io/*` and `github.com/XSAM/otelsql`) is grouped into a single update (`opentelemetry`) so coordinated instrumentation and SDK releases arrive in one combined pull request.
- **Frontend packages (`npm`)**: checks `frontend/package.json` for npm package updates.
- **GitHub Actions (`github-actions`)**: checks workflow action references in `.github/workflows/`.

## Prose linting with Vale

Vale checks prose style in the user-facing documentation only. It is pinned in
`mise.toml` like every other tool, and `.vale.ini` at the repository root decides
the styles.

**Scope is 27 files**, and it is the same set `tools/sitegen/content.go` renders
into the showcase site:

- `pkg/gui/docs/*.md` - the 25 guide articles the application embeds, and the bulk
  of the user-facing prose.
- `README.md` - the project README.
- `docs/debugging.md` - the debugging guide.

Everything else is internal and is not linted: the design specs and plans under
`docs/superpowers/`, `docs/proposals/`, `docs/architecture/`, `AGENTS.md`,
`THIRD_PARTY_NOTICES.md`, the `website/demo/` fixtures, and every Go and TypeScript
comment. A page under `pkg/gui/docs/` needs no registration, because both the site
generator and the lint script read that directory; a page anywhere else must be
added to `tools/sitegen/content.go` and to the script's file list.

```bash
mise run lint:prose            # full report, every finding (syncs the Vale styles first)
SUMMARY=1 mise run lint:prose  # counts, noisiest rules, worst files
STRICT=1 mise run lint:prose   # exit non-zero on error-level alerts
```

**Run it through mise, not the script.** `mise run lint:prose` depends on
`install:vale-styles`, which runs `vale sync`. `bash scripts/lint-prose.sh` skips that, and
Vale then checks against whatever is in `styles/`: nothing in a fresh clone, and a stale
set in a tree from an earlier session. Both report fewer findings than CI and both look
green, which is how a change passes locally and fails the prose job it is named after.
Sync the styles first if a direct run is genuinely wanted.

### What it reports today

**316 alerts across all 27 files: 0 errors, 131 warnings and 185 suggestions.**
For scale, pointing the same styles at every tracked file reported 68,525 alerts
and 6,875 errors, which is why the scope is the documentation rather than the
repository.

The generated `00-start-here.md` index carries a copy of every other article's
description, so a description that trips an error-level rule fails the build in two
places. That is deliberate: a description is user-facing, in the viewer's search and
sidebar, and it is better held to the same rules as the prose.

The noisiest rules are `Google.Passive` (109), `neighbor.AmpersandInProse` (75),
`Google.Contractions` (42) and `Google.Semicolons` (24), and the noisiest files are
`23-ai-world-generation.md` (44), `04-campaigns.md` (30) and `22-editing-content.md` (28).
Re-run `SUMMARY=1 mise run lint:prose` rather than trusting these figures, which drift
with every new article.

**CI gates on the error level.** `STRICT=1` makes the task exit non-zero when an
error is reported, and the workflow runs it that way. Warnings and suggestions are
printed but never fail a build, because they are style preferences rather than
mistakes. Run `mise run lint:prose` for the full report, or `SUMMARY=1` for the
counts and the noisiest rules.

Reaching zero took three passes: narrowing the scope to the documentation, adding
the vocabulary, and then rewriting the prose the remaining rules objected to. A
fourth pass then worked down the warnings and suggestions, which took the report
from 293 alerts to 149. Two later articles, the on-disk and package formats guide and
the AI world generation guide, added 41 and 44, and the style packs article and a
correction to the counts took it to 290. Fix the number from the tool rather than
from this paragraph, which is a history rather than a reading.

Everything left is deliberate. `neighbor.AmpersandInProse` (75) fires on `&` in
headings and bolded feature labels, which is a design convention rather than prose.
`Google.Passive` (109) and `Google.Semicolons` (24) are style preferences, and
passive voice and semicolons are both correct in technical writing.
`neighbor.DeviceSpecificAction` (6) objects to "click", which is the real action in
a desktop app. `neighbor.DirectionalLanguage` (3) flags "progress bar" and "prompt
bar", which are widget names rather than layout instructions, and `Google.FirstPerson`
(1) fires on "my guild swore an oath", which is a quoted player utterance.

### The vocabulary

`styles/config/vocabularies/LocalRPG/accept.txt` lists the words Vale's dictionary
does not know: product names, the acronyms this project writes in prose, and domain
vocabulary. It is **committed**, and `.gitignore` carries a ladder that ignores the
downloaded styles while keeping this file.

Adding a word here is always better than rewording a correct sentence. This one file
removed 188 errors, which was 82% of them.

The vocabulary also produces a `Vale.LocalRPG.Terms` rule demanding one
capitalisation per word, and that rule is **switched off**: it fired on the provider
catalogue's `http` and `cli` table values, on `Frontmatter` in a YAML `title:` and
an H1, and on a bolded `**Config (...)**` label. All correct as written. Both cases
of a word are listed in the vocabulary wherever the documentation uses both.

### Rules switched off, and why

Twenty-one rules are off in `.vale.ini`, grouped by reason. Each one fires on correct,
deliberate writing rather than on a mistake:

- **Readability grade scores** (`Polysyllables`, `FleschReadingEase`, `FleschKincaid`,
  `ColemanLiau`, `SMOG`, `LIX`, `GunningFog`, `AutomatedReadability`). They measure
  word and sentence complexity, which a guide about local language models and audio
  pipelines has by nature. Satisfying them means writing around the vocabulary the
  reader came to learn.
- **Colon usage** (`Google.Colons`, `ai-tells.ColonUsage`). This documentation is
  built on definition lists and labelled steps, so the rule objects to the format.
- **`Google.Acronyms`.** Spelling out TTS, STT, LLM, MCP and CLI at every first use
  is what the vocabulary file exists to avoid.
- **`Google.Parens`.** Parentheticals carry asides and unit conversions here.
- **`write-good.E-Prime`.** Bans the verb "to be" outright, which is impossible in
  technical English.
- **`neighbor.AllCapsProse`.** The acronyms are correct; renaming them would break
  every cross-reference.
- **`Google.Headings`.** The documentation uses Title Case throughout and the
  showcase site's design is built on it. This is the one suppression that is a house
  style rather than a property of technical writing, so revisit it if the headings
  are ever re-cased.

Four `ai-tells` rules are off as well, and the rest of that style is on and clean:

- **`ai-tells.VerbTricolon`.** It looks for a rhetorical tricolon but matches any
  list of three, which is ordinary English and everywhere in a guide. It fired on
  "system identity, action modes, and metadata" and on "ComfyUI, Automatic1111,
  LocalAI" - three servers, not a figure of speech.
- **`ai-tells.SemicolonUsage`.** Semicolons joining related clauses are correct, and
  this repository's own convention prefers them to em dashes.
- **`ai-tells.EmDashUsage`.** It reports "em-dash detected" for U+2013, and 15 of its
  16 findings were en dashes in ranges such as `4-6 GB` and `16-24 GB`, where an en
  dash is the right character.
- **`proselint.Annotations`.** It sees `[!NOTE]` and reports a note left in the text,
  but that is live syntax: remark-github-blockquote-alert renders it as a callout in
  the app and on the site.

Two more report something that is not a fault:

- **`write-good.Passive`.** It and `Google.Passive` check for the same thing and
  agree on every instance, so the same 66 findings were reported twice. Google's
  wording is the more specific, so write-good's copy is the one switched off.
- **`neighbor.ExclusiveLanguage`.** It flags "Master" as non-inclusive, but every
  instance is "Game Master" - the standard tabletop term for the role, and the name
  of a role in this engine's own configuration. Renaming it would break the domain
  vocabulary, the documentation and the config keys together.

One contradiction is worth knowing about: **`Google.Latin` demands "for example" in
place of "e.g.", and `ai-tells.FormalTransitions` objects to "for example".** Both
are satisfied by writing "such as", which is what the documentation now does.

### Two things that will bite

**Vale does not read `.gitignore`.** It walks every file its configuration has a
section for, so `vale .` would descend into `node_modules`, `bin` and the generated
site. `scripts/lint-prose.sh` asks `git ls-files` for the three paths instead.

**A `.vale.ini` `Packages` entry is a name, a URL, a zip path, or a directory path.
There is no `Name.URL` form.** `neighbor. https://.../ai-tells.zip` reads like one
package with a URL, but it parses as a single malformed entry, `vale sync` stops with
exit 2, and nothing after it installs. Separate entries with commas.

**The `install:vale-styles` task writes `outputs = ["styles/Google"]`, not
`["styles"]`.** The committed vocabulary means `styles/` exists in a fresh clone, so
keying the task on that directory would make mise treat the download as already done
and Vale would run with no styles at all.

## Undocumented exported symbols (revive)

`mise run lint:revive` runs `revive` over every Go package with exactly one check
enabled, `exported`, which asks for a doc comment on each exported type, function,
method and constant block. `revive` is pinned in `mise.toml` like every other tool and
configured by the checked-in `revive.toml`, which carries the reason for each choice —
the stutter check is off, because it asks for `entity.Frontmatter` where the type's name
is the Markdown block the file defines.

**It is a gate**, run by `mise run lint` under `STRICT=1`, because the count
is still large: **271 findings**, down from 336 now that `pkg/state`, `pkg/entity`,
`pkg/storage`, `pkg/rules` and `pkg/engine` are documented (RD-5). `STRICT=1` makes it
exit non-zero, which is how it becomes a gate once the count is near zero; the
remaining packages, `pkg/gui` (65) and `pkg/provider` (51) first, are their own bead.

A doc comment here answers what the symbol is for and the invariant a caller cannot see
from the signature — that `state` paths are dot-separated, that `state.Raw` is a shallow
copy, that a `storage.Store` a `Pool` handed out closes as a no-op. A comment that
restates the signature is worse than none.

```bash
mise run lint:revive            # print every finding, exit 0
STRICT=1 mise run lint:revive   # exit non-zero on findings
mise run lint:revive | grep -c '^pkg/gui/'   # one package's share
```

## Static analysis (CodeQL)

CodeQL runs in **advanced setup**: `.github/workflows/codeql.yml` analyses the three languages default setup used to (Go, the JavaScript/TypeScript frontend, and the workflow files themselves), against `.github/codeql/codeql-config.yml`. It was default setup until the configuration mattered: default setup reads a config file only when the repository carries the `github-codeql-config-file` property, custom properties are an organisation feature, and this repository is owned by a user, so default setup could only be told which query suite to run. The analysis is the same one either way; only the configuration is now ours. Findings are older than whoever is reading them: **28 were open on `main`** when the quality phase started, unactioned, because nothing in the repository pointed at them.

**`go/path-injection` is excluded, deliberately.** Every filesystem sink here takes a path the user or a content package supplied, so the query reports the shape of the program: exporting where the user asked, importing the folder they named, serving an asset under the campaign's own directory. Its forty alerts were read and dismissed one by one as contained by a `pkg/pathutil` sanitiser or as a user-chosen destination, and not one was a defect. The exclusion is configuration rather than forty dismissals because a dismissal is attached to a line: any refactor that moves the code re-raises it. The RD-2 route split did exactly that to the game and world asset sinks, both already dismissed once. `go/zipslip` still runs and still covers archive extraction, which is where an imported package is genuinely untrusted, and `pathutil.ValidateID`, `SanitizeID` and `ResolveSafeChild` remain the control everywhere else. Revisit this if the application grows a remote or multi-user surface.

**Model packs would be the better fix, and cannot be used yet.** Modelling those three sanitisers as Go barrier and barrier-guard models clears the false positives and keeps the query: the CodeQL CLI honours them and takes this repository from 127 results to 113 in a local analysis, which is how the claim was checked. GitHub code scanning does not, because model packs are in preview for C/C++, C#, Java/Kotlin, Python, Ruby and Rust, and not for Go, so a pack under `.github/codeql/extensions` would be inert here. Re-check that list before reaching for an exclusion on another query.

**A guard only counts if it is on the value the sink receives.** Two rules, and the second is the one that costs the most time:

1. **It has to be visible.** CodeQL models a sanitiser it can see at the sink; a call to a function that performs the same check is invisible to it, however correct the function is. `pathutil.ResolveSafeChild` and `pathutil.ValidateID` are both correct and neither clears an alert on its own.
2. **It has to be on the used value, not a copy of it.** Checking a parsed URL, or a lower-cased string, leaves the original unsanitised, and `url.Parse` carries the taint into the fields of its result besides. Guard the variable the call actually passes.

An outbound URL took **three shapes** before it cleared, and the first two were both plausible: a correct helper, then a scheme check on the parsed URL, then an allow-list on a lower-cased copy. The one that worked was `strings.HasPrefix` on the URL itself, with that same variable passed to the request. Spell the guard out where the tainted value is used, and keep the helper behind it for the narrower rule:

| The taint | What to inline at the sink |
|---|---|
| A single path component (an id, a filename, a checksum) | reject a separator and a `..` segment, with `strings.ContainsAny` and `strings.Contains` |
| A path that may have components | `filepath.Abs(base)`, then `strings.HasPrefix(target, abs + os.PathSeparator)` |
| A URL for an outbound request | `url.Parse`, then an inline scheme allow-list of `http` and `https` |

Two fixes in one commit demonstrated it: the archive extractors inlined the containment check and their alerts cleared, while `GetLocationArt` called `pathutil.ValidateID` and its alert stayed open. **Treat a still-red alert as a shape problem before concluding it is a false positive**, and confirm by re-reading the alert rather than trusting the merge: a fixed flow shows up again at its new line numbers if the guard was invisible.

**Dismissals are for what is genuinely by design or already contained**, and the mechanics matter as much as the judgement:

- A dismissal comment is capped at **280 characters**. The API rejects a longer one outright, with `Only 280 characters are allowed`, rather than truncating it.
- This repository **rejects `false positive` as a dismissal reason**, so a correct-but-invisible guard is recorded as `won't fix` with the explanation in the comment.
- **Alerts carry over to a large pull request.** The scanner says so itself: "Alerts not introduced by this pull request might have been detected because the code changes were too large". Ten of the fourteen alerts a refactor appeared to introduce were in a file it never touched. Check the alert's path against the diff, and check whether the same alert is already open on `main`, before believing it is yours.

**A passing CodeQL check on a pull request does not mean the alert is cleared.** The check answers "did this change add an alert", not "are there any". A flow that already exists on `main` stays open however the pull request rewrites it, and the check goes green either way. Confirm with `gh api repos/darkliquid/LocalRPG/code-scanning/alerts?state=open` and read the alert's instance commit: if it predates the merge, the analysis has not run yet.

**Some alerts are answered by a decision rather than a shape.** Four shapes failed to clear one outbound URL, and the alert was finally resolved by recording it as a design tradeoff: the scheme is restricted to http and https, and the host is deliberately open because a registry on localhost is supported for a local-first app, so the residual risk requires the user to have added a hostile registry. Write the reasoning into the dismissal, and name the hardening if there is one.

Read them with `gh api repos/darkliquid/LocalRPG/code-scanning/alerts?state=open`, and **read the error when a write to them fails**: three dismissal batches were lost to an unread 280-character rejection, and the API had said so each time.

**`gh api -f` does not read a file.** `-f body=@file.md` sends the literal string `@file.md`, because only `-F`/`--field` expands `@`; `gh pr edit --body-file` and `gh issue edit --body-file` are clearer still. Four pull requests were opened here with a body of `@/tmp/...`, and nothing catches it: the API accepts the string, the checks go green, and the description is only wrong when someone reads it. Read the body back after writing it.

## Build gotcha: the frontend is embedded in the Go binary

`pkg/gui/assets.go` declares `//go:embed all:dist`, so **`go build` fails if `pkg/gui/dist/` does not exist**. It is gitignored except for a tracked `.gitkeep` placeholder. Vite writes straight into `pkg/gui/dist` (`frontend/vite.config.ts` sets `outDir: ../pkg/gui/dist`, `emptyOutDir: true`), which would delete that `.gitkeep`; the build tasks in `mise.toml` and `frontend/package.json` automatically touch `.gitkeep` immediately after building so `git status` stays clean.

Always build the frontend before the backend, or use `mise run build`, which enforces the ordering. `pkg/gui/assets.go` also falls back to `frontend/dist/` and `pkg/gui/dist/` on local disk, then to a placeholder HTML page, for backend-only development.

## Architecture

Three-tier on-disk separation, resolved through `core.PathResolver` (`pkg/core/types.go`). A campaign has exactly one database, `games/<id>/cache/index.db`, and it is only ever opened through `storage.OpenGameStore(paths, gameID)` — which resolves `PathResolver.GameDBPath`, retires any legacy `game.db` to `game.db.legacy`, and returns a pooled handle (`storage.Pool`, `shared` stores whose `Close` is a no-op; `Pool.Close` owns their lifetime). Do not call `storage.NewStore` for a game.

- `systems/<id>/` — mechanics: `system.yaml`, `mechanics.js`, `prompts/rules.md`
- `worlds/<id>/` — lore: `world.yaml`, `prompts/lore.md`, `entities/*.md`, `system_overrides/<system-id>/hooks.js`
- `games/<id>/` — one campaign: `game.yaml`, `history.jsonl`, `entities/*.md`, `assets/`, `cache/index.db`

`systems/`, `worlds/`, and `games/` are empty in the repo; there is no shipped sample content. All content is created through the GUI studios or by hand.

Turn lifecycle: `cmd/localrpg/play.go` wires `storage.Store` + `engine.Timeline` + `rules.JSEngine` + `harness.Router` into `engine.TurnOrchestrator`. `ProcessAction` (`pkg/engine/orchestrator.go`) loads history, handles `/undo` and `/gm <directive>`, evaluates `Roll`-mode dice or JS action hooks, assembles a 4-layer context prompt, calls `router.GenerateForRole(ctx, "gm", …)`, resolves entity mentions and dialogue segments, then hands the turn to `Timeline.RecordTurn`, which owns every write (see below). `onTurnEnd` hooks run last.

Context prompt layering lives in `pkg/harness/context.go:ContextAssembler.Assemble(ContextRequest)`: system rules, world lore, voice-profile catalog, then scene scope, living-world arcs, present characters, and the player action (entities reachable from the current location via `edges`).

The opening location is never hardcoded: `engine.ResolveStartLocation` (`pkg/engine/startlocation.go`) prefers a pinned `settings.start_location`, then the player's own `location`/wikilink reference, then any indexed `location`, and only then derives a location from the world manifest's name and description into `games/<id>/entities/opening-scene.md`. `engine.InitGame` resolves once at campaign creation and records the result in `game.yaml`; `localrpg play` re-resolves and reindexes the game's `entities/` directory at startup so it picks up hand edits.

Extracted entities are reconciled before they are written: `harness.MatchExistingEntity` (`pkg/harness/extractor.go`) matches by exact ID, then name (including partial token overlap such as "Evelyn" vs "Lady Evelyn"), then context (same type plus a shared location and role tag). Matches update the existing entity, preserving its ID, authored `voice`/`state`, and file hash; only genuinely new entities are created, named `<id>.md` from their slugified name.

`engine.Timeline` (`pkg/engine/timeline.go`) is the single writer across the three layers a turn touches. `RecordTurn` stages entity notes, appends the turn to `history.jsonl`, then indexes it; notes are written before the log so a record never points at a note that does not exist, and an index failure is repaired later by `EnsureIndexed`. `RewindToTurn` (used by `/undo`) trims the log, deletes the indexed turns, and prunes entity `history` numbers while leaving entity prose alone. `EnsureIndexed` replays `history.jsonl` into the database whenever the two disagree, and runs at campaign creation and at `localrpg play` startup.

Memory model: entities are Markdown files with YAML frontmatter; `[[wikilinks]]` in the body plus the frontmatter `location`/`faction` fields become graph edges on parse (`pkg/entity/entity.go`). `storage.Syncer.Sync` hashes file contents and upserts into SQLite (`modernc.org/sqlite`, no CGO) — `cache/index.db` is disposable and rebuildable from the Markdown. `history.jsonl` is the canonical timeline and append-only; each record carries the number, the player's raw prompt, the narrator's rewrite, the entities involved (`entity.Mention` with a `player`/`location`/`wikilink`/`extracted`/`speech` kind), and ordered narration/speech `entity.TurnSegment`s whose speakers are resolved to entity IDs. The index mirrors it as `turns` + `turn_entities`; `/undo` rewrites the log below the target turn number.

GUI: `pkg/gui.Service` (`service.go`) is the entire API surface; `pkg/gui/server.go` maps it to `/api/...` routes and serves the embedded SPA with client-side-routing fallback. `frontend/src/api/client.ts` is the only fetch layer and mirrors those routes — when adding an endpoint, update `Service`, `server.go`, `frontend/src/types.ts`, and `client.ts` together. A failure the player can act on is reported through `useFailures` (`frontend/src/context/FailuresContext.tsx`), which renders one region near the top of the app; a component's own form error stays beside its field, and a background or already-surfaced failure keeps its console line with a comment saying why. The desktop app plays turns through `POST /api/game/{id}/turn`, which streams newline-delimited JSON; turns are serialised per campaign by `Service.BeginTurn` (409 while one is in flight); nothing is persisted for a cancelled or disconnected turn; and provider setup lives in `pkg/harness.RouterFromConfig`/`ExtractorFromConfig` rather than in `package main`.

Runtime modes in `cmd/localrpg/gui.go`: default is a native Wails v3 window (zero TCP); if `$DISPLAY`/`$WAYLAND_DISPLAY` are unset, or `--headless`/`--socket` is passed, it serves the same handler over a `0600` Unix socket (`pkg/gui/socket.go`, unlinks stale sockets); `--port N` opts into `127.0.0.1:N` TCP for a browser.

## Provider model (uniform across LLMs and media)

Every AI/LLM and media backend (LLM agents, TTS, STT, image) uses the same config shape: `type` is one of `builtin`, `cli`, `http`, `disabled` (plus `mock` for LLMs), with `builtin_name` selecting an in-process implementation. Factories: `harness.NewModelProvider` (`pkg/harness/factory.go`), `media.NewTTSClient`/`NewSTTClient`/`NewImageClient` (`pkg/media/providers.go`).

Notable built-ins that need no server or GPU: `narrative-oracle` (LLM), `native-os` (TTS via `spd-say`/`say`/PowerShell), `procedural-art` (pure-Go SVG image generator). `harness.Router` maps role name -> provider ID with optional per-role fallbacks; roles are arbitrary strings ("gm" is the only one the orchestrator hardcodes; "narrator" exists only as a config default).

## Configuration

Resolution order (`pkg/config/manager.go`): `$LOCALRPG_CONFIG_DIR`, else the XDG config search path (`$XDG_CONFIG_HOME` then `$XDG_CONFIG_DIRS`), for `config.yaml`; then an optional `./localrpg.yaml` merged on top, which flips `IsLocalOverride`. `Save` writes to the local override when one exists, otherwise to the user config. `gui.NewService(rootDir)` has its own twist: when `--dir` is set to anything other than `.`, it treats `<rootDir>/config.yaml` as the user config and resolves relative `paths.*` against `rootDir`.

Storage paths are resolved by `pkg/paths.Resolve` from the XDG bases (`github.com/adrg/xdg`), which fall back to native per-OS directories: `systems`/`worlds`/`games` under `DataHome/localrpg`, `cache` under `CacheHome/localrpg`. An empty `paths.*` uses those defaults, an absolute value is used verbatim, and a relative value joins the category base — unless `--dir` or `./localrpg.yaml` puts the process in project mode, where relative values resolve against the project root (the previous behaviour). Nothing is migrated; the GUI logs a `paths.legacy_relative` warning when a legacy working-directory folder exists and the resolved XDG directory is empty.

## Testing

Two suites, one runner each. `mise run test` runs both plus the backend.

**Both suites have a coverage floor, and neither is a target.** Go statements must stay at or above `72.0`, checked by `scripts/check-coverage.sh`, which runs the suite with one profile and fails below the number (`GO_COVER_MIN` overrides it, `GO_COVER_KEEP` saves the profile). The frontend floors live in `frontend/vitest.config.ts` and are enforced by `vitest run --coverage` in `mise run test:frontend`: statements `33`, lines `34`, functions `33`, branches `30`.

The floors sit just below what the suite reaches today — 73.3% and 33.36% at the time of writing — rather than on it, so that a machine which skips a capability-dependent test cannot fail a build it did not cause. They are raised deliberately, in a commit that says why, and never lowered without one saying why either. `include: ['src/**']` in the frontend config matters: the default counts only the files a test happened to import, which flatters the number by leaving everything else out of it.

**The Go percentage is not the number of statements you can see, and it is not evidence for a refactor.** A profile entry is `file:range numStmt count`, and `numStmt` is the statement count of the whole *basic block* that range belongs to, not of the lines it spans: `cmd/cover` emits one counter per code range inside a block, and a comment or a blank line inside a block splits it into another range that repeats the whole count. So a small range carrying a large count is not a corrupted profile, and the totals move with layout rather than with behaviour — measured, `92.9%` against `93.3%` for the same two functions with a comment moved between their statements. This is why RD-3's spec recorded the movement backwards: it read a layout-sensitive number as evidence.

**What to argue a behaviour-preserving change with** is `mise run coverage:diff BEFORE.prof AFTER.prof` (`scripts/coverage-diff.sh`), which compares the *functions* that were fully covered before the change and are not after. No comment, blank line or expression layout can make a function lose that, so a name in its output is a block that stopped running. Generate the two profiles from the two revisions with `go test -count=1 -covermode=atomic -coverprofile=….prof ./pkg/…`, and see the script's header for the reasoning in full.

- **Frontend unit and component tests: Vitest + React Testing Library + jsdom.** The runner is configured in `frontend/vitest.config.ts` (not the app's `vite.config.ts`, which sets an output directory and loads Tailwind). Tests live beside the code as `*.test.ts`/`*.test.tsx` and import `describe`/`it`/`expect` from `vitest` explicitly; there are no globals. `frontend/src/test/setup.ts` installs the jest-dom matchers, stubs the browser APIs jsdom lacks, and calls RTL's `cleanup`. The old `frontend/scripts/check*.mjs` files are gone, and the docs lint is no longer a Node script at all: `mise run lint:docs` runs `rumdl` over `pkg/gui/docs`.
- **A component reads the campaign from the context rather than receiving it.** `ChronicleView` and
  `StoryTheater` take what defines what they render (`turns`, `genre`, the display state a stream is
  in) and the flows they call back into, and read the playback, the portraits and the campaign's own
  values from `useCampaign()`. That is what keeps their prop lists at eleven and five rather than
  twenty-five and nineteen, and a consumer's test renders it inside a `CampaignProvider` rather than
  threading a mock through a prop: what the theatre asks the server to play is asserted on the client
  call the provider makes.

- **A campaign has one owner: `CampaignProvider`.** It is mounted in `App.tsx`, above the component
  that chooses a campaign, and it owns the campaign's data, the turn stream that writes it and the
  playback that reads it, because the three are written by the same code. `App.tsx` is a shell (under
  100 lines): the launcher-or-campaign switch, the chrome that sits above both (the menu bar, the help
  reader, the content studio, the About dialog) and the providers. `CampaignScreen.tsx` is the
  campaign's chrome and the flows that drive it. A consumer reads `useCampaign()` rather than receiving
  the value as a prop, and the provider exposes **actions, not setters**: `leaveCampaign`,
  `openEntity`, `spendAdvancement`, `dismissTurnError` and their neighbours, because a context that
  hands out setters is a prop list wearing a context's clothes. The value comes from a
  `useCampaignValue()` hook, so the value object and the hook's return type are provably the same
  thing rather than two hand-written lists.
- **A studio is a container and a set of views.** `components/SystemsStudio.tsx` and
  `components/WorldsStudio.tsx` hold the state, the API calls, the handlers, the layout and the
  dialogs; their regions live in `components/studios/system/` and `components/studios/world/`, one
  file per region, named for what it renders. A view holds no state and calls no API, and it takes
  **focused props by name** rather than the container's state object: a view that reads one boolean
  takes that boolean, so the compiler says what the view depends on and a test can render one tab
  without the other four. The container keeps its path and export name, so the lazy imports in
  `LauncherHub.tsx` are untouched. The two studios are deliberately not one shared studio: they would
  need a parameter for every difference between a system with a script and a world with entities.
- **A dialog's test asserts a clean axe run.** `frontend/src/test/a11y.ts` exports `expectNoViolations(container)`, which runs `axe-core` over what the test rendered and fails with the rules it broke, each named with the element that broke it. Every test for a surface that declares `role="dialog"` calls it, so the primitive is checked once and each surface is checked where it renders. jsdom has no layout, so the helper switches `color-contrast` off: left on it reports nothing and costs a log line per render. One axe check you cannot switch off, the icon-ligature test inside the name rules, still asks jsdom for a canvas and makes it print "Not implemented" in a couple of files; that line is not a failure. `axe-core` is a direct devDependency for this and nothing else. `jest-axe` was the alternative and was not taken: it is a CommonJS package with no bundled types that would pull four more packages into a tree that already has `axe-core` through `eslint-plugin-jsx-a11y`, and its only addition over the helper is a matcher.
- **Browser end-to-end tests: `pkg/e2e`, behind the `e2e` build tag.** The default `go test ./...` never compiles them, so a machine without Chrome stays fast; run them with `mise run test:e2e` (`go test -tags e2e ./pkg/e2e/...`). `harness.go` wraps chromedp (`NewBrowser`, `Click`, `Type`, `WaitFor`, `Poll`, `InstrumentAudio`) and `fixture.go` starts the real `gui.Service` on a temp root behind an `httptest` server (`NewFixture`, `WriteSystem`, `WriteWorld`, `InitGame`, `WriteEntities`, `WriteHistory`). The tests skip, never fail, when `driver.Available` finds no usable browser.
- **They live in `pkg/e2e`, not `pkg/gui`, because the harness imports `pkg/gui`** (`NewService`, `NewServer`, `AssetHandler`); keeping them in `pkg/gui` would be an import cycle. They use only the exported `pkg/gui` surface.
- **A failure writes artifacts** to `$E2E_ARTIFACT_DIR/<test name>/` (default `test-results/`): `body.txt`, `dom.json`, and `failure.png`. The CI `e2e` job installs Chrome with `browser-actions/setup-chrome`, runs the suite, and uploads `test-results/` so a red run is debuggable.
- **A test must never resolve the developer's real config.** `config.DetectConfigFile` reads `LOCALRPG_CONFIG_DIR` on every call, then falls back to XDG, and `xdg` caches its directories when the package is first used, so setting `HOME` in a test does not redirect anything: a test that then saves writes `~/.config/localrpg/config.yaml` and destroys the provider setup on the machine running it. `TestOfflineConfigCommands` did exactly that on every `go test ./...`. `cmd/localrpg` now redirects the whole package through `LOCALRPG_CONFIG_DIR` in its `TestMain` and refuses to run if a resolution escapes it; a test that resolves the config itself must do the same and assert the path is inside its own `t.TempDir()`.

## Conventions

- Go: standard library only for tests (`testing`, `t.TempDir()`); no testify. Errors wrapped with `fmt.Errorf("...: %w", err)`. Use modern Go idioms: prefer `any` over `interface{}`, the `slices`/`maps` helpers over hand-rolled loops, `min`/`max` over inline comparisons, and range-over-int; `go vet` must stay clean.
- Tests that need hardware a CI runner does not have — a browser, a sound card — skip rather than fail, and detect the capability instead of assuming it. `driver.Available` probes for a browser, because having Chrome installed is not the same as being able to start it. The audio device tests play a short clip and watch it finish, because a context opens on a host that cannot play anything and no cheaper signal distinguishes the two.
- Identifiers: `entity.Slugify` (display name -> kebab-case ID) and `entity.WikilinkTarget` (unwraps `[[target|label]]`) are the shared helpers — reuse them instead of writing local slug/link parsing.
- TypeScript: React 19 + Tailwind v4 (config lives in CSS via `@import "tailwindcss"` in `frontend/src/index.css`, there is no `tailwind.config.js`). `tsconfig.json` has `strict`, `noUnusedLocals`, `noUnusedParameters`, so `npm run build`/`tsc --noEmit` fails on unused imports. Components live in `frontend/src/components/`, icons come from `lucide-react`.
- **The frontend's lint is Oxlint plus Prettier, run by `mise run lint:frontend`.** `frontend/.oxlintrc.json` is the configuration and `frontend/prettier.config.mjs` the formatter's; both carry the reasoning for every non-default choice. Prettier's `format:check` and Oxlint's **error** level gate a build; Oxlint's **warnings** print without failing it. That split is the same shape as the prose lint, and each warning group names the proposal that fixes it and promotes it: accessibility to FE-6, React Compiler readiness (refs during render, setState in an effect) to FE-3 and FE-2, and `no-explicit-any` to FE-8. One a11y rule is off rather than warned: `control-has-associated-label` reports the same unlabelled inputs as `label-has-associated-control`, from the control's side, so the two together gave 258 findings for one class of defect.
- **The Go and TypeScript DTO contract is checked, not trusted.** `pkg/gui/types.go` and
  `frontend/src/types.ts` are two hand-maintained mirrors of one wire contract, so
  `pkg/gui/testdata/dto.json` is generated from the Go structs by
  `go test ./pkg/gui -update-dto` and `frontend/src/test/dtoContract.test.ts` compares it against
  `types.ts`, parsed with the TypeScript compiler. A rename on either side, a field the client cannot
  see, a property the server never sends, or a changed `omitempty` fails naming the type and the
  field. The pairing is not a table: a mirror is the interface whose property set is exactly the
  DTO's fields, so a rename breaks the match. Two exception maps in that test carry the deliberate
  differences, each with a reason, and an entry whose type or field is gone fails the suite.
  `types.ts` is not generated from Go: it carries discriminated unions, view models and comments a
  generator would drop, and it is read by the frontend more than by the backend.
- **Three accessibility rules have a gate behind them.** A drawn progress bar is either the only
  statement of its value, in which case it is a `ui/ProgressBar` with a required label, or it repeats a
  number written beside it, in which case it is `aria-hidden` and says which text it defers to; two
  statements of one fact is one announcement too many. A change the player is waiting for is announced
  through `ui/LiveStatus`, a `role="status"` region that is mounted empty and then given a sentence,
  because a region that appears with its text is announced by few screen readers. An icon-only button
  carries an `aria-label`: `frontend/src/test/iconButtons.test.ts` parses every `.tsx` with the
  TypeScript compiler and fails on a button whose subtree renders no text and no expression and which
  has no name, and it deliberately does not accept `title`, which is a tooltip that needs a pointer.
- TypeScript 7 decouples from the legacy JS compiler API; frontend linting is handled natively by Oxlint in Rust without a peer version bottleneck.
- Commits: Conventional Commits with a scope, e.g. `feat(harness): …`, `fix(frontend): …`, `docs: …`. Keep the subject under 72 chars. Commit after every turn of work is preferred, but only on a feature branch: never commit directly on `main` — branch first, then commit there.
- Design work is spec-first: `docs/superpowers/specs/` holds approved design docs and `docs/superpowers/plans/` holds task-by-task implementation plans with `- [ ]` checkboxes, including a "File Map" listing files to create/modify per feature. Read the relevant spec before changing a subsystem; the plans reference the superpowers skills workflow. Note `.superpowers/` is gitignored while `docs/superpowers/` is tracked.

## Gotchas

- Media cache keys must incorporate voice identity *and* prosody: `ComputeAudioCacheKeyWithRate` (speaker, voice ID, pitch, speech rate, text) exists precisely to avoid cache collisions between characters sharing a voice. Use it rather than the older `ComputeAudioCacheKey`.
- `AssignVoiceProfile` (`pkg/harness/extractor.go:64`) resolves NPC voices in order: literal profile-ID match in name/body, tag scoring, then a deterministic FNV hash of the entity ID. Voice profiles are only assigned to entities of type `character` and never overwrite an existing `Voice`.
- `storage.Sync` silently skips malformed Markdown files (missing `---` frontmatter or unparsable YAML) — a missing entity usually means a frontmatter parse failure, not a sync bug.
- Per-turn extraction reuses the `gm` provider unless `agents.roles.extractor` names another one; set that role to `disabled` to record deterministic mentions only. A failed extractor never loses the turn, and `harness.ResolveEntityMentions` needs no model at all.
- The extractor is only reached through `Timeline.RecordTurn`. Calling `harness.Extractor.Extract` directly returns records without persisting anything, by design.
- An entity note is written as `<id>.md` because `gui.Service.GetGraph` derives node IDs from file names. Template notes copied from a world are renamed to their frontmatter ID on game creation for the same reason.
- `pkg/gui` exposes `ProtectCrossOrigin` (`middleware.go`), which wraps handlers in the standard library's `http.CrossOriginProtection` instead of hand-rolled CORS headers. Cross-origin browser writes get a 403; same-origin callers, the native Wails webview (`wails://wails`), and header-less local tooling are allowed. `Server.ServeHTTP` is now a bare mux delegation, so do not add `Access-Control-*` headers back.
- `gui.Service.ensureIndexed` repairs a campaign's index once per process, the first time that process serves the game. Timeline queries (`GetEntityTurns`) answer from the database; chronicle reads still come from `history.jsonl`, which is where the segments live.
- The desktop app plays turns through `POST /api/game/{id}/turn`, streaming newline-delimited JSON; turns are serialised per campaign by `Service.BeginTurn` (409 while one is in flight); nothing is persisted for a cancelled or disconnected turn; and provider setup lives in `pkg/harness.RouterFromConfig`/`ExtractorFromConfig` rather than in `package main`.
- Speech is synthesized per **group**, not per sentence: `pkg/media.GroupPlan` merges adjacent same-speaker segments (up to the provider's `MaxSpeakers`) into one request, and the clip is content-addressed by `ComputeGroupCacheKey` (a `v4:` hash of the effective lines, so it is segmentation-stable). A group clip is shared by every segment it covers, so the GUI renders one play/stop/regenerate control per group and regen targets the group key. Set `media.tts.grouping: off` to revert to the per-sentence path. Grouping and sentence streaming are mutually exclusive: `media.tts.stream_sentences` (off by default for a metered provider) wins where it runs, so grouping applies to metered/remote providers and export, and streaming to local ones.
