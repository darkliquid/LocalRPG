# Provider Key Identity Design

**Date:** 2026-09-28
**Status:** Proposed
**Scope:** One canonical, family-namespaced provider key — with an optional instance discriminator and most-specific-first lookup — produced by one resolver and consumed by the usage ledger, pricing, rate-limit blocks, generation failures, the provider catalogue, and the generated documentation
**Related:** `pkg/provider`, `pkg/harness` (router, limits, failures), `pkg/media`, `pkg/embeddings`, `pkg/pricing`, `pkg/storage`, `pkg/gui`, `pkg/tools`, `pkg/config`, `frontend/`; builds on `docs/superpowers/specs/2026-09-24-provider-capability-model-design.md` and `docs/superpowers/specs/2026-09-26-usage-cost-and-provider-limits-design.md`

## 1. Overview & Goals

A provider is named in at least four ways today, and no two agree:

| Concept | Produced by | Example |
| --- | --- | --- |
| Registry/descriptor ID | `pkg/provider` | `tts-elevenlabs`, `image-http`, `openaichat` |
| Ledger/pricing key | provider packages and `pkg/gui` | `openaichat`, `builtin:elevenlabs`, `http:localhost:8880`, `http` |
| Rate-limit / funds key | `gui.providerKeyForRole` | `http`, `gemini`, `builtin:el...` |
| Role name in the router | `RouterFromConfig` | `gm`, `narrator` |

The consequences are visible in the shipped Usage view: most rows show **no price
configured** because the built-in price table is keyed by `openaichat` and
`gemini` while media rows are keyed by something else; an HTTP LLM rate-limit
backoff is recorded against `http` while its spend is recorded against
`openaichat`; and the built-in `elevenlabs` price can never match a TTS row.

This specification defines one canonical key of the form `<family>:<adapter>`,
an optional instance discriminator `<family>:<adapter>@<discriminator>`, and a
most-specific-first lookup that falls back from the instance to the adapter.
The capability-model resolver is the single function that produces a key, and
every consumer uses it.

**Goals:**

- Define one canonical provider key grammar and type, shared by every subsystem.
- Make the config-to-registry resolver the only producer of keys.
- Record the most specific key available (instance when the config names an
  endpoint or command, otherwise the adapter), and resolve prices, blocks, and
  failures most-specific-first with adapter fallback.
- Cover the embedding family too, and wire its usage into the ledger.
- Guarantee keys are unique within a family and cannot collide across families.
- Add validation that fails loudly when a key is malformed, duplicated, or
  hand-built outside the resolver.
- Publish every key, its adapter, and its price so users can write prices.

**Non-Goals:**

- Automatic price discovery from provider APIs.
- Key aliasing or rewriting of historical ledger rows; this is a hard cut (§7).

**Success Criteria:**

- For any configuration, the ledger key, the price key, and the limit key for the
  same provider are the same string.
- `llm:gemini`, `tts:gemini`, and `image:gemini` are three distinct keys with
  three independent prices.
- Two HTTP endpoints of the same adapter can be priced and blocked separately via
  their instance keys, and a price written at the adapter key applies to both.
- Every key the resolver can produce, instance or adapter, parses and is unique.
- Every key that can be recorded is listed in the generated provider catalogue.
- Embedding calls appear in the ledger under an `embedding:*` key.

## 2. Investigation Findings

- The registry already names adapters uniquely, and `register` panics on a
  duplicate ID (`pkg/provider/provider.go`). IDs are family-prefixed with a dash
  (`tts-elevenlabs`, `image-http`, `stt-whisper-http`) except for the LLM family,
  whose IDs are bare (`openaichat`, `gemini`, `cli`, `narrative-oracle`,
  `pkg/provider/openaichat/openaichat.go:15`,
  `pkg/provider/oracle/oracle.go:15`). The capability model proposed a different
  spelling (`elevenlabs`, `openai-http`, `sd-http`;
  `2026-09-24-provider-capability-model-design.md` §2.6), so the shipped IDs never
  matched the planned ones. This specification supersedes that table.
- The capability model established that a small resolver maps config `type` +
  `builtin_name` to a registry ID, and that this resolver is the only translation
  layer. Four such resolvers exist: `harness.ProviderIDFor`
  (`pkg/harness/exports.go:6`), `media.TTSProviderIDFor` (`pkg/media/exports.go:21`),
  `media.STTProviderIDFor` (`pkg/media/exports.go:67`), and
  `media.ImageProviderIDFor` (`pkg/media/exports.go:106`). None of them produce the
  string that is actually recorded.
- The recorded key is produced separately, in three places:
  - LLM adapters hardcode their own name: `Provider: "gemini"`
    (`pkg/provider/geminillm/provider.go:599`) and `Provider: "openaichat"`
    (`pkg/provider/openaichat/http.go:354`), ignoring the ID the facade built them
    with (`harness.BuildModelFor` passes the role name, `pkg/harness/factory.go:70`).
  - TTS uses `media.ProviderKey(cfg.Media.TTS)` (`pkg/gui/service.go:1810`), which
    builds `gemini:tts`, `builtin:<name>`, `cli:<command>`, or `http:<host>`
    (`pkg/media/catalog.go:59`).
  - Image and STT use `builtin_name || type` inline
    (`pkg/gui/image_generation.go:47`, `pkg/gui/service.go:2994`).
- The rate-limit/funds key is a fourth derivation, `gui.providerKeyForRole`
  (`pkg/gui/limits.go:15`), using `builtin_name || type` for LLM
  (`roleProviderKey`, `pkg/gui/limits.go:37`), `media.ProviderKey` for TTS, and
  `builtin_name || type` for image and STT. For an HTTP LLM this yields `http`
  while the ledger records `openaichat`.
- Pricing matches the recorded string (`pricing.Resolve`, `pkg/pricing/pricing.go:39`)
  against a built-in table keyed `gemini`, `openaichat`, and `elevenlabs`
  (`pkg/pricing/pricing.go:21`). The `elevenlabs` entry cannot match a TTS row,
  whose key is `builtin:elevenlabs`.
- Collisions are reachable: every HTTP image and STT provider records `http`,
  every HTTP TTS provider on the same host records the same `http:<host>`, and
  `gemini` is shared by the LLM and image families.
- Embeddings compute `LastUsage()` with `Provider: c.ID()`
  (`pkg/provider/openaiembedding/openai.go:181`), where `ID()` is a config
  identifier, and nothing consumes it; `geminiembedding` ignores its usage
  metadata entirely (`pkg/provider/geminiembedding/gemini.go:66-102`). Embedding
  calls happen in the index worker (`pkg/storage/embedding_worker.go:187`) and the
  memory tools (`pkg/tools/memory.go:41`, `pkg/tools/tools.go:130,294`), none of
  which hold a usage sink today.
- The generated provider catalogue (`pkg/gui/docs_catalogue_test.go`) documents a
  per-provider "ledger key" but computes it with its own copy of the derivation
  logic, so the documentation can describe a key the runtime does not produce.

## 3. Key Model

### 3.1 Grammar

```
key        = family ":" adapter
instance   = key "@" discriminator
family     = "llm" | "tts" | "stt" | "image" | "embedding"
adapter    = 1*( %x61-7A / DIGIT / "-" )        ; lowercase letters, digits, hyphen
discriminator = 1*( %x61-7A / DIGIT / "-" / "." / ":" )
```

Examples: `llm:openaichat`, `tts:gemini`, `tts:http`,
`tts:http@localhost:8880`, `image:http@127.0.0.1:8188`, `embedding:openai`.

The registry descriptor ID becomes the canonical adapter key. Adapters are
renamed from the dash form to the colon form (`tts-elevenlabs` becomes
`tts:elevenlabs`), and the bare LLM IDs gain their family (`openaichat` becomes
`llm:openaichat`). The family is part of the key so that one vendor serving
several families cannot collide.

### 3.2 Identity table

| Adapter key | Adapter | Today's registry ID | Today's ledger key |
| --- | --- | --- | --- |
| `llm:openaichat` | OpenAI-compatible HTTP | `openaichat` | `openaichat` |
| `llm:gemini` | Google Gemini | `gemini` | `gemini` |
| `llm:cli` | Command-line LLM | `cli` | not reported |
| `llm:narrative-oracle` | Narrative Oracle | `narrative-oracle` | not reported |
| `tts:gemini` | Gemini TTS | `tts-gemini` | `gemini:tts` |
| `tts:elevenlabs` | ElevenLabs | `tts-elevenlabs` | `builtin:elevenlabs` |
| `tts:native-os` | Native OS speech | `tts-native-os` | `builtin:native-os` |
| `tts:sherpa-onnx` | Sherpa-ONNX Kokoro | `tts-sherpa-onnx` | `builtin:sherpa-onnx` |
| `tts:piper` | Piper CLI | `tts-piper` | `cli:piper` |
| `tts:http` | OpenAI-compatible speech HTTP | `tts-openai-http` | `http:<host>` |
| `stt:whisper-http` | Whisper HTTP | `stt-whisper-http` | `http` |
| `stt:whisper-cli` | Whisper CLI | `stt-whisper-cli` | `cli` |
| `stt:web-speech` | Web Speech API | `stt-webspeech` | `web-speech` |
| `image:gemini` | Gemini / Imagen | `image-gemini` | `gemini` |
| `image:http` | Image HTTP / ComfyUI | `image-http` | `http` |
| `image:cli` | Image CLI | `image-cli` | `cli` |
| `image:procedural-art` | Procedural art | `image-procedural-art` | `procedural-art` |
| `embedding:builtin` | Built-in hash projection | `openaiembedding` / local | not recorded |
| `embedding:openai` | OpenAI embeddings | `openai-embedding` | not recorded |
| `embedding:gemini` | Gemini embeddings | `gemini-embedding` | not recorded |

`llm:openaichat` keeps its name: "OpenAI-compatible" is a wire specification many
vendors implement, so the name is neutral.

The generated catalogue in `pkg/gui/docs/12-provider-catalogue.md` is
regenerated from the registry and becomes the authoritative published table.

### 3.3 Instance keys

An instance key adds a discriminator that identifies one configuration of an
adapter, so two endpoints of the same adapter can be told apart:

| Adapter | Discriminator | Instance example |
| --- | --- | --- |
| HTTP (any family) | endpoint host and port | `tts:http@localhost:8880` |
| CLI (any family) | basename of the command | `tts:piper@piper`, `llm:cli@claude` |
| Gemini / builtin (media and LLM) | none — the adapter is the instance | `tts:gemini`, `image:procedural-art` |
| Embedding | endpoint host, else the literal `default` | `embedding:openai@api.openai.com`, `embedding:gemini@default` |

Browser-only providers have no key. `stt:web-speech` runs in the browser and
never reaches the server-side STT factory, so `STTKeyFor` returns no key for it
and it is never recorded or blocked. Its descriptor stays for the UI only.

Instance keys are recorded and priced, so per-endpoint pricing is possible. A
value written at the adapter key continues to apply to every instance of it,
because lookups fall back (§3.4). This replaces today's `media.ProviderKey`
(`pkg/media/catalog.go:59`), whose `http:<host>` form is an instance key written
in a different spelling.

### 3.4 Most-specific-first lookup

Every lookup resolves a recorded key `K` and a model `M` against a table of
entries, stopping at the first match:

1. `(K, M)` — the exact instance and model
2. `(K, "")` — the exact instance, any model
3. `(parent(K), M)` — the adapter, exact model
4. `(parent(K), "")` — the adapter, any model

`parent(K)` is `K` with anything from the first `@` removed; for an adapter key
`parent(K)` is `K` itself, so steps 3-4 describe it and steps 1-2 are skipped.
The same candidate list drives pricing (§5.2), rate-limit blocks (§5.3), funds
failures, and failure metadata (§5.4). A built-in price table is keyed by adapter
key only and is consulted after the configured entries, still matching model
before model-less.

## 4. One Resolver, One Producer

### 4.1 Interface

A single resolver entry point per family, returning the adapter key and, when the
config names an endpoint or command, the instance key. These live beside the
existing facades because `pkg/provider` must stay a leaf with no config import
(capability model §3).

```go
// pkg/provider — the key type and its grammar, no config dependency.
type Family string

const (
    FamilyLLM       Family = "llm"
    FamilyTTS       Family = "tts"
    FamilySTT       Family = "stt"
    FamilyImage     Family = "image"
    FamilyEmbedding Family = "embedding"
)

type Key string

func NewKey(f Family, adapter string) (Key, error) // the only exported constructor
func ParseKey(s string) (Key, error)               // for storage reads and config
func (k Key) Family() Family                       // "llm"
func (k Key) Adapter() string                       // "openaichat"
func (k Key) Instance() (discriminator string, ok bool)
func (k Key) Parent() Key                           // drops the instance

// An instance key, built from an adapter key plus a discriminator.
func NewInstanceKey(k Key, discriminator string) (Key, error)

// pkg/harness — the LLM facade resolver.
func KeyFor(cfg ProviderConfig) (provider.Key, bool)

// pkg/media — one resolver per media family.
func TTSKeyFor(cfg config.TTSConfig) (provider.Key, bool)
func STTKeyFor(cfg config.STTConfig) (provider.Key, bool)
func ImageKeyFor(cfg config.ImageConfig) (provider.Key, bool)

// pkg/embeddings — the embedding facade resolver.
func KeyFor(cfg config.EmbeddingsConfig) (provider.Key, bool)
```

The `bool` return is deliberate: an unconfigured or disabled provider has no key,
and callers must not invent one. Each resolver returns the instance key when the
config carries a discriminator (endpoint host, command) and the adapter key
otherwise; a single helper builds both so the discriminator rule lives once. The
existing `…ProviderIDFor` functions become thin wrappers returning
`string(key.Parent())` during the transition and are then deleted.

### 4.2 Deletions

- `harness.ProviderIDFor` is replaced by `harness.KeyFor`.
- `media.TTSProviderIDFor` / `STTProviderIDFor` / `ImageProviderIDFor` are
  replaced by `TTSKeyFor` / `STTKeyFor` / `ImageKeyFor`.
- `media.ProviderKey` is replaced by the TTS resolver's instance key.
- The inline `builtin_name || type` derivations in `pkg/gui` are deleted.
- The hardcoded `Provider: "gemini"` / `Provider: "openaichat"` in adapter
  packages are deleted; the facade names the provider and the recorder fills it
  from the resolver.

## 5. Consumers

Every consumer below obtains its key from the resolver and resolves values with
the §3.4 fallback. No consumer builds a key.

### 5.1 Usage ledger

`harness.Usage.Provider` holds the most specific key the resolver produced.
The recording paths (`pkg/gui/usage.go:saveUsage`, `pkg/gui/service.go` TTS,
`pkg/gui/image_generation.go`, `pkg/gui/service.go` STT) all use the same
resolver. `storage.UsageRecord.Provider` stores it verbatim.

### 5.2 Pricing

`pricing.Resolve` takes a recorded key and a model and applies the §3.4 candidate
list to the configured entries, then to the built-in table by adapter key.
`config.PriceConfig.Provider` is a canonical key string, validated on load; an
invalid entry is rejected with a message naming the accepted grammar.
`pkg/pricing.BuiltinPrices` is rekeyed:

```go
var BuiltinPrices = []config.PriceConfig{
    {Provider: "llm:gemini", PerMillionInput: 125_000, PerMillionOutput: 500_000},
    {Provider: "llm:openaichat", PerMillionInput: 150_000, PerMillionOutput: 600_000},
}
```

The dead `elevenlabs` entry is removed: its rate is zero and it currently implies
a support that does not exist.

### 5.3 Rate limits and funds

`LimitKey.Provider` holds the most specific key. `gui.providerKeyForRole` is
deleted; `guardRole` and `noteFailure` resolve through the family resolver.
`Blocked` and `Block` consult the §3.4 candidate list, so a block on
`tts:http@hostA` does not stop `hostB`, while a block on `tts:http` stops both.

### 5.4 Failures

`harness.GenerationFailure` attempt metadata (`pkg/harness/failure.go`) carries
the most specific key instead of a role or adapter name, so a failure, a block,
and a ledger row can be joined. `Router.ProviderIDForRole` keeps its role
meaning for routing but is no longer used as a provider identity in errors.

### 5.5 Catalogue and UI

Descriptor IDs are canonical adapter keys, so `/api/providers` exposes keys
directly and `Descriptor.ID` is a `provider.Key`. The frontend catalog continues
to key by ID; the change is the ID's shape. Frontend preset values and
`SettingsStudio` builtin names are config values (`type`, `builtin_name`), not
keys, and are unaffected except where a provider ID is displayed.

### 5.6 Documentation

`pkg/gui/docs_catalogue_test.go` stops re-deriving a ledger key and calls the
resolver, so the published key is the produced key. The generated
`12-provider-catalogue.md` and `13-configuration-reference.md` are regenerated.
`11-usage-and-pricing.md` drops its prose rules table for the catalogue's keys,
and documents the instance fallback.

### 5.7 Embeddings

Embedding usage is recorded in this change, which is why the family is in scope:

- `embeddings.KeyFor` resolves `embedding:builtin`, `embedding:openai`,
  `embedding:gemini`, or an instance key from the configured endpoint, falling
  back to the literal discriminator `default` when no endpoint is named
  (`embedding:gemini@default`).
- The index worker and the memory tools receive a usage sink. `EmbeddingWorker`
  (`pkg/storage/embedding_worker.go:187`) and the tools executor
  (`pkg/tools/tools.go:130,294`, `pkg/tools/memory.go:41`) call
  `provider.LastUsage()` after a batch and record under role `embedding`.
- `geminiembedding` is extended to parse `UsageMetadata` and expose
  `LastUsage()`, matching `openaiembedding`
  (`pkg/provider/openaiembedding/openai.go:181`). `openaiembedding` keeps its
  parsed usage but reports the canonical key instead of `c.ID()`.
- A cache hit records nothing, mirroring the TTS pipeline.
- Embeddings run outside a turn, so their records use the global usage scope and
  turn `0`, the same convention previews already use.

### 5.8 Voice catalogue

The voice catalogue snapshot stores the instance key in its `Provider` field
(`pkg/media/voice_catalog.go`), so a catalogue fetched from one endpoint is never
served for another endpoint of the same adapter. Prices and blocks still resolve
through the adapter fallback of §3.4.

## 6. Uniqueness and Validation

- **R1 — well-formed.** Every registered descriptor ID parses as an adapter key,
  and every instance key built by a resolver parses as an instance key. Enforced
  by a startup validation and a test over `provider.List()`.
- **R2 — unique.** `provider.Register` already panics on a duplicate ID; the
  suite additionally asserts no two IDs share a `(family, adapter)` pair and that
  every family's IDs are distinct.
- **R3 — total and deterministic.** For every preset in the catalogue and a table
  of representative configs, the resolver returns a key; for every unsupported or
  disabled config, and for browser-only providers such as `stt:web-speech`, it
  returns `false` and nothing is recorded or blocked.
- **R4 — single producer.** A test scans `pkg/` for the removed patterns
  (hardcoded `Provider:` literals, `builtin_name || type` derivations, and
  `"<family>:"` literals outside `pkg/provider` and the resolver facades) and
  fails when one reappears.
- **R5 — documented.** Every key the resolver can produce appears in the
  generated catalogue; the catalogue's freshness test enforces this.
- **R6 — parented instances.** Every instance key has a parent adapter key: a
  test asserts `ParseKey(K).Parent()` is itself a registered adapter key, so a
  fallback always has somewhere to land.

## 7. Hard Cut and Compatibility

The user chose a hard cut: new keys only, no aliasing, no row rewriting.

- **Config version.** `config.Version` is bumped from `1` to `2`. On load, a
  version-1 file logs a one-time warning explaining that `providers.prices`
  entries now use `<family>:<adapter>` keys, optionally with `@<instance>`, and
  linking to the catalogue.
- **Price entries.** An entry whose `provider` is not a valid key is rejected at
  load with the accepted grammar and a pointer to the catalogue. It is not
  silently ignored, because silent ignore is what made the current state
  confusing.
- **No escape hatch.** There is no alias table and no way to price a legacy
  string: a version-1 key is simply not a valid key, so it can never match a
  configured entry. This is deliberate; the release note is the migration path.
- **Historical ledger rows.** Rows keep their old provider strings and always show
  **no price configured**. The Usage view continues to display them as history.
- **Built-in prices.** Rekeyed; the `elevenlabs` entry is removed. No historical
  row is rewritten.
- **Release note.** States the old-to-new key mapping for every adapter (the
  §3.2 table) so users can update their config by hand.

## 8. Data Model

- `storage.UsageRecord.Provider` stays `TEXT` and stores the key verbatim; the
  column is opaque and no schema migration is required.
- `storage` summaries group by the stored string, unchanged.
- `LimitKey.Provider` is an in-memory key.
- `config.PriceConfig.Provider` is a key string, validated at load.

## 9. Error Handling

- `NewKey`, `NewInstanceKey`, and `ParseKey` return an error naming the offending
  input and the grammar.
- `KeyFor` and the media/embedding resolvers return `false` for disabled or
  unknown config; callers skip recording rather than guessing.
- A price entry with an invalid `provider` fails config validation with the
  catalogue link, so the failure is at load, not at spend time.
- `Resolve` with an instance key whose adapter is unknown falls through to the
  parent, then the built-ins, then zero cost.
- A recorded key that fails `ParseKey` on read (a legacy row) is displayed as-is
  and never priced, because a configured entry must itself be a valid key.

## 10. Testing Strategy

- `TestAllRegistryIDsAreCanonicalKeys` — the registry validates (R1).
- `TestDescriptorKeysAreUniquePerFamily` — no `(family, adapter)` collision (R2).
- `TestResolverIsTotalForCatalogue` — every preset resolves to a key (R3).
- `TestResolverMatchesRecordingPaths` — for each family, a config table asserts
  the key the recorder stores equals the resolver's key (§5.1).
- `TestInstanceKeyFallbackPrecedence` — a table of entries and recorded keys
  asserts the §3.4 order, including instance-over-adapter and model-over-any.
- `TestLimitsAndLedgerShareProviderKey` — simulate a failure and a usage record
  for the same config and assert `LimitKey.Provider == Usage.Provider`.
- `TestInstanceBlocksDoNotLeak` — a block on `tts:http@hostA` leaves `hostB`
  usable while a block on `tts:http` stops both.
- `TestEmbeddingUsageIsRecorded` — a fake embedding provider with `LastUsage`
  produces a ledger row under role `embedding` with the canonical key.
- `TestEmbeddingDefaultInstanceKey` — an embedding provider with no endpoint
  resolves to `embedding:<adapter>@default`.
- `TestBrowserOnlyProvidersHaveNoKey` — `STTKeyFor` returns no key for
  `web-speech`, and no ledger row or block is produced.
- `TestNoSecondKeyDerivation` — source scan for removed patterns (R4).
- `TestInstanceKeysHaveRegisteredParents` — every resolver-produced instance key
  has a registered adapter parent (R6).
- `TestProviderCatalogueIsCurrent` — the existing freshness test, now driven by
  the resolver (R5).
- `TestPriceConfigRejectsLegacyKey` — an `openaichat` price fails validation with
  the canonical examples in the message.

## 11. Rollout

1. Add `provider.Key`, `ParseKey`, and the instance helpers; rekey registry IDs.
2. Add the family resolvers; keep `…ProviderIDFor` wrappers for one commit.
3. Point usage recording, pricing, limits, and failures at the resolver.
4. Wire embedding usage (worker and tools), and extend `geminiembedding`.
5. Delete the wrappers and the inline derivations.
6. Rekey `BuiltinPrices`; bump `config.Version`; add the rejection message.
7. Regenerate the catalogue and config reference; update `11-usage-and-pricing.md`
   and the release note.

Steps 1-2 are pure additions and keep the suite green; each later step is
independently reviewable.

## 12. Risks and Tradeoffs

- **Breaking change to prices.** Accepted explicitly. The version warning and the
  rejection message keep it from being silent.
- **Instance keys in config.** Users can now write either `tts:http` or
  `tts:http@localhost:8880`. The fallback makes the adapter form the common case
  and the instance form available when needed.
- **Embedding plumbing breadth.** Recording embedding usage touches the storage
  worker and the tools executor, both outside the turn loop. This is the largest
  new surface in the change.
- **Frontend churn.** Any UI that displays a provider ID now shows the colon form.
  Small, but visible.
- **`web-speech` is browser-only.** The `stt-webspeech` preset configures
  `type: web-speech`, which the server-side STT factory does not build and which
  therefore has no key. Its descriptor stays for the UI, and its usage is never
  recorded or blocked.

## 13. Alternatives Considered

- **Registry adapter ID only (`tts-elevenlabs`).** Simpler rename, but keeps the
  cross-family collision (`gemini` LLM vs image) unfixable without a second
  namespace, and does not express the family in the key users write.
- **Adapter key only, no instances.** Simple and stable, but two HTTP endpoints of
  one adapter can never be priced or blocked apart, which the current
  `media.ProviderKey` already partially supports.
- **Instance keys everywhere with no fallback.** Fully explicit, but forces every
  user to write the endpoint in each price and breaks the moment a server moves.
  The most-specific-first fallback avoids both.
- **Alias table with no rekeying.** Least disruptive, but leaves two key spaces
  alive forever and contradicts "one identity for everything".
- **Migrating historical rows on open.** Clean data, but a destructive rewrite of
  the canonical spend history.

## 14. Resolved Decisions

| Decision | Choice |
| --- | --- |
| Key shape | `<family>:<adapter>` (`llm:gemini`, `tts:gemini`, `image:gemini`) |
| Instance keys | Available for every family, with most-specific-first fallback to the adapter |
| Compatibility | Hard cut; no alias table, no row rewriting |
| Legacy price entries | Rejected at load; the release note is the migration path |
| `llm:openaichat` name | Kept; "OpenAI-compatible" is a wire specification, not a vendor |
| Embedding usage | In scope for this change |
| Embedding discriminator | Endpoint host, else the literal `default` |
| Voice catalogue `Provider` | Stores the instance key |
| `stt:web-speech` | Browser-only; no key, never recorded |

No open questions remain.

## 15. Self-Review Notes

- Grammar and examples cover every key in the §3.2 table plus the §3.3 instance
  forms.
- `pkg/provider` stays config-free, so the resolvers live in `pkg/harness`,
  `pkg/media`, and `pkg/embeddings`, consistent with the capability model.
- The §3.4 candidate list is stated once and reused by pricing, limits, funds, and
  failures rather than restated per consumer.
- Uniqueness is enforced by validation, by a source scan, and by the instance
  parent rule (R6), not only by convention.
- Browser-only providers are named as having no key (R3), so the resolver's
  totality claim is honest rather than a silent omission.
- Embedding recording is in scope with its call sites named (§5.7), so it is not
  an implicit "and then" in the rollout.
- Hard-cut behaviour is stated once (§7) and referenced by the data model and
  error handling.
