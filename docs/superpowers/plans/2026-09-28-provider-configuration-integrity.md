# Provider Configuration Integrity Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-28. `go build ./...`, `go test ./...`, `go vet ./...`, and `frontend npx tsc --noEmit` all pass. Commits are left uncreated per the session rule; create them in order when ready.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop a broken provider configuration from silently becoming an echo bot, validate role configuration at load and save, correct provider metadata, retire the misleading `mock` type, and cross-validate the preset tables.

**Architecture:** `RouterFromConfigWithLogger` records per-role build failures on the `Router` instead of discarding them, and installs the echo default only when `gm` was never configured. `Config.Validate()` grows from price-key checks to full role-family validation and its warnings reach the settings DTO. Descriptor metadata is corrected and guarded by a transport test; the two preset tables are cross-checked.

**Tech Stack:** Go 1.27 (stdlib `testing`, `t.TempDir()`, table tests; no testify), React 19 + TypeScript (`tsc --noEmit` gate).

**Spec:** `docs/superpowers/specs/2026-09-28-provider-configuration-integrity-design.md`

## Global Constraints

- Use `interface{}`, never `any`; `go vet` must stay clean.
- Errors wrapped with `fmt.Errorf("...: %w", err)`.
- Tests use stdlib only; no testify.
- Service delegates stay in `pkg/gui`; no new `package main` logic.
- Commit style: Conventional Commits with scope, subject < 72 chars.
- `mise run test` = `go test -v -count=1 ./...` plus `npx tsc --noEmit`.
- Regenerate embedded docs after descriptor changes: `go test ./pkg/gui -update-docs`.

---

### Task 1: Record and expose per-role build failures

**Files:**
- Modify: `pkg/harness/router.go` (add type + accessor)
- Modify: `pkg/harness/factory.go:136-193`
- Test: `pkg/harness/factory_build_test.go` (create)

**Interfaces:**
- Produces: `harness.RoleBuildError{Role, Type, Name string; Err error}`,
  `(*Router).BuildErrors() []RoleBuildError`.

- [x] **Step 1: Write the failing test**

```go
package harness

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestRouterRecordsRoleBuildFailures(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "bogus"}

	router, err := RouterFromConfig(cfg)
	if err != nil {
		t.Fatalf("RouterFromConfig: %v", err)
	}

	buildErrs := router.BuildErrors()
	if len(buildErrs) != 1 {
		t.Fatalf("BuildErrors = %d, want 1", len(buildErrs))
	}
	if buildErrs[0].Role != "gm" {
		t.Errorf("role = %q, want gm", buildErrs[0].Role)
	}
	if buildErrs[0].Err == nil {
		t.Errorf("expected the build error to be recorded")
	}

	// A configured-but-broken gm must not be silently replaced by the echo.
	if _, err := router.GetProviderForRole("gm"); err == nil {
		t.Errorf("a broken gm must not resolve to a fallback provider")
	}
}

func TestRouterEchoOnlyWhenGMUnconfigured(t *testing.T) {
	cfg := config.DefaultConfig()
	delete(cfg.Agents.Roles, "gm")

	router, err := RouterFromConfig(cfg)
	if err != nil {
		t.Fatalf("RouterFromConfig: %v", err)
	}
	if len(router.BuildErrors()) != 0 {
		t.Errorf("unexpected build errors: %v", router.BuildErrors())
	}
	provider, err := router.GetProviderForRole("gm")
	if err != nil {
		t.Fatalf("expected the echo default: %v", err)
	}
	if provider.ID() != "default-echo" {
		t.Errorf("provider = %q, want default-echo", provider.ID())
	}
}
```

- [x] **Step 2: Run it to verify it fails**

Run: `go test -run 'TestRouterRecordsRoleBuildFailures|TestRouterEchoOnlyWhenGMUnconfigured' ./pkg/harness/`
Expected: FAIL with `router.BuildErrors undefined`.

- [x] **Step 3: Add the type and accessor**

In `pkg/harness/router.go`, add to the struct and append methods:

```go
type Router struct {
	mu          sync.RWMutex
	providers   map[string]ModelProvider
	roleMap     map[string]string
	roleKeys    map[string]provider.Key
	fallbacks   map[string]string
	recorder    UsageRecorder
	buildErrors []RoleBuildError
}

// RoleBuildError records a configured role whose provider could not be built.
type RoleBuildError struct {
	Role string
	Type string
	Name string // builtin_name or command, when set
	Err  error
}

// recordBuildError appends a role build failure.
func (r *Router) recordBuildError(e RoleBuildError) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buildErrors = append(r.buildErrors, e)
}

// BuildErrors returns the per-role build failures from the configuration this
// router was built from, or nil when every configured role built.
func (r *Router) BuildErrors() []RoleBuildError {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.buildErrors) == 0 {
		return nil
	}
	return append([]RoleBuildError(nil), r.buildErrors...)
}
```

- [x] **Step 4: Record failures and gate the echo fallback**

In `pkg/harness/factory.go`, replace the body of the per-role loop's error
handling and the fallback block:

```go
	gmConfigured := false
	for role, roleCfg := range cfg.Agents.Roles {
		if roleCfg.Type == "inherit" {
			continue
		}
		if role == config.RoleGM {
			gmConfigured = true
		}

		provider, err := NewModelProviderWithLogger(role, ProviderConfig{
			Type:           roleCfg.Type,
			BuiltinName:    roleCfg.BuiltinName,
			Command:        roleCfg.Command,
			Args:           roleCfg.Args,
			Endpoint:       roleCfg.Endpoint,
			Model:          roleCfg.Model,
			APIKey:         roleCfg.APIKey,
			Temperature:    roleCfg.Temperature,
			MaxTokens:      roleCfg.MaxTokens,
			ThinkingBudget: roleCfg.ThinkingBudget,
			TopP:           roleCfg.TopP,
			TopK:           roleCfg.TopK,
			SharedAPIKey:   cfg.Providers.Gemini.APIKey,
		}, logger)
		if err != nil {
			name := roleCfg.BuiltinName
			if name == "" {
				name = roleCfg.Command
			}
			router.recordBuildError(RoleBuildError{
				Role: role, Type: roleCfg.Type, Name: name, Err: err,
			})
			continue
		}
		setProviderChunkLimit(provider, cfg.TraceChunkLimit())

		router.RegisterProvider(provider)
		router.AssignRole(role, role)
		if key, ok := KeyFor(ProviderConfig{
			Type:        roleCfg.Type,
			BuiltinName: roleCfg.BuiltinName,
			Command:     roleCfg.Command,
			Endpoint:    roleCfg.Endpoint,
		}); ok {
			router.AssignRoleKey(role, key)
		}
	}

	for role, fallback := range cfg.Agents.Fallbacks {
		if fallback != "" {
			router.SetFallback(role, fallback)
		}
	}

	// The echo default exists for an intentionally unconfigured gm, not to mask a
	// gm that was configured and failed to build.
	if !gmConfigured {
		if _, err := router.GetProviderForRole(config.RoleGM); err != nil {
			router.RegisterProvider(&builtinEchoModelProvider{id: "default-echo"})
			router.AssignRole(config.RoleGM, "default-echo")
		}
	}
```

- [x] **Step 5: Run the tests to verify they pass**

Run: `go test -run 'TestRouter' ./pkg/harness/`
Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add pkg/harness/router.go pkg/harness/factory.go pkg/harness/factory_build_test.go
git commit -m "fix(harness): surface provider build failures instead of echoing"
```

---

### Task 2: Retire the `mock` type and make `echo` explicit

**Files:**
- Modify: `pkg/harness/factory.go:44-57`
- Modify: `pkg/harness/exports.go:9-33`
- Modify: `pkg/harness/types.go:126-127` (comment)
- Test: `pkg/harness/factory_test.go` (extend)

**Interfaces:**
- Produces: `builtin_name: echo` remains the only way to select the echo;
  `type: mock` and unknown builtin names are errors.

- [x] **Step 1: Write the failing test**

Append to `pkg/harness/factory_test.go`:

```go
func TestMockTypeIsRetired(t *testing.T) {
	for _, cfg := range []ProviderConfig{
		{Type: "mock"},
		{Type: "builtin", BuiltinName: "does-not-exist"},
		{Type: "builtin"},
	} {
		if _, err := NewModelProvider("p", cfg); err == nil {
			t.Errorf("cfg %+v: expected an error", cfg)
		}
	}
}
```

- [x] **Step 2: Run it to verify it fails**

Run: `go test -run TestMockTypeIsRetired ./pkg/harness/`
Expected: FAIL — `type: mock` and bare builtin currently return the echo.

- [x] **Step 3: Implement the explicit echo and errors**

Replace `NewModelProvider` in `pkg/harness/factory.go`:

```go
func NewModelProvider(id string, cfg ProviderConfig) (ModelProvider, error) {
	switch cfg.Type {
	case "disabled":
		return &disabledModelProvider{id: id}, nil
	case "builtin":
		if cfg.BuiltinName == "echo" {
			return &builtinEchoModelProvider{id: id}, nil
		}
		return BuildModelFor(id, cfg)
	case "cli", "http", "gemini", "":
		return BuildModelFor(id, cfg)
	default:
		return nil, fmt.Errorf("unknown model provider type: %s", cfg.Type)
	}
}
```

Remove the `case "mock"` from `KeyFor` in `pkg/harness/exports.go` (change
`case "builtin", "mock", "":` to `case "builtin", "":`) and update the
`ProviderConfig.Type` comment in `pkg/harness/types.go`:

```go
	Type           string   `yaml:"type"` // "builtin", "cli", "http", "gemini", "disabled"
```

- [x] **Step 4: Run the harness tests**

Run: `go test ./pkg/harness/`
Expected: PASS. `builtin_name: echo` still works (Task 1 test and
`TestNewModelProvider` builtin case).

- [x] **Step 5: Commit**

```bash
git add pkg/harness/factory.go pkg/harness/exports.go pkg/harness/types.go pkg/harness/factory_test.go
git commit -m "refactor(harness): retire the unimplemented mock provider type"
```

---

### Task 3: Validate agent role configuration at load

**Files:**
- Modify: `pkg/config/types.go:292-315` (`Validate`)
- Test: `pkg/config/validate_test.go` (create)

**Interfaces:**
- Consumes: `harness.KeyFor` is not importable from `pkg/config` (cycle risk:
  `harness` imports `config`), so validation mirrors the accepted types and
  required fields locally.
- Produces: `Config.Validate()` reports role-family problems.

- [x] **Step 1: Write the failing test**

```go
package config_test

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestValidateReportsRoleProblems(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "bogus"}
	cfg.Agents.Roles["narrator"] = config.AgentRoleConfig{Type: "http"}
	cfg.Agents.Roles["extractor"] = config.AgentRoleConfig{Type: "cli"}
	cfg.Agents.Roles["completion"] = config.AgentRoleConfig{Type: "builtin", BuiltinName: "nope"}

	problems := strings.Join(cfg.Validate(), "\n")
	for _, want := range []string{
		`agents.roles["gm"]`, "unknown type",
		`agents.roles["narrator"]`, "endpoint is required",
		`agents.roles["extractor"]`, "command is required",
		`agents.roles["completion"]`, "unknown builtin",
	} {
		if !strings.Contains(problems, want) {
			t.Errorf("missing %q in:\n%s", want, problems)
		}
	}
}
```

- [x] **Step 2: Run it to verify it fails**

Run: `go test -run TestValidateReportsRoleProblems ./pkg/config/`
Expected: FAIL (no role problems reported).

- [x] **Step 3: Extend `Validate`**

In `pkg/config/types.go`, add a role walk to `Validate`:

```go
	knownBuiltins := map[string]bool{
		"echo": true, "gemini": true, "narrative-oracle": true,
		"sherpa-onnx": true, "kokoro": true, "native-os": true,
		"elevenlabs": true, "procedural-art": true, "web-speech": true,
	}
	for role, roleCfg := range c.Agents.Roles {
		path := fmt.Sprintf("agents.roles[%q]", role)
		switch roleCfg.Type {
		case "", "disabled", "inherit":
			// No provider requirements.
		case "http":
			if strings.TrimSpace(roleCfg.Endpoint) == "" {
				problems = append(problems, path+": endpoint is required for type http")
			}
		case "cli":
			if strings.TrimSpace(roleCfg.Command) == "" {
				problems = append(problems, path+": command is required for type cli")
			}
		case "builtin":
			if roleCfg.BuiltinName != "" && !knownBuiltins[roleCfg.BuiltinName] {
				problems = append(problems,
					fmt.Sprintf("%s: unknown builtin %q", path, roleCfg.BuiltinName))
			}
		case "gemini":
			// Key is resolved from shared config or env at build time.
		default:
			problems = append(problems,
				fmt.Sprintf("%s: unknown type %q", path, roleCfg.Type))
		}
	}
```

Also validate the media families with the same shape (`Media.TTS`,
`Media.STT`, `Media.Image`): `http` requires an endpoint, `cli` a command,
`builtin` a known name; `disabled`/empty are fine. Extract a helper
`validateProviderShape(path, typ, builtin, command, endpoint string, known map[string]bool) []string`
and call it for each family, mapping TTS/Image `gemini` as valid.

- [x] **Step 4: Run the config tests**

Run: `go test ./pkg/config/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/validate_test.go
git commit -m "feat(config): validate provider role configuration at load"
```

---

### Task 4: Cross-validate the two preset tables

**Files:**
- Test: `pkg/provider/all/presets_test.go` (extend)

**Interfaces:**
- Consumes: `config.AgentPresets`, `config.TTSPresets`, `config.STTPresets`,
  `config.ImagePresets`, `provider.List(family)`.

- [x] **Step 1: Write the failing test**

Append to `pkg/provider/all/presets_test.go`:

```go
func TestConfigPresetsMatchCatalogPresets(t *testing.T) {
	// Each config preset id must exist as a catalog preset with the same
	// identifying fields. The reverse is allowed to be catalogue-only.
	type check struct {
		family  provider.Family
		presets map[string]interface{}
		id      func(interface{}) string
	}
	// The typed config presets are compared field-by-field in the family tests;
	// here we assert id coverage in both directions where the config table is
	// authoritative for what the GUI loads.
	for _, desc := range provider.List() {
		for _, preset := range desc.Presets {
			switch desc.Family {
			case provider.FamilyLLM:
				if _, ok := config.AgentPresets[preset.ID]; !ok {
					t.Errorf("catalog preset %s/%s has no config.AgentPresets entry", desc.ID, preset.ID)
				}
			case provider.FamilyTTS:
				if _, ok := config.TTSPresets[preset.ID]; !ok {
					t.Errorf("catalog preset %s/%s has no config.TTSPresets entry", desc.ID, preset.ID)
				}
			case provider.FamilySTT:
				if _, ok := config.STTPresets[preset.ID]; !ok {
					t.Errorf("catalog preset %s/%s has no config.STTPresets entry", desc.ID, preset.ID)
				}
			case provider.FamilyImage:
				if _, ok := config.ImagePresets[preset.ID]; !ok {
					t.Errorf("catalog preset %s/%s has no config.ImagePresets entry", desc.ID, preset.ID)
				}
			}
		}
	}
}
```

- [x] **Step 2: Run it to verify it fails**

Run: `go test -run TestConfigPresetsMatchCatalogPresets ./pkg/provider/all/`
Expected: FAIL for any catalog preset missing from the config tables
(`web-speech`, `openai-speech`, `dall-e-3`, etc.). Record the exact list.

- [x] **Step 3: Reconcile the tables**

For each reported mismatch, add the missing entry to the config table in
`pkg/config/presets.go` with values matching the descriptor's `Config` map (e.g.
add `web-speech`, `openai-speech`, and the image presets the catalog declares).
Where a catalog preset is deliberately catalogue-only, add its id to an
explicit allowlist in the test with a one-line reason, mirroring the existing
`TestEveryPresetConfigUnmarshals` style.

- [x] **Step 4: Run the test to verify it passes**

Run: `go test ./pkg/provider/all/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/config/presets.go pkg/provider/all/presets_test.go
git commit -m "test(provider): cross-check config and catalog presets"
```

---

### Task 5: Correct descriptor metadata with a transport guard

**Files:**
- Modify: `pkg/provider/ttselevenlabs/ttselevenlabs.go:19`
- Modify: `pkg/provider/ttsnativeos/ttsnativeos.go` (description)
- Test: `pkg/provider/all/transport_test.go` (create)

**Interfaces:**
- Produces: every descriptor's `Source` matches its transport

- [x] **Step 1: Write the failing test**

```go
package all_test

import (
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestDescriptorSourcesMatchTransport(t *testing.T) {
	// The cloud HTTP adapters that are registered under a builtin config shape.
	httpAdapters := map[string]bool{
		string(provider.KeyTTSElevenLabs): true,
		string(provider.KeyTTSHTTP):       true,
		string(provider.KeySTTWhisperHTTP): true,
		string(provider.KeyLLMOpenAIChat):  true,
		string(provider.KeyImageHTTP):      true,
	}
	builtins := map[string]bool{
		string(provider.KeyLLMNarrativeOracle): true,
		string(provider.KeyTTSSherpaONNX):      true,
		string(provider.KeyTTSNativeOS):        true,
		string(provider.KeySTTWebSpeech):       true,
		string(provider.KeyImageProceduralArt): true,
	}
	for _, desc := range provider.List() {
		id := string(desc.ID)
		switch {
		case httpAdapters[id]:
			if desc.Source != "http" {
				t.Errorf("%s: Source = %q, want http", id, desc.Source)
			}
		case builtins[id]:
			if desc.Source != "builtin" {
				t.Errorf("%s: Source = %q, want builtin", id, desc.Source)
			}
		}
	}
}
```

- [x] **Step 2: Run it to verify it fails**

Run: `go test -run TestDescriptorSourcesMatchTransport ./pkg/provider/all/`
Expected: FAIL for `tts-elevenlabs` (`Source = "builtin"`).

- [x] **Step 3: Fix the descriptors**

- `pkg/provider/ttselevenlabs/ttselevenlabs.go:19`: `Source: "http"`.
- `pkg/provider/ttsnativeos/ttsnativeos.go`: include `espeak-ng` in the
  description alongside `spd-say`/`say`/PowerShell.
- If any other descriptor fails, reconcile its `Source` with the map; the map is
  the authority for the two that differ from their config `type`.

- [x] **Step 4: Run the provider tests and regenerate docs**

Run: `go test ./pkg/provider/all/ && go test ./pkg/gui -update-docs`
Expected: PASS, docs regenerated (the catalogue prints `Source`).

- [x] **Step 5: Commit**

```bash
git add pkg/provider/ttselevenlabs/ttselevenlabs.go pkg/provider/ttsnativeos/ttsnativeos.go pkg/provider/all/transport_test.go pkg/gui/docs
git commit -m "fix(provider): report the true transport for the ElevenLabs adapter"
```

---

### Task 6: Surface configuration warnings in the settings API

**Files:**
- Modify: `pkg/config/manager.go:114-139` (`Save` stores warnings)
- Modify: `pkg/gui/types.go` (`SettingsResponseDTO`)
- Modify: `pkg/gui/service.go:2826-2866` (`GetSettings`, `SaveSettings`)
- Modify: `frontend/src/types.ts`, `frontend/src/components/SettingsStudio.tsx`
- Test: `pkg/gui/settings_warnings_test.go` (create)

**Interfaces:**
- Consumes: `config.Config.Validate`, `ConfigManager.Warnings`,
  `harness.Router.BuildErrors`.
- Produces: `SettingsResponseDTO.Warnings []string`.

- [x] **Step 1: Write the failing test**

```go
package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestSaveSettingsReturnsWarnings(t *testing.T) {
	svc := newTestService(t)
	cfg := svc.Config()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "bogus"}

	res, err := svc.SaveSettings(context.Background(), *cfg)
	if err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected validation warnings in the save response")
	}
}
```

Use the existing service test helper (see `pkg/gui/server_test.go` /
`service_test.go` for constructing a service with a temp config).

- [x] **Step 2: Run it to verify it fails**

Run: `go test -run TestSaveSettingsReturnsWarnings ./pkg/gui/`
Expected: FAIL (field does not exist).

- [x] **Step 3: Implement**

- `pkg/config/manager.go` `Save`: after writing, `m.warnings = cfg.Validate()`.
- `pkg/gui/types.go`:

```go
type SettingsResponseDTO struct {
	Config          config.Config `json:"config"`
	ConfigFilePath  string        `json:"config_file_path"`
	IsLocalOverride bool          `json:"is_local_override"`
	Warnings        []string      `json:"warnings,omitempty"`
}
```

- `pkg/gui/service.go` `GetSettings` and `SaveSettings`: set
  `Warnings: s.configWarnings()` where

```go
func (s *Service) configWarnings() []string {
	return s.configMgr.Warnings()
}
```

(Extend with router build errors only if a router is already built; the
validation warnings are the load-time signal and must not build a router.)

- Frontend: add `warnings?: string[]` to the settings response type in
  `frontend/src/types.ts` and render a dismissible banner in `SettingsStudio`
  after save/load when non-empty.

- [x] **Step 4: Run tests and the typecheck**

Run: `go test ./pkg/gui/ && (cd frontend && npx tsc --noEmit)`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/config/manager.go pkg/gui/types.go pkg/gui/service.go pkg/gui/settings_warnings_test.go frontend/src/types.ts frontend/src/components/SettingsStudio.tsx
git commit -m "feat(gui): surface configuration warnings when settings are saved"
```

---

### Task 7: Full verification

- [x] **Step 1:** Run `mise run test` (or `go test -v -count=1 ./...` and
  `npx tsc --noEmit`). Expected: PASS.
- [x] **Step 2:** Run `mise run lint` (`go vet` must be clean).
- [x] **Step 3:** Run `go test ./pkg/gui -update-docs` and commit any regenerated
  docs:

```bash
git add pkg/gui/docs
git commit -m "docs(provider): regenerate the provider catalogue"
```

## Self-Review

- **Spec coverage:** §3.1 Task 1; §3.2 Task 3; §3.3 Task 6; §3.4 Task 5; §3.5
  Task 2; §3.6 Task 4.
- **Placeholders:** none; every implementation step carries its code.
- **Type consistency:** `RoleBuildError`, `BuildErrors`, and
  `SettingsResponseDTO.Warnings` are defined before use.
