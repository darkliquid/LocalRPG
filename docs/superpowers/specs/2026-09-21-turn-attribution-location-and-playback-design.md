# Design Specification: Turn Attribution, Location Tracking, and Timeline Completeness

**Date:** 2026-09-21  
**Status:** Draft — pending review  
**Topic:** Closing the open questions from the canonical-database and timeline work: a first-class extraction role, per-entity check outcomes, a speech convention the GM is actually told about, the missing player note, dynamic player location with per-turn recording, and playback that finally uses the recorded dialogue

---

## 1. Problem Statement & Motivation

The canonical-database and timeline work shipped the machinery to record what happens in a campaign. Five gaps remain, three of them open questions carried forward from that spec, two of them defects found while closing them.

1. **The extraction role is implicit.** `cmd/localrpg/play.go` reuses the `gm` provider whenever `agents.roles.extractor` is unset, so extraction runs every turn with no way to see it, tune it, or point it at a cheap fast model. The settings UI compounds this: `frontend/src/components/SettingsStudio.tsx` hardcodes `'gm' | 'narrator' | 'evaluator'`, offering a role (`evaluator`) the backend has never routed and omitting the one the engine now reads.
2. **Check outcomes are unanswerable.** `rules.RollResult` carries `Notation`, `Total`, `Successes`, `RollCount`; `rules.ActionResult` carries a bare `Success bool`, and `js_engine.go` interprets exactly three keys (`success`, `message`, `roll`) while everything else lands untyped in `Data`. So a turn records *that* a roll happened but not what the system called its outcome, and "every turn Garrick failed a check in" cannot be asked.
3. **The GM is never told how to write speech.** `dialogue.Parse` attributes a line only when it is written as `Name: "…"`, and nothing in the prompt instructs that shape. Deterministic attribution therefore depends on luck or on the extractor, which is a model call — and the parser misses common forms such as markdown-bold speaker prefixes.
4. **`InitGame` never creates the player note.** `engine.InitGame` copies world templates and creates the opening location, but `games/<id>/entities/<player>.md` is only ever hand-written. `gui.Service.GetGameState` reads that file and fails without it, so a GUI-created campaign cannot serve state; and because `harness.ResolveEntityMentions` skips unknown IDs, the player is silently absent from every turn's involvement list until someone authors the note by hand.
5. **Location is frozen at campaign open.** `NewTurnOrchestrator` takes a `locationID` fixed to the resolved start location, so a turn records where the campaign *began*, never where it *is*. Nothing writes an entity's `location` field after `InitGame`, and `harness.MergeExtractedEntity` deliberately refuses to overwrite an authored location, so no path exists to move the player at all. Without a per-turn location there is no scene key: the export cannot group turns by place, and imagery cannot be tied to a location.
6. **Nothing plays the dialogue it records.** `SynthesizeSegments`, `SynthesizeUtterance`, and `NewTTSPipeline` have no production caller; there is no audio route in `pkg/gui/server.go`, no audio capability in `pkg/tui`, no `new Audio` in `frontend/src`, and `preferences.tts.auto_play` and `master_volume` have no consumer. `Turn.AudioRefs` is populated by nothing.
7. **The art pipeline is unwired and mislabelled.** `media.ImagePipeline.GenerateSceneImage` already computes a content-addressed key and reuses cached bytes, but its only callers are tests and the settings diagnostic, it appends `".webp"` to the key regardless of the bytes returned (the built-in generator produces SVG), and its only variation input is the caller-supplied appearance string.
8. **The database has no migration path.** `pkg/storage/db.go` applies one `CREATE TABLE IF NOT EXISTS` schema on open, with no `PRAGMA user_version`, no `ALTER TABLE`, and no versioning anywhere. Adding a column silently does nothing to an existing campaign database, and the query that reads it then fails at runtime.

---

## 2. Decisions & Non-Goals

Decisions settled by review:

| Area | Decision |
| --- | --- |
| Extraction role | A dedicated `extractor` role exists in the default config, inheriting the `gm` provider until explicitly overridden, selectable in the settings UI |
| Speech convention | Engine-owned, always-on instruction block; no new config toggle |
| Parser breadth | Line-leading forms only, plus markdown emphasis and wikilink wrappers and smart quotes; **no** mid-sentence attribution |
| Check outcomes | A reserved `outcome` key on the object an `onAction` handler returns; stored on the turn and copied onto every entity link for that turn |
| Player note | Created by `InitGame` when absent, linked to the resolved opening location, never overwriting an authored note |
| Location state | The player note's `location` field is the single source of truth, read at the start of every turn and recorded on the turn |
| Movement | A rules host call and a `/go <location>` command are authoritative; extractor-proposed moves apply only to a resolvable location entity that differs from the current one |
| Scene key | Scenes are locations; a scene is a run of consecutive turns sharing one |
| Art variation | An explicit `appearance` field first, then tags and state; the note body is never an input |
| Art generation | Lazily on demand, content-addressed, with an explicit force-regenerate path |
| Art in the GUI | Shown when a turn's scene changes |
| Playback | Per-segment on-demand audio, sequential playback honouring `auto_play`, silent degradation when TTS is unconfigured |
| Playback scope | The whole turn: narration in the narrator voice, speech in each character's voice |
| Export audio | Generated at export time, reusing the cache, skippable (owned by the export spec) |
| Export video | Go raster frames sharing one scene-data model with the web player (owned by the export spec) |
| Video pacing | Reading-time estimation, never fixed spans: words per minute with a character-based fallback, a minimum dwell floor, and audio duration taking precedence when a clip exists (owned by the export spec) |
| Spec split | This spec (attribution, location, playback), then the animated export and video, then GUI turn submission as its own spec |

Non-goals:

- The animated export player and the video renderer. This spec produces the data and the clip machinery they consume.
- GUI turn submission (`localrpg gui` cannot play a turn at all today). Its own spec.
- WebAssembly host parity. `pkg/rules/wasm_engine.go` exports `host_roll` only and has no production caller, so new host calls are JavaScript-only until Wasm is wired.
- Generated art from a remote provider by default. The built-in generator remains the guaranteed offline source.
- Chunked/streaming audio delivery. Clips are short and fetched whole.
- Audio in the TUI, which has no audio capability or dependency today and stays text-only.

---

## 3. The Extraction Role

### 3.1 Configuration

`AgentRoleConfig` (`pkg/config/types.go`) gains one field:

```go
type AgentRoleConfig struct {
	Type        string   `yaml:"type" json:"type"` // "builtin", "http", "cli", "inherit", "disabled"
	InheritFrom string   `yaml:"inherit_from,omitempty" json:"inherit_from,omitempty"`
	// … unchanged fields …
}
```

`DefaultConfig` seeds the role explicitly, so it is visible and editable rather than implicit:

```yaml
agents:
  roles:
    gm:
      type: cli
      command: echo
    extractor:
      type: inherit
      inherit_from: gm
```

`inherit` resolves at load time to the named role's provider, which avoids a trap the alternative would create: seeding `extractor` with a copy of the shipped default (`cli echo`) would leave extraction pointed at `echo` forever, silently producing unparseable output the moment a user pointed `gm` at a real model. With `inherit`, changing `gm` changes extraction until someone deliberately overrides `extractor`.

Resolution order in `cmd/localrpg/play.go:resolveExtractor`:

| `agents.roles.extractor` | Behaviour |
| --- | --- |
| absent | Inherit the `gm` provider (unchanged behaviour, keeps pre-existing config files working) |
| `type: inherit` | Inherit `inherit_from` (defaulting to `gm`); a missing role disables extraction |
| `type: builtin` / `cli` / `http` | Use it |
| `type: disabled` | No model call; deterministic mentions only |

A failure to resolve is not fatal: extraction is skipped and the turn records deterministic mentions, exactly as a failed extractor behaves today.

### 3.2 Settings UI

`SettingsStudio.tsx` stops hardcoding roles. It enumerates `Object.keys(config.agents.roles)`, always offers `extractor` as a suggested role (writing the entry with `type: inherit` when first selected), and drops `evaluator` — a role no backend code has ever routed. Selecting `inherit` exposes the `inherit_from` role as a second dropdown; selecting a concrete type exposes the existing provider fields.

### 3.3 Cost visibility

The extractor runs on every turn by default, so its cost must be legible: the role row shows the resolved provider and model (e.g. “inherits gm → ollama/llama3.2”) and a per-turn token ceiling via the existing `max_tokens` field, which `harness.Extractor.Extract` passes through as `GenerateRequest.MaxTokens`.

---

## 4. Speech Attribution

### 4.1 The instruction

`harness.ContextAssembler` gains a section injected for every turn, after the rules and lore blocks, regardless of system or world:

```
## SPEECH FORMATTING
Write each spoken line on its own line, formatted as  Name: "the words spoken"
Use a character's established name, or [[their note name]] to link them.
Keep narration on its own lines with no leading name. If you cannot name the
speaker, leave the words in the narration instead of inventing a name.
```

Four lines, no configuration. The engine needs one shape it can resolve deterministically; the instruction asks for exactly that and tells the model what to do when it cannot comply.

### 4.2 Parser rules

`pkg/dialogue/dialogue.go` accepts, on a line-start:

| Form | Example |
| --- | --- |
| Plain | `Garrick: "You didn't see me here."` |
| Markdown emphasis | `**Garrick:** "You didn't see me here."` |
| Wikilink | `[[Garrick the Fence]]: "You didn't see me here."` |
| Wikilink with label | `[[Garrick the Fence|Garrick]]: "You didn't see me here."` |
| Smart quotes | `Garrick: “You didn't see me here.”` |

Rules:

- A candidate speaker is trimmed and must be at most 64 characters. There is no punctuation rule: titles and initials such as `Mr. Garrick` are legitimate names, and resolution against the entity index is the real gate, so a sentence fragment like `He turns away. She says` is rejected because no such entity exists rather than because of its punctuation.
- Quotes must be terminated on the same line. An unterminated quote leaves the line as narration.
- Text after the closing quote becomes a narration segment, so `Garrick: "Keep walking." He turns away.` yields speech followed by prose rather than losing the trailing sentence.
- A candidate that does not resolve to an entity stays narration. This is the gate that removed the `As you declare: "I draw my blade."` false positive, and it stays.
- No mid-sentence patterns (`Garrick says, "…"`). Attributing those reliably is the extractor's job; a heuristic here reintroduces exactly the false-positive class the gate exists to prevent.

Attribution precedence is unchanged: extractor-provided `dialogue` entries win for a span they match, the parser fills the rest. With the instruction in place the deterministic path stands on its own, which matters when the extractor is `disabled`.

### 4.3 Extracted entity schema

The extractor's response gains an optional `appearance` field on entities, so a location's visual state can be tracked without a second call:

```json
{
  "entities": [
    { "id": "alden-tavern", "name": "Alden Tavern", "type": "location",
      "appearance": "gutted by fire, roof collapsed, charred beams" }
  ],
  "dialogue": [],
  "player_location": "[[aldon-harbour]]"
}
```

`appearance` is merged like other extracted fields (only onto entities whose field is empty) and feeds Section 7. `player_location` feeds Section 6.

---

## 5. Check Outcomes

### 5.1 Reporting

`rules.ActionResult` gains a label:

```go
type ActionResult struct {
	Success bool
	Outcome string // the system's own vocabulary: "critical_failure", "partial", "clean"
	Message string
	Roll    *RollResult
	Data    map[string]interface{}
}
```

`js_engine.go:ExecuteAction` reads a fourth reserved key, `outcome`, alongside `success`, `message`, and `roll`:

```js
onAction("attack", (ctx) => {
  const r = roll("1d20+5");
  return { roll: r, success: r.total >= 15, outcome: r.total >= 15 ? "clean_hit" : "glancing_blow" };
});
```

The vocabulary belongs to the system, is stored verbatim, and is never interpreted by the engine — the same schema-agnostic rule that governs stats and actions. `success` remains an independent boolean: a script that reports only an `outcome` does not imply success, and the engine will not infer one.

### 5.2 Storage

| Store | Change |
| --- | --- |
| `engine.Turn` | new `Outcome` field, serialised as JSON `outcome` |
| `storage.TurnRecord` | `Outcome string` |
| `turns` table | `outcome TEXT` |
| `turn_entities` table | `outcome TEXT` — the turn's label copied onto every link, so per-entity queries stay single-table |

`turn_entities.outcome` is denormalised on purpose: `ListTurnsForEntity` answers "which turns involved this entity" and `WHERE entity_id = ? AND outcome = 'critical_failure'` answers the question this section exists for, without a join.

Turns with no check (a `Say` turn, a pure narration turn) leave `outcome` empty, and empty means "no system-reported outcome" rather than success or failure.

### 5.3 Surface

`TurnDTO` gains `Outcome string`. `GetEntityTurns` already returns `TurnDTO`s, so the API answers "this entity's turns, with their outcomes" without a new endpoint.

---

## 6. Player Location & Movement

### 6.1 Source of truth

The player note's frontmatter `location:` field is canonical. It already exists (`entity.Entity.Location`), `engine.ResolveStartLocation` already prefers it, and the player note will now actually be created (Section 6.4).

`TurnOrchestrator` stops carrying a fixed `locationID`. Each turn it reads the player entity from the store and uses that note's `location` value, falling back to the game manifest's pinned `start_location` when the player has none. The value is resolved to an entity ID with the existing normalisation. A location that does not resolve falls back to the previous turn's recorded location, so a typo or a deleted note cannot punch a hole in the scene sequence; only a campaign's first turn can record no location at all.

### 6.2 Per-turn recording

| Store | Change |
| --- | --- |
| `engine.Turn` | new `Location` field, serialised as JSON `location`: the entity ID where the turn happened |
| `storage.TurnRecord` | `Location string` |
| `turns` table | `location TEXT` |
| `turn_entities` | unchanged: the location already arrives as `entity.MentionLocation`, now reflecting the real location |

`TurnDTO` gains `LocationID`, `LocationName`, and `LocationArtURL`. Turns recorded before this change have no location; consumers treat empty as "unknown scene" and never group them with each other.

### 6.3 Movement

Three paths, in order of authority:

1. **Rules.** `GameHostAPI` gains `SetLocation(entityID string) error` and `GetLocation() (string, error)`, implemented on `DefaultHostBridge` (validating that the target exists and is a `location`) and bound in `js_engine.go` as `setLocation(id)` / `getLocation()`, following the existing `getStat`/`setStat` pattern. This is how a system moves the party deterministically.
2. **Command.** `/go <location>` in `TurnOrchestrator.ProcessAction`, next to `/undo` and `/gm`: it resolves the argument against indexed locations by ID and by name, updates the player note, and returns a `System`-mode turn recording the move without calling a model. An unresolvable or non-location target returns an error and changes nothing.
3. **Extraction.** `Extraction.PlayerLocation` (from `player_location` in the response) is applied only when it resolves to an existing `location` entity and differs from the current one. This is what makes "I walk down to the docks" work without a command, while the resolvability gate prevents the model from teleporting the party into a place that does not exist.

A move discovered by the rules or the extractor takes effect from the **following** turn, because the current turn happened where it started — `Turn.Location` records the scene the player was in when they acted, not where they ended up. `/go` is the exception: the move *is* the turn, so its record carries the destination. Every applied move is visible either way — in the player note's `location`, in the turn's mention list, and in the following turn's `Turn.Location`. A rejected proposal is simply ignored.

### 6.4 The player note

`engine.InitGame` creates `games/<id>/entities/<player>.md` when it does not exist, after the opening location is resolved:

```markdown
---
id: sean
name: Sean
type: character
location: "[[opening-scene]]"
---

The player character.
```

The ID is the slugified player name, the body line is a placeholder the player is expected to replace, and an existing note is never touched. This fixes GUI-created campaigns failing `GetGameState`, and it means the player is recorded in every turn's involvement from the first turn onward.

A world that ships a note for the same ID wins: the note is only created when the file is absent.

---

## 7. Location Imagery

### 7.1 Variation inputs

The image for a location changes when its appearance changes and is reused otherwise. The hash input is, in order:

1. The location note's `appearance` field when set (authored, or set by the extractor as in Section 4.3).
2. Otherwise the entity's sorted `tags` plus the canonical JSON of its `state`.

The note body is excluded deliberately: `Timeline.stageEntities` appends extracted prose to a matched entity's body on nearly every turn, so hashing the body would regenerate art continuously for no visual reason.

```go
// pkg/media
func AppearanceHash(appearance string, tags []string, state map[string]interface{}) string
```

The final key keeps the existing shape — `ComputeArtCacheKey(locationID, appearanceHash, worldStyleHash)` (`pkg/media/cache.go:23`) — where `worldStyleHash` covers the world's `art_style` and `genre` plus the configured provider and model, so switching models produces a distinctly cached image rather than silently reusing the old one.

### 7.2 Fixes to the existing pipeline

| Defect | Fix |
| --- | --- |
| `GenerateSceneImage` appends `".webp"` to every key (`pkg/media/image.go:38`) although the built-in generator returns SVG | Choose the extension from the produced bytes: an `<svg` signature yields `.svg`, otherwise `.webp`. Cache hits must look for either extension before generating. |
| `procedural_art.go:41` seeds from `int64(len(prompt) * 31)`, so prompts of equal length share a particle layout | Seed from a hash of the prompt, making the seed a real content fingerprint and increasing variety at no cost |
| `BuildPrompt` joins appearance and world style only | Compose from `appearance` when set, otherwise the location's name, the first 200 characters of its body, and its tags, plus the world style |

The generator remains deterministic for identical input, which is what makes the cache correct rather than merely convenient.

### 7.3 Generation policy

Generation is lazy, on first request from a display or export path, keyed as above, and reused thereafter. `media.image.type: disabled` means no provider call; a new `media.image.builtin_fallback` boolean (default `true`) decides whether the built-in procedural generator stands in when no provider is configured or a call fails, so imagery exists offline without overriding an explicit provider choice. Regeneration is available through an explicit force flag (a query parameter on the GUI route, a flag on the export) which bypasses the cache lookup and rewrites the same key.

### 7.4 Surface

`gui.Service` gains `GetLocationArt(ctx, gameID, locationID string, force bool) (string, string, error)` returning a path and content type, exposed as `GET /api/game/{id}/location/{eid}/art` and served with the correct `Content-Type` for SVG or raster. The chronicle requests it when a turn's `LocationID` differs from the previous turn's, so the audience sees the scene change.

---

## 8. Playback

### 8.1 Audio for a segment

```text
GET /api/game/{id}/turn/{n}/segment/{i}/audio
```

| Case | Response |
| --- | --- |
| Clip cached or synthesis succeeds | `200` with the audio bytes and its content type |
| TTS unconfigured or `disabled` | `204 No Content` — the client stays silent, no error surfaced |
| Unknown turn or segment index | `404` |
| Provider failure | `502` with the provider's error, logged once per turn rather than per retry |

The clip is produced by a new `media.TTSPipeline.SynthesizeSegment(ctx, segment, narratorVoice, voiceFor)` helper, with `SynthesizeSegments` delegating to it so playback and export share one implementation: narration uses the narrator's configured voice, speech uses the speaking entity's `voice:` frontmatter, and an unresolved speaker falls back to the narrator voice. Caching keys on speaker, voice, pitch, rate, and text as it already does, so repeated playback and repeated turns are free, and the endpoint never writes to the turn record.

`TurnDTO.Segments[i].AudioURL` is populated with this route whenever a TTS provider is configured, and left empty otherwise so the client renders no controls.

### 8.2 Client behaviour

- When `preferences.tts.auto_play` is true, arriving at a turn plays its segments in order: narration in the narrator voice, each spoken line in its character's voice, with a play/pause control and per-segment click-to-play.
- `preferences.tts.master_volume` is applied to the audio elements.
- The chronicle and the in-app Story Theater share one playback component, so pacing and controls cannot drift between them.

### 8.3 CLI

`cmd/localrpg/media.go` currently prints `Synthesizing TTS: …` and does nothing. It gains a real implementation: synthesize the given text with the configured provider and speaker, write the clip to the media cache, and print its path. That gives the engine a scriptable way to verify a voice configuration and gives the export spec a starting point.

---

## 9. Schema Migrations

Adding the columns in Sections 5.2 and 6.2 to an existing campaign database does nothing today, because `CREATE TABLE IF NOT EXISTS` is a no-op for an existing table and the new queries then fail at runtime. `pkg/storage/db.go` gains a real migration path:

```go
type migration struct {
	version int
	apply   func(*sql.DB) error
}

var migrations = []migration{
	{version: 1, apply: func(db *sql.DB) error { /* turns.location, turns.outcome, turn_entities.outcome */ }},
	{version: 2, apply: func(db *sql.DB) error { /* drop the never-written turns.audio_refs_json */ }},
}
```

- `OpenDB` reads `PRAGMA user_version`, applies every migration above it inside a transaction, then sets `user_version`.
- Each migration is idempotent: columns are added only when `PRAGMA table_info` does not already list them, so a partially applied database converges rather than failing.
- The existing `CREATE TABLE IF NOT EXISTS` schema remains as the version-zero baseline for brand-new databases and is updated to include the new columns, so a fresh database is already at the target shape and the migrations are no-ops for it.
- `ALTER TABLE … DROP COLUMN` is available in the bundled SQLite (`modernc.org/sqlite` v1.59), so the vestigial `audio_refs_json` column is removed rather than left behind.

`Turn.AudioRefs` and `storage.TurnRecord.AudioRefsJSON` are deleted with it. Nothing has ever written them; on-demand cache keys (Section 8) are the only audio path, and keeping a second, always-empty path is exactly the kind of dead surface that misleads the next reader.

---

## 10. Component & File Map

| File | Change |
| --- | --- |
| `pkg/config/types.go` | `AgentRoleConfig.InheritFrom`, `type: inherit`, default `extractor` role, `RoleExtractor` reused |
| `cmd/localrpg/play.go` | `resolveExtractor` honours `inherit`; wires the extractor with the turn's location |
| `pkg/dialogue/dialogue.go` | Emphasis and wikilink speaker forms, smart quotes, trailing-text split, candidate sanity limits |
| `pkg/dialogue/dialogue_test.go` | New forms plus the retained false-positive regressions |
| `pkg/harness/context.go` | Speech-formatting instruction block |
| `pkg/harness/extractor.go` | `Extraction.PlayerLocation`, entity `appearance`, `inherited` provider plumbing |
| `pkg/rules/host_api.go` | `ActionResult.Outcome`, `SetLocation`, `GetLocation` |
| `pkg/rules/js_engine.go` | `outcome` key, `setLocation`/`getLocation` bindings |
| `pkg/engine/orchestrator.go` | Dynamic location from the player note, `/go`, outcome capture, speech-format block |
| `pkg/engine/game.go` | Create the player note when absent |
| `pkg/engine/history.go` | `Turn.Location`, `Turn.Outcome`, drop `AudioRefs` |
| `pkg/engine/timeline.go` | Record location and outcome; copy the outcome onto links |
| `pkg/storage/db.go` | `user_version` migrations; new columns; drop `audio_refs_json` |
| `pkg/storage/turn.go` | `TurnRecord.Location`/`Outcome`, links carry outcomes |
| `pkg/media/image.go` | Extension from produced bytes, cache hit on either extension, richer `BuildPrompt` |
| `pkg/media/procedural_art.go` | Seed from a prompt hash |
| `pkg/media/location_art.go` *(new)* | `AppearanceHash`, location prompt composition, resolve-or-generate helper |
| `pkg/gui/service.go` | Segment audio and location art, DTO fields, player note on create |
| `pkg/gui/server.go` | `…/segment/{i}/audio` and `…/location/{eid}/art` routes |
| `pkg/gui/types.go` | `TurnDTO.LocationID/LocationName/LocationArtURL/Outcome`, `SegmentDTO.AudioURL` |
| `pkg/tui/app.go` | Render the turn's location in the header |
| `cmd/localrpg/media.go` | Implement the `tts` command |
| `frontend/src/components/SettingsStudio.tsx` | Roles from config, `extractor` supported, `evaluator` removed, `inherit` UI |
| `frontend/src/components/ChronicleView.tsx` | Location art on scene change, per-segment playback controls |
| `frontend/src/components/TurnSegments.tsx` | Play/pause and per-segment audio |
| `frontend/src/types.ts` | New DTO fields, `inherit_from` on `AgentRoleConfig` |

---

## 11. Migration & Compatibility

- **Existing databases** migrate on open: new columns appear, `audio_refs_json` is dropped, `user_version` moves forward. A database created before the turn tables existed gains them through the baseline schema and then the migrations.
- **Existing `history.jsonl`** loads unchanged: `location` and `outcome` are absent and treated as unknown. Segments and mentions are unaffected.
- **Existing config files** keep working: an unset `extractor` role still inherits `gm`.
- **Existing campaigns** have no player note. The first turn after upgrade reads the player from the store; if absent, it falls back to the manifest's pinned start location, and the note is created on the next `InitGame` (i.e. a new campaign). A hand-authored player note is never touched.
- **Frontend** tolerates missing fields, since every new DTO field is optional.

---

## 12. Verification & Testing Plan

**Provider resolution (`cmd/localrpg`, `pkg/config`)**
- A role with `type: inherit` resolves to the named role's provider at wiring time; an inherited role that does not exist disables extraction rather than failing the turn.
- A config with no `extractor` entry behaves exactly as before.
- `AgentRoleConfig.InheritFrom` round-trips through YAML and JSON.

**`pkg/dialogue`**
- Every accepted form in Section 4.2 parses into a speech segment with the right speaker and text.
- Markdown-emphasis and wikilink speakers resolve; smart quotes parse.
- `Garrick: "Keep walking." He turns away.` yields speech plus a narration segment.
- An over-long, punctuated, or unterminated candidate stays narration.
- `As you declare: "I draw my blade."` still stays narration (regression).

**`pkg/harness`**
- The speech-formatting block appears in assembled context for a game with no system or world prompt.
- `player_location` and entity `appearance` reach the caller; `appearance` merges only onto an empty field.

**`pkg/rules`**
- A script returning `{ outcome: "glancing_blow" }` reaches `ActionResult.Outcome`; `success` stays false.
- `setLocation` rejects unknown and non-location targets; `getLocation` returns the player's location.

**`pkg/engine`**
- A turn records the player note's location, and the location changes when the note changes.
- `/go` updates the note, records a `System` turn with the new location, and changes nothing on an unresolvable target.
- An extractor-proposed `player_location` that resolves applies; one that does not is ignored.
- `InitGame` creates the player note with the opening location and leaves an existing note alone.
- The player appears in the first turn's mentions of a newly created campaign (the defect this spec exists to fix).

**`pkg/storage`**
- A database created with the pre-migration schema gains `turns.location`, `turns.outcome`, and `turn_entities.outcome` and loses `audio_refs_json`, with existing rows intact.
- Migrations are idempotent: opening twice leaves `user_version` and the schema unchanged.
- `turn_entities.outcome` round-trips and supports filtering by outcome for one entity.

**`pkg/media`**
- `AppearanceHash` is stable for identical tags and state regardless of key order, and changes when the appearance changes.
- The same location and appearance produce a cache hit; a changed appearance produces a new file.
- A generated SVG is stored as `.svg` and reused on the next call; a raster is stored as `.webp`. Both hit the cache on repeat.
- The built-in generator is deterministic for identical prompts and differs for different ones.

**`pkg/gui`**
- The segment audio route returns `200` with a cached clip, `204` when TTS is disabled, and `404` for an unknown index; a second request is served from cache.
- The location art route returns SVG with an SVG content type and honours the force flag.
- `TurnDTO` carries location, location art, and outcome; `EntityDTO` and `GetEntityTurns` expose outcomes per entity.

**Project**
- `go vet ./...`, `mise run test` (Go suite plus `tsc --noEmit`), and every commit builds standalone as checked by a scratch worktree.

---

## 13. Phased Delivery

1. **Migrations and columns** — `user_version` mechanism, new columns, `audio_refs_json` removal with its dead fields. Independently shippable and a prerequisite for everything below.
2. **Player location** — the created player note, dynamic location from the note, `/go`, extractor-proposed moves, per-turn recording, and the index column.
3. **Check outcomes** — `ActionResult.Outcome`, the JS key, turn and link storage, DTOs.
4. **Extraction role** — `inherit`, the default entry, resolver changes, and the settings UI rework including the `evaluator` removal.
5. **Speech attribution** — the instruction block and the broadened parser.
6. **Imagery** — appearance hashing, the pipeline fixes, the location art route, and the chronicle's scene-change art.
7. **Playback** — the segment audio route, client playback with `auto_play` and `master_volume`, and the `tts` command.

---

## 14. Open Questions

None outstanding. The remaining decisions live in the export spec (reading-time constants, scene transitions, bundle layout, raster typography) and the GUI-play spec (streaming transport, orchestrator lifetime in the service).
