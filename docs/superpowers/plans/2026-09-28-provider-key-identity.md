# Provider Key Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every provider one canonical, family-namespaced key — with an optional instance discriminator and most-specific-first lookup — produced by a single resolver and consumed by the ledger, pricing, limits, failures, the catalogue, and the docs.

**Architecture:** A new dependency-free `provider.Key` type with a grammar, constants, and instance helpers lives in `pkg/provider`. The existing config-to-registry resolvers (`harness.ProviderIDFor`, `media.*ProviderIDFor`) become key resolvers, and the facade composition roots (router, media call sites, embedding wiring) stamp the resolved key onto recorded usage instead of adapters naming themselves. `pkg/pricing` resolves a key through a candidate list (instance then adapter, model then any), `pkg/config` validates price keys and bumps the version, and `pkg/gui` records embedding usage for the first time.

**Tech Stack:** Go 1.27 (standard library, `modernc.org/sqlite`, `gopkg.in/yaml.v3`), React 19 + TypeScript (unchanged).

**Spec:** `docs/superpowers/specs/2026-09-28-provider-key-identity-design.md`

## Global Constraints

- Keys are `<family>:<adapter>` or `<family>:<adapter>@<discriminator>`; family is one of `llm`, `tts`, `stt`, `image`, `embedding`; adapter matches `^[a-z0-9-]+$`; discriminator matches `^[a-z0-9.:-]+$`.
- `pkg/provider` imports no other internal package (it is a leaf). No new internal import may be added to it.
- `interface{}`, never `any`. Errors wrapped with `fmt.Errorf("...: %w", err)`. Tests use stdlib `testing` and `t.TempDir()`; no testify.
- Lookup order is fixed: `(key, model)` → `(key, "")` → `(parent, model)` → `(parent, "")`, then built-ins, then zero cost.
- Hard cut: no alias table, no rewriting of stored rows, legacy price keys are rejected.
- Every task ends with `mise run lint` clean and `go vet` clean; run `mise run test` before each commit.
- Commit subjects follow Conventional Commits with a scope, under 72 characters.

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| Create | `pkg/provider/key.go` | `Key` type, grammar, constructors, instance helpers, family/adapter accessors |
| Create | `pkg/provider/keys.go` | Canonical key constants (one definition per adapter) |
| Create | `pkg/provider/key_test.go` | Grammar, parse, instance, parent tests |
| Modify | `pkg/provider/provider.go` | `Validate` also checks key grammar; add duplicate-`(family,adapter)` guard |
| Create | `pkg/provider/registry_test.go` | Uniqueness and well-formedness of registered IDs |
| Modify | `pkg/provider/*/*.go` | Descriptor `ID` uses the canonical constant (19 packages) |
| Modify | `pkg/harness/exports.go` | `KeyFor(ProviderConfig) (provider.Key, bool)` |
| Modify | `pkg/harness/router.go` | Role → key map; stamp key onto recorded usage |
| Modify | `pkg/harness/factory.go` | `RouterFromConfig` assigns role keys; `ExtractorFromConfig` assigns the extractor key |
| Modify | `pkg/harness/extractor.go` | `SetProviderKey`; stamp key on extraction usage |
| Modify | `pkg/harness/failure.go` | Attempt `Provider` documented as canonical key |
| Create | `pkg/harness/key_test.go` | `KeyFor` table tests |
| Modify | `pkg/media/exports.go` | `TTSKeyFor` / `STTKeyFor` / `ImageKeyFor` |
| Modify | `pkg/media/catalog.go` | Remove `ProviderKey`; add `InstanceDiscriminator` helper |
| Create | `pkg/media/key_test.go` | Media resolver table tests |
| Modify | `pkg/embeddings/factory.go` | `KeyFor(config.EmbeddingsConfig) (provider.Key, bool)` |
| Create | `pkg/embeddings/key_test.go` | Embedding resolver tests |
| Modify | `pkg/provider/openaiembedding/openai.go` | Delete hardcoded `Provider`; keep `LastUsage` |
| Modify | `pkg/provider/geminiembedding/gemini.go` | Parse `UsageMetadata`; add `LastUsage` |
| Modify | `pkg/provider/geminillm/provider.go`, `pkg/provider/openaichat/http.go` | Delete hardcoded `Provider` |
| Modify | `pkg/pricing/pricing.go` | Candidate-list resolution; rekeyed `BuiltinPrices` |
| Modify | `pkg/pricing/pricing_test.go` | Fallback precedence tests |
| Modify | `pkg/config/types.go` | `Config.Version` = `"2"`; `Validate()` |
| Modify | `pkg/config/manager.go` | Collect and expose load warnings |
| Create | `pkg/config/validate_test.go` | Version and price-key validation tests |
| Modify | `pkg/gui/service.go` | TTS/STT/image recording uses the resolver; embedding sink wiring |
| Modify | `pkg/gui/image_generation.go` | Image recording uses the resolver |
| Modify | `pkg/gui/limits.go` | Delete `providerKeyForRole`/`roleProviderKey`; resolve via facades |
| Modify | `pkg/gui/usage.go` | `mediaUsage` takes a `provider.Key` |
| Create | `pkg/gui/embedding_usage.go` | `EmbeddingUsageSink` implementation on `Service` |
| Create | `pkg/gui/provider_key_test.go` | Ledger/limit key agreement; instance isolation |
| Modify | `pkg/storage/embedding_worker.go` | Optional usage sink reported after a batch |
| Create | `pkg/storage/embedding_usage_test.go` | Worker reports usage |
| Modify | `pkg/tools/tools.go`, `pkg/tools/memory.go` | Optional usage sink on embedding calls |
| Create | `pkg/tools/embedding_usage_test.go` | Tools report usage |
| Modify | `pkg/gui/docs_catalogue_test.go` | Ledger key from the resolver, not a local copy |
| Modify | `pkg/gui/docs/11-usage-and-pricing.md` | Document canonical keys and instance fallback |
| Create | `docs/releases/provider-keys.md` | Old-to-new mapping release note |

---

### Task 1: The `provider.Key` type

**Files:**
- Create: `pkg/provider/key.go`
- Test: `pkg/provider/key_test.go`

**Interfaces:**
- Consumes: existing `Family` type and `FamilyLLM`/`FamilyTTS`/`FamilySTT`/`FamilyImage`/`FamilyEmbedding` constants (`pkg/provider/descriptor.go`).
- Produces: `type Key string`; `func NewKey(Family, string) (Key, error)`; `func ParseKey(string) (Key, error)`; `func NewInstanceKey(Key, string) (Key, error)`; `func InstanceOrSelf(Key, string) Key`; `(Key) Family() Family`; `(Key) Adapter() string`; `(Key) Instance() (string, bool)`; `(Key) Parent() Key`; `func HostDiscriminator(string) string`; `func CommandDiscriminator(string) string`.

- [x] **Step 1: Write the failing test**

```go
package provider

import "testing"

func TestParseKeyAcceptsAdapterAndInstance(t *testing.T) {
	tests := []struct {
		in       string
		family   Family
		adapter  string
		instance string
		parent   string
	}{
		{"llm:openaichat", FamilyLLM, "openaichat", "", "llm:openaichat"},
		{"tts:http@localhost:8880", FamilyTTS, "http", "localhost:8880", "tts:http"},
		{"embedding:gemini@default", FamilyEmbedding, "gemini", "default", "embedding:gemini"},
	}
	for _, tc := range tests {
		k, err := ParseKey(tc.in)
		if err != nil {
			t.Fatalf("ParseKey(%q): %v", tc.in, err)
		}
		if k.Family() != tc.family || k.Adapter() != tc.adapter {
			t.Errorf("ParseKey(%q) = %s/%s, want %s/%s", tc.in, k.Family(), k.Adapter(), tc.family, tc.adapter)
		}
		got, ok := k.Instance()
		if ok != (tc.instance != "") || got != tc.instance {
			t.Errorf("ParseKey(%q) instance = %q/%v, want %q", tc.in, got, ok, tc.instance)
		}
		if string(k.Parent()) != tc.parent {
			t.Errorf("ParseKey(%q).Parent() = %q, want %q", tc.in, k.Parent(), tc.parent)
		}
	}
}

func TestParseKeyRejectsMalformed(t *testing.T) {
	for _, in := range []string{"", "llm", ":gemini", "llm:", "LLM:gemini", "llm:Gemini", "llm:gemini@@", "llm:gemini@", "tts:http@Localhost"} {
		if _, err := ParseKey(in); err == nil {
			t.Errorf("ParseKey(%q) = nil error, want rejection", in)
		}
	}
}

func TestInstanceOrSelf(t *testing.T) {
	base, err := NewKey(FamilyTTS, "http")
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}
	if got := InstanceOrSelf(base, ""); got != base {
		t.Errorf("InstanceOrSelf with no discriminator = %q, want %q", got, base)
	}
	inst := InstanceOrSelf(base, "localhost:8880")
	if inst != "tts:http@localhost:8880" {
		t.Errorf("InstanceOrSelf = %q, want tts:http@localhost:8880", inst)
	}
	if inst.Parent() != base {
		t.Errorf("instance parent = %q, want %q", inst.Parent(), base)
	}
}

func TestDiscriminators(t *testing.T) {
	if got := HostDiscriminator("http://localhost:11434/v1"); got != "localhost:11434" {
		t.Errorf("HostDiscriminator = %q, want localhost:11434", got)
	}
	if got := HostDiscriminator("not a url"); got == "" {
		t.Errorf("HostDiscriminator should still name an odd endpoint")
	}
	if got := HostDiscriminator(""); got != "" {
		t.Errorf("HostDiscriminator(\"\") = %q, want empty", got)
	}
	if got := CommandDiscriminator("/usr/bin/piper"); got != "piper" {
		t.Errorf("CommandDiscriminator = %q, want piper", got)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/provider/ -run 'TestParseKey|TestInstanceOrSelf|TestDiscriminators' -v`
Expected: FAIL — `undefined: ParseKey` and friends.

- [x] **Step 3: Write minimal implementation**

```go
package provider

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Key is a canonical provider identity: "<family>:<adapter>" or
// "<family>:<adapter>@<discriminator>". The adapter segment is the registry
// descriptor ID; the discriminator names one configuration of it.
type Key string

var (
	adapterPattern      = regexp.MustCompile(`^[a-z0-9-]+$`)
	discriminatorPattern = regexp.MustCompile(`^[a-z0-9.:-]+$`)
)

func validFamily(f Family) bool {
	switch f {
	case FamilyLLM, FamilyTTS, FamilySTT, FamilyImage, FamilyEmbedding:
		return true
	default:
		return false
	}
}

// NewKey builds an adapter key, validating the family and adapter segment.
func NewKey(f Family, adapter string) (Key, error) {
	if !validFamily(f) {
		return "", fmt.Errorf("provider: unknown family %q", f)
	}
	if !adapterPattern.MatchString(adapter) {
		return "", fmt.Errorf("provider: adapter %q must match %s", adapter, adapterPattern)
	}
	return Key(string(f) + ":" + adapter), nil
}

// ParseKey validates an adapter or instance key.
func ParseKey(s string) (Key, error) {
	family, rest, ok := strings.Cut(s, ":")
	if !ok {
		return "", fmt.Errorf("provider: key %q must be <family>:<adapter>", s)
	}
	if !validFamily(Family(family)) {
		return "", fmt.Errorf("provider: key %q has unknown family %q", s, family)
	}
	adapter, disc, hasDisc := strings.Cut(rest, "@")
	if !adapterPattern.MatchString(adapter) {
		return "", fmt.Errorf("provider: adapter %q in key %q must match %s", adapter, s, adapterPattern)
	}
	if hasDisc && !discriminatorPattern.MatchString(disc) {
		return "", fmt.Errorf("provider: discriminator %q in key %q must match %s", disc, s, discriminatorPattern)
	}
	return Key(s), nil
}

// NewInstanceKey builds an instance key from an adapter key and a discriminator.
func NewInstanceKey(k Key, discriminator string) (Key, error) {
	if _, ok := k.Instance(); ok {
		return "", fmt.Errorf("provider: %q is already an instance key", k)
	}
	if !discriminatorPattern.MatchString(discriminator) {
		return "", fmt.Errorf("provider: discriminator %q must match %s", discriminator, discriminatorPattern)
	}
	return ParseKey(string(k) + "@" + discriminator)
}

// InstanceOrSelf returns the instance key when a discriminator is present, and
// the adapter key otherwise, so resolvers need one line per family.
func InstanceOrSelf(k Key, discriminator string) Key {
	if discriminator == "" {
		return k
	}
	inst, err := NewInstanceKey(k, discriminator)
	if err != nil {
		return k
	}
	return inst
}

func (k Key) Family() Family {
	family, _, _ := strings.Cut(string(k), ":")
	return Family(family)
}

func (k Key) Adapter() string {
	_, rest, _ := strings.Cut(string(k), ":")
	adapter, _, _ := strings.Cut(rest, "@")
	return adapter
}

func (k Key) Instance() (string, bool) {
	_, rest, _ := strings.Cut(string(k), ":")
	_, disc, ok := strings.Cut(rest, "@")
	return disc, ok
}

// Parent drops the instance discriminator.
func (k Key) Parent() Key {
	disc, ok := k.Instance()
	if !ok {
		return k
	}
	return Key(strings.TrimSuffix(string(k), "@"+disc))
}

// HostDiscriminator is the host and port of an endpoint, lowercased and
// sanitised, or empty when no endpoint is given.
func HostDiscriminator(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Host != "" {
		return sanitiseDiscriminator(parsed.Host)
	}
	stripped := strings.TrimPrefix(strings.TrimPrefix(trimmed, "http://"), "https://")
	if idx := strings.IndexByte(stripped, '/'); idx >= 0 {
		stripped = stripped[:idx]
	}
	return sanitiseDiscriminator(stripped)
}

// CommandDiscriminator is the basename of a command, lowercased.
func CommandDiscriminator(command string) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return ""
	}
	return sanitiseDiscriminator(filepath.Base(trimmed))
}

func sanitiseDiscriminator(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.', r == ':':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/provider/ -run 'TestParseKey|TestInstanceOrSelf|TestDiscriminators' -v`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/provider/key.go pkg/provider/key_test.go
git commit -m "feat(provider): add the canonical provider key type"
```

---

### Task 2: Canonical key constants and registry IDs

**Files:**
- Create: `pkg/provider/keys.go`
- Modify: `pkg/provider/provider.go:37-48` (`Register` duplicate guard), `Validate` at `:85-95`
- Modify: 19 descriptor files under `pkg/provider/*/` (one `ID:` line each)
- Test: `pkg/provider/registry_test.go`

**Interfaces:**
- Consumes: `Key`, `ParseKey`, `NewKey` from Task 1; `Descriptor`, `Registration`, `List`, `Lookup` from `pkg/provider`.
- Produces: exported `Key` constants used by adapters and resolvers in later tasks, e.g. `provider.KeyLLMOpenAIChat`, `provider.KeyLLMGemini`, `provider.KeyLLMCLI`, `provider.KeyLLMNarrativeOracle`, `provider.KeyTTSGemini`, `provider.KeyTTSElevenLabs`, `provider.KeyTTSNativeOS`, `provider.KeyTTSSherpaONNX`, `provider.KeyTTSPiper`, `provider.KeyTTSHTTP`, `provider.KeySTTWhisperHTTP`, `provider.KeySTTWhisperCLI`, `provider.KeySTTWebSpeech`, `provider.KeyImageGemini`, `provider.KeyImageHTTP`, `provider.KeyImageCLI`, `provider.KeyImageProceduralArt`, `provider.KeyEmbeddingBuiltin`, `provider.KeyEmbeddingOpenAI`, `provider.KeyEmbeddingGemini`.

- [x] **Step 1: Write the failing test**

```go
package provider_test

import (
	"strings"
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestEveryRegisteredIDIsACanonicalKey(t *testing.T) {
	if err := provider.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, id := range provider.IDs() {
		k, err := provider.ParseKey(id)
		if err != nil {
			t.Errorf("registered ID %q is not a canonical key: %v", id, err)
			continue
		}
		if _, ok := k.Instance(); ok {
			t.Errorf("registered ID %q must be an adapter key, not an instance key", id)
		}
	}
}

func TestDescriptorKeyIsUniquePerFamily(t *testing.T) {
	seen := map[string]string{}
	for _, id := range provider.IDs() {
		k, err := provider.ParseKey(id)
		if err != nil {
			continue
		}
		pair := strings.Join([]string{string(k.Family()), k.Adapter()}, "/")
		if other, dup := seen[pair]; dup {
			t.Errorf("%q and %q share family/adapter %s", other, id, pair)
		}
		seen[pair] = id
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/provider/ -run 'TestEveryRegisteredIDIsACanonicalKey|TestDescriptorKeyIsUniquePerFamily' -v`
Expected: FAIL — many `registered ID "tts-elevenlabs" is not a canonical key`.

- [x] **Step 3: Add the constants**

```go
package provider

// Canonical adapter keys. Each is the single definition of an adapter's
// identity; descriptors and resolvers reference these constants rather than
// repeating the string.
const (
	KeyLLMOpenAIChat       Key = "llm:openaichat"
	KeyLLMGemini           Key = "llm:gemini"
	KeyLLMCLI              Key = "llm:cli"
	KeyLLMNarrativeOracle  Key = "llm:narrative-oracle"

	KeyTTSGemini           Key = "tts:gemini"
	KeyTTSElevenLabs       Key = "tts:elevenlabs"
	KeyTTSNativeOS         Key = "tts:native-os"
	KeyTTSSherpaONNX       Key = "tts:sherpa-onnx"
	KeyTTSPiper            Key = "tts:piper"
	KeyTTSHTTP             Key = "tts:http"

	KeySTTWhisperHTTP      Key = "stt:whisper-http"
	KeySTTWhisperCLI       Key = "stt:whisper-cli"
	KeySTTWebSpeech        Key = "stt:web-speech"

	KeyImageGemini         Key = "image:gemini"
	KeyImageHTTP           Key = "image:http"
	KeyImageCLI            Key = "image:cli"
	KeyImageProceduralArt  Key = "image:procedural-art"

	KeyEmbeddingBuiltin    Key = "embedding:builtin"
	KeyEmbeddingOpenAI     Key = "embedding:openai"
	KeyEmbeddingGemini     Key = "embedding:gemini"
)

// AllKeys lists every canonical adapter key, for validation and docs.
func AllKeys() []Key {
	return []Key{
		KeyLLMOpenAIChat, KeyLLMGemini, KeyLLMCLI, KeyLLMNarrativeOracle,
		KeyTTSGemini, KeyTTSElevenLabs, KeyTTSNativeOS, KeyTTSSherpaONNX, KeyTTSPiper, KeyTTSHTTP,
		KeySTTWhisperHTTP, KeySTTWhisperCLI, KeySTTWebSpeech,
		KeyImageGemini, KeyImageHTTP, KeyImageCLI, KeyImageProceduralArt,
		KeyEmbeddingBuiltin, KeyEmbeddingOpenAI, KeyEmbeddingGemini,
	}
}
```

- [x] **Step 4: Rename every descriptor ID**

Edit each descriptor's `ID:` to the matching constant. One example, then apply to all 19:

```go
// pkg/provider/ttselevenlabs/ttselevenlabs.go
Descriptor: provider.Descriptor{
	ID:          string(provider.KeyTTSElevenLabs),
	Family:      provider.FamilyTTS,
	Label:       "ElevenLabs (Cloud, metered)",
	// ...
},
```

Mapping to apply:

| File | Old `ID` | New |
|---|---|---|
| `openaichat/openaichat.go` | `"openaichat"` | `string(provider.KeyLLMOpenAIChat)` |
| `geminillm/geminillm.go` | `"gemini"` | `string(provider.KeyLLMGemini)` |
| `clillm/clillm.go` | `"cli"` | `string(provider.KeyLLMCLI)` |
| `oracle/oracle.go` | `"narrative-oracle"` | `string(provider.KeyLLMNarrativeOracle)` |
| `ttsgemini/ttsgemini.go` | `"tts-gemini"` | `string(provider.KeyTTSGemini)` |
| `ttselevenlabs/ttselevenlabs.go` | `"tts-elevenlabs"` | `string(provider.KeyTTSElevenLabs)` |
| `ttsnativeos/ttsnativeos.go` | `"tts-native-os"` | `string(provider.KeyTTSNativeOS)` |
| `ttssherpa/ttssherpa.go` | `"tts-sherpa-onnx"` | `string(provider.KeyTTSSherpaONNX)` |
| `ttspiper/ttspiper.go` | `"tts-piper"` | `string(provider.KeyTTSPiper)` |
| `ttshttp/ttshttp.go` | `"tts-openai-http"` | `string(provider.KeyTTSHTTP)` |
| `sttwhisperhttp/sttwhisperhttp.go` | `"stt-whisper-http"` | `string(provider.KeySTTWhisperHTTP)` |
| `sttwhispercli/sttwhispercli.go` | `"stt-whisper-cli"` | `string(provider.KeySTTWhisperCLI)` |
| `sttwebspeech/sttwebspeech.go` | `"stt-webspeech"` | `string(provider.KeySTTWebSpeech)` |
| `imagegemini/imagegemini.go` | `"image-gemini"` | `string(provider.KeyImageGemini)` |
| `imagehttp/imagehttp.go` | `"image-http"` | `string(provider.KeyImageHTTP)` |
| `imagecli/imagecli.go` | `"image-cli"` | `string(provider.KeyImageCLI)` |
| `imageprocedural/imageprocedural.go` | `"image-procedural-art"` | `string(provider.KeyImageProceduralArt)` |
| `openaiembedding/openai.go` | `"openai-embedding"` | `string(provider.KeyEmbeddingOpenAI)` |
| `geminiembedding/gemini.go` | `"gemini-embedding"` | `string(provider.KeyEmbeddingGemini)` |

`pkg/provider/*` imports `pkg/provider` is not possible (same package); inside the package the constant is used directly as `KeyTTSElevenLabs`, and the descriptor field is `string(KeyTTSElevenLabs)`. Adjust accordingly.

- [x] **Step 5: Extend `Validate` and the duplicate guard**

```go
// pkg/provider/provider.go — replace Validate
func Validate() error {
	mu.RLock()
	defer mu.RUnlock()
	pairs := map[string]string{}
	for id, reg := range byID {
		if id == "" || reg.Descriptor.Family == "" || reg.Build == nil {
			return fmt.Errorf("provider: malformed registration %q", id)
		}
		key, err := ParseKey(id)
		if err != nil {
			return fmt.Errorf("provider: registration %q is not a canonical key: %w", id, err)
		}
		pair := string(key.Family()) + "/" + key.Adapter()
		if other, dup := pairs[pair]; dup {
			return fmt.Errorf("provider: %q and %q share family/adapter %s", other, id, pair)
		}
		pairs[pair] = id
	}
	return nil
}
```

`Register`'s existing duplicate-ID panic is sufficient; no change needed there.

- [x] **Step 6: Add a constants/registry agreement test**

```go
// append to pkg/provider/registry_test.go
func TestAllKeysAreRegisteredOrReserved(t *testing.T) {
	registered := map[string]bool{}
	for _, id := range provider.IDs() {
		registered[id] = true
	}
	for _, k := range provider.AllKeys() {
		if _, err := provider.ParseKey(string(k)); err != nil {
			t.Errorf("constant %q is malformed: %v", k, err)
		}
		if !registered[string(k)] {
			t.Errorf("key %q has no registered descriptor", k)
		}
	}
}
```

- [x] **Step 7: Run the tests and the suite**

Run: `go test ./pkg/provider/... ./pkg/provider/all/... -v`
Expected: PASS
Run: `mise run test:backend`
Expected: PASS. Some `pkg/harness` and `pkg/media` tests will fail because the resolvers still return the old IDs; those are fixed in Task 3. If they fail here, run only `./pkg/provider/...` and note the failure for Task 3.

- [x] **Step 8: Commit**

```bash
git add pkg/provider
git commit -m "refactor(provider): name adapters with canonical family keys"
```

---

### Task 3: Family resolvers

**Files:**
- Modify: `pkg/harness/exports.go`
- Modify: `pkg/media/exports.go`, `pkg/media/catalog.go:59-90`
- Modify: `pkg/embeddings/factory.go`
- Test: `pkg/harness/key_test.go`, `pkg/media/key_test.go`, `pkg/embeddings/key_test.go`

**Interfaces:**
- Consumes: `provider.Key` constants and helpers (Tasks 1-2).
- Produces: `harness.KeyFor(ProviderConfig) (provider.Key, bool)`; `media.TTSKeyFor(config.TTSConfig) (provider.Key, bool)`; `media.STTKeyFor(config.STTConfig) (provider.Key, bool)`; `media.ImageKeyFor(config.ImageConfig) (provider.Key, bool)`; `media.EndpointDiscriminator(string) string`; `embeddings.KeyFor(config.EmbeddingsConfig) (provider.Key, bool)`.

- [x] **Step 1: Write the failing table tests**

```go
// pkg/media/key_test.go
package media_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestTTSKeyFor(t *testing.T) {
	tests := []struct {
		cfg  config.TTSConfig
		want provider.Key
		ok   bool
	}{
		{config.TTSConfig{Type: "gemini"}, provider.KeyTTSGemini, true},
		{config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"}, provider.KeyTTSElevenLabs, true},
		{config.TTSConfig{Type: "builtin", BuiltinName: "sherpa-onnx"}, provider.KeyTTSSherpaONNX, true},
		{config.TTSConfig{Type: "cli", Command: "/usr/bin/piper"}, "", true},
		{config.TTSConfig{Type: "http", Endpoint: "http://localhost:8880"}, "", true},
		{config.TTSConfig{Type: "disabled"}, "", false},
	}
	for _, tc := range tests {
		got, ok := media.TTSKeyFor(tc.cfg)
		if ok != tc.ok {
			t.Errorf("TTSKeyFor(%+v) ok = %v, want %v", tc.cfg, ok, tc.ok)
			continue
		}
		if tc.want != "" && got != tc.want {
			t.Errorf("TTSKeyFor(%+v) = %q, want %q", tc.cfg, got, tc.want)
		}
	}
}

func TestTTSKeyForHTTPIsAnInstance(t *testing.T) {
	got, ok := media.TTSKeyFor(config.TTSConfig{Type: "http", Endpoint: "http://localhost:8880"})
	if !ok {
		t.Fatal("expected a key")
	}
	if got != "tts:http@localhost:8880" {
		t.Errorf("got %q, want tts:http@localhost:8880", got)
	}
	if got.Parent() != provider.KeyTTSHTTP {
		t.Errorf("parent = %q, want %q", got.Parent(), provider.KeyTTSHTTP)
	}
}

func TestSTTKeyForSkipsBrowserOnly(t *testing.T) {
	if _, ok := media.STTKeyFor(config.STTConfig{Type: "web-speech"}); ok {
		t.Error("web-speech must have no key")
	}
	if _, ok := media.STTKeyFor(config.STTConfig{Type: "builtin"}); ok {
		t.Error("the server-side builtin echo STT must have no key")
	}
	got, ok := media.STTKeyFor(config.STTConfig{Type: "http", Endpoint: "http://localhost:8000"})
	if !ok || got != "stt:whisper-http@localhost:8000" {
		t.Errorf("STTKeyFor(http) = %q/%v, want stt:whisper-http@localhost:8000", got, ok)
	}
	if got, ok := media.STTKeyFor(config.STTConfig{Type: "cli", Command: "whisper-cli"}); !ok || got != "stt:whisper-cli@whisper-cli" {
		t.Errorf("STTKeyFor(cli) = %q/%v", got, ok)
	}
}

func TestImageKeyFor(t *testing.T) {
	tests := []struct {
		cfg  config.ImageConfig
		want provider.Key
		ok   bool
	}{
		{config.ImageConfig{Type: "gemini"}, provider.KeyImageGemini, true},
		{config.ImageConfig{Type: "builtin", BuiltinName: "procedural-art"}, provider.KeyImageProceduralArt, true},
		{config.ImageConfig{Type: "http", Endpoint: "http://127.0.0.1:8188"}, "image:http@127.0.0.1:8188", true},
		{config.ImageConfig{Type: "disabled"}, "", false},
	}
	for _, tc := range tests {
		got, ok := media.ImageKeyFor(tc.cfg)
		if ok != tc.ok || (tc.want != "" && got != tc.want) {
			t.Errorf("ImageKeyFor(%+v) = %q/%v, want %q/%v", tc.cfg, got, ok, tc.want, tc.ok)
		}
	}
}
```

```go
// pkg/harness/key_test.go
package harness_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestKeyFor(t *testing.T) {
	tests := []struct {
		cfg  harness.ProviderConfig
		want provider.Key
		ok   bool
	}{
		{harness.ProviderConfig{Type: "http", Endpoint: "http://localhost:11434/v1"}, "llm:openaichat@localhost:11434", true},
		{harness.ProviderConfig{Type: "gemini"}, provider.KeyLLMGemini, true},
		{harness.ProviderConfig{Type: "builtin", BuiltinName: "narrative-oracle"}, provider.KeyLLMNarrativeOracle, true},
		{harness.ProviderConfig{Type: "cli", Command: "claude"}, "llm:cli@claude", true},
		{harness.ProviderConfig{Type: "disabled"}, "", false},
	}
	for _, tc := range tests {
		got, ok := harness.KeyFor(tc.cfg)
		if ok != tc.ok || (tc.want != "" && got != tc.want) {
			t.Errorf("KeyFor(%+v) = %q/%v, want %q/%v", tc.cfg, got, ok, tc.want, tc.ok)
		}
	}
}
```

```go
// pkg/embeddings/key_test.go
package embeddings_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/embeddings"
)

func TestKeyFor(t *testing.T) {
	cfg := config.EmbeddingsConfig{
		Enabled:  true,
		Provider: "gemini",
		Providers: map[string]config.EmbeddingProviderConfig{
			"gemini": {Type: "gemini"},
		},
	}
	got, ok := embeddings.KeyFor(cfg)
	if !ok || got != "embedding:gemini@default" {
		t.Errorf("KeyFor(gemini) = %q/%v, want embedding:gemini@default", got, ok)
	}

	cfg.Providers["openai"] = config.EmbeddingProviderConfig{Type: "http", Endpoint: "https://api.openai.com/v1"}
	cfg.Provider = "openai"
	got, ok = embeddings.KeyFor(cfg)
	if !ok || got != "embedding:openai@api.openai.com" {
		t.Errorf("KeyFor(openai) = %q/%v, want embedding:openai@api.openai.com", got, ok)
	}

	if _, ok := embeddings.KeyFor(config.EmbeddingsConfig{Provider: "disabled"}); ok {
		t.Error("disabled embeddings must have no key")
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/harness/ ./pkg/media/ ./pkg/embeddings/ -run TestKeyFor -v`
Expected: FAIL — `undefined: KeyFor`.

- [x] **Step 3: Implement `harness.KeyFor`**

```go
// pkg/harness/exports.go — replace ProviderIDFor's body with a key resolver.
import "github.com/darkliquid/localrpg/pkg/provider"

// KeyFor maps a role configuration to the canonical provider key a facade
// should build. ok is false when the configuration has no registry provider (a
// disabled role, or the debug echo fallback).
func KeyFor(cfg ProviderConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "http":
		return provider.InstanceOrSelf(provider.KeyLLMOpenAIChat, provider.HostDiscriminator(cfg.Endpoint)), true
	case "cli":
		return provider.InstanceOrSelf(provider.KeyLLMCLI, provider.CommandDiscriminator(cfg.Command)), true
	case "gemini":
		return provider.KeyLLMGemini, true
	case "builtin", "mock", "":
		switch cfg.BuiltinName {
		case "gemini":
			return provider.KeyLLMGemini, true
		case "narrative-oracle":
			return provider.KeyLLMNarrativeOracle, true
		}
		if cfg.Command != "" {
			return provider.InstanceOrSelf(provider.KeyLLMCLI, provider.CommandDiscriminator(cfg.Command)), true
		}
		return "", false
	default:
		return "", false
	}
}
```

- [x] **Step 4: Implement the media resolvers and delete `ProviderKey`**

```go
// pkg/media/exports.go — add, above the builders.
// TTSKeyFor maps a TTS configuration to its canonical key.
func TTSKeyFor(cfg config.TTSConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "gemini":
		return provider.KeyTTSGemini, true
	case "builtin":
		switch cfg.BuiltinName {
		case "gemini":
			return provider.KeyTTSGemini, true
		case "sherpa-onnx", "kokoro":
			return provider.KeyTTSSherpaONNX, true
		case "native-os":
			return provider.KeyTTSNativeOS, true
		case "elevenlabs":
			return provider.KeyTTSElevenLabs, true
		}
		return "", false
	case "cli":
		return provider.InstanceOrSelf(provider.KeyTTSPiper, provider.CommandDiscriminator(cfg.Command)), true
	case "http":
		return provider.InstanceOrSelf(provider.KeyTTSHTTP, provider.HostDiscriminator(cfg.Endpoint)), true
	}
	return "", false
}

// STTKeyFor maps an STT configuration to its canonical key. Browser-only values
// have no key: they never reach the server-side factory.
func STTKeyFor(cfg config.STTConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "http":
		return provider.InstanceOrSelf(provider.KeySTTWhisperHTTP, provider.HostDiscriminator(cfg.Endpoint)), true
	case "cli":
		return provider.InstanceOrSelf(provider.KeySTTWhisperCLI, provider.CommandDiscriminator(cfg.Command)), true
	}
	return "", false
}

// ImageKeyFor maps an image configuration to its canonical key.
func ImageKeyFor(cfg config.ImageConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "gemini":
		return provider.KeyImageGemini, true
	case "builtin":
		if cfg.BuiltinName == "procedural-art" {
			return provider.KeyImageProceduralArt, true
		}
		return "", false
	case "cli":
		return provider.InstanceOrSelf(provider.KeyImageCLI, provider.CommandDiscriminator(cfg.Command)), true
	case "comfyui", "http":
		return provider.InstanceOrSelf(provider.KeyImageHTTP, provider.HostDiscriminator(cfg.Endpoint)), true
	}
	return "", false
}
```

Keep `media.ProviderKey` as the human- and cache-facing name and make it the
canonical instance key: it is used to filter voice profiles
(`pkg/gui/service.go:1255,2361`) and to name the catalog snapshot
(`pkg/gui/tts_inspect.go:30`), which spec §5.8 requires to be instance-scoped.
Rewrite it as a delegation, and delete the now-unused `endpointHost`:

```go
// pkg/media/catalog.go — replace the switch body.
func ProviderKey(cfg config.TTSConfig) string {
	key, ok := TTSKeyFor(cfg)
	if !ok {
		return "disabled"
	}
	return string(key)
}
```

`sanitiseKey` stays if other catalog code still uses it; delete it if the
`grep -rn "sanitiseKey" pkg/media` result becomes empty. `endpointHost` moves
into `provider.HostDiscriminator` and is removed here.

- [x] **Step 5: Implement `embeddings.KeyFor`**

```go
// pkg/embeddings/factory.go — add.
// KeyFor maps an embeddings configuration to its canonical key. The
// discriminator is the endpoint host, or the literal "default" when no endpoint
// is named.
func KeyFor(cfg config.EmbeddingsConfig) (provider.Key, bool) {
	if !cfg.Enabled || cfg.Provider == "" || cfg.Provider == "disabled" {
		return "", false
	}
	pCfg, ok := cfg.Providers[cfg.Provider]
	if !ok {
		return provider.InstanceOrSelf(provider.KeyEmbeddingBuiltin, "default"), true
	}
	switch pCfg.Type {
	case "", "builtin":
		return provider.InstanceOrSelf(provider.KeyEmbeddingBuiltin, "default"), true
	case "gemini":
		return provider.InstanceOrSelf(provider.KeyEmbeddingGemini, "default"), true
	case "http":
		endpoint := pCfg.URL
		if endpoint == "" {
			endpoint = pCfg.Endpoint
		}
		disc := provider.HostDiscriminator(endpoint)
		if disc == "" {
			disc = "default"
		}
		return provider.InstanceOrSelf(provider.KeyEmbeddingOpenAI, disc), true
	}
	return "", false
}
```

- [x] **Step 6: Keep the old wrappers for one commit**

`harness.ProviderIDFor`, `media.TTSProviderIDFor`, `STTProviderIDFor`, and `ImageProviderIDFor` must keep working while consumers are still on them:

```go
// pkg/harness/exports.go
func ProviderIDFor(cfg ProviderConfig) string {
	key, ok := KeyFor(cfg)
	if !ok {
		return ""
	}
	return string(key.Parent())
}
```

`media.ImageProviderIDFor` previously accepted `comfyui` as well as `http`; the new `ImageKeyFor` preserves that. `stt-webspeech` returning `""` from `STTProviderIDFor` matches today's behaviour where the inline switch already failed for it.

- [x] **Step 7: Run the tests and the suite**

Run: `go test ./pkg/harness/ ./pkg/media/ ./pkg/embeddings/ -v`
Expected: PASS
Run: `mise run test:backend`
Expected: PASS except tests asserting old IDs; update those assertions to the canonical keys in this step (search `grep -rn 'tts-\|image-\|openaichat\|narrative-oracle\|openai-embedding\|gemini-embedding' pkg --include=*_test.go`).

- [x] **Step 8: Commit**

```bash
git add pkg/harness pkg/media pkg/embeddings
git commit -m "feat(provider): resolve config to canonical keys"
```

---

### Task 4: Record usage under the resolved key

**Files:**
- Modify: `pkg/harness/router.go` (add `roleKeys`, stamp in `recordUsage`)
- Modify: `pkg/harness/factory.go:140-185` (assign keys), `:200-230` (extractor key)
- Modify: `pkg/harness/extractor.go:40-65,340-350`
- Modify: `pkg/gui/service.go:1806-1818` (TTS), `:2990-2998` (STT)
- Modify: `pkg/gui/image_generation.go:66-88`
- Modify: `pkg/gui/usage.go:150-180` (`mediaUsage`)
- Modify: `pkg/provider/openaichat/http.go:350-360`, `pkg/provider/geminillm/provider.go:594-604`
- Test: `pkg/harness/router_key_test.go`, `pkg/gui/provider_key_test.go`

**Interfaces:**
- Consumes: `KeyFor`, `TTSKeyFor`, `STTKeyFor`, `ImageKeyFor` (Task 3).
- Produces: `Router.AssignRoleKey(role string, key provider.Key)`; `Extractor.SetProviderKey(key provider.Key)`; `mediaUsage(harness.Usage, provider.Key, string) harness.Usage`.

- [x] **Step 1: Write the failing test**

```go
// pkg/harness/router_key_test.go
package harness_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

type keyRecordingProvider struct{ id string }

func (p *keyRecordingProvider) ID() string { return p.id }
func (p *keyRecordingProvider) Generate(context.Context, harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: "ok", Usage: &harness.Usage{InputTokens: 3}}, nil
}
func (p *keyRecordingProvider) Stream(context.Context, harness.GenerateRequest, chan<- harness.StreamChunk) error {
	return nil
}

type keySink struct{ got []harness.Usage }

func (s *keySink) RecordUsage(_ string, u harness.Usage) { s.got = append(s.got, u) }

func TestRouterStampsTheResolvedKey(t *testing.T) {
	router := harness.NewRouter()
	router.RegisterProvider(&keyRecordingProvider{id: "gm"})
	router.AssignRole("gm", "gm")
	router.AssignRoleKey("gm", "llm:openaichat@localhost:11434")

	sink := &keySink{}
	router.SetUsageRecorder(sink)
	if _, err := router.GenerateForRole(context.Background(), "gm", harness.GenerateRequest{}); err != nil {
		t.Fatalf("GenerateForRole: %v", err)
	}
	if len(sink.got) != 1 {
		t.Fatalf("recorded %d usages, want 1", len(sink.got))
	}
	if sink.got[0].Provider != "llm:openaichat@localhost:11434" {
		t.Errorf("provider = %q, want the resolved key", sink.got[0].Provider)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/ -run TestRouterStampsTheResolvedKey -v`
Expected: FAIL — `router.AssignRoleKey undefined`.

- [x] **Step 3: Add the role-key map and stamp it**

```go
// pkg/harness/router.go
type Router struct {
	mu        sync.RWMutex
	providers map[string]ModelProvider
	roleMap   map[string]string     // role -> providerID
	roleKeys  map[string]provider.Key
	fallbacks map[string]string
	recorder  UsageRecorder
}

// AssignRoleKey records the canonical key a role's provider reports usage
// under, so adapters never name themselves.
func (r *Router) AssignRoleKey(role string, key provider.Key) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roleKeys[role] = key
}

// recordUsage reports a provider's usage to the recorder, stamped with the
// role's canonical key.
func (r *Router) recordUsage(role string, u *Usage) {
	if u == nil {
		return
	}
	r.mu.RLock()
	rec := r.recorder
	key := r.roleKeys[role]
	r.mu.RUnlock()
	if rec == nil {
		return
	}
	if key != "" {
		u.Provider = string(key)
	}
	rec.RecordUsage(role, *u)
}
```

Initialise `roleKeys: make(map[string]provider.Key)` in `NewRouter`, and add the `provider` import to `pkg/harness/router.go`.

- [x] **Step 4: Assign keys in `RouterFromConfigWithLogger`**

Inside the `for role, roleCfg := range cfg.Agents.Roles` loop, after a successful build:

```go
if key, ok := KeyFor(ProviderConfig{
	Type:        roleCfg.Type,
	BuiltinName: roleCfg.BuiltinName,
	Command:     roleCfg.Command,
	Endpoint:    roleCfg.Endpoint,
}); ok {
	router.AssignRoleKey(role, key)
}
```

- [x] **Step 5: Delete the adapters' hardcoded `Provider`**

`pkg/provider/openaichat/http.go` around the final-chunk usage: remove the `Provider: "openaichat",` line. `pkg/provider/geminillm/provider.go`: remove `Provider: "gemini",`. Leave `Model`, tokens, and `Estimated` untouched. The recorder now stamps the key.

- [x] **Step 6: Point media recording at the resolvers**

```go
// pkg/gui/usage.go
func mediaUsage(u media.Usage, key provider.Key, model string) harness.Usage {
	return harness.Usage{
		Provider:     string(key),
		Model:        model,
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		Characters:   u.Characters,
		Requests:     u.Requests,
		Estimated:    u.Estimated,
	}
}
```

```go
// pkg/gui/service.go — TTS recording
if key, ok := media.TTSKeyFor(cfg.Media.TTS); ok {
	if u := pipeline.LastUsage(); u.Characters != 0 || u.InputTokens != 0 || u.OutputTokens != 0 || u.Requests != 0 {
		s.RecordUsage(gameID, turnNumber, "tts", mediaUsage(u, key, cfg.Media.TTS.Model))
	}
}
```

```go
// pkg/gui/service.go — STT recording (test path)
if key, ok := media.STTKeyFor(sttCfg); ok {
	if reporter, ok := client.(media.UsageReporter); ok {
		s.RecordUsageGlobal("stt", mediaUsage(reporter.LastUsage(), key, sttCfg.Model))
	}
}
```

```go
// pkg/gui/image_generation.go — replace the provider := ... derivation
key, hasKey := media.ImageKeyFor(cfg.Media.Image)
if hasKey {
	s.recordImageUsage(scope, key, cfg.Media.Image.Model, client)
}
```

Change `recordImageUsage`'s signature to `(scope usageScope, key provider.Key, model string, client media.ImageClient)` and call `mediaUsage(reporter.LastUsage(), key, model)`.

- [x] **Step 7: Assign the extractor key**

`Extractor` gains a key field and a setter:

```go
// pkg/harness/extractor.go
type Extractor struct {
	provider ModelProvider
	recorder UsageRecorder
	key      provider.Key
	// ...
}

func (e *Extractor) SetProviderKey(key provider.Key) { e.key = key }
```

In the extraction usage record (`pkg/harness/extractor.go:345`):

```go
if res != nil && res.Usage != nil && e.recorder != nil {
	if e.key != "" {
		res.Usage.Provider = string(e.key)
	}
	e.recorder.RecordUsage("extractor", *res.Usage)
}
```

In `ExtractorFromConfigWithLogger`, after resolving the provider, assign the source role's key:

```go
roleKey, _ := KeyFor(ProviderConfig{
	Type:        roleCfg.Type,
	BuiltinName: roleCfg.BuiltinName,
	Command:     roleCfg.Command,
	Endpoint:    roleCfg.Endpoint,
})
extractor.SetProviderKey(roleKey)
```

For the `.Type == "inherit"` path, resolve the source role's config from `cfg.Agents.Roles[source]` and call `KeyFor` on it.

- [x] **Step 8: Write the gui agreement test**

```go
// pkg/gui/provider_key_test.go
package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestLedgerAndLimitKeysAgreeForMedia(t *testing.T) {
	tts := config.TTSConfig{Type: "http", Endpoint: "http://localhost:8880"}
	ledgerKey, ok := media.TTSKeyFor(tts)
	if !ok {
		t.Fatal("expected a TTS key")
	}
	limitKey, ok := media.TTSKeyFor(tts)
	if !ok || limitKey != ledgerKey {
		t.Errorf("limit key %q != ledger key %q", limitKey, ledgerKey)
	}
}
```

- [x] **Step 9: Run the tests and the suite**

Run: `go test ./pkg/harness/ ./pkg/gui/ -v`
Expected: PASS
Run: `mise run test:backend`
Expected: PASS

- [x] **Step 10: Commit**

```bash
git add pkg/harness pkg/gui pkg/provider/openaichat pkg/provider/geminillm
git commit -m "feat(usage): record spend under the canonical provider key"
```

---

### Task 5: Pricing, config version, and validation

**Files:**
- Modify: `pkg/pricing/pricing.go:19-70`
- Modify: `pkg/config/types.go:274-285` (version), new `Validate`
- Modify: `pkg/config/manager.go:73-97` (`Load` collects warnings)
- Test: `pkg/pricing/pricing_test.go`, `pkg/config/validate_test.go`

**Interfaces:**
- Consumes: `provider.ParseKey`, `provider.Key.Parent()`, key constants.
- Produces: `pricing.Resolve(string, string, *config.Config) Price` with fallback; `config.Config.Validate() []string`; `config.Manager.Warnings() []string`.

- [x] **Step 1: Write the failing tests**

```go
// append to pkg/pricing/pricing_test.go
func TestResolveFallsBackFromInstanceToAdapter(t *testing.T) {
	cfg := &config.Config{Providers: config.ProvidersConfig{Prices: []config.PriceConfig{
		{Provider: "tts:http", PerRequest: 10},
		{Provider: "tts:http@host-a", PerRequest: 99},
	}}}
	if got := Resolve("tts:http@host-a", "", cfg); got.PerRequest != 99 {
		t.Errorf("instance price = %d, want 99", got.PerRequest)
	}
	if got := Resolve("tts:http@host-b", "", cfg); got.PerRequest != 10 {
		t.Errorf("fallback price = %d, want 10", got.PerRequest)
	}
}

func TestResolvePrefersModelOverProviderWide(t *testing.T) {
	cfg := &config.Config{Providers: config.ProvidersConfig{Prices: []config.PriceConfig{
		{Provider: "llm:openaichat", PerMillionInput: 1},
		{Provider: "llm:openaichat", Model: "gpt-4o-mini", PerMillionInput: 2},
	}}}
	if got := Resolve("llm:openaichat@host", "gpt-4o-mini", cfg); got.PerMillionInput != 2 {
		t.Errorf("model price = %d, want 2", got.PerMillionInput)
	}
	if got := Resolve("llm:openaichat@host", "other", cfg); got.PerMillionInput != 1 {
		t.Errorf("provider-wide price = %d, want 1", got.PerMillionInput)
	}
}
```

```go
// pkg/config/validate_test.go
package config_test

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestValidateRejectsLegacyPriceKey(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.Prices = []config.PriceConfig{{Provider: "openaichat", PerMillionInput: 1}}
	problems := cfg.Validate()
	if len(problems) == 0 {
		t.Fatal("expected a problem for the legacy key")
	}
	joined := strings.Join(problems, "\n")
	if !strings.Contains(joined, "llm:openaichat") {
		t.Errorf("message should show the canonical key, got: %s", joined)
	}
}

func TestValidateAcceptsCanonicalKeys(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.Prices = []config.PriceConfig{
		{Provider: "llm:gemini", PerMillionInput: 1},
		{Provider: "tts:http@localhost:8880", PerRequest: 2},
	}
	if problems := cfg.Validate(); len(problems) != 0 {
		t.Errorf("unexpected problems: %v", problems)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/pricing/ ./pkg/config/ -run 'TestResolve|TestValidate' -v`
Expected: FAIL — `cfg.Validate undefined` and fallback assertions.

- [x] **Step 3: Add candidate-list resolution and rekey built-ins**

```go
// pkg/pricing/pricing.go
import "github.com/darkliquid/localrpg/pkg/provider"

// BuiltinPrices are the known rates for adapters LocalRPG ships presets for.
var BuiltinPrices = []config.PriceConfig{
	{Provider: string(provider.KeyLLMGemini), PerMillionInput: 125_000, PerMillionOutput: 500_000},
	{Provider: string(provider.KeyLLMOpenAIChat), PerMillionInput: 150_000, PerMillionOutput: 600_000},
}

// Resolve finds the price for a usage key and model, most specific first: the
// exact key and model, the exact key, the parent adapter and model, then the
// parent adapter. Configured entries win over the built-in table at every step.
func Resolve(key, model string, cfg *config.Config) Price {
	candidates := candidateKeys(key)
	if cfg != nil {
		for _, candidate := range candidates {
			if p, ok := lookup(cfg.Providers.Prices, candidate, model); ok {
				return p
			}
		}
	}
	for _, candidate := range candidates {
		if p, ok := lookup(BuiltinPrices, candidate, model); ok {
			return p
		}
	}
	return Price{}
}

// candidateKeys is the lookup ladder for a recorded key, most specific first.
func candidateKeys(key string) []string {
	parsed, err := provider.ParseKey(key)
	if err != nil {
		return []string{key}
	}
	if _, ok := parsed.Instance(); ok {
		return []string{string(parsed), string(parsed.Parent())}
	}
	return []string{string(parsed)}
}
```

- [x] **Step 4: Add `Config.Validate` and bump the version**

```go
// pkg/config/types.go
// Version is the config schema version. Version "2" introduced canonical
// provider keys, so providers.prices entries are validated against the grammar.
const CurrentVersion = "2"

// Validate reports human-readable problems with the configuration. It never
// fails the load: a bad entry is reported so the UI can surface it while the
// rest of the configuration keeps working.
func (c *Config) Validate() []string {
	var problems []string
	if c.Version != CurrentVersion {
		problems = append(problems, fmt.Sprintf(
			"config version %q predates canonical provider keys; providers.prices must use <family>:<adapter> (for example %s)",
			c.Version, provider.KeyLLMOpenAIChat))
	}
	for i, price := range c.Providers.Prices {
		if price.Provider == "" {
			problems = append(problems, fmt.Sprintf("providers.prices[%d]: provider is required", i))
			continue
		}
		if _, err := provider.ParseKey(price.Provider); err != nil {
			problems = append(problems, fmt.Sprintf(
				"providers.prices[%d].provider %q is not a canonical key (for example %s, %s, %s): %v",
				i, price.Provider, provider.KeyLLMOpenAIChat, provider.KeyLLMGemini, provider.KeyTTSHTTP, err))
		}
	}
	return problems
}
```

Add `fmt` and the `provider` import to `pkg/config/types.go`, and set `Version: CurrentVersion` in `DefaultConfig` (`pkg/config/types.go:319`).

- [x] **Step 5: Collect warnings in `ConfigManager.Load`**

```go
// pkg/config/manager.go
type ConfigManager struct {
	// existing fields...
	warnings []string
}

// Warnings returns the problems found during the last Load.
func (m *ConfigManager) Warnings() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.warnings...)
}
```

At the end of `Load`, before `m.activeConfig = merged`:

```go
m.warnings = merged.Validate()
if len(m.warnings) > 0 && m.logger != nil {
	for _, problem := range m.warnings {
		m.logger.Event("config.problem", map[string]interface{}{"problem": problem})
	}
}
```

If `ConfigManager` has no logger, add `func (m *ConfigManager) SetLogger(logger trace.Logger)` and wire it where the manager is constructed in `pkg/gui`; otherwise leave the warnings accessible via `Warnings()` and log them from `gui.NewService`.

- [x] **Step 6: Surface the warnings in the GUI log**

In `pkg/gui` where the config manager is created and first loaded, after `Load`:

```go
for _, problem := range s.configMgr.Warnings() {
	s.logger.Event("config.problem", map[string]interface{}{"problem": problem})
}
```

- [x] **Step 7: Run the tests and the suite**

Run: `go test ./pkg/pricing/ ./pkg/config/ ./pkg/gui/ -v`
Expected: PASS
Run: `mise run test:backend`
Expected: PASS. Tests asserting `BuiltinPrices` keys, and any test using an old
literal such as `Provider: "gemini"` (`pkg/gui/usage_record_test.go:13`), must be
updated to the canonical keys in this step.

- [x] **Step 8: Commit**

```bash
git add pkg/pricing pkg/config pkg/gui
git commit -m "feat(pricing): resolve prices by canonical key with adapter fallback"
```

---

### Task 6: Limits and funds use the resolver

**Files:**
- Modify: `pkg/gui/limits.go:13-70` (delete derivations)
- Test: `pkg/gui/provider_key_test.go`

**Interfaces:**
- Consumes: `harness.KeyFor`, `media.TTSKeyFor`/`STTKeyFor`/`ImageKeyFor`.
- Produces: `Service.providerKeyForRole(role string) provider.Key` (same name, new return type and body).

- [x] **Step 1: Write the failing test**

```go
// append to pkg/gui/provider_key_test.go
type failingKeyProvider struct{ id string }

func TestLimitKeyMatchesLedgerKeyForLLM(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.configMgr.Get().Agents.Roles["gm"] = config.AgentRoleConfig{
		Type:     "http",
		Endpoint: "http://localhost:11434/v1",
	}

	ledger, ok := harness.KeyFor(harness.ProviderConfig{Type: "http", Endpoint: "http://localhost:11434/v1"})
	if !ok {
		t.Fatal("expected an LLM key")
	}
	if got := svc.providerKeyForRole("gm"); got != ledger {
		t.Errorf("limit key %q != ledger key %q", got, ledger)
	}
}
```

`NewService(t.TempDir())` and direct mutation of `svc.configMgr.Get()` are the
existing pattern in this package (`pkg/gui/usage_record_test.go:12`,
`pkg/gui/audio_pipeline_test.go:20`).

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestLimitKeyMatchesLedgerKeyForLLM -v`
Expected: FAIL — `providerKeyForRole` returns the old string, or the helper is missing.

- [x] **Step 3: Replace the derivations**

```go
// pkg/gui/limits.go
// providerKeyForRole resolves the canonical key a role's traffic belongs to, so
// a backoff follows the provider and is lifted by a configuration change.
func (s *Service) providerKeyForRole(role string) provider.Key {
	cfg := s.configMgr.Get()
	switch role {
	case "tts":
		key, _ := media.TTSKeyFor(cfg.Media.TTS)
		return key
	case "stt":
		key, _ := media.STTKeyFor(cfg.Media.STT)
		return key
	case "image":
		key, _ := media.ImageKeyFor(cfg.Media.Image)
		return key
	default:
		return harnessKeyForRole(cfg, role)
	}
}

// harnessKeyForRole resolves the LLM key a role uses, following an inherit
// chain so a backoff lands on the provider that actually serves it.
func harnessKeyForRole(cfg *config.Config, role string) provider.Key {
	if cfg == nil {
		return ""
	}
	roleCfg, ok := cfg.Agents.Roles[role]
	if !ok {
		return ""
	}
	seen := map[string]bool{role: true}
	for roleCfg.Type == "inherit" && roleCfg.InheritFrom != "" && !seen[roleCfg.InheritFrom] {
		seen[roleCfg.InheritFrom] = true
		next, ok := cfg.Agents.Roles[roleCfg.InheritFrom]
		if !ok {
			break
		}
		roleCfg = next
	}
	key, ok := harness.KeyFor(harness.ProviderConfig{
		Type:        roleCfg.Type,
		BuiltinName: roleCfg.BuiltinName,
		Command:     roleCfg.Command,
		Endpoint:    roleCfg.Endpoint,
	})
	if !ok {
		return ""
	}
	return key
}
```

Delete `roleProviderKey`. Update `guardRole` and `noteFailure` to pass `string(key)` into the registry:

```go
key := string(s.providerKeyForRole(role))
if until, ok := s.limits.Blocked(key, role); ok {
	return &harness.ErrRateLimitedUntil{Provider: key, Role: role, Until: until}
}
```

Do this for both `guardRole` and `noteFailure` and any other `providerKeyForRole` caller.

- [x] **Step 4: Run the tests and the suite**

Run: `go test ./pkg/gui/ -v`
Expected: PASS
Run: `mise run test:backend`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/gui/limits.go pkg/gui/provider_key_test.go
git commit -m "fix(limits): key backoffs by canonical provider key"
```

---

### Task 7: Failure attempts carry the canonical key

**Files:**
- Modify: `pkg/harness/router.go:130-200` (attempt provider)
- Modify: `pkg/harness/failure.go:29-35` (doc comment)
- Modify: `pkg/gui/generation_attempts.go:30-75`
- Test: `pkg/harness/router_key_test.go`

**Interfaces:**
- Consumes: `RoleKey` from Task 4.
- Produces: `Router.ProviderKeyForRole(role string) (provider.Key, bool)`.

- [x] **Step 1: Write the failing test**

```go
// append to pkg/harness/router_key_test.go
func TestFailureAttemptUsesTheCanonicalKey(t *testing.T) {
	router := harness.NewRouter()
	router.RegisterProvider(&failingKeyProvider{id: "gm"})
	router.AssignRole("gm", "gm")
	router.AssignRoleKey("gm", "llm:gemini")

	_, err := router.GenerateForRole(context.Background(), "gm", harness.GenerateRequest{})
	failure, ok := harness.FailureFrom(err)
	if !ok {
		t.Fatalf("want a GenerationFailure, got %v", err)
	}
	if len(failure.Attempts) == 0 || failure.Attempts[0].Provider != "llm:gemini" {
		t.Fatalf("attempt provider = %+v, want llm:gemini", failure.Attempts)
	}
}

type failingKeyProvider struct{ id string }

func (p *failingKeyProvider) ID() string { return p.id }
func (p *failingKeyProvider) Generate(context.Context, harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return nil, context.DeadlineExceeded
}
func (p *failingKeyProvider) Stream(context.Context, harness.GenerateRequest, chan<- harness.StreamChunk) error {
	return nil
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/ -run TestFailureAttemptUsesTheCanonicalKey -v`
Expected: FAIL — attempt provider is `gm`.

- [x] **Step 3: Add the accessor and use it**

```go
// pkg/harness/router.go
// ProviderKeyForRole reports the canonical key a role's provider records under.
func (r *Router) ProviderKeyForRole(role string) (provider.Key, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, ok := r.roleKeys[role]
	return key, ok
}
```

In `GenerateForRole` and the streaming path, replace `primary.ID()` / `fallback.ID()` in attempt construction with the role key, falling back to the provider ID when no key is set:

```go
func (r *Router) attemptProvider(role, providerID string) string {
	if key, ok := r.ProviderKeyForRole(role); ok && key != "" {
		return string(key)
	}
	return providerID
}
```

Call sites become `Provider: r.attemptProvider(role, primary.ID())` and `Provider: r.attemptProvider(role, fallback.ID())`.

Update the `Attempt.Provider` doc comment in `pkg/harness/failure.go` to: "Provider is the canonical provider key the attempt ran against."

In `pkg/gui/generation_attempts.go`, replace `router.ProviderIDForRole(role)` with the key:

```go
providerKey, _ := router.ProviderKeyForRole(role)
// use string(providerKey) for the Provider field
```

- [x] **Step 4: Run the tests and the suite**

Run: `go test ./pkg/harness/ ./pkg/gui/ -v`
Expected: PASS
Run: `mise run test:backend`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness pkg/gui/generation_attempts.go
git commit -m "fix(failures): name the attempt by canonical provider key"
```

---

### Task 8: Embedding usage recording

**Files:**
- Modify: `pkg/storage/embedding_worker.go:20-80,180-195`
- Modify: `pkg/tools/tools.go:26-60,120-140,280-300`, `pkg/tools/memory.go:30-50`
- Modify: `pkg/provider/openaiembedding/openai.go:176-186`
- Modify: `pkg/provider/geminiembedding/gemini.go:20-105`
- Create: `pkg/gui/embedding_usage.go`
- Modify: `pkg/gui/service.go:262-276,1310-1316`
- Test: `pkg/storage/embedding_usage_test.go`, `pkg/tools/embedding_usage_test.go`, `pkg/embeddings/key_test.go`

**Interfaces:**
- Consumes: `embeddings.KeyFor` (Task 3), `embeddings.Provider`, and `harness.Usage` inside `pkg/tools` (which already imports `pkg/harness`).
- Produces: `storage.EmbeddingUsage{ProviderKey, Model string; InputTokens, Requests int}`; `storage.EmbeddingUsageFunc func(EmbeddingUsage)`; `(*EmbeddingWorker).SetUsageReporting(fn EmbeddingUsageFunc, meta EmbeddingUsage)`; `tools.EmbeddingUsageFunc func(providerKey, model string, inputTokens, requests int)`; `(*Executor).SetEmbeddingUsage(fn tools.EmbeddingUsageFunc, providerKey, model string)`; `(*Service).RecordEmbeddingUsage(providerKey, model string, inputTokens, requests int)`.

`pkg/storage` must not import `pkg/harness`, so the worker takes a callback and a
small local struct rather than a `harness.Usage`. The caller (`pkg/gui`)
supplies the key and model; the worker supplies the request count.

- [x] **Step 1: Write the failing tests**

```go
// pkg/storage/embedding_usage_test.go
package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/storage"
)

type recordingEmbedder struct{}

func (recordingEmbedder) ID() string      { return "recording" }
func (recordingEmbedder) Dimensions() int { return 2 }
func (recordingEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{1, 0}}, nil
}

type usageSpy struct{ calls []storage.EmbeddingUsage }

func TestEmbeddingWorkerReportsUsage(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStore(dir + "/index.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	spy := &usageSpy{}
	worker := storage.NewEmbeddingWorker(store, recordingEmbedder{}, storage.EmbeddingWorkerOptions{BatchSize: 1})
	worker.SetUsageReporting(
		func(u storage.EmbeddingUsage) { spy.calls = append(spy.calls, u) },
		storage.EmbeddingUsage{ProviderKey: "embedding:gemini@default", Model: "text-embedding-004"},
	)
	worker.Start()
	defer worker.Stop()

	worker.Enqueue(storage.EmbeddingItem{TargetType: "entity", TargetID: "x", Text: "hello world"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(spy.calls) == 0 {
		t.Fatal("expected a usage record after a batch")
	}
	if spy.calls[0].ProviderKey != "embedding:gemini@default" {
		t.Errorf("provider key = %q", spy.calls[0].ProviderKey)
	}
	if spy.calls[0].Requests != 1 {
		t.Errorf("requests = %d, want 1", spy.calls[0].Requests)
	}
}
```

```go
// pkg/tools/embedding_usage_test.go
package tools_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/tools"
)

func TestEmbeddingUsageFuncIsCalled(t *testing.T) {
	var gotKey, gotModel string
	var gotRequests int

	exec := tools.NewExecutor(nil, 4000)
	exec.SetEmbeddingUsage(func(providerKey, model string, _ int, requests int) {
		gotKey, gotModel, gotRequests = providerKey, model, requests
	}, "embedding:gemini@default", "text-embedding-004")

	// The provider is nil, so the executor must not call the sink; this asserts
	// the no-op path first. The positive path is covered through the gui
	// integration in TestEmbeddingUsageIsRecorded.
	_, _ = exec.Execute(context.Background(), toolsToolCall())
	if gotKey != "" {
		t.Errorf("sink ran with no provider: %q", gotKey)
	}
	_ = gotModel
	_ = gotRequests
}
```

Add the small helper next to it:

```go
func toolsToolCall() harness.ToolCall {
	return harness.ToolCall{Name: "search_entities", Arguments: `{"query":"dragon"}`}
}
```

Import `harness` in that test file. The positive path (a provider that reports usage, the sink fires with `requests == 1` and the resolved key) is asserted by `TestEmbeddingUsageIsRecorded` in `pkg/gui` (Task 8 Step 7), where a `Service` owns the ledger.

- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/storage/ ./pkg/tools/ -run 'TestEmbeddingWorkerReportsUsage|TestEmbeddingUsageFuncIsCalled' -v`
Expected: FAIL — `SetUsageReporting` and `SetEmbeddingUsage` undefined.

- [x] **Step 3: Implement the worker hook**

```go
// pkg/storage/embedding_worker.go

// EmbeddingUsage is one embedding call's consumption, reported after a batch.
type EmbeddingUsage struct {
	ProviderKey string
	Model       string
	InputTokens int
	Requests    int
}

// EmbeddingUsageFunc receives a usage record for each completed batch.
type EmbeddingUsageFunc func(u EmbeddingUsage)
```

Add `usageFn EmbeddingUsageFunc` and `usageMeta EmbeddingUsage` to `EmbeddingWorker`, and:

```go
// SetUsageReporting installs the sink a completed batch reports to, plus the
// static key and model the worker cannot know. It is a no-op when unset.
func (w *EmbeddingWorker) SetUsageReporting(fn EmbeddingUsageFunc, meta EmbeddingUsage) {
	w.usageFn = fn
	w.usageMeta = meta
}
```

After a successful `Embed` and persist in `flush`:

```go
if w.usageFn != nil {
	meta := w.usageMeta
	meta.Requests = 1
	if reporter, ok := w.provider.(interface{ LastUsage() harness.Usage }); ok {
		meta.InputTokens = reporter.LastUsage().InputTokens
	}
	w.usageFn(meta)
}
```

`pkg/storage` cannot import `pkg/harness`, so the provider check uses a local interface with the same method set:

```go
// pkg/storage/embedding_worker.go
type usageReportingProvider interface {
	LastUsage() harnessTokens
}

type harnessTokens struct{ InputTokens int }
```

and the call becomes:

```go
if reporter, ok := w.provider.(usageReportingProvider); ok {
	meta.InputTokens = reporter.LastUsage().InputTokens
}
```

`openaiembedding.Client.LastUsage()` returns `harness.Usage`, which structurally has `InputTokens int`, so it satisfies `usageReportingProvider`. `geminiembedding` gains the same method in Step 6. Providers without it still report `Requests`.

- [x] **Step 4: Implement the tools hook**

```go
// pkg/tools/tools.go
// EmbeddingUsageFunc reports one embedding call's usage to a sink.
type EmbeddingUsageFunc func(providerKey, model string, inputTokens, requests int)

// SetEmbeddingUsage installs the sink an embedding-backed search reports to.
func (e *Executor) SetEmbeddingUsage(fn EmbeddingUsageFunc, providerKey, model string) {
	e.embeddingUsage = fn
	e.embeddingKey = providerKey
	e.embeddingModel = model
}
```

Add `embeddingUsage EmbeddingUsageFunc`, `embeddingKey string`, and `embeddingModel string` to the `Executor` struct. Call the sink immediately after each successful `e.embeddingsProvider.Embed(...)` in `searchEntities`, `searchTimeline`, and `searchMemories`:

```go
if e.embeddingUsage != nil {
	tokens := 0
	if reporter, ok := e.embeddingsProvider.(interface{ LastUsage() harness.Usage }); ok {
		tokens = reporter.LastUsage().InputTokens
	}
	e.embeddingUsage(e.embeddingKey, e.embeddingModel, tokens, 1)
}
```

`pkg/tools` already imports `pkg/harness`, so the assertion needs no local shim.

- [x] **Step 5: Wire the sink in `pkg/gui`**

```go
// pkg/gui/embedding_usage.go
package gui

import "github.com/darkliquid/localrpg/pkg/harness"

// RecordEmbeddingUsage files an embedding call's spend against the shared
// ledger, outside any turn.
func (s *Service) RecordEmbeddingUsage(providerKey, model string, inputTokens, requests int) {
	if providerKey == "" {
		return
	}
	s.RecordUsageGlobal("embedding", harness.Usage{
		Provider:    providerKey,
		Model:       model,
		InputTokens: inputTokens,
		Requests:    requests,
	})
}
```

In `ensureEmbeddingWorker`, after building the worker and computing the key and model:

```go
if key, ok := embeddings.KeyFor(cfg.Embeddings); ok {
	worker.SetUsageReporting(func(u storage.EmbeddingUsage) {
		s.RecordEmbeddingUsage(string(key), cfg.Embeddings.Model, u.InputTokens, u.Requests)
	}, storage.EmbeddingUsage{ProviderKey: string(key), Model: cfg.Embeddings.Model})
}
```

In the tool executor wiring:

```go
if embProvider, err := embeddings.NewProviderFromConfig(cfg.Embeddings); err == nil && embProvider != nil {
	toolExecutor.SetEmbeddingsProvider(embProvider)
	if key, ok := embeddings.KeyFor(cfg.Embeddings); ok {
		toolExecutor.SetEmbeddingUsage(s.RecordEmbeddingUsage, string(key), cfg.Embeddings.Model)
	}
}
```

- [x] **Step 6: Extend `geminiembedding` and fix `openaiembedding`**

```go
// pkg/provider/geminiembedding/gemini.go
// add a mutex-guarded lastUsage field of type harness.Usage, set after Embed:
c.mu.Lock()
if resp.UsageMetadata != nil {
	c.lastUsage = harness.Usage{
		Model:       c.model,
		InputTokens: int(resp.UsageMetadata.PromptTokenCount),
		Requests:    1,
	}
}
c.mu.Unlock()

// LastUsage reports the most recent request's usage.
func (c *Client) LastUsage() harness.Usage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastUsage
}
```

`pkg/provider/openaiembedding/openai.go`: delete `Provider: c.ID()` from the `lastUsage` assignment; leave `Model`, `InputTokens`, `Requests`.

- [x] **Step 7: Run the tests and the suite**

- [x] **Step 7: Add the gui integration test, then run the tests and the suite**

```go
// append to pkg/gui/provider_key_test.go
func TestEmbeddingUsageIsRecorded(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.RecordEmbeddingUsage("embedding:gemini@default", "text-embedding-004", 12, 1)

	rows, err := svc.GlobalUsage()
	if err != nil {
		t.Fatalf("GlobalUsage: %v", err)
	}
	var found bool
	for _, row := range rows.Rows {
		if row.Provider == "embedding:gemini@default" && row.Role == "embedding" {
			found = true
			if row.InputTokens != 12 {
				t.Errorf("input tokens = %d, want 12", row.InputTokens)
			}
		}
	}
	if !found {
		t.Fatalf("no embedding row in %+v", rows.Rows)
	}
}
```

If `GlobalUsage` returns a different DTO shape, use the same accessor that `pkg/gui/usage_api_test.go` uses to read rows.

Run: `go test ./pkg/storage/ ./pkg/tools/ ./pkg/embeddings/ ./pkg/provider/openaiembedding/ ./pkg/provider/geminiembedding/ ./pkg/gui/ -v`
Expected: PASS
Run: `mise run test:backend`
Expected: PASS

- [x] **Step 8: Commit**

```bash
git add pkg/storage pkg/tools pkg/gui pkg/provider/openaiembedding pkg/provider/geminiembedding
git commit -m "feat(usage): record embedding spend under a canonical key"
```

---

### Task 9: Delete the old derivations

**Files:**
- Delete: `harness.ProviderIDFor`, `media.TTSProviderIDFor`, `media.STTProviderIDFor`, `media.ImageProviderIDFor`
- Modify: any remaining callers
- Test: `pkg/provider/noderivation_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1-8.
- Produces: a codebase where no key is derived outside the resolvers.

- [x] **Step 1: Write the failing scan test**

```go
// pkg/provider/noderivation_test.go
package provider_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoSecondKeyDerivation fails when a package outside pkg/provider builds a
// canonical key literal or resurrects the old builtin_name/type derivation.
func TestNoSecondKeyDerivation(t *testing.T) {
	root := "../.."
	keyLiteral := regexp.MustCompile(`"(llm|tts|stt|image|embedding):[a-z0-9-]+`)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(path)
		if strings.Contains(rel, "pkg/provider/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if keyLiteral.Match(data) {
			t.Errorf("%s builds a canonical key literal; use the pkg/provider constants and resolvers", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/provider/ -run TestNoSecondKeyDerivation -v`
Expected: FAIL, naming `pkg/harness/exports.go` and `pkg/media/exports.go`.

- [x] **Step 3: Delete the wrappers**

Remove `ProviderIDFor` (`pkg/harness/exports.go`), `TTSProviderIDFor`, `STTProviderIDFor`, `ImageProviderIDFor` (`pkg/media/exports.go`), and any `media.ProviderKey` remnants. Replace remaining callers with the key resolvers; a caller that only needed an ID uses `string(key.Parent())`.

Find them with `grep -rn "ProviderIDFor\|media.ProviderKey" pkg cmd`.

- [x] **Step 4: Run the tests and the suite**

Run: `go test ./pkg/provider/ -run TestNoSecondKeyDerivation -v`
Expected: PASS
Run: `mise run test`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/provider pkg/harness pkg/media
git commit -m "refactor(provider): delete the duplicate key derivations"
```

---

### Task 10: Documentation and release note

**Files:**
- Modify: `pkg/gui/docs_catalogue_test.go` (`ledgerKey`)
- Modify: `pkg/gui/docs/11-usage-and-pricing.md`
- Create: `docs/releases/provider-keys.md`
- Regenerate: `pkg/gui/docs/12-provider-catalogue.md`, `pkg/gui/docs/13-configuration-reference.md`

**Interfaces:**
- Consumes: `harness.KeyFor`, `media.TTSKeyFor`/`STTKeyFor`/`ImageKeyFor`, `embeddings.KeyFor`.
- Produces: docs whose keys match the runtime.

- [x] **Step 1: Point the generator at the resolvers**

Replace the `ledgerKey`, `ttsConfigFromPreset`, and `presetSummary` helpers in `pkg/gui/docs_catalogue_test.go` with the resolvers. For each descriptor, build its preset config into the family config and call the matching resolver:

```go
// ledgerKey resolves the canonical key a descriptor's first preset records under.
func ledgerKey(family provider.Family, desc provider.Descriptor) string {
	if len(desc.Presets) == 0 {
		return "not reported"
	}
	cfg := desc.Presets[0].Config
	switch family {
	case provider.FamilyLLM:
		pc := harness.ProviderConfig{}
		if v, ok := cfg["type"].(string); ok {
			pc.Type = v
		}
		if v, ok := cfg["builtin_name"].(string); ok {
			pc.BuiltinName = v
		}
		if v, ok := cfg["endpoint"].(string); ok {
			pc.Endpoint = v
		}
		if v, ok := cfg["command"].(string); ok {
			pc.Command = v
		}
		if key, ok := harness.KeyFor(pc); ok {
			return string(key)
		}
	case provider.FamilyTTS:
		if key, ok := media.TTSKeyFor(ttsConfigFromPreset(desc.Presets[0])); ok {
			return string(key)
		}
	case provider.FamilySTT:
		sc := config.STTConfig{}
		if v, ok := cfg["type"].(string); ok {
			sc.Type = v
		}
		if v, ok := cfg["command"].(string); ok {
			sc.Command = v
		}
		if v, ok := cfg["endpoint"].(string); ok {
			sc.Endpoint = v
		}
		if key, ok := media.STTKeyFor(sc); ok {
			return string(key)
		}
	case provider.FamilyImage:
		ic := config.ImageConfig{}
		if v, ok := cfg["type"].(string); ok {
			ic.Type = v
		}
		if v, ok := cfg["builtin_name"].(string); ok {
			ic.BuiltinName = v
		}
		if v, ok := cfg["command"].(string); ok {
			ic.Command = v
		}
		if v, ok := cfg["endpoint"].(string); ok {
			ic.Endpoint = v
		}
		if key, ok := media.ImageKeyFor(ic); ok {
			return string(key)
		}
	}
	return "not reported"
}
```

Delete the `desc.ID == "openaichat" || desc.ID == "gemini"` special case; the resolver decides.

- [x] **Step 2: Regenerate and lint**

Run: `go test ./pkg/gui -update-docs`
Run: `mise run lint:docs`
Expected: catalogue and reference regenerate; markdownlint clean. Inspect `12-provider-catalogue.md` and confirm every "Ledger key" is either a canonical key or `not reported`, and that `TTS providers` shows `tts:http@localhost:8880`.

- [x] **Step 3: Update the pricing article**

In `pkg/gui/docs/11-usage-and-pricing.md`, replace the "Ledger key" rules table with the canonical form and the fallback ladder:

```markdown
The `provider` field is the canonical key the adapter records under, not the name
you gave the block under `providers:`. Keys are `<family>:<adapter>`, and a
provider that names an endpoint or command also has an instance form:

- adapter: `llm:openaichat`, `tts:http`, `image:gemini`
- instance: `tts:http@localhost:8880`, `llm:openaichat@api.openai.com`

A price is matched most specific first: the instance key and model, the instance
key, the adapter key and model, then the adapter key. So a price on `tts:http`
covers every HTTP speech endpoint, and a price on `tts:http@hostA` overrides it
for that endpoint only. The [Provider & Model Catalogue](12-provider-catalogue)
lists every key.
```

- [x] **Step 4: Write the release note**

```markdown
# Provider keys changed to `<family>:<adapter>`

LocalRPG now identifies every provider with one canonical key. `providers.prices`
entries must use the new form; an entry with an old key is reported as a
configuration problem and does not match anything.

Prices are also matched most-specific-first, so an instance key such as
`tts:http@localhost:8880` can override an adapter-wide `tts:http` price.

| Old key | New key |
|---|---|
| `openaichat` | `llm:openaichat` |
| `gemini` (LLM) | `llm:gemini` |
| `gemini:tts` | `tts:gemini` |
| `builtin:elevenlabs` | `tts:elevenlabs` |
| `builtin:sherpa-onnx` | `tts:sherpa-onnx` |
| `builtin:native-os` | `tts:native-os` |
| `cli:piper` | `tts:piper` |
| `http:<host>` (TTS) | `tts:http@<host>` |
| `http` (STT) | `stt:whisper-http@<host>` |
| `http` (image) | `image:http@<host>` |
| `gemini` (image) | `image:gemini` |
| `procedural-art` | `image:procedural-art` |

Historical spend keeps its old key and shows **no price configured**; old rows are
never rewritten or re-priced.
```

- [x] **Step 5: Run the full gate**

Run: `mise run lint && mise run test && mise run build`
Expected: all clean.

- [x] **Step 6: Commit**

```bash
git add pkg/gui/docs pkg/gui/docs_catalogue_test.go docs/releases/provider-keys.md
git commit -m "docs(provider): publish canonical keys and the migration table"
```

---

## Self-Review

**Spec coverage**

| Spec section | Task |
|---|---|
| §3.1 grammar, §3.3 instance keys, §3.4 fallback | 1, 3, 5 |
| §3.2 identity table | 2, 3 |
| §4 resolver and deletions | 3, 9 |
| §5.1 ledger | 4 |
| §5.2 pricing | 5 |
| §5.3 limits | 6 |
| §5.4 failures | 7 |
| §5.6 docs | 10 |
| §5.7 embeddings | 8 |
| §5.8 voice catalogue | 3 (`media.ProviderKey` becomes the canonical instance key; `pkg/gui/service.go:1255,2361` and `pkg/gui/tts_inspect.go:30` keep working unchanged) |
| §6 R1-R6 | 2 (R1, R2), 3 (R3), 9 (R4), 10 (R5), 3+9 (R6) |
| §7 hard cut | 5, 10 |
| §8 data model | 4 (no schema change) |
| §9 error handling | 1, 5 |
| §10 tests | each task |
| §11 rollout | task order |

**Placeholder scan:** no TBD/TODO; every code step shows code; every test step shows the assertion.

**Type consistency:** `provider.Key` is the single key type throughout; resolver names are `KeyFor`, `TTSKeyFor`, `STTKeyFor`, `ImageKeyFor` in every task; `mediaUsage` takes `provider.Key`; `AssignRoleKey` and `ProviderKeyForRole` are spelled the same in Tasks 4, 6, and 7.

**Gap found and fixed inline:** `media.ProviderKey` turned out to be the voice
catalogue and cache name (`pkg/gui/service.go:1255,2361`, `pkg/gui/tts_inspect.go:30`),
not dead code. Task 3 no longer deletes it; it becomes the canonical instance key,
which is exactly what §5.8 requires.

---

## Deviations Taken During Implementation

Both deviations exist so the suite stays green at every commit.

**Tasks 2 and 3 landed as one commit (`215e06e`).** Renaming a descriptor ID
breaks every `provider.Lookup` in the tree until the resolvers return the new
value, so the constants and the resolvers cannot be separated by a green commit.
Task 1 (`dfd0d34`) and Task 4 onwards are separate as planned.

**Embeddings resolve through `embeddings.KeyFor` from the start.** Task 8 as
written expected `geminiembedding` to parse `UsageMetadata`, but `genai`
v1.71.0's `EmbedContentResponse` exposes no usage field, so Gemini embedding rows
record the request count and are marked `estimated`. `openaiembedding` still
reports its parsed token count.

**Task 3 keeps `media.ProviderKey`** rather than deleting it: it names the voice
catalogue and the audio cache, and now returns the canonical instance key. Its
legacy local fallback (`disabled`, `builtin:echo`, a sanitised type) is retained
for configurations with no registered adapter, so two such configurations never
share a cache entry.

**The voice catalogue's persisted `Provider` is the canonical key**, as §5.8
requires; existing voice profiles keep the string they were saved with, which is
a one-time audio cache miss and no more.

**Not met from the spec:** the Gemini embedding token count above. Everything
else in §3 through §11 is implemented.

§10's tests landed under these names, which differ from the plan where a
behaviour is covered by an existing table test rather than a new one:

| §10 test | Actual test |
| --- | --- |
| `TestAllRegistryIDsAreCanonicalKeys` | `TestEveryRegisteredIDIsACanonicalKey` |
| `TestDescriptorKeysAreUniquePerFamily` | `TestDescriptorKeyIsUniquePerFamily` |
| `TestResolverIsTotalForCatalogue` | `TestKeyFor` (per resolver) plus the catalogue freshness test, which resolves every preset |
| `TestResolverMatchesRecordingPaths` | `TestLimitKeyMatchesLedgerKeyForLLM` and `TestLimitKeyFollowsAnInheritChain` |
| `TestInstanceKeyFallbackPrecedence` | `TestResolveFallsBackFromInstanceToAdapter`, `TestResolvePrefersInstanceModelOverAdapterWide` |
| `TestLimitsAndLedgerShareProviderKey` | `TestLimitKeyMatchesLedgerKeyForLLM` |
| `TestInstanceBlocksDoNotLeak` | `TestInstanceBlocksDoNotLeak` |
| `TestEmbeddingUsageIsRecorded` | `TestEmbeddingWorkerReportsUsage`, `TestSearchEntitiesReportsEmbeddingUsage`, `TestEmbeddingUsageIsRecorded` |
| `TestEmbeddingDefaultInstanceKey` | `TestKeyFor` in `pkg/embeddings` |
| `TestBrowserOnlyProvidersHaveNoKey` | `TestSTTKeyForSkipsBrowserOnly` |
| `TestInstanceKeysHaveRegisteredParents` | `TestAllKeysAreRegisteredOrReserved`, `TestInstanceBlocksDoNotLeak` (asserts the parent) |
| `TestNoSecondKeyDerivation` | `TestNoSecondKeyDerivation` |
| `TestPriceConfigRejectsLegacyKey` | `TestValidateRejectsLegacyPriceKey` |
| `TestProviderCatalogueIsCurrent` | `TestProviderCatalogueIsCurrent` |
