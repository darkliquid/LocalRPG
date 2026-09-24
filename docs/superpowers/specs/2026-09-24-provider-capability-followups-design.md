# Provider Capability Model — Follow-ups Specification

- **Date:** 2026-09-24
- **Status:** Proposed
- **Scope:** Closes the three gaps left by the provider capability model:
  preset parity in Go, an id-preserving registry path for model providers, and
  the physical move of adapter implementations into their provider packages.
- **Related:** `docs/superpowers/specs/2026-09-24-provider-capability-model-design.md`,
  `pkg/provider`, `pkg/harness`, `pkg/media`,
  `frontend/src/lib/providerPresetsFallback.ts`.

---

## 1. Context

The provider capability model shipped with a leaf registry, self-registering
packages, capability derivation, a drift guard, `GET /api/providers`, and a
settings UI that overlays catalogue presets on a built-in fallback. Three gaps
remain:

1. **Preset parity.** Descriptor `Presets` are empty in Go. The frontend fallback
   (`providerPresetsFallback.ts`, renamed from `providerPresets.ts`) still holds
   every preset, so the catalogue is not yet the source of truth and the fallback
   cannot be deleted.
2. **Model provider ids.** `harness.NewModelProvider(id, cfg)` builds a provider
   whose `ID()` is the role name it was created for (`gm`, `narrator`). The
   registry's `Build` receives only the family config, so a registry-first path
   would return a provider named after its descriptor and break role routing.
   The LLM factory therefore still uses its inline switch.
3. **Adapter bodies.** The concrete provider implementations still live in
   `pkg/harness` and `pkg/media`; the provider packages own identity,
   descriptors, and construction but delegate to exported constructors. The
   self-registration pattern is real, the code move is not.

## 2. Goals

1. Move every preset into the provider that owns it, so `GET /api/providers`
   alone drives the settings UI, and delete the frontend fallback.
2. Let the registry build a model provider under the caller's role id, so
   `NewModelProvider` can delegate and the inline switch can be deleted.
3. Move each adapter implementation into its provider package, leaving
   `harness`/`media` as interfaces, shared helpers, and facades.

## 3. Non-Goals

- Changing turn execution, routing, or configuration schemas.
- Moving the router or context assembler out of `harness` (they stay; they use
  the facade).
- Runtime plugins; the provider set remains fixed at build time.

## 4. Gap 1 — Preset parity

### 4.1 Data move

Each provider package declares its `Descriptor.Presets` by moving the entries
from `providerPresetsFallback.ts` verbatim (same ids, labels, descriptions,
config, and `Order`), so the UI list order is unchanged.

| Family | Registry id | Presets |
|--------|-------------|---------|
| llm | `openaichat` | ollama, lm-studio, localai, vllm |
| llm | `clillm` | llama-cli, claude-cli |
| llm | `oracle` | narrative-oracle |
| llm | `geminillm` | gemini |
| tts | `ttssherpa` | sherpa-onnx |
| tts | `ttshttp` | kokoro-fastapi, alltalk, openai-speech |
| tts | `ttspiper` | piper |
| tts | `ttsnativeos` | native-os |
| tts | `ttselevenlabs` | elevenlabs |
| tts | `ttsgemini` | gemini-3.8-flash-tts, gemini-3.8-flash-lite-tts, gemini-3.1-flash-tts, gemini-2.5-flash-tts, gemini-2.5-pro-tts |
| stt | `sttwhisperhttp` | faster-whisper, openai-whisper |
| stt | `sttwhispercli` | whisper-cli |
| stt | `sttwebspeech` (new) | web-speech |
| image | `imagehttp` | comfyui, automatic1111, localai-image, dall-e-3 |
| image | `imagecli` | sd-cli |
| image | `imageprocedural` | procedural-art |
| image | `imagegemini` | imagen-3, imagen-3-fast, nano-banana-2, nano-banana-2-lite, nano-banana-pro, nano-banana |

### 4.2 The `web-speech` gap

`web-speech` is a browser-native STT type the backend never constructs
(`media.NewSTTClient` has no case for it). Today its preset lives only in the
frontend. To keep the catalogue authoritative:

- Register a `stt-webspeech` descriptor with a `Build` that returns a small
  `WebSpeechSTTClient` whose `Transcribe` returns an explanatory error ("web
  speech recognition runs in the browser; the backend client is a placeholder").
- The frontend keeps its own browser implementation; the descriptor exists so
  the preset and its capability list come from the catalogue.

### 4.3 Fallback removal

Once every preset is served by a descriptor, delete
`frontend/src/lib/providerPresetsFallback.ts` and drop the merge in
`SettingsStudio.tsx`. A guard test asserts the catalogue contains at least one
preset per family, so an empty catalogue fails CI rather than silently emptying
the UI.

## 5. Gap 2 — Id-preserving registry build

### 5.1 Payload

`harness` gains a build payload that carries the caller's id alongside the
config, mirroring `media.TTSBuildPayload`:

```go
type ModelBuildPayload struct {
	ID     string         `json:"id"`
	Config ProviderConfig `json:"config"`
}
```

Each LLM provider package unmarshals it and passes `payload.ID` to its
constructor, falling back to the descriptor id when empty (the drift guard
builds without an id).

### 5.2 Facade

```go
// BuildModelFor builds the provider for cfg's registry id, named id.
func BuildModelFor(id string, cfg ProviderConfig) (ModelProvider, error) {
	regID := ProviderIDFor(cfg)
	if regID == "" {
		return nil, fmt.Errorf("harness: no registry provider for type %q", cfg.Type)
	}
	reg, ok := provider.Lookup(regID)
	if !ok {
		return nil, fmt.Errorf("harness: provider %q is not registered", regID)
	}
	raw, err := json.Marshal(ModelBuildPayload{ID: id, Config: cfg})
	...
	model, ok := built.(ModelProvider)
	...
}
```

`NewModelProvider` becomes registry-first and keeps the inline switch only as a
fallback for configurations the registry does not model (the debug echo). A test
proves a registry-built provider reports the caller's id and that
`RouterFromConfig` resolves every role by name.

## 6. Gap 3 — Move adapter bodies

### 6.1 Target layout

| Moved from | Moved to |
|------------|----------|
| `harness/http_provider.go` | `provider/openaichat` |
| `harness/cli_provider.go` | `provider/clillm` |
| `harness/gemini_provider.go` | `provider/geminillm` |
| `harness/oracle_provider.go` | `provider/oracle` |
| `media/native_tts.go` | `provider/ttsnativeos` |
| `media/sherpa_tts.go` | `provider/ttssherpa` |
| `media/elevenlabs_tts.go` | `provider/ttselevenlabs` |
| `media/gemini_tts.go` | `provider/ttsgemini` |
| `media/providers.go` (cli/http TTS, STT, image types) | split across the matching `provider/*` packages |
| `media/procedural_art.go` | `provider/imageprocedural` |
| `media/gemini_image.go` | `provider/imagegemini` |

`harness` and `media` keep: the family interfaces, shared types and helpers
(`GenerateRequest`, `Message`, `ProviderVoice`, `VoiceOptions` alias, cache,
speakable text, HTTP transports), the registry facades, and the router/context.

### 6.2 No cycle

Provider packages import `harness`/`media` for types; `harness`/`media` import
`pkg/provider` (leaf) for the facade, never the subpackages. That is not a cycle
because `pkg/provider` and `provider/openaichat` are distinct packages.

The one real constraint is tests: `harness`'s internal test files cannot import
`provider/all` (that would be `harness → all → openaichat → harness`). Move any
test that needs registered providers into an external `harness_test` file, or
register a test-only provider directly. This is the same constraint already
handled in `pkg/gui`.

### 6.3 Shared helpers

Helpers used by more than one provider package move to a small internal package
`pkg/provider/internal/httpx` (importable only under `pkg/provider`) or stay in
`media`/`harness` and are exported. Prefer keeping them in `harness`/`media` and
exporting, to avoid a new internal tree.

## 7. Testing

1. **Preset guard:** `pkg/provider/all` asserts each family has at least one
   preset and that every preset `Config` unmarshals into its family config.
2. **Id preservation:** a harness test builds via `BuildModelFor("gm", cfg)` and
   asserts `ID() == "gm"`, plus a router test that every configured role
   resolves.
3. **Move parity:** for each provider, the registry-built value is the same type
   as before (compile-time) and its existing package tests move with it.
4. **Frontend:** `tsc --noEmit` after the fallback is deleted; a catalogue stub
   with presets renders them.
5. `go vet ./...` and `go test -count=1 ./...` remain the gate.

## 8. Rollout

One plan, four tasks:

1. Preset parity in Go for every family, plus `stt-webspeech`.
2. Id-preserving `BuildModelFor`, registry-first `NewModelProvider`, switch
   deletion.
3. Move the LLM adapters into their packages; relocate the harness test that
   needs registration.
4. Move the media adapters; delete the frontend fallback.

## 9. Risks

| Risk | Mitigation |
|------|------------|
| Preset data drift during the move | Move verbatim with `Order`; guard test on configs |
| Role id regression | Id-preservation test and router role test |
| Import cycle in internal tests | External test packages; the cycle is package-scoped, not production |
| Large move breaks helpers | Move one family per task; suite green each step |
| Fallback deletion empties the UI | Preset guard test fails if a family has no presets |
