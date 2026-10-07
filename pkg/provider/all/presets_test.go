package all_test

import (
	"encoding/json"
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestEachFamilyHasPresets(t *testing.T) {
	for _, family := range []provider.Family{
		provider.FamilyLLM, provider.FamilyTTS, provider.FamilySTT, provider.FamilyImage, provider.FamilyEmbedding,
	} {
		count := 0
		for _, desc := range provider.List(family) {
			count += len(desc.Presets)
		}
		if count == 0 {
			t.Errorf("family %s has no presets in the catalogue", family)
		}
	}
}

func TestEveryPresetConfigUnmarshals(t *testing.T) {
	for _, desc := range provider.List() {
		for _, preset := range desc.Presets {
			raw, err := json.Marshal(preset.Config)
			if err != nil {
				t.Fatalf("preset %s/%s: encode: %v", desc.ID, preset.ID, err)
			}
			var target interface{}
			switch desc.Family {
			case provider.FamilyLLM:
				target = &harness.ProviderConfig{}
			case provider.FamilyTTS:
				target = &config.TTSConfig{}
			case provider.FamilySTT:
				target = &config.STTConfig{}
			case provider.FamilyImage:
				target = &config.ImageConfig{}
			case provider.FamilyEmbedding:
				target = &config.EmbeddingProviderConfig{}
			default:
				continue
			}
			if err := json.Unmarshal(raw, target); err != nil {
				t.Errorf("preset %s/%s does not fit its family config: %v", desc.ID, preset.ID, err)
			}
		}
	}
}
