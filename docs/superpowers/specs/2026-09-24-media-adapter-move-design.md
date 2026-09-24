# Provider Capability Model — Media Adapter Move Specification

- **Date:** 2026-09-24
- **Status:** Proposed
- **Scope:** Move every media adapter implementation (TTS, STT, image) out of
  `pkg/media` and into its provider package, leaving `pkg/media` as interfaces,
  shared helpers, pipelines, catalogues, and registry facades.
- **Related:** `docs/superpowers/specs/2026-09-24-provider-capability-model-design.md`,
  `docs/superpowers/specs/2026-09-24-provider-capability-followups-design.md`,
  `pkg/media`, `pkg/provider`.

---

## 1. Context

The provider capability model and its first follow-up moved the LLM adapters into
their packages. The media adapters still live in `pkg/media`:

- `providers.go` defines `cliTTSClient`, `httpTTSClient`, `cliSTTClient`,
  `httpSTTClient`, `cliImageClient`, `httpImageClient`, `comfyUIImageClient`, and
  the `disabled*`/`echo*` placeholders, and its factories
  (`NewTTSClientWithSharedKey`, `NewSTTClient`, `NewImageClientWithSharedKey`)
  construct them in an inline switch.
- `native_tts.go`, `sherpa_tts.go`, `elevenlabs_tts.go`, `gemini_tts.go`,
  `procedural_art.go`, and `gemini_image.go` each define one client.

The provider packages already own each provider's descriptor, presets, and
`Build`; the media factories are registry-first and fall back to the inline
switch. This spec removes the inline adapters and the fallback switch for them.

## 2. Goals

1. Each media client lives in its provider package next to its descriptor.
2. `pkg/media` keeps only shared pieces: the family interfaces, capability
   interfaces, shared types, pipelines, catalogues, and the registry facades.
3. The media factories build only through the registry, keeping a fallback for
   the `disabled` and unknown-builtin cases.
4. Media tests move with the adapters they exercise.

## 3. Non-Goals

- Changing media behaviour, cache keys, or pipeline logic.
- Moving the TTS pipeline, content cache, speakable-text policy, or the Gemini
  catalogues out of `pkg/media`.
- Runtime plugins.

## 4. Target layout

| Client | Moved to |
|--------|----------|
| `nativeOSTTSClient` (`native_tts.go`) | `pkg/provider/ttsnativeos` |
| `SherpaTTSClient` (`sherpa_tts.go`) | `pkg/provider/ttssherpa` |
| `ElevenLabsTTSClient` (`elevenlabs_tts.go`) | `pkg/provider/ttselevenlabs` |
| `GeminiTTSClient` (`gemini_tts.go`) | `pkg/provider/ttsgemini` |
| `cliTTSClient` (`providers.go`) | `pkg/provider/ttspiper` |
| `httpTTSClient` (`providers.go`) | `pkg/provider/ttshttp` |
| `cliSTTClient` (`providers.go`) | `pkg/provider/sttwhispercli` |
| `httpSTTClient` (`providers.go`) | `pkg/provider/sttwhisperhttp` |
| `proceduralArtClient` (`procedural_art.go`) | `pkg/provider/imageprocedural` |
| `GeminiImageClient` (`gemini_image.go`) | `pkg/provider/imagegemini` |
| `cliImageClient` (`providers.go`) | `pkg/provider/imagecli` |
| `httpImageClient`, `comfyUIImageClient` (`providers.go`) | `pkg/provider/imagehttp` |

Kept in `pkg/media`:

- Interfaces: `TTSClient`, `STTClient`, `ImageClient`, and the capability
  interfaces (`VoiceCatalog`, `VoiceOptions`, `MeteredProvider`,
  `SpeechCueAdvertiser`, `MarkdownAware`, `ExtendedVoiceSearcher`).
- Shared types: `ProviderVoice`, `VoiceOption` (alias of `provider.Tunable`),
  `SpeechCueCapabilities`, `SpeechCueCapabilities` resolution.
- The placeholders `disabledTTSClient`, `disabledSTTClient`,
  `disabledImageClient`, `echoTTSClient`, `echoSTTClient`, `echoImageClient`.
- Pipelines and utilities: `TTSPipeline`, `ContentCache`, speakable-text policy,
  audio content type/extension, `ProviderKey`, `KeyPresent`.
- Catalogues: `voice_catalog.go`, `gemini_catalog.go`, `gemini_tts`'s prebuilt
  voice list where shared, `kokoro_profiles.go`.
- Facades: `NewTTSClientWithSharedKey`, `NewSTTClient`,
  `NewImageClientWithSharedKey` (and their `New*Client` wrappers).

## 5. Shared helpers

The moved clients need a small number of helpers that currently live in
`pkg/media`:

| Helper | Disposition |
|--------|-------------|
| `voiceOptions(voice)` | Export as `media.VoiceOptionsOf(*entity.VoiceConfig) map[string]interface{}`; used by ElevenLabs and Gemini TTS |
| `isComfyUI(endpoint)` | Move to `imagehttp` (exported within the package); `media` no longer needs it |
| `AudioExtension`, `AudioContentType` | Stay in `media`; already exported |
| `SpeakableTextFor`, `TextPolicyFromConfig` | Stay in `media`; already exported |
| `ResolveGeminiTTSAPIKey`, `ErrGeminiTTSAPIKeyRequired`, `ListGeminiVoices` | Stay in `media`; already exported |
| `ResolveGeminiImageAPIKey`, `ErrGeminiImageAPIKeyRequired` | Stay in `media`; already exported |
| `endpointHost`, `sanitiseKey` | Stay in `media`; used only by `ProviderKey` |

`gemini_tts.go`'s private `geminiPrebuiltVoices` and `mapGeminiTTSError` move with
it. `GeminiTTSClient.ListExtendedVoices` calls `media.ListGeminiVoices`.

## 6. Factories and fallback

After the move, the three factories keep registry-first construction and a
minimal fallback:

```go
func NewTTSClientWithSharedKey(cfg config.TTSConfig, sharedKey string) (TTSClient, error) {
	if id := TTSProviderIDFor(cfg); id != "" {
		if client, err := BuildTTS(id, cfg, sharedKey); err == nil {
			return client, nil
		}
	}
	// Only disabled and unknown builtins reach here.
	switch cfg.Type {
	case "disabled", "":
		return &disabledTTSClient{}, nil
	default:
		return &echoTTSClient{}, nil
	}
}
```

`NewSTTClient` and `NewImageClientWithSharedKey` follow the same shape. A test
proves that with the registry present the real client is built, and with it
absent the disabled/echo fallback still answers, so a backend-only test build
keeps working.

## 7. Import cycle

Provider packages import `pkg/media` for shared types; `pkg/media` imports
`pkg/provider` (leaf) for the facade; no provider subpackage is imported by
`pkg/media`. That is not a cycle.

The constraint is tests: `pkg/media`'s **internal** test files (package `media`)
may not import `provider/all`, because that would be `media → all →
provider/ttsgemini → media`. Any media test that needs registered providers moves
to an external `media_test` file (which may import `all`) or to the provider
package's own test. This mirrors the harness arrangement.

## 8. Testing

1. **Move parity:** each client's existing tests move to its provider package and
   keep passing. Tests that used unexported package internals become exported or
   move with the client.
2. **Factory fallback:** `media` tests assert the disabled/echo fallback and that
   a registered provider is preferred.
3. **Capability drift guard:** the existing `pkg/provider/all` drift guard already
   builds each media provider; it continues to hold after the move.
4. **Build tag independence:** a `go build ./pkg/media/...` with no provider
   packages imported still compiles (the facade has no compile-time dependency on
   subpackages).
5. `go vet ./...` and `go test -count=1 ./...` remain the gate.

## 9. Rollout

One plan, tasks ordered by coupling so each ends green:

1. Export `VoiceOptionsOf`; move `native-os`, `procedural-art`, `gemini-image`,
   `sherpa` (self-contained).
2. Move the CLI and HTTP TTS clients (`piper`, `openai-http`).
3. Move the STT clients (http, cli).
4. Move the image CLI and HTTP/ComfyUI clients.
5. Move `elevenlabs` and `gemini` TTS (they need `VoiceOptionsOf`).
6. Split `providers.go` tests, reduce factories to the disabled/echo fallback,
   and add the factory-fallback test.

## 10. Risks

| Risk | Mitigation |
|------|------------|
| `providers.go` split breaks a shared helper | Move one client per task; suite green each step |
| Media internal tests hit the `all` import cycle | Move them to external test packages or the provider package |
| `voiceOptions` export changes call sites | Add `VoiceOptionsOf` first, then update the two callers |
| ComfyUI detection duplicated | Move `isComfyUI` to `imagehttp`; no other caller |
| Fallback divergence | Factory test asserts disabled/echo and registry preference |
| Large diff obscures a behaviour change | No logic changes; only relocation and qualifier changes |
