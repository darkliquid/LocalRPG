package refsystems

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/systemtest"
)

func TestEverySystemShipsScenarios(t *testing.T) {
	for _, s := range List() {
		if len(ScenarioFiles(s.ID)) == 0 {
			t.Errorf("%s ships no scenarios", s.ID)
		}
	}
}

func TestCorpusScenariosPass(t *testing.T) {
	for _, s := range List() {
		sys := systemtest.System{ID: s.ID, Script: s.Script, Mechanics: s.Mechanics}
		for _, file := range ScenarioFiles(s.ID) {
			scenario, err := systemtest.LoadScenario(file.Data)
			if err != nil {
				t.Errorf("%s/%s: %v", s.ID, file.Name, err)
				continue
			}
			if failures := systemtest.RunAll(sys, []systemtest.Scenario{scenario}); len(failures) > 0 {
				t.Errorf("%s/%s: %+v", s.ID, file.Name, failures)
			}
		}
	}
}
