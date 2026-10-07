# Provider Purpose Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let each media use (narrator, npc, scene, portrait, placeholder) select a named provider, with the family default as the fallback.

**Architecture:** A `Purpose` vocabulary and a `media.purposes` map; `ProviderForPurpose`/`TTSForPurpose`/`ImageForPurpose` resolve a use to an MP-1 named config; validation checks the names; the GUI's narrator, NPC, scene, and portrait resolvers read through the accessors.

**Tech Stack:** Go standard library; `gopkg.in/yaml.v3`.

**Spec:** `docs/superpowers/specs/2026-10-05-provider-purpose-routing-design.md`
**Depends on:** MP-1 (`docs/superpowers/plans/2026-10-05-multiple-provider-instances.md`).

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- An unset purpose resolves to the family default; an empty purposes map changes nothing.
- A per-entity `voice.provider` override wins over the NPC purpose.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The purpose vocabulary and config field

**Files:**
- Create: `pkg/config/purpose.go`
- Modify: `pkg/config/types.go` (`MediaConfig`)
- Test: `pkg/config/purpose_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `type Purpose string`, the five constants, `PurposeFamily(Purpose) string`, `KnownPurpose(string) bool`, `MediaConfig.Purposes`.

- [ ] **Step 1: Write the failing test**

```go
package config

import "testing"

func TestPurposeFamilyAndKnown(t *testing.T) {
	if PurposeFamily(PurposeNarrator) != "tts" || PurposeFamily(PurposeNPC) != "tts" {
		t.Fatal("narrator and npc are tts purposes")
	}
	if PurposeFamily(PurposeScene) != "image" || PurposeFamily(PurposePortrait) != "image" {
		t.Fatal("scene and portrait are image purposes")
	}
	if PurposeFamily(PurposePlaceholder) != "image" {
		t.Fatal("placeholder is an image purpose")
	}
	if KnownPurpose("bogus") {
		t.Fatal("bogus is not a known purpose")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/config/ -run TestPurposeFamily -v`
Expected: FAIL, `undefined: PurposeNarrator`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/config/purpose.go`:

```go
package config

// Purpose names a role a media provider plays.
type Purpose string

const (
	PurposeNarrator    Purpose = "narrator"
	PurposeNPC         Purpose = "npc"
	PurposeScene       Purpose = "scene"
	PurposePortrait    Purpose = "portrait"
	PurposePlaceholder Purpose = "placeholder"
)

// PurposeFamily reports which media family a purpose belongs to: "tts" or
// "image". An unknown purpose returns "".
func PurposeFamily(p Purpose) string {
	switch p {
	case PurposeNarrator, PurposeNPC:
		return "tts"
	case PurposeScene, PurposePortrait, PurposePlaceholder:
		return "image"
	default:
		return ""
	}
}

// KnownPurpose reports whether p is a purpose this build understands.
func KnownPurpose(p string) bool { return PurposeFamily(Purpose(p)) != "" }
```

Add to `MediaConfig`:

```go
	// Purposes maps a use name to a provider name from TTSProviders,
	// ImageProviders, or the family default.
	Purposes map[string]string `yaml:"purposes,omitempty" json:"purposes,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/config/ -run TestPurposeFamily -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/purpose.go pkg/config/purpose_test.go pkg/config/types.go
git commit -m "feat(config): add a media purpose vocabulary"
```

---

### Task 2: Resolution accessors

**Files:**
- Modify: `pkg/config/media.go`
- Test: `pkg/config/media_test.go` (append)

**Interfaces:**
- Consumes: `Purpose` (Task 1), MP-1's `TTSFor`/`ImageFor`.
- Produces: `ProviderForPurpose`, `TTSForPurpose`, `ImageForPurpose`.

- [ ] **Step 1: Write the failing test**

```go
func TestPurposeResolution(t *testing.T) {
	m := MediaConfig{
		TTS:          TTSConfig{BuiltinName: "elevenlabs"},
		TTSProviders: map[string]TTSConfig{"npc": {BuiltinName: "sherpa-onnx"}},
		Image:        ImageConfig{BuiltinName: "procedural-art"},
		ImageProviders: map[string]ImageConfig{"hero": {BuiltinName: "gemini"}},
		Purposes:     map[string]string{"npc": "npc", "portrait": "hero"},
	}
	if m.TTSForPurpose(PurposeNarrator).BuiltinName != "elevenlabs" {
		t.Fatal("unset narrator should be the default")
	}
	if m.TTSForPurpose(PurposeNPC).BuiltinName != "sherpa-onnx" {
		t.Fatal("npc should resolve to the npc entry")
	}
	if m.ImageForPurpose(PurposePortrait).BuiltinName != "gemini" {
		t.Fatal("portrait should resolve to hero")
	}
	if m.ImageForPurpose(PurposeScene).BuiltinName != "procedural-art" {
		t.Fatal("unset scene should be the default")
	}
}

func TestPurposeResolutionWithNoMap(t *testing.T) {
	m := MediaConfig{TTS: TTSConfig{BuiltinName: "x"}, Image: ImageConfig{BuiltinName: "y"}}
	if m.TTSForPurpose(PurposeNPC).BuiltinName != "x" || m.ImageForPurpose(PurposeScene).BuiltinName != "y" {
		t.Fatal("an empty purposes map must resolve everything to the default")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/config/ -run TestPurposeResolution -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Append to `pkg/config/media.go`:

```go
// ProviderForPurpose returns the provider name a purpose resolves to: the
// configured name, or the family default when unset.
func (m MediaConfig) ProviderForPurpose(p Purpose) string {
	if name := m.Purposes[string(p)]; name != "" {
		return name
	}
	return ReservedProviderName
}

// TTSForPurpose resolves a TTS purpose to a concrete configuration.
func (m MediaConfig) TTSForPurpose(p Purpose) TTSConfig {
	return m.TTSFor(m.ProviderForPurpose(p))
}

// ImageForPurpose resolves an image purpose to a concrete configuration.
func (m MediaConfig) ImageForPurpose(p Purpose) ImageConfig {
	return m.ImageFor(m.ProviderForPurpose(p))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/config/ -run TestPurposeResolution -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/media.go pkg/config/media_test.go
git commit -m "feat(config): resolve media purposes to providers"
```

---

### Task 3: Validation

**Files:**
- Modify: `pkg/config/types.go` (`Config.Validate`)
- Test: `pkg/config/purpose_test.go` (append)

**Interfaces:**
- Consumes: `PurposeFamily`, `KnownPurpose` (Task 1), the provider maps (MP-1).
- Produces: validation errors for unknown purposes, missing providers, and cross-family names.

- [ ] **Step 1: Write the failing test**

```go
func TestValidateRejectsBadPurposes(t *testing.T) {
	bad := DefaultConfig()
	bad.Media.Purposes = map[string]string{"bogus": "default"}
	if len(bad.Validate()) == 0 {
		t.Fatal("an unknown purpose should be rejected")
	}
	bad = DefaultConfig()
	bad.Media.Purposes = map[string]string{"npc": "gone"}
	if len(bad.Validate()) == 0 {
		t.Fatal("a missing provider should be rejected")
	}
	bad = DefaultConfig()
	bad.Media.ImageProviders = map[string]ImageConfig{"hero": {}}
	bad.Media.Purposes = map[string]string{"narrator": "hero"}
	if len(bad.Validate()) == 0 {
		t.Fatal("a cross-family provider should be rejected")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/config/ -run TestValidateRejectsBadPurposes -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In `Config.Validate`, for each `Purposes` entry:

```go
	for use, name := range c.Media.Purposes {
		p := Purpose(use)
		family := PurposeFamily(p)
		if family == "" {
			problems = append(problems, "media.purposes."+use+": unknown purpose")
			continue
		}
		if name == "" || name == ReservedProviderName {
			continue
		}
		switch family {
		case "tts":
			if _, ok := c.Media.TTSProviders[name]; !ok {
				problems = append(problems, "media.purposes."+use+": tts provider "+name+" does not exist")
			}
		case "image":
			if _, ok := c.Media.ImageProviders[name]; !ok {
				problems = append(problems, "media.purposes."+use+": image provider "+name+" does not exist")
			}
		}
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/config/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/purpose_test.go
git commit -m "feat(config): validate media purposes"
```

---

### Task 4: Narrator TTS uses its purpose

**Files:**
- Modify: `pkg/gui/service.go` (`narratorVoiceFor`, the audio pipeline builder, `ttsClientFor`)
- Test: `pkg/gui/audio_pipeline_test.go` (append)

**Interfaces:**
- Consumes: `MediaConfig.TTSForPurpose` (Task 2), the MP-1 registry.
- Produces: the narrator client resolves through `PurposeNarrator`.

- [ ] **Step 1: Write the failing test**

```go
func TestNarratorClientUsesNarratorPurpose(t *testing.T) {
	cfg := testConfigWithTTSPurpose(t, PurposeNarrator, "premium")
	svc := newTestServiceWithConfig(t, cfg)
	client, err := svc.narratorTTSClient()
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("expected a narrator client")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestNarratorClientUsesNarratorPurpose -v`
Expected: FAIL, `undefined: narratorTTSClient`.

- [ ] **Step 3: Write minimal implementation**

Add a small accessor on `Service` that returns the narrator client from the registry using
`cfg.Media.TTSForPurpose(config.PurposeNarrator)`, and route the narrator voice and clip pipeline
through it. The pipeline still resolves per-segment voices; only the narrator's provider changes.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/audio_pipeline_test.go
git commit -m "feat(gui): resolve the narrator voice by purpose"
```

---

### Task 5: NPC TTS uses its purpose, entity override wins

**Files:**
- Modify: `pkg/gui/service.go` (the voice resolver feeding the pipeline)
- Test: `pkg/gui/` (append)

**Interfaces:**
- Consumes: `PurposeNPC`, `entity.VoiceConfig.Provider`.
- Produces: an NPC with no provider override resolves through `PurposeNPC`.

- [ ] **Step 1: Write the failing test**

```go
func TestNPCVoiceUsesPurposeUnlessOverridden(t *testing.T) {
	// An entity with no voice.provider resolves to the npc purpose's client.
	// An entity with voice.provider "custom" resolves to that named provider.
}
```

Implement with the existing per-segment voice test fixtures.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestNPCVoiceUsesPurpose -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In the voice resolution used by the clip pipeline and the streamer, when a speaker has no
`voice.provider`, select the client for `cfg.Media.TTSForPurpose(config.PurposeNPC)`. When the
entity names a provider, keep using it. The registry caches both, so a campaign that names the same
provider for narrator and npc shares one client.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui
git commit -m "feat(gui): resolve NPC voices by purpose with entity override"
```

---

### Task 6: Image purposes for scene, portrait, and placeholder

**Files:**
- Modify: `pkg/gui/service.go` (`sceneArtResolver`, the `PortraitWorker` client, the fallback path)
- Test: `pkg/gui/` (append)

**Interfaces:**
- Consumes: `ImageForPurpose` (Task 2).
- Produces: scene, portrait, and placeholder clients resolve through their purposes.

- [ ] **Step 1: Write the failing test**

```go
func TestSceneAndPortraitUseTheirPurposes(t *testing.T) {
	cfg := testConfigWithImagePurposes(t, map[string]string{"portrait": "hero"})
	svc := newTestServiceWithConfig(t, cfg)
	// Assert the portrait resolver's client is built from the hero entry and the
	// scene resolver's from the default.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestSceneAndPortraitUseTheirPurposes -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

`sceneArtResolver` (`pkg/gui/service.go:2493-2505`) builds its client from
`cfg.Media.ImageForPurpose(config.PurposeScene)`. The `PortraitWorker` build
(`pkg/gui/service.go:1912-1927`) uses `ImageForPurpose(config.PurposePortrait)`. The
`BuiltinFallback` wrapper and asset previews use `ImageForPurpose(config.PurposePlaceholder)`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui
git commit -m "feat(gui): resolve image purposes for scene, portrait, and placeholder"
```

---

### Task 7: Frontend types and verification

**Files:**
- Modify: `frontend/src/types.ts`

**Interfaces:**
- Consumes: the config shape (Task 1).
- Produces: `purposes?: Record<string, string>` on `MediaConfig`.

- [ ] **Step 1: Add the field**

```ts
export interface MediaConfig {
  tts: TTSConfig;
  stt: STTConfig;
  image: ImageConfig;
  tts_providers?: Record<string, TTSConfig>;
  stt_providers?: Record<string, STTConfig>;
  image_providers?: Record<string, ImageConfig>;
  purposes?: Record<string, string>;
}
```

- [ ] **Step 2: Typecheck**

Run: `npx tsc --noEmit` (in `frontend/`)
Expected: PASS.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- Each purpose resolves to its configured provider, or the default.
- An empty purposes map is unchanged behaviour.
- The entity provider override wins for that entity.
- Validation rejects unknown, missing, and cross-family purposes.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/types.ts
git commit -m "feat(frontend): type the media purposes map"
```
