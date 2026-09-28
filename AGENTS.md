# AGENTS.md

LocalRPG is a local-first, turn-based tabletop RPG client: a single Go binary that runs a Wails v3 desktop GUI, a Bubbletea terminal TUI, an HTTP/Unix-socket API daemon, plus media generation and story export. Module path: `github.com/darkliquid/localrpg`. The engine is **schema-agnostic** — no HP/Mana/classes are hardcoded anywhere; all RPG state is opaque YAML frontmatter plus sandboxed JS/Wasm hooks.

## Commands

Toolchain is pinned by `mise.toml` (Go 1.27.1, Node 26.9.0). Prefer `mise run`, but plain `go`/`npm` work if the toolchain is already active.

```bash
mise run setup          # go mod download + cd frontend && npm install
mise run build          # frontend bundle -> pkg/gui/dist, then bin/localrpg
mise run build:frontend # npm run build in frontend/ (tsc + vite)
mise run build:backend  # depends on build:frontend
mise run test           # go test -v -count=1 ./...  AND  npx tsc --noEmit
mise run test:backend   # go test -v -count=1 ./...
mise run test:frontend  # npx tsc --noEmit (in frontend/)
mise run lint           # markdownlint on pkg/gui/docs, then go vet ./...
mise run lint:docs      # markdownlint-cli2 on the embedded help articles
mise run dev:gui        # go run ./cmd/localrpg gui --port 8080
mise run dev:frontend   # vite dev server on :3000, proxies /api -> localhost:8080
mise run clean
```

Run a single Go test: `go test -run TestTurnOrchestrator ./pkg/engine/`.

Regenerate the generated embedded docs (provider catalogue, config reference)
after changing a provider, preset, or config struct:
`go test ./pkg/gui -update-docs`.

CLI surface (`localrpg <cmd>`): `roll <notation>`, `prompt`, `play <game-id>`, `tts`, `image`, `gui`, `export <web|video>`, `debug <test-run|server>`, `version`.

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

Context prompt layering lives in `pkg/harness/context.go:AssembleContextWithProfiles`: system rules, world lore, voice-profile catalog, then scene scope, living-world arcs, present characters, and the player action (entities reachable from the current location via `edges`).

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

## Conventions

- Go: standard library only for tests (`testing`, `t.TempDir()`); no testify. Errors wrapped with `fmt.Errorf("...: %w", err)`. Use `interface{}`, not `any` — the codebase is uniform on this even though gopls suggests otherwise; `go vet` must stay clean.
- Identifiers: `entity.Slugify` (display name -> kebab-case ID) and `entity.WikilinkTarget` (unwraps `[[target|label]]`) are the shared helpers — reuse them instead of writing local slug/link parsing.
- TypeScript: React 19 + Tailwind v4 (config lives in CSS via `@import "tailwindcss"` in `frontend/src/index.css`, there is no `tailwind.config.js`). `tsconfig.json` has `strict`, `noUnusedLocals`, `noUnusedParameters`, so `npm run build`/`tsc --noEmit` fails on unused imports — that is the frontend lint gate. Components live in `frontend/src/components/`, icons come from `lucide-react`.
- Commits: Conventional Commits with a scope, e.g. `feat(harness): …`, `fix(frontend): …`, `docs: …`. Keep the subject under 72 chars.
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