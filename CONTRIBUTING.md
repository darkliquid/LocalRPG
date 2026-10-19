# Contributing to LocalRPG

LocalRPG is a local-first, turn-based tabletop RPG client: one Go binary that
runs a desktop GUI, a terminal client, and an HTTP API over the same engine.
Contributions are welcome, whether they are a bug report, a documentation fix, a
new provider, or a change to the engine.

This is the short on-ramp. Two longer documents sit behind it:

- [`docs/architecture/overview.md`](docs/architecture/overview.md) is the living
  architecture: the three content tiers, the package map, the turn pipeline, and
  where a change goes.
- [`AGENTS.md`](AGENTS.md) is the same ground covered for AI assistants, and it is
  the most detailed description of the repository's conventions and gotchas. It is
  worth reading even if you are not an agent.

## Before you start

- **Go 1.27.2**, **Node 26.9.0**, and the rest of the toolchain are pinned in
  `mise.toml`. Install [mise](https://mise.jdx.dev) and run `mise install`, or
  make sure the pinned versions are what you have.
- **A CGO toolchain**, as the desktop build links a native window library. Every
  release platform is built natively for this reason.
- **For the browser tests**, a working Chrome. The end-to-end suite detects one
  and skips rather than failing when it cannot start.

```bash
mise run setup       # go mod download, and the frontend's dependencies
mise run build       # build the frontend, then the binary into bin/localrpg
```

The frontend is embedded in the Go binary, so the backend cannot be built before
the frontend. `mise run build` enforces that ordering; do not run `go build` on a
clean checkout without building the frontend first.

## Running it

```bash
mise run dev:gui        # the desktop app on :8080
mise run dev:frontend   # the Vite dev server on :3000, proxying /api to :8080
```

`mise run dev:frontend` plus a backend started separately is the fast loop for
frontend work; it reloads on save.

## Before you open a pull request

```bash
mise run test    # Go tests with a coverage floor, the frontend suite, and the browser suite
mise run lint    # gofmt, staticcheck, go vet, Prettier, Oxlint, rumdl, goreleaser, actionlint
```

If you changed the guides, the configuration, a provider, or a preset, regenerate
the generated documentation as well:

```bash
go test ./pkg/gui -update-docs
```

That rewrites the index article, the configuration reference, and the provider
catalogue from the code, and the matching tests fail until it is run. Both test
suites have a coverage floor; raise it deliberately if you add a lot of testable
code, and never lower it without saying why in the commit.

The state of a work-in-progress build is partly reported rather than gated:
`mise run lint:prose` (Vale) and `mise run lint:revive` (undocumented exported
symbols) print findings without failing. `STRICT=1 mise run lint:prose` gates on
error-level alerts, which is what CI does.

## Branches and commits

- **Never commit on `main`.** Branch first, then commit there.
- **Conventional Commits with a scope:** `feat(harness): …`, `fix(frontend): …`,
  `docs: …`. Keep the subject under 72 characters.
- Commit after each turn of work rather than in one lump at the end; a reviewer
  reads the history.
- Explain the *why* in the message body. A reader who has never seen the code
  should understand what problem the change solves.

## How work is tracked

Development work lives in **Beads** (`bd`), a local, dependency-aware tracker
whose database is in `.beads/`. GitHub hosts the repository and reviews pull
requests; it is deliberately not the issue tracker.

```bash
bd ready                     # the claimable frontier, in priority order
bd show <id>                 # a bead's description, dependencies and acceptance criteria
bd update <id> --claim       # take it
bd close <id> --reason "..." # finish it, with the reason as the record
```

Work is organised into **waves** (a `wave/<n>` label, an ordering), **epics** (one
per theme), and **proposals** (one bead per unit of work). Ordering constraints are
`blocks` dependencies rather than prose, because only `blocks` affects `bd ready`.

If you find something worth fixing while working on something else, record it with
`bd q "..."` and link it rather than expanding the change in front of you.

## Spec-first, for changes that decide something

A change that decides an interface, a behaviour, or a tradeoff gets a design doc
and a plan before it gets code:

- `docs/superpowers/specs/<date>-<slug>-design.md` — the design.
- `docs/superpowers/plans/<date>-<slug>.md` — the task-by-task plan, including a
  file map.

Mechanical work, such as a formatting sweep, a rename, a dependency bump, or a
test that mirrors an existing one, decides nothing and carries no design step.
Read the relevant spec before changing a subsystem; the plans are also a record of
what was tried.

## Where things live

| Area | Where |
| --- | --- |
| Turn orchestration | `pkg/engine` |
| Model providers and prompt assembly | `pkg/harness` |
| Speech, images, and their caching | `pkg/media` |
| The API surface and the SPA host | `pkg/gui` |
| The terminal client | `pkg/tui` |
| Entities, the graph, and the on-disk format | `pkg/entity`, `pkg/storage` |
| Tracing and the OTLP export | `pkg/trace`, `pkg/telemetry` |
| Frontend components and styles | `frontend/src` |
| The user guide the app embeds | `pkg/gui/docs` |

The engine is schema-agnostic: no hit points, mana, or classes are hardcoded
anywhere. State is opaque YAML plus sandboxed JavaScript hooks, and a change that
introduces knowledge of a particular game system is almost always in the wrong
place.

## Adding an API endpoint

`frontend/src/api/client.ts` is the only fetch layer, and it mirrors `pkg/gui/server.go`.
An endpoint is added in four places together:

1. `pkg/gui` — the method on `Service`.
2. `pkg/gui/server.go` — the route.
3. `frontend/src/types.ts` — the wire type.
4. `frontend/src/api/client.ts` — the call.

The Go and TypeScript types are two hand-written mirrors of one contract, and a
test compares them: `go test ./pkg/gui -update-dto` regenerates the fixture that
`frontend/src/test/dtoContract.test.ts` checks. A rename on either side, or a field
the client cannot see, fails the frontend suite.

## Adding a guide article

A page under `pkg/gui/docs/` needs no registration: both the documentation viewer
and the showcase site read the directory. Give it frontmatter with an `id`,
`title`, `category`, `order`, and `description`, repeat the title as the first
heading, and regenerate the index with `go test ./pkg/gui -update-docs`.

The prose is linted by Vale (`mise run lint:prose`), and the articles are checked
by rumdl (`mise run lint:docs`). A word Vale does not know belongs in
`styles/config/vocabularies/LocalRPG/accept.txt`, which is the vocabulary the
project maintains for exactly that purpose.

## Frontend tests

Unit and component tests live beside the code as `*.test.ts`/`*.test.tsx` and use
Vitest with React Testing Library. Every test for a surface that declares
`role="dialog"` calls `expectNoViolations` from `frontend/src/test/a11y.ts`, which
runs axe over what the test rendered. Accessibility is checked in the component
rather than in a review, so a new dialog is expected to carry that check with it.

## Secrets

Assume anything committed is public the moment it is pushed.

- Never commit a provider credential. The documented form keeps the value out of
  the file: `api_key: "env:OPENAI_API_KEY"`.
- `/localrpg.yaml`, `/config.yaml`, and `.env` are gitignored, but the real
  control is not committing the value in the first place.
- `mise run secrets:scan` runs gitleaks over the full history and the staged
  changes. It is a backstop, not a substitute for care.
- Screenshots leak too: `website/screenshots/` is committed, and a shot of the
  settings panels can show a key.

## Licence

By contributing you agree that your work is licensed under the same terms as the
project; see [`LICENSE`](LICENSE). Third-party components and their licences are
listed in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
