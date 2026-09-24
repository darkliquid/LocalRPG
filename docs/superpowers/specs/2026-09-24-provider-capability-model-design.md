# Provider Capability Model Specification

- **Date:** 2026-09-24
- **Status:** Approved (design); spec pending review
- **Scope:** A single capability source of truth for every provider — LLM, TTS,
  STT, and image. Introduces a leaf-level `pkg/provider` registry, self-registering
  provider packages, a shared `Descriptor` vocabulary, `harness`/`media`
  facades that keep existing call sites stable, descriptor-derived presets, and a
  capability-driven settings UI.
- **Related:** `pkg/harness` (`ModelProvider`, `ToolCaller`, `factory.go`),
  `pkg/media` (`TTSClient`, `VoiceCatalog`, `VoiceOptions`, `MeteredProvider`,
  `SpeechCueAdvertiser`, `MarkdownAware`, `providers.go`), `pkg/gui`
  (`tts_inspect.go`, `service.go`, `server.go`),
  `frontend/src/templates/providerPresets.ts`,
  `frontend/src/components/SettingsStudio.tsx`.

---

## 1. Overview & Goals

Provider capabilities are currently expressed three times and drift apart:

1. **Go interfaces**, discovered by type assertion inside `pkg/gui`
   (`harness.ToolCaller`, `media.VoiceCatalog`, `media.VoiceOptions`,
   `media.MeteredProvider`, `media.SpeechCueAdvertiser`, `media.MarkdownAware`).
2. **A live inspect endpoint** (`POST /api/tts/inspect`) that re-derives some of
   the same facts at request time.
3. **The frontend**, which hardcodes engine dropdowns, per-provider fields, and a
   515-line `providerPresets.ts` list that duplicates Go configuration.

Adding a provider therefore means editing a factory switch, a preset list, and UI
conditionals. This specification makes one registry the source of truth and makes
the UI render from capabilities rather than provider names.

### 1.1 Goals

1. **Self-registering providers.** Each provider lives in its own package and
   registers itself into a global registry on import.
2. **One vocabulary.** A `Descriptor` declares family, features, tunables, and
   presets once, shared by the CLI, the API, and the UI.
3. **Stable call sites.** `harness` and `media` keep their interfaces and
   factories; the factories become facades over the registry, so the
   orchestrator and tests do not change while providers migrate.
4. **Capability-driven UI.** Engine lists, tunables, metered/key badges, voice
   catalog affordances, and cue hints render from the descriptor.
5. **Derived presets.** Quick-load presets become descriptor data, deleting the
   frontend duplicate.
6. **No silent loss.** A drift guard fails the build if a registered provider's
   declared features are not backed by its real interfaces.

### 1.2 Non-Goals

1. Runtime third-party plugins. The provider set is fixed at build time;
   self-registration is a packaging convenience, not a dynamic loader.
2. Changing turn execution. `engine.TurnOrchestrator` continues to type-assert
   `harness.ToolCaller` directly.
3. Changing the config schema (`AgentRoleConfig`, `TTSConfig`, `STTConfig`,
   `ImageConfig`) beyond adding no new required fields.
4. Replacing `POST /api/tts/inspect`; it remains the **live** probe (catalogues,
   key presence, network errors).

### 1.3 Success Criteria

- `go build ./...` with `pkg/provider/all` imported registers every shipped
  provider; the drift-guard test proves each feature is backed.
- `GET /api/providers` returns descriptors for every family and the settings UI
  renders engine choices and tunables from it with no provider-name conditionals.
- Deleting `providerPresets.ts` does not change which presets a user can load.
- `go vet ./...` and `go test ./...` remain the gate.

---

## 2. Architecture

### 2.1 Package layout

```
pkg/provider/              registry + Descriptor vocabulary + Registration (leaf)
pkg/provider/all/          blank-imports every provider package
pkg/provider/openaichat/   LLM: OpenAI-compatible HTTP
pkg/provider/clillm/       LLM: CLI command
pkg/provider/oracle/       LLM: narrative-oracle (deterministic)
pkg/provider/geminillm/    LLM: Google Gemini
pkg/provider/nativeostts/  TTS: host OS speech
pkg/provider/sherpatts/    TTS: Sherpa-ONNX Kokoro
pkg/provider/elevenlabstts/TTS: ElevenLabs
pkg/provider/geminitts/    TTS: Google Gemini
pkg/provider/httptts/      TTS: OpenAI-compatible HTTP
pkg/provider/pipertts/     TTS: Piper CLI
pkg/provider/whisperstt/   STT: Whisper (HTTP) and whisper-cli (CLI)
pkg/provider/webspeechstt/ STT: browser-native passthrough
pkg/provider/proceduralart/Image: procedural SVG
pkg/provider/geminiimage/  Image: Gemini/Imagen
pkg/provider/httpimage/    Image: A1111/ComfyUI/LocalAI HTTP and CLI
```

Dependency rules:

- `pkg/provider` imports **no** internal package. It is a leaf.
- Provider packages import `pkg/provider`, `pkg/harness`, `pkg/media`,
  `pkg/config`, and `pkg/entity` as needed, and register on `init`.
- `pkg/harness` and `pkg/media` import `pkg/provider` for their facades.
- `pkg/provider/all` blank-imports every provider package; `cmd/localrpg`
  imports it for side effects.

Because registration happens in `init`, a program that forgets the blank import
registers fewer providers. §6 makes that failure loud.

### 2.2 Registry contract (`pkg/provider`)

```go
package provider

// Family is the kind of capability a provider offers.
type Family string

const (
	FamilyLLM   Family = "llm"
	FamilyTTS   Family = "tts"
	FamilySTT   Family = "stt"
	FamilyImage Family = "image"
)

// Registration is one provider as the registry sees it. Build unmarshals the
// family's config from raw JSON, so pkg/provider stays free of config and
// harness/media types; the caller type-asserts the result to the family
// interface.
type Registration struct {
	Descriptor Descriptor
	Build      func(ctx context.Context, raw []byte) (interface{}, error)
}

// Register adds a provider. It panics on an empty or duplicate ID: both are
// build-time bugs, and a panic at init is the cheapest way to catch them.
func Register(reg Registration)

// Lookup returns a registration by descriptor ID.
func Lookup(id string) (Registration, bool)

// List returns descriptors, optionally filtered by family.
func List(families ...Family) []Descriptor

// IDs returns every registered ID, sorted. Used by startup validation.
func IDs() []string

// Validate reports registrations that are malformed (empty family, no build,
// duplicate tunable keys). It is called at startup and in tests.
func Validate() error

// Reset clears the registry. It exists for tests only.
func Reset()
```

The registry is a process-global guarded by a `sync.RWMutex`. `Register` is
called only from `init`, before any concurrent read, but the lock makes
`Validate` and test `Reset` safe.

### 2.3 Descriptor vocabulary

```go
type Feature string

const (
	FeatureStreaming        Feature = "streaming"
	FeatureTools            Feature = "tools"
	FeatureThinking         Feature = "thinking"
	FeatureVision           Feature = "vision"
	FeatureVoiceCatalog     Feature = "voice_catalog"
	FeatureVoiceOptions     Feature = "voice_options"
	FeatureSpeechCues       Feature = "speech_cues"
	FeatureMarkdownEmphasis Feature = "markdown_emphasis"
	FeatureMetered          Feature = "metered"
	FeatureKeyRequired      Feature = "key_required"
	FeatureModelCatalogue   Feature = "model_catalogue"
	FeatureExtendedVoices   Feature = "extended_voices"
	FeatureOffline          Feature = "offline"
	FeatureAutoGenerate     Feature = "auto_generate"
	FeatureSessions         Feature = "sessions"
	FeatureContextCache     Feature = "context_cache"
)

// Tunable declares one user-adjustable provider parameter. It generalises
// media.VoiceOption (which becomes a type alias) and the agent generation
// parameters, so one renderer covers every family.
type Tunable struct {
	Key     string      `json:"key"`
	Label   string      `json:"label"`
	Kind    string      `json:"kind"` // float | int | bool | string | enum
	Min     float64     `json:"min,omitempty"`
	Max     float64     `json:"max,omitempty"`
	Step    float64     `json:"step,omitempty"`
	Options []string    `json:"options,omitempty"`
	Default interface{} `json:"default,omitempty"`
	Help    string      `json:"help,omitempty"`
}

// Preset is a ready-made configuration skeleton. Config keys match the
// family's config struct so the UI can deep-merge it into the editor.
type Preset struct {
	ID          string                 `json:"id"`
	Label       string                 `json:"label"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config"`
	Order       int                    `json:"order"`
}

// Descriptor is everything a caller needs to know about a provider without
// constructing it.
type Descriptor struct {
	ID          string    `json:"id"`
	Family      Family    `json:"family"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	Source      string    `json:"source"` // builtin | cli | http | gemini
	Features    []Feature `json:"features"`
	Tunables    []Tunable `json:"tunables,omitempty"`
	Presets     []Preset  `json:"presets,omitempty"`
}
```

### 2.4 Capability derivation (`harness` / `media`)

Adapters keep their optional interfaces. Derivation maps those interfaces to
features so the descriptor cannot claim what the code cannot do.

`pkg/harness/capabilities.go`:

```go
// Capabilities is the neutral, package-local description a provider adapter
// offers. pkg/provider converts it into a Descriptor.
type Capabilities struct {
	Streaming    bool
	Tools        bool
	Thinking     bool
	Vision       bool
	Sessions     bool
	ContextCache bool
}

// Describe derives capabilities from the adapter's real surfaces.
func Describe(p ModelProvider) Capabilities {
	caps := Capabilities{Streaming: true}
	if caller, ok := p.(ToolCaller); ok {
		caps.Tools = caller.ToolCallerCapable()
	}
	if thinker, ok := p.(interface{ SupportsThinking() bool }); ok {
		caps.Thinking = thinker.SupportsThinking()
	}
	if _, ok := p.(SessionProvider); ok {
		caps.Sessions = true
	}
	if _, ok := p.(ContextCacher); ok {
		caps.ContextCache = true
	}
	return caps
}
```

`SessionProvider` and `ContextCacher` are defined in `pkg/harness/session.go`
(see the turn-context spec for their full contract and semantics):

```go
type SessionHandle struct {
	ID          string
	ThroughTurn int
}

type SessionProvider interface {
	StartSession(ctx context.Context, req GenerateRequest) (*SessionHandle, error)
	ContinueSession(ctx context.Context, session *SessionHandle, req GenerateRequest) (*GenerateResponse, error)
}

type ContextCacher interface {
	EnsureCache(ctx context.Context, prefix string, ttl time.Duration) (string, error)
	InvalidateCache(ctx context.Context, cacheID string) error
}
```

`pkg/media/capabilities.go`:

```go
type Capabilities struct {
	VoiceCatalog     bool
	VoiceOptions     bool
	SpeechCues       bool
	MarkdownEmphasis bool
	Metered          bool
	ExtendedVoices   bool
}

func Describe(client TTSClient) Capabilities {
	var caps Capabilities
	if _, ok := client.(VoiceCatalog); ok { caps.VoiceCatalog = true }
	if _, ok := client.(VoiceOptions); ok { caps.VoiceOptions = true }
	if _, ok := client.(SpeechCueAdvertiser); ok { caps.SpeechCues = true }
	if aware, ok := client.(MarkdownAware); ok { caps.MarkdownEmphasis = aware.SupportsMarkdown() }
	if metered, ok := client.(MeteredProvider); ok { caps.Metered = metered.Metered() }
	if _, ok := client.(ExtendedVoiceSearcher); ok { caps.ExtendedVoices = true }
	return caps
}
```

`ExtendedVoiceSearcher` is the optional interface already implied by
`media.ListGeminiVoices`; it is promoted from a package function to an interface
on the client so the descriptor can advertise it.

### 2.5 Facades (`harness` / `media`)

To keep call sites stable while providers migrate, the existing factories become
facades over the registry:

```go
// pkg/harness/factory.go
func BuildModel(id string, cfg ProviderConfig) (ModelProvider, error) {
	reg, ok := provider.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("harness: no provider registered for %q", id)
	}
	raw, err := json.Marshal(cfg)
	if err != nil { return nil, fmt.Errorf("harness: encode %s config: %w", id, err) }
	built, err := reg.Build(context.Background(), raw)
	if err != nil { return nil, err }
	model, ok := built.(ModelProvider)
	if !ok {
		return nil, fmt.Errorf("harness: provider %q is not a model provider", id)
	}
	return model, nil
}
```

`NewModelProvider` keeps its current switch during migration, delegating the
migrated cases to `provider.Lookup`; when every case is migrated the switch is
deleted. `media.NewTTSClientWithSharedKey` follows the same pattern.

### 2.6 Construction IDs

Registry IDs are stable and namespaced so config can address them:

| Family | ID | Source |
|--------|----|--------|
| llm | `openaichat` | http |
| llm | `cli` | cli |
| llm | `narrative-oracle` | builtin |
| llm | `gemini` | gemini |
| tts | `native-os` | builtin |
| tts | `sherpa-onnx` | builtin |
| tts | `elevenlabs` | builtin |
| tts | `gemini` | gemini |
| tts | `piper` | cli |
| tts | `openai-http` | http |
| stt | `whisper-http` | http |
| stt | `whisper-cli` | cli |
| stt | `web-speech` | builtin |
| image | `procedural-art` | builtin |
| image | `gemini` | gemini |
| image | `sd-http` | http |
| image | `sd-cli` | cli |

Config keeps addressing providers the way it does today; a small resolver maps
config `type` + `builtin_name` to a registry ID. That resolver is the only new
translation layer.

---

## 3. Capability Catalogue and Presets

The catalogue **is** the registry: `provider.List()` returns every registered
descriptor with its features, tunables, and presets. There is no separate
`Catalog(cfg)` assembler and no config import in `pkg/provider`, which keeps the
package a leaf.

Presets are declared by the provider that owns them, inside its
`Descriptor.Presets`, so they travel with the adapter and cannot drift from it.
The current `frontend/src/templates/providerPresets.ts` entries move into their
owning provider package nearly verbatim; `Preset.Order` preserves the existing
list order so the UI does not reshuffle when the source moves from TypeScript to
Go.

The one translation the frontend still needs is from a config value
(`type` + `builtin_name`) to a registry ID. That mapping lives in
`pkg/harness` and `pkg/media` (the facades in §2.5), not in `pkg/provider`.

---

## 4. Exposure

### 4.1 API

New endpoint, updated in the four places AGENTS.md requires:
`Service` (`pkg/gui/service.go`), `server.go`, `frontend/src/types.ts`,
`frontend/src/api/client.ts`.

```
GET /api/providers
200 { "providers": [ Descriptor, ... ] }
```

`Service.ListProviders(ctx) (*ProviderCatalogDTO, error)` returns
`provider.List()`. Descriptors contain no secrets and no live data.

### 4.2 TTS inspect stays live

`POST /api/tts/inspect` keeps answering live questions (fetched catalogue, key
presence, network errors). Its static fields (`options`, `speech_cues`) are
cross-checked against the descriptor in a test so the two cannot disagree.

### 4.3 Frontend

- New `useProviderCatalog()` hook fetches `/api/providers` once and caches it.
- `SettingsStudio.tsx` renders:
  - engine/builtin dropdowns from descriptors for the active family;
  - `Tunable` controls with `VoiceOptionsControl` (retargeted at `provider.Tunable`);
  - metered/key/feature badges from `Descriptor.Features`;
  - voice-catalog and extended-search affordances from features;
  - the quick-load preset list from `Descriptor.Presets`.
- Provider-name conditionals in the settings components are removed.
- A generated fallback (a small static list) keeps the pane usable if the
  endpoint fails, but it is no longer the source of truth.

---

## 5. Drift Guard and Testing

1. **Drift guard** (`pkg/provider/all/all_test.go`): for every registration,
   build a zero/default config, assert the built value implements the family
   interface, then assert each declared `Feature` is backed by the derived
   capability. A feature with no backing fails the test.
2. **Registration coverage**: assert `all` registers at least one provider per
   family and that IDs are unique and non-empty.
3. **Preset validity**: every `Preset.Config` unmarshals into the family's
   config struct without error.
4. **Facade parity**: `harness.BuildModel(id, cfg)` and the legacy
   `NewModelProvider` produce the same adapter for a migrated provider.
5. **Endpoint**: `GET /api/providers` returns the registered descriptors;
   `tsc --noEmit` passes with the new types.
6. **UI**: no settings test references a provider name; the pane renders from a
   stubbed catalogue in a component test if one exists, otherwise the TypeScript
   gate is the guard.

`provider.Validate()` runs once at startup after `all` is imported and logs the
registered IDs at debug level, so a missing import is visible in a trace.

---

## 6. Migration Strategy

Move one family at a time; the suite stays green at every step.

1. Land `pkg/provider` + `pkg/provider/all` + drift guard, with **no** providers
   moved yet (registry empty, legacy factories untouched).
2. Move the LLM providers (`openaichat`, `clillm`, `oracle`, `geminillm`);
   `harness.NewModelProvider` delegates each migrated case to the registry.
3. Move TTS providers; `media.NewTTSClientWithSharedKey` delegates.
4. Move STT and image providers.
5. Add `GET /api/providers`, relocate presets, and migrate the frontend; delete
   `providerPresets.ts`.
6. Delete the dead switches in `harness`/`media` once every case delegates.

Each step ends with `go vet ./... && go test -count=1 ./...` passing.

---

## 7. Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Silent provider loss when a blank import is missing | Drift guard asserts one provider per family; `Validate()` logs registered IDs at startup. |
| Import cycle (provider ↔ harness/media) | `pkg/provider` imports no internal package; only facades import it. |
| Descriptor claims a capability the adapter lacks | Features are derived from real interfaces by `Describe`; the drift guard re-checks after building. |
| Large mechanical move breaks call sites | Facades keep signatures stable; one family per task; suite green at each step. |
| Frontend regressions from deleting `providerPresets.ts` | Presets relocate verbatim with preserved `Order`; a fallback list guards endpoint failure. |
| Duplicate registration panic in tests | `provider.Reset()` and per-test package imports. |

---

## 8. Testing & Verification Summary

1. Registry unit tests (register, lookup, duplicate panic, list filter, validate).
2. Capability derivation tests for each family's optional interfaces.
3. Drift-guard test over `pkg/provider/all`.
4. Preset-unmarshals tests.
5. Facade-parity tests against the legacy constructors.
6. `GET /api/providers` route test.
7. Frontend `tsc --noEmit` with descriptor-driven settings.
8. `go vet ./...` and `go test -count=1 ./...` remain the gate.
