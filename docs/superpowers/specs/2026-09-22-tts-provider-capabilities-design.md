# Design Spec: TTS Provider Capabilities, Voice Catalogs, and Provider Options

**Date:** 2026-09-22
**Status:** Draft
**Target:** `pkg/media`, `pkg/config`, `pkg/entity`, `pkg/gui`, `pkg/harness`, `frontend`
**Source proposal:** `docs/proposals/2026-09-22-speech-and-llm-provider-expansion.md` (P1, P2, P4, P5)
**Research:** `~/.local/share/crush/research/localrpg-voice-and-llm-providers/report.md`

---

## 1. Executive Summary

LocalRPG's speech layer assumes we know every provider's voices and knobs. That holds for our own
engines (Kokoro, native OS speech) because we author both sides, and it breaks for any provider whose
voice list is fetched and whose options are its own.

This spec adds the two capabilities that make a provider self-describing:

1. **`VoiceCatalog`** - a provider can enumerate the voices it offers, with metadata and defaults.
2. **`VoiceOptions`** - a provider declares the tunables it accepts, so the UI renders controls from
   the declaration rather than from provider-specific frontend code.

It then threads provider options through the profile/entity model and the audio cache so a per-character
tuning is persisted and cannot collide with another tuning. Finally it defines the catalog API with
on-disk caching, and the Settings/codex surface that uses it.

ElevenLabs is the first consumer and is specified separately in
`docs/superpowers/specs/2026-09-22-elevenlabs-tts-provider-design.md`.

---

## 2. Findings

### 2.1 A provider contract with no vocabulary for voices

`TTSClient` is a single method (`pkg/media/tts.go:84`):

```go
type TTSClient interface {
	Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error)
}
```

There is no `ListVoices`. The provider factory (`pkg/media/providers.go:486`) switches on
`config.TTSConfig.Type` and returns a client; nothing downstream can ask a client what it can do.

### 2.2 Voice data lives in three places, none provider-aware

- `config.VoiceProfile` (`pkg/config/types.go:81`) - authored archetypes: id, name, voice_id,
  provider, pitch, speech_rate, tags, description.
- `entity.VoiceConfig` (`pkg/entity/entity.go`) - what a note carries: provider, voice_id, pitch,
  speech_rate.
- `config.TTSConfig.VoiceProfiles` (`pkg/config/types.go:106`) - the library, seeded per preset.

All three are closed structs. A provider that wants `stability` or `style` has nowhere to put it, and
a voice library that a provider supplies cannot be represented at all.

### 2.3 The audio cache key cannot express a new knob

`ComputeAudioCacheKeyWithRate(speakerID, voiceID, pitch, speechRate, text)` (`pkg/media/cache.go:17`)
is what the DTO version token uses, and `ComputeAudioCacheKey(speakerID, voiceHash, text)`
(`pkg/media/cache.go:11`) is what the pipeline uses, with `voiceHash` built from
`provider:voiceID:pitch:rate` in `SynthesizeUtterance`. Two consequences:

- any new tunable added to the request but not the key serves the wrong clip;
- the DTO key omits `provider`, so the same `voice_id` under two providers would collide once more
  than one provider exists.

### 2.4 The Settings UI hard-codes the knobs

`frontend/src/components/SettingsStudio.tsx` renders `pitch` and `speech_rate` inputs and a
`voice_profiles` list editor. Any provider-specific tuning would require branching in that component
per provider, which is exactly the coupling the capability interface is meant to avoid.

### 2.5 Matching is tag-based and provider-blind

`harness.AssignVoiceProfile` (`pkg/harness/extractor.go:60`) scores `profile.Tags` against the
character's name/body/appearance/aliases. `config.TTSConfig.VoiceProfiles` arrives via
`Timeline.SetVoiceProfiles`. Nothing filters the candidate set to profiles that the *active* provider
can actually synthesise, so a Kokoro profile can be assigned while ElevenLabs is active.

### 2.6 The public API has no catalog surface

`registerRoutes` (`pkg/gui/server.go`) exposes `/api/settings` and `/api/settings/test-provider`
(`TestProviderRequestDTO.Provider` is an opaque `interface{}`, so extra fields already pass through),
but nothing for voices or options.

---

## 3. Design

### 3.1 Capability interfaces (`pkg/media/catalog.go`)

Small optional interfaces, asserted at runtime, mirroring the existing `MarkdownAware`
(`pkg/media/speakable.go:18`) and the pattern the researched library uses for its optional
capabilities.

```go
// ProviderVoice is one voice a provider offers.
type ProviderVoice struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Language    string         `json:"language,omitempty"`   // BCP-47 when known
	Gender      string         `json:"gender,omitempty"`
	Accent      string         `json:"accent,omitempty"`
	Categories  []string       `json:"categories,omitempty"` // provider taxonomy, e.g. premade, cloned
	Tags        []string       `json:"tags,omitempty"`       // normalised matching vocabulary
	Description string         `json:"description,omitempty"`
	PreviewURL  string         `json:"preview_url,omitempty"`
	Defaults    map[string]any `json:"defaults,omitempty"`   // the provider's own stored settings
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// VoiceCatalog is implemented by providers that can enumerate their voices.
type VoiceCatalog interface {
	ListVoices(ctx context.Context) ([]ProviderVoice, error)
}

// VoiceOption declares one tunable a provider accepts.
type VoiceOption struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Kind    string   `json:"kind"` // "float" | "int" | "bool" | "string" | "enum"
	Min     float64  `json:"min,omitempty"`
	Max     float64  `json:"max,omitempty"`
	Step    float64  `json:"step,omitempty"`
	Options []string `json:"options,omitempty"` // for Kind == "enum"
	Default any      `json:"default,omitempty"`
	Help    string   `json:"help,omitempty"`
}

// VoiceOptions is implemented by providers that declare the tunables they accept.
type VoiceOptions interface {
	VoiceOptions() []VoiceOption
}

// MeteredProvider is implemented by providers that charge per request.
type MeteredProvider interface {
	Metered() bool
}
```

Design rules:

- **Options are additive.** `pitch` and `speech_rate` stay the portable baseline that every provider
  maps as best it can; `VoiceOptions` declares only provider extras. This avoids two controls writing
  the same knob (a provider that has no pitch declares none and documents the limitation in `Help`).
- **`Tags` is the join point with matching.** Catalog labels are normalised into the same lowercase
  vocabulary archetypes already use, so one matcher scores both without knowing where a profile came
  from.
- **`Defaults` carries the provider's stored settings** for a voice, used when a profile imports it.
- **A provider that cannot enumerate simply does not implement the interface**; the UI then falls back
  to authored profiles with no error.

### 3.2 Provider identity and config

Add a stable, filesystem-safe key so catalogs, the API, and diagnostics all name a provider the same
way:

```go
// ProviderKey derives a stable identifier from a TTS configuration, used for
// catalog filenames, API parameters, and diagnostics.
// "builtin:elevenlabs", "builtin:sherpa-onnx", "http:localhost:8880", "cli:piper".
func ProviderKey(cfg config.TTSConfig) string
```

`config.TTSConfig` gains one field:

```go
	// Metered marks a provider that charges per request. It overrides the
	// provider's own declaration so an operator can flag a proxied endpoint.
	Metered *bool `yaml:"metered,omitempty" json:"metered,omitempty"`
```

### 3.3 Options on profiles and entities

`config.VoiceProfile` (`pkg/config/types.go:81`) gains:

```go
	// Options holds provider-declared tunables, keyed by VoiceOption.Key. It is
	// opaque to the engine the same way entity State is: only the provider
	// interprets it.
	Options map[string]any `yaml:"options,omitempty" json:"options,omitempty"`
```

`entity.VoiceConfig` (`pkg/entity/entity.go`) gains the same field, persisted in frontmatter:

```yaml
voice:
  provider: builtin:elevenlabs
  voice_id: EXAVITQu4vr4xnSDxMaL
  speech_rate: 1.0
  options:
    stability: 0.35
    similarity_boost: 0.8
    style: 0.2
    model: eleven_multilingual_v2
```

Semantics:

- Absent options mean "the provider's own defaults". We never invent values, and an empty map is
  written as omitted.
- **Reserved keys** interpreted by the platform, not the provider: `model` (request model). Everything
  else is provider-defined. A provider may treat an unknown reserved key as an error.
- Unknown keys survive a load and are dropped before a request, so a profile authored against a newer
  provider degrades rather than failing.

Validation and coercion (`pkg/media/options.go`):

```go
// ValidateVoiceOptions clamps and types-checks a value map against a provider's
// declarations. It returns the canonical map to persist and a list of human
// warnings for the UI. Keys absent from the schema are dropped.
func ValidateVoiceOptions(schema []VoiceOption, values map[string]any) (map[string]any, []string)
```

Called when Settings saves a profile and again at synthesis time (defence in depth for hand-edited
notes). Canonical values are `bool`, `float64`, or `string` only, which keeps YAML and JSON round-trips
stable.

### 3.4 Option-aware cache key (`pkg/media/cache.go`)

```go
// ComputeAudioCacheKeyForVoice hashes everything that changes a clip: provider,
// voice, prosody, provider options, and text. A voice with no options and no
// provider falls back to the legacy key so existing campaigns stay warm.
func ComputeAudioCacheKeyForVoice(speakerID string, voice *entity.VoiceConfig, text string) string
```

- Canonicalisation is free: `encoding/json` marshals map keys sorted, so a struct wrapper around the
  option map hashes deterministically regardless of insertion order.
- The key is prefixed (`v2:`) so it cannot be confused with a legacy key.
- **Rules:** with `voice == nil` or no options, delegate to the existing
  `ComputeAudioCacheKeyWithRate` (preserves warm caches); with options, hash
  `provider|voice_id|pitch|rate|json(options)`.

Callers to switch to `ComputeAudioCacheKeyForVoice`:

- `TTSPipeline.SynthesizeUtterance` (`pkg/media/tts.go`), which currently builds `voiceHash` inline;
- `segmentDTOs` (`pkg/gui/service.go`), which already resolves the voice for its version token, so the
  provider omission there is fixed by the same change.

Tests must assert: changing `stability` alone changes the key; map key order does not; a voice with no
options produces the same key as the legacy function.

### 3.5 Catalog caching (`pkg/media/voice_catalog.go`)

```go
// CachedVoiceCatalog fetches a provider's voices at most once per TTL and keeps
// the last successful answer on disk so an outage does not empty the picker.
type CachedVoiceCatalog struct {
	cacheDir string
	ttl      time.Duration // default 24h
}

func (c *CachedVoiceCatalog) Load(ctx context.Context, providerID string, client TTSClient, refresh bool) (CatalogSnapshot, error)

type CatalogSnapshot struct {
	Provider  string          `json:"provider"`
	FetchedAt time.Time       `json:"fetched_at"`
	Stale     bool            `json:"stale"`
	Voices    []ProviderVoice `json:"voices"`
}
```

- Stored at `<cacheDir>/voices/<providerID>.json`, alongside the existing content cache directories.
- Fresh within the TTL unless `refresh` is set; on fetch failure the last snapshot is returned with
  `Stale: true`; a failure with no snapshot returns an empty non-stale snapshot plus the error, and the
  UI says "catalog unavailable" rather than showing a broken list.
- A provider without `VoiceCatalog` returns an empty snapshot and `CatalogAvailable=false` from the
  inspect endpoint, so the UI can explain instead of offering a dead button.

### 3.6 API surface

One endpoint, so the editor and the picker always agree with the exact configuration in hand:

```
POST /api/tts/inspect
{
  "config":  { ...TTSConfig... },   // may be unsaved
  "refresh": false
}
->
{
  "provider_key": "builtin:elevenlabs",
  "metered": true,
  "options": [ ...VoiceOption... ],
  "catalog": { "available": true, "fetched_at": "...", "stale": false, "voices": [ ... ] },
  "error": ""                       // catalog failure, non-fatal
}
```

Housekeeping:

- `POST` because the caller supplies a config, matching `/api/settings/test-provider`.
- The response never echoes `api_key` back.
- `ModelDownloadModal`-style size of the payload: a 5000-voice catalog is a few hundred KB of JSON; the
  endpoint returns `voice_id`, `name`, tags, categories, and `preview_url` only. `Defaults` and
  `Metadata` are included because the profile editor needs them.
- `/api/settings/test-provider` gains an optional `voice_id` so a preview can audition a catalog voice
  without saving it first (the field passes through the existing opaque `Provider` blob, so only the
  service needs to read it).
- A convenience `GET /api/tts/voices?provider=<key>&refresh=1` is **not** included; the picker uses the
  saved config from the client, which it already holds to render Settings.

**Security note (decision required):** `GET /api/settings` currently returns `api_key` to the client.
This spec does not change that, but a follow-up should return a redacted key plus an "unchanged"
sentinel so the browser never round-trips a secret. Flagged in §8.

### 3.7 Matching stays deterministic, and provider-aware

- The GUI filters the profile library it hands to the timeline: a profile whose `Provider` is set and
  differs from the active `ProviderKey(cfg)` is excluded from `SetVoiceProfiles`. `harness` therefore
  needs no change, and a Kokoro profile can no longer be assigned while ElevenLabs is active.
- Catalog-imported profiles carry normalised tags, so `AssignVoiceProfile` scores them identically to
  authored archetypes.
- Shared tag vocabulary (`pkg/media/tags.go`): `NormaliseVoiceTags(raw ...string) []string` lowercases,
  trims, maps common synonyms (`middle-aged` -> `middle-aged`, `american` -> `american`), and drops
  empties. Used by every catalog implementation so matching behaviour is provider-independent.

### 3.8 Frontend

**Settings Studio** (`frontend/src/components/SettingsStudio.tsx`)

- A `useTTSInspect(config, refresh)` hook (new, `frontend/src/hooks/useTTSInspect.ts`) debounces config
  changes and calls `POST /api/tts/inspect`.
- The TTS panel renders `options` generically: `float`/`int` -> range input with min/max/step,
  `enum` -> select, `bool` -> checkbox, `string` -> text, each with `Help` beneath. No provider name
  appears in the component.
- A catalog strip: "N voices - refreshed 2h ago - Refresh", plus "unavailable" state.
- The NPC Voice Profiles library gains a collapsible per-profile options editor driven by the same
  schema, showing only the keys that differ from the provider defaults.
- A "Metered" badge when `metered` is true, with the cost note from the inspect response.

**Codex voice picker** (`frontend/src/components/CodexDrawer.tsx`)

- Two sections in the archetype select: authored profiles, then provider catalog voices (search field,
  filter by category, audition button using `preview_url`).
- "Add as profile" turns a catalog pick into a `config.VoiceProfile` bound to the provider key and
  voice id, with `Defaults` as the starting options and normalised tags.
- Profiles whose provider does not match are shown with a "belongs to X" hint rather than hidden.

### 3.9 Cost guardrails (P5)

- `inspect.metered` is surfaced in Settings and in the codex picker.
- Before a bulk operation that synthesises uncached audio (export with audio, "play all turns",
  regenerate after a voice change), the GUI warns with the count of uncached beats. Count comes from a
  new read-only service helper `CountUncachedBeats(ctx, gameID) (cached, uncached int, err error)`
  that walks a campaign's segments and tests `ComputeAudioCacheKeyForVoice` against the content cache.
- The existing `media.tts.request` trace event already reports `chars`, so a session-level character
  count is a trace-side tally, not new plumbing.

---

## 4. Data Flow

```text
Settings edit ──► useTTSInspect(config) ──► POST /api/tts/inspect
                                               ├─ NewTTSClient(config)
                                               ├─ VoiceOptions() ──► rendered controls
                                               └─ CachedVoiceCatalog.Load() ──► voices (TTL, stale-on-error)

Codex pick a catalog voice
  └─ "Add as profile" ──► config.VoiceProfile{provider, voice_id, tags, options=Defaults}
       └─ Settings save ──► ValidateVoiceOptions(schema, values) ──► config.yaml

Turn playback
  └─ voiceFor(speaker) ──► entity.VoiceConfig (with options)
       └─ TTSPipeline.SynthesizeSegment
            ├─ SpeakableTextFor(policy, client, text)
            └─ ComputeAudioCacheKeyForVoice(speaker, voice, spoken) ──► clip file
```

---

## 5. Compatibility and Migration

| Change | Impact | Mitigation |
| --- | --- | --- |
| New fields on `VoiceProfile`/`VoiceConfig` | Older files lack `options`; unknown keys ignored by YAML/JSON | Omitted when empty, so old configs re-save unchanged |
| New cache key function | Would cold-start every existing clip | Delegate to the legacy key when no options are present |
| `segmentDTOs` switch to the new key | Version tokens change for optionless voices only if we let them | Same delegation rule keeps existing URLs identical |
| Provider-filtered profiles | A campaign authored against Kokoro that switches to ElevenLabs loses its assignments | Profiles are filtered for *assignment*, not removed; switching back restores them, and the UI warns |
| New `/api/tts/inspect` route | Additive | None |

---

## 6. File Map

| Action | Path | Description |
| :--- | :--- | :--- |
| Create | `pkg/media/catalog.go` | `ProviderVoice`, `VoiceCatalog`, `VoiceOption`, `VoiceOptions`, `MeteredProvider`, `ProviderKey` |
| Create | `pkg/media/options.go` | `ValidateVoiceOptions`, canonical value coercion |
| Create | `pkg/media/tags.go` | `NormaliseVoiceTags` and the shared tag vocabulary |
| Create | `pkg/media/voice_catalog.go` | `CachedVoiceCatalog`, `CatalogSnapshot`, TTL and stale-serve |
| Create | `pkg/media/catalog_test.go`, `options_test.go`, `voice_catalog_test.go` | Unit tests |
| Modify | `pkg/media/cache.go` | `ComputeAudioCacheKeyForVoice`; keep the legacy functions |
| Modify | `pkg/media/tts.go` | `SynthesizeUtterance` uses the new key |
| Modify | `pkg/config/types.go` | `VoiceProfile.Options`, `TTSConfig.Metered` |
| Modify | `pkg/entity/entity.go` | `VoiceConfig.Options` (frontmatter round-trip) |
| Modify | `pkg/gui/types.go` | Inspect request/response DTOs; `TestProviderRequestDTO.VoiceID` |
| Create | `pkg/gui/tts_inspect.go` | Inspect service method + route handler |
| Modify | `pkg/gui/server.go` | `POST /api/tts/inspect` in `registerRoutes` |
| Modify | `pkg/gui/service.go` | Preview honours `voice_id`; `CountUncachedBeats`; provider-filtered profiles |
| Modify | `frontend/src/types.ts` | `VoiceOption`, `CatalogSnapshot`, `TTSInspect*`; `VoiceProfile.options`; `TTSConfig.metered` |
| Modify | `frontend/src/api/client.ts` | `inspectTTS` |
| Create | `frontend/src/hooks/useTTSInspect.ts` | Debounced inspect hook |
| Modify | `frontend/src/components/SettingsStudio.tsx` | Schema-driven options, catalog strip, per-profile options |
| Modify | `frontend/src/components/CodexDrawer.tsx` | Catalog section, audition, "Add as profile" |
| Modify | `frontend/src/components/Export`/turn controls | Uncached-beat warning for metered providers |
| Modify | `pkg/config/presets.go` | `elevenlabs` preset (see the ElevenLabs spec) |

---

## 7. Acceptance Criteria

1. A provider implementing `VoiceOptions` gets its controls rendered with no provider-specific
   frontend code; a provider without it shows the baseline pitch/rate only.
2. A provider implementing `VoiceCatalog` has its voices listed, searchable, and auditionable, and its
   catalog survives a network outage by serving the last snapshot as stale.
3. A profile can carry provider options; they persist to `config.yaml`, and a profile option reaches
   the entity frontmatter through the codex.
4. Changing any option changes the audio cache key; two options sets never share a clip; a voice with
   no options produces the identical key it produced before this change.
5. Assignable profiles are filtered to the active provider, and a mismatched profile is labelled, not
   silently dropped.
6. `/api/tts/inspect` never returns an API key.
7. Bulk synthesis on a metered provider warns with the uncached-beat count first.
8. `go test -count=1 ./...`, `go vet ./...`, and `npx tsc --noEmit` pass.

---

## 8. Out of Scope / Follow-ups

- **Streaming synthesis** (`SynthesizeStream`) - needs streaming playback to be worth it.
- **Model catalog for LLMs** (`ModelCatalog`) - the same capability idea applied to
  `pkg/harness`; proposed as P7a, specced separately.
- **`omnivoice-core` adapter** - P6; becomes a single file behind §3.1 if adopted.
- **Secret handling** - redacting `api_key` from `GET /api/settings` and accepting an "unchanged"
  sentinel; a prerequisite for shipping any cloud provider to cautious users.
- **Content-pinned casts** - letting a world ship its own voice profile list.
- **Rules-system access to options** - a JS hook requiring "narrator uses low stability".
