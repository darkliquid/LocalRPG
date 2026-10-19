# LocalRPG Architecture

- **Status:** Living document. It describes the system as it is, and it is updated
  as the system changes.
- **Audience:** A contributor who has cloned the repository and wants to know where
  a change belongs.
- **Companions:** [`review-2026-09-24.md`](review-2026-09-24.md) and
  [`quality-review-2026-10-12.md`](quality-review-2026-10-12.md) are dated reviews.
  They are history: each records the system at one commit, and each says so in its
  own header. Read them for reasoning, not for the current shape.
- **For agents:** [`AGENTS.md`](../../AGENTS.md) covers the same ground with the
  conventions and gotchas that matter when an assistant is editing the tree.

## 1. What the program is

LocalRPG is a single Go binary with three front-ends over one engine:

- A **desktop GUI**, a Wails v3 native window by default. When no display is
  available, or `--headless` or `--socket` is passed, the same handler is served
  over a `0600` Unix socket; `--port N` opts into `127.0.0.1:N` TCP for a browser.
- A **terminal client**, Bubbletea in `pkg/tui`, reached through `localrpg play`.
- A **command line and daemon**, one-shot commands plus the HTTP API.

It is local-first. There is no account and no required network call: the engine
runs offline, the content is files on disk, and the AI providers can be local
processes. A campaign is never held behind a service.

The engine is deliberately **schema-agnostic**. No hit points, mana, classes, or
attributes are hardcoded anywhere. RPG state is opaque YAML frontmatter plus
sandboxed JavaScript hooks, and the engine stores, displays, and passes it without
interpreting it. That decision is the source of the system's strongest property,
and of its recurring cost: anything that needs to reason about state does so
through an opaque map.

## 2. The three content tiers

Content is separated on disk into three tiers, resolved by `core.PathResolver`
from the XDG data directories (`pkg/paths`). A campaign is the only tier that
holds runtime state.

| Tier | Directory | Holds |
| --- | --- | --- |
| System | `systems/<id>/` | Mechanics: `system.yaml`, `mechanics.js`, `prompts/rules.md`, `tests/`. |
| World | `worlds/<id>/` | Lore: `world.yaml`, `prompts/lore.md`, `entities/*.md`, and per-system overrides in `system_overrides/<system-id>/hooks.js`. |
| Campaign | `games/<id>/` | One playthrough: `game.yaml`, `history.jsonl`, `entities/*.md`, `assets/`, and the disposable `cache/index.db`. |

Two rules follow from this and are worth stating plainly:

- **The campaign has exactly one database, and it is disposable.** It is
  `games/<id>/cache/index.db`, it is only ever opened through
  `storage.OpenGameStore`, and deleting it costs a rebuild and nothing else.
  `history.jsonl` is the canonical timeline; the index mirrors it.
- **Nothing in the repository ships sample content.** `systems/`, `worlds/`, and
  `games/` are empty in a checkout, and all content is authored through the
  studios or by hand.

## 3. Runtime shape

The intended dependency direction is
`cmd → {gui, tui} → engine → {harness, media, tools, storage} → {entity, state, config, core}`,
and the graph respects it. There are no import cycles.

| Package | Role |
| --- | --- |
| `cmd/localrpg` | The composition root: flags, wiring, and process lifetime. |
| `pkg/gui` | The whole API surface, the session plumbing, and the embedded SPA host. |
| `pkg/tui` | The terminal client. |
| `pkg/engine` | Turn orchestration, the timeline, rules hooks, and the start-location resolver. |
| `pkg/harness` | Model providers, the role router, context assembly, and entity extraction. |
| `pkg/media` | Speech, transcription, image generation, caching, and playback. |
| `pkg/tools` | The tool executor a turn's model calls reach. |
| `pkg/storage` | The SQLite index, the connection pool, and the sync from Markdown. |
| `pkg/entity` | The entity model: frontmatter, wikilinks, and the graph edges they imply. |
| `pkg/state` | The opaque state map and its dot-separated paths. |
| `pkg/config` | The configuration structs, their defaults, and the resolver. |
| `pkg/core` | Path resolution and the on-disk manifest types. |
| `pkg/trace` | The JSONL trace sink and its redaction policy. |
| `pkg/telemetry` | The OpenTelemetry providers, the metrics catalogue, and the log bridge. |

Four seams carry most of the design:

1. **`trace.Logger`** is a flat `Event(name, fields)` interface, nil-safe through
   `trace.OrNil`, and threaded everywhere. It is the model to copy for a new
   cross-cutting concern, and it is what lets telemetry attach without touching a
   call site.
2. **`harness.ModelProvider`**, with optional capability interfaces, is the
   implementation seam for a language model.
3. **`media.TTSClient`, `STTClient`, and `ImageClient`**, with the same optional
   capability interfaces, are the seam for media.
4. **`storage.OpenGameStore`** is the only sanctioned way to open a campaign
   database. Legacy retirement and shared-handle lifetime live behind it.

Provider descriptors are registered in a leaf package, `pkg/provider`, and each
provider package registers itself on import. The GUI renders its catalogue from
that registry rather than from a second hand-written list.

## 4. The turn pipeline

A turn is played over `POST /api/game/{id}/turn`, streamed as newline-delimited
JSON, and serialised per campaign: a second request while one is in flight is
refused with a 409.

```text
POST /api/game/{id}/turn
  └── Service.BeginTurn
       └── engine.TurnOrchestrator.ProcessAction
            ├── load history; answer /undo, /recap, /go, and /gm
            ├── evaluate a Roll, or a JavaScript action hook
            ├── harness.ContextAssembler.Assemble   (layered prompt, under a token budget)
            ├── runGenerationLoop                   (bounded tool rounds)
            │     └── Router.GenerateForRole("gm") → a provider, streamed
            │           └── tools.Executor.Execute on each tool call
            ├── resolve entity mentions and dialogue segments
            ├── engine.Timeline.RecordTurn          (the single writer)
            ├── onTurnEnd hooks
            └── (detached) the chronicler regenerates the story so far
```

The context prompt is assembled in layers, in the order
`pkg/harness/context.go` composes them: system rules, world lore, the voice-profile
catalog, then the scene scope, the living-world arcs, the characters present, and
the player's action. Entities are reachable from the current location through the
graph edges their wikilinks create.

The opening location is never hardcoded. `engine.ResolveStartLocation` prefers a
pinned `settings.start_location`, then the player's own location, then any indexed
location, and only then derives one from the world manifest, writing it into the
campaign as an opening scene.

Exactly one component writes: `engine.Timeline`. `RecordTurn` stages entity notes,
appends the turn to `history.jsonl`, and then indexes it. The order matters: notes
are written before the log, so a record never points at a note that does not exist,
and an index failure is repaired later rather than losing the turn.

## 5. Storage invariants

- `history.jsonl` is canonical and append-only. `/undo` is the one operation that
  rewrites it, trimming the log below the target turn and deleting the indexed
  turns above it.
- `cache/index.db` is derived. `Timeline.EnsureIndexed` replays the log into it
  whenever the two disagree, which is what makes the index safe to delete.
- Any new per-turn persistence goes through `Timeline.RecordTurn`. A second writer
  would break the ordering the rest of the system assumes.
- A `storage.Store` handed out by the pool is shared, and its `Close` is a no-op;
  `Pool.Close` owns the lifetime.
- `storage.Sync` silently skips a Markdown file whose frontmatter is missing or
  whose YAML does not parse. A missing entity is usually a parse failure, not a
  sync bug.
- Entity notes are named `<id>.md`, because the graph derives node identifiers from
  file names.

## 6. The provider model

Every AI and media backend, once it is configured, has the same shape: a `type`
that selects how it is reached (`builtin`, `cli`, `http`, or `disabled`, plus
`mock` for models and `builtin_name` to name an in-process implementation), and a
set of fields for its endpoint, credentials, or command.

Roles are arbitrary strings mapped to provider identifiers, with optional
per-role fallbacks: `harness.Router` holds the mapping, and `"gm"` is the only role
the orchestrator hardcodes. Media providers are reached through
`media.NewTTSClient`, `NewSTTClient`, and `NewImageClient`.

A metered call records a **ledger key**: the adapter that served it and the target
it reached. Prices match on that key, and the usage ledger aggregates by it. The
provider catalogue in the user guide is generated from the registry, so the
documented identifiers cannot drift from the code.

## 7. The frontend

`frontend/src/api/client.ts` is the only fetch layer in the application, and it
mirrors the routes in `pkg/gui/server.go`. `frontend/src/types.ts` mirrors the Go
DTOs in `pkg/gui/types.go`; the two are compared by a test against a fixture that
`go test ./pkg/gui -update-dto` regenerates, so a rename on either side fails the
suite rather than the running app.

The application shell, `App.tsx`, chooses between the launcher and a campaign and
mounts the chrome that sits above both. A campaign has one owner,
`CampaignProvider`, which holds its data, the turn stream that writes it, and the
playback that reads it. Consumers read `useCampaign()` rather than receiving the
campaign through a prop list, and the context exposes actions rather than setters.

The two studios are containers plus views: the container holds the state and the
API calls, and each region under `components/studios/` renders one part of it with
focused props by name. They are deliberately not one shared studio: they would
need a parameter for every difference between a system with a script and a world
with entities.

A failure the player can act on goes through `useFailures` and is reported in one
region near the top of the app; a form's own error stays beside its field.

## 8. Configuration and paths

`pkg/config` resolves configuration in order: `$LOCALRPG_CONFIG_DIR`, then the XDG
config search path, for `config.yaml`; then an optional `./localrpg.yaml` merged on
top, which flips the local-override flag. `Save` writes to the local override when
one exists and to the user configuration otherwise. `gui.NewService(rootDir)` has
one twist: when `--dir` names something other than `.`, that directory's
`config.yaml` is the user configuration, and relative `paths.*` resolve against it.

Storage paths are resolved from the XDG base directories, with native per-OS
fallbacks. An empty `paths.*` uses the default for its category, an absolute value
is used verbatim, and a relative value joins the category base unless the process
is in project mode.

## 9. Testing and the quality gates

There are two test suites, one runner each, and both have a coverage floor that
sits just below what the suite reaches.

- `mise run test:backend` runs the Go tests with a coverage floor enforced by
  `scripts/check-coverage.sh`. A behaviour-preserving change is argued with
  `mise run coverage:diff`, which compares the functions that stopped being fully
  covered rather than a statement total.
- `mise run test:frontend` runs `tsc --noEmit` and `vitest run --coverage`, with
  the frontend floors configured in `frontend/vitest.config.ts`.
- `mise run test:e2e` runs the browser suite, which lives in `pkg/e2e` behind the
  `e2e` build tag so that a machine without Chrome stays fast. Those tests skip,
  never fail, when no usable browser is found.

Static analysis is a mix of gates and reports. `gofmt`, `staticcheck`, `go vet`,
Prettier, Oxlint, `rumdl`, `goreleaser check`, and `actionlint` gate a build.
Vale, over the user-facing prose, and `revive`, over undocumented exported symbols,
report without failing until their counts are near zero. CodeQL runs in advanced
setup over Go, the frontend, and the workflow files.

## 10. Where a change goes

| The change | Where it goes |
| --- | --- |
| A new model provider | A self-registering package under `pkg/provider`, plus a factory in `pkg/harness`. |
| A new media backend | `pkg/media`, behind the existing client interfaces. |
| A new API endpoint | `pkg/gui` (the method), `server.go` (the route), `types.ts` and `client.ts` (the client), then `-update-dto`. |
| A new user-facing feature | A spec and a plan under `docs/superpowers`, then the engine, then the client. |
| A new guide article | `pkg/gui/docs`, with frontmatter, then `-update-docs`. |
| A new agent role | Configuration only: roles are arbitrary names, and `"gm"` is the engine's one reserved role. |
| A new RPG mechanic | A system's `mechanics.js`, or a new system. Not the engine. |

## 11. A short history

The two dated reviews recorded in this directory are the reasoning behind the
current shape, and each is explicit about the system it measured:

- **`review-2026-09-24.md`** measured package coupling, layering, and the seams,
  and recommended the provider registry, a typed turn context, and OpenTelemetry.
  All three have since landed.
- **`quality-review-2026-10-12.md`** measured readability, coverage, dependencies,
  and CI, and produced the task list the work has been landing against since,
  including the API-surface split, the accessibility work, and the documentation
  coverage this document is part of.

Where the two disagree with this document, this document is the one to trust, and
a fix to it is a contribution in its own right.
