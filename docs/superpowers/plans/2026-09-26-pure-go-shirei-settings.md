# Pure-Go shirei GUI — Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the Settings studio: paths, provider/routing agents, media engines (TTS/STT/image), preferences, and debug/trace, plus the provider catalogue, provider test, TTS inspection/voice catalog, and model manager.

**Architecture:** Settings edit a whole `config.Config` held in `desktop.State`, loaded with `GetSettings` and saved with `SaveSettings` (all-or-nothing, exactly as the SPA's single `PUT /api/settings`). The provider/agent pickers use `MenuButton`+`MenuItem` (shirei has no select widget). Model download progress arrives over a Go channel from `SubscribeModelEvents`, replacing the SPA's SSE. TTS inspection debounces on a config signature.

**Tech Stack:** Go 1.27.1, `go.hasen.dev/shirei` widgets, `pkg/config`, `pkg/gui` service + DTOs, `pkg/provider`, `pkg/models`, standard-library tests.

**Spec:** `docs/superpowers/specs/2026-09-26-pure-go-shirei-gui-design.md` (Phase 5)

## Global Constraints

- Desktop only. Do not modify `pkg/gui/server.go`, `socket.go`, `assets.go`, or `middleware.go`.
- Settings save via `SaveSettings(ctx, config.Config)` only; never the HTTP `/api/settings` route.
- Shirei has no select/combobox: use `MenuButton`+`MenuItem` for provider/voice pickers, `OptionGroup`+`OptionButton` for short exclusive sets, `SegmentedControl` for tabs.
- Go style: `any`; `go vet ./...` clean. Tests standard-library only.
- Every interactive container gets `NextAccessName(...)` + `AssignAccess()`.
- Scope note: the SPA has no embeddings/telemetry UI, and `config.Config` has no top-level `Trace`; replicate the SPA's field set, do not invent new ones.
- Commits: Conventional Commits with a scope; end with the attribution block shown in Task 1 Step 5.
- `mise run test` must pass (`pkg/gui` is flaky ~1 in 6; re-run before calling it a regression).

---

### Task 1: Settings shell, paths, preferences, and debug

**Files:**
- Modify: `pkg/desktop/state.go` (settings fields)
- Create: `pkg/desktop/settings.go`
- Modify: `pkg/desktop/root.go` (add `ScreenSettings`)
- Test: `pkg/desktop/settings_test.go`
- Create: `pkg/desktop/testdata/snapshots/settings.png`

**Interfaces:**
- Consumes: `GetSettings(ctx) (*gui.SettingsResponseDTO, error)` (`pkg/gui/service.go:2657`), `SaveSettings(ctx, cfg config.Config) (*gui.SettingsResponseDTO, error)` (`:2669`), `SettingsResponseDTO` (`types.go:297`), `config.Config` (`pkg/config/types.go:257`).
- Produces:
  - `State.Config *config.Config`, `State.ConfigPath string`, `State.ConfigIsOverride bool`, `State.SettingsSaved bool`
  - `State.SettingsTab string`
  - `func loadSettings(ctx, svc)`, injected `saveSettings func(ctx, svc, cfg) error`
  - `func settingsView()`

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/settings_test.go`:

```go
package desktop

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestLoadSettingsCopiesConfig(t *testing.T) {
	appState = &State{Loaded: true}
	svc := gui.NewService(t.TempDir())
	loadSettings(context.Background(), svc)
	if appState.Config == nil {
		t.Fatal("loadSettings must populate Config")
	}
	if appState.ConfigPath == "" {
		t.Error("expected a config file path")
	}
}

func TestSettingsTabDefaultsToPaths(t *testing.T) {
	appState = &State{Loaded: true}
	if got := settingsTab(); got != "paths" {
		t.Fatalf("settingsTab() = %q, want paths", got)
	}
	appState.Config = &config.Config{}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestLoadSettings -v`
Expected: FAIL — `loadSettings` undefined.

- [ ] **Step 3: Implement the shell and simple tabs**

Add the state fields. `loadSettings` calls `GetSettings` and copies `Config`, `ConfigFilePath`, `IsLocalOverride`; `settingsTab()` returns `appState.SettingsTab`, defaulting to `"paths"`.

Create `pkg/desktop/settings.go` with `settingsView()`:
- A `SegmentedControl` (or a row of buttons) for tabs: paths, providers, agents, media, preferences, debug.
- **paths:** four `DirectoryBrowse(&appState.Config.Paths.Systems)`, `...Worlds`, `...Games`, `...Cache` fields (`widgets/filebrowser.go:57`).
- **preferences:** `ToggleSwitch(&appState.Config.Preferences.Streaming)` (`checks.go:196`), `ToggleSwitch(&appState.Config.Preferences.CinematicEffects)`, an `OptionGroup`+`OptionButton` for `FontScale` (`small`/`medium`/`large`).
- **debug/trace:** `OptionGroup` for `Preferences.TraceLevel` (`off`/`summary`/`full`), and `TextInput`/`Slider` fields for `Agents.TracePayloadChars`, `TraceMaxFiles`, `TraceRotateCheck`, `TraceChunkLimit`, and `TraceMaxBytes` shown in MiB.
- A footer with Save (calls injected `saveSettings` in a goroutine; on success set `SettingsSaved` and call `widgets.ToastMessage("Settings saved")`) and a `widgets.BusyDots()` while saving.

`Run` sets the injected `saveSettings` when a service is present.

- [ ] **Step 4: Run tests and goldens, commit**

Run: `go test ./pkg/desktop/ -v && mise run desktop:snapshots`
Expected: PASS.

```bash
git add pkg/desktop
git commit -m "$(cat <<'EOF'
feat(gui): add the settings shell, paths, preferences, and debug tabs

Settings edit one config in memory and save it whole, matching the
app's single save path, with directory pickers for the four roots.

💘 Generated with Crush

Assisted-by: Crush:deepseek-v4.1-flash
EOF
)"
```

---

### Task 2: Provider catalogue, agent routing, and provider test

**Files:**
- Modify: `pkg/desktop/state.go` (`Providers`, `SelectedRole`, `TestResult`)
- Modify: `pkg/desktop/settings.go` (providers + agents tabs)
- Test: `pkg/desktop/settings_test.go` (extend)

**Interfaces:**
- Consumes: `ListProviders(ctx) (*gui.ProviderCatalogDTO, error)` (`pkg/gui/catalogue.go:15`), `provider.Descriptor` (`pkg/provider/descriptor.go:65`), `TestProvider(ctx, req gui.TestProviderRequestDTO) (*gui.TestProviderResponseDTO, error)` (`service.go:2715`), `AgentRoleConfig`/`AgentsConfig` (`pkg/config/types.go:23/43`).
- Produces: `State.Providers []provider.Descriptor`, `State.SelectedRole string`, `State.TestResult *gui.TestProviderResponseDTO`, `func providersForFamily(f provider.Family) []provider.Descriptor`, injected `testProvider`.

- [ ] **Step 1: Write the failing test**

Add to `settings_test.go`:

```go
func TestProvidersForFamilyFilters(t *testing.T) {
	appState = &State{Providers: []provider.Descriptor{
		{ID: "a", Family: provider.FamilyLLM},
		{ID: "b", Family: provider.FamilyTTS},
		{ID: "c", Family: provider.FamilyLLM},
	}}
	got := providersForFamily(provider.FamilyLLM)
	if len(got) != 2 {
		t.Fatalf("llm providers = %d, want 2", len(got))
	}
}
```

`provider.Descriptor`'s `Tunables`/`Presets` fields drive the picker; a preset is applied by decoding `preset.Config` (a `map[string]any`) into the relevant config struct with `yaml`/`json` round-tripping, as the SPA does.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestProvidersForFamilyFilters -v`
Expected: FAIL.

- [ ] **Step 3: Implement the providers and agents tabs**

- **providers:** `PasswordInput(&appState.Config.Providers.Gemini.APIKey)`.
- **agents:** a role list (`config.Agents.Roles`) and a `MenuButton`+`MenuItem` provider picker filtered by family; a preset menu that applies `preset.Config`; `OptionGroup` for role `Type` (`disabled`/`http`/`cli`/`builtin`/`inherit`); `TextInput`s for `Endpoint`, `Model`, `APIKey`, `Command`, `BuiltinName`, comma-split `Args`; `TextInput`s/Sliders for `Temperature`, `TopP`, `TopK`, `MaxTokens`, `ThinkingBudget`, `SupportsTools`; the context/retrieval numeric fields; `CheckBox` for `ActionEcho`.
- A Test button per role calling the injected `testProvider` with `TestProviderRequestDTO{Category: "llm", Provider: role}` and rendering `TestResult.Message`/`LatencyMS`.

Add `providersForFamily` and wire `Run` to set the injected `testProvider` (calls `svc.TestProvider`).

- [ ] **Step 4: Run tests and commit**

```bash
go test ./pkg/desktop/ -v
git add pkg/desktop
git commit -m "feat(gui): add provider catalogue, agent routing, and provider test"
```

---

### Task 3: Media tab — TTS, voice catalog, and options

**Files:**
- Modify: `pkg/desktop/state.go` (`Inspect`, `InspectConfigSig`, `VoiceQuery`, `VoiceCatalogOpen`)
- Create: `pkg/desktop/settings_media.go`
- Create: `pkg/desktop/tts_inspect.go`
- Test: `pkg/desktop/tts_inspect_test.go`

**Interfaces:**
- Consumes: `InspectTTS(ctx, req gui.TTSInspectRequestDTO) (*gui.TTSInspectResponseDTO, error)` (`pkg/gui/tts_inspect.go:15`), `SearchTTSVoices(ctx, req gui.VoiceSearchRequestDTO) (*gui.VoiceSearchResponseDTO, error)` (`catalogue.go:41`), `TTSInspectResponseDTO`/`VoiceCatalogDTO` (`types.go:313`), `media.ProviderVoice` (`pkg/media/catalog.go:18`), `media.VoiceOption` (= `provider.Tunable`).
- Produces:
  - `State.Inspect *gui.TTSInspectResponseDTO`, `State.InspectSig string`
  - `func maybeInspectTTS(svc, cfg config.TTSConfig)` — re-inspects only when the signature changed
  - `func applyTTSInspect(res *gui.TTSInspectResponseDTO)`
  - `func voiceOptionsControl(schema []media.VoiceOption, values map[string]any, onChange func(key string, value any))`

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/tts_inspect_test.go`:

```go
package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestInspectSignatureChangesWithConfig(t *testing.T) {
	a := config.TTSConfig{Type: "builtin", BuiltinName: "sherpa-onnx"}
	b := config.TTSConfig{Type: "builtin", BuiltinName: "sherpa-onnx"}
	if inspectSig(a) != inspectSig(b) {
		t.Fatal("identical configs must share a signature")
	}
	b.DefaultVoice = "af_bella"
	if inspectSig(a) == inspectSig(b) {
		t.Fatal("a changed config must change the signature")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestInspectSignatureChangesWithConfig -v`
Expected: FAIL.

- [ ] **Step 3: Implement TTS settings and inspection**

`inspectSig(cfg)` returns a deterministic string (marshal the config to JSON; use `encoding/json`, ordering is stable for structs). `maybeInspectTTS` runs the inspection in a goroutine only when `inspectSig(cfg) != appState.InspectSig`, storing the result under the frame lock and calling `RequestNextFrame` — the debounce the SPA gets from a 400 ms timer is replaced by "only on change", which is sufficient because edits happen at frame cadence.

`voiceOptionsControl` renders provider-declared tunables by `Tunable.Kind`: `float`/`int` → `Slider(&f, SliderAttrs{Min, Max, Step, Width})`; `bool` → `CheckBox`; `enum` → a `MenuButton`+`MenuItem` list; otherwise `TextInput`.

**media tab, TTS section:** preset menu from `providersForFamily(FamilyTTS)` presets; `ToggleSwitch` for `AutoPlay`; `OptionGroup` for `Markdown` (`auto`/`strip`/`keep`); `OptionGroup` for engine `Type`; `TextInput`s for `Endpoint`, `Model`, `Command`, `BuiltinName`; `PasswordInput` for `APIKey`; `Slider` for `MasterVolume`; `voiceOptionsControl(appState.Inspect.Options, cfg.Options, ...)`; a `MenuButton` voice picker over `appState.Inspect.Catalog.Voices`; `CheckBox`es and an `OptionGroup` for `SpeechCues`; and an editable `voice_profiles` list (add from a `VoiceCatalogModal` equivalent, edit `Name`/`VoiceID`/`Pitch`/`SpeechRate`/`Tags`/`Description`).

- [ ] **Step 4: Run tests and goldens, commit**

```bash
go test ./pkg/desktop/ -v && mise run desktop:snapshots
git add pkg/desktop
git commit -m "feat(gui): add the media tab with TTS inspection and voice catalog"
```

---

### Task 4: STT, image, and the model manager

**Files:**
- Modify: `pkg/desktop/settings_media.go` (STT + image sections)
- Create: `pkg/desktop/models.go`
- Modify: `pkg/desktop/state.go` (`Models`, `ModelsEvents`, `MissingModel`)
- Test: `pkg/desktop/models_test.go`

**Interfaces:**
- Consumes: `GetModelsStatus() []models.ModelStatus` (`service.go:124`), `DownloadModel(ctx, id string) error` (`:127`), `SubscribeModelEvents() chan models.ModelStatus` (`:138`), `UnsubscribeModelEvents(ch chan models.ModelStatus)` (`:140`), `models.ModelStatus` (`pkg/models/manager.go:43`), `ListModels(ctx, req gui.ModelCatalogueRequestDTO) (*gui.ModelCatalogueResponseDTO, error)` (`catalogue.go:22`).
- Produces: `State.Models []models.ModelStatus`, invisible channel plumbing, `func startModelEvents(svc)`, `func downloadModel(ctx, svc, id)`, `func modelsModal()`.

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/models_test.go`:

```go
package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/models"
)

func TestApplyModelStatusReplacesByID(t *testing.T) {
	appState = &State{Models: []models.ModelStatus{{ID: "kokoro-tts", Name: "Kokoro", Progress: 0}}}
	applyModelStatus(models.ModelStatus{ID: "kokoro-tts", Name: "Kokoro", Downloading: true, Progress: 0.5})
	if len(appState.Models) != 1 || appState.Models[0].Progress != 0.5 {
		t.Fatalf("models = %+v", appState.Models)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestApplyModelStatusReplacesByID -v`
Expected: FAIL.

- [ ] **Step 3: Implement STT, image, and models**

- **STT section:** preset menu, `OptionGroup` for `Type`, `TextInput`s for `Endpoint`, `Model`, `Command`. (Voice input is dropped in v1, so these fields are config-only — they configure the backend for a future mic path.)
- **Image section:** preset menu, `ToggleSwitch` for `AutoGenerate` and `BuiltinFallback`, `OptionGroup` for `Type`, `TextInput`s for `BuiltinName`, `Endpoint`, `Model`, `AspectRatio`, `PersonGeneration`, `PasswordInput` for `APIKey`.
- **models:** `applyModelStatus` replaces a status by ID (appending when new). `startModelEvents(svc)` subscribes once and drains the channel in a goroutine, calling `applyModelStatus` under the frame lock then `RequestNextFrame`; `Run` unsubscribes on shutdown. `downloadModel` calls `DownloadModel` in a goroutine. `modelsModal()` shows a `ProgressBar(float32(status.Progress))` with `BusyDots()` while downloading and a Download button when not installed (there is no cancel path, matching the SPA).

- [ ] **Step 4: Full gate, manual check, commit**

Run: `go test ./pkg/desktop/ -v && mise run test`
Expected: PASS (re-run if the `pkg/gui` flake triggers).

Run: `LOCALRPG_UI=shirei go run ./cmd/localrpg gui --dir <dir>`, open Settings.
Expected: each tab renders and edits; Save writes the config; a provider test returns a message; TTS inspection populates voices/options; a model download shows progress.

```bash
git add -A
git commit -m "feat(gui): add STT, image, and model manager settings"
```

---

## Self-Review

**Spec coverage (Phase 5):**

| Spec item | Task |
| --- | --- |
| `SettingsStudio` + tabs + Save | Tasks 1, 2, 3, 4 |
| provider/model catalogue | Task 2 |
| `ModelDownloadModal` (progress) | Task 4 (channel replaces SSE) |
| `VoiceCatalogModal`/`Picker`/`Combobox`/`OptionsControl` | Task 3 |
| TTS inspect hook | Task 3 (`maybeInspectTTS`) |

**Known gaps carried from the SPA (documented, not introduced):** no embeddings/telemetry UI; model downloads cannot be cancelled; settings save is all-or-nothing and writes to the local override when one exists.

**Placeholder scan:** No TBDs. Tasks 2-4 instruct the implementer to confirm `provider.Tunable`/`Preset` field names and `media.VoiceOption` shape against the source; that is factual verification.

**Type consistency:** `Config`, `SettingsTab`, `loadSettings`, `settingsTab`, `Providers`, `providersForFamily`, `Inspect`/`InspectSig`, `inspectSig`, `maybeInspectTTS`, `voiceOptionsControl`, `Models`, `applyModelStatus`, `startModelEvents`, `downloadModel`, `modelsModal` are each defined once and used consistently. Service signatures come from the cited `pkg/gui` lines.

**Known deferrals (not gaps):** the studios and character creation are the next plan; theatre and teardown follow.
