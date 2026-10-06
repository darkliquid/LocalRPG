package config

import "testing"

func TestInteractiveRollsDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.InteractiveRolls() != "continuation" {
		t.Fatalf("default = %q", cfg.InteractiveRolls())
	}
	cfg.Interactive.Rolls = "single-turn"
	if cfg.InteractiveRolls() != "single-turn" {
		t.Fatalf("override = %q", cfg.InteractiveRolls())
	}
	cfg.Interactive.Rolls = "nonsense"
	if cfg.InteractiveRolls() != "continuation" {
		t.Fatalf("unknown value should normalise, got %q", cfg.InteractiveRolls())
	}
}

func TestImageTriggerDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ImageTrigger() != "significant" {
		t.Fatalf("default = %q", cfg.ImageTrigger())
	}
	cfg.Media.Image.Trigger = "every_turn"
	if cfg.ImageTrigger() != "every_turn" {
		t.Fatalf("override = %q", cfg.ImageTrigger())
	}
	cfg.Media.Image.Trigger = "nonsense"
	if cfg.ImageTrigger() != "significant" {
		t.Fatalf("unknown value should normalise, got %q", cfg.ImageTrigger())
	}
}
