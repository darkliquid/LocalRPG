# AGENTS.md

LocalRPG is a local-first, turn-based tabletop RPG client: a single Go binary that runs a Wails v3 desktop GUI, a Bubbletea terminal TUI, an HTTP/Unix-socket API daemon, plus media generation and story export. Module path: `github.com/darkliquid/localrpg`. The engine is **schema-agnostic** — no HP/Mana/classes are hardcoded anywhere; all RPG state is opaque YAML frontmatter plus sandboxed JavaScript hooks.

## Commands

Toolchain is pinned by `mise.toml` (Go 1.27.1, Node 26.9.0, GoReleaser 2.18.2). Prefer `mise run`, but plain `go`/`npm` work if the toolchain is already active.

```bash
mise run setup          # go mod download + cd frontend && npm install
mise run install:vale-styles # vale sync: download the style packages when .vale.ini changes
mise run build          # frontend bundle -> pkg/gui/dist, then bin/localrpg
mise run build:frontend # npm run build in frontend/ (tsc + vite)
mise run build:backend  # depends on build:frontend
mise run test           # go test -v -count=1 ./...  AND  mise run test:frontend  AND  mise run test:e2e
mise run test:backend   # go test -v -count=1 ./...
mise run test:frontend  # npx tsc --noEmit and npx vitest run (in frontend/)
mise run test:e2e       # go test -tags e2e ./pkg/e2e/... (skips without a browser)
mise run lint           # rumdl, goreleaser check, actionlint, go vet ./...
mise run lint:docs      # rumdl over the embedded help articles (pkg/gui/docs, .rumdl.toml)
mise run lint:prose     # Vale over tracked prose and source comments (report only; STRICT=1 to gate)
mise run lint:goreleaser # goreleaser check
mise run lint:actions   # actionlint over .github/workflows
mise run secrets:scan   # gitleaks over the full git history and staged changes
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

Regenerate the generated embedded docs (provider catalogue, config reference)
after changing a provider, preset, or config struct:
`go test ./pkg/gui -update-docs`.

CLI surface (`localrpg <cmd>`): `roll <notation>`, `prompt`, `play <game-id>`, `tts`, `image`, `gui`, `export <web|video>`, `debug <test-run|server>`, `config <cmd>`, `version`.

## Working the project

Development work is tracked in a long-lived GitHub Project, identified by `PROJECT_ID` in
`scripts/project.conf` (currently **LocalRPG Next Phase**,
<https://github.com/users/darkliquid/projects/2>). The project outlives any single phase: rename it,
and keep adding waves, areas, epics, and proposals as the work grows. There are three levels, one per
GitHub object:

- **Wave** - a milestone, named `Wave <n> - <title>`. Waves are dependency-ordered; work in the
  earliest incomplete wave first.
- **Epic** - one issue per theme, labelled `type/epic` and `area/<theme>`, with the theme's proposals
  as **sub-issues** and a `## Proposals` task list.
- **Proposal** - one issue per unit of work, labelled `area/*`, `wave/*`, `effort/*`, and a type
  label. Its body carries a summary, its epic, its dependencies, and a **definition of done**.

The board tracks every item with a **Status** single-select, and the status moves with the work:

| Status | Means |
|---|---|
| `Proposal` | captured; no spec yet |
| `Specced` | the design spec is written |
| `Planned` | the implementation plan is written |
| `In Progress` | implementation underway |
| `Testing` | implemented, under verification |
| `Done` | complete |

**Spec-first.** A proposal is specced and planned before it is implemented: the spec is a design doc
in `docs/superpowers/specs/<date>-<slug>-design.md`, and the plan is a task-by-task implementation
plan in `docs/superpowers/plans/<date>-<slug>.md`, following the superpowers workflow. Read the
relevant spec before changing a subsystem. The proposal's definition of done is a checklist - spec,
plan, implemented, tests and lint, user-facing docs - and each box is ticked when it is true.

### The `project:*` tasks

These wrap `scripts/project.sh`, which reads `scripts/project.conf` and resolves the project, its
fields, and their option IDs by name at run time. Use them rather than `gh project` directly, so the
vocabulary stays in one place. Because nothing is hardcoded but the project ID, the project's name
and contents can change freely.

```bash
mise run project:list                        # every item
mise run project:list --wave 1               # filter by --wave, --status, --area, or --kind
mise run project:next                        # the next ready item (earliest wave, In Progress first)
mise run project:show 37                      # one item: status, fields, spec, plan, and DoD
mise run project:scaffold 37                  # create the spec and plan skeletons for an issue
mise run project:status 37 in-progress        # set the board status
mise run project:check 37 implemented         # tick a DoD box (--uncheck clears it)
mise run project:add-wave 5 "Hardening"       # a new wave: milestone, field option, and label
mise run project:add-area audio               # a new theme: label and field option
mise run project:add-epic --area systems --title "New theme" --wave 4 --summary "..."
mise run project:add-proposal --epic 23 --id RB-7 --title "Repair X" --area robustness \
    --wave 4 --effort M --kind feature --summary "..." --deps "RB-1"
mise run project:link 23 99                   # attach an existing issue as a sub-issue
mise run project:new --title "Next Phase"     # only to start a brand new project
```

Waves and areas are open-ended: add them with `project:add-wave` and `project:add-area`, which create
the milestone or label and the matching field option, and the tasks pick them up. The statuses, kinds,
efforts, and definition-of-done boxes are convention and live in `scripts/project.conf`.

A worker's loop for one proposal:

```bash
mise run project:next                  # or project:show <issue>
mise run project:scaffold <issue>      # writes the spec and plan skeletons; fill them in
mise run project:check <issue> spec
mise run project:status <issue> specced
mise run project:check <issue> plan
mise run project:status <issue> planned
mise run project:status <issue> in-progress   # when implementation starts
mise run project:check <issue> implemented
mise run project:check <issue> tested
mise run project:status <issue> done
```

`project.sh` runs project-scoped `gh` commands without an ambient `GITHUB_TOKEN`, because a token set
in the environment (CI, some shells) can lack the `project` scope while the keyring login has it. If
`gh project` reports a missing scope, run `gh auth refresh -s project,read:project`.

## CI, releases and the showcase site

Three workflows live in `.github/workflows/`. `ci.yml` runs on every push to `main` and every pull request: a Go job (native headers, frontend build, `go vet`, `go test`), a web job (`tsc --noEmit`, vitest, rumdl), and a packaging job (`goreleaser check`, `mise run site:build`, plus the built site as an artifact). It also declares `workflow_call`, so `release.yml` reuses it as a `verify` job and a tag can only ship what passed.

`release.yml` runs on `v*` tags and published releases. Every platform needs CGO, so no runner can cross-compile the matrix: `.goreleaser.yaml` defines one build per target (OS plus architecture), each with a `skip` template that disables it unless `RELEASE_TARGET` names its target. The matrix runs one native runner per target — `ubuntu-latest` and `ubuntu-24.04-arm` for the two Linux architectures (a CGO build for arm64 cannot come from an amd64 host), `macos-latest` twice for both macOS slices, and `windows-latest` — uploads archives, and a final job combines the checksums and publishes or enriches the release with `gh`. GoReleaser's own split/merge is a Pro feature, which is why publishing is done with `gh`. The arm64 Linux runner is a hosted runner, free only for public repositories; on a private repo, drop that matrix entry. `Version` in `cmd/localrpg/main.go` is a `var` so `-X main.Version` can stamp the tag.

`pages.yml` builds the showcase site and deploys it to GitHub Pages. The site is generated by `tools/sitegen`, a small Go program that renders `README.md` and `pkg/gui/docs/*.md` into `website/dist` using the same glassmorphic styling as the app (the palette and blur values are copied from `frontend/src/index.css`). Nothing about the site is hand-maintained prose: it reads the documentation the binary embeds, so the two cannot drift. `mise run site:serve` builds it and previews it at `http://localhost:4173`. `website/screenshots/` holds the gallery images (see its README); `website/demo/` is the fixture campaign the capture task plays against. GitHub Pages must be enabled with "GitHub Actions" as the source.

`secrets.yml` scans for credentials on every push and pull request, and weekly so newly published detection rules are applied to history that has not moved.

## Secrets

`mise run secrets:scan` runs gitleaks over the full git history and over staged changes; `.github/workflows/secrets.yml` runs the same task in CI. `.gitleaks.toml` *extends* the default ruleset (a config without `useDefault` silently disables every shipped rule) and allowlists only the two fake credentials the `pkg/trace` redaction tests need.

Anything that reaches a commit is public the moment it is pushed, so scanning is a backstop, not the control. The real protections are:

- **Enable secret scanning and push protection** in the repository's security settings. Push protection rejects a push containing a recognised credential before it ever lands in history. Both are free on public repositories.
- **Never commit provider credentials.** `/localrpg.yaml`, `/config.yaml` and `.env` are gitignored because the app writes API keys into a workspace override when it runs in project mode. The documented form is `api_key: "env:OPENAI_API_KEY"`, which keeps the value out of the file entirely.
- **Screenshots leak too.** `website/screenshots/` is committed, and a shot of the Settings Studio or a provider panel can show a key. Look at them before committing.

If a key does reach a commit, rotate it first: rewriting history does not un-publish it.

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
`rumdl check` reports the same result on the corpus: no issues in 23 files.

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

## Prose linting with Vale

Vale checks prose style in the user-facing documentation only. It is pinned in
`mise.toml` like every other tool, and `.vale.ini` at the repository root decides
the styles.

**Scope is 25 files**, and it is the same set `tools/sitegen/content.go` renders
into the showcase site:

- `pkg/gui/docs/*.md` - the 23 guide articles the application embeds, and the bulk
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
mise run install:vale-styles   # vale sync; runs automatically when .vale.ini changes
mise run lint:prose            # full report, every finding
SUMMARY=1 mise run lint:prose  # counts, noisiest rules, worst files
STRICT=1 mise run lint:prose   # exit non-zero on error-level alerts
```

### What it reports today

**233 alerts across all 25 files: 0 errors, 100 warnings and 133 suggestions.**
For scale, pointing the same styles at every tracked file reported 68,525 alerts
and 6,875 errors, which is why the scope is the documentation rather than the
repository.

**CI gates on the error level.** `STRICT=1` makes the task exit non-zero when an
error is reported, and the workflow runs it that way. Warnings and suggestions are
printed but never fail a build, because they are style preferences rather than
mistakes. Run `mise run lint:prose` for the full report, or `SUMMARY=1` for the
counts and the noisiest rules.

Reaching zero took three passes: narrowing the scope to the documentation, adding
the vocabulary, and then rewriting the prose the remaining rules objected to. A
fourth pass then worked down the warnings and suggestions, which took the report
from 293 alerts to 149. The guide that documents the on-disk and package formats
added 41 more, and the AI world generation guide added 43, so the count stands at 233.

Everything left is deliberate. `neighbor.AmpersandInProse` (71) fires on `&` in
headings and bolded feature labels, which is a design convention rather than prose.
`Google.Passive` (66) and `Google.Semicolons` (19) are style preferences, and
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

GUI: `pkg/gui.Service` (`service.go`) is the entire API surface; `pkg/gui/server.go` maps it to `/api/...` routes and serves the embedded SPA with client-side-routing fallback. `frontend/src/api/client.ts` is the only fetch layer and mirrors those routes — when adding an endpoint, update `Service`, `server.go`, `frontend/src/types.ts`, and `client.ts` together. The desktop app plays turns through `POST /api/game/{id}/turn`, which streams newline-delimited JSON; turns are serialised per campaign by `Service.BeginTurn` (409 while one is in flight); nothing is persisted for a cancelled or disconnected turn; and provider setup lives in `pkg/harness.RouterFromConfig`/`ExtractorFromConfig` rather than in `package main`.

Runtime modes in `cmd/localrpg/gui.go`: default is a native Wails v3 window (zero TCP); if `$DISPLAY`/`$WAYLAND_DISPLAY` are unset, or `--headless`/`--socket` is passed, it serves the same handler over a `0600` Unix socket (`pkg/gui/socket.go`, unlinks stale sockets); `--port N` opts into `127.0.0.1:N` TCP for a browser.

## Provider model (uniform across LLMs and media)

Every AI/LLM and media backend (LLM agents, TTS, STT, image) uses the same config shape: `type` is one of `builtin`, `cli`, `http`, `disabled` (plus `mock` for LLMs), with `builtin_name` selecting an in-process implementation. Factories: `harness.NewModelProvider` (`pkg/harness/factory.go`), `media.NewTTSClient`/`NewSTTClient`/`NewImageClient` (`pkg/media/providers.go`).

Notable built-ins that need no server or GPU: `narrative-oracle` (LLM), `native-os` (TTS via `spd-say`/`say`/PowerShell), `procedural-art` (pure-Go SVG image generator). `harness.Router` maps role name -> provider ID with optional per-role fallbacks; roles are arbitrary strings ("gm" is the only one the orchestrator hardcodes; "narrator" exists only as a config default).

## Configuration

Resolution order (`pkg/config/manager.go`): `$LOCALRPG_CONFIG_DIR`, else the XDG config search path (`$XDG_CONFIG_HOME` then `$XDG_CONFIG_DIRS`), for `config.yaml`; then an optional `./localrpg.yaml` merged on top, which flips `IsLocalOverride`. `Save` writes to the local override when one exists, otherwise to the user config. `gui.NewService(rootDir)` has its own twist: when `--dir` is set to anything other than `.`, it treats `<rootDir>/config.yaml` as the user config and resolves relative `paths.*` against `rootDir`.

Storage paths are resolved by `pkg/paths.Resolve` from the XDG bases (`github.com/adrg/xdg`), which fall back to native per-OS directories: `systems`/`worlds`/`games` under `DataHome/localrpg`, `cache` under `CacheHome/localrpg`. An empty `paths.*` uses those defaults, an absolute value is used verbatim, and a relative value joins the category base — unless `--dir` or `./localrpg.yaml` puts the process in project mode, where relative values resolve against the project root (the previous behaviour). Nothing is migrated; the GUI logs a `paths.legacy_relative` warning when a legacy working-directory folder exists and the resolved XDG directory is empty.

## Testing

Two suites, one runner each. `mise run test` runs both plus the backend.

- **Frontend unit and component tests: Vitest + React Testing Library + jsdom.** The runner is configured in `frontend/vitest.config.ts` (not the app's `vite.config.ts`, which sets an output directory and loads Tailwind). Tests live beside the code as `*.test.ts`/`*.test.tsx` and import `describe`/`it`/`expect` from `vitest` explicitly; there are no globals. `frontend/src/test/setup.ts` installs the jest-dom matchers, stubs the browser APIs jsdom lacks, and calls RTL's `cleanup`. The old `frontend/scripts/check*.mjs` files are gone, and the docs lint is no longer a Node script at all: `mise run lint:docs` runs `rumdl` over `pkg/gui/docs`.
- **Browser end-to-end tests: `pkg/e2e`, behind the `e2e` build tag.** The default `go test ./...` never compiles them, so a machine without Chrome stays fast; run them with `mise run test:e2e` (`go test -tags e2e ./pkg/e2e/...`). `harness.go` wraps chromedp (`NewBrowser`, `Click`, `Type`, `WaitFor`, `Poll`, `InstrumentAudio`) and `fixture.go` starts the real `gui.Service` on a temp root behind an `httptest` server (`NewFixture`, `WriteSystem`, `WriteWorld`, `InitGame`, `WriteEntities`, `WriteHistory`). The tests skip, never fail, when `driver.Available` finds no usable browser.
- **They live in `pkg/e2e`, not `pkg/gui`, because the harness imports `pkg/gui`** (`NewService`, `NewServer`, `AssetHandler`); keeping them in `pkg/gui` would be an import cycle. They use only the exported `pkg/gui` surface.
- **A failure writes artifacts** to `$E2E_ARTIFACT_DIR/<test name>/` (default `test-results/`): `body.txt`, `dom.json`, and `failure.png`. The CI `e2e` job installs Chrome with `browser-actions/setup-chrome`, runs the suite, and uploads `test-results/` so a red run is debuggable.

## Conventions

- Go: standard library only for tests (`testing`, `t.TempDir()`); no testify. Errors wrapped with `fmt.Errorf("...: %w", err)`. Use modern Go idioms: prefer `any` over `interface{}`, the `slices`/`maps` helpers over hand-rolled loops, `min`/`max` over inline comparisons, and range-over-int; `go vet` must stay clean.
- Tests that need hardware a CI runner does not have — a browser, a sound card — skip rather than fail, and detect the capability instead of assuming it. `driver.Available` probes for a browser, because having Chrome installed is not the same as being able to start it. The audio device tests play a short clip and watch it finish, because a context opens on a host that cannot play anything and no cheaper signal distinguishes the two.
- Identifiers: `entity.Slugify` (display name -> kebab-case ID) and `entity.WikilinkTarget` (unwraps `[[target|label]]`) are the shared helpers — reuse them instead of writing local slug/link parsing.
- TypeScript: React 19 + Tailwind v4 (config lives in CSS via `@import "tailwindcss"` in `frontend/src/index.css`, there is no `tailwind.config.js`). `tsconfig.json` has `strict`, `noUnusedLocals`, `noUnusedParameters`, so `npm run build`/`tsc --noEmit` fails on unused imports — that is the frontend lint gate. Components live in `frontend/src/components/`, icons come from `lucide-react`.
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
