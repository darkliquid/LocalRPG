# Multiple Provider Instances Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let several named TTS/STT/image configurations coexist, with the existing singleton as the default, so nothing existing changes behaviour.

**Architecture:** `MediaConfig` gains three optional maps and `TTSFor`/`STTFor`/`ImageFor` accessors; `Config.Validate` checks the names; `pkg/media` gains per-family registries that build one cached client per name; the GUI resolves the default through the registry.

**Tech Stack:** Go standard library; `gopkg.in/yaml.v3`.

**Spec:** `docs/superpowers/specs/2026-10-05-multiple-provider-instances-design.md`
**Depends on:** MP-2 (`docs/superpowers/plans/2026-10-05-provider-instance-identity.md`).

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- The existing `media.tts`/`stt`/`image` fields remain the default entry; a config without the new
  maps must be byte-identical after a save/load.
- `default` is a reserved provider name.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The config fields and accessors

**Files:**
- Modify: `pkg/config/types.go` (`MediaConfig`)
- Create: `pkg/config/media.go`
- Test: `pkg/config/media_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `MediaConfig.TTSProviders/STTProviders/ImageProviders`, `TTSFor`, `STTFor`, `ImageFor`, `TTSNames`, `STTNames`, `ImageNames`, `const ReservedProviderName = "default"`.

- [ ] **Step 1: Write the failing test**

```go
package config

import "testing"

func TestTTSForResolution(t *testing.T) {
	m := MediaConfig{
		TTS: TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"},
		TTSProviders: map[string]TTSConfig{
			"npc": {Type: "builtin", BuiltinName: "sherpa-onnx"},
		},
	}
	if m.TTSFor("").BuiltinName != "elevenlabs" {
		t.Fatal("empty name should resolve to the default")
	}
	if m.TTSFor("default").BuiltinName != "elevenlabs" {
		t.Fatal("the reserved name should resolve to the default")
	}
	if m.TTSFor("npc").BuiltinName != "sherpa-onnx" {
		t.Fatal("a named entry should resolve to itself")
	}
	if m.TTSFor("gone").BuiltinName != "elevenlabs" {
		t.Fatal("an unknown name should fall back to the default")
	}
	names := m.TTSNames()
	if len(names) != 2 || names[0] != "default" {
		t.Fatalf("names = %v, want default first", names)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/config/ -run TestTTSForResolution -v`
Expected: FAIL, `m.TTSFor undefined`.

- [ ] **Step 3: Write minimal implementation**

In `pkg/config/types.go`, add the three maps to `MediaConfig` (see the spec §4.1). Create
`pkg/config/media.go`:

```go
package config

import "sort"

// ReservedProviderName names the default entry, which is the singleton field.
const ReservedProviderName = "default"

// TTSFor returns the named TTS configuration, or the default when name is empty,
// "default", or unknown.
func (m MediaConfig) TTSFor(name string) TTSConfig {
	if name == "" || name == ReservedProviderName {
		return m.TTS
	}
	if cfg, ok := m.TTSProviders[name]; ok {
		return cfg
	}
	return m.TTS
}

// STTFor and ImageFor mirror TTSFor.
func (m MediaConfig) STTFor(name string) STTConfig {
	if name == "" || name == ReservedProviderName {
		return m.STT
	}
	if cfg, ok := m.STTProviders[name]; ok {
		return cfg
	}
	return m.STT
}

func (m MediaConfig) ImageFor(name string) ImageConfig {
	if name == "" || name == ReservedProviderName {
		return m.Image
	}
	if cfg, ok := m.ImageProviders[name]; ok {
		return cfg
	}
	return m.Image
}

// TTSNames returns every TTS name, default first.
func (m MediaConfig) TTSNames() []string { return names(m.TTSProviders) }

// STTNames and ImageNames mirror TTSNames.
func (m MediaConfig) STTNames() []string   { return names(m.STTProviders) }
func (m MediaConfig) ImageNames() []string { return names(m.ImageProviders) }

func names[V any](providers map[string]V) []string {
	out := []string{ReservedProviderName}
	rest := make([]string, 0, len(providers))
	for name := range providers {
		rest = append(rest, name)
	}
	sort.Strings(rest)
	return append(out, rest...)
}
```

Note: the codebase avoids `any` in Go source; use `interface{}` in `names` instead:
`func names[V interface{}](providers map[string]V) []string`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/config/ -run TestTTSForResolution -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/media.go pkg/config/media_test.go
git commit -m "feat(config): allow several named media configurations"
```

---

### Task 2: Round-trip and validation

**Files:**
- Modify: `pkg/config/types.go` (`Config.Validate`)
- Test: `pkg/config/media_test.go` (append), `pkg/config/types_test.go` (append)

**Interfaces:**
- Consumes: Task 1.
- Produces: validation errors for reserved, empty, and malformed provider names.

- [ ] **Step 1: Write the failing tests**

```go
func TestMediaProvidersRoundTrip(t *testing.T) {
	in := []byte("media:\n  tts:\n    type: builtin\n  tts_providers:\n    npc:\n      type: builtin\n")
	var cfg Config
	if err := yaml.Unmarshal(in, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Media.TTSFor("npc").Type != "builtin" {
		t.Fatal("named entry did not survive the round trip")
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "tts_providers") {
		t.Fatal("tts_providers was dropped on save")
	}
}

func TestValidateRejectsBadProviderNames(t *testing.T) {
	bad := DefaultConfig()
	bad.Media.TTSProviders = map[string]TTSConfig{"default": {}}
	if len(bad.Validate()) == 0 {
		t.Fatal("the reserved name should be rejected")
	}
	bad = DefaultConfig()
	bad.Media.TTSProviders = map[string]TTSConfig{"Bad Name": {}}
	if len(bad.Validate()) == 0 {
		t.Fatal("a malformed name should be rejected")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/config/ -run 'TestMediaProvidersRoundTrip|TestValidateRejectsBadProviderNames' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In `Config.Validate`, add a helper that checks one family's map:

```go
func validateProviderNames(path string, names []string, problems *[]string) {
	for _, name := range names {
		switch {
		case name == ReservedProviderName:
			*problems = append(*problems, path+": name "+ReservedProviderName+" is reserved")
		case !providerNamePattern.MatchString(name):
			*problems = append(*problems, path+": name "+name+" must match "+providerNamePattern.String())
		}
	}
}

var providerNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
```

Call it for `media.tts_providers`, `media.stt_providers`, and `media.image_providers`, passing the
map keys. Ensure `yaml.Marshal` writes the maps (the `omitempty` tag drops an empty map, which is
the desired round-trip behaviour).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/config/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/media_test.go pkg/config/types_test.go
git commit -m "feat(config): validate media provider names"
```

---

### Task 3: The TTS registry

**Files:**
- Create: `pkg/media/registry.go`
- Test: `pkg/media/registry_test.go`

**Interfaces:**
- Consumes: `MediaConfig.TTSFor`/`TTSNames`, `NewTTSClientWithSharedKey`.
- Produces: `type TTSRegistry`, `NewTTSRegistry(cfg *config.Config, logger trace.Logger) *TTSRegistry`, `(*TTSRegistry).For(name string) (TTSClient, error)`, `(*TTSRegistry).Default() (TTSClient, error)`, `(*TTSRegistry).Names() []string`.

- [ ] **Step 1: Write the failing test**

```go
func TestTTSRegistryBuildsPerName(t *testing.T) {
	cfg := &config.Config{}
	cfg.Media.TTS = config.TTSConfig{Type: "builtin", BuiltinName: "echo"}
	cfg.Media.TTSProviders = map[string]config.TTSConfig{
		"npc": {Type: "builtin", BuiltinName: "echo"},
	}
	r := NewTTSRegistry(cfg, trace.OrNil(nil))
	if _, err := r.For(""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.For("npc"); err != nil {
		t.Fatal(err)
	}
	if len(r.Names()) != 2 {
		t.Fatalf("names = %v", r.Names())
	}
}

func TestTTSRegistryCaches(t *testing.T) {
	// Two For calls with the same name return the same client pointer.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/ -run TestTTSRegistry -v`
Expected: FAIL, `undefined: NewTTSRegistry`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/media/registry.go`:

```go
package media

import (
	"sync"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// TTSRegistry holds one TTS client per configured name, built lazily and cached
// so a named provider's model loads once.
type TTSRegistry struct {
	cfg    *config.Config
	logger trace.Logger
	mu     sync.Mutex
	byName map[string]TTSClient
}

func NewTTSRegistry(cfg *config.Config, logger trace.Logger) *TTSRegistry {
	return &TTSRegistry{cfg: cfg, logger: trace.OrNil(logger), byName: map[string]TTSClient{}}
}

// For returns the client for a name, or the default when the name is empty or
// unknown.
func (r *TTSRegistry) For(name string) (TTSClient, error) {
	key := name
	if key == "" {
		key = config.ReservedProviderName
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.byName[key]; ok {
		return c, nil
	}
	cfg := r.cfg.Media.TTSFor(name)
	client, err := NewTTSClientWithSharedKey(cfg, SharedProviderKey(cfg, TTSKeyFor(cfg)))
	if err != nil {
		return nil, err
	}
	r.byName[key] = client
	return client, nil
}

func (r *TTSRegistry) Default() (TTSClient, error) { return r.For("") }

// Names returns every configured name, default first.
func (r *TTSRegistry) Names() []string { return r.cfg.Media.TTSNames() }
```

Add `Invalidate()` to clear the cache, called when settings change. Adapt the shared-key argument
to the real `SharedProviderKey`/`TTSKeyFor` signatures (`pkg/media/exports.go:22-77`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ -run TestTTSRegistry -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/registry.go pkg/media/registry_test.go
git commit -m "feat(media): add a named TTS registry"
```

---

### Task 4: The STT and image registries

**Files:**
- Modify: `pkg/media/registry.go`
- Test: `pkg/media/registry_test.go` (append)

**Interfaces:**
- Consumes: Task 3.
- Produces: `STTRegistry`, `ImageRegistry` with the same shape.

- [ ] **Step 1: Write the failing test**

```go
func TestImageRegistryBuildsPerName(t *testing.T) {
	cfg := &config.Config{}
	cfg.Media.Image = config.ImageConfig{Type: "builtin", BuiltinName: "procedural-art"}
	r := NewImageRegistry(cfg, trace.OrNil(nil))
	if _, err := r.Default(); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/ -run TestImageRegistry -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Mirror `TTSRegistry` as `STTRegistry` (using `NewSTTClientWithSharedKey`, `STTKeyFor`) and
`ImageRegistry` (using `NewSceneImageClientWithSharedKey`, `ImageKeyFor`, and the `BuiltinFallback`
wrapper). Share the cache logic through a small generic helper or by repetition, whichever reads
better in the file.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/registry.go pkg/media/registry_test.go
git commit -m "feat(media): add named STT and image registries"
```

---

### Task 5: Resolve the default through the registry in the GUI

**Files:**
- Modify: `pkg/gui/service.go` (the audio pipeline builder, `ttsClientFor`)
- Test: `pkg/gui/audio_pipeline_test.go` (append)

**Interfaces:**
- Consumes: `media.TTSRegistry` (Task 3).
- Produces: the GUI holds one registry per service, invalidated on settings change.

- [ ] **Step 1: Write the failing test**

```go
func TestAudioPipelineUsesDefaultWhenNoName(t *testing.T) {
	svc := newTestService(t) // existing helper
	client, err := svc.ttsClient()
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("expected a default client")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestAudioPipelineUsesDefaultWhenNoName -v`
Expected: FAIL if no such accessor exists; add it as part of this task.

- [ ] **Step 3: Write minimal implementation**

Add a `ttsRegistry *media.TTSRegistry` (and STT/image) to `Service`, built lazily from the current
config and invalidated in `SaveSettings`. Replace the per-call client construction in the audio
pipeline with `s.ttsRegistry().Default()`. Behaviour is identical because `Default()` resolves the
same singleton config.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -v`
Expected: PASS, including the existing audio-pipeline tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/audio_pipeline_test.go
git commit -m "refactor(gui): resolve the default TTS client through the registry"
```

---

### Task 6: Mirror the config in the frontend types

**Files:**
- Modify: `frontend/src/types.ts`

**Interfaces:**
- Consumes: the config shape (Task 1).
- Produces: `tts_providers`, `stt_providers`, `image_providers` on `MediaConfig`.

- [ ] **Step 1: Add the fields**

```ts
export interface MediaConfig {
  tts: TTSConfig;
  stt: STTConfig;
  image: ImageConfig;
  tts_providers?: Record<string, TTSConfig>;
  stt_providers?: Record<string, STTConfig>;
  image_providers?: Record<string, ImageConfig>;
}
```

- [ ] **Step 2: Typecheck**

Run: `npx tsc --noEmit` (in `frontend/`)
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/types.ts
git commit -m "feat(frontend): type the named media providers"
```

---

### Task 7: Verification

- [ ] **Step 1: Round-trip regression**

Add a test asserting that a config with no `*_providers` maps marshals byte-identically to the
pre-change output, so an existing `config.yaml` is untouched.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Several named TTS/STT/image configs coexist.
- The singleton is the default; unset behaviour is unchanged.
- The reserved, empty, and malformed names are rejected.
- The registries build and cache one client per name.
- The GUI still resolves the default for every existing path.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the media config round trip"
```
