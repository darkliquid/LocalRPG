# Provider Capability Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make provider capabilities a single, self-registering source of truth: every provider lives in its own package, registers into `pkg/provider` on import, and the settings UI renders from descriptors instead of provider names.

**Architecture:** `pkg/provider` is a leaf package holding the registry and the `Descriptor` vocabulary. Each provider package registers itself in `init`. `harness` and `media` keep their interfaces and gain facades that build through the registry, so existing call sites never change. `GET /api/providers` exposes the catalogue and the frontend consumes it.

**Tech Stack:** Go 1.27.1, existing `pkg/harness`, `pkg/media`, `pkg/gui`, `pkg/config`; React 19 + TypeScript strict, Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-09-24-provider-capability-model-design.md`

## Global Constraints

- `pkg/provider` imports **no** internal package; it is a leaf.
- Provider packages import `pkg/provider` and register in `init`; duplicate or empty IDs panic.
- `harness`/`media` keep their existing interfaces and constructor signatures; migration is per family and the suite stays green.
- Use `interface{}`, not `any`; wrap errors with `%w`; tests are stdlib-only (`testing`, `t.TempDir()`).
- When adding an endpoint, update `Service`, `server.go`, `frontend/src/types.ts`, and `client.ts` together.
- `go vet ./...` and `go test -count=1 ./...` plus `npx tsc --noEmit` are the gate.

---

### Task 1: `pkg/provider` registry foundation

**Files:**
- Create: `pkg/provider/provider.go`
- Create: `pkg/provider/descriptor.go`
- Create: `pkg/provider/provider_test.go`
- Create: `pkg/provider/all/all.go`
- Modify: `cmd/localrpg/gui.go`, `cmd/localrpg/play.go` (blank import `pkg/provider/all`, call `provider.Validate`)

**Interfaces:**
- Produces: `provider.Family`, `provider.Feature`, `provider.Tunable`, `provider.Preset`, `provider.Descriptor`, `provider.Registration`, `provider.Register`, `provider.Lookup`, `provider.List`, `provider.IDs`, `provider.Validate`, `provider.Reset`.

- [x] **Step 1: Write the failing registry test**

Create `pkg/provider/provider_test.go`:

```go
package provider_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestRegisterLookupAndDuplicatePanics(t *testing.T) {
	provider.Reset()
	t.Cleanup(provider.Reset)

	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{ID: "probe", Family: provider.FamilyLLM, Label: "Probe"},
		Build:      func(context.Context, []byte) (interface{}, error) { return "built", nil },
	})

	reg, ok := provider.Lookup("probe")
	if !ok || reg.Descriptor.Family != provider.FamilyLLM {
		t.Fatalf("lookup failed: %+v ok=%v", reg, ok)
	}
	if got := len(provider.List()); got != 1 {
		t.Fatalf("List = %d, want 1", got)
	}
	if got := len(provider.List(provider.FamilyTTS)); got != 0 {
		t.Fatalf("family filter = %d, want 0", got)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected a duplicate registration to panic")
		}
	}()
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{ID: "probe", Family: provider.FamilyLLM},
		Build:      func(context.Context, []byte) (interface{}, error) { return nil, nil },
	})
}

func TestValidateRejectsEmptyID(t *testing.T) {
	provider.Reset()
	t.Cleanup(provider.Reset)
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{Family: provider.FamilyLLM},
		Build:      func(context.Context, []byte) (interface{}, error) { return nil, nil },
	})
	if err := provider.Validate(); err == nil {
		t.Fatal("expected Validate to reject an empty ID")
	}
}
```

- [x] **Step 2: Run it to verify it fails**

Run: `go test -run TestRegister ./pkg/provider/ -v`
Expected: FAIL — package does not exist.

- [x] **Step 3: Implement the descriptor vocabulary**

Create `pkg/provider/descriptor.go` exactly as specified in the spec §2.3
(`Family` + constants, `Feature` + constants, `Tunable`, `Preset`, `Descriptor`).
No imports beyond `context` where needed.

- [x] **Step 4: Implement the registry**

Create `pkg/provider/provider.go`:

```go
// Package provider is the leaf registry of every provider the binary ships.
// Providers register themselves on import; pkg/provider imports no other
// internal package so that no provider can create an import cycle.
package provider

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

type Registration struct {
	Descriptor Descriptor
	Build      func(ctx context.Context, raw []byte) (interface{}, error)
}

var (
	mu    sync.RWMutex
	byID  = map[string]Registration{}
)

func Register(reg Registration) {
	if reg.Descriptor.ID == "" {
		panic("provider: registration with an empty ID")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, exists := byID[reg.Descriptor.ID]; exists {
		panic(fmt.Sprintf("provider: duplicate registration %q", reg.Descriptor.ID))
	}
	byID[reg.Descriptor.ID] = reg
}

func Lookup(id string) (Registration, bool) {
	mu.RLock()
	defer mu.RUnlock()
	reg, ok := byID[id]
	return reg, ok
}

func List(families ...Family) []Descriptor {
	mu.RLock()
	defer mu.RUnlock()
	descs := make([]Descriptor, 0, len(byID))
	for _, reg := range byID {
		if len(families) > 0 && !containsFamily(families, reg.Descriptor.Family) {
			continue
		}
		descs = append(descs, reg.Descriptor)
	}
	sort.Slice(descs, func(i, j int) bool { return descs[i].ID < descs[j].ID })
	return descs
}

func IDs() []string {
	mu.RLock()
	defer mu.RUnlock()
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func Validate() error {
	mu.RLock()
	defer mu.RUnlock()
	for id, reg := range byID {
		if id == "" || reg.Descriptor.Family == "" || reg.Build == nil {
			return fmt.Errorf("provider: malformed registration %q", id)
		}
	}
	return nil
}

func Reset() {
	mu.Lock()
	defer mu.Unlock()
	byID = map[string]Registration{}
}

func containsFamily(families []Family, candidate Family) bool {
	for _, family := range families {
		if family == candidate {
			return true
		}
	}
	return false
}
```

- [x] **Step 5: Add the aggregate import package**

Create `pkg/provider/all/all.go`:

```go
// Package all blank-imports every provider package so their init functions
// register with pkg/provider. Import it from main for side effects.
package all
```

(It will gain blank imports as families migrate in Tasks 3-5.)

- [x] **Step 6: Wire startup validation**

In `cmd/localrpg/gui.go` and `cmd/localrpg/play.go`, add the blank import and a
validation call near startup:

```go
import _ "github.com/darkliquid/localrpg/pkg/provider/all"

if err := provider.Validate(); err != nil {
	return fmt.Errorf("provider registry: %w", err)
}
```

- [x] **Step 7: Run the tests**

Run: `go vet ./... && go test -count=1 ./pkg/provider/ ./cmd/...`
Expected: PASS.

- [x] **Step 8: Commit**

```bash
git add pkg/provider cmd/localrpg
git commit -m "feat(provider): add the self-registration registry and descriptor vocabulary"
```

---

### Task 2: Capability derivation and the extended-voice interface

**Files:**
- Create: `pkg/harness/capabilities.go`, `pkg/harness/capabilities_test.go`
- Create: `pkg/media/capabilities.go`, `pkg/media/capabilities_test.go`
- Modify: `pkg/media/gemini_tts.go` (implement `ExtendedVoiceSearcher`)
- Modify: `pkg/media/catalog.go` (alias `VoiceOption` to `provider.Tunable`)

**Interfaces:**
- Consumes: `provider.Feature` constants.
- Produces: `harness.Capabilities`, `harness.Describe`, `media.Capabilities`, `media.Describe`, `media.ExtendedVoiceSearcher`.

- [x] **Step 1: Write the failing derivation tests**

`pkg/harness/capabilities_test.go`:

```go
func TestDescribeReportsToolsForToolCaller(t *testing.T) {
	caps := harness.Describe(&stubToolProvider{})
	if !caps.Tools || !caps.Streaming {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
}
```

(Define `stubToolProvider` in the test: a struct implementing `ModelProvider` and
`ToolCaller` returning true.)

`pkg/media/capabilities_test.go` asserts that a `GeminiTTSClient` derives
`VoiceCatalog`, `VoiceOptions`, `SpeechCues`, `Metered`, and `ExtendedVoices`.

- [x] **Step 2: Run them to verify they fail**

Run: `go test -run TestDescribe ./pkg/harness/ ./pkg/media/ -v`
Expected: FAIL — undefined `Describe`.

- [x] **Step 3: Implement `harness.Describe`**

Create `pkg/harness/capabilities.go` per the spec §2.4. Add an optional
`SupportsThinking()` check behind an anonymous interface so no existing provider
must change.

- [x] **Step 4: Implement `media.Describe` and the extended-voice interface**

Create `pkg/media/capabilities.go` per the spec §2.4. Add:

```go
// ExtendedVoiceSearcher is implemented by TTS clients that publish a searchable
// library beyond their default catalog.
type ExtendedVoiceSearcher interface {
	ListExtendedVoices(ctx context.Context, query string) ([]ProviderVoice, error)
}
```

Implement it on `GeminiTTSClient`, delegating to the existing
`media.ListGeminiVoices` with the client's model and resolved key.

- [x] **Step 5: Alias `VoiceOption` to `provider.Tunable`**

In `pkg/media/catalog.go`:

```go
// VoiceOption is kept as an alias so existing callers and tests compile while
// the vocabulary moves to pkg/provider.
type VoiceOption = provider.Tunable
```

Run the whole suite: `go test -count=1 ./...` and fix any struct-literal fallout
(field names are identical by design).

- [x] **Step 6: Commit**

```bash
git add pkg/harness/capabilities.go pkg/harness/capabilities_test.go pkg/media/capabilities.go pkg/media/capabilities_test.go pkg/media/catalog.go pkg/media/gemini_tts.go
git commit -m "feat(provider): derive capabilities from adapter interfaces"
```

---

### Task 3: Migrate the LLM family

**Files:**
- Create: `pkg/provider/openaichat/openaichat.go`
- Create: `pkg/provider/clillm/clillm.go`
- Create: `pkg/provider/oracle/oracle.go`
- Create: `pkg/provider/geminillm/geminillm.go`
- Modify: `pkg/harness/factory.go` (facade `BuildModel`, delegate migrated cases)
- Create: `pkg/provider/all/all.go` (add blank imports)
- Create: `pkg/provider/all/all_test.go` (drift guard for LLM)

**Interfaces:**
- Consumes: `provider.Register`, `harness.NewHTTPProviderWithLogger`, `harness.NewCLIProviderWithLogger`, `harness.NewNarrativeOracleProvider`, `harness.NewGeminiProvider`.
- Produces: `harness.BuildModel(id string, cfg ProviderConfig) (ModelProvider, error)`.

- [x] **Step 1: Write a failing facade-parity test**

In `pkg/harness/factory_test.go` add:

```go
func TestBuildModelResolvesRegisteredProvider(t *testing.T) {
	_ = all.Register // import pkg/provider/all for side effects
	p, err := harness.BuildModel("narrative-oracle", harness.ProviderConfig{Type: "builtin", BuiltinName: "narrative-oracle"})
	if err != nil {
		t.Fatalf("BuildModel: %v", err)
	}
	if p.ID() == "" {
		t.Fatal("expected a provider ID")
	}
}
```

Run and watch it fail.

- [x] **Step 2: Implement the LLM provider packages**

Each package embeds its adapter and registers. Example
`pkg/provider/geminillm/geminillm.go`:

```go
package geminillm

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "gemini",
			Family:      provider.FamilyLLM,
			Label:       "Google Gemini",
			Description: "Cloud model with shared-key support.",
			Source:      "gemini",
			Features:    []provider.Feature{provider.FeatureStreaming, provider.FeatureTools, provider.FeatureThinking, provider.FeatureKeyRequired, provider.FeatureModelCatalogue},
		},
		Build: func(ctx context.Context, raw []byte) (interface{}, error) {
			var cfg harness.ProviderConfig
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return nil, err
			}
			return harness.NewLegacyGeminiProvider("gemini", cfg)
		},
	})
}
```

`NewLegacyGeminiProvider` is the existing construction body extracted from
`NewModelProvider`'s `case "gemini"`/`case "builtin"` Gemini branch, so the body
exists once. The other three packages do the same for their branches.

- [x] **Step 3: Add the `BuildModel` facade and delegate**

In `pkg/harness/factory.go`:

```go
func BuildModel(id string, cfg ProviderConfig) (ModelProvider, error) {
	reg, ok := provider.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("harness: no provider registered for %q", id)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("harness: encode %s config: %w", id, err)
	}
	built, err := reg.Build(context.Background(), raw)
	if err != nil {
		return nil, err
	}
	model, ok := built.(ModelProvider)
	if !ok {
		return nil, fmt.Errorf("harness: provider %q is not a model provider", id)
	}
	return model, nil
}
```

Change `NewModelProvider` so the migrated cases return
`BuildModel("gemini", cfg)` etc.; leave the `default` fallback. This introduces a
`harness → provider` import, which is allowed (provider is a leaf).

- [x] **Step 4: Register in `all` and add the drift guard**

`pkg/provider/all/all.go`:

```go
import (
	_ "github.com/darkliquid/localrpg/pkg/provider/clillm"
	_ "github.com/darkliquid/localrpg/pkg/provider/geminillm"
	_ "github.com/darkliquid/localrpg/pkg/provider/openaichat"
	_ "github.com/darkliquid/localrpg/pkg/provider/oracle"
)
```

`pkg/provider/all/all_test.go` builds each LLM descriptor with a minimal valid
config JSON and asserts the result implements `harness.ModelProvider` and that
each declared feature is backed by `harness.Describe`.

- [x] **Step 5: Run and commit**

Run: `go vet ./... && go test -count=1 ./pkg/harness/ ./pkg/provider/... ./pkg/engine/`
Expected: PASS (engine tests prove the facade did not change behaviour).

```bash
git add pkg/provider pkg/harness
git commit -m "refactor(provider): move llm providers into self-registering packages"
```

---

### Task 4: Migrate the TTS family

**Files:**
- Create: `pkg/provider/nativeostts/`, `pkg/provider/sherpatts/`, `pkg/provider/elevenlabstts/`, `pkg/provider/geminitts/`, `pkg/provider/pipertts/`, `pkg/provider/httptts/`
- Modify: `pkg/media/providers.go` (`BuildTTS` facade; delegate migrated cases)
- Modify: `pkg/provider/all/all.go`, `pkg/provider/all/all_test.go`

**Interfaces:**
- Produces: `media.BuildTTS(id string, cfg config.TTSConfig, sharedKey string) (TTSClient, error)`.
- Consumes: existing `media.New*TTSClient` constructors.

- [x] **Step 1: Write a failing `BuildTTS` test**

```go
func TestBuildTTSResolvesRegisteredProvider(t *testing.T) {
	_ = all.Register
	client, err := media.BuildTTS("native-os", config.TTSConfig{Type: "builtin", BuiltinName: "native-os"}, "")
	if err != nil {
		t.Fatalf("BuildTTS: %v", err)
	}
	if client == nil {
		t.Fatal("expected a client")
	}
}
```

- [x] **Step 2: Implement the TTS provider packages**

Each registers its descriptor (features derived in a test, presets carried from
`providerPresets.ts`) and its `Build` body calls the existing constructor. The
`sharedKey` is injected by the facade rather than by `init`, so the `Build`
signature takes only config; the facade sets `cfg`-adjacent shared key via the
existing `NewTTSClientWithSharedKey` calls inside each package by reading the
`shared_api_key` field the facade injects into the JSON.

- [x] **Step 3: Add the `BuildTTS` facade**

Mirror `BuildModel`: look up the ID, inject `shared_api_key` into the JSON,
call `reg.Build`, type-assert `TTSClient`. Make
`media.NewTTSClientWithSharedKey` delegate its migrated cases.

- [x] **Step 4: Extend `all` and the drift guard**

Blank-import the six TTS packages; extend `all_test.go` to build each with a
minimal config and assert `media.Describe` backs every declared feature.

- [x] **Step 5: Run and commit**

Run: `go vet ./... && go test -count=1 ./pkg/media/... ./pkg/provider/... ./pkg/gui/`
Expected: PASS.

```bash
git add pkg/provider pkg/media
git commit -m "refactor(provider): move tts providers into self-registering packages"
```

---

### Task 5: Migrate STT and image families

**Files:**
- Create: `pkg/provider/whisperstt/`, `pkg/provider/webspeechstt/`, `pkg/provider/proceduralart/`, `pkg/provider/geminiimage/`, `pkg/provider/httpimage/`
- Modify: `pkg/media/providers.go` (`BuildSTT`, `BuildImage` facades; delegate)
- Modify: `pkg/provider/all/all.go`, `pkg/provider/all/all_test.go`

**Interfaces:**
- Produces: `media.BuildSTT`, `media.BuildImage`.

- [x] **Step 1: Write failing `BuildSTT`/`BuildImage` tests** for `web-speech` and `procedural-art`.
- [x] **Step 2: Implement the five packages** with descriptors (image carries `FeatureAutoGenerate`, `FeatureOffline` for procedural-art) and `Build` bodies calling existing constructors.
- [x] **Step 3: Add the facades** and delegate the migrated cases.
- [x] **Step 4: Extend `all` and the drift guard.**
- [x] **Step 5: Run and commit**

```bash
git add pkg/provider pkg/media
git commit -m "refactor(provider): move stt and image providers into self-registering packages"
```

---

### Task 6: Remove dead factory switches and validate startup

**Files:**
- Modify: `pkg/harness/factory.go`, `pkg/media/providers.go` (delete unmigrated branches; keep the facades)
- Modify: `pkg/harness/factory_test.go`, `pkg/media/providers_test.go` (delete tests for removed branches)

- [x] **Step 1: Delete the now-dead switch cases** and any helper only they used.
- [x] **Step 2: Run `go vet ./... && go test -count=1 ./...`** and fix fallout.
- [x] **Step 3: Commit**

```bash
git add pkg/harness pkg/media
git commit -m "refactor(provider): remove dead factory switches"
```

---

### Task 7: Expose the catalogue and migrate the frontend

**Files:**
- Modify: `pkg/gui/types.go` (add `ProviderCatalogDTO`)
- Modify: `pkg/gui/service.go` (add `ListProviders`)
- Modify: `pkg/gui/server.go` (route `GET /api/providers`)
- Modify: `pkg/gui/server_test.go`
- Modify: `frontend/src/types.ts`, `frontend/src/api/client.ts`
- Create: `frontend/src/hooks/useProviderCatalog.ts`
- Modify: `frontend/src/components/SettingsStudio.tsx`
- Create: `frontend/src/lib/providerCatalogFallback.ts` (generated-equivalent fallback)
- Delete: `frontend/src/templates/providerPresets.ts`

**Interfaces:**
- Produces: `GET /api/providers` returning `{ "providers": Descriptor[] }`; `APIClient.listProviders()`; `useProviderCatalog()`.

- [x] **Step 1: Add the endpoint and a route test**

`pkg/gui/types.go`:

```go
type ProviderCatalogDTO struct {
	Providers []provider.Descriptor `json:"providers"`
}
```

`pkg/gui/service.go`:

```go
func (s *Service) ListProviders(ctx context.Context) (*ProviderCatalogDTO, error) {
	return &ProviderCatalogDTO{Providers: provider.List()}, nil
}
```

Register `GET /api/providers` in `server.go`; add a `server_test.go` assertion
that the response decodes and contains at least one descriptor per family.

- [x] **Step 2: Add frontend types and client**

In `types.ts` mirror `Descriptor`, `Feature`, `Tunable`, `Preset`, and
`ProviderCatalog`. In `client.ts`:

```ts
static async listProviders(): Promise<ProviderCatalog> {
  const res = await fetch('/api/providers');
  if (!res.ok) throw new Error(`listProviders: ${res.statusText}`);
  return res.json();
}
```

- [x] **Step 3: Add the hook**

`useProviderCatalog.ts` fetches once and falls back to
`providerCatalogFallback.ts` on error, exposing `{ providers, byFamily, presets, loading }`.

- [x] **Step 4: Migrate SettingsStudio**

Replace engine/builtin dropdown literals, per-provider fields, and preset lists
with descriptor-driven rendering:
- family dropdowns from `byFamily(family)`;
- `VoiceOptionsControl` consuming `provider.Tunable` via `Descriptor.Tunables`;
- badges from `Descriptor.Features` (`metered`, `key_required`, `tools`, ...);
- catalog/extended-search buttons from `voice_catalog`/`extended_voices`;
- quick-load presets from `Descriptor.Presets` sorted by `Order`.

- [x] **Step 5: Delete `providerPresets.ts`** and remove its imports. Presets now
arrive from the endpoint.

- [x] **Step 6: Verify**

Run: `go test -count=1 ./pkg/gui/ && cd frontend && npx tsc --noEmit`
Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat(provider): expose the capability catalogue and render settings from it"
```

---

## File Map

| File | Responsibility |
|------|----------------|
| `pkg/provider/descriptor.go` | `Family`, `Feature`, `Tunable`, `Preset`, `Descriptor` |
| `pkg/provider/provider.go` | Registry, `Register`/`Lookup`/`List`/`Validate`/`Reset` |
| `pkg/provider/all/all.go` | Blank imports registering every provider |
| `pkg/provider/all/all_test.go` | Drift guard: features backed, families covered |
| `pkg/provider/*/` | One self-registering package per provider |
| `pkg/harness/capabilities.go` | `Describe(ModelProvider) Capabilities` |
| `pkg/harness/factory.go` | `BuildModel` facade; delegating legacy switch |
| `pkg/media/capabilities.go` | `Describe(TTSClient) Capabilities`, `ExtendedVoiceSearcher` |
| `pkg/media/providers.go` | `BuildTTS`/`BuildSTT`/`BuildImage` facades |
| `pkg/media/catalog.go` | `VoiceOption` alias of `provider.Tunable` |
| `pkg/gui/service.go`, `server.go`, `types.go` | `GET /api/providers` |
| `frontend/src/hooks/useProviderCatalog.ts` | Catalogue fetch + fallback |
| `frontend/src/components/SettingsStudio.tsx` | Capability-driven rendering |
| `frontend/src/lib/providerCatalogFallback.ts` | Offline fallback list |

## Self-Review

- **Spec coverage:** §2.1 layout → Tasks 1, 3-5; §2.2 registry → Task 1; §2.3 vocabulary → Task 1; §2.4 derivation → Task 2; §2.5 facades → Tasks 3-5; §2.6 IDs → Task 3-5 descriptors; §3 presets → Task 7 and provider packages; §4 exposure → Task 7; §5 drift guard → Task 1/3-5; §6 migration → Tasks 3-6; §7 risks are addressed by the guard and per-family staging.
- **Placeholder scan:** no TBD/TODO; the only deferred bodies are the mechanical `Build` wrappers (each names the exact legacy constructor) and the frontend fallback list (explicitly generated-equivalent). No vague steps.
- **Type consistency:** `provider.Registration`/`Descriptor`/`Tunable` defined in Task 1 are consumed unchanged in Tasks 2-7; `BuildModel`/`BuildTTS`/`BuildSTT`/`BuildImage` signatures are fixed in their tasks before use; `media.VoiceOption` alias keeps Task 2 compiling against existing callers.
