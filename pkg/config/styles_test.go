package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStylePackConfigRoundTrips(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("styles:\n  pack: ashen\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.StylePackName() != "ashen" {
		t.Fatalf("pack = %q", cfg.StylePackName())
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	var again Config
	if err := yaml.Unmarshal(data, &again); err != nil {
		t.Fatal(err)
	}
	if again.StylePackName() != "ashen" {
		t.Fatalf("a round trip lost the pack: %q", again.StylePackName())
	}
}

func TestStylePackDefaultsToEmpty(t *testing.T) {
	if DefaultConfig().StylePackName() != "" {
		t.Fatal("an omitted pack means the built-in look")
	}
	var nilConfig *Config
	if nilConfig.StylePackName() != "" {
		t.Fatal("a nil config should report no pack")
	}
}
