# Multiple Provider Instances Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#27 MP-1](https://github.com/darkliquid/LocalRPG/issues/27)
**Epic:** [#16 Multiple providers of the same type](https://github.com/darkliquid/LocalRPG/issues/16)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §1 (MP-1)
**Depends on:** [#28 MP-2](https://github.com/darkliquid/LocalRPG/issues/28) (instance ids)
**Scope:** `pkg/config`, `pkg/gui`, `pkg/media`, `frontend`

---

## 1. Problem

Media is singleton per family. `MediaConfig` holds exactly one `TTSConfig`, one `STTConfig`, and
one `ImageConfig` (`pkg/config/types.go:224-228`), and none of those structs carries an id. A
campaign therefore cannot:

- use a premium voice for the narrator and a free local one for incidental NPCs;
- use a cheap procedural image for placeholders and a premium model for the act-opening scene;
- keep two LLM keys of one vendor (that is the LLM side, handled by roles + MP-2).

The only family that already supports several same-typed configs is embeddings:
`EmbeddingsConfig.Providers map[string]EmbeddingProviderConfig` plus `Provider string`
(`pkg/config/types.go:292-299`). It is the precedent.

The obstacle to copying it wholesale is churn: `cfg.Media.TTS`, `.STT`, and `.Image` are read at
**188** call sites across `pkg/` and `cmd/`. A reshape that turns each family into
`{Default, Providers}` would touch every one of them, including assignments in `SaveSettings`, for
no behavioural gain beyond what an additive shape gives.

## 2. Goals

- Several named configurations per media family, coexisting in one config file.
- The existing singleton field remains the **default** entry, so every current reader is unchanged
  and no migration is needed.
- A resolver names any entry by name, with the default as the fallback.
- Validation rejects an empty or malformed name and a duplicate instance id (MP-2).
- The runtime can build more than one client per family (MP-3 selects between them).

## 3. Non-goals

- Selecting a provider per purpose. That is MP-3.
- A manager UI. That is MP-5.
- Changing the LLM side; roles already give it several providers.
- Unifying the media shape with `EmbeddingsConfig`'s. A later refactor may, but it is not required
  to meet the goal and it is the churn this spec avoids.

## 4. Design

### 4.1 Config shape

`MediaConfig` gains three optional maps; the existing fields keep their meaning as the default
entry:

```go
type MediaConfig struct {
	TTS   TTSConfig   `yaml:"tts" json:"tts"`
	STT   STTConfig   `yaml:"stt" json:"stt"`
	Image ImageConfig `yaml:"image" json:"image"`

	// TTSProviders, STTProviders, and ImageProviders hold additional named
	// configurations. The TTS/STT/Image fields above are the default entry, used
	// when no name is selected, so a config written before this field existed
	// behaves exactly as it did.
	TTSProviders   map[string]TTSConfig   `yaml:"tts_providers,omitempty" json:"tts_providers,omitempty"`
	STTProviders   map[string]STTConfig   `yaml:"stt_providers,omitempty" json:"stt_providers,omitempty"`
	ImageProviders map[string]ImageConfig `yaml:"image_providers,omitempty" json:"image_providers,omitempty"`
}
```

Example:

```yaml
media:
  tts:
    type: builtin
    builtin_name: elevenlabs
    api_key: "env:ELEVENLABS_API_KEY"
    default_voice: Rachel
  tts_providers:
    npc:
      type: builtin
      builtin_name: sherpa-onnx
      model_path: ~/.cache/localrpg/models/tts/kokoro
    crowd:
      type: builtin
      builtin_name: native-os
  image:
    type: builtin
    builtin_name: procedural-art
  image_providers:
    hero:
      type: builtin
      builtin_name: gemini
      api_key: "env:GEMINI_API_KEY"
```

The name `default` is reserved for the singleton entry, so a map entry may not use it.

### 4.2 Resolvers

`MediaConfig` gains three methods, so callers never index the maps directly:

```go
// TTSFor returns the named TTS configuration, or the default when name is empty
// or unknown. A name is validated at load, so "unknown" only happens for a stale
// reference and falls back rather than failing a turn.
func (m MediaConfig) TTSFor(name string) TTSConfig

func (m MediaConfig) STTFor(name string) STTConfig
func (m MediaConfig) ImageFor(name string) ImageConfig

// TTSNames returns every configured TTS name, default first, for a picker.
func (m MediaConfig) TTSNames() []string
```

`TTSFor("")` returns `m.TTS`; `TTSFor("default")` returns `m.TTS`; `TTSFor("npc")` returns
`m.TTSProviders["npc"]` when present, else `m.TTS`.

### 4.3 Validation

`Config.Validate()` (`pkg/config/types.go:329-356`) gains, per family:

- every map key is non-empty, matches `^[a-z0-9][a-z0-9-]*$`, and is not `default`;
- no key equals another (map keys are unique by construction, so this is about the reserved name);
- each entry's `Instance` (MP-2) follows the same rules as the default's.

An invalid name is a validation error naming the config path, surfaced where `Validate` already
runs (settings save and startup).

### 4.4 The runtime registry

`pkg/media` gains a small builder so the rest of the app holds clients, not configs:

```go
// TTSRegistry holds one TTS client per configured name, plus the default. It
// builds lazily and caches, so a named provider's model loads once.
type TTSRegistry struct { /* … */ }

func NewTTSRegistry(cfg *config.Config, sharedKey string, logger trace.Logger) *TTSRegistry
func (r *TTSRegistry) For(name string) (TTSClient, error)
func (r *TTSRegistry) Default() (TTSClient, error)
```

`STTRegistry` and `ImageRegistry` mirror it. The registries reuse the existing factories
(`NewTTSClientWithSharedKey`, `pkg/media/providers.go:68-84`) per entry, so no provider code
changes.

The GUI's `audioPipeline`/`ttsClientFor` (`pkg/gui/service.go`) resolve through the registry rather
than building a client from `cfg.Media.TTS` each time. Until MP-3 lands, `For("")` is the only call,
so behaviour is unchanged.

### 4.5 Migration

None. The singleton fields are the default entry, so an existing `config.yaml` resolves identically
and needs no rewrite. `config.Version` does not change. Saving a config with no extra instances
writes no `*_providers` keys, so a round-trip is byte-stable.

## 5. Behaviour

| Config | Resolved |
| --- | --- |
| only `media.tts` | `TTSFor("")`, `TTSFor("default")`, `TTSFor("npc")` all return it |
| `media.tts` + `tts_providers.npc` | `TTSFor("npc")` returns the npc entry; the rest return the default |
| `tts_providers.default` | validation error (reserved) |
| `tts_providers[""]` | validation error |
| `tts_providers.bad name` | validation error |
| a stale `TTSFor("gone")` | falls back to the default, no error |

## 6. Testing

- `pkg/config`: a round-trip keeps `tts_providers`; `TTSFor` resolves default, named, and unknown;
  `TTSNames` puts the default first; `Validate` rejects the reserved name, an empty name, and a
  malformed name; a config with no maps is byte-identical after a save/load.
- `pkg/media`: `TTSRegistry.For` builds one client per name and caches it; `For("")` equals
  `Default()`; a failing named entry reports an error without affecting the default.
- `pkg/gui`: the audio pipeline resolves the default when no name is set (unchanged behaviour).
- A regression guard: a config file written before this change loads and resolves to the same
  provider as before.

## 7. Rollout

Additive and backward compatible. No migration, no `config.Version` bump. The maps are absent in
every existing config.

## 8. Risks

- **Two sources of truth.** The default is the singleton field, not `Providers["default"]`. A reader
  that assumed a map would be surprised; the accessors and the reserved-name rule make the model
  explicit. The alternative (full reshape) is the churn this spec avoids, and can be done later
  without changing the YAML.
- **Cache namespacing.** As MP-2 notes, the audio/art cache key does not include the provider key,
  so two instances with different voice settings could collide. Adding the key to the cache key is a
  deliberate follow-up because it invalidates warm caches; it should land with or before MP-3.
- **Registry lifetime.** A registry that caches clients must be invalidated when settings change,
  mirroring the existing per-turn rebuild. Wire it into the same config-revision invalidation the
  latency work already proposes.
