package gui

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/pricing"
	"github.com/darkliquid/localrpg/pkg/provider"
	_ "github.com/darkliquid/localrpg/pkg/provider/all"
)

var updateDocs = flag.Bool("update-docs", false, "rewrite the generated embedded documentation articles")

const providerCataloguePath = "docs/12-provider-catalogue.md"

// renderProviderCatalogue builds the catalogue article from the registered
// provider descriptors, so the documented IDs and models cannot drift from the
// code that ships them.
func renderProviderCatalogue() string {
	var b strings.Builder

	b.WriteString("---\n")
	b.WriteString("id: 12-provider-catalogue\n")
	b.WriteString("title: Provider & Model Catalogue\n")
	b.WriteString("category: Configuration & Providers\n")
	b.WriteString("order: 7\n")
	b.WriteString("description: Every registered provider ID, its presets and models, and the ledger key that prices match against.\n")
	b.WriteString("---\n\n")

	b.WriteString("# Provider & Model Catalogue\n\n")
	b.WriteString("This page is generated from the provider registry, so it always lists the IDs\n")
	b.WriteString("LocalRPG actually ships. Use it when writing a `providers.<id>` block, assigning\n")
	b.WriteString("an agent role, or adding a `providers.prices` entry.\n\n")
	b.WriteString("The **Ledger key** column is the exact `provider` value a metered call records\n")
	b.WriteString("and therefore the value a price must use. It is also the value shown in the\n")
	b.WriteString("Provider column of the Usage tab. A price with no `model` matches every model of\n")
	b.WriteString("that key.\n\n")

	b.WriteString("> [!NOTE]\n")
	b.WriteString("> A provider ID such as `llm:openaichat` is the built-in adapter name, not the key\n")
	b.WriteString("> you choose for `providers.<your-id>`. The ledger key is fixed by the adapter;\n")
	b.WriteString("> the config key under `providers:` is yours to name.\n\n")

	b.WriteString("## Ledger key rules\n\n")
	b.WriteString("| Family | Ledger key | Example |\n")
	b.WriteString("| --- | --- | --- |\n")
	b.WriteString("| LLM | the adapter key | `llm:openaichat`, `llm:gemini` |\n")
	b.WriteString("| Speech (TTS) | `tts:gemini`, `tts:<name>`, `tts:piper@<command>`, `tts:http@<host>` | `tts:http@localhost:8880` |\n")
	b.WriteString("| Transcription (STT) | `stt:whisper-http@<host>`, `stt:whisper-cli@<command>` | `stt:whisper-http@localhost:8000` |\n")
	b.WriteString("| Image | `image:gemini`, `image:http@<host>`, `image:cli@<command>` | `image:http@127.0.0.1:8188` |\n")
	b.WriteString("| Embedding | `embedding:openai`, `embedding:gemini`, `embedding:builtin` | `embedding:gemini@default` |\n\n")

	families := []struct {
		family provider.Family
		title  string
	}{
		{provider.FamilyLLM, "LLM providers"},
		{provider.FamilyTTS, "Speech (TTS) providers"},
		{provider.FamilySTT, "Transcription (STT) providers"},
		{provider.FamilyImage, "Image providers"},
		{provider.FamilyEmbedding, "Embedding providers"},
	}

	for _, entry := range families {
		descs := provider.List(entry.family)
		if len(descs) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", entry.title)
		b.WriteString("| Provider ID | Source | Ledger key | Presets and models |\n")
		b.WriteString("| --- | --- | --- | --- |\n")
		for _, desc := range descs {
			presets := presetSummary(desc)
			fmt.Fprintf(&b, "| `%s` | %s | `%s` | %s |\n",
				desc.ID, desc.Source, ledgerKey(entry.family, desc), presets)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Built-in default prices\n\n")
	b.WriteString("These rates apply when no `providers.prices` entry matches, and only when the\n")
	b.WriteString("ledger key matches exactly. A row with no model applies to every model of that\n")
	b.WriteString("key. Add a config entry to override or extend them. Local and built-in adapters\n")
	b.WriteString("are absent on purpose: they are not metered.\n\n")
	b.WriteString("| Ledger key | Model | Input (per 1M) | Output (per 1M) | Per character | Per request |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, price := range pricingBuiltinPrices() {
		model := price.Model
		if model == "" {
			model = "any"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | %d | %d | %d | %d |\n",
			price.Provider, model, price.PerMillionInput, price.PerMillionOutput, price.PerCharacter, price.PerRequest)
	}
	b.WriteString("\nPrices are micros: one millionth of a currency unit, so `2500000` is 2.50.\n")

	return b.String()
}

// pricingBuiltinPrices is split out so the generator has no deep dependency on
// the pricing package's internals beyond its exported table.
func pricingBuiltinPrices() []config.PriceConfig {
	return pricing.BuiltinPrices
}

func presetSummary(desc provider.Descriptor) string {
	if len(desc.Presets) == 0 {
		return "-"
	}
	seen := make(map[string]bool, len(desc.Presets))
	models := make([]string, 0, len(desc.Presets))
	for _, preset := range desc.Presets {
		model, _ := preset.Config["model"].(string)
		if model == "" {
			model = preset.ID
		}
		if seen[model] {
			continue
		}
		seen[model] = true
		models = append(models, "`"+model+"`")
	}
	sort.Strings(models)
	return strings.Join(models, ", ")
}

// ledgerKey reports the canonical key a descriptor's first preset records
// under, by asking the same resolvers the runtime uses.
func ledgerKey(family provider.Family, desc provider.Descriptor) string {
	if family == provider.FamilyEmbedding {
		// The embedding descriptors are already canonical keys and carry no
		// presets, so the adapter key is the ledger key.
		return desc.ID
	}
	if len(desc.Presets) == 0 {
		return "not reported"
	}
	preset := desc.Presets[0]
	switch family {
	case provider.FamilyLLM:
		key, ok := harness.KeyFor(ProviderConfigFromPreset(preset.Config))
		if ok {
			return string(key)
		}
	case provider.FamilyTTS:
		if key, ok := media.TTSKeyFor(ttsConfigFromPreset(preset)); ok {
			return string(key)
		}
	case provider.FamilySTT:
		if key, ok := media.STTKeyFor(sttConfigFromPreset(preset)); ok {
			return string(key)
		}
	case provider.FamilyImage:
		if key, ok := media.ImageKeyFor(imageConfigFromPreset(preset)); ok {
			return string(key)
		}
	case provider.FamilyEmbedding:
		// The embedding descriptors are already canonical keys and carry no
		// presets, so the adapter key is the ledger key.
		return desc.ID
	}
	return "not reported"
}

// ProviderConfigFromPreset builds a harness provider config from a preset's
// config map, so the catalogue and the runtime resolve the same key.
func ProviderConfigFromPreset(cfg map[string]interface{}) harness.ProviderConfig {
	out := harness.ProviderConfig{}
	if v, ok := cfg["type"].(string); ok {
		out.Type = v
	}
	if v, ok := cfg["builtin_name"].(string); ok {
		out.BuiltinName = v
	}
	if v, ok := cfg["command"].(string); ok {
		out.Command = v
	}
	if v, ok := cfg["endpoint"].(string); ok {
		out.Endpoint = v
	}
	return out
}

func ttsConfigFromPreset(preset provider.Preset) config.TTSConfig {
	cfg := config.TTSConfig{}
	if v, ok := preset.Config["type"].(string); ok {
		cfg.Type = v
	}
	if v, ok := preset.Config["builtin_name"].(string); ok {
		cfg.BuiltinName = v
	}
	if v, ok := preset.Config["command"].(string); ok {
		cfg.Command = v
	}
	if v, ok := preset.Config["endpoint"].(string); ok {
		cfg.Endpoint = v
	}
	return cfg
}

func sttConfigFromPreset(preset provider.Preset) config.STTConfig {
	cfg := config.STTConfig{}
	if v, ok := preset.Config["type"].(string); ok {
		cfg.Type = v
	}
	if v, ok := preset.Config["builtin_name"].(string); ok {
		cfg.BuiltinName = v
	}
	if v, ok := preset.Config["command"].(string); ok {
		cfg.Command = v
	}
	if v, ok := preset.Config["endpoint"].(string); ok {
		cfg.Endpoint = v
	}
	return cfg
}

func imageConfigFromPreset(preset provider.Preset) config.ImageConfig {
	cfg := config.ImageConfig{}
	if v, ok := preset.Config["type"].(string); ok {
		cfg.Type = v
	}
	if v, ok := preset.Config["builtin_name"].(string); ok {
		cfg.BuiltinName = v
	}
	if v, ok := preset.Config["command"].(string); ok {
		cfg.Command = v
	}
	if v, ok := preset.Config["endpoint"].(string); ok {
		cfg.Endpoint = v
	}
	return cfg
}

func TestProviderCatalogueIsCurrent(t *testing.T) {
	assertGeneratedDoc(t, providerCataloguePath, renderProviderCatalogue())
}

// assertGeneratedDoc writes the file when -update-docs is set and otherwise
// fails when it has drifted from what the generator produces.
func assertGeneratedDoc(t *testing.T, path, want string) {
	t.Helper()
	if *updateDocs {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s (regenerate with -update-docs): %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s is stale; regenerate with:\n  go test ./pkg/gui -update-docs", path)
	}
}
