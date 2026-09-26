# AGENTS.md

LocalRPG is a local-first, turn-based tabletop RPG client: a single Go binary that runs an in-process pure-Go desktop GUI (`go-shirei`), plus media generation and story export. Module path: `github.com/darkliquid/localrpg`. The engine is **schema-agnostic** — no HP/Mana/classes are hardcoded anywhere; all RPG state is opaque YAML frontmatter plus sandboxed JS/Wasm hooks.

## Commands

Toolchain is pinned by `mise.toml` (Go 1.27.1). Prefer `mise run`, but plain `go` works if the toolchain is already active.

```bash
mise run setup            # go mod download
mise run build            # build bin/localrpg
mise run build:backend    # go build ./cmd/localrpg
mise run test             # go test -v -count=1 ./...
mise run lint             # go vet ./...
mise run dev:gui          # go run ./cmd/localrpg
mise run desktop:snapshots # regenerate shirei golden snapshots
mise run desktop:build    # CGO-free cross-compile for linux/windows/macos
mise run desktop:png      # render one GUI frame to a PNG
mise run clean
```

Run a single Go test: `go test -run TestTurnOrchestrator ./pkg/engine/`.

CLI surface: `localrpg` with no command opens the GUI. Flags: `--dir <path>`, `--png <path>`, `--version`. Commands: `roll <notation>`, `prompt`, `tts`, `image`, `export <web|video>`, `version`.

## Architecture

> The desktop GUI is in-process (`pkg/desktop`) on `go.hasen.dev/shirei`; the application core (`pkg/gui`) is transport-free and `pkg/theater` is the shared theatre view used by both the live window and the video exporter. Keep the default binary buildable with `CGO_ENABLED=0`: any new dependency that needs cgo must be behind an opt-in build tag.

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

GUI: `pkg/gui.Service` (`service.go`) is the transport-free application core. The desktop GUI lives in `pkg/desktop` (built on `go.hasen.dev/shirei`) and calls the core in-process; `pkg/theater` holds the shared theatre view used by both the live window and the video exporter. Turns are serialised per campaign by `Service.BeginTurn`, and provider setup lives in `pkg/harness.RouterFromConfig`/`ExtractorFromConfig` rather than in `package main`.

Runtime: `cmd/localrpg` boots the GUI from the root command (see `cmd/localrpg/gui.go`, `bootDesktop`). `--png <path>` renders a single frame and exits.

## Provider model (uniform across LLMs and media)

Every AI/LLM and media backend (LLM agents, TTS, STT, image) uses the same config shape: `type` is one of `builtin`, `cli`, `http`, `disabled` (plus `mock` for LLMs), with `builtin_name` selecting an in-process implementation. Factories: `harness.NewModelProvider` (`pkg/harness/factory.go`), `media.NewTTSClient`/`NewSTTClient`/`NewImageClient` (`pkg/media/providers.go`).

Notable built-ins that need no server or GPU: `narrative-oracle` (LLM), `native-os` (TTS via `spd-say`/`say`/PowerShell), `procedural-art` (pure-Go SVG image generator). `harness.Router` maps role name -> provider ID with optional per-role fallbacks; roles are arbitrary strings ("gm" is the only one the orchestrator hardcodes; "narrator" exists only as a config default).

## Configuration

Resolution order (`pkg/config/manager.go`): `$LOCALRPG_CONFIG_DIR`, else `$XDG_CONFIG_HOME/localrpg`, else `~/.config/localrpg`, for `config.yaml`; then an optional `./localrpg.yaml` merged on top, which flips `IsLocalOverride`. `Save` writes to the local override when one exists, otherwise to the user config. `gui.NewService(rootDir)` has its own twist: when `--dir` is set to anything other than `.`, it treats `<rootDir>/config.yaml` as the user config and resolves relative `paths.*` against `rootDir`.

## Conventions

- Go: standard library only for tests (`testing`, `t.TempDir()`); no testify. Errors wrapped with `fmt.Errorf("...: %w", err)`. Prefer `any` over `interface{}`, and current-Go idioms the pinned toolchain affords (`min`/`max`, `slices`/`maps`, `for i := range n`, typed `sync/atomic`, `errors.Join`, `log/slog`, `clear`). `go vet` must stay clean.
- Identifiers: `entity.Slugify` (display name -> kebab-case ID) and `entity.WikilinkTarget` (unwraps `[[target|label]]`) are the shared helpers — reuse them instead of writing local slug/link parsing.
- Commits: Conventional Commits with a scope, e.g. `feat(harness): …`, `fix(desktop): …`, `docs: …`. Keep the subject under 72 chars.
- Design work is spec-first: `docs/superpowers/specs/` holds approved design docs and `docs/superpowers/plans/` holds task-by-task implementation plans with `- [ ]` checkboxes, including a "File Map" listing files to create/modify per feature. Read the relevant spec before changing a subsystem; the plans reference the superpowers skills workflow. Note `.superpowers/` is gitignored while `docs/superpowers/` is tracked.

## Gotchas

- Media cache keys must incorporate voice identity *and* prosody: `ComputeAudioCacheKeyWithRate` (speaker, voice ID, pitch, speech rate, text) exists precisely to avoid cache collisions between characters sharing a voice. Use it rather than the older `ComputeAudioCacheKey`.
- `AssignVoiceProfile` (`pkg/harness/extractor.go:64`) resolves NPC voices in order: literal profile-ID match in name/body, tag scoring, then a deterministic FNV hash of the entity ID. Voice profiles are only assigned to entities of type `character` and never overwrite an existing `Voice`.
- `storage.Sync` silently skips malformed Markdown files (missing `---` frontmatter or unparsable YAML) — a missing entity usually means a frontmatter parse failure, not a sync bug.
- Per-turn extraction reuses the `gm` provider unless `agents.roles.extractor` names another one; set that role to `disabled` to record deterministic mentions only. A failed extractor never loses the turn, and `harness.ResolveEntityMentions` needs no model at all.
- The extractor is only reached through `Timeline.RecordTurn`. Calling `harness.Extractor.Extract` directly returns records without persisting anything, by design.
- An entity note is written as `<id>.md` because `gui.Service.GetGraph` derives node IDs from file names. Template notes copied from a world are renamed to their frontmatter ID on game creation for the same reason.
- `gui.Service.ensureIndexed` repairs a campaign's index once per process, the first time that process serves the game. Timeline queries (`GetEntityTurns`) answer from the database; chronicle reads still come from `history.jsonl`, which is where the segments live.
- The desktop app plays turns in-process through `Service.BeginTurn`; turns are serialised per campaign, and nothing is persisted for a cancelled turn. Provider setup lives in `pkg/harness.RouterFromConfig`/`ExtractorFromConfig` rather than in `package main`.