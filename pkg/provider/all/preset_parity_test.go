package all_test

import (
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// catalogueOnlyPresets are catalog presets that deliberately have no config
// table entry, with the reason. Keep this list short and explicit.
var catalogueOnlyPresets = map[string]string{}

func TestConfigPresetsMatchCatalogPresets(t *testing.T) {
	for _, desc := range provider.List() {
		for _, preset := range desc.Presets {
			key := string(desc.ID) + "/" + preset.ID
			if reason, ok := catalogueOnlyPresets[key]; ok {
				_ = reason
				continue
			}
			var found bool
			switch desc.Family {
			case provider.FamilyLLM:
				_, found = config.AgentPresets[preset.ID]
			case provider.FamilyTTS:
				_, found = config.TTSPresets[preset.ID]
			case provider.FamilySTT:
				_, found = config.STTPresets[preset.ID]
			case provider.FamilyImage:
				_, found = config.ImagePresets[preset.ID]
			default:
				continue
			}
			if !found {
				t.Errorf("catalog preset %s has no config table entry", key)
			}
		}
	}
}
