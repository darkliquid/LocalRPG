package media_test

import (
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func chainConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Media.TTS = config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"}
	cfg.Media.TTSProviders = map[string]config.TTSConfig{
		"npc": {Type: "builtin", BuiltinName: "sherpa-onnx"},
	}
	cfg.Media.Image = config.ImageConfig{Type: "builtin", BuiltinName: "procedural-art"}
	cfg.Media.ImageProviders = map[string]config.ImageConfig{
		"hero": {Type: "gemini"},
	}
	return cfg
}

func TestSelectChainOrdersMediaLocalFirst(t *testing.T) {
	cfg := chainConfig()
	got := media.SelectChain([]string{"default", "npc"}, config.SelectLocalFirst, "", "tts", cfg)
	if len(got) != 2 || got[0] != "npc" {
		t.Fatalf("local-first order = %v", got)
	}
}

func TestSelectChainByTagOrdersMedia(t *testing.T) {
	cfg := chainConfig()
	got := media.SelectChain([]string{"default", "hero"}, config.SelectByTag, string(provider.FeatureKeyRequired), "image", cfg)
	if len(got) != 2 || got[0] != "hero" {
		t.Fatalf("by-tag order = %v", got)
	}
}

func TestSelectChainDropsAnUnknownName(t *testing.T) {
	cfg := chainConfig()
	got := media.SelectChain([]string{"gone", "npc"}, config.SelectFirst, "", "tts", cfg)
	if len(got) != 1 || got[0] != "npc" {
		t.Fatalf("an unknown name must be dropped, got %v", got)
	}
}

func TestSelectChainIsAPermutation(t *testing.T) {
	cfg := chainConfig()
	in := []string{"default", "npc"}
	for _, rule := range []string{config.SelectFirst, config.SelectCheapest, config.SelectLocalFirst, config.SelectByTag} {
		got := media.SelectChain(in, rule, string(provider.FeatureOffline), "tts", cfg)
		if len(got) != len(in) {
			t.Fatalf("%s: %v is not a permutation of %v", rule, got, in)
		}
	}
}

func TestPurposeChainNamesUsesTheDeclaredChain(t *testing.T) {
	cfg := chainConfig()
	cfg.Media.PurposeChains = map[string]config.ChainConfig{
		"scene": {Chain: []string{"default", "hero"}, Select: config.SelectLocalFirst},
	}
	got := media.PurposeChainNames(cfg, config.PurposeScene)
	if len(got) != 2 || got[0] != "default" {
		t.Fatalf("scene chain = %v", got)
	}
}

func TestPurposeChainNamesFallsBackToTheSingleProvider(t *testing.T) {
	cfg := chainConfig()
	cfg.Media.Purposes = map[string]string{"portrait": "hero"}
	got := media.PurposeChainNames(cfg, config.PurposePortrait)
	if len(got) != 1 || got[0] != "hero" {
		t.Fatalf("portrait = %v", got)
	}
}
