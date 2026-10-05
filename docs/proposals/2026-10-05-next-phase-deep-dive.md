# Next Phase: Deep Dive and Proposals

**Date:** 2026-10-05
**Status:** Proposal (research + recommendations; no code changed, no spec written)
**Scope:** `pkg/provider`, `pkg/config`, `pkg/harness`, `pkg/media`, `pkg/scene`, `pkg/rules`,
`pkg/engine`, `pkg/turnstream`, `pkg/gui`, `pkg/export`, `pkg/embeddings`, `pkg/models`,
`frontend`
**Method:** direct reads of the current tree with `file:line` citations, plus the prior research
in `docs/proposals/` and the specs under `docs/superpowers/specs/`. Every "today" claim below was
checked against the code at the time of writing.

This document is the intake for the next phase. It does not approve anything and it does not
design any single feature; it maps each of the eleven directions to what already exists, where the
gaps are, and a numbered set of proposals that can be taken, one at a time, through the
brainstorming → spec → plan flow. Proposal IDs are stable (`MP-1`, `LF-3`, `SYS-2`, …) so later
specs can cite them.

---

## 0. How to read this

- **Part A** is the summary: a single table of every proposal with effort, value, and dependencies,
  then a recommended wave ordering.
- **Parts B-F** are the deep dives, grouped by theme:
  - **B. Provider breadth** — multiple providers of one type (topic 1), zero-GPU/local-first
    offerings (topic 2).
  - **C. Systems depth** — richer mechanics and rolls (topic 3), interactive rolls (topic 11).
  - **D. Presentation** — scene images from turns (topic 4), placeholder/fallback variety
    (topic 7), theatre and export efficiency (topic 10).
  - **E. Robustness** — malformed LLM output and doubled speech (topic 5).
  - **F. Content lifecycle** — packaging and registries (topic 6), AI world generation (topic 8),
    AI system generation (topic 9).
- Each topic has the same shape: **Today** (facts), **Gaps**, **Proposals**, **Risks and open
  questions**.
- Effort is `S` (days), `M` (one to two weeks), `L` (multi-week, likely its own spec). Value is
  relative, not a promise of priority.

A note on what is already settled: the provider-key identity work
(`docs/superpowers/specs/2026-09-28-provider-key-identity-design.md`) is largely implemented, the
mechanics engagement ladder (`off`/`auto`/`ask`) is implemented, advancement landed, and the turn
protocol is the progressive `@`-record stream (`2026-10-03-progressive-turn-stream-design.md`).
Anything below that builds on those assumes they stay.

---

## Part A — Proposals at a glance

### A.1 Full list

| ID | Proposal | Topic | Effort | Value | Depends on |
|---|---|---|---|---|---|
| MP-1 | Named provider instances per family (config as a map with a default selector) | 1 | L | High | — |
| MP-2 | User-chosen instance discriminator in the canonical key | 1 | M | High | — |
| MP-3 | Per-purpose provider routing (narrator vs NPC vs scene vs portrait) | 1 | M | High | MP-1 |
| MP-4 | Ordered provider chains with selection rules (cheapest / first-that-works / by-tag) | 1 | M | Medium | MP-1, MP-2 |
| MP-5 | Provider manager UI (list, duplicate, remove, "used by") | 1 | M | High | MP-1 |
| LF-1 | ONNX embedding provider (bge-small class) behind the model manager | 2 | M | High | — |
| LF-2 | Narrative-oracle as a real state-aware expert system | 2 | M | Medium | SYS-1 |
| LF-3 | Honest capability tiers and caveats in the catalogue and docs | 2 | S | High | — |
| LF-4 | One-click "fully offline" preset bundle | 2 | S | Medium | MP-1, LF-3 |
| LF-5 | Local-first onboarding page with a capability matrix | 2 | S | Medium | LF-3 |
| SYS-1 | Check modifiers: skills, per-check bonuses, resolution expression | 3 | M | High | — |
| SYS-2 | Named resolution profiles (PbtA ladder, d20+DC, dice pool, position/effect) | 3 | L | High | SYS-1 |
| SYS-3 | Surface success-count and position/effect in results and the UI | 3 | M | Medium | SYS-2 |
| SYS-4 | Opposed rolls | 3 | M | Medium | SYS-1 |
| SYS-5 | GUI mechanics editor (stats, skills, health, checks, advancement) | 3 | L | High | — |
| SYS-6 | Reference systems corpus (PbtA, d20-lite, Blades-lite) plus tests | 3 | M | High | SYS-1, SYS-2 |
| SYS-7 | Deterministic system test harness (scripted actions → asserted rolls/state) | 3 | M | High | SYS-5 |
| SYS-8 | Housekeeping: wire or delete `Verdict`/`dismissed_checks`, dedupe `newCheckID`, fix doc drift | 3 | S | Medium | — |
| IR-1 | Resolve-pending-check endpoint that continues a paused turn | 11 | L | High | SYS-1 |
| IR-2 | Roll card UI (stakes, difficulty, Roll button, Argue) | 11 | M | High | IR-1 |
| IR-3 | Turn-resume stream protocol | 11 | L | High | IR-1 |
| IR-4 | Manual/physical roll entry with a recorded source | 11 | S | Medium | IR-1 |
| IR-5 | Renegotiation flow (player proposes stakes/difficulty) | 11 | M | Medium | IR-2 |
| IR-6 | Route `Roll` mode through the interactive path | 11 | S | Medium | IR-1 |
| IMG-1 | Turn-aware scene prompt (narration + action + entities + outcome tone) | 4 | M | High | — |
| IMG-2 | Configurable image trigger policy with a significance heuristic | 4 | M | High | IMG-1 |
| IMG-3 | Scene consistency (seed/style bible, optional img2img reference) | 4 | M | Medium | IMG-1 |
| IMG-4 | Image cost guardrails (budget, preview/approve) | 4 | S | Medium | IMG-2 |
| IMG-5 | Carry turn scene illustrations into exports | 4 | M | Medium | IMG-1 |
| PH-1 | Expand procedural art (genre palettes, structures, time-of-day, weather) | 7 | M | Medium | — |
| PH-2 | Procedural portrait variety (species, archetype, expression from state) | 7 | M | Medium | PH-1 |
| PH-3 | Layered SVG scenes for parallax and theatre | 7 | M | Medium | PH-1 |
| PH-4 | Genre-aware text/gradient/empty-state fallbacks | 7 | S | Medium | — |
| PH-5 | Style packs (user-supplied palettes) | 7 | S | Low | PH-1 |
| TH-1 | Theatre perf: event-driven beats, prefetch, tighter gaps | 10 | M | High | — |
| TH-2 | Richer transitions and effects (Ken Burns, parallax, mood tint) | 10 | M | Medium | PH-3 |
| TH-3 | Visible missing-audio handling and reading-speed pacing | 10 | S | High | — |
| TH-4 | Captions/subtitles track in theatre and export | 10 | M | Medium | TH-3 |
| TH-5 | Export improvements (chapters, subtitles, compression, transitions) | 10 | M | Medium | TH-4, IMG-5 |
| RB-1 | Record repair layer (never silently drop `@persona`/`@roll`) | 5 | M | High | — |
| RB-2 | Guaranteed single-play: unify streamed and finalised clip keys | 5 | M | High | — |
| RB-3 | Authoritative playback ledger shared by server and client | 5 | M | High | RB-2 |
| RB-4 | Bounded malformed-response retry with a repair instruction | 5 | M | Medium | — |
| RB-5 | Diagnostics for dropped/degraded records in the UI and Debug panel | 5 | S | Medium | RB-1 |
| RB-6 | Fuzz/property tests for the parser and group planner | 5 | S | Medium | — |
| PKG-1 | Content package format (`.lrpgpack`: manifest, checksums, assets) | 6 | L | High | — |
| PKG-2 | Export/import routes and studio UI | 6 | M | High | PKG-1 |
| PKG-3 | Versioning, dependencies, and a per-campaign lockfile | 6 | M | High | PKG-1 |
| PKG-4 | Registry client (index, download, verify, install, update) | 6 | L | High | PKG-1, PKG-3 |
| PKG-5 | Provenance: checksums and optional signatures | 6 | M | Medium | PKG-1 |
| PKG-6 | Editing content outside the app (documented interchange format) | 6 | S | Medium | PKG-1 |
| WG-1 | Whole-world generation pipeline (outline → factions/locations/characters) | 8 | L | High | PKG-1 |
| WG-2 | Batch entity generation with cross-links | 8 | M | High | WG-1 |
| WG-3 | Enhance-existing-world flow with diff review | 8 | M | Medium | WG-1 |
| WG-4 | Source ingestion (folder and URL import → world) | 8 | L | Medium | WG-1 |
| WG-5 | Draft-world review UI (accept/reject per entity) | 8 | M | High | WG-1 |
| WG-6 | Cost/latency controls and a deterministic fallback | 8 | S | Medium | WG-1 |
| SG-1 | Whole-system generation (mechanics schema + `mechanics.js` + `rules.md`) | 9 | L | High | SYS-5, SYS-7 |
| SG-2 | Schema-first generation (declarative mechanics, JS only for hooks) | 9 | M | High | SG-1 |
| SG-3 | Generated-system smoke test gate before save | 9 | M | High | SYS-7 |
| SG-4 | Enhance/explain an existing system | 9 | M | Medium | SG-1 |
| SG-5 | Base-system catalogue to derive from | 9 | S | Medium | SYS-6 |

### A.2 Recommended waves

The ordering follows dependency and risk, not excitement. Each wave is independently shippable.

**Wave 1 — Foundations (unblock the rest).**
`RB-1`, `RB-2`, `RB-5` (correctness and diagnosis first: a doubled line or a silently dropped roll
erodes trust faster than any missing feature). `SYS-1` and `SYS-8` (the resolution model is the
substrate for interactive rolls and system generation). `MP-2` (instance identity is the substrate
for multi-provider and for local-first presets). `LF-3` (caveats cost almost nothing and prevent
disappointment). `PH-1` (better placeholders backfill every scene that has no image).

**Wave 2 — Player-facing depth.**
`MP-1`, `MP-3`, `MP-5` (multiple providers, routed by purpose, managed in the UI). `SYS-2`,
`SYS-3`, `SYS-5`, `SYS-6`, `SYS-7` (resolution profiles, the mechanics editor, and the test
harness, so systems can be authored and exercised). `IR-1`, `IR-2`, `IR-3`, `IR-6` (interactive
rolls end to end).
`IMG-1`, `IMG-2` (turn-aware images with a policy). `TH-1`, `TH-3` (theatre responsiveness and
missing audio).

**Wave 3 — Content lifecycle and generation.**
`PKG-1` through `PKG-6` (packages and a registry), then `WG-1`..`WG-6` and `SG-1`..`SG-5` on top
of them. `LF-1`, `LF-2`, `LF-4`, `LF-5` (local-first depth once multi-provider and systems are in).

**Wave 4 — Polish and reach.**
`IMG-3`, `IMG-4`, `IMG-5`, `TH-2`, `TH-4`, `TH-5`, `PH-2`, `PH-3`, `PH-4`, `PH-5`, `RB-3`,
`RB-4`, `RB-6`, `SYS-4`, `IR-4`, `IR-5`, `MP-4`.

**Deliberately deferred:** `WG-4` (scraping) is the most likely to produce bad content and the
hardest to make polite and reliable; ship folder import first and treat URL ingestion as a
separate spike. `MP-4` (selection rules) only earns its complexity once there are enough
providers to choose between.

---

## Part B — Provider breadth

### 1. Multiple providers of the same type

#### Today

- **LLM providers are keyed by role, not by provider.** `agents.roles` is
  `map[string]AgentRoleConfig` (`pkg/config/types.go:48`) and only four role names are consumed:
  `gm`, `narrator`, `extractor`, `completion` (`pkg/config/types.go:19-24`). A role config carries
  exactly one provider (`pkg/config/types.go:26-44`). `RouterFromConfig` builds one provider
  **named after the role** and assigns the role to it (`pkg/harness/factory.go:155-202`).
- **Media is singleton per family.** `MediaConfig` holds exactly one `TTSConfig`, one `STTConfig`,
  one `ImageConfig` (`pkg/config/types.go:224-228`), and none of those structs has an `id` or
  `name` field (`pkg/config/types.go:134-221`). Only one TTS, one STT, and one image provider can
  be active at a time.
- **Embeddings is the sole named-map family.** `EmbeddingsConfig.Providers
  map[string]EmbeddingProviderConfig` keyed by a user-chosen name, selected by
  `EmbeddingsConfig.Provider` (`pkg/config/types.go:292-308`). It is the precedent to copy.
- **Instance keys exist but are not user-controllable.** The canonical key grammar is
  `family:adapter` or `family:adapter@discriminator` (`pkg/provider/key.go:11-14`), and the
  discriminator is derived **only** from the endpoint host (`HostDiscriminator`,
  `pkg/provider/key.go:116`) or the CLI command basename (`CommandDiscriminator`,
  `pkg/provider/key.go:133`). Two configs that share an endpoint (for example two API keys against
  the same host) collapse to the same key and are indistinguishable.
- **The registry and capability model are solid.** `pkg/provider` is a leaf with self-registering
  adapters, a `Descriptor` (family, features, tunables, presets) and a drift guard
  (`pkg/provider/provider.go:17-97`, `pkg/provider/descriptor.go:65-74`,
  `pkg/harness/capabilities.go:17-32`). Presets exist both on descriptors and in
  `pkg/config/presets.go`.
- **Shared credentials are singular per vendor** (`providers.gemini.api_key`,
  `providers.inworld.api_key`, `providers.cartesia.api_key`, `pkg/config/types.go:256-290`), with a
  fixed precedence chain (role/media field → shared → environment) resolved in
  `harness.ResolveGeminiAPIKey` (`pkg/harness/gemini_key.go:15`) and `media.SharedProviderKey`
  (`pkg/media/exports.go:64-77`). There is no `env:` interpolation syntax.

#### Gaps

1. **You cannot run two TTS providers at once.** A campaign cannot use a premium voice for the
   narrator and a free local one for incidental NPCs; it cannot use a local model for bulk lines
   and a cloud model for the lines that matter.
2. **You cannot run two image providers at once.** There is no way to use a cheap procedural or
   fast model for placeholders and a premium model for the scene that opens an act.
3. **You cannot run two LLM instances for cost routing.** The `extractor` and `completion` roles can
   already point elsewhere, but there is no way to say "use the cheap key for extraction and the
   good key for the GM" if both are the same vendor and endpoint, because the instance key is the
   same.
4. **No user-facing identity.** A provider instance has no name the user chose, so usage, pricing,
   and failure reports cannot tell two same-endpoint configs apart
   (`docs/superpowers/specs/2026-09-28-provider-key-identity-design.md` §3.3 is the cause).
5. **Per-purpose routing is hardcoded.** Media is selected globally, so "narrator voice" and "NPC
   voice" share one provider, and "scene image" and "portrait" share one provider.

#### Proposals

**MP-1 — Named provider instances per family.** Reshape each media family from a singleton struct
to a named map plus a default selector, mirroring `EmbeddingsConfig`:

```yaml
media:
  tts:
    default: narrator
    providers:
      narrator: { type: builtin, builtin_name: elevenlabs, api_key: "...", default_voice: "..." }
      npc:      { type: builtin, builtin_name: sherpa-onnx, model_path: "..." }
  image:
    default: procedural
    providers:
      procedural: { type: builtin, builtin_name: procedural-art }
      hero:       { type: builtin, builtin_name: gemini, api_key: "..." }
```

Keep a one-entry migration that lifts the old singleton into `providers["default"]` so existing
`config.yaml` files keep working, and bump `config.Version`. Add `config.Validate()` rules that
every `default` names an existing entry and that names are non-empty. Effort `L`; this is the
single largest config change in this document and should be its own spec.

**MP-2 — User-chosen instance discriminator.** Extend the key grammar so a config can carry an
optional `id`, producing `family:adapter@<id>`, and keep `HostDiscriminator`/`CommandDiscriminator`
as the fallback when no id is set. `provider.NewInstanceKey` and `InstanceOrSelf`
(`pkg/provider/key.go:61,73`) gain an id argument; the four resolvers
(`harness.KeyFor`, `media.TTSKeyFor`/`STTKeyFor`/`ImageKeyFor`, `embeddings.KeyFor`) thread it
through. This makes the usage ledger, pricing, and rate-limit keys distinguish two configs of the
same adapter. It also gives a stable cache namespace for local model instances.

**MP-3 — Per-purpose provider routing.** Introduce named **uses** for media: `narrator`, `npc`
(optionally per-entity via the existing voice profile), `scene`, `portrait`, `placeholder`. Each
use names a provider instance (or falls back to the family default). This is the mechanism that
makes MP-1 useful: without it, multiple providers are configured but never selected. Reuse the
`RoleRoutingConfig`/`Router` shape for consistency. Entity `voice.provider` and the per-entity
voice profile already provide a per-entity override for TTS; extend the same idea to image.

**MP-4 — Ordered provider chains with selection rules.** Let a role or use declare an ordered list
of instances and a selection rule: `first` (current fallback behaviour), `cheapest` (by the
pricing ledger), `local-first` (prefer `FeatureOffline`, fall back to metered), or `by-tag`. This
turns the existing single fallback (`Router.fallbacks`, `pkg/harness/router.go:122`) into a policy.
Defer until there are enough configured providers to matter.

**MP-5 — Provider manager UI.** The Settings Studio today edits one role at a time and one
singleton per media family (`frontend/src/components/SettingsStudio.tsx`). Add a per-family list
with add / duplicate / remove / rename, a default selector, a "used by" view (which roles and uses
resolve to this instance), and a live key-presence and capability badge from
`/api/providers` and `/api/tts/inspect`. Add a duplicate action because "same provider, different
key" is the common case.

#### Risks and open questions

- The config migration is the risky part: users have hand-edited `config.yaml`. The migration must
  be one-way and lossless, and `Save` must not silently drop unknown keys.
- Two instances of the same adapter with the same endpoint but different keys: does the key need to
  incorporate a key fingerprint to avoid cache collisions on generated media? Probably yes for
  audio (voice settings differ), and it argues for MP-2 landing before any per-instance caching.
- Cost routing needs the pricing ledger to be able to price an instance, which the current schema
  supports only for published rates (`docs/proposals/2026-09-28-provider-costs-and-usage-research.md`).

---

### 2. Zero-GPU / local-first offerings

#### Today

- **True offline built-ins exist but are shallow.** `llm:narrative-oracle` is a deterministic
  template engine (`pkg/provider/oracle/provider.go:65-132`) that parses a mechanics tier and
  picks an opener; `tts:native-os` shells out to `say`/`espeak-ng`/`spd-say`/PowerShell and always
  falls back to a sine tone (`pkg/provider/ttsnativeos/native.go:21-83`); `image:procedural-art`
  is keyword-palette SVG (`pkg/media/procedural_art.go:17-102`); `stt:web-speech` is browser-only
  and unavailable in the Wails webview (`pkg/provider/sttwebspeech/sttwebspeech.go:17`);
  `embedding:builtin` is an FNV hash projection, not a model (`pkg/embeddings/builtin.go:13-108`).
- **The only in-process neural model is Sherpa-ONNX Kokoro TTS** (`pkg/provider/ttssherpa/client.go`),
  managed by `pkg/models/manager.go` with a pinned SHA-256, on-demand download, and SSE progress
  (`pkg/models/manager.go:26-195`). It is the template for any new ONNX model.
- **No ONNX embedding, no ONNX LLM, no ONNX image.** The embedding spec proposed a local ONNX
  model (`docs/superpowers/specs/2026-09-25-semantic-search-and-embeddings-design.md:25`) but it
  was never implemented; `embedding:builtin` is the fallback.
- **Capability vocabulary exists.** `FeatureOffline`, `FeatureKeyRequired`, `FeatureMetered`, and
  `Descriptor.Source` (`builtin`/`cli`/`http`/`gemini`) classify every provider
  (`pkg/provider/descriptor.go:17-74`), and `media.Describe` derives capabilities from the real
  interfaces so a descriptor cannot over-claim (`pkg/media/capabilities.go:19-40`).
- **Docs cover the local stacks** (`pkg/gui/docs/14`..`19`: Ollama, Kokoro, Fish Audio, Whisper,
  ComfyUI, docker-compose), but there is no page that says plainly what the zero-GPU path can and
  cannot do.

#### Gaps

1. **`narrative-oracle` is not playable.** It ignores entity state, declared stats, the scene, and
   the world; its output is a handful of openers. Calling it "a storyteller" in the catalogue
   over-sells it.
2. **No local embedding model.** Semantic search and recall degrade to the hash projection, which
   has no semantic generalisation. A small ONNX embedding model is cheap, CPU-only, and
   high-value.
3. **No honest caveats.** The UI shows "Zero-GPU" as a badge but never explains that this is not
   comparable to a 7B local model or a cloud provider, so a new user forms the wrong expectation.
4. **No coherent "fully offline" story.** A user who wants zero network calls must assemble
   oracle + native-os + procedural-art + builtin embeddings by hand, with no preset and no
   guarantee that nothing else reaches out.
5. **Local image generation is genuinely hard** and should not be promised: an ONNX text-to-image
   model is large and slow on CPU. The honest answer is better procedural art (topic 7) plus
   documented `image:cli`/`image:http` local servers.

#### Proposals

**LF-1 — ONNX embedding provider.** Add `embedding:onnx` backed by a small model (for example a
bge-small class encoder, 384 dims) loaded in-process, reusing `pkg/models` for download and
verification and the same "model missing" SSE event as Sherpa. Register it in `pkg/provider` with
`FeatureOffline`, and add it to the embedding factory (`pkg/embeddings/factory.go:13-80`). This is
the highest-value local-first item because it improves an existing feature (recall) rather than
adding a new one, and the runtime is already vendored.

**LF-2 — Narrative-oracle as a real expert system.** Replace the template engine with a small,
deterministic, state-aware narrator: read the player's declared stats and relevant entity state
(the host bridge already exposes `ListStats`/`ListSkills`/`CheckConventions`,
`pkg/rules/host_api.go:206-227`), use the mechanics result and the scene entities, and assemble
prose from a richer grammar (openers, consequences per outcome tier, entity-specific lines,
location flavour). It stays pure Go and offline. It will never write like a model; the goal is
"coherent and stateful", not "good".

**LF-3 — Honest capability tiers and caveats.** Add a coarse tier to the descriptor or derive it:
`offline-basic` (rule/template, no model), `offline-neural` (in-process ONNX), `local-server`
(needs a local daemon), `cloud` (needs a key). Surface the tier as a badge with a one-line caveat
in the provider catalogue, the preset picker, and the docs, and state plainly what each tier is
not. This is small and prevents the biggest disappointment vector.

**LF-4 — One-click offline preset bundle.** A single preset that configures a fully local stack
(oracle + native-os or Sherpa + procedural-art + builtin/ONNX embeddings) with one action, plus a
"verify offline" check that flags any configured provider whose descriptor is not `FeatureOffline`.
Depends on MP-1 for the multi-instance config.

**LF-5 — Local-first onboarding page.** A docs page (`pkg/gui/docs/`) and a launcher entry that
present a capability matrix: for each of LLM/TTS/STT/image/embedding, what runs with no GPU, what
runs with a GPU, what needs a server, and what needs a key, with expected quality and speed. Wire
it into the same site generation as the other articles.

#### Risks and open questions

- ONNX runtime bloat and CGO: Sherpa already pulls per-OS libraries (`go.mod:85-87`). A second
  ONNX runtime or a second model in the same runtime needs a size and licensing check. Reusing
  Sherpa's runtime is preferable to adding `onnxruntime-go`.
- The oracle rewrite is easy to over-invest in. Time-box it and keep the bar at "coherent".
- "Fully offline" must be verifiable, not aspirational; a check that inspects descriptors is worth
  more than a marketing claim.

---

## Part C — Systems depth

### 3. Systems: complex state, richer rolls

#### Today

- **The rules runtime is capable.** `JSEngine` binds `roll`, `getStat`/`setStat`,
  `getLocation`/`setLocation`, `injectGMDirection`, `log`, `grantXP`, and the hooks `onAction`,
  `onTurnBegin`, `onTurnEnd`, `onWorldTick`, `onCheck`, `onHealthZero`
  (`pkg/rules/js_engine.go:46-174`). `LoadRules` loads `system.yaml`, then
  `systems/<id>/mechanics.js`, then the world override
  (`pkg/rules/loader.go:23-50`). The per-turn reload bug is fixed: `prepareTurn` now calls
  `LoadRules` every turn (`pkg/gui/service.go:1819-1825`).
- **Dice are richer than the engine exposes.** `pkg/rules/dice.go` is a thin adapter over
  `github.com/darkliquid/roll` (`pkg/rules/dice.go:21-48`), which supports keep/drop, exploding,
  reroll, target/success counting, Fate, percentile, and groups. But `RollResult` surfaces only
  `Notation`, `Total`, `Successes`, `RollCount`, `Dice` (`pkg/rules/dice.go:10-19`), and
  `SchemaResolver` reduces every check to `total >= target` with a **single additive stat bonus**
  (`pkg/rules/resolver.go:23-59`). There is no skill bonus, no per-check modifier, no opposed
  roll, no position/effect.
- **The check request already has room.** `CheckRequest` carries `Actor, Target, CheckKind, Stat,
  Difficulty, Stakes, Outcomes map[string]string, Notation` (`pkg/harness/turn.go:70-79`), but
  only `Stat` and `Notation` influence resolution.
- **Defaults are hardcoded.** `2d6` is the fallback in three places (`pkg/rules/resolver.go:29`,
  `pkg/engine/check_resolver.go:21`, the studio's `defaultMechanicsScript`
  `pkg/gui/service.go:4002`) and the pass threshold is 8 (`pkg/engine/check_resolver.go:28`).
- **The declarative schema is implemented but unauthorable.** `MechanicsSpec` covers stats, skills,
  health, checks, `allow_freeform_state`, engagement, and advancement
  (`pkg/core/mechanics.go:6-20`), and `StatSpec`/`SkillSpec`/`HealthSpec`/`CheckConventions`/
  `DifficultySpec` are all defined (`pkg/core/mechanics.go:76-111`). But the studio DTOs
  (`SystemDetailDTO`, `CreateSystemRequestDTO`, `pkg/gui/types.go:410-428`) **do not expose the
  `mechanics` block**, so it can only be hand-edited in YAML. The docs are stale: they show an
  `action_modes` field and a `rollDice` global that do not exist
  (`pkg/gui/docs/09-systems-studio.md:20-59`).
- **Engagement and cadence are implemented.** `off`/`auto`/`ask` are in
  `FormatMechanicsInstructions` (`pkg/harness/mechanics_instructions.go:24-78`), the cadence floor
  is in `pkg/engine/cadence.go`, and `onWorldTick`/`onHealthZero` fire on cadence
  (`pkg/engine/mechanics_engagement.go`). Advancement landed in `pkg/rules/advancement.go` but has
  no GUI spend route.
- **Dead and duplicated code.** `Turn.Verdict` is never assigned (only read at
  `pkg/gui/service.go:1525`), `dismissed_checks` is not implemented anywhere, and `newCheckID` is
  duplicated verbatim in `pkg/engine/check_resolver.go:38` and `pkg/rules/resolver.go:121`.

#### Gaps

1. **Skills and attributes do not modify rolls.** A system can declare skills but the resolver
   only adds one stat value.
2. **No resolution variety.** The engine cannot express a dice pool with success counting, an
   opposed roll, or Blades-style position/effect, even though the dice library supports most of
   the arithmetic.
3. **The GM cannot choose a resolution style per action.** Notation comes from conventions or the
   request; there is no notion of "this action uses the combat profile".
4. **The mechanics schema is invisible in the studio**, so authoring a real system means editing
   YAML by hand.
5. **No corpus and no tests.** There is one reference system and no scripted test that exercises
   complex state, so regressions are invisible.
6. **Dead code misleads.** The vestigial `Verdict`/`dismissed_checks` types suggest enforcement
   that does not exist.

#### Proposals

**SYS-1 — Check modifiers.** Extend resolution so a check sums declared bonuses: the governing
stat, an optional skill rating, and any per-check modifiers the system supplies. Add a `Skill`
field and a `Modifiers []CheckModifier` to `CheckRequest` (`pkg/harness/turn.go:70-79`), teach
`SchemaResolver.statValue` to read skills as well as stats (`pkg/rules/resolver.go:62-78`), and
teach `request_check` to name a skill (`pkg/harness/turn_tools.go:34-49`). This is the smallest
change that makes declared skills mean something.

**SYS-2 — Named resolution profiles.** Let a system declare resolution styles in the `mechanics`
block, for example:

```yaml
mechanics:
  checks:
    notation: 2d6
    profiles:
      pbta:    { notation: 2d6,  ladder: [ {min: 10, outcome: strong}, {min: 7, outcome: weak}, {min: 0, outcome: miss} ] }
      d20:     { notation: 1d20, dc: 15, outcome_on: [ {meet: true, outcome: success}, {meet: false, outcome: fail} ] }
      pool:    { notation: 5d10, success_on: ">=8", outcomes: { strong: ">=3", weak: "1-2", miss: "0" } }
      blades:  { notation: 2d6, position: [controlled, risky, desperate], effect: [limited, standard, great], ladder: [...] }
```

The GM names a profile per check (default from conventions). The engine maps the roll to an
outcome via the profile instead of `total >= target`. This is the largest systems item and should
be its own spec; the dice library already computes the raw numbers.

**SYS-3 — Surface success-count and position/effect.** Extend `CheckResult` and the DTO so the
chronicle and `DiceCheckCard` can show "3 successes", "position: risky, effect: limited", and the
profile's outcome label, not just total versus threshold. Pairs with the visibility work already
proposed in the engagement research.

**SYS-4 — Opposed rolls.** A check where the target is the opponent's stat rather than a fixed
difficulty. Add an opposed form to the profile and resolve both sides.

**SYS-5 — GUI mechanics editor.** Expose `MechanicsSpec` through `SystemDetailDTO` and
`CreateSystemRequestDTO` (`pkg/gui/types.go:410-428`) and add an editor surface in
`SystemsStudio.tsx` for stats, skills, health, checks (notation, outcomes, difficulties,
profiles), advancement, and `allow_freeform_state`, with validation against `pkg/core/mechanics.go`.
This removes the "YAML by hand" barrier and is a prerequisite for generated systems.

**SYS-6 — Reference systems corpus.** Ship two or three complete systems as templates: a PbtA 2d6
ladder, a d20 + DC system, and a dice-pool system (optionally Blades-flavoured). Each with
`system.yaml`, `mechanics.js`, and `rules.md`, exercised by tests. This is both documentation and
the regression suite for SYS-1/SYS-2.

**SYS-7 — Deterministic system test harness.** A scripted runner that feeds a sequence of actions
to the rules engine plus a stub host bridge, and asserts the rolls, outcomes, and state changes.
Generalise what a test of the reference system would do, and expose it as a `localrpg debug` mode
and a studio "run tests" action. This is what makes complex systems safe to author and is the gate
for generated systems (SG-3).

**SYS-8 — Housekeeping.** Either wire `Turn.Verdict`/`dismissed_checks` (enforce that a proposed
check is resolved or dismissed, as the trigger-and-cadence spec intended) or delete the types;
dedupe `newCheckID`; and fix the studio docs to match the real host API and manifest shape.

#### Risks and open questions

- SYS-2 risks becoming a rules engine in its own right. Keep profiles declarative and small; push
  anything exotic into `mechanics.js` via `onCheck`, which already exists.
- The outcome vocabulary and the profile ladder must not fight; the existing
  `CheckConventions.Outcome` list should be the single source of truth.
- Enforcement (SYS-8) can break existing systems that ignore proposed checks. Gate it behind
  engagement `auto` and make it a warning before it is an error.

---

### 11. Interactive rolls

#### Today

- **The `ask` handshake is turn-boundary only.** Under `ask`, `propose_check` ends the turn with a
  `PendingCheck` (`pkg/engine/orchestrator.go:2035-2048`), and an `@roll` record under `ask` does
  the same (`pkg/engine/orchestrator.go:1026-1029`). The turn is persisted with
  `Turn.PendingCheck` (`pkg/engine/history.go:60`), mapped to the DTO (`pkg/gui/types.go:217`),
  and rendered as `pending_check` in the frontend (`frontend/src/types.ts:136-141`). The player's
  **next** turn carries `PendingCheckRef` (`pkg/gui/types.go:582-584`), which the engine resolves
  before generation (`pkg/engine/orchestrator.go:861-884`).
- **There is no mid-turn pause.** The turn streams as NDJSON
  (`pkg/gui/server.go:1482-1543`), the `onChunk` callback can only abort
  (`pkg/engine/orchestrator.go:545-547`), and nothing waits for user input mid-generation.
- **`Roll` mode is a dead proposal path.** It builds a `ProposedCheck` and a
  `[PROPOSED CHECK: …]` directive and is never executed
  (`pkg/engine/orchestrator.go:708-717`).
- **Roll continuation exists but only under `auto`**, capped at
  `maxRollContinuations = 3` (`pkg/engine/orchestrator.go:948,1030-1056`).

#### Gaps

1. **The player must compose a whole new turn to roll.** There is no "Roll" button that just rolls.
2. **No renegotiation.** The player cannot argue the stakes or difficulty the GM set.
3. **The stream cannot resume.** A pending check ends the turn; resolving it is a fresh request
   with a new turn number, so the fiction is split across two records.
4. **No manual roll.** A tabletop player who rolled physical dice cannot enter the result.

#### Proposals

**IR-1 — Resolve-pending-check endpoint.** `POST /api/game/{id}/turn/{n}/resolve-check` takes the
pending ref, rolls (server-side, verifiable), resolves the check through the same resolver as
`auto`, and produces the adjudication. The result is recorded against the same turn, or as a
labelled continuation, so the fiction stays in one place.

**IR-2 — Roll card UI.** Replace the current pending-check card with a prominent panel showing the
stakes, the difficulty or target, the outcome vocabulary, the actor and their relevant stats, a
**Roll** button, and an **Argue** affordance. The Roll button calls IR-1. This is the visible
half of the feature and the part the player judges.

**IR-3 — Turn-resume stream protocol.** Extend the NDJSON protocol (or add a second stream) so a
pending check can be resolved and the turn continued without a new turn number: a `pending` turn
state, a `resolve` request that re-enters the generation loop with the roll result in context, and
events that the client merges into the existing turn. This is the hard part; it touches the
single-writer timeline and the per-campaign turn lock (`Service.BeginTurn`, `pkg/gui/service.go`).
Design it as its own spec.

**IR-4 — Manual roll entry.** Let the player type the dice they rolled, recorded with a `source`
of `manual` versus `engine`. Local-first means trusting the player; the record should be honest
about provenance rather than preventing it.

**IR-5 — Renegotiation.** The Argue action sends a player counter-proposal (different stakes,
difficulty, or approach). The GM re-adjudicates: accept the new stakes, adjust the difficulty, or
hold firm, with the negotiation recorded so the chronicle can show it. This is where the "argue to
renegotiate" idea from the request becomes a concrete flow.

**IR-6 — Route `Roll` mode through the interactive path.** When the player sends a `Roll` action,
create a pending check the same way `ask` does, so the dead `[PROPOSED CHECK]` path becomes the
live one.

#### Risks and open questions

- Resuming a stream is a real protocol change. The alternative, a second request that produces a
  continuation turn, is simpler and may be good enough; decide this in the spec.
- Trust and cheating: a local-first app cannot and should not police a physical roll. The design
  goal is a faithful record, not an enforced one.
- Timeouts: a paused turn holds the per-campaign lock. The resume path must not block the
  campaign, so the lock has to be released at the pending boundary (as it already is after a turn
  records, `pkg/gui/service.go:2059-2061`).

---

## Part D — Presentation

### 4. Scene images from turns

#### Today

- **Two pipelines.** Location art (the recurring backdrop) is generated from the **location entity
  and world style only**: `BuildLocationPrompt` (`pkg/media/image.go:87-111`) uses authored
  `Appearance` or `Name` + the first 200 characters of the body + tags, then the world style, and
  the cache key excludes the note body entirely (`AppearanceHash`, `pkg/media/image.go:61-82`).
  Turn scene illustration **does** use turn content, but **only on a scene break**
  (`pkg/engine/orchestrator.go:1295-1301,1462-1473`), composing `cue + location + style` in
  `BuildScenePrompt` (`pkg/engine/scene_worker.go:17-35`), where the cue is the paragraph after a
  `---` rule or an extractor `VisualCue` (`pkg/engine/scene_worker.go:38-67`).
- **Scene illustrations are not exported.** The export pipeline uses location art only
  (`pkg/export/script.go:492-496`).
- **Portraits use appearance only** (`pkg/engine/orchestrator.go:1475-1480`), with a procedural
  bust fallback (`pkg/media/procedural_bust.go`).

#### Gaps

1. **Ordinary turns never get a turn-specific image.** The backdrop is static per location, so a
   dramatic turn in a familiar room looks the same as the turn the player walked in.
2. **The prompt ignores the fiction.** Narration, the player's action, present entities, and the
   outcome tier do not reach the image prompt except indirectly through the extractor cue.
3. **No continuity.** Two images of the same scene can look unrelated; there is no seed or style
   carry-over.
4. **No policy.** Image generation is all-or-nothing per trigger; there is no cadence, budget, or
   significance control, so the expensive path cannot be turned on safely.
5. **No cost guardrail.** A metered image provider can be called on every break with no ceiling.

#### Proposals

**IMG-1 — Turn-aware scene prompt.** Compose the scene prompt from a narration excerpt, the
player's action, the present entities (with their appearance descriptors), the location, the world
style, and the **outcome tone** (success/partial/failure → mood words). Extend `BuildScenePrompt`
and, for the backdrop, allow a "scene of the moment" overlay. Keep the fixed quality suffix.

**IMG-2 — Configurable trigger policy.** A per-campaign setting: `off`, `scene_break` (today),
`significant` (a heuristic on outcome tier, location change, entity entrance, or an extractor
signal), `every_turn`, and `manual`. The significance heuristic is a small, testable function over
the turn; default to `significant` so the feature is visible without being ruinous.

**IMG-3 — Scene consistency.** Carry a per-scene seed and a small "style bible" (palette, lighting,
key descriptors) so successive images of one location cohere. Where the provider supports image
conditioning, pass the previous scene image as a reference. Where it does not, keep the seed and
prompt skeleton stable.

**IMG-4 — Cost guardrails.** A per-campaign image budget (count or currency, using the existing
usage ledger), a preview/approve mode for metered providers, and automatic fallback to procedural
art when the budget is spent or the provider fails.

**IMG-5 — Export parity.** Include turn scene illustrations in the web and video exports, choosing
the illustration for beats where one exists and the location backdrop otherwise.

#### Risks and open questions

- Per-turn generation is the most expensive feature in this document. The policy and budget
  (IMG-2, IMG-4) are not optional; they should ship with IMG-1.
- Provider support for image conditioning varies. IMG-3 must degrade gracefully to
  seed-and-prompt-only.

---

### 7. Placeholder and fallback variety

#### Today

- **Procedural art is thin.** `GenerateImage` has about three keyword palettes and three structure
  silhouettes (`pkg/media/procedural_art.go:17-102`); `GenerateProceduralBustSVG` derives two hues
  from an FNV hash (`pkg/media/procedural_bust.go:9-41`).
- **Fallback is wired and on by default.** `fallbackImageClient` covers a failing primary
  (`pkg/media/providers.go:111-144`) and `BuiltinFallback` defaults to true
  (`pkg/config/types.go:214-217,497`).
- **Theatre has a gradient and scrim** (`pkg/scene/theater.go:95-137`); the app root has a CSS
  radial gradient fallback (`frontend/src/App.tsx:740`).
- **The frontend already has a richer procedural system** for launcher assets: six palettes, a
  genre→icon map, banners, and monograms (`frontend/src/components/launcher/ProceduralAsset.tsx:23-118`).

#### Gaps

1. **Repetition.** Three palettes across a whole campaign reads as a bug, not a style.
2. **No art direction.** Nothing varies by genre, mood, time of day, or weather.
3. **No portraits beyond a generic bust.**
4. **No parallax-ready layers** for the theatre, so the no-image path is flat.
5. **Text and empty states are unvaried.**

#### Proposals

**PH-1 — Expand procedural art.** More palettes keyed by genre and mood, more structures (city,
forest, coast, interior, dungeon, sky, interior), time-of-day and weather variation, and a seed
derived from location plus current state so the art is stable but not identical everywhere. This
is the single highest-leverage placeholder change because it improves every scene that has no
generated image.

**PH-2 — Procedural portrait variety.** Derive species, archetype, palette, and expression from
entity tags and state (the entity model already carries tags and state), so NPCs look distinct and
consistent with their description.

**PH-3 — Layered SVG scenes.** Emit foreground/midground/background layers so the theatre can
parallax them and the video export can pan, reusing the same art.

**PH-4 — Genre-aware text and gradient fallbacks.** Campaign banners, empty states, and loading
states that vary by genre, with a shared palette source so the app, the site, and the export agree.

**PH-5 — Style packs.** Allow a user-supplied palette/style file so the procedural output can be
themed without code changes.

#### Risks and open questions

- Procedural art must stay deterministic and cheap; it runs on the no-GPU path and in exports.
- Keep the palette source in one place so the launcher, the theatre, and `pkg/scene` cannot drift.

---

### 10. Theatre and export efficiency

#### Today

- **Theatre.** `StoryTheater.tsx` has a native audio path (`:152-170`) and a browser path via
  `useSegmentPlayback` (`:119-125`), with `BEAT_GAP_MS = 300` (`:36`) and a 500 ms
  `/api/audio/status` poll per beat (the latency research cites `frontend/src/App.tsx:317-331`).
  The Go renderer mirrors the look for video (`pkg/scene/theater.go`, `pkg/scene/render.go`).
- **Web export** is a single gzipped base64 bundle (`pkg/export/web.go:181`), skips a missing
  asset rather than failing (`pkg/export/web.go:113-152`), and can be diagnosed without a browser
  (`pkg/export/inspect.go:38`).
- **Video export** is pure-Go VP8 + Opus WebM with atomic rename, reuses audio without re-encoding,
  and reuses frames (`pkg/export/video.go:138-384`).
- **Missing audio is handled by omission.** `scene.Compile` reports the first failure
  (`pkg/scene/compile.go:291-302`) and export drops unrepairable clips
  (`pkg/export/script.go:155-194`); the theatre advances on a timer when a beat has no audio.

#### Gaps

1. **Theatre pacing is poll-driven and serialised.** A 500 ms poll and a 300 ms gap per line add
   real time, and beat N+1 is requested only after beat N ends.
2. **No prefetch** of the next beat's audio or image.
3. **Missing audio is invisible.** A silent beat looks like a hang, not a missing clip.
4. **Effects are limited** to a crossfade; no Ken Burns, parallax, or mood tint.
5. **Exports have no captions, chapters, or subtitles**, and the bundle is large.

#### Proposals

**TH-1 — Theatre performance.** Replace the status poll with a completion event (the latency
research already proposed this), prefetch the next beat's audio and image, preload images, and
retune `BEAT_GAP_MS`. These are the same wins the turn-latency research identified and can be
lifted almost directly.

**TH-2 — Richer transitions and effects.** Ken Burns on stills, subtle parallax on layered
procedural scenes (PH-3), a mood tint derived from the turn outcome, and optional weather overlays.

**TH-3 — Missing-audio handling.** A visible "no audio" affordance per beat and pacing from an
estimated reading speed rather than a fixed gap, so a silent clip does not read as a stall.

**TH-4 — Captions and subtitles.** Render the spoken lines as captions in the theatre and emit a
WebVTT track in both exports, sourced from the same segments. This also improves accessibility.

**TH-5 — Export improvements.** Chapter markers, subtitles (TH-4), better compression, and
transitions in video; include turn scene illustrations (IMG-5) and richer portraits. Re-measure the
bundle after the changes.

#### Risks and open questions

- The browser and native audio paths must not both play. RB-3 (a shared playback ledger) is the
  durable fix; TH-1 must not widen the window.
- Captions and pacing interact: a beat's duration should be the max of audio length and reading
  time, which needs the segment text, not just the clip length.

---

## Part E — Robustness

### 5. Malformed LLM output and doubled speech

#### Today

- **The stream parser contains malformed records but drops them silently.** `record` classifies a
  line and, on invalid JSON, retains it with `Err` and produces **no event**
  (`pkg/turnstream/parser.go:216-241`). A malformed `@persona` loses speaker attribution so the
  following speech becomes narration (`pkg/turnstream/parser.go:173-177`); a malformed `@roll` is
  skipped by `pendingRoll` (`pkg/engine/streamsegments.go:83`), so the model's roll is silently
  ignored. `applyRecords` logs and skips bad records (`pkg/engine/streamsegments.go:41-53`).
- **No bounded retry on a malformed or empty reply.** On repeated failure the turn aborts
  (`pkg/engine/orchestrator.go:1010,1122`). Recovery is completion trim/continue with one attempt
  (`pkg/engine/recovery.go:129,82`) and a provider fallback chain
  (`pkg/engine/orchestrator.go:1669-1723`). Tool-argument parse errors are fed back to the model
  rather than aborting (`pkg/engine/orchestrator.go:2013-2015,2040`).
- **Single-play is key-equality based.** `clipSet.take` guarantees "heard once" per key
  (`pkg/gui/turn_audio.go:43-53`), per-key single-flight prevents duplicate synthesis
  (`pkg/media/tts.go:467,526,687`), and the frontend marks a key heard only on `onended`
  (`frontend/src/hooks/useStreamedSpeech.ts:34-45`).
- **Known double-play windows.** A streamed sentence clip key can differ from the finalised group
  key, so the group is emitted at finalise and the sentence is heard twice (flagged in
  `docs/superpowers/specs/2026-09-29-streamed-narration-audio-design.md` §5); `useStreamedSpeech`
  deliberately replays a partially heard clip; and the browser and device paths are only separated
  by an `enabled` flag (`frontend/src/hooks/useStreamedSpeech.ts:11-12`).

#### Gaps

1. **Malformed `@` records are silent data loss.** A dropped `@roll` means the model's resolution
   vanishes; a dropped `@persona` misattributes dialogue.
2. **No repair attempt** before dropping.
3. **No diagnostics** for degraded turns.
4. **Streamed versus finalised key divergence** can double a line.
5. **No property tests** over the parser or the group planner.

#### Proposals

**RB-1 — Record repair layer.** On a malformed `@` record, attempt a bounded repair (trim to
balanced braces, quote the common breakages) before dropping; if repair fails, surface a visible
diagnostic and continue, so a roll or persona is never silently lost. Extend the existing bracket
trim used by the extractor (`pkg/harness/extractor.go:467-494`) into a shared helper.

**RB-2 — Guaranteed single-play.** Make the streamer fold under exactly the same grouping and key
as finalise (assert parity at runtime, not only in tests, building on
`pkg/media/groupstream.go` and `pkg/media/caps.go`), or gate streaming off when grouping is on so
the two cannot disagree.

**RB-3 — Authoritative playback ledger.** A single "heard keys" registry shared between server and
client, so a partially played clip is recorded as heard with a resume offset instead of replayed
from the start, and the browser and device paths cannot both play. This replaces the
`enabled`-flag separation with an explicit ownership handshake.

**RB-4 — Bounded malformed-response retry.** A small, configurable retry that re-asks the model
with a repair instruction (for example "your previous reply contained an invalid `@roll` record;
re-emit it") before the turn is aborted. Cap it and keep the existing prose fallback.

**RB-5 — Diagnostics.** Surface dropped or degraded records in the turn UI and the Debug panel
(count, kind, excerpt), so a "the GM ignored my roll" report is answerable.

**RB-6 — Fuzz and property tests.** Fuzz the `turnstream` parser and property-test the group
planner (streamed fold equals finalised plan for arbitrary segment streams). These are cheap and
catch exactly the class of bug this section is about.

#### Risks and open questions

- Repair must never invent content; a repaired record that changes meaning is worse than a dropped
  one. Repair only structural damage.
- RB-3 adds coordination between two processes; keep the ledger derived from the same keys as the
  clip cache so it cannot drift.

---

## Part F — Content lifecycle

### 6. Packages and registries

#### Today

- **Content is plain directories.** `systems/<id>/` (`system.yaml`, `mechanics.js`,
  `prompts/rules.md`) and `worlds/<id>/` (`world.yaml`, `prompts/lore.md`, `entities/*.md`,
  `system_overrides/<system-id>/hooks.js`), resolved through `core.PathResolver`
  (`pkg/core/types.go:76-149`) and `pkg/paths` (`pkg/paths/paths.go:34-55`).
- **There is no archive, checksum, manifest, version pin, or registry.** `SystemManifest.Version`
  is free text (`pkg/core/types.go:14`). `pkg/export` is story replay only. Archive handling exists
  solely for model downloads (`pkg/models/manager.go:419-457`).
- **There is no content import/export.** The only upload is multipart banner/icon
  (`pkg/gui/server.go:394-413`).
- **Safety primitives exist.** `pathutil.ValidateID`/`SanitizeID`/`ResolveSafeChild`
  (`pkg/pathutil/pathutil.go:24-87`) and the duplicate-id guard pattern (`ErrWorldExists`,
  `pkg/gui/service.go:4156`).
- **CRUD gaps.** There is no `DeleteSystem` or `DeleteWorld`; world create and update are split.

#### Gaps

1. **Content cannot leave or enter the app** except by copying directories by hand.
2. **No version, no dependency, no provenance.** A world that needs a specific system cannot say
   so.
3. **No sharing surface.** There is no way to publish or subscribe to content.
4. **No lockfile**, so a campaign cannot record which content version it played against.

#### Proposals

**PKG-1 — Content package format.** A `.lrpgpack` (tar.gz or zip) containing a `package.yaml`
manifest (id, name, version, type `system`|`world`, dependencies, file checksums, license, author,
description, minimum app version) plus the content tree and any assets. Define it in `pkg/core` or
a new `pkg/content` leaf so both the CLI and the GUI can use it.

**PKG-2 — Export/import routes and UI.** Export a system or world (with entities and assets) and
import a package: validate the manifest, sanitise every path with `pathutil.ResolveSafeChild`,
verify checksums, refuse overwrite or offer rename, and surface conflicts. Add a studio "Export"
and "Import" action and CLI verbs (`localrpg content export|import`).

**PKG-3 — Versioning, dependencies, and a lockfile.** Enforce semver on `Version`, allow a world to
declare a required system (and version range), and write a per-campaign lockfile recording the
resolved content versions so a save is reproducible.

**PKG-4 — Registry client.** A `registries` config (a list of index URLs), a static `index.json`
schema, and `localrpg registry add|install|update|search`. Support both a simple static index and
a git-based registry. Download to a staging area, verify checksums (PKG-5), then install
atomically. Cache the index and respect offline mode.

**PKG-5 — Provenance.** Record checksums in the manifest and verify on install; support an optional
signature and surface a trust state (verified / unsigned / mismatch) in the UI.

**PKG-6 — Editing content outside the app.** Document the on-disk format and the package format as
the interchange so users can edit in any editor and repackage with the CLI. This is mostly a docs
task on top of PKG-1.

#### Risks and open questions

- Security is the headline risk: an imported package is untrusted input that can contain
  `mechanics.js` (executed) and entity Markdown (parsed). Sandboxing is already the model for JS
  (goja), but the import path must validate every path and size, and the UI must be explicit that
  importing runs code.
- Registry hosting and moderation are out of scope for the client; a static index keeps the client
  simple and defers the policy question.
- Deleting systems/worlds (the missing CRUD) becomes necessary once content can be installed;
  decide whether delete is soft or hard.

---

### 8. AI world generation

#### Today

- **Generation is field-scoped.** `/api/generate-text` generates single fields for a world
  (`name, description, genre, art_style, lore_prompt`) or a system (`name, description,
  rules_prompt`) with per-form prompts (`pkg/gui/text_generate.go:18-122`). Character generation is
  separate (`pkg/gui/character_generate.go`). Image previews are stateless
  (`pkg/gui/service.go:4954`).
- **The seams are clear.** `buildTextGeneratorPrompt` (`pkg/gui/text_generate.go:52`),
  `buildAssetPrompt` (`pkg/gui/service.go:4914`), and the frontend `AIGenerateButton`
  (`frontend/src/components/ui/AIGenerateButton.tsx`) are the extension points.
- **No whole-world synthesis, no enhancement, no ingestion.** `chromedp` is app-driver only
  (`pkg/driver/driver.go:17-18`).

#### Gaps

1. **No "generate a world"** with lore and a set of entities.
2. **No "enhance this world"** with proposals.
3. **No ingestion** of a folder or a set of URLs into a world.
4. **No review flow** for generated content.

#### Proposals

**WG-1 — Whole-world generation pipeline.** A multi-step generation from a short brief: outline →
factions and locations → characters → cross-linked entity notes, assembled into a draft world
(`world.yaml` + `entities/` + `prompts/lore.md`). Use structured outputs (the superseded
structured-GM spec's idea is still right for this) and the GM provider, with per-step progress.
Produce a draft, never a live world.

**WG-2 — Batch entity generation.** Generate a set of entities from an existing world's lore, with
wikilinks resolved between them, so a world can be filled out after the fact.

**WG-3 — Enhance existing world.** Given a world, propose additions (a new faction, three NPCs, a
plot hook) and lore expansions, presented as a diff to accept or reject.

**WG-4 — Source ingestion.** Import a folder of Markdown/text or a set of URLs, extract lore and
entities, and build a world. Folder import first (safe, offline, testable); URL ingestion as a
separate spike with robots handling, caching, and a clear "this fetches the network" notice.

**WG-5 — Draft review UI.** A draft world in the studio with accept/reject per entity and per lore
section, and a one-click commit to `worlds/`.

**WG-6 — Cost and latency controls.** Show the estimated call count and let the user cap it, and
provide a deterministic template-based fallback (the oracle, LF-2) when no provider is configured.

#### Risks and open questions

- Generated worlds are only as good as the model. The review UI (WG-5) is what makes it usable;
  generation without review produces clutter.
- Ingestion raises copyright and provenance questions; keep it user-driven and local.
- Structured outputs are provider-dependent; a non-tool provider needs a parse-and-repair path
  (RB-1 helps).

---

### 9. AI system generation

#### Today

- **System generation is field-scoped** (`name`, `description`, `rules_prompt`,
  `pkg/gui/text_generate.go:98`). `mechanics.js` is not generated. The studio seeds a draft from
  the reference template (`frontend/src/components/SystemsStudio.tsx:91-141`) and submits
  `rules_prompt` + `script`.
- **The `mechanics` block is not editable** through the studio (SYS-5), and there is no test
  harness (SYS-7).

#### Gaps

1. **No complete-system generation** (mechanics schema + `mechanics.js` + `rules.md`).
2. **No validation** that a generated system actually loads and runs.
3. **No enhancement or explanation** of an existing system.

#### Proposals

**SG-1 — Whole-system generation.** From a description ("a gritty d20 system with sanity and
wounds"), generate `system.yaml` (the `mechanics` block), `mechanics.js` hooks, and `rules.md`,
then load the result into the JS engine and run a smoke test before offering it.

**SG-2 — Schema-first generation.** Target the declarative `mechanics` block (SYS-5) for the
common cases and generate `mechanics.js` only for the escape hatches, so the output is inspectable
and editable rather than a wall of JavaScript.

**SG-3 — Generated-system smoke-test gate.** Reuse SYS-7: a generated system must pass a
deterministic test (load, roll a check, apply a state change) before it can be saved. A system that
cannot load is never offered.

**SG-4 — Enhance/explain an existing system.** Add stats, skills, checks, or advancement to an
existing system; or explain a system's mechanics in prose for the author.

**SG-5 — Base-system catalogue.** Ship the SYS-6 reference systems as bases to derive from, so
generation starts from a working skeleton rather than a blank file.

#### Risks and open questions

- Generating executable `mechanics.js` is the highest-risk generation in the app. The smoke-test
  gate (SG-3) and the schema-first bias (SG-2) are the mitigations; both should be mandatory, not
  optional.
- The generated system must not assume the engine knows rules it does not; keep the target the
  declarative schema plus the documented host API.

---

## Appendix — Cross-cutting concerns and open questions

**A. Cost.** Three of the proposals add metered calls (IMG-1/2, WG-1, SG-1). The usage ledger and
pricing already exist (`pkg/pricing`, `docs/proposals/2026-09-28-provider-costs-and-usage-research.md`),
but every new generation path needs a budget and a preview mode, not just a toggle.

**B. The `gui.Service` monolith.** The architecture review already flagged `pkg/gui/service.go`
(~2700 lines, fan-out 14) as the largest locality risk
(`docs/architecture/review-2026-09-24.md:109-116`). Several proposals here add endpoints to it.
The domain-service split the review recommended is worth doing before Wave 3 rather than during
it.

**C. Composition root.** `cmd/localrpg` wires everything by hand and `gui.go`/`play.go` duplicate
it (review §3). The provider manager (MP-1) and registry client (PKG-4) both add wiring; a
`pkg/app` composition root would remove the drift risk.

**D. Docs.** Several specs and docs are already stale (`09-systems-studio.md` shows fields that do
not exist; the structured-GM specs are superseded). Every proposal here that changes a user-visible
surface should update the corresponding `pkg/gui/docs/` article, and the site generator renders
those, so the drift is user-visible.

**E. Testing.** The repo's testing conventions are standard library only, skip on missing hardware,
and prefer determinism. SYS-7, RB-6, and the smoke-test gates fit that. The interactive-roll and
resume work (IR-1/3) needs an end-to-end test through the driver, which already exists for export
parity (`pkg/gui/export_player_e2e_test.go`).

**F. Questions to settle before specing.**
1. Multi-provider: does a media family get one default plus per-use overrides, or a full routing
   policy? (MP-1 vs MP-3/MP-4)
2. Interactive rolls: resume the stream, or a second request that produces a continuation? (IR-3)
3. Content trust: how loud must the "this runs JavaScript" warning be on import? (PKG-2)
4. Generation: is a generated world/system a draft that must be reviewed, or can it be committed
   directly with an undo? (WG-5, SG-3)
5. Local-first: is the oracle worth investing in (LF-2), or is the honest answer "use Ollama"? The
   caveat work (LF-3) is worth doing either way.
