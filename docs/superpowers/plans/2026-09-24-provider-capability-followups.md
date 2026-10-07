# Provider Capability Model — Follow-ups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the three gaps left by the provider capability model: preset parity in Go, an id-preserving registry build for model providers, and moving each adapter implementation into its provider package.

**Architecture:** `pkg/provider` stays a leaf registry. Provider packages own descriptors, presets, and `Build`, and (in this plan) the adapter implementations themselves, importing `harness`/`media` for types. `harness`/`media` keep interfaces, shared helpers, and facades that read the registry; they never import the subpackages, so there is no production cycle.

**Tech Stack:** Go 1.27.1; `pkg/provider`, `pkg/harness`, `pkg/media`, `pkg/gui`; React 19 + TypeScript strict.

**Spec:** `docs/superpowers/specs/2026-09-24-provider-capability-followups-design.md`

## Global Constraints

- `pkg/provider` imports no internal package; provider packages import `harness`/`media`; `harness`/`media` import `pkg/provider` but never a subpackage.
- A model provider's `ID()` is the role id it was built for.
- Move preset data verbatim, preserving ids and `Order`.
- Use `any`, not `interface{}`; wrap errors with `%w`; stdlib tests only.
- `go vet ./...`, `go test -count=1 ./...`, and `npx tsc --noEmit` are the gate.

---

### Task 1: Preset parity in Go

**Files:**
- Modify: `pkg/provider/openaichat/openaichat.go`, `clillm/clillm.go`, `oracle/oracle.go`, `geminillm/geminillm.go`
- Modify: `pkg/provider/ttssherpa`, `ttshttp`, `ttspiper`, `ttsnativeos`, `ttselevenlabs`, `ttsgemini`
- Modify: `pkg/provider/sttwhisperhttp`, `sttwhispercli`
- Create: `pkg/provider/sttwebspeech/sttwebspeech.go`
- Modify: `pkg/provider/imagehttp`, `imagecli`, `imageprocedural`, `imagegemini`
- Create: `pkg/provider/all/presets_test.go`
- Create: `pkg/media/webspeech_stt.go` (placeholder client)

**Interfaces:**
- Consumes: `provider.Preset`, `provider.Descriptor.Presets`.
- Produces: a catalogue with at least one preset per family.

- [x] **Step 1: Write the failing preset guard test**

Create `pkg/provider/all/presets_test.go`:

```go
package all_test

import (
	"encoding/json"
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestEachFamilyHasPresets(t *testing.T) {
	for _, family := range []provider.Family{
		provider.FamilyLLM, provider.FamilyTTS, provider.FamilySTT, provider.FamilyImage,
	} {
		count := 0
		for _, desc := range provider.List(family) {
			count += len(desc.Presets)
		}
		if count == 0 {
			t.Errorf("family %s has no presets in the catalogue", family)
		}
	}
}

func TestEveryPresetConfigUnmarshals(t *testing.T) {
	for _, desc := range provider.List() {
		for _, preset := range desc.Presets {
			raw, err := json.Marshal(preset.Config)
			if err != nil {
				t.Fatalf("preset %s/%s: encode: %v", desc.ID, preset.ID, err)
			}
			var target interface{}
			switch desc.Family {
			case provider.FamilyLLM:
				target = &harness.ProviderConfig{}
			case provider.FamilyTTS:
				target = &config.TTSConfig{}
			case provider.FamilySTT:
				target = &config.STTConfig{}
			case provider.FamilyImage:
				target = &config.ImageConfig{}
			}
			if err := json.Unmarshal(raw, target); err != nil {
				t.Errorf("preset %s/%s does not fit its family config: %v", desc.ID, preset.ID, err)
			}
		}
	}
}
```

Run: `go test -run 'TestEachFamilyHasPresets|TestEveryPresetConfigUnmarshals' ./pkg/provider/all/ -v`
Expected: FAIL — no family has presets.

- [x] **Step 2: Move the preset data into descriptors**

For each provider package in the spec's §4.1 table, add the presets from
`frontend/src/lib/providerPresetsFallback.ts` verbatim. Example,
`pkg/provider/openaichat/openaichat.go`:

```go
Presets: []provider.Preset{
	{ID: "ollama", Order: 1, Label: "Ollama (Local HTTP)",
		Description: "Connects to local Ollama on port 11434 with llama3.2.",
		Config: map[string]interface{}{
			"type": "http", "endpoint": "http://localhost:11434/v1",
			"model": "llama3.2", "temperature": 0.7, "max_tokens": 1024,
		}},
	{ID: "lm-studio", Order: 2, Label: "LM Studio (Local HTTP)",
		Description: "Connects to LM Studio local server on port 1234.",
		Config: map[string]interface{}{
			"type": "http", "endpoint": "http://localhost:1234/v1",
			"model": "default", "temperature": 0.7, "max_tokens": 1024,
		}},
	// localai, vllm ...
},
```

Use string keys and plain JSON values so the config unmarshals into the family
config. Keep the `Order` fields matching the fallback file so the UI does not
reshuffle. (The TTS/STT/image packages are filled the same way from the same
table.)

- [x] **Step 3: Register the web-speech STT descriptor**

Create `pkg/media/webspeech_stt.go`:

```go
package media

import (
	"context"
	"errors"
)

// ErrWebSpeechIsClientSide reports that browser speech recognition has no
// backend client. The descriptor exists so the catalogue can publish the preset
// and its capabilities; the browser performs the recognition.
var ErrWebSpeechIsClientSide = errors.New("web speech recognition runs in the browser; the backend client is a placeholder")

type webSpeechSTTClient struct{}

func (webSpeechSTTClient) Transcribe(context.Context, []byte) (string, error) {
	return "", ErrWebSpeechIsClientSide
}

// NewWebSpeechSTTProvider returns the placeholder browser-speech client.
func NewWebSpeechSTTProvider() STTClient { return webSpeechSTTClient{} }
```

Create `pkg/provider/sttwebspeech/sttwebspeech.go` registering descriptor
`stt-webspeech` (builtin, feature `offline`) with the `web-speech` preset and a
`Build` returning `media.NewWebSpeechSTTProvider()`. Add the import to
`pkg/provider/all/all.go`.

- [x] **Step 4: Run the guard test**

Run: `go test -count=1 ./pkg/provider/all/ -v`
Expected: PASS.

- [x] **Step 5: Verify the whole suite and commit**

Run: `go vet ./... && go test -count=1 ./...`
Expected: PASS.

```bash
git add pkg/provider pkg/media
git commit -m "feat(provider): move every preset into the provider catalogue"
```

---

### Task 2: Id-preserving registry build

**Files:**
- Modify: `pkg/harness/factory.go` (`ModelBuildPayload`, `BuildModelFor`, registry-first `NewModelProvider`)
- Modify: `pkg/provider/openaichat`, `clillm`, `oracle`, `geminillm` (read the id from the payload)
- Create: `pkg/harness/factory_id_test.go`

**Interfaces:**
- Produces: `harness.ModelBuildPayload`, `harness.BuildModelFor(id string, cfg ProviderConfig) (ModelProvider, error)`.

- [x] **Step 1: Write the failing id-preservation test**

```go
package harness_test

import (
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestBuildModelForPreservesID(t *testing.T) {
	model, err := harness.BuildModelFor("gm", harness.ProviderConfig{Type: "http", Endpoint: "http://localhost:11434/v1"})
	if err != nil {
		t.Fatalf("BuildModelFor: %v", err)
	}
	if model.ID() != "gm" {
		t.Fatalf("provider id = %q, want gm", model.ID())
	}
}
```

Run: `go test -run TestBuildModelForPreservesID ./pkg/harness/ -v`
Expected: FAIL — undefined `BuildModelFor`.

- [x] **Step 2: Add the payload and facade**

In `pkg/harness/factory.go`:

```go
type ModelBuildPayload struct {
	ID     string         `json:"id"`
	Config ProviderConfig `json:"config"`
}

// BuildModelFor builds the provider for cfg's registry id, named id, so role
// routing keeps working when construction goes through the registry.
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
	if err != nil {
		return nil, fmt.Errorf("harness: encode %s config: %w", regID, err)
	}
	built, err := reg.Build(context.Background(), raw)
	if err != nil {
		return nil, err
	}
	model, ok := built.(ModelProvider)
	if !ok {
		return nil, fmt.Errorf("harness: provider %q is not a model provider", regID)
	}
	return model, nil
}
```

Keep the existing `BuildModel` for the drift guard, or make it call
`BuildModelFor(id, cfg)` with the descriptor id.

- [x] **Step 3: Read the id in each LLM provider package**

Change each `Build` to unmarshal `harness.ModelBuildPayload` and use
`payload.ID`, falling back to the descriptor id when empty:

```go
Build: func(_ context.Context, raw []byte) (interface{}, error) {
	var payload harness.ModelBuildPayload
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
	}
	id := payload.ID
	if id == "" {
		id = "openaichat"
	}
	return harness.NewOpenAIChatProvider(id, payload.Config), nil
},
```

- [x] **Step 4: Make `NewModelProvider` registry-first and delete the switch**

```go
func NewModelProvider(id string, cfg ProviderConfig) (ModelProvider, error) {
	if cfg.Type == "disabled" {
		return &disabledModelProvider{id: id}, nil
	}
	if model, err := BuildModelFor(id, cfg); err == nil {
		return model, nil
	}
	// Only the debug echo fallback and the disabled role reach here.
	return &builtinEchoModelProvider{id: id}, nil
}
```

Delete the now-dead inline cases from `factory.go`. Move any internal harness
test that needs registered providers into an external `harness_test` file (the
internal test package cannot import `provider/all` without a cycle).

- [x] **Step 5: Run the tests and commit**

Run: `go vet ./... && go test -count=1 ./pkg/harness/ ./pkg/engine/ ./pkg/provider/... ./pkg/gui/`
Expected: PASS.

```bash
git add pkg/harness pkg/provider
git commit -m "refactor(provider): build model providers through the registry by role id"
```

---

### Task 3: Move the LLM adapters

**Files:**
- Move: `pkg/harness/http_provider.go` -> `pkg/provider/openaichat/`
- Move: `pkg/harness/cli_provider.go` -> `pkg/provider/clillm/`
- Move: `pkg/harness/gemini_provider.go` -> `pkg/provider/geminillm/`
- Move: `pkg/harness/oracle_provider.go` -> `pkg/provider/oracle/`
- Move: their `_test.go` files alongside
- Modify: `pkg/harness/factory.go` (`NewOpenAIChatProvider` etc. move or thin out)
- Modify: `pkg/harness/router.go` if it referenced the moved types

**Interfaces:**
- Consumes: `harness.ModelProvider`, `harness.GenerateRequest`, `harness.ToolCaller`.
- Produces: the same provider behaviour from the new packages.

- [x] **Step 1: Confirm the cycle boundary**

Verify `pkg/harness` imports `pkg/provider` but no subpackage:

```bash
go list -deps ./pkg/harness | grep 'pkg/provider/' || echo "no subpackage import (correct)"
```

- [x] **Step 2: Move one provider at a time**

For each provider: move the file into its package, change `package harness` to
the new package name, export the constructor as needed, add the `provider` import
where the types now live, and update the factory facade to construct via the
registry. Move its tests too, adjusting the package name. Run the suite after
each provider.

- [x] **Step 3: Verify and commit**

Run: `go vet ./... && go test -count=1 ./...`
Expected: PASS.

```bash
git add pkg/harness pkg/provider
git commit -m "refactor(provider): move the llm adapters into their packages"
```

---

### Task 4: Move the media adapters and delete the fallback

**Files:**
- Move: `pkg/media/native_tts.go`, `sherpa_tts.go`, `elevenlabs_tts.go`, `gemini_tts.go`, `procedural_art.go`, `gemini_image.go`
- Split: `pkg/media/providers.go` into the TTS/STT/image provider packages
- Modify: `pkg/media/providers.go` to keep only shared helpers and the facades
- Delete: `frontend/src/lib/providerPresetsFallback.ts`
- Modify: `frontend/src/components/SettingsStudio.tsx` (drop the merge, use catalogue presets directly)

**Interfaces:**
- Produces: no frontend preset fallback; the catalogue is the only source.

- [x] **Step 1: Move one media provider at a time**, running `go test ./pkg/media/... ./pkg/provider/...` after each, ending with `go test -count=1 ./...`.

- [x] **Step 2: Delete the fallback and simplify the UI**

Remove `providerPresetsFallback.ts`; in `SettingsStudio.tsx` build the preset
maps directly from `catalogPresets(family)` and drop `mergePresets`.

- [x] **Step 3: Verify**

Run: `go vet ./... && go test -count=1 ./... && cd frontend && npx tsc --noEmit`
Expected: PASS.

- [x] **Step 4: Commit**

```bash
git add pkg/media pkg/provider frontend/src
git commit -m "refactor(provider): move the media adapters and drop the preset fallback"
```

---

## File Map

| File | Responsibility |
|------|----------------|
| `pkg/provider/*/**.go` | Each provider's descriptor, presets, adapter, and Build |
| `pkg/provider/sttwebspeech` | Browser-speech placeholder descriptor and preset |
| `pkg/provider/all/presets_test.go` | Preset parity guard |
| `pkg/harness/factory.go` | `ModelBuildPayload`, `BuildModelFor`, registry-first factory |
| `pkg/media/providers.go` | Shared media helpers and registry facades only |
| `frontend/src/components/SettingsStudio.tsx` | Catalogue-only preset rendering |
| `frontend/src/lib/providerPresetsFallback.ts` | Deleted |

## Self-Review

- **Spec coverage:** §4 preset parity → Task 1; §4.2 web-speech → Task 1 Step 3; §4.3 fallback removal → Task 4; §5 id preservation → Task 2; §6 adapter move → Tasks 3-4; §7 tests appear in every task.
- **Placeholder scan:** no TBD/TODO. Preset data is moved from a named source file; the id-preservation and guard tests contain runnable code.
- **Type consistency:** `provider.Preset` (existing) and `harness.ModelBuildPayload`/`BuildModelFor` are defined before use; provider package names match the registry layout.
