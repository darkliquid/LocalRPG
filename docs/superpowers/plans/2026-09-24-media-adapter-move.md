# Media Adapter Move Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move every media adapter out of `pkg/media` into its provider package, leaving `pkg/media` as interfaces, shared helpers, pipelines, catalogues, and registry facades.

**Architecture:** Each provider package owns its client next to its descriptor and `Build`. `pkg/media` keeps the family interfaces and shared types and imports the leaf registry for its facades; it never imports a provider subpackage. Provider packages import `pkg/media` for shared types. Tests that need registered providers live in external test packages.

**Tech Stack:** Go 1.27.1; `pkg/media`, `pkg/provider`.

**Spec:** `docs/superpowers/specs/2026-09-24-media-adapter-move-design.md`

## Global Constraints

- No behaviour change: move code and qualify identifiers only.
- `pkg/media` imports `pkg/provider` (leaf), never a subpackage.
- Media internal test files (package `media`) must not import `provider/all`.
- Use `any`, not `interface{}`; wrap errors with `%w`; stdlib tests only.
- `go vet ./...` and `go test -count=1 ./...` are the gate.

---

### Task 1: Export helpers and move the self-contained adapters

**Files:**
- Modify: `pkg/media/elevenlabs_tts.go` (public helper) or add `pkg/media/voice_options.go`
- Move: `pkg/media/native_tts.go` -> `pkg/provider/ttsnativeos/native.go`
- Move: `pkg/media/procedural_art.go` -> `pkg/provider/imageprocedural/art.go`
- Move: `pkg/media/gemini_image.go` -> `pkg/provider/imagegemini/client.go`
- Move: `pkg/media/sherpa_tts.go` -> `pkg/provider/ttssherpa/client.go`
- Move their `_test.go` counterparts
- Modify: `pkg/media/providers.go` (drop the moved cases from the factories)

**Interfaces:**
- Produces: `media.VoiceOptionsOf(*entity.VoiceConfig) map[string]interface{}`.

- [x] **Step 1: Add the exported helper**

```go
// VoiceOptionsOf returns a voice's provider options, or nil when it carries none.
func VoiceOptionsOf(voice *entity.VoiceConfig) map[string]interface{} {
	return voiceOptions(voice)
}
```

Confirm no cycle: `go build ./pkg/media/`.

- [x] **Step 2: Move one adapter and require its constructor locally**

For each adapter: `git mv` the file into the provider package, change
`package media` to the provider package, import `pkg/media` plus shared packages,
qualify every `media` identifier (`TTSClient`, `ImageClient`, `ProviderVoice`,
`SpeechCueCapabilities`, `ResolveGeminiImageAPIKey`, `ListGeminiVoices`, ...), and
change the provider's `Build` to call the local constructor instead of
`media.New*`.

Example for `ttsnativeos`:

```go
// pkg/provider/ttsnativeos/native.go
package ttsnativeos

import (
	"context"
	...
	"github.com/darkliquid/localrpg/pkg/media"
)

type nativeOSTTSClient struct{}

func NewNativeOSTTSClient() media.TTSClient { return &nativeOSTTSClient{} }
...
```

and its registration:

```go
Build: func(_ context.Context, _ []byte) (interface{}, error) {
	return NewNativeOSTTSClient(), nil
},
```

- [x] **Step 3: Drop the moved cases from the media factories**

In `pkg/media/providers.go`, remove the `native-os`, `sherpa-onnx`/`kokoro`,
`gemini`, `elevenlabs`, `procedural-art`, and `gemini` image branches from the
inline switches. The registry-first path already builds them; the fallback keeps
`disabled` and the echo cases only.

- [x] **Step 4: Move the tests**

Move each moved file's `_test.go` to the provider package, changing the test
package to `<package>_test` (or `<package>` for tests that need internals) and
qualifying `media.` identifiers. If a test needs registered providers, add the
blank import `_ "github.com/darkliquid/localrpg/pkg/provider/all"`.

- [x] **Step 5: Verify and commit**

Run: `go vet ./... && go test -count=1 ./...`
Expected: PASS.

```bash
git add pkg/media pkg/provider
git commit -m "refactor(provider): move the self-contained media adapters"
```

---

### Task 2: Move the CLI and HTTP TTS clients

**Files:**
- Move: `cliTTSClient` from `pkg/media/providers.go` -> `pkg/provider/ttspiper/client.go`
- Move: `httpTTSClient` from `pkg/media/providers.go` -> `pkg/provider/ttshttp/client.go`
- Move: the matching tests out of `pkg/media/providers_test.go`
- Modify: `pkg/media/providers.go` (drop those cases)

**Interfaces:**
- Produces: `ttspiper.NewCLITTSClient(cfg config.TTSConfig) media.TTSClient`, `ttshttp.NewHTTPTTSClient(cfg config.TTSConfig) media.TTSClient`.

- [x] **Step 1: Move `cliTTSClient`**

Copy the type and its `Synthesize` into `pkg/provider/ttspiper/client.go`, drop
the media inline case, point `Build` at `NewCLITTSClient`, and move its tests.

- [x] **Step 2: Move `httpTTSClient`**

Same for `ttshttp`, keeping the shared transport from
`telemetry.HTTPTransport(nil)`.

- [x] **Step 3: Verify and commit**

Run: `go vet ./... && go test -count=1 ./...`
Expected: PASS.

```bash
git add pkg/media pkg/provider
git commit -m "refactor(provider): move the cli and http tts clients"
```

---

### Task 3: Move the STT clients

**Files:**
- Move: `cliSTTClient` -> `pkg/provider/sttwhispercli/client.go`
- Move: `httpSTTClient` -> `pkg/provider/sttwhisperhttp/client.go`
- Move: matching tests

- [x] **Step 1: Move both clients**, dropping their media cases and pointing each
  `Build` at the local constructor.

- [x] **Step 2: Verify and commit**

Run: `go vet ./... && go test -count=1 ./...`
Expected: PASS.

```bash
git add pkg/media pkg/provider
git commit -m "refactor(provider): move the stt clients"
```

---

### Task 4: Move the image CLI and HTTP/ComfyUI clients

**Files:**
- Move: `cliImageClient` -> `pkg/provider/imagecli/client.go`
- Move: `httpImageClient`, `comfyUIImageClient`, `isComfyUI` -> `pkg/provider/imagehttp/client.go`
- Move: matching tests

- [x] **Step 1: Move `cliImageClient`**, drop the media case, point `Build` at the
  local constructor.

- [x] **Step 2: Move the HTTP and ComfyUI clients**, taking `isComfyUI` with them
  and dropping media's use of it.

- [x] **Step 3: Verify and commit**

Run: `go vet ./... && go test -count=1 ./...`
Expected: PASS.

```bash
git add pkg/media pkg/provider
git commit -m "refactor(provider): move the image cli and http clients"
```

---

### Task 5: Move ElevenLabs and Gemini TTS

**Files:**
- Move: `pkg/media/elevenlabs_tts.go` -> `pkg/provider/ttselevenlabs/client.go`
- Move: `pkg/media/gemini_tts.go` -> `pkg/provider/ttsgemini/client.go`
- Move: their tests
- Modify: `pkg/media/providers.go` (drop both cases)

**Interfaces:**
- Consumes: `media.VoiceOptionsOf`, `media.ListGeminiVoices`, `media.ResolveGeminiTTSAPIKey`.

- [x] **Step 1: Move both adapters**, replacing `voiceOptions(voice)` with
  `media.VoiceOptionsOf(voice)` and pointing each `Build` at the local
  constructor.

- [x] **Step 2: Move the tests**, adding the `provider/all` import where a test
  needs registered providers.

- [x] **Step 3: Verify and commit**

Run: `go vet ./... && go test -count=1 ./...`
Expected: PASS.

```bash
git add pkg/media pkg/provider
git commit -m "refactor(provider): move the elevenlabs and gemini tts clients"
```

---

### Task 6: Reduce the factories and split the tests

**Files:**
- Modify: `pkg/media/providers.go` (factories keep only the disabled/echo fallback)
- Modify: `pkg/media/providers_test.go` (keep fallback and shared-helper tests)
- Create: `pkg/media/factory_test.go`

**Interfaces:**
- Produces: registry-only `NewTTSClientWithSharedKey`, `NewSTTClient`, `NewImageClientWithSharedKey`.

- [x] **Step 1: Write the factory-fallback test**

```go
package media_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestDisabledFactoriesReturnPlaceholders(t *testing.T) {
	tts, err := media.NewTTSClient(config.TTSConfig{Type: "disabled"})
	if err != nil || tts == nil {
		t.Fatalf("disabled tts: %v", err)
	}
	stt, err := media.NewSTTClient(config.STTConfig{Type: "disabled"})
	if err != nil || stt == nil {
		t.Fatalf("disabled stt: %v", err)
	}
	img, err := media.NewImageClient(config.ImageConfig{Type: "disabled"})
	if err != nil || img == nil {
		t.Fatalf("disabled image: %v", err)
	}
}
```

- [x] **Step 2: Trim the switches** to the disabled/echo fallback and confirm the
  registry-first branch still builds the real clients in a binary that imports
  `provider/all`.

- [x] **Step 3: Split `providers_test.go`** so the moved-client tests live with
  their clients and `media` keeps only fallback, catalogue, pipeline, and shared
  helper tests.

- [x] **Step 4: Verify and commit**

Run: `go vet ./... && go test -count=1 ./...`
Expected: PASS.

```bash
git add pkg/media pkg/provider
git commit -m "refactor(provider): build media clients through the registry only"
```

---

## File Map

| File | Responsibility |
|------|----------------|
| `pkg/media/providers.go` | Registry facades and the disabled/echo fallback only |
| `pkg/media/catalog.go`, `voice_catalog.go`, `gemini_catalog.go` | Shared types, catalogues, `ProviderKey` |
| `pkg/media/tts.go`, `speakable.go`, `cache.go` | Pipeline, cache, and text policy |
| `pkg/provider/*/client.go` | One moved adapter per provider package |
| `pkg/provider/*/**_test.go` | The adapter's tests, beside it |
| `pkg/media/factory_test.go` | Fallback and registry-preference tests |

## Self-Review

- **Spec coverage:** §4 layout → Tasks 1-5; §5 helpers → Task 1 Step 1 and Task 5; §6 fallback → Task 6; §7 cycle → test-package moves in each task; §8 tests appear in every task.
- **Placeholder scan:** no TBD/TODO; the fallback test is runnable and each task names the exact file moves.
- **Type consistency:** `media.VoiceOptionsOf`, the client names, and the `Build`-to-local-constructor pattern match the spec; provider package names match the registry layout.
