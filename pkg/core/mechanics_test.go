package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSystemManifestMechanics(t *testing.T) {
	dir := t.TempDir()
	yaml := `id: narrative
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
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
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
