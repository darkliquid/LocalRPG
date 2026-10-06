package media

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/trace"
)

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
	if len(r.Names()) != 2 || r.Names()[0] != "default" {
		t.Fatalf("names = %v", r.Names())
	}
}

func TestTTSRegistryCaches(t *testing.T) {
	cfg := &config.Config{}
	cfg.Media.TTS = config.TTSConfig{Type: "builtin", BuiltinName: "echo"}
	r := NewTTSRegistry(cfg, trace.OrNil(nil))
	first, err := r.For("")
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.For("")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("a repeated For call should return the cached client")
	}
	r.Invalidate()
	if _, err := r.For(""); err != nil {
		t.Fatalf("For after Invalidate: %v", err)
	}
}

func TestImageRegistryBuildsPerName(t *testing.T) {
	cfg := &config.Config{}
	cfg.Media.Image = config.ImageConfig{Type: "builtin", BuiltinName: "procedural-art"}
	cfg.Media.ImageProviders = map[string]config.ImageConfig{
		"maps": {Type: "builtin", BuiltinName: "procedural-art"},
	}
	r := NewImageRegistry(cfg, trace.OrNil(nil))
	if _, err := r.Default(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.For("maps"); err != nil {
		t.Fatal(err)
	}
	if len(r.Names()) != 2 {
		t.Fatalf("names = %v", r.Names())
	}
}

func TestSTTRegistryResolvesTheDefault(t *testing.T) {
	cfg := &config.Config{}
	cfg.Media.STT = config.STTConfig{Type: "http", Endpoint: "http://localhost:8000"}
	r := NewSTTRegistry(cfg)
	if _, err := r.Default(); err != nil {
		t.Fatal(err)
	}
	if len(r.Names()) != 1 {
		t.Fatalf("names = %v", r.Names())
	}
}
