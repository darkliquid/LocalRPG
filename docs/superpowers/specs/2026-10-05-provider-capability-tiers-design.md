# Provider Capability Tiers Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#34 LF-3](https://github.com/darkliquid/LocalRPG/issues/34)
**Epic:** [#17 Zero-GPU and local-first offerings](https://github.com/darkliquid/LocalRPG/issues/17)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §2 (LF-3)
**Scope:** `pkg/provider`, `pkg/gui`, `frontend`, `pkg/gui/docs`

---

## 1. Problem

LocalRPG classifies every provider with `Descriptor.Source` (`builtin | cli | http | gemini`,
`pkg/provider/descriptor.go:70`) and a set of `Feature`s including `FeatureOffline`,
`FeatureKeyRequired`, and `FeatureMetered` (`pkg/provider/descriptor.go:19-36`). Those flags drive
diagnostics, but nothing tells a **user** what a provider actually is.

The result is that `llm:narrative-oracle` ("Deterministic pure-Go storyteller that needs no model
or network", `pkg/provider/oracle/oracle.go:17`) and `llm:gemini` both appear in the same catalogue
with no indication that one writes template prose and the other is a frontier model. A new user
who picks the offline option expecting a local model is disappointed, and the disappointment is
the product's fault, not theirs.

The catalogue (`GET /api/providers` → `ProviderCatalogDTO`, `pkg/gui/catalogue.go:15-17`) and the
preset picker render `Descriptor` fields but never say "this is not that".

## 2. Goals

- Every provider carries an honest, coarse **tier** that a user can understand at a glance.
- The tier comes with a one-line **caveat** stating what it is not.
- The catalogue, the preset picker, and the docs all show the tier.
- The tier cannot silently drift from the provider's real capabilities.

## 3. Non-goals

- A full onboarding page and capability matrix. That is LF-5.
- Changing any provider's behaviour.
- A new provider. This is labelling only.

## 4. Design

### 4.1 A tier vocabulary

In `pkg/provider/descriptor.go`:

```go
// Tier is a coarse, user-facing classification of how a provider runs.
type Tier string

const (
	// TierOfflineBasic is pure algorithm or template: no model, no network.
	TierOfflineBasic Tier = "offline-basic"
	// TierOfflineNeural runs a small model in-process on the CPU, no network.
	TierOfflineNeural Tier = "offline-neural"
	// TierLocalServer needs a server the user runs (Ollama, Kokoro-FastAPI,
	// ComfyUI, faster-whisper). Local, but only offline if that server is.
	TierLocalServer Tier = "local-server"
	// TierCloud sends data to a remote provider and needs a key.
	TierCloud Tier = "cloud"
)
```

`Descriptor` gains:

```go
	Tier   Tier   `json:"tier"`
	Caveat string `json:"caveat,omitempty"`
```

The `Caveat` is a short, plain sentence. When empty, the UI falls back to the tier's default caveat
(§4.3), so most adapters set only the tier.

### 4.2 Which tier each provider declares

| Provider | Tier |
| --- | --- |
| `llm:narrative-oracle` | offline-basic |
| `tts:native-os` | offline-basic (falls back to a tone) |
| `image:procedural-art` | offline-basic |
| `embedding:builtin` | offline-basic |
| `stt:web-speech` | offline-basic (browser only) |
| `tts:sherpa-onnx` | offline-neural |
| `llm:cli`, `tts:piper`, `stt:whisper-cli`, `image:cli` | local-server |
| `tts:http`, `stt:whisper-http`, `image:http` | local-server |
| `llm:openaichat`, `llm:gemini`, `llm:inworld` | cloud |
| `tts:gemini`, `tts:elevenlabs`, `tts:fish-audio`, `tts:inworld`, `tts:cartesia` | cloud |
| `stt:inworld`, `stt:cartesia` | cloud |
| `image:gemini` | cloud |
| `embedding:openai`, `embedding:gemini` | cloud |

A `cli` adapter is `local-server` because the binary it shells out to may reach the network; the
tier describes the architecture, and the caveat makes the caveat explicit.

### 4.3 Default caveats

One function maps a tier to its default sentence, in `pkg/provider/tier.go`:

```go
// TierCaveat returns the default honest description of a tier.
func TierCaveat(t Tier) string {
	switch t {
	case TierOfflineBasic:
		return "Runs with no model and no network. Deterministic and simple; its output is limited and repetitive next to a model."
	case TierOfflineNeural:
		return "Runs a small model on your CPU with no network. Quality is well below a large local or cloud model."
	case TierLocalServer:
		return "Needs a server you run yourself. Local, but only offline while that server is."
	case TierCloud:
		return "Sends your text to a remote provider and needs an API key. Usually metered."
	}
	return ""
}
```

A descriptor's own `Caveat` overrides the default.

### 4.4 A drift guard

The provider registry already validates descriptors (`provider.Validate`,
`pkg/provider/provider.go:78-97`). Extend it:

- `TierCloud` requires `FeatureKeyRequired`.
- `TierOfflineBasic` and `TierOfflineNeural` require `FeatureOffline`.
- `TierLocalServer` requires neither, and must not claim `FeatureKeyRequired` unless it is really
  cloud.

A mismatch is a registration error, caught by the existing drift-guard test in `pkg/provider/all`,
so a new adapter cannot ship an untrue tier.

### 4.5 Where it shows

- **Catalogue.** `ProviderCatalogDTO` already carries the whole `Descriptor`, so the tier arrives
  without a DTO change. The catalogue UI renders a `TierBadge` (a small pill) with the tier label
  and the caveat as a title/tooltip and an inline sub-line.
- **Preset picker.** The preset list shows the same badge, so "Zero-GPU" presets are labelled
  offline-basic or offline-neural rather than a bare "Zero-GPU".
- **Docs.** A short "How providers run" section in `pkg/gui/docs/05-providers.md` (or the new LF-5
  page) lists the four tiers and their caveats verbatim, generated from the same source if
  practical.

Tier labels, user-facing:

| Tier | Label |
| --- | --- |
| offline-basic | Offline · basic |
| offline-neural | Offline · small model |
| local-server | Local server |
| cloud | Cloud |

## 5. Behaviour

| Situation | Result |
| --- | --- |
| Catalogue lists `narrative-oracle` | badge "Offline · basic" + caveat |
| Catalogue lists `gemini` | badge "Cloud" + caveat |
| A descriptor declares cloud without `key_required` | registration error |
| A descriptor declares offline-neural without `offline` | registration error |
| A descriptor sets its own caveat | shown instead of the default |

## 6. Testing

- `pkg/provider`: `TierCaveat` returns a non-empty string for every tier; a descriptor with a
  mismatched tier/feature fails `Validate`.
- `pkg/provider/all`: the drift-guard test asserts every registered descriptor has a valid tier and
  passes the tier/feature check.
- `pkg/gui`: the catalogue DTO carries the tier and caveat.
- `frontend`: `TierBadge` renders the label and caveat; a component test for each tier.
- `mise run lint:docs` for the doc section.

## 7. Rollout

Additive to the descriptor and the catalogue JSON. Older clients ignore the new fields. Every
adapter must be updated in the same change, because the drift guard fails registration for a
descriptor with an empty tier; the guard should treat an empty tier as "unclassified" during a
one-release transition or be enforced immediately if all adapters are updated together (preferred).

## 8. Risks

- **Tier debates.** Is `tts:native-os` offline-basic or offline-neural? It shells out to an OS voice
  and falls back to a tone (`pkg/provider/ttsnativeos/native.go:78-82`), so offline-basic with a
  caveat that mentions the tone is honest.
- **Over-promising offline-neural.** Sherpa-ONNX Kokoro is genuinely good for TTS but the tier also
  covers a future small LLM, where "small model" would be generous. The caveat's "well below a
  large local or cloud model" is the hedge.
- **Churn.** Adding a field to every descriptor touches many small files. It is mechanical and the
  drift guard makes it safe.
