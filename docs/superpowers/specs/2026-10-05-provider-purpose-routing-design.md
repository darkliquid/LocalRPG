# Provider Purpose Routing Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#29 MP-3](https://github.com/darkliquid/LocalRPG/issues/29)
**Epic:** [#16 Multiple providers of the same type](https://github.com/darkliquid/LocalRPG/issues/16)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §1 (MP-3)
**Depends on:** [#27 MP-1](https://github.com/darkliquid/LocalRPG/issues/27)
**Scope:** `pkg/config`, `pkg/media`, `pkg/gui`, `pkg/engine`, `frontend`

---

## 1. Problem

MP-1 lets several named configurations coexist, but nothing selects between them. Every consumer
still resolves the default: the narrator's voice, an NPC's voice, a scene image, a portrait, and a
placeholder all come from `cfg.Media.TTS` or `cfg.Media.Image`.

The deep-dive's motivating cases are exactly these distinctions:

- a premium voice for the narrator and a free local one for incidental NPCs;
- a cheap procedural image for placeholders and a premium model for the scene that opens an act;
- a fast provider for bulk portraits and a careful one for the few that matter.

Without a selector, configuring several providers achieves nothing.

## 2. Goals

- Name the **uses** a media provider plays: `narrator`, `npc` (TTS); `scene`, `portrait`,
  `placeholder` (image).
- Each use selects a provider instance by name, with the family default as the fallback.
- A use that is unset resolves to the default, so an unconfigured campaign is unchanged.
- The existing per-entity TTS provider override still wins for a specific entity.
- Validation rejects an unknown use or a name that does not exist in that family.

## 3. Non-goals

- A UI for choosing uses. That is MP-5.
- Selection *rules* (cheapest, first-that-works). That is MP-4.
- LLM routing; roles already give it several providers.

## 4. Design

### 4.1 The purpose vocabulary

In `pkg/config` (or `pkg/media` if it reads better as a media concept):

```go
// Purpose names a role a media provider plays.
type Purpose string

const (
	PurposeNarrator    Purpose = "narrator"    // TTS: the narration voice
	PurposeNPC         Purpose = "npc"         // TTS: a character with no voice override
	PurposeScene       Purpose = "scene"       // image: a location or turn scene
	PurposePortrait    Purpose = "portrait"    // image: a character portrait
	PurposePlaceholder Purpose = "placeholder" // image: a fallback or preview
)

// PurposeFamily reports which media family a purpose belongs to.
func PurposeFamily(p Purpose) string // "tts" or "image"
```

### 4.2 The configuration

`MediaConfig` gains one map, keyed by purpose, valued by a provider name from MP-1:

```go
	// Purposes maps a use name to a provider name from TTSProviders,
	// ImageProviders, or the family default. Absent uses fall back to the
	// family default.
	Purposes map[string]string `yaml:"purposes,omitempty" json:"purposes,omitempty"`
```

```yaml
media:
  tts:
    type: builtin
    builtin_name: elevenlabs
  tts_providers:
    npc: { type: builtin, builtin_name: sherpa-onnx }
  image:
    type: builtin
    builtin_name: procedural-art
  image_providers:
    hero: { type: builtin, builtin_name: gemini }
  purposes:
    narrator: default
    npc: npc
    scene: default
    portrait: hero
    placeholder: default
```

### 4.3 Resolution

```go
// ProviderForPurpose returns the provider name a purpose resolves to: the
// configured name, or the family default when unset.
func (m MediaConfig) ProviderForPurpose(p Purpose) string

// TTSForPurpose and ImageForPurpose resolve a purpose to a concrete config.
func (m MediaConfig) TTSForPurpose(p Purpose) TTSConfig
func (m MediaConfig) ImageForPurpose(p Purpose) ImageConfig
```

`TTSForPurpose(p)` is `TTSFor(m.ProviderForPurpose(p))` (MP-1's accessor). When the purposes map is
empty, every purpose resolves to the default, which is exactly today's behaviour.

A sensible default when `placeholder` is unset but an image provider named `procedural` exists: the
spec chooses **not** to guess. An unset purpose is always the default; a user who wants the
procedural generator for placeholders names it. Guessing would surprise.

### 4.4 Consumers

- **TTS narrator.** The narrator voice resolution (`narratorVoiceFor`, `pkg/gui/service.go`) and the
  clip pipeline build the client from `TTSForPurpose(PurposeNarrator)`.
- **TTS NPC.** When a speech segment's speaker has no per-entity provider override, the client comes
  from `TTSForPurpose(PurposeNPC)`. The existing entity `voice.provider` field still wins
  (`entity.VoiceConfig.Provider`), so a single character can override the purpose default.
- **Image scene.** `sceneArtResolver` (`pkg/gui/service.go:2493-2505`) uses
  `ImageForPurpose(PurposeScene)`.
- **Image portrait.** `PortraitWorker`'s client uses `ImageForPurpose(PurposePortrait)`
  (`pkg/gui/service.go:1912-1927`).
- **Image placeholder.** The `BuiltinFallback` path and asset previews use
  `ImageForPurpose(PurposePlaceholder)`.

The registries (MP-1) resolve per purpose by name, so a purpose that shares a provider name shares
one cached client.

### 4.5 Validation

`Config.Validate()` gains:

- every key in `Purposes` is a known purpose;
- a value names an existing entry in that purpose's family (`tts_providers` or `image_providers`) or
  is `default` or empty;
- a purpose is not given a provider from the wrong family (for example `narrator` cannot name an
  image provider).

Each error names the config path.

## 5. Behaviour

| Config | `narrator` | `npc` | `scene` | `portrait` |
| --- | --- | --- | --- | --- |
| nothing | default | default | default | default |
| `purposes.npc: npc` | default | npc entry | default | default |
| `purposes.portrait: hero` | default | default | default | hero entry |
| `purposes.narrator: missing` | validation error | | | |
| `purposes.narrator: hero` (an image provider) | validation error | | | |
| an entity with `voice.provider: X` | default for others | X for that entity | | |

## 6. Testing

- `pkg/config`: `ProviderForPurpose` and `TTSForPurpose`/`ImageForPurpose` resolve configured,
  unset, and unknown names; an empty map resolves everything to the default; `Validate` rejects an
  unknown purpose, a missing provider, and a cross-family provider.
- `pkg/media`: the registry returns the purpose's client and caches it.
- `pkg/gui`: the narrator client comes from the narrator purpose; an entity provider override still
  wins for that entity; the scene and portrait resolvers use their purposes.
- A regression guard: with an empty purposes map, every resolver returns the same config as before.

## 7. Rollout

Additive. `purposes` is absent in every existing config, so every purpose resolves to the default
and behaviour is unchanged. No migration.

## 8. Risks

- **Two ways to override a voice.** A per-entity `voice.provider` and a purpose default can both
  apply. The rule is explicit: the entity override wins, the purpose fills the gap. Documented in
  the providers guide.
- **Surprise cost.** A user who points `scene` at a metered provider and enables per-turn images
  pays per turn. IMG-4 (image cost guardrails) is the mitigation; until then, the default stays the
  default.
- **Purpose sprawl.** Adding a purpose later is a config-compatible change, but each purpose is a
  place consumers must honour. Keep the set small and resist adding `banner`, `icon`, etc. until
  they are needed.
