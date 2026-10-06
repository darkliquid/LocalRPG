// Package systemtest runs deterministic scenarios against a system's mechanics,
// so a system author can assert "a do action on a Might 2 character yields a weak
// hit" without a store, a provider, or a network.
package systemtest

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Scenario is one deterministic test of a system's mechanics.
type Scenario struct {
	Name  string    `yaml:"name" json:"name"`
	Seed  int64     `yaml:"seed" json:"seed"`
	Setup SetupSpec `yaml:"setup,omitempty" json:"setup,omitempty"`
	Steps []Step    `yaml:"steps" json:"steps"`
}

// SetupSpec is the initial state a scenario runs against.
type SetupSpec struct {
	Player SetupEntity `yaml:"player,omitempty" json:"player,omitempty"`
}

// SetupEntity is one entity's starting state and tags.
type SetupEntity struct {
	Stats map[string]interface{} `yaml:"stats,omitempty" json:"stats,omitempty"`
	Tags  []string               `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// Step is one action in a scenario and what it must produce.
type Step struct {
	Action string       `yaml:"action" json:"action"`
	Input  string       `yaml:"input,omitempty" json:"input,omitempty"`
	Expect Expectations `yaml:"expect,omitempty" json:"expect,omitempty"`
}

// Expectations are the assertions a step must satisfy. An empty Expectations
// makes the step a setup step: it runs and asserts nothing.
type Expectations struct {
	Outcome         string                 `yaml:"outcome,omitempty" json:"outcome,omitempty"`
	Total           *Range                 `yaml:"total,omitempty" json:"total,omitempty"`
	State           map[string]interface{} `yaml:"state,omitempty" json:"state,omitempty"`
	MessageContains string                 `yaml:"message_contains,omitempty" json:"message_contains,omitempty"`
}

// Empty reports whether the expectations assert anything.
func (e Expectations) Empty() bool {
	return e.Outcome == "" && e.Total == nil && len(e.State) == 0 && e.MessageContains == ""
}

// Range is an inclusive numeric range.
type Range struct {
	Min int `yaml:"min" json:"min"`
	Max int `yaml:"max" json:"max"`
}

// LoadScenario parses one scenario and rejects one with no steps, which could
// never fail and so would be a false pass.
func LoadScenario(data []byte) (Scenario, error) {
	var s Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		return Scenario{}, fmt.Errorf("parse scenario: %w", err)
	}
	if len(s.Steps) == 0 {
		return Scenario{}, fmt.Errorf("scenario %q has no steps", s.Name)
	}
	return s, nil
}
