package all_test

import (
	"context"
	"encoding/json"
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// llmDerivableFeatures are the llm features the capability derivation can prove
// from an adapter's interfaces.
var llmDerivableFeatures = map[provider.Feature]func(harness.Capabilities) bool{
	provider.FeatureStreaming:    func(c harness.Capabilities) bool { return c.Streaming },
	provider.FeatureTools:        func(c harness.Capabilities) bool { return c.Tools },
	provider.FeatureThinking:     func(c harness.Capabilities) bool { return c.Thinking },
	provider.FeatureSessions:     func(c harness.Capabilities) bool { return c.Sessions },
	provider.FeatureContextCache: func(c harness.Capabilities) bool { return c.ContextCache },
}

// ttsDerivableFeatures are the tts features the capability derivation can prove.
var ttsDerivableFeatures = map[provider.Feature]func(media.Capabilities) bool{
	provider.FeatureVoiceCatalog:     func(c media.Capabilities) bool { return c.VoiceCatalog },
	provider.FeatureVoiceOptions:     func(c media.Capabilities) bool { return c.VoiceOptions },
	provider.FeatureSpeechCues:       func(c media.Capabilities) bool { return c.SpeechCues },
	provider.FeatureMarkdownEmphasis: func(c media.Capabilities) bool { return c.MarkdownEmphasis },
	provider.FeatureMetered:          func(c media.Capabilities) bool { return c.Metered },
	provider.FeatureExtendedVoices:   func(c media.Capabilities) bool { return c.ExtendedVoices },
}

func TestLLMDescriptorsBuildAndFeaturesAreBacked(t *testing.T) {
	descs := provider.List(provider.FamilyLLM)
	if len(descs) == 0 {
		t.Fatal("expected at least one registered llm provider")
	}
	for _, desc := range descs {
		desc := desc
		t.Run(desc.ID, func(t *testing.T) {
			reg, ok := provider.Lookup(desc.ID)
			if !ok {
				t.Fatalf("descriptor %q has no registration", desc.ID)
			}
			raw, err := json.Marshal(harness.ModelBuildPayload{Config: harness.ProviderConfig{Type: "http", APIKey: "test-key"}})
			if err != nil {
				t.Fatalf("encode config: %v", err)
			}
			built, err := reg.Build(context.Background(), raw)
			if err != nil {
				t.Fatalf("build %s: %v", desc.ID, err)
			}
			model, ok := built.(harness.ModelProvider)
			if !ok {
				t.Fatalf("provider %q is not a model provider", desc.ID)
			}
			caps := harness.Describe(model)
			for _, feature := range desc.Features {
				check, derivable := llmDerivableFeatures[feature]
				if derivable && !check(caps) {
					t.Errorf("provider %q declares %q but the adapter does not back it", desc.ID, feature)
				}
			}
		})
	}
}

func TestTTSDescriptorsBuildAndFeaturesAreBacked(t *testing.T) {
	descs := provider.List(provider.FamilyTTS)
	if len(descs) == 0 {
		t.Fatal("expected at least one registered tts provider")
	}
	for _, desc := range descs {
		desc := desc
		t.Run(desc.ID, func(t *testing.T) {
			reg, ok := provider.Lookup(desc.ID)
			if !ok {
				t.Fatalf("descriptor %q has no registration", desc.ID)
			}
			payload := media.TTSBuildPayload{Config: config.TTSConfig{APIKey: "test-key"}}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("encode config: %v", err)
			}
			built, err := reg.Build(context.Background(), raw)
			if err != nil {
				t.Fatalf("build %s: %v", desc.ID, err)
			}
			client, ok := built.(media.TTSClient)
			if !ok {
				t.Fatalf("provider %q is not a tts client", desc.ID)
			}
			caps := media.Describe(client)
			for _, feature := range desc.Features {
				check, derivable := ttsDerivableFeatures[feature]
				if derivable && !check(caps) {
					t.Errorf("provider %q declares %q but the adapter does not back it", desc.ID, feature)
				}
			}
		})
	}
}

func TestSTTAndImageDescriptorsBuild(t *testing.T) {
	for _, family := range []provider.Family{provider.FamilySTT, provider.FamilyImage} {
		descs := provider.List(family)
		if len(descs) == 0 {
			t.Fatalf("expected a registered %s provider", family)
		}
		for _, desc := range descs {
			desc := desc
			t.Run(string(family)+"/"+desc.ID, func(t *testing.T) {
				reg, ok := provider.Lookup(desc.ID)
				if !ok {
					t.Fatalf("descriptor %q has no registration", desc.ID)
				}
				var raw []byte
				var err error
				if family == provider.FamilySTT {
					raw, err = json.Marshal(media.STTBuildPayload{Config: config.STTConfig{APIKey: "test-key"}})
				} else {
					raw, err = json.Marshal(media.ImageBuildPayload{Config: config.ImageConfig{APIKey: "test-key"}})
				}
				if err != nil {
					t.Fatalf("encode config: %v", err)
				}
				built, err := reg.Build(context.Background(), raw)
				if err != nil {
					t.Fatalf("build %s: %v", desc.ID, err)
				}
				switch family {
				case provider.FamilySTT:
					if _, ok := built.(media.STTClient); !ok {
						t.Fatalf("provider %q is not an stt client", desc.ID)
					}
				case provider.FamilyImage:
					if _, ok := built.(media.ImageClient); !ok {
						t.Fatalf("provider %q is not an image client", desc.ID)
					}
				}
			})
		}
	}
}
