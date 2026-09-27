package core

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadSystemManifestMechanics(t *testing.T) {
	dir := t.TempDir()
	yamlDoc := `id: narrative
name: Narrative
mechanics:
  stats:
    - {id: might, type: number, default: 0}
  skills:
    - {id: athletics, label: Athletics, stat: might}
  health:
    stat: hp
    max_stat: hp_max
  checks:
    notation: "2d6"
    outcome: [pass, fail]
    difficulty:
      - {id: hard, target: 10}
`
	path := filepath.Join(dir, "system.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadSystemManifest(path)
	if err != nil {
		t.Fatalf("LoadSystemManifest: %v", err)
	}
	if m.Mechanics == nil || len(m.Mechanics.Stats) != 1 || m.Mechanics.Stats[0].ID != "might" {
		t.Fatalf("mechanics = %+v", m.Mechanics)
	}
	if m.Mechanics.Checks.Notation != "2d6" || len(m.Mechanics.Checks.Difficulty) != 1 {
		t.Fatalf("checks = %+v", m.Mechanics.Checks)
	}
}

func TestLoadSystemManifestWithoutMechanics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "system.yaml")
	if err := os.WriteFile(path, []byte("id: plain\nname: Plain\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadSystemManifest(path)
	if err != nil {
		t.Fatalf("LoadSystemManifest: %v", err)
	}
	if m.Mechanics != nil {
		t.Fatalf("mechanics = %+v, want nil", m.Mechanics)
	}
}

func TestAdvancementSpecRoundTrips(t *testing.T) {
	raw := `
id: sys
name: Sys
mechanics:
  stats:
    - { id: xp, label: Experience, type: number, default: 0 }
  advancement:
    currency: { stat: xp, label: Experience }
    mode: spend
    earn:
      - { on: miss, amount: 1 }
      - { on: check_outcome, outcome: strong, amount: 2 }
    unlocks:
      - id: stat-increase
        label: Increase a stat
        cost: 5
        effects:
          - { type: stat_increase, amount: 1, max: 18 }
    levels:
      - { at: 300, label: "Level 2", effects: [{ type: stat_increase, amount: 1 }] }
`
	var manifest SystemManifest
	if err := yaml.Unmarshal([]byte(raw), &manifest); err != nil {
		t.Fatal(err)
	}
	adv := manifest.Mechanics.Advancement
	if adv == nil {
		t.Fatal("advancement block did not parse")
	}
	if adv.Currency.Stat != "xp" || adv.Mode != "spend" || len(adv.Earn) != 2 {
		t.Fatalf("advancement = %+v", adv)
	}
	if len(adv.Unlocks) != 1 || adv.Unlocks[0].Effects[0].Max != 18 {
		t.Fatalf("unlocks = %+v", adv.Unlocks)
	}
	if len(adv.Levels) != 1 || adv.Levels[0].At != 300 {
		t.Fatalf("levels = %+v", adv.Levels)
	}
}
