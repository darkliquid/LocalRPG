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

func TestParseScenarioScreenshot(t *testing.T) {
	yamlContent := `
name: "Showcase Capture"
steps:
  - action: "navigate"
    url: "/"
  - action: "screenshot"
    path: "website/screenshots/01-launcher.png"
`

	sc, err := driver.ParseScenario(strings.NewReader(yamlContent))
	if err != nil {
		t.Fatalf("ParseScenario failed: %v", err)
	}
	if len(sc.Steps) != 2 {
		t.Fatalf("Expected 2 steps, got %d", len(sc.Steps))
	}
	if sc.Steps[1].Action != driver.ActionScreenshot {
		t.Errorf("Expected a screenshot action, got %q", sc.Steps[1].Action)
	}
	if sc.Steps[1].Path != "website/screenshots/01-launcher.png" {
		t.Errorf("Unexpected screenshot path: %q", sc.Steps[1].Path)
	}
}

func TestScreenshotRequiresPath(t *testing.T) {
	badYaml := `
name: "Missing Path"
steps:
  - action: "screenshot"
`
	if _, err := driver.ParseScenario(strings.NewReader(badYaml)); err == nil {
		t.Error("Expected an error for a screenshot step without a path")
	}
}
