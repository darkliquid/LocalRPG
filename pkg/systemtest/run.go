package systemtest

import (
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/rules"
)

// System is the runner's view of a system, so it accepts a reference system or a
// studio draft without an import cycle.
type System struct {
	ID        string
	Script    string
	Mechanics *core.MechanicsSpec
}

// Failure is one expectation a scenario did not meet.
type Failure struct {
	Scenario string `json:"scenario"`
	Step     int    `json:"step"`
	Detail   string `json:"detail"`
}

// Run loads a system into a fresh engine with a deterministic bridge, executes
// every step, and returns the expectations that failed. A script that will not
// load is a failure, not a panic.
func Run(system System, scenario Scenario) []Failure {
	bridge := NewBridge(system.Mechanics, scenario.Seed, scenario.Setup.Player)
	engine := rules.NewJSEngine(bridge)
	engine.SetManifest(&core.SystemManifest{ID: system.ID, Mechanics: system.Mechanics})
	if err := engine.LoadScript(system.Script); err != nil {
		return []Failure{{Scenario: scenario.Name, Detail: "load script: " + err.Error()}}
	}

	var failures []Failure
	for i, step := range scenario.Steps {
		result, err := engine.ExecuteAction(step.Action, map[string]interface{}{
			"action": step.Input,
			"player": PlayerID,
			"turn":   1,
		})
		if err != nil {
			failures = append(failures, Failure{Scenario: scenario.Name, Step: i + 1, Detail: "action: " + err.Error()})
			continue
		}
		if step.Expect.Empty() {
			continue
		}
		failures = append(failures, checkStep(scenario.Name, i+1, bridge, result, step.Expect)...)
	}
	return failures
}

// RunAll runs every scenario for a system.
func RunAll(system System, scenarios []Scenario) []Failure {
	var failures []Failure
	for _, scenario := range scenarios {
		failures = append(failures, Run(system, scenario)...)
	}
	return failures
}

func checkStep(scenario string, step int, bridge *Bridge, result *rules.ActionResult, expect Expectations) []Failure {
	if result == nil {
		result = &rules.ActionResult{}
	}
	var failures []Failure
	add := func(format string, args ...interface{}) {
		failures = append(failures, Failure{Scenario: scenario, Step: step, Detail: fmt.Sprintf(format, args...)})
	}
	if expect.Outcome != "" && result.Outcome != expect.Outcome {
		add("outcome = %q, want %q", result.Outcome, expect.Outcome)
	}
	if expect.Total != nil {
		if result.Roll == nil {
			add("no roll to check total %d..%d", expect.Total.Min, expect.Total.Max)
		} else if result.Roll.Total < expect.Total.Min || result.Roll.Total > expect.Total.Max {
			add("total = %d, want %d..%d", result.Roll.Total, expect.Total.Min, expect.Total.Max)
		}
	}
	if expect.MessageContains != "" && !strings.Contains(result.Message, expect.MessageContains) {
		add("message %q does not contain %q", result.Message, expect.MessageContains)
	}
	for path, want := range expect.State {
		got, _ := bridge.StateValue(path)
		if !valuesEqual(got, want) {
			add("state %s = %v, want %v", path, got, want)
		}
	}
	return failures
}

func valuesEqual(got, want interface{}) bool {
	if gn, ok := toFloat(got); ok {
		if wn, ok := toFloat(want); ok {
			return gn == wn
		}
	}
	return fmt.Sprint(got) == fmt.Sprint(want)
}

func toFloat(value interface{}) (float64, bool) {
	switch n := value.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}
