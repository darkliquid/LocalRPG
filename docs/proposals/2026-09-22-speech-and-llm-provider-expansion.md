# Proposal: Speech and LLM Provider Expansion

**Date:** 2026-09-22
**Status:** Proposal (not approved; no implementation started)
**Scope:** `pkg/media`, `pkg/config`, `pkg/harness`, `pkg/gui`, `frontend`

**Research:** deep-research report at
`~/.local/share/crush/research/localrpg-voice-and-llm-providers/report.md` (sources registry alongside
it). This document turns that research into concrete, reviewable proposals.

**Specs written from this proposal:**
- P1/P2/P4/P5 - `docs/superpowers/specs/2026-09-22-tts-provider-capabilities-design.md`
- P3 - `docs/superpowers/specs/2026-09-22-elevenlabs-tts-provider-design.md`

P6 (OmniVoice adapter) and P7 (LLM model catalog / OmniLLM) remain proposals only.

---

## 1. Summary of the problem

LocalRPG's speech layer is a single method (`Synthesize`) plus authored voice archetypes. That works
for our own engines because we know their options. It does not work for providers like ElevenLabs,
where:

1. the voice list is **account-scoped and fetched** (`GET /v2/voices`: 5000+ voices, paged, filtered,
   labelled), and
2. every voice has **provider-specific tunables** (`stability`, `similarity_boost`, `style`,
   `use_speaker_boost`, `speed`) plus request-level choices (`model_id`, `output_format`, `seed`).

Two gaps follow: no **voice catalog** and no **provider option schema**. Without them, adding a
provider degrades to "one voice id in a text box", and adding a tunable silently breaks the
content-addressed audio cache.

The same shape exists on the LLM side: no model catalog, no per-provider request options beyond
temperature/max tokens.

---

## 2. What we are proposing, in one line each

| # | Proposal | Effort | Value |
| --- | --- | --- | --- |
| P1 | Provider capability interfaces: `VoiceCatalog`, `VoiceOptions` | S | Unblocks all provider work |
| P2 | Profile/entity option maps + option-aware audio cache key | S | Correctness; no more collisions |
| P3 | ElevenLabs built-in TTS provider | M | The requested provider |
| P4 | Voice catalog API + caching + Settings/codex UI | M | Makes the catalog usable |
| P5 | Cost guardrails for metered providers | S | Protects players' wallets |
| P6 | Optional `omnivoice` adapter behind P1 | M | Breadth without coupling |
| P7 | LLM side: `ModelCatalog` + optional `omnillm` adapter | M | Anthropic/Gemini/Bedrock |

Recommended order: **P1 → P2 → P3 → P4 → P5**, then reassess P6/P7 against demand.

---

## 3. P1 - Provider capability interfaces (`pkg/media`)

Small optional interfaces asserted at runtime, mirroring the `MarkdownAware` pattern we already use
and the capability-interface design OmniVoice validates (`VoiceCloner`, `ReferenceSynthesizer`, ...).

```go
// ProviderVoice is one voice a provider offers.
type ProviderVoice struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Language    string         `json:"language,omitempty"`   // BCP-47 when known
	Gender      string         `json:"gender,omitempty"`
	Accent      string         `json:"accent,omitempty"`
	Categories  []string       `json:"categories,omitempty"` // e.g. premade, cloned
	Tags        []string       `json:"tags,omitempty"`       // normalised matching vocabulary
	Description string         `json:"description,omitempty"`
	PreviewURL  string         `json:"preview_url,omitempty"`
	Defaults    map[string]any `json:"defaults,omitempty"`   // provider's stored settings
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// VoiceCatalog is implemented by providers that can enumerate voices.
type VoiceCatalog interface {
	ListVoices(ctx context.Context) ([]ProviderVoice, error)
}

// VoiceOption describes one tunable a provider accepts. The UI renders from
// this, and the value map is validated against it, so a provider never needs a
// bespoke settings screen.
type VoiceOption struct {
	Key     string         `json:"key"`
	Label   string         `json:"label"`
	Kind    string         `json:"kind"` // float | int | bool | string | enum
	Min     float64        `json:"min,omitempty"`
	Max     float64        `json:"max,omitempty"`
	Step    float64        `json:"step,omitempty"`
	Options []string       `json:"options,omitempty"`
	Default any            `json:"default,omitempty"`
	Help    string         `json:"help,omitempty"`
}

// VoiceOptions is implemented by providers that declare their tunables.
type VoiceOptions interface {
	VoiceOptions() []VoiceOption
}
```

Notes:
- `ListVoices` takes a context because it is a network call; a provider with a static list simply
  returns it.
- `Tags` is the join point with `harness.AssignVoiceProfile`: catalog labels are normalised into the
  same lowercase vocabulary our archetypes already use (`american`, `female`, `elder`, ...), so one
  matcher scores both.
- `Metadata` carries anything catalog-specific we do not model (`category`, `available_for_tiers`,
  `verified_languages`) without polluting the typed fields.
- `VoiceOptions` is per provider *instance* (a value receiver), so a provider with modes can vary it.

---

## 4. P2 - Options on profiles and entities, and a cache key that includes them

### 4.1 Schema

`config.VoiceProfile` gains:

```go
	// Options holds provider-declared tunables for this profile, keyed by
	// VoiceOption.Key. It is opaque to the engine, exactly like entity State:
	// only the provider interprets it.
	Options map[string]any `yaml:"options,omitempty" json:"options,omitempty"`
```

`entity.VoiceConfig` gains the same `Options map[string]any`, persisted in frontmatter:

```yaml
voice:
  provider: elevenlabs
  voice_id: EXAVITQu4vr4xnSDxMaL
  options:
    stability: 0.35
    similarity_boost: 0.8
    style: 0.2
    model: eleven_multilingual_v2
```

Rules:
- Absent options mean "provider defaults"; we never invent values.
- Unknown keys are preserved on load and dropped before the request, so a profile written for a newer
  provider version degrades instead of failing.
- Reserved keys: `model` (request model) and `format` (output format). Everything else is
  provider-defined.
- This keeps the engine schema-agnostic: `options` is a map the same way `state` is.

### 4.2 Cache key

The current key is `ComputeAudioCacheKeyWithRate(speakerID, voiceID, pitch, speechRate, text)`. Any
new tunable must participate or two tunings collide.

```go
// ComputeAudioCacheKeyForVoice hashes everything that changes the audio.
func ComputeAudioCacheKeyForVoice(speakerID string, voice *entity.VoiceConfig, text string) string

// ComputeAudioCacheKeyWithRate is kept for callers with no option map and
// delegates to the above, preserving warm caches for existing campaigns.
func ComputeAudioCacheKeyWithRate(speakerID, voiceID string, pitch, speechRate float64, text string) string
```

Canonicalisation: sort option keys, format floats to fixed precision, and hash
`provider|voice_id|pitch|rate|len|json(options)|text`. A regression test must assert that changing
`stability` alone changes the key, and that key order does not.

### 4.3 Validation

`ValidateVoiceOptions(options []VoiceOption, values map[string]any) (map[string]any, []string)` clamps
and reports: unknown keys dropped, out-of-range clamped, type mismatches dropped with a message.
Called at save time (Settings) and at synthesis time (defence in depth).

---

## 5. P3 - ElevenLabs built-in provider (`pkg/media/elevenlabs_tts.go`)

**Wiring:** `type: builtin`, `builtin_name: elevenlabs`, with `api_key`, `model`
(default `eleven_multilingual_v2`), `default_voice`, and option defaults on the TTS config. A
`TTSPresets["elevenlabs"]` entry (disabled by default, no key) so it is discoverable but never
implicit -- consistent with the repo's no-defaults stance.

**API mapping**

| LocalRPG | ElevenLabs |
| --- | --- |
| `voice.VoiceID` | path `{voice_id}` |
| `options["model"]` or `cfg.Model` | `model_id` |
| `voice.Pitch` | *not supported directly*; approximate with `speed` only, and say so in `Help` |
| `voice.SpeechRate` | `voice_settings.speed` |
| `options["stability" ...]` | `voice_settings{stability, similarity_boost, style, use_speaker_boost}` |
| `options["format"]` or `cfg` | `output_format` query |
| `text` | `text` (after `SpeakableText`, since ElevenLabs does not read Markdown) |

**Behaviour**
- `ListVoices` paginates `GET /v2/voices` (`page_size=100`, `next_page_token`), maps `labels` to
  normalised tags, keeps `category`/`description`/`preview_url`/`settings` as metadata/defaults.
- `VoiceOptions` returns the four voice-setting sliders plus `model` (enum of the models we support)
  and `format` (enum of `mp3_44100_128`, `pcm_24000`, `ulaw_8000`, ...). PCM/ulaw chosen deliberately:
  our pipeline already sniffs bytes and names files from content, so format is free.
- Does **not** implement `MarkdownAware`: the existing `SpeakableText` reduction applies, and the
  `markdown` policy already gives an escape hatch.
- Errors are mapped to actionable messages: 401 "API key rejected", 402/429 "quota/rate limit", 422
  "voice or model not available on this account".
- Streaming (`/stream`) is **not** implemented in this proposal; `Synthesize` is a single
  buffered call, which matches how the pipeline writes whole beats to disk.

**Deliberately out of scope:** Conversational AI, agents, phone/Twilio, voice cloning, pronunciation
dictionaries, `previous_text`/`next_text` continuity. Recorded as future work in §10.

---

## 6. P4 - Catalog + options API and UI

### Endpoints

```
GET /api/tts/voices?provider=<id>[&refresh=1]
  -> { "provider": "elevenlabs", "fetched_at": "...", "stale": false, "voices": [ ... ] }

GET /api/tts/options?provider=<id>
  -> { "provider": "elevenlabs", "options": [ ... ] }

POST /api/settings/test-provider        (existing; gains an "options" field and an optional
                                         voice_id so a preview can audition a catalog voice)
```

### Caching

Catalogs are written to `<cache>/voices/<provider>.json` with `fetched_at`. Policy:
serve fresh for 24h, refresh on explicit request, and **serve stale on network failure** (a player
choosing a voice offline still sees the last known list). Costs nothing to implement and prevents a
cloud outage from breaking the voice picker.

### Settings Studio

- TTS panel renders provider option controls from the schema (sliders, selects, toggles) instead of
  hard-coded pitch/rate fields; the existing pitch/rate remain for providers that declare them.
- "Voice catalog: 5,412 voices, refreshed 2h ago" with a Refresh button and a search field.
- Per-profile option overrides in the NPC Voice Profiles library, validated against the schema.

### Codex voice picker

- Two sections: **Archetypes** (authored, tag-matched, provider-scoped) and **Provider voices**
  (catalog, searchable, filterable by category/gender/accent/language).
- Audition via `preview_url` (free, no synthesis credits) before committing.
- "Add as profile" turns a catalog pick into a `config.VoiceProfile` bound to that provider and voice
  id, with the provider's stored settings as the default options map.
- Profiles whose `provider` does not match the active TTS provider are shown greyed with a
  "belongs to <provider>" hint rather than being silently skipped.

This is the direct answer to "voice profiles should load lists of what is available from a provider and
configure the various options supported by each one".

---

## 7. P5 - Cost guardrails for metered providers

The content cache already makes a clip pay-once, but nothing stops a player spending real money by
accident. Propose:

- A provider capability flag `Metered() bool` (or a config field `metered: true`), surfaced in UI.
- Warn before whole-campaign operations (export with audio, "replay all turns", regenerating audio
  after a voice change), stating the number of uncached beats.
- A per-session counter of synthesised characters with the running provider's approximate price in
  the trace panel (already have `media.tts.request` events to count).
- Never auto-play a metered provider for the *first* turn without the warning having been seen once.

---

## 8. P6 - Optional `omnivoice` adapter (deferred)

If we later want Cartesia, Deepgram Aura, Azure, or Google without writing four clients, wrap
`omnivoice-core`'s `tts.Provider` behind P1's interfaces:

```go
type omniVoiceClient struct {
	provider tts.Provider          // e.g. elevenlabs.New(...)
	options  []VoiceOption
}

func (c *omniVoiceClient) Synthesize(ctx context.Context, text string, v *entity.VoiceConfig) ([]byte, error)
func (c *omniVoiceClient) ListVoices(ctx context.Context) ([]ProviderVoice, error)
func (c *omniVoiceClient) VoiceOptions() []VoiceOption
```

Shape mapping: `entity.VoiceConfig` -> `tts.SynthesisConfig` (`Options` -> typed fields where they
exist, everything else -> `Extensions["<provider>.<key>"]`); `tts.Voice` -> `ProviderVoice`.

Why deferred, not now:
- It is a 0.x dependency (2 stars, single org, fast minor churn) for providers nobody has asked for
  yet; ElevenLabs is ~200 lines natively.
- The parts that would justify it (transports, gateways, call systems, barge-in) are real-time
  telephony, which a turn-based narrative app does not use.
- Adopting it behind P1 keeps the option open at bounded cost: one file, one `go.mod` entry.

A spike to validate the mapping is worth doing before committing, not after.

---

## 9. P7 - LLM side

The same two gaps exist: no model catalog, no per-provider request options.

**P7a - `ModelCatalog` (do this regardless).** A capability interface analogous to `VoiceCatalog`:
providers that can list models (`GET /v1/models` for OpenAI-compatible servers, a static list for
CLI/builtin, Ollama's `/api/tags`) expose `ListModels`, and the Settings role editor gets a dropdown
instead of a free-text model field.

```go
type ModelInfo struct { ID, Name, ContextWindow string; Metadata map[string]any }
type ModelCatalog interface { ListModels(ctx context.Context) ([]ModelInfo, error) }
```

**P7b - `omnillm` adapter (optional).** A `builtin_name: omnillm` provider implementing
`harness.ModelProvider`, configured with a provider name, `api_key`, `base_url`, and model. This buys
Anthropic/Gemini/Bedrock plus circuit breaking and token estimation without us writing three protocol
clients. Trade-offs: a 0.x dependency, an extra abstraction layer between us and the API, and no
model catalog (so P7a stays ours).

**P7c - Native Anthropic/Gemini providers (alternative).** Mirror P3: two small HTTP clients behind
the existing `ModelProvider`. More code, no dependency, full control. Pick this if we want only one or
two cloud LLMs and would rather not carry a framework.

Recommendation: **P7a now, P7b/P7c only when a specific provider is requested.** The OpenAI-compatible
`http` path already covers Ollama, vLLM, LM Studio, LocalAI, llama.cpp, Groq, and Together, and
`narrative-oracle` guarantees an offline experience.

---

## 10. Explicitly considered and rejected (for now)

| Idea | Why not now |
| --- | --- |
| Adopt `omnivoice-core` as the core TTS contract | Couples a 0.x, 2-star dependency to every provider, including our own built-ins; drags in telephony concepts |
| Adopt `omnillm-core` as the core LLM contract | Roles/`inherit`/`disabled`, built-ins, and CLI providers are ours and load-bearing; the adapter would be larger than the value |
| ElevenLabs streaming (`/stream`) | We render whole beats to files; streaming only pays off with streaming playback |
| Voice cloning / Conversational AI / Twilio | Different product; account-scoped cloned voices also raise consent questions in shared campaign files |
| Shipping a static ElevenLabs voice list | It is account-scoped and exceeds 500 voices; must be fetched |
| Reading Markdown via ElevenLabs pronunciation dictionaries | `SpeakableText` already solves it provider-agnostically |

---

## 11. Acceptance criteria (for the P1-P5 slice)

1. A provider can declare its options and enumerate its voices; the Settings UI renders controls from
   the declaration with no provider-specific frontend code.
2. The voice picker lists catalog voices by provider, searches them, and auditions them for free.
3. A profile can carry provider options, they persist into the entity note, and they are visible in
   the codex.
4. Changing any option changes the audio cache key; two tunings never share a clip.
5. The ElevenLabs preset is opt-in, requires a key, and never becomes a default.
6. Catalogs survive a network outage by serving the last known list.
7. `go test -count=1 ./...`, `go vet ./...`, and `npx tsc --noEmit` pass.

---

## 12. Open decisions for the reviewer

1. **Provider key storage.** Plain `api_key` in `config.yaml` (current pattern) versus an environment
   variable (`ELEVENLABS_API_KEY`) versus a keyring. Plain config is consistent but writes a secret to
   disk; the GUI should at minimum mask it and support env fallback.
2. **Second cloud provider.** Cartesia (quality/latency) or Deepgram Aura (cost) or neither.
3. **Pin catalogs into content.** Should a world ship "these are the cast voices", hard-coding
   account-scoped ids, or stay provider-agnostic?
4. **Expose options to rules systems.** JS hooks could require "narrator: low stability"; not
   proposed here.
5. **`omnivoice`/`omnillm` adoption trigger.** What concrete need flips P6/P7b on -- a named provider,
   a dependency-count ceiling, or user demand?
