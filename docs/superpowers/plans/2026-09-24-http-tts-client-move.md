# HTTP TTS Client Move Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the OpenAI-compatible HTTP TTS client, its endpoint resolver, and its voice-catalog support from `pkg/media` into `pkg/provider/ttshttp`, and let the media TTS factory build it through the registry only.

**Architecture:** `ttshttp` owns the client next to its descriptor and `Build`. `pkg/media` keeps the shared interfaces and types and imports the leaf registry for its facade; it never imports `ttshttp`. Tests move to an external `ttshttp_test` package.

**Tech Stack:** Go 1.27.1; `pkg/media`, `pkg/provider/ttshttp`.

**Spec:** `docs/superpowers/specs/2026-09-24-http-tts-client-move-design.md`

## Global Constraints

- No behaviour change: move code and qualify identifiers only.
- `pkg/media` imports `pkg/provider` (leaf), never `ttshttp`.
- Media internal test files (package `media`) must not import `provider/all`.
- Use `any`, not `interface{}`; wrap errors with `%w`; stdlib tests only.
- `go vet ./...` and `go test -count=1 ./...` are the gate.

---

### Task 1: Extract the client and endpoint resolver into `ttshttp`

**Files:**
- Create: `pkg/provider/ttshttp/client.go`
- Modify: `pkg/media/providers.go` (remove the client, `kokoroVoiceItem`, `ResolveHTTPEndpoints`, the `http` case)
- Modify: `pkg/media/exports.go` (remove `NewHTTPTTSProvider`)
- Modify: `pkg/provider/ttshttp/ttshttp.go` (local constructor)

**Interfaces:**
- Produces: `ttshttp.NewHTTPTTSClient(config.TTSConfig) media.TTSClient`, `ttshttp.ResolveHTTPEndpoints(string) (string, string)`.

- [x] **Step 1: Confirm the resolver has no other callers**

Run: `grep -rn "ResolveHTTPEndpoints" --include=*.go pkg cmd | grep -v _test`
Expected: only `pkg/media/providers.go`.

- [x] **Step 2: Create the provider client**

Move `ResolveHTTPEndpoints`, `httpTTSClient`, `kokoroVoiceItem`, and the three
methods into `pkg/provider/ttshttp/client.go`, changing `package media` to
`package ttshttp` and qualifying `media` identifiers (`TTSClient`,
`ProviderVoice`, `VoiceCatalog`, `SpeechCueCapabilities`). Add the constructor:

```go
// NewHTTPTTSClient builds the OpenAI-compatible speech client.
func NewHTTPTTSClient(cfg config.TTSConfig) media.TTSClient {
	return &httpTTSClient{
		endpoint: cfg.Endpoint,
		model:    cfg.Model,
		apiKey:   cfg.APIKey,
		client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 30 * time.Second},
	}
}
```

Keep `ResolveHTTPEndpoints` exported.

- [x] **Step 3: Point the registration at the local constructor**

In `pkg/provider/ttshttp/ttshttp.go`, replace
`media.NewHTTPTTSProvider(payload.Config)` with `NewHTTPTTSClient(payload.Config)`.

- [x] **Step 4: Remove the media pieces**

- Delete the moved block from `pkg/media/providers.go` and the `case "http":`
  TTS branch.
- Delete `NewHTTPTTSProvider` from `pkg/media/exports.go`.

Run: `go build ./...`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/media pkg/provider/ttshttp
git commit -m "refactor(provider): extract the http tts client into its package"
```

---

### Task 2: Move the tests

**Files:**
- Create: `pkg/provider/ttshttp/client_test.go`
- Modify: `pkg/media/providers_test.go` (remove the moved cases)
- Create: `pkg/media/factory_test.go` if not present

**Interfaces:**
- Consumes: `ttshttp.NewHTTPTTSClient`, `ttshttp.ResolveHTTPEndpoints`.

- [x] **Step 1: Move the `ResolveHTTPEndpoints` table test**

Cut `TestResolveHTTPEndpoints` from `pkg/media/providers_test.go`, change the
package to `ttshttp_test`, and call `ttshttp.ResolveHTTPEndpoints`.

- [x] **Step 2: Add a synthesize test and a voice-list test**

```go
package ttshttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/provider/ttshttp"
)

func TestSynthesizeSendsKokoroFields(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Write([]byte("RIFF....WAVE"))
	}))
	defer server.Close()

	client := ttshttp.NewHTTPTTSClient(config.TTSConfig{Type: "http", Endpoint: server.URL, Model: "kokoro"})
	if _, err := client.Synthesize(context.Background(), "hello", &entity.VoiceConfig{VoiceID: "af_bella"}); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if !strings.Contains(body, `"response_format":"mp3"`) || !strings.Contains(body, `"allow_voice_tags":true`) {
		t.Errorf("expected kokoro fields, got %s", body)
	}
}
```

For the voice list, serve a Kokoro-style JSON array and assert
`ListVoices` maps it to `[]media.ProviderVoice`, using `media` only for the type.

- [x] **Step 3: Add the factory fallback test** in `pkg/media/factory_test.go`:

```go
package media_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestHTTPTTSFallsBackWithoutRegistry(t *testing.T) {
	client, err := media.NewTTSClient(config.TTSConfig{Type: "http", Endpoint: "http://localhost:9"})
	if err != nil || client == nil {
		t.Fatalf("expected a fallback client, got %v %v", client, err)
	}
}
```

- [x] **Step 4: Verify and commit**

Run: `go vet ./... && go test -count=1 ./...`
Expected: PASS.

```bash
git add pkg/media pkg/provider/ttshttp
git commit -m "test(provider): move the http tts tests beside the client"
```

---

### Task 3: Confirm no residue and the registry path

**Files:**
- Modify: `pkg/media/providers.go` (confirm only disabled/echo remain for TTS)

- [x] **Step 1: Grep for residue**

Run: `grep -rn "ResolveHTTPEndpoints\|httpTTSClient\|NewHTTPTTSProvider" --include=*.go pkg | grep -v "pkg/provider/ttshttp"`
Expected: no matches.

- [x] **Step 2: Add a registry-preference test**

In `pkg/media/factory_test.go`, blank-import `provider/all` in a separate
external file and assert that a `type: http` TTS config builds a client whose
`ListVoices` reports a `VoiceCatalog` (proving the `ttshttp` client was built,
not the echo fallback). If an internal media test would need `all`, place the
test in `media_test` instead.

- [x] **Step 3: Verify and commit**

Run: `go vet ./... && go test -count=1 ./...`
Expected: PASS.

```bash
git add pkg/media pkg/provider/ttshttp
git commit -m "test(provider): assert the http tts client is built from the registry"
```

---

## File Map

| File | Responsibility |
|------|----------------|
| `pkg/provider/ttshttp/client.go` | HTTP TTS client, endpoint resolver, voice list, cue capabilities |
| `pkg/provider/ttshttp/ttshttp.go` | Descriptor, presets, and `Build` |
| `pkg/provider/ttshttp/client_test.go` | Moved and new client tests |
| `pkg/media/providers.go` | Registry facade and disabled/echo fallback only |
| `pkg/media/factory_test.go` | Fallback and registry-preference tests |

## Self-Review

- **Spec coverage:** §4 shape → Task 1; §5 factory/registration → Task 1 Steps 3-4 and Task 3; §6 cycle → external test packages in Task 2; §7 tests → Task 2 and Task 3.
- **Placeholder scan:** no TBD/TODO; the synthesize test is concrete, and the voice-list test names its assertion.
- **Type consistency:** `ttshttp.NewHTTPTTSClient` and `ttshttp.ResolveHTTPEndpoints` are defined in Task 1 and used unchanged in Tasks 2-3; the media fallback matches the factory shape already in the tree.
