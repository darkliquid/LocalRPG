# Provider Instance Identity Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#28 MP-2](https://github.com/darkliquid/LocalRPG/issues/28)
**Epic:** [#16 Multiple providers of the same type](https://github.com/darkliquid/LocalRPG/issues/16)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §1 (MP-2)
**Extends:** `docs/superpowers/specs/2026-09-28-provider-key-identity-design.md`
**Scope:** `pkg/provider`, `pkg/config`, `pkg/harness`, `pkg/media`, `pkg/embeddings`

---

## 1. Problem

The provider-key identity work defined a canonical key
`<family>:<adapter>[@<discriminator>]` (`pkg/provider/key.go:11-14`) and derives the discriminator
from the configuration: `HostDiscriminator(endpoint)` (`pkg/provider/key.go:116-130`) or
`CommandDiscriminator(command)` (`pkg/provider/key.go:133-139`).

That derivation is enough to tell two *different* endpoints apart, but it cannot tell two
configurations of the **same adapter at the same endpoint** apart. Two API keys against
`https://api.openai.com`, or two `tts:http` entries against the same local server with different
voice settings, collapse to one key.

The consequence is concrete: the usage ledger (`Usage.Provider`), pricing resolution
(`pricing.Resolve`), rate-limit and funds accounting, failure metadata, and content-addressed media
caches all key off this identity. Two configs that collapse to one key share a cache namespace and a
usage row, so one instance's spend and clips are attributed to the other.

The proposal for multiple providers (MP-1) introduces named configuration entries. This spec is its
foundation: a **user-chosen discriminator** the resolver prefers over the derived one.

## 2. Goals

- A provider configuration can carry an optional **instance id**.
- The canonical key uses it: `family:adapter@<id>`.
- When no id is set, the current endpoint/command derivation is used unchanged, so nothing existing
  changes key.
- Two same-endpoint configs with different ids get different keys, so usage, pricing, and caches
  separate them.
- Invalid or duplicate instance ids are rejected at config validation, not at request time.

## 3. Non-goals

- The named-map config shape and the default selector. That is MP-1.
- Per-purpose routing (narrator vs scene). That is MP-3.
- Any UI. MP-5 covers the manager.

## 4. Design

### 4.1 A config field

Each provider-carrying config struct gains an optional `Instance`:

```go
	// Instance is an optional user-chosen discriminator for this provider
	// configuration. It becomes the key's "@<instance>" segment, so two configs
	// of one adapter at one endpoint stay distinct. Empty means the discriminator
	// is derived from the endpoint or command.
	Instance string `yaml:"instance,omitempty" json:"instance,omitempty"`
```

Added to `AgentRoleConfig`, `TTSConfig`, `STTConfig`, `ImageConfig`
(`pkg/config/types.go:26-221`), and `EmbeddingProviderConfig` (`pkg/config/types.go:301-308`).

### 4.2 The resolver prefers it

One helper decides the discriminator, so every family agrees:

```go
// InstanceDiscriminator returns the user-chosen instance id when set, else the
// derived discriminator for the configuration's transport.
func InstanceDiscriminator(instance, derived string) string {
	if s := strings.TrimSpace(instance); s != "" {
		return s
	}
	return derived
}
```

Each resolver builds `derived` as it does today and passes it through:

- `harness.KeyFor` (`pkg/harness/exports.go:9-35`): for `http` it uses
  `HostDiscriminator(endpoint)`, for `cli` `CommandDiscriminator(command)`.
- `media.TTSKeyFor`/`STTKeyFor`/`ImageKeyFor` (`pkg/media/exports.go:22-186`).
- `embeddings.KeyFor` (`pkg/embeddings/factory.go:85-110`).

Then the key is built with the existing `provider.InstanceOrSelf(parentKey, discriminator)`
(`pkg/provider/key.go:73-82`), which already returns the adapter key when the discriminator is
empty, so the unset case is byte-identical to today.

For a `builtin` adapter (Gemini, ElevenLabs, sherpa-onnx, narrative-oracle), there is no derived
discriminator, so an `instance` is the only way to give it one; that is exactly the "two API keys
for one builtin" case.

### 4.3 Validation

`Config.Validate()` (`pkg/config/types.go:329-356`) gains, per family:

- An `instance` value must match the key discriminator grammar `^[a-z0-9.:-]+$`
  (`pkg/provider/key.go:18`). A value outside it is an error, reported with the config path.
- Two configurations of the same family must not share an instance id, because they would produce
  the same key. The check spans the LLM roles (`agents.roles`), the media singletons (each is one
  config, so only a self-check applies), and the named embedding providers. The error names both
  paths.

Validation runs where `Validate` already runs (settings save and startup), so a bad id is caught
before it reaches the ledger.

### 4.4 What changes downstream

Nothing in the consumers changes: they already key off the canonical key the resolvers return. The
only difference is that a configured instance now reaches them. The media cache key
(`ComputeAudioCacheKey*`, `pkg/media/cache.go`) does not include the provider key today; adding the
instance to it is a follow-up (noted in the deep-dive as a risk for MP-1), because changing it
invalidates warm caches. This spec leaves the cache key alone.

## 5. Behaviour

| Configuration | Key |
| --- | --- |
| `tts: http, endpoint: http://localhost:8880` | `tts:http@localhost:8880` |
| same, plus `instance: narrator` | `tts:http@narrator` |
| two `http` roles, same endpoint, no instance | both `llm:openaichat@api.openai.com` (collide, as today) |
| two `http` roles, same endpoint, `instance: cheap` and `instance: good` | `llm:openaichat@cheap`, `llm:openaichat@good` |
| `instance: "Bad Id"` | validation error |
| two roles with `instance: same` | validation error |

## 6. Testing

- `pkg/config`: `Validate` rejects a malformed instance and a duplicate within a family; accepts a
  unique valid one.
- `pkg/harness`: `KeyFor` returns the instance key when `Instance` is set and the derived key when
  it is not; two configs differing only by instance produce different keys.
- `pkg/media`: the same for `TTSKeyFor`/`STTKeyFor`/`ImageKeyFor`, including a builtin adapter with
  an instance (no derived discriminator).
- `pkg/embeddings`: `KeyFor` honours a named provider's instance.
- A regression guard: a config with no `instance` produces exactly the key it produced before.

## 7. Rollout

Additive and backward compatible. `instance` is optional; unset keeps every existing key. No
migration. The config `Version` does not change, because the shape only gains an optional field.

## 8. Risks

- **Instance collisions.** Two configs with the same id silently share a key, which is the bug this
  spec exists to prevent. Mitigated by the validation in §4.3.
- **Cache namespacing.** As noted, the audio/art cache key does not include the provider key, so two
  instances of one adapter with different voice settings could still collide in the cache. That is a
  separate, deliberate follow-up because it invalidates warm caches; flag it when MP-1 lands.
- **`@` in an id.** The grammar forbids it, so a key can never contain two `@` segments. Good.
