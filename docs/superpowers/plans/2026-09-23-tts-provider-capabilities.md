# TTS Provider Capabilities (Backend and API) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a TTS provider self-describing (voice catalog and option schema), thread provider options through profiles, entities, and the audio cache, and expose it all through one inspect endpoint.

**Architecture:** Small optional interfaces in `pkg/media` mirror the existing `MarkdownAware`: a provider either implements `VoiceCatalog`/`VoiceOptions` or it does not. A new `ComputeAudioCacheKeyForVoice` is the single cache-key authority. Options are opaque maps on `config.VoiceProfile` and `entity.VoiceConfig`, validated against the provider's declarations. `POST /api/tts/inspect` returns the provider key, its options, and a cached catalog snapshot.

**Tech Stack:** Go 1.27.1 (standard library only for tests), existing `pkg/media` providers, `pkg/config`, `pkg/entity`, `pkg/gui`, `pkg/harness`.

**Spec:** `docs/superpowers/specs/2026-09-22-tts-provider-capabilities-design.md`

## Scope

This spec covers two subsystems and is split into two plans. **This plan is the backend and API half**: capability interfaces, options model, cache key, catalog cache, config/entity plumbing, and `/api/tts/inspect`.

The **frontend half** (a `useTTSInspect` hook, schema-driven Settings controls, the catalog strip, the codex catalog picker, the metered warning, and `CountUncachedBeats` with its route) is a second plan that consumes this API. Acceptance criteria 1, 2, 5 (visible labelling), and 7 (the metered warning) land there; this plan delivers the data and the matching rule they rely on.

## Discovery: the cache-key rule changes the pipeline key

Spec §3.4 says that a voice with no options delegates to `ComputeAudioCacheKeyWithRate`, "preserving warm caches". That preserves the value token used by `segmentDTOs` (which already calls `ComputeAudioCacheKeyWithRate`), but it does **not** preserve the pipeline's key: `SynthesizeUtterance` currently calls `ComputeAudioCacheKey(speakerID, "provider:voiceID:pitch:rate", text)`, a different string. No single function can reproduce both old keys.

This plan follows the spec's explicit rule (delegate to `ComputeAudioCacheKeyWithRate`). Consequence: the first playback after upgrade re-synthesises clips whose voice carries a `provider`, for any provider. Clips with no `provider` are unchanged. The spec's acceptance criterion 4 ("identical key it produced before") holds for `segmentDTOs`; for the pipeline it holds only for provider-less voices. This is flagged for the spec owner.

## Global Constraints

- Tests use only `testing` and `t.TempDir()`; no testify, no new dependencies.
- Use `interface{}`, never `any`. `go vet ./...` must stay clean.
- Never write em dashes in source code; use commas, periods, parentheses, or semicolons.
- Zero-safe config: an omitted value returns a documented default and old files re-save unchanged.
- Errors wrapped with `fmt.Errorf("...: %w", err)`.
- Conventional Commits with a scope, subject under 72 characters.
- Verification: `mise run test` (`go test -v -count=1 ./...` and `npx tsc --noEmit`) and `mise run lint` (`go vet ./...`).
- Single Go test example: `go test -run TestProviderKey ./pkg/media/`.
- `api_key` must never appear in an `/api/tts/inspect` response.

### File Map

| Action | Path | Responsibility |
| :--- | :--- | :--- |
| Create | `pkg/media/catalog.go` | `ProviderVoice`, `VoiceCatalog`, `VoiceOption`, `VoiceOptions`, `MeteredProvider`, `ProviderKey` |
| Create | `pkg/media/catalog_test.go` | `ProviderKey` table tests |
| Create | `pkg/media/tags.go` | `NormaliseVoiceTags` and the shared vocabulary |
| Create | `pkg/media/tags_test.go` | Normalisation and dedupe tests |
| Create | `pkg/media/options.go` | `ValidateVoiceOptions` and canonical coercion |
| Create | `pkg/media/options_test.go` | Clamp/type/enum/drop tests |
| Modify | `pkg/media/cache.go` | `ComputeAudioCacheKeyForVoice` |
| Modify | `pkg/media/cache_test.go` | Key determinism and legacy-delegation tests |
| Modify | `pkg/media/tts.go` | `SynthesizeUtterance` uses the new key |
| Modify | `pkg/config/types.go` | `VoiceProfile.Options`, `TTSConfig.Metered` |
| Modify | `pkg/config/types_test.go` | Options and Metered tests |
| Modify | `pkg/entity/entity.go` | `VoiceConfig.Options` |
| Modify | `pkg/entity/entity_test.go` | Frontmatter round-trip with options |
| Modify | `pkg/harness/extractor.go` | `AssignVoiceProfile` copies options |
| Modify | `pkg/harness/extractor_test.go` | Options copied onto an entity |
| Create | `pkg/media/voice_catalog.go` | `CachedVoiceCatalog`, `CatalogSnapshot`, TTL and stale-serve |
| Create | `pkg/media/voice_catalog_test.go` | Fetch, TTL, stale-on-error tests |
| Create | `pkg/media/filter.go` | `FilterVoiceProfiles` |
| Create | `pkg/media/filter_test.go` | Provider filtering tests |
| Modify | `pkg/gui/types.go` | Inspect DTOs, `TestProviderRequestDTO.VoiceID` |
| Create | `pkg/gui/tts_inspect.go` | `InspectTTS` service method |
| Modify | `pkg/gui/service.go` | Factory seam, `segmentDTOs`, `TestProvider` voice_id, filtered profiles |
| Modify | `pkg/gui/server.go` | `POST /api/tts/inspect` |
| Create | `pkg/gui/tts_inspect_test.go` | Inspect service and route tests |

---

## Task 1: Provider capability interfaces and identity

**Files:**
- Create: `pkg/media/catalog.go`
- Test: `pkg/media/catalog_test.go`

**Interfaces:**
- Consumes: `config.TTSConfig`.
- Produces: `media.ProviderVoice`, `media.VoiceCatalog`, `media.VoiceOption`, `media.VoiceOptions`, `media.MeteredProvider`, `media.ProviderKey(config.TTSConfig) string`.

- [x] **Step 1: Write the failing test**

Create `pkg/media/catalog_test.go`:

```go
package media

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestProviderKey(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.TTSConfig
		want string
	}{
		{"disabled", config.TTSConfig{Type: "disabled"}, "disabled"},
		{"empty", config.TTSConfig{}, "disabled"},
		{"builtin named", config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"}, "builtin:elevenlabs"},
		{"builtin sherpa", config.TTSConfig{Type: "builtin", BuiltinName: "sherpa-onnx"}, "builtin:sherpa-onnx"},
		{"builtin unnamed", config.TTSConfig{Type: "builtin"}, "builtin:echo"},
		{"http host and port", config.TTSConfig{Type: "http", Endpoint: "http://localhost:8880/v1/audio/speech"}, "http:localhost:8880"},
		{"cli basename", config.TTSConfig{Type: "cli", Command: "/usr/local/bin/piper"}, "cli:piper"},
		{"unknown type", config.TTSConfig{Type: "Foo Bar"}, "foo-bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProviderKey(tc.cfg); got != tc.want {
				t.Errorf("ProviderKey(%+v) = %q, want %q", tc.cfg, got, tc.want)
			}
		})
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestProviderKey ./pkg/media/ -v`
Expected: FAIL with "undefined: ProviderKey".

- [x] **Step 3: Write minimal implementation**

Create `pkg/media/catalog.go`:

```go
package media

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
)

// ProviderVoice is one voice a provider offers. It is the shared shape every
// catalog maps into, so the picker and the matcher never learn a provider's own
// vocabulary.
type ProviderVoice struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Language    string                 `json:"language,omitempty"`
	Gender      string                 `json:"gender,omitempty"`
	Accent      string                 `json:"accent,omitempty"`
	Categories  []string               `json:"categories,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Description string                 `json:"description,omitempty"`
	PreviewURL  string                 `json:"preview_url,omitempty"`
	Defaults    map[string]interface{} `json:"defaults,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// VoiceCatalog is implemented by providers that can enumerate their voices. A
// provider that cannot simply does not implement it, and the UI falls back to
// authored profiles with no error.
type VoiceCatalog interface {
	ListVoices(ctx context.Context) ([]ProviderVoice, error)
}

// VoiceOption declares one tunable a provider accepts, so the UI renders controls
// from the declaration rather than from provider-specific code.
type VoiceOption struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Kind    string   `json:"kind"` // "float" | "int" | "bool" | "string" | "enum"
	Min     float64  `json:"min,omitempty"`
	Max     float64  `json:"max,omitempty"`
	Step    float64  `json:"step,omitempty"`
	Options []string `json:"options,omitempty"`
	Default any      `json:"default,omitempty"`
	Help    string   `json:"help,omitempty"`
}

// VoiceOptions is implemented by providers that declare the tunables they accept.
// Pitch and speech rate are the portable baseline and never declared here.
type VoiceOptions interface {
	VoiceOptions() []VoiceOption
}

// MeteredProvider is implemented by providers that charge per request.
type MeteredProvider interface {
	Metered() bool
}

// ProviderKey derives a stable identifier from a TTS configuration, used for
// catalog filenames, API parameters, and diagnostics:
// "builtin:elevenlabs", "builtin:sherpa-onnx", "http:localhost:8880", "cli:piper".
func ProviderKey(cfg config.TTSConfig) string {
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "", "disabled":
		return "disabled"
	case "builtin":
		name := strings.ToLower(strings.TrimSpace(cfg.BuiltinName))
		if name == "" {
			name = "echo"
		}
		return "builtin:" + sanitiseKey(name)
	case "cli":
		command := strings.ToLower(strings.TrimSpace(cfg.Command))
		if command == "" {
			return "cli"
		}
		return "cli:" + sanitiseKey(filepath.Base(command))
	case "http":
		return "http:" + sanitiseKey(endpointHost(cfg.Endpoint))
	default:
		return sanitiseKey(strings.ToLower(strings.TrimSpace(cfg.Type)))
	}
}

// endpointHost is the host and port of an endpoint, or the raw value when it does
// not parse, so an odd URL still names a distinct provider.
func endpointHost(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return "endpoint"
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return strings.TrimPrefix(strings.TrimPrefix(trimmed, "http://"), "https://")
	}
	return parsed.Host
}

// sanitiseKey lowercases and reduces a fragment to characters that are safe in an
// identifier. The colon that separates a namespace is preserved.
func sanitiseKey(value string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-', r == ':':
			sb.WriteRune(r)
		default:
			sb.WriteRune('-')
		}
	}
	return strings.Trim(sb.String(), "-")
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestProviderKey ./pkg/media/ -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/media/catalog.go pkg/media/catalog_test.go
git commit -m "feat(media): add provider voice capabilities and stable identity"
```

---

## Task 2: Shared voice tag vocabulary

**Files:**
- Create: `pkg/media/tags.go`
- Test: `pkg/media/tags_test.go`

**Interfaces:**
- Produces: `media.NormaliseVoiceTags(raw ...string) []string`.

- [x] **Step 1: Write the failing test**

Create `pkg/media/tags_test.go`:

```go
package media

import (
	"reflect"
	"testing"
)

func TestNormaliseVoiceTags(t *testing.T) {
	cases := []struct {
		name string
		raw  []string
		want []string
	}{
		{"lowercases and trims", []string{" Elder ", "MALE"}, []string{"elder", "male"}},
		{"synonyms collapse", []string{"Masculine", "UK"}, []string{"male", "british"}},
		{"middle aged is hyphenated", []string{"middle aged"}, []string{"middle-aged"}},
		{"duplicates collapse", []string{"male", "Male", "male"}, []string{"male"}},
		{"empties drop", []string{"", "  ", "eerie"}, []string{"eerie"}},
		{"no tags", nil, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormaliseVoiceTags(tc.raw...)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("NormaliseVoiceTags(%v) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestNormaliseVoiceTags ./pkg/media/ -v`
Expected: FAIL with "undefined: NormaliseVoiceTags".

- [x] **Step 3: Write minimal implementation**

Create `pkg/media/tags.go`:

```go
package media

import "strings"

// tagSynonyms maps the ways providers spell a trait onto one vocabulary, so the
// matcher scores an authored archetype and a catalog voice identically.
var tagSynonyms = map[string]string{
	"masculine":         "male",
	"feminine":          "female",
	"nonbinary":         "androgynous",
	"non-binary":        "androgynous",
	"american english":  "american",
	"british english":   "british",
	"uk":                "british",
	"us":                "american",
	"united states":     "american",
	"united kingdom":    "british",
	"middle aged":       "middle-aged",
	"middleaged":        "middle-aged",
	"older":             "elder",
	"senior":            "elder",
	"youthful":          "young",
}

// NormaliseVoiceTags lowercases, trims, expands common synonyms, and drops
// empties, so one matcher scores authored and catalog voices without knowing
// where a profile came from.
func NormaliseVoiceTags(raw ...string) []string {
	seen := make(map[string]bool)
	tags := make([]string, 0, len(raw))
	for _, tag := range raw {
		normalised := normaliseTag(tag)
		if normalised == "" || seen[normalised] {
			continue
		}
		seen[normalised] = true
		tags = append(tags, normalised)
	}
	return tags
}

func normaliseTag(tag string) string {
	cleaned := strings.Join(strings.Fields(strings.ToLower(tag)), " ")
	if cleaned == "" {
		return ""
	}
	if synonym, ok := tagSynonyms[cleaned]; ok {
		return synonym
	}
	return strings.ReplaceAll(cleaned, " ", "-")
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestNormaliseVoiceTags ./pkg/media/ -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/media/tags.go pkg/media/tags_test.go
git commit -m "feat(media): share one voice tag vocabulary across providers"
```

---

## Task 3: Option validation and coercion

**Files:**
- Create: `pkg/media/options.go`
- Test: `pkg/media/options_test.go`

**Interfaces:**
- Consumes: `media.VoiceOption`.
- Produces: `media.ValidateVoiceOptions(schema []VoiceOption, values map[string]interface{}) (map[string]interface{}, []string)`.

- [x] **Step 1: Write the failing test**

Create `pkg/media/options_test.go`:

```go
package media

import (
	"reflect"
	"testing"
)

func optionSchema() []VoiceOption {
	return []VoiceOption{
		{Key: "stability", Label: "Stability", Kind: "float", Min: 0, Max: 1, Step: 0.05},
		{Key: "seed", Label: "Seed", Kind: "int", Min: 0, Max: 1000},
		{Key: "use_speaker_boost", Label: "Speaker boost", Kind: "bool"},
		{Key: "model", Label: "Model", Kind: "enum", Options: []string{"a", "b"}},
		{Key: "note", Label: "Note", Kind: "string"},
	}
}

func TestValidateVoiceOptions(t *testing.T) {
	t.Run("clamps a float to the declared range", func(t *testing.T) {
		got, warnings := ValidateVoiceOptions(optionSchema(), map[string]interface{}{"stability": 1.7})
		if len(warnings) != 0 {
			t.Fatalf("warnings = %v", warnings)
		}
		if got["stability"] != 1.0 {
			t.Errorf("stability = %v, want 1.0", got["stability"])
		}
	})

	t.Run("rounds an int", func(t *testing.T) {
		got, _ := ValidateVoiceOptions(optionSchema(), map[string]interface{}{"seed": 12.6})
		if got["seed"] != 13 {
			t.Errorf("seed = %v (%T), want 13", got["seed"], got["seed"])
		}
	})

	t.Run("accepts a bool and a string", func(t *testing.T) {
		got, _ := ValidateVoiceOptions(optionSchema(), map[string]interface{}{"use_speaker_boost": true, "note": "warm"})
		if got["use_speaker_boost"] != true || got["note"] != "warm" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("rejects an enum value outside the list", func(t *testing.T) {
		got, warnings := ValidateVoiceOptions(optionSchema(), map[string]interface{}{"model": "c"})
		if len(warnings) != 1 || got != nil {
			t.Errorf("got %v, warnings %v", got, warnings)
		}
	})

	t.Run("drops an unknown key with a warning", func(t *testing.T) {
		got, warnings := ValidateVoiceOptions(optionSchema(), map[string]interface{}{"banana": 1})
		if got != nil || len(warnings) != 1 {
			t.Errorf("got %v, warnings %v", got, warnings)
		}
	})

	t.Run("an empty value map is omitted", func(t *testing.T) {
		got, warnings := ValidateVoiceOptions(optionSchema(), map[string]interface{}{})
		if got != nil || len(warnings) != 0 {
			t.Errorf("got %v, warnings %v", got, warnings)
		}
	})

	t.Run("keeps one good value and warns about the bad one", func(t *testing.T) {
		got, warnings := ValidateVoiceOptions(optionSchema(), map[string]interface{}{"stability": 0.4, "seed": "abc"})
		if len(warnings) != 1 {
			t.Errorf("warnings = %v", warnings)
		}
		want := map[string]interface{}{"stability": 0.4}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestValidateVoiceOptions ./pkg/media/ -v`
Expected: FAIL with "undefined: ValidateVoiceOptions".

- [x] **Step 3: Write minimal implementation**

Create `pkg/media/options.go`:

```go
package media

import (
	"fmt"
	"math"
	"strings"
)

// ValidateVoiceOptions clamps and types-checks a value map against a provider's
// declarations. Keys absent from the schema are dropped. It returns the canonical
// map to persist, omitted when empty, and human warnings for the UI. Canonical
// values are bool, float64, int, or string only, so YAML and JSON round-trips
// stay stable.
func ValidateVoiceOptions(schema []VoiceOption, values map[string]interface{}) (map[string]interface{}, []string) {
	if len(values) == 0 {
		return nil, nil
	}

	canonical := make(map[string]interface{})
	warnings := make([]string, 0)

	for _, option := range schema {
		raw, ok := values[option.Key]
		if !ok {
			continue
		}
		value, warning := coerceVoiceOption(option, raw)
		if warning != "" {
			warnings = append(warnings, warning)
			continue
		}
		canonical[option.Key] = value
	}

	for key := range values {
		if !hasVoiceOption(schema, key) {
			warnings = append(warnings, fmt.Sprintf("dropped unknown option %q", key))
		}
	}

	if len(canonical) == 0 {
		return nil, warnings
	}
	return canonical, warnings
}

func hasVoiceOption(schema []VoiceOption, key string) bool {
	for _, option := range schema {
		if option.Key == key {
			return true
		}
	}
	return false
}

func coerceVoiceOption(option VoiceOption, raw interface{}) (interface{}, string) {
	switch option.Kind {
	case "float":
		value, ok := optionFloat(raw)
		if !ok {
			return nil, fmt.Sprintf("%s: %v is not a number", option.Key, raw)
		}
		return clampVoiceOption(value, option), ""
	case "int":
		value, ok := optionFloat(raw)
		if !ok {
			return nil, fmt.Sprintf("%s: %v is not a number", option.Key, raw)
		}
		return int(math.Round(clampVoiceOption(value, option))), ""
	case "bool":
		value, ok := optionBool(raw)
		if !ok {
			return nil, fmt.Sprintf("%s: %v is not a boolean", option.Key, raw)
		}
		return value, ""
	case "string":
		value, ok := optionString(raw)
		if !ok {
			return nil, fmt.Sprintf("%s: %v is not a string", option.Key, raw)
		}
		return value, ""
	case "enum":
		value, ok := optionString(raw)
		if !ok || !containsString(option.Options, value) {
			return nil, fmt.Sprintf("%s: %v is not one of %s", option.Key, raw, strings.Join(option.Options, ", "))
		}
		return value, ""
	default:
		return nil, fmt.Sprintf("%s: unknown option kind %q", option.Key, option.Kind)
	}
}

// clampVoiceOption bounds a number only when the declaration names a real range.
// A zero Min is indistinguishable from an omitted one, so a range is honoured
// only when Max is greater than Min.
func clampVoiceOption(value float64, option VoiceOption) float64 {
	if option.Max > option.Min {
		if value < option.Min {
			return option.Min
		}
		if value > option.Max {
			return option.Max
		}
	}
	return value
}

func optionFloat(raw interface{}) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case int32:
		return float64(value), true
	default:
		return 0, false
	}
}

func optionBool(raw interface{}) (bool, bool) {
	value, ok := raw.(bool)
	return value, ok
}

func optionString(raw interface{}) (string, bool) {
	value, ok := raw.(string)
	return value, ok
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestValidateVoiceOptions ./pkg/media/ -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/media/options.go pkg/media/options_test.go
git commit -m "feat(media): validate provider options against their schema"
```

---

## Task 4: One option-aware cache key

**Files:**
- Modify: `pkg/media/cache.go`
- Test: `pkg/media/cache_test.go` (create if absent)

**Interfaces:**
- Consumes: `entity.VoiceConfig`.
- Produces: `media.ComputeAudioCacheKeyForVoice(speakerID string, voice *entity.VoiceConfig, text string) string`.

- [x] **Step 1: Write the failing test**

If `pkg/media/cache_test.go` does not exist, create it; otherwise append:

```go
package media

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestComputeAudioCacheKeyForVoice(t *testing.T) {
	base := &entity.VoiceConfig{VoiceID: "af_bella", Pitch: 1, SpeechRate: 1}

	t.Run("no options reproduces the legacy key", func(t *testing.T) {
		got := ComputeAudioCacheKeyForVoice("speaker", base, "hello")
		want := ComputeAudioCacheKeyWithRate("speaker", "af_bella", 1, 1, "hello")
		if got != want {
			t.Errorf("key = %q, want the legacy key %q", got, want)
		}
	})

	t.Run("a nil voice reproduces the legacy key", func(t *testing.T) {
		got := ComputeAudioCacheKeyForVoice("speaker", nil, "hello")
		want := ComputeAudioCacheKeyWithRate("speaker", "", 0, 0, "hello")
		if got != want {
			t.Errorf("key = %q, want the legacy key %q", got, want)
		}
	})

	t.Run("changing one option changes the key", func(t *testing.T) {
		low := &entity.VoiceConfig{VoiceID: "af_bella", Pitch: 1, SpeechRate: 1, Options: map[string]interface{}{"stability": 0.35}}
		high := &entity.VoiceConfig{VoiceID: "af_bella", Pitch: 1, SpeechRate: 1, Options: map[string]interface{}{"stability": 0.8}}
		if ComputeAudioCacheKeyForVoice("speaker", low, "hello") == ComputeAudioCacheKeyForVoice("speaker", high, "hello") {
			t.Errorf("expected different keys for different options")
		}
	})

	t.Run("insertion order does not change the key", func(t *testing.T) {
		first := &entity.VoiceConfig{VoiceID: "af_bella", Options: map[string]interface{}{"stability": 0.35, "style": 0.2}}
		second := &entity.VoiceConfig{VoiceID: "af_bella", Options: map[string]interface{}{"style": 0.2, "stability": 0.35}}
		if ComputeAudioCacheKeyForVoice("speaker", first, "hello") != ComputeAudioCacheKeyForVoice("speaker", second, "hello") {
			t.Errorf("expected map order not to change the key")
		}
	})

	t.Run("provider distinguishes two voices with the same id", func(t *testing.T) {
		one := &entity.VoiceConfig{Provider: "builtin:kokoro", VoiceID: "af_bella", Options: map[string]interface{}{"style": 0.2}}
		two := &entity.VoiceConfig{Provider: "builtin:elevenlabs", VoiceID: "af_bella", Options: map[string]interface{}{"style": 0.2}}
		if ComputeAudioCacheKeyForVoice("speaker", one, "hello") == ComputeAudioCacheKeyForVoice("speaker", two, "hello") {
			t.Errorf("expected the provider to separate the keys")
		}
	})
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestComputeAudioCacheKeyForVoice ./pkg/media/ -v`
Expected: FAIL with "undefined: ComputeAudioCacheKeyForVoice".

- [x] **Step 3: Write minimal implementation**

In `pkg/media/cache.go`, add the import `"encoding/json"` and `"github.com/darkliquid/localrpg/pkg/entity"`, then add:

```go
// ComputeAudioCacheKeyForVoice hashes everything that changes a clip: provider,
// voice, prosody, provider options, and text. A voice with no options falls back
// to the legacy key, so the value token a client already holds stays valid.
func ComputeAudioCacheKeyForVoice(speakerID string, voice *entity.VoiceConfig, text string) string {
	if voice == nil || len(voice.Options) == 0 {
		voiceID, pitch, rate := "", 0.0, 0.0
		if voice != nil {
			voiceID, pitch, rate = voice.VoiceID, voice.Pitch, voice.SpeechRate
		}
		return ComputeAudioCacheKeyWithRate(speakerID, voiceID, pitch, rate, text)
	}

	// encoding/json sorts map keys, so the options hash is deterministic
	// regardless of insertion order.
	payload := struct {
		Provider   string                 `json:"provider"`
		VoiceID    string                 `json:"voice_id"`
		Pitch      float64                `json:"pitch"`
		SpeechRate float64                `json:"speech_rate"`
		Options    map[string]interface{} `json:"options"`
	}{
		Provider:   voice.Provider,
		VoiceID:    voice.VoiceID,
		Pitch:      voice.Pitch,
		SpeechRate: voice.SpeechRate,
		Options:    voice.Options,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		// A map of canonical scalars cannot fail to marshal; degrade rather than
		// panic so a hand-edited note never loses a turn.
		encoded = []byte(voice.Provider + "|" + voice.VoiceID)
	}

	hash := sha256.Sum256([]byte("v2:" + speakerID + ":" + string(encoded)))
	return hex.EncodeToString(hash[:])
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestComputeAudioCacheKeyForVoice ./pkg/media/ -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/media/cache.go pkg/media/cache_test.go
git commit -m "feat(media): hash provider options into the audio cache key"
```

---

## Task 5: Switch both cache-key callers

**Files:**
- Modify: `pkg/media/tts.go:172-179`
- Modify: `pkg/gui/service.go:250-268`
- Test: `pkg/media/cache_test.go`, `pkg/gui/service_test.go`

**Interfaces:**
- Consumes: `media.ComputeAudioCacheKeyForVoice` (Task 4).
- Produces: no new exports; clips and value tokens become option-sensitive.

- [x] **Step 1: Write the failing test**

Append to `pkg/media/cache_test.go`:

```go
func TestPipelineKeyFollowsVoiceOptions(t *testing.T) {
	cache := NewContentCache(t.TempDir())
	client := &echoTTSClient{}
	pipeline := NewTTSPipeline(client, cache)

	low := &entity.VoiceConfig{VoiceID: "af_bella", Options: map[string]interface{}{"stability": 0.35}}
	high := &entity.VoiceConfig{VoiceID: "af_bella", Options: map[string]interface{}{"stability": 0.8}}

	lowKey := ComputeAudioCacheKeyForVoice("speaker", low, "hello")
	highKey := ComputeAudioCacheKeyForVoice("speaker", high, "hello")

	if _, err := pipeline.SynthesizeUtterance(context.Background(), "speaker", low, "hello"); err != nil {
		t.Fatalf("SynthesizeUtterance: %v", err)
	}
	if !cache.Exists("audio", lowKey+".wav") && !cache.Exists("audio", lowKey+".mp3") {
		t.Errorf("expected a clip stored under the low-options key %q", lowKey)
	}
	if cache.Exists("audio", highKey+".wav") || cache.Exists("audio", highKey+".mp3") {
		t.Errorf("a second options set must not share the first clip")
	}
}
```

Add `"context"` and `"github.com/darkliquid/localrpg/pkg/entity"` to the test imports if missing.

Append to `pkg/gui/service_test.go`:

```go
func TestSegmentDTOKeysFollowVoiceOptions(t *testing.T) {
	voices := map[string]*entity.VoiceConfig{
		"aldric": {VoiceID: "af_bella", Options: map[string]interface{}{"stability": 0.35}},
	}
	voiceFor := func(ref string) *entity.VoiceConfig { return voices[ref] }

	segments := []entity.TurnSegment{{Kind: entity.SegmentSpeech, SpeakerID: "aldric", Text: "Hello there."}}
	before := segmentDTOs(segments, "campaign", 1, true, func(name string) string { return name }, voiceFor)

	voices["aldric"] = &entity.VoiceConfig{VoiceID: "af_bella", Options: map[string]interface{}{"stability": 0.8}}
	after := segmentDTOs(segments, "campaign", 1, true, func(name string) string { return name }, voiceFor)

	if before[0].AudioKey == after[0].AudioKey {
		t.Errorf("expected a changed option to change the value token")
	}
	if before[0].AudioURL == after[0].AudioURL {
		t.Errorf("expected a changed option to change the audio URL")
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestPipelineKeyFollowsVoiceOptions|TestSegmentDTOKeysFollowVoiceOptions' ./pkg/media/ ./pkg/gui/ -v`
Expected: FAIL. The pipeline test fails because the clip lands under the old key; the DTO test passes already only if `voiceFor` feeds the old function, so it should fail on the option-sensitivity assertion.

- [x] **Step 3: Write minimal implementation**

In `pkg/media/tts.go`, replace the voice-hash block in `SynthesizeUtterance`:

```go
func (p *TTSPipeline) SynthesizeUtterance(ctx context.Context, speakerID string, voice *entity.VoiceConfig, text string) (string, error) {
	base := ComputeAudioCacheKeyForVoice(speakerID, voice, text)
	start := time.Now()

	voiceID, pitch, rate := "", 0.0, 0.0
	if voice != nil {
		voiceID, pitch, rate = voice.VoiceID, voice.Pitch, voice.SpeechRate
	}
	p.logger = trace.OrNil(p.logger)
	p.logger.Event("media.tts.request", map[string]interface{}{
		"speaker":   speakerID,
		"voice_id":  voiceID,
		"pitch":     pitch,
		"rate":      rate,
		"chars":     len([]rune(text)),
		"cache_key": base,
	})
```

Remove the now-unused `fmt` usage only if nothing else in the file needs it; `fmt` is still used by error wrapping, so keep the import.

In `pkg/gui/service.go`, replace the key derivation in `segmentDTOs`:

```go
		if audioAvailable {
			// The ref is what the synthesis pipeline uses to find a voice, so the
			// same value is used here to derive a voice-sensitive version token. A
			// changed voice or provider option changes the URL, which keeps the
			// browser from serving a clip read under the previous tuning.
			ref := segment.SpeakerID
			if ref == "" {
				ref = segment.Speaker
			}
			var voice *entity.VoiceConfig
			if voiceFor != nil {
				voice = voiceFor(ref)
			}
			key := media.ComputeAudioCacheKeyForVoice(ref, voice, segment.Text)
			dto.AudioKey = key
			dto.AudioURL = fmt.Sprintf("/api/game/%s/turn/%d/segment/%d/audio?v=%s", gameID, turnNumber, i, key[:12])
		}
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ ./pkg/gui/ -count=1`
Expected: PASS, including the pre-existing media and gui suites.

- [x] **Step 5: Commit**

```bash
git add pkg/media/tts.go pkg/gui/service.go pkg/media/cache_test.go pkg/gui/service_test.go
git commit -m "feat(media): key clips by provider options end to end"
```

---

## Task 6: Options on profiles, entities, and config

**Files:**
- Modify: `pkg/config/types.go:81-90` (`VoiceProfile`), `pkg/config/types.go:92-111` (`TTSConfig`)
- Modify: `pkg/entity/entity.go:17-22` (`VoiceConfig`)
- Modify: `pkg/harness/extractor.go:60-122` (`AssignVoiceProfile`)
- Test: `pkg/config/types_test.go`, `pkg/entity/entity_test.go`, `pkg/harness/extractor_test.go`

**Interfaces:**
- Produces: `config.VoiceProfile.Options map[string]interface{}`, `config.TTSConfig.Metered *bool`, `entity.VoiceConfig.Options map[string]interface{}`; `AssignVoiceProfile` copies a profile's options onto the entity.

- [x] **Step 1: Write the failing tests**

Append to `pkg/config/types_test.go`:

```go
func TestVoiceProfileOptionsAndMeteredRoundTrip(t *testing.T) {	profile := VoiceProfile{
		ID:      "hushed",
		VoiceID: "bf_emma",
		Options: map[string]interface{}{"stability": 0.35, "model": "eleven_multilingual_v2"},
	}
	encoded, err := yaml.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	var decoded VoiceProfile
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal profile: %v", err)
	}
	if decoded.Options["stability"] != 0.35 || decoded.Options["model"] != "eleven_multilingual_v2" {
		t.Errorf("options did not round-trip: %v", decoded.Options)
	}

	off := false
	cfg := TTSConfig{Metered: &off}
	encoded, err = yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal tts config: %v", err)
	}
	var decodedCfg TTSConfig
	if err := yaml.Unmarshal(encoded, &decodedCfg); err != nil {
		t.Fatalf("unmarshal tts config: %v", err)
	}
	if decodedCfg.Metered == nil || *decodedCfg.Metered {
		t.Errorf("metered did not round-trip: %v", decodedCfg.Metered)
	}

	// A config that never set the pointer omits it entirely.
	plain, err := yaml.Marshal(TTSConfig{})
	if err != nil {
		t.Fatalf("marshal plain config: %v", err)
	}
	if strings.Contains(string(plain), "metered") {
		t.Errorf("an unset metered must be omitted, got %s", plain)
	}
}
```

Update `pkg/config/types_test.go`'s import block to import `strings` and `gopkg.in/yaml.v3` (unaliased, matching `pkg/config/manager.go`).

Append to `pkg/entity/entity_test.go`:

```go
func TestVoiceOptionsRoundTripThroughFrontmatter(t *testing.T) {
	original := &Entity{
		ID:   "aldric",
		Name: "Aldric",
		Type: "character",
		Body: "A guarded mercenary.",
		Voice: &VoiceConfig{
			Provider:   "builtin:elevenlabs",
			VoiceID:    "EXAVITQu4vr4xnSDxMaL",
			SpeechRate: 1,
			Options:    map[string]interface{}{"stability": 0.35, "similarity_boost": 0.8},
		},
	}

	data, err := original.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown: %v", err)
	}
	parsed, err := ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("ParseMarkdownEntity: %v", err)
	}
	if parsed.Voice == nil {
		t.Fatalf("voice did not round-trip")
	}
	if parsed.Voice.Options["stability"] != 0.35 || parsed.Voice.Options["similarity_boost"] != 0.8 {
		t.Errorf("options = %v", parsed.Voice.Options)
	}
}
```

The entry points are `(*Entity).SerializeMarkdown` and `entity.ParseMarkdownEntity`.

Append to `pkg/harness/extractor_test.go`:

```go
func TestAssignVoiceProfileCopiesOptions(t *testing.T) {
	ent := &entity.Entity{ID: "aldric", Name: "Aldric the Gruff", Type: "character", Body: "A mercenary."}
	profiles := []config.VoiceProfile{{
		ID:      "gruff",
		VoiceID: "am_adam",
		Tags:    []string{"gruff", "mercenary"},
		Options: map[string]interface{}{"stability": 0.2},
	}}

	AssignVoiceProfile(ent, profiles)
	if ent.Voice == nil {
		t.Fatalf("expected a voice assigned")
	}
	if ent.Voice.Options["stability"] != 0.2 {
		t.Errorf("options = %v, want the profile's", ent.Voice.Options)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestVoiceProfileOptionsAndMeteredRoundTrip|TestVoiceOptionsRoundTripThroughFrontmatter|TestAssignVoiceProfileCopiesOptions' ./pkg/config/ ./pkg/entity/ ./pkg/harness/ -v`
Expected: FAIL to compile with "unknown field Options" and "unknown field Metered".

- [x] **Step 3: Write minimal implementation**

In `pkg/config/types.go`, add to `VoiceProfile` after `Description`:

```go
	// Options holds provider-declared tunables, keyed by VoiceOption.Key. It is
	// opaque to the engine the same way entity State is: only the provider
	// interprets it, and an empty map is omitted.
	Options map[string]interface{} `yaml:"options,omitempty" json:"options,omitempty"`
```

Add to `TTSConfig` after `Markdown`:

```go
	// Metered marks a provider that charges per request. It overrides the
	// provider's own declaration, so an operator can flag a proxied endpoint.
	Metered *bool `yaml:"metered,omitempty" json:"metered,omitempty"`
```

In `pkg/entity/entity.go`, add to `VoiceConfig` after `SpeechRate`:

```go
	// Options carries provider-declared tunables for this voice, keyed by the
	// provider's VoiceOption.Key. Absent means the provider's own defaults.
	Options map[string]interface{} `yaml:"options,omitempty" json:"options,omitempty"`
```

In `pkg/harness/extractor.go`, add a helper and use it at all three construction sites in `AssignVoiceProfile`:

```go
// voiceFromProfile is the one place a profile becomes a voice, so a provider
// tunable an operator authored travels with the character.
func voiceFromProfile(profile config.VoiceProfile) *entity.VoiceConfig {
	return &entity.VoiceConfig{
		Provider:   profile.Provider,
		VoiceID:    profile.VoiceID,
		Pitch:      profile.Pitch,
		SpeechRate: profile.SpeechRate,
		Options:    profile.Options,
	}
}
```

Replace the direct-match block body:

```go
			ent.Voice = voiceFromProfile(p)
			return
```

Replace the tag-score block:

```go
	if bestProfile != nil {
		ent.Voice = voiceFromProfile(*bestProfile)
		return
	}
```

Replace the hash-fallback block:

```go
	p := profiles[idx]
	ent.Voice = voiceFromProfile(p)
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/config/ ./pkg/entity/ ./pkg/harness/ -count=1`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/types_test.go pkg/entity/entity.go pkg/entity/entity_test.go pkg/harness/extractor.go pkg/harness/extractor_test.go
git commit -m "feat: carry provider voice options on profiles and entities"
```

---

## Task 7: Cached voice catalog

**Files:**
- Create: `pkg/media/voice_catalog.go`
- Test: `pkg/media/voice_catalog_test.go`

**Interfaces:**
- Consumes: `media.TTSClient`, `media.VoiceCatalog`, `media.ProviderVoice`.
- Produces: `media.CachedVoiceCatalog`, `media.CatalogSnapshot`, `media.NewCachedVoiceCatalog(cacheDir string) *CachedVoiceCatalog`, `(*CachedVoiceCatalog).Load(ctx, providerID string, client TTSClient, refresh bool) (CatalogSnapshot, error)`.

- [x] **Step 1: Write the failing test**

Create `pkg/media/voice_catalog_test.go`:

```go
package media

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// catalogClient is a TTSClient that also enumerates voices, with a fail switch.
type catalogClient struct {
	voices []ProviderVoice
	calls  int
	fail   bool
}

func (c *catalogClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return nil, nil
}

func (c *catalogClient) ListVoices(ctx context.Context) ([]ProviderVoice, error) {
	c.calls++
	if c.fail {
		return nil, errors.New("catalog unavailable")
	}
	return c.voices, nil
}

// plainClient only synthesises, so it has no catalog to offer.
type plainClient struct{}

func (p *plainClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return nil, nil
}

func TestCachedVoiceCatalogFetchesThenServesFromDisk(t *testing.T) {
	client := &catalogClient{voices: []ProviderVoice{{ID: "v1", Name: "Voice One"}}}
	catalog := NewCachedVoiceCatalog(t.TempDir())

	first, err := catalog.Load(context.Background(), "builtin:test", client, false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(first.Voices) != 1 || first.Stale || first.Provider != "builtin:test" {
		t.Errorf("first snapshot = %+v", first)
	}
	if client.calls != 1 {
		t.Fatalf("expected one fetch, got %d", client.calls)
	}

	second, err := catalog.Load(context.Background(), "builtin:test", client, false)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if client.calls != 1 {
		t.Errorf("a fresh snapshot should not refetch, calls = %d", client.calls)
	}
	if len(second.Voices) != 1 {
		t.Errorf("second snapshot = %+v", second)
	}

	refreshed, err := catalog.Load(context.Background(), "builtin:test", client, true)
	if err != nil {
		t.Fatalf("refresh Load: %v", err)
	}
	if client.calls != 2 {
		t.Errorf("refresh should refetch, calls = %d", client.calls)
	}
	if refreshed.Stale {
		t.Errorf("a successful refresh is not stale")
	}
}

func TestCachedVoiceCatalogServesStaleOnError(t *testing.T) {
	client := &catalogClient{voices: []ProviderVoice{{ID: "v1"}}}
	catalog := NewCachedVoiceCatalog(t.TempDir())
	if _, err := catalog.Load(context.Background(), "builtin:test", client, false); err != nil {
		t.Fatalf("Load: %v", err)
	}

	client.fail = true
	catalog.ttl = 0 // force a refetch
	stale, err := catalog.Load(context.Background(), "builtin:test", client, true)
	if err != nil {
		t.Fatalf("expected the last snapshot, got %v", err)
	}
	if !stale.Stale || len(stale.Voices) != 1 {
		t.Errorf("stale snapshot = %+v", stale)
	}
}

func TestCachedVoiceCatalogErrorWithoutSnapshot(t *testing.T) {
	client := &catalogClient{fail: true}
	catalog := NewCachedVoiceCatalog(t.TempDir())

	snapshot, err := catalog.Load(context.Background(), "builtin:test", client, false)
	if err == nil {
		t.Fatalf("expected an error when nothing is cached")
	}
	if len(snapshot.Voices) != 0 || snapshot.Stale {
		t.Errorf("snapshot = %+v", snapshot)
	}
}

func TestCachedVoiceCatalogWithoutCatalogSupport(t *testing.T) {
	catalog := NewCachedVoiceCatalog(t.TempDir())
	snapshot, err := catalog.Load(context.Background(), "builtin:test", &plainClient{}, false)
	if err != nil {
		t.Fatalf("a provider without a catalog is not an error: %v", err)
	}
	if len(snapshot.Voices) != 0 {
		t.Errorf("snapshot = %+v", snapshot)
	}
}

func TestCachedVoiceCatalogHonoursTTL(t *testing.T) {
	client := &catalogClient{voices: []ProviderVoice{{ID: "v1"}}}
	catalog := NewCachedVoiceCatalog(t.TempDir())
	catalog.ttl = time.Hour
	now := time.Now()
	catalog.now = func() time.Time { return now }

	if _, err := catalog.Load(context.Background(), "builtin:test", client, false); err != nil {
		t.Fatalf("Load: %v", err)
	}

	now = now.Add(2 * time.Hour)
	if _, err := catalog.Load(context.Background(), "builtin:test", client, false); err != nil {
		t.Fatalf("Load after TTL: %v", err)
	}
	if client.calls != 2 {
		t.Errorf("an expired snapshot should refetch, calls = %d", client.calls)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestCachedVoiceCatalog ./pkg/media/ -v`
Expected: FAIL with "undefined: NewCachedVoiceCatalog".

- [x] **Step 3: Write minimal implementation**

Create `pkg/media/voice_catalog.go`:

```go
package media

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// defaultCatalogTTL is how long a fetched catalog is trusted before another fetch.
const defaultCatalogTTL = 24 * time.Hour

// catalogFetchTimeout bounds one provider catalog call.
const catalogFetchTimeout = 20 * time.Second

// CatalogSnapshot is a provider's voices as of a moment, with Stale set when the
// provider could not be reached and this is the last answer kept on disk.
type CatalogSnapshot struct {
	Provider  string          `json:"provider"`
	FetchedAt time.Time       `json:"fetched_at"`
	Stale     bool            `json:"stale"`
	Voices    []ProviderVoice `json:"voices"`
}

// CachedVoiceCatalog fetches a provider's voices at most once per TTL and keeps
// the last successful answer on disk, so an outage does not empty the picker.
type CachedVoiceCatalog struct {
	cacheDir string
	ttl      time.Duration
	now      func() time.Time
}

// NewCachedVoiceCatalog stores snapshots under <cacheDir>/voices.
func NewCachedVoiceCatalog(cacheDir string) *CachedVoiceCatalog {
	return &CachedVoiceCatalog{cacheDir: cacheDir, ttl: defaultCatalogTTL, now: time.Now}
}

// Load returns a provider's catalog, from disk when it is fresh, from the
// provider otherwise. A provider that cannot enumerate voices yields an empty
// snapshot and no error. A fetch failure returns the last snapshot marked stale;
// with nothing cached it returns the error and an empty snapshot.
func (c *CachedVoiceCatalog) Load(ctx context.Context, providerID string, client TTSClient, refresh bool) (CatalogSnapshot, error) {
	path := c.snapshotPath(providerID)
	last, haveLast := readCatalogSnapshot(path)

	catalog, ok := client.(VoiceCatalog)
	if !ok {
		return CatalogSnapshot{Provider: providerID, Voices: []ProviderVoice{}}, nil
	}

	if !refresh && haveLast && c.now().Sub(last.FetchedAt) < c.ttl {
		last.Stale = false
		return last, nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, catalogFetchTimeout)
	defer cancel()

	voices, err := catalog.ListVoices(fetchCtx)
	if err != nil {
		if haveLast {
			last.Stale = true
			return last, nil
		}
		return CatalogSnapshot{Provider: providerID, Voices: []ProviderVoice{}}, err
	}

	snapshot := CatalogSnapshot{Provider: providerID, FetchedAt: c.now().UTC(), Voices: voices}
	writeCatalogSnapshot(path, snapshot)
	return snapshot, nil
}

func (c *CachedVoiceCatalog) snapshotPath(providerID string) string {
	safe := strings.NewReplacer(":", "-", "/", "-", "\\", "-").Replace(providerID)
	return filepath.Join(c.cacheDir, "voices", safe+".json")
}

func readCatalogSnapshot(path string) (CatalogSnapshot, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CatalogSnapshot{}, false
	}
	var snapshot CatalogSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return CatalogSnapshot{}, false
	}
	return snapshot, true
}

func writeCatalogSnapshot(path string, snapshot CatalogSnapshot) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestCachedVoiceCatalog ./pkg/media/ -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/media/voice_catalog.go pkg/media/voice_catalog_test.go
git commit -m "feat(media): cache a provider voice catalog with stale fallback"
```

---

## Task 8: The inspect endpoint

**Files:**
- Modify: `pkg/gui/types.go:254-258` (`TestProviderRequestDTO`)
- Create: `pkg/gui/tts_inspect.go`
- Modify: `pkg/gui/service.go` (`Service` struct, `NewService`, `TestProvider` voice_id)
- Modify: `pkg/gui/server.go` (route and handler)
- Test: `pkg/gui/tts_inspect_test.go`

**Interfaces:**
- Consumes: `media.ProviderKey`, `media.VoiceOptions`, `media.VoiceCatalog`, `media.MeteredProvider`, `media.NewCachedVoiceCatalog`.
- Produces: `gui.TTSInspectRequestDTO`, `gui.TTSInspectResponseDTO`, `gui.VoiceCatalogDTO`, `(*Service).InspectTTS(ctx, req TTSInspectRequestDTO) (*TTSInspectResponseDTO, error)`, `POST /api/tts/inspect`, `TestProviderRequestDTO.VoiceID`.

- [x] **Step 1: Write the failing test**

Create `pkg/gui/tts_inspect_test.go`:

```go
package gui

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

// inspectingClient implements every optional capability, so one fake covers the
// whole inspect response.
type inspectingClient struct{}

func (c *inspectingClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return []byte("audio"), nil
}

func (c *inspectingClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error) {
	return []media.ProviderVoice{{ID: "v1", Name: "Voice One", Tags: []string{"male"}}}, nil
}

func (c *inspectingClient) VoiceOptions() []media.VoiceOption {
	return []media.VoiceOption{{Key: "stability", Label: "Stability", Kind: "float", Min: 0, Max: 1}}
}

func (c *inspectingClient) Metered() bool { return true }

// bareClient only synthesises, so it has no options and no catalog.
type bareClient struct{}

func (c *bareClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return []byte("audio"), nil
}

func TestInspectTTSReportsCapabilities(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &inspectingClient{}, nil
	}

	res, err := svc.InspectTTS(context.Background(), TTSInspectRequestDTO{
		Config: config.TTSConfig{Type: "http", Endpoint: "http://localhost:8880/v1/audio/speech", APIKey: "secret"},
	})
	if err != nil {
		t.Fatalf("InspectTTS: %v", err)
	}
	if res.ProviderKey != "http:localhost:8880" {
		t.Errorf("ProviderKey = %q", res.ProviderKey)
	}
	if !res.Metered {
		t.Errorf("expected the provider's Metered declaration to surface")
	}
	if len(res.Options) != 1 || res.Options[0].Key != "stability" {
		t.Errorf("options = %+v", res.Options)
	}
	if !res.Catalog.Available || len(res.Catalog.Voices) != 1 {
		t.Errorf("catalog = %+v", res.Catalog)
	}

	encoded, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "api_key") {
		t.Errorf("inspect response leaked a secret: %s", encoded)
	}
}

func TestInspectTTSWithoutCapabilities(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &bareClient{}, nil
	}

	res, err := svc.InspectTTS(context.Background(), TTSInspectRequestDTO{Config: config.TTSConfig{Type: "builtin", BuiltinName: "native-os"}})
	if err != nil {
		t.Fatalf("InspectTTS: %v", err)
	}
	if len(res.Options) != 0 {
		t.Errorf("expected no options, got %+v", res.Options)
	}
	if res.Catalog.Available {
		t.Errorf("expected no catalog")
	}
	if res.Metered {
		t.Errorf("a provider that is not metered must report false")
	}
}

func TestInspectTTSConfigOverridesMetered(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &bareClient{}, nil
	}

	on := true
	res, err := svc.InspectTTS(context.Background(), TTSInspectRequestDTO{Config: config.TTSConfig{Type: "cli", Command: "piper", Metered: &on}})
	if err != nil {
		t.Fatalf("InspectTTS: %v", err)
	}
	if !res.Metered {
		t.Errorf("a configured metered flag must override the provider")
	}
}

func TestInspectTTSRoute(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &inspectingClient{}, nil
	}
	server := NewServer(svc, AssetHandler())

	body := `{"config":{"type":"http","endpoint":"http://localhost:8880/v1/audio/speech"}}`
	req := httptest.NewRequest("POST", "/api/tts/inspect", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "provider_key") {
		t.Errorf("body = %s", rec.Body.String())
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestInspectTTS ./pkg/gui/ -v`
Expected: FAIL with "unknown field newTTSClient" and "undefined: TTSInspectRequestDTO".

- [x] **Step 3: Write minimal implementation**

In `pkg/gui/types.go`, add `"time"` to the imports and the DTOs after `TestProviderResponseDTO`:

```go
// TTSInspectRequestDTO asks what a TTS configuration can do. The config may be
// unsaved, which is what lets the editor describe a provider before it is applied.
type TTSInspectRequestDTO struct {
	Config  config.TTSConfig `json:"config"`
	Refresh bool             `json:"refresh,omitempty"`
}

// VoiceCatalogDTO is a provider's voices plus whether the provider can enumerate
// at all, so the UI can explain instead of offering a dead button.
type VoiceCatalogDTO struct {
	Available bool                  `json:"available"`
	FetchedAt time.Time             `json:"fetched_at,omitempty"`
	Stale     bool                  `json:"stale"`
	Voices    []media.ProviderVoice `json:"voices"`
}

// TTSInspectResponseDTO is everything the speech editor needs about one
// configuration. It never carries the configuration's API key.
type TTSInspectResponseDTO struct {
	ProviderKey string            `json:"provider_key"`
	Metered     bool              `json:"metered"`
	Options     []media.VoiceOption `json:"options,omitempty"`
	Catalog     VoiceCatalogDTO   `json:"catalog"`
	// Error is a non-fatal catalog failure, so the editor still renders options.
	Error string `json:"error,omitempty"`
}

Edit the existing `TestProviderRequestDTO` in place (do not add a second definition) to add one field:

```go
type TestProviderRequestDTO struct {
	Category   string      `json:"category"` // "llm", "tts", "stt", "image"
	Provider   interface{} `json:"provider"`
	TestPrompt string      `json:"test_prompt,omitempty"`
	// VoiceID lets a probe audition a catalog voice that has not been saved yet.
	VoiceID string `json:"voice_id,omitempty"`
}
```

Create `pkg/gui/tts_inspect.go`:

```go
package gui

import (
	"context"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

// InspectTTS describes one TTS configuration: its provider identity, whether it
// is metered, the tunables it accepts, and its voice catalog. It builds the
// client from the supplied configuration rather than the saved one, so the editor
// can describe a provider before it is applied.
func (s *Service) InspectTTS(ctx context.Context, req TTSInspectRequestDTO) (*TTSInspectResponseDTO, error) {
	cfg := req.Config
	if cfg.Type == "builtin" && (cfg.BuiltinName == "sherpa-onnx" || cfg.BuiltinName == "kokoro") {
		if cfg.ModelPath == "" && s.modelsManager != nil {
			cfg.ModelPath = s.modelsManager.ModelDir("kokoro-tts")
		}
	}

	response := &TTSInspectResponseDTO{
		ProviderKey: media.ProviderKey(cfg),
		Catalog:     VoiceCatalogDTO{Voices: []media.ProviderVoice{}},
	}

	client, err := s.ttsClientFor(cfg)
	if err != nil {
		response.Error = err.Error()
		return response, nil
	}

	if options, ok := client.(media.VoiceOptions); ok {
		response.Options = options.VoiceOptions()
	}

	response.Metered = cfg.Metered != nil && *cfg.Metered
	if cfg.Metered == nil {
		if metered, ok := client.(media.MeteredProvider); ok {
			response.Metered = metered.Metered()
		}
	}

	catalog := media.NewCachedVoiceCatalog(s.resolver.CacheDir())
	snapshot, err := catalog.Load(ctx, response.ProviderKey, client, req.Refresh)
	if _, ok := client.(media.VoiceCatalog); ok {
		response.Catalog.Available = true
	}
	response.Catalog.FetchedAt = snapshot.FetchedAt
	response.Catalog.Stale = snapshot.Stale
	if snapshot.Voices != nil {
		response.Catalog.Voices = snapshot.Voices
	}
	if err != nil {
		response.Error = err.Error()
	}

	return response, nil
}

// ttsClientFor builds a TTS client, using the injectable factory so a test can
// describe a provider that needs no network.
func (s *Service) ttsClientFor(cfg config.TTSConfig) (media.TTSClient, error) {
	if s.newTTSClient != nil {
		return s.newTTSClient(cfg)
	}
	return media.NewTTSClient(cfg)
}
```

In `pkg/gui/service.go`, add the field to the `Service` struct after `modelsManager`:

```go
	// newTTSClient builds a TTS client from configuration. It is a field so a test
	// can describe a provider without a network, and nil means the real factory.
	newTTSClient func(config.TTSConfig) (media.TTSClient, error)
```

In `TestProvider`'s `"tts"` case, honour a requested voice:

```go
		voiceID := ttsCfg.DefaultVoice
		if req.VoiceID != "" {
			voiceID = req.VoiceID
		}
		voice := &entity.VoiceConfig{
			VoiceID:    voiceID,
			Pitch:      ttsCfg.Pitch,
			SpeechRate: ttsCfg.SpeechRate,
		}
```

In `pkg/gui/server.go`, register the route after `/api/settings/test-provider`:

```go
	s.mux.HandleFunc("/api/tts/inspect", s.handleTTSInspectRoute)
```

and add the handler next to `handleTestProviderRoute`:

```go
func (s *Server) handleTTSInspectRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TTSInspectRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	res, err := s.service.InspectTTS(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}
```

Add `"github.com/darkliquid/localrpg/pkg/media"` to `pkg/gui/types.go` imports.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestInspectTTS ./pkg/gui/ -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/tts_inspect.go pkg/gui/service.go pkg/gui/server.go pkg/gui/tts_inspect_test.go
git commit -m "feat(gui): describe a TTS provider's voices and options"
```

---

## Task 9: Filter profiles to the active provider

**Files:**
- Create: `pkg/media/filter.go`
- Test: `pkg/media/filter_test.go`, `pkg/gui/service_test.go`
- Modify: `pkg/gui/service.go:1059` and `pkg/gui/service.go:1775`

**Interfaces:**
- Consumes: `config.VoiceProfile`, `media.ProviderKey`.
- Produces: `media.FilterVoiceProfiles(profiles []config.VoiceProfile, activeProvider string) []config.VoiceProfile`.

- [x] **Step 1: Write the failing test**

Create `pkg/media/filter_test.go`:

```go
package media

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestFilterVoiceProfiles(t *testing.T) {
	profiles := []config.VoiceProfile{
		{ID: "portable", VoiceID: "x"},
		{ID: "kokoro-one", Provider: "builtin:kokoro", VoiceID: "af_bella"},
		{ID: "eleven-one", Provider: "builtin:elevenlabs", VoiceID: "EXAVIT"},
	}

	kept := FilterVoiceProfiles(profiles, "builtin:kokoro")
	if len(kept) != 2 {
		t.Fatalf("kept %d profiles, want the portable one and the Kokoro one", len(kept))
	}
	if kept[0].ID != "portable" || kept[1].ID != "kokoro-one" {
		t.Errorf("kept = %+v", kept)
	}

	if kept := FilterVoiceProfiles(profiles, "builtin:elevenlabs"); len(kept) != 2 || kept[1].ID != "eleven-one" {
		t.Errorf("kept = %+v", kept)
	}

	// A disabled provider keeps only portable profiles, because nothing else can
	// be synthesised.
	if kept := FilterVoiceProfiles(profiles, "disabled"); len(kept) != 1 || kept[0].ID != "portable" {
		t.Errorf("kept = %+v", kept)
	}
}
```

Append to `pkg/gui/service_test.go`:

```go
func TestPrepareTurnFiltersProfilesToTheActiveProvider(t *testing.T) {
	profiles := []config.VoiceProfile{
		{ID: "portable", VoiceID: "x"},
		{ID: "other", Provider: "builtin:elevenlabs", VoiceID: "y"},
	}
	active := "builtin:kokoro"
	kept := media.FilterVoiceProfiles(profiles, active)
	if len(kept) != 1 || kept[0].ID != "portable" {
		t.Errorf("kept = %+v", kept)
	}
}
```

Add `"github.com/darkliquid/localrpg/pkg/media"` to the test imports if missing. (This asserts the helper; the wiring is verified by the task's edit and the full suite.)

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestFilterVoiceProfiles|TestPrepareTurnFiltersProfilesToTheActiveProvider' ./pkg/media/ ./pkg/gui/ -v`
Expected: FAIL with "undefined: FilterVoiceProfiles".

- [x] **Step 3: Write minimal implementation**

Create `pkg/media/filter.go`:

```go
package media

import "github.com/darkliquid/localrpg/pkg/config"

// FilterVoiceProfiles returns the profiles the active provider can synthesise. A
// profile with no provider is portable and always kept; a profile bound to
// another provider is excluded from assignment rather than removed, so switching
// back restores it.
func FilterVoiceProfiles(profiles []config.VoiceProfile, activeProvider string) []config.VoiceProfile {
	kept := make([]config.VoiceProfile, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Provider == "" || profile.Provider == activeProvider {
			kept = append(kept, profile)
		}
	}
	return kept
}
```

In `pkg/gui/service.go`, change `prepareTurn`'s profile wiring (line 1059):

```go
	timeline.SetVoiceProfiles(media.FilterVoiceProfiles(cfg.Media.TTS.VoiceProfiles, media.ProviderKey(cfg.Media.TTS)))
```

and the codex assignment (line 1775):

```go
	cfg := s.configMgr.Get()
	harness.AssignVoiceProfile(ent, media.FilterVoiceProfiles(cfg.Media.TTS.VoiceProfiles, media.ProviderKey(cfg.Media.TTS)))
```

Match the surrounding code's existing `cfg` handling; if the function already holds a `cfg`, reuse it instead of re-reading the manager.

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ ./pkg/gui/ -count=1`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/media/filter.go pkg/media/filter_test.go pkg/gui/service.go pkg/gui/service_test.go
git commit -m "feat: assign only voices the active provider can synthesise"
```

---

## Task 10: Full gate

**Files:** none (verification only).

- [x] **Step 1: Run the whole suite**

Run: `go test ./... -count=1`
Expected: every package ok.

- [x] **Step 2: Vet and typecheck**

Run: `mise run lint` and `mise run test:frontend`
Expected: `go vet ./...` clean and `npx tsc --noEmit` clean (no frontend change in this plan, so this guards against accidental breakage).

- [x] **Step 3: Confirm the acceptance criteria this plan owns**

- 3: a profile can carry provider options and they persist (Task 6).
- 4: changing an option changes the cache key; no-options keys match the legacy value token (Task 4).
- 6: `/api/tts/inspect` never returns an API key (Task 8 test).
- 8: the gate above.

Criteria 1, 2, 5 (visible labelling), and 7 (the metered warning) belong to the frontend plan.

---

## Self-Review

**Spec coverage (backend scope):**

- 3.1 capability interfaces: Task 1.
- 3.2 provider identity and `Metered`: Tasks 1 and 8 (config field in Task 6, resolution in Task 8).
- 3.3 options on profiles and entities, plus `ValidateVoiceOptions`: Tasks 3 and 6.
- 3.4 option-aware cache key and its two callers: Tasks 4 and 5.
- 3.5 catalog caching: Task 7.
- 3.6 API surface: Task 8 (inspect endpoint and `voice_id` on test-provider); the "do not add a GET voices route" decision is honoured.
- 3.7 deterministic, provider-aware matching: Task 9 (filtering) and Task 2 (shared tags). Tag normalisation is exercised directly; consumer catalog implementations land with the ElevenLabs spec.
- 3.8 frontend: deferred to the follow-up plan.
- 3.9 cost guardrails: deferred with the frontend plan except `/api/tts/inspect`'s `metered` flag, which is Task 8.
- 5 compatibility table: Task 4 (legacy delegation), Task 6 (omitted-when-empty fields), Task 9 (filter, not remove).

**Placeholder scan:** no "TBD"/"implement later" text; every code step has real code.

**Type consistency:** `ProviderKey`, `ProviderVoice`, `VoiceOption`, `VoiceOptions`, `MeteredProvider` are defined in Task 1 and used in Tasks 7-9. `ComputeAudioCacheKeyForVoice` is defined in Task 4 and called in Task 5. `CatalogSnapshot` is defined in Task 7 and consumed in Task 8. `ValidateVoiceOptions` is defined in Task 3 and is called by the Settings save path in the follow-up plan. `FilterVoiceProfiles` is defined and called in Task 9. `newTTSClient` is defined and used in Task 8.

**Known deviations to report:** the cache-key rule and its pipeline consequence (see Discovery); `pkg/harness/extractor.go` and `pkg/media/filter.go` are added to the File Map beyond the spec's list, because options must travel with an assigned voice and filtering has to live somewhere testable.
