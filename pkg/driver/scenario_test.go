package driver_test

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/driver"
)

func TestParseScenario(t *testing.T) {
	yamlContent := `
name: "Smoke Test"
description: "Verify launcher navigation and game start"
setup:
  world: "valeria"
  system: "dnd5e"
steps:
  - action: "navigate"
    url: "/"
  - action: "wait_visible"
    selector: "[data-testid='campaign-card']"
    timeout_ms: 3000
  - action: "click"
    selector: "[data-testid='campaign-card']"
  - action: "type"
    selector: "input[type='text']"
    text: "I look around the tavern."
  - action: "assert_visible"
    selector: ".turn-narrative"
`

	sc, err := driver.ParseScenario(strings.NewReader(yamlContent))
	if err != nil {
		t.Fatalf("ParseScenario failed: %v", err)
	}

	if sc.Name != "Smoke Test" {
		t.Errorf("Expected name 'Smoke Test', got %s", sc.Name)
	}
	if len(sc.Steps) != 5 {
		t.Fatalf("Expected 5 steps, got %d", len(sc.Steps))
	}
	if sc.Steps[0].Action != driver.ActionNavigate || sc.Steps[0].URL != "/" {
		t.Errorf("Unexpected step 0: %+v", sc.Steps[0])
	}
	if sc.Steps[1].TimeoutMs != 3000 {
		t.Errorf("Expected timeout 3000ms, got %d", sc.Steps[1].TimeoutMs)
	}
}

func TestValidateScenario(t *testing.T) {
	badYaml := `
name: ""
steps: []
`
	_, err := driver.ParseScenario(strings.NewReader(badYaml))
	if err == nil {
		t.Errorf("Expected error for empty scenario name and steps")
	}
}
