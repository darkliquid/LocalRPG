package all_test

import (
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"github.com/darkliquid/localrpg/pkg/provider"
)

// Descriptor Sources are hand-declared catalogue metadata, so they drift from
// the adapter's actual transport. This maps each registered adapter to the only
// Source it may declare. Every registered descriptor must appear in exactly one
// map; a missing entry fails the test rather than being ignored.
var transportByKey = map[provider.Key]string{
	provider.KeyLLMOpenAIChat:      "http",
	provider.KeyLLMGemini:          "gemini",
	provider.KeyLLMCLI:             "cli",
	provider.KeyLLMNarrativeOracle: "builtin",

	provider.KeyTTSGemini:     "gemini",
	provider.KeyTTSElevenLabs: "http",
	provider.KeyTTSNativeOS:   "builtin",
	provider.KeyTTSSherpaONNX: "builtin",
	provider.KeyTTSPiper:      "cli",
	provider.KeyTTSFishAudio:  "http",
	provider.KeyTTSHTTP:       "http",
	provider.KeyTTSCartesia:   "http",

	provider.KeySTTWhisperHTTP: "http",
	provider.KeySTTWhisperCLI:  "cli",
	provider.KeySTTWebSpeech:   "builtin",
	provider.KeySTTCartesia:    "http",

	provider.KeyImageGemini:        "gemini",
	provider.KeyImageHTTP:          "http",
	provider.KeyImageCLI:           "cli",
	provider.KeyImageProceduralArt: "builtin",

	provider.KeyEmbeddingGemini: "gemini",
	provider.KeyEmbeddingOpenAI: "http",
}

func TestDescriptorSourcesMatchTransport(t *testing.T) {
	for _, desc := range provider.List() {
		want, known := transportByKey[provider.Key(desc.ID)]
		if !known {
			// The builtin embedding adapter is reserved but not registered as a
			// descriptor; everything else must be classified.
			if provider.Key(desc.ID) == provider.KeyEmbeddingBuiltin {
				continue
			}
			t.Errorf("%s: not classified in transportByKey", desc.ID)
			continue
		}
		if desc.Source != want {
			t.Errorf("%s: Source = %q, want %q", desc.ID, desc.Source, want)
		}
	}
}
