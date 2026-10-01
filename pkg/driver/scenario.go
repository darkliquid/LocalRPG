package driver

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseScenario deserializes and validates a scenario YAML stream.
func ParseScenario(r io.Reader) (*Scenario, error) {
	var s Scenario
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("driver: decode scenario yaml: %w", err)
	}

	if s.Name == "" {
		return nil, fmt.Errorf("driver: scenario requires a name")
	}
	if len(s.Steps) == 0 {
		return nil, fmt.Errorf("driver: scenario must contain at least one step")
	}

	for i, step := range s.Steps {
		switch step.Action {
		case ActionNavigate:
			if step.URL == "" {
				return nil, fmt.Errorf("driver: step %d (navigate) requires 'url'", i)
			}
		case ActionClick, ActionWaitVisible, ActionAssertVisible:
			if step.Selector == "" {
				return nil, fmt.Errorf("driver: step %d (%s) requires 'selector'", i, step.Action)
			}
		case ActionTypeInput:
			if step.Selector == "" || step.Text == "" {
				return nil, fmt.Errorf("driver: step %d (type) requires both 'selector' and 'text'", i)
			}
		case ActionAssertTurnOutcome:
			if step.Expected == "" {
				return nil, fmt.Errorf("driver: step %d (assert_turn_outcome) requires 'expected'", i)
			}
		case ActionScreenshot:
			if step.Path == "" {
				return nil, fmt.Errorf("driver: step %d (screenshot) requires 'path'", i)
			}
			switch strings.ToLower(step.Format) {
			case "", "png", "jpeg", "jpg":
				// allowed
			default:
				return nil, fmt.Errorf("driver: step %d (screenshot) has unknown format '%s'", i, step.Format)
			}
		case ActionFaultInjection, ActionSleep:
			// allowed
		default:
			return nil, fmt.Errorf("driver: step %d has unknown action '%s'", i, step.Action)
		}
	}

	return &s, nil
}
