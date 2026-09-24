# HTTP TTS Client Move Specification

- **Date:** 2026-09-24
- **Status:** Proposed
- **Scope:** Move the OpenAI-compatible HTTP TTS client, its endpoint resolver,
  and its voice-catalog support out of `pkg/media` into
  `pkg/provider/ttshttp`.
- **Related:** `docs/superpowers/specs/2026-09-24-media-adapter-move-design.md`,
  `pkg/media/providers.go`, `pkg/provider/ttshttp`, `pkg/gui/tts_inspect.go`.

---

## 1. Context

The media adapter move relocated the self-contained clients and the CLI TTS
client. The last TTS adapter still in `pkg/media` is the HTTP client, and it is
larger than the others because it carries three things at once:

- `ResolveHTTPEndpoints(endpoint) (speechURL, voicesURL)` — normalises a base
  URL, a full `/v1/audio/speech` URL, or an AllTalk URL.
- `httpTTSClient.Synthesize` — the OpenAI-compatible request, with a Kokoro
  response-format/voice-tags variant and an AllTalk `/api/tts-generate` variant.
- `httpTTSClient.ListVoices` — a `VoiceCatalog` implementation that reads a
  Kokoro-FastAPI voice list, plus `SpeechCueCapabilities`.

`media.NewTTSClientWithSharedKey` still constructs it in the inline case, and
`media`'s `providers.go` and `providers_test.go` are its only other users.

## 2. Goals

1. `httpTTSClient` lives in `pkg/provider/ttshttp` next to its descriptor.
2. `ResolveHTTPEndpoints` moves with it and stays exported, because the HTTP
   client and its tests are its only callers.
3. `pkg/media` keeps only the interfaces and shared types the client implements.
4. The media TTS factory builds HTTP clients through the registry only,
   retaining the disabled/echo fallback.

## 3. Non-Goals

- Moving the STT, image, ElevenLabs, or Gemini adapters (separate tasks).
- Changing request shapes, cache keys, or voice-catalogue behaviour.
- Moving `CachedVoiceCatalog`, `ProviderKey`, or `KeyPresent`.

## 4. Target shape

`pkg/provider/ttshttp` gains:

```go
// NewHTTPTTSClient builds the OpenAI-compatible speech client.
func NewHTTPTTSClient(cfg config.TTSConfig) media.TTSClient

// ResolveHTTPEndpoints normalises a speech endpoint into a synthesis URL and a
// voice-catalogue URL. Exported because it is the client's own test surface.
func ResolveHTTPEndpoints(endpoint string) (speechURL, voicesURL string)

type httpTTSClient struct { endpoint, model, apiKey string; client *http.Client }

func (h *httpTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error)
func (h *httpTTSClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error)
func (h *httpTTSClient) SpeechCueCapabilities() media.SpeechCueCapabilities
var _ media.VoiceCatalog = (*httpTTSClient)(nil)
```

`kokoroVoiceItem` moves with `ListVoices`.

`pkg/media` keeps `TTSClient`, `VoiceCatalog`, `VoiceCatalog` snapshot cache,
`ProviderVoice`, `SpeechCueCapabilities` and `ResolveSpeechCueCapabilities`.

## 5. Registration and factory

The `ttshttp` descriptor's `Build` calls `NewHTTPTTSClient(payload.Config)`
instead of `media.NewHTTPTTSProvider`. In `pkg/media/providers.go`:

- remove the `case "http":` TTS branch; registry-first already builds it.
- remove `NewHTTPTTSProvider` from `pkg/media/exports.go`.

The fallback keeps `disabled` and the echo case.

## 6. Import cycle

`ttshttp` imports `pkg/media` for the shared interface and types;
`pkg/media` imports `pkg/provider` (leaf) for the facade and never imports
`ttshttp`. That is not a cycle.

Tests move to an external `ttshttp_test` package. A test that needs registered
providers imports `provider/all`; `pkg/media`'s internal tests must not.

## 7. Testing

1. `ResolveHTTPEndpoints` cases (base URL, full speech URL, `/v1`, AllTalk) move
   from `providers_test.go` to `ttshttp` and keep passing.
2. A `Synthesize` test asserts the OpenAI request shape and the Kokoro
   `response_format`/`allow_voice_tags` fields, using an `httptest` server.
3. A `ListVoices` test asserts the Kokoro voice list maps into `ProviderVoice`.
4. A media factory test asserts the HTTP TTS config builds the `ttshttp` client
   when the registry is present and the echo fallback when it is not.
5. `go vet ./...` and `go test -count=1 ./...` remain the gate.

## 8. Risks

| Risk | Mitigation |
|------|------------|
| `ResolveHTTPEndpoints` has hidden callers | Confirmed only `providers.go` and its tests use it; grep before moving |
| Voice-catalogue tests depend on media internals | Move them to the provider package; keep shared catalogue tests in media |
| Kokoro detection duplicated | Keep one branch inside `Synthesize`; no new helper |
| Factory fallback regression | Explicit factory test for registry-present and registry-absent |
